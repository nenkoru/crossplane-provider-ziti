package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PortRange defines a range of ports.
type PortRange struct {
	// Low is the first port of the range.
	Low int32 `json:"low"`
	// High is the last port of the range.
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

	// Addresses are the host names, IP addresses and CIDR ranges the tunnelers
	// of clients intercept. A name that starts with "*." stands for a whole
	// domain.
	Addresses []string `json:"addresses"`

	// Protocols the tunnelers of clients intercept: tcp, udp.
	Protocols []string `json:"protocols"`

	// PortRanges are the ports the tunnelers of clients intercept.
	PortRanges []PortRange `json:"portRanges"`

	// DialOptions say how the tunnelers of clients dial the service.
	// +optional
	DialOptions *DialOptions `json:"dialOptions,omitempty"`

	// SourceIP is the address the hosting tunneler connects from, for a
	// destination that must see where the client is. The host config of the
	// service must allow it in allowedSourceAddresses.
	// +optional
	SourceIP string `json:"sourceIp,omitempty"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DialOptions defines dial options for intercepted connections.
type DialOptions struct {
	// ConnectTimeoutSeconds is how long the tunneler of a client waits for the
	// service to accept a connection.
	// +optional
	ConnectTimeoutSeconds *int32 `json:"connectTimeoutSeconds,omitempty"`
	// Identity is the name of the hosting identity to dial, for a service
	// whose hosts set listenOptions.bindUsingEdgeIdentity.
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

// ConfigInterceptV1 is a Ziti config of type intercept.v1: the addresses and
// ports the tunnelers of clients intercept for a service.
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
