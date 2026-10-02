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

// Package servicepolicy maps the ServicePolicy managed resource to Ziti
// service policies.
package servicepolicy

import (
	"cmp"
	"context"
	"encoding/json"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// Kind describes how a ServicePolicy maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.ServicePolicy]{
	GVK:        v1alpha1.ServicePolicyGroupVersionKind,
	List:       &v1alpha1.ServicePolicyList{},
	Collection: "service-policies",
	Desired:    desired,
	Observe: func(mg *v1alpha1.ServicePolicy, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

func desired(ctx context.Context, api *client.Client, mg *v1alpha1.ServicePolicy) (map[string]any, error) {
	p := mg.Spec.ForProvider

	serviceRoles, err := generic.ResolveRoles(ctx, api, "services", p.ServiceRoles)
	if err != nil {
		return nil, err
	}
	identityRoles, err := generic.ResolveRoles(ctx, api, "identities", p.IdentityRoles)
	if err != nil {
		return nil, err
	}
	postureCheckRoles, err := generic.ResolveRoles(ctx, api, "posture-checks", p.PostureCheckRoles)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"name":              p.Name,
		"type":              string(p.Type),
		"semantic":          cmp.Or(string(p.Semantic), "AllOf"),
		"serviceRoles":      serviceRoles,
		"identityRoles":     identityRoles,
		"postureCheckRoles": postureCheckRoles,
		"tags":              generic.Tags(p.Tags),
	}, nil
}
