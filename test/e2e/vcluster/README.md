# The end-to-end test in a virtual cluster

`test/e2e/e2e.sh all` needs Docker and kind. This directory runs the same
test in an existing Kubernetes cluster without installing anything into it
directly: CRDs, Crossplane and cluster roles go into a virtual cluster
([vcluster](https://www.vcluster.com)) that lives in one namespace of the
host cluster.

```
host cluster
└── namespace provider-ziti-e2e
    └── virtual cluster
        ├── OpenZiti controller, new with every run
        ├── Crossplane and function-patch-and-transform
        ├── CRDs of the provider
        └── e2e-runner: the provider and test/e2e/e2e.sh
```

The host cluster sees only pods, services, secrets and config maps of the
namespace. The virtual cluster keeps no state: restarting its pod gives a
new, empty cluster.

## Running it

`kubectl` points at the host cluster. The namespace must exist;
[`namespace.yaml`](namespace.yaml) creates one with a quota, default limits
and a network policy, for a cluster that is shared with other workloads.

```shell
kubectl apply -f test/e2e/vcluster/namespace.yaml
test/e2e/vcluster/bed.sh up     # the virtual cluster, OpenZiti and the runner
test/e2e/vcluster/bed.sh run    # build the provider, copy it into the runner, run the test
test/e2e/vcluster/bed.sh down   # remove everything but the namespace
```

`run` can be repeated: it starts from a new OpenZiti network and removes the
resources an earlier run left behind. It prints the same output as
`test/e2e/e2e.sh test` and ends with `All end-to-end checks passed`.

| Variable | Default | What it sets |
|----------|---------|--------------|
| `E2E_NAMESPACE` | `provider-ziti-e2e` | The namespace of the host cluster. |
| `E2E_PRIORITY_CLASS` | none | A priority class of the host cluster for all pods of the bed, for example a low one in a shared cluster. |
| `E2E_GOARCH` | `amd64` | The architecture of the nodes. |
| `E2E_CROSSPLANE` | `true` | Install Crossplane and test the Composition. |
| `E2E_RESET` | `true` | Start `run` from a new OpenZiti network. |
| `E2E_TIMEOUT` | `180` | Seconds to wait for the provider to converge. |

## What it needs from the host cluster

- Permission to create deployments, services, secrets, config maps, roles and
  role bindings in the namespace, and to `exec` and `port-forward` there.
  With `E2E_PRIORITY_CLASS`, also to read that priority class.
- About 1 CPU and 2 GiB of memory in requests, for seven pods.
- Nodes that can pull from `ghcr.io`, `docker.io` and `xpkg.crossplane.io`.

The provider is only ever connected to the OpenZiti controller of the bed.
Never point the test at a Ziti network you care about: it creates, changes
and deletes entities.
