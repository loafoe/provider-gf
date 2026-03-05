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

package contactpoint

import (
	"context"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/crossplane/provider-gf/apis/alerting/v1alpha1"
)

// getContactPointType returns the contact point type from the spec.
func (e *external) getContactPointType(cr *v1alpha1.ContactPoint) string { //nolint:gocyclo // switch over contact point types
	fp := cr.Spec.ForProvider
	switch {
	case fp.Alertmanager != nil:
		return "alertmanager"
	case fp.Discord != nil:
		return "discord"
	case fp.Email != nil:
		return "email"
	case fp.Jira != nil:
		return "jira"
	case fp.Kafka != nil:
		return "kafka"
	case fp.PagerDuty != nil:
		return "pagerduty"
	case fp.Teams != nil:
		return "teams"
	case fp.Slack != nil:
		return "slack"
	case fp.SNS != nil:
		return "sns"
	case fp.Telegram != nil:
		return "telegram"
	case fp.Webhook != nil:
		return "webhook"
	default:
		return ""
	}
}

// getDisableResolveMessage returns the disableResolveMessage setting.
func (e *external) getDisableResolveMessage(cr *v1alpha1.ContactPoint) bool { //nolint:gocyclo // switch over contact point types
	fp := cr.Spec.ForProvider
	switch {
	case fp.Alertmanager != nil && fp.Alertmanager.DisableResolveMessage != nil:
		return *fp.Alertmanager.DisableResolveMessage
	case fp.Discord != nil && fp.Discord.DisableResolveMessage != nil:
		return *fp.Discord.DisableResolveMessage
	case fp.Email != nil && fp.Email.DisableResolveMessage != nil:
		return *fp.Email.DisableResolveMessage
	case fp.Jira != nil && fp.Jira.DisableResolveMessage != nil:
		return *fp.Jira.DisableResolveMessage
	case fp.Kafka != nil && fp.Kafka.DisableResolveMessage != nil:
		return *fp.Kafka.DisableResolveMessage
	case fp.PagerDuty != nil && fp.PagerDuty.DisableResolveMessage != nil:
		return *fp.PagerDuty.DisableResolveMessage
	case fp.Teams != nil && fp.Teams.DisableResolveMessage != nil:
		return *fp.Teams.DisableResolveMessage
	case fp.Slack != nil && fp.Slack.DisableResolveMessage != nil:
		return *fp.Slack.DisableResolveMessage
	case fp.SNS != nil && fp.SNS.DisableResolveMessage != nil:
		return *fp.SNS.DisableResolveMessage
	case fp.Telegram != nil && fp.Telegram.DisableResolveMessage != nil:
		return *fp.Telegram.DisableResolveMessage
	case fp.Webhook != nil && fp.Webhook.DisableResolveMessage != nil:
		return *fp.Webhook.DisableResolveMessage
	default:
		return false
	}
}

// buildSettings builds the settings map for the contact point.
func (e *external) buildSettings(ctx context.Context, cr *v1alpha1.ContactPoint) (string, map[string]any, error) { //nolint:gocyclo
	fp := cr.Spec.ForProvider
	ns := cr.GetNamespace()

	switch {
	case fp.Alertmanager != nil:
		return e.buildAlertmanagerSettings(ctx, fp.Alertmanager, ns)
	case fp.Discord != nil:
		return e.buildDiscordSettings(ctx, fp.Discord, ns)
	case fp.Email != nil:
		return e.buildEmailSettings(fp.Email)
	case fp.Jira != nil:
		return e.buildJiraSettings(ctx, fp.Jira, ns)
	case fp.Kafka != nil:
		return e.buildKafkaSettings(ctx, fp.Kafka, ns)
	case fp.PagerDuty != nil:
		return e.buildPagerDutySettings(ctx, fp.PagerDuty, ns)
	case fp.Teams != nil:
		return e.buildTeamsSettings(ctx, fp.Teams, ns)
	case fp.Slack != nil:
		return e.buildSlackSettings(ctx, fp.Slack, ns)
	case fp.SNS != nil:
		return e.buildSNSSettings(ctx, fp.SNS, ns)
	case fp.Telegram != nil:
		return e.buildTelegramSettings(ctx, fp.Telegram, ns)
	case fp.Webhook != nil:
		return e.buildWebhookSettings(ctx, fp.Webhook, ns)
	default:
		return "", nil, errors.New("no contact point type configured")
	}
}

