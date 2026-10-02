package connector

import (
	"context"
	"encoding/json"
	"fmt"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	zitiv1alpha1 "github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
)

// ZitiConnector provides shared connectivity for all Ziti managed resources.
type ZitiConnector struct {
	Kube ctrl.Manager
}

// Connect creates an authenticated Ziti client using the ProviderConfig
// referenced by the managed resource.
func (c *ZitiConnector) Connect(ctx context.Context, ref *xpv2.ProviderConfigReference, ns string) (*client.ZitiClient, error) {
	if ref == nil {
		return nil, fmt.Errorf("no ProviderConfig reference")
	}

	cfg := &zitiv1alpha1.ProviderConfig{}
	cfg.Name = ref.Name
	cfg.Namespace = ns
	if err := c.Kube.GetAPIReader().Get(ctx, types.NamespacedName{Namespace: ns, Name: cfg.Name}, cfg); err != nil {
		return nil, fmt.Errorf("cannot get ProviderConfig %s/%s: %w", ns, ref.Name, err)
	}

	zc := client.Config{
		Host:     cfg.Spec.Host,
		Username: cfg.Spec.Username,
		Password: cfg.Spec.Password,
		CA:       cfg.Spec.CA,
		Cert:     cfg.Spec.Cert,
		Key:      cfg.Spec.Key,
	}

	// Extract credentials from secret if configured
	if cfg.Spec.Credentials != nil && cfg.Spec.Credentials.SecretRef != nil {
		username, password, err := c.extractCredentials(ctx, cfg, ns)
		if err != nil {
			return nil, err
		}
		zc.Username = username
		zc.Password = password
	}

	if zc.Username == "" || zc.Password == "" {
		return nil, fmt.Errorf("ProviderConfig %s: username and password required", ref.Name)
	}

	zitiClient, err := client.New(zc)
	if err != nil {
		return nil, fmt.Errorf("cannot create Ziti client: %w", err)
	}

	if err := zitiClient.Authenticate(zc); err != nil {
		return nil, fmt.Errorf("cannot authenticate: %w", err)
	}

	return zitiClient, nil
}

func (c *ZitiConnector) extractCredentials(ctx context.Context, cfg *zitiv1alpha1.ProviderConfig, ns string) (string, string, error) {
	secret := &corev1.Secret{}
	secret.Name = cfg.Spec.Credentials.SecretRef.Name
	if err := c.Kube.GetAPIReader().Get(ctx, types.NamespacedName{Namespace: ns, Name: secret.Name}, secret); err != nil {
		return "", "", fmt.Errorf("cannot get secret %s/%s: %w", ns, cfg.Spec.Credentials.SecretRef.Name, err)
	}

	key := "credentials"
	if creds, ok := secret.Data[key]; ok {
		return parseCredentials(creds)
	}

	// Try username/password keys separately
	if user, ok := secret.Data["username"]; ok {
		if pass, ok := secret.Data["password"]; ok {
			return string(user), string(pass), nil
		}
	}

	return "", "", fmt.Errorf("secret %s/%s: no 'credentials' or 'username'+'password' keys found", ns, cfg.Spec.Credentials.SecretRef.Name)
}

func parseCredentials(creds []byte) (string, string, error) {
	var jsonCreds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(creds, &jsonCreds); err == nil && jsonCreds.Username != "" {
		return jsonCreds.Username, jsonCreds.Password, nil
	}
	return "", "", fmt.Errorf("cannot parse credentials")
}
