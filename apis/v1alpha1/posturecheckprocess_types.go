package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostureCheckProcessSpec defines the desired state of a PostureCheckProcess.
type PostureCheckProcessSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              PostureCheckProcessParameters `json:"forProvider"`
}

// Process defines a process that must be running to pass a posture check.
type Process struct {
	// OsType is the operating system the process runs on.
	// +kubebuilder:validation:Enum=Windows;WindowsServer;Android;iOS;Linux;macOS
	OsType string `json:"osType"`
	// Path of the executable.
	Path string `json:"path"`
	// Hashes are the SHA-512 hashes the executable may have. Any executable
	// passes if there are none. Ziti keeps them as a set, so their order and
	// duplicates do not matter.
	// +optional
	Hashes []HexString `json:"hashes,omitempty"`
	// SignerFingerprint is the SHA-1 fingerprint of the certificate the
	// executable must be signed with.
	// +optional
	SignerFingerprint OptionalHexString `json:"signerFingerprint,omitempty"`
}

// PostureCheckProcessParameters define the desired state of a Process Posture
// Check.
type PostureCheckProcessParameters struct {
	// Name of the posture check.
	Name string `json:"name"`
	// Process that must be running.
	Process Process `json:"process"`
	// RoleAttributes are the attributes service policies select the posture
	// check by, as "#attribute".
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PostureCheckProcessStatus defines the observed state of a PostureCheckProcess.
type PostureCheckProcessStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 PostureCheckProcessObservation `json:"atProvider,omitempty"`
}

// ProcessObservation is a process as Ziti reports it, with its hashes and
// fingerprint in lower case and without separators.
type ProcessObservation struct {
	OsType            string   `json:"osType,omitempty"`
	Path              string   `json:"path,omitempty"`
	Hashes            []string `json:"hashes,omitempty"`
	SignerFingerprint string   `json:"signerFingerprint,omitempty"`
}

// PostureCheckProcessObservation keeps the observed state.
type PostureCheckProcessObservation struct {
	ID             string              `json:"id,omitempty"`
	Name           string              `json:"name,omitempty"`
	TypeID         string              `json:"typeId,omitempty"`
	Process        *ProcessObservation `json:"process,omitempty"`
	RoleAttributes []string            `json:"roleAttributes,omitempty"`
	Tags           map[string]string   `json:"tags,omitempty"`
	CreatedAt      string              `json:"createdAt,omitempty"`
	UpdatedAt      string              `json:"updatedAt,omitempty"`
}

// PostureCheckProcess is a Ziti posture check that a device passes while the
// process runs on it.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type PostureCheckProcess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostureCheckProcessSpec   `json:"spec"`
	Status PostureCheckProcessStatus `json:"status,omitempty"`
}

// PostureCheckProcessList contains a list of PostureCheckProcess.
// +kubebuilder:object:root=true
type PostureCheckProcessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostureCheckProcess `json:"items"`
}
