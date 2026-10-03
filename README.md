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

## Documentation

[`docs/`](docs/README.md) has a [quick start](docs/quickstart.md), a page per
kind with every setting and what it does in Ziti
([resources](docs/resources/README.md), or all on
[one page](docs/fields.md)), the [concepts](docs/concepts.md) behind the
provider, [troubleshooting](docs/troubleshooting.md) and what the
[tests](docs/testing.md) cover. [`examples/`](examples/README.md) has
manifests per kind, scenarios and a Composition.

## Resources

| Kind | Ziti entity | Types | Controller | Example | Unit test | End-to-end test |
|------|-------------|:-----:|:----------:|:-------:|:---------:|:---------------:|
| [`Service`](docs/resources/service.md) | service | yes | yes | yes | yes | core |
| [`ConfigHostV1`](docs/resources/confighostv1.md) | config of type `host.v1` | yes | yes | yes | yes | core |
| [`ConfigInterceptV1`](docs/resources/configinterceptv1.md) | config of type `intercept.v1` | yes | yes | yes | yes | core |
| [`ServicePolicy`](docs/resources/servicepolicy.md) | service policy (Dial, Bind) | yes | yes | yes | yes | core |
| [`Identity`](docs/resources/identity.md) | identity, one-time token enrollment | yes | yes | yes | yes | core |
| [`EdgeRouter`](docs/resources/edgerouter.md) | edge router | yes | yes | yes | yes | extended |
| [`EdgeRouterPolicy`](docs/resources/edgerouterpolicy.md) | edge router policy | yes | yes | yes | yes | extended |
| [`ServiceEdgeRouterPolicy`](docs/resources/serviceedgerouterpolicy.md) | service edge router policy | yes | yes | yes | yes | extended |
| [`PostureCheckOS`](docs/resources/posturecheckos.md) | posture check of type OS | yes | yes | yes | yes | extended |
| [`PostureCheckMFA`](docs/resources/posturecheckmfa.md) | posture check of type MFA | yes | yes | yes | yes | extended |
| [`AuthPolicy`](docs/resources/authpolicy.md) | auth policy | yes | yes | yes | yes | extended |
| [`ConfigHostV2`](docs/resources/confighostv2.md) | config of type `host.v2` | yes | yes | yes | yes | extended |
| [`IdentityCA`](docs/resources/identityca.md) | identity, enrollment with a certificate of a third-party CA | yes | yes | yes | yes | extended |
| [`IdentityUPDB`](docs/resources/identityupdb.md) | identity, password enrollment | yes | yes | yes | yes | extended |
| [`IdentityNone`](docs/resources/identitynone.md) | identity without enrollment | yes | yes | yes | yes | extended |
| [`PostureCheckDomain`](docs/resources/posturecheckdomain.md) | posture check of type DOMAIN | yes | yes | yes | yes | posture checks |
| [`PostureCheckMac`](docs/resources/posturecheckmac.md) | posture check of type MAC | yes | yes | yes | yes | posture checks |
| [`PostureCheckProcess`](docs/resources/posturecheckprocess.md) | posture check of type PROCESS | yes | yes | yes | yes | posture checks |
| [`PostureCheckMultiProcess`](docs/resources/posturecheckmultiprocess.md) | posture check of type PROCESS_MULTI | yes | yes | yes | yes | posture checks |
| [`CertificateAuthority`](docs/resources/certificateauthority.md) | third-party certificate authority | yes | yes | yes | yes | authentication |
| [`ExternalJWTSigner`](docs/resources/externaljwtsigner.md) | external JWT signer | yes | yes | yes | yes | authentication |

How to read the table:

- **Controller: yes** means all fields of the kind are created, updated and
  checked for drift.
- **Unit test** runs the kind through create, update and delete against an
  in-memory fake of the Ziti API.
- **End-to-end test** says which stage of [`test/e2e/e2e.sh`](test/e2e/e2e.sh)
  first covers the kind against a real OpenZiti controller: the entity is
  created as declared, follows a spec change, is not touched without one,
  and is deleted with its managed resource. The `lifecycle` stage then takes
  every kind through a change and a deletion made in Ziti directly. CI runs
  the test on every pull request against OpenZiti 2.0.6, and all twenty-one
  kinds pass; [Testing](docs/testing.md) has the details.

## How it behaves

