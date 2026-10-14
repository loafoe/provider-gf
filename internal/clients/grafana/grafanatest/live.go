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

// Package grafanatest provides helpers for tests that run against a real
// Grafana instance. Tests using it are skipped unless GRAFANA_URL and
// GRAFANA_TOKEN are set, so they never run in a normal `go test ./...`.
//
// The instance is mutated for real: every test must clean up the teams it
// creates.
package grafanatest

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/crossplane/provider-gf/internal/clients/grafana"
)

// TestPrefix is prepended to every team name created by a live test, so test
// resources are identifiable and removable.
const TestPrefix = "xp-live-test-"

// SkipUnlessLive skips the test unless a live Grafana is configured.
func SkipUnlessLive(t *testing.T) {
	t.Helper()
	if os.Getenv("GRAFANA_URL") == "" || os.Getenv("GRAFANA_TOKEN") == "" {
		t.Skip("set GRAFANA_URL and GRAFANA_TOKEN to run live Grafana tests")
	}
}

// Client returns a client pointed at the live Grafana instance.
func Client(t *testing.T) *grafana.Client {
	t.Helper()
	SkipUnlessLive(t)

	orgID := int64(1)
	if v := os.Getenv("GRAFANA_ORG_ID"); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			t.Fatalf("parse GRAFANA_ORG_ID %q: %v", v, err)
		}
		orgID = parsed
	}

	c, err := grafana.NewClient(grafana.Config{
		URL:   os.Getenv("GRAFANA_URL"),
		Token: os.Getenv("GRAFANA_TOKEN"),
		OrgID: &orgID,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c
}

// UniqueName returns a test-scoped name so parallel or repeated runs don't collide.
func UniqueName(suffix string) string {
	return TestPrefix + suffix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// CreateTeam creates a team for the test and deletes it during cleanup.
func CreateTeam(t *testing.T, c *grafana.Client, name string) int64 {
	t.Helper()

	resp, err := c.CreateTeam(t.Context(), grafana.TeamCreateRequest{Name: name})
	if err != nil {
		t.Fatalf("CreateTeam(%q) error = %v", name, err)
	}

	CleanupTeam(t, c, resp.TeamID)
	return resp.TeamID
}

// CleanupTeam deletes a team during test cleanup. Use it for teams created
// through the controller's own Create() rather than via CreateTeam, so a
// failing assertion can't leak the team.
func CleanupTeam(t *testing.T, c *grafana.Client, teamID int64) {
	t.Helper()

	t.Cleanup(func() {
		// Not t.Context(): the test context is already cancelled by the time
		// cleanups run, which would make every delete fail with "context
		// canceled" and leak teams.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := c.DeleteTeam(ctx, teamID); err != nil {
			t.Logf("cleanup DeleteTeam(%d) error = %v", teamID, err)
		}
	})
}

// CreateFolder creates a folder for the test and deletes it during cleanup.
// A folder with a team permission cannot be deleted while the team exists, so
// permissions are expected to be cleaned up by the test first.
func CreateFolder(t *testing.T, c *grafana.Client, title string) string {
	t.Helper()

	folder, err := c.CreateFolder(t.Context(), grafana.FolderCreateRequest{Title: title})
	if err != nil {
		t.Fatalf("CreateFolder(%q) error = %v", title, err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := c.DeleteFolderByUID(ctx, folder.UID); err != nil {
			t.Logf("cleanup DeleteFolder(%q) error = %v", folder.UID, err)
		}
	})
	return folder.UID
}

// CreateDashboard creates a dashboard in a folder and deletes it during cleanup.
func CreateDashboard(t *testing.T, c *grafana.Client, title, folderUID string) string {
	t.Helper()

	resp, err := c.CreateOrUpdateDashboard(t.Context(), grafana.DashboardCreateRequest{
		Dashboard: []byte(`{"title":"` + title + `","panels":[]}`),
		FolderUID: folderUID,
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateDashboard(%q) error = %v", title, err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := c.DeleteDashboardByUID(ctx, resp.UID); err != nil {
			t.Logf("cleanup DeleteDashboard(%q) error = %v", resp.UID, err)
		}
	})
	return resp.UID
}

// MemberIDs returns the user IDs currently in a team.
func MemberIDs(t *testing.T, c *grafana.Client, teamID int64) []int64 {
	t.Helper()

	members, err := c.GetTeamMembers(t.Context(), teamID)
	if err != nil {
		t.Fatalf("GetTeamMembers(%d) error = %v", teamID, err)
	}
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	return ids
}
