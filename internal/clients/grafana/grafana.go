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
	"strings"
	"time"
)

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
