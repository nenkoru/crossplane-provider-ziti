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

// Package externaljwtsigner maps the ExternalJWTSigner managed resource to
// Ziti external JWT signers.
package externaljwtsigner

import (
	"context"
	"encoding/json"

	"k8s.io/utils/ptr"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// defaultTargetToken is the default of the Ziti API.
const defaultTargetToken = "ACCESS"

// Kind describes how an ExternalJWTSigner maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.ExternalJWTSigner]{
	GVK:        v1alpha1.ExternalJWTSignerGroupVersionKind,
	List:       &v1alpha1.ExternalJWTSignerList{},
	Collection: "external-jwt-signers",
	Desired:    desired,
	// A PATCH ignores fields that are null, so it cannot remove a setting,
	// and a signer that changes from a certificate to a JWKS endpoint has to
	// remove the certificate. A replaced signer takes the defaults of Ziti
	// for what the desired state leaves out.
	ReplaceOnUpdate: true,
	Observe: func(mg *v1alpha1.ExternalJWTSigner, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

// desired describes every setting of a signer, with null for a setting that
// is not set, so that it is removed. The three settings Ziti replaces an
// empty value of with a default of its own are only sent when they are set.
func desired(ctx context.Context, api *client.Client, mg *v1alpha1.ExternalJWTSigner) (map[string]any, error) {
	p := mg.Spec.ForProvider

	body := map[string]any{
		"name":                          p.Name,
		"issuer":                        p.Issuer,
		"audience":                      p.Audience,
		"enabled":                       p.Enabled,
		"jwksEndpoint":                  p.JwksEndpoint,
		"certPem":                       p.CertPem,
		"kid":                           p.Kid,
		"useExternalId":                 p.UseExternalID,
		"externalAuthUrl":               p.ExternalAuthURL,
		"clientId":                      p.ClientID,
		"scopes":                        generic.Strings(p.Scopes),
		"targetToken":                   ptr.Deref(p.TargetToken, defaultTargetToken),
		"enrollToCertEnabled":           p.EnrollToCertEnabled,
		"enrollToTokenEnabled":          p.EnrollToTokenEnabled,
		"enrollAttributeClaimsSelector": ptr.Deref(p.EnrollAttributeClaimsSelector, ""),
		"tags":                          generic.Tags(p.Tags),
	}

	if p.ClaimsProperty != nil {
		body["claimsProperty"] = *p.ClaimsProperty
	}
	if p.EnrollNameClaimsSelector != nil {
		body["enrollNameClaimsSelector"] = *p.EnrollNameClaimsSelector
	}
	if p.EnrollAuthPolicyID != nil {
		id, err := api.ResolveID(ctx, "auth-policies", *p.EnrollAuthPolicyID)
		if err != nil {
			return nil, err
		}
		body["enrollAuthPolicyId"] = id
	}
	return body, nil
}
