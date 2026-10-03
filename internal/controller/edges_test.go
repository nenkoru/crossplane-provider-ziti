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
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/confighostv1"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

func newService() *v1alpha1.Service {
	return &v1alpha1.Service{
		ObjectMeta: meta1("web"),
		Spec: v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{
			Name: "web", RoleAttributes: []string{"web"}, Tags: map[string]string{"env": "prod"},
		}},
	}
}

// wantService checks that the service the managed resource points at is the
// only one named web and is as declared: as newService declares it, with the
// supplied role attributes, if any.
func wantService(t *testing.T, srv *fake.Server, mg *v1alpha1.Service, roleAttributes ...any) {
	t.Helper()

	id := meta.GetExternalName(mg)
	if got := named(srv, "services", mg); len(got) != 1 || got[0] != id {
		t.Fatalf("services named web: want only %q, got %v", id, got)
	}
	if len(roleAttributes) == 0 {
		roleAttributes = []any{"web"}
	}
	e := srv.Entity("services", id)
	if diff := cmp.Diff(roleAttributes, e["roleAttributes"]); diff != "" {
		t.Errorf("role attributes of the service: -want, +got:\n%s", diff)
	}
	if diff := cmp.Diff(map[string]any{"env": "prod"}, e["tags"]); diff != "" {
		t.Errorf("tags of the service: -want, +got:\n%s", diff)
	}
}

// synced returns the Synced condition of the managed resource.
func synced(mg *v1alpha1.Service) xpv2.Condition {
	return mg.GetCondition(xpv2.TypeSynced)
}

func TestExternalNameOfAMissingEntity(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	// The ID was recorded, but Ziti has no such entity, for instance after
	// the controller was restored from an older backup.
	mg := newService()
	meta.SetExternalName(mg, "gone")
	h := newHarness(t, srv, service.Kind, mg)

	mg = h.converge(6)
	if got := meta.GetExternalName(mg); got == "gone" {
		t.Fatalf("external name: want the ID of a new service, got %q", got)
	}
	wantService(t, srv, mg)
}

func TestEntityRecreatedUnderTheSameName(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	h := newHarness(t, srv, service.Kind, newService())
	id := meta.GetExternalName(h.converge(6))

	// Someone deletes the service and creates another one with its name,
	// as it would be created by hand: it is not the entity of the managed
	// resource, and is never taken over without being asked to.
	srv.Delete("services", id)
	foreign := map[string]any{
		"id": "foreign", "name": "web", "encryptionRequired": false, "roleAttributes": []any{"theirs"},
		"createdAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	srv.Put("services", foreign)

	for range 3 {
		h.reconcile()
	}
	mg := h.get()
	if diff := cmp.Diff(foreign, srv.Entity("services", "foreign")); diff != "" {
		t.Errorf("service of someone else: want it untouched, -want, +got:\n%s", diff)
	}
	if got := srv.Len("services"); got != 1 {
		t.Errorf("want one service in Ziti, got %d", got)
	}
	if c := synced(mg); c.Status != corev1.ConditionFalse || !strings.Contains(c.Message, "COULD_NOT_VALIDATE") {
		t.Errorf("Synced condition: want the name conflict Ziti reports, got %+v", c)
	}
	if got := mg.GetCondition(xpv2.TypeReady).Status; got == corev1.ConditionTrue {
		t.Errorf("Ready condition: want it not to be true while the managed resource has no entity")
	}
	if _, ok := mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed]; ok {
		t.Errorf("want no unconfirmed creation on record after Ziti refused it, got annotations %v", mg.GetAnnotations())
	}

	// Setting the external name adopts the entity, which then follows the
	// spec.
	h.update(func(mg *v1alpha1.Service) { meta.SetExternalName(mg, "foreign") })
	mg = h.converge(6)
	if got := meta.GetExternalName(mg); got != "foreign" {
		t.Errorf("external name after the adoption: want %q, got %q", "foreign", got)
	}
	wantService(t, srv, mg)
}

func TestNameConflictOnCreation(t *testing.T) {
	cases := map[string]struct {
		reason string
		// fault answers the creation, if Ziti is not to answer itself.
		fault *fake.Fault
	}{
		"NameTaken": {
			reason: "Ziti answers a name that is taken with a validation error.",
		},
		"Conflict": {
			reason: "A conflict is a refusal as well: nothing was created.",
			fault:  &fake.Fault{Method: http.MethodPost, Status: http.StatusConflict, Code: "CONFLICT"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			foreign := map[string]any{"id": "foreign", "name": "web", "createdAt": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)}
			if tc.fault == nil {
				srv.Put("services", foreign)
			} else {
				srv.Inject(*tc.fault)
			}

			h := newHarness(t, srv, service.Kind, newService())
			h.reconcile()
			mg := h.get()
			if c := synced(mg); c.Status != corev1.ConditionFalse {
				t.Errorf("\n%s\nSynced condition: want the refusal reported, got %+v", tc.reason, c)
			}
			if got := meta.GetExternalName(mg); got != "" {
				t.Errorf("\n%s\nexternal name: want none, got %q", tc.reason, got)
			}
			if _, ok := mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed]; ok {
				t.Errorf("\n%s\nwant no unconfirmed creation on record, got annotations %v", tc.reason, mg.GetAnnotations())
			}
			if tc.fault == nil {
				if diff := cmp.Diff(foreign, srv.Entity("services", "foreign")); diff != "" {
					t.Errorf("\n%s\nservice of someone else: -want, +got:\n%s", tc.reason, diff)
				}
				return
			}
			if got := srv.Len("services"); got != 0 {
				t.Errorf("\n%s\nwant no service in Ziti, got %d", tc.reason, got)
			}
			wantService(t, srv, h.converge(6))
		})
	}
}

