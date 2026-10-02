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

// IdentityNoneSpec defines the desired state of an IdentityNone.
type IdentityNoneSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              IdentityParameters `json:"forProvider"`
}

// IdentityNoneStatus defines the observed state of an IdentityNone.
type IdentityNoneStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 IdentityObservation `json:"atProvider,omitempty"`
}

// IdentityNone is a Ziti identity without an enrollment. It suits identities
// that authenticate through an external JWT signer.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type IdentityNone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IdentityNoneSpec   `json:"spec"`
	Status IdentityNoneStatus `json:"status,omitempty"`
}

// IdentityNoneList contains a list of IdentityNone.
// +kubebuilder:object:root=true
type IdentityNoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IdentityNone `json:"items"`
}
