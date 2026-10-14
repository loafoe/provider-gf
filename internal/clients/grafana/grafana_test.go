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
	"encoding/json"
	"io"
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

// capturedRequest records what the Grafana stub actually received.
type capturedRequest struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// newStubClient starts a stub Grafana that records the request it receives and
// replies with the supplied status and body.
func newStubClient(t *testing.T, status int, response string) (*Client, *capturedRequest) {
	t.Helper()

	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method, captured.path, captured.query = r.Method, r.URL.Path, r.URL.RawQuery
		if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, &captured.body); err != nil {
				t.Errorf("unmarshal request body %q: %v", raw, err)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)

	c, err := NewClient(Config{URL: server.URL, Token: "test-token"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c, captured
}

func TestCreateTeam(t *testing.T) {
	c, got := newStubClient(t, http.StatusOK, `{"message":"Team created","teamId":7,"uid":"abc"}`)

	resp, err := c.CreateTeam(context.Background(), TeamCreateRequest{Name: "platform", Email: "team@example.com"})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}

	if resp.TeamID != 7 || resp.UID != "abc" {
		t.Errorf("CreateTeam() = %+v, want teamId 7 / uid abc", resp)
	}
	if got.method != http.MethodPost || got.path != "/api/teams" {
		t.Errorf("request = %s %s, want POST /api/teams", got.method, got.path)
	}
	if got.body["name"] != "platform" || got.body["email"] != "team@example.com" {
		t.Errorf("request body = %v, want name/email of the team", got.body)
	}
}

