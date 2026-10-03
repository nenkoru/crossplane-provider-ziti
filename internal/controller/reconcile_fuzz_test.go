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
	"fmt"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/service"
)

// A model is what the model-based fuzz test of the reconciler knows about a
// kind: a managed resource, changes of its spec, changes of its entity in
// Ziti, and how its entity reports an enrollment token.
type model[T resource.ModernManaged] struct {
	kind generic.Kind[T]
	seed func(srv *fake.Server)
	mg   func() T

	// update is the method the kind updates its entities with.
	update string

	specChanges []func(mg T)
	tampers     []func(e map[string]any)

	// byHand are the fields of an entity created by hand under the name
	// of the managed resource, besides its ID, name and creation time.
	byHand map[string]any

	// secret is the name of the connection secret, if the kind has one, and
	// token returns the enrollment token Ziti reports for the entity.
	secret string
	token  func(e map[string]any) string
}

// The steps of the model-based fuzz test, between which the reconciler may
// run.
const (
	stepReconcile = iota
	stepDeleteInZiti
	stepChangeInZiti
	stepRecreateInZiti
	stepChangeSpec
	stepFault
	steps
)

// faultStatuses are the answers a fault gives; zero closes the connection.
var faultStatuses = []int{
	http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout,
	http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusConflict, 0,
}

// run plays the steps the input describes and checks the invariants of the
// reconciler after each reconcile and at the end:
//
//   - Ziti never has more than one entity with the name of the managed
//     resource.
//   - An entity created by hand under that name is never changed or deleted
//     by the provider, unless the managed resource adopts it by its
//     external name.
//   - A managed resource that is ready and synced points at an entity that
//     is as declared.
//
// At the end the faults are cleared, an entity created by hand is adopted,
// and the managed resource must settle within a few reconciles: exactly one
// entity, as declared, whose ID is the external name, and whose enrollment
// token is in the connection secret. Deleting the managed resource then
// deletes the entity.
func (m model[T]) run(t *testing.T, srv *fake.Server, data []byte) {
	srv.Reset()
	if m.seed != nil {
		m.seed(srv)
	}
	mg := m.mg()
	name := mg.GetName()
	h := newHarness(t, srv, m.kind, mg)
	g := &gen{data: data}
	var log []string

	// foreign is the entity created by hand under the name, as it was last
	// stored by hand.
	var foreign map[string]any
	foreignID := func() string {
		if foreign == nil {
			return ""
		}
		return foreign["id"].(string) //nolint:forcetypeassert // Set below.
	}
	current := func() string {
		ids := named(srv, m.kind.Collection, mg)
		if len(ids) > 1 {
			t.Fatalf("%d entities named %q in Ziti after %v", len(ids), name, log)
		}
		if len(ids) == 0 {
			return ""
		}
		return ids[0]
	}
	check := func() {
		t.Helper()
		id := current()
		got := h.get()
		adopted := foreign != nil && meta.GetExternalName(got) == foreignID()
		if foreign != nil && !adopted {
			if diff := cmp.Diff(foreign, deepCopy(srv.Entity(m.kind.Collection, foreignID()))); diff != "" {
				t.Fatalf("the provider changed the entity created by hand after %v: -want, +got:\n%s", log, diff)
			}
		}
		if ready(got) {
			if id == "" || id != meta.GetExternalName(got) {
				t.Fatalf("ready and synced, but the entity %q is not the one of the external name %q, after %v", id, meta.GetExternalName(got), log)
			}
			if o := h.observe(got); !o.ResourceUpToDate {
				t.Fatalf("ready and synced, but not up to date (%s) after %v", o.Diff, log)
			}
		}
	}

	for len(g.data) > 0 && len(log) < 32 {
		switch g.intn(steps) {
		case stepReconcile:
			log = append(log, "reconcile")
			h.reconcile()
			check()
		case stepDeleteInZiti:
			if id := current(); id != "" {
				log = append(log, "delete in Ziti")
				srv.Delete(m.kind.Collection, id)
				if id == foreignID() {
					foreign = nil
				}
			}
		case stepChangeInZiti:
			if id := current(); id != "" {
				i := g.intn(len(m.tampers))
				log = append(log, fmt.Sprintf("change %d in Ziti", i))
				tamper(srv, m.kind.Collection, id, m.tampers[i])
				if id == foreignID() {
					foreign = deepCopy(srv.Entity(m.kind.Collection, id))
				}
			}
		case stepRecreateInZiti:
			log = append(log, "create by hand")
			if id := current(); id != "" {
				srv.Delete(m.kind.Collection, id)
			}
			// It was created a while ago, by the clock of the controller,
			// before any creation of the provider that may be in doubt.
			foreign = map[string]any{
				"id": fmt.Sprintf("by-hand-%d", len(log)), "name": name,
				"createdAt": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano),
			}
			maps.Copy(foreign, deepCopy(m.byHand))
			srv.Put(m.kind.Collection, foreign)
			foreign = deepCopy(srv.Entity(m.kind.Collection, foreignID()))
		case stepChangeSpec:
			i := g.intn(len(m.specChanges))
			log = append(log, fmt.Sprintf("spec change %d", i))
			h.update(m.specChanges[i])
		case stepFault:
			f := fake.Fault{
				Method:     []string{http.MethodGet, http.MethodPost, m.update, http.MethodDelete}[g.intn(4)],
				Collection: m.kind.Collection,
				Status:     faultStatuses[g.intn(len(faultStatuses))],
			}
			// Ziti answers a request it refused with a conflict; any other
			// answer may come from a proxy after Ziti carried it out.
			f.CarriedOut = g.bool() && f.Status != http.StatusConflict
			log = append(log, fmt.Sprintf("fault %s %d carried out %t", f.Method, f.Status, f.CarriedOut))
			srv.Inject(f)
		}
	}

	// The faults pass, and whoever created an entity by hand lets the
	// managed resource adopt it.
	srv.ClearFaults()
	if foreign != nil && meta.GetExternalName(h.get()) != foreignID() {
		for range 2 {
			h.reconcile()
			check()
		}
		if ready(h.get()) {
			t.Fatalf("ready and synced while the name is taken by an entity created by hand, after %v", log)
		}
		h.update(func(mg T) { meta.SetExternalName(mg, foreignID()) })
		log = append(log, "adopt")
	}

	got := h.converge(10)
	id := current()
	if id == "" || id != meta.GetExternalName(got) {
		t.Fatalf("after settling: entity %q, external name %q, after %v", id, meta.GetExternalName(got), log)
	}
	if o := h.observe(got); !o.ResourceUpToDate {
		t.Fatalf("after settling: not up to date (%s), after %v", o.Diff, log)
	}
	if m.secret != "" {
		token := m.token(srv.Entity(m.kind.Collection, id))
		published := string(h.secret(m.secret)[identity.ConnectionKeyEnrollmentToken])
		if token != "" && published != token {
			t.Fatalf("after settling: connection secret has the token %q, Ziti reports %q, after %v", published, token, log)
		}
	}

	h.remove()
	for range 4 {
		h.reconcile()
	}
	if h.exists() {
		t.Fatalf("the managed resource is not deleted, after %v: %+v", log, h.get().GetCondition(xpv2.TypeSynced))
	}
	if ids := named(srv, m.kind.Collection, mg); len(ids) != 0 {
		t.Fatalf("entities left after the deletion: %v, after %v", ids, log)
	}
}

