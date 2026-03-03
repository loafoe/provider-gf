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

package dashboard

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
	errNotDashboard        = "managed resource is not a Dashboard custom resource"
	errTrackPCUsage        = "cannot track ProviderConfig usage"
	errGetPC               = "cannot get ProviderConfig"
	errGetCreds            = "cannot get credentials"
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

// SetupGated adds a controller that reconciles Dashboard managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Dashboard controller"))
		}
	}, v1alpha1.DashboardGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Dashboard managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.DashboardGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.DashboardList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.DashboardList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.DashboardGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Dashboard{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// connector is expected to produce an ExternalClient when its Connect method is called.
type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

// Connect produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a Grafana client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo // acceptable complexity for controller connect logic
	cr, ok := mg.(*v1alpha1.Dashboard)
	if !ok {
		return nil, errors.New(errNotDashboard)
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
	cr, ok := mg.(*v1alpha1.Dashboard)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotDashboard)
	}

	// Get the external name (format: <orgId>:<uid>)
	externalName := meta.GetExternalName(cr)

	// Try to determine the UID to look up
	var uid string

	if externalName != "" {
		// Parse the external name to extract the UID
		_, parsedUID, err := parseExternalName(externalName)
		if err == nil {
			uid = parsedUID
		}
	}

	// If we couldn't get UID from external-name, but user specified one in configJson,
	// try to recover from external-create-pending race condition by looking up by spec UID
	if uid == "" {
		var configData map[string]any
		if err := json.Unmarshal([]byte(cr.Spec.ForProvider.ConfigJSON), &configData); err == nil {
			if specUID, ok := configData["uid"].(string); ok && specUID != "" {
				uid = specUID
			}
		}
	}

	// If we still don't have a UID, resource doesn't exist yet
	if uid == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Fetch the dashboard from Grafana
	dash, err := e.client.GetDashboardByUID(ctx, uid)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get dashboard from Grafana")
	}

	if dash == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// If we found the resource but external-name wasn't set (recovery from create-pending),
	// set it now so the reconciler can proceed normally
	// Extract UID from dashboard response
	var dashUID string
	var dashData map[string]any
	if err := json.Unmarshal(dash.Dashboard, &dashData); err == nil {
		if u, ok := dashData["uid"].(string); ok {
			dashUID = u
		}
	}
	if dashUID != "" && (externalName == "" || meta.GetExternalName(cr) != formatExternalName(e.orgID, dashUID)) {
		meta.SetExternalName(cr, formatExternalName(e.orgID, dashUID))
	}

	// Update status with observed values
	if dashUID != "" {
		cr.Status.AtProvider.UID = &dashUID
	} else {
		cr.Status.AtProvider.UID = &uid
	}
	cr.Status.AtProvider.URL = &dash.Meta.URL
	cr.Status.AtProvider.Version = &dash.Meta.Version
	cr.Status.AtProvider.Folder = &dash.Meta.FolderUID

	// Extract dashboard ID from the dashboard JSON (reuse dashData from above)
	if id, ok := dashData["id"].(float64); ok {
		idVal := int64(id)
		cr.Status.AtProvider.DashboardID = &idVal
	}

	// Store the observed config JSON
	observedJSON := string(dash.Dashboard)
	cr.Status.AtProvider.ConfigJSON = observedJSON

	// Check if the dashboard is up to date by comparing the JSON
	// We need to normalize both JSONs for comparison
	isUpToDate, err := e.isUpToDate(cr.Spec.ForProvider.ConfigJSON, observedJSON)
	if err != nil {
		// If we can't compare, assume it's not up to date
		isUpToDate = false
	}

	// Check if folder matches
	if cr.Spec.ForProvider.Folder != nil && *cr.Spec.ForProvider.Folder != dash.Meta.FolderUID {
		isUpToDate = false
	}

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  isUpToDate,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// isUpToDate compares the desired and observed dashboard JSON.
func (e *external) isUpToDate(desired, observed string) (bool, error) {
	var desiredMap, observedMap map[string]any
	if err := json.Unmarshal([]byte(desired), &desiredMap); err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(observed), &observedMap); err != nil {
		return false, err
	}

	// Remove fields that are managed by Grafana and shouldn't be compared
	removeGrafanaManagedFields(desiredMap)
	removeGrafanaManagedFields(observedMap)

	// Compare the normalized JSON
	desiredNorm, err := json.Marshal(desiredMap)
	if err != nil {
		return false, err
	}
	observedNorm, err := json.Marshal(observedMap)
	if err != nil {
		return false, err
	}

	return bytes.Equal(desiredNorm, observedNorm), nil
}

