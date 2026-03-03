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

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
)

// PermissionItem represents a single permission entry for a dashboard.
type PermissionItem struct {
	// Permission is the permission level to grant. Must be one of "View", "Edit", or "Admin".
	// +kubebuilder:validation:Enum=View;Edit;Admin
	// +kubebuilder:validation:Required
	Permission string `json:"permission"`

	// Role is the name of the basic role to manage permissions for.
	// Options: Viewer, Editor, or Admin.
	// +kubebuilder:validation:Enum=Viewer;Editor;Admin
	// +optional
	Role *string `json:"role,omitempty"`

	// TeamID is the ID of the team to manage permissions for.
	// +optional
	TeamID *int64 `json:"teamId,omitempty"`

	// UserID is the ID of the user to manage permissions for.
	// +optional
	UserID *int64 `json:"userId,omitempty"`
}

// PermissionItemObservation represents the observed state of a permission entry.
type PermissionItemObservation struct {
	// Permission is the permission level.
	Permission *string `json:"permission,omitempty"`

	// Role is the name of the basic role.
	Role *string `json:"role,omitempty"`

	// TeamID is the ID of the team.
	TeamID *int64 `json:"teamId,omitempty"`

	// UserID is the ID of the user.
	UserID *int64 `json:"userId,omitempty"`
}

// DashboardPermissionParameters are the configurable fields of a DashboardPermission.
type DashboardPermissionParameters struct {
	// DashboardUID is the UID of the dashboard to apply permissions to.
	// +optional
	DashboardUID *string `json:"dashboardUid,omitempty"`

	// DashboardRef is a reference to a Dashboard resource to populate dashboardUid.
	// +optional
	DashboardRef *xpv1.Reference `json:"dashboardRef,omitempty"`

	// DashboardSelector selects a Dashboard resource to populate dashboardUid.
	// +optional
	DashboardSelector *xpv1.Selector `json:"dashboardSelector,omitempty"`

	// OrgID is the Organization ID. If not set, the Org ID defined in the
	// provider config will be used.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`

	// OrgRef is a reference to an Organization to populate orgId.
	// +optional
	OrgRef *xpv1.Reference `json:"orgRef,omitempty"`

	// OrgSelector selects an Organization to populate orgId.
	// +optional
	OrgSelector *xpv1.Selector `json:"orgSelector,omitempty"`

	// Permissions are the permission items to add/update. Items that are omitted
	// from the list will be removed.
	// +optional
	Permissions []PermissionItem `json:"permissions,omitempty"`
}

// DashboardPermissionObservation are the observable fields of a DashboardPermission.
type DashboardPermissionObservation struct {
	// ID is the unique identifier of the dashboard permission set.
	ID *string `json:"id,omitempty"`

	// DashboardUID is the UID of the dashboard.
	DashboardUID *string `json:"dashboardUid,omitempty"`

	// OrgID is the Organization ID.
	OrgID *int64 `json:"orgId,omitempty"`

	// Permissions are the current permissions on the dashboard.
	Permissions []PermissionItemObservation `json:"permissions,omitempty"`
}

// A DashboardPermissionSpec defines the desired state of a DashboardPermission.
type DashboardPermissionSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              DashboardPermissionParameters `json:"forProvider"`
}

// A DashboardPermissionStatus represents the observed state of a DashboardPermission.
type DashboardPermissionStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          DashboardPermissionObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A DashboardPermission manages the complete set of permissions for a Grafana dashboard.
// Permissions that aren't specified when applying this resource will be removed.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type DashboardPermission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DashboardPermissionSpec   `json:"spec"`
	Status DashboardPermissionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DashboardPermissionList contains a list of DashboardPermission
type DashboardPermissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DashboardPermission `json:"items"`
}

// DashboardPermission type metadata.
var (
	DashboardPermissionKind             = reflect.TypeOf(DashboardPermission{}).Name()
	DashboardPermissionGroupKind        = schema.GroupKind{Group: Group, Kind: DashboardPermissionKind}.String()
	DashboardPermissionKindAPIVersion   = DashboardPermissionKind + "." + SchemeGroupVersion.String()
	DashboardPermissionGroupVersionKind = SchemeGroupVersion.WithKind(DashboardPermissionKind)
)

func init() {
	SchemeBuilder.Register(&DashboardPermission{}, &DashboardPermissionList{})
}
