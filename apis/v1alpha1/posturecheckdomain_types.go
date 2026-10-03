package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostureCheckDomainSpec defines the desired state of a PostureCheckDomain.
type PostureCheckDomainSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              PostureCheckDomainParameters `json:"forProvider"`
}

// PostureCheckDomainParameters define the desired state of a Windows Domain
// Posture Check.
type PostureCheckDomainParameters struct {
	// Name of the posture check.
	Name string `json:"name"`
	// Domains are the Windows domains that pass the check: a device must be
	// joined to one of them. Ziti keeps them as a set, so their order and
	// duplicates do not matter.
	// +kubebuilder:validation:MinItems=1
	Domains []string `json:"domains"`
	// RoleAttributes are the attributes service policies select the posture
	// check by, as "#attribute".
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`
	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PostureCheckDomainStatus defines the observed state of a PostureCheckDomain.
type PostureCheckDomainStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 PostureCheckDomainObservation `json:"atProvider,omitempty"`
}

// PostureCheckDomainObservation keeps the observed state.
type PostureCheckDomainObservation struct {
	ID             string            `json:"id,omitempty"`
	Name           string            `json:"name,omitempty"`
	TypeID         string            `json:"typeId,omitempty"`
	Domains        []string          `json:"domains,omitempty"`
	RoleAttributes []string          `json:"roleAttributes,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	CreatedAt      string            `json:"createdAt,omitempty"`
	UpdatedAt      string            `json:"updatedAt,omitempty"`
}

// PostureCheckDomain is a Ziti posture check that a Windows device passes if
// it is joined to one of the listed domains.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type PostureCheckDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostureCheckDomainSpec   `json:"spec"`
	Status PostureCheckDomainStatus `json:"status,omitempty"`
}

// PostureCheckDomainList contains a list of PostureCheckDomain.
// +kubebuilder:object:root=true
type PostureCheckDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostureCheckDomain `json:"items"`
}
