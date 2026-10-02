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
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/authpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv1"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv2"
	"github.com/crossplane/provider-ziti/internal/controller/configinterceptv1"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouter"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/identityca"
	"github.com/crossplane/provider-ziti/internal/controller/identitynone"
	"github.com/crossplane/provider-ziti/internal/controller/identityupdb"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckmfa"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheckos"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/serviceedgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

// A lifecycle describes a managed resource going through creation, an update
// and deletion against a fake Ziti controller.
type lifecycle[T resource.ModernManaged] struct {
	kind generic.Kind[T]

	// seed stores the entities the managed resource refers to.
	seed func(srv *fake.Server)

	// mg is the managed resource to create.
	mg T

	// created is the entity expected in Ziti after creation, without its id.
	created map[string]any

	// update changes the spec of the managed resource.
	update func(mg T)

	// updated is the entity expected in Ziti after the update, without its id.
	updated map[string]any

	// details are the connection details expected once the entity exists.
	details managed.ConnectionDetails
}

func (l lifecycle[T]) run(t *testing.T) {
	t.Helper()

	srv := fake.NewServer()
	defer srv.Close()
	if l.seed != nil {
		l.seed(srv)
	}

	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	ctx := context.Background()
	e := generic.NewExternalClient(l.kind, api)
	mg := l.mg

	// Nothing exists before the resource has an external name.
	o, err := e.Observe(ctx, mg)
	if err != nil || o.ResourceExists {
		t.Fatalf("Observe(...) before creation: want a missing resource, got %+v, %v", o, err)
	}

	if _, err := e.Create(ctx, mg); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	id := meta.GetExternalName(mg)
	if id == "" {
		t.Fatalf("Create(...): want the external name to be the Ziti id, got none")
	}
	if diff := cmp.Diff(l.created, entity(t, srv, l.kind.Collection, id)); diff != "" {
		t.Errorf("entity after Create(...): -want, +got:\n%s", diff)
	}

	o, err = e.Observe(ctx, mg)
	if err != nil || !o.ResourceExists || !o.ResourceUpToDate {
		t.Fatalf("Observe(...) after creation: want an up to date resource, got %+v, %v", o, err)
	}
	if diff := cmp.Diff(l.details, o.ConnectionDetails); diff != "" {
		t.Errorf("connection details: -want, +got:\n%s", diff)
	}
	if got := mg.GetCondition(xpv2.TypeReady).Status; got != corev1.ConditionTrue {
		t.Errorf("Ready condition after creation: want True, got %q", got)
	}

	l.update(mg)
	o, err = e.Observe(ctx, mg)
	if err != nil || !o.ResourceExists || o.ResourceUpToDate {
		t.Fatalf("Observe(...) after a spec change: want an outdated resource, got %+v, %v", o, err)
	}
	if _, err := e.Update(ctx, mg); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	if diff := cmp.Diff(l.updated, entity(t, srv, l.kind.Collection, id)); diff != "" {
		t.Errorf("entity after Update(...): -want, +got:\n%s", diff)
	}
	o, err = e.Observe(ctx, mg)
	if err != nil || !o.ResourceUpToDate {
		t.Fatalf("Observe(...) after the update: want an up to date resource, got %+v, %v", o, err)
	}

	if _, err := e.Delete(ctx, mg); err != nil {
		t.Fatalf("Delete(...): %v", err)
	}
	if got := srv.Entity(l.kind.Collection, id); got != nil {
		t.Errorf("entity after Delete(...): want none, got %v", got)
	}
	o, err = e.Observe(ctx, mg)
	if err != nil || o.ResourceExists {
		t.Fatalf("Observe(...) after deletion: want a missing resource, got %+v, %v", o, err)
	}
	if _, err := e.Delete(ctx, mg); err != nil {
		t.Errorf("Delete(...) of a missing entity: %v", err)
	}
}

// entity returns the stored entity without its id and the time it was
// created, as generic JSON.
func entity(t *testing.T, srv *fake.Server, collection, id string) map[string]any {
	t.Helper()

	raw, err := json.Marshal(srv.Entity(collection, id))
	if err != nil {
		t.Fatalf("cannot encode entity: %v", err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("cannot decode entity: %v", err)
	}
	delete(out, "id")
	delete(out, "createdAt")
	return out
}

func meta1(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: "default"}
}

