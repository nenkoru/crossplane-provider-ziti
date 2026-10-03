package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EdgeRouterSpec specifies the desired state of a Ziti Edge Router.
type EdgeRouterSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              EdgeRouterParameters `json:"forProvider"`
}

// EdgeRouterStatus represents the observed state of a Ziti Edge Router.
type EdgeRouterStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 EdgeRouterObservation `json:"atProvider,omitempty"`
}

// EdgeRouterParameters define the desired state of a Ziti Edge Router.
type EdgeRouterParameters struct {
	// Name of the edge router.
	Name string `json:"name"`

	// IsTunnelerEnabled lets the router intercept and host services itself,
	// like a tunneler. Ziti creates an identity with the name of the router
	// for that.
	// +optional
	IsTunnelerEnabled *bool `json:"isTunnelerEnabled,omitempty"`

	// NoTraversal keeps traffic of other routers from passing through the
	// router: circuits may only start or end at it.
	// +optional
	NoTraversal *bool `json:"noTraversal,omitempty"`

	// Cost of a route through the router: Ziti prefers cheaper routes.
	// +optional
	Cost *int64 `json:"cost,omitempty"`

	// RoleAttributes are the attributes policies select the edge router by, as
	// "#attribute".
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`

	// Tags are free-form labels Ziti stores with the edge router. They do not
	// affect access or routing.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EdgeRouterObservation represents the observed state of a Ziti Edge Router.
type EdgeRouterObservation struct {
	ID                string            `json:"id,omitempty"`
	Name              string            `json:"name,omitempty"`
	IsTunnelerEnabled bool              `json:"isTunnelerEnabled,omitempty"`
	NoTraversal       bool              `json:"noTraversal,omitempty"`
	Cost              int64             `json:"cost,omitempty"`
	RoleAttributes    []string          `json:"roleAttributes,omitempty"`
	Tags              map[string]string `json:"tags,omitempty"`
	// IsVerified is true once the edge router has enrolled.
	IsVerified bool `json:"isVerified,omitempty"`
	// EnrollmentExpiresAt is when the enrollment token expires.
	EnrollmentExpiresAt string `json:"enrollmentExpiresAt,omitempty"`
	CreatedAt           string `json:"createdAt,omitempty"`
	UpdatedAt           string `json:"updatedAt,omitempty"`
}

// EdgeRouter is a Ziti edge router. Its enrollment token is published to the
// connection secret under the key enrollmentToken until the router enrolls.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type=string,JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
// +genclient
type EdgeRouter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EdgeRouterSpec   `json:"spec"`
	Status EdgeRouterStatus `json:"status,omitempty"`
}

// EdgeRouterList contains a list of EdgeRouter.
// +kubebuilder:object:root=true
type EdgeRouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EdgeRouter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EdgeRouter{}, &EdgeRouterList{})
}
