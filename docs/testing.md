# Testing

## What is tested for every kind

The end-to-end test, [`test/e2e/e2e.sh`](../test/e2e/e2e.sh), runs the
provider against a real OpenZiti controller and looks at the result through
the Ziti Edge Management API. For each of the twenty-one kinds it checks
five things, in the stages the table names:

1. **Created**: the entity exists in Ziti as the manifest declares it, and
   the resource records its ID.
2. **Changed**: a change of the spec reaches Ziti, and the entity keeps its
   ID. Nothing is updated when the spec does not change: the update time of
   the entity stays the same over several polls.
3. **Drift**: a change made to the entity in Ziti directly is reverted.
4. **Gone in Ziti**: an entity that is deleted in Ziti directly is created
   anew under a new ID, as declared, and the resource is ready again.
5. **Deleted**: deleting the resource deletes the entity.

| Kind | Created, changed, deleted | Drift reverted | Gone in Ziti, created anew |
|------|---------------------------|----------------|----------------------------|
| [`Service`](resources/service.md) | core, scenarios, composition, lifecycle | drift, lifecycle | drift, lifecycle, composition |
| [`ConfigHostV1`](resources/confighostv1.md) | core, scenarios, composition, lifecycle | drift, lifecycle | drift, lifecycle |
| [`ConfigInterceptV1`](resources/configinterceptv1.md) | core, scenarios, composition, lifecycle | lifecycle | lifecycle |
| [`ServicePolicy`](resources/servicepolicy.md) | core, scenarios, composition, lifecycle | drift, lifecycle | drift, lifecycle |
| [`Identity`](resources/identity.md) | core, scenarios, lifecycle | core, drift, lifecycle | drift, lifecycle |
| [`EdgeRouter`](resources/edgerouter.md) | extended, scenarios, lifecycle | lifecycle | lifecycle |
| [`EdgeRouterPolicy`](resources/edgerouterpolicy.md) | extended, scenarios, lifecycle | lifecycle | lifecycle |
| [`ServiceEdgeRouterPolicy`](resources/serviceedgerouterpolicy.md) | extended, scenarios, composition, lifecycle | lifecycle | lifecycle |
| [`PostureCheckOS`](resources/posturecheckos.md) | extended, lifecycle | lifecycle | lifecycle |
| [`PostureCheckMFA`](resources/posturecheckmfa.md) | extended, lifecycle | lifecycle | lifecycle |
| [`AuthPolicy`](resources/authpolicy.md) | extended, authentication, lifecycle | authentication, lifecycle | lifecycle |
| [`ConfigHostV2`](resources/confighostv2.md) | extended, scenarios, lifecycle | lifecycle | lifecycle |
| [`IdentityCA`](resources/identityca.md) | extended, authentication, lifecycle | lifecycle | lifecycle |
| [`IdentityUPDB`](resources/identityupdb.md) | extended, lifecycle | lifecycle | lifecycle |
| [`IdentityNone`](resources/identitynone.md) | extended, lifecycle | lifecycle | lifecycle |
| [`PostureCheckDomain`](resources/posturecheckdomain.md) | posture_checks, lifecycle | posture_checks, lifecycle | lifecycle |
| [`PostureCheckMac`](resources/posturecheckmac.md) | posture_checks, lifecycle | posture_checks, lifecycle | lifecycle |
| [`PostureCheckProcess`](resources/posturecheckprocess.md) | posture_checks, lifecycle | posture_checks, lifecycle | lifecycle |
| [`PostureCheckMultiProcess`](resources/posturecheckmultiprocess.md) | posture_checks, lifecycle | posture_checks, lifecycle | lifecycle |
| [`CertificateAuthority`](resources/certificateauthority.md) | authentication, lifecycle | authentication, lifecycle | lifecycle |
| [`ExternalJWTSigner`](resources/externaljwtsigner.md) | authentication, lifecycle | authentication, lifecycle | lifecycle |

`go test ./apis/...` fails if a kind has no row in the `lifecycle` stage or
no example.

## What else the stages check

