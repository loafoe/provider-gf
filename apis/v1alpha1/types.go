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
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A ProviderConfigStatus defines the status of a Provider.
type ProviderConfigStatus struct {
	xpv1.ProviderConfigStatus `json:",inline"`
}

// AuthType specifies the type of authentication to use.
// +kubebuilder:validation:Enum=basic;token
type AuthType string

const (
	// AuthTypeBasic uses HTTP Basic Authentication with username and password.
	AuthTypeBasic AuthType = "basic"
	// AuthTypeToken uses a service account token for authentication.
	AuthTypeToken AuthType = "token"
)

// BasicAuth contains credentials for HTTP Basic Authentication.
type BasicAuth struct {
	// UsernameSecretRef is a reference to a secret key containing the username.
	UsernameSecretRef xpv1.SecretKeySelector `json:"usernameSecretRef"`

	// PasswordSecretRef is a reference to a secret key containing the password.
	PasswordSecretRef xpv1.SecretKeySelector `json:"passwordSecretRef"`
}

// TokenAuth contains credentials for token-based authentication.
type TokenAuth struct {
	// TokenSecretRef is a reference to a secret key containing the service account token.
	TokenSecretRef xpv1.SecretKeySelector `json:"tokenSecretRef"`
}

// GrafanaCredentials contains authentication configuration for Grafana.
type GrafanaCredentials struct {
	// Source of the provider credentials.
	// +kubebuilder:validation:Enum=None;Secret
	// +kubebuilder:default=Secret
	Source xpv1.CredentialsSource `json:"source"`

	// AuthType specifies the type of authentication to use.
	// +kubebuilder:validation:Required
	AuthType AuthType `json:"authType"`

	// BasicAuth contains credentials for HTTP Basic Authentication.
	// Required when authType is "basic".
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`

	// TokenAuth contains credentials for token-based authentication.
	// Required when authType is "token".
	// +optional
	TokenAuth *TokenAuth `json:"tokenAuth,omitempty"`
}

// ProviderConfigSpec defines the configuration for connecting to Grafana.
type ProviderConfigSpec struct {
	// URL is the URL of the Grafana instance.
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// OrgID is the Organization ID. If not set, defaults to the default org.
	// +optional
	OrgID *int64 `json:"orgId,omitempty"`

	// Credentials contains authentication configuration for Grafana.
	// +kubebuilder:validation:Required
	Credentials GrafanaCredentials `json:"credentials"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.url"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,provider,gf}
// A ProviderConfig configures a Grafana provider.
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="CONFIG-NAME",type="string",JSONPath=".providerConfigRef.name"
// +kubebuilder:printcolumn:name="RESOURCE-KIND",type="string",JSONPath=".resourceRef.kind"
// +kubebuilder:printcolumn:name="RESOURCE-NAME",type="string",JSONPath=".resourceRef.name"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,provider,gf}
// A ProviderConfigUsage indicates that a resource is using a ProviderConfig.
type ProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	xpv2.TypedProviderConfigUsage `json:",inline"`
}

// +kubebuilder:object:root=true

// ProviderConfigUsageList contains a list of ProviderConfigUsage
type ProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfigUsage `json:"items"`
}
