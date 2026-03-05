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

// ContactPointParameters are the configurable fields of a ContactPoint.
type ContactPointParameters struct {
	// Name is the name of the contact point.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// DisableProvenance allows modifying the contact point from the Grafana UI.
	// +optional
	// +kubebuilder:default=true
	DisableProvenance *bool `json:"disableProvenance,omitempty"`

	// Alertmanager configures an Alertmanager contact point.
	// +optional
	Alertmanager *AlertmanagerConfig `json:"alertmanager,omitempty"`

	// Discord configures a Discord contact point.
	// +optional
	Discord *DiscordConfig `json:"discord,omitempty"`

	// Email configures an Email contact point.
	// +optional
	Email *EmailConfig `json:"email,omitempty"`

	// Jira configures a Jira contact point.
	// +optional
	Jira *JiraConfig `json:"jira,omitempty"`

	// Kafka configures a Kafka contact point.
	// +optional
	Kafka *KafkaConfig `json:"kafka,omitempty"`

	// PagerDuty configures a PagerDuty contact point.
	// +optional
	PagerDuty *PagerDutyConfig `json:"pagerduty,omitempty"`

	// Teams configures a Microsoft Teams contact point.
	// +optional
	Teams *TeamsConfig `json:"teams,omitempty"`

	// Slack configures a Slack contact point.
	// +optional
	Slack *SlackConfig `json:"slack,omitempty"`

	// SNS configures an AWS SNS contact point.
	// +optional
	SNS *SNSConfig `json:"sns,omitempty"`

	// Telegram configures a Telegram contact point.
	// +optional
	Telegram *TelegramConfig `json:"telegram,omitempty"`

	// Webhook configures a Webhook contact point.
	// +optional
	Webhook *WebhookConfig `json:"webhook,omitempty"`

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

// AlertmanagerConfig configures an Alertmanager contact point.
type AlertmanagerConfig struct {
	// URL is the Alertmanager webhook URL.
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// BasicAuthUser is the username for basic authentication.
	// +optional
	BasicAuthUser *string `json:"basicAuthUser,omitempty"`

	// BasicAuthPasswordSecretRef is the password for basic authentication.
	// +optional
	BasicAuthPasswordSecretRef *xpv1.SecretKeySelector `json:"basicAuthPasswordSecretRef,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// DiscordConfig configures a Discord contact point.
type DiscordConfig struct {
	// URLSecretRef is the Discord webhook URL.
	// +kubebuilder:validation:Required
	URLSecretRef xpv1.SecretKeySelector `json:"urlSecretRef"`

	// Title is the message title.
	// +optional
	Title *string `json:"title,omitempty"`

	// Message is the message content.
	// +optional
	Message *string `json:"message,omitempty"`

	// AvatarURL is the URL of the avatar image.
	// +optional
	AvatarURL *string `json:"avatarUrl,omitempty"`

	// UseDiscordUsername uses the Discord username.
	// +optional
	UseDiscordUsername *bool `json:"useDiscordUsername,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// EmailConfig configures an Email contact point.
type EmailConfig struct {
	// Addresses is the list of email addresses.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Addresses []string `json:"addresses"`

	// Subject is the email subject.
	// +optional
	Subject *string `json:"subject,omitempty"`

	// Message is the email body.
	// +optional
	Message *string `json:"message,omitempty"`

	// SingleEmail sends a single email to all addresses.
	// +optional
	SingleEmail *bool `json:"singleEmail,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// JiraConfig configures a Jira contact point.
type JiraConfig struct {
	// APIURL is the Jira API URL.
	// +kubebuilder:validation:Required
	APIURL string `json:"apiUrl"`

	// APITokenSecretRef is the Jira API token.
	// +optional
	APITokenSecretRef *xpv1.SecretKeySelector `json:"apiTokenSecretRef,omitempty"`

	// UserSecretRef is the Jira username.
	// +optional
	UserSecretRef *xpv1.SecretKeySelector `json:"userSecretRef,omitempty"`

	// PasswordSecretRef is the Jira password.
	// +optional
	PasswordSecretRef *xpv1.SecretKeySelector `json:"passwordSecretRef,omitempty"`

	// Project is the Jira project key.
	// +kubebuilder:validation:Required
	Project string `json:"project"`

	// IssueType is the Jira issue type (e.g., "Bug").
	// +kubebuilder:validation:Required
	IssueType string `json:"issueType"`

	// Summary is the issue summary template.
	// +optional
	Summary *string `json:"summary,omitempty"`

	// Description is the issue description template.
	// +optional
	Description *string `json:"description,omitempty"`

	// Priority is the issue priority.
	// +optional
	Priority *string `json:"priority,omitempty"`

	// Labels is a list of labels to add to the issue.
	// +optional
	Labels []string `json:"labels,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// KafkaConfig configures a Kafka contact point.
type KafkaConfig struct {
	// RESTProxyURLSecretRef is the Kafka REST proxy URL.
	// +kubebuilder:validation:Required
	RESTProxyURLSecretRef xpv1.SecretKeySelector `json:"restProxyUrlSecretRef"`

	// Topic is the Kafka topic.
	// +kubebuilder:validation:Required
	Topic string `json:"topic"`

	// ClusterID is the Kafka cluster ID.
	// +optional
	ClusterID *string `json:"clusterId,omitempty"`

	// APIVersion is the Kafka API version.
	// +optional
	APIVersion *string `json:"apiVersion,omitempty"`

	// Username is the authentication username.
	// +optional
	Username *string `json:"username,omitempty"`

	// PasswordSecretRef is the authentication password.
	// +optional
	PasswordSecretRef *xpv1.SecretKeySelector `json:"passwordSecretRef,omitempty"`

	// Description is the event description field.
	// +optional
	Description *string `json:"description,omitempty"`

	// Details is the event details field.
	// +optional
	Details *string `json:"details,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// PagerDutyConfig configures a PagerDuty contact point.
type PagerDutyConfig struct {
	// IntegrationKeySecretRef is the PagerDuty integration key.
	// +kubebuilder:validation:Required
	IntegrationKeySecretRef xpv1.SecretKeySelector `json:"integrationKeySecretRef"`

	// Severity is the alert severity (critical, error, warning, info).
	// +optional
	Severity *string `json:"severity,omitempty"`

	// Component is the component name.
	// +optional
	Component *string `json:"component,omitempty"`

	// Class is the event classification.
	// +optional
	Class *string `json:"class,omitempty"`

	// Group is the incident group.
	// +optional
	Group *string `json:"group,omitempty"`

	// Source is the alert source.
	// +optional
	Source *string `json:"source,omitempty"`

	// Client is the client name.
	// +optional
	Client *string `json:"client,omitempty"`

	// ClientURL is the client URL.
	// +optional
	ClientURL *string `json:"clientUrl,omitempty"`

	// Summary is the event summary template.
	// +optional
	Summary *string `json:"summary,omitempty"`

	// URL is the incident URL.
	// +optional
	URL *string `json:"url,omitempty"`

	// Details is a set of arbitrary key/value pairs that provide further detail
	// about the incident.
	// +optional
	Details map[string]string `json:"details,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// TeamsConfig configures a Microsoft Teams contact point.
type TeamsConfig struct {
	// URLSecretRef is the Teams webhook URL.
	// +kubebuilder:validation:Required
	URLSecretRef xpv1.SecretKeySelector `json:"urlSecretRef"`

	// Title is the message title.
	// +optional
	Title *string `json:"title,omitempty"`

	// Message is the message content.
	// +optional
	Message *string `json:"message,omitempty"`

	// SectionTitle is the message section title.
	// +optional
	SectionTitle *string `json:"sectionTitle,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// SlackConfig configures a Slack contact point.
type SlackConfig struct {
	// URLSecretRef is the Slack webhook URL. Either this or TokenSecretRef must be set.
	// +optional
	URLSecretRef *xpv1.SecretKeySelector `json:"urlSecretRef,omitempty"`

	// TokenSecretRef is the Slack API token. Either this or URLSecretRef must be set.
	// +optional
	TokenSecretRef *xpv1.SecretKeySelector `json:"tokenSecretRef,omitempty"`

	// Recipient is the Slack channel or user.
	// +optional
	Recipient *string `json:"recipient,omitempty"`

	// Title is the message title.
	// +optional
	Title *string `json:"title,omitempty"`

	// Text is the message text.
	// +optional
	Text *string `json:"text,omitempty"`

	// Username is the bot username.
	// +optional
	Username *string `json:"username,omitempty"`

	// IconURL is the bot avatar URL.
	// +optional
	IconURL *string `json:"iconUrl,omitempty"`

	// IconEmoji is the bot emoji avatar.
	// +optional
	IconEmoji *string `json:"iconEmoji,omitempty"`

	// MentionChannel is the channel mention format (here, channel).
	// +optional
	MentionChannel *string `json:"mentionChannel,omitempty"`

	// MentionUsers is a comma-separated list of user IDs to mention.
	// +optional
	MentionUsers *string `json:"mentionUsers,omitempty"`

	// MentionGroups is a comma-separated list of group IDs to mention.
	// +optional
	MentionGroups *string `json:"mentionGroups,omitempty"`

	// EndpointURL is a custom Slack endpoint URL.
	// +optional
	EndpointURL *string `json:"endpointUrl,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// SNSConfig configures an AWS SNS contact point.
type SNSConfig struct {
	// AccessKeySecretRef is the AWS access key.
	// +optional
	AccessKeySecretRef *xpv1.SecretKeySelector `json:"accessKeySecretRef,omitempty"`

	// SecretKeySecretRef is the AWS secret key.
	// +optional
	SecretKeySecretRef *xpv1.SecretKeySelector `json:"secretKeySecretRef,omitempty"`

	// AuthProvider is the authentication method ("arn" or "keys").
	// +optional
	AuthProvider *string `json:"authProvider,omitempty"`

	// AssumeRoleARN is the IAM role ARN to assume.
	// +optional
	AssumeRoleARN *string `json:"assumeRoleArn,omitempty"`

	// ExternalID is the external ID for role assumption.
	// +optional
	ExternalID *string `json:"externalId,omitempty"`

	// Topic is the SNS topic ARN.
	// +kubebuilder:validation:Required
	Topic string `json:"topic"`

	// Subject is the message subject.
	// +optional
	Subject *string `json:"subject,omitempty"`

	// Body is the message body template.
	// +optional
	Body *string `json:"body,omitempty"`

	// MessageFormat is the alert message format.
	// +optional
	MessageFormat *string `json:"messageFormat,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// TelegramConfig configures a Telegram contact point.
type TelegramConfig struct {
	// TokenSecretRef is the Telegram bot token.
	// +kubebuilder:validation:Required
	TokenSecretRef xpv1.SecretKeySelector `json:"tokenSecretRef"`

	// ChatID is the Telegram chat ID.
	// +kubebuilder:validation:Required
	ChatID string `json:"chatId"`

	// ParseMode is the message parse mode (HTML, Markdown, MarkdownV2).
	// +optional
	ParseMode *string `json:"parseMode,omitempty"`

	// Message is the message template.
	// +optional
	Message *string `json:"message,omitempty"`

	// DisableNotifications silences notifications.
	// +optional
	DisableNotifications *bool `json:"disableNotifications,omitempty"`

	// DisableWebPagePreview disables link previews.
	// +optional
	DisableWebPagePreview *bool `json:"disableWebPagePreview,omitempty"`

	// ProtectContent prevents message forwarding.
	// +optional
	ProtectContent *bool `json:"protectContent,omitempty"`

	// MessageThreadID is the thread ID for grouped messages.
	// +optional
	MessageThreadID *string `json:"messageThreadId,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// WebhookConfig configures a Webhook contact point.
type WebhookConfig struct {
	// URLSecretRef is the webhook URL.
	// +kubebuilder:validation:Required
	URLSecretRef xpv1.SecretKeySelector `json:"urlSecretRef"`

	// HTTPMethod is the HTTP method to use.
	// +optional
	// +kubebuilder:validation:Enum=POST;PUT
	HTTPMethod *string `json:"httpMethod,omitempty"`

	// MaxAlerts is the maximum number of alerts to send.
	// +optional
	MaxAlerts *int `json:"maxAlerts,omitempty"`

	// BasicAuthUser is the username for basic authentication.
	// +optional
	BasicAuthUser *string `json:"basicAuthUser,omitempty"`

	// BasicAuthPasswordSecretRef is the password for basic authentication.
	// +optional
	BasicAuthPasswordSecretRef *xpv1.SecretKeySelector `json:"basicAuthPasswordSecretRef,omitempty"`

	// AuthorizationScheme is the authorization header scheme.
	// +optional
	AuthorizationScheme *string `json:"authorizationScheme,omitempty"`

	// AuthorizationCredentialsSecretRef is the authorization credentials.
	// +optional
	AuthorizationCredentialsSecretRef *xpv1.SecretKeySelector `json:"authorizationCredentialsSecretRef,omitempty"`

	// Title is the message title.
	// +optional
	Title *string `json:"title,omitempty"`

	// Message is the message body.
	// +optional
	Message *string `json:"message,omitempty"`

	// DisableResolveMessage disables sending resolve messages.
	// +optional
	DisableResolveMessage *bool `json:"disableResolveMessage,omitempty"`
}

// ContactPointObservation are the observable fields of a ContactPoint.
type ContactPointObservation struct {
	// UID is the unique identifier of the contact point.
	// +optional
	UID *string `json:"uid,omitempty"`

	// OrgID is the Organization ID.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`
}

// ContactPointSpec defines the desired state of a ContactPoint.
type ContactPointSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ContactPointParameters `json:"forProvider"`
}

// ContactPointStatus represents the observed state of a ContactPoint.
type ContactPointStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ContactPointObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// ContactPoint is the Schema for the ContactPoint API.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type ContactPoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ContactPointSpec   `json:"spec"`
	Status            ContactPointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ContactPointList contains a list of ContactPoint.
type ContactPointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContactPoint `json:"items"`
}

// ContactPoint type metadata.
var (
	ContactPointKind             = reflect.TypeOf(ContactPoint{}).Name()
	ContactPointGroupKind        = schema.GroupKind{Group: Group, Kind: ContactPointKind}.String()
	ContactPointKindAPIVersion   = ContactPointKind + "." + SchemeGroupVersion.String()
	ContactPointGroupVersionKind = SchemeGroupVersion.WithKind(ContactPointKind)
)

func init() {
	SchemeBuilder.Register(&ContactPoint{}, &ContactPointList{})
}
