#!/bin/bash
# test-permutations.sh - Generate and apply permutation tests for Ziti resources

set -euo pipefail

NAMESPACE="default"
PROVIDER_CONFIG="default"

# Create test directory
TEST_DIR="/tmp/ziti-permutation-tests"
mkdir -p "$TEST_DIR"

echo "=== Generating permutation test manifests ==="

# ============================================
# SERVICE PERMUTATIONS
# ============================================

cat >"$TEST_DIR/service-minimal.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: service-minimal
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "service-minimal"
EOF

cat >"$TEST_DIR/service-with-encryption.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: service-with-encryption
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "service-with-encryption"
    encryptionRequired: true
EOF

cat >"$TEST_DIR/service-no-encryption.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: service-no-encryption
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "service-no-encryption"
    encryptionRequired: false
EOF

cat >"$TEST_DIR/service-max-idle.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: service-max-idle
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "service-max-idle"
    maxIdleTimeMillis: 300000
EOF

for STRATEGY in smartrouting weighted random; do
	cat >"$TEST_DIR/service-${STRATEGY}.yaml" <<EOF
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: service-${STRATEGY}
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "service-${STRATEGY}"
    terminatorStrategy: "${STRATEGY}"
EOF
done

cat >"$TEST_DIR/service-with-configs.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: service-with-configs
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "service-with-configs"
    configs:
      - "host-v1-simple"
      - "intercept-v1-simple"
EOF

cat >"$TEST_DIR/service-full.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: Service
metadata:
  name: service-full-test
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "service-full-test"
    encryptionRequired: true
    maxIdleTimeMillis: 60000
    terminatorStrategy: "weighted"
    configs:
      - "host-v1-simple"
      - "intercept-v1-simple"
    roleAttributes:
      - "role:api"
      - "role:web"
      - "role:admin"
    tags:
      env: "test"
      type: "full-service"
      test: "permutation"
EOF

# ============================================
# CONFIG HOST V1 PERMUTATIONS
# ============================================

cat >"$TEST_DIR/confighostv1-minimal.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-minimal
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-minimal"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
EOF

cat >"$TEST_DIR/confighostv1-forward-protocol.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-forward-protocol
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-forward-protocol"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    forwardProtocol: true
    allowedProtocols:
      - "tcp"
      - "udp"
EOF

cat >"$TEST_DIR/confighostv1-forward-port.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-forward-port
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-forward-port"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    forwardPort: true
    allowedPortRanges:
      - low: 8000
        high: 9000
EOF

cat >"$TEST_DIR/confighostv1-forward-address.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-forward-address
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-forward-address"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    forwardAddress: true
    allowedAddresses:
      - "192.168.1.0/24"
      - "10.0.0.0/8"
EOF

cat >"$TEST_DIR/confighostv1-with-translations.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-translations
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-translations"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    forwardAddress: true
    allowedAddresses:
      - "192.168.1.0"
    forwardAddressTranslations:
      - from: "192.168.1.0"
        to: "10.10.0.0"
        prefixLength: 24
EOF

cat >"$TEST_DIR/confighostv1-with-listen-options.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-listen-options
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-listen-options"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    listenOptions:
      bindUsingEdgeIdentity: true
      connectTimeout: "10s"
      cost: 100
      maxConnections: 1000
      precedence: "required"
EOF

cat >"$TEST_DIR/confighostv1-with-proxy.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-proxy
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-proxy"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    proxy:
      address: "proxy.example.com:8080"
      type: "http"
EOF

cat >"$TEST_DIR/confighostv1-with-http-checks.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-http-checks
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-http-checks"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    httpChecks:
      - url: "http://health.example.com/health"
        method: "GET"
        interval: "30s"
        timeout: "10s"
        expectStatus: 200
        expectInBody: "OK"
        actions:
          - trigger: "fail"
            action: "mark unhealthy"
            duration: "60s"
            consecutiveEvents: 3
EOF

cat >"$TEST_DIR/confighostv1-with-port-checks.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-port-checks
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-port-checks"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    portChecks:
      - address: "10.0.0.1:8080"
        interval: "15s"
        timeout: "5s"
        actions:
          - trigger: "fail"
            action: "mark unhealthy"
            duration: "30s"
            consecutiveEvents: 2
EOF

