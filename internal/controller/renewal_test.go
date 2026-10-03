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
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/go-cmp/cmp"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/crossplane/provider-ziti/apis"
	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/edgerouter"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
	"github.com/crossplane/provider-ziti/internal/controller/identityca"
	"github.com/crossplane/provider-ziti/internal/controller/identitynone"
	"github.com/crossplane/provider-ziti/internal/controller/identityupdb"
)

const (
	// staleToken is the enrollment token of an entity before it is renewed.
	staleToken = "stale-token"

	// defaultLifetime is how long a new enrollment token is valid unless the
	// managed resource says otherwise.
	defaultLifetime = 180 * time.Minute

	createEnrollment  = "POST /enrollments"
	refreshEnrollment = "POST /enrollments/enr-1/refresh"
	reEnrollRouter    = "POST /edge-routers/{id}/re-enroll"
)

// stamp formats a time the way Ziti does.
func stamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// A renewalCase is a state of enrollment the Ziti controller reports for an
// entity, and what the provider is to do about it.
type renewalCase struct {
	reason string

	// reported are the fields of the entity that tell the state of its
	// enrollment, given the time on the local clock.
	reported func(now time.Time) map[string]any

	// ahead is how far the clock of the controller is ahead.
	ahead time.Duration

	// seed optionally stores the other entities the controller reports.
	seed func(srv *fake.Server)

	// want is the request that gives the entity a new enrollment token. It
	// is empty if the entity is to be left alone.
	want string
}

// A renewal runs a kind through the states of enrollment of its entity.
type renewal[T resource.ModernManaged] struct {
	kind generic.Kind[T]

	// seed stores the entities the managed resource refers to.
	seed func(srv *fake.Server)

	// mg returns the managed resource to create.
	mg func() T

	// enrollment returns the enrollment token of the entity and when it
	// expires, as the fake controller reports them.
	enrollment func(entity map[string]any) (token, expiresAt string)

	// status returns when the enrollment token expires according to the
	// status of the managed resource.
	status func(mg T) string

	// lifetime is how long a new token is to be valid.
	lifetime time.Duration

	// created are the settings an enrollment is to be created with, besides
	// those of every enrollment.
	created map[string]any
}

// renewals returns the requests for a new enrollment token the fake
// controller received.
func renewals(srv *fake.Server) []string {
	var out []string
	for _, r := range srv.Requests() {
		if strings.HasPrefix(r, createEnrollment) || strings.HasSuffix(r, "/re-enroll") {
			out = append(out, r)
		}
	}
	return out
}

// start creates the entity of the managed resource in a fake controller and
// makes the controller report the supplied state of its enrollment.
func (r renewal[T]) start(t *testing.T, tc renewalCase) (*fake.Server, managed.TypedExternalClient[T], T) {
	t.Helper()

	srv := fake.NewServer()
	t.Cleanup(srv.Close)
	if r.seed != nil {
		r.seed(srv)
	}
	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	e, mg := generic.NewExternalClient(r.kind, api), r.mg()
	if _, err := e.Create(context.Background(), mg); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	created := srv.Entity(r.kind.Collection, meta.GetExternalName(mg))
	delete(created, "enrollment")
	delete(created, "enrollmentJwt")
	maps.Copy(created, tc.reported(time.Now()))
	srv.Put(r.kind.Collection, created)
	srv.SetClockAhead(tc.ahead)
	if tc.seed != nil {
		tc.seed(srv)
	}
	return srv, e, mg
}

