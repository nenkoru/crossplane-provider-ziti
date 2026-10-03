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

package main

import (
	"github.com/crossplane/provider-ziti/internal/controller/authpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/certificateauthority"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv1"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv2"
	"github.com/crossplane/provider-ziti/internal/controller/configinterceptv1"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouter"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/externaljwtsigner"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/identityca"
	"github.com/crossplane/provider-ziti/internal/controller/identitynone"
	"github.com/crossplane/provider-ziti/internal/controller/identityupdb"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckdomain"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmac"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmfa"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmultiprocess"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckos"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckprocess"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/serviceedgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

// A kind says what the pages cannot take from the CRD of a kind.
type kind struct {
	Kind string
	// Entity is what the kind is in Ziti, in words.
	Entity string
	// Collection of the Ziti Edge Management API, and whether an update
	// replaces the entity: both as the controller of the kind has them.
	Collection string
	Replace    bool
	// Secret is the key of the connection secret, if the kind writes one.
	Secret string
	// Example is the example file whose manifest the page shows; the first
	// one with the kind if empty.
	Example string
	// Prefix of the fields of spec.forProvider in the Ziti entity, for the
	// kinds whose settings are the data of a config.
	Prefix string
	// Fields are the fields that are not found in the Ziti entity under
	// their own path: what they become instead.
	Fields map[string]string
	// Constant is what the provider sends that no field sets.
	Constant string
}

const enrollmentToken = "enrollmentToken"

// roles is what becomes of the "@name" entries of policy roles.
const roles = "; `@name` is sent as `@id`"

// identityFields are the fields of the identity kinds that are not fields of
// the Ziti identity.
var identityFields = map[string]string{
	"authPolicyId":              "`authPolicyId`, as an ID",
	"serviceHostingCosts":       "`serviceHostingCosts`, keyed by service ID",
	"serviceHostingPrecedences": "`serviceHostingPrecedences`, keyed by service ID",
	"enrollmentDuration":        "`expiresAt` of the enrollment that replaces one that expired or is gone",
	"ottca":                     "`enrollment.ottca`, as the ID of the certificate authority",
	"updbUsername":              "`enrollment.updb`",
}

// configFields are the fields of the config kinds that are not part of the
// data of the config.
var configFields = map[string]string{
	"name": "`name`",
	"tags": "`tags`",
}

var kinds = []kind{
	{
		Kind: "Service", Entity: "service",
		Collection: service.Kind.Collection, Replace: service.Kind.ReplaceOnUpdate,
		Example: "examples/service/service.yaml",
		Fields:  map[string]string{"configs": "`configs`, as IDs"},
	},
	{
		Kind: "ConfigInterceptV1", Entity: "config of type `intercept.v1`",
		Collection: configinterceptv1.Kind.Collection, Replace: configinterceptv1.Kind.ReplaceOnUpdate,
		Example: "examples/service/service.yaml",
		Prefix:  "data.", Fields: configFields, Constant: "`configTypeId`, the ID of the config type `intercept.v1`",
	},
	{
		Kind: "ConfigHostV1", Entity: "config of type `host.v1`",
		Collection: confighostv1.Kind.Collection, Replace: confighostv1.Kind.ReplaceOnUpdate,
		Example: "examples/service/service.yaml",
		Prefix:  "data.", Fields: configFields, Constant: "`configTypeId`, the ID of the config type `host.v1`",
	},
	{
		Kind: "ConfigHostV2", Entity: "config of type `host.v2`",
		Collection: confighostv2.Kind.Collection, Replace: confighostv2.Kind.ReplaceOnUpdate,
		Prefix: "data.", Fields: configFields, Constant: "`configTypeId`, the ID of the config type `host.v2`",
	},
	{
		Kind: "ServicePolicy", Entity: "service policy",
		Collection: servicepolicy.Kind.Collection, Replace: servicepolicy.Kind.ReplaceOnUpdate,
		Example: "examples/servicepolicy/servicepolicy.yaml",
		Fields: map[string]string{
			"serviceRoles":      "`serviceRoles`" + roles,
			"identityRoles":     "`identityRoles`" + roles,
			"postureCheckRoles": "`postureCheckRoles`" + roles,
		},
	},
	{
		Kind: "EdgeRouterPolicy", Entity: "edge router policy",
		Collection: edgerouterpolicy.Kind.Collection, Replace: edgerouterpolicy.Kind.ReplaceOnUpdate,
		Fields: map[string]string{
			"edgeRouterRoles": "`edgeRouterRoles`" + roles,
			"identityRoles":   "`identityRoles`" + roles,
		},
	},
	{
		Kind: "ServiceEdgeRouterPolicy", Entity: "service edge router policy",
		Collection: serviceedgerouterpolicy.Kind.Collection, Replace: serviceedgerouterpolicy.Kind.ReplaceOnUpdate,
		Fields: map[string]string{
			"serviceRoles":    "`serviceRoles`" + roles,
			"edgeRouterRoles": "`edgeRouterRoles`" + roles,
		},
	},
	{
		Kind: "Identity", Entity: "identity that enrolls with a one-time token",
		Collection: identity.Kind.Collection, Replace: identity.Kind.ReplaceOnUpdate,
		Secret: enrollmentToken, Example: "examples/identity/identity.yaml",
		Fields: identityFields, Constant: "`enrollment.ott` when it creates the identity",
	},
	{
		Kind: "IdentityCA", Entity: "identity that enrolls with a one-time token and a certificate of a third-party CA",
		Collection: identityca.Kind.Collection, Replace: identityca.Kind.ReplaceOnUpdate,
		Secret: enrollmentToken, Example: "examples/identity/ca.yaml",
		Fields: identityFields,
	},
	{
		Kind: "IdentityUPDB", Entity: "identity that authenticates with a username and a password",
		Collection: identityupdb.Kind.Collection, Replace: identityupdb.Kind.ReplaceOnUpdate,
		Secret: enrollmentToken, Example: "examples/identity/updb.yaml",
		Fields: identityFields,
	},
	{
		Kind: "IdentityNone", Entity: "identity without an enrollment",
		Collection: identitynone.Kind.Collection, Replace: identitynone.Kind.ReplaceOnUpdate,
		Example: "examples/identity/none.yaml",
		Fields:  identityFields,
	},
	{
		Kind: "EdgeRouter", Entity: "edge router",
		Collection: edgerouter.Kind.Collection, Replace: edgerouter.Kind.ReplaceOnUpdate,
		Secret: enrollmentToken, Example: "examples/edgerouter/edgerouter.yaml",
	},
	{
		Kind: "AuthPolicy", Entity: "auth policy",
		Collection: authpolicy.Kind.Collection, Replace: authpolicy.Kind.ReplaceOnUpdate,
		Example: "examples/authpolicy/authpolicy.yaml",
		Fields: map[string]string{
			"primary.extJwt.allowedSigners": "`primary.extJwt.allowedSigners`, as IDs",
			"secondary.requireExtJwtSigner": "`secondary.requireExtJwtSigner`, as an ID",
		},
	},
	{
		Kind: "ExternalJWTSigner", Entity: "external JWT signer",
		Collection: externaljwtsigner.Kind.Collection, Replace: externaljwtsigner.Kind.ReplaceOnUpdate,
		Fields: map[string]string{"enrollAuthPolicyId": "`enrollAuthPolicyId`, as an ID"},
	},
	{
		Kind: "CertificateAuthority", Entity: "third-party certificate authority",
		Collection: certificateauthority.Kind.Collection, Replace: certificateauthority.Kind.ReplaceOnUpdate,
	},
	{
		Kind: "PostureCheckOS", Entity: "posture check of type `OS`",
		Collection: posturecheckos.Kind.Collection, Replace: posturecheckos.Kind.ReplaceOnUpdate,
		Example: "examples/posturecheck/os.yaml", Constant: "`typeId: OS`",
	},
	{
		Kind: "PostureCheckMFA", Entity: "posture check of type `MFA`",
		Collection: posturecheckmfa.Kind.Collection, Replace: posturecheckmfa.Kind.ReplaceOnUpdate,
		Constant: "`typeId: MFA`",
	},
	{
		Kind: "PostureCheckDomain", Entity: "posture check of type `DOMAIN`",
		Collection: posturecheckdomain.Kind.Collection, Replace: posturecheckdomain.Kind.ReplaceOnUpdate,
		Constant: "`typeId: DOMAIN`",
	},
	{
		Kind: "PostureCheckMac", Entity: "posture check of type `MAC`",
		Collection: posturecheckmac.Kind.Collection, Replace: posturecheckmac.Kind.ReplaceOnUpdate,
		Constant: "`typeId: MAC`",
	},
	{
		Kind: "PostureCheckProcess", Entity: "posture check of type `PROCESS`",
		Collection: posturecheckprocess.Kind.Collection, Replace: posturecheckprocess.Kind.ReplaceOnUpdate,
		Constant: "`typeId: PROCESS`",
	},
	{
		Kind: "PostureCheckMultiProcess", Entity: "posture check of type `PROCESS_MULTI`",
		Collection: posturecheckmultiprocess.Kind.Collection, Replace: posturecheckmultiprocess.Kind.ReplaceOnUpdate,
		Constant: "`typeId: PROCESS_MULTI`",
	},
}

// inZiti says where a field of spec.forProvider ends up in the Ziti entity.
func (k kind) inZiti(path string) string {
	if in, ok := k.Fields[path]; ok {
		return in
	}
	return "`" + k.Prefix + path + "`"
}
