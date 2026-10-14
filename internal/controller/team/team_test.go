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

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
)

func TestExternalNameRoundTrip(t *testing.T) {
	orgID, teamID, err := ParseExternalName(FormatExternalName(42, 7))
	if err != nil {
		t.Fatalf("ParseExternalName() error = %v", err)
	}
	if orgID != 42 || teamID != 7 {
		t.Errorf("ParseExternalName() = (%d, %d), want (42, 7)", orgID, teamID)
	}
}

func TestParseExternalName_Invalid(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"missing team id":   "42:",
		"missing separator": "42",
		"non numeric org":   "org:7",
		"non numeric team":  "42:team",
	}

	for name, externalName := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ParseExternalName(externalName); err == nil {
				t.Errorf("ParseExternalName(%q) error = nil, want error", externalName)
			}
		})
	}
}

func newTeam(params v1alpha1.TeamParameters) *v1alpha1.Team {
	return &v1alpha1.Team{Spec: v1alpha1.TeamSpec{ForProvider: params}}
}

func TestIsUpToDate(t *testing.T) {
	const (
		teamName = "platform"
		emailA   = "a@example.com"
	)
	str := func(s string) *string { return &s }

	cases := map[string]struct {
		params   v1alpha1.TeamParameters
		team     *grafana.Team
		members  []grafana.TeamMember
		wantUpTo bool
	}{
		"name matches": {
			params:   v1alpha1.TeamParameters{Name: teamName},
			team:     &grafana.Team{ID: 7, Name: teamName},
			wantUpTo: true,
		},
		"name differs": {
			params:   v1alpha1.TeamParameters{Name: teamName},
			team:     &grafana.Team{ID: 7, Name: "platform-eng"},
			wantUpTo: false,
		},
		"email matches": {
			params:   v1alpha1.TeamParameters{Name: teamName, Email: str("team@example.com")},
			team:     &grafana.Team{ID: 7, Name: teamName, Email: "team@example.com"},
			wantUpTo: true,
		},
		"email differs": {
			params:   v1alpha1.TeamParameters{Name: teamName, Email: str("new@example.com")},
			team:     &grafana.Team{ID: 7, Name: teamName, Email: "old@example.com"},
			wantUpTo: false,
		},
		"unset email matches empty observed": {
			params:   v1alpha1.TeamParameters{Name: teamName},
			team:     &grafana.Team{ID: 7, Name: teamName, Email: ""},
			wantUpTo: true,
		},
		// Membership is only owned when members or admins are set, so an
		// unlisted user must not trigger an update.
		"membership unmanaged ignores drift": {
			params:   v1alpha1.TeamParameters{Name: teamName},
			team:     &grafana.Team{ID: 7, Name: teamName},
			members:  []grafana.TeamMember{{UserID: 1, Email: emailA}},
			wantUpTo: true,
		},
		"members match": {
			params:   v1alpha1.TeamParameters{Name: teamName, Members: []string{emailA}},
			team:     &grafana.Team{ID: 7, Name: teamName},
			members:  []grafana.TeamMember{{UserID: 1, Email: emailA}},
			wantUpTo: true,
		},
		"members match ignoring case": {
			params:   v1alpha1.TeamParameters{Name: teamName, Members: []string{"A@Example.com"}},
			team:     &grafana.Team{ID: 7, Name: teamName},
			members:  []grafana.TeamMember{{UserID: 1, Email: emailA}},
			wantUpTo: true,
		},
		"members include an admin": {
			params:   v1alpha1.TeamParameters{Name: teamName, Members: []string{emailA}, Admins: []string{"b@example.com"}},
			team:     &grafana.Team{ID: 7, Name: teamName},
			members:  []grafana.TeamMember{{UserID: 1, Email: emailA}, {UserID: 2, Email: "b@example.com"}},
			wantUpTo: true,
		},
		"member missing": {
			params:   v1alpha1.TeamParameters{Name: teamName, Members: []string{emailA}},
			team:     &grafana.Team{ID: 7, Name: teamName},
			members:  []grafana.TeamMember{{UserID: 2, Email: "b@example.com"}},
			wantUpTo: false,
		},
		// The membership set is authoritative, so extra users are drift too.
		"extra observed member": {
			params:   v1alpha1.TeamParameters{Name: teamName, Members: []string{emailA}},
			team:     &grafana.Team{ID: 7, Name: teamName},
			members:  []grafana.TeamMember{{UserID: 1, Email: emailA}, {UserID: 3, Email: "c@example.com"}},
			wantUpTo: false,
		},
		"desired members empty but admins set": {
			params:   v1alpha1.TeamParameters{Name: teamName, Admins: []string{emailA}},
			team:     &grafana.Team{ID: 7, Name: teamName},
			members:  []grafana.TeamMember{{UserID: 1, Email: emailA}},
			wantUpTo: true,
		},
	}

	e := &external{}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := e.isUpToDate(newTeam(tc.params), tc.team, tc.members)
			if got != tc.wantUpTo {
				t.Errorf("isUpToDate() = %v, want %v", got, tc.wantUpTo)
			}
		})
	}
}