In short; [Concepts](docs/concepts.md) has the details.

- The `crossplane.io/external-name` annotation holds the Ziti ID of the
  entity. An entity that merely has the same name is never taken over; set
  the annotation to adopt one.
- Every field the provider sends is compared with what Ziti reports on every
  poll (one minute by default, `--poll`). A change made in Ziti directly is
  reverted, and an entity that is deleted there is created anew.
- Resources refer to each other by name: `Service.configs`, `@name` roles of
  policies, `IdentityCA.ottca`, `authPolicyId`, signers of an `AuthPolicy`.
  A reference to something that does not exist yet is reported in the
  `Synced` condition and retried, so the order of creation does not matter.
- `Identity`, `IdentityCA`, `IdentityUPDB` and `EdgeRouter` write their
  enrollment JWT to the Secret named in `spec.writeConnectionSecretToRef`,
  under the key `enrollmentToken`, and replace a token that expired or was
  deleted until the identity or router has enrolled.
- Deleting a managed resource deletes the Ziti entity. Set
  `spec.managementPolicies` to keep it.

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

## Quick start

This walks through exposing a web service over Ziti with the manifests in
[`examples/`](examples). The end-to-end test applies the same files to a
real OpenZiti controller, and `go test ./apis/...` validates them against
the CRDs.

