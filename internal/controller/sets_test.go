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
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/go-cmp/cmp"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouter"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/service"
	"github.com/crossplane/provider-ziti/internal/controller/servicepolicy"
)

// TestListsWithDuplicatesSettle checks that a list of a spec that names
// something twice is not taken for drift. Ziti stores the lists of strings
// of an entity as sets and reports each item once, so an entity that was
// created as declared would otherwise be updated on every poll.
func TestListsWithDuplicatesSettle(t *testing.T) {
	seed := func(srv *fake.Server) {
		srv.Put("configs", map[string]any{"id": "cfg-1", "name": "web-host"})
		srv.Put("identities", map[string]any{"id": "idn-1", "name": "web-client"})
	}

	cases := map[string]struct {
		mg         resource.ModernManaged
		create     func(ctx context.Context, api *client.Client, mg resource.ModernManaged) (managed.ExternalObservation, error)
		collection string
		want       map[string]any
	}{
		"Service": {
			mg: &v1alpha1.Service{
				ObjectMeta: meta1("web"),
				Spec: v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{
					Name:           "web",
					RoleAttributes: []string{"web", "api", "web"},
					// The same config by its name and by its ID.
					Configs: []string{"web-host", "cfg-1"},
				}},
			},
			create:     createAndObserve(service.Kind),
			collection: "services",
			want:       map[string]any{"roleAttributes": []any{"api", "web"}, "configs": []any{"cfg-1"}},
		},
		"ServicePolicy": {
			mg: &v1alpha1.ServicePolicy{
				ObjectMeta: meta1("web-dial"),
				Spec: v1alpha1.ServicePolicySpec{ForProvider: v1alpha1.ServicePolicyParameters{
					Name:          "web-dial",
					Type:          v1alpha1.ServicePolicyTypeDial,
					ServiceRoles:  []string{"#web", "#web"},
					IdentityRoles: []string{"@web-client", "@idn-1"},
				}},
			},
			create:     createAndObserve(servicepolicy.Kind),
			collection: "service-policies",
			want:       map[string]any{"serviceRoles": []any{"#web"}, "identityRoles": []any{"@idn-1"}},
		},
		"Identity": {
			mg: &v1alpha1.Identity{
				ObjectMeta: meta1("web-server"),
				Spec:       v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{Name: "web-server", RoleAttributes: []string{"servers", "servers"}}},
			},
			create:     createAndObserve(identity.Kind),
			collection: "identities",
			want:       map[string]any{"roleAttributes": []any{"servers"}},
		},
		"EdgeRouter": {
			mg: &v1alpha1.EdgeRouter{
				ObjectMeta: meta1("router-1"),
				Spec:       v1alpha1.EdgeRouterSpec{ForProvider: v1alpha1.EdgeRouterParameters{Name: "router-1", RoleAttributes: []string{"public", "public"}}},
			},
			create:     createAndObserve(edgerouter.Kind),
			collection: "edge-routers",
			want:       map[string]any{"roleAttributes": []any{"public"}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			seed(srv)
			api, err := srv.Client()
			if err != nil {
				t.Fatalf("cannot create client: %v", err)
			}

			o, err := tc.create(context.Background(), api, tc.mg)
			if err != nil || !o.ResourceExists || !o.ResourceUpToDate {
				t.Errorf("Observe(...) after creation: want an up to date resource, got %+v, %v", o, err)
			}
			got := srv.Entity(tc.collection, meta.GetExternalName(tc.mg))
			for field, want := range tc.want {
				if diff := cmp.Diff(want, got[field]); diff != "" {
					t.Errorf("%s in Ziti: -want, +got:\n%s", field, diff)
				}
			}
		})
	}
}

// createAndObserve returns a function that creates the entity of a managed
// resource of the supplied kind and then observes it.
func createAndObserve[T resource.ModernManaged](kind generic.Kind[T]) func(context.Context, *client.Client, resource.ModernManaged) (managed.ExternalObservation, error) {
	return func(ctx context.Context, api *client.Client, mg resource.ModernManaged) (managed.ExternalObservation, error) {
		e := generic.NewExternalClient(kind, api)
		//nolint:forcetypeassert // Each case pairs a kind with a managed resource of that kind.
		if _, err := e.Create(ctx, mg.(T)); err != nil {
			return managed.ExternalObservation{}, err
		}
		//nolint:forcetypeassert // See above.
		return e.Observe(ctx, mg.(T))
	}
}
