package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceEdgeRouterPolicySemantic represents the semantic of the policy.
type ServiceEdgeRouterPolicySemantic string

const (
	// ServiceEdgeRouterPolicySemanticAnyOf matches any of the roles.
	ServiceEdgeRouterPolicySemanticAnyOf ServiceEdgeRouterPolicySemantic = "AnyOf"
	// ServiceEdgeRouterPolicySemanticAllOf matches all of the roles.
	ServiceEdgeRouterPolicySemanticAllOf ServiceEdgeRouterPolicySemantic = "AllOf"
)

// ServiceEdgeRouterPolicySpec specifies the desired state of a Ziti Service Edge Router Policy.
type ServiceEdgeRouterPolicySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceEdgeRouterPolicyParameters `json:"forProvider"`
}

// ServiceEdgeRouterPolicyStatus represents the observed state of a Ziti Service Edge Router Policy.
type ServiceEdgeRouterPolicyStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServiceEdgeRouterPolicyObservation `json:"atProvider,omitempty"`
}

// ServiceEdgeRouterPolicyParameters define the desired state of a Ziti Service Edge Router Policy.
type ServiceEdgeRouterPolicyParameters struct {
	// Name of the policy.
	Name string `json:"name"`

	// Semantic is the matching semantic (AnyOf or AllOf).
	// +optional
	// +kubebuilder:default=AllOf
	// +kubebuilder:validation:Enum=AnyOf;AllOf
	Semantic ServiceEdgeRouterPolicySemantic `json:"semantic,omitempty"`

	// ServiceRoles is a list of service role selectors: "#all", "#attribute" or
	// "@name", where name is the name or the ID of a service. Names are
	// resolved to IDs.
	// +optional
	ServiceRoles []string `json:"serviceRoles,omitempty"`

	// EdgeRouterRoles is a list of edge router role selectors: "#all", "#attribute" or
	// "@name", where name is the name or the ID of an edge router. Names are
	// resolved to IDs.
	// +optional
	EdgeRouterRoles []string `json:"edgeRouterRoles,omitempty"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ServiceEdgeRouterPolicyObservation represents the observed state of a Ziti Service Edge Router Policy.
type ServiceEdgeRouterPolicyObservation struct {
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name,omitempty"`
	Semantic        string            `json:"semantic,omitempty"`
	ServiceRoles    []string          `json:"serviceRoles,omitempty"`
	EdgeRouterRoles []string          `json:"edgeRouterRoles,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
	CreatedAt       string            `json:"createdAt,omitempty"`
	UpdatedAt       string            `json:"updatedAt,omitempty"`
}

// ServiceEdgeRouterPolicy is the Schema for the ServiceEdgeRouterPolicies API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type=string,JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
// +genclient
type ServiceEdgeRouterPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceEdgeRouterPolicySpec   `json:"spec"`
	Status ServiceEdgeRouterPolicyStatus `json:"status,omitempty"`
}

// ServiceEdgeRouterPolicyList contains a list of ServiceEdgeRouterPolicy.
// +kubebuilder:object:root=true
type ServiceEdgeRouterPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceEdgeRouterPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceEdgeRouterPolicy{}, &ServiceEdgeRouterPolicyList{})
}
