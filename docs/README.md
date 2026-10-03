# provider-ziti documentation

| Page | What is in it |
|------|---------------|
| [Quick start](quickstart.md) | From nothing to a service published in a Ziti network, in ten minutes. |
| [Concepts](concepts.md) | How a managed resource relates to its entity in Ziti: names, references, drift, enrollment tokens, deletion. |
| [Resources](resources/README.md) | A page per kind: example, every setting and what it does in Ziti, status, behaviour, errors. |
| [Fields and what they do in Ziti](fields.md) | The settings of all kinds on one page. |
| [Examples](../examples/README.md) | Manifests per kind, scenarios that belong together, and a Composition. |
| [Troubleshooting](troubleshooting.md) | The errors a resource reports and what to do about them. |
| [Testing](testing.md) | What the tests check for every kind, and how to run them in kind and in a virtual cluster. |

The pages under `resources/` and `fields.md` are generated from the CRDs, the
examples and the notes in `hack/docs/notes` by `make generate`.
