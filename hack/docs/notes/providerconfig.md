A `ProviderConfig` connects the managed resources of its namespace to a Ziti
controller. A `ClusterProviderConfig` has the same settings and serves all
namespaces. A managed resource names one of them in `spec.providerConfigRef`;
without it, it uses the `ClusterProviderConfig` named `default`.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ziti-credentials
  namespace: default
stringData:
  username: admin
  password: change-me
---
apiVersion: ziti.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: default
spec:
  host: https://controller.example.com:1280
  credentials:
    source: Secret
    secretRef:
      name: ziti-credentials
```

More in [`examples/provider/config.yaml`](../../examples/provider/config.yaml)
and [`examples/provider/clusterconfig.yaml`](../../examples/provider/clusterconfig.yaml).

<!-- fields -->

## How it behaves

- The Secret may hold the keys `host`, `username`, `password`, `cert`, `key` and `ca`, or one key `credentials` with the same settings as JSON. What the Secret holds takes precedence over the inline settings of the spec.
- `cert` and `key` select certificate authentication; otherwise the provider signs in with `username` and `password`. The account must be a Ziti administrator.
- `host` must be an `https` URL: credentials are never sent in the clear. A trailing `/edge/management/v1` is accepted.
- The certificate of the controller is verified against the system roots. Set `ca` for a controller with its own PKI. `insecureSkipTLSVerify` is for test controllers only.
- A `ClusterProviderConfig` must name the namespace of its Secret; a `ProviderConfig` reads the Secret of its own namespace.
- Inline `password` and `key` can be read by anyone who can read the `ProviderConfig`. Prefer the Secret.
- A change of the Secret or of the settings takes effect at the next reconcile of each managed resource.

## When something goes wrong

Every managed resource that uses the configuration reports the error in its
`Synced` condition:

- `cannot get ProviderConfig` or `cannot get credentials secret`: the name, the kind or the namespace in `spec.providerConfigRef` or `credentials.secretRef` is wrong.
- `x509: certificate signed by unknown authority`: set `ca` to the CA bundle of the controller.
- An authentication error with status `401`: the username, the password or the certificate is not accepted by the controller.
- `connection refused` or a timeout: the provider cannot reach `host` from where it runs.

See [Troubleshooting](../troubleshooting.md).
