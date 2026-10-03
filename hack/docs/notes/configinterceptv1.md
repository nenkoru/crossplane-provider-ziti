## How it behaves

- The settings become the `data` of a config of type `intercept.v1`. Ziti validates the data against the schema of the config type and refuses what does not fit.
- A config does nothing on its own: list it in `configs` of a [`Service`](service.md).
- `addresses`, `protocols` and `portRanges` keep the order of the spec; they are compared in that order.
- `dialOptions` that are removed from the spec are removed in Ziti.

## When something goes wrong

- Ziti answers `400` and names a property of the data: the value does not fit the schema of `intercept.v1`, for example a port range whose `low` is above its `high`, or an address that is neither a host name, an IP address nor a CIDR range.
- Clients do not intercept the address: the service needs a Dial [`ServicePolicy`](servicepolicy.md) for the identity, and tunnelers only pick up host names, not arbitrary URLs.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
