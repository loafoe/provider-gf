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

// FolderPermissionItem represents a single permission entry for a folder.
type FolderPermissionItem struct {
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

// FolderPermissionItemObservation represents the observed state of a permission entry.
type FolderPermissionItemObservation struct {
	// Permission is the permission level.
	Permission *string `json:"permission,omitempty"`

	// Role is the name of the basic role.
	Role *string `json:"role,omitempty"`

	// TeamID is the ID of the team.
	TeamID *int64 `json:"teamId,omitempty"`

	// UserID is the ID of the user.
	UserID *int64 `json:"userId,omitempty"`
}

// FolderPermissionParameters are the configurable fields of a FolderPermission.
type FolderPermissionParameters struct {
	// FolderUID is the UID of the folder to apply permissions to.
	// +optional
	FolderUID *string `json:"folderUid,omitempty"`

	// FolderRef is a reference to a Folder resource to populate folderUid.
	// +optional
	FolderRef *xpv1.Reference `json:"folderRef,omitempty"`

	// FolderSelector selects a Folder resource to populate folderUid.
	// +optional
	FolderSelector *xpv1.Selector `json:"folderSelector,omitempty"`

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
	Permissions []FolderPermissionItem `json:"permissions,omitempty"`
}

// FolderPermissionObservation are the observable fields of a FolderPermission.
type FolderPermissionObservation struct {
	// ID is the unique identifier of the folder permission set.
	ID *string `json:"id,omitempty"`

	// FolderUID is the UID of the folder.
	FolderUID *string `json:"folderUid,omitempty"`

	// OrgID is the Organization ID.
	OrgID *int64 `json:"orgId,omitempty"`

	// Permissions are the current permissions on the folder.
	Permissions []FolderPermissionItemObservation `json:"permissions,omitempty"`
}

// A FolderPermissionSpec defines the desired state of a FolderPermission.
type FolderPermissionSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              FolderPermissionParameters `json:"forProvider"`
}

// A FolderPermissionStatus represents the observed state of a FolderPermission.
type FolderPermissionStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          FolderPermissionObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A FolderPermission manages the complete set of permissions for a Grafana folder.
// Permissions that aren't specified when applying this resource will be removed.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type FolderPermission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FolderPermissionSpec   `json:"spec"`
	Status FolderPermissionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FolderPermissionList contains a list of FolderPermission
type FolderPermissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FolderPermission `json:"items"`
}

// FolderPermission type metadata.
var (
	FolderPermissionKind             = reflect.TypeOf(FolderPermission{}).Name()
	FolderPermissionGroupKind        = schema.GroupKind{Group: Group, Kind: FolderPermissionKind}.String()
	FolderPermissionKindAPIVersion   = FolderPermissionKind + "." + SchemeGroupVersion.String()
	FolderPermissionGroupVersionKind = SchemeGroupVersion.WithKind(FolderPermissionKind)
)

func init() {
	SchemeBuilder.Register(&FolderPermission{}, &FolderPermissionList{})
}
