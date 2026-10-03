## How it behaves

- Ziti keeps the domains as a set: their order and duplicates do not matter, and `status.atProvider` shows them sorted.
- A posture check does nothing on its own: name it in `postureCheckRoles` of a [`ServicePolicy`](servicepolicy.md), as `@name` or by a role attribute.

## When something goes wrong

- Devices that are not Windows never pass this check. Combine it with other checks through role attributes and the semantic of the service policy.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
