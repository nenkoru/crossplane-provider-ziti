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

// Package posturecheckmfa maps the PostureCheckMFA managed resource to Ziti
// posture checks of type MFA.
package posturecheckmfa

import (
	"context"
	"encoding/json"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// Kind describes how a PostureCheckMFA maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.PostureCheckMFA]{
	GVK:        v1alpha1.PostureCheckMFAGroupVersionKind,
	List:       &v1alpha1.PostureCheckMFAList{},
	Collection: "posture-checks",
	Desired:    desired,
	Observe: func(mg *v1alpha1.PostureCheckMFA, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

func desired(_ context.Context, _ *client.Client, mg *v1alpha1.PostureCheckMFA) (map[string]any, error) {
	p := mg.Spec.ForProvider

	body := map[string]any{
		"name": p.Name,
		// The type discriminates posture checks and is required on update too.
		"typeId":         "MFA",
		"roleAttributes": generic.Strings(p.RoleAttributes),
	}
	if p.TimeoutSeconds != nil {
		body["timeoutSeconds"] = *p.TimeoutSeconds
	}
	return body, nil
}
