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

// IdentityCASpec defines the desired state of an IdentityCA.
type IdentityCASpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              IdentityCAParameters `json:"forProvider"`
}

// IdentityCAParameters define the desired state of a Ziti identity that
// enrolls with a certificate of a certificate authority.
type IdentityCAParameters struct {
	IdentityParameters `json:",inline"`

	// Ottca is the name or ID of the certificate authority whose certificates
	// the identity may enroll with. It cannot be changed after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ottca cannot be changed after creation"
	Ottca string `json:"ottca"`
}

// IdentityCAStatus defines the observed state of an IdentityCA.
type IdentityCAStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 IdentityObservation `json:"atProvider,omitempty"`
}

// IdentityCA is a Ziti identity that enrolls with a one-time token and a
// client certificate issued by a third-party certificate authority (OTT CA).
// The enrollment token is published to the connection secret under the key
// enrollmentToken until the identity enrolls.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type IdentityCA struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IdentityCASpec   `json:"spec"`
	Status IdentityCAStatus `json:"status,omitempty"`
}

// IdentityCAList contains a list of IdentityCA.
// +kubebuilder:object:root=true
type IdentityCAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IdentityCA `json:"items"`
}
