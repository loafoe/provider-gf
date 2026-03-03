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

// FolderParameters are the configurable fields of a Folder.
type FolderParameters struct {
	// Title is the title of the folder.
	// +kubebuilder:validation:Required
	Title string `json:"title"`

	// UID is the unique identifier of the folder.
	// If not specified, Grafana will generate one.
	// +optional
	UID *string `json:"uid,omitempty"`

	// ParentFolderUID is the UID of the parent folder.
	// Requires the nestedFolders feature flag in Grafana.
	// +optional
	ParentFolderUID *string `json:"parentFolderUid,omitempty"`

	// ParentFolderRef is a reference to a Folder to populate parentFolderUid.
	// +optional
	ParentFolderRef *xpv1.Reference `json:"parentFolderRef,omitempty"`

	// ParentFolderSelector selects a Folder to populate parentFolderUid.
	// +optional
	ParentFolderSelector *xpv1.Selector `json:"parentFolderSelector,omitempty"`
}

// FolderObservation are the observable fields of a Folder.
type FolderObservation struct {
	// ID is the numeric ID of the folder.
	// +optional
	ID *int64 `json:"id,omitempty"`

	// UID is the unique identifier of the folder.
	// +optional
	UID *string `json:"uid,omitempty"`

	// Title is the title of the folder.
	// +optional
	Title *string `json:"title,omitempty"`

	// URL is the full URL of the folder.
	// +optional
	URL *string `json:"url,omitempty"`

	// Version is the folder version for optimistic locking.
	// +optional
	Version *int64 `json:"version,omitempty"`

	// ParentFolderUID is the UID of the parent folder.
	// +optional
	ParentFolderUID *string `json:"parentFolderUid,omitempty"`
}

// FolderSpec defines the desired state of a Folder.
type FolderSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              FolderParameters `json:"forProvider"`
}

// FolderStatus represents the observed state of a Folder.
type FolderStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          FolderObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// Folder is the Schema for the Folder API.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type Folder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FolderSpec   `json:"spec"`
	Status            FolderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FolderList contains a list of Folder.
type FolderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Folder `json:"items"`
}

// Folder type metadata.
var (
	FolderKind             = reflect.TypeOf(Folder{}).Name()
	FolderGroupKind        = schema.GroupKind{Group: Group, Kind: FolderKind}.String()
	FolderKindAPIVersion   = FolderKind + "." + SchemeGroupVersion.String()
	FolderGroupVersionKind = SchemeGroupVersion.WithKind(FolderKind)
)

func init() {
	SchemeBuilder.Register(&Folder{}, &FolderList{})
}
