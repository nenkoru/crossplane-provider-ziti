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

	"github.com/crossplane/provider-ziti/internal/controller/authpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/config"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv1"
	"github.com/crossplane/provider-ziti/internal/controller/configinterceptv1"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouter"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmfa"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckos"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/serviceedgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

// SetupGated creates all Ziti controllers with safe-start support and adds them to
// the supplied manager.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		config.Setup,
		service.Setup,
		edgerouter.Setup,
		confighostv1.Setup,
		configinterceptv1.Setup,
		servicepolicy.Setup,
		serviceedgerouterpolicy.Setup,
		edgerouterpolicy.Setup,
		identity.Setup,
		posturecheckos.Setup,
		posturecheckmfa.Setup,
		authpolicy.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
