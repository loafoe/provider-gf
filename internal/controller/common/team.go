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
	"strconv"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reference"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
)

// ExtractTeamID extracts the numeric team ID from a Team resource.
//
// This lives here rather than in the team controller package because the team
// controller already imports this package to resolve org references, so it
// cannot be imported back.
func ExtractTeamID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		tm, ok := mg.(*v1alpha1.Team)
		if !ok {
			return ""
		}
		if tm.Status.AtProvider.ID != nil {
			return strconv.FormatInt(*tm.Status.AtProvider.ID, 10)
		}
		// Fall back to the external name so a Team that hasn't been observed
		// yet can still be referenced.
		parts := splitExternalName(meta.GetExternalName(tm))
		if len(parts) != 2 || parts[1] == "" {
			return ""
		}
		if _, err := strconv.ParseInt(parts[1], 10, 64); err != nil {
			return ""
		}
		return parts[1]
	}
}

// ResolveTeamID resolves a Team reference or selector to a numeric team ID.
func ResolveTeamID(ctx context.Context, kube client.Client, mg resource.Managed, ref *xpv1.Reference, selector *xpv1.Selector) (int64, error) {
	if ref == nil && selector == nil {
		return 0, errors.New("no team reference or selector supplied")
	}

	rsp, err := reference.NewAPIResolver(kube, mg).Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: "",
		Reference:    ref,
		Selector:     selector,
		To:           reference.To{Managed: &v1alpha1.Team{}, List: &v1alpha1.TeamList{}},
		Extract:      ExtractTeamID(),
		Namespace:    mg.(resource.ModernManaged).GetNamespace(),
	})
	if err != nil {
		return 0, errors.Wrap(err, "cannot resolve team reference")
	}
	if rsp.ResolvedValue == "" {
		return 0, errors.New("cannot resolve team reference: no Team matched")
	}

	teamID, err := strconv.ParseInt(rsp.ResolvedValue, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "cannot parse resolved team ID")
	}
	if teamID == 0 {
		return 0, errors.New("resolved team ID is 0: the Team has not been created yet")
	}

	return teamID, nil
}

// splitExternalName splits an <orgId>:<id> external name.
func splitExternalName(externalName string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(externalName); i++ {
		if externalName[i] == ':' {
			parts = append(parts, externalName[start:i])
			start = i + 1
		}
	}
	return append(parts, externalName[start:])
}
