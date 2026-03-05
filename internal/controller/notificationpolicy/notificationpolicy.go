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

package notificationpolicy

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

	"github.com/crossplane/provider-gf/apis/alerting/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-gf/apis/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
	"github.com/crossplane/provider-gf/internal/controller/common"
)

const (
	errNotNotificationPolicy = "managed resource is not a NotificationPolicy custom resource"
	errTrackPCUsage          = "cannot track ProviderConfig usage"
	errGetPC                 = "cannot get ProviderConfig"
	errNewClient             = "cannot create Grafana client" //nolint:unused
	errResolveOrgRef         = "cannot resolve organization reference"
	errResolveContactPoint   = "cannot resolve contact point reference"
)

func formatExternalName(orgID int64) string {
	return strconv.FormatInt(orgID, 10)
}

// ExtractContactPointName returns the contact point name from a ContactPoint resource.
func ExtractContactPointName() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		cp, ok := mg.(*v1alpha1.ContactPoint)
		if !ok {
			return ""
		}
		return cp.Spec.ForProvider.Name
	}
}

// Setup adds a controller that reconciles NotificationPolicy managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.NotificationPolicyGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.NotificationPolicyList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.NotificationPolicyGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.NotificationPolicy{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.NotificationPolicy)
	if !ok {
		return nil, errors.New(errNotNotificationPolicy)
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

	gfClient, err := c.newGrafanaClient(ctx, m.GetNamespace(), pc.Spec)
	if err != nil {
		return nil, err
	}

	orgID, err := common.ResolveOrgID(ctx, c.kube, cr,
		cr.Spec.ForProvider.OrgRef,
		cr.Spec.ForProvider.OrgSelector,
		cr.Spec.ForProvider.OrgID,
		pc.Spec.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, errResolveOrgRef)
	}

	return &external{client: gfClient, kube: c.kube, orgID: orgID}, nil
}

func (c *connector) newGrafanaClient(ctx context.Context, namespace string, pcSpec apisv1alpha1.ProviderConfigSpec) (*grafana.Client, error) {
	cfg := grafana.Config{URL: pcSpec.URL, OrgID: pcSpec.OrgID}

	switch pcSpec.Credentials.AuthType {
	case apisv1alpha1.AuthTypeBasic:
		if pcSpec.Credentials.BasicAuth == nil {
			return nil, errors.New("basicAuth is required when authType is basic")
		}
		username, err := c.getSecretValue(ctx, namespace, pcSpec.Credentials.BasicAuth.UsernameSecretRef)
		if err != nil {
			return nil, errors.Wrap(err, "cannot get username from secret")
		}
		password, err := c.getSecretValue(ctx, namespace, pcSpec.Credentials.BasicAuth.PasswordSecretRef)
		if err != nil {
			return nil, errors.Wrap(err, "cannot get password from secret")
		}
		cfg.Username = username
		cfg.Password = password
	case apisv1alpha1.AuthTypeToken:
		if pcSpec.Credentials.TokenAuth == nil {
			return nil, errors.New("tokenAuth is required when authType is token")
		}
		token, err := c.getSecretValue(ctx, namespace, pcSpec.Credentials.TokenAuth.TokenSecretRef)
		if err != nil {
			return nil, errors.Wrap(err, "cannot get token from secret")
		}
		cfg.Token = token
	default:
		return nil, errors.Errorf("unsupported auth type: %s", pcSpec.Credentials.AuthType)
	}

	return grafana.NewClient(cfg)
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
	kube   client.Client
	orgID  int64
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.NotificationPolicy)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotNotificationPolicy)
	}

	// Get the current notification policy from Grafana
	policy, err := e.client.GetNotificationPolicy(ctx)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get notification policy")
	}

	// If no policy exists or it's the default, consider it as not existing
	// The default policy has receiver "" or "grafana-default-email"
	if policy == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Set external name if not set
	externalName := meta.GetExternalName(cr)
	expectedExtName := formatExternalName(e.orgID)
	if externalName != expectedExtName {
		meta.SetExternalName(cr, expectedExtName)
	}

	cr.Status.AtProvider.OrgID = &e.orgID
	cr.Status.SetConditions(xpv1.Available())

	// Resolve contact point name for comparison
	contactPointName, err := e.resolveContactPointName(ctx, cr, cr.Spec.ForProvider.ContactPoint,
		cr.Spec.ForProvider.ContactPointRef, cr.Spec.ForProvider.ContactPointSelector)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errResolveContactPoint)
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: e.isUpToDate(cr, policy, contactPointName),
	}, nil
}

