package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostureCheckMultiProcessSpec defines the desired state of a
// PostureCheckMultiProcess.
type PostureCheckMultiProcessSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              PostureCheckMultiProcessParameters `json:"forProvider"`
}

// MultiProcess defines one of the processes of a multi process posture check.
type MultiProcess struct {
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
	// SignerFingerprints are the SHA-1 fingerprints of the certificates the
	// executable may be signed with. Any signer passes if there are none.
	// Ziti keeps them as a set, so their order and duplicates do not matter.
	// +optional
	SignerFingerprints []HexString `json:"signerFingerprints,omitempty"`
}

// PostureCheckMultiProcessParameters define the desired state of a Multi
// Process Posture Check.
type PostureCheckMultiProcessParameters struct {
	// Name of the posture check.
	Name string `json:"name"`
	// Semantic says whether all of the processes must be running (AllOf) or
	// any one of them (AnyOf).
	// +optional
	// +kubebuilder:default=AllOf
	// +kubebuilder:validation:Enum=AllOf;AnyOf
	Semantic string `json:"semantic,omitempty"`
	// Processes to look for. Ziti identifies a process by its operating
	// system and path, so each pair may be listed once, and their order does
	// not matter.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=osType
	// +listMapKey=path
	Processes []MultiProcess `json:"processes"`
	// RoleAttributes for the posture check.
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PostureCheckMultiProcessStatus defines the observed state of a
// PostureCheckMultiProcess.
type PostureCheckMultiProcessStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 PostureCheckMultiProcessObservation `json:"atProvider,omitempty"`
}

// MultiProcessObservation is a process as Ziti reports it, with its hashes
// and fingerprints in lower case and without separators.
type MultiProcessObservation struct {
	OsType             string   `json:"osType,omitempty"`
	Path               string   `json:"path,omitempty"`
	Hashes             []string `json:"hashes,omitempty"`
	SignerFingerprints []string `json:"signerFingerprints,omitempty"`
}

// PostureCheckMultiProcessObservation keeps the observed state.
type PostureCheckMultiProcessObservation struct {
	ID             string                    `json:"id,omitempty"`
	Name           string                    `json:"name,omitempty"`
	TypeID         string                    `json:"typeId,omitempty"`
	Semantic       string                    `json:"semantic,omitempty"`
	Processes      []MultiProcessObservation `json:"processes,omitempty"`
	RoleAttributes []string                  `json:"roleAttributes,omitempty"`
	Tags           map[string]string         `json:"tags,omitempty"`
	CreatedAt      string                    `json:"createdAt,omitempty"`
	UpdatedAt      string                    `json:"updatedAt,omitempty"`
}

// PostureCheckMultiProcess is the top level Ziti Multi Process Posture Check resource.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type PostureCheckMultiProcess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostureCheckMultiProcessSpec   `json:"spec"`
	Status PostureCheckMultiProcessStatus `json:"status,omitempty"`
}

// PostureCheckMultiProcessList contains a list of PostureCheckMultiProcess.
// +kubebuilder:object:root=true
type PostureCheckMultiProcessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostureCheckMultiProcess `json:"items"`
}
