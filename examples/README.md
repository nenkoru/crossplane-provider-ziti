# Ziti Provider Examples

This directory contains example manifests for using the Crossplane Ziti provider.

## Directory Structure

```
examples/
├── provider/
│   └── config.yaml          # ProviderConfig examples
└── service/
    ├── minimal.yaml         # Minimal service with host config
    ├── service.yaml         # Standard service with host + intercept configs
    └── advanced.yaml        # Full-featured service with all options
```

## Quick Start

### 1. Apply ProviderConfig

```bash
kubectl apply -f examples/provider/config.yaml
```

### 2. Apply Service Examples

#### Minimal Service (host config only)

```bash
kubectl apply -f examples/service/minimal.yaml
```

#### Standard Service (host + intercept configs)

```bash
kubectl apply -f examples/service/service.yaml
```

#### Advanced Service (all features)

```bash
kubectl apply -f examples/service/advanced.yaml
```

## Resource Reference

### ConfigHostV1

Defines how a service is hosted (terminator configuration).

Key fields:

- `address`: Target host address
- `port`: Target port
- `protocol`: tcp/udp
- `forwardProtocol/forwardPort/forwardAddress`: Forward intercepted values
- `allowedProtocols/allowedAddresses/allowedSourceAddresses`: Filtering
- `forwardAddressTranslations`: NAT translations
- `allowedPortRanges`: Port ranges to allow
- `listenOptions`: Terminator listen options (cost, max connections, etc.)
- `proxy`: Outbound proxy configuration
- `httpChecks`: HTTP health checks
- `portChecks`: Port health checks

### ConfigInterceptV1

Defines how client traffic is intercepted.

Key fields:

- `addresses`: Domain addresses to intercept
- `protocols`: Protocols to intercept (tcp/udp)
- `portRanges`: Port ranges to intercept
- `dialOptions`: Connection dial options (timeout, identity)
- `sourceIp`: Source IP for outbound connections
- `matchDomainStrategy`: Domain matching strategy

### Service

Defines a Ziti service with associated configs.

Key fields:

- `name`: Service name
- `encryptionRequired`: Require encryption (default: true)
- `maxIdleTimeMillis`: Max idle time in milliseconds
- `terminatorStrategy`: smartrouting/weighted/random/ha
- `configs`: List of config names (host or intercept)
- `roleAttributes`: Role attributes for policy
- `tags`: Metadata tags

## ProviderConfig

The ProviderConfig specifies how to connect to the Ziti controller:

```yaml
apiVersion: ziti.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  host: https://ziti-controller:1280
  username: admin
  password: admin
  # OR use certificate auth:
  # cert: |
  #   -----BEGIN CERTIFICATE-----
  #   ...
  #   -----END CERTIFICATE-----
  # key: |
  #   -----BEGIN PRIVATE KEY-----
  #   ...
  #   -----END PRIVATE KEY-----
  # ca: |
  #   -----BEGIN CERTIFICATE-----
  #   ...
  #   -----END CERTIFICATE-----
```

## Notes

- Config names in Service `configs` array are resolved to Ziti config IDs automatically
- Both ConfigHostV1 and ConfigInterceptV1 configs can be mixed in a Service
- All resources support `managementPolicies` for granular control (Observe, Create, Update, Delete, LateInitialize)
- The provider handles drift detection and reconciliation automatically
