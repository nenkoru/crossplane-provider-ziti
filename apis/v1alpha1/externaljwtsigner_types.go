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

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExternalJWTSignerSpec defines the desired state of an ExternalJWTSigner.
type ExternalJWTSignerSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ExternalJWTSignerParameters `json:"forProvider"`
}

// ExternalJWTSignerParameters define the desired state of a Ziti external
// JWT signer. The keys that verify its tokens come from jwksEndpoint or from
// certPem.
// +kubebuilder:validation:XValidation:rule="has(self.jwksEndpoint) != has(self.certPem)",message="exactly one of jwksEndpoint and certPem is required"
type ExternalJWTSignerParameters struct {
	// Name of the external JWT signer.
	Name string `json:"name"`

	// Issuer is the iss claim of the tokens of this signer. No two signers
	// may have the same issuer.
	// +kubebuilder:validation:MinLength=1
	Issuer string `json:"issuer"`

	// Audience is the aud claim a token must have to be accepted.
	// +kubebuilder:validation:MinLength=1
	Audience string `json:"audience"`

	// Enabled makes Ziti accept the tokens of this signer.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// JwksEndpoint is the URL of the JSON Web Key Set of the signer. It must
	// name a host, and its scheme must be http or https, in any case.
	// +optional
	// +kubebuilder:validation:Pattern=`^[Hh][Tt][Tt][Pp][Ss]?://([^/?#@]*@)?(\[[0-9A-Fa-f:.]+\]|[^/?#@:\[\]]+)(:[0-9]*)?([/?#].*)?$`
	JwksEndpoint *string `json:"jwksEndpoint,omitempty"`

	// CertPem is the PEM encoded certificate that verifies the tokens of the
	// signer, for a signer without a JSON Web Key Set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	CertPem *string `json:"certPem,omitempty"`

	// Kid is the key ID of the certificate in certPem: the kid header of the
	// tokens it verifies.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Kid *string `json:"kid,omitempty"`

	// ClaimsProperty is the claim of a token that tells which identity it is
	// for. Ziti takes the subject if it is not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ClaimsProperty *string `json:"claimsProperty,omitempty"`

	// UseExternalID looks the identity up by its externalId instead of its
	// ID.
	// +optional
	UseExternalID bool `json:"useExternalId,omitempty"`

	// ExternalAuthURL is the URL clients obtain a token of this signer from.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExternalAuthURL *string `json:"externalAuthUrl,omitempty"`

	// ClientID is the OAuth client ID clients obtain a token with.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ClientID *string `json:"clientId,omitempty"`

	// Scopes are the OAuth scopes clients request a token with.
	// +optional
	// +listType=set
	Scopes []string `json:"scopes,omitempty"`

	// TargetToken is the token clients present to Ziti: the access token or
	// the ID token.
	// +optional
	// +kubebuilder:default=ACCESS
	// +kubebuilder:validation:Enum=ACCESS;ID
	TargetToken *string `json:"targetToken,omitempty"`

	// EnrollToCertEnabled lets the holder of a token of this signer enroll,
	// which creates an identity that authenticates with a certificate.
	// +optional
	EnrollToCertEnabled bool `json:"enrollToCertEnabled,omitempty"`

	// EnrollToTokenEnabled lets the holder of a token of this signer enroll,
	// which creates an identity that authenticates with tokens of this
	// signer.
	// +optional
	EnrollToTokenEnabled bool `json:"enrollToTokenEnabled,omitempty"`

	// EnrollNameClaimsSelector is the claim of a token that names the
	// identity an enrollment creates. Ziti takes the subject if it is not
	// set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	EnrollNameClaimsSelector *string `json:"enrollNameClaimsSelector,omitempty"`

	// EnrollAttributeClaimsSelector is the claim of a token that holds the
	// role attributes of the identity an enrollment creates.
	// +optional
	EnrollAttributeClaimsSelector *string `json:"enrollAttributeClaimsSelector,omitempty"`

	// EnrollAuthPolicyID is the name or ID of the auth policy of the
	// identities an enrollment creates. Ziti takes the default auth policy
	// if it is not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	EnrollAuthPolicyID *string `json:"enrollAuthPolicyId,omitempty"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ExternalJWTSignerStatus defines the observed state of an
// ExternalJWTSigner.
type ExternalJWTSignerStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ExternalJWTSignerObservation `json:"atProvider,omitempty"`
}

// ExternalJWTSignerObservation keeps the observed state.
type ExternalJWTSignerObservation struct {
	ID              string   `json:"id,omitempty"`
	Name            string   `json:"name,omitempty"`
	Issuer          string   `json:"issuer,omitempty"`
	Audience        string   `json:"audience,omitempty"`
	Enabled         bool     `json:"enabled,omitempty"`
	JwksEndpoint    string   `json:"jwksEndpoint,omitempty"`
	Kid             string   `json:"kid,omitempty"`
	ClaimsProperty  string   `json:"claimsProperty,omitempty"`
	UseExternalID   bool     `json:"useExternalId,omitempty"`
	ExternalAuthURL string   `json:"externalAuthUrl,omitempty"`
	ClientID        string   `json:"clientId,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	TargetToken     string   `json:"targetToken,omitempty"`
	// CommonName, Fingerprint, NotBefore and NotAfter describe the
	// certificate of a signer that has one.
	CommonName                    string            `json:"commonName,omitempty"`
	Fingerprint                   string            `json:"fingerprint,omitempty"`
	NotBefore                     string            `json:"notBefore,omitempty"`
	NotAfter                      string            `json:"notAfter,omitempty"`
	EnrollToCertEnabled           bool              `json:"enrollToCertEnabled,omitempty"`
	EnrollToTokenEnabled          bool              `json:"enrollToTokenEnabled,omitempty"`
	EnrollNameClaimsSelector      string            `json:"enrollNameClaimsSelector,omitempty"`
	EnrollAttributeClaimsSelector string            `json:"enrollAttributeClaimsSelector,omitempty"`
	EnrollAuthPolicyID            string            `json:"enrollAuthPolicyId,omitempty"`
	Tags                          map[string]string `json:"tags,omitempty"`
	CreatedAt                     string            `json:"createdAt,omitempty"`
	UpdatedAt                     string            `json:"updatedAt,omitempty"`
}

// ExternalJWTSigner is an issuer of JSON Web Tokens, such as an OpenID
// Connect provider, whose tokens Ziti accepts for authentication. Auth
// policies refer to it by its name or ID.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type ExternalJWTSigner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalJWTSignerSpec   `json:"spec"`
	Status ExternalJWTSignerStatus `json:"status,omitempty"`
}

// ExternalJWTSignerList contains a list of ExternalJWTSigner.
// +kubebuilder:object:root=true
type ExternalJWTSignerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalJWTSigner `json:"items"`
}
