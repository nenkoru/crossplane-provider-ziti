## How it behaves

- Ziti keeps the addresses as a set, in lower case and without separators. The spec may give them in either case and with `:`, `-`, `.` or spaces between the digits; the provider sends and compares them in the form Ziti stores, which is what `status.atProvider` shows.
- A posture check does nothing on its own: name it in `postureCheckRoles` of a [`ServicePolicy`](servicepolicy.md), as `@name` or by a role attribute.

## When something goes wrong

- The API server rejects the manifest: the list is empty, or an address has characters that are neither hexadecimal digits nor separators.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
