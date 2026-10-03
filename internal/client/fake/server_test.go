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
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/crossplane/provider-ziti/internal/client/fake"
)

// TestPatchCertificateAuthority checks that a PATCH of a certificate
// authority treats its external ID claim like the real controller does: a
// PATCH that leaves it out or sets it to null removes it, and the other
// fields that are null are ignored.
func TestPatchCertificateAuthority(t *testing.T) {
	claim := map[string]any{"location": "COMMON_NAME", "matcher": "ALL", "parser": "NONE"}
	other := map[string]any{"location": "SAN_URI", "matcher": "SCHEME", "matcherCriteria": "spiffe", "parser": "NONE"}

	cases := map[string]struct {
		patch map[string]any
		want  any
	}{
		"LeftOut": {
			patch: map[string]any{"identityRoles": []any{"sensors"}},
		},
		"Null": {
			patch: map[string]any{"externalIdClaim": nil},
		},
		"Changed": {
			patch: map[string]any{"externalIdClaim": other, "identityNameFormat": nil},
			want:  other,
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
			srv.Put("cas", map[string]any{"id": "ca-1", "name": "devices", "identityNameFormat": "[caName]-[commonName]", "externalIdClaim": claim})

			if err := api.Patch(context.Background(), "cas", "ca-1", tc.patch); err != nil {
				t.Fatalf("Patch(...): %v", err)
			}
			ca := srv.Entity("cas", "ca-1")
			if diff := cmp.Diff(tc.want, ca["externalIdClaim"]); diff != "" {
				t.Errorf("externalIdClaim after the PATCH: -want, +got:\n%s", diff)
			}
			if got := ca["identityNameFormat"]; got != "[caName]-[commonName]" {
				t.Errorf("identityNameFormat after the PATCH: want it kept, got %v", got)
			}
		})
	}
}
