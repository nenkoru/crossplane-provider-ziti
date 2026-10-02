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

// Package edgerouterpolicy maps the EdgeRouterPolicy managed resource to Ziti
// edge router policies.
package edgerouterpolicy

import (
	"cmp"
	"context"
	"encoding/json"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// Kind describes how an EdgeRouterPolicy maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.EdgeRouterPolicy]{
	GVK:        v1alpha1.EdgeRouterPolicyGroupVersionKind,
	List:       &v1alpha1.EdgeRouterPolicyList{},
	Collection: "edge-router-policies",
	Desired:    desired,
	Observe: func(mg *v1alpha1.EdgeRouterPolicy, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

func desired(ctx context.Context, api *client.Client, mg *v1alpha1.EdgeRouterPolicy) (map[string]any, error) {
	p := mg.Spec.ForProvider

	edgeRouterRoles, err := generic.ResolveRoles(ctx, api, "edge-routers", p.EdgeRouterRoles)
	if err != nil {
		return nil, err
	}
	identityRoles, err := generic.ResolveRoles(ctx, api, "identities", p.IdentityRoles)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"name":            p.Name,
		"semantic":        cmp.Or(string(p.Semantic), "AllOf"),
		"edgeRouterRoles": edgeRouterRoles,
		"identityRoles":   identityRoles,
		"tags":            generic.Tags(p.Tags),
	}, nil
}