func TestService(t *testing.T) {
	lifecycle[*v1alpha1.Service]{
		kind: service.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("configs", map[string]any{"id": "cfg-1", "name": "web-host"})
			srv.Put("configs", map[string]any{"id": "cfg-2", "name": "web-intercept"})
		},
		mg: &v1alpha1.Service{
			ObjectMeta: meta1("web"),
			Spec: v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{
				Name:               "web",
				EncryptionRequired: ptr.To(true),
				Configs:            []string{"web-host", "cfg-2"},
				RoleAttributes:     []string{"web"},
			}},
		},
		created: map[string]any{
			"name":               "web",
			"encryptionRequired": true,
			"configs":            []any{"cfg-1", "cfg-2"},
			"roleAttributes":     []any{"web"},
			"tags":               map[string]any{},
		},
		update: func(mg *v1alpha1.Service) {
			mg.Spec.ForProvider.EncryptionRequired = ptr.To(false)
			mg.Spec.ForProvider.Configs = []string{"web-host"}
			mg.Spec.ForProvider.RoleAttributes = nil
			mg.Spec.ForProvider.TerminatorStrategy = ptr.To(v1alpha1.TerminatorStrategyWeighted)
			mg.Spec.ForProvider.MaxIdleTimeMillis = ptr.To(int64(60000))
			mg.Spec.ForProvider.Tags = map[string]string{"team": "web"}
		},
		updated: map[string]any{
			"name":               "web",
			"encryptionRequired": false,
			"configs":            []any{"cfg-1"},
			"roleAttributes":     []any{},
			"terminatorStrategy": "weighted",
			"maxIdleTimeMillis":  float64(60000),
			"tags":               map[string]any{"team": "web"},
		},
	}.run(t)
}

func TestServiceWaitsForItsConfigs(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	mg := &v1alpha1.Service{
		ObjectMeta: meta1("web"),
		Spec:       v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{Name: "web", Configs: []string{"web-host"}}},
	}

	_, err = generic.NewExternalClient(service.Kind, api).Create(context.Background(), mg)
	if err == nil || !strings.Contains(err.Error(), `no entity named "web-host" in configs`) {
		t.Errorf("Create(...): want an error about the missing config, got %v", err)
	}
	if got := srv.Len("services"); got != 0 {
		t.Errorf("want no service to be created, got %d", got)
	}
}

func TestConfigHostV1(t *testing.T) {
	lifecycle[*v1alpha1.ConfigHostV1]{
		kind: confighostv1.Kind,
		mg: &v1alpha1.ConfigHostV1{
			ObjectMeta: meta1("web-host"),
			Spec: v1alpha1.ConfigHostV1Spec{ForProvider: v1alpha1.ConfigHostV1Parameters{
				Name: "web-host",
				HostTerminator: v1alpha1.HostTerminator{
					Address:        ptr.To("localhost"),
					Port:           ptr.To(int32(8080)),
					Protocol:       ptr.To("tcp"),
					ForwardAddress: ptr.To(false),
				},
				Tags: map[string]string{"team": "web"},
			}},
		},
		created: map[string]any{
			"name":         "web-host",
			"configTypeId": "host-v1-id",
			"data":         map[string]any{"address": "localhost", "port": float64(8080), "protocol": "tcp"},
			"tags":         map[string]any{"team": "web"},
		},
		update: func(mg *v1alpha1.ConfigHostV1) {
			mg.Spec.ForProvider.Address = nil
			mg.Spec.ForProvider.ForwardAddress = ptr.To(true)
			mg.Spec.ForProvider.AllowedAddresses = []string{"10.0.0.0/8"}
			mg.Spec.ForProvider.ListenOptions = &v1alpha1.ListenOptions{Precedence: ptr.To("required")}
			mg.Spec.ForProvider.Tags = nil
		},
		updated: map[string]any{
			"name":         "web-host",
			"configTypeId": "host-v1-id",
			"data": map[string]any{
				"port":             float64(8080),
				"protocol":         "tcp",
				"forwardAddress":   true,
				"allowedAddresses": []any{"10.0.0.0/8"},
				"listenOptions":    map[string]any{"precedence": "required"},
			},
			"tags": map[string]any{},
		},
	}.run(t)
}

