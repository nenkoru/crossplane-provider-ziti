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

// IdentityUPDBSpec defines the desired state of an IdentityUPDB.
type IdentityUPDBSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              IdentityUPDBParameters `json:"forProvider"`
}

// IdentityUPDBParameters define the desired state of a Ziti identity that
// enrolls by choosing a password.
type IdentityUPDBParameters struct {
	IdentityParameters `json:",inline"`

	// UpdbUsername is the username the identity authenticates with. It
	// cannot be changed after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="updbUsername cannot be changed after creation"
	UpdbUsername string `json:"updbUsername"`
}

// IdentityUPDBStatus defines the observed state of an IdentityUPDB.
type IdentityUPDBStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 IdentityObservation `json:"atProvider,omitempty"`
}

// IdentityUPDB is a Ziti identity that authenticates with a username and a
// password (UPDB). It enrolls by setting its password with the enrollment
// token, which is published to the connection secret under the key
// enrollmentToken until then.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=`.spec.forProvider.name`
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.status.atProvider.id`
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,ziti}
type IdentityUPDB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IdentityUPDBSpec   `json:"spec"`
	Status IdentityUPDBStatus `json:"status,omitempty"`
}

// IdentityUPDBList contains a list of IdentityUPDB.
// +kubebuilder:object:root=true
type IdentityUPDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IdentityUPDB `json:"items"`
}
