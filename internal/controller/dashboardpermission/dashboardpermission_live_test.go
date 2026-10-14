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

package dashboardpermission

import (
	"strconv"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana/grafanatest"
)

const (
	liveOrgID = int64(1)

	// testNamespace is where the fake-client Team objects live.
	testNamespace = "default"
)

// schemeWithTeams registers the types the fake client needs to resolve refs.
var schemeWithTeams = runtime.NewScheme()

func init() {
	_ = v1alpha1.SchemeBuilder.AddToScheme(schemeWithTeams)
}

// teamResource builds a Team that already exists in Grafana, so its external
// name carries a real team ID. Teams use the <orgId>:<teamId> format, which
// is this package's team parser's contract, not formatExternalName (that one
// is for dashboards).
func teamResource(t *testing.T, name string, teamID int64) *v1alpha1.Team {
	t.Helper()
	return &v1alpha1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"crossplane.io/external-name": strconv.FormatInt(liveOrgID, 10) + ":" + strconv.FormatInt(teamID, 10),
			},
		},
		Spec:   v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{Name: name}},
		Status: v1alpha1.TeamStatus{AtProvider: v1alpha1.TeamObservation{ID: &teamID}},
	}
}

// resolveWith builds an external plus a fake kube client holding the Teams.
func resolveWith(t *testing.T, teams ...*v1alpha1.Team) (*external, *v1alpha1.DashboardPermission) {
	t.Helper()

	objects := make([]runtime.Object, 0, len(teams))
	for _, tm := range teams {
		objects = append(objects, tm)
	}
	kube := fake.NewClientBuilder().WithScheme(schemeWithTeams).WithRuntimeObjects(objects...).Build()

	cr := &v1alpha1.DashboardPermission{
		ObjectMeta: metav1.ObjectMeta{Name: "live-test", Namespace: testNamespace},
		Spec: v1alpha1.DashboardPermissionSpec{
			ForProvider: v1alpha1.DashboardPermissionParameters{
				Permissions: []v1alpha1.PermissionItem{{
					Permission: "Edit",
					TeamRef:    &xpv1.Reference{Name: "platform-team"},
				}},
			},
		},
	}

	return &external{kube: kube, orgID: liveOrgID}, cr
}

// A teamRef must resolve to the real team ID, otherwise the permission would
// be granted to whatever team happens to have that ID - or to nobody.
func TestResolvePermissions_TeamRefResolvesToTeamID(t *testing.T) {
	e, cr := resolveWith(t, teamResource(t, "platform-team", 7))

	items, err := e.resolvePermissions(t.Context(), cr)
	if err != nil {
		t.Fatalf("resolvePermissions() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("resolvePermissions() returned %d items, want 1", len(items))
	}
	if items[0].TeamID == nil || *items[0].TeamID != 7 {
		t.Errorf("resolved TeamID = %v, want 7", items[0].TeamID)
	}
	if items[0].TeamRef != nil || items[0].TeamSelector != nil {
		t.Error("resolved item still carries the teamRef/teamSelector")
	}
	// The spec must not be mutated - it is the user's declared state.
	if cr.Spec.ForProvider.Permissions[0].TeamID != nil {
		t.Error("resolvePermissions() mutated the spec")
	}
}

func TestResolvePermissions_TeamSelectorResolves(t *testing.T) {
	tm := teamResource(t, "platform-team", 9)
	tm.Labels = map[string]string{"team": "platform"}

	kube := fake.NewClientBuilder().WithScheme(schemeWithTeams).WithRuntimeObjects(tm).Build()
	e := &external{kube: kube, orgID: liveOrgID}

	cr := &v1alpha1.DashboardPermission{
		ObjectMeta: metav1.ObjectMeta{Name: "live-test", Namespace: testNamespace},
		Spec: v1alpha1.DashboardPermissionSpec{ForProvider: v1alpha1.DashboardPermissionParameters{
			Permissions: []v1alpha1.PermissionItem{{
				Permission:   "View",
				TeamSelector: &xpv1.Selector{MatchLabels: map[string]string{"team": "platform"}},
			}},
		}},
	}

	items, err := e.resolvePermissions(t.Context(), cr)
	if err != nil {
		t.Fatalf("resolvePermissions() error = %v", err)
	}
	if items[0].TeamID == nil || *items[0].TeamID != 9 {
		t.Errorf("resolved TeamID = %v, want 9", items[0].TeamID)
	}
}

// An explicit teamId must win over a reference, matching how dashboardUid and
// orgId behave elsewhere in the provider.
func TestResolvePermissions_ExplicitTeamIDWins(t *testing.T) {
	e, cr := resolveWith(t, teamResource(t, "platform-team", 7))

	cr.Spec.ForProvider.Permissions[0].TeamID = ptr(int64(3))

	items, err := e.resolvePermissions(t.Context(), cr)
	if err != nil {
		t.Fatalf("resolvePermissions() error = %v", err)
	}
	if items[0].TeamID == nil || *items[0].TeamID != 3 {
		t.Errorf("resolved TeamID = %v, want 3 (explicit teamId wins)", items[0].TeamID)
	}
}

// Items without a team (role- or user-based) must pass through untouched.
func TestResolvePermissions_LeavesNonTeamItemsAlone(t *testing.T) {
	e, cr := resolveWith(t)

	cr.Spec.ForProvider.Permissions = []v1alpha1.PermissionItem{
		{Permission: "Admin", Role: ptr("Admin")},
		{Permission: "Edit", UserID: ptr(int64(5))},
	}

	items, err := e.resolvePermissions(t.Context(), cr)
	if err != nil {
		t.Fatalf("resolvePermissions() error = %v", err)
	}
	if items[0].TeamID != nil {
		t.Errorf("role item gained a TeamID: %v", *items[0].TeamID)
	}
	if items[1].TeamID != nil {
		t.Errorf("user item gained a TeamID: %v", *items[1].TeamID)
	}
}

// An unresolvable teamRef must be an error. Silently dropping the team would
// make Observe see drift and Update then strip the team's permission.
func TestResolvePermissions_UnresolvableRefIsAnError(t *testing.T) {
	e, _ := resolveWith(t) // no Teams exist

	cr := &v1alpha1.DashboardPermission{
		ObjectMeta: metav1.ObjectMeta{Name: "live-test", Namespace: testNamespace},
		Spec: v1alpha1.DashboardPermissionSpec{ForProvider: v1alpha1.DashboardPermissionParameters{
			Permissions: []v1alpha1.PermissionItem{{
				Permission: "Edit",
				TeamRef:    &xpv1.Reference{Name: "missing-team"},
			}},
		}},
	}

	if _, err := e.resolvePermissions(t.Context(), cr); err == nil {
		t.Fatal("resolvePermissions() error = nil, want an error for a missing Team")
	}
}

// A team that hasn't been created yet has no ID, which must not be turned into
// team 0.
func TestResolvePermissions_TeamNotYetCreated(t *testing.T) {
	uncreated := &v1alpha1.Team{
		ObjectMeta: metav1.ObjectMeta{Name: "platform-team", Namespace: "default"},
		Spec:       v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{Name: "platform"}},
	}
	e, cr := resolveWith(t, uncreated)

	if _, err := e.resolvePermissions(t.Context(), cr); err == nil {
		t.Fatal("resolvePermissions() error = nil, want an error for an uncreated Team")
	}
}

