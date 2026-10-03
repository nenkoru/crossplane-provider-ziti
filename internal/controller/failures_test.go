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
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/configinterceptv1"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouter"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/externaljwtsigner"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/identityca"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/serviceedgerouterpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

// TestMissingReferences gives every reference of the kinds that refer to
// other entities by name one that does not exist. Nothing is created, and
// the error names what is missing, so that the managed resource reports it
// and is retried.
func TestMissingReferences(t *testing.T) {
	seed := func(srv *fake.Server) {
		srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})
		srv.Put("identities", map[string]any{"id": "idn-1", "name": "web-client"})
		srv.Put("edge-routers", map[string]any{"id": "er-1", "name": "router-1"})
		srv.Put("posture-checks", map[string]any{"id": "pc-1", "name": "mfa"})
	}
	dial := func(change func(p *v1alpha1.ServicePolicyParameters)) resource.ModernManaged {
		p := v1alpha1.ServicePolicyParameters{
			Name: "web-dial", Type: v1alpha1.ServicePolicyTypeDial,
			ServiceRoles: []string{"@web"}, IdentityRoles: []string{"@web-client"}, PostureCheckRoles: []string{"@mfa"},
		}
		change(&p)
		return &v1alpha1.ServicePolicy{ObjectMeta: meta1("web-dial"), Spec: v1alpha1.ServicePolicySpec{ForProvider: p}}
	}
	routers := func(change func(p *v1alpha1.EdgeRouterPolicyParameters)) resource.ModernManaged {
		p := v1alpha1.EdgeRouterPolicyParameters{Name: "routers", EdgeRouterRoles: []string{"@router-1"}, IdentityRoles: []string{"@web-client"}}
		change(&p)
		return &v1alpha1.EdgeRouterPolicy{ObjectMeta: meta1("routers"), Spec: v1alpha1.EdgeRouterPolicySpec{ForProvider: p}}
	}
	serviceRouters := func(change func(p *v1alpha1.ServiceEdgeRouterPolicyParameters)) resource.ModernManaged {
		p := v1alpha1.ServiceEdgeRouterPolicyParameters{Name: "service-routers", ServiceRoles: []string{"@web"}, EdgeRouterRoles: []string{"@router-1"}}
		change(&p)
		return &v1alpha1.ServiceEdgeRouterPolicy{ObjectMeta: meta1("service-routers"), Spec: v1alpha1.ServiceEdgeRouterPolicySpec{ForProvider: p}}
	}
	identityWith := func(change func(p *v1alpha1.IdentityParameters)) resource.ModernManaged {
		p := v1alpha1.IdentityParameters{Name: "web-server"}
		change(&p)
		return &v1alpha1.Identity{ObjectMeta: meta1("web-server"), Spec: v1alpha1.IdentitySpec{ForProvider: p}}
	}

	cases := map[string]struct {
		mg         resource.ModernManaged
		create     func(ctx context.Context, api *client.Client, mg resource.ModernManaged) (managed.ExternalObservation, error)
		collection string
		want       string
	}{
		"ServiceRoleOfAServicePolicy": {
			mg:     dial(func(p *v1alpha1.ServicePolicyParameters) { p.ServiceRoles = []string{"@gone"} }),
			create: createAndObserve(servicepolicy.Kind), collection: "service-policies",
			want: `no entity named "gone" in services`,
		},
		"IdentityRoleOfAServicePolicy": {
			mg:     dial(func(p *v1alpha1.ServicePolicyParameters) { p.IdentityRoles = []string{"#all", "@gone"} }),
			create: createAndObserve(servicepolicy.Kind), collection: "service-policies",
			want: `no entity named "gone" in identities`,
		},
		"PostureCheckRoleOfAServicePolicy": {
			mg:     dial(func(p *v1alpha1.ServicePolicyParameters) { p.PostureCheckRoles = []string{"@gone"} }),
			create: createAndObserve(servicepolicy.Kind), collection: "service-policies",
			want: `no entity named "gone" in posture-checks`,
		},
		"EdgeRouterRoleOfAnEdgeRouterPolicy": {
			mg:     routers(func(p *v1alpha1.EdgeRouterPolicyParameters) { p.EdgeRouterRoles = []string{"@gone"} }),
			create: createAndObserve(edgerouterpolicy.Kind), collection: "edge-router-policies",
			want: `no entity named "gone" in edge-routers`,
		},
		"IdentityRoleOfAnEdgeRouterPolicy": {
			mg:     routers(func(p *v1alpha1.EdgeRouterPolicyParameters) { p.IdentityRoles = []string{"@gone"} }),
			create: createAndObserve(edgerouterpolicy.Kind), collection: "edge-router-policies",
			want: `no entity named "gone" in identities`,
		},
		"ServiceRoleOfAServiceEdgeRouterPolicy": {
			mg:     serviceRouters(func(p *v1alpha1.ServiceEdgeRouterPolicyParameters) { p.ServiceRoles = []string{"@gone"} }),
			create: createAndObserve(serviceedgerouterpolicy.Kind), collection: "service-edge-router-policies",
			want: `no entity named "gone" in services`,
		},
		"EdgeRouterRoleOfAServiceEdgeRouterPolicy": {
			mg:     serviceRouters(func(p *v1alpha1.ServiceEdgeRouterPolicyParameters) { p.EdgeRouterRoles = []string{"@gone"} }),
			create: createAndObserve(serviceedgerouterpolicy.Kind), collection: "service-edge-router-policies",
			want: `no entity named "gone" in edge-routers`,
		},
		"HostingCostOfAnIdentity": {
			mg:     identityWith(func(p *v1alpha1.IdentityParameters) { p.ServiceHostingCosts = map[string]int32{"gone": 1} }),
			create: createAndObserve(identity.Kind), collection: "identities",
			want: `no entity named "gone" in services`,
		},
		"HostingPrecedenceOfAnIdentity": {
			mg: identityWith(func(p *v1alpha1.IdentityParameters) {
				p.ServiceHostingPrecedences = map[string]string{"gone": "failed"}
			}),
			create: createAndObserve(identity.Kind), collection: "identities",
			want: `no entity named "gone" in services`,
		},
		"AuthPolicyOfAnIdentity": {
			mg:     identityWith(func(p *v1alpha1.IdentityParameters) { p.AuthPolicyID = ptr.To("gone") }),
			create: createAndObserve(identity.Kind), collection: "identities",
			want: `no entity named "gone" in auth-policies`,
		},
		"EnrollmentAuthPolicyOfASigner": {
			mg: &v1alpha1.ExternalJWTSigner{ObjectMeta: meta1("sso"), Spec: v1alpha1.ExternalJWTSignerSpec{ForProvider: v1alpha1.ExternalJWTSignerParameters{
				Name: "sso", Issuer: "https://sso.example.com", Audience: "ziti", CertPem: ptr.To(certificate), EnrollAuthPolicyID: ptr.To("gone"),
			}}},
			create: createAndObserve(externaljwtsigner.Kind), collection: "external-jwt-signers",
			want: `no entity named "gone" in auth-policies`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			seed(srv)

			_, err := tc.create(context.Background(), zitiClient(t, srv), tc.mg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Create(...): want an error containing %q, got %v", tc.want, err)
			}
			if got := named(srv, tc.collection, tc.mg); len(got) != 0 {
				t.Errorf("want nothing created in %s, got %v", tc.collection, got)
			}
			if _, ok := tc.mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed]; ok {
				t.Errorf("want no creation on record that was never sent, got annotations %v", tc.mg.GetAnnotations())
			}
		})
	}
}

