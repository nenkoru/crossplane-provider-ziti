## How it behaves

- The enrollment JWT of the router is written to the connection secret under `enrollmentToken` until the router enrolls, and `status.atProvider.enrollmentExpiresAt` says until when it can be used. `status.atProvider.isVerified` is true once the router has enrolled.
- A router whose enrollment has expired or was deleted in Ziti, and that has not enrolled, is enrolled anew, which makes a new token. A router that has enrolled is never enrolled anew: that would take its certificate away and disconnect it.
- A router that is deleted in Ziti is created anew under a new ID with a new token. The router process has to enroll again with it.
- With `isTunnelerEnabled` Ziti creates an identity with the name of the router. That identity belongs to the router and is not a managed resource.

## When something goes wrong

- `ziti router enroll` refuses the token: it has expired. The provider replaces it at the next poll; read the Secret again.
- The router is enrolled but identities do not use it: it needs a role attribute or name that an [`EdgeRouterPolicy`](edgerouterpolicy.md) selects.

Create, update, drift, deletion in Ziti and deletion of the resource are covered by the end-to-end test, see [Testing](../testing.md).
