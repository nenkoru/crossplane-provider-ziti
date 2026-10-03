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
	"os"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis"
	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckdomain"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmac"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmultiprocess"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckos"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckprocess"
)

// The specs below give lists out of order and with duplicates, and
// hexadecimal values in upper case and with separators. The entities expected
// in Ziti hold them the way Ziti stores them: the fake controller keeps what
// it is sent, so an entity in any other form would be reported as outdated by
// the real one on every poll.

func TestPostureCheckDomain(t *testing.T) {
	lifecycle[*v1alpha1.PostureCheckDomain]{
		kind: posturecheckdomain.Kind,
		mg: &v1alpha1.PostureCheckDomain{
			ObjectMeta: meta1("corporate"),
			Spec: v1alpha1.PostureCheckDomainSpec{ForProvider: v1alpha1.PostureCheckDomainParameters{
				Name:    "corporate",
				Domains: []string{"corp.example.com", "branch.example.com", "corp.example.com"},
			}},
		},
		created: map[string]any{
			"name":           "corporate",
			"typeId":         "DOMAIN",
			"roleAttributes": []any{},
			"domains":        []any{"branch.example.com", "corp.example.com"},
			"tags":           map[string]any{},
		},
		update: func(mg *v1alpha1.PostureCheckDomain) {
			mg.Spec.ForProvider.Domains = []string{"corp.example.com"}
			mg.Spec.ForProvider.RoleAttributes = []string{"managed"}
		},
		updated: map[string]any{
			"name":           "corporate",
			"typeId":         "DOMAIN",
			"roleAttributes": []any{"managed"},
			"domains":        []any{"corp.example.com"},
			"tags":           map[string]any{},
		},
	}.run(t)
}

func TestPostureCheckMac(t *testing.T) {
	lifecycle[*v1alpha1.PostureCheckMac]{
		kind: posturecheckmac.Kind,
		mg: &v1alpha1.PostureCheckMac{
			ObjectMeta: meta1("registered"),
			Spec: v1alpha1.PostureCheckMacSpec{ForProvider: v1alpha1.PostureCheckMacParameters{
				Name:         "registered",
				MacAddresses: []v1alpha1.HexString{"0A-1B-2C-3D-4E-5F", "00:1a:2b:3c:4d:5e", "001A.2B3C.4D5E"},
			}},
		},
		created: map[string]any{
			"name":           "registered",
			"typeId":         "MAC",
			"roleAttributes": []any{},
			"macAddresses":   []any{"001a2b3c4d5e", "0a1b2c3d4e5f"},
			"tags":           map[string]any{},
		},
		update: func(mg *v1alpha1.PostureCheckMac) {
			mg.Spec.ForProvider.MacAddresses = []v1alpha1.HexString{"00:1A:2B:3C:4D:5E"}
			mg.Spec.ForProvider.Tags = map[string]string{"team": "it"}
		},
		updated: map[string]any{
			"name":           "registered",
			"typeId":         "MAC",
			"roleAttributes": []any{},
			"macAddresses":   []any{"001a2b3c4d5e"},
			"tags":           map[string]any{"team": "it"},
		},
	}.run(t)
}

