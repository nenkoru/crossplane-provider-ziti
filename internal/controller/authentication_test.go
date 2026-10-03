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
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis"
	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller/authpolicy"
	"github.com/crossplane/provider-ziti/internal/controller/certificateauthority"
	"github.com/crossplane/provider-ziti/internal/controller/externaljwtsigner"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
)

// certificate stands in for a PEM encoded certificate. The fake controller
// does not parse it.
const certificate = "-----BEGIN CERTIFICATE-----\nMIIBoDCCAUWgAwIBAgIU\n-----END CERTIFICATE-----\n"

func TestCertificateAuthority(t *testing.T) {
	// The controller adds what only it knows, and keeps it and the
	// certificate when the certificate authority is replaced.
	ca := func(fields map[string]any) map[string]any {
		fields["name"] = "device-ca"
		fields["certPem"] = certificate
		fields["fingerprint"] = "fingerprint-of-device-ca"
		fields["isVerified"] = false
		fields["verificationToken"] = "token-for-device-ca"
		return fields
	}

	lifecycle[*v1alpha1.CertificateAuthority]{
		kind: certificateauthority.Kind,
		mg: &v1alpha1.CertificateAuthority{
			ObjectMeta: meta1("device-ca"),
			Spec: v1alpha1.CertificateAuthoritySpec{ForProvider: v1alpha1.CertificateAuthorityParameters{
				Name:                     "device-ca",
				CertPem:                  certificate,
				IsOttCaEnrollmentEnabled: true,
				ExternalIDClaim:          &v1alpha1.ExternalIDClaim{Location: "SAN_URI", Matcher: "SCHEME", MatcherCriteria: "spiffe"},
			}},
		},
		created: ca(map[string]any{
			"isAuthEnabled":             false,
			"isOttCaEnrollmentEnabled":  true,
			"isAutoCaEnrollmentEnabled": false,
			"identityRoles":             []any{},
			"identityNameFormat":        "[caName]-[commonName]",
			"externalIdClaim": map[string]any{
				"location": "SAN_URI", "matcher": "SCHEME", "matcherCriteria": "spiffe",
				"parser": "NONE", "parserCriteria": "", "index": float64(0),
			},
			"tags": map[string]any{},
		}),
		update: func(mg *v1alpha1.CertificateAuthority) {
			p := &mg.Spec.ForProvider
			p.IsAuthEnabled = true
			p.IsAutoCaEnrollmentEnabled = true
			p.IdentityRoles = []string{"devices"}
			p.IdentityNameFormat = ptr.To("[commonName]")
			p.ExternalIDClaim = nil
			p.Tags = map[string]string{"team": "iot"}
		},
		updated: ca(map[string]any{
			"isAuthEnabled":             true,
			"isOttCaEnrollmentEnabled":  true,
			"isAutoCaEnrollmentEnabled": true,
			"identityRoles":             []any{"devices"},
			"identityNameFormat":        "[commonName]",
			"externalIdClaim":           nil,
			"tags":                      map[string]any{"team": "iot"},
		}),
	}.run(t)
}

func TestCertificateAuthorityIsReadyBeforeItIsVerified(t *testing.T) {
	// The provider cannot verify a certificate authority: that takes its
	// private key. It reports what its owner needs to do so.
	for name, verified := range map[string]bool{"Unverified": false, "Verified": true} {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()
			srv.Put("cas", map[string]any{
				"id": "ca-1", "name": "device-ca", "certPem": certificate, "fingerprint": "0a1b2c",
				"isVerified": verified, "verificationToken": "token-1", "isOttCaEnrollmentEnabled": true,
				"identityNameFormat": "[caName]-[commonName]",
			})

			api, err := srv.Client()
			if err != nil {
				t.Fatalf("cannot create client: %v", err)
			}

			mg := &v1alpha1.CertificateAuthority{ObjectMeta: meta1("device-ca")}
			mg.Spec.ForProvider = v1alpha1.CertificateAuthorityParameters{Name: "device-ca", CertPem: certificate, IsOttCaEnrollmentEnabled: true}
			meta.SetExternalName(mg, "ca-1")

			o, err := generic.NewExternalClient(certificateauthority.Kind, api).Observe(context.Background(), mg)
			if err != nil || !o.ResourceExists || !o.ResourceUpToDate {
				t.Fatalf("Observe(...): want an up to date resource, got %+v, %v", o, err)
			}
			if got := mg.GetCondition(xpv2.TypeReady).Status; got != corev1.ConditionTrue {
				t.Errorf("Ready condition: want True, got %q", got)
			}
			want := v1alpha1.CertificateAuthorityObservation{
				ID: "ca-1", Name: "device-ca", Fingerprint: "0a1b2c", IsVerified: verified, VerificationToken: "token-1",
				IsOttCaEnrollmentEnabled: true, IdentityNameFormat: "[caName]-[commonName]",
			}
			if diff := cmp.Diff(want, mg.Status.AtProvider); diff != "" {
				t.Errorf("status.atProvider: -want, +got:\n%s", diff)
			}
		})
	}
}

