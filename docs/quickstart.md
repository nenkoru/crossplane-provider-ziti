# Quick start

This publishes a PostgreSQL database in a Ziti network: clients of the
network reach `orders-db.ziti:5432`, and a tunneler next to the database
forwards them to `postgres.internal:5432`. It takes a Kubernetes cluster,
`kubectl`, Go and the credentials of an administrator of a Ziti controller.

There is no published package yet, so the provider runs on your machine
against the cluster.

## 1. A Ziti controller

Use the controller you have. To try the provider without one, start a
throwaway controller with Docker; its administrator is `admin` with the
password `admin`, and it answers at `https://localhost:1280`:

```shell
git clone --recurse-submodules https://github.com/nenkoru/crossplane-provider-ziti
cd crossplane-provider-ziti
ZITI_VERSION=2.0.6 docker compose -f ziti-docker-compose.yml up -d ziti-controller
```

## 2. The provider

Install the CRDs and start the provider. It uses the cluster of your current
`kubectl` context:

```shell
kubectl apply -f package/crds
go run ./cmd/provider --debug
```

Leave it running and continue in another terminal.

## 3. The connection to the controller

Put the URL of the controller and the credentials into
[`examples/provider/config.yaml`](../examples/provider/config.yaml) and apply
it. For the throwaway controller that is `https://localhost:1280`, `admin`,
`admin`, and `insecureSkipTLSVerify: true`, because its certificate comes
from a private PKI.

```shell
kubectl apply -f examples/provider/config.yaml
```

[ProviderConfig](resources/providerconfig.md) has all settings, including
certificate authentication and a CA bundle.

## 4. The service

[`examples/scenarios/publish-service.yaml`](../examples/scenarios/publish-service.yaml)
has everything the service takes:

| Resource | What it is for |
|----------|----------------|
| `ConfigHostV1` `orders-db-host` | Where the hosting tunneler sends the traffic: `postgres.internal:5432`. |
| `ConfigInterceptV1` `orders-db-intercept` | What client tunnelers intercept: `orders-db.ziti:5432`. |
| `Service` `orders-db` | The service, with both configs and the role attribute `databases`. |
| `ServicePolicy` `orders-db-dial` | Identities with the attribute `orders-db-clients` may connect. |
| `ServicePolicy` `orders-db-bind` | Identities with the attribute `orders-db-hosts` may host. |
| `Identity` `orders-api` | A client. |
| `Identity` `orders-db-tunnel` | The tunneler next to the database. |

```shell
kubectl apply -f examples/scenarios/publish-service.yaml
kubectl wait --for=condition=Ready --timeout=3m -f examples/scenarios/publish-service.yaml
kubectl get ziti
```

The order of the resources does not matter: one that refers to something
that does not exist yet reports it in its `Synced` condition and is retried.

`kubectl get ziti` shows the name of each entity in Ziti, its ID and whether
it is ready. If a resource does not get ready, its `Synced` condition says
why; see [Troubleshooting](troubleshooting.md).

## 5. Edge routers

Identities and services meet at edge routers. If the network has no policies
for that yet,
[`examples/scenarios/edge-router.yaml`](../examples/scenarios/edge-router.yaml)
adds an edge router `site-a-router`, lets every identity use the routers with
the attribute `site-a`, and makes the services with the attribute `databases`
available on them:

```shell
kubectl apply -f examples/scenarios/edge-router.yaml
```

## 6. Enrollment

Each identity and the router wrote a one-time enrollment token to a Secret:

```shell
kubectl get secret orders-api-enrollment -o jsonpath='{.data.enrollmentToken}' | base64 -d > orders-api.jwt
kubectl get secret orders-db-tunnel-enrollment -o jsonpath='{.data.enrollmentToken}' | base64 -d > orders-db-tunnel.jwt
kubectl get secret site-a-router-enrollment -o jsonpath='{.data.enrollmentToken}' | base64 -d > site-a-router.jwt
```

Enroll the tunnelers and the router with them, for example
`ziti edge enroll orders-api.jwt`. A token is valid for a few hours; the
provider replaces one that expired, so read the Secret again if enrollment
says the token is too old.

## 7. Changes

Change a manifest and apply it again; the entity in Ziti follows. A change
someone makes in Ziti directly is reverted at the next poll, and an entity
that is deleted there is created anew. [Concepts](concepts.md) says exactly
what is compared and what is left alone.

## 8. Cleaning up

Deleting the resources deletes the entities in Ziti:

```shell
kubectl delete -f examples/scenarios/edge-router.yaml -f examples/scenarios/publish-service.yaml
```

## Next

- [`examples/scenarios/configs.yaml`](../examples/scenarios/configs.yaml):
  intercept and host configs for a single port, a whole subnet, a wildcard
  domain with a health check, and two destinations behind one service.
- [`examples/composition`](../examples/composition): one `PublishedService`
  resource instead of six, with a Crossplane Composition.
- [Resources](resources/README.md): every kind and every setting.
