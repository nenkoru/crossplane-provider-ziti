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
	// Primary authentication methods.
	// +optional
	Primary *AuthMethods `json:"primary,omitempty"`
	// Secondary authentication methods.
	// +optional
	Secondary *AuthMethods `json:"secondary,omitempty"`
}

// AuthMethods defines primary or secondary auth methods.
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
	// +kubebuilder:default=true
	Allowed bool `json:"allowed,omitempty"`
	// AllowExpiredCerts allows expired certificates.
	// +optional
	AllowExpiredCerts bool `json:"allowExpiredCerts,omitempty"`
}

// UPDBAuth defines password-based authentication settings.
type UPDBAuth struct {
	// Allowed enables password authentication.
	// +optional
	// +kubebuilder:default=true
	Allowed bool `json:"allowed,omitempty"`
	// MinPasswordLength minimum password length.
	// +optional
	MinPasswordLength int `json:"minPasswordLength,omitempty"`
	// MaxAttempts maximum login attempts before lockout.
	// +optional
	MaxAttempts int `json:"maxAttempts,omitempty"`
	// LockoutDurationMinutes duration of lockout in minutes.
	// +optional
	LockoutDurationMinutes int `json:"lockoutDurationMinutes,omitempty"`
}

// ExtJWTAuth defines external JWT authentication settings.
type ExtJWTAuth struct {
	// Allowed enables external JWT authentication.
	// +optional
	Allowed bool `json:"allowed,omitempty"`
	// AllowedSigners list of allowed JWT signer IDs.
	// +optional
	AllowedSigners []string `json:"allowedSigners,omitempty"`
}

// AuthPolicyStatus defines the observed state of an AuthPolicy.
type AuthPolicyStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 AuthPolicyObservation `json:"atProvider,omitempty"`
}

// AuthPolicyObservation keeps the observed state.
type AuthPolicyObservation struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
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
