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

// TerminatorStrategy represents the strategy for creating terminators.
type TerminatorStrategy string

const (
	// TerminatorStrategySmartRouting creates terminators using smart routing.
	TerminatorStrategySmartRouting TerminatorStrategy = "smartrouting"
	// TerminatorStrategyWeighted creates terminators using weighted distribution.
	TerminatorStrategyWeighted TerminatorStrategy = "weighted"
	// TerminatorStrategyRandom creates terminators randomly across edge routers.
	TerminatorStrategyRandom TerminatorStrategy = "random"
	// TerminatorStrategyHA creates terminators for high availability.
	TerminatorStrategyHA TerminatorStrategy = "ha"
)

// ServiceSpec specifies the desired state of a Ziti Service.
type ServiceSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceParameters `json:"forProvider"`
}

// ServiceStatus represents the observed state of a Ziti Service.
type ServiceStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServiceObservation `json:"atProvider,omitempty"`
}

// ServiceParameters define the desired state of a Ziti Service.
type ServiceParameters struct {
	// Name of the service.
	Name string `json:"name"`

	// EncryptionRequired indicates whether encryption is required for the service.
	// +optional
	// +kubebuilder:default=true
	EncryptionRequired *bool `json:"encryptionRequired,omitempty"`

	// MaxIdleTimeMillis is the maximum idle time in milliseconds.
	// +optional
	MaxIdleTimeMillis *int64 `json:"maxIdleTimeMillis,omitempty"`

	// TerminatorStrategy defines how terminators are selected.
	// Valid values: smartrouting, weighted, random, ha
	// +optional
	// +kubebuilder:validation:Enum=smartrouting;weighted;random;ha
	TerminatorStrategy *TerminatorStrategy `json:"terminatorStrategy,omitempty"`

	// Configs is a list of names or IDs of the configs associated with this
	// service. Names are resolved to IDs.
	// +optional
	Configs []string `json:"configs,omitempty"`

	// RoleAttributes is a list of role attributes for the service.
	// +optional
	RoleAttributes []string `json:"roleAttributes,omitempty"`

	// Tags is a map of tags for the service.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ServiceObservation represents the observed state of a Ziti Service.
type ServiceObservation struct {
	// ID is the unique identifier of the service.
	ID string `json:"id,omitempty"`

	// Name is the current name of the service.
	Name string `json:"name,omitempty"`

	// EncryptionRequired indicates whether encryption is required.
	EncryptionRequired bool `json:"encryptionRequired,omitempty"`

	// MaxIdleTimeMillis is the maximum idle time in milliseconds.
	MaxIdleTimeMillis int64 `json:"maxIdleTimeMillis,omitempty"`

	// TerminatorStrategy is the current terminator strategy.
	TerminatorStrategy string `json:"terminatorStrategy,omitempty"`

	// Configs is the list of config IDs.
	Configs []string `json:"configs,omitempty"`

	// RoleAttributes is the list of role attributes.
	RoleAttributes []string `json:"roleAttributes,omitempty"`

	// Tags is the map of tags.
	Tags map[string]string `json:"tags,omitempty"`

	// CreatedAt is the creation timestamp.
	CreatedAt string `json:"createdAt,omitempty"`

	// UpdatedAt is the last update timestamp.
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// Service is the Schema for the Services API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type=string,JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
// +genclient
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec"`
	Status ServiceStatus `json:"status,omitempty"`
}

// ServiceList contains a list of Service.
// +kubebuilder:object:root=true
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
