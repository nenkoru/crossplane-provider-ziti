## How it behaves

- `configs` takes names or IDs of configs. A config that does not exist yet is reported in the `Synced` condition and retried, so a service and its configs can be applied together.
- A service is replaced with `PUT`, because a `PATCH` of a service ignores `encryptionRequired`. A setting that is left out of the spec is therefore reset to the default of Ziti.
- When a config of the service is deleted in Ziti, Ziti takes it out of the service. The provider creates the config anew and puts its new ID back into the service.
- Deleting the service in Ziti drops the terminators its hosts created; they come back when the hosts bind the new service.

## When something goes wrong

- `no entity named "..." in configs` in `Synced`: a name in `configs` matches no config. Check the name of the config in Ziti, which is `spec.forProvider.name` of the config resource, not its `metadata.name`.
- Ziti answers `400` with `configs` in the message: two configs of the same type are listed. A service takes one `intercept.v1` and one of `host.v1` or `host.v2`.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
