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
	// From is the source address pattern.
	From string `json:"from"`
	// To is the destination address pattern.
	To string `json:"to"`
	// PrefixLength is the prefix length for the translation.
	PrefixLength int32 `json:"prefixLength"`
}

// ListenOptions defines listen options for the host config.
type ListenOptions struct {
	// BindUsingEdgeIdentity indicates whether to bind using edge identity.
	// +optional
	BindUsingEdgeIdentity *bool `json:"bindUsingEdgeIdentity,omitempty"`

	// ConnectTimeout is the connection timeout (e.g., "5s").
	// +optional
	ConnectTimeout *string `json:"connectTimeout,omitempty"`

	// Cost is the cost for this listener.
	// +optional
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	Cost *int32 `json:"cost,omitempty"`

	// MaxConnections is the maximum number of connections.
	// +optional
	// +kubebuilder:default=65535
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	MaxConnections *int32 `json:"maxConnections,omitempty"`

	// Precedence is the precedence level (default, required, failed).
	// +optional
	// +kubebuilder:default=default
	// +kubebuilder:validation:Enum=default;required;failed
	Precedence *string `json:"precedence,omitempty"`
}

// ProxyConfig defines proxy configuration.
type ProxyConfig struct {
	// Address is the proxy address.
	// +optional
	Address *string `json:"address,omitempty"`

	// Type is the proxy type (e.g., "http").
	// +optional
	// +kubebuilder:default=http
	// +kubebuilder:validation:Enum=http
	Type *string `json:"type,omitempty"`
}

// CheckAction defines an action for health checks.
type CheckAction struct {
	// Trigger is the trigger condition (pass, fail, change).
	// +kubebuilder:validation:Enum=pass;fail;change
	Trigger string `json:"trigger"`

	// Duration is the duration for the action.
	Duration string `json:"duration"`

	// Action is the action to take (mark unhealthy, mark healthy, send event, increase cost N, decrease cost N).
	Action string `json:"action"`

	// ConsecutiveEvents is the number of consecutive events required.
	// +optional
	// +kubebuilder:default=1
	ConsecutiveEvents *int32 `json:"consecutiveEvents,omitempty"`
}

// HTTPCheck defines an HTTP health check.
type HTTPCheck struct {
	// URL is the URL to check.
	URL string `json:"url"`

	// Method is the HTTP method (GET, PUT, POST, PATCH).
	// +kubebuilder:validation:Enum=GET;PUT;POST;PATCH
	Method string `json:"method"`

	// Body is the request body.
	// +optional
	Body *string `json:"body,omitempty"`

	// ExpectStatus is the expected HTTP status code.
	// +optional
	// +kubebuilder:default=200
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	ExpectStatus *int32 `json:"expectStatus,omitempty"`

	// ExpectInBody is the expected string in response body.
	// +optional
	ExpectInBody *string `json:"expectInBody,omitempty"`

	// Interval is the check interval (e.g., "10s").
	Interval string `json:"interval"`

	// Timeout is the check timeout (e.g., "5s").
	Timeout string `json:"timeout"`

	// Actions are the actions to take on check results.
	Actions []CheckAction `json:"actions"`
}

// PortCheck defines a port health check.
type PortCheck struct {
	// Address is the address to check.
	Address string `json:"address"`

	// Interval is the check interval (e.g., "10s").
	Interval string `json:"interval"`

	// Timeout is the check timeout (e.g., "5s").
	Timeout string `json:"timeout"`

	// Actions are the actions to take on check results.
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

// ConfigHostV1Parameters define the desired state of a Ziti Host V1 Config.
type ConfigHostV1Parameters struct {
	// Name of the config.
	Name string `json:"name"`

	// Address is the host address.
	// +optional
	Address *string `json:"address,omitempty"`

	// Port is the host port.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port *int32 `json:"port,omitempty"`

	// Protocol is the host protocol (tcp, udp).
	// +optional
	// +kubebuilder:validation:Enum=tcp;udp
	Protocol *string `json:"protocol,omitempty"`

	// ForwardProtocol indicates whether to forward allowed protocols.
	// +optional
	ForwardProtocol *bool `json:"forwardProtocol,omitempty"`

	// ForwardPort indicates whether to forward allowed port ranges.
	// +optional
	ForwardPort *bool `json:"forwardPort,omitempty"`

	// ForwardAddress indicates whether to forward allowed addresses.
	// +optional
	ForwardAddress *bool `json:"forwardAddress,omitempty"`

	// AllowedProtocols is a list of protocols that can be forwarded (tcp, udp).
	// +optional
	// +kubebuilder:validation:Items:Enum=tcp;udp
	AllowedProtocols []string `json:"allowedProtocols,omitempty"`

	// AllowedAddresses is a list of addresses that can be forwarded.
	// +optional
	AllowedAddresses []string `json:"allowedAddresses,omitempty"`

	// AllowedSourceAddresses is a list of source addresses that can be forwarded.
	// +optional
	AllowedSourceAddresses []string `json:"allowedSourceAddresses,omitempty"`

	// ForwardAddressTranslations is a list of address translations to forward.
	// +optional
	ForwardAddressTranslations []ForwardAddressTranslation `json:"forwardAddressTranslations,omitempty"`

	// AllowedPortRanges is a list of port ranges that can be forwarded.
	// +optional
	AllowedPortRanges []PortRange `json:"allowedPortRanges,omitempty"`

	// ListenOptions defines listen options.
	// +optional
	ListenOptions *ListenOptions `json:"listenOptions,omitempty"`

	// Proxy defines proxy configuration.
	// +optional
	Proxy *ProxyConfig `json:"proxy,omitempty"`

	// HTTPChecks is a list of HTTP health checks.
	// +optional
	HTTPChecks []HTTPCheck `json:"httpChecks,omitempty"`

	// PortChecks is a list of port health checks.
	// +optional
	PortChecks []PortCheck `json:"portChecks,omitempty"`

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

// ConfigHostV1 is the Schema for the ConfigHostV1s API.
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
