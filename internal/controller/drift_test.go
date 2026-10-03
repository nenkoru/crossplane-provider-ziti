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

package controller_test

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/authpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/certificateauthority"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv1"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv2"
	"github.com/crossplane/provider-ziti/internal/controller/configinterceptv1"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouter"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/externaljwtsigner"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/identityca"
	"github.com/crossplane/provider-ziti/internal/controller/identitynone"
	"github.com/crossplane/provider-ziti/internal/controller/identityupdb"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckdomain"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmac"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmfa"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmultiprocess"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckos"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckprocess"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/serviceedgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

// oldToken is the enrollment token of an entity before it is deleted in Ziti.
const oldToken = "old-token"

// A drifter is a managed resource reconciled by the harness, whatever its
// kind.
type drifter interface {
	settle(limit int) resource.Managed
	reconcile() int
	remove()
	exists() bool
	secret(name string) map[string][]byte
}

type drifting[T resource.ModernManaged] struct{ *harness[T] }

func (d drifting[T]) settle(limit int) resource.Managed { return d.converge(limit) }

// reconciled returns a function that starts reconciling the managed resource
// the supplied function returns.
func reconciled[T resource.ModernManaged](kind generic.Kind[T], mg func() T) func(*testing.T, *fake.Server) drifter {
	return func(t *testing.T, srv *fake.Server) drifter {
		return drifting[T]{newHarness(t, srv, kind, mg())}
	}
}

// A driftCase is a managed resource whose entity is changed and deleted in
// Ziti behind the provider's back.
type driftCase struct {
	collection string

	// seed stores the entities the managed resource refers to.
	seed func(srv *fake.Server)

	// start starts reconciling the managed resource.
	start func(t *testing.T, srv *fake.Server) drifter

	// tampers change a field the managed resource manages: each is reverted.
	tampers map[string]func(e map[string]any)

	// owned changes what Ziti owns, or what the spec leaves to Ziti: it is
	// not drift and is left alone.
	owned func(e map[string]any)

	// token returns the enrollment token Ziti reports for the entity, for a
	// kind that publishes it to its connection secret.
	token func(e map[string]any) string
}

// nested returns the object under the supplied path of keys.
func nested(e map[string]any, path ...string) map[string]any {
	for _, key := range path {
		e, _ = e[key].(map[string]any)
	}
	return e
}

// identityToken returns the enrollment token an identity of the supplied
// method has.
func identityToken(method string) func(map[string]any) string {
	return func(e map[string]any) string {
		token, _ := nested(e, "enrollment", method)["jwt"].(string)
		return token
	}
}

// setIdentityToken makes Ziti report the supplied enrollment token.
func setToken(e map[string]any, token string) {
	if _, ok := e["enrollmentJwt"]; ok {
		e["enrollmentJwt"] = token
		return
	}
	for _, enrollment := range nested(e, "enrollment") {
		if m, ok := enrollment.(map[string]any); ok {
			m["jwt"] = token
		}
	}
}

// identityTampers change what every identity kind manages.
var identityTampers = map[string]func(e map[string]any){
	"Scalar":             func(e map[string]any) { e["isAdmin"] = true },
	"RoleAttributes":     func(e map[string]any) { e["roleAttributes"] = []any{"clients", "added-by-hand"} },
	"Tags":               func(e map[string]any) { e["tags"] = map[string]any{"team": "other"} },
	"AppData":            func(e map[string]any) { e["appData"] = map[string]any{"site": "paris", "floor": "2"} },
	"HostingCosts":       func(e map[string]any) { e["serviceHostingCosts"] = map[string]any{"svc-1": float64(1)} },
	"HostingPrecedences": func(e map[string]any) { delete(e, "serviceHostingPrecedences") },
	"DefaultHostingCost": func(e map[string]any) { e["defaultHostingCost"] = float64(0) },
}

// identityOwned is what Ziti reports for an identity that has enrolled and
// what it fills in for the settings the spec leaves out.
func identityOwned(e map[string]any) {
	e["enrollment"] = map[string]any{}
	e["authenticators"] = map[string]any{"cert": map[string]any{"id": "auth-1", "fingerprint": "abc"}}
	e["authPolicyId"] = "default"
	e["externalId"] = nil
	e["hasApiSession"] = true
	e["typeId"] = "Default"
	e["sdkInfo"] = map[string]any{"type": "ziti-edge-tunnel"}
	e["updatedAt"] = "2030-01-01T00:00:00.000Z"
}