// removeGrafanaManagedFields removes fields that are managed by Grafana.
func removeGrafanaManagedFields(m map[string]any) {
	// These fields are managed by Grafana and should not be compared
	delete(m, "id")
	delete(m, "version")
	delete(m, "uid") // UID might be set by Grafana if not provided
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Dashboard)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotDashboard)
	}

	cr.Status.SetConditions(xpv1.Creating())

	// Parse the config JSON to inject/extract UID
	var dashData map[string]any
	if err := json.Unmarshal([]byte(cr.Spec.ForProvider.ConfigJSON), &dashData); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot parse dashboard configJson")
	}

	// If external name is set, parse it to get the UID
	if externalName := meta.GetExternalName(cr); externalName != "" {
		_, uid, err := parseExternalName(externalName)
		if err == nil {
			dashData["uid"] = uid
		}
	}

	// Set id to null for new dashboards
	dashData["id"] = nil

	dashJSON, err := json.Marshal(dashData)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot marshal dashboard JSON")
	}

	overwrite := false
	if cr.Spec.ForProvider.Overwrite != nil {
		overwrite = *cr.Spec.ForProvider.Overwrite
	}

	req := grafana.DashboardCreateRequest{
		Dashboard: dashJSON,
		Overwrite: overwrite,
	}

	if cr.Spec.ForProvider.Folder != nil {
		req.FolderUID = *cr.Spec.ForProvider.Folder
	}
	if cr.Spec.ForProvider.Message != nil {
		req.Message = *cr.Spec.ForProvider.Message
	}

	resp, err := e.client.CreateOrUpdateDashboard(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create dashboard in Grafana")
	}

	// Set the external name in format <orgId>:<uid>
	meta.SetExternalName(cr, formatExternalName(e.orgID, resp.UID))

	// Update status
	cr.Status.AtProvider.UID = &resp.UID
	cr.Status.AtProvider.URL = &resp.URL
	cr.Status.AtProvider.Version = &resp.Version
	cr.Status.AtProvider.DashboardID = &resp.ID
	id := strconv.FormatInt(resp.ID, 10)
	cr.Status.AtProvider.ID = &id

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Dashboard)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotDashboard)
	}

	// Parse the config JSON
	var dashData map[string]any
	if err := json.Unmarshal([]byte(cr.Spec.ForProvider.ConfigJSON), &dashData); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse dashboard configJson")
	}

	// Parse the external name to get the UID
	externalName := meta.GetExternalName(cr)
	_, uid, err := parseExternalName(externalName)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse external name")
	}
	dashData["uid"] = uid

	// Get current version if we have it
	if cr.Status.AtProvider.Version != nil {
		dashData["version"] = *cr.Status.AtProvider.Version
	}

	dashJSON, err := json.Marshal(dashData)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot marshal dashboard JSON")
	}

	overwrite := false
	if cr.Spec.ForProvider.Overwrite != nil {
		overwrite = *cr.Spec.ForProvider.Overwrite
	}

	req := grafana.DashboardCreateRequest{
		Dashboard: dashJSON,
		Overwrite: overwrite,
	}

	if cr.Spec.ForProvider.Folder != nil {
		req.FolderUID = *cr.Spec.ForProvider.Folder
	}
	if cr.Spec.ForProvider.Message != nil {
		req.Message = *cr.Spec.ForProvider.Message
	}

	resp, err := e.client.CreateOrUpdateDashboard(ctx, req)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update dashboard in Grafana")
	}

	// Update status
	cr.Status.AtProvider.UID = &resp.UID
	cr.Status.AtProvider.URL = &resp.URL
	cr.Status.AtProvider.Version = &resp.Version
	cr.Status.AtProvider.DashboardID = &resp.ID

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Dashboard)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotDashboard)
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

	if err := e.client.DeleteDashboardByUID(ctx, uid); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete dashboard from Grafana")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}