func TestPostureCheckProcess(t *testing.T) {
	lifecycle[*v1alpha1.PostureCheckProcess]{
		kind: posturecheckprocess.Kind,
		mg: &v1alpha1.PostureCheckProcess{
			ObjectMeta: meta1("agent"),
			Spec: v1alpha1.PostureCheckProcessSpec{ForProvider: v1alpha1.PostureCheckProcessParameters{
				Name: "agent",
				Process: v1alpha1.Process{
					OsType:            "Windows",
					Path:              `C:\Program Files\Agent\agent.exe`,
					Hashes:            []v1alpha1.HexString{"FFEE01", "00aa11", "ffee01"},
					SignerFingerprint: "A9:09:50:2D",
				},
			}},
		},
		created: map[string]any{
			"name":           "agent",
			"typeId":         "PROCESS",
			"roleAttributes": []any{},
			"process": map[string]any{
				"osType":            "Windows",
				"path":              `C:\Program Files\Agent\agent.exe`,
				"hashes":            []any{"00aa11", "ffee01"},
				"signerFingerprint": "a909502d",
			},
			"tags": map[string]any{},
		},
		// Hashes and a fingerprint that are removed from the spec are sent as
		// empty, so that Ziti drops them.
		update: func(mg *v1alpha1.PostureCheckProcess) {
			mg.Spec.ForProvider.Process = v1alpha1.Process{OsType: "Linux", Path: "/usr/bin/agent"}
		},
		updated: map[string]any{
			"name":           "agent",
			"typeId":         "PROCESS",
			"roleAttributes": []any{},
			"process": map[string]any{
				"osType":            "Linux",
				"path":              "/usr/bin/agent",
				"hashes":            []any{},
				"signerFingerprint": "",
			},
			"tags": map[string]any{},
		},
	}.run(t)
}

func TestPostureCheckMultiProcess(t *testing.T) {
	lifecycle[*v1alpha1.PostureCheckMultiProcess]{
		kind: posturecheckmultiprocess.Kind,
		mg: &v1alpha1.PostureCheckMultiProcess{
			ObjectMeta: meta1("agents"),
			Spec: v1alpha1.PostureCheckMultiProcessSpec{ForProvider: v1alpha1.PostureCheckMultiProcessParameters{
				Name: "agents",
				Processes: []v1alpha1.MultiProcess{
					{OsType: "macOS", Path: "/Library/Agent/agent", SignerFingerprints: []v1alpha1.HexString{"F5:B9", "a9 09", "f5b9"}},
					{OsType: "Linux", Path: "/usr/bin/agent", Hashes: []v1alpha1.HexString{"FFEE01", "00aa11"}},
					{OsType: "Linux", Path: "/opt/agent"},
				},
			}},
		},
		// Ziti returns the processes in the order of their operating system
		// and path. The semantic the API server defaults is AllOf.
		created: map[string]any{
			"name":           "agents",
			"typeId":         "PROCESS_MULTI",
			"roleAttributes": []any{},
			"semantic":       "AllOf",
			"processes": []any{
				map[string]any{"osType": "Linux", "path": "/opt/agent", "hashes": []any{}, "signerFingerprints": []any{}},
				map[string]any{"osType": "Linux", "path": "/usr/bin/agent", "hashes": []any{"00aa11", "ffee01"}, "signerFingerprints": []any{}},
				map[string]any{"osType": "macOS", "path": "/Library/Agent/agent", "hashes": []any{}, "signerFingerprints": []any{"a909", "f5b9"}},
			},
			"tags": map[string]any{},
		},
		update: func(mg *v1alpha1.PostureCheckMultiProcess) {
			mg.Spec.ForProvider.Semantic = "AnyOf"
			mg.Spec.ForProvider.Processes = []v1alpha1.MultiProcess{{OsType: "Windows", Path: `C:\agent.exe`, Hashes: []v1alpha1.HexString{"00AA11"}}}
		},
		updated: map[string]any{
			"name":           "agents",
			"typeId":         "PROCESS_MULTI",
			"roleAttributes": []any{},
			"semantic":       "AnyOf",
			"processes": []any{
				map[string]any{"osType": "Windows", "path": `C:\agent.exe`, "hashes": []any{"00aa11"}, "signerFingerprints": []any{}},
			},
			"tags": map[string]any{},
		},
	}.run(t)
}

