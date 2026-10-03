## How it behaves

- The resource is ready when the certificate authority exists in Ziti as declared, verified or not. The provider cannot verify it: that takes a certificate signed with the private key of the certificate authority.
- `status.atProvider.verificationToken` is the common name the proof certificate must have. Sign a certificate with that common name with the key of the certificate authority and post it to `/edge/management/v1/cas/<id>/verify`. `status.atProvider.isVerified` tells whether it is done; until then Ziti does not accept certificates of the certificate authority.
- `certPem` cannot be changed after creation. `status.atProvider.fingerprint` tells which certificate Ziti holds.
- A certificate authority is replaced with `PUT`, because a `PATCH` drops its external ID claim. A setting that is left out of the spec is removed in Ziti or reset to its default.
- Deleting a certificate authority deletes the pending enrollments with its certificates. An [`IdentityCA`](identityca.md) that refers to it stays, but cannot enroll until the certificate authority exists again.
- A certificate authority that is deleted in Ziti is created anew and is not verified: it has to be verified again.

## When something goes wrong

- Ziti answers `400` about the certificate: `certPem` is not a CA certificate, or another certificate authority already has it.
- Identities cannot enroll or sign in with their certificates: the certificate authority is not verified, or `isAuthEnabled`, `isOttCaEnrollmentEnabled` or `isAutoCaEnrollmentEnabled` is off for what they try.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