func (e *external) buildAlertmanagerSettings(ctx context.Context, cfg *v1alpha1.AlertmanagerConfig, ns string) (string, map[string]any, error) {
	settings := map[string]any{"url": cfg.URL}
	if cfg.BasicAuthUser != nil {
		settings["basicAuthUser"] = *cfg.BasicAuthUser
	}
	if cfg.BasicAuthPasswordSecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.BasicAuthPasswordSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get basicAuthPassword")
		}
		settings["basicAuthPassword"] = val
	}
	return "alertmanager", settings, nil
}

func (e *external) buildDiscordSettings(ctx context.Context, cfg *v1alpha1.DiscordConfig, ns string) (string, map[string]any, error) {
	url, err := e.getSecretValue(ctx, ns, cfg.URLSecretRef)
	if err != nil {
		return "", nil, errors.Wrap(err, "cannot get discord URL")
	}
	settings := map[string]any{"url": url}
	if cfg.Title != nil {
		settings["title"] = *cfg.Title
	}
	if cfg.Message != nil {
		settings["message"] = *cfg.Message
	}
	if cfg.AvatarURL != nil {
		settings["avatar_url"] = *cfg.AvatarURL
	}
	if cfg.UseDiscordUsername != nil {
		settings["use_discord_username"] = *cfg.UseDiscordUsername
	}
	return "discord", settings, nil
}

func (e *external) buildEmailSettings(cfg *v1alpha1.EmailConfig) (string, map[string]any, error) {
	settings := map[string]any{"addresses": strings.Join(cfg.Addresses, ";")}
	if cfg.Subject != nil {
		settings["subject"] = *cfg.Subject
	}
	if cfg.Message != nil {
		settings["message"] = *cfg.Message
	}
	if cfg.SingleEmail != nil {
		settings["singleEmail"] = *cfg.SingleEmail
	}
	return "email", settings, nil
}

func (e *external) buildJiraSettings(ctx context.Context, cfg *v1alpha1.JiraConfig, ns string) (string, map[string]any, error) { //nolint:gocyclo // many optional fields
	settings := map[string]any{
		"apiUrl":    cfg.APIURL,
		"project":   cfg.Project,
		"issueType": cfg.IssueType,
	}
	if cfg.APITokenSecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.APITokenSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get jira apiToken")
		}
		settings["apiToken"] = val
	}
	if cfg.UserSecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.UserSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get jira user")
		}
		settings["user"] = val
	}
	if cfg.PasswordSecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.PasswordSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get jira password")
		}
		settings["password"] = val
	}
	if cfg.Summary != nil {
		settings["summary"] = *cfg.Summary
	}
	if cfg.Description != nil {
		settings["description"] = *cfg.Description
	}
	if cfg.Priority != nil {
		settings["priority"] = *cfg.Priority
	}
	if len(cfg.Labels) > 0 {
		settings["labels"] = strings.Join(cfg.Labels, ",")
	}
	return "jira", settings, nil
}

func (e *external) buildKafkaSettings(ctx context.Context, cfg *v1alpha1.KafkaConfig, ns string) (string, map[string]any, error) {
	restProxyURL, err := e.getSecretValue(ctx, ns, cfg.RESTProxyURLSecretRef)
	if err != nil {
		return "", nil, errors.Wrap(err, "cannot get kafka restProxyUrl")
	}
	settings := map[string]any{
		"kafkaRestProxy": restProxyURL,
		"kafkaTopic":     cfg.Topic,
	}
	if cfg.ClusterID != nil {
		settings["kafkaClusterId"] = *cfg.ClusterID
	}
	if cfg.APIVersion != nil {
		settings["apiVersion"] = *cfg.APIVersion
	}
	if cfg.Username != nil {
		settings["username"] = *cfg.Username
	}
	if cfg.PasswordSecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.PasswordSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get kafka password")
		}
		settings["password"] = val
	}
	if cfg.Description != nil {
		settings["description"] = *cfg.Description
	}
	if cfg.Details != nil {
		settings["details"] = *cfg.Details
	}
	return "kafka", settings, nil
}

