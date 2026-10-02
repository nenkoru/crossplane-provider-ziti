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

// Package certificateauthority maps the CertificateAuthority managed
// resource to Ziti certificate authorities.
package certificateauthority

import (
	"cmp"
	"context"
	"encoding/json"

	"k8s.io/utils/ptr"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// Defaults of the Ziti API. The identity name format is required when a
// certificate authority is replaced, and Ziti stores its own default in place
// of an empty one.
const (
	defaultIdentityNameFormat = "[caName]-[commonName]"
	defaultClaimLocation      = "COMMON_NAME"
	defaultClaimMatcher       = "ALL"
	defaultClaimParser        = "NONE"
)

// Kind describes how a CertificateAuthority maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.CertificateAuthority]{
	GVK:        v1alpha1.CertificateAuthorityGroupVersionKind,
	List:       &v1alpha1.CertificateAuthorityList{},
	Collection: "cas",
	Desired:    desired,
	// A PATCH of a certificate authority removes the external ID claim unless
	// it repeats it, and never applies its parserCriteria. The desired state
	// is always complete, so nothing is lost by replacing the certificate
	// authority. Its certificate, fingerprint and verification survive that.
	ReplaceOnUpdate: true,
	// Ziti has no way to change the certificate.
	CreateOnly: func(_ context.Context, _ *client.Client, mg *v1alpha1.CertificateAuthority) (map[string]any, error) {
		return map[string]any{"certPem": mg.Spec.ForProvider.CertPem}, nil
	},
	Observe: func(mg *v1alpha1.CertificateAuthority, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

func desired(_ context.Context, _ *client.Client, mg *v1alpha1.CertificateAuthority) (map[string]any, error) {
	p := mg.Spec.ForProvider

	return map[string]any{
		"name":                      p.Name,
		"isAuthEnabled":             p.IsAuthEnabled,
		"isOttCaEnrollmentEnabled":  p.IsOttCaEnrollmentEnabled,
		"isAutoCaEnrollmentEnabled": p.IsAutoCaEnrollmentEnabled,
		"identityRoles":             generic.Strings(p.IdentityRoles),
		"identityNameFormat":        ptr.Deref(p.IdentityNameFormat, defaultIdentityNameFormat),
		"externalIdClaim":           externalIDClaim(p.ExternalIDClaim),
		"tags":                      generic.Tags(p.Tags),
	}, nil
}

// externalIDClaim returns the external ID claim with all its fields, which
// the Ziti API requires, or nil to remove the claim.
func externalIDClaim(c *v1alpha1.ExternalIDClaim) any {
	if c == nil {
		return nil
	}
	return map[string]any{
		"location":        cmp.Or(c.Location, defaultClaimLocation),
		"matcher":         cmp.Or(c.Matcher, defaultClaimMatcher),
		"matcherCriteria": c.MatcherCriteria,
		"parser":          cmp.Or(c.Parser, defaultClaimParser),
		"parserCriteria":  c.ParserCriteria,
		"index":           c.Index,
	}
}
