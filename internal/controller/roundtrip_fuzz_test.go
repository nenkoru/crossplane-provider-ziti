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
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/authpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv1"
	"github.com/crossplane/provider-ziti/internal/controller/configinterceptv1"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmac"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmultiprocess"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckos"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckprocess"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

// A gen makes the choices of a fuzz test from the bytes of its input. Once
// the input is used up, every choice is the first option.
type gen struct{ data []byte }

func (g *gen) byte() byte {
	if len(g.data) == 0 {
		return 0
	}
	b := g.data[0]
	g.data = g.data[1:]
	return b
}

// intn returns a number from 0 to n-1.
func (g *gen) intn(n int) int { return int(g.byte()) % n }

func (g *gen) bool() bool { return g.byte()%2 == 1 }

func (g *gen) pick(options ...string) string { return options[g.intn(len(options))] }

// list returns up to max items picked from the options, which may repeat.
func (g *gen) list(max int, options ...string) []string {
	n := g.intn(max + 1)
	if n == 0 && g.bool() {
		return nil
	}
	out := make([]string, 0, n)
	for range n {
		out = append(out, g.pick(options...))
	}
	return out
}

// unique returns up to max distinct items of the options, in the order
// picked.
func (g *gen) unique(max int, options ...string) []string {
	var out []string
	for _, item := range g.list(max, options...) {
		if !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

// tags returns up to three tags with keys and values picked from the options.
func (g *gen) tags(options ...string) map[string]string {
	n := g.intn(4)
	if n == 0 {
		return nil
	}
	out := map[string]string{}
	for range n {
		out[g.pick(options...)] = g.pick(options...)
	}
	return out
}

func (g *gen) optionalBool() *bool {
	switch g.intn(3) {
	case 1:
		return ptr.To(false)
	case 2:
		return ptr.To(true)
	}
	return nil
}

// roundTrip creates the entity of a managed resource in the fake controller
// and checks that the provider takes it to be up to date right after, as
// Ziti reports it, that an update then changes nothing, and deletes it.
// A spec for which the provider sends something Ziti stores in another form
// fails here: it would be updated on every poll.
func roundTrip[T resource.ModernManaged](t *testing.T, srv *fake.Server, kind generic.Kind[T], mg T) {
	t.Helper()

	ctx := context.Background()
	e := generic.NewExternalClient(kind, zitiClient(t, srv))
	if _, err := e.Create(ctx, mg); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	id := meta.GetExternalName(mg)
	defer func() {
		if _, err := e.Delete(ctx, mg); err != nil {
			t.Errorf("Delete(...): %v", err)
		}
	}()

	o, err := e.Observe(ctx, mg)
	if err != nil || !o.ResourceExists || !o.ResourceUpToDate {
		t.Fatalf("Observe(...) right after Create(...): want an up to date resource, got %+v, %v\nentity: %v", o, err, srv.Entity(kind.Collection, id))
	}

	created := entity(t, srv, kind.Collection, id)
	if _, err := e.Update(ctx, mg); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	if diff := cmp.Diff(created, entity(t, srv, kind.Collection, id)); diff != "" {
		t.Fatalf("Update(...) of an up to date entity changed it: -before, +after:\n%s", diff)
	}
	if o, err := e.Observe(ctx, mg); err != nil || !o.ResourceUpToDate {
		t.Fatalf("Observe(...) after Update(...): want an up to date resource, got %+v, %v", o, err)
	}
}

// seedReferences stores what the managed resources of the round trips refer
// to. Each entity may be given by name or by ID.
func seedReferences(srv *fake.Server) {
	srv.Put("configs", map[string]any{"id": "cfg-1", "name": "web-host"})
	srv.Put("configs", map[string]any{"id": "cfg-2", "name": "web-intercept"})
	srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})
	srv.Put("services", map[string]any{"id": "svc-2", "name": "db"})
	srv.Put("identities", map[string]any{"id": "idn-1", "name": "web-client"})
	srv.Put("posture-checks", map[string]any{"id": "pc-1", "name": "mfa"})
	srv.Put("auth-policies", map[string]any{"id": "ap-1", "name": "certificates"})
	srv.Put("external-jwt-signers", map[string]any{"id": "sig-1", "name": "corporate-sso"})
	srv.Put("external-jwt-signers", map[string]any{"id": "sig-2", "name": "partner-sso"})
}

