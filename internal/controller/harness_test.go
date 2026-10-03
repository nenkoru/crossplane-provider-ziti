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
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// A harness reconciles one managed resource the way the provider does: with
// the reconciler of Crossplane that generic.Setup builds, against an
// in-memory Kubernetes API and a fake Ziti controller.
type harness[T resource.ModernManaged] struct {
	t    testing.TB
	srv  *fake.Server
	kube kube.Client
	kind generic.Kind[T]
	key  types.NamespacedName

	reconciler *managed.Reconciler
}

// manager is the part of a controller manager the reconciler uses.
type manager struct {
	ctrl.Manager

	client kube.Client
	scheme *runtime.Scheme
}

func (m manager) GetClient() kube.Client     { return m.client }
func (m manager) GetScheme() *runtime.Scheme { return m.scheme }

func (m manager) GetEventRecorderFor(string) record.EventRecorder {
	return &record.FakeRecorder{}
}

// newHarness stores the supplied managed resource in a new Kubernetes API
// and returns a harness that reconciles it against the supplied fake Ziti
// controller, with the supplied options in addition to those of the
// provider.
func newHarness[T resource.ModernManaged](t testing.TB, srv *fake.Server, kind generic.Kind[T], mg T, opts ...managed.ReconcilerOption) *harness[T] {
	t.Helper()

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, apis.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatalf("cannot build scheme: %v", err)
		}
	}
	// The API server fills in the default management policy.
	if len(mg.GetManagementPolicies()) == 0 {
		mg.SetManagementPolicies(xpv2.ManagementPolicies{xpv2.ManagementActionAll})
	}
	k := kubefake.NewClientBuilder().WithScheme(scheme).WithObjects(mg).WithStatusSubresource(mg).Build()

	features := &feature.Flags{}
	features.Enable(feature.EnableBetaManagementPolicies)
	o := xpcontroller.Options{Logger: logging.NewNopLogger(), PollInterval: time.Minute, Features: features}

	// Crossplane waits half a minute after a creation before it creates an
	// entity again that it does not find, in case the API it talks to
	// reports new entities late. Ziti does not, and the tests do not wait.
	opts = append([]managed.ReconcilerOption{managed.WithCreationGracePeriod(0)}, opts...)
	api := zitiClient(t, srv)
	r, err := generic.NewReconciler(manager{client: k, scheme: scheme}, o, connecterFn(func() (*client.Client, error) { return api, nil }), kind, opts...)
	if err != nil {
		t.Fatalf("cannot build the reconciler: %v", err)
	}
	return &harness[T]{t: t, srv: srv, kube: k, kind: kind, key: kube.ObjectKeyFromObject(mg), reconciler: r}
}

// reconcile reconciles the managed resource once and returns how many
// requests that change something it sent to Ziti.
func (h *harness[T]) reconcile() int {
	h.t.Helper()

	before := writes(h.srv)
	if _, err := h.reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: h.key}); err != nil {
		h.t.Fatalf("Reconcile(...): %v", err)
	}
	return writes(h.srv) - before
}

// writes counts the requests that change something the fake controller
// received.
func writes(srv *fake.Server) int {
	n := 0
	for _, r := range srv.Requests() {
		if !strings.HasPrefix(r, http.MethodGet+" ") {
			n++
		}
	}
	return n
}

// get returns the managed resource as the Kubernetes API has it, or nil
// once it is gone.
func (h *harness[T]) get() T {
	h.t.Helper()

	var zero T
	o, err := newObject(h)
	if err != nil {
		h.t.Fatalf("cannot create a managed resource: %v", err)
	}
	if err := h.kube.Get(context.Background(), h.key, o); err != nil {
		if kerrors.IsNotFound(err) {
			return zero
		}
		h.t.Fatalf("cannot get the managed resource: %v", err)
	}
	return o
}

