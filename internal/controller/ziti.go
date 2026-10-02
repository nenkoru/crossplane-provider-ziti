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

	"github.com/crossplane/provider-ziti/internal/connector"
	"github.com/crossplane/provider-ziti/internal/controller/authpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/certificateauthority"
	"github.com/crossplane/provider-ziti/internal/controller/config"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv1"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv2"
	"github.com/crossplane/provider-ziti/internal/controller/configinterceptv1"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouter"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/externaljwtsigner"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/identityca"
	"github.com/crossplane/provider-ziti/internal/controller/identitynone"
	"github.com/crossplane/provider-ziti/internal/controller/identityupdb"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckdomain"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmac"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmfa"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmultiprocess"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckos"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckprocess"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/serviceedgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

// SetupGated creates all Ziti controllers with safe-start support and adds them to
// the supplied manager.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	if err := config.Setup(mgr, o); err != nil {
		return err
	}

	// All controllers share the connector, and with it the Ziti API sessions.
	clients := connector.New(mgr.GetClient())

	for _, setup := range []func() error{
		func() error { return generic.SetupGated(mgr, o, clients, service.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, confighostv1.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, confighostv2.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, configinterceptv1.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, servicepolicy.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, serviceedgerouterpolicy.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, edgerouterpolicy.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, identity.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, identityca.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, identityupdb.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, identitynone.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, edgerouter.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, posturecheckos.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, posturecheckmfa.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, authpolicy.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, posturecheckdomain.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, posturecheckmac.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, posturecheckprocess.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, posturecheckmultiprocess.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, certificateauthority.Kind) },
		func() error { return generic.SetupGated(mgr, o, clients, externaljwtsigner.Kind) },
	} {
		if err := setup(); err != nil {
			return err
		}
	}
	return nil
}
