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
	"strings"
	"testing"
	"time"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/gate"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/customresourcesgate"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis"
	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client/fake"
	"github.com/crossplane/provider-ziti/internal/controller"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/identity"
)

const (
	namespace = "default"
	wait      = 60 * time.Second
	tick      = 200 * time.Millisecond
)

// TestControllers runs the controllers as the provider binary does against a
// real Kubernetes API server and a fake Ziti controller. It needs the envtest
// binaries, which "make test" provides, and is skipped without them:
//
//	KUBEBUILDER_ASSETS="$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.23 use -p path)" go test ./internal/controller/...
func TestControllers(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// CI must not pass by silently skipping the test.
		if os.Getenv("CI") != "" {
			t.Fatal("KUBEBUILDER_ASSETS is not set")
		}
		t.Skip("KUBEBUILDER_ASSETS is not set")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := fake.NewServer()
	defer srv.Close()

	k := startProvider(ctx, t)

	create(ctx, t, k,
		&corev1.Secret{
			ObjectMeta: meta1("ziti-credentials"),
			StringData: map[string]string{"username": fake.Username, "password": fake.Password},
		},
		&v1alpha1.ProviderConfig{
			ObjectMeta: meta1("default"),
			Spec: v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{
				Host: srv.URL,
				CA:   srv.CA(),
				Credentials: &v1alpha1.ProviderCredentials{
					Source:    xpv2.CredentialsSourceSecret,
					SecretRef: &v1alpha1.CredentialsSecretReference{Name: "ziti-credentials"},
				},
			}},
		},
	)

	// Credentials are only ever sent over TLS.
	plain := &v1alpha1.ProviderConfig{
		ObjectMeta: meta1("plain-http"),
		Spec:       v1alpha1.ProviderConfigSpec{ZitiProviderConfigSpec: v1alpha1.ZitiProviderConfigSpec{Host: "http://ziti.example.com:1280"}},
	}
	if err := k.Create(ctx, plain); !kerrors.IsInvalid(err) {
		t.Errorf("creating a ProviderConfig with an http host: want it to be rejected as invalid, got %v", err)
	}

	pc := xpv2.ManagedResourceSpec{ProviderConfigReference: &xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "default"}}
	withSecret := pc
	withSecret.WriteConnectionSecretToReference = &xpv2.LocalSecretReference{Name: "web-client-enrollment"}

	// The resources are created in an order in which each one refers to
	// something that does not exist yet.
	policy := &v1alpha1.ServicePolicy{
		ObjectMeta: meta1("web-dial"),
		Spec: v1alpha1.ServicePolicySpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.ServicePolicyParameters{
			Name: "web-dial", Type: v1alpha1.ServicePolicyTypeDial, ServiceRoles: []string{"@web"}, IdentityRoles: []string{"@web-client"},
		}},
	}
	svc := &v1alpha1.Service{
		ObjectMeta: meta1("web"),
		Spec: v1alpha1.ServiceSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.ServiceParameters{
			Name: "web", Configs: []string{"web-host"}, RoleAttributes: []string{"web"},
		}},
	}
	host := &v1alpha1.ConfigHostV1{
		ObjectMeta: meta1("web-host"),
		Spec: v1alpha1.ConfigHostV1Spec{ManagedResourceSpec: pc, ForProvider: v1alpha1.ConfigHostV1Parameters{
			Name:           "web-host",
			HostTerminator: v1alpha1.HostTerminator{Address: ptr.To("localhost"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp")},
		}},
	}
	idn := &v1alpha1.Identity{
		ObjectMeta: meta1("web-client"),
		Spec: v1alpha1.IdentitySpec{ManagedResourceSpec: withSecret, ForProvider: v1alpha1.IdentityParameters{
			Name: "web-client", RoleAttributes: []string{"clients"},
		}},
	}
	managed := []resource.Managed{policy, svc, host, idn}
	create(ctx, t, k, policy, svc, host, idn)

	for _, mg := range managed {
		eventually(t, "%T %s is ready and synced", func() bool {
			if err := k.Get(ctx, kube.ObjectKeyFromObject(mg), mg); err != nil {
				return false
			}
			return mg.GetCondition(xpv2.TypeReady).Status == corev1.ConditionTrue &&
				mg.GetCondition(xpv2.TypeSynced).Status == corev1.ConditionTrue
		}, mg, mg.GetName())
	}

	// Every resource points at the entity the fake controller stores.
	serviceID, hostID, identityID, policyID := meta.GetExternalName(svc), meta.GetExternalName(host), meta.GetExternalName(idn), meta.GetExternalName(policy)
	for collection, id := range map[string]string{"services": serviceID, "configs": hostID, "identities": identityID, "service-policies": policyID} {
		if id == "" || srv.Entity(collection, id) == nil {
			t.Fatalf("no entity in %s for external name %q", collection, id)
		}
	}
	if got, want := srv.Entity("services", serviceID)["configs"], []any{hostID}; len(got.([]any)) != 1 || got.([]any)[0] != want[0] {
		t.Errorf("service configs: want %v, got %v", want, got)
	}
	if got := srv.Entity("service-policies", policyID)["serviceRoles"].([]any); len(got) != 1 || got[0] != "@"+serviceID {
		t.Errorf("policy service roles: want [@%s], got %v", serviceID, got)
	}
	if got := svc.Status.AtProvider.ID; got != serviceID {
		t.Errorf("status.atProvider.id: want %q, got %q", serviceID, got)
	}

	eventually(t, "the enrollment token is published to the connection secret", func() bool {
		s := &corev1.Secret{}
		if err := k.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "web-client-enrollment"}, s); err != nil {
			return false
		}
		return string(s.Data[identity.ConnectionKeyEnrollmentToken]) == "jwt-for-web-client"
	})

	usage := &v1alpha1.ProviderConfigUsage{}
	if err := k.Get(ctx, types.NamespacedName{Namespace: namespace, Name: string(svc.GetUID())}, usage); err != nil {
		t.Errorf("cannot get the ProviderConfigUsage of the service: %v", err)
	}

	// A spec change reaches Ziti.
	if err := k.Get(ctx, kube.ObjectKeyFromObject(svc), svc); err != nil {
		t.Fatalf("cannot get service: %v", err)
	}
	svc.Spec.ForProvider.RoleAttributes = []string{"web", "updated"}
	if err := k.Update(ctx, svc); err != nil {
		t.Fatalf("cannot update service: %v", err)
	}
	eventually(t, "the service is updated in Ziti", func() bool {
		got, _ := srv.Entity("services", serviceID)["roleAttributes"].([]any)
		return len(got) == 2
	})

	// A change made in Ziti is reverted at the next poll.
	tampered := srv.Entity("identities", identityID)
	tampered["roleAttributes"] = []any{"tampered"}
	srv.Put("identities", tampered)
	eventually(t, "the identity is restored in Ziti", func() bool {
		got, _ := srv.Entity("identities", identityID)["roleAttributes"].([]any)
		return len(got) == 1 && got[0] == "clients"
	})

	// The type of an identity is fixed when it is created.
	eventually(t, "changing the type of an identity is rejected", func() bool {
		if err := k.Get(ctx, kube.ObjectKeyFromObject(idn), idn); err != nil {
			return false
		}
		idn.Spec.ForProvider.Type = "Device"
		return kerrors.IsInvalid(k.Update(ctx, idn))
	})

	// A creation that was interrupted before the Ziti ID could be saved is
	// recovered: the entity it left behind is taken over, not duplicated.
	srv.Put("services", map[string]any{"id": "left-behind", "name": "interrupted", "createdAt": time.Now().UTC().Format(time.RFC3339Nano)})
	interrupted := &v1alpha1.Service{
		ObjectMeta: meta1("interrupted"),
		Spec:       v1alpha1.ServiceSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.ServiceParameters{Name: "interrupted"}},
	}
	meta.SetExternalCreatePending(interrupted, time.Now())
	create(ctx, t, k, interrupted)
	eventually(t, "the service whose creation was interrupted takes over the entity left behind", func() bool {
		if err := k.Get(ctx, kube.ObjectKeyFromObject(interrupted), interrupted); err != nil {
			return false
		}
		return meta.GetExternalName(interrupted) == "left-behind" &&
			interrupted.GetCondition(xpv2.TypeReady).Status == corev1.ConditionTrue &&
			interrupted.GetCondition(xpv2.TypeSynced).Status == corev1.ConditionTrue
	})
	if got := srv.Len("services"); got != 2 {
		t.Errorf("want the two services of the two managed resources in Ziti, got %d", got)
	}

	// A creation Ziti carries out without answering is recovered as well.
	srv.LoseNextCreateResponse()
	unanswered := &v1alpha1.Service{
		ObjectMeta: meta1("unanswered"),
		Spec:       v1alpha1.ServiceSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.ServiceParameters{Name: "unanswered"}},
	}
	create(ctx, t, k, unanswered)
	eventually(t, "the service whose creation was not answered finds its entity", func() bool {
		if err := k.Get(ctx, kube.ObjectKeyFromObject(unanswered), unanswered); err != nil {
			return false
		}
		_, onRecord := unanswered.GetAnnotations()[generic.AnnotationKeyCreateUnconfirmed]
		return meta.GetExternalName(unanswered) != "" && !onRecord &&
			unanswered.GetCondition(xpv2.TypeReady).Status == corev1.ConditionTrue &&
			unanswered.GetCondition(xpv2.TypeSynced).Status == corev1.ConditionTrue
	})
	if got := srv.Len("services"); got != 3 {
		t.Errorf("want one service per managed resource in Ziti, three in all, got %d", got)
	}

	// A resource whose creation was interrupted while someone else held its
	// name owns nothing: it reports the conflict and can be deleted, and the
	// entity of the other party is left alone.
	srv.Put("services", map[string]any{"id": "foreign", "name": "taken", "createdAt": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)})
	taken := &v1alpha1.Service{
		ObjectMeta: meta1("taken"),
		Spec:       v1alpha1.ServiceSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.ServiceParameters{Name: "taken"}},
	}
	meta.SetExternalCreatePending(taken, time.Now())
	create(ctx, t, k, taken)
	eventually(t, "the service whose name is taken reports the conflict", func() bool {
		if err := k.Get(ctx, kube.ObjectKeyFromObject(taken), taken); err != nil {
			return false
		}
		synced := taken.GetCondition(xpv2.TypeSynced)
		return synced.Status == corev1.ConditionFalse && strings.Contains(synced.Message, "COULD_NOT_VALIDATE")
	})
	if got := meta.GetExternalName(taken); got != "" {
		t.Errorf("external name of the service whose name is taken: want none, got %q", got)
	}
	if err := k.Delete(ctx, taken); err != nil {
		t.Fatalf("cannot delete the service whose name is taken: %v", err)
	}
	eventually(t, "the service whose name is taken is gone", func() bool {
		return kerrors.IsNotFound(k.Get(ctx, kube.ObjectKeyFromObject(taken), taken))
	})
	if srv.Entity("services", "foreign") == nil {
		t.Errorf("want the service of the other party to be left alone, but it is gone")
	}
	srv.Delete("services", "foreign")

	// The kinds that share their logic with another kind have schemas of
	// their own.
	hosts := &v1alpha1.ConfigHostV2{
		ObjectMeta: meta1("web-hosts"),
		Spec: v1alpha1.ConfigHostV2Spec{ManagedResourceSpec: pc, ForProvider: v1alpha1.ConfigHostV2Parameters{
			Name: "web-hosts",
			Terminators: []v1alpha1.HostTerminator{
				{Address: ptr.To("web-1.internal"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp")},
				{Address: ptr.To("web-2.internal"), Port: ptr.To(int32(8080)), Protocol: ptr.To("tcp")},
			},
		}},
	}
	operator := &v1alpha1.IdentityUPDB{
		ObjectMeta: meta1("operator"),
		Spec: v1alpha1.IdentityUPDBSpec{ManagedResourceSpec: pc, ForProvider: v1alpha1.IdentityUPDBParameters{
			IdentityParameters: v1alpha1.IdentityParameters{Name: "operator", Type: "User"},
			UpdbUsername:       "operator",
		}},
	}
	create(ctx, t, k, hosts, operator)
	for _, mg := range []resource.Managed{hosts, operator} {
		eventually(t, "%T %s is ready and synced", func() bool {
			if err := k.Get(ctx, kube.ObjectKeyFromObject(mg), mg); err != nil {
				return false
			}
			return mg.GetCondition(xpv2.TypeReady).Status == corev1.ConditionTrue &&
				mg.GetCondition(xpv2.TypeSynced).Status == corev1.ConditionTrue
		}, mg, mg.GetName())
	}
	if got, _ := srv.Entity("configs", meta.GetExternalName(hosts))["configTypeId"].(string); got != "host-v2-id" {
		t.Errorf("config type of the host.v2 config: want host-v2-id, got %q", got)
	}
	eventually(t, "changing the username of a password identity is rejected", func() bool {
		if err := k.Get(ctx, kube.ObjectKeyFromObject(operator), operator); err != nil {
			return false
		}
		operator.Spec.ForProvider.UpdbUsername = "someone-else"
		return kerrors.IsInvalid(k.Update(ctx, operator))
	})
	empty := &v1alpha1.ConfigHostV2{
		ObjectMeta: meta1("no-hosts"),
		Spec: v1alpha1.ConfigHostV2Spec{ManagedResourceSpec: pc, ForProvider: v1alpha1.ConfigHostV2Parameters{
			Name: "no-hosts", Terminators: []v1alpha1.HostTerminator{},
		}},
	}
	if err := k.Create(ctx, empty); !kerrors.IsInvalid(err) {
		t.Errorf("creating a host.v2 config without terminators: want it to be rejected as invalid, got %v", err)
	}

	// Deleting the managed resources deletes the entities.
	managed = []resource.Managed{policy, svc, host, idn, interrupted, unanswered, hosts, operator}
	for _, mg := range managed {
		if err := k.Delete(ctx, mg); err != nil {
			t.Fatalf("cannot delete %T %s: %v", mg, mg.GetName(), err)
		}
	}
	for _, mg := range managed {
		eventually(t, "%T %s is gone", func() bool {
			return kerrors.IsNotFound(k.Get(ctx, kube.ObjectKeyFromObject(mg), mg))
		}, mg, mg.GetName())
	}
	for _, collection := range []string{"services", "configs", "identities", "service-policies"} {
		if got := srv.Len(collection); got != 0 {
			t.Errorf("want no entities left in %s, got %d", collection, got)
		}
	}
}

// startProvider starts a Kubernetes API server with the provider's CRDs and
// a controller manager set up like the one of the provider binary.
func startProvider(ctx context.Context, t *testing.T) kube.Client {
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
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, apiextensionsv1.AddToScheme, apis.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatalf("cannot build scheme: %v", err)
		}
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme,
		Metrics: server.Options{BindAddress: "0"},
		Client:  kube.Options{Cache: &kube.CacheOptions{DisableFor: []kube.Object{&corev1.Secret{}}}},
	})
	if err != nil {
		t.Fatalf("cannot create manager: %v", err)
	}

	features := &feature.Flags{}
	features.Enable(feature.EnableBetaManagementPolicies)
	o := xpcontroller.Options{
		Logger:                  logging.NewNopLogger(),
		MaxConcurrentReconciles: 2,
		PollInterval:            time.Second,
		GlobalRateLimiter:       ratelimiter.NewGlobal(100),
		Features:                features,
		Gate:                    new(gate.Gate[schema.GroupVersionKind]),
	}
	if err := customresourcesgate.Setup(mgr, o); err != nil {
		t.Fatalf("cannot setup the CRD gate: %v", err)
	}
	if err := controller.SetupGated(mgr, o); err != nil {
		t.Fatalf("cannot setup controllers: %v", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Errorf("manager failed: %v", err)
		}
	}()

	k, err := kube.New(cfg, kube.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("cannot create client: %v", err)
	}
	return k
}

func create(ctx context.Context, t *testing.T, k kube.Client, objects ...kube.Object) {
	t.Helper()

	for _, o := range objects {
		if err := k.Create(ctx, o); err != nil {
			t.Fatalf("cannot create %T %s: %v", o, o.GetName(), err)
		}
	}
}

func eventually(t *testing.T, format string, condition func() bool, args ...any) {
	t.Helper()

	start := time.Now()
	for !condition() {
		if time.Since(start) > wait {
			t.Fatalf("timed out waiting until "+format, args...)
		}
		time.Sleep(tick)
	}
	t.Logf(format+" (after %s)", append(args, time.Since(start).Round(100*time.Millisecond))...)
}