// Teams live in an org, so every call must be org-scoped rather than relying
// on the Grafana instance default.
func TestTeamRequests_SendOrgHeader(t *testing.T) {
	var gotOrgHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrgHeader = r.Header.Get("X-Grafana-Org-Id")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, err := NewClient(Config{URL: server.URL, Token: "test-token"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.SetOrgID(42)

	if _, err := c.GetTeamMembers(context.Background(), 7); err != nil {
		t.Fatalf("GetTeamMembers() error = %v", err)
	}

	if gotOrgHeader != "42" {
		t.Errorf("X-Grafana-Org-Id header = %q, want %q", gotOrgHeader, "42")
	}
}

func TestGetTeamByID_NotFoundReturnsNil(t *testing.T) {
	c, _ := newStubClient(t, http.StatusNotFound, `{"message":"Team not found"}`)

	team, err := c.GetTeamByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetTeamByID() error = %v, want nil (not found is not an error)", err)
	}
	if team != nil {
		t.Errorf("GetTeamByID() = %+v, want nil", team)
	}
}

// The controller treats a nil team as "recreate it", so a 404 must not be
// surfaced as an error.
func TestDeleteTeam_NotFoundIsSuccess(t *testing.T) {
	c, _ := newStubClient(t, http.StatusNotFound, `{"message":"Team not found"}`)

	if err := c.DeleteTeam(context.Background(), 7); err != nil {
		t.Errorf("DeleteTeam() error = %v, want nil for an already-deleted team", err)
	}
}

func TestGetTeamByName(t *testing.T) {
	c, got := newStubClient(t, http.StatusOK,
		`{"totalCount":2,"teams":[{"id":7,"name":"platform"},{"id":8,"name":"platform-eng"}]}`)

	team, err := c.GetTeamByName(context.Background(), "platform")
	if err != nil {
		t.Fatalf("GetTeamByName() error = %v", err)
	}

	// Grafana's name search is a substring match, so the controller's exact
	// match is what prevents adopting the wrong team.
	if team == nil || team.ID != 7 {
		t.Errorf("GetTeamByName() = %+v, want the team with ID 7", team)
	}
	if got.path != "/api/teams/search" {
		t.Errorf("request path = %q, want %q", got.path, "/api/teams/search")
	}
}

// Search results are URL-encoded, so team names containing spaces or other
// reserved characters must not break the request.
func TestSearchTeams_EncodesQuery(t *testing.T) {
	c, got := newStubClient(t, http.StatusOK, `{"totalCount":0,"teams":[]}`)

	if _, err := c.SearchTeams(context.Background(), "platform team/x"); err != nil {
		t.Fatalf("SearchTeams() error = %v", err)
	}

	if want := "perpage=1000&page=1&query=platform+team%2Fx"; got.query != want {
		t.Errorf("query = %q, want %q", got.query, want)
	}
}

func TestSetTeamMembers(t *testing.T) {
	c, got := newStubClient(t, http.StatusOK, `{"message":"Team memberships have been updated"}`)

	err := c.SetTeamMembers(context.Background(), 7, TeamMembersRequest{
		Members: []string{"a@example.com"},
		Admins:  []string{"b@example.com"},
	})
	if err != nil {
		t.Fatalf("SetTeamMembers() error = %v", err)
	}

	if got.method != http.MethodPut || got.path != "/api/teams/7/members" {
		t.Errorf("request = %s %s, want PUT /api/teams/7/members", got.method, got.path)
	}
	if got.body["members"] == nil || got.body["admins"] == nil {
		t.Errorf("request body = %v, want members and admins", got.body)
	}
}

func TestAddTeamMember(t *testing.T) {
	c, got := newStubClient(t, http.StatusOK, `{"message":"Member added to Team"}`)

	if err := c.AddTeamMember(context.Background(), 7, TeamMemberAddRequest{UserID: 3}); err != nil {
		t.Fatalf("AddTeamMember() error = %v", err)
	}

	if got.method != http.MethodPost || got.path != "/api/teams/7/members" {
		t.Errorf("request = %s %s, want POST /api/teams/7/members", got.method, got.path)
	}
	if got.body["userId"] != float64(3) {
		t.Errorf("request body = %v, want userId 3", got.body)
	}
}

func TestTeamPreferences(t *testing.T) {
	c, got := newStubClient(t, http.StatusOK,
		`{"theme":"dark","timezone":"utc","homeDashboardId":0,"homeDashboardUID":"abc"}`)

	prefs, err := c.GetTeamPreferences(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetTeamPreferences() error = %v", err)
	}

	if prefs.Theme != "dark" || prefs.Timezone != "utc" || prefs.HomeDashboardUID != "abc" {
		t.Errorf("GetTeamPreferences() = %+v, want dark/utc/abc", prefs)
	}
	if got.path != "/api/teams/7/preferences" {
		t.Errorf("request path = %q, want %q", got.path, "/api/teams/7/preferences")
	}
}

// A deleted team takes its preferences with it, so a 404 means "preferences do
// not exist" rather than "preferences could not be read".
func TestGetTeamPreferences_TeamNotFoundReturnsNil(t *testing.T) {
	c, _ := newStubClient(t, http.StatusNotFound, `{"message":"Team not found"}`)

	prefs, err := c.GetTeamPreferences(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetTeamPreferences() error = %v, want nil (not found is not an error)", err)
	}
	if prefs != nil {
		t.Errorf("GetTeamPreferences() = %+v, want nil", prefs)
	}
}

// GetUserByLoginOrEmail resolves against the org's user list rather than
// /api/users/lookup, so a service account only needs org.users:read.
func TestGetUserByLoginOrEmail_UsesOrgUsersEndpoint(t *testing.T) {
	c, got := newStubClient(t, http.StatusOK,
		`[{"orgId":1,"userId":1,"email":"admin@localhost","login":"admin","role":"Admin"}]`)

	user, err := c.GetUserByLoginOrEmail(context.Background(), "admin@localhost")
	if err != nil {
		t.Fatalf("GetUserByLoginOrEmail() error = %v", err)
	}
	if user == nil || user.UserID != 1 {
		t.Fatalf("GetUserByLoginOrEmail() = %+v, want user 1", user)
	}
	if got.path != "/api/orgs/1/users" {
		t.Errorf("request path = %q, want %q", got.path, "/api/orgs/1/users")
	}
	if q := got.query; q != "" {
		t.Errorf("query = %q, want none - the endpoint ignores loginOrEmail and the match is client-side", q)
	}
}

func TestGetUserByLoginOrEmail_Matching(t *testing.T) {
	body := `[{"orgId":1,"userId":1,"email":"admin@localhost","login":"admin"},
	          {"orgId":1,"userId":35,"email":"Viewer","login":"Viewer"}]`

	cases := map[string]struct {
		loginOrEmail string
		wantUserID   int64
	}{
		"matches email":             {"admin@localhost", 1},
		"matches login":             {"admin", 1},
		"is case insensitive":       {"Admin@LocalHost", 1},
		"matches an odd email":      {"viewer", 35},
		"case insensitive odd mail": {"VIEWER", 35},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, _ := newStubClient(t, http.StatusOK, body)
			user, err := c.GetUserByLoginOrEmail(context.Background(), tc.loginOrEmail)
			if err != nil {
				t.Fatalf("GetUserByLoginOrEmail() error = %v", err)
			}
			if user == nil || user.UserID != tc.wantUserID {
				t.Errorf("GetUserByLoginOrEmail(%q) = %+v, want user %d", tc.loginOrEmail, user, tc.wantUserID)
			}
		})
	}
}

// A user outside the org is not a valid team member, so an unmatched lookup
// must report "not found" rather than an error.
func TestGetUserByLoginOrEmail_NoMatchReturnsNil(t *testing.T) {
	c, _ := newStubClient(t, http.StatusOK,
		`[{"orgId":1,"userId":1,"email":"admin@localhost","login":"admin"}]`)

	user, err := c.GetUserByLoginOrEmail(context.Background(), "someone@example.com")
	if err != nil {
		t.Fatalf("GetUserByLoginOrEmail() error = %v, want nil", err)
	}
	if user != nil {
		t.Errorf("GetUserByLoginOrEmail() = %+v, want nil", user)
	}
}

// The lookup must follow the org the client is pinned to, not the org 1
// default, so multi-org ProviderConfigs don't resolve the wrong user.
func TestGetUserByLoginOrEmail_UsesClientOrg(t *testing.T) {
	c, got := newStubClient(t, http.StatusOK, `[]`)
	c.SetOrgID(42)

	if _, err := c.GetUserByLoginOrEmail(context.Background(), "someone@example.com"); err != nil {
		t.Fatalf("GetUserByLoginOrEmail() error = %v", err)
	}
	if got.path != "/api/orgs/42/users" {
		t.Errorf("request path = %q, want %q", got.path, "/api/orgs/42/users")
	}
}
