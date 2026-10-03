# Examples

Scenarios that belong together, a Composition, and one directory per kind.
Every manifest of a kind of the provider is validated against its CRD by
`go test ./apis/...`, and the end-to-end test applies them to a real OpenZiti
controller. The [documentation](../docs/README.md) has a page per kind with
all settings.

## Scenarios

Each file is complete: apply it after a `ProviderConfig` and it works.

| Path | What it does |
|------|--------------|
| `scenarios/publish-service.yaml` | Publishes a service: host and intercept configs, the service, a Dial and a Bind policy, a client identity and a hosting identity with their enrollment tokens. The [quick start](../docs/quickstart.md) walks through it. |
| `scenarios/edge-router.yaml` | Adds an edge router with its enrollment token, lets every identity use the routers of its site, and makes the databases available on them. |
| `scenarios/configs.yaml` | Intercept and host configs for the usual cases: one name and port, a whole subnet with the address, port and protocol forwarded, a wildcard domain with a health check, and two destinations with a `host.v2` config. |

## Composition

[`composition/`](composition) turns one `PublishedService` into the six
resources a published service takes, with Crossplane. See its
[README](composition/README.md).

## Per kind

| Path | What it shows |
|------|---------------|
| `provider/config.yaml` | A `ProviderConfig` with credentials in a Secret. |
| `provider/clusterconfig.yaml` | A `ClusterProviderConfig` for all namespaces. |
| `service/minimal.yaml` | A service with a host config. |
| `service/service.yaml` | A service with a host and an intercept config. |
| `service/advanced.yaml` | Every setting of the host and intercept configs. |
| `service/sticky.yaml` | A service with the `sticky` terminator strategy. |
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
| `authpolicy/jwt.yaml` | An auth policy that accepts the tokens of one external JWT signer, by name. |
| `certificateauthority/certificateauthority.yaml` | A third-party certificate authority, for `identity/ca.yaml`. |
| `externaljwtsigner/externaljwtsigner.yaml` | An OpenID Connect provider as an external JWT signer. |

## Trying them

Run the provider as the [quick start](../docs/quickstart.md) describes, edit
`provider/config.yaml` to point at your Ziti controller, then:

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
