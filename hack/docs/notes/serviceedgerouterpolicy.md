## How it behaves

- Roles are `#attribute`, `#all` or `@name`; names are looked up and sent as `@id`, as in a [`ServicePolicy`](servicepolicy.md).
- A service is only reachable through the edge routers a policy of this kind gives it.
- Roles are compared as sets: their order and duplicates do not matter.

## When something goes wrong

- `no entity named "..." in ...` in `Synced`: an `@name` role matches no service or edge router yet.
- A client may dial the service and still gets no route: the client and the service have no edge router in common.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
