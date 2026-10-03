## How it behaves

- The identity is created with a one-time token enrollment. The enrollment JWT is written to the connection secret under `enrollmentToken`, and `status.atProvider.enrollmentExpiresAt` says until when it can be used.
- Once the identity has enrolled, Ziti no longer reports the token; the Secret keeps the last one and `status.atProvider.enrolled` is true.
- A token that has expired, or whose enrollment was deleted in Ziti, is replaced as long as the identity cannot sign in. The new token is valid for `enrollmentDuration`. An identity with an `externalId`, or under an auth policy that accepts external JWTs from a signer that looks identities up by their ID, counts as able to sign in and gets no new token.
- An identity that is deleted in Ziti is created anew under a new ID, with a new token: whatever enrolled with the old one has to enroll again.
- `authPolicyId` and the keys of `serviceHostingCosts` and `serviceHostingPrecedences` take names or IDs. Two keys that name the same service, by its name and by its ID, must have the same value.
- `tags`, `appData`, `serviceHostingCosts` and `serviceHostingPrecedences` are cleared in Ziti when they are left out of the spec. Other optional settings that are left out are not managed.
- `type` cannot be changed after creation.
- Before the identity is deleted its hosting settings per service are dropped: Ziti keeps them when a service is deleted and then refuses to delete the identity.

## When something goes wrong

- The Secret has no `enrollmentToken`: `spec.writeConnectionSecretToRef` is not set, or the identity had enrolled before the resource first saw it.
- `ziti edge enroll` refuses the token: it has expired. The provider replaces an expired token at the next poll; read the Secret again.
- `no entity named "..." in ...` in `Synced`: `authPolicyId` or a key of the hosting settings names something that does not exist yet.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