func TestPostureCheckOSOutOfOrder(t *testing.T) {
	spec := func() *v1alpha1.PostureCheckOS {
		return &v1alpha1.PostureCheckOS{
			ObjectMeta: meta1("supported"),
			Spec: v1alpha1.PostureCheckOSSpec{ForProvider: v1alpha1.PostureCheckOSParameters{
				Name: "supported",
				OperatingSystems: []v1alpha1.OperatingSystem{
					{Type: "macOS", Versions: []string{">=14.0.0", ">=13.0.0"}},
					{Type: "Windows"},
					{Type: "Linux", Versions: []string{">=6.0.0", ">=5.0.0", ">=6.0.0"}},
				},
			}},
		}
	}

	// Ziti returns the operating systems in the order of their type.
	lifecycle[*v1alpha1.PostureCheckOS]{
		kind: posturecheckos.Kind,
		mg:   spec(),
		created: map[string]any{
			"name":           "supported",
			"typeId":         "OS",
			"roleAttributes": []any{},
			"operatingSystems": []any{
				map[string]any{"type": "Linux", "versions": []any{">=5.0.0", ">=6.0.0"}},
				map[string]any{"type": "Windows", "versions": []any{}},
				map[string]any{"type": "macOS", "versions": []any{">=13.0.0", ">=14.0.0"}},
			},
			"tags": map[string]any{},
		},
		update: func(mg *v1alpha1.PostureCheckOS) {
			mg.Spec.ForProvider.OperatingSystems = []v1alpha1.OperatingSystem{
				{Type: "iOS", Versions: []string{">=17.0.0"}},
				{Type: "Android", Versions: []string{">=14.0.0"}},
			}
		},
		updated: map[string]any{
			"name":           "supported",
			"typeId":         "OS",
			"roleAttributes": []any{},
			"operatingSystems": []any{
				map[string]any{"type": "Android", "versions": []any{">=14.0.0"}},
				map[string]any{"type": "iOS", "versions": []any{">=17.0.0"}},
			},
			"tags": map[string]any{},
		},
	}.run(t)

	// The entity as Ziti reports it for that spec is up to date, so the
	// provider does not update it again after the creation.
	srv := fake.NewServer()
	defer srv.Close()
	srv.Put("posture-checks", map[string]any{
		"id":             "os-1",
		"name":           "supported",
		"typeId":         "OS",
		"roleAttributes": []any{},
		"operatingSystems": []any{
			map[string]any{"type": "Linux", "versions": []any{">=5.0.0", ">=6.0.0"}},
			map[string]any{"type": "Windows", "versions": nil},
			map[string]any{"type": "macOS", "versions": []any{">=13.0.0", ">=14.0.0"}},
		},
		"tags": map[string]any{},
	})
	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}
	mg := spec()
	meta.SetExternalName(mg, "os-1")
	o, err := generic.NewExternalClient(posturecheckos.Kind, api).Observe(context.Background(), mg)
	if err != nil || !o.ResourceExists || !o.ResourceUpToDate {
		t.Errorf("Observe(...) of the entity Ziti stores for the spec: want an up to date resource, got %+v, %v", o, err)
	}
}

