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

// NotificationPolicyParameters are the configurable fields of a NotificationPolicy.
type NotificationPolicyParameters struct {
	// ContactPoint is the default contact point to use when no routing matches.
	// +optional
	ContactPoint *string `json:"contactPoint,omitempty"`

	// ContactPointRef is a reference to a ContactPoint to populate contactPoint.
	// +optional
	ContactPointRef *xpv1.Reference `json:"contactPointRef,omitempty"`

	// ContactPointSelector selects a ContactPoint to populate contactPoint.
	// +optional
	ContactPointSelector *xpv1.Selector `json:"contactPointSelector,omitempty"`

	// GroupBy is a list of alert labels to group alerts by.
	// Use "..." to group by all labels.
	// +optional
	GroupBy []string `json:"groupBy,omitempty"`

	// GroupInterval is the minimum time interval between notifications for a group.
	// Default: "5m"
	// +optional
	GroupInterval *string `json:"groupInterval,omitempty"`

	// GroupWait is the time to wait before sending the first notification for a group.
	// Default: "30s"
	// +optional
	GroupWait *string `json:"groupWait,omitempty"`

	// RepeatInterval is the minimum time interval between repeated notifications.
	// Default: "4h"
	// +optional
	RepeatInterval *string `json:"repeatInterval,omitempty"`

	// Policy is a list of nested routing policies.
	// +optional
	Policy []PolicyRoute `json:"policy,omitempty"`

	// DisableProvenance allows modifying the notification policy from the Grafana UI.
	// +optional
	// +kubebuilder:default=true
	DisableProvenance *bool `json:"disableProvenance,omitempty"`

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

// PolicyRoute represents a nested routing policy.
type PolicyRoute struct {
	// ContactPoint is the contact point to use for this route.
	// +optional
	ContactPoint *string `json:"contactPoint,omitempty"`

	// ContactPointRef is a reference to a ContactPoint to populate contactPoint.
	// +optional
	ContactPointRef *xpv1.Reference `json:"contactPointRef,omitempty"`

	// ContactPointSelector selects a ContactPoint to populate contactPoint.
	// +optional
	ContactPointSelector *xpv1.Selector `json:"contactPointSelector,omitempty"`

	// Continue determines whether to continue matching subsequent routes after this one matches.
	// +optional
	Continue *bool `json:"continue,omitempty"`

	// GroupBy is a list of alert labels to group alerts by for this route.
	// +optional
	GroupBy []string `json:"groupBy,omitempty"`

	// GroupInterval is the minimum time interval between notifications for this route.
	// +optional
	GroupInterval *string `json:"groupInterval,omitempty"`

	// GroupWait is the time to wait before sending the first notification for this route.
	// +optional
	GroupWait *string `json:"groupWait,omitempty"`

	// RepeatInterval is the minimum time interval between repeated notifications for this route.
	// +optional
	RepeatInterval *string `json:"repeatInterval,omitempty"`

	// Matchers is a list of matchers to match alerts against.
	// +optional
	Matchers []PolicyMatcher `json:"matchers,omitempty"`

	// MuteTimings is a list of mute timing names to apply to this route.
	// +optional
	MuteTimings []string `json:"muteTimings,omitempty"`

	// ActiveTimings is a list of active timing names. Alerts are only routed
	// if the current time falls within an active timing.
	// +optional
	ActiveTimings []string `json:"activeTimings,omitempty"`

	// Policy is a list of nested routing policies within this route.
	// Note: Maximum nesting depth is 3 levels.
	// +optional
	Policy []NestedPolicyRoute `json:"policy,omitempty"`
}

// NestedPolicyRoute represents a second-level nested routing policy.
type NestedPolicyRoute struct {
	// ContactPoint is the contact point to use for this route.
	// +optional
	ContactPoint *string `json:"contactPoint,omitempty"`

	// ContactPointRef is a reference to a ContactPoint to populate contactPoint.
	// +optional
	ContactPointRef *xpv1.Reference `json:"contactPointRef,omitempty"`

	// ContactPointSelector selects a ContactPoint to populate contactPoint.
	// +optional
	ContactPointSelector *xpv1.Selector `json:"contactPointSelector,omitempty"`

	// Continue determines whether to continue matching subsequent routes after this one matches.
	// +optional
	Continue *bool `json:"continue,omitempty"`

	// GroupBy is a list of alert labels to group alerts by for this route.
	// +optional
	GroupBy []string `json:"groupBy,omitempty"`

	// GroupInterval is the minimum time interval between notifications for this route.
	// +optional
	GroupInterval *string `json:"groupInterval,omitempty"`

	// GroupWait is the time to wait before sending the first notification for this route.
	// +optional
	GroupWait *string `json:"groupWait,omitempty"`

	// RepeatInterval is the minimum time interval between repeated notifications for this route.
	// +optional
	RepeatInterval *string `json:"repeatInterval,omitempty"`

	// Matchers is a list of matchers to match alerts against.
	// +optional
	Matchers []PolicyMatcher `json:"matchers,omitempty"`

	// MuteTimings is a list of mute timing names to apply to this route.
	// +optional
	MuteTimings []string `json:"muteTimings,omitempty"`

	// ActiveTimings is a list of active timing names. Alerts are only routed
	// if the current time falls within an active timing.
	// +optional
	ActiveTimings []string `json:"activeTimings,omitempty"`

	// Policy is a list of nested routing policies within this route.
	// Note: This is the final nesting level.
	// +optional
	Policy []LeafPolicyRoute `json:"policy,omitempty"`
}

// LeafPolicyRoute represents the deepest level of routing policy (no further nesting).
type LeafPolicyRoute struct {
	// ContactPoint is the contact point to use for this route.
	// +optional
	ContactPoint *string `json:"contactPoint,omitempty"`

	// ContactPointRef is a reference to a ContactPoint to populate contactPoint.
	// +optional
	ContactPointRef *xpv1.Reference `json:"contactPointRef,omitempty"`

	// ContactPointSelector selects a ContactPoint to populate contactPoint.
	// +optional
	ContactPointSelector *xpv1.Selector `json:"contactPointSelector,omitempty"`

	// Continue determines whether to continue matching subsequent routes after this one matches.
	// +optional
	Continue *bool `json:"continue,omitempty"`

	// GroupBy is a list of alert labels to group alerts by for this route.
	// +optional
	GroupBy []string `json:"groupBy,omitempty"`

	// GroupInterval is the minimum time interval between notifications for this route.
	// +optional
	GroupInterval *string `json:"groupInterval,omitempty"`

	// GroupWait is the time to wait before sending the first notification for this route.
	// +optional
	GroupWait *string `json:"groupWait,omitempty"`

	// RepeatInterval is the minimum time interval between repeated notifications for this route.
	// +optional
	RepeatInterval *string `json:"repeatInterval,omitempty"`

	// Matchers is a list of matchers to match alerts against.
	// +optional
	Matchers []PolicyMatcher `json:"matchers,omitempty"`

	// MuteTimings is a list of mute timing names to apply to this route.
	// +optional
	MuteTimings []string `json:"muteTimings,omitempty"`

	// ActiveTimings is a list of active timing names. Alerts are only routed
	// if the current time falls within an active timing.
	// +optional
	ActiveTimings []string `json:"activeTimings,omitempty"`
}

// PolicyMatcher represents a matcher for routing alerts.
type PolicyMatcher struct {
	// Label is the name of the label to match.
	// +kubebuilder:validation:Required
	Label string `json:"label"`

	// Match is the type of match to perform.
	// Valid values are: "=", "!=", "=~", "!~"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="=";"!=";"=~";"!~"
	Match string `json:"match"`

	// Value is the value to match against.
	// +kubebuilder:validation:Required
	Value string `json:"value"`
}

// NotificationPolicyObservation are the observable fields of a NotificationPolicy.
type NotificationPolicyObservation struct {
	// OrgID is the Organization ID.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`
}

// NotificationPolicySpec defines the desired state of a NotificationPolicy.
type NotificationPolicySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              NotificationPolicyParameters `json:"forProvider"`
}

// NotificationPolicyStatus represents the observed state of a NotificationPolicy.
type NotificationPolicyStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          NotificationPolicyObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// NotificationPolicy is the Schema for the NotificationPolicy API.
// It manages the entire notification policy tree for a Grafana organization.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type NotificationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NotificationPolicySpec   `json:"spec"`
	Status            NotificationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NotificationPolicyList contains a list of NotificationPolicy.
type NotificationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NotificationPolicy `json:"items"`
}

// NotificationPolicy type metadata.
var (
	NotificationPolicyKind             = reflect.TypeOf(NotificationPolicy{}).Name()
	NotificationPolicyGroupKind        = schema.GroupKind{Group: Group, Kind: NotificationPolicyKind}.String()
	NotificationPolicyKindAPIVersion   = NotificationPolicyKind + "." + SchemeGroupVersion.String()
	NotificationPolicyGroupVersionKind = SchemeGroupVersion.WithKind(NotificationPolicyKind)
)

func init() {
	SchemeBuilder.Register(&NotificationPolicy{}, &NotificationPolicyList{})
}
