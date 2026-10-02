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

// Package connector produces Ziti clients for managed resources from the
// ProviderConfig they reference.
package connector

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	kube "sigs.k8s.io/controller-runtime/pkg/client"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
)

const (
	errTrackPCUsage    = "cannot track ProviderConfig usage"
	errNoPCRef         = "managed resource does not reference a ProviderConfig"
	errGetPC           = "cannot get ProviderConfig"
	errGetCPC          = "cannot get ClusterProviderConfig"
	errUnsupportedKind = "unsupported provider config kind %q"
	errSource          = "unsupported credentials source %q"
	errNoSecretRef     = "credentials.secretRef is required"
	errSecretNamespace = "credentials.secretRef.namespace is required for a ClusterProviderConfig"
	errGetSecret       = "cannot get credentials secret"
	errParseSecret     = "cannot parse the credentials key of the credentials secret"
	errNewClient       = "cannot create Ziti client"

	// keyCredentials is the secret key that may hold all settings as JSON.
	keyCredentials = "credentials"

	// Settings a credentials secret may provide.
	keyHost     = "host"
	keyUsername = "username"
	keyPassword = "password"
	keyCert     = "cert"
	keyKey      = "key"
	keyCA       = "ca"
)

// A Connector produces Ziti clients for managed resources. Clients are cached
// per ProviderConfig so that reconciles share one API session.
type Connector struct {
	kube  kube.Client
	usage *resource.ProviderConfigUsageTracker

	mu      sync.Mutex
	clients map[string]cached
}

type cached struct {
	cfg    client.Config
	client *client.Client
}

// New returns a Connector that reads ProviderConfigs and their credentials
// with the supplied client.
func New(k kube.Client) *Connector {
	return &Connector{
		kube:    k,
		usage:   resource.NewProviderConfigUsageTracker(k, &v1alpha1.ProviderConfigUsage{}),
		clients: map[string]cached{},
	}
}

// Connect returns a Ziti client configured by the ProviderConfig the supplied
// managed resource references.
func (c *Connector) Connect(ctx context.Context, mg resource.ModernManaged) (*client.Client, error) {
	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	ref := mg.GetProviderConfigReference()
	if ref == nil {
		return nil, errors.New(errNoPCRef)
	}

	cfg, err := c.config(ctx, ref, mg.GetNamespace())
	if err != nil {
		return nil, err
	}

	key := ref.Kind + "/" + mg.GetNamespace() + "/" + ref.Name
	if ref.Kind == v1alpha1.ClusterProviderConfigKind {
		key = ref.Kind + "/" + ref.Name
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if hit, ok := c.clients[key]; ok && hit.cfg == cfg {
		return hit.client, nil
	}

	zc, err := client.New(cfg)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}
	c.clients[key] = cached{cfg: cfg, client: zc}
	return zc, nil
}

// spec returns the spec of the referenced provider config and the namespace
// its credentials secret must be read from.
func (c *Connector) spec(ctx context.Context, ref *xpv2.ProviderConfigReference, namespace string) (v1alpha1.ZitiProviderConfigSpec, string, error) {
	switch ref.Kind {
	case v1alpha1.ProviderConfigKind:
		pc := &v1alpha1.ProviderConfig{}
		if err := c.kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, pc); err != nil {
			return v1alpha1.ZitiProviderConfigSpec{}, "", errors.Wrap(err, errGetPC)
		}
		// A namespaced ProviderConfig must not reach into other namespaces.
		return pc.Spec.ZitiProviderConfigSpec, namespace, nil
	case v1alpha1.ClusterProviderConfigKind:
		cpc := &v1alpha1.ClusterProviderConfig{}
		if err := c.kube.Get(ctx, types.NamespacedName{Name: ref.Name}, cpc); err != nil {
			return v1alpha1.ZitiProviderConfigSpec{}, "", errors.Wrap(err, errGetCPC)
		}
		spec := cpc.Spec.ZitiProviderConfigSpec
		if spec.Credentials != nil && spec.Credentials.SecretRef != nil {
			return spec, spec.Credentials.SecretRef.Namespace, nil
		}
		return spec, "", nil
	default:
		return v1alpha1.ZitiProviderConfigSpec{}, "", errors.Errorf(errUnsupportedKind, ref.Kind)
	}
}

// config resolves the referenced provider config into client settings.
func (c *Connector) config(ctx context.Context, ref *xpv2.ProviderConfigReference, namespace string) (client.Config, error) {
	spec, secretNamespace, err := c.spec(ctx, ref, namespace)
	if err != nil {
		return client.Config{}, err
	}

	cfg := client.Config{
		Host:                  spec.Host,
		Username:              spec.Username,
		Password:              spec.Password,
		Cert:                  spec.Cert,
		Key:                   spec.Key,
		CA:                    spec.CA,
		InsecureSkipTLSVerify: spec.InsecureSkipTLSVerify,
	}

	if spec.Credentials == nil {
		return cfg, nil
	}
	if spec.Credentials.Source != "" && spec.Credentials.Source != xpv2.CredentialsSourceSecret {
		return client.Config{}, errors.Errorf(errSource, spec.Credentials.Source)
	}
	if spec.Credentials.SecretRef == nil {
		return client.Config{}, errors.New(errNoSecretRef)
	}
	if secretNamespace == "" {
		return client.Config{}, errors.New(errSecretNamespace)
	}

	secret := &corev1.Secret{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: spec.Credentials.SecretRef.Name, Namespace: secretNamespace}, secret); err != nil {
		return client.Config{}, errors.Wrap(err, errGetSecret)
	}

	if err := overlay(&cfg, secret.Data); err != nil {
		return client.Config{}, err
	}
	return cfg, nil
}

// overlay applies the settings found in a credentials secret on top of cfg.
func overlay(cfg *client.Config, data map[string][]byte) error {
	settings := map[string]string{}

	if raw, ok := data[keyCredentials]; ok {
		if err := json.Unmarshal(raw, &settings); err != nil {
			// The parse error may quote secret content, so it is not wrapped.
			return errors.New(errParseSecret)
		}
	}
	for k, v := range data {
		if k != keyCredentials {
			settings[k] = string(v)
		}
	}

	for k, target := range map[string]*string{
		keyHost:     &cfg.Host,
		keyUsername: &cfg.Username,
		keyPassword: &cfg.Password,
		keyCert:     &cfg.Cert,
		keyKey:      &cfg.Key,
		keyCA:       &cfg.CA,
	} {
		if v := settings[k]; v != "" {
			*target = v
		}
	}
	return nil
}
