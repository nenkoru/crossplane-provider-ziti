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
	"fmt"
	"time"

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
	Repair:            renewEnrollment,
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

// renewEnrollment finds out whether an edge router is left without a usable
// enrollment token: it has not enrolled, and its enrollment has expired or
// is gone. The function it then returns has Ziti enroll the router anew,
// which gives it a new token that is valid for as long as the controller is
// configured to make it.
//
// Enrolling a router anew that has enrolled takes its certificate away and
// disconnects it. Only a router of which Ziti says that it is not verified
// and that has no certificate is taken not to have enrolled.
func renewEnrollment(_ context.Context, api *client.Client, _ *v1alpha1.EdgeRouter, raw json.RawMessage) (string, func(context.Context) error, error) {
	var e struct {
		ID                  string `json:"id"`
		IsVerified          *bool  `json:"isVerified"`
		Fingerprint         string `json:"fingerprint"`
		EnrollmentJWT       string `json:"enrollmentJwt"`
		EnrollmentExpiresAt string `json:"enrollmentExpiresAt"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return "", nil, err
	}
	if e.IsVerified == nil || *e.IsVerified || e.Fingerprint != "" {
		return "", nil, nil
	}

	defect := "the edge router has not enrolled and has no enrollment"
	if e.EnrollmentJWT != "" {
		expiry, err := time.Parse(time.RFC3339, e.EnrollmentExpiresAt)
		if err != nil {
			return "", nil, err
		}
		if !generic.Expired(api, expiry) {
			return "", nil, nil
		}
		defect = fmt.Sprintf("the edge router has not enrolled and its enrollment expired at %s", e.EnrollmentExpiresAt)
	}
	return defect, func(ctx context.Context) error {
		return api.Act(ctx, "edge-routers", e.ID, "re-enroll", nil)
	}, nil
}