func (e *external) isUpToDate(cr *v1alpha1.NotificationPolicy, policy *grafana.NotificationPolicyTree, contactPointName string) bool {
	// Check receiver
	if policy.Receiver != contactPointName {
		return false
	}

	// Check groupBy
	if !stringSlicesEqual(policy.GroupBy, cr.Spec.ForProvider.GroupBy) {
		return false
	}

	// Check intervals
	if !stringPtrEqual(cr.Spec.ForProvider.GroupWait, policy.GroupWait) {
		return false
	}
	if !stringPtrEqual(cr.Spec.ForProvider.GroupInterval, policy.GroupInterval) {
		return false
	}
	if !stringPtrEqual(cr.Spec.ForProvider.RepeatInterval, policy.RepeatInterval) {
		return false
	}

	// Check routes count
	if len(cr.Spec.ForProvider.Policy) != len(policy.Routes) {
		return false
	}

	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringPtrEqual(ptr *string, val string) bool {
	if ptr == nil {
		return val == ""
	}
	return *ptr == val
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.NotificationPolicy)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotNotificationPolicy)
	}

	cr.Status.SetConditions(xpv1.Creating())

	policy, err := e.buildPolicy(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot build notification policy")
	}

	disableProvenance := true
	if cr.Spec.ForProvider.DisableProvenance != nil {
		disableProvenance = *cr.Spec.ForProvider.DisableProvenance
	}

	if err := e.client.SetNotificationPolicy(ctx, policy, disableProvenance); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot set notification policy")
	}

	meta.SetExternalName(cr, formatExternalName(e.orgID))

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.NotificationPolicy)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotNotificationPolicy)
	}

	policy, err := e.buildPolicy(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot build notification policy")
	}

	disableProvenance := true
	if cr.Spec.ForProvider.DisableProvenance != nil {
		disableProvenance = *cr.Spec.ForProvider.DisableProvenance
	}

	if err := e.client.SetNotificationPolicy(ctx, policy, disableProvenance); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update notification policy")
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.NotificationPolicy)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotNotificationPolicy)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	// Reset the notification policy to default
	if err := e.client.ResetNotificationPolicy(ctx); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot reset notification policy")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}

func (e *external) buildPolicy(ctx context.Context, cr *v1alpha1.NotificationPolicy) (grafana.NotificationPolicyTree, error) {
	contactPointName, err := e.resolveContactPointName(ctx, cr, cr.Spec.ForProvider.ContactPoint,
		cr.Spec.ForProvider.ContactPointRef, cr.Spec.ForProvider.ContactPointSelector)
	if err != nil {
		return grafana.NotificationPolicyTree{}, errors.Wrap(err, "cannot resolve contact point")
	}

	policy := grafana.NotificationPolicyTree{
		Receiver: contactPointName,
		GroupBy:  cr.Spec.ForProvider.GroupBy,
	}

	if cr.Spec.ForProvider.GroupWait != nil {
		policy.GroupWait = *cr.Spec.ForProvider.GroupWait
	}
	if cr.Spec.ForProvider.GroupInterval != nil {
		policy.GroupInterval = *cr.Spec.ForProvider.GroupInterval
	}
	if cr.Spec.ForProvider.RepeatInterval != nil {
		policy.RepeatInterval = *cr.Spec.ForProvider.RepeatInterval
	}

	routes, err := e.buildRoutes(ctx, cr, cr.Spec.ForProvider.Policy)
	if err != nil {
		return grafana.NotificationPolicyTree{}, err
	}
	policy.Routes = routes

	return policy, nil
}

