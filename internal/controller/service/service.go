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

// Package service maps the Service managed resource to Ziti services.
package service

import (
	"context"
	"encoding/json"

	"k8s.io/utils/ptr"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// Kind describes how a Service maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.Service]{
	GVK:        v1alpha1.ServiceGroupVersionKind,
	List:       &v1alpha1.ServiceList{},
	Collection: "services",
	Desired:    desired,
	// A PATCH of a service silently ignores encryptionRequired.
	ReplaceOnUpdate: true,
	Observe: func(mg *v1alpha1.Service, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

func desired(ctx context.Context, api *client.Client, mg *v1alpha1.Service) (map[string]any, error) {
	p := mg.Spec.ForProvider

	configs, err := generic.ResolveIDs(ctx, api, "configs", p.Configs)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"name":               p.Name,
		"encryptionRequired": ptr.Deref(p.EncryptionRequired, true),
		"configs":            configs,
		"roleAttributes":     generic.Strings(p.RoleAttributes),
		"tags":               generic.Tags(p.Tags),
	}
	if p.MaxIdleTimeMillis != nil {
		body["maxIdleTimeMillis"] = *p.MaxIdleTimeMillis
	}
	if p.TerminatorStrategy != nil {
		body["terminatorStrategy"] = string(*p.TerminatorStrategy)
	}
	return body, nil
}
