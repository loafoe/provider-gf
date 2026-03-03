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

package folderpermission

import (
	"context"
	"sort"
	"strconv"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reference"
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
	errNotFolderPermission = "managed resource is not a FolderPermission custom resource"
	errTrackPCUsage        = "cannot track ProviderConfig usage"
	errGetPC               = "cannot get ProviderConfig"
	errNewClient           = "cannot create Grafana client"
	errInvalidExternalName = "invalid external name format, expected <orgId>:<folderUid>"
	errResolveOrgRef       = "cannot resolve organization reference"
	errResolveFolderRef    = "cannot resolve folder reference"
)

// formatExternalName creates an external name in the format <orgId>:<folderUid>.
func formatExternalName(orgID int64, folderUID string) string {
	return strconv.FormatInt(orgID, 10) + ":" + folderUID
}

// parseExternalName parses an external name in the format <orgId>:<folderUid>.
func parseExternalName(externalName string) (int64, string, error) { //nolint:unparam
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

// ExtractFolderUID extracts the folder UID from a Folder resource's external name.
func ExtractFolderUID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		folder, ok := mg.(*v1alpha1.Folder)
		if !ok {
			return ""
		}
		externalName := meta.GetExternalName(folder)
		if externalName == "" {
			return ""
		}
		// External name format is <orgId>:<uid>
		parts := strings.SplitN(externalName, ":", 2)
		if len(parts) != 2 {
			return ""
		}
		return parts[1]
	}
}

// SetupGated adds a controller that reconciles FolderPermission managed resources.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup FolderPermission controller"))
		}
	}, v1alpha1.FolderPermissionGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles FolderPermission managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.FolderPermissionGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.FolderPermissionList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.FolderPermissionGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.FolderPermission{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.FolderPermission)
	if !ok {
		return nil, errors.New(errNotFolderPermission)
	}

	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	m := mg.(resource.ModernManaged)
	ref := m.GetProviderConfigReference()

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: m.GetNamespace()}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}
	pcSpec := pc.Spec

	cfg := grafana.Config{URL: pcSpec.URL, OrgID: pcSpec.OrgID}

	switch pcSpec.Credentials.AuthType {
	case apisv1alpha1.AuthTypeBasic:
		if pcSpec.Credentials.BasicAuth == nil {
			return nil, errors.New("basicAuth is required when authType is basic")
		}
		username, err := c.getSecretValue(ctx, m.GetNamespace(), pcSpec.Credentials.BasicAuth.UsernameSecretRef)
		if err != nil {
			return nil, errors.Wrap(err, "cannot get username from secret")
		}
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

	// Resolve folderUID from FolderRef/FolderSelector or direct FolderUID
	folderUID, err := c.resolveFolderUID(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, errResolveFolderRef)
	}

	return &external{client: gfClient, kube: c.kube, orgID: orgID, folderUID: folderUID}, nil
}

func (c *connector) getSecretValue(ctx context.Context, namespace string, ref xpv1.SecretKeySelector) (string, error) {
	secretRef := ref
	if secretRef.Namespace == "" {
		secretRef.Namespace = namespace
	}
	data, err := resource.CommonCredentialExtractor(ctx, xpv1.CredentialsSourceSecret, c.kube, xpv1.CommonCredentialSelectors{SecretRef: &secretRef})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// extractUIDFromExternalName attempts to extract the UID from an external name.
func extractUIDFromExternalName(cr resource.Managed) string {
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return ""
	}
	_, uid, err := parseExternalName(externalName)
	if err != nil {
		return ""
	}
	return uid
}

func (c *connector) resolveFolderUID(ctx context.Context, cr *v1alpha1.FolderPermission) (string, error) {
	// If direct UID is provided, use it
	if cr.Spec.ForProvider.FolderUID != nil && *cr.Spec.ForProvider.FolderUID != "" {
		return *cr.Spec.ForProvider.FolderUID, nil
	}

	// Try to resolve from reference
	if cr.Spec.ForProvider.FolderRef != nil || cr.Spec.ForProvider.FolderSelector != nil {
		rsp, err := reference.NewAPIResolver(c.kube, cr).Resolve(ctx, reference.ResolutionRequest{
			CurrentValue: "",
			Reference:    cr.Spec.ForProvider.FolderRef,
			Selector:     cr.Spec.ForProvider.FolderSelector,
			To:           reference.To{Managed: &v1alpha1.Folder{}, List: &v1alpha1.FolderList{}},
			Extract:      ExtractFolderUID(),
			Namespace:    cr.GetNamespace(),
		})
		if err == nil && rsp.ResolvedValue != "" {
			return rsp.ResolvedValue, nil
		}
		// If resolution fails but we have an external name (e.g., during deletion), extract UID from it
		if uid := extractUIDFromExternalName(cr); uid != "" {
			return uid, nil
		}
		if err != nil {
			return "", errors.Wrap(err, "cannot resolve folder reference")
		}
	}

	// Fallback: try to extract from external name (for deletion scenarios)
	if uid := extractUIDFromExternalName(cr); uid != "" {
		return uid, nil
	}

	return "", errors.New("folderUid must be specified via folderUid, folderRef, or folderSelector")
}

