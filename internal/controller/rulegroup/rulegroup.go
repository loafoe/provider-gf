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

package rulegroup

import (
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
	"github.com/crossplane/crossplane-runtime/v2/pkg/reference"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/provider-gf/apis/alerting/v1alpha1"
	ossv1alpha1 "github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-gf/apis/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
	"github.com/crossplane/provider-gf/internal/controller/common"
)

const (
	errNotRuleGroup        = "managed resource is not a RuleGroup custom resource"
	errTrackPCUsage        = "cannot track ProviderConfig usage"
	errGetPC               = "cannot get ProviderConfig"
	errInvalidExternalName = "invalid external name format, expected <orgId>:<folderUid>:<groupName>"
	errResolveOrgRef       = "cannot resolve organization reference"
	errMissingFolder       = "folder reference is required: specify folderUid, folderRef, or folderSelector"
)

func formatExternalName(orgID int64, folderUID, groupName string) string {
	return strconv.FormatInt(orgID, 10) + ":" + folderUID + ":" + groupName
}

func parseExternalName(externalName string) (int64, string, string, error) { //nolint:unparam
	parts := strings.SplitN(externalName, ":", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return 0, "", "", errors.New(errInvalidExternalName)
	}
	orgID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", "", errors.Wrap(err, errInvalidExternalName)
	}
	return orgID, parts[1], parts[2], nil
}

// ExtractFolderUID returns a function that extracts the folder UID from a Folder resource.
func ExtractFolderUID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		f, ok := mg.(*ossv1alpha1.Folder)
		if !ok {
			return ""
		}
		externalName := meta.GetExternalName(f)
		if externalName == "" {
			return ""
		}
		parts := strings.SplitN(externalName, ":", 2)
		if len(parts) != 2 {
			return ""
		}
		return parts[1]
	}
}

// Setup adds a controller that reconciles RuleGroup managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.RuleGroupGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.RuleGroupList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.RuleGroupGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.RuleGroup{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.RuleGroup)
	if !ok {
		return nil, errors.New(errNotRuleGroup)
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

	// Don't fail Connect() if folder resolution fails - this allows Delete() to work
	// when the referenced folder doesn't exist. Create() and Update() will return
	// a clear error if folder is required but not resolved.
	folderUID := c.resolveFolderUID(ctx, cr)

	return &external{client: gfClient, kube: c.kube, orgID: orgID, folderUID: folderUID}, nil
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

func (c *connector) resolveFolderUID(ctx context.Context, cr *v1alpha1.RuleGroup) string {
	if cr.Spec.ForProvider.FolderUID != nil && *cr.Spec.ForProvider.FolderUID != "" {
		return *cr.Spec.ForProvider.FolderUID
	}

	if cr.Spec.ForProvider.FolderRef != nil || cr.Spec.ForProvider.FolderSelector != nil {
		rsp, err := reference.NewAPIResolver(c.kube, cr).Resolve(ctx, reference.ResolutionRequest{
			CurrentValue: "",
			Reference:    cr.Spec.ForProvider.FolderRef,
			Selector:     cr.Spec.ForProvider.FolderSelector,
			To:           reference.To{Managed: &ossv1alpha1.Folder{}, List: &ossv1alpha1.FolderList{}},
			Extract:      ExtractFolderUID(),
			Namespace:    cr.GetNamespace(),
		})
		if err == nil && rsp.ResolvedValue != "" {
			return rsp.ResolvedValue
		}
	}

	// Fallback to external name (useful during deletion when referenced folder is already gone)
	return extractFolderUIDFromExternalName(cr)
}

func extractFolderUIDFromExternalName(cr resource.Managed) string {
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return ""
	}
	_, folderUID, _, err := parseExternalName(externalName)
	if err != nil {
		return ""
	}
	return folderUID
}

type external struct {
	client    *grafana.Client
	kube      client.Client
	orgID     int64
	folderUID string
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.RuleGroup)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotRuleGroup)
	}

	groupName, folderUID := e.resolveGroupIdentifiers(cr)
	if folderUID == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	e.folderUID = folderUID

	rg, err := e.client.GetRuleGroup(ctx, folderUID, groupName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get rule group")
	}
	if rg == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	e.updateExternalNameAndStatus(cr, groupName, rg)

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: e.isUpToDate(cr, rg),
	}, nil
}

func (e *external) resolveGroupIdentifiers(cr *v1alpha1.RuleGroup) (groupName, folderUID string) {
	groupName = cr.Spec.ForProvider.Name
	folderUID = e.folderUID
	externalName := meta.GetExternalName(cr)

	if externalName != "" {
		_, parsedFolderUID, parsedGroupName, err := parseExternalName(externalName)
		if err == nil {
			if folderUID == "" {
				folderUID = parsedFolderUID
			}
			groupName = parsedGroupName
		}
	}
	return groupName, folderUID
}

func (e *external) updateExternalNameAndStatus(cr *v1alpha1.RuleGroup, groupName string, rg *grafana.AlertRuleGroup) {
	expectedExtName := formatExternalName(e.orgID, e.folderUID, groupName)
	if meta.GetExternalName(cr) != expectedExtName {
		meta.SetExternalName(cr, expectedExtName)
	}

	cr.Status.AtProvider.FolderUID = &e.folderUID
	cr.Status.AtProvider.OrgID = &e.orgID
	cr.Status.AtProvider.Rules = make([]v1alpha1.RuleObservation, len(rg.Rules))
	for i, rule := range rg.Rules {
		uid := rule.UID
		title := rule.Title
		cr.Status.AtProvider.Rules[i] = v1alpha1.RuleObservation{
			UID:   &uid,
			Title: &title,
		}
	}
	cr.Status.SetConditions(xpv1.Available())
}

