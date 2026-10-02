package v1alpha1

import (
	resource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// interface checks to ensure our types conform to the crossplane-runtime interfaces
var (
	_ resource.ProviderConfig           = &ProviderConfig{}
	_ resource.ProviderConfig           = &ClusterProviderConfig{}
	_ resource.TypedProviderConfigUsage = &ProviderConfigUsage{}
	_ resource.TypedProviderConfigUsage = &ClusterProviderConfigUsage{}
	_ resource.ProviderConfigUsageList  = &ProviderConfigUsageList{}
	_ resource.ProviderConfigUsageList  = &ClusterProviderConfigUsageList{}
)

// A ProviderConfigStatus defines the status of a Provider.
type ProviderConfigStatus struct {
	xpv2.ProviderConfigStatus `json:",inline"`
}

// ZitiProviderConfigSpec defines the desired state of a Ziti ProviderConfig.
type ZitiProviderConfigSpec struct {
	// Host is the Ziti Controller URL (e.g. https://controller.example.com:1280).
	// It must use https. A trailing /edge/management/v1 is accepted and ignored.
	// +optional
	// +kubebuilder:validation:Pattern=`^https://.+`
	Host string `json:"host,omitempty"`

	// Username for password-based authentication.
	// Required when using password auth (cert/key not provided).
	// +optional
	Username string `json:"username,omitempty"`

	// Password for password-based authentication.
	// Prefer supplying it through credentials.secretRef: anyone who can read
	// this ProviderConfig can read an inline password.
	// +optional
	Password string `json:"password,omitempty"`

	// Cert is the PEM-encoded client certificate for certificate authentication.
	// +optional
	Cert string `json:"cert,omitempty"`

	// Key is the PEM-encoded private key for certificate authentication.
	// Prefer supplying it through credentials.secretRef: anyone who can read
	// this ProviderConfig can read an inline key.
	// +optional
	Key string `json:"key,omitempty"`

	// CA is the PEM-encoded CA bundle used to verify the controller
	// certificate. The system roots are used when it is not set.
	// +optional
	CA string `json:"ca,omitempty"`

	// InsecureSkipTLSVerify disables verification of the controller
	// certificate. Use it only for testing.
	// +optional
	InsecureSkipTLSVerify bool `json:"insecureSkipTLSVerify,omitempty"`

	// Credentials required to authenticate to this provider.
	// When specified, takes precedence over inline fields above.
	// +optional
	Credentials *ProviderCredentials `json:"credentials,omitempty"`
}

// ProviderCredentials required to authenticate.
type ProviderCredentials struct {
	// Source of the provider credentials. Only Secret is supported.
	// +optional
	// +kubebuilder:default=Secret
	// +kubebuilder:validation:Enum=Secret
	Source xpv2.CredentialsSource `json:"source,omitempty"`

	// SecretRef is a reference to the credentials secret.
	// The secret may contain the keys host, username, password, cert, key
	// and ca, or a key named "credentials" with a JSON object:
	// {
	//   "host": "https://controller.example.com:1280",
	//   "username": "admin",
	//   "password": "secret",
	//   // OR for cert auth:
	//   "cert": "-----BEGIN CERTIFICATE-----...",
	//   "key": "-----BEGIN RSA PRIVATE KEY-----...",
	//   "ca": "-----BEGIN CERTIFICATE-----..."
	// }
	// +optional
	SecretRef *CredentialsSecretReference `json:"secretRef,omitempty"`
}

// CredentialsSecretReference is a reference to a secret holding credentials.
type CredentialsSecretReference struct {
	// Name of the secret.
	Name string `json:"name"`

	// Namespace of the secret. Required for a ClusterProviderConfig. A
	// ProviderConfig always reads the secret from its own namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ProviderConfigSpec specifies the state of a ProviderConfig.
type ProviderConfigSpec struct {
	ZitiProviderConfigSpec `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="SECRET-NAME",type="string",JSONPath=".spec.credentials.secretRef.name",priority=1
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,provider,ziti}
// A ProviderConfig configures Ziti providers with credentials required to authenticate.
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig.
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="CONFIG-NAME",type="string",JSONPath=".providerConfigRef.name"
// +kubebuilder:printcolumn:name="RESOURCE-KIND",type="string",JSONPath=".resourceRef.kind"
// +kubebuilder:printcolumn:name="RESOURCE-NAME",type="string",JSONPath=".resourceRef.name"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,provider,ziti}
// A ProviderConfigUsage indicates that a resource is using a ProviderConfig.
type ProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	xpv2.TypedProviderConfigUsage `json:",inline"`
}

// +kubebuilder:object:root=true

// ProviderConfigUsageList contains a list of ProviderConfigUsage.
type ProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfigUsage `json:"items"`
}

// +kubebuilder:object:root=true

// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="SECRET-NAME",type="string",JSONPath=".spec.credentials.secretRef.name",priority=1
// +kubebuilder:resource:scope=Cluster,categories={crossplane,provider,ziti}
// A ClusterProviderConfig configures a Ziti provider cluster-wide.
type ClusterProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterProviderConfigList contains a list of ClusterProviderConfig.
type ClusterProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="CONFIG-NAME",type="string",JSONPath=".providerConfigRef.name"
// +kubebuilder:printcolumn:name="RESOURCE-KIND",type="string",JSONPath=".resourceRef.kind"
// +kubebuilder:printcolumn:name="RESOURCE-NAME",type="string",JSONPath=".resourceRef.name"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,provider,ziti}
// A ClusterProviderConfigUsage indicates that a resource is using a ClusterProviderConfig.
type ClusterProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	xpv2.TypedProviderConfigUsage `json:",inline"`
}

// +kubebuilder:object:root=true

// ClusterProviderConfigUsageList contains a list of ClusterProviderConfigUsage.
type ClusterProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProviderConfigUsage `json:"items"`
}
