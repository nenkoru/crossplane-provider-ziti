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
// that enroll with a one-time token. It also holds what the other identity
// kinds, which differ only in how the identity enrolls, share with it.
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

const (
	// Collection is the Ziti API collection identities of every kind are
	// stored in.
	Collection = "identities"

	// ConnectionKeyEnrollmentToken is the connection secret key that holds
	// the enrollment token (JWT) of an identity.
	ConnectionKeyEnrollmentToken = "enrollmentToken"
)

// BeforeDelete drops the hosting settings an identity has per service. Ziti
// keeps them when a service is deleted and then refuses to delete the
// identity because that service does not exist.
var BeforeDelete = map[string]any{
	"serviceHostingCosts":       map[string]any{},
	"serviceHostingPrecedences": map[string]any{},
}

// Kind describes how an Identity maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.Identity]{
	GVK:          v1alpha1.IdentityGroupVersionKind,
	List:         &v1alpha1.IdentityList{},
	Collection:   Collection,
	BeforeDelete: BeforeDelete,
	Desired: func(ctx context.Context, api *client.Client, mg *v1alpha1.Identity) (map[string]any, error) {
		return Desired(ctx, api, mg.Spec.ForProvider)
	},
	CreateOnly: func(_ context.Context, _ *client.Client, mg *v1alpha1.Identity) (map[string]any, error) {
		return CreateOnly(mg.Spec.ForProvider, map[string]any{"ott": true}), nil
	},
	Observe: func(mg *v1alpha1.Identity, raw json.RawMessage) error {
		return Observe(raw, &mg.Status.AtProvider)
	},
	ConnectionDetails: ConnectionDetails,
}

// methods are the enrollment methods of the identity kinds, in the order in
// which a pending enrollment is looked for.
var methods = []string{"ott", "ottca", "updb"}

// enrollment is a pending enrollment of a Ziti identity.
type enrollment struct {
	JWT       string `json:"jwt"`
	ExpiresAt string `json:"expiresAt"`
}

// entity holds the fields of a Ziti identity that are not copied to the
// status as is.
type entity struct {
	Authenticators map[string]json.RawMessage `json:"authenticators"`
	Enrollment     map[string]enrollment      `json:"enrollment"`
}

// pending returns the enrollment the identity has not completed yet, if any.
func (e entity) pending() enrollment {
	for _, method := range methods {
		if pending := e.Enrollment[method]; pending.JWT != "" {
			return pending
		}
	}
	return enrollment{}
}

// enrolled returns true if the identity has something to authenticate with.
func (e entity) enrolled() bool {
	for _, authenticator := range e.Authenticators {
		if len(authenticator) > 0 && string(authenticator) != "null" {
			return true
		}
	}
	return false
}

// Desired returns the fields of a Ziti identity that identities of every
// kind manage.
func Desired(ctx context.Context, api *client.Client, p v1alpha1.IdentityParameters) (map[string]any, error) {
	costs, err := generic.ResolveKeys(ctx, api, "services", p.ServiceHostingCosts)
	if err != nil {
		return nil, err
	}
	precedences, err := generic.ResolveKeys(ctx, api, "services", p.ServiceHostingPrecedences)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"name":                      p.Name,
		"isAdmin":                   ptr.Deref(p.IsAdmin, false),
		"roleAttributes":            generic.Strings(p.RoleAttributes),
		"serviceHostingCosts":       costs,
		"serviceHostingPrecedences": precedences,
		"appData":                   generic.Tags(p.AppData),
		"tags":                      generic.Tags(p.Tags),
	}

	if p.AuthPolicyID != nil {
		id, err := api.ResolveID(ctx, "auth-policies", *p.AuthPolicyID)
		if err != nil {
			return nil, err
		}
		body["authPolicyId"] = id
	}
	if p.ExternalID != nil {
		body["externalId"] = *p.ExternalID
	}
	if p.DefaultHostingCost != nil {
		body["defaultHostingCost"] = *p.DefaultHostingCost
	}
	if p.DefaultHostingPrecedence != nil {
		body["defaultHostingPrecedence"] = *p.DefaultHostingPrecedence
	}
	return body, nil
}

// CreateOnly returns the fields of a Ziti identity that can only be set when
// it is created: its type and how it enrolls. An identity without an
// enrollment takes nil.
func CreateOnly(p v1alpha1.IdentityParameters, enrollment map[string]any) map[string]any {
	body := map[string]any{"type": cmp.Or(p.Type, "Default")}
	if enrollment != nil {
		body["enrollment"] = enrollment
	}
	return body
}

// Observe copies a Ziti identity into the status of its managed resource.
func Observe(raw json.RawMessage, o *v1alpha1.IdentityObservation) error {
	if err := generic.Unmarshal(raw, o); err != nil {
		return err
	}

	var e entity
	if err := json.Unmarshal(raw, &e); err != nil {
		return err
	}
	o.Enrolled = e.enrolled()
	o.EnrollmentExpiresAt = e.pending().ExpiresAt
	return nil
}

// ConnectionDetails returns the enrollment token, which Ziti only reports
// until the identity enrolls.
func ConnectionDetails(raw json.RawMessage) (managed.ConnectionDetails, error) {
	var e entity
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}

	token := e.pending().JWT
	if token == "" {
		return nil, nil
	}
	return managed.ConnectionDetails{ConnectionKeyEnrollmentToken: []byte(token)}, nil
}
