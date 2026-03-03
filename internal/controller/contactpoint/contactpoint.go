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

package contactpoint

import (
	"context"
	"fmt"
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

	"github.com/crossplane/provider-gf/apis/alerting/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-gf/apis/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
	"github.com/crossplane/provider-gf/internal/controller/common"
)

const (
	errNotContactPoint     = "managed resource is not a ContactPoint custom resource"
	errTrackPCUsage        = "cannot track ProviderConfig usage"
	errGetPC               = "cannot get ProviderConfig"
	errNewClient           = "cannot create Grafana client"
	errInvalidExternalName = "invalid external name format, expected <orgId>:<uid>"
	errResolveOrgRef       = "cannot resolve organization reference"
)

func formatExternalName(orgID int64, uid string) string {
	return strconv.FormatInt(orgID, 10) + ":" + uid
}

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

// SetupGated adds a controller that reconciles ContactPoint managed resources.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup ContactPoint controller"))
		}
	}, v1alpha1.ContactPointGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles ContactPoint managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ContactPointGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.ContactPointList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.ContactPointGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.ContactPoint{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.ContactPoint)
	if !ok {
		return nil, errors.New(errNotContactPoint)
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

	return &external{client: gfClient, kube: c.kube, orgID: orgID}, nil
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

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.ContactPoint)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotContactPoint)
	}

	externalName := meta.GetExternalName(cr)
	var uid string

	if externalName != "" {
		_, parsedUID, err := parseExternalName(externalName)
		if err == nil {
			uid = parsedUID
		}
	}

	// Recovery: try to find by name if UID not available
	if uid == "" {
		cp, err := e.client.GetContactPointByName(ctx, cr.Spec.ForProvider.Name)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, "cannot get contact point by name")
		}
		if cp != nil {
			uid = cp.UID
		}
	}

	if uid == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	cp, err := e.client.GetContactPointByUID(ctx, uid)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get contact point")
	}
	if cp == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Set external name if recovered
	if externalName == "" || meta.GetExternalName(cr) != formatExternalName(e.orgID, cp.UID) {
		meta.SetExternalName(cr, formatExternalName(e.orgID, cp.UID))
	}

	cr.Status.AtProvider.UID = &cp.UID
	cr.Status.SetConditions(xpv1.Available())

	// Check if up to date by comparing settings
	isUpToDate, err := e.isUpToDate(ctx, cr, cp)
	if err != nil {
		isUpToDate = false // If we can't compare, assume not up to date
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate,
	}, nil
}

// isUpToDate compares the desired and observed contact point.
func (e *external) isUpToDate(ctx context.Context, cr *v1alpha1.ContactPoint, cp *grafana.ContactPoint) (bool, error) {
	// Check name
	if cp.Name != cr.Spec.ForProvider.Name {
		return false, nil
	}

	// Check type
	cpType := e.getContactPointType(cr)
	if cp.Type != cpType {
		return false, nil
	}

	// Check disableResolveMessage
	if cp.DisableResolveMessage != e.getDisableResolveMessage(cr) {
		return false, nil
	}

	// Build desired settings and compare
	_, desiredSettings, err := e.buildSettings(ctx, cr)
	if err != nil {
		return false, err
	}

	// Compare settings (simplified - check key fields)
	for key, desiredVal := range desiredSettings {
		observedVal, ok := cp.Settings[key]
		if !ok {
			return false, nil
		}
		if fmt.Sprintf("%v", desiredVal) != fmt.Sprintf("%v", observedVal) {
			return false, nil
		}
	}

	return true, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.ContactPoint)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotContactPoint)
	}

	cr.Status.SetConditions(xpv1.Creating())

	cpType, settings, err := e.buildSettings(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot build contact point settings")
	}

	req := grafana.ContactPointCreateRequest{
		Name:                  cr.Spec.ForProvider.Name,
		Type:                  cpType,
		Settings:              settings,
		DisableResolveMessage: e.getDisableResolveMessage(cr),
	}

	disableProvenance := true
	if cr.Spec.ForProvider.DisableProvenance != nil {
		disableProvenance = *cr.Spec.ForProvider.DisableProvenance
	}

	resp, err := e.client.CreateContactPoint(ctx, req, disableProvenance)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create contact point")
	}

	meta.SetExternalName(cr, formatExternalName(e.orgID, resp.UID))
	cr.Status.AtProvider.UID = &resp.UID

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.ContactPoint)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotContactPoint)
	}

	externalName := meta.GetExternalName(cr)
	_, uid, err := parseExternalName(externalName)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse external name")
	}

	cpType, settings, err := e.buildSettings(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot build contact point settings")
	}

	req := grafana.ContactPointCreateRequest{
		UID:                   uid,
		Name:                  cr.Spec.ForProvider.Name,
		Type:                  cpType,
		Settings:              settings,
		DisableResolveMessage: e.getDisableResolveMessage(cr),
	}

	disableProvenance := true
	if cr.Spec.ForProvider.DisableProvenance != nil {
		disableProvenance = *cr.Spec.ForProvider.DisableProvenance
	}

	_, err = e.client.UpdateContactPoint(ctx, uid, req, disableProvenance)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update contact point")
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.ContactPoint)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotContactPoint)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalDelete{}, nil
	}

	_, uid, err := parseExternalName(externalName)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot parse external name")
	}

	if err := e.client.DeleteContactPointByUID(ctx, uid); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete contact point")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}
