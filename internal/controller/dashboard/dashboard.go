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
	"github.com/crossplane/provider-gf/internal/controller/common"
)

const (
	errNotDashboard          = "managed resource is not a Dashboard custom resource"
	errTrackPCUsage          = "cannot track ProviderConfig usage"
	errGetPC                 = "cannot get ProviderConfig"
	errNewClient             = "cannot create Grafana client"
	errInvalidExternalName   = "invalid external name format, expected <orgId>:<uid>"
	errInvalidExternalNameV2 = "invalid external name format for v2, expected <orgId>:<apiVersion>:<name>"
	errResolveOrgRef         = "cannot resolve organization reference"
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

// formatExternalNameV2 creates an external name for V2 dashboards in the format <orgId>:<apiVersion>:<name>.
// Note: namespace is derived from orgID using grafana.OrgIDToNamespace(), so it's not stored in the external name.
func formatExternalNameV2(orgID int64, apiVersion, name string) string {
	return strconv.FormatInt(orgID, 10) + ":" + apiVersion + ":" + name
}

// parseExternalNameV2 parses an external name in the format <orgId>:<apiVersion>:<name>.
// Returns orgID, apiVersion, name, and an error if the format is invalid.
// Note: namespace should be derived from orgID using grafana.OrgIDToNamespace().
func parseExternalNameV2(externalName string) (int64, string, string, error) {
	parts := strings.SplitN(externalName, ":", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return 0, "", "", errors.New(errInvalidExternalNameV2)
	}
	orgID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", "", errors.Wrap(err, errInvalidExternalNameV2)
	}
	return orgID, parts[1], parts[2], nil
}

// isV2ExternalName checks if the external name is in V2 format (has 3 parts with apiVersion pattern).
// V1 format: <orgId>:<uid> (2 parts)
// V2 format: <orgId>:<apiVersion>:<name> (3 parts, apiVersion matches v*alpha* or v*beta* pattern)
func isV2ExternalName(externalName string) bool {
	parts := strings.SplitN(externalName, ":", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return false
	}
	// Check if the second part looks like a Grafana API version (v0alpha1, v1beta1, v2beta1, etc.)
	apiVersion := parts[1]
	return strings.HasPrefix(apiVersion, "v") &&
		(strings.Contains(apiVersion, "alpha") || strings.Contains(apiVersion, "beta"))
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

	// Resolve orgID from OrgRef/OrgSelector, direct OrgID, or ProviderConfig
	orgID, err := common.ResolveOrgID(ctx, c.kube, cr,
		cr.Spec.ForProvider.OrgRef,
		cr.Spec.ForProvider.OrgSelector,
		cr.Spec.ForProvider.OrgID,
		pcSpec.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, errResolveOrgRef)
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

	// Check if using V2 API format
	configJSON := []byte(cr.Spec.ForProvider.ConfigJSON)
	if grafana.IsDashboardV2Format(configJSON) {
		return e.observeV2(ctx, cr, configJSON)
	}

	return e.observeV1(ctx, cr)
}

// observeV1 handles observation for legacy dashboard API format.
func (e *external) observeV1(ctx context.Context, cr *v1alpha1.Dashboard) (managed.ExternalObservation, error) { //nolint:gocyclo // acceptable complexity for observe logic
	// Get the external name (format: <orgId>:<uid>)
	externalName := meta.GetExternalName(cr)

	// Try to determine the UID to look up
	var uid string

	if externalName != "" && !isV2ExternalName(externalName) {
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
	isUpToDate, err := e.isUpToDateV1(cr.Spec.ForProvider.ConfigJSON, observedJSON)
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

// observeV2 handles observation for K8s-style dashboard API format.
func (e *external) observeV2(ctx context.Context, cr *v1alpha1.Dashboard, configJSON []byte) (managed.ExternalObservation, error) { //nolint:gocyclo // acceptable complexity for observe logic
	// Parse the desired dashboard
	desiredDash, err := grafana.ParseDashboardV2(configJSON)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot parse dashboard v2 configJson")
	}

	// Get API version from the desired dashboard
	apiVersion := grafana.GetDashboardV2APIVersion(desiredDash)

	// Derive namespace from orgID (OSS/On-Premise: org 1 = "default", org N = "org-N")
	namespace := grafana.OrgIDToNamespace(e.orgID)

	// Get the external name (format: <orgId>:<apiVersion>:<name>)
	externalName := meta.GetExternalName(cr)

	var name string

	if externalName != "" && isV2ExternalName(externalName) {
		// Parse the external name to extract apiVersion and name
		_, extAPIVersion, extName, err := parseExternalNameV2(externalName)
		if err == nil {
			apiVersion = extAPIVersion
			name = extName
		} else {
			// Fall back to name from configJson
			name = desiredDash.Metadata.Name
		}
	} else {
		// Use name from configJson for recovery
		name = desiredDash.Metadata.Name
	}

	// If we don't have a name, resource doesn't exist yet
	if name == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Fetch the dashboard from Grafana
	dash, err := e.client.GetDashboardV2ByName(ctx, apiVersion, namespace, name)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get dashboard v2 from Grafana")
	}

	if dash == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// If we found the resource but external-name wasn't set (recovery from create-pending),
	// set it now so the reconciler can proceed normally
	observedAPIVersion := grafana.GetDashboardV2APIVersion(&dash.DashboardV2)
	expectedExtName := formatExternalNameV2(e.orgID, observedAPIVersion, dash.Metadata.Name)
	if externalName == "" || meta.GetExternalName(cr) != expectedExtName {
		meta.SetExternalName(cr, expectedExtName)
	}

	// Update status with observed values
	cr.Status.AtProvider.UID = &dash.Metadata.Name // In V2, name is the unique identifier
	folderUID := grafana.GetDashboardV2FolderUID(&dash.DashboardV2)
	cr.Status.AtProvider.Folder = &folderUID
	cr.Status.AtProvider.Version = &dash.Metadata.Generation

	// Store the observed config JSON
	observedJSON, err := json.Marshal(dash.DashboardV2)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot marshal observed dashboard v2")
	}
	cr.Status.AtProvider.ConfigJSON = string(observedJSON)

	// Check if the dashboard is up to date by comparing the spec
	isUpToDate, err := e.isUpToDateV2(desiredDash, &dash.DashboardV2)
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