func (e *external) buildRoutes(ctx context.Context, cr *v1alpha1.NotificationPolicy, policyRoutes []v1alpha1.PolicyRoute) ([]grafana.NotificationRoute, error) {
	routes := make([]grafana.NotificationRoute, len(policyRoutes))
	for i, pr := range policyRoutes {
		route, err := e.buildRoute(ctx, cr, pr)
		if err != nil {
			return nil, err
		}
		routes[i] = route
	}
	return routes, nil
}

func (e *external) buildRoute(ctx context.Context, cr *v1alpha1.NotificationPolicy, pr v1alpha1.PolicyRoute) (grafana.NotificationRoute, error) {
	route := grafana.NotificationRoute{
		GroupBy:             pr.GroupBy,
		MuteTimeIntervals:   pr.MuteTimings,
		ActiveTimeIntervals: pr.ActiveTimings,
	}

	// Resolve contact point
	if pr.ContactPoint != nil || pr.ContactPointRef != nil || pr.ContactPointSelector != nil {
		cpName, err := e.resolveContactPointName(ctx, cr, pr.ContactPoint, pr.ContactPointRef, pr.ContactPointSelector)
		if err != nil {
			return grafana.NotificationRoute{}, errors.Wrap(err, "cannot resolve contact point for route")
		}
		route.Receiver = cpName
	}

	if pr.Continue != nil {
		route.Continue = *pr.Continue
	}
	if pr.GroupWait != nil {
		route.GroupWait = *pr.GroupWait
	}
	if pr.GroupInterval != nil {
		route.GroupInterval = *pr.GroupInterval
	}
	if pr.RepeatInterval != nil {
		route.RepeatInterval = *pr.RepeatInterval
	}

	// Build matchers
	route.ObjectMatchers = buildMatchers(pr.Matchers)

	// Build nested routes (level 2)
	if len(pr.Policy) > 0 {
		nestedRoutes, err := e.buildNestedRoutes(ctx, cr, pr.Policy)
		if err != nil {
			return grafana.NotificationRoute{}, err
		}
		route.Routes = nestedRoutes
	}

	return route, nil
}

func (e *external) buildNestedRoutes(ctx context.Context, cr *v1alpha1.NotificationPolicy, policyRoutes []v1alpha1.NestedPolicyRoute) ([]grafana.NotificationRoute, error) {
	routes := make([]grafana.NotificationRoute, len(policyRoutes))
	for i, pr := range policyRoutes {
		route, err := e.buildNestedRoute(ctx, cr, pr)
		if err != nil {
			return nil, err
		}
		routes[i] = route
	}
	return routes, nil
}

func (e *external) buildNestedRoute(ctx context.Context, cr *v1alpha1.NotificationPolicy, pr v1alpha1.NestedPolicyRoute) (grafana.NotificationRoute, error) {
	route := grafana.NotificationRoute{
		GroupBy:             pr.GroupBy,
		MuteTimeIntervals:   pr.MuteTimings,
		ActiveTimeIntervals: pr.ActiveTimings,
	}

	// Resolve contact point
	if pr.ContactPoint != nil || pr.ContactPointRef != nil || pr.ContactPointSelector != nil {
		cpName, err := e.resolveContactPointName(ctx, cr, pr.ContactPoint, pr.ContactPointRef, pr.ContactPointSelector)
		if err != nil {
			return grafana.NotificationRoute{}, errors.Wrap(err, "cannot resolve contact point for nested route")
		}
		route.Receiver = cpName
	}

	if pr.Continue != nil {
		route.Continue = *pr.Continue
	}
	if pr.GroupWait != nil {
		route.GroupWait = *pr.GroupWait
	}
	if pr.GroupInterval != nil {
		route.GroupInterval = *pr.GroupInterval
	}
	if pr.RepeatInterval != nil {
		route.RepeatInterval = *pr.RepeatInterval
	}

	// Build matchers
	route.ObjectMatchers = buildMatchers(pr.Matchers)

	// Build leaf routes (level 3)
	if len(pr.Policy) > 0 {
		leafRoutes, err := e.buildLeafRoutes(ctx, cr, pr.Policy)
		if err != nil {
			return grafana.NotificationRoute{}, err
		}
		route.Routes = leafRoutes
	}

	return route, nil
}

