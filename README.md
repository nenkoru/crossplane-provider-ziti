# provider-ziti

`provider-ziti` is a [Crossplane](https://crossplane.io/) provider that manages
the resources of an [OpenZiti](https://openziti.io/) network: services, configs,
policies, identities, edge routers and posture checks become Kubernetes
resources that are created, kept in sync and deleted through the Edge
Management API of the Ziti controller.

It targets Crossplane v2: managed resources and `ProviderConfig` are
namespaced, and a cluster-scoped `ClusterProviderConfig` is available.

> **Status: alpha, not released.** The API group is `ziti.crossplane.io/v1alpha1`
> and may change. There is no published package yet; see
> [Running the provider](#running-the-provider).

## Resources

| Kind | Ziti entity | Types | Controller | Example | Unit test | End-to-end test |
|------|-------------|:-----:|:----------:|:-------:|:---------:|:---------------:|
| `Service` | service | yes | yes | yes | yes | core |
| `ConfigHostV1` | config of type `host.v1` | yes | yes | yes | yes | core |
| `ConfigInterceptV1` | config of type `intercept.v1` | yes | yes | yes | yes | core |
| `ServicePolicy` | service policy (Dial, Bind) | yes | yes | yes | yes | core |
| `Identity` | identity, one-time token enrollment | yes | yes | yes | yes | core |
| `EdgeRouter` | edge router | yes | yes | yes | yes | extended |
| `EdgeRouterPolicy` | edge router policy | yes | yes | yes | yes | extended |
| `ServiceEdgeRouterPolicy` | service edge router policy | yes | yes | yes | yes | extended |
| `PostureCheckOS` | posture check of type OS | yes | yes | yes | yes | extended |
| `PostureCheckMFA` | posture check of type MFA | yes | yes | yes | yes | extended |
| `AuthPolicy` | auth policy | yes | yes | yes | yes | extended |
| `ConfigHostV2` | config of type `host.v2` | no | no | no | no | no |
| `IdentityCA` | identity, CA enrollment | no | no | no | no | no |
| `IdentityUPDB` | identity, password enrollment | no | no | no | no | no |
| `IdentityNone` | identity without enrollment | no | no | no | no | no |
| `PostureCheckDomain` | posture check of type DOMAIN | no | no | no | no | no |
| `PostureCheckMac` | posture check of type MAC | no | no | no | no | no |
| `PostureCheckProcess` | posture check of type PROCESS | no | no | no | no | no |
| `PostureCheckMultiProcess` | posture check of type PROCESS_MULTI | no | no | no | no | no |
| `CertificateAuthority` | certificate authority | no | no | no | no | no |
| `ExternalJWTSigner` | external JWT signer | no | no | no | no | no |

How to read the table:

- **Controller: yes** means all fields of the kind are created, updated and
  checked for drift.
- **Unit test** runs the kind through create, update and delete against an
  in-memory fake of the Ziti API.
- **End-to-end test** says which stage of [`test/e2e/e2e.sh`](test/e2e/e2e.sh)
  covers the kind against a real OpenZiti controller: the entity is created
  as declared, follows a spec change, is not touched without one, and is
  deleted with its managed resource. All eleven kinds pass against OpenZiti
  2.0.6 (last run on 2026-10-02).

## How it behaves

- **Identity of an entity.** The `crossplane.io/external-name` annotation holds
  the Ziti ID. It is empty until the entity is created. To adopt an existing
  entity, create the managed resource with the annotation set to its ID.
- **Drift.** Every field the provider sends is compared with what Ziti
  reports, on every poll (one minute by default, `--poll`). A change made in
  Ziti directly is reverted. Optional settings that are left unset in the
  spec are not managed, except lists and `tags`, which are cleared when unset.
- **References by name.** `Service.configs` takes config names or IDs. Policy
  roles take `#attribute`, `#all` or `@name`, where the name is resolved to an
  ID. A resource that refers to something that does not exist yet reports the
  error in its `Synced` condition and is retried.
- **Enrollment tokens.** `Identity` and `EdgeRouter` write their enrollment
  JWT to the Secret named in `spec.writeConnectionSecretToRef`, under the key
  `enrollmentToken`. Ziti stops reporting the token once it is used; the
  Secret keeps the last one. An expired token is not renewed yet.
- **Deletion.** Deleting a managed resource deletes the Ziti entity. Set
  `spec.managementPolicies` to keep it.
- **Updates.** Entities are updated with `PATCH`, so settings the provider
  does not manage are left alone. `Service` and `AuthPolicy` are replaced
  with `PUT` instead: a `PATCH` of a service ignores `encryptionRequired`, and
  a `PATCH` of an auth policy ignores some password settings.

## Running the provider

There is no published package yet. To try the provider, run it out-of-cluster
against a cluster that has its CRDs:

```shell
git clone --recurse-submodules https://github.com/nenkoru/crossplane-provider-ziti
cd crossplane-provider-ziti
kubectl apply -f package/crds
go run ./cmd/provider --debug
```

`make dev` does the same in a new kind cluster.

## Connecting to a Ziti controller

Put the credentials of a Ziti administrator in a Secret and reference it from
a `ProviderConfig` in the same namespace:

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

- The Secret may hold the keys `host`, `username`, `password`, `cert`, `key`
  and `ca`, or one key `credentials` with the same settings as JSON. `cert`
  and `key` select certificate authentication.
- The controller certificate is verified against the system roots. Set
  `spec.ca` for a controller with its own PKI, or
  `spec.insecureSkipTLSVerify: true` for testing.
- A `ClusterProviderConfig` works for all namespaces and must name the
  namespace of its Secret, see
  [`examples/provider/clusterconfig.yaml`](examples/provider/clusterconfig.yaml).
- `username`, `password`, `cert` and `key` can also be set inline in the spec.
  Anyone who can read the `ProviderConfig` can read them, so prefer the Secret.

## Example

A service, an identity that may dial it and the token to enroll that identity:

```yaml
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: web-host
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  forProvider:
    name: web-host
    address: web.internal
    port: 8080
    protocol: tcp
---
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: web
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  forProvider:
    name: web
    configs:
      - web-host
---
apiVersion: ziti.crossplane.io/v1alpha1
kind: Identity
metadata:
  name: web-client
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  writeConnectionSecretToRef:
    name: web-client-enrollment
  forProvider:
    name: web-client
    roleAttributes:
      - web-clients
---
apiVersion: ziti.crossplane.io/v1alpha1
kind: ServicePolicy
metadata:
  name: web-dial
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  forProvider:
    name: web-dial
    type: Dial
    serviceRoles:
      - "@web"
    identityRoles:
      - "#web-clients"
```

```shell
kubectl get secret web-client-enrollment -o jsonpath='{.data.enrollmentToken}' | base64 -d > web-client.jwt
```

`providerConfigRef` defaults to the `ClusterProviderConfig` named `default`.
More manifests, one directory per kind, are in [`examples/`](examples).

## Developing

The repository uses the Crossplane build submodule; run `make submodules` once
after cloning without `--recurse-submodules`.

| Command | What it does |
|---------|--------------|
| `make generate` | Regenerates deepcopy code, managed resource methods and CRDs after a change in `apis/`. |
| `make lint` | Runs golangci-lint. |
| `make test` | Runs the unit tests and the controller integration test, which starts a Kubernetes API server with envtest. |
| `make reviewable` | Runs all of the above; do this before opening a pull request. |
| `make e2e.ziti` | Runs the end-to-end test. Needs docker, kind, kubectl, curl and jq. |

How the code is organized:

- `apis/v1alpha1` holds the types. Field names follow the Ziti API and the
  [Terraform provider for Ziti](https://github.com/netfoundry/terraform-provider-ziti).
- `internal/client` is a small client for the Ziti Edge Management API, with
  an in-memory fake of the API in `internal/client/fake`.
- `internal/controller/generic` implements observe, create, update and delete
  once for all kinds.
- `internal/controller/<kind>` describes one kind: its Ziti collection, how
  its spec becomes an entity and how an entity becomes its status. Adding a
  kind means adding types, one such description, an example and a test case
  in `internal/controller/kinds_test.go`.

The end-to-end test starts an OpenZiti controller from
`ziti-docker-compose.yml` and a kind cluster, runs the provider
out-of-cluster, applies the examples and checks the result through the Ziti
API: the entities exist as declared, follow spec changes, are restored after
being changed in Ziti directly, are not updated without a spec change, and
are deleted with their managed resources.

`test/e2e/e2e.sh test` runs only the checks, against a provider that is
already running: point `KUBECONFIG` at the cluster with the CRDs through
`E2E_WORK_DIR` (the script reads `$E2E_WORK_DIR/kubeconfig`) and set
`ZITI_URL`, `ZITI_USER` and `ZITI_PWD` for the test controller. Never point
it at a Ziti network you care about: it creates, changes and deletes
entities.

## Roadmap

1. Run the end-to-end suite in CI on every pull request.
2. Add the missing kinds, starting with `ConfigHostV2` and the other identity
   enrollments.
3. Renew expired enrollment tokens.
4. Package and install the provider through Crossplane in the end-to-end test,
   then publish a first release.

## License

Apache 2.0, see [LICENSE](LICENSE).
