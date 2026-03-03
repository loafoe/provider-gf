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
	"github.com/crossplane/crossplane-runtime/v2/pkg/reference"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/provider-gf/apis/oss/v1alpha1"
)

// ExtractOrgID extracts the organization ID from an Organization resource.
// This is used for cross-resource reference resolution.
func ExtractOrgID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		org, ok := mg.(*v1alpha1.Organization)
		if !ok {
			return ""
		}
		if org.Status.AtProvider.ID == nil {
			return ""
		}
		return strconv.FormatInt(*org.Status.AtProvider.ID, 10)
	}
}

// ResolveOrgID resolves the organization ID from OrgRef/OrgSelector, direct OrgID, or ProviderConfig.
// Returns the resolved orgID (defaults to 1 if nothing is specified).
func ResolveOrgID(ctx context.Context, kube client.Client, mg resource.Managed, orgRef *xpv1.Reference, orgSelector *xpv1.Selector, directOrgID *int64, providerConfigOrgID *int64) (int64, error) {
	orgID := int64(1)

	// Try to resolve org reference first
	if orgRef != nil || orgSelector != nil {
		rsp, err := reference.NewAPIResolver(kube, mg).Resolve(ctx, reference.ResolutionRequest{
			CurrentValue: "",
			Reference:    orgRef,
			Selector:     orgSelector,
			To:           reference.To{Managed: &v1alpha1.Organization{}, List: &v1alpha1.OrganizationList{}},
			Extract:      ExtractOrgID(),
			Namespace:    mg.(resource.ModernManaged).GetNamespace(),
		})
		if err != nil {
			return 0, errors.Wrap(err, "cannot resolve organization reference")
		}
		if rsp.ResolvedValue != "" {
			resolvedOrgID, err := strconv.ParseInt(rsp.ResolvedValue, 10, 64)
			if err != nil {
				return 0, errors.Wrap(err, "cannot parse resolved org ID")
			}
			orgID = resolvedOrgID
		}
	} else if directOrgID != nil {
		orgID = *directOrgID
	} else if providerConfigOrgID != nil {
		orgID = *providerConfigOrgID
	}

	return orgID, nil
}
