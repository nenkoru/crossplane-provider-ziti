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

// CertificateAuthoritySpec defines the desired state of a
// CertificateAuthority.
type CertificateAuthoritySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              CertificateAuthorityParameters `json:"forProvider"`
}

// CertificateAuthorityParameters define the desired state of a Ziti
// certificate authority.
type CertificateAuthorityParameters struct {
	// Name of the certificate authority.
	Name string `json:"name"`

	// CertPem is the PEM encoded certificate of the certificate authority. It
	// must be a CA certificate that no other certificate authority in Ziti
	// has, and it cannot be changed after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="certPem cannot be changed after creation"
	CertPem string `json:"certPem"`

	// IsAuthEnabled lets identities authenticate with a certificate of this
	// certificate authority.
	// +optional
	IsAuthEnabled bool `json:"isAuthEnabled,omitempty"`

	// IsOttCaEnrollmentEnabled lets identities enroll with a one-time token
	// and a certificate of this certificate authority.
	// +optional
	IsOttCaEnrollmentEnabled bool `json:"isOttCaEnrollmentEnabled,omitempty"`

	// IsAutoCaEnrollmentEnabled lets anyone with a certificate of this
	// certificate authority enroll, which creates an identity.
	// +optional
	IsAutoCaEnrollmentEnabled bool `json:"isAutoCaEnrollmentEnabled,omitempty"`

	// IdentityRoles are the role attributes of the identities that automatic
	// enrollment creates.
	// +optional
	// +listType=set
	IdentityRoles []string `json:"identityRoles,omitempty"`

	// IdentityNameFormat is the name of the identities that automatic
	// enrollment creates. It may contain [caName], [caId], [commonName],
	// [requestedName] and [identityId].
	// +optional
	// +kubebuilder:default="[caName]-[commonName]"
	// +kubebuilder:validation:MinLength=1
	IdentityNameFormat *string `json:"identityNameFormat,omitempty"`

	// ExternalIDClaim takes the external ID of an identity from its
	// certificate. Without it, identities are recognized by the fingerprint
	// of their certificate.
	// +optional
	ExternalIDClaim *ExternalIDClaim `json:"externalIdClaim,omitempty"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ExternalIDClaim defines where in a client certificate the external ID of
// an identity is found.
// +kubebuilder:validation:XValidation:rule="self.location == 'SAN_URI' ? self.matcher in ['ALL', 'SCHEME'] : self.matcher != 'SCHEME'",message="the SCHEME matcher is for the SAN_URI location, which takes no PREFIX or SUFFIX matcher"
// +kubebuilder:validation:XValidation:rule="self.matcher == 'ALL' || (has(self.matcherCriteria) && size(self.matcherCriteria) > 0)",message="matcherCriteria is required unless the matcher is ALL"
// +kubebuilder:validation:XValidation:rule="self.parser == 'NONE' || (has(self.parserCriteria) && size(self.parserCriteria) > 0)",message="parserCriteria is required for the SPLIT parser"
type ExternalIDClaim struct {
	// Location of the values in the certificate: its common name, or the
	// URIs or email addresses of its subject alternative names.
	// +optional
	// +kubebuilder:default=COMMON_NAME
	// +kubebuilder:validation:Enum=COMMON_NAME;SAN_URI;SAN_EMAIL
	Location string `json:"location,omitempty"`

	// Matcher selects the values to consider: ALL of them, those with a
	// PREFIX or a SUFFIX, or the URIs with a SCHEME.
	// +optional
	// +kubebuilder:default=ALL
	// +kubebuilder:validation:Enum=ALL;PREFIX;SUFFIX;SCHEME
	Matcher string `json:"matcher,omitempty"`

	// MatcherCriteria is the prefix, suffix or scheme the matcher looks for.
	// +optional
	MatcherCriteria string `json:"matcherCriteria,omitempty"`

	// Parser turns a selected value into external IDs: NONE takes it as is,
	// SPLIT splits it at a separator.
	// +optional
	// +kubebuilder:default=NONE
	// +kubebuilder:validation:Enum=NONE;SPLIT
	Parser string `json:"parser,omitempty"`

	// ParserCriteria is the separator of the SPLIT parser.
	// +optional
	ParserCriteria string `json:"parserCriteria,omitempty"`

	// Index of the external ID to use among the ones found, starting at zero.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Index int64 `json:"index,omitempty"`
}

// CertificateAuthorityStatus defines the observed state of a
// CertificateAuthority.
type CertificateAuthorityStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 CertificateAuthorityObservation `json:"atProvider,omitempty"`
}

// CertificateAuthorityObservation keeps the observed state.
type CertificateAuthorityObservation struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	// Fingerprint is the SHA-1 fingerprint of the certificate.
	Fingerprint string `json:"fingerprint,omitempty"`
	// IsVerified is true once someone has proven to Ziti that they hold the
	// private key of the certificate authority. Only then does Ziti accept
	// its certificates for authentication and automatic enrollment.
	IsVerified bool `json:"isVerified,omitempty"`
	// VerificationToken is the common name of the certificate that proves it:
	// a certificate with this common name, signed by the certificate
	// authority, is posted to the verify endpoint of the Ziti API.
	VerificationToken         string            `json:"verificationToken,omitempty"`
	IsAuthEnabled             bool              `json:"isAuthEnabled,omitempty"`
	IsOttCaEnrollmentEnabled  bool              `json:"isOttCaEnrollmentEnabled,omitempty"`
	IsAutoCaEnrollmentEnabled bool              `json:"isAutoCaEnrollmentEnabled,omitempty"`
	IdentityRoles             []string          `json:"identityRoles,omitempty"`
	IdentityNameFormat        string            `json:"identityNameFormat,omitempty"`
	ExternalIDClaim           *ExternalIDClaim  `json:"externalIdClaim,omitempty"`
	Tags                      map[string]string `json:"tags,omitempty"`
	CreatedAt                 string            `json:"createdAt,omitempty"`
	UpdatedAt                 string            `json:"updatedAt,omitempty"`
}

// CertificateAuthority is a third-party certificate authority whose
// certificates Ziti accepts for enrollment and authentication. It is ready
// when it exists in Ziti as declared, verified or not: only the holder of
// its private key can verify it, see status.atProvider.verificationToken.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="VERIFIED",type="boolean",JSONPath=`.status.atProvider.isVerified`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type CertificateAuthority struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CertificateAuthoritySpec   `json:"spec"`
	Status CertificateAuthorityStatus `json:"status,omitempty"`
}

// CertificateAuthorityList contains a list of CertificateAuthority.
// +kubebuilder:object:root=true
type CertificateAuthorityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CertificateAuthority `json:"items"`
}
