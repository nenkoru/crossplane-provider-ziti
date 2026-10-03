## How it behaves

- The identity enrolls by choosing a password for the username in `updbUsername`. The enrollment JWT is written to the connection secret under `enrollmentToken`.
- `updbUsername` cannot be changed after creation. Ziti shows the username only after the identity has enrolled.
- The auth policy of the identity must allow passwords (`primary.updb.allowed`), see [`AuthPolicy`](authpolicy.md); the default policy does.
- Token renewal works as for an [`Identity`](identity.md).
- `authPolicyId` and the keys of `serviceHostingCosts` and `serviceHostingPrecedences` take names or IDs. Two keys that name the same service, by its name and by its ID, must have the same value.
- `tags`, `appData`, `serviceHostingCosts` and `serviceHostingPrecedences` are cleared in Ziti when they are left out of the spec. Other optional settings that are left out are not managed.
- `type` cannot be changed after creation.
- Before the identity is deleted its hosting settings per service are dropped: Ziti keeps them when a service is deleted and then refuses to delete the identity.

## When something goes wrong

- Ziti refuses the password at enrollment: it does not meet the password rules of the auth policy.
- `no entity named "..." in auth-policies` in `Synced`: `authPolicyId` names a policy that does not exist yet.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