func (r renewal[T]) run(t *testing.T, cases map[string]renewalCase) {
	t.Helper()

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv, e, mg := r.start(t, tc)
			ctx := context.Background()
			id := meta.GetExternalName(mg)
			before := entity(t, srv, r.kind.Collection, id)

			o, err := e.Observe(ctx, mg)
			if err != nil {
				t.Fatalf("\n%s\nObserve(...): %v", tc.reason, err)
			}
			if got, want := o.ResourceUpToDate, tc.want == ""; got != want {
				t.Fatalf("\n%s\nObserve(...): want up to date %t, got %t (%s)", tc.reason, want, got, o.Diff)
			}

			// An update that some other difference causes must leave the
			// enrollment alone as well.
			if _, err := e.Update(ctx, mg); err != nil {
				t.Fatalf("\n%s\nUpdate(...): %v", tc.reason, err)
			}
			if tc.want == "" {
				if got := renewals(srv); len(got) != 0 {
					t.Errorf("\n%s\nwant no request for a new enrollment token, got %v", tc.reason, got)
				}
				if diff := cmp.Diff(before, entity(t, srv, r.kind.Collection, id)); diff != "" {
					t.Errorf("\n%s\nentity after Update(...): -want, +got:\n%s", tc.reason, diff)
				}
				return
			}

			// The next observations publish the new token and ask for no
			// other one.
			for range 2 {
				if o, err = e.Observe(ctx, mg); err != nil || !o.ResourceUpToDate {
					t.Fatalf("\n%s\nObserve(...) after the renewal: want an up to date resource, got %+v, %v", tc.reason, o, err)
				}
			}
			if diff := cmp.Diff([]string{strings.ReplaceAll(tc.want, "{id}", id)}, renewals(srv)); diff != "" {
				t.Errorf("\n%s\nrequests for a new enrollment token: -want, +got:\n%s", tc.reason, diff)
			}

			token, expiresAt := r.enrollment(srv.Entity(r.kind.Collection, id))
			if token == "" || token == staleToken {
				t.Errorf("\n%s\nenrollment token in Ziti: want a new one, got %q", tc.reason, token)
			}
			if got := string(o.ConnectionDetails[identity.ConnectionKeyEnrollmentToken]); got != token {
				t.Errorf("\n%s\nenrollment token in the connection details: want %q, got %q", tc.reason, token, got)
			}
			if got := r.status(mg); got != expiresAt {
				t.Errorf("\n%s\nstatus.atProvider.enrollmentExpiresAt: want %q, got %q", tc.reason, expiresAt, got)
			}

			// The token lives as long as intended from now on the clock of
			// the controller.
			expires, err := time.Parse(time.RFC3339, expiresAt)
			if err != nil {
				t.Fatalf("\n%s\ncannot parse the expiry %q: %v", tc.reason, expiresAt, err)
			}
			if got := time.Until(expires) - tc.ahead; got < r.lifetime-time.Minute || got > r.lifetime+time.Minute {
				t.Errorf("\n%s\nlifetime of the new enrollment token: want %s, got %s", tc.reason, r.lifetime, got.Round(time.Second))
			}

			if tc.want != createEnrollment {
				return
			}
			enrollments, _ := srv.Entity(r.kind.Collection, id)["enrollment"].(map[string]any)
			for _, created := range enrollments {
				settings := maps.Clone(created.(map[string]any)) //nolint:forcetypeassert // The fake controller stores enrollments as maps.
				for _, assigned := range []string{"id", "jwt", "expiresAt"} {
					delete(settings, assigned)
				}
				if diff := cmp.Diff(r.created, settings); diff != "" {
					t.Errorf("\n%s\nsettings of the new enrollment: -want, +got:\n%s", tc.reason, diff)
				}
			}
		})
	}
}

