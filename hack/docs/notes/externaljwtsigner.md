## How it behaves

- A signer verifies tokens either with the keys of `jwksEndpoint` or with the certificate in `certPem`, whose key ID is `kid`. Exactly one of the two is set.
- A signer is replaced with `PUT`: a `PATCH` cannot remove a setting, which a signer that changes from `jwksEndpoint` to `certPem` needs. A setting that is left out of the spec is removed in Ziti or reset to its default.
- Ziti ignores the tags of a signer that is being created; the provider sets them with an update right after.
- Ziti does not delete a signer that an auth policy refers to. The resource reports the conflict and is deleted once the policy is gone.
- A signer with `useExternalId: false` finds identities by their Ziti ID. While such a signer is enabled and allowed by an auth policy, the identities under that policy count as able to sign in, and their expired enrollment tokens are not replaced; see [`Identity`](identity.md).

## When something goes wrong

- `CAN_NOT_DELETE_REFERENCED_ENTITY` in `Synced` while deleting: an [`AuthPolicy`](authpolicy.md) still names the signer.
- Ziti answers `400` about the issuer: another signer has the same `issuer`.
- The API server rejects the manifest with `exactly one of jwksEndpoint and certPem is required`, or because `jwksEndpoint` is not an `http` or `https` URL with a host.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
