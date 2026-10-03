## How it behaves

- Roles are `#attribute`, `#all` or `@name`. A name is looked up in Ziti and sent as `@id`; an ID is accepted as well. Ziti stores IDs, so `status.atProvider` shows `@id`.
- When an entity a role names is deleted in Ziti, Ziti takes the role out of the policy. The provider puts it back once an entity with that name exists again, with its new ID.
- A role that names something that does not exist yet is reported in `Synced` and retried, so policies can be applied together with what they refer to.
- `semantic` says how the `#attribute` roles of one list combine.
- Roles are compared as sets: their order and duplicates do not matter.

## When something goes wrong

- `no entity named "..." in ...` in `Synced`: an `@name` role matches nothing in Ziti yet.
- The policy is ready but the identity still cannot dial: check that the identity and the service share an edge router through an [`EdgeRouterPolicy`](edgerouterpolicy.md) and a [`ServiceEdgeRouterPolicy`](serviceedgerouterpolicy.md), and that the identity passes the posture checks in `postureCheckRoles`.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