// identityRenewalCases are the states of enrollment of an identity that
// enrolls with the supplied method.
func identityRenewalCases(method string) map[string]renewalCase {
	// An identity with the supplied authenticators whose enrollment, if it
	// has one, expires so long after now.
	reported := func(authenticators map[string]any, expiresIn *time.Duration) func(time.Time) map[string]any {
		return func(now time.Time) map[string]any {
			enrollment := map[string]any{}
			if expiresIn != nil {
				enrollment[method] = map[string]any{"id": "enr-1", "jwt": staleToken, "expiresAt": stamp(now.Add(*expiresIn))}
			}
			return map[string]any{"authenticators": authenticators, "enrollment": enrollment}
		}
	}
	in := func(d time.Duration) *time.Duration { return &d }
	none := map[string]any{}
	certificate := map[string]any{"cert": map[string]any{"id": "auth-1", "fingerprint": "abc"}}

	// with adds fields to what reported returns.
	with := func(reported func(time.Time) map[string]any, fields map[string]any) func(time.Time) map[string]any {
		return func(now time.Time) map[string]any {
			e := reported(now)
			maps.Copy(e, fields)
			return e
		}
	}
	// policy is the default auth policy, which allows external JWTs, if at
	// all, of the supplied signers or, if there are none, of every signer.
	policy := func(allowed bool, signers []string) map[string]any {
		return map[string]any{"id": "default", "name": "Default", "primary": map[string]any{
			"extJwt": map[string]any{"allowed": allowed, "allowedSigners": signers},
		}}
	}
	signer := func(id string, enabled, useExternalID bool) map[string]any {
		return map[string]any{"id": id, "name": id, "enabled": enabled, "useExternalId": useExternalID}
	}
	// seed stores an auth policy and external JWT signers.
	seed := func(policy map[string]any, signers ...map[string]any) func(*fake.Server) {
		return func(srv *fake.Server) {
			srv.Put("auth-policies", policy)
			for _, s := range signers {
				srv.Put("external-jwt-signers", s)
			}
		}
	}
	byID := signer("by-id", true, false)

	return map[string]renewalCase{
		"ExpiredAndNotEnrolled": {
			reason:   "An identity that cannot enroll anymore gets a new token for its enrollment.",
			reported: reported(none, in(-time.Minute)),
			want:     refreshEnrollment,
		},
		"MissingAndNotEnrolled": {
			reason:   "An identity that has neither enrolled nor an enrollment gets a new enrollment.",
			reported: reported(none, nil),
			want:     createEnrollment,
		},
		"Enrolled": {
			reason:   "An identity that has enrolled has no enrollment and needs none.",
			reported: reported(certificate, nil),
		},
		"EnrolledWithAnExpiredEnrollment": {
			reason:   "An identity that has enrolled is left alone, whatever became of its enrollment.",
			reported: reported(certificate, in(-time.Hour)),
		},
		"NotExpired": {
			reason:   "A token that can still be used is not replaced.",
			reported: reported(none, in(time.Hour)),
		},
		"ExpiredAMomentAgo": {
			reason:   "A token is only replaced once it has expired for certain, the clocks being compared to within seconds.",
			reported: reported(none, in(-2*time.Second)),
		},
		"ExpiredOnTheClockOfTheController": {
			reason:   "The controller decides whether a token has expired, and the new one must expire in its future.",
			reported: reported(none, in(time.Hour)),
			ahead:    24 * time.Hour,
			want:     refreshEnrollment,
		},
		"NotExpiredOnTheClockOfTheController": {
			reason:   "A token the controller still accepts is not replaced because the local clock is ahead.",
			reported: reported(none, in(-time.Hour)),
			ahead:    -24 * time.Hour,
		},
		"MissingOnAControllerWhoseClockIsAhead": {
			reason:   "A new enrollment must expire in the future of the controller, too.",
			reported: reported(none, nil),
			ahead:    24 * time.Hour,
			want:     createEnrollment,
		},
		"ExpiredWithAnExternalID": {
			reason:   "An identity with an external ID can sign in with a JWT or a certificate that names it, for which Ziti reports no authenticator.",
			reported: with(reported(none, in(-time.Hour)), map[string]any{"externalId": "sensor-7"}),
		},
		"MissingWithAnExternalID": {
			reason:   "An identity with an external ID needs no enrollment.",
			reported: with(reported(none, nil), map[string]any{"externalId": "sensor-7"}),
		},
		"ExpiredAndSignsInWithItsID": {
			reason:   "An identity can sign in with a JWT that names its ID if its auth policy allows a signer that looks identities up by ID.",
			reported: reported(none, in(-time.Hour)),
			seed:     seed(policy(true, nil), signer("by-external-id", true, true), byID),
		},
		"MissingAndSignsInWithItsID": {
			reason:   "An identity that can sign in with a JWT that names its ID needs no enrollment.",
			reported: reported(none, nil),
			seed:     seed(policy(true, nil), byID),
		},
		"SignsInWithItsIDUnderItsOwnPolicy": {
			reason:   "What counts is the auth policy of the identity.",
			reported: with(reported(none, in(-time.Hour)), map[string]any{"authPolicyId": "sso"}),
			seed: func(srv *fake.Server) {
				seed(policy(false, nil), byID)(srv)
				srv.Put("auth-policies", map[string]any{"id": "sso", "name": "sso", "primary": map[string]any{"extJwt": map[string]any{"allowed": true}}})
			},
		},
		"SignsInWithItsIDThroughAnAllowedSigner": {
			reason:   "A policy that names signers allows those.",
			reported: reported(none, in(-time.Hour)),
			seed:     seed(policy(true, []string{"by-id"}), byID),
		},
		"SignerLooksUpExternalIDs": {
			reason:   "A signer that looks identities up by their external ID does not find an identity that has none.",
			reported: reported(none, in(-time.Hour)),
			seed:     seed(policy(true, nil), signer("by-external-id", true, true)),
			want:     refreshEnrollment,
		},
		"SignerDisabled": {
			reason:   "Tokens of a signer that is disabled are refused.",
			reported: reported(none, nil),
			seed:     seed(policy(true, nil), signer("disabled", false, false)),
			want:     createEnrollment,
		},
		"PolicyRefusesExternalJWTs": {
			reason:   "An identity whose auth policy does not allow external JWTs cannot sign in with one.",
			reported: reported(none, in(-time.Hour)),
			seed:     seed(policy(false, nil), byID),
			want:     refreshEnrollment,
		},
		"PolicyAllowsOtherSigners": {
			reason:   "An identity cannot sign in with a JWT of a signer its auth policy does not name.",
			reported: reported(none, in(-time.Hour)),
			seed:     seed(policy(true, []string{"by-external-id", "gone"}), byID, signer("by-external-id", true, true)),
			want:     refreshEnrollment,
		},
		"AuthenticatorsUnknown": {
			reason: "Without the list of its authenticators there is no telling whether an identity has enrolled.",
			reported: func(now time.Time) map[string]any {
				e := reported(none, in(-time.Hour))(now)
				delete(e, "authenticators")
				return e
			},
		},
	}
}

