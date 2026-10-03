package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HexString is a hexadecimal value of a posture check: a MAC address, the
// hash of a file or the fingerprint of a certificate. Its digits may be in
// either case and separated by colons, dashes, dots or spaces. Ziti stores
// the value in lower case and without separators, so "00:1A:2B:3C:4D:5E" and
// "001a2b3c4d5e" are the same value.
// +kubebuilder:validation:Pattern=`^[0-9A-Fa-f]+([ :.-][0-9A-Fa-f]+)*$`
type HexString string

// OptionalHexString is a HexString of a field that may be left out. It may
// also be empty, which is the same and clears the value in Ziti.
// +kubebuilder:validation:Pattern=`^([0-9A-Fa-f]+([ :.-][0-9A-Fa-f]+)*)?$`
type OptionalHexString string

// PostureCheckMacSpec defines the desired state of a PostureCheckMac.
type PostureCheckMacSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              PostureCheckMacParameters `json:"forProvider"`
}

// PostureCheckMacParameters define the desired state of a MAC Address Posture
// Check.
type PostureCheckMacParameters struct {
	// Name of the posture check.
	Name string `json:"name"`
	// MacAddresses that pass the check: a device must have one of them. Ziti
	// keeps them as a set, so their order and duplicates do not matter.
	// +kubebuilder:validation:MinItems=1
	MacAddresses []HexString `json:"macAddresses"`
	// RoleAttributes for the posture check.
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PostureCheckMacStatus defines the observed state of a PostureCheckMac.
type PostureCheckMacStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 PostureCheckMacObservation `json:"atProvider,omitempty"`
}

// PostureCheckMacObservation keeps the observed state.
type PostureCheckMacObservation struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	TypeID string `json:"typeId,omitempty"`
	// MacAddresses as Ziti stores them: in lower case and without separators.
	MacAddresses   []string          `json:"macAddresses,omitempty"`
	RoleAttributes []string          `json:"roleAttributes,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	CreatedAt      string            `json:"createdAt,omitempty"`
	UpdatedAt      string            `json:"updatedAt,omitempty"`
}

// PostureCheckMac is the top level Ziti MAC Address Posture Check resource.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type PostureCheckMac struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostureCheckMacSpec   `json:"spec"`
	Status PostureCheckMacStatus `json:"status,omitempty"`
}

// PostureCheckMacList contains a list of PostureCheckMac.
// +kubebuilder:object:root=true
type PostureCheckMacList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostureCheckMac `json:"items"`
}
