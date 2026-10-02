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

// Package identityca maps the IdentityCA managed resource to Ziti identities
// that enroll with a certificate of a third-party certificate authority.
package identityca

import (
	"context"
	"encoding/json"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
)

// Kind describes how an IdentityCA maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.IdentityCA]{
	GVK:        v1alpha1.IdentityCAGroupVersionKind,
	List:       &v1alpha1.IdentityCAList{},
	Collection: identity.Collection,
	Desired: func(ctx context.Context, api *client.Client, mg *v1alpha1.IdentityCA) (map[string]any, error) {
		return identity.Desired(ctx, api, mg.Spec.ForProvider.IdentityParameters)
	},
	CreateOnly: createOnly,
	Observe: func(mg *v1alpha1.IdentityCA, raw json.RawMessage) error {
		return identity.Observe(raw, &mg.Status.AtProvider)
	},
	ConnectionDetails: identity.ConnectionDetails,
}

func createOnly(ctx context.Context, api *client.Client, mg *v1alpha1.IdentityCA) (map[string]any, error) {
	p := mg.Spec.ForProvider

	ca, err := api.ResolveID(ctx, "cas", p.Ottca)
	if err != nil {
		return nil, err
	}
	return identity.CreateOnly(p.IdentityParameters, map[string]any{"ottca": ca}), nil
}