func TestConfigHostV2(t *testing.T) {
	lifecycle[*v1alpha1.ConfigHostV2]{
		kind: confighostv2.Kind,
		mg: &v1alpha1.ConfigHostV2{
			ObjectMeta: meta1("web-hosts"),
			Spec: v1alpha1.ConfigHostV2Spec{ForProvider: v1alpha1.ConfigHostV2Parameters{
				Name: "web-hosts",
				Terminators: []v1alpha1.HostTerminator{
					{Address: ptr.To("web-1.internal"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp"), ForwardPort: ptr.To(false)},
					{Address: ptr.To("web-2.internal"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp")},
				},
			}},
		},
		created: map[string]any{
			"name":         "web-hosts",
			"configTypeId": "host-v2-id",
			"data": map[string]any{"terminators": []any{
				map[string]any{"address": "web-1.internal", "port": float64(8080), "protocol": "tcp"},
				map[string]any{"address": "web-2.internal", "port": float64(8080), "protocol": "tcp"},
			}},
			"tags": map[string]any{},
		},
		update: func(mg *v1alpha1.ConfigHostV2) {
			mg.Spec.ForProvider.Terminators = []v1alpha1.HostTerminator{{
				Address:       ptr.To("web-1.internal"),
				Protocol:      ptr.To("tcp"),
				ForwardPort:   ptr.To(true),
				ListenOptions: &v1alpha1.ListenOptions{Precedence: ptr.To("required")},
				AllowedPortRanges: []v1alpha1.PortRange{
					{Low: 8000, High: 8999},
				},
			}}
			mg.Spec.ForProvider.Tags = map[string]string{"team": "web"}
		},
		updated: map[string]any{
			"name":         "web-hosts",
			"configTypeId": "host-v2-id",
			"data": map[string]any{"terminators": []any{
				map[string]any{
					"address":           "web-1.internal",
					"protocol":          "tcp",
					"forwardPort":       true,
					"listenOptions":     map[string]any{"precedence": "required"},
					"allowedPortRanges": []any{map[string]any{"low": float64(8000), "high": float64(8999)}},
				},
			}},
			"tags": map[string]any{"team": "web"},
		},
	}.run(t)
}

func TestConfigInterceptV1(t *testing.T) {
	lifecycle[*v1alpha1.ConfigInterceptV1]{
		kind: configinterceptv1.Kind,
		mg: &v1alpha1.ConfigInterceptV1{
			ObjectMeta: meta1("web-intercept"),
			Spec: v1alpha1.ConfigInterceptV1Spec{ForProvider: v1alpha1.ConfigInterceptV1Parameters{
				Name:       "web-intercept",
				Addresses:  []string{"web.ziti"},
				Protocols:  []string{"tcp"},
				PortRanges: []v1alpha1.PortRange{{Low: 80, High: 80}},
			}},
		},
		created: map[string]any{
			"name":         "web-intercept",
			"configTypeId": "intercept-v1-id",
			"data": map[string]any{
				"addresses":  []any{"web.ziti"},
				"protocols":  []any{"tcp"},
				"portRanges": []any{map[string]any{"low": float64(80), "high": float64(80)}},
			},
			"tags": map[string]any{},
		},
		update: func(mg *v1alpha1.ConfigInterceptV1) {
			mg.Spec.ForProvider.Addresses = []string{"web.ziti", "www.ziti"}
			mg.Spec.ForProvider.DialOptions = &v1alpha1.DialOptions{ConnectTimeoutSeconds: ptr.To(int32(5))}
		},
		updated: map[string]any{
			"name":         "web-intercept",
			"configTypeId": "intercept-v1-id",
			"data": map[string]any{
				"addresses":   []any{"web.ziti", "www.ziti"},
				"protocols":   []any{"tcp"},
				"portRanges":  []any{map[string]any{"low": float64(80), "high": float64(80)}},
				"dialOptions": map[string]any{"connectTimeoutSeconds": float64(5)},
			},
			"tags": map[string]any{},
		},
	}.run(t)
}

func TestServicePolicy(t *testing.T) {
	lifecycle[*v1alpha1.ServicePolicy]{
		kind: servicepolicy.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})
			srv.Put("identities", map[string]any{"id": "idn-1", "name": "web-client"})
		},
		mg: &v1alpha1.ServicePolicy{
			ObjectMeta: meta1("web-dial"),
			Spec: v1alpha1.ServicePolicySpec{ForProvider: v1alpha1.ServicePolicyParameters{
				Name:          "web-dial",
				Type:          v1alpha1.ServicePolicyTypeDial,
				ServiceRoles:  []string{"@web"},
				IdentityRoles: []string{"#clients", "@web-client"},
			}},
		},
		created: map[string]any{
			"name":              "web-dial",
			"type":              "Dial",
			"semantic":          "AllOf",
			"serviceRoles":      []any{"@svc-1"},
			"identityRoles":     []any{"#clients", "@idn-1"},
			"postureCheckRoles": []any{},
			"tags":              map[string]any{},
		},
		update: func(mg *v1alpha1.ServicePolicy) {
			mg.Spec.ForProvider.Semantic = v1alpha1.ServicePolicySemanticAnyOf
			mg.Spec.ForProvider.IdentityRoles = []string{"#all"}
		},
		updated: map[string]any{
			"name":              "web-dial",
			"type":              "Dial",
			"semantic":          "AnyOf",
			"serviceRoles":      []any{"@svc-1"},
			"identityRoles":     []any{"#all"},
			"postureCheckRoles": []any{},
			"tags":              map[string]any{},
		},
	}.run(t)
}

func TestServiceEdgeRouterPolicy(t *testing.T) {
	lifecycle[*v1alpha1.ServiceEdgeRouterPolicy]{
		kind: serviceedgerouterpolicy.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("edge-routers", map[string]any{"id": "er-1", "name": "router-1"})
		},
		mg: &v1alpha1.ServiceEdgeRouterPolicy{
			ObjectMeta: meta1("all-services"),
			Spec: v1alpha1.ServiceEdgeRouterPolicySpec{ForProvider: v1alpha1.ServiceEdgeRouterPolicyParameters{
				Name:            "all-services",
				Semantic:        v1alpha1.ServiceEdgeRouterPolicySemanticAnyOf,
				ServiceRoles:    []string{"#all"},
				EdgeRouterRoles: []string{"@router-1"},
			}},
		},
		created: map[string]any{
			"name":            "all-services",
			"semantic":        "AnyOf",
			"serviceRoles":    []any{"#all"},
			"edgeRouterRoles": []any{"@er-1"},
			"tags":            map[string]any{},
		},
		update: func(mg *v1alpha1.ServiceEdgeRouterPolicy) {
			mg.Spec.ForProvider.EdgeRouterRoles = []string{"#public"}
		},
		updated: map[string]any{
			"name":            "all-services",
			"semantic":        "AnyOf",
			"serviceRoles":    []any{"#all"},
			"edgeRouterRoles": []any{"#public"},
			"tags":            map[string]any{},
		},
	}.run(t)
}