// TestReferenceGoneAfterCreation deletes the service a policy refers to by
// name for good. The policy reports the missing service, is not reported
// ready by mistake, and its entity is left as it is.
func TestReferenceGoneAfterCreation(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()
	srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})

	h := newHarness(t, srv, servicepolicy.Kind, &v1alpha1.ServicePolicy{
		ObjectMeta: meta1("web-dial"),
		Spec: v1alpha1.ServicePolicySpec{ForProvider: v1alpha1.ServicePolicyParameters{
			Name: "web-dial", Type: v1alpha1.ServicePolicyTypeDial, ServiceRoles: []string{"@web"}, IdentityRoles: []string{"#all"},
		}},
	})
	id := meta.GetExternalName(h.converge(6))
	srv.Delete("services", "svc-1")
	before := writes(srv)
	for range 2 {
		h.reconcile()
	}

	mg := h.get()
	if c := mg.GetCondition(xpv2.TypeSynced); c.Status != corev1.ConditionFalse || !strings.Contains(c.Message, `no entity named "web" in services`) {
		t.Errorf("Synced condition: want the missing service reported, got %+v", c)
	}
	if n := writes(srv) - before; n != 0 {
		t.Errorf("want no request that changes something, got %d", n)
	}
	if srv.Entity("service-policies", id) == nil {
		t.Errorf("want the policy to stay in Ziti")
	}
}

