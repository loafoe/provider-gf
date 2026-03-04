/*
Copyright 2020 The Crossplane Authors.

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

package controller

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/provider-gf/internal/controller/config"
	"github.com/crossplane/provider-gf/internal/controller/contactpoint"
	"github.com/crossplane/provider-gf/internal/controller/dashboard"
	"github.com/crossplane/provider-gf/internal/controller/dashboardpermission"
	"github.com/crossplane/provider-gf/internal/controller/datasource"
	"github.com/crossplane/provider-gf/internal/controller/folder"
	"github.com/crossplane/provider-gf/internal/controller/folderpermission"
	"github.com/crossplane/provider-gf/internal/controller/librarypanel"
	"github.com/crossplane/provider-gf/internal/controller/organization"
	"github.com/crossplane/provider-gf/internal/controller/rulegroup"
)

// Setup creates all Grafana controllers and adds them to the supplied manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		config.Setup,
		contactpoint.Setup,
		dashboard.Setup,
		dashboardpermission.Setup,
		datasource.Setup,
		folder.Setup,
		folderpermission.Setup,
		librarypanel.Setup,
		organization.Setup,
		rulegroup.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
