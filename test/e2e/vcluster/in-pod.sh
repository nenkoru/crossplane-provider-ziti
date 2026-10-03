#!/usr/bin/env bash
# Runs inside the e2e-runner pod of the virtual cluster, see bed.sh: starts
# the provider against the virtual cluster and runs the end-to-end test
# against the test Ziti controller.
set -euo pipefail

cd /work
export E2E_WORK_DIR=/work/e2e
mkdir -p "${E2E_WORK_DIR}"

# The service account of the pod administers the virtual cluster. The token
# is read from its file, which the kubelet renews.
SA=/var/run/secrets/kubernetes.io/serviceaccount
export KUBECONFIG="${E2E_WORK_DIR}/kubeconfig"
(umask 077 && cat >"${KUBECONFIG}" <<KUBECONFIG
apiVersion: v1
kind: Config
clusters:
  - name: virtual
    cluster:
      server: https://kubernetes.default.svc
      certificate-authority: ${SA}/ca.crt
users:
  - name: e2e-runner
    user:
      tokenFile: ${SA}/token
contexts:
  - name: virtual
    context: {cluster: virtual, user: e2e-runner, namespace: default}
current-context: virtual
KUBECONFIG
)
export ZITI_URL=https://ziti-edge-controller.default.svc:1280
export ZITI_USER=admin
ZITI_PWD="$(cat /secrets/ziti/password)"
export ZITI_PWD

pkill -x provider 2>/dev/null || true
if [ "${E2E_RESET:-true}" = "true" ]; then
	echo ">>> Removing what an earlier run left behind"
	kubectl delete publishedservices.platform.example.org --all -A --ignore-not-found --wait=false >/dev/null 2>&1 || true
	for crd in $(kubectl get crd -o name | grep 'ziti\.crossplane\.io$' || true); do
		kind="${crd#*/}"
		kubectl get "${kind}" -A -o json |
			jq -r '.items[] | "\(.metadata.namespace // "") \(.metadata.name)"' |
			while read -r namespace name; do
				kubectl patch "${kind}" ${namespace:+-n "${namespace}"} "${name}" --type merge -p '{"metadata": {"finalizers": []}}' >/dev/null
			done
	done
	kubectl delete -f /work/repo/package/crds --ignore-not-found >/dev/null
	# The controller keeps its state in an emptyDir: a new pod is a new network.
	kubectl rollout restart deployment/ziti-edge-controller >/dev/null
	kubectl rollout status deployment/ziti-edge-controller --timeout=300s >/dev/null
fi

echo "virtual cluster: $(kubectl version -o json | jq -r '.serverVersion.gitVersion')"
echo "ziti controller: $(curl -sk --fail --retry 30 --retry-delay 2 --retry-all-errors "${ZITI_URL}/edge/client/v1/version" | jq -r '.data.version')"

echo ">>> Installing CRDs into the virtual cluster"
kubectl apply -f /work/repo/package/crds >/dev/null
kubectl wait --for=condition=Established --timeout=60s -f /work/repo/package/crds >/dev/null

if [ "${E2E_CROSSPLANE:-true}" = "true" ]; then
	/work/repo/test/e2e/e2e.sh crossplane
fi

echo ">>> Starting the provider"
/work/provider --debug --poll="${E2E_POLL:-10s}" >"${E2E_WORK_DIR}/provider.log" 2>&1 &
PROVIDER=$!
trap 'status=$?; [ "${status}" -eq 0 ] || /work/repo/test/e2e/e2e.sh logs || true; kill ${PROVIDER} 2>/dev/null || true' EXIT
sleep 3
kill -0 "${PROVIDER}" 2>/dev/null || {
	echo "the provider exited" >&2
	exit 1
}

/work/repo/test/e2e/e2e.sh test