// isUpToDateV1 compares the desired and observed dashboard JSON for legacy API.
func (e *external) isUpToDateV1(desired, observed string) (bool, error) {
	var desiredMap, observedMap map[string]any
	if err := json.Unmarshal([]byte(desired), &desiredMap); err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(observed), &observedMap); err != nil {
		return false, err
	}

	// Remove fields that are managed by Grafana and shouldn't be compared
	removeGrafanaManagedFieldsV1(desiredMap)
	removeGrafanaManagedFieldsV1(observedMap)

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

// removeGrafanaManagedFieldsV1 removes fields that are managed by Grafana for legacy API.
func removeGrafanaManagedFieldsV1(m map[string]any) {
	// These fields are managed by Grafana and should not be compared
	delete(m, "id")
	delete(m, "version")
	delete(m, "uid") // UID might be set by Grafana if not provided
}

// isUpToDateV2 compares the desired and observed dashboard for K8s-style API.
func (e *external) isUpToDateV2(desired, observed *grafana.DashboardV2) (bool, error) {
	// Compare the spec only - metadata is managed by Grafana
	desiredSpec, err := json.Marshal(desired.Spec)
	if err != nil {
		return false, err
	}
	observedSpec, err := json.Marshal(observed.Spec)
	if err != nil {
		return false, err
	}

	if !bytes.Equal(desiredSpec, observedSpec) {
		return false, nil
	}

	// Also compare relevant annotations (folder, message)
	desiredFolder := ""
	if desired.Metadata.Annotations != nil {
		desiredFolder = desired.Metadata.Annotations[grafana.DashboardV2AnnotationFolder]
	}
	observedFolder := ""
	if observed.Metadata.Annotations != nil {
		observedFolder = observed.Metadata.Annotations[grafana.DashboardV2AnnotationFolder]
	}
	if desiredFolder != observedFolder {
		return false, nil
	}

	return true, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Dashboard)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotDashboard)
	}

	cr.Status.SetConditions(xpv1.Creating())

	// Check if using V2 API format
	configJSON := []byte(cr.Spec.ForProvider.ConfigJSON)
	if grafana.IsDashboardV2Format(configJSON) {
		return e.createV2(ctx, cr, configJSON)
	}

	return e.createV1(ctx, cr)
}

// createV1 creates a dashboard using the legacy API.
func (e *external) createV1(ctx context.Context, cr *v1alpha1.Dashboard) (managed.ExternalCreation, error) {
	// Parse the config JSON to inject/extract UID
	var dashData map[string]any
	if err := json.Unmarshal([]byte(cr.Spec.ForProvider.ConfigJSON), &dashData); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot parse dashboard configJson")
	}

	// If external name is set, parse it to get the UID
	if externalName := meta.GetExternalName(cr); externalName != "" && !isV2ExternalName(externalName) {
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

// createV2 creates a dashboard using the K8s-style V2 API.
func (e *external) createV2(ctx context.Context, cr *v1alpha1.Dashboard, configJSON []byte) (managed.ExternalCreation, error) {
	// Parse the dashboard
	dash, err := grafana.ParseDashboardV2(configJSON)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot parse dashboard v2 configJson")
	}

	// Set namespace from orgID (OSS/On-Premise: org 1 = "default", org N = "org-N")
	dash.Metadata.Namespace = grafana.OrgIDToNamespace(e.orgID)

	// Apply folder from spec if not set in annotations
	if cr.Spec.ForProvider.Folder != nil {
		if dash.Metadata.Annotations == nil {
			dash.Metadata.Annotations = make(map[string]string)
		}
		if dash.Metadata.Annotations[grafana.DashboardV2AnnotationFolder] == "" {
			dash.Metadata.Annotations[grafana.DashboardV2AnnotationFolder] = *cr.Spec.ForProvider.Folder
		}
	}

	// Apply message from spec if not set in annotations
	if cr.Spec.ForProvider.Message != nil {
		if dash.Metadata.Annotations == nil {
			dash.Metadata.Annotations = make(map[string]string)
		}
		if dash.Metadata.Annotations[grafana.DashboardV2AnnotationMessage] == "" {
			dash.Metadata.Annotations[grafana.DashboardV2AnnotationMessage] = *cr.Spec.ForProvider.Message
		}
	}

	resp, err := e.client.CreateDashboardV2(ctx, dash)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create dashboard v2 in Grafana")
	}

	// Set the external name in format <orgId>:<apiVersion>:<name>
	apiVersion := grafana.GetDashboardV2APIVersion(&resp.DashboardV2)
	meta.SetExternalName(cr, formatExternalNameV2(e.orgID, apiVersion, resp.Metadata.Name))

	// Update status
	cr.Status.AtProvider.UID = &resp.Metadata.Name
	folderUID := grafana.GetDashboardV2FolderUID(&resp.DashboardV2)
	cr.Status.AtProvider.Folder = &folderUID
	cr.Status.AtProvider.Version = &resp.Metadata.Generation

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Dashboard)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotDashboard)
	}

	// Check if using V2 API format
	configJSON := []byte(cr.Spec.ForProvider.ConfigJSON)
	if grafana.IsDashboardV2Format(configJSON) {
		return e.updateV2(ctx, cr, configJSON)
	}

	return e.updateV1(ctx, cr)
}

