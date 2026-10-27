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

package dashboard

import "testing"

// Regression test for a bug where external names using a stable (GA) Grafana
// dashboard API version - "v1" or "v2", with no alpha/beta suffix - were
// misclassified as the legacy V1 external-name format. That sent Observe and
// Delete down the legacy /api/dashboards/uid/... path with the literal string
// "v2:<name>" as the UID, which 404s. Grafana never actually deletes the
// dashboard, Observe keeps reporting it as gone from the wrong lookup, and the
// managed resource's finalizer never clears: the Dashboard CR gets stuck
// forever in "Deleting".
func TestIsV2ExternalName(t *testing.T) {
	cases := []struct {
		name         string
		externalName string
		want         bool
	}{
		// Stable/GA versions - the bug this test guards against.
		{"stable v1", "1:v1:my-dashboard", true},
		{"stable v2", "1:v2:my-dashboard", true},
		{"stable v10", "1:v10:my-dashboard", true},

		// Pre-release versions.
		{"v0alpha1", "1:v0alpha1:my-dashboard", true},
		{"v1beta1", "1:v1beta1:my-dashboard", true},
		{"v2alpha1", "1:v2alpha1:my-dashboard", true},
		{"v2beta1", "1:v2beta1:my-dashboard", true},

		// Legacy V1 format: <orgId>:<uid>, 2 parts.
		{"legacy uid, no colon", "1:abc123", false},
		{"legacy uid that happens to start with v", "1:version-123", false},

		// Malformed / edge cases.
		{"empty", "", false},
		{"only orgId", "1", false},
		{"missing name", "1:v2:", false},
		{"missing apiVersion", "1::my-dashboard", false},
		{"non-version middle part", "1:prod:my-dashboard", false},
		{"name itself contains a colon", "1:v2:my:dashboard", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isV2ExternalName(tc.externalName); got != tc.want {
				t.Errorf("isV2ExternalName(%q) = %v, want %v", tc.externalName, got, tc.want)
			}
		})
	}
}

func TestFormatParseExternalNameV2_RoundTrip(t *testing.T) {
	cases := []struct {
		name       string
		orgID      int64
		apiVersion string
		dashName   string
	}{
		{"stable v1", 1, "v1", "my-dashboard"},
		{"stable v2", 1, "v2", "my-dashboard"},
		{"v2beta1", 42, "v2beta1", "another-dashboard"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			formatted := formatExternalNameV2(tc.orgID, tc.apiVersion, tc.dashName)

			if !isV2ExternalName(formatted) {
				t.Fatalf("isV2ExternalName(%q) = false, want true (apiVersion %q must be classified as V2)", formatted, tc.apiVersion)
			}

			orgID, apiVersion, dashName, err := parseExternalNameV2(formatted)
			if err != nil {
				t.Fatalf("parseExternalNameV2(%q) error = %v", formatted, err)
			}
			if orgID != tc.orgID {
				t.Errorf("orgID = %d, want %d", orgID, tc.orgID)
			}
			if apiVersion != tc.apiVersion {
				t.Errorf("apiVersion = %q, want %q", apiVersion, tc.apiVersion)
			}
			if dashName != tc.dashName {
				t.Errorf("name = %q, want %q", dashName, tc.dashName)
			}
		})
	}
}

// A V1 (legacy) external name must never be parsed as if it were V2, even
// though parseExternalNameV2 would happily split it on a colon that doesn't
// exist in practice for real Grafana UIDs.
func TestIsV2ExternalName_DoesNotMisclassifyLegacyFormat(t *testing.T) {
	legacy := formatExternalName(1, "abc123XYZ")
	if isV2ExternalName(legacy) {
		t.Errorf("isV2ExternalName(%q) = true, want false (this is a legacy V1 external name)", legacy)
	}
}
