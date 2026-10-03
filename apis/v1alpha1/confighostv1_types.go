/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ForwardAddressTranslation defines an address translation rule.
type ForwardAddressTranslation struct {
	// From is the first address of the range clients ask for, as a single
	// IPv4 or IPv6 address without a prefix length, such as 10.0.0.0. Ziti
	// refuses CIDR notation here: prefixLength gives the size of the range.
	From string `json:"from"`
	// To is the first address of the range the hosting tunneler connects to
	// instead, as a single address of the same family as from, such as
	// 192.168.0.0.
	To string `json:"to"`
	// PrefixLength is the length of the prefix of from that is replaced by
	// the prefix of to: the bits after it are kept. At most 32 for IPv4 and
	// 128 for IPv6.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=128
	PrefixLength int32 `json:"prefixLength"`
}

// ListenOptions defines listen options for the host config.
type ListenOptions struct {
	// BindUsingEdgeIdentity offers the service under the name of the hosting
	// identity, so that a client can dial one host among several.
	// +optional
	BindUsingEdgeIdentity *bool `json:"bindUsingEdgeIdentity,omitempty"`

	// ConnectTimeout is how long the hosting tunneler waits for the
	// destination to accept a connection, as a duration such as 5s.
	// +optional
	ConnectTimeout *string `json:"connectTimeout,omitempty"`

	// Cost of the terminator: Ziti prefers cheaper terminators.
	// +optional
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	Cost *int32 `json:"cost,omitempty"`

	// MaxConnections is the number of connections to edge routers the hosting
	// tunneler listens for the service on.
	// +optional
	// +kubebuilder:default=65535
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	MaxConnections *int32 `json:"maxConnections,omitempty"`

	// Precedence of the terminator: one with required is used before one with
	// default, and one with failed only when nothing else is left.
	// +optional
	// +kubebuilder:default=default
	// +kubebuilder:validation:Enum=default;required;failed
	Precedence *string `json:"precedence,omitempty"`
}

// ProxyConfig defines proxy configuration.
type ProxyConfig struct {
	// Address of the proxy, as host:port.
	// +optional
	Address *string `json:"address,omitempty"`

	// Type of the proxy. Ziti knows http.
	// +optional
	// +kubebuilder:default=http
	// +kubebuilder:validation:Enum=http
	Type *string `json:"type,omitempty"`
}

// CheckAction defines an action for health checks.
type CheckAction struct {
	// Trigger says which results run the action: checks that pass, checks that
	// fail, or a change from one to the other.
	// +kubebuilder:validation:Enum=pass;fail;change
	Trigger string `json:"trigger"`

	// Duration is how long the results must have lasted before the action
	// runs, as a duration such as 30s.
	Duration string `json:"duration"`

	// Action to take: "mark unhealthy" stops offering the terminator, "mark
	// healthy" offers it again, "increase cost N" and "decrease cost N" change
	// its cost, "send event" reports the result to the controller.
	Action string `json:"action"`

	// ConsecutiveEvents is how many results in a row it takes before the
	// action runs.
	// +optional
	// +kubebuilder:default=1
	ConsecutiveEvents *int32 `json:"consecutiveEvents,omitempty"`
}

// HTTPCheck defines an HTTP health check.
type HTTPCheck struct {
	// URL the hosting tunneler requests.
	URL string `json:"url"`

	// Method of the request: GET, PUT, POST or PATCH.
	// +kubebuilder:validation:Enum=GET;PUT;POST;PATCH
	Method string `json:"method"`

	// Body of the request.
	// +optional
	Body *string `json:"body,omitempty"`

	// ExpectStatus is the status code of a check that passes.
	// +optional
	// +kubebuilder:default=200
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=599
	ExpectStatus *int32 `json:"expectStatus,omitempty"`

	// ExpectInBody is text the response of a check that passes contains.
	// +optional
	ExpectInBody *string `json:"expectInBody,omitempty"`

	// Interval between two checks, as a duration such as 10s.
	Interval string `json:"interval"`

	// Timeout after which a check without an answer fails, as a duration such
	// as 5s.
	Timeout string `json:"timeout"`

	// Actions to take on the results of the check.
	Actions []CheckAction `json:"actions"`
}

// PortCheck defines a port health check.
type PortCheck struct {
	// Address the hosting tunneler connects to, as host:port.
	Address string `json:"address"`

	// Interval between two checks, as a duration such as 10s.
	Interval string `json:"interval"`

	// Timeout after which a check without an answer fails, as a duration such
	// as 5s.
	Timeout string `json:"timeout"`

	// Actions to take on the results of the check.
	Actions []CheckAction `json:"actions"`
}

// ConfigHostV1Spec specifies the desired state of a Ziti Host V1 Config.
type ConfigHostV1Spec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ConfigHostV1Parameters `json:"forProvider"`
}

// ConfigHostV1Status represents the observed state of a Ziti Host V1 Config.
type ConfigHostV1Status struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ConfigHostV1Observation `json:"atProvider,omitempty"`
}

