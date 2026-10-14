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

package team

import (
	"context"
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
	errNotTeam          = "managed resource is not a Team custom resource"
	errTrackPCUsage     = "cannot track ProviderConfig usage"
	errGetPC            = "cannot get ProviderConfig"
	errNewClient        = "cannot create Grafana client"
	errInvalidExternals = "invalid external name format, expected <orgId>:<teamId>"
	errResolveOrgRef    = "cannot resolve organization reference"
)

// FormatExternalName creates an external name in the format <orgId>:<teamId>.
func FormatExternalName(orgID, teamID int64) string {
	return strconv.FormatInt(orgID, 10) + ":" + strconv.FormatInt(teamID, 10)
}

// ParseExternalName parses an external name in the format <orgId>:<teamId>.
func ParseExternalName(externalName string) (int64, int64, error) {
	parts := strings.SplitN(externalName, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return 0, 0, errors.New(errInvalidExternals)
	}
	orgID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, errors.Wrap(err, errInvalidExternals)
	}
	teamID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, errors.Wrap(err, errInvalidExternals)
	}
	return orgID, teamID, nil
}

// Setup adds a controller that reconciles Team managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.TeamGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.TeamList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.TeamGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Team{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return nil, errors.New(errNotTeam)
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

	// Pin already-created teams to their existing org so a Selector matching
	// multiple Organizations can't drift them to a newly created one.
	currentOrgID := ""
	if en := meta.GetExternalName(cr); en != "" {
		if oid, _, err := ParseExternalName(en); err == nil {
			currentOrgID = strconv.FormatInt(oid, 10)
		}
	}

	orgID, err := common.ResolveOrgID(ctx, c.kube, cr,
		cr.Spec.ForProvider.OrgRef,
		cr.Spec.ForProvider.OrgSelector,
		cr.Spec.ForProvider.OrgID,
		pcSpec.OrgID,
		currentOrgID)
	if err != nil {
		return nil, errors.Wrap(err, errResolveOrgRef)
	}
	gfClient.SetOrgID(orgID)

	return &external{client: gfClient, orgID: orgID}, nil
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
	orgID  int64
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotTeam)
	}

	externalName := meta.GetExternalName(cr)
	var teamID int64

	if externalName != "" {
		if _, tid, err := ParseExternalName(externalName); err == nil {
			teamID = tid
		}
	}

	// Recovery: find an orphaned team by name. Team names are unique per
	// org, so this cannot adopt somebody else's team.
	if teamID == 0 {
		existing, err := e.client.GetTeamByName(ctx, cr.Spec.ForProvider.Name)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, "cannot get team by name from Grafana")
		}
		if existing != nil {
			teamID = existing.ID
			meta.SetExternalName(cr, FormatExternalName(e.orgID, teamID))
			externalName = meta.GetExternalName(cr)
		}
	}

	if teamID == 0 {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	team, err := e.client.GetTeamByID(ctx, teamID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get team from Grafana")
	}
	if team == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	expectedExternalName := FormatExternalName(e.orgID, team.ID)
	if externalName != expectedExternalName {
		meta.SetExternalName(cr, expectedExternalName)
	}

	// Only fetch membership when it's actually managed, to avoid an extra
	// API call for teams that use standalone TeamMember resources.
	var members []grafana.TeamMember
	if managesMembers(cr) {
		members, err = e.client.GetTeamMembers(ctx, team.ID)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, "cannot get team members from Grafana")
		}
	}

	cr.Status.AtProvider.ID = &team.ID
	cr.Status.AtProvider.UID = &team.UID
	cr.Status.AtProvider.OrgID = &team.OrgID
	cr.Status.AtProvider.Name = &team.Name
	cr.Status.AtProvider.Email = &team.Email
	memberCount := int64(len(members))
	if memberCount == 0 {
		memberCount = team.MemberCount
	}
	cr.Status.AtProvider.MemberCount = &memberCount
	cr.Status.AtProvider.Members = observedMembers(members)

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: e.isUpToDate(cr, team, members),
	}, nil
}

