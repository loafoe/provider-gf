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

package teampreference

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
	"github.com/crossplane/provider-gf/internal/controller/team"
)

const (
	errNotTeamPreference = "managed resource is not a TeamPreference custom resource"
	errTrackPCUsage      = "cannot track ProviderConfig usage"
	errGetPC             = "cannot get ProviderConfig"
	errNewClient         = "cannot create Grafana client"
	errInvalidExternals  = "invalid external name format, expected <orgId>:<teamId>"
	errResolveOrgRef     = "cannot resolve organization reference"
	errMissingTeam       = "team reference is required: specify teamId, teamRef, or teamSelector"
	errTeamNotFound      = "cannot find team in Grafana: check that teamId, teamRef or teamSelector is correct"
)

// Setup adds a controller that reconciles TeamPreference managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.TeamPreferenceGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.TeamPreferenceList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.TeamPreferenceGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.TeamPreference{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.TeamPreference)
	if !ok {
		return nil, errors.New(errNotTeamPreference)
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

	// Pin already-applied preferences to their org so a Selector matching
	// multiple Organizations can't drift them to a newly created one.
	currentOrgID := ""
	if en := meta.GetExternalName(cr); en != "" {
		if oid, _, err := team.ParseExternalName(en); err == nil {
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

	return &external{client: gfClient, orgID: orgID, teamID: c.resolveTeamID(ctx, cr)}, nil
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

// resolveTeamID resolves the team from teamId, teamRef/teamSelector, or - as a
// last resort for deletion - the external name. It never fails so that Delete
// still works when the referenced Team is gone.
func (c *connector) resolveTeamID(ctx context.Context, cr *v1alpha1.TeamPreference) int64 {
	fp := cr.Spec.ForProvider

	if fp.TeamID != nil && *fp.TeamID != 0 {
		return *fp.TeamID
	}

	if fp.TeamRef != nil || fp.TeamSelector != nil {
		rsp, err := reference.NewAPIResolver(c.kube, cr).Resolve(ctx, reference.ResolutionRequest{
			CurrentValue: "",
			Reference:    fp.TeamRef,
			Selector:     fp.TeamSelector,
			To:           reference.To{Managed: &v1alpha1.Team{}, List: &v1alpha1.TeamList{}},
			Extract:      common.ExtractTeamID(),
			Namespace:    cr.GetNamespace(),
		})
		if err == nil && rsp.ResolvedValue != "" {
			if id, err := strconv.ParseInt(rsp.ResolvedValue, 10, 64); err == nil {
				return id
			}
		}
	}

	_, teamID, err := team.ParseExternalName(meta.GetExternalName(cr))
	if err != nil {
		return 0
	}
	return teamID
}

type external struct {
	client *grafana.Client
	orgID  int64
	teamID int64
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.TeamPreference)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotTeamPreference)
	}

	if e.teamID == 0 {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	prefs, err := e.client.GetTeamPreferences(ctx, e.teamID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get team preferences from Grafana")
	}
	if prefs == nil {
		// The team doesn't exist, so neither do its preferences.
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	externalName := team.FormatExternalName(e.orgID, e.teamID)
	if meta.GetExternalName(cr) != externalName {
		meta.SetExternalName(cr, externalName)
	}

	cr.Status.AtProvider.ID = &externalName
	cr.Status.AtProvider.TeamID = &e.teamID
	cr.Status.AtProvider.OrgID = &e.orgID
	cr.Status.AtProvider.Theme = &prefs.Theme
	cr.Status.AtProvider.Timezone = &prefs.Timezone
	cr.Status.AtProvider.HomeDashboardUID = &prefs.HomeDashboardUID
	cr.Status.AtProvider.HomeDashboardID = &prefs.HomeDashboardID

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, prefs),
	}, nil
}

// isUpToDate compares every preference against the spec. Grafana's update
// endpoint replaces the whole object, so an omitted field is reconciled to
// the Grafana default ("") rather than being left alone - which means an
// out-of-band change to a field the spec doesn't mention is still drift.
func isUpToDate(fp v1alpha1.TeamPreferenceParameters, prefs *grafana.TeamPreferences) bool {
	return stringOrEmpty(fp.Theme) == prefs.Theme &&
		stringOrEmpty(fp.Timezone) == prefs.Timezone &&
		stringOrEmpty(fp.HomeDashboardUID) == prefs.HomeDashboardUID
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.TeamPreference)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotTeamPreference)
	}

	if e.teamID == 0 {
		return managed.ExternalCreation{}, errors.New(errMissingTeam)
	}

	cr.Status.SetConditions(xpv1.Creating())

	t, err := e.client.GetTeamByID(ctx, e.teamID)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errTeamNotFound)
	}
	if t == nil {
		return managed.ExternalCreation{}, errors.New(errTeamNotFound)
	}

	if err := e.client.UpdateTeamPreferences(ctx, e.teamID, desiredPreferences(cr.Spec.ForProvider)); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot update team preferences in Grafana")
	}

	meta.SetExternalName(cr, team.FormatExternalName(e.orgID, e.teamID))

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.TeamPreference)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotTeamPreference)
	}

	if e.teamID == 0 {
		return managed.ExternalUpdate{}, errors.New(errMissingTeam)
	}

	if err := e.client.UpdateTeamPreferences(ctx, e.teamID, desiredPreferences(cr.Spec.ForProvider)); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update team preferences in Grafana")
	}

	meta.SetExternalName(cr, team.FormatExternalName(e.orgID, e.teamID))

	return managed.ExternalUpdate{}, nil
}

// Delete is a no-op: Grafana has no API to reset team preferences to their
// defaults, and they hold no data that outlives the team itself.
func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	if _, ok := mg.(*v1alpha1.TeamPreference); !ok {
		return managed.ExternalDelete{}, errors.New(errNotTeamPreference)
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}

// desiredPreferences builds the update body. Grafana replaces omitted keys
// with system defaults, so unset fields are sent as their default value.
func desiredPreferences(fp v1alpha1.TeamPreferenceParameters) grafana.TeamPreferences {
	prefs := grafana.TeamPreferences{
		Theme:            stringOrEmpty(fp.Theme),
		Timezone:         stringOrEmpty(fp.Timezone),
		HomeDashboardUID: stringOrEmpty(fp.HomeDashboardUID),
	}
	return prefs
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
