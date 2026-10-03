# Concepts

How a managed resource of this provider relates to its entity in Ziti. The
pages of the [kinds](resources/README.md) say what is particular to each.

## What every managed resource has

| Field | What it does |
|-------|--------------|
| `metadata.name` | The name of the resource in Kubernetes. It does not have to be the name in Ziti, but the examples keep them equal. |
| `spec.forProvider.name` | The name of the entity in Ziti. References from other resources (`configs`, `@name` roles, `ottca`, `authPolicyId`) use this name. |
| `spec.providerConfigRef` | The [`ProviderConfig` or `ClusterProviderConfig`](resources/providerconfig.md) that says which Ziti controller the entity lives in. Defaults to the `ClusterProviderConfig` named `default`. |
| `spec.managementPolicies` | What the provider may do with the entity. `["*"]` is the default; leave `Delete` out to keep the entity when the resource is deleted, or use `["Observe"]` to only watch an entity. |
| `spec.writeConnectionSecretToRef` | The Secret, in the namespace of the resource, that gets the enrollment token of an identity or edge router under the key `enrollmentToken`. |
| `metadata.annotations["crossplane.io/external-name"]` | The ID of the entity in Ziti. The provider sets it after creation; set it yourself to adopt an existing entity. |
| `status.atProvider` | The entity as Ziti reports it. |
| `status.conditions` | `Ready` is true when the entity exists in Ziti. `Synced` is true when the last reconcile succeeded; when it is false its message has the error, see [Troubleshooting](troubleshooting.md). |

Every kind is in the categories `managed` and `ziti`: `kubectl get ziti`
lists all of them with their name in Ziti, their ID and whether they are
ready.

## How it behaves

- **Identity of an entity.** The `crossplane.io/external-name` annotation holds
  the Ziti ID. It is empty until the entity is created. To adopt an existing
  entity, create the managed resource with the annotation set to its ID. An
  entity that merely has the same name is never taken over: creating the
  managed resource fails with the name conflict Ziti reports.
- **Creation with an unknown result.** The provider may stop after Ziti
  created the entity and before the ID was saved, or the answer of Ziti may
  get lost on the way. The next reconcile then finds the entity by its name
  and continues with it, provided Ziti created it after that creation
  started; the clocks of the controller and the provider are compared for
  this, to within five seconds. An entity that had the name before is left
  alone: the resource reports the name conflict and can be deleted without
  touching it. A request that was not answered stays on record in the
  `ziti.crossplane.io/create-unconfirmed` annotation until it is settled.
