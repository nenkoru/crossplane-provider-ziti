## How it behaves

- Ziti keeps one entry per operating system type, in the order of the types, and the versions of an entry as a set. The spec may list them in any order; the API server rejects a type that is listed twice.
- A posture check does nothing on its own: name it in `postureCheckRoles` of a [`ServicePolicy`](servicepolicy.md), as `@name` or by a role attribute.

## When something goes wrong

- Ziti answers `400` about a version: versions are semantic version constraints such as `>=13.0.0`.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
