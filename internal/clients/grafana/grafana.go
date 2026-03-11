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

// Package grafana provides an HTTP client for interacting with the Grafana API.
package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIError represents an error response from the Grafana API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("grafana API error (status %d): %s", e.StatusCode, e.Message)
}

// IsNotFound returns true if the error is a 404 Not Found error.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// newAPIError creates an APIError from an HTTP response.
func newAPIError(statusCode int, body []byte) *APIError {
	// Try to parse Grafana's JSON error response
	var errResp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
		return &APIError{StatusCode: statusCode, Message: errResp.Message}
	}
	// Fall back to raw body if not JSON
	return &APIError{StatusCode: statusCode, Message: string(body)}
}

// Client is a Grafana API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
	authHeader string
	orgID      *int64
}

// Config contains configuration for creating a Grafana client.
type Config struct {
	// URL is the base URL of the Grafana instance.
	URL string
	// Username for basic auth (optional if using token).
	Username string
	// Password for basic auth (optional if using token).
	Password string
	// Token for service account authentication (optional if using basic auth).
	Token string
	// OrgID is the organization ID (optional).
	OrgID *int64
}

// NewClient creates a new Grafana API client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("grafana URL is required")
	}

	// Normalize URL - remove trailing slash
	baseURL := strings.TrimSuffix(cfg.URL, "/")

	var authHeader string
	switch {
	case cfg.Token != "":
		authHeader = "Bearer " + cfg.Token
	case cfg.Username != "" && cfg.Password != "":
		// Store username/password for basic auth
		authHeader = "basic:" + cfg.Username + ":" + cfg.Password
	default:
		return nil, fmt.Errorf("either token or username/password must be provided")
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		authHeader: authHeader,
		orgID:      cfg.OrgID,
	}, nil
}

// doRequest performs an HTTP request with authentication.
func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Set authentication header
	if strings.HasPrefix(c.authHeader, "basic:") {
		parts := strings.SplitN(c.authHeader, ":", 3)
		if len(parts) == 3 {
			req.SetBasicAuth(parts[1], parts[2])
		}
	} else if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}

	// Set org ID header if specified
	if c.orgID != nil {
		req.Header.Set("X-Grafana-Org-Id", fmt.Sprintf("%d", *c.orgID))
	}

	return c.httpClient.Do(req)
}

// DashboardCreateRequest represents a request to create/update a dashboard.
type DashboardCreateRequest struct {
	Dashboard json.RawMessage `json:"dashboard"`
	FolderUID string          `json:"folderUid,omitempty"`
	Message   string          `json:"message,omitempty"`
	Overwrite bool            `json:"overwrite"`
}

// DashboardResponse represents the response from Grafana dashboard API.
type DashboardResponse struct {
	ID      int64  `json:"id"`
	UID     string `json:"uid"`
	URL     string `json:"url"`
	Status  string `json:"status"`
	Version int64  `json:"version"`
	Slug    string `json:"slug"`
}

// DashboardGetResponse represents the response from getting a dashboard.
type DashboardGetResponse struct {
	Dashboard json.RawMessage `json:"dashboard"`
	Meta      DashboardMeta   `json:"meta"`
}

// DashboardMeta contains metadata about a dashboard.
type DashboardMeta struct {
	Slug        string `json:"slug"`
	URL         string `json:"url"`
	FolderUID   string `json:"folderUid"`
	FolderTitle string `json:"folderTitle"`
	Version     int64  `json:"version"`
	IsFolder    bool   `json:"isFolder"`
}

