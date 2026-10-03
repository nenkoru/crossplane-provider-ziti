## How it behaves

- The identity enrolls with a one-time token and a client certificate issued by the certificate authority `ottca` names, see [`CertificateAuthority`](certificateauthority.md). The token is written to the connection secret under `enrollmentToken`.
- `ottca` takes the name or the ID of the certificate authority and cannot be changed after creation.
- When the certificate authority is deleted, Ziti deletes the pending enrollments with it. The identity stays; once a certificate authority with that name exists again, the provider gives the identity a new enrollment with it and a new token.
- Token renewal works as for an [`Identity`](identity.md).
- `authPolicyId` and the keys of `serviceHostingCosts` and `serviceHostingPrecedences` take names or IDs. Two keys that name the same service, by its name and by its ID, must have the same value.
- `tags`, `appData`, `serviceHostingCosts` and `serviceHostingPrecedences` are cleared in Ziti when they are left out of the spec. Other optional settings that are left out are not managed.
- `type` cannot be changed after creation.
- Before the identity is deleted its hosting settings per service are dropped: Ziti keeps them when a service is deleted and then refuses to delete the identity.

## When something goes wrong

- `no entity named "..." in cas` in `Synced`: no certificate authority has the name in `ottca` yet.
- Enrollment is refused although the token is valid: the certificate authority is not verified, or `isOttCaEnrollmentEnabled` is off, or the client certificate is not issued by it.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
