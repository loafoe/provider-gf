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

// TeamParameters are the configurable fields of a Team.
type TeamParameters struct {
	// Name is the name of the team. It must be unique within the organization.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Email is the team email address, used for notifications such as team
	// invitations. Grafana accepts an empty string, so an omitted field is
	// reconciled to "".
	// +optional
	Email *string `json:"email,omitempty"`

	// Members is the list of email addresses of users who should be regular
	// members of the team. Users must already exist in Grafana.
	//
	// Membership is only managed when this field or Admins is set. When either
	// is set the whole membership set is authoritative: users not listed in
	// either field are removed from the team. Leave both unset to manage
	// membership exclusively with TeamMember resources.
	// +optional
	Members []string `json:"members,omitempty"`

	// Admins is the list of email addresses of users who should be admins of
	// the team. Users must already exist in Grafana.
	//
	// Grafana's team member API does not report which members are admins, so a
	// membership change made outside Crossplane is only detected when the set
	// of member email addresses changes.
	// +optional
	Admins []string `json:"admins,omitempty"`

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

// TeamObservation are the observable fields of a Team.
type TeamObservation struct {
	// ID is the numeric ID of the team in Grafana.
	// +optional
	ID *int64 `json:"id,omitempty"`

	// UID is the UID of the team in Grafana.
	// +optional
	UID *string `json:"uid,omitempty"`

	// OrgID is the Organization ID the team belongs to.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`

	// Name is the name of the team.
	// +optional
	Name *string `json:"name,omitempty"`

	// Email is the team email address.
	// +optional
	Email *string `json:"email,omitempty"`

	// MemberCount is the number of users in the team.
	// +optional
	MemberCount *int64 `json:"memberCount,omitempty"`

	// Members are the users currently in the team.
	// +optional
	Members []TeamMemberObservation `json:"members,omitempty"`
}

// TeamSpec defines the desired state of a Team.
type TeamSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              TeamParameters `json:"forProvider"`
}

// TeamStatus represents the observed state of a Team.
type TeamStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          TeamObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// Team is the Schema for the Team API.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="TEAM",type="string",JSONPath=".status.atProvider.name"
// +kubebuilder:printcolumn:name="MEMBERS",type="integer",JSONPath=".status.atProvider.memberCount"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type Team struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TeamSpec   `json:"spec"`
	Status            TeamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TeamList contains a list of Team.
type TeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Team `json:"items"`
}

// Team type metadata.
var (
	TeamKind             = reflect.TypeOf(Team{}).Name()
	TeamGroupKind        = schema.GroupKind{Group: Group, Kind: TeamKind}.String()
	TeamKindAPIVersion   = TeamKind + "." + SchemeGroupVersion.String()
	TeamGroupVersionKind = SchemeGroupVersion.WithKind(TeamKind)
)

func init() {
	SchemeBuilder.Register(&Team{}, &TeamList{})
}