func TestEdgeRouterPolicy(t *testing.T) {
	lifecycle[*v1alpha1.EdgeRouterPolicy]{
		kind: edgerouterpolicy.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("identities", map[string]any{"id": "idn-1", "name": "web-client"})
		},
		mg: &v1alpha1.EdgeRouterPolicy{
			ObjectMeta: meta1("all-routers"),
			Spec: v1alpha1.EdgeRouterPolicySpec{ForProvider: v1alpha1.EdgeRouterPolicyParameters{
				Name:            "all-routers",
				EdgeRouterRoles: []string{"#all"},
				IdentityRoles:   []string{"@web-client"},
			}},
		},
		created: map[string]any{
			"name":            "all-routers",
			"semantic":        "AllOf",
			"edgeRouterRoles": []any{"#all"},
			"identityRoles":   []any{"@idn-1"},
			"tags":            map[string]any{},
		},
		update: func(mg *v1alpha1.EdgeRouterPolicy) {
			mg.Spec.ForProvider.IdentityRoles = []string{"#all"}
			mg.Spec.ForProvider.Tags = map[string]string{"scope": "all"}
		},
		updated: map[string]any{
			"name":            "all-routers",
			"semantic":        "AllOf",
			"edgeRouterRoles": []any{"#all"},
			"identityRoles":   []any{"#all"},
			"tags":            map[string]any{"scope": "all"},
		},
	}.run(t)
}

// identityBody is the Ziti identity expected for the supplied fields: what
// the spec leaves out is sent empty, so that it is cleared.
func identityBody(fields map[string]any) map[string]any {
	body := map[string]any{
		"type":                      "Default",
		"isAdmin":                   false,
		"roleAttributes":            []any{},
		"serviceHostingCosts":       map[string]any{},
		"serviceHostingPrecedences": map[string]any{},
		"appData":                   map[string]any{},
		"tags":                      map[string]any{},
	}
	maps.Copy(body, fields)
	return body
}

// pendingEnrollment is how the fake controller reports an enrollment that
// has not been completed.
func pendingEnrollment(method, identity string) map[string]any {
	return map[string]any{method: map[string]any{"jwt": "jwt-for-" + identity, "expiresAt": "2030-01-01T00:00:00.000Z"}}
}