// fuzzServer returns a fake controller with the references stored, for all
// inputs of a fuzz test.
func fuzzServer(f *testing.F) *fake.Server {
	srv := fake.NewServer()
	f.Cleanup(srv.Close)
	seedReferences(srv)
	return srv
}

// seeds adds inputs that make different choices early on.
func seeds(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	f.Add([]byte{255, 3, 0, 2, 1, 3, 1, 0, 3, 3, 2, 2, 1, 1, 0, 0, 5, 4, 3, 2, 1})
	f.Add([]byte("a service, a policy and an identity"))
}

var roles = []string{"web", "api", "role:web", "Web", "a b", "#hash"}

func FuzzServiceRoundTrip(f *testing.F) {
	srv := fuzzServer(f)
	seeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &gen{data: data}
		p := v1alpha1.ServiceParameters{
			Name:               "web-service",
			EncryptionRequired: g.optionalBool(),
			Configs:            g.list(4, "web-host", "cfg-1", "web-intercept", "cfg-2"),
			RoleAttributes:     g.list(4, roles...),
			Tags:               g.tags("env", "team", "e2e"),
		}
		if g.bool() {
			p.MaxIdleTimeMillis = ptr.To(int64(g.intn(4)) * 30000)
		}
		if g.bool() {
			p.TerminatorStrategy = ptr.To(v1alpha1.TerminatorStrategy(g.pick("smartrouting", "weighted", "random", "sticky")))
		}
		roundTrip(t, srv, service.Kind, &v1alpha1.Service{ObjectMeta: meta1("web-service"), Spec: v1alpha1.ServiceSpec{ForProvider: p}})
	})
}

func FuzzServicePolicyRoundTrip(f *testing.F) {
	srv := fuzzServer(f)
	seeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &gen{data: data}
		p := v1alpha1.ServicePolicyParameters{
			Name:              "web-policy",
			Type:              v1alpha1.ServicePolicyType(g.pick("Dial", "Bind")),
			Semantic:          v1alpha1.ServicePolicySemantic(g.pick("", "AllOf", "AnyOf")),
			ServiceRoles:      g.list(4, "#all", "#web", "@web", "@svc-1", "@db", "#Web"),
			IdentityRoles:     g.list(4, "#all", "#clients", "@web-client", "@idn-1"),
			PostureCheckRoles: g.list(3, "@mfa", "@pc-1", "#strict"),
			Tags:              g.tags("env", "team"),
		}
		roundTrip(t, srv, servicepolicy.Kind, &v1alpha1.ServicePolicy{ObjectMeta: meta1("web-policy"), Spec: v1alpha1.ServicePolicySpec{ForProvider: p}})
	})
}

func FuzzIdentityRoundTrip(f *testing.F) {
	srv := fuzzServer(f)
	seeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &gen{data: data}
		services := []string{"web", "svc-1", "db"}
		p := v1alpha1.IdentityParameters{
			Name:           "web-server",
			Type:           g.pick("", "Default", "User", "Device", "Service"),
			IsAdmin:        g.optionalBool(),
			RoleAttributes: g.list(4, roles...),
			AppData:        g.tags("site", "floor", "seats"),
			Tags:           g.tags("env", "team"),
		}
		if g.bool() {
			p.AuthPolicyID = ptr.To(g.pick("certificates", "ap-1"))
		}
		if g.bool() {
			p.ExternalID = ptr.To(g.pick("", "user@example.com"))
		}
		if g.bool() {
			p.DefaultHostingCost = ptr.To(int32(g.intn(3)) * 100)
		}
		if g.bool() {
			p.DefaultHostingPrecedence = ptr.To(g.pick("default", "required", "failed"))
		}
		for range g.intn(4) {
			if p.ServiceHostingCosts == nil {
				p.ServiceHostingCosts = map[string]int32{}
			}
			p.ServiceHostingCosts[g.pick(services...)] = int32(g.intn(3))
		}
		for range g.intn(3) {
			if p.ServiceHostingPrecedences == nil {
				p.ServiceHostingPrecedences = map[string]string{}
			}
			p.ServiceHostingPrecedences[g.pick(services...)] = g.pick("default", "required", "failed")
		}
		mg := &v1alpha1.Identity{ObjectMeta: meta1("web-server"), Spec: v1alpha1.IdentitySpec{ForProvider: p}}

		// web and svc-1 are the same service, which may only be given
		// twice with the same value.
		if ambiguous(p.ServiceHostingCosts) || ambiguous(p.ServiceHostingPrecedences) {
			_, err := generic.NewExternalClient(identity.Kind, zitiClient(t, srv)).Create(context.Background(), mg)
			if err == nil || !strings.Contains(err.Error(), "name the same entity") {
				t.Fatalf("Create(...) with a service named twice with different values: want an error, got %v", err)
			}
			return
		}
		roundTrip(t, srv, identity.Kind, mg)
	})
}

