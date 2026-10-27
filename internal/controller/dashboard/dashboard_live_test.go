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

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
	"github.com/crossplane/provider-gf/internal/clients/grafana"
	"github.com/crossplane/provider-gf/internal/clients/grafana/grafanatest"
)

const liveOrgID = int64(1)

// v2DashboardJSON builds the minimal K8s-style V2 dashboard configJson for a
// given apiVersion (e.g. "v2beta1", or the stable "v1"/"v2" that triggered
// the bug this test guards against).
//
// The v0alpha1/v1beta1/v1 versions use the traditional spec.panels schema;
// Grafana rejects the newer spec.elements/layout schema on those versions
// with "dashboard appears to be in v2 format", so the spec shape must match
// the API version under test, not just the apiVersion string.
func v2DashboardJSON(t *testing.T, apiVersion, name string) string {
	t.Helper()

	var spec map[string]any
	switch apiVersion {
	case "v0alpha1", "v1beta1", "v1":
		spec = map[string]any{
			"title":         name,
			"schemaVersion": 39,
			"panels":        []any{},
		}
	default:
		spec = map[string]any{
			"title":         name,
			"schemaVersion": 39,
			"elements":      map[string]any{},
			"layout": map[string]any{
				"kind": "GridLayout",
				"spec": map[string]any{"items": []any{}},
			},
		}
	}

	dash := grafana.DashboardV2{
		APIVersion: "dashboard.grafana.app/" + apiVersion,
		Kind:       "Dashboard",
		Metadata: grafana.DashboardV2Metadata{
			Name: name,
		},
		Spec: spec,
	}

	b, err := json.Marshal(dash)
	if err != nil {
		t.Fatalf("marshal v2 dashboard JSON: %v", err)
	}
	return string(b)
}

// Regression test for a stuck-finalizer bug: Dashboard CRs backed by a
// Grafana dashboard created under a stable (GA) API version - "v1" or "v2",
// with no alpha/beta suffix - had their external name misclassified as the
// legacy format. Observe and Delete then operated on
// /api/dashboards/uid/<orgId>:v2:<name> (the literal string after the first
// colon, used as a "UID"), which always 404s. Grafana never actually
// deleted the dashboard and the managed resource's finalizer never cleared.
//
// This exercises the real controller's Create -> Observe -> Delete -> Observe
// cycle against a live Grafana to prove the dashboard is both correctly
// classified as V2 and actually removed, end to end.
func TestLiveDashboardV2_StableAPIVersionRoundTrip(t *testing.T) {
	for _, apiVersion := range []string{"v2", "v1", "v2beta1", "v1beta1"} {
		t.Run(apiVersion, func(t *testing.T) {
			c := grafanatest.Client(t)
			name := grafanatest.UniqueName("v2-" + apiVersion)
			configJSON := v2DashboardJSON(t, apiVersion, name)

			// Belt-and-suspenders cleanup: if an assertion below fails the
			// test before e.Delete() runs, don't leak the dashboard.
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_ = c.DeleteDashboardV2ByName(ctx, apiVersion, "default", name)
			})

			cr := &v1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Spec: v1alpha1.DashboardSpec{
					ForProvider: v1alpha1.DashboardParameters{
						ConfigJSON: configJSON,
					},
				},
			}

			e := &external{client: c, orgID: liveOrgID}

			if _, err := e.Create(t.Context(), cr); err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			externalName := meta.GetExternalName(cr)
			if !isV2ExternalName(externalName) {
				t.Fatalf("external name %q was not classified as V2 after Create() - this is the bug under test", externalName)
			}
			_, gotAPIVersion, _, err := parseExternalNameV2(externalName)
			if err != nil {
				t.Fatalf("parseExternalNameV2(%q) error = %v", externalName, err)
			}
			if gotAPIVersion != apiVersion {
				t.Fatalf("apiVersion in external name = %q, want %q", gotAPIVersion, apiVersion)
			}

			// Observe must find it via the V2 path, not a bogus V1 lookup.
			obs, err := e.Observe(t.Context(), cr)
			if err != nil {
				t.Fatalf("Observe() error = %v", err)
			}
			if !obs.ResourceExists {
				t.Fatal("Observe() ResourceExists = false, want true right after Create()")
			}

			// Delete must actually remove the dashboard, not loop forever.
			if _, err := e.Delete(t.Context(), cr); err != nil {
				t.Fatalf("Delete() error = %v", err)
			}

			obs, err = e.Observe(t.Context(), cr)
			if err != nil {
				t.Fatalf("Observe() after Delete() error = %v", err)
			}
			if obs.ResourceExists {
				t.Fatal("Observe() after Delete() ResourceExists = true, want false - dashboard was not actually deleted (the stuck-finalizer bug)")
			}
		})
	}
}
