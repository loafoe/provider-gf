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

// TeamPreferenceParameters are the configurable fields of a TeamPreference.
// There is at most one TeamPreference per team.
type TeamPreferenceParameters struct {
	// TeamID is the numeric ID of the team to manage preferences for. Either
	// TeamID, TeamRef or TeamSelector is required.
	// +optional
	TeamID *int64 `json:"teamId,omitempty"`

	// TeamRef is a reference to a Team resource to populate teamId.
	// +optional
	TeamRef *xpv1.Reference `json:"teamRef,omitempty"`

	// TeamSelector selects a Team resource to populate teamId.
	// +optional
	TeamSelector *xpv1.Selector `json:"teamSelector,omitempty"`

	// Theme is the theme used when viewing this team. One of light, dark, or
	// the empty string for the Grafana default. Grafana replaces the whole
	// preference object on update, so an omitted field is reconciled to the
	// Grafana default rather than being left untouched.
	// +kubebuilder:validation:Enum=light;dark;""
	// +optional
	Theme *string `json:"theme,omitempty"`

	// Timezone is the timezone used when viewing this team. One of utc,
	// browser, or the empty string for the Grafana default. An omitted field
	// is reconciled to the Grafana default.
	// +kubebuilder:validation:Enum=utc;browser;""
	// +optional
	Timezone *string `json:"timezone,omitempty"`

	// HomeDashboardUID is the UID of the dashboard used as this team's home
	// dashboard. An omitted field is reconciled to the Grafana default, i.e.
	// no home dashboard.
	// +optional
	HomeDashboardUID *string `json:"homeDashboardUid,omitempty"`

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

// TeamPreferenceObservation are the observable fields of a TeamPreference.
type TeamPreferenceObservation struct {
	// ID is the unique identifier of the preferences, in the format
	// <orgId>:<teamId>.
	// +optional
	ID *string `json:"id,omitempty"`

	// TeamID is the numeric ID of the team.
	// +optional
	TeamID *int64 `json:"teamId,omitempty"`

	// Theme is the observed theme.
	// +optional
	Theme *string `json:"theme,omitempty"`

	// Timezone is the observed timezone.
	// +optional
	Timezone *string `json:"timezone,omitempty"`

	// HomeDashboardUID is the UID of the team's home dashboard.
	// +optional
	HomeDashboardUID *string `json:"homeDashboardUid,omitempty"`

	// HomeDashboardID is the numeric ID of the team's home dashboard.
	// +optional
	HomeDashboardID *int64 `json:"homeDashboardId,omitempty"`

	// OrgID is the Organization ID.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`
}

// TeamPreferenceSpec defines the desired state of a TeamPreference.
type TeamPreferenceSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              TeamPreferenceParameters `json:"forProvider"`
}

// TeamPreferenceStatus represents the observed state of a TeamPreference.
type TeamPreferenceStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          TeamPreferenceObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// TeamPreference manages the UI preferences of a Grafana team.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="TEAM",type="integer",JSONPath=".status.atProvider.teamId"
// +kubebuilder:printcolumn:name="THEME",type="string",JSONPath=".status.atProvider.theme"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type TeamPreference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TeamPreferenceSpec   `json:"spec"`
	Status            TeamPreferenceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TeamPreferenceList contains a list of TeamPreference.
type TeamPreferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TeamPreference `json:"items"`
}

// TeamPreference type metadata.
var (
	TeamPreferenceKind             = reflect.TypeOf(TeamPreference{}).Name()
	TeamPreferenceGroupKind        = schema.GroupKind{Group: Group, Kind: TeamPreferenceKind}.String()
	TeamPreferenceKindAPIVersion   = TeamPreferenceKind + "." + SchemeGroupVersion.String()
	TeamPreferenceGroupVersionKind = SchemeGroupVersion.WithKind(TeamPreferenceKind)
)

func init() {
	SchemeBuilder.Register(&TeamPreference{}, &TeamPreferenceList{})
}
