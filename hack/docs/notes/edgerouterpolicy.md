## How it behaves

- Roles are `#attribute`, `#all` or `@name`; names are looked up and sent as `@id`, as in a [`ServicePolicy`](servicepolicy.md).
- An identity needs at least one edge router it may use, or it cannot connect to the network at all.
- Roles are compared as sets: their order and duplicates do not matter.

## When something goes wrong

- `no entity named "..." in ...` in `Synced`: an `@name` role matches no edge router or identity yet.
- An identity is enrolled but offline: no edge router policy gives it a router that is online.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
