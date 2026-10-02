# provider-ziti

`provider-ziti` is a [Crossplane](https://crossplane.io/) Provider that manages
[OpenZiti](https://openziti.io/) resources on Kubernetes. It allows you to define
Ziti services, edge routers, policies, configurations, identities, and posture
checks as native Kubernetes resources.

## Resources

| Kind | Description |
|------|-------------|
| `Service` | Ziti service definitions |
| `EdgeRouter` | Edge router configurations |
| `ServicePolicy` | Service ↔ identity binding policies |
| `ServiceEdgeRouterPolicy` | Service ↔ edge router binding policies |
| `EdgeRouterPolicy` | Edge router ↔ identity binding policies |
| `ConfigHostV1` | Host configuration (V1) |
| `ConfigHostV2` | Host configuration (V2) |
| `ConfigInterceptV1` | Intercept configuration |
| `Identity` | Service identity (OTT enrollment) |
| `IdentityCA` | Service identity (CA enrollment) |
| `IdentityUPDB` | Service identity (UPDB enrollment) |
| `IdentityNone` | Service identity (no enrollment) |
| `PostureCheckDomain` | Domain posture check |
| `PostureCheckMac` | MAC address posture check |
| `PostureCheckMFA` | MFA posture check |
| `PostureCheckOS` | OS posture check |
| `PostureCheckProcess` | Process posture check |
| `PostureCheckMultiProcess` | Multi-process posture check |
| `AuthPolicy` | Authentication policy |
| `CertificateAuthority` | Certificate authority |
| `ExternalJWTSigner` | External JWT signer |

## Quick Start

1. Install the provider package:

```shell
up xpkg install xpkg.upbound.io/crossplane/provider-ziti
```

1. Create a `ProviderConfig`:

```yaml
apiVersion: ziti.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  host: https://controller.example.com:441
  username: admin
  password: supersecret
```

1. Create Ziti resources:

```yaml
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: my-service
spec:
  forProvider:
    name: my-service
    encryptionRequired: false
    terminatorStrategy: Closest
  providerConfigRef:
    name: default
```
