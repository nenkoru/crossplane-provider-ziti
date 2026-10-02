package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthPolicySpec defines the desired state of an AuthPolicy.
type AuthPolicySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              AuthPolicyParameters `json:"forProvider"`
}

// AuthPolicyParameters define the desired state of a Ziti Auth Policy.
type AuthPolicyParameters struct {
	// Name of the auth policy.
	Name string `json:"name"`
	// Primary authentication methods. A method that is not set is not allowed.
	// +optional
	Primary *AuthMethods `json:"primary,omitempty"`
	// Secondary authentication factors.
	// +optional
	Secondary *SecondaryAuth `json:"secondary,omitempty"`
	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// AuthMethods defines the primary authentication methods.
type AuthMethods struct {
	// Cert-based authentication.
	// +optional
	Cert *CertAuth `json:"cert,omitempty"`
	// Password (UPDB) authentication.
	// +optional
	UPDB *UPDBAuth `json:"updb,omitempty"`
	// External JWT authentication.
	// +optional
	ExtJWT *ExtJWTAuth `json:"extJwt,omitempty"`
}

// CertAuth defines certificate-based authentication settings.
type CertAuth struct {
	// Allowed enables cert authentication.
	// +optional
	Allowed bool `json:"allowed,omitempty"`
	// AllowExpiredCerts allows expired certificates.
	// +optional
	AllowExpiredCerts bool `json:"allowExpiredCerts,omitempty"`
}

// UPDBAuth defines password-based authentication settings.
type UPDBAuth struct {
	// Allowed enables password authentication.
	// +optional
	Allowed bool `json:"allowed,omitempty"`
	// MinPasswordLength is the minimum password length.
	// +optional
	// +kubebuilder:default=5
	MinPasswordLength *int64 `json:"minPasswordLength,omitempty"`
	// MaxAttempts is the number of failed logins before lockout.
	// +optional
	// +kubebuilder:default=5
	MaxAttempts *int64 `json:"maxAttempts,omitempty"`
	// LockoutDurationMinutes is the duration of a lockout in minutes. Zero
	// locks the identity until an administrator unlocks it.
	// +optional
	// +kubebuilder:default=0
	LockoutDurationMinutes *int64 `json:"lockoutDurationMinutes,omitempty"`
	// RequireMixedCase requires passwords to have upper and lower case letters.
	// +optional
	RequireMixedCase bool `json:"requireMixedCase,omitempty"`
	// RequireNumberChar requires passwords to have a digit.
	// +optional
	RequireNumberChar bool `json:"requireNumberChar,omitempty"`
	// RequireSpecialChar requires passwords to have a special character.
	// +optional
	RequireSpecialChar bool `json:"requireSpecialChar,omitempty"`
}

// ExtJWTAuth defines external JWT authentication settings.
type ExtJWTAuth struct {
	// Allowed enables external JWT authentication.
	// +optional
	Allowed bool `json:"allowed,omitempty"`
	// AllowedSigners is the list of IDs of the allowed external JWT signers.
	// +optional
	AllowedSigners []string `json:"allowedSigners,omitempty"`
}

// SecondaryAuth defines the secondary authentication factors.
type SecondaryAuth struct {
	// RequireTOTP requires a time-based one-time password.
	// +optional
	RequireTOTP bool `json:"requireTotp,omitempty"`
	// RequireExtJWTSigner is the ID of an external JWT signer whose token is
	// required in addition to the primary method.
	// +optional
	RequireExtJWTSigner *string `json:"requireExtJwtSigner,omitempty"`
}

// AuthPolicyStatus defines the observed state of an AuthPolicy.
type AuthPolicyStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 AuthPolicyObservation `json:"atProvider,omitempty"`
}

// AuthPolicyObservation keeps the observed state.
type AuthPolicyObservation struct {
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Primary   *AuthMethods      `json:"primary,omitempty"`
	Secondary *SecondaryAuth    `json:"secondary,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	CreatedAt string            `json:"createdAt,omitempty"`
	UpdatedAt string            `json:"updatedAt,omitempty"`
}

// AuthPolicy is the top level Ziti Auth Policy resource.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type AuthPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthPolicySpec   `json:"spec"`
	Status AuthPolicyStatus `json:"status,omitempty"`
}

// AuthPolicyList contains a list of AuthPolicy.
// +kubebuilder:object:root=true
type AuthPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthPolicy `json:"items"`
}
