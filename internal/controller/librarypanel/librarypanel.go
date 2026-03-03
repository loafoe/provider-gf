/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package librarypanel

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-gf/apis/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
)

const (
	errNotLibraryPanel     = "managed resource is not a LibraryPanel custom resource"
	errTrackPCUsage        = "cannot track ProviderConfig usage"
	errGetPC               = "cannot get ProviderConfig"
	errNewClient           = "cannot create Grafana client"
	errInvalidExternalName = "invalid external name format, expected <orgId>:<uid>"
)

// formatExternalName creates an external name in the format <orgId>:<uid>.
func formatExternalName(orgID int64, uid string) string {
	return strconv.FormatInt(orgID, 10) + ":" + uid
}

// parseExternalName parses an external name in the format <orgId>:<uid>.
// Returns orgID, uid, and an error if the format is invalid.
func parseExternalName(externalName string) (int64, string, error) { //nolint:unparam // orgID may be used in future
	parts := strings.SplitN(externalName, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return 0, "", errors.New(errInvalidExternalName)
	}
	orgID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", errors.Wrap(err, errInvalidExternalName)
	}
	return orgID, parts[1], nil
}

// SetupGated adds a controller that reconciles LibraryPanel managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup LibraryPanel controller"))
		}
	}, v1alpha1.LibraryPanelGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles LibraryPanel managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.LibraryPanelGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithExternalConnector(&connector{
			kube:  mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(o.ChangeLogOptions.ChangeLogger))
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.LibraryPanelList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.LibraryPanelList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.LibraryPanelGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.LibraryPanel{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// connector is expected to produce an ExternalClient when its Connect method is called.
type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

// Connect produces an ExternalClient for the LibraryPanel resource.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.LibraryPanel)
	if !ok {
		return nil, errors.New(errNotLibraryPanel)
	}

	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	// Get ProviderConfig reference
	m := mg.(resource.ModernManaged)
	ref := m.GetProviderConfigReference()

	// Get the ProviderConfig (namespaced, same namespace as the resource)
	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: m.GetNamespace()}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}
	pcSpec := pc.Spec

	// Build Grafana client config
	cfg := grafana.Config{
		URL:   pcSpec.URL,
		OrgID: pcSpec.OrgID,
	}

	// Get credentials based on auth type
	switch pcSpec.Credentials.AuthType {
	case apisv1alpha1.AuthTypeBasic:
		if pcSpec.Credentials.BasicAuth == nil {
			return nil, errors.New("basicAuth is required when authType is basic")
		}
		// Get username
		username, err := c.getSecretValue(ctx, m.GetNamespace(), pcSpec.Credentials.BasicAuth.UsernameSecretRef)
		if err != nil {
			return nil, errors.Wrap(err, "cannot get username from secret")
		}
		// Get password
		password, err := c.getSecretValue(ctx, m.GetNamespace(), pcSpec.Credentials.BasicAuth.PasswordSecretRef)
		if err != nil {
			return nil, errors.Wrap(err, "cannot get password from secret")
		}
		cfg.Username = username
		cfg.Password = password

	case apisv1alpha1.AuthTypeToken:
		if pcSpec.Credentials.TokenAuth == nil {
			return nil, errors.New("tokenAuth is required when authType is token")
		}
		// Get token
		token, err := c.getSecretValue(ctx, m.GetNamespace(), pcSpec.Credentials.TokenAuth.TokenSecretRef)
		if err != nil {
			return nil, errors.Wrap(err, "cannot get token from secret")
		}
		cfg.Token = token

	default:
		return nil, errors.Errorf("unsupported auth type: %s", pcSpec.Credentials.AuthType)
	}

	gfClient, err := grafana.NewClient(cfg)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	// Default to orgID 1 if not specified
	orgID := int64(1)
	if pcSpec.OrgID != nil {
		orgID = *pcSpec.OrgID
	}

	return &external{client: gfClient, orgID: orgID}, nil
}