// TestExternalClientFailures covers what the generic external client does
// when a step fails before or after it talks to Ziti.
func TestExternalClientFailures(t *testing.T) {
	errBoom := errors.New("boom")
	existing := func() *v1alpha1.Service {
		mg := newService()
		meta.SetExternalName(mg, "svc-1")
		return mg
	}

	t.Run("StatusCannotBeRead", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		// Ziti reports something the status has no room for.
		srv.Put("services", map[string]any{"id": "svc-1", "name": "web", "roleAttributes": "web"})

		mg := existing()
		o, err := generic.NewExternalClient(service.Kind, zitiClient(t, srv)).Observe(context.Background(), mg)
		if err == nil || !strings.Contains(err.Error(), "cannot observe the external resource") {
			t.Errorf("Observe(...): want an error, got %+v, %v", o, err)
		}
		if got := mg.GetCondition(xpv2.TypeReady).Status; got == corev1.ConditionTrue {
			t.Errorf("Ready condition: want it not to be true")
		}
	})

	t.Run("ConnectionDetailsCannotBeRead", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		srv.Put("edge-routers", map[string]any{"id": "er-1", "name": "router-1", "enrollmentJwt": 7})

		mg := &v1alpha1.EdgeRouter{ObjectMeta: meta1("router-1"), Spec: v1alpha1.EdgeRouterSpec{ForProvider: v1alpha1.EdgeRouterParameters{Name: "router-1"}}}
		meta.SetExternalName(mg, "er-1")
		if _, err := generic.NewExternalClient(edgerouter.Kind, zitiClient(t, srv)).Observe(context.Background(), mg); err == nil {
			t.Errorf("Observe(...): want an error, got none")
		}
	})

	t.Run("ExpiryCannotBeRead", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		srv.Put("edge-routers", map[string]any{
			"id": "er-1", "name": "router-1", "isVerified": false, "enrollmentJwt": "jwt", "enrollmentExpiresAt": "tomorrow",
		})

		mg := &v1alpha1.EdgeRouter{ObjectMeta: meta1("router-1"), Spec: v1alpha1.EdgeRouterSpec{ForProvider: v1alpha1.EdgeRouterParameters{Name: "router-1"}}}
		meta.SetExternalName(mg, "er-1")
		o, err := generic.NewExternalClient(edgerouter.Kind, zitiClient(t, srv)).Observe(context.Background(), mg)
		if err == nil {
			t.Errorf("Observe(...): want an error, got %+v", o)
		}
	})

	t.Run("DesiredStateFails", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})
		kind := service.Kind
		kind.Desired = func(context.Context, *client.Client, *v1alpha1.Service) (map[string]any, error) { return nil, errBoom }
		e := generic.NewExternalClient(kind, zitiClient(t, srv))
		ctx := context.Background()

		mg := existing()
		if _, err := e.Observe(ctx, mg); !errors.Is(err, errBoom) {
			t.Errorf("Observe(...): want %v, got %v", errBoom, err)
		}
		if got := mg.GetCondition(xpv2.TypeReady).Status; got == corev1.ConditionTrue {
			t.Errorf("Ready condition: want it not to be true when the desired state is unknown")
		}
		if _, err := e.Update(ctx, mg); !errors.Is(err, errBoom) {
			t.Errorf("Update(...): want %v, got %v", errBoom, err)
		}
		if _, err := e.Create(ctx, newService()); !errors.Is(err, errBoom) {
			t.Errorf("Create(...): want %v, got %v", errBoom, err)
		}
		if n := writes(srv); n != 0 {
			t.Errorf("want no request that changes something, got %v", srv.Requests())
		}
	})

	t.Run("CreateOnlyFieldsFail", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		// The config type of intercept configs is gone.
		srv.Delete("config-types", "intercept-v1-id")

		mg := &v1alpha1.ConfigInterceptV1{ObjectMeta: meta1("web-intercept"), Spec: v1alpha1.ConfigInterceptV1Spec{ForProvider: v1alpha1.ConfigInterceptV1Parameters{
			Name: "web-intercept", Addresses: []string{"web.ziti"},
		}}}
		_, err := createAndObserve(configinterceptv1.Kind)(context.Background(), zitiClient(t, srv), mg)
		if err == nil || !strings.Contains(err.Error(), `no entity named "intercept.v1" in config-types`) {
			t.Errorf("Create(...): want an error about the config type, got %v", err)
		}
		if got := srv.Len("configs"); got != 0 {
			t.Errorf("want no config created, got %d", got)
		}
	})

	t.Run("RepairCannotReadTheEntity", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		e := generic.NewExternalClient(identity.Kind, zitiClient(t, srv))
		ctx := context.Background()
		mg := &v1alpha1.Identity{ObjectMeta: meta1("web-client"), Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{Name: "web-client"}}}
		if _, err := e.Create(ctx, mg); err != nil {
			t.Fatalf("Create(...): %v", err)
		}

		// The entity is read again after the PATCH, to see what needs a
		// repair; Ziti fails that.
		srv.Inject(fake.Fault{Method: http.MethodGet, Collection: "identities", Status: http.StatusServiceUnavailable})
		if _, err := e.Update(ctx, mg); err == nil || !strings.Contains(err.Error(), "cannot repair the external resource") {
			t.Errorf("Update(...): want an error, got %v", err)
		}
	})

	t.Run("PreparationForTheDeletionFails", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		srv.Put("identities", map[string]any{"id": "idn-1", "name": "web-client"})
		mg := &v1alpha1.Identity{ObjectMeta: meta1("web-client"), Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{Name: "web-client"}}}
		meta.SetExternalName(mg, "idn-1")

		srv.Inject(fake.Fault{Method: http.MethodPatch, Status: http.StatusInternalServerError})
		if _, err := generic.NewExternalClient(identity.Kind, zitiClient(t, srv)).Delete(context.Background(), mg); err == nil {
			t.Errorf("Delete(...): want an error, got none")
		}
		if srv.Entity("identities", "idn-1") == nil {
			t.Errorf("want the identity to stay")
		}
	})
}