func (e *external) isUpToDate(cr *v1alpha1.RuleGroup, rg *grafana.AlertRuleGroup) bool {
	// Check interval
	if rg.Interval != cr.Spec.ForProvider.IntervalSeconds {
		return false
	}

	// Check number of rules
	if len(rg.Rules) != len(cr.Spec.ForProvider.Rules) {
		return false
	}

	// Check each rule (simplified comparison)
	for i, desiredRule := range cr.Spec.ForProvider.Rules {
		observedRule := rg.Rules[i]
		if observedRule.Title != desiredRule.Title {
			return false
		}
		if observedRule.Condition != desiredRule.Condition {
			return false
		}
		if desiredRule.For != nil && observedRule.For != *desiredRule.For {
			return false
		}
	}

	return true
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.RuleGroup)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotRuleGroup)
	}

	if e.folderUID == "" {
		return managed.ExternalCreation{}, errors.New(errMissingFolder)
	}

	cr.Status.SetConditions(xpv1.Creating())

	rg := e.buildRuleGroup(cr)
	disableProvenance := true
	if cr.Spec.ForProvider.DisableProvenance != nil {
		disableProvenance = *cr.Spec.ForProvider.DisableProvenance
	}

	_, err := e.client.CreateOrUpdateRuleGroup(ctx, e.folderUID, rg, disableProvenance)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create rule group")
	}

	meta.SetExternalName(cr, formatExternalName(e.orgID, e.folderUID, cr.Spec.ForProvider.Name))

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.RuleGroup)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotRuleGroup)
	}

	if e.folderUID == "" {
		return managed.ExternalUpdate{}, errors.New(errMissingFolder)
	}

	rg := e.buildRuleGroup(cr)
	disableProvenance := true
	if cr.Spec.ForProvider.DisableProvenance != nil {
		disableProvenance = *cr.Spec.ForProvider.DisableProvenance
	}

	_, err := e.client.CreateOrUpdateRuleGroup(ctx, e.folderUID, rg, disableProvenance)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update rule group")
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.RuleGroup)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotRuleGroup)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	groupName := cr.Spec.ForProvider.Name
	folderUID := e.folderUID

	// Try to extract from external name
	externalName := meta.GetExternalName(cr)
	if externalName != "" {
		_, parsedFolderUID, parsedGroupName, err := parseExternalName(externalName)
		if err == nil {
			folderUID = parsedFolderUID
			groupName = parsedGroupName
		}
	}

	if folderUID == "" {
		return managed.ExternalDelete{}, nil
	}

	if err := e.client.DeleteRuleGroup(ctx, folderUID, groupName); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete rule group")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}

func (e *external) buildRuleGroup(cr *v1alpha1.RuleGroup) grafana.AlertRuleGroup {
	rules := make([]grafana.AlertRule, len(cr.Spec.ForProvider.Rules))
	for i, r := range cr.Spec.ForProvider.Rules {
		rules[i] = buildAlertRule(r)
	}

	return grafana.AlertRuleGroup{
		Title:    cr.Spec.ForProvider.Name,
		Interval: cr.Spec.ForProvider.IntervalSeconds,
		Rules:    rules,
	}
}

func buildAlertRule(r v1alpha1.Rule) grafana.AlertRule {
	rule := grafana.AlertRule{
		Title:        r.Title,
		Condition:    r.Condition,
		Labels:       r.Labels,
		Annotations:  r.Annotations,
		For:          stringPtrOrDefault(r.For, "5m"),
		NoDataState:  stringPtrOrDefault(r.NoDataState, "NoData"),
		ExecErrState: stringPtrOrDefault(r.ExecErrState, "Error"),
	}

	if r.UID != nil {
		rule.UID = *r.UID
	}
	if r.IsPaused != nil {
		rule.IsPaused = *r.IsPaused
	}

	rule.Data = buildAlertQueries(r.Data)
	rule.NotificationSettings = buildNotificationSettings(r.NotificationSettings)

	return rule
}

func stringPtrOrDefault(ptr *string, defaultVal string) string {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

func buildAlertQueries(data []v1alpha1.RuleQuery) []grafana.AlertQuery {
	queries := make([]grafana.AlertQuery, len(data))
	for i, q := range data {
		var model map[string]any
		if q.Model.Raw != nil {
			_ = json.Unmarshal(q.Model.Raw, &model)
		}
		query := grafana.AlertQuery{
			RefID:         q.RefID,
			DatasourceUID: q.DatasourceUID,
			Model:         model,
		}
		if q.QueryType != nil {
			query.QueryType = *q.QueryType
		}
		if q.RelativeTimeRange != nil {
			query.RelativeTimeRange = &grafana.AlertTimeRange{
				From: q.RelativeTimeRange.From,
				To:   q.RelativeTimeRange.To,
			}
		}
		queries[i] = query
	}
	return queries
}

func buildNotificationSettings(ns *v1alpha1.NotificationSettings) *grafana.AlertNotification {
	if ns == nil {
		return nil
	}
	notification := &grafana.AlertNotification{
		Receiver:          ns.Receiver,
		GroupBy:           ns.GroupBy,
		MuteTimeIntervals: ns.MuteTimeIntervals,
	}
	if ns.GroupWait != nil {
		notification.GroupWait = *ns.GroupWait
	}
	if ns.GroupInterval != nil {
		notification.GroupInterval = *ns.GroupInterval
	}
	if ns.RepeatInterval != nil {
		notification.RepeatInterval = *ns.RepeatInterval
	}
	return notification
}
