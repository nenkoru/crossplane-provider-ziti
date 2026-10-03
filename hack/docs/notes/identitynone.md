## How it behaves

- The identity is created without an enrollment and never gets a token. It suits identities that sign in with a token of an [`ExternalJWTSigner`](externaljwtsigner.md), which finds them by `externalId` or by their ID, and identities of a [`CertificateAuthority`](certificateauthority.md) that takes the external ID from the certificate.
- `enrollmentDuration` has no effect on this kind.
- `authPolicyId` and the keys of `serviceHostingCosts` and `serviceHostingPrecedences` take names or IDs. Two keys that name the same service, by its name and by its ID, must have the same value.
- `tags`, `appData`, `serviceHostingCosts` and `serviceHostingPrecedences` are cleared in Ziti when they are left out of the spec. Other optional settings that are left out are not managed.
- `type` cannot be changed after creation.
- Before the identity is deleted its hosting settings per service are dropped: Ziti keeps them when a service is deleted and then refuses to delete the identity.

## When something goes wrong

- The identity cannot sign in: its auth policy must allow the method (`primary.extJwt` or `primary.cert`), and `externalId` must be what the signer finds in the claim it reads.
- `no entity named "..." in auth-policies` in `Synced`: `authPolicyId` names a policy that does not exist yet.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