cat >"$TEST_DIR/confighostv1-full.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigHostV1
metadata:
  name: confighostv1-full
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "confighostv1-full"
    address: "10.0.0.1"
    port: 8080
    protocol: "tcp"
    forwardProtocol: true
    forwardPort: true
    forwardAddress: true
    allowedProtocols:
      - "tcp"
      - "udp"
    allowedAddresses:
      - "192.168.1.0/24"
      - "10.0.0.0/8"
    allowedSourceAddresses:
      - "192.168.1.0/24"
    forwardAddressTranslations:
      - from: "192.168.1.0"
        to: "10.10.0.0"
        prefixLength: 24
    allowedPortRanges:
      - low: 8000
        high: 9000
      - low: 3000
        high: 3010
    listenOptions:
      bindUsingEdgeIdentity: true
      connectTimeout: "10s"
      cost: 100
      maxConnections: 1000
      precedence: "required"
    proxy:
      address: "proxy.example.com:8080"
      type: "http"
    httpChecks:
      - url: "http://health.example.com/health"
        method: "GET"
        interval: "30s"
        timeout: "10s"
        expectStatus: 200
        expectInBody: "OK"
        actions:
          - trigger: "fail"
            action: "mark unhealthy"
            duration: "60s"
            consecutiveEvents: 3
    portChecks:
      - address: "10.0.0.1:8080"
        interval: "15s"
        timeout: "5s"
        actions:
          - trigger: "fail"
            action: "mark unhealthy"
            duration: "30s"
            consecutiveEvents: 2
    tags:
      env: "test"
      team: "network"
EOF

# ============================================
# CONFIG INTERCEPT V1 PERMUTATIONS
# ============================================

cat >"$TEST_DIR/configinterceptv1-minimal.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigInterceptV1
metadata:
  name: configinterceptv1-minimal
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "configinterceptv1-minimal"
    addresses:
      - "app.example.com"
    protocols:
      - "tcp"
    portRanges:
      - low: 80
        high: 80
EOF

cat >"$TEST_DIR/configinterceptv1-multi-address.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigInterceptV1
metadata:
  name: configinterceptv1-multi-address
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "configinterceptv1-multi-address"
    addresses:
      - "api.example.com"
      - "app.example.com"
      - "web.example.com"
    protocols:
      - "tcp"
      - "udp"
    portRanges:
      - low: 80
        high: 80
      - low: 443
        high: 443
EOF

cat >"$TEST_DIR/configinterceptv1-with-dial-options.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigInterceptV1
metadata:
  name: configinterceptv1-dial-options
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "configinterceptv1-dial-options"
    addresses:
      - "app.example.com"
    protocols:
      - "tcp"
    portRanges:
      - low: 80
        high: 80
    dialOptions:
      connectTimeoutSeconds: 30
      identity: "my-identity"
EOF

cat >"$TEST_DIR/configinterceptv1-with-source-ip.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigInterceptV1
metadata:
  name: configinterceptv1-source-ip
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "configinterceptv1-source-ip"
    addresses:
      - "app.example.com"
    protocols:
      - "tcp"
    portRanges:
      - low: 80
        high: 80
    sourceIp: "10.0.0.1"
EOF

cat >"$TEST_DIR/configinterceptv1-full.yaml" <<'EOF'
apiVersion: ziti.crossplane.io/v1alpha1
kind: ConfigInterceptV1
metadata:
  name: configinterceptv1-full
spec:
  providerConfigRef:
    name: default
    kind: ProviderConfig
  forProvider:
    name: "configinterceptv1-full"
    addresses:
      - "api.example.com"
      - "app.example.com"
    protocols:
      - "tcp"
      - "udp"
    portRanges:
      - low: 80
        high: 80
      - low: 443
        high: 443
    dialOptions:
      connectTimeoutSeconds: 30
      identity: "my-identity"
    sourceIp: "10.0.0.1"
EOF

echo "=== Generated $(ls -1 $TEST_DIR/*.yaml | wc -l) test manifests ==="

# ============================================
# APPLY ALL TESTS
# ============================================

echo ""
echo "=== Applying test manifests ==="

# Apply ConfigHostV1 first (needed by services)
for f in "$TEST_DIR"/confighostv1-*.yaml; do
	echo "Applying $f..."
	kubectl apply -f "$f" || echo "  Failed (may already exist)"
done

# Apply ConfigInterceptV1
for f in "$TEST_DIR"/configinterceptv1-*.yaml; do
	echo "Applying $f..."
	kubectl apply -f "$f" || echo "  Failed (may already exist)"
done

# Apply Services
for f in "$TEST_DIR"/service-*.yaml; do
	echo "Applying $f..."
	kubectl apply -f "$f" || echo "  Failed (may already exist)"
done

echo ""
echo "=== Waiting for reconciliation ==="
sleep 30

echo ""
echo "=== Verification ==="

echo "Services:"
kubectl get service.ziti.crossplane.io -o custom-columns=NAME:.metadata.name,ID:.status.atProvider.id,READY:.status.conditions[?\(@.type==\"Ready\"\)].status,SYNCED:.status.conditions[?\(@.type==\"Synced\"\)].status

echo ""
echo "ConfigHostV1:"
kubectl get confighostv1 -o custom-columns=NAME:.metadata.name,ID:.status.atProvider.id,READY:.status.conditions[?\(@.type==\"Ready\"\)].status,SYNCED:.status.conditions[?\(@.type==\"Synced\"\)].status

echo ""
echo "ConfigInterceptV1:"
kubectl get configinterceptv1 -o custom-columns=NAME:.metadata.name,ID:.status.atProvider.id,READY:.status.conditions[?\(@.type==\"Ready\"\)].status,SYNCED:.status.conditions[?\(@.type==\"Synced\"\)].status
