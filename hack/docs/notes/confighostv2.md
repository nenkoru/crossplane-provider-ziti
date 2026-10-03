## How it behaves

- The settings become the `data` of a config of type `host.v2`: a list of terminators, each with the settings of a [`ConfigHostV1`](confighostv1.md).
- The terminators keep the order of the spec and are compared in that order.
- A service takes either a `host.v1` or a `host.v2` config, not both.

## When something goes wrong

- Ziti answers `400` and names `terminators`: one of them does not fit the schema, with the same causes as for [`ConfigHostV1`](confighostv1.md).
- The API server rejects the manifest: `terminators` is empty, or an entry of `forwardAddressTranslations` mixes IPv4 with IPv6, has something other than a single IP address in `from` or `to`, or has a `prefixLength` above 32 for IPv4.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