- **Drift.** Every field the provider sends is compared with what Ziti
  reports, on every poll (one minute by default, `--poll`). A change made in
  Ziti directly is reverted. Optional settings that are left unset in the
  spec are not managed, except lists and maps (`tags`, and `appData`,
  `serviceHostingCosts` and `serviceHostingPrecedences` of an identity),
  which are cleared when unset. Lists of strings at the top level of an
  entity, such as roles and role attributes, are compared as sets, as Ziti
  stores them: their order and duplicates do not matter. See
  [What happens when someone edits Ziti directly](#what-happens-when-someone-edits-ziti-directly).
- **References by name.** `Service.configs` takes config names or IDs, and so
  do `authPolicyId` and the keys of `serviceHostingCosts` and
  `serviceHostingPrecedences` of an identity, and `IdentityCA.ottca`. Policy
  roles take `#attribute`, `#all` or `@name`, where the name is resolved to an
  ID. A resource that refers to something that does not exist yet reports the
  error in its `Synced` condition and is retried. Two keys of
  `serviceHostingCosts` or `serviceHostingPrecedences` that name the same
  service, by its name and by its ID, must have the same value; otherwise
  the resource reports the conflict in its `Synced` condition and nothing is
  sent to Ziti.
- **Certificate authorities.** A `CertificateAuthority` is ready when it
  exists in Ziti as declared, verified or not. The provider cannot verify it:
  that takes a certificate signed with the private key of the certificate
  authority. Its status has what the owner of that key needs:
  `verificationToken` is the common name the certificate must have, to be
  posted to `/edge/management/v1/cas/<id>/verify`, and `isVerified` tells
  whether it is done. Until then Ziti does not accept certificates of the
  certificate authority for authentication. `fingerprint` tells which
  certificate Ziti holds. Deleting a certificate authority deletes the pending
  enrollments with its certificates: an `IdentityCA` that refers to it stays,
  but can no longer enroll.
- **External JWT signers.** An `AuthPolicy` takes names or IDs of signers in
  `primary.extJwt.allowedSigners` and `secondary.requireExtJwtSigner`, and an
  `ExternalJWTSigner` the name or ID of an auth policy in
  `enrollAuthPolicyId`. Ziti does not delete a signer that an auth policy
  refers to: the `ExternalJWTSigner` reports the conflict and is deleted once
  the policy is gone. Ziti ignores the tags of a signer that is being created;
  they follow with an update right after.
- **Identities.** The four identity kinds take the same settings and differ
  in how the identity enrolls: `Identity` with a one-time token, `IdentityCA`
  with a one-time token and a certificate of a third-party CA,
  `IdentityUPDB` by choosing a password, `IdentityNone` not at all.
- **Posture checks.** Ziti does not store the values of a posture check as
  they are sent: domains, MAC addresses, hashes, fingerprints and the
  versions of an operating system are kept as sets, and MAC addresses,
  hashes and fingerprints in lower case without separators. The spec may
  give them in any order, in either case and with `:`, `-`, `.` or spaces
  between the digits; the provider sends and compares them in the form Ziti
  stores, which is also what `status.atProvider` shows. Ziti keeps one
  entry per operating system type of a `PostureCheckOS` and per operating
  system and path of a `PostureCheckMultiProcess`, in their order, so the
  API server rejects an entry that is listed twice.
- **Enrollment tokens.** `Identity`, `IdentityCA`, `IdentityUPDB` and
  `EdgeRouter` write their enrollment JWT to the Secret named in
  `spec.writeConnectionSecretToRef`, under the key `enrollmentToken`, and
  show when it expires in `status.atProvider.enrollmentExpiresAt`. Ziti
  stops reporting the token once it is used; the Secret keeps the last one.
- **Renewal of enrollment tokens.** A token that can no longer be used is
  replaced, in Ziti and in the Secret, as long as the identity cannot sign
  in or the edge router has not enrolled: when its enrollment has expired,
  and when it has none because the enrollment was deleted in Ziti. An
  enrollment that has expired is refreshed, a missing one is created with
  the method of the kind, and an edge router is enrolled anew.
  - An identity counts as able to sign in, and is never given a token, if
    Ziti reports an authenticator for it, if it has an `externalId`, or if
    its auth policy allows external JWTs as the first factor from a signer
    that is enabled and has `useExternalId: false`, which looks identities
    up by their Ziti ID. Ziti reports no authenticator for an identity that
    signs in with an external JWT, nor for one that signs in with a
    certificate of a certificate authority that carries its external ID.
    The default auth policy allows every signer, so one such signer stops
    renewal for every identity under that policy.
  - It never happens to an edge router that is verified or has a
    certificate: it has enrolled, and enrolling it anew would take its
    certificate away and disconnect it. Nothing is renewed either when Ziti
    does not report the authenticators of an identity or whether an edge
    router is verified. An identity whose authenticators were all deleted in
    Ziti, and that cannot sign in otherwise, gets a token again.
  - A token is replaced only once it has expired on the clock of the
    controller, five seconds ago or more, never while it can still be used.
  - The new token of an identity is valid for
    `spec.forProvider.enrollmentDuration`, which is `180m` unless set, the
    default of Ziti; the minimum is `5m`. The first token and every token of
    an edge router are valid for as long as the controller is configured to
    make them.
  - A renewal is an update: it follows `spec.managementPolicies`, and one
    that fails is reported in the `Synced` condition and retried.
- **Deletion.** Deleting a managed resource deletes the Ziti entity. Set
  `spec.managementPolicies` to keep it. The hosting settings an identity has
  per service are dropped first: Ziti keeps them when a service is deleted
  and then refuses to delete the identity.
- **Immutable settings.** The `type` of an identity, `IdentityCA.ottca`,
  `IdentityUPDB.updbUsername` and `CertificateAuthority.certPem` cannot be
  changed after creation; the API server rejects the change.
- **Updates.** Entities are updated with `PATCH`, so settings the provider
  does not manage are left alone. `Service` and `AuthPolicy` are replaced
  with `PUT` instead: a `PATCH` of a service ignores `encryptionRequired`, and
  a `PATCH` of an auth policy ignores some password settings.
  `CertificateAuthority` and `ExternalJWTSigner` are replaced as well: a
  `PATCH` of a certificate authority drops its external ID claim, and a
  `PATCH` cannot remove a setting, which a signer that changes from
  `jwksEndpoint` to `certPem` needs. A setting of these two kinds that is
  left unset in the spec is therefore not left alone: it is removed in Ziti
  or reset to the default of Ziti.

## What happens when someone edits Ziti directly

The provider compares every entity with its managed resource on every poll
and puts it back as declared. The unit tests run the reconciler against a
fake of the Ziti API that stores entities the way Ziti does, and the drift
stage of the end-to-end test does the same against OpenZiti:

- **A changed setting is reverted** with an update at the next poll, for
  every field the spec manages: scalars, lists, roles and role attributes,
  tags and other maps, and the data of configs. Lists of strings are
  compared as sets; lists in the data of a config keep their order.
- **What Ziti owns is left alone**: timestamps, links, whether an identity
  or edge router has enrolled and a certificate authority is verified, and
  the settings the spec leaves unset, with the exceptions listed under
  **Drift** above.
- **A deleted entity is created anew** at the next poll, under a new ID,
  which the managed resource records in its external name. Ziti removes the
  references to a deleted entity from policies (`@id` roles) and services
  (configs); the resources that refer to it by name refer to the new ID
  once it exists, and report the missing entity until then. An identity or
  edge router created anew has a new enrollment token, which replaces the
  old one in its connection Secret: whatever enrolled with the deleted
  entity has to enroll again. Crossplane waits 30 seconds after it created
  an entity before it creates one again that it does not find.
- **An entity created by hand in its place is not taken over.** If someone
  deletes the entity and creates another one with the same name, the
  managed resource reports the name conflict Ziti answers, is not ready, and
  leaves the other entity alone. Set its `crossplane.io/external-name`
  annotation to the ID of that entity to adopt it, or delete the entity.
- **Enrollments.** Once an identity or edge router has enrolled, Ziti no
  longer reports its token, and the Secret keeps the last one. An
  enrollment that is deleted or has expired is replaced as described under
  **Renewal of enrollment tokens**.
- **Deletion.** A managed resource whose entity is already gone is deleted
  without an error. A deletion that Ziti refuses, because another entity
  refers to the one being deleted, is reported and retried.
- **Errors of Ziti or of the network**, such as answers with the status
  500, 502, 503, 504, 408 or 429, closed connections and timeouts, are
  reported in the `Synced` condition and retried. A managed resource that is
  ready and synced after a reconcile has its entity as declared, and no
  entity is lost or created twice: a creation that got no answer is settled
  as described under **Creation with an unknown result**.
