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

package connector

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
)

func newKube(t *testing.T, objects ...kube.Object) kube.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot add core/v1 to scheme: %v", err)
	}
	if err := v1alpha1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot add v1alpha1 to scheme: %v", err)
	}
	return kubefake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func service(kind, name string) *v1alpha1.Service {
	return &v1alpha1.Service{
		TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String(), Kind: v1alpha1.ServiceKind},
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a", UID: types.UID("web-uid")},
		Spec: v1alpha1.ServiceSpec{ManagedResourceSpec: xpv2.ManagedResourceSpec{
			ProviderConfigReference: &xpv2.ProviderConfigReference{Kind: kind, Name: name},
		}},
	}
}

func secret(namespace string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ziti-credentials", Namespace: namespace}, Data: map[string][]byte{}}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func credentials(namespace string) *v1alpha1.ProviderCredentials {
	return &v1alpha1.ProviderCredentials{
		Source:    xpv2.CredentialsSourceSecret,
		SecretRef: &v1alpha1.CredentialsSecretReference{Name: "ziti-credentials", Namespace: namespace},
	}
}

func TestConnect(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	inline := v1alpha1.ZitiProviderConfigSpec{Host: srv.URL, CA: srv.CA(), Username: fake.Username, Password: fake.Password}

	cases := map[string]struct {
		objects []kube.Object
		ref     xpv2.ProviderConfigReference
		wantErr string
	}{
		"InlineCredentials": {
			objects: []kube.Object{&v1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
				Spec:       v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: inline},
			}},
			ref: xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"},
		},
		"SecretKeys": {
			objects: []kube.Object{
				&v1alpha1.ProviderConfig{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
					Spec: v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{
						Host: srv.URL, CA: srv.CA(), Credentials: credentials(""),
					}},
				},
				secret("team-a", map[string]string{keyUsername: fake.Username, keyPassword: fake.Password}),
			},
			ref: xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"},
		},
		"SecretJSON": {
			objects: []kube.Object{
				&v1alpha1.ProviderConfig{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
					Spec: v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{
						CA: srv.CA(), Credentials: credentials(""),
					}},
				},
				secret("team-a", map[string]string{
					keyCredentials: `{"host": "` + srv.URL + `", "username": "` + fake.Username + `", "password": "` + fake.Password + `"}`,
				}),
			},
			ref: xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"},
		},
		"ProviderConfigCannotReadOtherNamespaces": {
			objects: []kube.Object{
				&v1alpha1.ProviderConfig{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
					Spec: v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{
						Host: srv.URL, CA: srv.CA(), Credentials: credentials("team-b"),
					}},
				},
				secret("team-b", map[string]string{keyUsername: fake.Username, keyPassword: fake.Password}),
			},
			ref:     xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"},
			wantErr: errGetSecret,
		},
		"ClusterProviderConfig": {
			objects: []kube.Object{
				&v1alpha1.ClusterProviderConfig{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
					Spec: v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{
						Host: srv.URL, CA: srv.CA(), Credentials: credentials("ziti-system"),
					}},
				},
				secret("ziti-system", map[string]string{keyUsername: fake.Username, keyPassword: fake.Password}),
			},
			ref: xpv2.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: "default"},
		},
		"ClusterProviderConfigNeedsSecretNamespace": {
			objects: []kube.Object{&v1alpha1.ClusterProviderConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
				Spec: v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{
					Host: srv.URL, CA: srv.CA(), Credentials: credentials(""),
				}},
			}},
			ref:     xpv2.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: "default"},
			wantErr: errSecretNamespace,
		},
		"MissingProviderConfig": {
			ref:     xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"},
			wantErr: errGetPC,
		},
		"UnsupportedKind": {
			ref:     xpv2.ProviderConfigReference{Kind: "Other", Name: "default"},
			wantErr: `unsupported provider config kind "Other"`,
		},
		"UnsupportedSource": {
			objects: []kube.Object{&v1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
				Spec: v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{
					Host: srv.URL, Credentials: &v1alpha1.ProviderCredentials{Source: xpv2.CredentialsSourceEnvironment},
				}},
			}},
			ref:     xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"},
			wantErr: `unsupported credentials source "Environment"`,
		},
		"MissingCredentials": {
			objects: []kube.Object{&v1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
				Spec:       v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{Host: srv.URL}},
			}},
			ref:     xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"},
			wantErr: errNewClient,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			k := newKube(t, tc.objects...)

			zc, err := New(k).Connect(ctx, service(tc.ref.Kind, tc.ref.Name))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Connect(...): want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Connect(...): %v", err)
			}

			// The client must be able to authenticate with the resolved settings.
			if _, err := zc.Find(ctx, "services", `name="web"`); err != nil {
				t.Errorf("Find(...): %v", err)
			}

			usage := &v1alpha1.ProviderConfigUsage{}
			if err := k.Get(ctx, types.NamespacedName{Namespace: "team-a", Name: "web-uid"}, usage); err != nil {
				t.Fatalf("cannot get ProviderConfigUsage: %v", err)
			}
			if got := usage.GetProviderConfigReference(); got != tc.ref {
				t.Errorf("ProviderConfigUsage: want reference %+v, got %+v", tc.ref, got)
			}
		})
	}
}

func TestConnectReusesClients(t *testing.T) {
	srv := fake.NewServer()
	defer srv.Close()

	ctx := context.Background()
	pc := &v1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"},
		Spec: v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{
			Host: srv.URL, CA: srv.CA(), Credentials: credentials(""),
		}},
	}
	creds := secret("team-a", map[string]string{keyUsername: fake.Username, keyPassword: fake.Password})
	k := newKube(t, pc, creds)
	c := New(k)

	first, err := c.Connect(ctx, service("ProviderConfig", "default"))
	if err != nil {
		t.Fatalf("Connect(...): %v", err)
	}
	second, err := c.Connect(ctx, service("ProviderConfig", "default"))
	if err != nil {
		t.Fatalf("Connect(...): %v", err)
	}
	if first != second {
		t.Errorf("Connect(...): want the client to be reused while the settings are unchanged")
	}

	// Rotated credentials must not be served from the cache.
	creds.Data[keyPassword] = []byte("rotated")
	if err := k.Update(ctx, creds); err != nil {
		t.Fatalf("cannot update secret: %v", err)
	}
	third, err := c.Connect(ctx, service("ProviderConfig", "default"))
	if err != nil {
		t.Fatalf("Connect(...): %v", err)
	}
	if third == first {
		t.Errorf("Connect(...): want a new client after the credentials changed")
	}
}
