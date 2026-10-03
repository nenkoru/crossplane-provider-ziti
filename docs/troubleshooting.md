# Troubleshooting

## Where to look

A resource that is not as it should be says why in its conditions:

```shell
kubectl get ziti
kubectl get services.ziti.crossplane.io web-service -o jsonpath='{.status.conditions}'
kubectl describe identities.ziti.crossplane.io web-client
```

- `Synced` is `False`: the last reconcile failed, and the message is the
  error. The provider retries with a growing delay.
- `Ready` is `False`: the entity does not exist in Ziti yet, or the resource
  is being deleted.
- Both `True`: the entity is in Ziti as declared.

The kinds are named like kinds of Kubernetes and of other providers
(`Service`, `Identity`): use the full name, `services.ziti.crossplane.io`,
or the category, `kubectl get ziti`.

Start the provider with `--debug` for a log line per reconcile.

## Errors in the `Synced` condition

An error starts with what the provider was doing, followed by what went
wrong. An answer of Ziti reads
`ziti API error <status> <code>: <message> (cause: ...)`.

### The connection to the controller

| Message | Cause | What to do |
|---------|-------|------------|
| `cannot connect to the Ziti controller: cannot get ProviderConfig` or `cannot get ClusterProviderConfig` | `spec.providerConfigRef` names a configuration that does not exist. Without `providerConfigRef` the resource uses the `ClusterProviderConfig` named `default`. | Create the configuration, or set `kind` and `name` in `spec.providerConfigRef`. A `ProviderConfig` must be in the namespace of the resource. |
| `credentials.secretRef is required` | The configuration has `credentials` without `secretRef`. | Name the Secret. |
| `credentials.secretRef.namespace is required for a ClusterProviderConfig` | A `ClusterProviderConfig` has no namespace of its own to look for the Secret in. | Set `credentials.secretRef.namespace`. |
| `cannot get credentials secret` | The Secret does not exist in that namespace. | Create it, or fix the name. |
| `cannot parse the credentials key of the credentials secret` | The key `credentials` of the Secret is not a JSON object. | Use the keys `host`, `username`, `password`, `cert`, `key`, `ca` instead, or fix the JSON. |
| `host must be an https URL` | `host` has another scheme or none. | Use `https://host:port`. |
| `either username and password or cert and key are required` | Neither pair is complete. | Set both of one pair, in the Secret or in the spec. |
| `cannot authenticate to the Ziti controller: 401 ...` | The controller does not accept the credentials. | Check the username and the password, or that the certificate belongs to an identity that is an administrator. |
| `x509: certificate signed by unknown authority` | The certificate of the controller is not issued by a CA the provider trusts. | Set `ca` to the CA bundle of the controller. `insecureSkipTLSVerify` is for test controllers only. |
| `connection refused`, `no such host`, `context deadline exceeded` | The provider cannot reach the controller from where it runs. | Check `host`, DNS and the network path from the provider. |

### References

| Message | Cause | What to do |
|---------|-------|------------|
| `cannot determine the desired state: no entity named "x" in <collection>` | The resource refers by name to an entity that is not in Ziti: a config of a service, an `@name` role of a policy, `ottca`, `authPolicyId`, a signer of an auth policy, a service in the hosting settings of an identity. | Nothing, if the other resource is being created: the reference is retried. Otherwise fix the name; it is `spec.forProvider.name` of the other resource. |
| `"a" and "b" name the same entity in services, with different settings` | Two keys of `serviceHostingCosts` or `serviceHostingPrecedences` name one service, by its name and by its ID, with different values. | Keep one of the keys. |

### Answers of Ziti