func (e *external) buildPagerDutySettings(ctx context.Context, cfg *v1alpha1.PagerDutyConfig, ns string) (string, map[string]any, error) { //nolint:gocyclo // many optional fields
	integrationKey, err := e.getSecretValue(ctx, ns, cfg.IntegrationKeySecretRef)
	if err != nil {
		return "", nil, errors.Wrap(err, "cannot get pagerduty integrationKey")
	}
	settings := map[string]any{"integrationKey": integrationKey}
	if cfg.Severity != nil {
		settings["severity"] = *cfg.Severity
	}
	if cfg.Component != nil {
		settings["component"] = *cfg.Component
	}
	if cfg.Class != nil {
		settings["class"] = *cfg.Class
	}
	if cfg.Group != nil {
		settings["group"] = *cfg.Group
	}
	if cfg.Source != nil {
		settings["source"] = *cfg.Source
	}
	if cfg.Client != nil {
		settings["client"] = *cfg.Client
	}
	if cfg.ClientURL != nil {
		settings["client_url"] = *cfg.ClientURL
	}
	if cfg.Summary != nil {
		settings["summary"] = *cfg.Summary
	}
	if cfg.URL != nil {
		settings["url"] = *cfg.URL
	}
	if len(cfg.Details) > 0 {
		settings["details"] = cfg.Details
	}
	return "pagerduty", settings, nil
}

func (e *external) buildTeamsSettings(ctx context.Context, cfg *v1alpha1.TeamsConfig, ns string) (string, map[string]any, error) {
	url, err := e.getSecretValue(ctx, ns, cfg.URLSecretRef)
	if err != nil {
		return "", nil, errors.Wrap(err, "cannot get teams URL")
	}
	settings := map[string]any{"url": url}
	if cfg.Title != nil {
		settings["title"] = *cfg.Title
	}
	if cfg.Message != nil {
		settings["message"] = *cfg.Message
	}
	if cfg.SectionTitle != nil {
		settings["sectiontitle"] = *cfg.SectionTitle
	}
	return "teams", settings, nil
}

func (e *external) buildSlackSettings(ctx context.Context, cfg *v1alpha1.SlackConfig, ns string) (string, map[string]any, error) { //nolint:gocyclo // many optional fields
	settings := make(map[string]any)
	if cfg.URLSecretRef != nil {
		url, err := e.getSecretValue(ctx, ns, *cfg.URLSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get slack URL")
		}
		settings["url"] = url
	}
	if cfg.TokenSecretRef != nil {
		token, err := e.getSecretValue(ctx, ns, *cfg.TokenSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get slack token")
		}
		settings["token"] = token
	}
	if cfg.Recipient != nil {
		settings["recipient"] = *cfg.Recipient
	}
	if cfg.Title != nil {
		settings["title"] = *cfg.Title
	}
	if cfg.Text != nil {
		settings["text"] = *cfg.Text
	}
	if cfg.Username != nil {
		settings["username"] = *cfg.Username
	}
	if cfg.IconURL != nil {
		settings["icon_url"] = *cfg.IconURL
	}
	if cfg.IconEmoji != nil {
		settings["icon_emoji"] = *cfg.IconEmoji
	}
	if cfg.MentionChannel != nil {
		settings["mentionChannel"] = *cfg.MentionChannel
	}
	if cfg.MentionUsers != nil {
		settings["mentionUsers"] = *cfg.MentionUsers
	}
	if cfg.MentionGroups != nil {
		settings["mentionGroups"] = *cfg.MentionGroups
	}
	if cfg.EndpointURL != nil {
		settings["endpointUrl"] = *cfg.EndpointURL
	}
	return "slack", settings, nil
}