1. **Connect the provider to the controller.** Put the URL of the controller
   and the credentials of a Ziti administrator into
   [`examples/provider/config.yaml`](examples/provider/config.yaml), see
   [Connecting to a Ziti controller](#connecting-to-a-ziti-controller), and
   apply it:

   ```shell
   kubectl apply -f examples/provider/config.yaml
   ```

2. **Describe the service.**
   [`examples/service/service.yaml`](examples/service/service.yaml) has a
   `ConfigHostV1` that tells the hosting identity where the service runs
   (`web-service.local:8080`), a `ConfigInterceptV1` that tells clients which
   addresses to intercept (`web.example.com` and `api.example.com`, ports 80
   and 443), and the `Service`, which refers to both configs by name and has
   the role attributes `role:web` and `role:api`.

3. **Create the identities.**
   [`examples/identity/identity.yaml`](examples/identity/identity.yaml) has
   `web-client`, with the role attribute `web-clients`, and `web-server`,
   which hosts the service. Each writes its one-time enrollment token to the
   Secret named in `writeConnectionSecretToRef`.

4. **Say who may do what.**
   [`examples/servicepolicy/servicepolicy.yaml`](examples/servicepolicy/servicepolicy.yaml)
   lets the identities with the attribute `web-clients` dial `@web-service`,
   and `@web-server` bind the services with the attribute `role:web`.
   [`examples/edgerouterpolicy/edgerouterpolicy.yaml`](examples/edgerouterpolicy/edgerouterpolicy.yaml)
   lets both use the edge routers with the attribute `public`, and
   [`examples/serviceedgerouterpolicy/serviceedgerouterpolicy.yaml`](examples/serviceedgerouterpolicy/serviceedgerouterpolicy.yaml)
   makes the service available on them.
   [`examples/edgerouter/edgerouter.yaml`](examples/edgerouter/edgerouter.yaml)
   creates such a router, for a network that has none.

   ```shell
   kubectl apply -f examples/service/service.yaml \
     -f examples/identity/identity.yaml \
     -f examples/servicepolicy/servicepolicy.yaml \
     -f examples/edgerouterpolicy/edgerouterpolicy.yaml \
     -f examples/serviceedgerouterpolicy/serviceedgerouterpolicy.yaml
   ```

   The order does not matter: a resource that refers to something that does
   not exist yet reports it and is retried.

5. **Check the status.** Every kind is in the categories `managed` and
   `ziti`. `kubectl get` shows the name of the entity in Ziti, its ID and
   whether it is ready:

   ```shell
   kubectl get ziti
   kubectl wait --for=condition=Ready --timeout=3m -f examples/service/service.yaml
   ```

   A resource is `Ready` when its entity exists in Ziti, and `Synced` when
   the last reconcile succeeded: the entity was found as declared or was
   updated to be. A `Synced` condition that is `False` has the error in its
   message: a reference that does not exist yet, or what Ziti answered. `status.atProvider` shows the entity as Ziti
   reports it, and the annotation `crossplane.io/external-name` holds its ID.

   ```shell
   kubectl get services.ziti.crossplane.io web-service -o jsonpath='{.status.conditions}'
   kubectl describe identities.ziti.crossplane.io web-client
   ```

6. **Enroll the identities.** The token of `web-client` is in its Secret;
   enroll the client with it, and `web-server` on the host that runs the web
   server, with the token in `web-server-enrollment`:

   ```shell
   kubectl get secret web-client-enrollment -o jsonpath='{.data.enrollmentToken}' | base64 -d > web-client.jwt
   ziti edge enroll web-client.jwt
   ```

7. **Clean up.** Deleting the managed resources deletes the entities in Ziti:

   ```shell
   kubectl delete -f examples/serviceedgerouterpolicy/serviceedgerouterpolicy.yaml \
     -f examples/edgerouterpolicy/edgerouterpolicy.yaml \
     -f examples/servicepolicy/servicepolicy.yaml \
     -f examples/identity/identity.yaml \
     -f examples/service/service.yaml
   ```

`providerConfigRef` defaults to the `ClusterProviderConfig` named `default`;
the examples name the `ProviderConfig` of their namespace instead. More
manifests, one directory per kind, are in [`examples/`](examples).

## Developing

The repository uses the Crossplane build submodule; run `make submodules` once
after cloning without `--recurse-submodules`.

| Command | What it does |
|---------|--------------|
| `make generate` | Regenerates deepcopy code, managed resource methods, CRDs and the reference pages of `docs/` after a change in `apis/`, `examples/` or `hack/docs/notes`. |
| `make lint` | Runs golangci-lint. |
| `make test` | Runs the unit tests and the controller integration test, which starts a Kubernetes API server with envtest. |
| `make reviewable` | Runs all of the above; do this before opening a pull request. |
| `make e2e.ziti` | Runs the end-to-end test. Needs docker, kind, kubectl, helm, curl, jq and openssl. |
| `make build` | Builds the provider, its image and the package `_output/xpkg/linux_<arch>/provider-ziti-<version>.xpkg`. On macOS add `PLATFORM=linux_arm64` or `PLATFORM=linux_amd64`. |
| `make e2e.package` | Installs the package `make build` wrote through Crossplane and runs the `core` and `scenarios` stages of the end-to-end test on it. |

The fuzz targets (`Fuzz...` in `internal/controller`, `generic` and
`posturecheck`) run their seed corpora with the unit tests. To fuzz one,
for example the reconciler of services:

```shell
go test -run '^$' -fuzz '^FuzzReconcileService$' -fuzztime 60s ./internal/controller
```

An input that fails is written to `testdata/fuzz` of the package; commit it
along with the fix, so that it keeps running with the unit tests.

How the code is organized:

- `apis/v1alpha1` holds the types. Field names follow the Ziti API and the
  [Terraform provider for Ziti](https://github.com/netfoundry/terraform-provider-ziti).
- `internal/client` is a small client for the Ziti Edge Management API, with
  an in-memory fake of the API in `internal/client/fake`.
- `internal/controller/generic` implements observe, create, update and delete
  once for all kinds.
- `internal/controller/<kind>` describes one kind: its Ziti collection, how
  its spec becomes an entity and how an entity becomes its status. Adding a
  kind means adding types, one such description, an example, a test case
  in `internal/controller/kinds_test.go`, an entry in `hack/docs/kinds.go`
  with notes in `hack/docs/notes`, and a row in the `lifecycle` stage of the
  end-to-end test.

The end-to-end test starts an OpenZiti controller from
`ziti-docker-compose.yml` and a kind cluster with Crossplane, runs the
provider out-of-cluster, applies the examples and checks the result through
the Ziti API. `make e2e.package` runs a part of it on the package instead,
installed through Crossplane. [Testing](docs/testing.md) says what it checks
for every kind, how to run a part of it, and how to run it inside a virtual
cluster of an existing Kubernetes cluster with
[`test/e2e/vcluster`](test/e2e/vcluster).
Never point it at a Ziti network you care about: it creates, changes and
deletes entities.

## Roadmap

1. Give the end-to-end suite and the test of the package CI jobs of their
   own; the `unit-tests` and `publish-artifacts` jobs run them for now.
2. Publish a first release. CI builds the package and tests it installed
   through Crossplane; it is not published anywhere yet.

## License

Apache 2.0, see [LICENSE](LICENSE).