// HostTerminator describes where a hosting tunneler sends the traffic of a
// service and how it listens for it. It is the whole of a host.v1 config and
// one terminator of a host.v2 config.
type HostTerminator struct {
	// Address is the host name or IP address the hosting tunneler connects to.
	// Leave it out when forwardAddress is set.
	// +optional
	Address *string `json:"address,omitempty"`

	// Port the hosting tunneler connects to. Leave it out when forwardPort is
	// set.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port *int32 `json:"port,omitempty"`

	// Protocol the hosting tunneler connects with: tcp or udp. Leave it out
	// when forwardProtocol is set.
	// +optional
	// +kubebuilder:validation:Enum=tcp;udp
	Protocol *string `json:"protocol,omitempty"`

	// ForwardProtocol connects with the protocol the client used, if
	// allowedProtocols has it.
	// +optional
	ForwardProtocol *bool `json:"forwardProtocol,omitempty"`

	// ForwardPort connects to the port the client asked for, if
	// allowedPortRanges has it.
	// +optional
	ForwardPort *bool `json:"forwardPort,omitempty"`

	// ForwardAddress connects to the address the client asked for, if
	// allowedAddresses has it.
	// +optional
	ForwardAddress *bool `json:"forwardAddress,omitempty"`

	// AllowedProtocols are the protocols forwardProtocol may connect with:
	// tcp, udp.
	// +optional
	// +kubebuilder:validation:Items:Enum=tcp;udp
	AllowedProtocols []string `json:"allowedProtocols,omitempty"`

	// AllowedAddresses are the IP addresses, CIDR ranges and host names
	// forwardAddress may connect to.
	// +optional
	AllowedAddresses []string `json:"allowedAddresses,omitempty"`

	// AllowedSourceAddresses are the addresses the hosting tunneler may
	// connect from when the intercept config of the service sets sourceIp.
	// +optional
	AllowedSourceAddresses []string `json:"allowedSourceAddresses,omitempty"`

	// ForwardAddressTranslations move the address the client asked for into
	// another range before the hosting tunneler connects. They apply when
	// forwardAddress is true.
	// +optional
	ForwardAddressTranslations []ForwardAddressTranslation `json:"forwardAddressTranslations,omitempty"`

	// AllowedPortRanges are the ports forwardPort may connect to.
	// +optional
	AllowedPortRanges []PortRange `json:"allowedPortRanges,omitempty"`

	// ListenOptions say how the hosting tunneler offers the service to the
	// network.
	// +optional
	ListenOptions *ListenOptions `json:"listenOptions,omitempty"`

	// Proxy makes the hosting tunneler connect through a proxy.
	// +optional
	Proxy *ProxyConfig `json:"proxy,omitempty"`

	// HTTPChecks are health checks the hosting tunneler makes with HTTP
	// requests. Their actions change how its terminator is offered.
	// +optional
	HTTPChecks []HTTPCheck `json:"httpChecks,omitempty"`

	// PortChecks are health checks the hosting tunneler makes by opening a TCP
	// connection. Their actions change how its terminator is offered.
	// +optional
	PortChecks []PortCheck `json:"portChecks,omitempty"`
}

// ConfigHostV1Parameters define the desired state of a Ziti Host V1 Config.
type ConfigHostV1Parameters struct {
	// Name of the config.
	Name string `json:"name"`

	HostTerminator `json:",inline"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ConfigHostV1Observation represents the observed state of a Ziti Host V1 Config.
type ConfigHostV1Observation struct {
	ID                         string                      `json:"id,omitempty"`
	Name                       string                      `json:"name,omitempty"`
	Address                    string                      `json:"address,omitempty"`
	Port                       int32                       `json:"port,omitempty"`
	Protocol                   string                      `json:"protocol,omitempty"`
	ForwardProtocol            bool                        `json:"forwardProtocol,omitempty"`
	ForwardPort                bool                        `json:"forwardPort,omitempty"`
	ForwardAddress             bool                        `json:"forwardAddress,omitempty"`
	AllowedProtocols           []string                    `json:"allowedProtocols,omitempty"`
	AllowedAddresses           []string                    `json:"allowedAddresses,omitempty"`
	AllowedSourceAddresses     []string                    `json:"allowedSourceAddresses,omitempty"`
	ForwardAddressTranslations []ForwardAddressTranslation `json:"forwardAddressTranslations,omitempty"`
	AllowedPortRanges          []PortRange                 `json:"allowedPortRanges,omitempty"`
	ListenOptions              *ListenOptions              `json:"listenOptions,omitempty"`
	Proxy                      *ProxyConfig                `json:"proxy,omitempty"`
	HTTPChecks                 []HTTPCheck                 `json:"httpChecks,omitempty"`
	PortChecks                 []PortCheck                 `json:"portChecks,omitempty"`
	Tags                       map[string]string           `json:"tags,omitempty"`
	CreatedAt                  string                      `json:"createdAt,omitempty"`
	UpdatedAt                  string                      `json:"updatedAt,omitempty"`
}

// ConfigHostV1 is a Ziti config of type host.v1: where the tunneler that hosts
// a service sends the traffic of the service.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type=string,JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
// +genclient
type ConfigHostV1 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigHostV1Spec   `json:"spec"`
	Status ConfigHostV1Status `json:"status,omitempty"`
}

// ConfigHostV1List contains a list of ConfigHostV1.
// +kubebuilder:object:root=true
type ConfigHostV1List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigHostV1 `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigHostV1{}, &ConfigHostV1List{})
}
