## How it behaves

- A method that is left out of `primary` is not allowed. Ziti wants all three methods described, and the provider sends them all.
- An auth policy is replaced with `PUT`, because a `PATCH` ignores some of the password settings. A setting that is left out of the spec is reset to the default of Ziti.
- `primary.extJwt.allowedSigners` and `secondary.requireExtJwtSigner` take names or IDs of [`ExternalJWTSigner`](externaljwtsigner.md) resources. An empty list of allowed signers allows every signer.
- Assign the policy to identities with `authPolicyId` of the identity kinds.
- When a signer the policy names is deleted in Ziti, the provider puts the new ID of the signer into the policy once the signer exists again.

## When something goes wrong

- `no entity named "..." in external-jwt-signers` in `Synced`: a signer the policy names does not exist yet.
- Deleting the policy is refused by Ziti and reported in `Synced`: identities or signers still refer to it. Assign them another policy first.
- Identities are locked out after the change: a policy that allows no method they have leaves them unable to sign in. Keep an administrator under the default policy.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