type external struct {
	client    *grafana.Client
	kube      client.Client
	orgID     int64
	folderUID string
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.FolderPermission)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotFolderPermission)
	}

	// If no folder UID resolved, resource cannot exist
	if e.folderUID == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Check if the folder exists by trying to get its permissions
	permissions, err := e.client.GetFolderPermissions(ctx, e.folderUID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get folder permissions")
	}
	if permissions == nil {
		// Folder doesn't exist
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Check if external name is set - if not and we have no desired permissions,
	// the resource doesn't exist yet (fresh create scenario)
	externalName := meta.GetExternalName(cr)
	expectedExternalName := formatExternalName(e.orgID, e.folderUID)

	// If we have no external name and no desired permissions, resource doesn't exist
	if externalName == "" && len(cr.Spec.ForProvider.Permissions) == 0 {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// For deletion: if the CR has a deletion timestamp and permissions are empty,
	// consider the resource deleted
	if cr.GetDeletionTimestamp() != nil && len(permissions) == 0 {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Set external name if not set (recovery from create-pending)
	if externalName == "" || externalName != expectedExternalName {
		meta.SetExternalName(cr, expectedExternalName)
	}

	// Update status with observed values
	cr.Status.AtProvider.FolderUID = &e.folderUID
	cr.Status.AtProvider.OrgID = &e.orgID
	cr.Status.AtProvider.ID = &expectedExternalName

	// Convert observed permissions to observation format
	observedPerms := make([]v1alpha1.FolderPermissionItemObservation, 0, len(permissions))
	for _, p := range permissions {
		permName := grafana.PermissionLevelToName(p.Permission)
		obs := v1alpha1.FolderPermissionItemObservation{
			Permission: &permName,
		}
		if p.Role != "" {
			obs.Role = &p.Role
		}
		if p.TeamID != 0 {
			obs.TeamID = &p.TeamID
		}
		if p.UserID != 0 {
			obs.UserID = &p.UserID
		}
		observedPerms = append(observedPerms, obs)
	}
	cr.Status.AtProvider.Permissions = observedPerms

	// Check if up to date by comparing desired vs observed permissions
	isUpToDate := e.isUpToDate(cr, permissions)

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate,
	}, nil
}

func (e *external) isUpToDate(cr *v1alpha1.FolderPermission, observed []grafana.FolderPermissionItem) bool {
	desired := cr.Spec.ForProvider.Permissions

	// Build sets for comparison
	desiredSet := make(map[string]int) // key -> permission level
	for _, p := range desired {
		key := e.permissionKey(p.Role, p.TeamID, p.UserID)
		desiredSet[key] = grafana.PermissionNameToLevel(p.Permission)
	}

	observedSet := make(map[string]int)
	for _, p := range observed {
		var role *string
		if p.Role != "" {
			role = &p.Role
		}
		var teamID *int64
		if p.TeamID != 0 {
			teamID = &p.TeamID
		}
		var userID *int64
		if p.UserID != 0 {
			userID = &p.UserID
		}
		key := e.permissionKey(role, teamID, userID)
		observedSet[key] = p.Permission
	}

	// Compare sets
	if len(desiredSet) != len(observedSet) {
		return false
	}

	for key, desiredLevel := range desiredSet {
		observedLevel, exists := observedSet[key]
		if !exists || desiredLevel != observedLevel {
			return false
		}
	}

	return true
}

func (e *external) permissionKey(role *string, teamID *int64, userID *int64) string {
	parts := []string{""}
	if role != nil {
		parts = append(parts, "role:"+*role)
	}
	if teamID != nil {
		parts = append(parts, "team:"+strconv.FormatInt(*teamID, 10))
	}
	if userID != nil {
		parts = append(parts, "user:"+strconv.FormatInt(*userID, 10))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.FolderPermission)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotFolderPermission)
	}

	cr.Status.SetConditions(xpv1.Creating())

	// Build permission request
	req := e.buildPermissionRequest(cr)

	// Set permissions
	if err := e.client.SetFolderPermissions(ctx, e.folderUID, req); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot set folder permissions")
	}

	// Set external name
	meta.SetExternalName(cr, formatExternalName(e.orgID, e.folderUID))

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.FolderPermission)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotFolderPermission)
	}

	// Build permission request
	req := e.buildPermissionRequest(cr)

	// Set permissions (replaces all existing)
	if err := e.client.SetFolderPermissions(ctx, e.folderUID, req); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot set folder permissions")
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) buildPermissionRequest(cr *v1alpha1.FolderPermission) grafana.FolderPermissionRequest {
	items := make([]grafana.FolderPermissionRequestItem, 0, len(cr.Spec.ForProvider.Permissions))

	for _, p := range cr.Spec.ForProvider.Permissions {
		item := grafana.FolderPermissionRequestItem{
			Permission: grafana.PermissionNameToLevel(p.Permission),
		}
		if p.Role != nil {
			item.Role = *p.Role
		}
		if p.TeamID != nil {
			item.TeamID = *p.TeamID
		}
		if p.UserID != nil {
			item.UserID = *p.UserID
		}
		items = append(items, item)
	}

	return grafana.FolderPermissionRequest{Items: items}
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.FolderPermission)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotFolderPermission)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	// Set empty permissions to remove all
	req := grafana.FolderPermissionRequest{Items: []grafana.FolderPermissionRequestItem{}}

	if err := e.client.SetFolderPermissions(ctx, e.folderUID, req); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot remove folder permissions")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}
