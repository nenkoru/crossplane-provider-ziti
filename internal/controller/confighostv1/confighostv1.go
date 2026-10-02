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

// Package confighostv1 maps the ConfigHostV1 managed resource to Ziti configs
// of type host.v1.
package confighostv1

import (
	"context"
	"encoding/json"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

const configType = "host.v1"

// Kind describes how a ConfigHostV1 maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.ConfigHostV1]{
	GVK:        v1alpha1.ConfigHostV1GroupVersionKind,
	List:       &v1alpha1.ConfigHostV1List{},
	Collection: "configs",
	Desired:    desired,
	CreateOnly: func(ctx context.Context, api *client.Client, _ *v1alpha1.ConfigHostV1) (map[string]any, error) {
		return generic.ConfigType(ctx, api, configType)
	},
	Observe: func(mg *v1alpha1.ConfigHostV1, entity json.RawMessage) error {
		return generic.UnmarshalConfig(entity, &mg.Status.AtProvider)
	},
}

func desired(_ context.Context, _ *client.Client, mg *v1alpha1.ConfigHostV1) (map[string]any, error) {
	p := mg.Spec.ForProvider

	data, err := generic.ConfigData(p)
	if err != nil {
		return nil, err
	}

	// The host.v1 schema only accepts true for these: forwarding is disabled
	// by leaving them out.
	for _, field := range []string{"forwardProtocol", "forwardPort", "forwardAddress"} {
		if data[field] == false {
			delete(data, field)
		}
	}

	return map[string]any{
		"name": p.Name,
		"data": data,
		"tags": generic.Tags(p.Tags),
	}, nil
}