func (e *external) isUpToDate(cr *v1alpha1.Team, team *grafana.Team, members []grafana.TeamMember) bool {
	fp := cr.Spec.ForProvider

	if fp.Name != team.Name {
		return false
	}
	// An unset email is reconciled to the Grafana default of "".
	if fp.Email != nil && *fp.Email != team.Email {
		return false
	}

	if !managesMembers(cr) {
		return true
	}

	return sameEmailSet(desiredEmails(fp), observedEmails(members))
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotTeam)
	}

	cr.Status.SetConditions(xpv1.Creating())

	fp := cr.Spec.ForProvider
	req := grafana.TeamCreateRequest{
		Name:  fp.Name,
		Email: stringOrEmpty(fp.Email),
	}

	resp, err := e.client.CreateTeam(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create team in Grafana")
	}

	meta.SetExternalName(cr, FormatExternalName(e.orgID, resp.TeamID))
	cr.Status.AtProvider.ID = &resp.TeamID

	// Membership can't be set in the create request, so apply it separately.
	if managesMembers(cr) {
		if err := e.client.SetTeamMembers(ctx, resp.TeamID, membershipRequest(cr)); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, "cannot set team members in Grafana")
		}
	}

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotTeam)
	}

	_, teamID, err := ParseExternalName(meta.GetExternalName(cr))
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errInvalidExternals)
	}

	fp := cr.Spec.ForProvider
	if err := e.client.UpdateTeam(ctx, teamID, grafana.TeamCreateRequest{
		Name:  fp.Name,
		Email: stringOrEmpty(fp.Email),
	}); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update team in Grafana")
	}

	if managesMembers(cr) {
		if err := e.client.SetTeamMembers(ctx, teamID, membershipRequest(cr)); err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, "cannot set team members in Grafana")
		}
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotTeam)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	if meta.GetExternalName(cr) == "" {
		return managed.ExternalDelete{}, nil
	}

	_, teamID, err := ParseExternalName(meta.GetExternalName(cr))
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errInvalidExternals)
	}

	if err := e.client.DeleteTeam(ctx, teamID); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete team from Grafana")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}

// managesMembers reports whether the Team resource owns its team membership.
// When neither members nor admins are specified membership is left alone so
// standalone TeamMember resources can own it.
func managesMembers(cr *v1alpha1.Team) bool {
	return len(cr.Spec.ForProvider.Members) > 0 || len(cr.Spec.ForProvider.Admins) > 0
}

func membershipRequest(cr *v1alpha1.Team) grafana.TeamMembersRequest {
	return grafana.TeamMembersRequest{
		Members: cr.Spec.ForProvider.Members,
		Admins:  cr.Spec.ForProvider.Admins,
	}
}

// desiredEmails returns every email address the spec wants in the team.
func desiredEmails(fp v1alpha1.TeamParameters) map[string]struct{} {
	emails := make(map[string]struct{}, len(fp.Members)+len(fp.Admins))
	for _, e := range fp.Members {
		emails[strings.ToLower(e)] = struct{}{}
	}
	for _, e := range fp.Admins {
		emails[strings.ToLower(e)] = struct{}{}
	}
	return emails
}

// observedEmails returns the email address of every user Grafana reports in
// the team.
func observedEmails(members []grafana.TeamMember) map[string]struct{} {
	emails := make(map[string]struct{}, len(members))
	for _, m := range members {
		if m.Email != "" {
			emails[strings.ToLower(m.Email)] = struct{}{}
		}
	}
	return emails
}

func sameEmailSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func observedMembers(members []grafana.TeamMember) []v1alpha1.TeamMemberObservation {
	if len(members) == 0 {
		return nil
	}
	out := make([]v1alpha1.TeamMemberObservation, 0, len(members))
	for _, m := range members {
		userID, login, email := m.UserID, m.Login, m.Email
		out = append(out, v1alpha1.TeamMemberObservation{
			UserID: &userID,
			Login:  &login,
			Email:  &email,
		})
	}
	return out
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