// reconcileSeeds are inputs that make the steps of the model-based fuzz
// tests come in different orders.
func reconcileSeeds(f *testing.F) {
	f.Add([]byte{})
	// Reconcile, delete in Ziti, reconcile twice.
	f.Add([]byte{0, 1, 0, 0})
	// Reconcile, change in Ziti, change the spec, reconcile.
	f.Add([]byte{0, 2, 0, 4, 1, 0})
	// Reconcile, create by hand, reconcile twice.
	f.Add([]byte{0, 3, 0, 0})
	// A creation answered with 502 after it was carried out, reconcile
	// three times.
	f.Add([]byte{5, 1, 1, 1, 0, 0, 0})
	// Reconcile, a closed connection on update, change the spec, reconcile,
	// delete in Ziti, a 504 on creation that was not carried out, reconcile
	// twice.
	f.Add([]byte{0, 5, 2, 7, 0, 4, 2, 0, 1, 5, 1, 3, 0, 0, 0})
}

func FuzzReconcileService(f *testing.F) {
	reconcileSeeds(f)
	m := model[*v1alpha1.Service]{
		kind:   service.Kind,
		update: http.MethodPut,
		seed: func(srv *fake.Server) {
			srv.Put("configs", map[string]any{"id": "cfg-1", "name": "web-host"})
		},
		mg: func() *v1alpha1.Service {
			return &v1alpha1.Service{ObjectMeta: meta1("web"), Spec: v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{
				Name: "web", Configs: []string{"web-host"}, RoleAttributes: []string{"web"},
			}}}
		},
		specChanges: []func(mg *v1alpha1.Service){
			func(mg *v1alpha1.Service) { mg.Spec.ForProvider.RoleAttributes = []string{"web", "api", "web"} },
			func(mg *v1alpha1.Service) { mg.Spec.ForProvider.RoleAttributes = nil },
			func(mg *v1alpha1.Service) { mg.Spec.ForProvider.EncryptionRequired = ptr.To(false) },
			func(mg *v1alpha1.Service) { mg.Spec.ForProvider.Tags = map[string]string{"env": "e2e"} },
			func(mg *v1alpha1.Service) { mg.Spec.ForProvider.MaxIdleTimeMillis = ptr.To(int64(30000)) },
			func(mg *v1alpha1.Service) { mg.Spec.ForProvider.Configs = []string{"cfg-1", "web-host"} },
		},
		byHand: map[string]any{"roleAttributes": []any{"theirs"}, "encryptionRequired": false},
		tampers: []func(e map[string]any){
			func(e map[string]any) { e["roleAttributes"] = []any{"by-hand"} },
			func(e map[string]any) { e["encryptionRequired"] = false },
			func(e map[string]any) { e["tags"] = map[string]any{"by": "hand"} },
			func(e map[string]any) { delete(e, "configs") },
			func(e map[string]any) { e["maxIdleTimeMillis"] = float64(1) },
		},
	}
	srv := fake.NewServer()
	f.Cleanup(srv.Close)
	f.Fuzz(func(t *testing.T, data []byte) { m.run(t, srv, data) })
}

