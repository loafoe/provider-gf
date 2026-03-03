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

// DashboardParameters are the configurable fields of a Dashboard.
type DashboardParameters struct {
	// ConfigJSON is the complete dashboard model JSON.
	// +kubebuilder:validation:Required
	ConfigJSON string `json:"configJson"`

	// Folder is the id or UID of the folder to save the dashboard in.
	// +optional
	Folder *string `json:"folder,omitempty"`

	// Message sets a commit message for the version history.
	// +optional
	Message *string `json:"message,omitempty"`

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

	// Overwrite set to true if you want to overwrite existing dashboard with
	// newer version, same dashboard title in folder or same dashboard uid.
	// +optional
	// +kubebuilder:default=false
	Overwrite *bool `json:"overwrite,omitempty"`
}

// DashboardObservation are the observable fields of a Dashboard.
type DashboardObservation struct {
	// ConfigJSON is the complete dashboard model JSON as observed.
	ConfigJSON string `json:"configJson,omitempty"`

	// DashboardID is the numeric ID of the dashboard computed by Grafana.
	DashboardID *int64 `json:"dashboardId,omitempty"`

	// Folder is the id or UID of the folder containing the dashboard.
	Folder *string `json:"folder,omitempty"`

	// ID is the unique identifier of the dashboard.
	ID *string `json:"id,omitempty"`

	// Message is the commit message for the version history.
	Message *string `json:"message,omitempty"`

	// OrgID is the Organization ID.
	OrgID *int64 `json:"orgId,omitempty"`

	// Overwrite indicates whether existing dashboard was overwritten.
	Overwrite *bool `json:"overwrite,omitempty"`

	// UID is the unique identifier of a dashboard used to construct its URL.
	UID *string `json:"uid,omitempty"`

	// URL is the full URL of the dashboard.
	URL *string `json:"url,omitempty"`

	// Version is the version number of the dashboard, incremented each time
	// the dashboard is saved.
	Version *int64 `json:"version,omitempty"`
}

// A DashboardSpec defines the desired state of a Dashboard.
type DashboardSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              DashboardParameters `json:"forProvider"`
}

// A DashboardStatus represents the observed state of a Dashboard.
type DashboardStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          DashboardObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Dashboard is a Grafana Dashboard resource.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type Dashboard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DashboardSpec   `json:"spec"`
	Status DashboardStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DashboardList contains a list of Dashboard
type DashboardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Dashboard `json:"items"`
}

// Dashboard type metadata.
var (
	DashboardKind             = reflect.TypeOf(Dashboard{}).Name()
	DashboardGroupKind        = schema.GroupKind{Group: Group, Kind: DashboardKind}.String()
	DashboardKindAPIVersion   = DashboardKind + "." + SchemeGroupVersion.String()
	DashboardGroupVersionKind = SchemeGroupVersion.WithKind(DashboardKind)
)

func init() {
	SchemeBuilder.Register(&Dashboard{}, &DashboardList{})
}
