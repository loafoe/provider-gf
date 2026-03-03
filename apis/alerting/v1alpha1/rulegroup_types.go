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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
)

// RuleGroupParameters are the configurable fields of a RuleGroup.
type RuleGroupParameters struct {
	// Name is the name of the rule group.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// FolderUID is the UID of the folder containing the rule group.
	// +optional
	FolderUID *string `json:"folderUid,omitempty"`

	// FolderRef is a reference to a Folder to populate folderUid.
	// +optional
	FolderRef *xpv1.Reference `json:"folderRef,omitempty"`

	// FolderSelector selects a Folder to populate folderUid.
	// +optional
	FolderSelector *xpv1.Selector `json:"folderSelector,omitempty"`

	// IntervalSeconds is the evaluation interval in seconds.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=10
	IntervalSeconds int64 `json:"intervalSeconds"`

	// Rules is the list of alert rules in the group.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Rules []Rule `json:"rules"`

	// DisableProvenance allows modifying the rule group from the Grafana UI.
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

// Rule represents an individual alert rule within a rule group.
type Rule struct {
	// UID is the unique identifier for the rule. If not specified, Grafana will generate one.
	// +optional
	UID *string `json:"uid,omitempty"`

	// Title is the name of the alert rule.
	// +kubebuilder:validation:Required
	Title string `json:"title"`

	// Condition is the refId of the query or expression that is the alert condition.
	// +kubebuilder:validation:Required
	Condition string `json:"condition"`

	// Data contains the queries and expressions that are evaluated to determine the alert state.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Data []RuleQuery `json:"data"`

	// For is the duration for which the condition must be true before the alert fires.
	// +optional
	// +kubebuilder:default="5m"
	For *string `json:"for,omitempty"`

	// NoDataState defines the behavior when the alert query returns no data.
	// +optional
	// +kubebuilder:validation:Enum=Alerting;NoData;OK
	// +kubebuilder:default="NoData"
	NoDataState *string `json:"noDataState,omitempty"`

	// ExecErrState defines the behavior when the alert query fails.
	// +optional
	// +kubebuilder:validation:Enum=Alerting;Error;OK
	// +kubebuilder:default="Error"
	ExecErrState *string `json:"execErrState,omitempty"`

	// Labels are key-value pairs that are attached to the alert.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are key-value pairs that provide additional context for the alert.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// IsPaused determines whether the alert rule is paused.
	// +optional
	IsPaused *bool `json:"isPaused,omitempty"`

	// NotificationSettings configures how notifications are sent for this rule.
	// +optional
	NotificationSettings *NotificationSettings `json:"notificationSettings,omitempty"`
}

// RuleQuery represents a query or expression within an alert rule.
type RuleQuery struct {
	// RefID is the unique identifier of the query within the rule.
	// +kubebuilder:validation:Required
	RefID string `json:"refId"`

	// DatasourceUID is the UID of the datasource to query.
	// Use "__expr__" for expression queries.
	// +kubebuilder:validation:Required
	DatasourceUID string `json:"datasourceUid"`

	// QueryType specifies the type of query.
	// +optional
	QueryType *string `json:"queryType,omitempty"`

	// RelativeTimeRange specifies the time range for the query relative to the evaluation time.
	// +optional
	RelativeTimeRange *RelativeTimeRange `json:"relativeTimeRange,omitempty"`

	// Model contains the query-specific model. The structure depends on the datasource.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Model runtime.RawExtension `json:"model"`
}

// RelativeTimeRange specifies the time range relative to the evaluation time.
type RelativeTimeRange struct {
	// From is the start of the time range in seconds relative to the evaluation time.
	// +kubebuilder:validation:Required
	From int64 `json:"from"`

	// To is the end of the time range in seconds relative to the evaluation time.
	// +kubebuilder:validation:Required
	To int64 `json:"to"`
}

// NotificationSettings configures notification behavior for an alert rule.
type NotificationSettings struct {
	// Receiver is the name of the contact point to send notifications to.
	// +kubebuilder:validation:Required
	Receiver string `json:"receiver"`

	// GroupBy is a list of labels to group alerts by.
	// +optional
	GroupBy []string `json:"groupBy,omitempty"`

	// GroupWait is the time to wait before sending the first notification.
	// +optional
	GroupWait *string `json:"groupWait,omitempty"`

	// GroupInterval is the minimum time between notifications for a group.
	// +optional
	GroupInterval *string `json:"groupInterval,omitempty"`

	// RepeatInterval is the minimum time between repeated notifications.
	// +optional
	RepeatInterval *string `json:"repeatInterval,omitempty"`

	// MuteTimeIntervals is a list of mute time interval names.
	// +optional
	MuteTimeIntervals []string `json:"muteTimeIntervals,omitempty"`
}

// RuleGroupObservation are the observable fields of a RuleGroup.
type RuleGroupObservation struct {
	// FolderUID is the UID of the folder containing the rule group.
	// +optional
	FolderUID *string `json:"folderUid,omitempty"`

	// OrgID is the Organization ID.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`

	// Rules contains the observed state of the rules.
	// +optional
	Rules []RuleObservation `json:"rules,omitempty"`
}

// RuleObservation represents the observed state of a rule.
type RuleObservation struct {
	// UID is the unique identifier of the rule.
	// +optional
	UID *string `json:"uid,omitempty"`

	// Title is the name of the alert rule.
	// +optional
	Title *string `json:"title,omitempty"`
}

// RuleGroupSpec defines the desired state of a RuleGroup.
type RuleGroupSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              RuleGroupParameters `json:"forProvider"`
}

// RuleGroupStatus represents the observed state of a RuleGroup.
type RuleGroupStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          RuleGroupObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// RuleGroup is the Schema for the RuleGroup API.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type RuleGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RuleGroupSpec   `json:"spec"`
	Status            RuleGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RuleGroupList contains a list of RuleGroup.
type RuleGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RuleGroup `json:"items"`
}

// RuleGroup type metadata.
var (
	RuleGroupKind             = reflect.TypeOf(RuleGroup{}).Name()
	RuleGroupGroupKind        = schema.GroupKind{Group: Group, Kind: RuleGroupKind}.String()
	RuleGroupKindAPIVersion   = RuleGroupKind + "." + SchemeGroupVersion.String()
	RuleGroupGroupVersionKind = SchemeGroupVersion.WithKind(RuleGroupKind)
)

func init() {
	SchemeBuilder.Register(&RuleGroup{}, &RuleGroupList{})
}