func TestDeletionOfAnEntityThatIsGone(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()
	srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})

	idn := &v1alpha1.Identity{
		ObjectMeta: meta1("web-server"),
		Spec:       v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{Name: "web-server", ServiceHostingCosts: map[string]int32{"web": 10}}},
	}
	h := newHarness(t, srv, identity.Kind, idn)
	id := meta.GetExternalName(h.converge(6))

	srv.Delete("identities", id)
	h.remove()
	for range 3 {
		h.reconcile()
	}
	if h.exists() {
		t.Errorf("want the managed resource of an entity that is gone to be deleted")
	}

	// Deleting an entity that is gone succeeds, even for a kind that
	// changes the entity first.
	api := zitiClient(t, srv)
	e := generic.NewExternalClient(identity.Kind, api)
	if _, err := e.Delete(context.Background(), idn); err != nil {
		t.Errorf("Delete(...) of an entity that is gone: %v", err)
	}
}

func TestDeletionRefused(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	h := newHarness(t, srv, service.Kind, newService())
	id := meta.GetExternalName(h.converge(6))

	// Ziti refuses to delete an entity that another one refers to.
	srv.Inject(fake.Fault{Method: http.MethodDelete, Status: http.StatusConflict, Code: "CAN_NOT_DELETE_REFERENCED_ENTITY"})
	h.remove()
	h.reconcile()
	if !h.exists() {
		t.Fatalf("want the managed resource to stay while Ziti refuses to delete its entity")
	}
	if c := synced(h.get()); c.Status != corev1.ConditionFalse || !strings.Contains(c.Message, "CAN_NOT_DELETE_REFERENCED_ENTITY") {
		t.Errorf("Synced condition: want the refusal reported, got %+v", c)
	}
	if srv.Entity("services", id) == nil {
		t.Fatalf("want the entity to stay")
	}

	// Once the reference is gone, the deletion goes through.
	for range 3 {
		h.reconcile()
	}
	if h.exists() || srv.Entity("services", id) != nil {
		t.Errorf("want the managed resource and its entity to be gone")
	}
}