// TestPostureCheckSchemas checks against a real Kubernetes API server that
// the schemas of the posture check kinds turn away what the Ziti API does,
// and what Ziti would store as something else than was declared. Like
// TestControllers it needs the envtest binaries.
func TestPostureCheckSchemas(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// CI must not pass by silently skipping the test.
		if os.Getenv("CI") != "" {
			t.Fatal("KUBEBUILDER_ASSETS is not set")
		}
		t.Skip("KUBEBUILDER_ASSETS is not set")
	}

	env := &envtest.Environment{CRDDirectoryPaths: []string{"../../package/crds"}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("cannot start the API server: %v", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Logf("cannot stop the API server: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot build scheme: %v", err)
	}
	k, err := kube.New(cfg, kube.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	ctx := context.Background()
	pc := xpv2.ManagedResourceSpec{ProviderConfigReference: &xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"}}

	domain := func(name string, domains ...string) kube.Object {
		return &v1alpha1.PostureCheckDomain{
			ObjectMeta: meta1(name),
			Spec: v1alpha1.PostureCheckDomainSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.PostureCheckDomainParameters{
				Name: name, Domains: domains,
			}},
		}
	}
	mac := func(name string, addresses ...v1alpha1.HexString) kube.Object {
		return &v1alpha1.PostureCheckMac{
			ObjectMeta: meta1(name),
			Spec: v1alpha1.PostureCheckMacSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.PostureCheckMacParameters{
				Name: name, MacAddresses: addresses,
			}},
		}
	}
	process := func(name string, p v1alpha1.Process) kube.Object {
		return &v1alpha1.PostureCheckProcess{
			ObjectMeta: meta1(name),
			Spec: v1alpha1.PostureCheckProcessSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.PostureCheckProcessParameters{
				Name: name, Process: p,
			}},
		}
	}
	multi := func(name, semantic string, processes ...v1alpha1.MultiProcess) *v1alpha1.PostureCheckMultiProcess {
		return &v1alpha1.PostureCheckMultiProcess{
			ObjectMeta: meta1(name),
			Spec: v1alpha1.PostureCheckMultiProcessSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.PostureCheckMultiProcessParameters{
				Name: name, Semantic: semantic, Processes: processes,
			}},
		}
	}

	agents := multi("agents", "",
		v1alpha1.MultiProcess{OsType: "Linux", Path: "/usr/bin/agent", Hashes: []v1alpha1.HexString{"FFEE01"}},
		v1alpha1.MultiProcess{OsType: "macOS", Path: "/usr/bin/agent", SignerFingerprints: []v1alpha1.HexString{"A9:09:50:2D"}},
	)
	create(ctx, t, k,
		domain("corporate", "corp.example.com"),
		mac("registered", "00:1A:2B:3C:4D:5E", "0a-1b-2c-3d-4e-5f", "001a.2b3c.4d5e", "001a2b3c4d5e"),
		process("agent", v1alpha1.Process{OsType: "Windows", Path: `C:\agent.exe`, Hashes: []v1alpha1.HexString{"FFEE01"}, SignerFingerprint: "A9 09 50 2D"}),
		agents,
	)
	if got := agents.Spec.ForProvider.Semantic; got != "AllOf" {
		t.Errorf("semantic of a multi process check that names none: want AllOf, got %q", got)
	}

	for reason, o := range map[string]kube.Object{
		"a domain check without domains":                   domain("invalid"),
		"a MAC address check without addresses":            mac("invalid"),
		"a MAC address that is not hexadecimal":            mac("invalid", "00:1A:2B:3C:4D:5G"),
		"a process of an unknown operating system":         process("invalid", v1alpha1.Process{OsType: "linux", Path: "/usr/bin/agent"}),
		"a hash with a prefix":                             process("invalid", v1alpha1.Process{OsType: "Linux", Path: "/usr/bin/agent", Hashes: []v1alpha1.HexString{"sha512:ffee01"}}),
		"a fingerprint that is not hexadecimal":            process("invalid", v1alpha1.Process{OsType: "Linux", Path: "/usr/bin/agent", SignerFingerprint: "unsigned"}),
		"a multi process check without processes":          multi("invalid", "AnyOf"),
		"a multi process check with an unknown semantic":   multi("invalid", "anyOf", v1alpha1.MultiProcess{OsType: "Linux", Path: "/usr/bin/agent"}),
		"a multi process check that names a process twice": multi("invalid", "AnyOf", v1alpha1.MultiProcess{OsType: "Linux", Path: "/usr/bin/agent"}, v1alpha1.MultiProcess{OsType: "Linux", Path: "/usr/bin/agent"}),
		"an OS check that names an operating system twice": &v1alpha1.PostureCheckOS{
			ObjectMeta: meta1("invalid"),
			Spec: v1alpha1.PostureCheckOSSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.PostureCheckOSParameters{
				Name: "invalid", OperatingSystems: []v1alpha1.OperatingSystem{{Type: "Linux"}, {Type: "Linux", Versions: []string{">=6.0.0"}}},
			}},
		},
		"a multi process check with a fingerprint with a label": multi("invalid", "AnyOf", v1alpha1.MultiProcess{OsType: "Linux", Path: "/usr/bin/agent", SignerFingerprints: []v1alpha1.HexString{"sha1=a909"}}),
	} {
		if err := k.Create(ctx, o); !kerrors.IsInvalid(err) {
			t.Errorf("creating %s: want it to be rejected as invalid, got %v", reason, err)
		}
	}
}