// CreateOrUpdateDashboard creates or updates a dashboard.
func (c *Client) CreateOrUpdateDashboard(ctx context.Context, req DashboardCreateRequest) (*DashboardResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/dashboards/db", req)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update dashboard: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create/update dashboard: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var dashResp DashboardResponse
	if err := json.Unmarshal(body, &dashResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &dashResp, nil
}

// GetDashboardByUID gets a dashboard by its UID.
func (c *Client) GetDashboardByUID(ctx context.Context, uid string) (*DashboardGetResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/dashboards/uid/"+uid, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Dashboard doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get dashboard: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var dashResp DashboardGetResponse
	if err := json.Unmarshal(body, &dashResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &dashResp, nil
}

// DeleteDashboardByUID deletes a dashboard by its UID.
func (c *Client) DeleteDashboardByUID(ctx context.Context, uid string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/dashboards/uid/"+uid, nil)
	if err != nil {
		return fmt.Errorf("failed to delete dashboard: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete dashboard: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// ============================================================================
// Dashboard V2 API (Kubernetes-style API)
// Endpoint: /apis/dashboard.grafana.app/v1beta1/namespaces/:namespace/dashboards
// ============================================================================

// DashboardV2Metadata represents metadata for a K8s-style dashboard resource.
type DashboardV2Metadata struct {
	Name              string            `json:"name,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	UID               string            `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

// DashboardV2 represents a K8s-style dashboard resource.
type DashboardV2 struct {
	APIVersion string              `json:"apiVersion"`
	Kind       string              `json:"kind"`
	Metadata   DashboardV2Metadata `json:"metadata"`
	Spec       map[string]any      `json:"spec"`
	Status     map[string]any      `json:"status,omitempty"`
}

// DashboardV2Response wraps the response from the V2 API which includes the full resource.
type DashboardV2Response struct {
	DashboardV2
}

// Annotation keys used by Grafana Dashboard V2 API.
const (
	DashboardV2AnnotationFolder  = "grafana.app/folder"
	DashboardV2AnnotationMessage = "grafana.app/message"

	// DefaultNamespace is the namespace used for org ID 1 in OSS/On-Premise Grafana.
	DefaultNamespace = "default"
)

// OrgIDToNamespace converts a Grafana organization ID to the API namespace.
// For OSS/On-Premise Grafana:
//   - Org ID 1 → "default"
//   - Org ID > 1 → "org-<id>" (e.g., org 42 → "org-42")
func OrgIDToNamespace(orgID int64) string {
	if orgID <= 1 {
		return DefaultNamespace
	}
	return fmt.Sprintf("org-%d", orgID)
}

// IsDashboardV2Format checks if the given JSON is in the K8s-style Dashboard V2 format.
// It returns true if the JSON has an apiVersion starting with "dashboard.grafana.app/".
func IsDashboardV2Format(configJSON []byte) bool {
	var partial struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(configJSON, &partial); err != nil {
		return false
	}
	return strings.HasPrefix(partial.APIVersion, "dashboard.grafana.app/") && partial.Kind == "Dashboard"
}

// GetDashboardV2APIVersion extracts the API version from the dashboard's apiVersion field.
// Returns the version part (e.g., "v1beta1" from "dashboard.grafana.app/v1beta1").
// Defaults to "v1beta1" if not found or invalid.
func GetDashboardV2APIVersion(dash *DashboardV2) string {
	if dash.APIVersion == "" {
		return "v1beta1"
	}
	parts := strings.SplitN(dash.APIVersion, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "v1beta1"
	}
	return parts[1]
}

// ParseDashboardV2 parses a K8s-style dashboard JSON into a DashboardV2 struct.
func ParseDashboardV2(configJSON []byte) (*DashboardV2, error) {
	var dash DashboardV2
	if err := json.Unmarshal(configJSON, &dash); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard v2: %w", err)
	}
	return &dash, nil
}

// GetDashboardV2FolderUID extracts the folder UID from a V2 dashboard's annotations.
func GetDashboardV2FolderUID(dash *DashboardV2) string {
	if dash.Metadata.Annotations == nil {
		return ""
	}
	return dash.Metadata.Annotations[DashboardV2AnnotationFolder]
}

// GetDashboardV2UID extracts the UID from a V2 dashboard.
// It first checks metadata.uid, then falls back to metadata.name.
func GetDashboardV2UID(dash *DashboardV2) string {
	if dash.Metadata.UID != "" {
		return dash.Metadata.UID
	}
	return dash.Metadata.Name
}

// CreateDashboardV2 creates a dashboard using the K8s-style V2 API.
func (c *Client) CreateDashboardV2(ctx context.Context, dash *DashboardV2) (*DashboardV2Response, error) {
	namespace := dash.Metadata.Namespace
	if namespace == "" {
		namespace = DefaultNamespace
	}

	apiVersion := GetDashboardV2APIVersion(dash)
	path := fmt.Sprintf("/apis/dashboard.grafana.app/%s/namespaces/%s/dashboards", apiVersion, namespace)

	resp, err := c.doRequest(ctx, http.MethodPost, path, dash)
	if err != nil {
		return nil, fmt.Errorf("failed to create dashboard v2: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create dashboard v2: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var dashResp DashboardV2Response
	if err := json.Unmarshal(body, &dashResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &dashResp, nil
}

// UpdateDashboardV2 updates a dashboard using the K8s-style V2 API.
func (c *Client) UpdateDashboardV2(ctx context.Context, dash *DashboardV2) (*DashboardV2Response, error) {
	namespace := dash.Metadata.Namespace
	if namespace == "" {
		namespace = DefaultNamespace
	}

	name := dash.Metadata.Name
	if name == "" {
		return nil, fmt.Errorf("dashboard metadata.name is required for update")
	}

	apiVersion := GetDashboardV2APIVersion(dash)
	path := fmt.Sprintf("/apis/dashboard.grafana.app/%s/namespaces/%s/dashboards/%s", apiVersion, namespace, name)

	resp, err := c.doRequest(ctx, http.MethodPut, path, dash)
	if err != nil {
		return nil, fmt.Errorf("failed to update dashboard v2: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update dashboard v2: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var dashResp DashboardV2Response
	if err := json.Unmarshal(body, &dashResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &dashResp, nil
}

// GetDashboardV2ByName gets a dashboard by name using the K8s-style V2 API.
// apiVersion should be the version string (e.g., "v1beta1", "v2beta1").
func (c *Client) GetDashboardV2ByName(ctx context.Context, apiVersion, namespace, name string) (*DashboardV2Response, error) {
	if namespace == "" {
		namespace = DefaultNamespace
	}
	if apiVersion == "" {
		apiVersion = "v1beta1"
	}

	path := fmt.Sprintf("/apis/dashboard.grafana.app/%s/namespaces/%s/dashboards/%s", apiVersion, namespace, name)

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard v2: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Dashboard doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get dashboard v2: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var dashResp DashboardV2Response
	if err := json.Unmarshal(body, &dashResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &dashResp, nil
}

// DeleteDashboardV2ByName deletes a dashboard by name using the K8s-style V2 API.
// apiVersion should be the version string (e.g., "v1beta1", "v2beta1").
func (c *Client) DeleteDashboardV2ByName(ctx context.Context, apiVersion, namespace, name string) error {
	if namespace == "" {
		namespace = DefaultNamespace
	}
	if apiVersion == "" {
		apiVersion = "v1beta1"
	}

	path := fmt.Sprintf("/apis/dashboard.grafana.app/%s/namespaces/%s/dashboards/%s", apiVersion, namespace, name)

	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete dashboard v2: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete dashboard v2: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// DataSource represents a Grafana data source.
type DataSource struct {
	ID              int64          `json:"id,omitempty"`
	UID             string         `json:"uid,omitempty"`
	OrgID           int64          `json:"orgId,omitempty"`
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	Access          string         `json:"access,omitempty"`
	URL             string         `json:"url,omitempty"`
	User            string         `json:"user,omitempty"`
	Database        string         `json:"database,omitempty"`
	BasicAuth       bool           `json:"basicAuth,omitempty"`
	BasicAuthUser   string         `json:"basicAuthUser,omitempty"`
	IsDefault       bool           `json:"isDefault,omitempty"`
	JSONData        map[string]any `json:"jsonData,omitempty"`
	SecureJSONData  map[string]any `json:"secureJsonData,omitempty"`
	ReadOnly        bool           `json:"readOnly,omitempty"`
	WithCredentials bool           `json:"withCredentials,omitempty"`
}

// DataSourceCreateRequest represents a request to create/update a data source.
type DataSourceCreateRequest struct {
	UID            string         `json:"uid,omitempty"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Access         string         `json:"access,omitempty"`
	URL            string         `json:"url,omitempty"`
	User           string         `json:"user,omitempty"`
	Database       string         `json:"database,omitempty"`
	BasicAuth      bool           `json:"basicAuth,omitempty"`
	BasicAuthUser  string         `json:"basicAuthUser,omitempty"`
	IsDefault      bool           `json:"isDefault,omitempty"`
	JSONData       map[string]any `json:"jsonData,omitempty"`
	SecureJSONData map[string]any `json:"secureJsonData,omitempty"`
}

// DataSourceResponse represents the response from creating a data source.
type DataSourceResponse struct {
	ID         int64       `json:"id"`
	UID        string      `json:"uid"`
	Message    string      `json:"message"`
	Name       string      `json:"name"`
	DataSource *DataSource `json:"datasource,omitempty"`
}

// CreateDataSource creates a new data source.
func (c *Client) CreateDataSource(ctx context.Context, req DataSourceCreateRequest) (*DataSourceResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/datasources", req)
	if err != nil {
		return nil, fmt.Errorf("failed to create data source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create data source: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var dsResp DataSourceResponse
	if err := json.Unmarshal(body, &dsResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// The UID is in the nested datasource object, not at the top level
	if dsResp.DataSource != nil {
		dsResp.UID = dsResp.DataSource.UID
	}

	return &dsResp, nil
}

// UpdateDataSource updates an existing data source by UID.
func (c *Client) UpdateDataSource(ctx context.Context, uid string, req DataSourceCreateRequest) (*DataSourceResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, "/api/datasources/uid/"+uid, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update data source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update data source: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var dsResp DataSourceResponse
	if err := json.Unmarshal(body, &dsResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// The UID is in the nested datasource object, not at the top level
	if dsResp.DataSource != nil {
		dsResp.UID = dsResp.DataSource.UID
	}

	return &dsResp, nil
}

// GetDataSourceByUID gets a data source by its UID.
func (c *Client) GetDataSourceByUID(ctx context.Context, uid string) (*DataSource, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/datasources/uid/"+uid, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get data source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Data source doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get data source: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var ds DataSource
	if err := json.Unmarshal(body, &ds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &ds, nil
}

// DeleteDataSourceByUID deletes a data source by its UID.
func (c *Client) DeleteDataSourceByUID(ctx context.Context, uid string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/datasources/uid/"+uid, nil)
	if err != nil {
		return fmt.Errorf("failed to delete data source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete data source: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// LibraryPanel represents a Grafana library panel (library element).
type LibraryPanel struct {
	ID       int64             `json:"id,omitempty"`
	OrgID    int64             `json:"orgId,omitempty"`
	FolderID int64             `json:"folderId,omitempty"`
	UID      string            `json:"uid,omitempty"`
	Name     string            `json:"name"`
	Kind     int               `json:"kind"` // 1 = panel, 2 = variable
	Type     string            `json:"type,omitempty"`
	Model    json.RawMessage   `json:"model,omitempty"`
	Version  int64             `json:"version,omitempty"`
	Meta     *LibraryPanelMeta `json:"meta,omitempty"`
}

// LibraryPanelMeta contains metadata about a library panel.
type LibraryPanelMeta struct {
	FolderName          string `json:"folderName,omitempty"`
	FolderUID           string `json:"folderUid,omitempty"`
	ConnectedDashboards int64  `json:"connectedDashboards,omitempty"`
	Created             string `json:"created,omitempty"`
	Updated             string `json:"updated,omitempty"`
}

// LibraryPanelCreateRequest represents a request to create a library panel.
type LibraryPanelCreateRequest struct {
	UID       string          `json:"uid,omitempty"`
	FolderUID string          `json:"folderUid,omitempty"`
	Name      string          `json:"name"`
	Model     json.RawMessage `json:"model"`
	Kind      int             `json:"kind"` // 1 = panel
}

// LibraryPanelUpdateRequest represents a request to update a library panel.
type LibraryPanelUpdateRequest struct {
	UID       string          `json:"uid,omitempty"`
	FolderUID string          `json:"folderUid,omitempty"`
	Name      string          `json:"name"`
	Model     json.RawMessage `json:"model"`
	Kind      int             `json:"kind"`
	Version   int64           `json:"version"`
}

// LibraryPanelResponse represents the response from library panel API.
type LibraryPanelResponse struct {
	Result *LibraryPanel `json:"result,omitempty"`
}

// CreateLibraryPanel creates a new library panel.
func (c *Client) CreateLibraryPanel(ctx context.Context, req LibraryPanelCreateRequest) (*LibraryPanel, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/library-elements", req)
	if err != nil {
		return nil, fmt.Errorf("failed to create library panel: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create library panel: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var lpResp LibraryPanelResponse
	if err := json.Unmarshal(body, &lpResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return lpResp.Result, nil
}

// UpdateLibraryPanel updates an existing library panel by UID.
func (c *Client) UpdateLibraryPanel(ctx context.Context, uid string, req LibraryPanelUpdateRequest) (*LibraryPanel, error) {
	resp, err := c.doRequest(ctx, http.MethodPatch, "/api/library-elements/"+uid, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update library panel: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update library panel: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var lpResp LibraryPanelResponse
	if err := json.Unmarshal(body, &lpResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return lpResp.Result, nil
}

// GetLibraryPanelByUID gets a library panel by its UID.
func (c *Client) GetLibraryPanelByUID(ctx context.Context, uid string) (*LibraryPanel, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/library-elements/"+uid, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get library panel: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Library panel doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get library panel: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var lpResp LibraryPanelResponse
	if err := json.Unmarshal(body, &lpResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return lpResp.Result, nil
}

// DeleteLibraryPanelByUID deletes a library panel by its UID.
func (c *Client) DeleteLibraryPanelByUID(ctx context.Context, uid string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/library-elements/"+uid, nil)
	if err != nil {
		return fmt.Errorf("failed to delete library panel: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete library panel: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// ContactPoint represents a Grafana contact point.
type ContactPoint struct {
	UID                   string         `json:"uid,omitempty"`
	Name                  string         `json:"name"`
	Type                  string         `json:"type"`
	Settings              map[string]any `json:"settings"`
	DisableResolveMessage bool           `json:"disableResolveMessage,omitempty"`
	Provenance            string         `json:"provenance,omitempty"`
}

// ContactPointCreateRequest represents a request to create a contact point.
type ContactPointCreateRequest struct {
	UID                   string         `json:"uid,omitempty"`
	Name                  string         `json:"name"`
	Type                  string         `json:"type"`
	Settings              map[string]any `json:"settings"`
	DisableResolveMessage bool           `json:"disableResolveMessage,omitempty"`
}

// CreateContactPoint creates a new contact point.
func (c *Client) CreateContactPoint(ctx context.Context, req ContactPointCreateRequest, disableProvenance bool) (*ContactPoint, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/provisioning/contact-points", req)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if disableProvenance {
		httpReq.Header.Set("X-Disable-Provenance", "true")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create contact point: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create contact point: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var cp ContactPoint
	if err := json.Unmarshal(body, &cp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &cp, nil
}

// UpdateContactPoint updates an existing contact point by UID.
func (c *Client) UpdateContactPoint(ctx context.Context, uid string, req ContactPointCreateRequest, disableProvenance bool) (*ContactPoint, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/provisioning/contact-points/"+uid, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if disableProvenance {
		httpReq.Header.Set("X-Disable-Provenance", "true")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to update contact point: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update contact point: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var cp ContactPoint
	if err := json.Unmarshal(body, &cp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &cp, nil
}

// GetContactPointByUID gets a contact point by its UID.
func (c *Client) GetContactPointByUID(ctx context.Context, uid string) (*ContactPoint, error) {
	// Grafana API doesn't have a direct get by UID endpoint, so we list and filter
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/provisioning/contact-points", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get contact points: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get contact points: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var contactPoints []ContactPoint
	if err := json.Unmarshal(body, &contactPoints); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	for i := range contactPoints {
		if contactPoints[i].UID == uid {
			return &contactPoints[i], nil
		}
	}

	return nil, nil // Not found
}

// GetContactPointByName gets a contact point by its name.
func (c *Client) GetContactPointByName(ctx context.Context, name string) (*ContactPoint, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/provisioning/contact-points", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get contact points: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get contact points: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var contactPoints []ContactPoint
	if err := json.Unmarshal(body, &contactPoints); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	for i := range contactPoints {
		if contactPoints[i].Name == name {
			return &contactPoints[i], nil
		}
	}

	return nil, nil // Not found
}

// DeleteContactPointByUID deletes a contact point by its UID.
func (c *Client) DeleteContactPointByUID(ctx context.Context, uid string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/provisioning/contact-points/"+uid, nil)
	if err != nil {
		return fmt.Errorf("failed to delete contact point: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete contact point: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// newRequest creates a new HTTP request with authentication headers.
// Folder represents a Grafana folder.
type Folder struct {
	ID            int64  `json:"id,omitempty"`
	UID           string `json:"uid,omitempty"`
	Title         string `json:"title"`
	URL           string `json:"url,omitempty"`
	Version       int64  `json:"version,omitempty"`
	ParentUID     string `json:"parentUid,omitempty"`
	OrgID         int64  `json:"orgId,omitempty"`
	CanSave       bool   `json:"canSave,omitempty"`
	CanEdit       bool   `json:"canEdit,omitempty"`
	CanAdmin      bool   `json:"canAdmin,omitempty"`
	CanDelete     bool   `json:"canDelete,omitempty"`
	HasACL        bool   `json:"hasAcl,omitempty"`
	Created       string `json:"created,omitempty"`
	Updated       string `json:"updated,omitempty"`
	CreatedBy     string `json:"createdBy,omitempty"`
	UpdatedBy     string `json:"updatedBy,omitempty"`
	FolderUID     string `json:"folderUid,omitempty"` // Nested folder parent
	AccessControl any    `json:"accessControl,omitempty"`
}

// FolderCreateRequest represents a request to create a folder.
type FolderCreateRequest struct {
	UID       string `json:"uid,omitempty"`
	Title     string `json:"title"`
	ParentUID string `json:"parentUid,omitempty"`
}

// FolderUpdateRequest represents a request to update a folder.
type FolderUpdateRequest struct {
	Title     string `json:"title"`
	Version   int64  `json:"version"`
	ParentUID string `json:"parentUid,omitempty"`
}

// CreateFolder creates a new folder.
func (c *Client) CreateFolder(ctx context.Context, req FolderCreateRequest) (*Folder, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/folders", req)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create folder: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var folder Folder
	if err := json.Unmarshal(body, &folder); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &folder, nil
}

// UpdateFolder updates an existing folder by UID.
func (c *Client) UpdateFolder(ctx context.Context, uid string, req FolderUpdateRequest) (*Folder, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, "/api/folders/"+uid, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update folder: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update folder: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var folder Folder
	if err := json.Unmarshal(body, &folder); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &folder, nil
}

// GetFolderByUID gets a folder by its UID.
func (c *Client) GetFolderByUID(ctx context.Context, uid string) (*Folder, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/folders/"+uid, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Folder doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get folder: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var folder Folder
	if err := json.Unmarshal(body, &folder); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &folder, nil
}

// DeleteFolderByUID deletes a folder by its UID.
func (c *Client) DeleteFolderByUID(ctx context.Context, uid string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/folders/"+uid, nil)
	if err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete folder: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// Organization represents a Grafana organization.
type Organization struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

// OrganizationCreateRequest represents a request to create an organization.
type OrganizationCreateRequest struct {
	Name string `json:"name"`
}

// OrganizationCreateResponse represents the response from creating an organization.
type OrganizationCreateResponse struct {
	OrgID   int64  `json:"orgId"`
	Message string `json:"message"`
}

// OrgUser represents a user in an organization.
type OrgUser struct {
	OrgID  int64  `json:"orgId,omitempty"`
	UserID int64  `json:"userId,omitempty"`
	Email  string `json:"email,omitempty"`
	Login  string `json:"login,omitempty"`
	Role   string `json:"role,omitempty"`
}

// OrgUserAddRequest represents a request to add a user to an organization.
type OrgUserAddRequest struct {
	LoginOrEmail string `json:"loginOrEmail"`
	Role         string `json:"role"`
}

// CreateOrganization creates a new organization.
func (c *Client) CreateOrganization(ctx context.Context, req OrganizationCreateRequest) (*OrganizationCreateResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/orgs", req)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create organization: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var orgResp OrganizationCreateResponse
	if err := json.Unmarshal(body, &orgResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &orgResp, nil
}

// UpdateOrganization updates an existing organization by ID.
func (c *Client) UpdateOrganization(ctx context.Context, orgID int64, req OrganizationCreateRequest) error {
	resp, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/orgs/%d", orgID), req)
	if err != nil {
		return fmt.Errorf("failed to update organization: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update organization: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// GetOrganizationByID gets an organization by its ID.
func (c *Client) GetOrganizationByID(ctx context.Context, orgID int64) (*Organization, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/orgs/%d", orgID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Organization doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get organization: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var org Organization
	if err := json.Unmarshal(body, &org); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &org, nil
}

// GetOrganizationByName gets an organization by its name.
func (c *Client) GetOrganizationByName(ctx context.Context, name string) (*Organization, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/orgs/name/"+name, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Organization doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get organization: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var org Organization
	if err := json.Unmarshal(body, &org); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &org, nil
}

// DeleteOrganization deletes an organization by its ID.
func (c *Client) DeleteOrganization(ctx context.Context, orgID int64) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/orgs/%d", orgID), nil)
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete organization: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// GetOrgUsers gets all users in an organization.
func (c *Client) GetOrgUsers(ctx context.Context, orgID int64) ([]OrgUser, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/orgs/%d/users", orgID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization users: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get organization users: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var users []OrgUser
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return users, nil
}

// AddOrgUser adds a user to an organization.
func (c *Client) AddOrgUser(ctx context.Context, orgID int64, req OrgUserAddRequest) error {
	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/orgs/%d/users", orgID), req)
	if err != nil {
		return fmt.Errorf("failed to add user to organization: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to add user to organization: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// UpdateOrgUserRole updates a user's role in an organization.
func (c *Client) UpdateOrgUserRole(ctx context.Context, orgID, userID int64, role string) error {
	req := struct {
		Role string `json:"role"`
	}{Role: role}
	resp, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/orgs/%d/users/%d", orgID, userID), req)
	if err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update user role: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// RemoveOrgUser removes a user from an organization.
func (c *Client) RemoveOrgUser(ctx context.Context, orgID, userID int64) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/orgs/%d/users/%d", orgID, userID), nil)
	if err != nil {
		return fmt.Errorf("failed to remove user from organization: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to remove user from organization: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// GetUserByLoginOrEmail gets a user by their login or email.
func (c *Client) GetUserByLoginOrEmail(ctx context.Context, loginOrEmail string) (*OrgUser, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/users/lookup?loginOrEmail="+loginOrEmail, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // User doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var user OrgUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &user, nil
}

// DashboardPermissionItem represents a single permission entry for a dashboard.
type DashboardPermissionItem struct {
	ID             int64  `json:"id,omitempty"`
	DashboardID    int64  `json:"dashboardId,omitempty"`
	UserID         int64  `json:"userId,omitempty"`
	UserLogin      string `json:"userLogin,omitempty"`
	UserEmail      string `json:"userEmail,omitempty"`
	TeamID         int64  `json:"teamId,omitempty"`
	Team           string `json:"team,omitempty"`
	Role           string `json:"role,omitempty"`
	Permission     int    `json:"permission"`
	PermissionName string `json:"permissionName,omitempty"`
	UID            string `json:"uid,omitempty"`
	Title          string `json:"title,omitempty"`
	Slug           string `json:"slug,omitempty"`
	IsFolder       bool   `json:"isFolder,omitempty"`
	URL            string `json:"url,omitempty"`
	Created        string `json:"created,omitempty"`
	Updated        string `json:"updated,omitempty"`
}

// DashboardPermissionRequest represents the request body for setting dashboard permissions.
type DashboardPermissionRequest struct {
	Items []DashboardPermissionRequestItem `json:"items"`
}

// DashboardPermissionRequestItem represents a single permission item in the request.
type DashboardPermissionRequestItem struct {
	Role       string `json:"role,omitempty"`
	Permission int    `json:"permission"`
	TeamID     int64  `json:"teamId,omitempty"`
	UserID     int64  `json:"userId,omitempty"`
}

// PermissionNameToLevel converts a permission name to its numeric level.
// View=1, Edit=2, Admin=4
func PermissionNameToLevel(name string) int {
	switch name {
	case "View":
		return 1
	case "Edit":
		return 2
	case "Admin":
		return 4
	default:
		return 0
	}
}

// PermissionLevelToName converts a numeric permission level to its name.
func PermissionLevelToName(level int) string {
	switch level {
	case 1:
		return "View"
	case 2:
		return "Edit"
	case 4:
		return "Admin"
	default:
		return ""
	}
}

// GetDashboardPermissions gets all permissions for a dashboard by its UID.
func (c *Client) GetDashboardPermissions(ctx context.Context, dashboardUID string) ([]DashboardPermissionItem, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/dashboards/uid/"+dashboardUID+"/permissions", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard permissions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Dashboard doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get dashboard permissions: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var permissions []DashboardPermissionItem
	if err := json.Unmarshal(body, &permissions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return permissions, nil
}

// SetDashboardPermissions sets all permissions for a dashboard by its UID.
// This operation replaces all existing permissions.
func (c *Client) SetDashboardPermissions(ctx context.Context, dashboardUID string, req DashboardPermissionRequest) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/dashboards/uid/"+dashboardUID+"/permissions", req)
	if err != nil {
		return fmt.Errorf("failed to set dashboard permissions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to set dashboard permissions: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// FolderPermissionItem represents a single permission entry for a folder.
type FolderPermissionItem struct {
	ID             int64  `json:"id,omitempty"`
	FolderUID      string `json:"uid,omitempty"`
	UserID         int64  `json:"userId,omitempty"`
	UserLogin      string `json:"userLogin,omitempty"`
	UserEmail      string `json:"userEmail,omitempty"`
	TeamID         int64  `json:"teamId,omitempty"`
	Team           string `json:"team,omitempty"`
	Role           string `json:"role,omitempty"`
	Permission     int    `json:"permission"`
	PermissionName string `json:"permissionName,omitempty"`
	Created        string `json:"created,omitempty"`
	Updated        string `json:"updated,omitempty"`
}

// FolderPermissionRequest represents the request body for setting folder permissions.
type FolderPermissionRequest struct {
	Items []FolderPermissionRequestItem `json:"items"`
}

// FolderPermissionRequestItem represents a single permission item in the request.
type FolderPermissionRequestItem struct {
	Role       string `json:"role,omitempty"`
	Permission int    `json:"permission"`
	TeamID     int64  `json:"teamId,omitempty"`
	UserID     int64  `json:"userId,omitempty"`
}

// GetFolderPermissions gets all permissions for a folder by its UID.
func (c *Client) GetFolderPermissions(ctx context.Context, folderUID string) ([]FolderPermissionItem, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/folders/"+folderUID+"/permissions", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder permissions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Folder doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get folder permissions: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var permissions []FolderPermissionItem
	if err := json.Unmarshal(body, &permissions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return permissions, nil
}

// SetFolderPermissions sets all permissions for a folder by its UID.
// This operation replaces all existing permissions.
func (c *Client) SetFolderPermissions(ctx context.Context, folderUID string, req FolderPermissionRequest) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/folders/"+folderUID+"/permissions", req)
	if err != nil {
		return fmt.Errorf("failed to set folder permissions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to set folder permissions: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Set authentication header
	if strings.HasPrefix(c.authHeader, "basic:") {
		parts := strings.SplitN(c.authHeader, ":", 3)
		if len(parts) == 3 {
			req.SetBasicAuth(parts[1], parts[2])
		}
	} else if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}

	// Set org ID header if specified
	if c.orgID != nil {
		req.Header.Set("X-Grafana-Org-Id", fmt.Sprintf("%d", *c.orgID))
	}

	return req, nil
}

// =============================================================================
// Rule Group API
// =============================================================================

// AlertRuleGroup represents a Grafana alert rule group.
type AlertRuleGroup struct {
	FolderUID string      `json:"folderUid,omitempty"`
	Title     string      `json:"title"`
	Interval  int64       `json:"interval"`
	Rules     []AlertRule `json:"rules"`
}

// AlertRule represents a single alert rule within a rule group.
type AlertRule struct {
	UID                  string             `json:"uid,omitempty"`
	OrgID                int64              `json:"orgID,omitempty"`
	FolderUID            string             `json:"folderUID,omitempty"`
	RuleGroup            string             `json:"ruleGroup,omitempty"`
	Title                string             `json:"title"`
	Condition            string             `json:"condition"`
	Data                 []AlertQuery       `json:"data"`
	For                  string             `json:"for,omitempty"`
	NoDataState          string             `json:"noDataState,omitempty"`
	ExecErrState         string             `json:"execErrState,omitempty"`
	Labels               map[string]string  `json:"labels,omitempty"`
	Annotations          map[string]string  `json:"annotations,omitempty"`
	IsPaused             bool               `json:"isPaused,omitempty"`
	NotificationSettings *AlertNotification `json:"notification_settings,omitempty"`
	Provenance           string             `json:"provenance,omitempty"`
}

// AlertQuery represents a query within an alert rule.
type AlertQuery struct {
	RefID             string          `json:"refId"`
	DatasourceUID     string          `json:"datasourceUid"`
	QueryType         string          `json:"queryType,omitempty"`
	RelativeTimeRange *AlertTimeRange `json:"relativeTimeRange,omitempty"`
	Model             map[string]any  `json:"model"`
}

// AlertTimeRange represents a relative time range for a query.
type AlertTimeRange struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

// AlertNotification represents notification settings for an alert rule.
type AlertNotification struct {
	Receiver          string   `json:"receiver"`
	GroupBy           []string `json:"group_by,omitempty"`
	GroupWait         string   `json:"group_wait,omitempty"`
	GroupInterval     string   `json:"group_interval,omitempty"`
	RepeatInterval    string   `json:"repeat_interval,omitempty"`
	MuteTimeIntervals []string `json:"mute_time_intervals,omitempty"`
}

// GetRuleGroup retrieves a rule group by folder UID and group name.
func (c *Client) GetRuleGroup(ctx context.Context, folderUID, groupName string) (*AlertRuleGroup, error) {
	path := fmt.Sprintf("/api/v1/provisioning/folder/%s/rule-groups/%s", folderUID, url.PathEscape(groupName))
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get rule group: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get rule group: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var rg AlertRuleGroup
	if err := json.Unmarshal(body, &rg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &rg, nil
}

// CreateOrUpdateRuleGroup creates or updates a rule group.
func (c *Client) CreateOrUpdateRuleGroup(ctx context.Context, folderUID string, rg AlertRuleGroup, disableProvenance bool) (*AlertRuleGroup, error) {
	path := fmt.Sprintf("/api/v1/provisioning/folder/%s/rule-groups/%s", folderUID, url.PathEscape(rg.Title))

	httpReq, err := c.newRequest(ctx, http.MethodPut, path, rg)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if disableProvenance {
		httpReq.Header.Set("X-Disable-Provenance", "true")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update rule group: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		return nil, newAPIError(resp.StatusCode, body)
	}

	var result AlertRuleGroup
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// DeleteRuleGroup deletes a rule group.
func (c *Client) DeleteRuleGroup(ctx context.Context, folderUID, groupName string) error {
	path := fmt.Sprintf("/api/v1/provisioning/folder/%s/rule-groups/%s", folderUID, url.PathEscape(groupName))
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete rule group: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete rule group: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// NotificationPolicyTree represents the entire notification policy tree in Grafana.
type NotificationPolicyTree struct {
	Receiver       string              `json:"receiver"`
	GroupBy        []string            `json:"group_by,omitempty"`
	GroupWait      string              `json:"group_wait,omitempty"`
	GroupInterval  string              `json:"group_interval,omitempty"`
	RepeatInterval string              `json:"repeat_interval,omitempty"`
	Routes         []NotificationRoute `json:"routes,omitempty"`
}

// NotificationRoute represents a nested routing policy.
type NotificationRoute struct {
	Receiver            string              `json:"receiver,omitempty"`
	GroupBy             []string            `json:"group_by,omitempty"`
	GroupWait           string              `json:"group_wait,omitempty"`
	GroupInterval       string              `json:"group_interval,omitempty"`
	RepeatInterval      string              `json:"repeat_interval,omitempty"`
	Continue            bool                `json:"continue,omitempty"`
	ObjectMatchers      [][]string          `json:"object_matchers,omitempty"`
	MuteTimeIntervals   []string            `json:"mute_time_intervals,omitempty"`
	ActiveTimeIntervals []string            `json:"active_time_intervals,omitempty"`
	Routes              []NotificationRoute `json:"routes,omitempty"`
}

// GetNotificationPolicy retrieves the notification policy tree.
func (c *Client) GetNotificationPolicy(ctx context.Context) (*NotificationPolicyTree, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/provisioning/policies", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification policy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get notification policy: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result NotificationPolicyTree
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// SetNotificationPolicy sets the notification policy tree.
func (c *Client) SetNotificationPolicy(ctx context.Context, policy NotificationPolicyTree, disableProvenance bool) error {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/provisioning/policies", policy)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if disableProvenance {
		httpReq.Header.Set("X-Disable-Provenance", "true")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to set notification policy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to set notification policy: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// ResetNotificationPolicy resets the notification policy to the default.
func (c *Client) ResetNotificationPolicy(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/provisioning/policies", nil)
	if err != nil {
		return fmt.Errorf("failed to reset notification policy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to reset notification policy: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}
