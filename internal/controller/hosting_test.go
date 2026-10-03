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
	"strings"
	"testing"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
)

// TestHostingSettingsNameAServiceTwice gives an identity hosting settings
// for one service under its name and under its ID. With different values
// there is no telling which one is meant, and the value sent to Ziti would
// change from one poll to the next; the spec is refused instead. With the
// same value it is unambiguous.
func TestHostingSettingsNameAServiceTwice(t *testing.T) {
	cases := map[string]struct {
		costs       map[string]int32
		precedences map[string]string
		wantErr     string
	}{
		"CostsDiffer": {
			costs:   map[string]int32{"web": 10, "svc-1": 20},
			wantErr: `"svc-1" and "web" name the same entity in services, with different settings`,
		},
		"PrecedencesDiffer": {
			precedences: map[string]string{"web": "default", "svc-1": "required"},
			wantErr:     `"svc-1" and "web" name the same entity in services, with different settings`,
		},
		"SameCost": {
			costs: map[string]int32{"web": 10, "svc-1": 10, "db": 20},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			srv.Put("services", map[string]any{"id": "svc-1", "name": "web"})
			srv.Put("services", map[string]any{"id": "svc-2", "name": "db"})

			mg := &v1alpha1.Identity{
				ObjectMeta: meta1("web-server"),
				Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{
					Name: "web-server", ServiceHostingCosts: tc.costs, ServiceHostingPrecedences: tc.precedences,
				}},
			}
			o, err := createAndObserve(identity.Kind)(context.Background(), zitiClient(t, srv), mg)
			if tc.wantErr == "" {
				if err != nil || !o.ResourceUpToDate {
					t.Errorf("Observe(...) after creation: want an up to date resource, got %+v, %v", o, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Create(...): want an error containing %q, got %v", tc.wantErr, err)
			}
			if got := srv.Len("identities"); got != 0 {
				t.Errorf("want no identity created, got %d", got)
			}
		})
	}
}