// TestTransientErrors makes Ziti, or a proxy in front of it, fail one
// request of each step of a reconcile, and checks that the managed resource
// reports the error, that no entity is lost or duplicated, and that the next
// reconciles put everything right.
func TestTransientErrors(t *testing.T) {
	type failure struct {
		status int
		// timeout lets the request run into the deadline of the reconcile.
		timeout bool
	}
	failures := map[string]failure{
		"500":     {status: http.StatusInternalServerError},
		"502":     {status: http.StatusBadGateway},
		"503":     {status: http.StatusServiceUnavailable},
		"504":     {status: http.StatusGatewayTimeout},
		"408":     {status: http.StatusRequestTimeout},
		"429":     {status: http.StatusTooManyRequests},
		"Closed":  {},
		"Timeout": {timeout: true},
	}

	steps := map[string]struct {
		method string
		// prepare brings the managed resource to the step.
		prepare func(h *harness[*v1alpha1.Service])
		deleted bool
	}{
		"Observe": {
			method:  http.MethodGet,
			prepare: func(h *harness[*v1alpha1.Service]) { h.converge(6) },
		},
		"Create": {
			method:  http.MethodPost,
			prepare: func(*harness[*v1alpha1.Service]) {},
		},
		"Update": {
			// Services are replaced.
			method: http.MethodPut,
			prepare: func(h *harness[*v1alpha1.Service]) {
				h.converge(6)
				h.update(func(mg *v1alpha1.Service) { mg.Spec.ForProvider.RoleAttributes = []string{"web", "updated"} })
			},
		},
		"Delete": {
			method: http.MethodDelete,
			prepare: func(h *harness[*v1alpha1.Service]) {
				h.converge(6)
				h.remove()
			},
			deleted: true,
		},
	}

	for step, s := range steps {
		for name, f := range failures {
			for _, carriedOut := range []bool{false, true} {
				if s.method == http.MethodGet && carriedOut {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/CarriedOut=%t", step, name, carriedOut), func(t *testing.T) {
					srv := fake.NewServer()
					defer srv.Close()

					var opts []managed.ReconcilerOption
					fault := fake.Fault{Method: s.method, Collection: "services", Status: f.status, CarriedOut: carriedOut}
					if f.timeout {
						opts = append(opts, managed.WithTimeout(200*time.Millisecond))
						fault.Status, fault.Delay = http.StatusGatewayTimeout, time.Minute
					}
					h := newHarness(t, srv, service.Kind, newService(), opts...)
					s.prepare(h)

					srv.Inject(fault)
					h.reconcile()
					if left := srv.ClearFaults(); len(left) != 0 {
						t.Fatalf("the reconcile did not send the request that fails")
					}
					if got := srv.Len("services"); got > 1 {
						t.Errorf("want at most one service in Ziti, got %d", got)
					}
					if h.exists() {
						mg := h.get()
						// The HTTP client sends a GET again by itself on a
						// connection that was closed without an answer.
						retried := s.method == http.MethodGet && f.status == 0 && !f.timeout
						if c := synced(mg); c.Status != corev1.ConditionFalse && !retried {
							t.Errorf("Synced condition after a failed request: want False, got %+v", c)
						}
						if s.method == http.MethodPost && mg.GetCondition(xpv2.TypeReady).Status == corev1.ConditionTrue {
							t.Errorf("Ready condition after a failed creation: want it not to be true")
						}
						unconfirmed := mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed] != ""
						if s.method == http.MethodPost && !unconfirmed {
							t.Errorf("want a creation without an answer put on record, got annotations %v", mg.GetAnnotations())
						}
					}

					if s.deleted {
						for range 3 {
							h.reconcile()
						}
						if h.exists() || srv.Len("services") != 0 {
							t.Errorf("want the managed resource and its entity gone, got %d services", srv.Len("services"))
						}
						return
					}
					mg := h.converge(8)
					if step == "Update" {
						wantService(t, srv, mg, "updated", "web")
					} else {
						wantService(t, srv, mg)
					}
					if _, ok := mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed]; ok {
						t.Errorf("want the unconfirmed creation settled, got annotations %v", mg.GetAnnotations())
					}
				})
			}
		}
	}
}

// TestReferencesFollowARecreatedEntity deletes a config and a service in Ziti
// that a service and a policy refer to by name. Ziti drops the references to
// a deleted entity; once the entities are created anew, under new IDs, the
// references are restored with the new IDs.
func TestReferencesFollowARecreatedEntity(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	host := newHarness(t, srv, confighostv1.Kind, &v1alpha1.ConfigHostV1{
		ObjectMeta: meta1("web-host"),
		Spec: v1alpha1.ConfigHostV1Spec{ForProvider: v1alpha1.ConfigHostV1Parameters{
			Name: "web-host", HostTerminator: v1alpha1.HostTerminator{Address: ptr.To("localhost"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp")},
		}},
	})
	svc := newService()
	svc.Spec.ForProvider.Configs = []string{"web-host"}
	web := newHarness(t, srv, service.Kind, svc)
	dial := newHarness(t, srv, servicepolicy.Kind, &v1alpha1.ServicePolicy{
		ObjectMeta: meta1("web-dial"),
		Spec: v1alpha1.ServicePolicySpec{ForProvider: v1alpha1.ServicePolicyParameters{
			Name: "web-dial", Type: v1alpha1.ServicePolicyTypeDial, ServiceRoles: []string{"@web"}, IdentityRoles: []string{"#all"},
		}},
	})
	hostID := meta.GetExternalName(host.converge(6))
	serviceID := meta.GetExternalName(web.converge(6))
	policyID := meta.GetExternalName(dial.converge(6))

	srv.Delete("configs", hostID)
	srv.Delete("services", serviceID)
	if got := srv.Entity("service-policies", policyID)["serviceRoles"]; len(got.([]any)) != 0 { //nolint:forcetypeassert // The fake controller stores lists as []any.
		t.Fatalf("want Ziti to drop the role of the deleted service, got %v", got)
	}

	// The policy cannot be put right before the service exists again.
	dial.reconcile()
	if c := dial.get().GetCondition(xpv2.TypeSynced); c.Status != corev1.ConditionFalse || !strings.Contains(c.Message, `no entity named "web"`) {
		t.Errorf("Synced condition of the policy: want the missing service reported, got %+v", c)
	}

	hostID = meta.GetExternalName(host.converge(6))
	serviceID = meta.GetExternalName(web.converge(6))
	dial.converge(6)
	if diff := cmp.Diff([]any{hostID}, srv.Entity("services", serviceID)["configs"]); diff != "" {
		t.Errorf("configs of the service created anew: -want, +got:\n%s", diff)
	}
	if diff := cmp.Diff([]any{"@" + serviceID}, srv.Entity("service-policies", policyID)["serviceRoles"]); diff != "" {
		t.Errorf("service roles of the policy: -want, +got:\n%s", diff)
	}
}