// signerBody is the Ziti external JWT signer expected for the supplied
// fields: a setting the spec leaves out is sent as null, so that it is
// removed, and the controller fills in its defaults.
func signerBody(fields map[string]any) map[string]any {
	body := map[string]any{
		"enabled":                       false,
		"jwksEndpoint":                  nil,
		"certPem":                       nil,
		"kid":                           nil,
		"claimsProperty":                "/sub",
		"useExternalId":                 false,
		"externalAuthUrl":               nil,
		"clientId":                      nil,
		"scopes":                        []any{},
		"targetToken":                   "ACCESS",
		"enrollToCertEnabled":           false,
		"enrollToTokenEnabled":          false,
		"enrollNameClaimsSelector":      "/sub",
		"enrollAttributeClaimsSelector": "",
		"enrollAuthPolicyId":            "default",
	}
	for k, v := range fields {
		body[k] = v
	}
	return body
}

func TestExternalJWTSigner(t *testing.T) {
	lifecycle[*v1alpha1.ExternalJWTSigner]{
		kind: externaljwtsigner.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("auth-policies", map[string]any{"id": "ap-1", "name": "sso-only"})
		},
		mg: &v1alpha1.ExternalJWTSigner{
			ObjectMeta: meta1("corporate-sso"),
			Spec: v1alpha1.ExternalJWTSignerSpec{ForProvider: v1alpha1.ExternalJWTSignerParameters{
				Name:         "corporate-sso",
				Issuer:       "https://sso.example.com",
				Audience:     "ziti",
				Enabled:      true,
				JwksEndpoint: ptr.To("https://sso.example.com/jwks"),
				ClientID:     ptr.To("ziti-clients"),
				Scopes:       []string{"email"},
			}},
		},
		created: signerBody(map[string]any{
			"name":         "corporate-sso",
			"issuer":       "https://sso.example.com",
			"audience":     "ziti",
			"enabled":      true,
			"jwksEndpoint": "https://sso.example.com/jwks",
			"clientId":     "ziti-clients",
			"scopes":       []any{"email"},
		}),
		// The signer changes from a JWKS endpoint to a certificate and loses
		// its client ID, which only a replacement removes.
		update: func(mg *v1alpha1.ExternalJWTSigner) {
			p := &mg.Spec.ForProvider
			p.Audience = "ziti-edge"
			p.JwksEndpoint = nil
			p.CertPem = ptr.To(certificate)
			p.Kid = ptr.To("key-1")
			p.ClaimsProperty = ptr.To("email")
			p.UseExternalID = true
			p.ClientID = nil
			p.Scopes = nil
			p.TargetToken = ptr.To("ID")
			p.EnrollToCertEnabled = true
			p.EnrollNameClaimsSelector = ptr.To("preferred_username")
			p.EnrollAttributeClaimsSelector = ptr.To("groups")
			p.EnrollAuthPolicyID = ptr.To("sso-only")
			p.Tags = map[string]string{"team": "sso"}
		},
		updated: signerBody(map[string]any{
			"name":                          "corporate-sso",
			"issuer":                        "https://sso.example.com",
			"audience":                      "ziti-edge",
			"enabled":                       true,
			"certPem":                       certificate,
			"kid":                           "key-1",
			"claimsProperty":                "email",
			"useExternalId":                 true,
			"targetToken":                   "ID",
			"enrollToCertEnabled":           true,
			"enrollNameClaimsSelector":      "preferred_username",
			"enrollAttributeClaimsSelector": "groups",
			"enrollAuthPolicyId":            "ap-1",
			"tags":                          map[string]any{"team": "sso"},
		}),
	}.run(t)
}

