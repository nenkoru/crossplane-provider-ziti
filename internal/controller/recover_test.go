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
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	kube "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/service"
)

// connecterFn connects every managed resource to the same Ziti controller.
type connecterFn func() (*client.Client, error)

func (fn connecterFn) Connect(_ context.Context, _ resource.ModernManaged) (*client.Client, error) {
	return fn()
}

func TestCreationRecoverer(t *testing.T) {
	started := time.Now().Add(-10 * time.Second)

	cases := map[string]struct {
		reason string

		// pending is when the creation started, if one did.
		pending time.Time
		// succeeded is when the creation was recorded as done, if it was.
		succeeded time.Time
		// entity is the Ziti service named like the managed resource, if any.
		entity map[string]any

		wantExternalName string
		wantIncomplete   bool
		wantUpdates      int
		wantRequests     int
	}{
		"NoCreationStarted": {
			reason: "A resource that was never created is left alone.",
		},
		"CreationWasRecorded": {
			reason:    "A creation whose result was saved needs no recovery.",
			pending:   started,
			succeeded: started.Add(time.Second),
			entity:    map[string]any{"id": "svc-1", "name": "web", "createdAt": started.UTC().Format(time.RFC3339Nano)},
		},
		"EntityWasCreated": {
			reason:           "The entity an interrupted creation left behind is taken over.",
			pending:          started,
			entity:           map[string]any{"id": "svc-1", "name": "web", "createdAt": started.Add(time.Second).UTC().Format(time.RFC3339Nano)},
			wantExternalName: "svc-1",
			wantUpdates:      1,
			wantRequests:     1,
		},
		"NothingWasCreated": {
			reason:       "An interrupted creation that left nothing behind may be retried.",
			pending:      started,
			wantUpdates:  1,
			wantRequests: 1,
		},
		"EntityIsOlderThanTheCreation": {
			reason:         "An entity that was there before the creation started belongs to someone else.",
			pending:        started,
			entity:         map[string]any{"id": "svc-1", "name": "web", "createdAt": started.Add(-time.Hour).UTC().Format(time.RFC3339Nano)},
			wantIncomplete: true,
			wantRequests:   1,
		},
		"EntityOfUnknownAge": {
			reason:         "An entity that does not say when it was created is not taken over.",
			pending:        started,
			entity:         map[string]any{"id": "svc-1", "name": "web"},
			wantIncomplete: true,
			wantRequests:   1,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			if tc.entity != nil {
				srv.Put("services", tc.entity)
			}

			updates := 0
			k := &test.MockClient{MockUpdate: func(_ context.Context, _ kube.Object, _ ...kube.UpdateOption) error {
				updates++
				return nil
			}}

			mg := &v1alpha1.Service{
				ObjectMeta: meta1("web"),
				Spec:       v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{Name: "web"}},
			}
			if !tc.pending.IsZero() {
				meta.SetExternalCreatePending(mg, tc.pending)
			}
			if !tc.succeeded.IsZero() {
				meta.SetExternalCreateSucceeded(mg, tc.succeeded)
			}

			r := generic.NewCreationRecoverer(k, connecterFn(srv.Client), service.Kind.Collection)
			if err := r.Initialize(context.Background(), mg); err != nil {
				t.Fatalf("\n%s\nInitialize(...): %v", tc.reason, err)
			}

			if got := meta.GetExternalName(mg); got != tc.wantExternalName {
				t.Errorf("\n%s\nexternal name: want %q, got %q", tc.reason, tc.wantExternalName, got)
			}
			if got := meta.ExternalCreateIncomplete(mg); got != tc.wantIncomplete {
				t.Errorf("\n%s\ncreation incomplete: want %t, got %t", tc.reason, tc.wantIncomplete, got)
			}
			if updates != tc.wantUpdates {
				t.Errorf("\n%s\nupdates of the managed resource: want %d, got %d", tc.reason, tc.wantUpdates, updates)
			}
			if got := len(srv.Requests()); got != tc.wantRequests {
				t.Errorf("\n%s\nrequests to Ziti: want %d, got %v", tc.reason, tc.wantRequests, srv.Requests())
			}
			if got := srv.Len("services"); tc.entity != nil && got != 1 {
				t.Errorf("\n%s\nservices in Ziti: want the one that was there, got %d", tc.reason, got)
			}
		})
	}
}
