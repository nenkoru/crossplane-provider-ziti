## How it behaves

- `semantic` says whether all of the processes must run (`AllOf`) or any one (`AnyOf`).
- Ziti identifies a process by its operating system and path, and keeps the processes in that order. The API server rejects a pair that is listed twice.
- Hashes and fingerprints are kept as sets, in lower case without separators, as for a [`PostureCheckProcess`](posturecheckprocess.md).
- A posture check does nothing on its own: name it in `postureCheckRoles` of a [`ServicePolicy`](servicepolicy.md), as `@name` or by a role attribute.

## When something goes wrong

- The API server rejects the manifest: `processes` is empty, an `osType` is not one of the listed values, or the same operating system and path are listed twice.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