func TestExternalJWTSignerGetsItsTagsAfterCreation(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	api, err := srv.Client()
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}

	ctx := context.Background()
	e := generic.NewExternalClient(externaljwtsigner.Kind, api)
	mg := &v1alpha1.ExternalJWTSigner{
		ObjectMeta: meta1("corporate-sso"),
		Spec: v1alpha1.ExternalJWTSignerSpec{ForProvider: v1alpha1.ExternalJWTSignerParameters{
			Name: "corporate-sso", Issuer: "https://sso.example.com", Audience: "ziti",
			JwksEndpoint: ptr.To("https://sso.example.com/jwks"),
			Tags:         map[string]string{"team": "sso"},
		}},
	}
	if _, err := e.Create(ctx, mg); err != nil {
		t.Fatalf("Create(...): %v", err)
	}

	// Ziti ignores the tags of a signer that is being created.
	o, err := e.Observe(ctx, mg)
	if err != nil || o.ResourceUpToDate || !strings.Contains(o.Diff, `field "tags" differs`) {
		t.Fatalf("Observe(...) after creation: want the tags to be missing, got %+v, %v", o, err)
	}
	if _, err := e.Update(ctx, mg); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	o, err = e.Observe(ctx, mg)
	if err != nil || !o.ResourceUpToDate {
		t.Fatalf("Observe(...) after the update: want an up to date resource, got %+v, %v", o, err)
	}
	if diff := cmp.Diff(map[string]string{"team": "sso"}, mg.Status.AtProvider.Tags); diff != "" {
		t.Errorf("status.atProvider.tags: -want, +got:\n%s", diff)
	}
}

func TestAuthPolicyRefersToSignersByName(t *testing.T) {
	policy := func(extJWT, secondary map[string]any) map[string]any {
		return map[string]any{
			"name": "sso",
			"primary": map[string]any{
				"cert": map[string]any{"allowed": false, "allowExpiredCerts": false},
				"updb": map[string]any{
					"allowed": false, "minPasswordLength": float64(5), "maxAttempts": float64(5), "lockoutDurationMinutes": float64(0),
					"requireMixedCase": false, "requireNumberChar": false, "requireSpecialChar": false,
				},
				"extJwt": extJWT,
			},
			"secondary": secondary,
			"tags":      map[string]any{},
		}
	}

	lifecycle[*v1alpha1.AuthPolicy]{
		kind: authpolicy.Kind,
		seed: func(srv *fake.Server) {
			srv.Put("external-jwt-signers", map[string]any{"id": "sig-2", "name": "corporate-sso"})
			srv.Put("external-jwt-signers", map[string]any{"id": "sig-1", "name": "partner-sso"})
		},
		mg: &v1alpha1.AuthPolicy{
			ObjectMeta: meta1("sso"),
			Spec: v1alpha1.AuthPolicySpec{ForProvider: v1alpha1.AuthPolicyParameters{
				Name: "sso",
				Primary: &v1alpha1.AuthMethods{ExtJWT: &v1alpha1.ExtJWTAuth{
					Allowed: true,
					// A name and an ID, in the order opposite to the one Ziti
					// reports them in.
					AllowedSigners: []string{"corporate-sso", "sig-1"},
				}},
				Secondary: &v1alpha1.SecondaryAuth{RequireExtJWTSigner: ptr.To("partner-sso")},
			}},
		},
		created: policy(
			map[string]any{"allowed": true, "allowedSigners": []any{"sig-1", "sig-2"}},
			map[string]any{"requireTotp": false, "requireExtJwtSigner": "sig-1"},
		),
		update: func(mg *v1alpha1.AuthPolicy) {
			mg.Spec.ForProvider.Primary.ExtJWT.AllowedSigners = []string{"corporate-sso"}
			mg.Spec.ForProvider.Secondary = nil
		},
		updated: policy(
			map[string]any{"allowed": true, "allowedSigners": []any{"sig-2"}},
			map[string]any{"requireTotp": false, "requireExtJwtSigner": nil},
		),
	}.run(t)
}