func TestIdentity(t *testing.T) {
	lifecycle[*v1alpha1.Identity]{
		kind: identity.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})
			srv.Put("auth-policies", map[string]any{"id": "ap-1", "name": "certificates"})
		},
		mg: &v1alpha1.Identity{
			ObjectMeta: meta1("web-client"),
			Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{
				Name:           "web-client",
				RoleAttributes: []string{"clients"},
			}},
		},
		created: identityBody(map[string]any{
			"name":           "web-client",
			"roleAttributes": []any{"clients"},
			"enrollment":     pendingEnrollment("ott", "web-client"),
		}),
		update: func(mg *v1alpha1.Identity) {
			p := &mg.Spec.ForProvider
			p.RoleAttributes = []string{"clients", "web"}
			p.AuthPolicyID = ptr.To("certificates")
			p.ExternalID = ptr.To("web-client@example.com")
			p.DefaultHostingCost = ptr.To(int32(10))
			p.DefaultHostingPrecedence = ptr.To("required")
			p.ServiceHostingCosts = map[string]int32{"web": 100}
			p.ServiceHostingPrecedences = map[string]string{"svc-1": "failed"}
			p.AppData = map[string]string{"site": "berlin"}
		},
		updated: identityBody(map[string]any{
			"name":                      "web-client",
			"roleAttributes":            []any{"clients", "web"},
			"authPolicyId":              "ap-1",
			"externalId":                "web-client@example.com",
			"defaultHostingCost":        float64(10),
			"defaultHostingPrecedence":  "required",
			"serviceHostingCosts":       map[string]any{"svc-1": float64(100)},
			"serviceHostingPrecedences": map[string]any{"svc-1": "failed"},
			"appData":                   map[string]any{"site": "berlin"},
			"enrollment":                pendingEnrollment("ott", "web-client"),
		}),
		details: managed.ConnectionDetails{identity.ConnectionKeyEnrollmentToken: []byte("jwt-for-web-client")},
	}.run(t)
}

func TestIdentityOutlivesTheServiceItHosts(t *testing.T) {
	newIdentity := func() *v1alpha1.Identity {
		return &v1alpha1.Identity{
			ObjectMeta: meta1("web-server"),
			Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{
				Name:                "web-server",
				ServiceHostingCosts: map[string]int32{"web": 100},
			}},
		}
	}

	// Ziti keeps the hosting settings for a service that is deleted and then
	// refuses to delete the identity, answering that the service is not found.
	cases := map[string]struct {
		reason  string
		kind    generic.Kind[*v1alpha1.Identity]
		wantErr bool
	}{
		"HostingSettingsAreDroppedFirst": {
			reason: "The identity kinds drop the hosting settings before the deletion.",
			kind:   identity.Kind,
		},
		"NotFoundIsNotTakenForDeleted": {
			reason: "An entity that is still there is not deleted, whatever Ziti reports as not found.",
			kind: func() generic.Kind[*v1alpha1.Identity] {
				k := identity.Kind
				k.BeforeDelete = nil
				return k
			}(),
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})

			api, err := srv.Client()
			if err != nil {
				t.Fatalf("cannot create client: %v", err)
			}

			ctx := context.Background()
			e := generic.NewExternalClient(tc.kind, api)
			mg := newIdentity()
			if _, err := e.Create(ctx, mg); err != nil {
				t.Fatalf("Create(...): %v", err)
			}
			srv.Delete("services", "svc-1")

			_, err = e.Delete(ctx, mg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("\n%s\nDelete(...): want error %t, got %v", tc.reason, tc.wantErr, err)
			}
			if gone := srv.Entity("identities", meta.GetExternalName(mg)) == nil; gone == tc.wantErr {
				t.Errorf("\n%s\nidentity gone after Delete(...): want %t, got %t", tc.reason, !tc.wantErr, gone)
			}
		})
	}
}

func TestIdentityCA(t *testing.T) {
	lifecycle[*v1alpha1.IdentityCA]{
		kind: identityca.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("cas", map[string]any{"id": "ca-1", "name": "devices"})
		},
		mg: &v1alpha1.IdentityCA{
			ObjectMeta: meta1("sensor"),
			Spec: v1alpha1.IdentityCASpec{ForProvider: v1alpha1.IdentityCAParameters{
				IdentityParameters: v1alpha1.IdentityParameters{Name: "sensor", Type: "Device"},
				Ottca:              "devices",
			}},
		},
		created: identityBody(map[string]any{
			"name":       "sensor",
			"type":       "Device",
			"enrollment": pendingEnrollment("ottca", "sensor"),
		}),
		update: func(mg *v1alpha1.IdentityCA) {
			mg.Spec.ForProvider.RoleAttributes = []string{"sensors"}
		},
		updated: identityBody(map[string]any{
			"name":           "sensor",
			"type":           "Device",
			"roleAttributes": []any{"sensors"},
			"enrollment":     pendingEnrollment("ottca", "sensor"),
		}),
		details: managed.ConnectionDetails{identity.ConnectionKeyEnrollmentToken: []byte("jwt-for-sensor")},
	}.run(t)
}

func TestIdentityCAWaitsForItsCertificateAuthority(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	mg := &v1alpha1.IdentityCA{
		ObjectMeta: meta1("sensor"),
		Spec: v1alpha1.IdentityCASpec{ForProvider: v1alpha1.IdentityCAParameters{
			IdentityParameters: v1alpha1.IdentityParameters{Name: "sensor"},
			Ottca:              "devices",
		}},
	}

	_, err = generic.NewExternalClient(identityca.Kind, api).Create(context.Background(), mg)
	if err == nil || !strings.Contains(err.Error(), `no entity named "devices" in cas`) {
		t.Errorf("Create(...): want an error about the missing certificate authority, got %v", err)
	}
	if got := srv.Len("identities"); got != 0 {
		t.Errorf("want no identity to be created, got %d", got)
	}
}

