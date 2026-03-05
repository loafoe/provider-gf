# Provider-GF Resource Implementation Guide

This document provides a step-by-step guide for implementing new resources in the Provider-GF Crossplane provider.

## Overview

Provider-GF is a Crossplane v2 provider for Grafana with namespace-scoped resources. All resources follow a consistent pattern for implementation.

## File Structure

When implementing a new resource, you need to create/modify these files:

1. **Types**: `apis/<group>/v1alpha1/<resource>_types.go`
2. **Controller**: `internal/controller/<resource>/<resource>.go`
3. **Client Methods**: `internal/clients/grafana/grafana.go`
4. **Register Controller**: `internal/controller/register.go`
5. **Example**: `examples/<group>/<resource>.yaml`

## Step-by-Step Implementation

### 1. Define Types (`apis/<group>/v1alpha1/<resource>_types.go`)

```go
package v1alpha1

import (
    "reflect"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime/schema"
    xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
    xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
)

// <Resource>Parameters are the configurable fields
type <Resource>Parameters struct {
    // Required fields
    Name string `json:"name"`

    // Optional fields
    // +optional
    SomeField *string `json:"someField,omitempty"`

    // References to other resources (use for Crossplane resources)
    // +optional
    SomeRef *xpv1.Reference `json:"someRef,omitempty"`
    // +optional
    SomeSelector *xpv1.Selector `json:"someSelector,omitempty"`

    // Org reference (standard pattern)
    // +optional
    OrgID *int64 `json:"orgId,omitempty"`
    // +optional
    OrgRef *xpv1.Reference `json:"orgRef,omitempty"`
    // +optional
    OrgSelector *xpv1.Selector `json:"orgSelector,omitempty"`
}

// <Resource>Observation are the observable fields
type <Resource>Observation struct {
    UID   *string `json:"uid,omitempty"`
    OrgID *int64  `json:"orgId,omitempty"`
}

// <Resource>Spec defines the desired state
type <Resource>Spec struct {
    xpv2.ManagedResourceSpec `json:",inline"`
    ForProvider              <Resource>Parameters `json:"forProvider"`
}

// <Resource>Status represents the observed state
type <Resource>Status struct {
    xpv1.ResourceStatus `json:",inline"`
    AtProvider          <Resource>Observation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type <Resource> struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              <Resource>Spec   `json:"spec"`
    Status            <Resource>Status `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type <Resource>List struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []<Resource> `json:"items"`
}

var (
    <Resource>Kind             = reflect.TypeOf(<Resource>{}).Name()
    <Resource>GroupKind        = schema.GroupKind{Group: Group, Kind: <Resource>Kind}.String()
    <Resource>KindAPIVersion   = <Resource>Kind + "." + SchemeGroupVersion.String()
    <Resource>GroupVersionKind = SchemeGroupVersion.WithKind(<Resource>Kind)
)

func init() {
    SchemeBuilder.Register(&<Resource>{}, &<Resource>List{})
}
```

### 2. Add Client Methods (`internal/clients/grafana/grafana.go`)

Add the Grafana API types and client methods:

```go
// <Resource> represents the Grafana API model
type <Resource> struct {
    UID  string `json:"uid,omitempty"`
    Name string `json:"name"`
    // ... other fields
}

// Get<Resource>ByUID retrieves a resource by UID
func (c *Client) Get<Resource>ByUID(ctx context.Context, uid string) (*<Resource>, error) {
    resp, err := c.doRequest(ctx, http.MethodGet, "/api/<resource>/"+uid, nil)
    // ... handle response
}

// Create<Resource> creates a new resource
func (c *Client) Create<Resource>(ctx context.Context, req <Resource>Request) (*<Resource>, error) {
    // ... implement
}

// Update<Resource> updates an existing resource
func (c *Client) Update<Resource>(ctx context.Context, uid string, req <Resource>Request) error {
    // ... implement
}

// Delete<Resource>ByUID deletes a resource
func (c *Client) Delete<Resource>ByUID(ctx context.Context, uid string) error {
    // ... implement
}
```

### 3. Create Controller (`internal/controller/<resource>/<resource>.go`)

Follow this pattern for the controller:

```go
package <resource>

import (
    "context"
    "strconv"
    "strings"
    // ... imports
)

const (
    errNot<Resource>        = "managed resource is not a <Resource>"
    errTrackPCUsage         = "cannot track ProviderConfig usage"
    errGetPC                = "cannot get ProviderConfig"
    errInvalidExternalName  = "invalid external name format"
    errResolveOrgRef        = "cannot resolve organization reference"
)

// External name format: <orgId>:<uid>
func formatExternalName(orgID int64, uid string) string {
    return strconv.FormatInt(orgID, 10) + ":" + uid
}