func TestAuthPolicyWaitsForItsSigners(t *testing.T) {
	cases := map[string]v1alpha1.AuthPolicyParameters{
		"AllowedSigner": {
			Name:    "sso",
			Primary: &v1alpha1.AuthMethods{ExtJWT: &v1alpha1.ExtJWTAuth{Allowed: true, AllowedSigners: []string{"corporate-sso"}}},
		},
		"RequiredSigner": {
			Name:      "sso",
			Secondary: &v1alpha1.SecondaryAuth{RequireExtJWTSigner: ptr.To("corporate-sso")},
		},
	}

	for name, parameters := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fake.NewServer()
			defer srv.Close()

			api, err := srv.Client()
			if err != nil {
				t.Fatalf("cannot create client: %v", err)
			}

			mg := &v1alpha1.AuthPolicy{ObjectMeta: meta1("sso"), Spec: v1alpha1.AuthPolicySpec{ForProvider: parameters}}

			_, err = generic.NewExternalClient(authpolicy.Kind, api).Create(context.Background(), mg)
			if err == nil || !strings.Contains(err.Error(), `no entity named "corporate-sso" in external-jwt-signers`) {
				t.Errorf("Create(...): want an error about the missing signer, got %v", err)
			}
			if got := srv.Len("auth-policies"); got != 0 {
				t.Errorf("want no auth policy to be created, got %d", got)
			}
		})
	}
}