// getSecretValue gets a value from a secret.
func (c *connector) getSecretValue(ctx context.Context, namespace string, ref xpv1.SecretKeySelector) (string, error) {
	// If namespace is not specified in the secret ref, use the resource namespace
	secretRef := ref
	if secretRef.Namespace == "" {
		secretRef.Namespace = namespace
	}

	data, err := resource.CommonCredentialExtractor(ctx, xpv1.CredentialsSourceSecret, c.kube, xpv1.CommonCredentialSelectors{
		SecretRef: &secretRef,
	})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// external observes, then either creates, updates, or deletes an external resource.
type external struct {
	client *grafana.Client
	orgID  int64
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) { //nolint:gocyclo // acceptable complexity for observe logic
	cr, ok := mg.(*v1alpha1.LibraryPanel)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotLibraryPanel)
	}

	// Get the external name (format: <orgId>:<uid>)
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Parse the external name to extract the UID
	// If the external name doesn't match the expected format, the resource hasn't been created yet
	_, uid, err := parseExternalName(externalName)
	if err != nil {
		// External name doesn't match our format - resource doesn't exist yet
		return managed.ExternalObservation{ResourceExists: false}, nil //nolint:nilerr // intentional: invalid format means resource not yet created
	}

	// Fetch the library panel from Grafana
	lp, err := e.client.GetLibraryPanelByUID(ctx, uid)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get library panel from Grafana")
	}

	if lp == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Update status with observed values
	cr.Status.AtProvider.ID = &lp.ID
	cr.Status.AtProvider.UID = &lp.UID
	cr.Status.AtProvider.Type = &lp.Type
	cr.Status.AtProvider.Version = &lp.Version

	// Extract description from model if available
	var modelData map[string]any
	if err := json.Unmarshal(lp.Model, &modelData); err == nil {
		if desc, ok := modelData["description"].(string); ok {
			cr.Status.AtProvider.Description = &desc
		}
		if id, ok := modelData["id"].(float64); ok {
			panelID := int64(id)
			cr.Status.AtProvider.PanelID = &panelID
		}
	}

	// Set metadata fields
	if lp.Meta != nil {
		cr.Status.AtProvider.FolderName = &lp.Meta.FolderName
		cr.Status.AtProvider.FolderUID = &lp.Meta.FolderUID
		cr.Status.AtProvider.Created = &lp.Meta.Created
		cr.Status.AtProvider.Updated = &lp.Meta.Updated
		cr.Status.AtProvider.ConnectedDashboards = &lp.Meta.ConnectedDashboards
	}

	// Check if the library panel is up to date
	isUpToDate, err := e.isUpToDate(cr, lp)
	if err != nil {
		isUpToDate = false
	}

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  isUpToDate,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// isUpToDate compares the desired and observed library panel state.
func (e *external) isUpToDate(cr *v1alpha1.LibraryPanel, lp *grafana.LibraryPanel) (bool, error) {
	fp := cr.Spec.ForProvider

	// Check name
	if fp.Name != lp.Name {
		return false, nil
	}

	// Check folder UID
	if fp.FolderUID != nil && lp.Meta != nil && *fp.FolderUID != lp.Meta.FolderUID {
		return false, nil
	}

	// Compare model JSON
	var desiredModel, observedModel map[string]any
	if err := json.Unmarshal([]byte(fp.ModelJSON), &desiredModel); err != nil {
		return false, err
	}
	if err := json.Unmarshal(lp.Model, &observedModel); err != nil {
		return false, err
	}

	// Remove fields managed by Grafana
	removeGrafanaManagedFields(desiredModel)
	removeGrafanaManagedFields(observedModel)

	desiredBytes, err := json.Marshal(desiredModel)
	if err != nil {
		return false, err
	}
	observedBytes, err := json.Marshal(observedModel)
	if err != nil {
		return false, err
	}

	return bytes.Equal(desiredBytes, observedBytes), nil
}

// removeGrafanaManagedFields removes fields that are managed by Grafana.
func removeGrafanaManagedFields(m map[string]any) {
	delete(m, "id")
	delete(m, "libraryPanel")
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.LibraryPanel)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotLibraryPanel)
	}

	cr.Status.SetConditions(xpv1.Creating())

	fp := cr.Spec.ForProvider

	// Parse the model JSON
	var modelData map[string]any
	if err := json.Unmarshal([]byte(fp.ModelJSON), &modelData); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot parse modelJson")
	}

	// Remove id field if present (Grafana will assign one)
	delete(modelData, "id")

	modelJSON, err := json.Marshal(modelData)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot marshal model JSON")
	}

	req := grafana.LibraryPanelCreateRequest{
		Name:  fp.Name,
		Model: modelJSON,
		Kind:  1, // 1 = panel
	}

	if fp.UID != nil {
		req.UID = *fp.UID
	}

	if fp.FolderUID != nil {
		req.FolderUID = *fp.FolderUID
	}

	resp, err := e.client.CreateLibraryPanel(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create library panel in Grafana")
	}

	// Set the external name in format <orgId>:<uid>
	meta.SetExternalName(cr, formatExternalName(e.orgID, resp.UID))

	// Update status
	cr.Status.AtProvider.ID = &resp.ID
	cr.Status.AtProvider.UID = &resp.UID
	cr.Status.AtProvider.Version = &resp.Version

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.LibraryPanel)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotLibraryPanel)
	}

	// Parse the external name to get the UID
	externalName := meta.GetExternalName(cr)
	_, uid, err := parseExternalName(externalName)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse external name")
	}

	fp := cr.Spec.ForProvider

	// Parse the model JSON
	var modelData map[string]any
	if err := json.Unmarshal([]byte(fp.ModelJSON), &modelData); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse modelJson")
	}

	// Remove id field if present
	delete(modelData, "id")

	modelJSON, err := json.Marshal(modelData)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot marshal model JSON")
	}

	// Get current version
	version := int64(1)
	if cr.Status.AtProvider.Version != nil {
		version = *cr.Status.AtProvider.Version
	}

	req := grafana.LibraryPanelUpdateRequest{
		UID:     uid,
		Name:    fp.Name,
		Model:   modelJSON,
		Kind:    1, // 1 = panel
		Version: version,
	}

	if fp.FolderUID != nil {
		req.FolderUID = *fp.FolderUID
	}

	resp, err := e.client.UpdateLibraryPanel(ctx, uid, req)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update library panel in Grafana")
	}

	// Update status
	cr.Status.AtProvider.ID = &resp.ID
	cr.Status.AtProvider.UID = &resp.UID
	cr.Status.AtProvider.Version = &resp.Version

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.LibraryPanel)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotLibraryPanel)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		// No external name means nothing to delete
		return managed.ExternalDelete{}, nil
	}

	// Parse the external name to get the UID
	_, uid, err := parseExternalName(externalName)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot parse external name")
	}

	if err := e.client.DeleteLibraryPanelByUID(ctx, uid); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete library panel from Grafana")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}
