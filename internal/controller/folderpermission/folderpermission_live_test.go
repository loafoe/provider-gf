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

package folderpermission

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

const liveOrgID = int64(1)

var schemeWithTeams = runtime.NewScheme()

func init() {
	_ = v1alpha1.SchemeBuilder.AddToScheme(schemeWithTeams)
}

func teamResource(name string, teamID int64) *v1alpha1.Team {
	return &v1alpha1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Annotations: map[string]string{
				"crossplane.io/external-name": strconv.FormatInt(liveOrgID, 10) + ":" + strconv.FormatInt(teamID, 10),
			},
		},
		Spec:   v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{Name: name}},
		Status: v1alpha1.TeamStatus{AtProvider: v1alpha1.TeamObservation{ID: &teamID}},
	}
}

func TestResolvePermissions_TeamRefResolvesToTeamID(t *testing.T) {
	kube := fake.NewClientBuilder().
		WithScheme(schemeWithTeams).
		WithRuntimeObjects(teamResource("platform-team", 7)).
		Build()
	e := &external{kube: kube, orgID: liveOrgID}

	cr := &v1alpha1.FolderPermission{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha1.FolderPermissionSpec{ForProvider: v1alpha1.FolderPermissionParameters{
			Permissions: []v1alpha1.FolderPermissionItem{{
				Permission: "View",
				TeamRef:    &xpv1.Reference{Name: "platform-team"},
			}},
		}},
	}

	items, err := e.resolvePermissions(t.Context(), cr)
	if err != nil {
		t.Fatalf("resolvePermissions() error = %v", err)
	}
	if items[0].TeamID == nil || *items[0].TeamID != 7 {
		t.Errorf("resolved TeamID = %v, want 7", items[0].TeamID)
	}
	if cr.Spec.ForProvider.Permissions[0].TeamID != nil {
		t.Error("resolvePermissions() mutated the spec")
	}
}

func TestResolvePermissions_UnresolvableRefIsAnError(t *testing.T) {
	e := &external{kube: fake.NewClientBuilder().WithScheme(schemeWithTeams).Build(), orgID: liveOrgID}

	cr := &v1alpha1.FolderPermission{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha1.FolderPermissionSpec{ForProvider: v1alpha1.FolderPermissionParameters{
			Permissions: []v1alpha1.FolderPermissionItem{{
				Permission: "View",
				TeamRef:    &xpv1.Reference{Name: "missing-team"},
			}},
		}},
	}

	if _, err := e.resolvePermissions(t.Context(), cr); err == nil {
		t.Fatal("resolvePermissions() error = nil, want an error for a missing Team")
	}
}

// End-to-end against a real Grafana: the resolved team ID must be the team
// Grafana reports back on the folder's permissions.
func TestLiveFolderPermission_TeamRefGrantsTeamAccess(t *testing.T) {
	c := grafanatest.Client(t)
	name := grafanatest.UniqueName("folder-team-ref")
	teamID := grafanatest.CreateTeam(t, c, name)
	folderUID := grafanatest.CreateFolder(t, c, name+"-folder")

	kube := fake.NewClientBuilder().
		WithScheme(schemeWithTeams).
		WithRuntimeObjects(teamResource("platform-team", teamID)).
		Build()
	e := &external{client: c, kube: kube, orgID: liveOrgID, folderUID: folderUID}

	cr := &v1alpha1.FolderPermission{
		ObjectMeta: metav1.ObjectMeta{Name: "live-test", Namespace: "default"},
		Spec: v1alpha1.FolderPermissionSpec{ForProvider: v1alpha1.FolderPermissionParameters{
			FolderUID: &folderUID,
			Permissions: []v1alpha1.FolderPermissionItem{{
				Permission: "Edit",
				TeamRef:    &xpv1.Reference{Name: "platform-team"},
			}},
		}},
	}

	if _, err := e.Create(t.Context(), cr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	observed, err := c.GetFolderPermissions(t.Context(), folderUID)
	if err != nil {
		t.Fatalf("GetFolderPermissions() error = %v", err)
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

	obs, err := e.Observe(t.Context(), cr)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if !obs.ResourceExists || !obs.ResourceUpToDate {
		t.Errorf("Observe() = (exists=%v, upToDate=%v), want (true, true)", obs.ResourceExists, obs.ResourceUpToDate)
	}

	// Clear the permissions so the folder can be cleaned up.
	if _, err := e.Delete(t.Context(), cr); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}
