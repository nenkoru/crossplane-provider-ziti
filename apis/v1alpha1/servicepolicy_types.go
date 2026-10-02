package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServicePolicyType represents the type of service policy.
type ServicePolicyType string

const (
	// ServicePolicyTypeDial is a Dial policy.
	ServicePolicyTypeDial ServicePolicyType = "Dial"
	// ServicePolicyTypeBind is a Bind policy.
	ServicePolicyTypeBind ServicePolicyType = "Bind"
)

// ServicePolicySemantic represents the semantic of the policy.
type ServicePolicySemantic string

const (
	// ServicePolicySemanticAnyOf matches any of the roles.
	ServicePolicySemanticAnyOf ServicePolicySemantic = "AnyOf"
	// ServicePolicySemanticAllOf matches all of the roles.
	ServicePolicySemanticAllOf ServicePolicySemantic = "AllOf"
)

// ServicePolicySpec specifies the desired state of a Ziti Service Policy.
type ServicePolicySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServicePolicyParameters `json:"forProvider"`
}

// ServicePolicyStatus represents the observed state of a Ziti Service Policy.
type ServicePolicyStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServicePolicyObservation `json:"atProvider,omitempty"`
}

// ServicePolicyParameters define the desired state of a Ziti Service Policy.
type ServicePolicyParameters struct {
	// Name of the policy.
	Name string `json:"name"`

	// Type is the type of policy (Dial or Bind).
	// +kubebuilder:validation:Enum=Dial;Bind
	Type ServicePolicyType `json:"type"`

	// Semantic is the matching semantic (AnyOf or AllOf).
	// +kubebuilder:validation:Enum=AnyOf;AllOf
	Semantic ServicePolicySemantic `json:"semantic"`

	// ServiceRoles is a list of service role selectors.
	ServiceRoles []string `json:"serviceRoles,omitempty"`

	// IdentityRoles is a list of identity role selectors.
	IdentityRoles []string `json:"identityRoles,omitempty"`

	// PostureCheckRoles is a list of posture check role selectors.
	// +optional
	PostureCheckRoles []string `json:"postureCheckRoles,omitempty"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ServicePolicyObservation represents the observed state of a Ziti Service Policy.
type ServicePolicyObservation struct {
	ID                string            `json:"id,omitempty"`
	Name              string            `json:"name,omitempty"`
	Type              string            `json:"type,omitempty"`
	Semantic          string            `json:"semantic,omitempty"`
	ServiceRoles      []string          `json:"serviceRoles,omitempty"`
	IdentityRoles     []string          `json:"identityRoles,omitempty"`
	PostureCheckRoles []string          `json:"postureCheckRoles,omitempty"`
	Tags              map[string]string `json:"tags,omitempty"`
	CreatedAt         string            `json:"createdAt,omitempty"`
	UpdatedAt         string            `json:"updatedAt,omitempty"`
}

// ServicePolicy is the Schema for the ServicePolicies API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type=string,JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.forProvider.type`
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
// +genclient
type ServicePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServicePolicySpec   `json:"spec"`
	Status ServicePolicyStatus `json:"status,omitempty"`
}

// ServicePolicyList contains a list of ServicePolicy.
// +kubebuilder:object:root=true
type ServicePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServicePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServicePolicy{}, &ServicePolicyList{})
}
