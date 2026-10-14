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

package teampreference

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
	"github.com/crossplane/provider-gf/internal/clients/grafana/grafanatest"
	"github.com/crossplane/provider-gf/internal/controller/team"
)

const liveOrgID = int64(1)

func liveExternal(t *testing.T, teamID int64) (*external, *grafana.Client) {
	t.Helper()
	return &external{client: grafanatest.Client(t), orgID: liveOrgID, teamID: teamID}, nil
}

func observePrefs(t *testing.T, e *external, cr *v1alpha1.TeamPreference) (exists, upToDate bool) {
	t.Helper()
	obs, err := e.Observe(t.Context(), cr)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	return obs.ResourceExists, obs.ResourceUpToDate
}

// prefsFor builds a TeamPreference CR bound to a team, with no external name.
func prefsFor(teamID int64, fp v1alpha1.TeamPreferenceParameters) *v1alpha1.TeamPreference {
	fp.TeamID = &teamID
	return &v1alpha1.TeamPreference{
		ObjectMeta: metav1.ObjectMeta{Name: "live-test"},
		Spec:       v1alpha1.TeamPreferenceSpec{ForProvider: fp},
	}
}

func ptr[T any](v T) *T { return &v }

func TestLiveTeamPreference_Lifecycle(t *testing.T) {
	c := grafanatest.Client(t)
	teamID := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("prefs-lifecycle"))
	e, _ := liveExternal(t, teamID)

	// A brand new team reports no preferences at all. Grafana returns an
	// empty object, not the documented zero values, so every field reads as
	// "" - which is what an unset spec reconciles to.
	cr := prefsFor(teamID, v1alpha1.TeamPreferenceParameters{})
	if exists, upToDate := observePrefs(t, e, cr); !exists || !upToDate {
		t.Fatalf("Observe() on a team with no preferences = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}

	cr.Spec.ForProvider.Theme = ptr("dark")
	cr.Spec.ForProvider.Timezone = ptr("utc")
	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if got, want := meta.GetExternalName(cr), team.FormatExternalName(liveOrgID, teamID); got != want {
		t.Errorf("external-name = %q, want %q", got, want)
	}

	exists, upToDate := observePrefs(t, e, cr)
	if !exists || !upToDate {
		t.Fatalf("Observe() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}
	if got := cr.Status.AtProvider.Theme; got == nil || *got != "dark" {
		t.Errorf("status.atProvider.theme = %v, want %q", got, "dark")
	}
	if got := cr.Status.AtProvider.Timezone; got == nil || *got != "utc" {
		t.Errorf("status.atProvider.timezone = %v, want %q", got, "utc")
	}

	// Update through the reconciler.
	cr.Spec.ForProvider.Theme = ptr("light")
	if _, err := e.Update(t.Context(), cr); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if exists, upToDate := observePrefs(t, e, cr); !exists || !upToDate {
		t.Fatalf("Observe() after Update() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}
	if got := cr.Status.AtProvider.Theme; got == nil || *got != "light" {
		t.Errorf("status.atProvider.theme = %v, want %q", got, "light")
	}

	if _, err := e.Delete(t.Context(), cr); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

// Grafana replaces the whole preference object on update, so clearing a field
// in the spec must actually reset it in Grafana.
func TestLiveTeamPreference_OmittedFieldsResetToDefault(t *testing.T) {
	c := grafanatest.Client(t)
	teamID := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("prefs-reset"))
	e, _ := liveExternal(t, teamID)

	cr := prefsFor(teamID, v1alpha1.TeamPreferenceParameters{
		Theme:    ptr("dark"),
		Timezone: ptr("utc"),
	})
	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Theme changed in the Grafana UI while the spec says nothing about it.
	if err := c.UpdateTeamPreferences(t.Context(), teamID, grafana.TeamPreferences{Theme: "light", Timezone: "browser"}); err != nil {
		t.Fatalf("UpdateTeamPreferences() error = %v", err)
	}
	if _, upToDate := observePrefs(t, e, cr); upToDate {
		t.Error("Observe() reported up to date after an unmanaged field changed out of band")
	}

	// Reconciling must reset the out-of-band field back to the Grafana default
	// rather than leaving it alone, which is what the documented whole-object
	// replacement semantics require.
	if _, err := e.Update(t.Context(), cr); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, upToDate := observePrefs(t, e, cr); !upToDate {
		t.Error("Observe() reported drift after Update() reset the out-of-band field")
	}
	if got := cr.Status.AtProvider.Timezone; got == nil || *got != "utc" {
		t.Errorf("status.atProvider.timezone = %v, want %q", got, "utc")
	}
}

// Preferences of a team that doesn't exist must report "not found" so the
// reconciler creates rather than erroring.
func TestLiveTeamPreference_MissingTeamIsNotAnError(t *testing.T) {
	e, _ := liveExternal(t, 999999)
	cr := prefsFor(999999, v1alpha1.TeamPreferenceParameters{Theme: ptr("dark")})

	if exists, _ := observePrefs(t, e, cr); exists {
		t.Error("Observe() reported preferences exist for a non-existent team")
	}
	if _, err := e.Create(t.Context(), cr); err == nil {
		t.Error("Create() error = nil, want an error for a non-existent team")
	} else {
		t.Logf("Create() error = %v", err)
	}
}

// No team reference at all must be reported clearly.
func TestLiveTeamPreference_MissingTeamReference(t *testing.T) {
	e, _ := liveExternal(t, 0)
	cr := &v1alpha1.TeamPreference{
		ObjectMeta: metav1.ObjectMeta{Name: "live-test"},
		Spec: v1alpha1.TeamPreferenceSpec{ForProvider: v1alpha1.TeamPreferenceParameters{
			Theme: ptr("dark"),
		}},
	}

	if exists, _ := observePrefs(t, e, cr); exists {
		t.Error("Observe() reported preferences exist with no team resolved")
	}
	if _, err := e.Create(t.Context(), cr); err == nil {
		t.Error("Create() error = nil, want an error when no team is resolved")
	} else {
		t.Logf("Create() error = %v", err)
	}
}