// TestRenewalFailures covers renewals that cannot go ahead.
func TestRenewalFailures(t *testing.T) {
	expired := func(srv *fake.Server, id, method string) {
		tamper(srv, "identities", id, func(e map[string]any) {
			e["authenticators"] = map[string]any{}
			e["enrollment"] = map[string]any{}
			if method != "" {
				e["enrollment"] = map[string]any{method: map[string]any{
					"id": "enr-1", "jwt": staleToken, "expiresAt": stamp(time.Now().Add(-time.Hour)),
				}}
			}
		})
	}

	t.Run("CertificateAuthorityGone", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		srv.Put("cas", map[string]any{"id": "ca-1", "name": "devices"})
		e := generic.NewExternalClient(identityca.Kind, zitiClient(t, srv))
		ctx := context.Background()
		mg := &v1alpha1.IdentityCA{ObjectMeta: meta1("sensor"), Spec: v1alpha1.IdentityCASpec{ForProvider: v1alpha1.IdentityCAParameters{
			IdentityParameters: v1alpha1.IdentityParameters{Name: "sensor"}, Ottca: "devices",
		}}}
		if _, err := e.Create(ctx, mg); err != nil {
			t.Fatalf("Create(...): %v", err)
		}

		// Ziti deletes the enrollments of a certificate authority with it.
		srv.Delete("cas", "ca-1")
		expired(srv, meta.GetExternalName(mg), "")
		if o, err := e.Observe(ctx, mg); err != nil || o.ResourceUpToDate {
			t.Fatalf("Observe(...): want an outdated resource, got %+v, %v", o, err)
		}
		if _, err := e.Update(ctx, mg); err == nil || !strings.Contains(err.Error(), `no entity named "devices" in cas`) {
			t.Errorf("Update(...): want an error about the certificate authority, got %v", err)
		}
		if got := renewals(srv); len(got) != 0 {
			t.Errorf("want no new enrollment, got %v", got)
		}
	})

	// Whether an identity can sign in with an external JWT takes its auth
	// policy and the signers; if Ziti does not tell, nothing is renewed.
	for name, fault := range map[string]fake.Fault{
		"AuthPolicyUnknown":        {Method: http.MethodGet, Collection: "auth-policies", Status: http.StatusServiceUnavailable},
		"SignersUnknown":           {Method: http.MethodGet, Collection: "external-jwt-signers", Status: http.StatusServiceUnavailable},
		"SignerOfThePolicyUnknown": {Method: http.MethodGet, Collection: "external-jwt-signers", Status: http.StatusBadGateway},
	} {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			var signers []string
			if name == "SignerOfThePolicyUnknown" {
				signers = []string{"sig-1"}
			}
			srv.Put("auth-policies", map[string]any{"id": "default", "name": "Default", "primary": map[string]any{
				"extJwt": map[string]any{"allowed": true, "allowedSigners": signers},
			}})
			e := generic.NewExternalClient(identity.Kind, zitiClient(t, srv))
			ctx := context.Background()
			mg := &v1alpha1.Identity{ObjectMeta: meta1("web-client"), Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{Name: "web-client"}}}
			if _, err := e.Create(ctx, mg); err != nil {
				t.Fatalf("Create(...): %v", err)
			}
			expired(srv, meta.GetExternalName(mg), "ott")

			srv.Inject(fault)
			if o, err := e.Observe(ctx, mg); err == nil {
				t.Errorf("Observe(...): want an error, got %+v", o)
			}
			if got := mg.GetCondition(xpv2.TypeReady).Status; got == corev1.ConditionTrue {
				t.Errorf("Ready condition: want it not to be true")
			}
			if got := renewals(srv); len(got) != 0 {
				t.Errorf("want no new enrollment, got %v", got)
			}
		})
	}

	t.Run("SignerCannotBeRead", func(t *testing.T) {
		srv := fake.NewServer()
		defer srv.Close()
		srv.Put("auth-policies", map[string]any{"id": "default", "name": "Default", "primary": map[string]any{
			"extJwt": map[string]any{"allowed": true},
		}})
		srv.Put("external-jwt-signers", map[string]any{"id": "sig-1", "name": "sso", "enabled": "yes"})
		e := generic.NewExternalClient(identity.Kind, zitiClient(t, srv))
		ctx := context.Background()
		mg := &v1alpha1.Identity{ObjectMeta: meta1("web-client"), Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{Name: "web-client"}}}
		if _, err := e.Create(ctx, mg); err != nil {
			t.Fatalf("Create(...): %v", err)
		}
		expired(srv, meta.GetExternalName(mg), "ott")
		var syntax *json.UnmarshalTypeError
		if _, err := e.Observe(ctx, mg); !errors.As(err, &syntax) {
			t.Errorf("Observe(...): want an error about the signer, got %v", err)
		}
	})
}