// ambiguous returns true if the hosting settings give web, under its name
// and its ID, different values.
func ambiguous[V comparable](byService map[string]V) bool {
	byName, named := byService["web"]
	byID, identified := byService["svc-1"]
	return named && identified && byName != byID
}

func FuzzConfigRoundTrip(f *testing.F) {
	srv := fuzzServer(f)
	seeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &gen{data: data}
		if g.bool() {
			p := v1alpha1.ConfigInterceptV1Parameters{
				Name:      "web-intercept-fuzz",
				Addresses: g.unique(3, "web.ziti", "api.ziti", "*.ziti", "10.0.0.1"),
				Protocols: g.unique(2, "tcp", "udp"),
				Tags:      g.tags("env"),
			}
			for range g.intn(3) {
				low := int32(g.intn(4)) * 1000
				p.PortRanges = append(p.PortRanges, v1alpha1.PortRange{Low: low, High: low + int32(g.intn(3))})
			}
			if g.bool() {
				p.DialOptions = &v1alpha1.DialOptions{ConnectTimeoutSeconds: ptr.To(int32(g.intn(3)) * 10)}
			}
			roundTrip(t, srv, configinterceptv1.Kind, &v1alpha1.ConfigInterceptV1{ObjectMeta: meta1("web-intercept-fuzz"), Spec: v1alpha1.ConfigInterceptV1Spec{ForProvider: p}})
			return
		}

		h := v1alpha1.HostTerminator{
			ForwardProtocol:  g.optionalBool(),
			ForwardPort:      g.optionalBool(),
			ForwardAddress:   g.optionalBool(),
			AllowedProtocols: g.unique(2, "tcp", "udp"),
			AllowedAddresses: g.unique(2, "10.0.0.0/8", "web.internal"),
		}
		if h.ForwardProtocol == nil || !*h.ForwardProtocol {
			h.Protocol = ptr.To(g.pick("tcp", "udp"))
		}
		if h.ForwardPort == nil || !*h.ForwardPort {
			h.Port = ptr.To(int32(8000 + g.intn(3)))
		}
		if h.ForwardAddress == nil || !*h.ForwardAddress {
			h.Address = ptr.To(g.pick("localhost", "web.internal"))
		}
		if g.bool() {
			h.ListenOptions = &v1alpha1.ListenOptions{Precedence: ptr.To(g.pick("default", "required")), Cost: ptr.To(int32(g.intn(3)))}
		}
		p := v1alpha1.ConfigHostV1Parameters{Name: "web-host-fuzz", HostTerminator: h, Tags: g.tags("env")}
		roundTrip(t, srv, confighostv1.Kind, &v1alpha1.ConfigHostV1{ObjectMeta: meta1("web-host-fuzz"), Spec: v1alpha1.ConfigHostV1Spec{ForProvider: p}})
	})
}

// hexStrings returns up to max hexadecimal values in the forms a spec may
// give them, some of which Ziti stores the same.
func (g *gen) hexStrings(max int) []v1alpha1.HexString {
	l := g.list(max, "00:1A:2B:3C:4D:5E", "001a2b3c4d5e", "001a.2b3c.4d5e", "0A-1B-2C-3D-4E-5F", "FFEE01", "ffee01", "a9 09 50 2d")
	out := make([]v1alpha1.HexString, 0, len(l))
	for _, v := range l {
		out = append(out, v1alpha1.HexString(v))
	}
	return out
}

var operatingSystems = []string{"Windows", "WindowsServer", "Android", "iOS", "Linux", "macOS"}