// End-to-end against a real Grafana: the resolved team ID must actually be the
// team Grafana reports back on the dashboard's permissions.
func TestLiveDashboardPermission_TeamRefGrantsTeamAccess(t *testing.T) {
	c := grafanatest.Client(t)
	name := grafanatest.UniqueName("team-ref-perms")
	teamID := grafanatest.CreateTeam(t, c, name)

	folderUID := grafanatest.CreateFolder(t, c, name+"-folder")
	dashUID := grafanatest.CreateDashboard(t, c, name+"-dash", folderUID)

	kube := fake.NewClientBuilder().
		WithScheme(schemeWithTeams).
		WithRuntimeObjects(teamResource(t, "platform-team", teamID)).
		Build()
	e := &external{client: c, kube: kube, orgID: liveOrgID, dashboardUID: dashUID}

	cr := &v1alpha1.DashboardPermission{
		ObjectMeta: metav1.ObjectMeta{Name: "live-test", Namespace: testNamespace},
		Spec: v1alpha1.DashboardPermissionSpec{ForProvider: v1alpha1.DashboardPermissionParameters{
			DashboardUID: &dashUID,
			Permissions: []v1alpha1.PermissionItem{{
				Permission: "Edit",
				TeamRef:    &xpv1.Reference{Name: "platform-team"},
			}},
		}},
	}

	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Grafana must report the grant against the referenced team.
	observed, err := c.GetDashboardPermissions(t.Context(), dashUID)
	if err != nil {
		t.Fatalf("GetDashboardPermissions() error = %v", err)
	}
	found := false
	for _, p := range observed {
		if p.TeamID == teamID {
			found = true
			if p.Permission != 2 {
				t.Errorf("permission = %d, want 2 (Edit)", p.Permission)
			}
		}
	}
	if !found {
		t.Fatalf("no permission found for team %d; Grafana reported %+v", teamID, observed)
	}

	if exists, upToDate := observePerm(t, e, cr); !exists || !upToDate {
		t.Errorf("Observe() = (exists=%v, upToDate=%v), want (true, true)", exists, upToDate)
	}

	if _, err := e.Delete(t.Context(), cr); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func observePerm(t *testing.T, e *external, cr *v1alpha1.DashboardPermission) (exists, upToDate bool) {
	t.Helper()
	obs, err := e.Observe(t.Context(), cr)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	return obs.ResourceExists, obs.ResourceUpToDate
}

func ptr[T any](v T) *T { return &v }
