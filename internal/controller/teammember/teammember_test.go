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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
)

func withExternalName(cr *v1alpha1.TeamMember, externalName string) *v1alpha1.TeamMember {
	cr.ObjectMeta = metav1.ObjectMeta{Annotations: map[string]string{"crossplane.io/external-name": externalName}}
	return cr
}

func newTeamMember(params v1alpha1.TeamMemberParameters) *v1alpha1.TeamMember {
	return &v1alpha1.TeamMember{Spec: v1alpha1.TeamMemberSpec{ForProvider: params}}
}

func TestExternalNameRoundTrip(t *testing.T) {
	orgID, teamID, userID, err := parseExternalName(formatExternalName(42, 7, 3))
	if err != nil {
		t.Fatalf("parseExternalName() error = %v", err)
	}
	if orgID != 42 || teamID != 7 || userID != 3 {
		t.Errorf("parseExternalName() = (%d, %d, %d), want (42, 7, 3)", orgID, teamID, userID)
	}
}

func TestParseExternalName_Invalid(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"missing user id":  "42:7:",
		"missing team id":  "42::3",
		"too few parts":    "42:7",
		"non numeric user": "42:7:user",
		"non numeric org":  "org:7:3",
		"trailing garbage": "42:7:3:4:5",
	}

	for name, externalName := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := parseExternalName(externalName); err == nil {
				t.Errorf("parseExternalName(%q) error = nil, want error", externalName)
			}
		})
	}
}

// The external name is the only record of which user a membership was
// created for, so a spec.userId change must be tracked through it.
func TestLookupUserID_PrefersSpecUserID(t *testing.T) {
	e := &external{}
	userID := int64(9)

	cr := withExternalName(newTeamMember(v1alpha1.TeamMemberParameters{UserID: &userID}), "1:7:3")
	got, ok := e.lookupUserID(t.Context(), cr)
	if !ok || got != 9 {
		t.Errorf("lookupUserID() = (%d, %v), want (9, true)", got, ok)
	}
}

func TestLookupUserID_FallsBackToExternalName(t *testing.T) {
	e := &external{}

	// No spec.userId - the external name is the only thing that identifies
	// the member we created.
	cr := withExternalName(newTeamMember(v1alpha1.TeamMemberParameters{}), "1:7:3")
	got, ok := e.lookupUserID(t.Context(), cr)
	if !ok || got != 3 {
		t.Errorf("lookupUserID() = (%d, %v), want (3, true)", got, ok)
	}
}

func TestLookupUserID_Unidentifiable(t *testing.T) {
	e := &external{}

	if _, ok := e.lookupUserID(t.Context(), newTeamMember(v1alpha1.TeamMemberParameters{})); ok {
		t.Error("lookupUserID() ok = true, want false when neither spec nor external name identifies a user")
	}

	cr := withExternalName(newTeamMember(v1alpha1.TeamMemberParameters{}), "not-a-name")
	if _, ok := e.lookupUserID(t.Context(), cr); ok {
		t.Error("lookupUserID() ok = true, want false for an unparseable external name")
	}
}

func TestResolveUserID_RequiresAUserReference(t *testing.T) {
	e := &external{}

	if _, err := e.resolveUserID(t.Context(), newTeamMember(v1alpha1.TeamMemberParameters{})); err == nil {
		t.Error("resolveUserID() error = nil, want error when no user is specified")
	}
}