func FuzzPostureCheckRoundTrip(f *testing.F) {
	srv := fuzzServer(f)
	seeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &gen{data: data}
		name := "posture-fuzz"
		attributes := g.list(3, roles...)

		switch g.intn(4) {
		case 0:
			p := v1alpha1.PostureCheckOSParameters{Name: name, RoleAttributes: attributes}
			// The schema allows each type once.
			for _, os := range g.unique(4, operatingSystems...) {
				p.OperatingSystems = append(p.OperatingSystems, v1alpha1.OperatingSystem{Type: os, Versions: g.list(3, ">=14.0.0", ">=13.0.0", "10.0.19041")})
			}
			if len(p.OperatingSystems) == 0 {
				p.OperatingSystems = []v1alpha1.OperatingSystem{{Type: "Linux"}}
			}
			roundTrip(t, srv, posturecheckos.Kind, &v1alpha1.PostureCheckOS{ObjectMeta: meta1(name), Spec: v1alpha1.PostureCheckOSSpec{ForProvider: p}})
		case 1:
			p := v1alpha1.PostureCheckMacParameters{Name: name, RoleAttributes: attributes, MacAddresses: g.hexStrings(4)}
			if len(p.MacAddresses) == 0 {
				p.MacAddresses = []v1alpha1.HexString{"00:1A:2B:3C:4D:5E"}
			}
			roundTrip(t, srv, posturecheckmac.Kind, &v1alpha1.PostureCheckMac{ObjectMeta: meta1(name), Spec: v1alpha1.PostureCheckMacSpec{ForProvider: p}})
		case 2:
			p := v1alpha1.PostureCheckProcessParameters{Name: name, RoleAttributes: attributes, Process: v1alpha1.Process{
				OsType: g.pick("Windows", "Linux", "macOS"), Path: g.pick("/usr/bin/agent", `C:\agent.exe`),
				Hashes: g.hexStrings(3), SignerFingerprint: v1alpha1.OptionalHexString(g.pick("", "A9:09:50:2D", "a90950 2d")),
			}}
			roundTrip(t, srv, posturecheckprocess.Kind, &v1alpha1.PostureCheckProcess{ObjectMeta: meta1(name), Spec: v1alpha1.PostureCheckProcessSpec{ForProvider: p}})
		default:
			p := v1alpha1.PostureCheckMultiProcessParameters{Name: name, RoleAttributes: attributes, Semantic: g.pick("", "AllOf", "AnyOf")}
			// The schema allows each operating system and path once.
			seen := map[string]bool{}
			for range g.intn(4) + 1 {
				process := v1alpha1.MultiProcess{
					OsType: g.pick("Windows", "Linux", "macOS"), Path: g.pick("/usr/bin/agent", "/opt/agent", `C:\agent.exe`),
					Hashes: g.hexStrings(3), SignerFingerprints: g.hexStrings(2),
				}
				if key := process.OsType + "-" + process.Path; !seen[key] {
					seen[key] = true
					p.Processes = append(p.Processes, process)
				}
			}
			roundTrip(t, srv, posturecheckmultiprocess.Kind, &v1alpha1.PostureCheckMultiProcess{ObjectMeta: meta1(name), Spec: v1alpha1.PostureCheckMultiProcessSpec{ForProvider: p}})
		}
	})
}

func FuzzAuthPolicyRoundTrip(f *testing.F) {
	srv := fuzzServer(f)
	seeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &gen{data: data}
		signers := []string{"corporate-sso", "sig-1", "partner-sso", "sig-2"}
		p := v1alpha1.AuthPolicyParameters{Name: "policy-fuzz", Tags: g.tags("team")}
		if g.bool() {
			p.Primary = &v1alpha1.AuthMethods{}
			if g.bool() {
				p.Primary.Cert = &v1alpha1.CertAuth{Allowed: g.bool(), AllowExpiredCerts: g.bool()}
			}
			if g.bool() {
				p.Primary.UPDB = &v1alpha1.UPDBAuth{Allowed: g.bool(), RequireMixedCase: g.bool(), RequireNumberChar: g.bool()}
				if g.bool() {
					p.Primary.UPDB.MinPasswordLength = ptr.To(int64(5 + g.intn(10)))
				}
				if g.bool() {
					p.Primary.UPDB.LockoutDurationMinutes = ptr.To(int64(g.intn(3)))
				}
			}
			if g.bool() {
				p.Primary.ExtJWT = &v1alpha1.ExtJWTAuth{Allowed: g.bool(), AllowedSigners: g.list(4, signers...)}
			}
		}
		if g.bool() {
			p.Secondary = &v1alpha1.SecondaryAuth{RequireTOTP: g.bool()}
			if g.bool() {
				p.Secondary.RequireExtJWTSigner = ptr.To(g.pick(append([]string{""}, signers...)...))
			}
		}
		roundTrip(t, srv, authpolicy.Kind, &v1alpha1.AuthPolicy{ObjectMeta: meta1("policy-fuzz"), Spec: v1alpha1.AuthPolicySpec{ForProvider: p}})
	})
}
