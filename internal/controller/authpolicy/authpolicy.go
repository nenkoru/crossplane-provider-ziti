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

	"k8s.io/utils/ptr"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// Defaults of the Ziti API for password authentication.
const (
	defaultMinPasswordLength int64 = 5
	defaultMaxAttempts       int64 = 5
)

// Kind describes how an AuthPolicy maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.AuthPolicy]{
	GVK:        v1alpha1.AuthPolicyGroupVersionKind,
	List:       &v1alpha1.AuthPolicyList{},
	Collection: "auth-policies",
	Desired:    desired,
	// A PATCH of an auth policy silently ignores some of the password
	// settings. The desired state is always complete, so nothing is lost by
	// replacing the policy.
	ReplaceOnUpdate: true,
	Observe: func(mg *v1alpha1.AuthPolicy, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

// desired always describes every authentication method: the Ziti API
// requires all of them, and a method that is left out of the spec is not
// allowed.
func desired(_ context.Context, _ *client.Client, mg *v1alpha1.AuthPolicy) (map[string]any, error) {
	p := mg.Spec.ForProvider

	primary := ptr.Deref(p.Primary, v1alpha1.AuthMethods{})
	cert := ptr.Deref(primary.Cert, v1alpha1.CertAuth{})
	updb := ptr.Deref(primary.UPDB, v1alpha1.UPDBAuth{})
	extJWT := ptr.Deref(primary.ExtJWT, v1alpha1.ExtJWTAuth{})
	secondary := ptr.Deref(p.Secondary, v1alpha1.SecondaryAuth{})

	return map[string]any{
		"name": p.Name,
		"primary": map[string]any{
			"cert": map[string]any{
				"allowed":           cert.Allowed,
				"allowExpiredCerts": cert.AllowExpiredCerts,
			},
			"updb": map[string]any{
				"allowed":                updb.Allowed,
				"minPasswordLength":      ptr.Deref(updb.MinPasswordLength, defaultMinPasswordLength),
				"maxAttempts":            ptr.Deref(updb.MaxAttempts, defaultMaxAttempts),
				"lockoutDurationMinutes": ptr.Deref(updb.LockoutDurationMinutes, 0),
				"requireMixedCase":       updb.RequireMixedCase,
				"requireNumberChar":      updb.RequireNumberChar,
				"requireSpecialChar":     updb.RequireSpecialChar,
			},
			"extJwt": map[string]any{
				"allowed":        extJWT.Allowed,
				"allowedSigners": generic.Strings(extJWT.AllowedSigners),
			},
		},
		"secondary": map[string]any{
			"requireTotp":         secondary.RequireTOTP,
			"requireExtJwtSigner": secondary.RequireExtJWTSigner,
		},
		"tags": generic.Tags(p.Tags),
	}, nil
}