// identityEnrollment returns the token and the expiry of the enrollment of
// the supplied method.
func identityEnrollment(method string) func(map[string]any) (string, string) {
	return func(entity map[string]any) (string, string) {
		enrollments, _ := entity["enrollment"].(map[string]any)
		enrollment, _ := enrollments[method].(map[string]any)
		token, _ := enrollment["jwt"].(string)
		expiresAt, _ := enrollment["expiresAt"].(string)
		return token, expiresAt
	}
}

func TestIdentityRenewal(t *testing.T) {
	renewal[*v1alpha1.Identity]{
		kind: identity.Kind,
		mg: func() *v1alpha1.Identity {
			return &v1alpha1.Identity{
				ObjectMeta: meta1("web-client"),
				Spec:       v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{Name: "web-client"}},
			}
		},
		enrollment: identityEnrollment("ott"),
		status:     func(mg *v1alpha1.Identity) string { return mg.Status.AtProvider.EnrollmentExpiresAt },
		lifetime:   defaultLifetime,
		created:    map[string]any{},
	}.run(t, identityRenewalCases("ott"))
}

func TestIdentityRenewalDuration(t *testing.T) {
	renewal[*v1alpha1.Identity]{
		kind: identity.Kind,
		mg: func() *v1alpha1.Identity {
			return &v1alpha1.Identity{
				ObjectMeta: meta1("web-client"),
				Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{
					Name:               "web-client",
					EnrollmentDuration: &metav1.Duration{Duration: 48 * time.Hour},
				}},
			}
		},
		enrollment: identityEnrollment("ott"),
		status:     func(mg *v1alpha1.Identity) string { return mg.Status.AtProvider.EnrollmentExpiresAt },
		lifetime:   48 * time.Hour,
		created:    map[string]any{},
	}.run(t, identityRenewalCases("ott"))
}

func TestIdentityCARenewal(t *testing.T) {
	renewal[*v1alpha1.IdentityCA]{
		kind: identityca.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("cas", map[string]any{"id": "ca-1", "name": "devices"})
		},
		mg: func() *v1alpha1.IdentityCA {
			return &v1alpha1.IdentityCA{
				ObjectMeta: meta1("sensor"),
				Spec: v1alpha1.IdentityCASpec{ForProvider: v1alpha1.IdentityCAParameters{
					IdentityParameters: v1alpha1.IdentityParameters{Name: "sensor"},
					Ottca:              "devices",
				}},
			}
		},
		enrollment: identityEnrollment("ottca"),
		status:     func(mg *v1alpha1.IdentityCA) string { return mg.Status.AtProvider.EnrollmentExpiresAt },
		lifetime:   defaultLifetime,
		created:    map[string]any{"caId": "ca-1"},
	}.run(t, identityRenewalCases("ottca"))
}