// updateV1 updates a dashboard using the legacy API.
func (e *external) updateV1(ctx context.Context, cr *v1alpha1.Dashboard) (managed.ExternalUpdate, error) {
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

// updateV2 updates a dashboard using the K8s-style V2 API.
func (e *external) updateV2(ctx context.Context, cr *v1alpha1.Dashboard, configJSON []byte) (managed.ExternalUpdate, error) { //nolint:gocyclo // acceptable complexity for update logic
	// Parse the dashboard
	dash, err := grafana.ParseDashboardV2(configJSON)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse dashboard v2 configJson")
	}

	// Set namespace from orgID (OSS/On-Premise: org 1 = "default", org N = "org-N")
	dash.Metadata.Namespace = grafana.OrgIDToNamespace(e.orgID)

	// Parse the external name to get apiVersion and name
	externalName := meta.GetExternalName(cr)
	if externalName != "" && isV2ExternalName(externalName) {
		_, apiVersion, name, err := parseExternalNameV2(externalName)
		if err == nil {
			// Update the apiVersion in the dashboard to match the external name
			dash.APIVersion = "dashboard.grafana.app/" + apiVersion
			dash.Metadata.Name = name
		}
	}

	// Apply folder from spec if not set in annotations
	if cr.Spec.ForProvider.Folder != nil {
		if dash.Metadata.Annotations == nil {
			dash.Metadata.Annotations = make(map[string]string)
		}
		if dash.Metadata.Annotations[grafana.DashboardV2AnnotationFolder] == "" {
			dash.Metadata.Annotations[grafana.DashboardV2AnnotationFolder] = *cr.Spec.ForProvider.Folder
		}
	}

	// Apply message from spec if not set in annotations
	if cr.Spec.ForProvider.Message != nil {
		if dash.Metadata.Annotations == nil {
			dash.Metadata.Annotations = make(map[string]string)
		}
		if dash.Metadata.Annotations[grafana.DashboardV2AnnotationMessage] == "" {
			dash.Metadata.Annotations[grafana.DashboardV2AnnotationMessage] = *cr.Spec.ForProvider.Message
		}
	}

	// Set resourceVersion for optimistic concurrency if we have it from status
	if cr.Status.AtProvider.Version != nil {
		dash.Metadata.ResourceVersion = strconv.FormatInt(*cr.Status.AtProvider.Version, 10)
	}

	resp, err := e.client.UpdateDashboardV2(ctx, dash)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update dashboard v2 in Grafana")
	}

	// Update status
	cr.Status.AtProvider.UID = &resp.Metadata.Name
	folderUID := grafana.GetDashboardV2FolderUID(&resp.DashboardV2)
	cr.Status.AtProvider.Folder = &folderUID
	cr.Status.AtProvider.Version = &resp.Metadata.Generation

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

	// Check if using V2 API format based on external name format
	if isV2ExternalName(externalName) {
		return e.deleteV2(ctx, externalName)
	}

	return e.deleteV1(ctx, externalName)
}

// deleteV1 deletes a dashboard using the legacy API.
func (e *external) deleteV1(ctx context.Context, externalName string) (managed.ExternalDelete, error) {
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

// deleteV2 deletes a dashboard using the K8s-style V2 API.
func (e *external) deleteV2(ctx context.Context, externalName string) (managed.ExternalDelete, error) {
	// Parse the external name to get orgID, apiVersion, and name
	orgID, apiVersion, name, err := parseExternalNameV2(externalName)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot parse external name v2")
	}

	// Derive namespace from orgID (OSS/On-Premise: org 1 = "default", org N = "org-N")
	namespace := grafana.OrgIDToNamespace(orgID)

	if err := e.client.DeleteDashboardV2ByName(ctx, apiVersion, namespace, name); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete dashboard v2 from Grafana")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}
