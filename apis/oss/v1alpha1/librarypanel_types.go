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

// LibraryPanelParameters are the configurable fields of a LibraryPanel.
type LibraryPanelParameters struct {
	// Name is the name of the library panel.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ModelJSON is the JSON model of the library panel.
	// +kubebuilder:validation:Required
	ModelJSON string `json:"modelJson"`

	// UID is the unique identifier of the library panel.
	// If not specified, Grafana will generate one.
	// +optional
	UID *string `json:"uid,omitempty"`

	// FolderUID is the UID of the folder containing the library panel.
	// Use "general" for the General folder.
	// +optional
	FolderUID *string `json:"folderUid,omitempty"`

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
}

// LibraryPanelObservation are the observable fields of a LibraryPanel.
type LibraryPanelObservation struct {
	// ID is the numeric ID of the library panel.
	// +optional
	ID *int64 `json:"id,omitempty"`

	// UID is the unique identifier of the library panel.
	// +optional
	UID *string `json:"uid,omitempty"`

	// Type is the type of the panel (e.g., "table", "graph").
	// +optional
	Type *string `json:"type,omitempty"`

	// Description is the description of the library panel.
	// +optional
	Description *string `json:"description,omitempty"`

	// FolderName is the name of the folder containing the library panel.
	// +optional
	FolderName *string `json:"folderName,omitempty"`

	// FolderUID is the UID of the folder containing the library panel.
	// +optional
	FolderUID *string `json:"folderUid,omitempty"`

	// Version is the version of the library panel.
	// +optional
	Version *int64 `json:"version,omitempty"`

	// PanelID is the numeric panel ID within the model.
	// +optional
	PanelID *int64 `json:"panelId,omitempty"`

	// Created is the timestamp when the library panel was created.
	// +optional
	Created *string `json:"created,omitempty"`

	// Updated is the timestamp when the library panel was last updated.
	// +optional
	Updated *string `json:"updated,omitempty"`

	// ConnectedDashboards is the number of dashboards using this library panel.
	// +optional
	ConnectedDashboards *int64 `json:"connectedDashboards,omitempty"`

	// OrgID is the Organization ID.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`
}

// LibraryPanelSpec defines the desired state of a LibraryPanel.
type LibraryPanelSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              LibraryPanelParameters `json:"forProvider"`
}

// LibraryPanelStatus represents the observed state of a LibraryPanel.
type LibraryPanelStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          LibraryPanelObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// LibraryPanel is the Schema for the LibraryPanel API.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type LibraryPanel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              LibraryPanelSpec   `json:"spec"`
	Status            LibraryPanelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LibraryPanelList contains a list of LibraryPanel.
type LibraryPanelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LibraryPanel `json:"items"`
}

// LibraryPanel type metadata.
var (
	LibraryPanelKind             = reflect.TypeOf(LibraryPanel{}).Name()
	LibraryPanelGroupKind        = schema.GroupKind{Group: Group, Kind: LibraryPanelKind}.String()
	LibraryPanelKindAPIVersion   = LibraryPanelKind + "." + SchemeGroupVersion.String()
	LibraryPanelGroupVersionKind = SchemeGroupVersion.WithKind(LibraryPanelKind)
)

func init() {
	SchemeBuilder.Register(&LibraryPanel{}, &LibraryPanelList{})
}