func TestIdentityUPDBRenewal(t *testing.T) {
	renewal[*v1alpha1.IdentityUPDB]{
		kind: identityupdb.Kind,
		mg: func() *v1alpha1.IdentityUPDB {
			return &v1alpha1.IdentityUPDB{
				ObjectMeta: meta1("operator"),
				Spec: v1alpha1.IdentityUPDBSpec{ForProvider: v1alpha1.IdentityUPDBParameters{
					IdentityParameters: v1alpha1.IdentityParameters{Name: "operator"},
					UpdbUsername:       "operator@example.com",
				}},
			}
		},
		enrollment: identityEnrollment("updb"),
		status:     func(mg *v1alpha1.IdentityUPDB) string { return mg.Status.AtProvider.EnrollmentExpiresAt },
		lifetime:   defaultLifetime,
		created:    map[string]any{"username": "operator@example.com"},
	}.run(t, identityRenewalCases("updb"))
}

func TestIdentityNoneHasNoEnrollmentToRenew(t *testing.T) {
	reported := func(time.Time) map[string]any {
		return map[string]any{"authenticators": map[string]any{}, "enrollment": map[string]any{}}
	}

	renewal[*v1alpha1.IdentityNone]{
		kind: identitynone.Kind,
		mg: func() *v1alpha1.IdentityNone {
			return &v1alpha1.IdentityNone{
				ObjectMeta: meta1("sso-user"),
				Spec:       v1alpha1.IdentityNoneSpec{ForProvider: v1alpha1.IdentityParameters{Name: "sso-user"}},
			}
		},
	}.run(t, map[string]renewalCase{
		"NoEnrollment": {
			reason:   "An identity that is declared without an enrollment does not get one.",
			reported: reported,
		},
	})
}

func TestEdgeRouterRenewal(t *testing.T) {
	// An edge router whose enrollment, if it has one, expires so long after
	// now.
	reported := func(verified bool, fingerprint string, expiresIn *time.Duration) func(time.Time) map[string]any {
		return func(now time.Time) map[string]any {
			e := map[string]any{"isVerified": verified, "fingerprint": fingerprint}
			if expiresIn != nil {
				e["enrollmentJwt"] = staleToken
				e["enrollmentExpiresAt"] = stamp(now.Add(*expiresIn))
			}
			return e
		}
	}
	in := func(d time.Duration) *time.Duration { return &d }

	renewal[*v1alpha1.EdgeRouter]{
		kind: edgerouter.Kind,
		mg: func() *v1alpha1.EdgeRouter {
			return &v1alpha1.EdgeRouter{
				ObjectMeta: meta1("router-1"),
				Spec:       v1alpha1.EdgeRouterSpec{ForProvider: v1alpha1.EdgeRouterParameters{Name: "router-1"}},
			}
		},
		enrollment: func(entity map[string]any) (string, string) {
			token, _ := entity["enrollmentJwt"].(string)
			expiresAt, _ := entity["enrollmentExpiresAt"].(string)
			return token, expiresAt
		},
		status:   func(mg *v1alpha1.EdgeRouter) string { return mg.Status.AtProvider.EnrollmentExpiresAt },
		lifetime: defaultLifetime,
	}.run(t, map[string]renewalCase{
		"ExpiredAndNotEnrolled": {
			reason:   "An edge router that cannot enroll anymore is enrolled anew.",
			reported: reported(false, "", in(-time.Minute)),
			want:     reEnrollRouter,
		},
		"MissingAndNotEnrolled": {
			reason:   "An edge router that has neither enrolled nor an enrollment is enrolled anew.",
			reported: reported(false, "", nil),
			want:     reEnrollRouter,
		},
		"Enrolled": {
			reason:   "An edge router that has enrolled must keep its certificate.",
			reported: reported(true, "abc", nil),
		},
		"EnrolledWithAnExpiredEnrollment": {
			reason:   "An edge router that has enrolled is left alone, whatever became of its enrollment.",
			reported: reported(true, "abc", in(-time.Hour)),
		},
		"VerifiedWithoutACertificate": {
			reason:   "An edge router Ziti calls verified has enrolled.",
			reported: reported(true, "", in(-time.Hour)),
		},
		"NotVerifiedWithACertificate": {
			reason:   "An edge router that has a certificate has enrolled, whatever else Ziti says.",
			reported: reported(false, "abc", in(-time.Hour)),
		},
		"VerificationUnknown": {
			reason: "Without Ziti saying that an edge router is not verified there is no telling whether it has enrolled.",
			reported: func(now time.Time) map[string]any {
				e := reported(false, "", in(-time.Hour))(now)
				delete(e, "isVerified")
				return e
			},
		},
		"NotExpired": {
			reason:   "A token that can still be used is not replaced.",
			reported: reported(false, "", in(time.Hour)),
		},
		"ExpiredAMomentAgo": {
			reason:   "A token is only replaced once it has expired for certain, the clocks being compared to within seconds.",
			reported: reported(false, "", in(-2*time.Second)),
		},
		"ExpiredOnTheClockOfTheController": {
			reason:   "The controller decides whether a token has expired.",
			reported: reported(false, "", in(time.Hour)),
			ahead:    24 * time.Hour,
			want:     reEnrollRouter,
		},
		"NotExpiredOnTheClockOfTheController": {
			reason:   "A token the controller still accepts is not replaced because the local clock is ahead.",
			reported: reported(false, "", in(-time.Hour)),
			ahead:    -24 * time.Hour,
		},
	})
}