// TestAuthenticationSchemas checks what the API server makes of the two
// kinds: the defaults it fills in and the specs it rejects. Like
// TestControllers it needs the envtest binaries.
func TestAuthenticationSchemas(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// CI must not pass by silently skipping the test.
		if os.Getenv("CI") != "" {
			t.Fatal("KUBEBUILDER_ASSETS is not set")
		}
		t.Skip("KUBEBUILDER_ASSETS is not set")
	}

	ctx := context.Background()
	k := startAPIServer(t)

	newCA := func(name string, claim *v1alpha1.ExternalIDClaim) *v1alpha1.CertificateAuthority {
		return &v1alpha1.CertificateAuthority{
			ObjectMeta: meta1(name),
			Spec: v1alpha1.CertificateAuthoritySpec{ForProvider: v1alpha1.CertificateAuthorityParameters{
				Name: name, CertPem: certificate, ExternalIDClaim: claim,
			}},
		}
	}

	ca := newCA("device-ca", &v1alpha1.ExternalIDClaim{})
	create(ctx, t, k, ca)
	want := v1alpha1.CertificateAuthorityParameters{
		Name:               "device-ca",
		CertPem:            certificate,
		IdentityNameFormat: ptr.To("[caName]-[commonName]"),
		ExternalIDClaim:    &v1alpha1.ExternalIDClaim{Location: "COMMON_NAME", Matcher: "ALL", Parser: "NONE"},
	}
	if diff := cmp.Diff(want, ca.Spec.ForProvider); diff != "" {
		t.Errorf("defaults of a certificate authority: -want, +got:\n%s", diff)
	}

	ca.Spec.ForProvider.CertPem = strings.Replace(certificate, "MIIB", "MIIC", 1)
	if err := k.Update(ctx, ca); !kerrors.IsInvalid(err) {
		t.Errorf("changing the certificate of a certificate authority: want it to be rejected as invalid, got %v", err)
	}

	claims := map[string]struct {
		claim v1alpha1.ExternalIDClaim
		valid bool
	}{
		"scheme-of-uri":       {claim: v1alpha1.ExternalIDClaim{Location: "SAN_URI", Matcher: "SCHEME", MatcherCriteria: "spiffe"}, valid: true},
		"suffix-and-split":    {claim: v1alpha1.ExternalIDClaim{Location: "SAN_EMAIL", Matcher: "SUFFIX", MatcherCriteria: "@example.com", Parser: "SPLIT", ParserCriteria: "@"}, valid: true},
		"scheme-of-name":      {claim: v1alpha1.ExternalIDClaim{Matcher: "SCHEME", MatcherCriteria: "spiffe"}},
		"prefix-of-uri":       {claim: v1alpha1.ExternalIDClaim{Location: "SAN_URI", Matcher: "PREFIX", MatcherCriteria: "spiffe"}},
		"prefix-without":      {claim: v1alpha1.ExternalIDClaim{Matcher: "PREFIX"}},
		"split-without":       {claim: v1alpha1.ExternalIDClaim{Parser: "SPLIT"}},
		"negative-index":      {claim: v1alpha1.ExternalIDClaim{Index: -1}},
		"unknown-location":    {claim: v1alpha1.ExternalIDClaim{Location: "SERIAL_NUMBER"}},
		"criteria-of-all":     {claim: v1alpha1.ExternalIDClaim{MatcherCriteria: "ignored"}, valid: true},
		"index-of-split-name": {claim: v1alpha1.ExternalIDClaim{Parser: "SPLIT", ParserCriteria: ".", Index: 1}, valid: true},
	}
	for name, tc := range claims {
		if err := k.Create(ctx, newCA(name, &tc.claim)); (err == nil) != tc.valid || (err != nil && !kerrors.IsInvalid(err)) {
			t.Errorf("creating a certificate authority with the external ID claim %+v: want valid %t, got %v", tc.claim, tc.valid, err)
		}
	}

	newSigner := func(name string, jwksEndpoint, certPem *string) *v1alpha1.ExternalJWTSigner {
		return &v1alpha1.ExternalJWTSigner{
			ObjectMeta: meta1(name),
			Spec: v1alpha1.ExternalJWTSignerSpec{ForProvider: v1alpha1.ExternalJWTSignerParameters{
				Name: name, Issuer: "https://" + name + ".example.com", Audience: "ziti", JwksEndpoint: jwksEndpoint, CertPem: certPem,
			}},
		}
	}

	signer := newSigner("corporate-sso", ptr.To("https://sso.example.com/jwks"), nil)
	create(ctx, t, k, signer, newSigner("static-key", nil, ptr.To(certificate)))
	if got := ptr.Deref(signer.Spec.ForProvider.TargetToken, ""); got != "ACCESS" {
		t.Errorf("default target token of a signer: want ACCESS, got %q", got)
	}

	// A signer may change from a JWKS endpoint to a certificate.
	signer.Spec.ForProvider.JwksEndpoint, signer.Spec.ForProvider.CertPem = nil, ptr.To(certificate)
	if err := k.Update(ctx, signer); err != nil {
		t.Errorf("changing a signer from a JWKS endpoint to a certificate: %v", err)
	}

	for name, signer := range map[string]*v1alpha1.ExternalJWTSigner{
		"no-keys":       newSigner("no-keys", nil, nil),
		"two-keys":      newSigner("two-keys", ptr.To("https://sso.example.com/jwks"), ptr.To(certificate)),
		"not-a-web-url": newSigner("not-a-web-url", ptr.To("file:///etc/jwks.json"), nil),
	} {
		if err := k.Create(ctx, signer); !kerrors.IsInvalid(err) {
			t.Errorf("creating the signer %s: want it to be rejected as invalid, got %v", name, err)
		}
	}

	// Ziti takes the scheme of a JWKS endpoint in any case and wants a host.
	endpoints := []struct {
		url   string
		valid bool
	}{
		{url: "HTTPS://sso.example.com/jwks", valid: true},
		{url: "Http://sso.example.com:8080/jwks?a=b", valid: true},
		{url: "https://sso.example.com", valid: true},
		{url: "https://[2001:db8::1]:8443/jwks", valid: true},
		{url: "https://reader@sso.example.com/jwks", valid: true},
		{url: "http:///jwks"},
		{url: "https://:8443/jwks"},
		{url: "https://client@/jwks"},
		{url: "https:/sso.example.com/jwks"},
		{url: "https://sso.example.com:https/jwks"},
		{url: "ftp://sso.example.com/jwks"},
	}
	for i, tc := range endpoints {
		err := k.Create(ctx, newSigner("endpoint-"+strconv.Itoa(i), ptr.To(tc.url), nil))
		if (err == nil) != tc.valid || (err != nil && !kerrors.IsInvalid(err)) {
			t.Errorf("creating a signer with the JWKS endpoint %q: want valid %t, got %v", tc.url, tc.valid, err)
		}
	}
}

// startAPIServer starts a Kubernetes API server with the provider's CRDs and
// no controllers.
func startAPIServer(t *testing.T) kube.Client {
	t.Helper()

	env := &envtest.Environment{CRDDirectoryPaths: []string{"../../package/crds"}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("cannot start the API server: %v", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Logf("cannot stop the API server: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot build scheme: %v", err)
	}
	k, err := kube.New(cfg, kube.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}
	return k
}
