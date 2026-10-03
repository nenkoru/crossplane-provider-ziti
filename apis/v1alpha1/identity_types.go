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

// IdentityParameters define the desired state of a Ziti Identity. All
// identity kinds share them; the kinds differ in how the identity enrolls.
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
	// RoleAttributes are the attributes policies select the identity by, as
	// "#attribute".
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	// AuthPolicyID is the name or ID of the auth policy that governs how the
	// identity authenticates. Ziti assigns its default auth policy when this
	// is not set.
	// +optional
	AuthPolicyID *string `json:"authPolicyId,omitempty"`
	// ExternalID identifies the identity to an external JWT signer.
	// +optional
	ExternalID *string `json:"externalId,omitempty"`
	// EnrollmentDuration is how long an enrollment token is valid that
	// replaces one that expired or is gone, 180m when this is not set. It
	// does not apply to the first token, which is valid for as long as the
	// Ziti controller is configured to make it, and an identity without an
	// enrollment has no use for it.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('5m')",message="enrollmentDuration must be at least 5m"
	EnrollmentDuration *metav1.Duration `json:"enrollmentDuration,omitempty"`
	// DefaultHostingCost is the cost of the terminators the identity hosts.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	DefaultHostingCost *int32 `json:"defaultHostingCost,omitempty"`
	// DefaultHostingPrecedence is the precedence of the terminators the
	// identity hosts.
	// +optional
	// +kubebuilder:validation:Enum=default;required;failed
	DefaultHostingPrecedence *string `json:"defaultHostingPrecedence,omitempty"`
	// ServiceHostingCosts overrides the hosting cost per service. Keys are
	// service names or IDs.
	// +optional
	ServiceHostingCosts map[string]int32 `json:"serviceHostingCosts,omitempty"`
	// ServiceHostingPrecedences overrides the hosting precedence (default,
	// required, failed) per service. Keys are service names or IDs.
	// +optional
	ServiceHostingPrecedences map[string]string `json:"serviceHostingPrecedences,omitempty"`
	// AppData is a map of data for the applications that use the identity.
	// +optional
	AppData map[string]string `json:"appData,omitempty"`
	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// IdentityStatus defines the observed state of an Identity.
type IdentityStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 IdentityObservation `json:"atProvider,omitempty"`
}

// IdentityObservation keeps the observed state of an identity of any kind.
type IdentityObservation struct {
	ID                        string            `json:"id,omitempty"`
	Name                      string            `json:"name,omitempty"`
	TypeID                    string            `json:"typeId,omitempty"`
	IsAdmin                   bool              `json:"isAdmin,omitempty"`
	RoleAttributes            []string          `json:"roleAttributes,omitempty"`
	AuthPolicyID              string            `json:"authPolicyId,omitempty"`
	ExternalID                string            `json:"externalId,omitempty"`
	DefaultHostingCost        int32             `json:"defaultHostingCost,omitempty"`
	DefaultHostingPrecedence  string            `json:"defaultHostingPrecedence,omitempty"`
	ServiceHostingCosts       map[string]int32  `json:"serviceHostingCosts,omitempty"`
	ServiceHostingPrecedences map[string]string `json:"serviceHostingPrecedences,omitempty"`
	AppData                   map[string]string `json:"appData,omitempty"`
	Tags                      map[string]string `json:"tags,omitempty"`
	// Enrolled is true once the identity has used its enrollment token.
	Enrolled bool `json:"enrolled,omitempty"`
	// EnrollmentExpiresAt is when the enrollment token expires.
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
