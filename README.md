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
| `CertificateAuthority` | third-party certificate authority | yes | yes | yes | yes | authentication |
| `ExternalJWTSigner` | external JWT signer | yes | yes | yes | yes | authentication |

How to read the table:

- **Controller: yes** means all fields of the kind are created, updated and
  checked for drift.
- **Unit test** runs the kind through create, update and delete against an
  in-memory fake of the Ziti API.
- **End-to-end test** says which stage of [`test/e2e/e2e.sh`](test/e2e/e2e.sh)
  covers the kind against a real OpenZiti controller: the entity is created
  as declared, follows a spec change, is not touched without one, and is
  deleted with its managed resource. CI runs it on every pull request
  against OpenZiti 2.0.6, and all twenty-one kinds pass.

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
  which are cleared when unset. Lists of strings at the top level of an
  entity, such as roles and role attributes, are compared as sets, as Ziti
  stores them: their order and duplicates do not matter. See
  [What happens when someone edits Ziti directly](#what-happens-when-someone-edits-ziti-directly).
- **References by name.** `Service.configs` takes config names or IDs, and so
  do `authPolicyId` and the keys of `serviceHostingCosts` and
  `serviceHostingPrecedences` of an identity, and `IdentityCA.ottca`. Policy
  roles take `#attribute`, `#all` or `@name`, where the name is resolved to an
  ID. A resource that refers to something that does not exist yet reports the
  error in its `Synced` condition and is retried. Two keys of
  `serviceHostingCosts` or `serviceHostingPrecedences` that name the same
  service, by its name and by its ID, must have the same value; otherwise
  the resource reports them.
- **Certificate authorities.** A `CertificateAuthority` is ready when it
  exists in Ziti as declared, verified or not. The provider cannot verify it:
  that takes a certificate signed with the private key of the certificate
  authority. Its status has what the owner of that key needs:
  `verificationToken` is the common name the certificate must have, to be
  posted to `/edge/management/v1/cas/<id>/verify`, and `isVerified` tells
  whether it is done. Until then Ziti does not accept certificates of the
  certificate authority for authentication. `fingerprint` tells which
  certificate Ziti holds. Deleting a certificate authority deletes the pending
  enrollments with its certificates: an `IdentityCA` that refers to it stays,
  but can no longer enroll.
- **External JWT signers.** An `AuthPolicy` takes names or IDs of signers in
  `primary.extJwt.allowedSigners` and `secondary.requireExtJwtSigner`, and an
  `ExternalJWTSigner` the name or ID of an auth policy in
  `enrollAuthPolicyId`. Ziti does not delete a signer that an auth policy
  refers to: the `ExternalJWTSigner` reports the conflict and is deleted once
  the policy is gone. Ziti ignores the tags of a signer that is being created;
  they follow with an update right after.
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
  `spec.writeConnectionSecretToRef`, under the key `enrollmentToken`, and
  show when it expires in `status.atProvider.enrollmentExpiresAt`. Ziti
  stops reporting the token once it is used; the Secret keeps the last one.
- **Renewal of enrollment tokens.** A token that can no longer be used is
  replaced, in Ziti and in the Secret, as long as the identity cannot sign
  in or the edge router has not enrolled: when its enrollment has expired,
  and when it has none because the enrollment was deleted in Ziti. An
  enrollment that has expired is refreshed, a missing one is created with
  the method of the kind, and an edge router is enrolled anew.
  - An identity counts as able to sign in, and is never given a token, if
    Ziti reports an authenticator for it, if it has an `externalId`, or if
    its auth policy allows external JWTs as the first factor from a signer
    that is enabled and has `useExternalId: false`, which looks identities
    up by their Ziti ID. Ziti reports no authenticator for an identity that
    signs in with an external JWT, nor for one that signs in with a
    certificate of a certificate authority that carries its external ID.
    The default auth policy allows every signer, so one such signer stops
    renewal for every identity under that policy.
  - It never happens to an edge router that is verified or has a
    certificate: it has enrolled, and enrolling it anew would take its
    certificate away and disconnect it. Nothing is renewed either when Ziti
    does not report the authenticators of an identity or whether an edge
    router is verified. An identity whose authenticators were all deleted in
    Ziti, and that cannot sign in otherwise, gets a token again.
  - A token is replaced only once it has expired on the clock of the
    controller, five seconds ago or more, never while it can still be used.
  - The new token of an identity is valid for
    `spec.forProvider.enrollmentDuration`, which is `180m` unless set, the
    default of Ziti; the minimum is `5m`. The first token and every token of
    an edge router are valid for as long as the controller is configured to
    make them.
  - A renewal is an update: it follows `spec.managementPolicies`, and one
    that fails is reported in the `Synced` condition and retried.
- **Deletion.** Deleting a managed resource deletes the Ziti entity. Set
  `spec.managementPolicies` to keep it. The hosting settings an identity has
  per service are dropped first: Ziti keeps them when a service is deleted
  and then refuses to delete the identity.
- **Immutable settings.** The `type` of an identity, `IdentityCA.ottca`,
  `IdentityUPDB.updbUsername` and `CertificateAuthority.certPem` cannot be
  changed after creation; the API server rejects the change.
- **Updates.** Entities are updated with `PATCH`, so settings the provider
  does not manage are left alone. `Service` and `AuthPolicy` are replaced
  with `PUT` instead: a `PATCH` of a service ignores `encryptionRequired`, and
  a `PATCH` of an auth policy ignores some password settings.
  `CertificateAuthority` and `ExternalJWTSigner` are replaced as well: a
  `PATCH` of a certificate authority drops its external ID claim, and a
  `PATCH` cannot remove a setting, which a signer that changes from
  `jwksEndpoint` to `certPem` needs. A setting of these two kinds that is
  left unset in the spec is therefore not left alone: it is removed in Ziti
  or reset to the default of Ziti.

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

## What happens when someone edits Ziti directly

The provider compares every entity with its managed resource on every poll
and puts it back as declared. The unit tests run the reconciler against a
fake of the Ziti API that stores entities the way Ziti does, and the drift
stage of the end-to-end test does the same against OpenZiti:

- **A changed setting is reverted** with an update at the next poll, for
  every field the spec manages: scalars, lists, roles and role attributes,
  tags and other maps, and the data of configs. Lists of strings are
  compared as sets; lists in the data of a config keep their order.
- **What Ziti owns is left alone**: timestamps, links, whether an identity
  or edge router has enrolled and a certificate authority is verified, and
  the settings the spec leaves unset, with the exceptions listed under
  **Drift** above.
- **A deleted entity is created anew** at the next poll, under a new ID,
  which the managed resource records in its external name. Ziti removes the
  references to a deleted entity from policies (`@id` roles) and services
  (configs); the resources that refer to it by name refer to the new ID
  once it exists, and report the missing entity until then. An identity or
  edge router created anew has a new enrollment token, which replaces the
  old one in its connection Secret: whatever enrolled with the deleted
  entity has to enroll again. Crossplane waits 30 seconds after it created
  an entity before it creates one again that it does not find.
- **An entity created by hand in its place is not taken over.** If someone
  deletes the entity and creates another one with the same name, the
  managed resource reports the name conflict Ziti answers, is not ready, and
  leaves the other entity alone. Set its `crossplane.io/external-name`
  annotation to the ID of that entity to adopt it, or delete the entity.
- **Enrollments.** Once an identity or edge router has enrolled, Ziti no
  longer reports its token, and the Secret keeps the last one. An
  enrollment that is deleted or has expired is replaced as described under
  **Renewal of enrollment tokens**.
- **Deletion.** A managed resource whose entity is already gone is deleted
  without an error. A deletion that Ziti refuses, because another entity
  refers to the one being deleted, is reported and retried.
- **Errors of Ziti or of the network**, such as answers with the status
  500, 502, 503, 504, 408 or 429, closed connections and timeouts, are
  reported in the `Synced` condition and retried. A managed resource that is
  ready and synced after a reconcile has its entity as declared, and no
  entity is lost or created twice: a creation that got no answer is settled
  as described under **Creation with an unknown result**.

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
  kind means adding types, one such description, an example and a test case
  in `internal/controller/kinds_test.go`.

The end-to-end test starts an OpenZiti controller from
`ziti-docker-compose.yml` and a kind cluster, runs the provider
out-of-cluster, applies the examples and checks the result through the Ziti
API: the entities exist as declared, follow spec changes, are restored after
being changed in Ziti directly, are created anew after being deleted there,
are not updated without a spec change, and are deleted with their managed
resources. It also lets an enrollment expire,
deletes others and enrolls an identity and an edge router, and checks that
the tokens are replaced and that what has enrolled, or can sign in without
an authenticator, is left alone. It starts
OpenZiti 2.0.6; set
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
2. Package and install the provider through Crossplane in the end-to-end test,
   then publish a first release.

## License

Apache 2.0, see [LICENSE](LICENSE).