func FuzzReconcileIdentity(f *testing.F) {
	reconcileSeeds(f)
	m := model[*v1alpha1.Identity]{
		kind:   identity.Kind,
		update: http.MethodPatch,
		seed: func(srv *fake.Server) {
			srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})
		},
		mg: func() *v1alpha1.Identity {
			return &v1alpha1.Identity{ObjectMeta: meta1("web-client"), Spec: v1alpha1.IdentitySpec{
				ManagedResourceSpec: withSecret("web-client"),
				ForProvider:         v1alpha1.IdentityParameters{Name: "web-client", RoleAttributes: []string{"clients"}},
			}}
		},
		specChanges: []func(mg *v1alpha1.Identity){
			func(mg *v1alpha1.Identity) { mg.Spec.ForProvider.RoleAttributes = []string{"clients", "web"} },
			func(mg *v1alpha1.Identity) { mg.Spec.ForProvider.IsAdmin = ptr.To(true) },
			func(mg *v1alpha1.Identity) { mg.Spec.ForProvider.AppData = map[string]string{"site": "berlin"} },
			func(mg *v1alpha1.Identity) { mg.Spec.ForProvider.ServiceHostingCosts = map[string]int32{"web": 10} },
			func(mg *v1alpha1.Identity) { mg.Spec.ForProvider.ServiceHostingCosts = nil },
		},
		tampers: []func(e map[string]any){
			func(e map[string]any) { e["roleAttributes"] = []any{"by-hand"} },
			func(e map[string]any) { e["isAdmin"] = true },
			func(e map[string]any) { e["appData"] = map[string]any{"by": "hand"} },
			// The enrollment is deleted: the identity gets a new one.
			func(e map[string]any) {
				e["enrollment"] = map[string]any{}
				e["authenticators"] = map[string]any{}
			},
			// The identity enrolls.
			func(e map[string]any) {
				e["enrollment"] = map[string]any{}
				e["authenticators"] = map[string]any{"cert": map[string]any{"id": "auth-1"}}
			},
		},
		// Ziti reports that an identity created by hand has neither
		// enrolled nor an enrollment: it gets one once it is adopted.
		byHand: map[string]any{"roleAttributes": []any{"theirs"}, "authenticators": map[string]any{}, "enrollment": map[string]any{}},
		secret: "web-client-enrollment",
		token:  identityToken("ott"),
	}
	srv := fake.NewServer()
	f.Cleanup(srv.Close)
	f.Fuzz(func(t *testing.T, data []byte) { m.run(t, srv, data) })
}

// deepCopy returns a copy of an entity that shares nothing with it.
func deepCopy(e map[string]any) map[string]any {
	if e == nil {
		return nil
	}
	raw, err := json.Marshal(e)
	if err != nil {
		panic(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}
