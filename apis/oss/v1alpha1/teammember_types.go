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

// TeamMemberParameters are the configurable fields of a TeamMember.
type TeamMemberParameters struct {
	// TeamID is the numeric ID of the team to add the user to. Either TeamID,
	// TeamRef or TeamSelector is required.
	// +optional
	TeamID *int64 `json:"teamId,omitempty"`

	// TeamRef is a reference to a Team resource to populate teamId.
	// +optional
	TeamRef *xpv1.Reference `json:"teamRef,omitempty"`

	// TeamSelector selects a Team resource to populate teamId.
	// +optional
	TeamSelector *xpv1.Selector `json:"teamSelector,omitempty"`

	// UserID is the numeric ID of the user to add to the team. Either UserID or
	// LoginOrEmail is required. UserID is preferred because it is stable.
	// +optional
	UserID *int64 `json:"userId,omitempty"`

	// LoginOrEmail is the login or email address of the user to add to the
	// team, matched case-insensitively. The user must already belong to the
	// same organization as the team - Grafana will not add a user to a team
	// in an organization they are not a member of.
	// +optional
	LoginOrEmail *string `json:"loginOrEmail,omitempty"`

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

// TeamMemberObservation is the observed state of a single team member. It is
// shared by TeamMember and by the member list reported on a Team.
type TeamMemberObservation struct {
	// ID is the unique identifier of the membership, in the format
	// <orgId>:<teamId>:<userId>.
	// +optional
	ID *string `json:"id,omitempty"`

	// TeamID is the numeric ID of the team the user belongs to.
	// +optional
	TeamID *int64 `json:"teamId,omitempty"`

	// UserID is the numeric ID of the user.
	// +optional
	UserID *int64 `json:"userId,omitempty"`

	// Login is the login name of the user.
	// +optional
	Login *string `json:"login,omitempty"`

	// Email is the email address of the user.
	// +optional
	Email *string `json:"email,omitempty"`

	// OrgID is the Organization ID.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`
}

// TeamMemberSpec defines the desired state of a TeamMember.
type TeamMemberSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              TeamMemberParameters `json:"forProvider"`
}

// TeamMemberStatus represents the observed state of a TeamMember.
type TeamMemberStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          TeamMemberObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// TeamMember manages a single user's membership of a Grafana team. Deleting
// this resource removes the user from the team.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="TEAM",type="integer",JSONPath=".status.atProvider.teamId"
// +kubebuilder:printcolumn:name="USER",type="string",JSONPath=".status.atProvider.login"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type TeamMember struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TeamMemberSpec   `json:"spec"`
	Status            TeamMemberStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TeamMemberList contains a list of TeamMember.
type TeamMemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TeamMember `json:"items"`
}

// TeamMember type metadata.
var (
	TeamMemberKind             = reflect.TypeOf(TeamMember{}).Name()
	TeamMemberGroupKind        = schema.GroupKind{Group: Group, Kind: TeamMemberKind}.String()
	TeamMemberKindAPIVersion   = TeamMemberKind + "." + SchemeGroupVersion.String()
	TeamMemberGroupVersionKind = SchemeGroupVersion.WithKind(TeamMemberKind)
)

func init() {
	SchemeBuilder.Register(&TeamMember{}, &TeamMemberList{})
}
