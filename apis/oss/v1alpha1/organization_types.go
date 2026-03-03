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

// OrganizationParameters are the configurable fields of an Organization.
type OrganizationParameters struct {
	// Name is the name of the organization.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Admins is a list of email addresses of users who should be granted admin
	// access to the organization. Note: users specified here must already exist
	// in Grafana.
	// +optional
	Admins []string `json:"admins,omitempty"`

	// Editors is a list of email addresses of users who should be granted editor
	// access to the organization. Note: users specified here must already exist
	// in Grafana.
	// +optional
	Editors []string `json:"editors,omitempty"`

	// Viewers is a list of email addresses of users who should be granted viewer
	// access to the organization. Note: users specified here must already exist
	// in Grafana.
	// +optional
	Viewers []string `json:"viewers,omitempty"`

	// CreateUsers specifies whether users that do not exist in Grafana should be
	// created during organization setup. Only works with basic authentication.
	// Defaults to false.
	// +optional
	CreateUsers *bool `json:"createUsers,omitempty"`
}

// OrganizationObservation are the observable fields of an Organization.
type OrganizationObservation struct {
	// ID is the numeric ID of the organization in Grafana.
	// +optional
	ID *int64 `json:"id,omitempty"`

	// Name is the name of the organization.
	// +optional
	Name *string `json:"name,omitempty"`

	// AdminUsers contains the list of admin user IDs.
	// +optional
	AdminUsers []int64 `json:"adminUsers,omitempty"`

	// EditorUsers contains the list of editor user IDs.
	// +optional
	EditorUsers []int64 `json:"editorUsers,omitempty"`

	// ViewerUsers contains the list of viewer user IDs.
	// +optional
	ViewerUsers []int64 `json:"viewerUsers,omitempty"`
}

// OrganizationSpec defines the desired state of an Organization.
type OrganizationSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              OrganizationParameters `json:"forProvider"`
}

// OrganizationStatus represents the observed state of an Organization.
type OrganizationStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          OrganizationObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// Organization is the Schema for the Organization API.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type Organization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OrganizationSpec   `json:"spec"`
	Status            OrganizationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrganizationList contains a list of Organization.
type OrganizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Organization `json:"items"`
}

// Organization type metadata.
var (
	OrganizationKind             = reflect.TypeOf(Organization{}).Name()
	OrganizationGroupKind        = schema.GroupKind{Group: Group, Kind: OrganizationKind}.String()
	OrganizationKindAPIVersion   = OrganizationKind + "." + SchemeGroupVersion.String()
	OrganizationGroupVersionKind = SchemeGroupVersion.WithKind(OrganizationKind)
)

func init() {
	SchemeBuilder.Register(&Organization{}, &OrganizationList{})
}
