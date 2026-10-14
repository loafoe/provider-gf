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

package team

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
	"github.com/crossplane/provider-gf/internal/clients/grafana/grafanatest"
)

// liveOrgID is the org the live tests reconcile against.
const liveOrgID = int64(1)

// liveExternal returns a Team external client pointed at the live Grafana.
func liveExternal(t *testing.T) (*external, *grafana.Client) {
	t.Helper()
	c := grafanatest.Client(t)
	return &external{client: c, orgID: liveOrgID}, c
}

// observeTeam runs Observe and returns existence and up-to-dateness.
func observeTeam(t *testing.T, e *external, cr *v1alpha1.Team) (exists, upToDate bool) {
	t.Helper()
	obs, err := e.Observe(t.Context(), cr)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	return obs.ResourceExists, obs.ResourceUpToDate
}

// teamWithName builds a Team CR with no external name, i.e. not yet created.
func teamWithName(name string) *v1alpha1.Team {
	return &v1alpha1.Team{
		ObjectMeta: metav1.ObjectMeta{Name: "live-test"},
		Spec:       v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{Name: name}},
	}
}

// adoptedTeam builds a Team CR already bound to an existing team.
func adoptedTeam(name string, teamID int64) *v1alpha1.Team {
	cr := teamWithName(name)
	cr.Annotations = map[string]string{
		"crossplane.io/external-name": FormatExternalName(liveOrgID, teamID),
	}
	return cr
}

func mustTeamID(t *testing.T, cr *v1alpha1.Team) int64 {
	t.Helper()
	_, teamID, err := ParseExternalName(meta.GetExternalName(cr))
	if err != nil {
		t.Fatalf("parse external-name: %v", err)
	}
	return teamID
}

func ptr[T any](v T) *T { return &v }