func (e *external) buildSNSSettings(ctx context.Context, cfg *v1alpha1.SNSConfig, ns string) (string, map[string]any, error) { //nolint:gocyclo // many optional fields
	settings := map[string]any{"topic": cfg.Topic}
	if cfg.AccessKeySecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.AccessKeySecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get sns accessKey")
		}
		settings["accessKey"] = val
	}
	if cfg.SecretKeySecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.SecretKeySecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get sns secretKey")
		}
		settings["secretKey"] = val
	}
	if cfg.AuthProvider != nil {
		settings["authProvider"] = *cfg.AuthProvider
	}
	if cfg.AssumeRoleARN != nil {
		settings["assumeRoleArn"] = *cfg.AssumeRoleARN
	}
	if cfg.ExternalID != nil {
		settings["externalId"] = *cfg.ExternalID
	}
	if cfg.Subject != nil {
		settings["subject"] = *cfg.Subject
	}
	if cfg.Body != nil {
		settings["body"] = *cfg.Body
	}
	if cfg.MessageFormat != nil {
		settings["messageFormat"] = *cfg.MessageFormat
	}
	return "sns", settings, nil
}

func (e *external) buildTelegramSettings(ctx context.Context, cfg *v1alpha1.TelegramConfig, ns string) (string, map[string]any, error) {
	token, err := e.getSecretValue(ctx, ns, cfg.TokenSecretRef)
	if err != nil {
		return "", nil, errors.Wrap(err, "cannot get telegram token")
	}
	settings := map[string]any{
		"bottoken": token,
		"chatid":   cfg.ChatID,
	}
	if cfg.ParseMode != nil {
		settings["parse_mode"] = *cfg.ParseMode
	}
	if cfg.Message != nil {
		settings["message"] = *cfg.Message
	}
	if cfg.DisableNotifications != nil {
		settings["disable_notification"] = *cfg.DisableNotifications
	}
	if cfg.DisableWebPagePreview != nil {
		settings["disable_web_page_preview"] = *cfg.DisableWebPagePreview
	}
	if cfg.ProtectContent != nil {
		settings["protect_content"] = *cfg.ProtectContent
	}
	if cfg.MessageThreadID != nil {
		settings["message_thread_id"] = *cfg.MessageThreadID
	}
	return "telegram", settings, nil
}

func (e *external) buildWebhookSettings(ctx context.Context, cfg *v1alpha1.WebhookConfig, ns string) (string, map[string]any, error) { //nolint:gocyclo // many optional fields
	url, err := e.getSecretValue(ctx, ns, cfg.URLSecretRef)
	if err != nil {
		return "", nil, errors.Wrap(err, "cannot get webhook URL")
	}
	settings := map[string]any{"url": url}
	if cfg.HTTPMethod != nil {
		settings["httpMethod"] = *cfg.HTTPMethod
	}
	if cfg.MaxAlerts != nil {
		settings["maxAlerts"] = *cfg.MaxAlerts
	}
	if cfg.BasicAuthUser != nil {
		settings["username"] = *cfg.BasicAuthUser
	}
	if cfg.BasicAuthPasswordSecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.BasicAuthPasswordSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get webhook basicAuthPassword")
		}
		settings["password"] = val
	}
	if cfg.AuthorizationScheme != nil {
		settings["authorization_scheme"] = *cfg.AuthorizationScheme
	}
	if cfg.AuthorizationCredentialsSecretRef != nil {
		val, err := e.getSecretValue(ctx, ns, *cfg.AuthorizationCredentialsSecretRef)
		if err != nil {
			return "", nil, errors.Wrap(err, "cannot get webhook authorizationCredentials")
		}
		settings["authorization_credentials"] = val
	}
	if cfg.Title != nil {
		settings["title"] = *cfg.Title
	}
	if cfg.Message != nil {
		settings["message"] = *cfg.Message
	}
	return "webhook", settings, nil
}

func (e *external) getSecretValue(ctx context.Context, namespace string, ref xpv1.SecretKeySelector) (string, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = namespace
	}
	secret := &corev1.Secret{}
	if err := e.kube.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, secret); err != nil {
		return "", err
	}
	data, ok := secret.Data[ref.Key]
	if !ok {
		return "", errors.Errorf("key %s not found in secret %s/%s", ref.Key, ns, ref.Name)
	}
	return string(data), nil
}
