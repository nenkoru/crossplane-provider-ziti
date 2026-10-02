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
| `ConfigHostV2` | config of type `host.v2` | yes | yes | yes | yes | extended |
| `IdentityCA` | identity, enrollment with a certificate of a third-party CA | yes | yes | yes | yes | extended |
| `IdentityUPDB` | identity, password enrollment | yes | yes | yes | yes | extended |
| `IdentityNone` | identity without enrollment | yes | yes | yes | yes | extended |
| `PostureCheckDomain` | posture check of type DOMAIN | yes | yes | yes | yes | posture checks |
| `PostureCheckMac` | posture check of type MAC | yes | yes | yes | yes | posture checks |
| `PostureCheckProcess` | posture check of type PROCESS | yes | yes | yes | yes | posture checks |
| `PostureCheckMultiProcess` | posture check of type PROCESS_MULTI | yes | yes | yes | yes | posture checks |
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
  deleted with its managed resource. CI runs it on every pull request
  against OpenZiti 2.0.6, and all fifteen kinds that have a controller pass.

## How it behaves

- **Identity of an entity.** The `crossplane.io/external-name` annotation holds
  the Ziti ID. It is empty until the entity is created. To adopt an existing
  entity, create the managed resource with the annotation set to its ID. An
  entity that merely has the same name is never taken over: creating the
  managed resource fails with the name conflict Ziti reports.
- **Creation with an unknown result.** The provider may stop after Ziti
  created the entity and before the ID was saved, or the answer of Ziti may
  get lost on the way. The next reconcile then finds the entity by its name
  and continues with it, provided Ziti created it after that creation
  started; the clocks of the controller and the provider are compared for
  this, to within five seconds. An entity that had the name before is left
  alone: the resource reports the name conflict and can be deleted without
  touching it. A request that was not answered stays on record in the
  `ziti.crossplane.io/create-unconfirmed` annotation until it is settled.
- **Drift.** Every field the provider sends is compared with what Ziti
  reports, on every poll (one minute by default, `--poll`). A change made in
  Ziti directly is reverted. Optional settings that are left unset in the
  spec are not managed, except lists and maps (`tags`, and `appData`,
  `serviceHostingCosts` and `serviceHostingPrecedences` of an identity),
  which are cleared when unset.
- **References by name.** `Service.configs` takes config names or IDs, and so
  do `authPolicyId` and the keys of `serviceHostingCosts` and
  `serviceHostingPrecedences` of an identity, and `IdentityCA.ottca`. Policy
  roles take `#attribute`, `#all` or `@name`, where the name is resolved to an
  ID. A resource that refers to something that does not exist yet reports the
  error in its `Synced` condition and is retried.
- **Identities.** The four identity kinds take the same settings and differ
  in how the identity enrolls: `Identity` with a one-time token, `IdentityCA`
  with a one-time token and a certificate of a third-party CA,
  `IdentityUPDB` by choosing a password, `IdentityNone` not at all.
- **Posture checks.** Ziti does not store the values of a posture check as
  they are sent: domains, MAC addresses, hashes, fingerprints and the
  versions of an operating system are kept as sets, and MAC addresses,
  hashes and fingerprints in lower case without separators. The spec may
  give them in any order, in either case and with `:`, `-`, `.` or spaces
  between the digits; the provider sends and compares them in the form Ziti
  stores, which is also what `status.atProvider` shows. Ziti keeps one
  entry per operating system type of a `PostureCheckOS` and per operating
  system and path of a `PostureCheckMultiProcess`, in their order, so the
  API server rejects an entry that is listed twice.
- **Enrollment tokens.** `Identity`, `IdentityCA`, `IdentityUPDB` and
  `EdgeRouter` write their enrollment JWT to the Secret named in
  `spec.writeConnectionSecretToRef`, under the key `enrollmentToken`. Ziti
  stops reporting the token once it is used; the Secret keeps the last one.
  An expired token is not renewed yet.
- **Deletion.** Deleting a managed resource deletes the Ziti entity. Set
  `spec.managementPolicies` to keep it. The hosting settings an identity has
  per service are dropped first: Ziti keeps them when a service is deleted
  and then refuses to delete the identity.
- **Immutable settings.** The `type` of an identity, `IdentityCA.ottca` and
  `IdentityUPDB.updbUsername` cannot be changed after creation; the API
  server rejects the change.
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
- `host` must be an `https` URL: credentials are never sent in the clear.
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
| `make e2e.ziti` | Runs the end-to-end test. Needs docker, kind, kubectl, curl, jq and openssl. |

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
are deleted with their managed resources. It starts OpenZiti 2.0.6; set
`ZITI_VERSION` for another release. In CI the `unit-tests` job runs it after
the unit tests, until the workflow gets a job of its own for it.

`test/e2e/e2e.sh test` runs only the checks, against a provider that is
already running: point `KUBECONFIG` at the cluster with the CRDs through
`E2E_WORK_DIR` (the script reads `$E2E_WORK_DIR/kubeconfig`) and set
`ZITI_URL`, `ZITI_USER` and `ZITI_PWD` for the test controller. Never point
it at a Ziti network you care about: it creates, changes and deletes
entities.

## Roadmap

1. Give the end-to-end suite its own CI job; the `unit-tests` job runs it for
   now.
2. Add the missing kinds: `CertificateAuthority` and `ExternalJWTSigner`.
3. Renew expired enrollment tokens.
4. Package and install the provider through Crossplane in the end-to-end test,
   then publish a first release.

## License

Apache 2.0, see [LICENSE](LICENSE).
