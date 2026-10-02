package v1alpha1

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// Package type metadata.
const (
	Group   = "ziti.crossplane.io"
	Version = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}
)

// ProviderConfig type metadata.
var (
	ProviderConfigKind             = reflect.TypeOf(ProviderConfig{}).Name()
	ProviderConfigGroupKind        = schema.GroupKind{Group: Group, Kind: ProviderConfigKind}.String()
	ProviderConfigGroupVersionKind = SchemeGroupVersion.WithKind(ProviderConfigKind)
)

// ProviderConfigUsage type metadata.
var (
	ProviderConfigUsageKind             = reflect.TypeOf(ProviderConfigUsage{}).Name()
	ProviderConfigUsageGroupVersionKind = SchemeGroupVersion.WithKind(ProviderConfigUsageKind)

	ProviderConfigUsageListKind             = reflect.TypeOf(ProviderConfigUsageList{}).Name()
	ProviderConfigUsageListGroupVersionKind = SchemeGroupVersion.WithKind(ProviderConfigUsageListKind)
)

// ClusterProviderConfig type metadata
var (
	ClusterProviderConfigKind             = reflect.TypeOf(ClusterProviderConfig{}).Name()
	ClusterProviderConfigGroupKind        = schema.GroupKind{Group: Group, Kind: ClusterProviderConfigKind}.String()
	ClusterProviderConfigGroupVersionKind = SchemeGroupVersion.WithKind(ClusterProviderConfigKind)
)

// ClusterProviderConfigUsage type metadata.
var (
	ClusterProviderConfigUsageKind             = reflect.TypeOf(ClusterProviderConfigUsage{}).Name()
	ClusterProviderConfigUsageGroupVersionKind = SchemeGroupVersion.WithKind(ClusterProviderConfigUsageKind)

	ClusterProviderConfigUsageListKind             = reflect.TypeOf(ClusterProviderConfigUsageList{}).Name()
	ClusterProviderConfigUsageListGroupVersionKind = SchemeGroupVersion.WithKind(ClusterProviderConfigUsageListKind)
)

// Service type metadata.
var (
	ServiceKind             = reflect.TypeOf(Service{}).Name()
	ServiceGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceKind}.String()
	ServiceGroupVersionKind = SchemeGroupVersion.WithKind(ServiceKind)
)

// EdgeRouter type metadata.
var (
	EdgeRouterKind             = reflect.TypeOf(EdgeRouter{}).Name()
	EdgeRouterGroupKind        = schema.GroupKind{Group: Group, Kind: EdgeRouterKind}.String()
	EdgeRouterGroupVersionKind = SchemeGroupVersion.WithKind(EdgeRouterKind)
)

// ConfigHostV1 type metadata.
var (
	ConfigHostV1Kind             = reflect.TypeOf(ConfigHostV1{}).Name()
	ConfigHostV1GroupKind        = schema.GroupKind{Group: Group, Kind: ConfigHostV1Kind}.String()
	ConfigHostV1GroupVersionKind = SchemeGroupVersion.WithKind(ConfigHostV1Kind)
)

// ConfigHostV2 type metadata.
var (
	ConfigHostV2Kind             = reflect.TypeOf(ConfigHostV2{}).Name()
	ConfigHostV2GroupKind        = schema.GroupKind{Group: Group, Kind: ConfigHostV2Kind}.String()
	ConfigHostV2GroupVersionKind = SchemeGroupVersion.WithKind(ConfigHostV2Kind)
)

// ConfigInterceptV1 type metadata.
var (
	ConfigInterceptV1Kind             = reflect.TypeOf(ConfigInterceptV1{}).Name()
	ConfigInterceptV1GroupKind        = schema.GroupKind{Group: Group, Kind: ConfigInterceptV1Kind}.String()
	ConfigInterceptV1GroupVersionKind = SchemeGroupVersion.WithKind(ConfigInterceptV1Kind)
)

// ServicePolicy type metadata.
var (
	ServicePolicyKind             = reflect.TypeOf(ServicePolicy{}).Name()
	ServicePolicyGroupKind        = schema.GroupKind{Group: Group, Kind: ServicePolicyKind}.String()
	ServicePolicyGroupVersionKind = SchemeGroupVersion.WithKind(ServicePolicyKind)
)

// ServiceEdgeRouterPolicy type metadata.
var (
	ServiceEdgeRouterPolicyKind             = reflect.TypeOf(ServiceEdgeRouterPolicy{}).Name()
	ServiceEdgeRouterPolicyGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceEdgeRouterPolicyKind}.String()
	ServiceEdgeRouterPolicyGroupVersionKind = SchemeGroupVersion.WithKind(ServiceEdgeRouterPolicyKind)
)

// EdgeRouterPolicy type metadata.
var (
	EdgeRouterPolicyKind             = reflect.TypeOf(EdgeRouterPolicy{}).Name()
	EdgeRouterPolicyGroupKind        = schema.GroupKind{Group: Group, Kind: EdgeRouterPolicyKind}.String()
	EdgeRouterPolicyGroupVersionKind = SchemeGroupVersion.WithKind(EdgeRouterPolicyKind)
)

// Identity type metadata.
var (
	IdentityKind             = reflect.TypeOf(Identity{}).Name()
	IdentityGroupKind        = schema.GroupKind{Group: Group, Kind: IdentityKind}.String()
	IdentityGroupVersionKind = SchemeGroupVersion.WithKind(IdentityKind)
)

// IdentityCA type metadata.
var (
	IdentityCAKind             = reflect.TypeOf(IdentityCA{}).Name()
	IdentityCAGroupKind        = schema.GroupKind{Group: Group, Kind: IdentityCAKind}.String()
	IdentityCAGroupVersionKind = SchemeGroupVersion.WithKind(IdentityCAKind)
)

