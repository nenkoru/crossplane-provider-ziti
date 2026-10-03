## How it behaves

- The identity passes while it has authenticated with TOTP. `timeoutSeconds` asks for it again after that long, `promptOnWake` and `promptOnUnlock` when the device wakes up or is unlocked.
- A posture check does nothing on its own: name it in `postureCheckRoles` of a [`ServicePolicy`](servicepolicy.md), as `@name` or by a role attribute.

## When something goes wrong

- Clients that do not support the timeout and the prompts fail the check: set `ignoreLegacyEndpoints`.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
