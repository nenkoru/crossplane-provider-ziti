package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PortRange defines a range of ports.
type PortRange struct {
	// Low is the low port number.
	Low int32 `json:"low"`
	// High is the high port number.
	High int32 `json:"high"`
}

// ConfigInterceptV1Spec specifies the desired state of a Ziti Intercept V1 Config.
type ConfigInterceptV1Spec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ConfigInterceptV1Parameters `json:"forProvider"`
}

// ConfigInterceptV1Status represents the observed state of a Ziti Intercept V1 Config.
type ConfigInterceptV1Status struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ConfigInterceptV1Observation `json:"atProvider,omitempty"`
}

// ConfigInterceptV1Parameters define the desired state of a Ziti Intercept V1 Config.
type ConfigInterceptV1Parameters struct {
	// Name of the config.
	Name string `json:"name"`

	// Addresses is a list of domain addresses to intercept.
	Addresses []string `json:"addresses"`

	// Protocols is a list of protocols to intercept (tcp, udp).
	Protocols []string `json:"protocols"`

	// PortRanges is a list of port ranges to intercept.
	PortRanges []PortRange `json:"portRanges"`

	// DialOptions is the dial options for the intercepted connection.
	// +optional
	DialOptions *DialOptions `json:"dialOptions,omitempty"`

	// SourceIP is the source IP to use for the intercepted connection.
	// +optional
	SourceIP string `json:"sourceIp,omitempty"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DialOptions defines dial options for intercepted connections.
type DialOptions struct {
	// ConnectTimeoutSeconds is the connection timeout in seconds.
	// +optional
	ConnectTimeoutSeconds *int32 `json:"connectTimeoutSeconds,omitempty"`
	// Identity is the identity to use for dialing.
	// +optional
	Identity *string `json:"identity,omitempty"`
}

// ConfigInterceptV1Observation represents the observed state of a Ziti Intercept V1 Config.
type ConfigInterceptV1Observation struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Addresses   []string          `json:"addresses,omitempty"`
	Protocols   []string          `json:"protocols,omitempty"`
	PortRanges  []PortRange       `json:"portRanges,omitempty"`
	DialOptions *DialOptions      `json:"dialOptions,omitempty"`
	SourceIP    *string           `json:"sourceIp,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	CreatedAt   string            `json:"createdAt,omitempty"`
	UpdatedAt   string            `json:"updatedAt,omitempty"`
}

// ConfigInterceptV1 is the Schema for the ConfigInterceptV1s API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type=string,JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
// +genclient
type ConfigInterceptV1 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigInterceptV1Spec   `json:"spec"`
	Status ConfigInterceptV1Status `json:"status,omitempty"`
}

// ConfigInterceptV1List contains a list of ConfigInterceptV1.
// +kubebuilder:object:root=true
type ConfigInterceptV1List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigInterceptV1 `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigInterceptV1{}, &ConfigInterceptV1List{})
}