func (e *external) buildLeafRoutes(ctx context.Context, cr *v1alpha1.NotificationPolicy, policyRoutes []v1alpha1.LeafPolicyRoute) ([]grafana.NotificationRoute, error) {
	routes := make([]grafana.NotificationRoute, len(policyRoutes))
	for i, pr := range policyRoutes {
		route, err := e.buildLeafRoute(ctx, cr, pr)
		if err != nil {
			return nil, err
		}
		routes[i] = route
	}
	return routes, nil
}

func (e *external) buildLeafRoute(ctx context.Context, cr *v1alpha1.NotificationPolicy, pr v1alpha1.LeafPolicyRoute) (grafana.NotificationRoute, error) {
	route := grafana.NotificationRoute{
		GroupBy:             pr.GroupBy,
		MuteTimeIntervals:   pr.MuteTimings,
		ActiveTimeIntervals: pr.ActiveTimings,
	}

	// Resolve contact point
	if pr.ContactPoint != nil || pr.ContactPointRef != nil || pr.ContactPointSelector != nil {
		cpName, err := e.resolveContactPointName(ctx, cr, pr.ContactPoint, pr.ContactPointRef, pr.ContactPointSelector)
		if err != nil {
			return grafana.NotificationRoute{}, errors.Wrap(err, "cannot resolve contact point for leaf route")
		}
		route.Receiver = cpName
	}

	if pr.Continue != nil {
		route.Continue = *pr.Continue
	}
	if pr.GroupWait != nil {
		route.GroupWait = *pr.GroupWait
	}
	if pr.GroupInterval != nil {
		route.GroupInterval = *pr.GroupInterval
	}
	if pr.RepeatInterval != nil {
		route.RepeatInterval = *pr.RepeatInterval
	}

	// Build matchers
	route.ObjectMatchers = buildMatchers(pr.Matchers)

	return route, nil
}

func buildMatchers(matchers []v1alpha1.PolicyMatcher) [][]string {
	result := make([][]string, len(matchers))
	for i, m := range matchers {
		result[i] = []string{m.Label, m.Match, m.Value}
	}
	return result
}

func (e *external) resolveContactPointName(ctx context.Context, cr *v1alpha1.NotificationPolicy, directValue *string, ref *xpv1.Reference, selector *xpv1.Selector) (string, error) {
	if directValue != nil && *directValue != "" {
		return *directValue, nil
	}

	if ref != nil || selector != nil {
		rsp, err := reference.NewAPIResolver(e.kube, cr).Resolve(ctx, reference.ResolutionRequest{
			CurrentValue: "",
			Reference:    ref,
			Selector:     selector,
			To:           reference.To{Managed: &v1alpha1.ContactPoint{}, List: &v1alpha1.ContactPointList{}},
			Extract:      ExtractContactPointName(),
			Namespace:    cr.GetNamespace(),
		})
		if err != nil {
			return "", errors.Wrap(err, "cannot resolve contact point reference")
		}
		if rsp.ResolvedValue != "" {
			return rsp.ResolvedValue, nil
		}
	}

	return "", nil
}
