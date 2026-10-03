## How it behaves

- The device passes while an executable at `process.path` runs on the operating system `process.osType`. With `hashes` the executable must have one of them; with `signerFingerprint` it must be signed with that certificate.
- Ziti keeps hashes and the fingerprint in lower case without separators, and the hashes as a set. The spec may give them in either case and with separators.
- A `signerFingerprint` that is removed from the spec is removed in Ziti.
- A posture check does nothing on its own: name it in `postureCheckRoles` of a [`ServicePolicy`](servicepolicy.md), as `@name` or by a role attribute.

## When something goes wrong

- The API server rejects the manifest: `osType` is not one of the listed values, or a hash or the fingerprint has characters that are not hexadecimal digits or separators.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
