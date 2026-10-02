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

// Package edgerouter maps the EdgeRouter managed resource to Ziti edge
// routers.
package edgerouter

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// ConnectionKeyEnrollmentToken is the connection secret key that holds the
// enrollment token (JWT) of an edge router.
const ConnectionKeyEnrollmentToken = "enrollmentToken"

// Kind describes how an EdgeRouter maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.EdgeRouter]{
	GVK:        v1alpha1.EdgeRouterGroupVersionKind,
	List:       &v1alpha1.EdgeRouterList{},
	Collection: "edge-routers",
	Desired:    desired,
	Observe: func(mg *v1alpha1.EdgeRouter, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
	ConnectionDetails: connectionDetails,
}

func desired(_ context.Context, _ *client.Client, mg *v1alpha1.EdgeRouter) (map[string]any, error) {
	p := mg.Spec.ForProvider

	body := map[string]any{
		"name":           p.Name,
		"roleAttributes": generic.Strings(p.RoleAttributes),
		"tags":           generic.Tags(p.Tags),
	}
	if p.IsTunnelerEnabled != nil {
		body["isTunnelerEnabled"] = *p.IsTunnelerEnabled
	}
	if p.NoTraversal != nil {
		body["noTraversal"] = *p.NoTraversal
	}
	if p.Cost != nil {
		body["cost"] = *p.Cost
	}
	return body, nil
}

// connectionDetails returns the enrollment token, which Ziti only reports
// until the edge router enrolls.
func connectionDetails(raw json.RawMessage) (managed.ConnectionDetails, error) {
	var e struct {
		EnrollmentJWT string `json:"enrollmentJwt"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	if e.EnrollmentJWT == "" {
		return nil, nil
	}
	return managed.ConnectionDetails{ConnectionKeyEnrollmentToken: []byte(e.EnrollmentJWT)}, nil
}
