package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostureCheckMFASpec defines the desired state of a PostureCheckMFA.
type PostureCheckMFASpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              PostureCheckMFAParameters `json:"forProvider"`
}

// PostureCheckMFAParameters define the desired state of an MFA Posture Check.
type PostureCheckMFAParameters struct {
	// Name of the posture check.
	Name string `json:"name"`
	// RoleAttributes for the posture check.
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	// TimeoutSeconds for the MFA check. -1 means no timeout.
	// +optional
	// +kubebuilder:default=-1
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`
}

// PostureCheckMFAStatus defines the observed state of a PostureCheckMFA.
type PostureCheckMFAStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 PostureCheckMFAObservation `json:"atProvider,omitempty"`
}

// PostureCheckMFAObservation keeps the observed state.
type PostureCheckMFAObservation struct {
	ID             string   `json:"id,omitempty"`
	Name           string   `json:"name,omitempty"`
	Type           string   `json:"type,omitempty"`
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	TimeoutSeconds int64    `json:"timeoutSeconds,omitempty"`
	CreatedAt      string   `json:"createdAt,omitempty"`
	UpdatedAt      string   `json:"updatedAt,omitempty"`
}

// PostureCheckMFA is the top level Ziti MFA Posture Check resource.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type PostureCheckMFA struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostureCheckMFASpec   `json:"spec"`
	Status PostureCheckMFAStatus `json:"status,omitempty"`
}

// PostureCheckMFAList contains a list of PostureCheckMFA.
// +kubebuilder:object:root=true
type PostureCheckMFAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostureCheckMFA `json:"items"`
}
