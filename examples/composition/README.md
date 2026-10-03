# A published service as one resource

A team that wants an address reachable in the Ziti network should not have
to write six resources. With this Composition it writes one:

```yaml
apiVersion: platform.example.org/v1alpha1
kind: PublishedService
metadata:
  name: wiki
  namespace: default
spec:
  upstream:
    address: wiki.internal
    port: 8443
  intercept:
    address: wiki.ziti
    port: 443
```

Crossplane composes it into:

| Resource | Name in Ziti | From |
|----------|--------------|------|
| `ConfigHostV1` | `wiki-host` | `spec.upstream` |
| `ConfigInterceptV1` | `wiki-intercept` | `spec.intercept` |
| `Service` | `wiki` | both configs, by name |
| `ServicePolicy`, Dial | `wiki-dial` | identities with the role attribute `wiki-clients` |
| `ServicePolicy`, Bind | `wiki-bind` | identities with the role attribute `wiki-hosts` |
| `ServiceEdgeRouterPolicy` | `wiki-routers` | `spec.edgeRouterRole`, `#all` unless set |

Giving an identity the attribute `wiki-clients` lets it connect; one with
`wiki-hosts` hosts the service. `status.serviceId` of the `PublishedService`
has the ID of the service in Ziti.

## Files

| File | What it is |
|------|------------|
| `function.yaml` | `function-patch-and-transform`, which the Composition runs. |
| `definition.yaml` | The `CompositeResourceDefinition` of `PublishedService`. |
| `composition.yaml` | The Composition. |
| `publishedservice.yaml` | The published service above. |

## Trying it

This takes Crossplane 2 in the cluster, the provider running, and a
`ProviderConfig` named `default` in the namespace of the published service,
see the [quick start](../../docs/quickstart.md).

```shell
kubectl apply -f examples/composition/function.yaml -f examples/composition/definition.yaml -f examples/composition/composition.yaml
kubectl get functionrevisions   # wait until RUNTIME-HEALTHY is True
kubectl apply -f examples/composition/publishedservice.yaml
kubectl get publishedservices,ziti
```

While the provider is not installed as a Crossplane package, Crossplane has
no permission to manage its kinds. Grant it:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: provider-ziti:aggregate-to-crossplane
  labels:
    rbac.crossplane.io/aggregate-to-crossplane: "true"
rules:
  - apiGroups: ["ziti.crossplane.io"]
    resources: ["*"]
    verbs: ["*"]
```

The end-to-end test applies these files, changes the published service,
deletes the composed service in Ziti and deletes the published service, and
checks Ziti after each step.

## Making it yours

- The group `platform.example.org` and the kind are placeholders: rename
  them in `definition.yaml` and `composition.yaml`.
- The composed resources use the `ProviderConfig` named `default`. To let the
  published service choose, add a field to the definition and patch
  `spec.providerConfigRef.name` from it.
- Composed resources are named by Crossplane; their names in Ziti come from
  `spec.forProvider.name`, which the Composition derives from the name of the
  published service. Two published services with the same name in different
  namespaces would ask for the same names in Ziti, and the second one would
  report the name conflict.