| Stage | Checks |
|-------|--------|
| `core` | A creation that was interrupted before the ID was saved continues with the entity it left behind. An entity that had the name before is not taken over and stays untouched. Enrollment tokens reach the connection secrets. |
| `drift` | After a deletion in Ziti, the service refers to the new config and the policy to the new service. An identity created anew has a new enrollment token. Lists that name an item twice settle. |
| `extended` | A `host.v2` config with two terminators, the four kinds of identities and their enrollments, an identity and its certificate authority deleted at the same time. |
| `posture_checks` | Hashes, fingerprints and MAC addresses in upper case and with separators, and lists out of order, settle without updates. A service policy refers to a posture check by name. |
| `authentication` | The certificate of a certificate authority cannot be changed. The owner of the certificate authority verifies it, and an identity enrolls with a certificate it issued. A signer changes from a JWKS endpoint to a certificate. Ziti keeps a signer that an auth policy refers to, and the signer goes once the policy is gone. |
| `renewal` | An enrollment token that expired is replaced, an enrollment that was deleted is created anew for every method and for an edge router, a new token can be used to enroll, and what has enrolled or can sign in without an authenticator is left alone. |
| `lifecycle` | A resource of every kind at once: drift, deletion in Ziti in one sweep, references that follow the new IDs (service to configs, policies to service and identity, auth policy to signer, identity to certificate authority), new enrollment tokens for the identities and the edge router created anew. |
| `scenarios` | The manifests of [`examples/scenarios`](../examples/scenarios) applied together: the client and the tunneler have access to the service, the service is available on the edge router, and the configs are stored as written. |
| `examples` | Every manifest under [`examples/`](../examples) but `provider/` and `composition/`, applied as it is in the repository and all at once: each resource becomes ready, and Ziti stores the `sticky` terminator strategy and the address translations as written. The stage takes whatever is in the directory, so an example cannot go untested. |
| `composition` | With Crossplane: the `PublishedService` of [`examples/composition`](../examples/composition) is composed into six entities, follows a change, gets its service back after a deletion in Ziti, and takes its entities with it when deleted. |

No stage prints an enrollment token, a password or a session token: tokens
are compared by digest, and diagnostics are redacted.

## Unit tests

`make test` runs the unit tests and the controller integration test, which
starts a Kubernetes API server with envtest. The reconciler of every kind
runs against an in-memory fake of the Ziti API that stores entities the way
Ziti does, including create, update, delete, drift, references, failures of
Ziti and of the network, and fuzzing of the comparison and of the reconciler.
The same package validates every example and every manifest of the README
against the CRDs.

## Running the end-to-end test

In a kind cluster, with docker, kind, kubectl, helm, go, curl, jq and
openssl:

```shell
make e2e.ziti                       # everything, with Crossplane
make build && make e2e.package      # the package, installed through Crossplane
test/e2e/e2e.sh up                  # or step by step: OpenZiti, kind, the provider
E2E_ONLY="lifecycle" test/e2e/e2e.sh test
test/e2e/e2e.sh logs
test/e2e/e2e.sh down
```

| Variable | Default | What it sets |
|----------|---------|--------------|
| `ZITI_VERSION` | `2.0.6` | The OpenZiti release `up` starts. |
| `E2E_CROSSPLANE` | `false`, `true` in `make e2e.ziti` | `up` installs Crossplane, which the `composition` stage needs. |
| `CROSSPLANE_VERSION` | `2.4.2` | The Crossplane release. |
| `E2E_ONLY` | all stages | The stages to run, separated by spaces. |
| `E2E_SKIP_EXTENDED` | `false` | Run only `core` and `drift`. |
| `E2E_TIMEOUT` | `180` | Seconds to wait for the provider to converge. |
| `E2E_POLL` | `10s` | How often the provider looks for drift. |
| `E2E_PACKAGE` | none, the package of `make build` in `make e2e.package` | A package file. `up` installs it through Crossplane instead of running the provider out-of-cluster. |
| `E2E_PACKAGE_STAGES` | `core scenarios` | The stages `make e2e.package` runs. |
| `E2E_PACKAGE_UPTIME` | `150` | Seconds the pod of the provider must have run without a restart. |
| `E2E_REGISTRY_PORT` | `5001` | The port of the host for the registry of the package. |
| `E2E_CROSSPLANE_CLI` | `crossplane` | The Crossplane CLI that pushes the package. |

`make e2e.package` tests what a user installs. It starts a registry on the
network of the kind cluster, pushes the package there, installs it with a
`Provider` and runs stages of the test against the provider that Crossplane
started. The provider gets no access but what Crossplane grants it. At the
end the test checks that the pod of the provider has run for two and a half
minutes without a restart: Crossplane reports a provider as healthy even
while its pod restarts, and a provider that may not read CRDs exits after two
minutes.

In an existing Kubernetes cluster, without installing anything into it
directly: [`test/e2e/vcluster`](../test/e2e/vcluster/README.md) runs the same
test inside a virtual cluster, next to an OpenZiti controller of its own.

`test/e2e/e2e.sh test` alone runs the checks against a provider that is
already running: put the kubeconfig of its cluster in
`$E2E_WORK_DIR/kubeconfig` and set `ZITI_URL`, `ZITI_USER` and `ZITI_PWD` for
the test controller.

Never point the test at a Ziti network you care about: it creates, changes
and deletes entities.

## CI

Every pull request runs the generators and fails on a difference
(`check-diff`), the linter, the unit tests, the build of the package, and the
end-to-end test in kind against OpenZiti 2.0.6. The `publish-artifacts`
job builds the package, then installs it through Crossplane and runs the
`core` and `scenarios` stages on it.
