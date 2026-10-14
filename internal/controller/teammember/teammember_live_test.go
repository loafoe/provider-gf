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

package teammember

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
	"github.com/crossplane/provider-gf/internal/clients/grafana/grafanatest"
)

const (
	liveOrgID = int64(1)

	// liveTestName is the placeholder metadata.name for live test CRs.
	liveTestName = "live-test"
)

// liveExternal returns a TeamMember external client for the live Grafana.
func liveExternal(t *testing.T, teamID int64) (*external, *grafana.Client) {
	t.Helper()
	return &external{client: grafanatest.Client(t), orgID: liveOrgID, teamID: teamID}, nil
}

func observeMember(t *testing.T, e *external, cr *v1alpha1.TeamMember) (exists, upToDate bool) {
	t.Helper()
	obs, err := e.Observe(t.Context(), cr)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	return obs.ResourceExists, obs.ResourceUpToDate
}

func membershipFor(teamID, userID int64) *v1alpha1.TeamMember {
	cr := &v1alpha1.TeamMember{
		ObjectMeta: metav1.ObjectMeta{Name: liveTestName},
		Spec: v1alpha1.TeamMemberSpec{ForProvider: v1alpha1.TeamMemberParameters{
			TeamID: &teamID,
			UserID: &userID,
		}},
	}
	cr.Annotations = map[string]string{
		"crossplane.io/external-name": formatExternalName(liveOrgID, teamID, userID),
	}
	return cr
}

// orgUser returns a user from the live org, skipping if none is available.
func orgUser(t *testing.T, c *grafana.Client) grafana.OrgUser {
	t.Helper()
	users, err := c.GetOrgUsers(t.Context(), liveOrgID)
	if err != nil {
		t.Fatalf("GetOrgUsers() error = %v", err)
	}
	for _, u := range users {
		if u.Email != "" {
			return u
		}
	}
	t.Skipf("no users with an email in org %d", liveOrgID)
	return grafana.OrgUser{}
}

