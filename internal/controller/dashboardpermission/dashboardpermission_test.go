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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
)

func dashboardWithExternalName(externalName string) *v1alpha1.Dashboard {
	d := &v1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "test-dashboard"}}
	if externalName != "" {
		d.Annotations = map[string]string{"crossplane.io/external-name": externalName}
	}
	return d
}

// Regression test: a DashboardPermission's dashboardRef/dashboardSelector
// resolves through ExtractDashboardUID. Before the fix, a referenced
// Dashboard created under a stable (GA) API version - "v1" or "v2", with no
// alpha/beta suffix - had its external name misclassified as the legacy
// format, so this returned the garbage string "v2:<name>" (or "v1:<name>")
// instead of the real dashboard name. That would make the permission target
// a nonexistent dashboard.
func TestExtractDashboardUID(t *testing.T) {
	const wantName = "my-dashboard"

	cases := []struct {
		name         string
		externalName string
		want         string
	}{
		{"legacy v1 uid", "1:abc123XYZ", "abc123XYZ"},
		{"stable v1", "1:v1:" + wantName, wantName},
		{"stable v2", "1:v2:" + wantName, wantName},
		{"v1beta1", "1:v1beta1:" + wantName, wantName},
		{"v2beta1", "1:v2beta1:" + wantName, wantName},
		{"v2alpha1", "1:v2alpha1:" + wantName, wantName},
		{"no external name", "", ""},
	}

	extract := ExtractDashboardUID()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extract(dashboardWithExternalName(tc.externalName))
			if got != tc.want {
				t.Errorf("ExtractDashboardUID()(%q) = %q, want %q", tc.externalName, got, tc.want)
			}
		})
	}
}