func TestIdentityUPDB(t *testing.T) {
	lifecycle[*v1alpha1.IdentityUPDB]{
		kind: identityupdb.Kind,
		mg: &v1alpha1.IdentityUPDB{
			ObjectMeta: meta1("operator"),
			Spec: v1alpha1.IdentityUPDBSpec{ForProvider: v1alpha1.IdentityUPDBParameters{
				IdentityParameters: v1alpha1.IdentityParameters{Name: "operator", Type: "User", IsAdmin: ptr.To(true)},
				UpdbUsername:       "operator",
			}},
		},
		created: identityBody(map[string]any{
			"name":       "operator",
			"type":       "User",
			"isAdmin":    true,
			"enrollment": pendingEnrollment("updb", "operator"),
		}),
		update: func(mg *v1alpha1.IdentityUPDB) {
			mg.Spec.ForProvider.IsAdmin = ptr.To(false)
			mg.Spec.ForProvider.Tags = map[string]string{"team": "ops"}
		},
		updated: identityBody(map[string]any{
			"name":       "operator",
			"type":       "User",
			"tags":       map[string]any{"team": "ops"},
			"enrollment": pendingEnrollment("updb", "operator"),
		}),
		details: managed.ConnectionDetails{identity.ConnectionKeyEnrollmentToken: []byte("jwt-for-operator")},
	}.run(t)
}

func TestIdentityNone(t *testing.T) {
	lifecycle[*v1alpha1.IdentityNone]{
		kind: identitynone.Kind,
		mg: &v1alpha1.IdentityNone{
			ObjectMeta: meta1("sso-user"),
			Spec: v1alpha1.IdentityNoneSpec{ForProvider: v1alpha1.IdentityParameters{
				Name:       "sso-user",
				Type:       "User",
				ExternalID: ptr.To("user@example.com"),
			}},
		},
		created: identityBody(map[string]any{
			"name":       "sso-user",
			"type":       "User",
			"externalId": "user@example.com",
		}),
		update: func(mg *v1alpha1.IdentityNone) {
			mg.Spec.ForProvider.ExternalID = ptr.To("user@example.org")
		},
		updated: identityBody(map[string]any{
			"name":       "sso-user",
			"type":       "User",
			"externalId": "user@example.org",
		}),
	}.run(t)
}

