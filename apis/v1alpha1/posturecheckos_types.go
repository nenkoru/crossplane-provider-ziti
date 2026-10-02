package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostureCheckOSSpec defines the desired state of a PostureCheckOS.
type PostureCheckOSSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              PostureCheckOSParameters `json:"forProvider"`
}

// OperatingSystem defines an OS posture check constraint.
type OperatingSystem struct {
	// Type of operating system.
	// +kubebuilder:validation:Enum=Windows;WindowsServer;Android;iOS;Linux;macOS
	Type string `json:"type"`
	// Versions constraint (e.g., [">=10.0.0"]). Ziti keeps them as a set, so
	// their order and duplicates do not matter.
	// +optional
	Versions []string `json:"versions,omitempty"`
}

// PostureCheckOSParameters define the desired state of an OS Posture Check.
type PostureCheckOSParameters struct {
	// Name of the posture check.
	Name string `json:"name"`
	// OperatingSystems that pass the check. Ziti keeps one entry per type, so
	// each type may be listed once, and their order does not matter.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=type
	OperatingSystems []OperatingSystem `json:"operatingSystems"`
	// RoleAttributes for the posture check.
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PostureCheckOSStatus defines the observed state of a PostureCheckOS.
type PostureCheckOSStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 PostureCheckOSObservation `json:"atProvider,omitempty"`
}

// PostureCheckOSObservation keeps the observed state.
type PostureCheckOSObservation struct {
	ID               string            `json:"id,omitempty"`
	Name             string            `json:"name,omitempty"`
	TypeID           string            `json:"typeId,omitempty"`
	OperatingSystems []OperatingSystem `json:"operatingSystems,omitempty"`
	RoleAttributes   []string          `json:"roleAttributes,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	CreatedAt        string            `json:"createdAt,omitempty"`
	UpdatedAt        string            `json:"updatedAt,omitempty"`
}

// PostureCheckOS is the top level Ziti OS Posture Check resource.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type PostureCheckOS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostureCheckOSSpec   `json:"spec"`
	Status PostureCheckOSStatus `json:"status,omitempty"`
}

// PostureCheckOSList contains a list of PostureCheckOS.
// +kubebuilder:object:root=true
type PostureCheckOSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostureCheckOS `json:"items"`
}