func TestRenewalFailure(t *testing.T) {
	r := renewal[*v1alpha1.Identity]{
		kind: identity.Kind,
		mg: func() *v1alpha1.Identity {
			return &v1alpha1.Identity{
				ObjectMeta: meta1("web-client"),
				Spec:       v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{Name: "web-client"}},
			}
		},
	}
	srv, e, mg := r.start(t, identityRenewalCases("ott")["ExpiredAndNotEnrolled"])
	ctx := context.Background()
	outdated := func(when string) {
		t.Helper()
		if o, err := e.Observe(ctx, mg); err != nil || o.ResourceUpToDate {
			t.Fatalf("Observe(...) %s: want an outdated resource, got %+v, %v", when, o, err)
		}
	}

	// A renewal Ziti refuses is reported, and the resource stays outdated so
	// that it is tried again.
	outdated("before the renewal")
	srv.FailNextRenewal()
	if _, err := e.Update(ctx, mg); err == nil || !strings.Contains(err.Error(), "cannot repair the external resource") {
		t.Fatalf("Update(...) while Ziti refuses the renewal: want an error that says so, got %v", err)
	}
	outdated("after the failed renewal")

	if _, err := e.Update(ctx, mg); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	o, err := e.Observe(ctx, mg)
	if err != nil || !o.ResourceUpToDate {
		t.Fatalf("Observe(...) after the renewal: want an up to date resource, got %+v, %v", o, err)
	}
	if got := string(o.ConnectionDetails[identity.ConnectionKeyEnrollmentToken]); got == "" || got == staleToken {
		t.Errorf("enrollment token in the connection details: want a new one, got %q", got)
	}
}

// TestEnrollmentDurationSchema checks what the API server accepts as the
// lifetime of a new enrollment token. Like TestControllers, it needs the
// envtest binaries.
func TestEnrollmentDurationSchema(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// CI must not pass by silently skipping the test.
		if os.Getenv("CI") != "" {
			t.Fatal("KUBEBUILDER_ASSETS is not set")
		}
		t.Skip("KUBEBUILDER_ASSETS is not set")
	}

	env := &envtest.Environment{CRDDirectoryPaths: []string{"../../package/crds"}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("cannot start the API server: %v", err)
	}
	defer func() {
		if err := env.Stop(); err != nil {
			t.Logf("cannot stop the API server: %v", err)
		}
	}()

	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot build scheme: %v", err)
	}
	k, err := kube.New(cfg, kube.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	cases := map[string]struct {
		duration time.Duration
		invalid  bool
	}{
		"ten-minutes":  {duration: 10 * time.Minute},
		"five-minutes": {duration: 5 * time.Minute},
		"one-minute":   {duration: time.Minute, invalid: true},
		"negative":     {duration: -time.Hour, invalid: true},
	}
	for name, tc := range cases {
		idn := &v1alpha1.Identity{
			ObjectMeta: meta1(name),
			Spec: v1alpha1.IdentitySpec{ForProvider: v1alpha1.IdentityParameters{
				Name:               name,
				EnrollmentDuration: &metav1.Duration{Duration: tc.duration},
			}},
		}
		if err := k.Create(context.Background(), idn); kerrors.IsInvalid(err) != tc.invalid || (err != nil && !tc.invalid) {
			t.Errorf("creating an Identity with enrollmentDuration %s: want invalid %t, got %v", tc.duration, tc.invalid, err)
		}
	}
}