// TestCreationRecovererFailures covers a recovery that cannot find out
// whether the entity was created: nothing is decided and nothing recorded,
// and the managed resource is tried again.
func TestCreationRecovererFailures(t *testing.T) {
	unconfirmed := func(name string) *v1alpha1.Service {
		mg := &v1alpha1.Service{ObjectMeta: meta1("web"), Spec: v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{Name: name}}}
		meta.AddAnnotations(mg, map[string]string{generic.AnnotationKeyCreateUnconfirmed: time.Now().UTC().Format(time.RFC3339)})
		return mg
	}

	cases := map[string]struct {
		mg      *v1alpha1.Service
		connect func() (*client.Client, error)
		fault   *fake.Fault
		entity  map[string]any
		wantErr bool
	}{
		"ZitiFails": {
			mg:      unconfirmed("web"),
			fault:   &fake.Fault{Method: http.MethodGet, Collection: "services", Status: http.StatusServiceUnavailable},
			wantErr: true,
		},
		"CannotConnect": {
			mg:      unconfirmed("web"),
			connect: func() (*client.Client, error) { return nil, errors.New("no credentials") },
			wantErr: true,
		},
		"EntityCannotBeRead": {
			mg:      unconfirmed("web"),
			entity:  map[string]any{"id": "svc-1", "name": "web", "createdAt": "yesterday"},
			wantErr: true,
		},
		"NoName": {
			mg: unconfirmed(""),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			if tc.fault != nil {
				srv.Inject(*tc.fault)
			}
			if tc.entity != nil {
				srv.Put("services", tc.entity)
			}
			connect := tc.connect
			if connect == nil {
				connect = srv.Client
			}

			updates := 0
			err := generic.NewCreationRecoverer(counter(&updates), connecterFn(connect), "services").Initialize(context.Background(), tc.mg)
			if (err != nil) != tc.wantErr {
				t.Errorf("Initialize(...): want error %t, got %v", tc.wantErr, err)
			}
			if updates != 0 {
				t.Errorf("want the managed resource not to be updated, got %d updates", updates)
			}
			if tc.mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed] == "" || meta.GetExternalName(tc.mg) != "" {
				t.Errorf("want nothing decided, got annotations %v", tc.mg.GetAnnotations())
			}
		})
	}
}