// identityParameters are the settings of the identities of every kind.
func identityParameters(name string) v1alpha1.IdentityParameters {
	return v1alpha1.IdentityParameters{
		Name:                      name,
		IsAdmin:                   ptr.To(false),
		RoleAttributes:            []string{"clients"},
		DefaultHostingCost:        ptr.To(int32(10)),
		ServiceHostingCosts:       map[string]int32{"web": 100},
		ServiceHostingPrecedences: map[string]string{"web": "required"},
		AppData:                   map[string]string{"site": "berlin"},
		Tags:                      map[string]string{"team": "web"},
	}
}

// withSecret writes the connection details of a managed resource to the
// connection secret <name>-enrollment.
func withSecret(name string) xpv2.ManagedResourceSpec {
	return xpv2.ManagedResourceSpec{WriteConnectionSecretToReference: &xpv2.LocalSecretReference{Name: name + "-enrollment"}}
}

func driftCases() map[string]driftCase {
	seedServices := func(srv *fake.Server) {
		srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})
	}
	common := func(e map[string]any) {
		e["updatedAt"] = "2030-01-01T00:00:00.000Z"
		e["_links"] = map[string]any{"self": map[string]any{"href": "./" + e["id"].(string)}} //nolint:forcetypeassert // Every entity has an ID.
		e["isSystem"] = false
	}

	return map[string]driftCase{
		"Service": {
			collection: "services",
			seed: func(srv *fake.Server) {
				srv.Put("configs", map[string]any{"id": "cfg-1", "name": "web-host"})
				srv.Put("configs", map[string]any{"id": "cfg-2", "name": "web-intercept"})
			},
			start: reconciled(service.Kind, func() *v1alpha1.Service {
				return &v1alpha1.Service{ObjectMeta: meta1("web"), Spec: v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{
					Name: "web", EncryptionRequired: ptr.To(true), MaxIdleTimeMillis: ptr.To(int64(60000)),
					Configs: []string{"web-host", "web-intercept"}, RoleAttributes: []string{"web"}, Tags: map[string]string{"env": "prod"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				// Only a PUT changes encryptionRequired, which is why
				// services are replaced.
				"EncryptionRequired": func(e map[string]any) { e["encryptionRequired"] = false },
				"MaxIdleTime":        func(e map[string]any) { e["maxIdleTimeMillis"] = float64(1) },
				"ConfigDropped":      func(e map[string]any) { e["configs"] = []any{"cfg-1"} },
				"RoleAttributeAdded": func(e map[string]any) { e["roleAttributes"] = []any{"web", "added-by-hand"} },
				"RoleAttributesGone": func(e map[string]any) { delete(e, "roleAttributes") },
				"TagChanged":         func(e map[string]any) { e["tags"] = map[string]any{"env": "dev"} },
			},
			owned: func(e map[string]any) {
				common(e)
				e["terminatorStrategy"] = "smartrouting"
				e["permissions"] = []any{"Bind", "Dial"}
			},
		},
		"ConfigHostV1": {
			collection: "configs",
			start: reconciled(confighostv1.Kind, func() *v1alpha1.ConfigHostV1 {
				return &v1alpha1.ConfigHostV1{ObjectMeta: meta1("web-host"), Spec: v1alpha1.ConfigHostV1Spec{ForProvider: v1alpha1.ConfigHostV1Parameters{
					Name: "web-host",
					HostTerminator: v1alpha1.HostTerminator{
						Address: ptr.To("localhost"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp"),
						AllowedAddresses: []string{"10.0.0.0/8", "192.168.0.0/16"},
						ListenOptions:    &v1alpha1.ListenOptions{Precedence: ptr.To("required"), Cost: ptr.To(int32(10))},
					},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"NestedScalar":        func(e map[string]any) { nested(e, "data")["port"] = float64(8443) },
				"NestedListReordered": func(e map[string]any) { nested(e, "data")["allowedAddresses"] = []any{"192.168.0.0/16", "10.0.0.0/8"} },
				"NestedObject":        func(e map[string]any) { nested(e, "data", "listenOptions")["cost"] = float64(1) },
				"FieldAdded":          func(e map[string]any) { nested(e, "data")["forwardPort"] = true },
				"TagAdded":            func(e map[string]any) { e["tags"] = map[string]any{"added": "by-hand"} },
			},
			owned: func(e map[string]any) {
				common(e)
				e["configType"] = map[string]any{"id": "host-v1-id", "name": "host.v1"}
			},
		},
		"ConfigHostV2": {
			collection: "configs",
			start: reconciled(confighostv2.Kind, func() *v1alpha1.ConfigHostV2 {
				return &v1alpha1.ConfigHostV2{ObjectMeta: meta1("web-hosts"), Spec: v1alpha1.ConfigHostV2Spec{ForProvider: v1alpha1.ConfigHostV2Parameters{
					Name: "web-hosts",
					Terminators: []v1alpha1.HostTerminator{
						{Address: ptr.To("web-1.internal"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp")},
						{Address: ptr.To("web-2.internal"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp")},
					},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"TerminatorChanged": func(e map[string]any) {
					nested(e, "data")["terminators"].([]any)[1].(map[string]any)["address"] = "web-3.internal" //nolint:forcetypeassert // Created that way above.
				},
				"TerminatorDropped": func(e map[string]any) {
					nested(e, "data")["terminators"] = nested(e, "data")["terminators"].([]any)[:1] //nolint:forcetypeassert // Created that way above.
				},
			},
			owned: common,
		},
		"ConfigInterceptV1": {
			collection: "configs",
			start: reconciled(configinterceptv1.Kind, func() *v1alpha1.ConfigInterceptV1 {
				return &v1alpha1.ConfigInterceptV1{ObjectMeta: meta1("web-intercept"), Spec: v1alpha1.ConfigInterceptV1Spec{ForProvider: v1alpha1.ConfigInterceptV1Parameters{
					Name: "web-intercept", Addresses: []string{"web.ziti", "api.ziti"}, Protocols: []string{"tcp"},
					PortRanges:  []v1alpha1.PortRange{{Low: 80, High: 80}, {Low: 443, High: 443}},
					DialOptions: &v1alpha1.DialOptions{ConnectTimeoutSeconds: ptr.To(int32(30))},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"AddressAdded": func(e map[string]any) { nested(e, "data")["addresses"] = []any{"web.ziti", "api.ziti", "evil.ziti"} },
				"PortRangeChanged": func(e map[string]any) {
					nested(e, "data")["portRanges"] = []any{map[string]any{"low": float64(1), "high": float64(65535)}}
				},
				"DialOptionsGone": func(e map[string]any) { delete(nested(e, "data"), "dialOptions") },
				"DataGone":        func(e map[string]any) { delete(e, "data") },
			},
			owned: common,
		},
		"ServicePolicy": {
			collection: "service-policies",
			seed: func(srv *fake.Server) {
				seedServices(srv)
				srv.Put("identities", map[string]any{"id": "idn-1", "name": "web-client"})
				srv.Put("posture-checks", map[string]any{"id": "pc-1", "name": "mfa"})
			},
			start: reconciled(servicepolicy.Kind, func() *v1alpha1.ServicePolicy {
				return &v1alpha1.ServicePolicy{ObjectMeta: meta1("web-dial"), Spec: v1alpha1.ServicePolicySpec{ForProvider: v1alpha1.ServicePolicyParameters{
					Name: "web-dial", Type: v1alpha1.ServicePolicyTypeDial, Semantic: v1alpha1.ServicePolicySemanticAnyOf,
					ServiceRoles: []string{"@web"}, IdentityRoles: []string{"#clients", "@web-client"}, PostureCheckRoles: []string{"@mfa"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"Type":             func(e map[string]any) { e["type"] = "Bind" },
				"Semantic":         func(e map[string]any) { e["semantic"] = "AllOf" },
				"ServiceRoles":     func(e map[string]any) { e["serviceRoles"] = []any{"#all"} },
				"IdentityRoles":    func(e map[string]any) { e["identityRoles"] = []any{"#clients"} },
				"PostureCheckGone": func(e map[string]any) { e["postureCheckRoles"] = []any{} },
				"Tags":             func(e map[string]any) { e["tags"] = map[string]any{"a": "b"} },
			},
			owned: common,
		},
		"EdgeRouterPolicy": {
			collection: "edge-router-policies",
			seed: func(srv *fake.Server) {
				srv.Put("edge-routers", map[string]any{"id": "er-1", "name": "router-1"})
			},
			start: reconciled(edgerouterpolicy.Kind, func() *v1alpha1.EdgeRouterPolicy {
				return &v1alpha1.EdgeRouterPolicy{ObjectMeta: meta1("all-routers"), Spec: v1alpha1.EdgeRouterPolicySpec{ForProvider: v1alpha1.EdgeRouterPolicyParameters{
					Name: "all-routers", EdgeRouterRoles: []string{"@router-1", "#public"}, IdentityRoles: []string{"#all"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"Semantic":        func(e map[string]any) { e["semantic"] = "AnyOf" },
				"EdgeRouterRoles": func(e map[string]any) { e["edgeRouterRoles"] = []any{"#public"} },
				"IdentityRoles":   func(e map[string]any) { e["identityRoles"] = []any{"#all", "#more"} },
			},
			owned: common,
		},
		"ServiceEdgeRouterPolicy": {
			collection: "service-edge-router-policies",
			seed:       seedServices,
			start: reconciled(serviceedgerouterpolicy.Kind, func() *v1alpha1.ServiceEdgeRouterPolicy {
				return &v1alpha1.ServiceEdgeRouterPolicy{ObjectMeta: meta1("web-routers"), Spec: v1alpha1.ServiceEdgeRouterPolicySpec{ForProvider: v1alpha1.ServiceEdgeRouterPolicyParameters{
					Name: "web-routers", Semantic: v1alpha1.ServiceEdgeRouterPolicySemanticAnyOf, ServiceRoles: []string{"@web"}, EdgeRouterRoles: []string{"#all"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"Semantic":        func(e map[string]any) { e["semantic"] = "AllOf" },
				"ServiceRoles":    func(e map[string]any) { e["serviceRoles"] = []any{} },
				"EdgeRouterRoles": func(e map[string]any) { e["edgeRouterRoles"] = nil },
			},
			owned: common,
		},
		"Identity": {
			collection: "identities",
			seed:       seedServices,
			start: reconciled(identity.Kind, func() *v1alpha1.Identity {
				return &v1alpha1.Identity{ObjectMeta: meta1("web-client"), Spec: v1alpha1.IdentitySpec{
					ManagedResourceSpec: withSecret("web-client"), ForProvider: identityParameters("web-client"),
				}}
			}),
			tampers: identityTampers,
			owned:   identityOwned,
			token:   identityToken("ott"),
		},
		"IdentityCA": {
			collection: "identities",
			seed: func(srv *fake.Server) {
				seedServices(srv)
				srv.Put("cas", map[string]any{"id": "ca-1", "name": "devices"})
			},
			start: reconciled(identityca.Kind, func() *v1alpha1.IdentityCA {
				return &v1alpha1.IdentityCA{ObjectMeta: meta1("sensor"), Spec: v1alpha1.IdentityCASpec{
					ManagedResourceSpec: withSecret("sensor"),
					ForProvider:         v1alpha1.IdentityCAParameters{IdentityParameters: identityParameters("sensor"), Ottca: "devices"},
				}}
			}),
			tampers: identityTampers,
			owned:   identityOwned,
			token:   identityToken("ottca"),
		},
		"IdentityUPDB": {
			collection: "identities",
			seed:       seedServices,
			start: reconciled(identityupdb.Kind, func() *v1alpha1.IdentityUPDB {
				return &v1alpha1.IdentityUPDB{ObjectMeta: meta1("operator"), Spec: v1alpha1.IdentityUPDBSpec{
					ManagedResourceSpec: withSecret("operator"),
					ForProvider:         v1alpha1.IdentityUPDBParameters{IdentityParameters: identityParameters("operator"), UpdbUsername: "operator"},
				}}
			}),
			tampers: identityTampers,
			owned:   identityOwned,
			token:   identityToken("updb"),
		},
		"IdentityNone": {
			collection: "identities",
			seed:       seedServices,
			start: reconciled(identitynone.Kind, func() *v1alpha1.IdentityNone {
				p := identityParameters("sso-user")
				p.ExternalID = ptr.To("user@example.com")
				return &v1alpha1.IdentityNone{ObjectMeta: meta1("sso-user"), Spec: v1alpha1.IdentityNoneSpec{ForProvider: p}}
			}),
			tampers: func() map[string]func(e map[string]any) {
				t := maps.Clone(identityTampers)
				t["ExternalID"] = func(e map[string]any) { e["externalId"] = "someone-else@example.com" }
				return t
			}(),
			owned: func(e map[string]any) {
				identityOwned(e)
				// The spec sets it.
				e["externalId"] = "user@example.com"
			},
		},
		"EdgeRouter": {
			collection: "edge-routers",
			start: reconciled(edgerouter.Kind, func() *v1alpha1.EdgeRouter {
				return &v1alpha1.EdgeRouter{ObjectMeta: meta1("router-1"), Spec: v1alpha1.EdgeRouterSpec{
					ManagedResourceSpec: withSecret("router-1"),
					ForProvider:         v1alpha1.EdgeRouterParameters{Name: "router-1", IsTunnelerEnabled: ptr.To(true), RoleAttributes: []string{"public"}},
				}}
			}),
			tampers: map[string]func(e map[string]any){
				"Tunneler":       func(e map[string]any) { e["isTunnelerEnabled"] = false },
				"RoleAttributes": func(e map[string]any) { e["roleAttributes"] = []any{"private"} },
				"Tags":           func(e map[string]any) { e["tags"] = map[string]any{"rack": "1"} },
			},
			// An edge router that has enrolled, with the settings the spec
			// leaves out.
			owned: func(e map[string]any) {
				common(e)
				delete(e, "enrollmentJwt")
				e["isVerified"] = true
				e["fingerprint"] = "abc"
				e["isOnline"] = true
				e["cost"] = float64(0)
				e["noTraversal"] = false
			},
			token: func(e map[string]any) string {
				token, _ := e["enrollmentJwt"].(string)
				return token
			},
		},
		"PostureCheckOS": {
			collection: "posture-checks",
			start: reconciled(posturecheckos.Kind, func() *v1alpha1.PostureCheckOS {
				return &v1alpha1.PostureCheckOS{ObjectMeta: meta1("os"), Spec: v1alpha1.PostureCheckOSSpec{ForProvider: v1alpha1.PostureCheckOSParameters{
					Name: "os", OperatingSystems: []v1alpha1.OperatingSystem{{Type: "macOS", Versions: []string{">=14.0.0"}}, {Type: "Linux"}},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"VersionAdded": func(e map[string]any) {
					e["operatingSystems"] = []any{
						map[string]any{"type": "Linux", "versions": []any{}},
						map[string]any{"type": "macOS", "versions": []any{">=13.0.0", ">=14.0.0"}},
					}
				},
				"SystemAdded": func(e map[string]any) {
					e["operatingSystems"] = append(e["operatingSystems"].([]any), map[string]any{"type": "Windows"}) //nolint:forcetypeassert // Created that way above.
				},
				"RoleAttributes": func(e map[string]any) { e["roleAttributes"] = []any{"x"} },
			},
			owned: common,
		},
		"PostureCheckMFA": {
			collection: "posture-checks",
			start: reconciled(posturecheckmfa.Kind, func() *v1alpha1.PostureCheckMFA {
				return &v1alpha1.PostureCheckMFA{ObjectMeta: meta1("mfa"), Spec: v1alpha1.PostureCheckMFASpec{ForProvider: v1alpha1.PostureCheckMFAParameters{
					Name: "mfa", TimeoutSeconds: ptr.To(int64(300)), PromptOnWake: ptr.To(true),
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"Timeout":      func(e map[string]any) { e["timeoutSeconds"] = float64(-1) },
				"PromptOnWake": func(e map[string]any) { delete(e, "promptOnWake") },
			},
			// Settings the spec leaves out are not managed.
			owned: func(e map[string]any) {
				common(e)
				e["promptOnUnlock"] = true
				e["ignoreLegacyEndpoints"] = true
			},
		},
		"PostureCheckDomain": {
			collection: "posture-checks",
			start: reconciled(posturecheckdomain.Kind, func() *v1alpha1.PostureCheckDomain {
				return &v1alpha1.PostureCheckDomain{ObjectMeta: meta1("corporate"), Spec: v1alpha1.PostureCheckDomainSpec{ForProvider: v1alpha1.PostureCheckDomainParameters{
					Name: "corporate", Domains: []string{"corp.example.com", "ad.example.com"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"DomainAdded": func(e map[string]any) { e["domains"] = []any{"ad.example.com", "corp.example.com", "evil.example.com"} },
			},
			owned: common,
		},
		"PostureCheckMac": {
			collection: "posture-checks",
			start: reconciled(posturecheckmac.Kind, func() *v1alpha1.PostureCheckMac {
				return &v1alpha1.PostureCheckMac{ObjectMeta: meta1("registered"), Spec: v1alpha1.PostureCheckMacSpec{ForProvider: v1alpha1.PostureCheckMacParameters{
					Name: "registered", MacAddresses: []v1alpha1.HexString{"00:1A:2B:3C:4D:5E", "0a-1b-2c-3d-4e-5f"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"AddressReplaced": func(e map[string]any) { e["macAddresses"] = []any{"ffffffffffff"} },
			},
			owned: common,
		},
		"PostureCheckProcess": {
			collection: "posture-checks",
			start: reconciled(posturecheckprocess.Kind, func() *v1alpha1.PostureCheckProcess {
				return &v1alpha1.PostureCheckProcess{ObjectMeta: meta1("agent"), Spec: v1alpha1.PostureCheckProcessSpec{ForProvider: v1alpha1.PostureCheckProcessParameters{
					Name:    "agent",
					Process: v1alpha1.Process{OsType: "Windows", Path: `C:\agent.exe`, Hashes: []v1alpha1.HexString{"FFEE01"}, SignerFingerprint: "A9:09"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"Path":        func(e map[string]any) { nested(e, "process")["path"] = `C:\other.exe` },
				"HashDropped": func(e map[string]any) { nested(e, "process")["hashes"] = []any{} },
				"Fingerprint": func(e map[string]any) { delete(nested(e, "process"), "signerFingerprint") },
			},
			owned: common,
		},
		"PostureCheckMultiProcess": {
			collection: "posture-checks",
			start: reconciled(posturecheckmultiprocess.Kind, func() *v1alpha1.PostureCheckMultiProcess {
				return &v1alpha1.PostureCheckMultiProcess{ObjectMeta: meta1("agents"), Spec: v1alpha1.PostureCheckMultiProcessSpec{ForProvider: v1alpha1.PostureCheckMultiProcessParameters{
					Name: "agents",
					Processes: []v1alpha1.MultiProcess{
						{OsType: "macOS", Path: "/usr/bin/agent", SignerFingerprints: []v1alpha1.HexString{"A9:09"}},
						{OsType: "Linux", Path: "/usr/bin/agent", Hashes: []v1alpha1.HexString{"FFEE01", "aa"}},
					},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"Semantic":       func(e map[string]any) { e["semantic"] = "AnyOf" },
				"ProcessDropped": func(e map[string]any) { e["processes"] = e["processes"].([]any)[:1] }, //nolint:forcetypeassert // Created that way above.
			},
			owned: common,
		},
		"AuthPolicy": {
			collection: "auth-policies",
			seed: func(srv *fake.Server) {
				srv.Put("external-jwt-signers", map[string]any{"id": "sig-1", "name": "corporate-sso"})
				srv.Put("external-jwt-signers", map[string]any{"id": "sig-2", "name": "partner-sso"})
			},
			start: reconciled(authpolicy.Kind, func() *v1alpha1.AuthPolicy {
				return &v1alpha1.AuthPolicy{ObjectMeta: meta1("sso"), Spec: v1alpha1.AuthPolicySpec{ForProvider: v1alpha1.AuthPolicyParameters{
					Name: "sso",
					Primary: &v1alpha1.AuthMethods{
						UPDB:   &v1alpha1.UPDBAuth{Allowed: true, MinPasswordLength: ptr.To(int64(12))},
						ExtJWT: &v1alpha1.ExtJWTAuth{Allowed: true, AllowedSigners: []string{"partner-sso", "corporate-sso"}},
					},
					Secondary: &v1alpha1.SecondaryAuth{RequireTOTP: true},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"PasswordLength": func(e map[string]any) { nested(e, "primary", "updb")["minPasswordLength"] = float64(5) },
				"SignerDropped":  func(e map[string]any) { nested(e, "primary", "extJwt")["allowedSigners"] = []any{"sig-1"} },
				"Certificates":   func(e map[string]any) { nested(e, "primary", "cert")["allowed"] = true },
				"TOTP":           func(e map[string]any) { nested(e, "secondary")["requireTotp"] = false },
			},
			owned: common,
		},
		"CertificateAuthority": {
			collection: "cas",
			start: reconciled(certificateauthority.Kind, func() *v1alpha1.CertificateAuthority {
				return &v1alpha1.CertificateAuthority{ObjectMeta: meta1("devices"), Spec: v1alpha1.CertificateAuthoritySpec{ForProvider: v1alpha1.CertificateAuthorityParameters{
					Name: "devices", CertPem: certificate, IsAuthEnabled: true, IsOttCaEnrollmentEnabled: true, IdentityRoles: []string{"sensors"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"Auth":          func(e map[string]any) { e["isAuthEnabled"] = false },
				"IdentityRoles": func(e map[string]any) { e["identityRoles"] = []any{"sensors", "added-by-hand"} },
				// The spec has no claim, so none is wanted.
				"ClaimAdded": func(e map[string]any) {
					e["externalIdClaim"] = map[string]any{"location": "COMMON_NAME", "matcher": "ALL", "parser": "NONE"}
				},
				"NameFormat": func(e map[string]any) { e["identityNameFormat"] = "[caName]-[requestedName]" },
			},
			// Verifying the certificate authority is up to its owner.
			owned: func(e map[string]any) {
				common(e)
				e["isVerified"] = true
				delete(e, "verificationToken")
			},
		},
		"ExternalJWTSigner": {
			collection: "external-jwt-signers",
			start: reconciled(externaljwtsigner.Kind, func() *v1alpha1.ExternalJWTSigner {
				return &v1alpha1.ExternalJWTSigner{ObjectMeta: meta1("corporate-sso"), Spec: v1alpha1.ExternalJWTSignerSpec{ForProvider: v1alpha1.ExternalJWTSignerParameters{
					Name: "corporate-sso", Issuer: "https://sso.example.com", Audience: "ziti", Enabled: true,
					JwksEndpoint: ptr.To("https://sso.example.com/jwks"), Scopes: []string{"openid", "email"}, Tags: map[string]string{"team": "platform"},
				}}}
			}),
			tampers: map[string]func(e map[string]any){
				"Audience": func(e map[string]any) { e["audience"] = "tampered" },
				"Disabled": func(e map[string]any) { e["enabled"] = false },
				"Scopes":   func(e map[string]any) { e["scopes"] = []any{"openid"} },
				// A certificate in place of the JWKS endpoint: the spec has
				// none, so it is removed.
				"CertificateInstead": func(e map[string]any) {
					delete(e, "jwksEndpoint")
					e["certPem"] = certificate
				},
				"TagsGone": func(e map[string]any) { delete(e, "tags") },
			},
			owned: common,
		},
	}
}

// TestDriftOfEveryKind changes and deletes the entity of a managed resource
// of every kind in Ziti behind the provider's back, and checks what the
// provider makes of it, reconciling as Crossplane does:
//
//   - A change of anything the managed resource manages is reverted with an
//     update, and the entity ends up as it was created.
//   - What Ziti owns, and what the spec leaves to Ziti, is left alone.
//   - An entity deleted in Ziti is created anew, once, under a new ID that
//     the managed resource records. An enrollment token of the new entity
//     replaces the old one in the connection secret.
//   - Once an identity or an edge router has enrolled, Ziti no longer
//     reports its token; the connection secret keeps the last one.
//   - Deleting the managed resource deletes the entity.
func TestDriftOfEveryKind(t *testing.T) {
	for name, tc := range driftCases() {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			if tc.seed != nil {
				tc.seed(srv)
			}
			d := tc.start(t, srv)

			mg := d.settle(8)
			id := meta.GetExternalName(mg)
			created := stored(t, srv, tc.collection, id)

			for _, change := range slices.Sorted(maps.Keys(tc.tampers)) {
				tamper(srv, tc.collection, id, tc.tampers[change])
				if diff := cmp.Diff(created, stored(t, srv, tc.collection, id)); diff == "" {
					t.Fatalf("%s: the change made in Ziti changes nothing", change)
				}
				if n := d.reconcile(); n == 0 {
					t.Errorf("%s: want the change made in Ziti to be reverted, got no request that changes something", change)
				}
				mg = d.settle(4)
				if got := meta.GetExternalName(mg); got != id {
					t.Errorf("%s: want the external name to stay %q, got %q", change, id, got)
				}
				if diff := cmp.Diff(created, stored(t, srv, tc.collection, id)); diff != "" {
					t.Errorf("%s: entity after the change was reverted: -want, +got:\n%s", change, diff)
				}
			}

			// An entity that is deleted in Ziti is created anew.
			var secret string
			if tc.token != nil {
				tamper(srv, tc.collection, id, func(e map[string]any) { setToken(e, oldToken) })
				d.reconcile()
				secret = mg.GetName() + "-enrollment"
				if got := string(d.secret(secret)[identity.ConnectionKeyEnrollmentToken]); got != oldToken {
					t.Fatalf("connection secret before the deletion: want the token %q, got %q", oldToken, got)
				}
			}
			srv.Delete(tc.collection, id)
			mg = d.settle(8)
			recreated := meta.GetExternalName(mg)
			if recreated == id || recreated == "" {
				t.Fatalf("external name after the entity was deleted in Ziti: want a new ID, got %q", recreated)
			}
			if got := named(srv, tc.collection, mg); len(got) != 1 || got[0] != recreated {
				t.Errorf("entities named like the managed resource: want only %q, got %v", recreated, got)
			}
			again := stored(t, srv, tc.collection, recreated)
			if tc.token != nil {
				token := tc.token(again)
				if token == "" || token == oldToken {
					t.Errorf("enrollment token of the new entity: want a new one, got %q", token)
				}
				if got := string(d.secret(secret)[identity.ConnectionKeyEnrollmentToken]); got != token {
					t.Errorf("connection secret after the entity was created anew: want the token %q, got %q", token, got)
				}
			}
			// Apart from what Ziti assigns, the new entity is the old one.
			for _, e := range []map[string]any{created, again} {
				delete(e, "id")
				setToken(e, "")
			}
			if diff := cmp.Diff(created, again); diff != "" {
				t.Errorf("entity created anew: -want, +got:\n%s", diff)
			}

			// What Ziti owns is not drift.
			tamper(srv, tc.collection, recreated, tc.owned)
			owned := stored(t, srv, tc.collection, recreated)
			for range 2 {
				if n := d.reconcile(); n != 0 {
					t.Errorf("want no request that changes something after a change of what Ziti owns, got %d: %v", n, srv.Requests())
				}
			}
			if diff := cmp.Diff(owned, stored(t, srv, tc.collection, recreated)); diff != "" {
				t.Errorf("entity after a change of what Ziti owns: -want, +got:\n%s", diff)
			}
			if tc.token != nil {
				if got := d.secret(secret)[identity.ConnectionKeyEnrollmentToken]; len(got) == 0 {
					t.Errorf("connection secret once Ziti reports no token: want the last token kept, got none")
				}
			}

			d.remove()
			for range 3 {
				d.reconcile()
			}
			if d.exists() {
				t.Errorf("want the managed resource to be gone after its deletion")
			}
			if got := named(srv, tc.collection, mg); len(got) != 0 {
				t.Errorf("want the entity to be deleted with its managed resource, got %v", got)
			}
		})
	}
}

// stored returns the entity as generic JSON, as the fake controller reports
// it.
func stored(t *testing.T, srv *fake.Server, collection, id string) map[string]any {
	t.Helper()

	e := entity(t, srv, collection, id)
	if len(e) == 0 {
		t.Fatalf("no entity %q in %s", id, collection)
	}
	e["id"] = id
	return e
}

// tamper changes an entity in Ziti behind the provider's back.
func tamper(srv *fake.Server, collection, id string, change func(e map[string]any)) {
	raw, err := json.Marshal(srv.Entity(collection, id))
	if err != nil {
		panic(err)
	}
	e := map[string]any{}
	if err := json.Unmarshal(raw, &e); err != nil {
		panic(err)
	}
	change(e)
	srv.Put(collection, e)
}

// named returns the IDs of the entities that have the name of the managed
// resource.
func named(srv *fake.Server, collection string, mg resource.Managed) []string {
	name := mg.GetName()
	var ids []string
	for _, e := range srv.Entities(collection) {
		if e["name"] == name {
			ids = append(ids, e["id"].(string)) //nolint:forcetypeassert // The fake controller stores IDs as strings.
		}
	}
	return ids
}
