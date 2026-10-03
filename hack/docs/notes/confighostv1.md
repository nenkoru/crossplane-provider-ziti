## How it behaves

- The settings become the `data` of a config of type `host.v1`. Ziti validates the data against the schema of the config type.
- `address`, `port` and `protocol` are each either set or forwarded: with `forwardAddress`, `forwardPort` or `forwardProtocol`, leave the fixed value out and list what may be forwarded in `allowedAddresses`, `allowedPortRanges` or `allowedProtocols`.
- A config does nothing on its own: list it in `configs` of a [`Service`](service.md), and let an identity host the service with a Bind [`ServicePolicy`](servicepolicy.md).
- For several destinations behind one service use [`ConfigHostV2`](confighostv2.md).

## When something goes wrong

- Ziti answers `400` and names a property of the data: the value does not fit the schema of `host.v1`. The usual causes are a fixed `address` together with `forwardAddress`, a forwarded value without its `allowed...` list, and a health check action without `duration`.
- The API server rejects the manifest: `port` is outside 1 to 65535, or `protocol`, `listenOptions.precedence` or a check `trigger` is not one of the listed values.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