// exists returns true while the Kubernetes API has the managed resource.
func (h *harness[T]) exists() bool {
	h.t.Helper()

	o, err := newObject(h)
	if err != nil {
		h.t.Fatalf("cannot create a managed resource: %v", err)
	}
	err = h.kube.Get(context.Background(), h.key, o)
	if err != nil && !kerrors.IsNotFound(err) {
		h.t.Fatalf("cannot get the managed resource: %v", err)
	}
	return err == nil
}

// newObject returns an empty managed resource of the kind of the harness.
func newObject[T resource.ModernManaged](h *harness[T]) (T, error) {
	var zero T
	o, err := h.kube.Scheme().New(h.kind.GVK)
	if err != nil {
		return zero, err
	}
	//nolint:forcetypeassert // The scheme returns the type registered for the kind.
	return o.(T), nil
}

// update changes the managed resource in the Kubernetes API.
func (h *harness[T]) update(change func(mg T)) {
	h.t.Helper()

	mg := h.get()
	change(mg)
	if err := h.kube.Update(context.Background(), mg); err != nil {
		h.t.Fatalf("cannot update the managed resource: %v", err)
	}
}

// remove asks the Kubernetes API to delete the managed resource.
func (h *harness[T]) remove() {
	h.t.Helper()

	if err := h.kube.Delete(context.Background(), h.get()); err != nil {
		h.t.Fatalf("cannot delete the managed resource: %v", err)
	}
}

// converge reconciles the managed resource until it is ready and synced and
// two reconciles in a row change nothing in Ziti, and fails the test if that
// takes more than the supplied number of reconciles.
func (h *harness[T]) converge(limit int) T {
	h.t.Helper()

	quiet := 0
	for range limit {
		if h.reconcile() == 0 {
			quiet++
		} else {
			quiet = 0
		}
		mg := h.get()
		if quiet >= 2 && ready(mg) {
			return mg
		}
	}
	mg := h.get()
	h.t.Fatalf("the managed resource does not converge within %d reconciles: external name %q, conditions %+v, requests %v",
		limit, meta.GetExternalName(mg), mg.GetCondition(xpv2.TypeSynced), h.srv.Requests())
	return mg
}

// ready returns true if the managed resource is ready and synced.
func ready(mg resource.Managed) bool {
	return mg.GetCondition(xpv2.TypeReady).Status == corev1.ConditionTrue &&
		mg.GetCondition(xpv2.TypeSynced).Status == corev1.ConditionTrue
}

// observe observes the managed resource without the faults injected into
// the fake controller.
func (h *harness[T]) observe(mg T) managed.ExternalObservation {
	h.t.Helper()

	faults := h.srv.ClearFaults()
	defer h.srv.Inject(faults...)

	//nolint:forcetypeassert // A deep copy has the type of the original.
	o, err := generic.NewExternalClient(h.kind, zitiClient(h.t, h.srv)).Observe(context.Background(), mg.DeepCopyObject().(T))
	if err != nil {
		h.t.Fatalf("Observe(...): %v", err)
	}
	return o
}

// secret returns the connection secret of the managed resource, or nil if
// there is none.
func (h *harness[T]) secret(name string) map[string][]byte {
	h.t.Helper()

	s := &corev1.Secret{}
	err := h.kube.Get(context.Background(), types.NamespacedName{Namespace: h.key.Namespace, Name: name}, s)
	if kerrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		h.t.Fatalf("cannot get the connection secret: %v", err)
	}
	return s.Data
}

// clients holds a client per fake controller. Fuzz tests run thousands of
// reconciles against one controller; a client per reconcile would use up
// the local ports with connections that are closed and waiting.
var (
	clientsMu sync.Mutex
	clients   = map[*fake.Server]*client.Client{}
)

// zitiClient returns the client of the fake controller.
func zitiClient(t testing.TB, srv *fake.Server) *client.Client {
	t.Helper()

	clientsMu.Lock()
	defer clientsMu.Unlock()
	if api, ok := clients[srv]; ok {
		return api
	}
	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}
	clients[srv] = api
	return api
}
