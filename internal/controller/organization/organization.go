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

package organization

import (
	"context"
	"strconv"

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
	errNotOrganization = "managed resource is not an Organization custom resource"
	errTrackPCUsage    = "cannot track ProviderConfig usage"
	errGetPC           = "cannot get ProviderConfig"
	errNewClient       = "cannot create Grafana client"
)

// SetupGated adds a controller that reconciles Organization managed resources.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Organization controller"))
		}
	}, v1alpha1.OrganizationGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Organization managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.OrganizationGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.OrganizationList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.OrganizationGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Organization{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.Organization)
	if !ok {
		return nil, errors.New(errNotOrganization)
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

	return &external{client: gfClient}, nil
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

type external struct {
	client *grafana.Client
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.Organization)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotOrganization)
	}

	externalName := meta.GetExternalName(cr)
	var orgID int64

	if externalName != "" {
		parsedID, err := strconv.ParseInt(externalName, 10, 64)
		if err == nil {
			orgID = parsedID
		}
	}

	// Recovery: try to find by name if external-name not available
	if orgID == 0 {
		org, err := e.client.GetOrganizationByName(ctx, cr.Spec.ForProvider.Name)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, "cannot get organization by name from Grafana")
		}
		if org != nil {
			orgID = org.ID
			meta.SetExternalName(cr, strconv.FormatInt(orgID, 10))
		}
	}

	if orgID == 0 {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	org, err := e.client.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get organization from Grafana")
	}
	if org == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Set external name if recovered
	if externalName == "" || meta.GetExternalName(cr) != strconv.FormatInt(org.ID, 10) {
		meta.SetExternalName(cr, strconv.FormatInt(org.ID, 10))
	}

	// Get current org users
	users, err := e.client.GetOrgUsers(ctx, org.ID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get organization users from Grafana")
	}

	// Update status
	cr.Status.AtProvider.ID = &org.ID
	cr.Status.AtProvider.Name = &org.Name

	// Categorize users by role
	var adminUsers, editorUsers, viewerUsers []int64
	for _, u := range users {
		switch u.Role {
		case "Admin":
			adminUsers = append(adminUsers, u.UserID)
		case "Editor":
			editorUsers = append(editorUsers, u.UserID)
		case "Viewer":
			viewerUsers = append(viewerUsers, u.UserID)
		}
	}
	cr.Status.AtProvider.AdminUsers = adminUsers
	cr.Status.AtProvider.EditorUsers = editorUsers
	cr.Status.AtProvider.ViewerUsers = viewerUsers

	cr.Status.SetConditions(xpv1.Available())

	// Check if up to date
	isUpToDate, err := e.isUpToDate(ctx, cr, org, users)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot check if organization is up to date")
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate,
	}, nil
}

