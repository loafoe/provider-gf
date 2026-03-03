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

package datasource

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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-gf/apis/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
)

const (
	errNotDataSource       = "managed resource is not a DataSource custom resource"
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

// SetupGated adds a controller that reconciles DataSource managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup DataSource controller"))
		}
	}, v1alpha1.DataSourceGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles DataSource managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.DataSourceGroupKind)

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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.DataSourceList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.DataSourceList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.DataSourceGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.DataSource{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// connector is expected to produce an ExternalClient when its Connect method is called.
type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

// Connect produces an ExternalClient for the DataSource resource.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.DataSource)
	if !ok {
		return nil, errors.New(errNotDataSource)
	}

	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	// Get the ProviderConfig
	pc := &apisv1alpha1.ProviderConfig{}
	pcRef := cr.Spec.ProviderConfigReference
	if pcRef == nil {
		return nil, errors.New("providerConfigRef is required")
	}

	if err := c.kube.Get(ctx, types.NamespacedName{Name: pcRef.Name, Namespace: cr.Namespace}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	pcSpec := pc.Spec

	// Get credentials from secret
	cfg := grafana.Config{
		URL:   pcSpec.URL,
		OrgID: pcSpec.OrgID,
	}

	creds := pcSpec.Credentials
	if creds.Source == xpv1.CredentialsSourceSecret {
		switch creds.AuthType {
		case apisv1alpha1.AuthTypeBasic:
			if creds.BasicAuth == nil {
				return nil, errors.New("basicAuth config required when authType is basic")
			}
			// Get username
			usernameSecret := &corev1.Secret{}
			usernameRef := creds.BasicAuth.UsernameSecretRef
			if err := c.kube.Get(ctx, types.NamespacedName{
				Namespace: usernameRef.Namespace,
				Name:      usernameRef.Name,
			}, usernameSecret); err != nil {
				return nil, errors.Wrap(err, errGetCreds)
			}
			cfg.Username = string(usernameSecret.Data[usernameRef.Key])

			// Get password
			passwordSecret := &corev1.Secret{}
			passwordRef := creds.BasicAuth.PasswordSecretRef
			if err := c.kube.Get(ctx, types.NamespacedName{
				Namespace: passwordRef.Namespace,
				Name:      passwordRef.Name,
			}, passwordSecret); err != nil {
				return nil, errors.Wrap(err, errGetCreds)
			}
			cfg.Password = string(passwordSecret.Data[passwordRef.Key])

		case apisv1alpha1.AuthTypeToken:
			if creds.TokenAuth == nil {
				return nil, errors.New("tokenAuth config required when authType is token")
			}
			tokenSecret := &corev1.Secret{}
			tokenRef := creds.TokenAuth.TokenSecretRef
			if err := c.kube.Get(ctx, types.NamespacedName{
				Namespace: tokenRef.Namespace,
				Name:      tokenRef.Name,
			}, tokenSecret); err != nil {
				return nil, errors.Wrap(err, errGetCreds)
			}
			cfg.Token = string(tokenSecret.Data[tokenRef.Key])

		default:
			return nil, errors.Errorf("unsupported auth type: %s", creds.AuthType)
		}
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

	return &external{client: gfClient, orgID: orgID, kube: c.kube}, nil
}

// external observes, then either creates, updates, or deletes an external resource.
type external struct {
	client *grafana.Client
	orgID  int64
	kube   client.Client
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.DataSource)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotDataSource)
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

	// Fetch the data source from Grafana
	ds, err := e.client.GetDataSourceByUID(ctx, uid)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot get data source from Grafana")
	}

	if ds == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Update status with observed values
	cr.Status.AtProvider.ID = &ds.ID
	cr.Status.AtProvider.UID = &ds.UID
	cr.Status.AtProvider.Name = &ds.Name
	cr.Status.AtProvider.Type = &ds.Type
	cr.Status.AtProvider.URL = &ds.URL
	cr.Status.AtProvider.AccessMode = &ds.Access
	cr.Status.AtProvider.IsDefault = &ds.IsDefault
	cr.Status.AtProvider.BasicAuthEnabled = &ds.BasicAuth
	cr.Status.AtProvider.ReadOnly = &ds.ReadOnly

	// Check if the data source is up to date
	isUpToDate := e.isUpToDate(cr, ds)

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  isUpToDate,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// isUpToDate compares the desired and observed data source state.
func (e *external) isUpToDate(cr *v1alpha1.DataSource, ds *grafana.DataSource) bool { //nolint:gocyclo
	fp := cr.Spec.ForProvider

	if fp.Name != ds.Name {
		return false
	}
	if fp.Type != ds.Type {
		return false
	}
	if fp.URL != nil && *fp.URL != ds.URL {
		return false
	}
	if fp.AccessMode != nil && *fp.AccessMode != ds.Access {
		return false
	}
	if fp.IsDefault != nil && *fp.IsDefault != ds.IsDefault {
		return false
	}
	if fp.BasicAuthEnabled != nil && *fp.BasicAuthEnabled != ds.BasicAuth {
		return false
	}
	if fp.BasicAuthUsername != nil && *fp.BasicAuthUsername != ds.BasicAuthUser {
		return false
	}
	if fp.DatabaseName != nil && *fp.DatabaseName != ds.Database {
		return false
	}
	if fp.Username != nil && *fp.Username != ds.User {
		return false
	}

	// Compare JSONData if specified
	if fp.JSONDataEncoded != nil {
		var desiredJSONData map[string]any
		if err := json.Unmarshal([]byte(*fp.JSONDataEncoded), &desiredJSONData); err == nil {
			// Simple comparison - could be improved with deep equality
			desiredBytes, err := json.Marshal(desiredJSONData)
			if err != nil {
				return false
			}
			observedBytes, err := json.Marshal(ds.JSONData)
			if err != nil {
				return false
			}
			if !bytes.Equal(desiredBytes, observedBytes) {
				return false
			}
		}
	}

	return true
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.DataSource)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotDataSource)
	}

	cr.Status.SetConditions(xpv1.Creating())

	fp := cr.Spec.ForProvider

	req := grafana.DataSourceCreateRequest{
		Name: fp.Name,
		Type: fp.Type,
	}

	// Set optional UID if specified
	if fp.UID != nil {
		req.UID = *fp.UID
	}

	if fp.URL != nil {
		req.URL = *fp.URL
	}
	if fp.AccessMode != nil {
		req.Access = *fp.AccessMode
	} else {
		req.Access = "proxy" // default
	}
	if fp.IsDefault != nil {
		req.IsDefault = *fp.IsDefault
	}
	if fp.BasicAuthEnabled != nil {
		req.BasicAuth = *fp.BasicAuthEnabled
	}
	if fp.BasicAuthUsername != nil {
		req.BasicAuthUser = *fp.BasicAuthUsername
	}
	if fp.DatabaseName != nil {
		req.Database = *fp.DatabaseName
	}
	if fp.Username != nil {
		req.User = *fp.Username
	}

	// Parse JSONData if specified
	if fp.JSONDataEncoded != nil {
		var jsonData map[string]any
		if err := json.Unmarshal([]byte(*fp.JSONDataEncoded), &jsonData); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, "cannot parse jsonDataEncoded")
		}
		req.JSONData = jsonData
	}

	// Get secure JSON data from secret if specified
	if fp.SecureJSONDataEncodedSecretRef != nil {
		secureData, err := e.getSecureJSONData(ctx, fp.SecureJSONDataEncodedSecretRef, cr.Namespace)
		if err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, "cannot get secure JSON data")
		}
		req.SecureJSONData = secureData
	}

	resp, err := e.client.CreateDataSource(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create data source in Grafana")
	}

	// Set the external name in format <orgId>:<uid>
	meta.SetExternalName(cr, formatExternalName(e.orgID, resp.UID))

	// Update status
	cr.Status.AtProvider.ID = &resp.ID
	cr.Status.AtProvider.UID = &resp.UID

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) { //nolint:gocyclo
	cr, ok := mg.(*v1alpha1.DataSource)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotDataSource)
	}

	// Parse the external name to get the UID
	externalName := meta.GetExternalName(cr)
	_, uid, err := parseExternalName(externalName)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse external name")
	}

	fp := cr.Spec.ForProvider

	req := grafana.DataSourceCreateRequest{
		Name: fp.Name,
		Type: fp.Type,
		UID:  uid,
	}

	if fp.URL != nil {
		req.URL = *fp.URL
	}
	if fp.AccessMode != nil {
		req.Access = *fp.AccessMode
	} else {
		req.Access = "proxy"
	}
	if fp.IsDefault != nil {
		req.IsDefault = *fp.IsDefault
	}
	if fp.BasicAuthEnabled != nil {
		req.BasicAuth = *fp.BasicAuthEnabled
	}
	if fp.BasicAuthUsername != nil {
		req.BasicAuthUser = *fp.BasicAuthUsername
	}
	if fp.DatabaseName != nil {
		req.Database = *fp.DatabaseName
	}
	if fp.Username != nil {
		req.User = *fp.Username
	}

	// Parse JSONData if specified
	if fp.JSONDataEncoded != nil {
		var jsonData map[string]any
		if err := json.Unmarshal([]byte(*fp.JSONDataEncoded), &jsonData); err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, "cannot parse jsonDataEncoded")
		}
		req.JSONData = jsonData
	}

	// Get secure JSON data from secret if specified
	if fp.SecureJSONDataEncodedSecretRef != nil {
		secureData, err := e.getSecureJSONData(ctx, fp.SecureJSONDataEncodedSecretRef, cr.Namespace)
		if err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, "cannot get secure JSON data")
		}
		req.SecureJSONData = secureData
	}

	resp, err := e.client.UpdateDataSource(ctx, uid, req)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update data source in Grafana")
	}

	// Update status
	cr.Status.AtProvider.ID = &resp.ID
	cr.Status.AtProvider.UID = &resp.UID

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.DataSource)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotDataSource)
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

	if err := e.client.DeleteDataSourceByUID(ctx, uid); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete data source from Grafana")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}

// getSecureJSONData retrieves secure JSON data from a secret.
func (e *external) getSecureJSONData(ctx context.Context, ref *xpv1.SecretKeySelector, namespace string) (map[string]any, error) {
	secret := &corev1.Secret{}
	ns := ref.Namespace
	if ns == "" {
		ns = namespace
	}
	if err := e.kube.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      ref.Name,
	}, secret); err != nil {
		return nil, err
	}

	data := secret.Data[ref.Key]
	if data == nil {
		return nil, errors.Errorf("key %s not found in secret %s/%s", ref.Key, ns, ref.Name)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal secure JSON data")
	}

	return result, nil
}
