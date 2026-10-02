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

// counter counts the updates of managed resources.
func counter(updates *int) kube.Client {
	return &test.MockClient{MockUpdate: func(_ context.Context, _ kube.Object, _ ...kube.UpdateOption) error {
		*updates++
		return nil
	}}
}

func TestCreationRecoverer(t *testing.T) {
	// When the creation in question started, on the local clock.
	started := time.Now().Add(-10 * time.Second).Truncate(time.Second)

	// A service named like the managed resource that the controller, whose
	// clock is ahead by the supplied duration, created so long after the
	// creation started.
	entity := func(after, ahead time.Duration) map[string]any {
		return map[string]any{"id": "svc-1", "name": "web", "createdAt": started.Add(after + ahead).UTC().Format(time.RFC3339Nano)}
	}

	cases := map[string]struct {
		reason string

		// pending is when a creation started, if the reconciler recorded one.
		pending time.Time
		// succeeded and failed are when its result was recorded, if it was.
		succeeded time.Time
		failed    time.Time
		// unconfirmed is when a creation request was sent that was not answered.
		unconfirmed time.Time
		// ahead is how far the clock of the controller is ahead.
		ahead time.Duration
		// entity is the Ziti service named like the managed resource, if any.
		entity map[string]any

		wantExternalName string
		wantIncomplete   bool
		wantUnconfirmed  bool
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
			entity:    entity(time.Second, 0),
		},
		"EntityWasCreated": {
			reason:           "The entity an interrupted creation left behind is taken over.",
			pending:          started,
			entity:           entity(time.Second, 0),
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
			reason:       "An entity that was there before the creation started belongs to someone else; the creation failed.",
			pending:      started,
			entity:       entity(-time.Hour, 0),
			wantUpdates:  1,
			wantRequests: 1,
		},
		"EntityIsSlightlyOlderThanTheCreation": {
			reason:       "Half a minute before the creation started is before it started.",
			pending:      started,
			entity:       entity(-30*time.Second, 0),
			wantUpdates:  1,
			wantRequests: 1,
		},
		"EntityOfUnknownAge": {
			reason:       "An entity that does not say when it was created is not taken over.",
			pending:      started,
			entity:       map[string]any{"id": "svc-1", "name": "web"},
			wantUpdates:  1,
			wantRequests: 1,
		},
		"ControllerClockAhead": {
			reason:           "The age of an entity is judged by the clock of the controller that stamped it.",
			pending:          started,
			ahead:            time.Hour,
			entity:           entity(time.Second, time.Hour),
			wantExternalName: "svc-1",
			wantUpdates:      1,
			wantRequests:     1,
		},
		"ControllerClockAheadOlderEntity": {
			reason:       "A clock that runs ahead does not make an older entity look new.",
			pending:      started,
			ahead:        time.Hour,
			entity:       entity(-30*time.Second, time.Hour),
			wantUpdates:  1,
			wantRequests: 1,
		},
		"ControllerClockBehind": {
			reason:           "A clock that runs behind does not make the entity of the creation look old.",
			pending:          started,
			ahead:            -time.Hour,
			entity:           entity(time.Second, -time.Hour),
			wantExternalName: "svc-1",
			wantUpdates:      1,
			wantRequests:     1,
		},
		"UnansweredRequestCreatedTheEntity": {
			reason:           "The entity of a request that was never answered is taken over.",
			pending:          started,
			failed:           started.Add(2 * time.Second),
			unconfirmed:      started,
			entity:           entity(time.Second, 0),
			wantExternalName: "svc-1",
			wantUpdates:      1,
			wantRequests:     1,
		},
		"UnansweredRequestCreatedNothingYet": {
			reason:          "A request that was never answered may still take effect, so it stays on record.",
			pending:         started,
			failed:          started.Add(2 * time.Second),
			unconfirmed:     started,
			wantUnconfirmed: true,
			wantRequests:    1,
		},
		"UnansweredRequestAndAnOlderEntity": {
			reason:       "A request that was never answered cannot have created an entity that was there before.",
			pending:      started,
			failed:       started.Add(2 * time.Second),
			unconfirmed:  started,
			entity:       entity(-time.Hour, 0),
			wantUpdates:  1,
			wantRequests: 1,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			srv.SetClockAhead(tc.ahead)
			if tc.entity != nil {
				srv.Put("services", tc.entity)
			}

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
			if !tc.failed.IsZero() {
				meta.SetExternalCreateFailed(mg, tc.failed)
			}
			if !tc.unconfirmed.IsZero() {
				meta.AddAnnotations(mg, map[string]string{generic.AnnotationKeyCreateUnconfirmed: tc.unconfirmed.UTC().Format(time.RFC3339)})
			}

			updates := 0
			r := generic.NewCreationRecoverer(counter(&updates), connecterFn(srv.Client), service.Kind.Collection)
			if err := r.Initialize(context.Background(), mg); err != nil {
				t.Fatalf("\n%s\nInitialize(...): %v", tc.reason, err)
			}

			if got := meta.GetExternalName(mg); got != tc.wantExternalName {
				t.Errorf("\n%s\nexternal name: want %q, got %q", tc.reason, tc.wantExternalName, got)
			}
			if got := meta.ExternalCreateIncomplete(mg); got != tc.wantIncomplete {
				t.Errorf("\n%s\ncreation incomplete: want %t, got %t", tc.reason, tc.wantIncomplete, got)
			}
			if _, got := mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed]; got != tc.wantUnconfirmed {
				t.Errorf("\n%s\nunanswered request on record: want %t, got %t", tc.reason, tc.wantUnconfirmed, got)
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

// TestUnansweredCreation follows a creation that Ziti carries out without
// the provider ever learning its result.
func TestUnansweredCreation(t *testing.T) {
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
		Spec:       v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{Name: "web"}},
	}

	srv.LoseNextCreateResponse()
	if _, err := e.Create(ctx, mg); err == nil {
		t.Fatalf("Create(...) without an answer: want an error, got none")
	}
	if got := meta.GetExternalName(mg); got != "" {
		t.Errorf("external name after a creation without an answer: want none, got %q", got)
	}
	if mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed] == "" {
		t.Fatalf("want the unanswered request to be put on record, got annotations %v", mg.GetAnnotations())
	}
	if got := srv.Len("services"); got != 1 {
		t.Fatalf("want Ziti to have created the service, got %d services", got)
	}

	updates := 0
	r := generic.NewCreationRecoverer(counter(&updates), connecterFn(srv.Client), service.Kind.Collection)
	if err := r.Initialize(ctx, mg); err != nil {
		t.Fatalf("Initialize(...): %v", err)
	}
	if got := meta.GetExternalName(mg); got == "" || srv.Entity("services", got) == nil {
		t.Errorf("external name after recovery: want the ID of the service Ziti created, got %q", got)
	}
	if _, ok := mg.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed]; ok {
		t.Errorf("want the unanswered request to be settled, got annotations %v", mg.GetAnnotations())
	}

	o, err := e.Observe(ctx, mg)
	if err != nil || !o.ResourceExists {
		t.Errorf("Observe(...) after recovery: want an existing resource, got %+v, %v", o, err)
	}

	// A request that Ziti rejects leaves no doubt and is not put on record.
	duplicate := &v1alpha1.Service{
		ObjectMeta: meta1("web-again"),
		Spec:       v1alpha1.ServiceSpec{ForProvider: v1alpha1.ServiceParameters{Name: "web"}},
	}
	if _, err := e.Create(ctx, duplicate); err == nil {
		t.Fatalf("Create(...) of a duplicate name: want an error, got none")
	}
	if _, ok := duplicate.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed]; ok {
		t.Errorf("want no record of a request Ziti rejected, got annotations %v", duplicate.GetAnnotations())
	}
}
