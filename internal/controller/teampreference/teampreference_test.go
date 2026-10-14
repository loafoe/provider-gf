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

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
)

func str(s string) *string { return &s }

func TestIsUpToDate(t *testing.T) {
	cases := map[string]struct {
		params   v1alpha1.TeamPreferenceParameters
		prefs    *grafana.TeamPreferences
		wantUpTo bool
	}{
		"all unset matches defaults": {
			params:   v1alpha1.TeamPreferenceParameters{},
			prefs:    &grafana.TeamPreferences{Theme: "", Timezone: ""},
			wantUpTo: true,
		},
		"theme matches": {
			params:   v1alpha1.TeamPreferenceParameters{Theme: str("dark")},
			prefs:    &grafana.TeamPreferences{Theme: "dark"},
			wantUpTo: true,
		},
		"theme differs": {
			params:   v1alpha1.TeamPreferenceParameters{Theme: str("dark")},
			prefs:    &grafana.TeamPreferences{Theme: "light"},
			wantUpTo: false,
		},
		// Grafana's update endpoint replaces the whole object, so an unset
		// field is reconciled to the default - an out-of-band change to a
		// field the spec doesn't mention is still drift.
		"unset theme vs observed theme": {
			params:   v1alpha1.TeamPreferenceParameters{},
			prefs:    &grafana.TeamPreferences{Theme: "dark"},
			wantUpTo: false,
		},
		"timezone differs": {
			params:   v1alpha1.TeamPreferenceParameters{Timezone: str("utc")},
			prefs:    &grafana.TeamPreferences{Timezone: "browser"},
			wantUpTo: false,
		},
		"home dashboard matches": {
			params:   v1alpha1.TeamPreferenceParameters{HomeDashboardUID: str("abc")},
			prefs:    &grafana.TeamPreferences{HomeDashboardUID: "abc"},
			wantUpTo: true,
		},
		"home dashboard differs": {
			params:   v1alpha1.TeamPreferenceParameters{HomeDashboardUID: str("abc")},
			prefs:    &grafana.TeamPreferences{HomeDashboardUID: "xyz"},
			wantUpTo: false,
		},
		// An unset home dashboard is reconciled to "" (Grafana's default),
		// so an observed dashboard is drift.
		"unset home dashboard vs observed dashboard": {
			params:   v1alpha1.TeamPreferenceParameters{},
			prefs:    &grafana.TeamPreferences{HomeDashboardUID: "abc"},
			wantUpTo: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isUpToDate(tc.params, tc.prefs); got != tc.wantUpTo {
				t.Errorf("isUpToDate() = %v, want %v", got, tc.wantUpTo)
			}
		})
	}
}

// Grafana resets omitted keys to system defaults on update, so unset fields
// must be sent as their default rather than left off the request.
func TestDesiredPreferences_FillsDefaults(t *testing.T) {
	got := desiredPreferences(v1alpha1.TeamPreferenceParameters{Theme: str("dark")})

	want := grafana.TeamPreferences{Theme: "dark", Timezone: "", HomeDashboardUID: ""}
	if got != want {
		t.Errorf("desiredPreferences() = %+v, want %+v", got, want)
	}
}