func TestIdentityStatus(t *testing.T) {
	cases := map[string]struct {
		entity string
		want   v1alpha1.IdentityObservation
	}{
		"PendingEnrollment": {
			entity: `{"id": "idn-1", "name": "web-client", "typeId": "Default", "authenticators": {},
				"enrollment": {"ott": {"jwt": "token", "expiresAt": "2030-01-01T00:00:00.000Z"}}}`,
			want: v1alpha1.IdentityObservation{ID: "idn-1", Name: "web-client", TypeID: "Default", EnrollmentExpiresAt: "2030-01-01T00:00:00.000Z"},
		},
		"Enrolled": {
			entity: `{"id": "idn-1", "name": "web-client", "typeId": "Default", "enrollment": {},
				"authenticators": {"cert": {"fingerprint": "abc"}}, "tags": {"managed": true}}`,
			want: v1alpha1.IdentityObservation{ID: "idn-1", Name: "web-client", TypeID: "Default", Enrolled: true, Tags: map[string]string{"managed": "true"}},
		},
		"PendingPasswordEnrollment": {
			entity: `{"id": "idn-1", "name": "web-client", "typeId": "User", "authenticators": {},
				"enrollment": {"updb": {"jwt": "token", "expiresAt": "2030-01-01T00:00:00.000Z"}}}`,
			want: v1alpha1.IdentityObservation{ID: "idn-1", Name: "web-client", TypeID: "User", EnrollmentExpiresAt: "2030-01-01T00:00:00.000Z"},
		},
		"EnrolledWithPassword": {
			entity: `{"id": "idn-1", "name": "web-client", "typeId": "User", "enrollment": {}, "authPolicyId": "default",
				"authenticators": {"updb": {"username": "web-client"}}, "appData": {"seats": 4},
				"defaultHostingCost": 10, "defaultHostingPrecedence": "required", "serviceHostingCosts": {"svc-1": 100}}`,
			want: v1alpha1.IdentityObservation{
				ID: "idn-1", Name: "web-client", TypeID: "User", Enrolled: true, AuthPolicyID: "default",
				AppData: map[string]string{"seats": "4"}, DefaultHostingCost: 10, DefaultHostingPrecedence: "required",
				ServiceHostingCosts: map[string]int32{"svc-1": 100},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()

			e := map[string]any{}
			if err := json.Unmarshal([]byte(tc.entity), &e); err != nil {
				t.Fatalf("cannot parse entity: %v", err)
			}
			srv.Put("identities", e)

			api, err := srv.Client()
			if err != nil {
				t.Fatalf("cannot create client: %v", err)
			}

			// A stale status must not survive an observation.
			mg := &v1alpha1.Identity{ObjectMeta: meta1("web-client")}
			mg.Spec.ForProvider.Name = "web-client"
			mg.Status.AtProvider = v1alpha1.IdentityObservation{RoleAttributes: []string{"stale"}, EnrollmentExpiresAt: "stale"}
			meta.SetExternalName(mg, "idn-1")

			if _, err := generic.NewExternalClient(identity.Kind, api).Observe(context.Background(), mg); err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if diff := cmp.Diff(tc.want, mg.Status.AtProvider); diff != "" {
				t.Errorf("status.atProvider: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestEdgeRouter(t *testing.T) {
	lifecycle[*v1alpha1.EdgeRouter]{
		kind: edgerouter.Kind,
		mg: &v1alpha1.EdgeRouter{
			ObjectMeta: meta1("router-1"),
			Spec: v1alpha1.EdgeRouterSpec{ForProvider: v1alpha1.EdgeRouterParameters{
				Name:              "router-1",
				IsTunnelerEnabled: ptr.To(true),
				RoleAttributes:    []string{"public"},
			}},
		},
		created: map[string]any{
			"name":              "router-1",
			"isTunnelerEnabled": true,
			"roleAttributes":    []any{"public"},
			"tags":              map[string]any{},
			"enrollmentJwt":     "jwt-for-router-1",
		},
		update: func(mg *v1alpha1.EdgeRouter) {
			mg.Spec.ForProvider.Cost = ptr.To(int64(10))
			mg.Spec.ForProvider.NoTraversal = ptr.To(true)
		},
		updated: map[string]any{
			"name":              "router-1",
			"isTunnelerEnabled": true,
			"noTraversal":       true,
			"cost":              float64(10),
			"roleAttributes":    []any{"public"},
			"tags":              map[string]any{},
			"enrollmentJwt":     "jwt-for-router-1",
		},
		details: managed.ConnectionDetails{edgerouter.ConnectionKeyEnrollmentToken: []byte("jwt-for-router-1")},
	}.run(t)
}

func TestPostureCheckMFA(t *testing.T) {
	lifecycle[*v1alpha1.PostureCheckMFA]{
		kind: posturecheckmfa.Kind,
		mg: &v1alpha1.PostureCheckMFA{
			ObjectMeta: meta1("mfa"),
			Spec: v1alpha1.PostureCheckMFASpec{ForProvider: v1alpha1.PostureCheckMFAParameters{
				Name:           "mfa",
				TimeoutSeconds: ptr.To(int64(-1)),
			}},
		},
		created: map[string]any{
			"name": "mfa", "typeId": "MFA", "roleAttributes": []any{}, "timeoutSeconds": float64(-1), "tags": map[string]any{},
		},
		update: func(mg *v1alpha1.PostureCheckMFA) {
			mg.Spec.ForProvider.TimeoutSeconds = ptr.To(int64(300))
			mg.Spec.ForProvider.PromptOnWake = ptr.To(true)
			mg.Spec.ForProvider.RoleAttributes = []string{"strict"}
		},
		updated: map[string]any{
			"name": "mfa", "typeId": "MFA", "roleAttributes": []any{"strict"}, "timeoutSeconds": float64(300),
			"promptOnWake": true, "tags": map[string]any{},
		},
	}.run(t)
}

func TestPostureCheckOS(t *testing.T) {
	lifecycle[*v1alpha1.PostureCheckOS]{
		kind: posturecheckos.Kind,
		mg: &v1alpha1.PostureCheckOS{
			ObjectMeta: meta1("linux"),
			Spec: v1alpha1.PostureCheckOSSpec{ForProvider: v1alpha1.PostureCheckOSParameters{
				Name:             "linux",
				OperatingSystems: []v1alpha1.OperatingSystem{{Type: "Linux"}},
			}},
		},
		created: map[string]any{
			"name":             "linux",
			"typeId":           "OS",
			"roleAttributes":   []any{},
			"operatingSystems": []any{map[string]any{"type": "Linux", "versions": []any{}}},
			"tags":             map[string]any{},
		},
		update: func(mg *v1alpha1.PostureCheckOS) {
			mg.Spec.ForProvider.OperatingSystems = []v1alpha1.OperatingSystem{{Type: "Linux", Versions: []string{">=5.0.0"}}}
		},
		updated: map[string]any{
			"name":             "linux",
			"typeId":           "OS",
			"roleAttributes":   []any{},
			"operatingSystems": []any{map[string]any{"type": "Linux", "versions": []any{">=5.0.0"}}},
			"tags":             map[string]any{},
		},
	}.run(t)
}

func TestAuthPolicy(t *testing.T) {
	// Every method is always sent, with what the spec leaves out not allowed.
	policy := func(cert, updb map[string]any, secondary map[string]any) map[string]any {
		return map[string]any{
			"name": "certs-only",
			"primary": map[string]any{
				"cert":   cert,
				"updb":   updb,
				"extJwt": map[string]any{"allowed": false, "allowedSigners": []any{}},
			},
			"secondary": secondary,
			"tags":      map[string]any{},
		}
	}
	noPasswords := map[string]any{
		"allowed": false, "minPasswordLength": float64(5), "maxAttempts": float64(5), "lockoutDurationMinutes": float64(0),
		"requireMixedCase": false, "requireNumberChar": false, "requireSpecialChar": false,
	}

	lifecycle[*v1alpha1.AuthPolicy]{
		kind: authpolicy.Kind,
		mg: &v1alpha1.AuthPolicy{
			ObjectMeta: meta1("certs-only"),
			Spec: v1alpha1.AuthPolicySpec{ForProvider: v1alpha1.AuthPolicyParameters{
				Name:    "certs-only",
				Primary: &v1alpha1.AuthMethods{Cert: &v1alpha1.CertAuth{Allowed: true}},
			}},
		},
		created: policy(
			map[string]any{"allowed": true, "allowExpiredCerts": false},
			noPasswords,
			map[string]any{"requireTotp": false, "requireExtJwtSigner": nil},
		),
		update: func(mg *v1alpha1.AuthPolicy) {
			mg.Spec.ForProvider.Primary.Cert.AllowExpiredCerts = true
			mg.Spec.ForProvider.Primary.UPDB = &v1alpha1.UPDBAuth{Allowed: true, MinPasswordLength: ptr.To(int64(12)), RequireNumberChar: true}
			mg.Spec.ForProvider.Secondary = &v1alpha1.SecondaryAuth{RequireTOTP: true}
		},
		updated: policy(
			map[string]any{"allowed": true, "allowExpiredCerts": true},
			map[string]any{
				"allowed": true, "minPasswordLength": float64(12), "maxAttempts": float64(5), "lockoutDurationMinutes": float64(0),
				"requireMixedCase": false, "requireNumberChar": true, "requireSpecialChar": false,
			},
			map[string]any{"requireTotp": true, "requireExtJwtSigner": nil},
		),
	}.run(t)
}

func TestExternalDriftIsDetected(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	ctx := context.Background()
	e := generic.NewExternalClient(service.Kind, api)
	mg := &v1alpha1.Service{
		ObjectMeta: meta1("web"),
		Spec:       v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{Name: "web", RoleAttributes: []string{"web"}}},
	}
	if _, err := e.Create(ctx, mg); err != nil {
		t.Fatalf("Create(...): %v", err)
	}

	drifted := srv.Entity("services", meta.GetExternalName(mg))
	drifted["roleAttributes"] = []any{"web", "added-by-hand"}
	srv.Put("services", drifted)

	o, err := e.Observe(ctx, mg)
	if err != nil || o.ResourceUpToDate {
		t.Fatalf("Observe(...): want an outdated resource, got %+v, %v", o, err)
	}
	if want := `field "roleAttributes" differs`; !strings.Contains(o.Diff, want) {
		t.Errorf("Observe(...): want a diff containing %q, got %q", want, o.Diff)
	}
	if diff := cmp.Diff([]string{"web", "added-by-hand"}, mg.Status.AtProvider.RoleAttributes); diff != "" {
		t.Errorf("status.atProvider.roleAttributes: -want, +got:\n%s", diff)
	}
}

func TestDeletionDoesNotResolveReferences(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()
	srv.Put("services", map[string]any{"id": "svc-1", "name": "web", "configs": []any{"cfg-1"}})

	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	// The config the service refers to no longer exists.
	now := metav1.Now()
	mg := &v1alpha1.Service{
		ObjectMeta: meta1("web"),
		Spec:       v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{Name: "web", Configs: []string{"web-host"}}},
	}
	mg.SetDeletionTimestamp(&now)
	meta.SetExternalName(mg, "svc-1")

	o, err := generic.NewExternalClient(service.Kind, api).Observe(context.Background(), mg)
	if err != nil || !o.ResourceExists {
		t.Fatalf("Observe(...) of a deleted resource: want an existing resource, got %+v, %v", o, err)
	}
}