// IdentityUPDB type metadata.
var (
	IdentityUPDBKind             = reflect.TypeOf(IdentityUPDB{}).Name()
	IdentityUPDBGroupKind        = schema.GroupKind{Group: Group, Kind: IdentityUPDBKind}.String()
	IdentityUPDBGroupVersionKind = SchemeGroupVersion.WithKind(IdentityUPDBKind)
)

// IdentityNone type metadata.
var (
	IdentityNoneKind             = reflect.TypeOf(IdentityNone{}).Name()
	IdentityNoneGroupKind        = schema.GroupKind{Group: Group, Kind: IdentityNoneKind}.String()
	IdentityNoneGroupVersionKind = SchemeGroupVersion.WithKind(IdentityNoneKind)
)

// PostureCheckOS type metadata.
var (
	PostureCheckOSKind             = reflect.TypeOf(PostureCheckOS{}).Name()
	PostureCheckOSGroupKind        = schema.GroupKind{Group: Group, Kind: PostureCheckOSKind}.String()
	PostureCheckOSGroupVersionKind = SchemeGroupVersion.WithKind(PostureCheckOSKind)
)

// PostureCheckMFA type metadata.
var (
	PostureCheckMFAGroupKind        = schema.GroupKind{Group: Group, Kind: "PostureCheckMFA"}.String()
	PostureCheckMFAGroupVersionKind = SchemeGroupVersion.WithKind("PostureCheckMFA")
)

// AuthPolicy type metadata.
var (
	AuthPolicyKind             = reflect.TypeOf(AuthPolicy{}).Name()
	AuthPolicyGroupKind        = schema.GroupKind{Group: Group, Kind: AuthPolicyKind}.String()
	AuthPolicyGroupVersionKind = SchemeGroupVersion.WithKind(AuthPolicyKind)
)

// PostureCheckDomain type metadata.
var (
	PostureCheckDomainKind             = reflect.TypeOf(PostureCheckDomain{}).Name()
	PostureCheckDomainGroupKind        = schema.GroupKind{Group: Group, Kind: PostureCheckDomainKind}.String()
	PostureCheckDomainGroupVersionKind = SchemeGroupVersion.WithKind(PostureCheckDomainKind)
)

// PostureCheckMac type metadata.
var (
	PostureCheckMacKind             = reflect.TypeOf(PostureCheckMac{}).Name()
	PostureCheckMacGroupKind        = schema.GroupKind{Group: Group, Kind: PostureCheckMacKind}.String()
	PostureCheckMacGroupVersionKind = SchemeGroupVersion.WithKind(PostureCheckMacKind)
)

// PostureCheckProcess type metadata.
var (
	PostureCheckProcessKind             = reflect.TypeOf(PostureCheckProcess{}).Name()
	PostureCheckProcessGroupKind        = schema.GroupKind{Group: Group, Kind: PostureCheckProcessKind}.String()
	PostureCheckProcessGroupVersionKind = SchemeGroupVersion.WithKind(PostureCheckProcessKind)
)

// PostureCheckMultiProcess type metadata.
var (
	PostureCheckMultiProcessKind             = reflect.TypeOf(PostureCheckMultiProcess{}).Name()
	PostureCheckMultiProcessGroupKind        = schema.GroupKind{Group: Group, Kind: PostureCheckMultiProcessKind}.String()
	PostureCheckMultiProcessGroupVersionKind = SchemeGroupVersion.WithKind(PostureCheckMultiProcessKind)
)

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
	SchemeBuilder.Register(&ProviderConfigUsage{}, &ProviderConfigUsageList{})
	SchemeBuilder.Register(&ClusterProviderConfig{}, &ClusterProviderConfigList{})
	SchemeBuilder.Register(&ClusterProviderConfigUsage{}, &ClusterProviderConfigUsageList{})
	SchemeBuilder.Register(&Service{}, &ServiceList{})
	SchemeBuilder.Register(&EdgeRouter{}, &EdgeRouterList{})
	SchemeBuilder.Register(&ConfigHostV1{}, &ConfigHostV1List{})
	SchemeBuilder.Register(&ConfigHostV2{}, &ConfigHostV2List{})
	SchemeBuilder.Register(&ConfigInterceptV1{}, &ConfigInterceptV1List{})
	SchemeBuilder.Register(&ServicePolicy{}, &ServicePolicyList{})
	SchemeBuilder.Register(&ServiceEdgeRouterPolicy{}, &ServiceEdgeRouterPolicyList{})
	SchemeBuilder.Register(&EdgeRouterPolicy{}, &EdgeRouterPolicyList{})
	SchemeBuilder.Register(&Identity{}, &IdentityList{})
	SchemeBuilder.Register(&IdentityCA{}, &IdentityCAList{})
	SchemeBuilder.Register(&IdentityUPDB{}, &IdentityUPDBList{})
	SchemeBuilder.Register(&IdentityNone{}, &IdentityNoneList{})
	SchemeBuilder.Register(&PostureCheckOS{}, &PostureCheckOSList{})
	SchemeBuilder.Register(&PostureCheckMFA{}, &PostureCheckMFAList{})
	SchemeBuilder.Register(&AuthPolicy{}, &AuthPolicyList{})
	SchemeBuilder.Register(&PostureCheckDomain{}, &PostureCheckDomainList{})
	SchemeBuilder.Register(&PostureCheckMac{}, &PostureCheckMacList{})
	SchemeBuilder.Register(&PostureCheckProcess{}, &PostureCheckProcessList{})
	SchemeBuilder.Register(&PostureCheckMultiProcess{}, &PostureCheckMultiProcessList{})
}