func TestLiveTeamMember_Lifecycle(t *testing.T) {
	c := grafanatest.Client(t)
	e, _ := liveExternal(t, 0)
	user := orgUser(t, c)
	teamID := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-lifecycle"))
	e.teamID = teamID

	cr := &v1alpha1.TeamMember{
		ObjectMeta: metav1.ObjectMeta{Name: liveTestName},
		Spec: v1alpha1.TeamMemberSpec{ForProvider: v1alpha1.TeamMemberParameters{
			TeamID: &teamID,
			UserID: &user.UserID,
		}},
	}

	if exists, _ := observeMember(t, e, cr); exists {
		t.Fatal("Observe() reported the membership exists before Create()")
	}

	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// The external name is what lets Delete remove the right membership even
	// if the spec changes later.
	if got, want := meta.GetExternalName(cr), formatExternalName(liveOrgID, teamID, user.UserID); got != want {
		t.Errorf("external-name = %q, want %q", got, want)
	}

	exists, upToDate := observeMember(t, e, cr)
	if !exists || !upToDate {
		t.Fatalf("Observe() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}
	if cr.Status.AtProvider.Email == nil || *cr.Status.AtProvider.Email != user.Email {
		t.Errorf("status.atProvider.email = %v, want %q", cr.Status.AtProvider.Email, user.Email)
	}

	// A member removed in the Grafana UI must be reported as gone, so the
	// reconciler re-adds them.
	if err := c.RemoveTeamMember(t.Context(), teamID, user.UserID); err != nil {
		t.Fatalf("RemoveTeamMember() error = %v", err)
	}
	if exists, _ := observeMember(t, e, cr); exists {
		t.Error("Observe() reported the membership still exists after it was removed out of band")
	}

	if _, err := e.Delete(t.Context(), cr); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if ids := grafanatest.MemberIDs(t, c, teamID); len(ids) != 0 {
		t.Errorf("team still has members %v after Delete()", ids)
	}
}

// Grafana answers 400 "User is already added to this team" if a member is
// added twice. Observe normally prevents that, but a member added in between
// must not fail Create with a confusing error.
func TestLiveTeamMember_AddExistingMemberIsNotAnError(t *testing.T) {
	c := grafanatest.Client(t)
	user := orgUser(t, c)
	teamID := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-existing"))
	e, _ := liveExternal(t, teamID)

	cr := membershipFor(teamID, user.UserID)

	// The member is added out of band, so Observe sees it and Create isn't
	// reached - call the client's add twice to pin the raw behaviour.
	if err := c.AddTeamMember(t.Context(), teamID, grafana.TeamMemberAddRequest{UserID: user.UserID}); err != nil {
		t.Fatalf("first AddTeamMember() error = %v", err)
	}
	if err := c.AddTeamMember(t.Context(), teamID, grafana.TeamMemberAddRequest{UserID: user.UserID}); err != nil {
		t.Errorf("second AddTeamMember() error = %v, want nil for an existing member", err)
	}

	if exists, upToDate := observeMember(t, e, cr); !exists || !upToDate {
		t.Errorf("Observe() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}
}

// Adding a user to a team that no longer exists must fail with a clear error,
// not a confusing one.
func TestLiveTeamMember_CreateFailsForMissingTeam(t *testing.T) {
	missingTeamID := int64(999999)
	e, _ := liveExternal(t, missingTeamID)
	cr := &v1alpha1.TeamMember{
		ObjectMeta: metav1.ObjectMeta{Name: liveTestName},
		Spec: v1alpha1.TeamMemberSpec{ForProvider: v1alpha1.TeamMemberParameters{
			TeamID: &missingTeamID,
			UserID: ptr(int64(1)),
		}},
	}

	_, err := e.Create(t.Context(), cr)
	if err == nil {
		t.Fatal("Create() error = nil, want an error for a non-existent team")
	}
	t.Logf("Create() error = %v", err)
}

// No team reference at all must be reported clearly, not panic.
func TestLiveTeamMember_MissingTeamReference(t *testing.T) {
	e, _ := liveExternal(t, 0)
	cr := &v1alpha1.TeamMember{
		ObjectMeta: metav1.ObjectMeta{Name: liveTestName},
		Spec:       v1alpha1.TeamMemberSpec{ForProvider: v1alpha1.TeamMemberParameters{UserID: ptr(int64(1))}},
	}

	exists, _ := observeMember(t, e, cr)
	if exists {
		t.Error("Observe() reported the membership exists with no team resolved")
	}
	if _, err := e.Create(t.Context(), cr); err == nil {
		t.Error("Create() error = nil, want an error when no team is resolved")
	} else {
		t.Logf("Create() error = %v", err)
	}
}

// No user reference at all must be reported clearly, not panic.
func TestLiveTeamMember_MissingUserReference(t *testing.T) {
	e, _ := liveExternal(t, 1)
	cr := &v1alpha1.TeamMember{
		ObjectMeta: metav1.ObjectMeta{Name: liveTestName},
		Spec:       v1alpha1.TeamMemberSpec{ForProvider: v1alpha1.TeamMemberParameters{TeamID: ptr(int64(1))}},
	}

	if _, err := e.Create(t.Context(), cr); err == nil {
		t.Error("Create() error = nil, want an error when no user is specified")
	}
}

// loginOrEmail is resolved against the org's user list, which needs only
// org.users:read - an instance-wide users:read lookup would be unavailable to
// a service account, since Grafana reserves it for server admins.
func TestLiveTeamMember_LoginOrEmailResolution(t *testing.T) {
	c := grafanatest.Client(t)
	user := orgUser(t, c)
	teamID := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-login"))
	e, _ := liveExternal(t, teamID)

	cr := &v1alpha1.TeamMember{
		ObjectMeta: metav1.ObjectMeta{Name: liveTestName},
		Spec: v1alpha1.TeamMemberSpec{ForProvider: v1alpha1.TeamMemberParameters{
			TeamID:       &teamID,
			LoginOrEmail: &user.Email,
		}},
	}

	if exists, _ := observeMember(t, e, cr); exists {
		t.Fatal("Observe() reported the membership exists before Create()")
	}

	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() with loginOrEmail error = %v", err)
	}

	// The lookup must resolve to the user the spec named, otherwise the
	// membership would be attached to whoever happened to match first.
	if got, want := meta.GetExternalName(cr), formatExternalName(liveOrgID, teamID, user.UserID); got != want {
		t.Errorf("external-name = %q, want %q", got, want)
	}
	if exists, _ := observeMember(t, e, cr); !exists {
		t.Error("Observe() did not see the membership created by loginOrEmail")
	}
	if ids := grafanatest.MemberIDs(t, c, teamID); len(ids) != 1 || ids[0] != user.UserID {
		t.Errorf("team members = %v, want only user %d", ids, user.UserID)
	}
}

// A loginOrEmail that isn't an org member must fail clearly rather than
// silently attaching nobody.
func TestLiveTeamMember_UnknownLoginOrEmailFails(t *testing.T) {
	c := grafanatest.Client(t)
	teamID := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-unknown"))
	e, _ := liveExternal(t, teamID)

	unknown := "no-such-user-xyz@example.invalid"
	cr := &v1alpha1.TeamMember{
		ObjectMeta: metav1.ObjectMeta{Name: liveTestName},
		Spec: v1alpha1.TeamMemberSpec{ForProvider: v1alpha1.TeamMemberParameters{
			TeamID:       &teamID,
			LoginOrEmail: &unknown,
		}},
	}

	if exists, _ := observeMember(t, e, cr); exists {
		t.Error("Observe() reported a membership for an unknown user")
	}
	if _, err := e.Create(t.Context(), cr); err == nil {
		t.Error("Create() error = nil, want an error for a user that isn't in the org")
	} else {
		t.Logf("Create() error = %v", err)
	}
	if ids := grafanatest.MemberIDs(t, c, teamID); len(ids) != 0 {
		t.Errorf("team has members %v, want none", ids)
	}
}

// Changing the user must move the membership, not duplicate it.
func TestLiveTeamMember_ChangingUserMovesMembership(t *testing.T) {
	c := grafanatest.Client(t)
	users, err := c.GetOrgUsers(t.Context(), liveOrgID)
	if err != nil {
		t.Fatalf("GetOrgUsers() error = %v", err)
	}
	var withEmail []grafana.OrgUser
	for _, u := range users {
		if u.Email != "" {
			withEmail = append(withEmail, u)
		}
	}
	if len(withEmail) < 2 {
		t.Skipf("need 2 users with an email in org %d, found %d", liveOrgID, len(withEmail))
	}
	first, second := withEmail[0], withEmail[1]

	teamID := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-move"))
	e, _ := liveExternal(t, teamID)

	cr := membershipFor(teamID, first.UserID)
	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if exists, _ := observeMember(t, e, cr); !exists {
		t.Fatal("membership for the first user was not observed")
	}

	cr.Spec.ForProvider.UserID = &second.UserID
	if _, err := e.Update(t.Context(), cr); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if got, want := meta.GetExternalName(cr), formatExternalName(liveOrgID, teamID, second.UserID); got != want {
		t.Errorf("external-name = %q, want %q", got, want)
	}
	ids := grafanatest.MemberIDs(t, c, teamID)
	if len(ids) != 1 || ids[0] != second.UserID {
		t.Errorf("team members = %v, want only user %d", ids, second.UserID)
	}
}

// Moving to another team must remove the old membership too.
func TestLiveTeamMember_ChangingTeamMovesMembership(t *testing.T) {
	c := grafanatest.Client(t)
	user := orgUser(t, c)
	fromTeam := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-from"))
	toTeam := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-to"))

	e, _ := liveExternal(t, fromTeam)
	cr := membershipFor(fromTeam, user.UserID)
	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	e.teamID = toTeam
	cr.Spec.ForProvider.TeamID = &toTeam
	if _, err := e.Update(t.Context(), cr); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if ids := grafanatest.MemberIDs(t, c, fromTeam); len(ids) != 0 {
		t.Errorf("old team still has members %v, want none", ids)
	}
	if ids := grafanatest.MemberIDs(t, c, toTeam); len(ids) != 1 || ids[0] != user.UserID {
		t.Errorf("new team members = %v, want only user %d", ids, user.UserID)
	}
}

// Delete must remove the membership recorded in the external name even when
// the spec no longer matches it.
func TestLiveTeamMember_DeleteUsesExternalName(t *testing.T) {
	c := grafanatest.Client(t)
	user := orgUser(t, c)
	teamID := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-del-by-ext"))
	e, _ := liveExternal(t, teamID)

	cr := membershipFor(teamID, user.UserID)
	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// The spec is edited to point at a different team after the membership
	// was created; Delete must still remove the real membership.
	otherTeam := grafanatest.CreateTeam(t, c, grafanatest.UniqueName("member-del-target"))
	cr.Spec.ForProvider.TeamID = &otherTeam
	e.teamID = otherTeam

	if _, err := e.Delete(t.Context(), cr); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if ids := grafanatest.MemberIDs(t, c, teamID); len(ids) != 0 {
		t.Errorf("original team still has members %v, want none", ids)
	}
}

func ptr[T any](v T) *T { return &v }
