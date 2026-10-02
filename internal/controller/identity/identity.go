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

// Package identity maps the Identity managed resource to Ziti identities
// that enroll with a one-time token.
package identity

import (
	"cmp"
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"k8s.io/utils/ptr"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// ConnectionKeyEnrollmentToken is the connection secret key that holds the
// one-time enrollment token (JWT) of an identity.
const ConnectionKeyEnrollmentToken = "enrollmentToken"

// Kind describes how an Identity maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.Identity]{
	GVK:               v1alpha1.IdentityGroupVersionKind,
	List:              &v1alpha1.IdentityList{},
	Collection:        "identities",
	Desired:           desired,
	CreateOnly:        createOnly,
	Observe:           observe,
	ConnectionDetails: connectionDetails,
}

// entity holds the fields of a Ziti identity that are not copied to the
// status as is.
type entity struct {
	Authenticators struct {
		Cert json.RawMessage `json:"cert"`
	} `json:"authenticators"`
	Enrollment struct {
		OTT struct {
			JWT       string `json:"jwt"`
			ExpiresAt string `json:"expiresAt"`
		} `json:"ott"`
	} `json:"enrollment"`
}

func desired(_ context.Context, _ *client.Client, mg *v1alpha1.Identity) (map[string]any, error) {
	p := mg.Spec.ForProvider

	return map[string]any{
		"name":           p.Name,
		"isAdmin":        ptr.Deref(p.IsAdmin, false),
		"roleAttributes": generic.Strings(p.RoleAttributes),
		"tags":           generic.Tags(p.Tags),
	}, nil
}

func createOnly(_ context.Context, _ *client.Client, mg *v1alpha1.Identity) (map[string]any, error) {
	return map[string]any{
		"type":       cmp.Or(mg.Spec.ForProvider.Type, "Default"),
		"enrollment": map[string]any{"ott": true},
	}, nil
}

func observe(mg *v1alpha1.Identity, raw json.RawMessage) error {
	if err := generic.Unmarshal(raw, &mg.Status.AtProvider); err != nil {
		return err
	}

	var e entity
	if err := json.Unmarshal(raw, &e); err != nil {
		return err
	}
	mg.Status.AtProvider.Enrolled = len(e.Authenticators.Cert) > 0 && string(e.Authenticators.Cert) != "null"
	mg.Status.AtProvider.EnrollmentExpiresAt = e.Enrollment.OTT.ExpiresAt
	return nil
}

// connectionDetails returns the enrollment token, which Ziti only reports
// until the identity enrolls.
func connectionDetails(raw json.RawMessage) (managed.ConnectionDetails, error) {
	var e entity
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	if e.Enrollment.OTT.JWT == "" {
		return nil, nil
	}
	return managed.ConnectionDetails{ConnectionKeyEnrollmentToken: []byte(e.Enrollment.OTT.JWT)}, nil
}
