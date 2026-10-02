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

package client_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/client/fake"
)

func newClient(t *testing.T, srv *fake.Server) *client.Client {
	t.Helper()

	c, err := client.New(client.Config{Host: srv.URL, Username: fake.Username, Password: fake.Password, CA: srv.CA()})
	if err != nil {
		t.Fatalf("New(...): %v", err)
	}
	return c
}

func TestNew(t *testing.T) {
	cases := map[string]struct {
		cfg     client.Config
		wantErr string
	}{
		"Valid": {
			cfg: client.Config{Host: "https://ziti.example.com:1280", Username: "u", Password: "p"},
		},
		"ManagementPathIsAccepted": {
			cfg: client.Config{Host: "https://ziti.example.com:1280/edge/management/v1/", Username: "u", Password: "p"},
		},
		"MissingScheme": {
			cfg:     client.Config{Host: "ziti.example.com:1280", Username: "u", Password: "p"},
			wantErr: "host must be an https URL",
		},
		"PlainHTTP": {
			cfg:     client.Config{Host: "http://ziti.example.com:1280", Username: "u", Password: "p"},
			wantErr: "host must be an https URL",
		},
		"MissingCredentials": {
			cfg:     client.Config{Host: "https://ziti.example.com:1280", Username: "u"},
			wantErr: "either username and password or cert and key are required",
		},
		"InvalidCA": {
			cfg:     client.Config{Host: "https://ziti.example.com:1280", Username: "u", Password: "p", CA: "nope"},
			wantErr: "ca does not contain a PEM-encoded certificate",
		},
		"InvalidKeyPair": {
			cfg:     client.Config{Host: "https://ziti.example.com:1280", Cert: "nope", Key: "nope"},
			wantErr: "cannot parse client certificate and key",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.New(tc.cfg)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("New(...): unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("New(...): want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestLifecycle(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	ctx := context.Background()
	c := newClient(t, srv)

	id, err := c.Create(ctx, "services", map[string]any{"name": "web", "encryptionRequired": true})
	if err != nil {
		t.Fatalf("Create(...): %v", err)
	}

	data, err := c.Get(ctx, "services", id)
	if err != nil {
		t.Fatalf("Get(...): %v", err)
	}
	got := map[string]any{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cannot parse entity: %v", err)
	}
	created := got["createdAt"]
	want := map[string]any{"id": id, "name": "web", "encryptionRequired": true, "createdAt": created}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Get(...): -want, +got:\n%s", diff)
	}

	if err := c.Patch(ctx, "services", id, map[string]any{"terminatorStrategy": "weighted"}); err != nil {
		t.Fatalf("Patch(...): %v", err)
	}
	patched := map[string]any{"id": id, "name": "web", "encryptionRequired": true, "terminatorStrategy": "weighted", "createdAt": created}
	if diff := cmp.Diff(patched, srv.Entity("services", id)); diff != "" {
		t.Errorf("entity after Patch(...): -want, +got:\n%s", diff)
	}

	// Put replaces the entity: what is left out is gone.
	if err := c.Put(ctx, "services", id, map[string]any{"name": "web", "encryptionRequired": false}); err != nil {
		t.Fatalf("Put(...): %v", err)
	}
	replaced := map[string]any{"id": id, "name": "web", "encryptionRequired": false, "createdAt": created}
	if diff := cmp.Diff(replaced, srv.Entity("services", id)); diff != "" {
		t.Errorf("entity after Put(...): -want, +got:\n%s", diff)
	}

	if err := c.Delete(ctx, "services", id); err != nil {
		t.Fatalf("Delete(...): %v", err)
	}
	if _, err := c.Get(ctx, "services", id); !client.IsNotFound(err) {
		t.Errorf("Get(...) after delete: want not found, got %v", err)
	}
	if err := c.Delete(ctx, "services", id); !client.IsNotFound(err) {
		t.Errorf("Delete(...) of a missing entity: want not found, got %v", err)
	}
}

func TestErrorIncludesCause(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	ctx := context.Background()
	c := newClient(t, srv)

	if _, err := c.Create(ctx, "services", map[string]any{"name": "web"}); err != nil {
		t.Fatalf("Create(...): %v", err)
	}

	_, err := c.Create(ctx, "services", map[string]any{"name": "web"})
	want := `ziti API error 400 COULD_NOT_VALIDATE: fake COULD_NOT_VALIDATE (cause: {"field":"name","reason":"duplicate value"})`
	if err == nil || err.Error() != want {
		t.Errorf("Create(...) of a duplicate:\nwant error %s\ngot        %v", want, err)
	}
	if client.IsNotFound(err) {
		t.Errorf("IsNotFound(%v): want false", err)
	}
}

func TestResolveID(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	srv.Put("configs", map[string]any{"id": "abc", "name": `my "quoted" config`})

	ctx := context.Background()
	c := newClient(t, srv)

	cases := map[string]struct {
		nameOrID string
		want     string
		wantErr  string
	}{
		"ByName":  {nameOrID: `my "quoted" config`, want: "abc"},
		"ByID":    {nameOrID: "abc", want: "abc"},
		"Missing": {nameOrID: "nope", wantErr: `no entity named "nope" in configs`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := c.ResolveID(ctx, "configs", tc.nameOrID)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("ResolveID(...): want error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ResolveID(...): want %q, got %q, %v", tc.want, got, err)
			}
		})
	}
}

func TestSessionIsReusedAndRenewed(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	ctx := context.Background()
	c := newClient(t, srv)

	for range 3 {
		if _, err := c.Find(ctx, "services", `name="x"`); err != nil {
			t.Fatalf("Find(...): %v", err)
		}
	}
	if got := srv.Logins(); got != 1 {
		t.Errorf("want 1 login for 3 requests, got %d", got)
	}

	srv.ExpireSessions()
	if _, err := c.Find(ctx, "services", `name="x"`); err != nil {
		t.Fatalf("Find(...) with an expired session: %v", err)
	}
	if got := srv.Logins(); got != 2 {
		t.Errorf("want 2 logins after the session expired, got %d", got)
	}
}

func TestClockAhead(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	ctx := context.Background()
	c := newClient(t, srv)

	if _, known := c.ClockAhead(); known {
		t.Errorf("ClockAhead() before any response: want the offset to be unknown")
	}

	for _, want := range []time.Duration{time.Hour, -time.Hour, 0} {
		srv.SetClockAhead(want)
		if _, err := c.Find(ctx, "services", `name="x"`); err != nil {
			t.Fatalf("Find(...): %v", err)
		}

		// The controller tells the time to the second.
		got, known := c.ClockAhead()
		if diff := got - want; !known || diff < -2*time.Second || diff > 2*time.Second {
			t.Errorf("ClockAhead() with a clock %s ahead: got %s, known %t", want, got, known)
		}
	}
}

func TestIsRejected(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	ctx := context.Background()
	c := newClient(t, srv)

	if _, err := c.Create(ctx, "services", map[string]any{"name": "web"}); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	_, err := c.Create(ctx, "services", map[string]any{"name": "web"})
	if !client.IsRejected(err) {
		t.Errorf("IsRejected(%v): want true for a request Ziti refused", err)
	}

	srv.LoseNextCreateResponse()
	_, err = c.Create(ctx, "services", map[string]any{"name": "api"})
	if err == nil || client.IsRejected(err) {
		t.Errorf("IsRejected(%v): want an error that leaves the outcome open", err)
	}
	if got := srv.Len("services"); got != 2 {
		t.Errorf("want Ziti to have carried out the request it did not answer, got %d services", got)
	}
}

func TestAuthenticationFailure(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	c, err := client.New(client.Config{Host: srv.URL, Username: fake.Username, Password: "wrong-password", CA: srv.CA()})
	if err != nil {
		t.Fatalf("New(...): %v", err)
	}

	_, err = c.Get(context.Background(), "services", "abc")
	want := "cannot authenticate to the Ziti controller: 401 INVALID_AUTH"
	if err == nil || err.Error() != want {
		t.Fatalf("Get(...): want error %q, got %v", want, err)
	}
}

func TestServerCertificateIsVerified(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	ctx := context.Background()

	untrusted, err := client.New(client.Config{Host: srv.URL, Username: fake.Username, Password: fake.Password})
	if err != nil {
		t.Fatalf("New(...): %v", err)
	}
	if _, err := untrusted.Get(ctx, "services", "abc"); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("Get(...) without the CA: want a certificate error, got %v", err)
	}

	insecure, err := client.New(client.Config{Host: srv.URL, Username: fake.Username, Password: fake.Password, InsecureSkipTLSVerify: true})
	if err != nil {
		t.Fatalf("New(...): %v", err)
	}
	if _, err := insecure.Get(ctx, "services", "abc"); !client.IsNotFound(err) {
		t.Errorf("Get(...) with verification disabled: want not found, got %v", err)
	}
}

func TestCertificateAuthentication(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	cert, key := selfSignedPair(t)
	c, err := client.New(client.Config{Host: srv.URL, Cert: cert, Key: key, CA: srv.CA()})
	if err != nil {
		t.Fatalf("New(...): %v", err)
	}

	if _, err := c.Get(context.Background(), "services", "abc"); !client.IsNotFound(err) {
		t.Errorf("Get(...): want not found, got %v", err)
	}
	if got := srv.Logins(); got != 1 {
		t.Errorf("want 1 login, got %d", got)
	}
}

func selfSignedPair(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("cannot generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "provider-ziti-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cannot create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("cannot encode key: %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}
