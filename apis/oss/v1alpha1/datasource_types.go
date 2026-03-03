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

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DataSourceParameters are the configurable fields of a DataSource.
type DataSourceParameters struct {
	// Name is a unique name for the data source.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Type is the data source type (e.g., prometheus, loki, tempo, etc.).
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// URL is the URL for the data source.
	// +optional
	URL *string `json:"url,omitempty"`

	// AccessMode controls how requests to the data source will be handled.
	// 'proxy' means the data source request will be routed through the Grafana backend/server.
	// 'direct' means the data source request will be sent directly from the browser.
	// Defaults to 'proxy'.
	// +kubebuilder:validation:Enum=proxy;direct
	// +kubebuilder:default=proxy
	// +optional
	AccessMode *string `json:"accessMode,omitempty"`

	// UID is the unique identifier for the data source.
	// If not set, Grafana will generate one.
	// +optional
	UID *string `json:"uid,omitempty"`

	// IsDefault sets the data source as the default data source.
	// Defaults to false.
	// +optional
	IsDefault *bool `json:"isDefault,omitempty"`

	// BasicAuthEnabled enables basic authentication for the data source.
	// Defaults to false.
	// +optional
	BasicAuthEnabled *bool `json:"basicAuthEnabled,omitempty"`

	// BasicAuthUsername is the username for basic authentication.
	// +optional
	BasicAuthUsername *string `json:"basicAuthUsername,omitempty"`

	// DatabaseName is the name of the database to use on the data source server.
	// +optional
	DatabaseName *string `json:"databaseName,omitempty"`

	// Username is the username to use to authenticate to the data source.
	// +optional
	Username *string `json:"username,omitempty"`

	// JSONDataEncoded is a JSON-encoded string containing the data source configuration.
	// +optional
	JSONDataEncoded *string `json:"jsonDataEncoded,omitempty"`

	// SecureJSONDataEncodedSecretRef is a reference to a secret containing the
	// secure JSON data for the data source (e.g., passwords, API keys).
	// +optional
	SecureJSONDataEncodedSecretRef *xpv1.SecretKeySelector `json:"secureJsonDataEncodedSecretRef,omitempty"`

	// HTTPHeadersSecretRef is a reference to a secret containing custom HTTP headers.
	// The secret should contain key-value pairs where keys are header names.
	// +optional
	HTTPHeadersSecretRef *xpv1.SecretKeySelector `json:"httpHeadersSecretRef,omitempty"`
}

// DataSourceObservation are the observable fields of a DataSource.
type DataSourceObservation struct {
	// ID is the numeric ID of the data source in Grafana.
	ID *int64 `json:"id,omitempty"`

	// UID is the unique identifier of the data source.
	UID *string `json:"uid,omitempty"`

	// Name is the name of the data source.
	Name *string `json:"name,omitempty"`

	// Type is the type of the data source.
	Type *string `json:"type,omitempty"`

	// URL is the URL of the data source.
	URL *string `json:"url,omitempty"`

	// AccessMode is the access mode of the data source.
	AccessMode *string `json:"accessMode,omitempty"`

	// IsDefault indicates if this is the default data source.
	IsDefault *bool `json:"isDefault,omitempty"`

	// BasicAuthEnabled indicates if basic auth is enabled.
	BasicAuthEnabled *bool `json:"basicAuthEnabled,omitempty"`

	// ReadOnly indicates if the data source is read-only.
	ReadOnly *bool `json:"readOnly,omitempty"`
}

// DataSourceSpec defines the desired state of a DataSource.
type DataSourceSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              DataSourceParameters `json:"forProvider"`
}

// DataSourceStatus represents the observed state of a DataSource.
type DataSourceStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          DataSourceObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// DataSource is the Schema for the DataSource API.
// A DataSource represents a Grafana data source that can be used in dashboards.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gf}
type DataSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataSourceSpec   `json:"spec"`
	Status DataSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DataSourceList contains a list of DataSource.
type DataSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataSource `json:"items"`
}

// DataSource type metadata.
var (
	DataSourceKind             = reflect.TypeOf(DataSource{}).Name()
	DataSourceGroupKind        = schema.GroupKind{Group: Group, Kind: DataSourceKind}.String()
	DataSourceKindAPIVersion   = DataSourceKind + "." + SchemeGroupVersion.String()
	DataSourceGroupVersionKind = SchemeGroupVersion.WithKind(DataSourceKind)
)

func init() {
	SchemeBuilder.Register(&DataSource{}, &DataSourceList{})
}