| Message | Cause | What to do |
|---------|-------|------------|
| `cannot create the external resource: ziti API error 400 COULD_NOT_VALIDATE ...` whose cause names the field `name` | An entity with that name exists and was not created by this resource. The provider never takes over an entity by its name. | To adopt the entity, set the annotation `crossplane.io/external-name` of the resource to its ID. Otherwise rename one of them or delete the entity in Ziti. |
| `ziti API error 400 ...` that names another field, on create or update | Ziti does not accept the value: the data of a config does not fit the schema of its type, a certificate is not a CA certificate, an issuer is used twice. The cause names the field. | Fix the value. The page of the [kind](resources/README.md) lists the usual causes. |
| `cannot delete the external resource: ziti API error 409 ...` with `CAN_NOT_DELETE_REFERENCED_ENTITY` | Another entity refers to the one being deleted: an auth policy to an external JWT signer, an identity to an auth policy. | Delete or change the other resource; the deletion is retried and goes through once nothing refers to the entity. |
| `ziti API error 401` in the middle of operation | The API session of the provider expired or was removed. | Nothing: the provider signs in again and retries. |
| `ziti API error 500`, `502`, `503`, `504`, `408`, `429`, closed connections, timeouts | The controller or the network had a problem. | Nothing: the request is retried, and a creation that got no answer is settled without creating the entity twice, see [Concepts](concepts.md#how-it-behaves). |

## Manifests the API server rejects

`kubectl apply` fails before the provider sees the resource:

| Message | Cause |
|---------|-------|
| `type cannot be changed after creation` | The `type` of an identity is fixed when it is created. Delete the resource and create it anew. |
| `ottca cannot be changed after creation`, `updbUsername cannot be changed after creation`, `certPem cannot be changed after creation` | These settings are part of how the entity was created. Delete the resource and create it anew. |
| `enrollmentDuration must be at least 5m` | A shorter token could expire before it is used. |
| `exactly one of jwksEndpoint and certPem is required` | An external JWT signer verifies tokens with one or the other. |
| `the SCHEME matcher is for the SAN_URI location`, `matcherCriteria is required ...`, `parserCriteria is required ...` | The parts of `externalIdClaim` of a certificate authority do not fit together. |
| `Duplicate value` | An operating system type of a `PostureCheckOS`, or an operating system and path of a `PostureCheckMultiProcess`, is listed twice. Ziti keeps one entry for each. |
| `Unsupported value`, `should be greater than or equal to`, `should match` | A value is not one of those the field allows; the page of the kind lists them. |
| `unknown field` | The manifest has a field the kind does not have, usually a typo or a field at the wrong level. |

## Things that look wrong and are not

- **The resource is ready, and the entity has another ID than before.** The
  entity was deleted in Ziti and created anew. Whatever enrolled with the
  old identity or router has to enroll again with the new token in the
  Secret.
- **A deleted entity does not come back at once.** Crossplane waits 30
  seconds after it created an entity before it creates one again that it
  does not find, and the provider notices the deletion at its next poll.
- **A setting changed in Ziti is not reverted.** Optional settings that are
  left out of the spec are not managed, except for the kinds that are
  updated with `PUT`; the page of the kind says which it is.
- **`status.atProvider` shows roles as `@id` and hashes in lower case.** That
  is how Ziti stores them. The spec keeps the names and the notation you
  wrote.
- **The Secret has no `enrollmentToken`.** `spec.writeConnectionSecretToRef`
  is not set, or the identity or router had enrolled before the resource
  first saw it. Ziti does not report a token that was used.
- **An expired token is not replaced.** The identity can sign in without it:
  it has an authenticator, an `externalId`, or an auth policy that accepts
  external JWTs from a signer that finds identities by their ID. An edge
  router that has enrolled is never enrolled anew.

## A resource that cannot be deleted

A resource keeps its finalizer until its entity is gone from Ziti. If the
deletion hangs, the `Synced` condition says why: Ziti refuses it, or the
controller cannot be reached.

To remove the resource and keep the entity, take `Delete` out of
`spec.managementPolicies`, for example
`["Observe", "Create", "Update", "LateInitialize"]`, and delete the resource
again.

If the controller is gone for good, remove the finalizer by hand:

```shell
kubectl patch services.ziti.crossplane.io web-service --type merge -p '{"metadata": {"finalizers": []}}'
```
