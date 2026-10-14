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

package common

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
)

// TestResolveOrgID_PinsToCurrentValueWhenSelectorMatchesDifferentOrg reproduces
// the datasource-replication bug: a Selector that matches multiple
// Organizations (e.g. a shared label) must not be allowed to silently
// re-target an already-resolved resource to a newly created Organization
// that also matches. Once a resource has resolved to an org, that
// resolution must be pinned unless explicitly cleared.
func TestResolveOrgID_PinsToCurrentValueWhenSelectorMatchesDifferentOrg(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	newOrgID := int64(99)
	newOrg := &v1alpha1.Organization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-org",
			Namespace: "default",
			Labels:    map[string]string{"tier": "shared"},
		},
		Status: v1alpha1.OrganizationStatus{
			AtProvider: v1alpha1.OrganizationObservation{ID: &newOrgID},
		},
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(newOrg).Build()

	ds := &v1alpha1.DataSource{
		ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "default"},
	}

	orgID, err := ResolveOrgID(context.Background(), kube, ds,
		nil, // orgRef
		&xpv1.Selector{MatchLabels: map[string]string{"tier": "shared"}},
		nil, // directOrgID
		nil, // providerConfigOrgID
		"5", // currentOrgID: this resource was already created in org 5
	)
	if err != nil {
		t.Fatalf("ResolveOrgID() error = %v", err)
	}
	if orgID != 5 {
		t.Errorf("ResolveOrgID() = %d, want 5 (pinned to already-resolved org; must not drift to newly matched org %d)", orgID, newOrgID)
	}
}

// TestResolveOrgID_ResolvesViaSelectorWhenNotYetCreated ensures a brand-new
// resource (no currentOrgID yet) still resolves normally via the Selector.
func TestResolveOrgID_ResolvesViaSelectorWhenNotYetCreated(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	orgIDVal := int64(7)
	org := &v1alpha1.Organization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "org",
			Namespace: "default",
			Labels:    map[string]string{"tier": "shared"},
		},
		Status: v1alpha1.OrganizationStatus{
			AtProvider: v1alpha1.OrganizationObservation{ID: &orgIDVal},
		},
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(org).Build()

	ds := &v1alpha1.DataSource{
		ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "default"},
	}

	orgID, err := ResolveOrgID(context.Background(), kube, ds,
		nil,
		&xpv1.Selector{MatchLabels: map[string]string{"tier": "shared"}},
		nil,
		nil,
		"", // not yet created - no pinned value
	)
	if err != nil {
		t.Fatalf("ResolveOrgID() error = %v", err)
	}
	if orgID != 7 {
		t.Errorf("ResolveOrgID() = %d, want 7", orgID)
	}
}

func TestExtractTeamID(t *testing.T) {
	id := int64(7)
	withStatus := &v1alpha1.Team{Status: v1alpha1.TeamStatus{AtProvider: v1alpha1.TeamObservation{ID: &id}}}
	if got := ExtractTeamID()(withStatus); got != "7" {
		t.Errorf("ExtractTeamID() from status = %q, want %q", got, "7")
	}

	// A Team that hasn't been observed yet still has an external name.
	withExternalName := &v1alpha1.Team{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		"crossplane.io/external-name": "42:7",
	}}}
	if got := ExtractTeamID()(withExternalName); got != "7" {
		t.Errorf("ExtractTeamID() from external-name = %q, want %q", got, "7")
	}

	if got := ExtractTeamID()(&v1alpha1.Team{}); got != "" {
		t.Errorf("ExtractTeamID() unresolved = %q, want empty", got)
	}
	if got := ExtractTeamID()(&v1alpha1.Folder{}); got != "" {
		t.Errorf("ExtractTeamID() wrong type = %q, want empty", got)
	}
	// A non-numeric team segment must not be handed on as an ID.
	nonNumeric := &v1alpha1.Team{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		"crossplane.io/external-name": "42:team",
	}}}
	if got := ExtractTeamID()(nonNumeric); got != "" {
		t.Errorf("ExtractTeamID() non-numeric = %q, want empty", got)
	}
}

// ResolveTeamID is only ever called when a ref or selector is present, so a
// bare call is a programming error rather than a user-facing one.
func TestResolveTeamID_RequiresReference(t *testing.T) {
	if _, err := ResolveTeamID(t.Context(), nil, &v1alpha1.Team{}, nil, nil); err == nil {
		t.Error("ResolveTeamID() error = nil, want an error when neither ref nor selector is set")
	}
}