func parseExternalName(externalName string) (int64, string, error) {
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

func Setup(mgr ctrl.Manager, o controller.Options) error {
    // Standard setup - see existing controllers
}

type connector struct {
    kube  client.Client
    usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
    // 1. Type assertion
    // 2. Track ProviderConfig usage
    // 3. Get ProviderConfig
    // 4. Create Grafana client
    // 5. Resolve OrgID
    // Return external client
}

type external struct {
    client *grafana.Client
    kube   client.Client
    orgID  int64
}

// CRITICAL: Observe() must handle race-condition recovery
func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
    cr := mg.(*v1alpha1.<Resource>)

    // 1. Try to get UID from external-name first
    externalName := meta.GetExternalName(cr)
    var uid string
    if externalName != "" {
        _, parsedUID, err := parseExternalName(externalName)
        if err == nil {
            uid = parsedUID
        }
    }

    // 2. RECOVERY: If no UID, try to find by spec UID or other identifier
    if uid == "" && cr.Spec.ForProvider.UID != nil {
        uid = *cr.Spec.ForProvider.UID
    }

    // 3. If still no UID, resource doesn't exist
    if uid == "" {
        return managed.ExternalObservation{ResourceExists: false}, nil
    }

    // 4. Get from Grafana
    resource, err := e.client.Get<Resource>ByUID(ctx, uid)
    if resource == nil {
        return managed.ExternalObservation{ResourceExists: false}, nil
    }

    // 5. Set external name if recovered (CRITICAL!)
    if externalName == "" || meta.GetExternalName(cr) != formatExternalName(e.orgID, uid) {
        meta.SetExternalName(cr, formatExternalName(e.orgID, uid))
    }

    // 6. Update status
    cr.Status.AtProvider.UID = &resource.UID
    cr.Status.SetConditions(xpv1.Available())

    return managed.ExternalObservation{
        ResourceExists:   true,
        ResourceUpToDate: e.isUpToDate(cr, resource),
    }, nil
}

// IMPORTANT: Do NOT use meta.WasDeleted() check in Observe
// This prevents Delete from being called properly

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
    cr := mg.(*v1alpha1.<Resource>)
    cr.Status.SetConditions(xpv1.Creating())

    // Build request and create
    resp, err := e.client.Create<Resource>(ctx, /* request */)
    if err != nil {
        return managed.ExternalCreation{}, errors.Wrap(err, "cannot create")
    }

    meta.SetExternalName(cr, formatExternalName(e.orgID, resp.UID))
    return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
    // Parse external name, build request, update
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
    cr := mg.(*v1alpha1.<Resource>)
    cr.Status.SetConditions(xpv1.Deleting())

    externalName := meta.GetExternalName(cr)
    if externalName == "" {
        return managed.ExternalDelete{}, nil
    }

    _, uid, _ := parseExternalName(externalName)
    e.client.Delete<Resource>ByUID(ctx, uid)
    return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
    return nil
}
```

### 4. Register Controller (`internal/controller/register.go`)

Add import and setup:

```go
import (
    // ...
    "<module>/internal/controller/<resource>"
)

func Setup(mgr ctrl.Manager, o controller.Options) error {
    for _, setup := range []func(ctrl.Manager, controller.Options) error{
        // ...existing controllers...
        <resource>.Setup,
    } {
        // ...
    }
}
```

### 5. Create Example (`examples/<group>/<resource>.yaml`)

```yaml
apiVersion: <group>.gf.m.crossplane.io/v1alpha1
kind: <Resource>
metadata:
  name: example-<resource>
  namespace: default
spec:
  providerConfigRef:
    name: grafana-config
    kind: ProviderConfig
  forProvider:
    name: "Example Resource"
    # ... other fields
```

## Key Patterns

### External Name Format

All resources use `<orgId>:<uid>` format except:
- `NotificationPolicy`: Uses just `<orgId>` (one per org)
- `RuleGroup`: Uses `<orgId>:<folderUid>:<groupName>`

### Reference Resolution

For references to other Crossplane resources:

```go
// Define extractor function
func ExtractUID() reference.ExtractValueFn {
    return func(mg resource.Managed) string {
        r, ok := mg.(*v1alpha1.<OtherResource>)
        if !ok {
            return ""
        }
        // Extract from external name or status
    }
}

// Use in Connect or controller
rsp, err := reference.NewAPIResolver(c.kube, cr).Resolve(ctx, reference.ResolutionRequest{
    CurrentValue: "",
    Reference:    cr.Spec.ForProvider.SomeRef,
    Selector:     cr.Spec.ForProvider.SomeSelector,
    To:           reference.To{Managed: &v1alpha1.<OtherResource>{}, List: &v1alpha1.<OtherResource>List{}},
    Extract:      ExtractUID(),
    Namespace:    cr.GetNamespace(),
})
```

### Permission Resources Pattern

For `*Permission` resources (DashboardPermission, FolderPermission):
- Use reference resolution with fallback to external-name for deletion scenarios
- Check `cr.GetDeletionTimestamp() != nil && len(permissions) == 0` to return `ResourceExists: false` during delete

### Disable Provenance

Most alerting resources support `disableProvenance` to allow UI editing:

```go
disableProvenance := true
if cr.Spec.ForProvider.DisableProvenance != nil {
    disableProvenance = *cr.Spec.ForProvider.DisableProvenance
}
// Pass to API request header: X-Disable-Provenance
```

## Code Generation

After creating types, run:

```bash
make generate
```

This generates:
- `zz_generated.deepcopy.go` - DeepCopy methods
- `zz_generated.managed.go` - Managed interface methods
- `zz_generated.managedlist.go` - ManagedList interface methods
- CRDs in `package/crds/`

## Testing

1. Apply CRDs: `kubectl apply -R -f package/crds`
2. Create test resources with appropriate ProviderConfig
3. Run locally: `make run`
4. Check status: `kubectl get <resource> -n <namespace>`
5. Debug: `kubectl describe <resource> <name> -n <namespace>`

## Common Issues

### Recursive Types

If you have recursive types (like nested policies), break them into explicit levels to avoid infinite recursion in code generation:
- `PolicyRoute` -> contains `[]NestedPolicyRoute`
- `NestedPolicyRoute` -> contains `[]LeafPolicyRoute`
- `LeafPolicyRoute` -> no further nesting

### Race Conditions with References

When a resource references another that's being created simultaneously, the first few reconciliation attempts may fail. The controller will eventually succeed once the referenced resource is ready.

### DNS Resolution

When running locally with `make run`, you can't resolve in-cluster DNS names. Use port-forwarding and a local ProviderConfig:

```yaml
spec:
  url: "http://localhost:3000"
```
