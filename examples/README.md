# Examples

One directory per kind. Every manifest is validated against the CRD of its
kind by `go test ./apis/...`.

| Path | What it shows |
|------|---------------|
| `provider/config.yaml` | A `ProviderConfig` with credentials in a Secret. |
| `provider/clusterconfig.yaml` | A `ClusterProviderConfig` for all namespaces. |
| `service/minimal.yaml` | A service with a host config. |
| `service/service.yaml` | A service with a host and an intercept config. |
| `service/advanced.yaml` | Every setting of the host and intercept configs. |
| `service/ha.yaml` | A service with the `ha` terminator strategy. |
| `service/hostv2.yaml` | A service hosted at two destinations with a `host.v2` config. |
| `identity/identity.yaml` | Identities whose enrollment token is written to a Secret. |
| `identity/ca.yaml` | An identity that enrolls with a certificate of a third-party CA. |
| `identity/updb.yaml` | An identity that authenticates with a username and a password. |
| `identity/none.yaml` | An identity without an enrollment, for an external JWT signer. |
| `servicepolicy/servicepolicy.yaml` | Who may dial and who may host the service of `service/service.yaml`. |
| `edgerouter/edgerouter.yaml` | An edge router whose enrollment token is written to a Secret. |
| `edgerouterpolicy/edgerouterpolicy.yaml` | Which identities may use which edge routers. |
| `serviceedgerouterpolicy/serviceedgerouterpolicy.yaml` | Which services are available on which edge routers. |
| `posturecheck/os.yaml`, `posturecheck/mfa.yaml` | Posture checks. |
| `authpolicy/authpolicy.yaml` | An auth policy. |

## Trying them

Edit `provider/config.yaml` to point at your Ziti controller, then:

```shell
kubectl apply -f examples/provider/config.yaml
kubectl apply -f examples/service/service.yaml -f examples/identity/identity.yaml -f examples/servicepolicy/servicepolicy.yaml
kubectl get managed
```

`service.yaml`, `identity.yaml` and `servicepolicy.yaml` belong together: the
policies refer to the service and the identities by name. The order in which
they are applied does not matter; a resource that refers to something that
does not exist yet is retried.

The enrollment token of an identity is in its connection secret:

```shell
kubectl get secret web-client-enrollment -o jsonpath='{.data.enrollmentToken}' | base64 -d > web-client.jwt
```
