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

package fake_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/client/fake"
)

func TestFaults(t *testing.T) {
	cases := map[string]struct {
		fault fake.Fault
		// timeout is how long the client waits for an answer.
		timeout time.Duration

		wantStatus  int
		wantCreated bool
	}{
		"Refused": {
			fault:      fake.Fault{Method: http.MethodPost, Collection: "services", Status: http.StatusServiceUnavailable, Code: "UNAVAILABLE"},
			wantStatus: http.StatusServiceUnavailable,
		},
		"CarriedOutWithoutAnAnswer": {
			fault:       fake.Fault{Method: http.MethodPost, Status: http.StatusGatewayTimeout, CarriedOut: true},
			wantStatus:  http.StatusGatewayTimeout,
			wantCreated: true,
		},
		"ConnectionClosed": {
			fault: fake.Fault{Collection: "services"},
		},
		"ClientGivesUp": {
			fault:       fake.Fault{Status: http.StatusGatewayTimeout, CarriedOut: true, Delay: time.Minute},
			timeout:     100 * time.Millisecond,
			wantCreated: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			api, err := srv.Client()
			if err != nil {
				t.Fatalf("cannot create client: %v", err)
			}

			// A fault for another collection or method is left for later.
			srv.Inject(fake.Fault{Method: http.MethodDelete, Status: http.StatusConflict}, fake.Fault{Collection: "identities", Status: http.StatusConflict})
			srv.Inject(tc.fault)

			ctx := context.Background()
			if tc.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.timeout)
				defer cancel()
			}
			_, err = api.Create(ctx, "services", map[string]any{"name": "web"})

			var apiErr *client.Error
			switch {
			case err == nil:
				t.Fatalf("Create(...): want an error, got none")
			case tc.wantStatus == 0 && errors.As(err, &apiErr):
				t.Errorf("Create(...): want an error without an answer, got %v", err)
			case tc.wantStatus != 0 && (!errors.As(err, &apiErr) || apiErr.StatusCode != tc.wantStatus):
				t.Errorf("Create(...): want an answer with status %d, got %v", tc.wantStatus, err)
			}
			if got := srv.Len("services") == 1; got != tc.wantCreated {
				t.Errorf("service created: want %t, got %t", tc.wantCreated, got)
			}

			if diff := cmp.Diff(2, len(srv.ClearFaults())); diff != "" {
				t.Errorf("faults left: -want, +got:\n%s", diff)
			}
			if _, err := api.Create(context.Background(), "services", map[string]any{"name": "api"}); err != nil {
				t.Errorf("Create(...) after the faults are cleared: %v", err)
			}
		})
	}
}

func TestStoredLikeZiti(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()
	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}
	ctx := context.Background()

	id, err := api.Create(ctx, "identities", map[string]any{"name": "web-client", "roleAttributes": []string{"web", "clients", "web"}})
	if err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	if diff := cmp.Diff([]any{"clients", "web"}, srv.Entity("identities", id)["roleAttributes"]); diff != "" {
		t.Errorf("role attributes after a creation: -want, +got:\n%s", diff)
	}
	if err := api.Patch(ctx, "identities", id, map[string]any{"roleAttributes": []string{"z", "a", "z"}}); err != nil {
		t.Fatalf("Patch(...): %v", err)
	}
	if diff := cmp.Diff([]any{"a", "z"}, srv.Entity("identities", id)["roleAttributes"]); diff != "" {
		t.Errorf("role attributes after a PATCH: -want, +got:\n%s", diff)
	}

	check, err := api.Create(ctx, "posture-checks", map[string]any{
		"name":         "devices",
		"typeId":       "MAC",
		"macAddresses": []string{"00:1A:2B:3C:4D:5E", "001a.2b3c.4d5e", "0A-1B-2C-3D-4E-5F"},
	})
	if err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	if diff := cmp.Diff([]any{"001a2b3c4d5e", "0a1b2c3d4e5f"}, srv.Entity("posture-checks", check)["macAddresses"]); diff != "" {
		t.Errorf("MAC addresses: -want, +got:\n%s", diff)
	}

	os, err := api.Create(ctx, "posture-checks", map[string]any{
		"name":   "os",
		"typeId": "OS",
		"operatingSystems": []map[string]any{
			{"type": "macOS", "versions": []string{"14", "13", "14"}},
			{"type": "Linux", "versions": []string{"5"}},
			{"type": "macOS", "versions": []string{"15"}},
		},
	})
	if err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	want := []any{
		map[string]any{"type": "Linux", "versions": []any{"5"}},
		map[string]any{"type": "macOS", "versions": []any{"15"}},
	}
	if diff := cmp.Diff(want, srv.Entity("posture-checks", os)["operatingSystems"]); diff != "" {
		t.Errorf("operating systems: -want, +got:\n%s", diff)
	}
}

func TestDeletionDropsReferences(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()
	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	srv.Put("configs", map[string]any{"id": "cfg-1", "name": "web-host"})
	srv.Put("services", map[string]any{"id": "svc-1", "name": "web", "configs": []any{"cfg-1", "cfg-2"}})
	srv.Put("identities", map[string]any{"id": "idn-1", "name": "web-server", "serviceHostingCosts": map[string]any{"svc-1": float64(10)}})
	srv.Put("service-policies", map[string]any{
		"id": "sp-1", "name": "web-bind", "serviceRoles": []any{"@svc-1", "#web"}, "identityRoles": []any{"@idn-1"},
	})

	srv.Delete("configs", "cfg-1")
	if diff := cmp.Diff([]any{"cfg-2"}, srv.Entity("services", "svc-1")["configs"]); diff != "" {
		t.Errorf("configs of the service: -want, +got:\n%s", diff)
	}
	if err := api.Delete(context.Background(), "services", "svc-1"); err != nil {
		t.Fatalf("Delete(...): %v", err)
	}

	policy := srv.Entity("service-policies", "sp-1")
	if diff := cmp.Diff([]any{"#web"}, policy["serviceRoles"]); diff != "" {
		t.Errorf("service roles of the policy: -want, +got:\n%s", diff)
	}
	if diff := cmp.Diff([]any{"@idn-1"}, policy["identityRoles"]); diff != "" {
		t.Errorf("identity roles of the policy: -want, +got:\n%s", diff)
	}
	if diff := cmp.Diff(map[string]any{"svc-1": float64(10)}, srv.Entity("identities", "idn-1")["serviceHostingCosts"]); diff != "" {
		t.Errorf("hosting costs of the identity: -want, +got:\n%s", diff)
	}
}
