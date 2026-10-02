package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IdentitySpec defines the desired state of an Identity.
type IdentitySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              IdentityParameters `json:"forProvider"`
}

// IdentityParameters define the desired state of a Ziti Identity.
type IdentityParameters struct {
	// Name of the identity.
	Name string `json:"name"`
	// Type of the identity. It cannot be changed after creation.
	// +optional
	// +kubebuilder:default=Default
	// +kubebuilder:validation:Enum=Default;User;Device;Service
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type cannot be changed after creation"
	Type string `json:"type,omitempty"`
	// IsAdmin grants admin privileges to the identity.
	// +optional
	// +kubebuilder:default=false
	IsAdmin *bool `json:"isAdmin,omitempty"`
	// RoleAttributes for the identity.
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// IdentityStatus defines the observed state of an Identity.
type IdentityStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 IdentityObservation `json:"atProvider,omitempty"`
}

// IdentityObservation keeps the observed state.
type IdentityObservation struct {
	ID             string            `json:"id,omitempty"`
	Name           string            `json:"name,omitempty"`
	TypeID         string            `json:"typeId,omitempty"`
	IsAdmin        bool              `json:"isAdmin,omitempty"`
	RoleAttributes []string          `json:"roleAttributes,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	// Enrolled is true once the one-time enrollment token has been used.
	Enrolled bool `json:"enrolled,omitempty"`
	// EnrollmentExpiresAt is when the one-time enrollment token expires.
	EnrollmentExpiresAt string `json:"enrollmentExpiresAt,omitempty"`
	CreatedAt           string `json:"createdAt,omitempty"`
	UpdatedAt           string `json:"updatedAt,omitempty"`
}

// Identity is a Ziti identity with one-time token (OTT) enrollment. The
// enrollment token is published to the connection secret under the key
// enrollmentToken until the identity enrolls.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type Identity struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IdentitySpec   `json:"spec"`
	Status IdentityStatus `json:"status,omitempty"`
}

// IdentityList contains a list of Identity.
// +kubebuilder:object:root=true
type IdentityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Identity `json:"items"`
}