func (e *external) isUpToDate(ctx context.Context, cr *v1alpha1.Organization, org *grafana.Organization, users []grafana.OrgUser) (bool, error) { //nolint:gocyclo
	fp := cr.Spec.ForProvider

	// Check name
	if fp.Name != org.Name {
		return false, nil
	}

	// If no users are specified, don't check user membership
	if len(fp.Admins) == 0 && len(fp.Editors) == 0 && len(fp.Viewers) == 0 {
		return true, nil
	}

	// Build current user maps by role
	currentAdmins := make(map[string]int64)
	currentEditors := make(map[string]int64)
	currentViewers := make(map[string]int64)

	for _, u := range users {
		switch u.Role {
		case "Admin":
			currentAdmins[u.Email] = u.UserID
		case "Editor":
			currentEditors[u.Email] = u.UserID
		case "Viewer":
			currentViewers[u.Email] = u.UserID
		}
	}

	// Check admins - all desired admins must exist
	for _, email := range fp.Admins {
		if _, ok := currentAdmins[email]; !ok {
			return false, nil
		}
	}

	// Check editors - all desired editors must exist
	for _, email := range fp.Editors {
		if _, ok := currentEditors[email]; !ok {
			return false, nil
		}
	}

	// Check viewers - all desired viewers must exist
	for _, email := range fp.Viewers {
		if _, ok := currentViewers[email]; !ok {
			return false, nil
		}
	}

	return true, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Organization)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotOrganization)
	}

	cr.Status.SetConditions(xpv1.Creating())

	fp := cr.Spec.ForProvider

	req := grafana.OrganizationCreateRequest{
		Name: fp.Name,
	}

	resp, err := e.client.CreateOrganization(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create organization in Grafana")
	}

	meta.SetExternalName(cr, strconv.FormatInt(resp.OrgID, 10))
	cr.Status.AtProvider.ID = &resp.OrgID

	// Add users to the organization
	if err := e.syncUsers(ctx, resp.OrgID, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot sync users to organization")
	}

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Organization)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotOrganization)
	}

	externalName := meta.GetExternalName(cr)
	orgID, err := strconv.ParseInt(externalName, 10, 64)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse external name as org ID")
	}

	fp := cr.Spec.ForProvider

	// Update org name if changed
	req := grafana.OrganizationCreateRequest{
		Name: fp.Name,
	}

	if err := e.client.UpdateOrganization(ctx, orgID, req); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update organization in Grafana")
	}

	// Sync users
	if err := e.syncUsers(ctx, orgID, cr); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot sync users to organization")
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) syncUsers(ctx context.Context, orgID int64, cr *v1alpha1.Organization) error { //nolint:gocyclo
	fp := cr.Spec.ForProvider

	// If no users are specified, don't manage users at all
	if len(fp.Admins) == 0 && len(fp.Editors) == 0 && len(fp.Viewers) == 0 {
		return nil
	}

	// Get current users
	currentUsers, err := e.client.GetOrgUsers(ctx, orgID)
	if err != nil {
		return errors.Wrap(err, "cannot get current organization users")
	}

	// Build current user map
	currentUserMap := make(map[string]grafana.OrgUser)
	for _, u := range currentUsers {
		currentUserMap[u.Email] = u
	}

	// Build desired user map with roles
	desiredUsers := make(map[string]string)
	for _, email := range fp.Admins {
		desiredUsers[email] = "Admin"
	}
	for _, email := range fp.Editors {
		desiredUsers[email] = "Editor"
	}
	for _, email := range fp.Viewers {
		desiredUsers[email] = "Viewer"
	}

	// Add or update users
	for email, role := range desiredUsers {
		if current, exists := currentUserMap[email]; exists {
			// User exists, check if role needs update
			if current.Role != role {
				if err := e.client.UpdateOrgUserRole(ctx, orgID, current.UserID, role); err != nil {
					return errors.Wrapf(err, "cannot update role for user %s", email)
				}
			}
		} else {
			// User doesn't exist in org, add them
			addReq := grafana.OrgUserAddRequest{
				LoginOrEmail: email,
				Role:         role,
			}
			if err := e.client.AddOrgUser(ctx, orgID, addReq); err != nil {
				return errors.Wrapf(err, "cannot add user %s to organization", email)
			}
		}
	}

	// Remove users not in desired list (but keep at least one admin)
	adminCount := 0
	for _, user := range currentUserMap {
		if user.Role == "Admin" {
			adminCount++
		}
	}

	for email, user := range currentUserMap {
		if _, exists := desiredUsers[email]; !exists {
			// Don't remove the last admin
			if user.Role == "Admin" && adminCount <= 1 {
				continue
			}
			if err := e.client.RemoveOrgUser(ctx, orgID, user.UserID); err != nil {
				return errors.Wrapf(err, "cannot remove user %s from organization", email)
			}
			if user.Role == "Admin" {
				adminCount--
			}
		}
	}

	return nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Organization)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotOrganization)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalDelete{}, nil
	}

	orgID, err := strconv.ParseInt(externalName, 10, 64)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot parse external name as org ID")
	}

	if err := e.client.DeleteOrganization(ctx, orgID); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete organization from Grafana")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}
