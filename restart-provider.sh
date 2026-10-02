#!/bin/bash
# restart-provider.sh - Properly restart the Crossplane provider

set -uo pipefail

echo "=== Stopping existing provider ==="
# Kill any process on port 8080
PID=$(lsof -ti :8080)
if [ ! -z "$PID" ]; then
	echo "Killing process on port 8080 (PID: $PID)"
	kill -9 $PID 2>/dev/null || true
	sleep 2
fi

# Also kill any go run processes for the provider
pkill -9 -f "go run cmd/provider/main.go" 2>/dev/null || true
pkill -9 -f "provider.*debug" 2>/dev/null || true
sleep 2

# Verify port is free
if lsof -i :8080 >/dev/null 2>&1; then
	echo "ERROR: Port 8080 still in use"
	lsof -i :8080
	exit 1
fi

echo "=== Building provider ==="
cd "$(dirname "${BASH_SOURCE[0]}")"
go build ./...

echo "=== Starting provider ==="
nohup go run cmd/provider/main.go --debug >/tmp/provider.log 2>&1 &
PROVIDER_PID=$!
echo "Provider started with PID: $PROVIDER_PID"

# Wait for provider to be ready
echo "=== Waiting for provider to be ready ==="
for i in {1..30}; do
	if lsof -i :8080 >/dev/null 2>&1; then
		echo "Provider is listening on port 8080"
		break
	fi
	sleep 1
done

if ! lsof -i :8080 >/dev/null 2>&1; then
	echo "ERROR: Provider failed to start on port 8080"
	echo "Last 20 lines of log:"
	tail -20 /tmp/provider.log
	exit 1
fi

echo "=== Provider ready ==="
echo "PID: $PROVIDER_PID"
echo "Log: /tmp/provider.log"
echo "Metrics: http://localhost:8080/metrics"
