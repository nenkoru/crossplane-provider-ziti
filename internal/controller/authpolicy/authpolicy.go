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

// Package authpolicy maps the AuthPolicy managed resource to Ziti auth
// policies.
package authpolicy

import (
	"context"
	"encoding/json"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// Kind describes how an AuthPolicy maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.AuthPolicy]{
	GVK:        v1alpha1.AuthPolicyGroupVersionKind,
	List:       &v1alpha1.AuthPolicyList{},
	Collection: "auth-policies",
	Desired:    desired,
	Observe: func(mg *v1alpha1.AuthPolicy, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

func desired(_ context.Context, _ *client.Client, mg *v1alpha1.AuthPolicy) (map[string]any, error) {
	p := mg.Spec.ForProvider

	body := map[string]any{"name": p.Name}
	if p.Primary != nil {
		body["primary"] = authMethods(p.Primary)
	}
	if p.Secondary != nil {
		body["secondary"] = authMethods(p.Secondary)
	}
	return body, nil
}

func authMethods(m *v1alpha1.AuthMethods) map[string]any {
	methods := map[string]any{}
	if m.Cert != nil {
		methods["cert"] = map[string]any{
			"allowed":           m.Cert.Allowed,
			"allowExpiredCerts": m.Cert.AllowExpiredCerts,
		}
	}
	if m.UPDB != nil {
		methods["updb"] = map[string]any{
			"allowed":                m.UPDB.Allowed,
			"minPasswordLength":      m.UPDB.MinPasswordLength,
			"maxAttempts":            m.UPDB.MaxAttempts,
			"lockoutDurationMinutes": m.UPDB.LockoutDurationMinutes,
		}
	}
	if m.ExtJWT != nil {
		methods["extJwt"] = map[string]any{
			"allowed":        m.ExtJWT.Allowed,
			"allowedSigners": generic.Strings(m.ExtJWT.AllowedSigners),
		}
	}
	return methods
}
