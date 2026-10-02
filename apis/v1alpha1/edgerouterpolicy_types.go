package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EdgeRouterPolicySemantic represents the semantic of the policy.
type EdgeRouterPolicySemantic string

const (
	// EdgeRouterPolicySemanticAnyOf matches any of the roles.
	EdgeRouterPolicySemanticAnyOf EdgeRouterPolicySemantic = "AnyOf"
	// EdgeRouterPolicySemanticAllOf matches all of the roles.
	EdgeRouterPolicySemanticAllOf EdgeRouterPolicySemantic = "AllOf"
)

// EdgeRouterPolicySpec specifies the desired state of a Ziti Edge Router Policy.
type EdgeRouterPolicySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              EdgeRouterPolicyParameters `json:"forProvider"`
}

// EdgeRouterPolicyStatus represents the observed state of a Ziti Edge Router Policy.
type EdgeRouterPolicyStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 EdgeRouterPolicyObservation `json:"atProvider,omitempty"`
}

// EdgeRouterPolicyParameters define the desired state of a Ziti Edge Router Policy.
type EdgeRouterPolicyParameters struct {
	// Name of the policy.
	Name string `json:"name"`

	// Semantic is the matching semantic (AnyOf or AllOf).
	// +optional
	// +kubebuilder:default=AllOf
	// +kubebuilder:validation:Enum=AnyOf;AllOf
	Semantic EdgeRouterPolicySemantic `json:"semantic,omitempty"`

	// EdgeRouterRoles is a list of edge router role selectors: "#all", "#attribute" or
	// "@name", where name is the name or the ID of an edge router. Names are
	// resolved to IDs.
	// +optional
	EdgeRouterRoles []string `json:"edgeRouterRoles,omitempty"`

	// IdentityRoles is a list of identity role selectors: "#all", "#attribute" or
	// "@name", where name is the name or the ID of an identity. Names are
	// resolved to IDs.
	// +optional
	IdentityRoles []string `json:"identityRoles,omitempty"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EdgeRouterPolicyObservation represents the observed state of a Ziti Edge Router Policy.
type EdgeRouterPolicyObservation struct {
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name,omitempty"`
	Semantic        string            `json:"semantic,omitempty"`
	EdgeRouterRoles []string          `json:"edgeRouterRoles,omitempty"`
	IdentityRoles   []string          `json:"identityRoles,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
	CreatedAt       string            `json:"createdAt,omitempty"`
	UpdatedAt       string            `json:"updatedAt,omitempty"`
}

// EdgeRouterPolicy is the Schema for the EdgeRouterPolicies API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type=string,JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
// +genclient
type EdgeRouterPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EdgeRouterPolicySpec   `json:"spec"`
	Status EdgeRouterPolicyStatus `json:"status,omitempty"`
}

// EdgeRouterPolicyList contains a list of EdgeRouterPolicy.
// +kubebuilder:object:root=true
type EdgeRouterPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EdgeRouterPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EdgeRouterPolicy{}, &EdgeRouterPolicyList{})
}
