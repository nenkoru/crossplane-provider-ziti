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

// ConfigHostV2Spec specifies the desired state of a Ziti Host V2 Config.
type ConfigHostV2Spec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ConfigHostV2Parameters `json:"forProvider"`
}

// ConfigHostV2Status represents the observed state of a Ziti Host V2 Config.
type ConfigHostV2Status struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ConfigHostV2Observation `json:"atProvider,omitempty"`
}

// ConfigHostV2Parameters define the desired state of a Ziti Host V2 Config.
type ConfigHostV2Parameters struct {
	// Name of the config.
	Name string `json:"name"`

	// Terminators are the destinations the service is hosted at. Each one
	// takes the settings of a host.v1 config.
	// +kubebuilder:validation:MinItems=1
	Terminators []HostTerminator `json:"terminators"`

	// Tags is a map of tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ConfigHostV2Observation represents the observed state of a Ziti Host V2 Config.
type ConfigHostV2Observation struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Terminators []HostTerminator  `json:"terminators,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	CreatedAt   string            `json:"createdAt,omitempty"`
	UpdatedAt   string            `json:"updatedAt,omitempty"`
}

// ConfigHostV2 is a Ziti config of type host.v2: a host config with several
// terminators.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type=string,JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type ConfigHostV2 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigHostV2Spec   `json:"spec"`
	Status ConfigHostV2Status `json:"status,omitempty"`
}

// ConfigHostV2List contains a list of ConfigHostV2.
// +kubebuilder:object:root=true
type ConfigHostV2List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigHostV2 `json:"items"`
}
