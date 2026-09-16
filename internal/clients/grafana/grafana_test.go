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

package grafana

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetOrgID_OverridesRequestOrgHeader(t *testing.T) {
	var gotOrgHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrgHeader = r.Header.Get("X-Grafana-Org-Id")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	providerConfigOrgID := int64(1)
	c, err := NewClient(Config{URL: server.URL, Token: "test-token", OrgID: &providerConfigOrgID})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// A resource resolves to a different org than the ProviderConfig default
	// (e.g. via orgRef/orgSelector/spec.forProvider.orgId).
	c.SetOrgID(42)

	if _, err := c.GetDataSourceByUID(context.Background(), "some-uid"); err != nil {
		t.Fatalf("GetDataSourceByUID() error = %v", err)
	}

	if gotOrgHeader != "42" {
		t.Errorf("X-Grafana-Org-Id header = %q, want %q (resolved org, not ProviderConfig default)", gotOrgHeader, "42")
	}
}

// TestNewClient_DefaultsOrgIDHeaderWhenNotSpecified guards the contract
// documented on ProviderConfigSpec.OrgID ("If not set, defaults to the
// default org"). Without this, an unset OrgID sends no header at all, and
// Grafana falls back to the authenticated (Basic Auth) user's ambient,
// stateful "current org" - which drifts whenever anyone, including a human
// admin using the same account, switches org in the Grafana UI. That drift
// is what caused datasources to be silently recreated under whatever org
// happened to be ambient at reconcile time.
func TestNewClient_DefaultsOrgIDHeaderWhenNotSpecified(t *testing.T) {
	var gotOrgHeader string
	sawHeader := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrgHeader, sawHeader = r.Header.Get("X-Grafana-Org-Id"), r.Header.Get("X-Grafana-Org-Id") != ""
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, err := NewClient(Config{URL: server.URL, Token: "test-token"}) // OrgID left nil
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := c.GetDataSourceByUID(context.Background(), "some-uid"); err != nil {
		t.Fatalf("GetDataSourceByUID() error = %v", err)
	}

	if !sawHeader || gotOrgHeader != "1" {
		t.Errorf("X-Grafana-Org-Id header = %q (sent=%v), want %q (default org, not ambient/unset)", gotOrgHeader, sawHeader, "1")
	}
}