func TestLiveTeam_Lifecycle(t *testing.T) {
	e, c := liveExternal(t)
	name := grafanatest.UniqueName("lifecycle")
	cr := teamWithName(name)

	// Not there yet.
	if exists, _ := observeTeam(t, e, cr); exists {
		t.Fatal("Observe() reported the team exists before Create()")
	}

	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	teamID := mustTeamID(t, cr)
	grafanatest.CleanupTeam(t, c, teamID)
	t.Logf("created team %q with id %d", name, teamID)

	exists, upToDate := observeTeam(t, e, cr)
	if !exists || !upToDate {
		t.Fatalf("Observe() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}
	if cr.Status.AtProvider.Name == nil || *cr.Status.AtProvider.Name != name {
		t.Errorf("status.atProvider.name = %v, want %q", cr.Status.AtProvider.Name, name)
	}
	if cr.Status.AtProvider.ID == nil || *cr.Status.AtProvider.ID != teamID {
		t.Errorf("status.atProvider.id = %v, want %d", cr.Status.AtProvider.ID, teamID)
	}

	// Update through the reconciler.
	cr.Spec.ForProvider.Name = name + "-renamed"
	cr.Spec.ForProvider.Email = ptr("renamed@example.com")
	if _, err := e.Update(t.Context(), cr); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	exists, upToDate = observeTeam(t, e, cr)
	if !exists || !upToDate {
		t.Fatalf("Observe() after Update() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}
	if got := cr.Status.AtProvider.Email; got == nil || *got != "renamed@example.com" {
		t.Errorf("status.atProvider.email = %v, want %q", got, "renamed@example.com")
	}

	if _, err := e.Delete(t.Context(), cr); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if exists, _ := observeTeam(t, e, cr); exists {
		t.Error("Observe() reported the team still exists after Delete()")
	}
}

// A team renamed in the Grafana UI must be reported as drift, not as up to date.
func TestLiveTeam_DetectsOutOfBandRename(t *testing.T) {
	e, c := liveExternal(t)
	name := grafanatest.UniqueName("drift")
	teamID := grafanatest.CreateTeam(t, c, name)
	cr := adoptedTeam(name, teamID)

	if exists, upToDate := observeTeam(t, e, cr); !exists || !upToDate {
		t.Fatalf("Observe() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}

	renamed := name + "-ui-edit"
	if err := c.UpdateTeam(t.Context(), teamID, grafana.TeamCreateRequest{Name: renamed}); err != nil {
		t.Fatalf("UpdateTeam() error = %v", err)
	}

	if exists, upToDate := observeTeam(t, e, cr); !exists || upToDate {
		t.Fatalf("Observe() after out-of-band rename = (exists=%v, upToDate=%v), want (true, false)", exists, upToDate)
	}
}

// Losing the external name must not orphan the team: Observe recovers it by
// name, because team names are unique within an org.
func TestLiveTeam_RecoversByNameWhenExternalNameLost(t *testing.T) {
	e, c := liveExternal(t)
	name := grafanatest.UniqueName("recover")
	teamID := grafanatest.CreateTeam(t, c, name)
	cr := teamWithName(name) // no external name, as if the annotation was lost

	if exists, upToDate := observeTeam(t, e, cr); !exists || !upToDate {
		t.Fatalf("Observe() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}

	if got, want := meta.GetExternalName(cr), FormatExternalName(liveOrgID, teamID); got != want {
		t.Errorf("external-name = %q, want %q (recovered by name)", got, want)
	}
	if cr.Status.AtProvider.ID == nil || *cr.Status.AtProvider.ID != teamID {
		t.Errorf("status.atProvider.id = %v, want %d", cr.Status.AtProvider.ID, teamID)
	}
}

// Grafana's name search is a substring match, so recovery must not adopt a
// team whose name merely contains the spec's name.
func TestLiveTeam_RecoveryRequiresExactNameMatch(t *testing.T) {
	e, c := liveExternal(t)
	decoy := grafanatest.UniqueName("decoy")
	grafanatest.CreateTeam(t, c, decoy)

	// Truncating the decoy's name makes the decoy a substring match, but no
	// team is named exactly this.
	cr := teamWithName(decoy[:len(decoy)-1])
	if exists, _ := observeTeam(t, e, cr); exists {
		t.Error("Observe() adopted a team whose name only contains the spec name")
	}

	// The exact name is adopted.
	cr.Spec.ForProvider.Name = decoy
	if exists, _ := observeTeam(t, e, cr); !exists {
		t.Error("Observe() did not adopt the exactly-named team")
	}
}

// Membership is only owned when members/admins is set, and then the set is
// authoritative - users added in the Grafana UI must be reconciled away.
func TestLiveTeam_MembershipIsAuthoritativeWhenSet(t *testing.T) {
	e, c := liveExternal(t)
	name := grafanatest.UniqueName("members")

	first, second := twoOrgUsers(t, c)
	t.Logf("testing membership with users %d (%s) and %d (%s)", first.UserID, first.Email, second.UserID, second.Email)

	cr := teamWithName(name)
	cr.Spec.ForProvider.Members = []string{first.Email}

	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	teamID := mustTeamID(t, cr)
	grafanatest.CleanupTeam(t, c, teamID)

	if exists, upToDate := observeTeam(t, e, cr); !exists || !upToDate {
		t.Fatalf("Observe() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}
	if ids := grafanatest.MemberIDs(t, c, teamID); len(ids) != 1 {
		t.Fatalf("team has %d members, want 1 (members applied on create)", len(ids))
	}
	if cr.Status.AtProvider.MemberCount == nil || *cr.Status.AtProvider.MemberCount != 1 {
		t.Errorf("status.atProvider.memberCount = %v, want 1", cr.Status.AtProvider.MemberCount)
	}

	// Someone adds a member through the UI.
	if err := c.AddTeamMember(t.Context(), teamID, grafana.TeamMemberAddRequest{UserID: second.UserID}); err != nil {
		t.Fatalf("AddTeamMember() error = %v", err)
	}
	if _, upToDate := observeTeam(t, e, cr); upToDate {
		t.Error("Observe() reported up to date after a member was added outside Crossplane")
	}

	if _, err := e.Update(t.Context(), cr); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, upToDate := observeTeam(t, e, cr); !upToDate {
		t.Error("Observe() reported drift after Update() removed the unlisted member")
	}
	if ids := grafanatest.MemberIDs(t, c, teamID); len(ids) != 1 {
		t.Errorf("team has %d members, want 1 after authoritative reconcile", len(ids))
	}
}

// With neither members nor admins set the Team must not touch membership, so
// standalone TeamMember resources can own it.
func TestLiveTeam_LeavesMembershipAloneWhenUnset(t *testing.T) {
	e, c := liveExternal(t)
	name := grafanatest.UniqueName("unmanaged-members")
	teamID := grafanatest.CreateTeam(t, c, name)
	cr := adoptedTeam(name, teamID)

	// Add a member out of band.
	user, _ := twoOrgUsers(t, c)
	if err := c.AddTeamMember(t.Context(), teamID, grafana.TeamMemberAddRequest{UserID: user.UserID}); err != nil {
		t.Fatalf("AddTeamMember() error = %v", err)
	}

	if exists, upToDate := observeTeam(t, e, cr); !exists || !upToDate {
		t.Fatalf("Observe() = (exists=%v, upToDate=%v), want (true, true) - membership is not owned", exists, upToDate)
	}
	if ids := grafanatest.MemberIDs(t, c, teamID); len(ids) != 1 {
		t.Errorf("team has %d members, want the out-of-band member left alone", len(ids))
	}
}

// Deleting a team that is already gone must succeed rather than wedge the CR.
func TestLiveTeam_DeleteIsIdempotent(t *testing.T) {
	e, c := liveExternal(t)
	name := grafanatest.UniqueName("idempotent-delete")
	teamID := grafanatest.CreateTeam(t, c, name)
	cr := adoptedTeam(name, teamID)

	if _, err := e.Delete(t.Context(), cr); err != nil {
		t.Fatalf("first Delete() error = %v", err)
	}
	if _, err := e.Delete(t.Context(), cr); err != nil {
		t.Errorf("second Delete() error = %v, want nil for an already-deleted team", err)
	}
}

// twoOrgUsers returns two distinct users in the live org, for membership tests.
func twoOrgUsers(t *testing.T, c *grafana.Client) (grafana.OrgUser, grafana.OrgUser) {
	t.Helper()

	users, err := c.GetOrgUsers(t.Context(), liveOrgID)
	if err != nil {
		t.Fatalf("GetOrgUsers() error = %v", err)
	}
	var found []grafana.OrgUser
	for _, u := range users {
		if u.Email != "" {
			found = append(found, u)
		}
	}
	if len(found) < 2 {
		t.Skipf("need 2 users with an email in org %d to test membership, found %d", liveOrgID, len(found))
	}
	return found[0], found[1]
}
