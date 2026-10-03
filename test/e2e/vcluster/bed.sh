#!/usr/bin/env bash
# Runs the end-to-end test of provider-ziti inside a virtual cluster (vcluster)
# of an existing Kubernetes cluster, for clusters where CRDs and controllers
# must not be installed directly. Everything lives in one namespace of the
# host cluster: the virtual cluster, and inside it a throwaway OpenZiti
# controller, Crossplane, and a pod that hosts the provider and the test.
#
#   test/e2e/vcluster/bed.sh up     # create the virtual cluster, OpenZiti and the runner
#   test/e2e/vcluster/bed.sh run    # build the provider, copy it into the runner, run the test
#   test/e2e/vcluster/bed.sh down   # remove everything "up" created, the namespace stays
#
# kubectl must point at the host cluster, and the namespace must exist; see
# namespace.yaml for one with a quota. The provider is connected only to the
# OpenZiti controller of the bed, which is new with every run.
#
#   E2E_NAMESPACE       namespace of the host cluster (provider-ziti-e2e)
#   E2E_PRIORITY_CLASS  priority class of the host cluster for all pods of the bed (none)
#   E2E_GOARCH          architecture of the nodes (amd64)
#   E2E_CROSSPLANE      install Crossplane and test the Composition (true)
#   E2E_RESET           start "run" from a new OpenZiti network (true)
#   E2E_ONLY            stages of the test to run instead of all, see e2e.sh
#
# Requires kubectl, helm, go and openssl.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${HERE}/../../.." && pwd)"
WORK="${E2E_WORK_DIR:-${ROOT}/_output/e2e-vcluster}"

NAMESPACE="${E2E_NAMESPACE:-provider-ziti-e2e}"
RELEASE="${E2E_VCLUSTER:-provider-ziti}"
PRIORITY_CLASS="${E2E_PRIORITY_CLASS:-}"
VCLUSTER_VERSION="${VCLUSTER_VERSION:-0.37.2}"
ZITI_VERSION="${ZITI_VERSION:-2.0.6}"
# The name of the runner pod in the host cluster.
RUNNER="e2e-runner-x-default-x-${RELEASE}"
PORT="${E2E_PORT:-18443}"

host() { kubectl -n "${NAMESPACE}" "$@"; }
virtual() { kubectl --kubeconfig "${WORK}/kubeconfig" "$@"; }

# manifests prints the objects of the virtual cluster in the host cluster.
manifests() {
	local args=()
	if [ -n "${PRIORITY_CLASS}" ]; then
		args+=(
			--set "controlPlane.statefulSet.scheduling.priorityClassName=${PRIORITY_CLASS}"
			--set "sync.toHost.pods.priorityClassName=${PRIORITY_CLASS}"
		)
	fi
	helm template "${RELEASE}" vcluster --repo https://charts.loft.sh --version "${VCLUSTER_VERSION}" \
		--namespace "${NAMESPACE}" -f "${HERE}/vcluster-values.yaml" ${args[@]+"${args[@]}"}
}

# connect forwards a local port to the API of the virtual cluster for as long
# as this script runs.
connect() {
	mkdir -p "${WORK}"
	(umask 077 && host get secret "vc-${RELEASE}" -o jsonpath='{.data.config}' | base64 -d |
		sed -E "s#server: https://[^ ]+#server: https://localhost:${PORT}#" >"${WORK}/kubeconfig")
	host port-forward "service/${RELEASE}" "${PORT}:443" >/dev/null 2>&1 &
	FORWARD=$!
	trap 'kill "${FORWARD}" 2>/dev/null || true' EXIT
	local deadline=$((SECONDS + 60))
	until virtual get namespace default >/dev/null 2>&1; do
		[ "${SECONDS}" -lt "${deadline}" ] || {
			echo "cannot reach the virtual cluster" >&2
			exit 1
		}
		sleep 1
	done
}

case "${1:-}" in
up)
	manifests | host apply -f -
	host rollout status "deployment/${RELEASE}" --timeout=300s
	until host get secret "vc-${RELEASE}" >/dev/null 2>&1; do sleep 2; done
	connect
	if [ -n "${PRIORITY_CLASS}" ]; then
		# The pods of the virtual cluster get the priority class in the host
		# cluster, which refuses a pod whose preemption policy is not the one
		# of the class. A default class inside the virtual cluster gives them
		# that policy.
		virtual apply -f - <<PRIORITY
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: bed-default
globalDefault: true
value: 0
preemptionPolicy: $(kubectl get priorityclass "${PRIORITY_CLASS}" -o jsonpath='{.preemptionPolicy}')
PRIORITY
		virtual -n kube-system delete pod --all --ignore-not-found
	fi
	# The password of the test controller is random and stays in the cluster.
	virtual -n default get secret ziti-admin >/dev/null 2>&1 ||
		openssl rand -hex 16 | tr -d '\n' | virtual -n default create secret generic ziti-admin --from-file=password=/dev/stdin
	# A pod cannot be changed: the runner is made anew.
	virtual -n default delete pod e2e-runner --ignore-not-found
	sed "s/ZITI_VERSION/${ZITI_VERSION}/" "${HERE}/bed.yaml" | virtual apply -f -
	virtual -n default rollout status deployment/ziti-edge-controller --timeout=600s
	virtual -n default wait --for=condition=Ready pod/e2e-runner --timeout=600s
	;;
run)
	rm -rf "${WORK}/stage"
	mkdir -p "${WORK}/stage/repo/test/e2e" "${WORK}/stage/repo/package"
	(cd "${ROOT}" && GOOS=linux GOARCH="${E2E_GOARCH:-amd64}" CGO_ENABLED=0 go build -ldflags='-s -w' -o "${WORK}/stage/provider" ./cmd/provider)
	cp -R "${ROOT}/package/crds" "${WORK}/stage/repo/package/"
	cp -R "${ROOT}/examples" "${WORK}/stage/repo/"
	cp "${ROOT}/test/e2e/e2e.sh" "${WORK}/stage/repo/test/e2e/"
	cp "${HERE}/in-pod.sh" "${WORK}/stage/"
	host exec -c runner "${RUNNER}" -- sh -c 'pkill -x provider; rm -rf /work/repo /work/provider /work/in-pod.sh /work/e2e; true'
	COPYFILE_DISABLE=1 tar -C "${WORK}/stage" -czf - . | host exec -i -c runner "${RUNNER}" -- tar -C /work -xzf -
	host exec -c runner "${RUNNER}" -- env \
		E2E_TIMEOUT="${E2E_TIMEOUT:-180}" \
		E2E_CROSSPLANE="${E2E_CROSSPLANE:-true}" \
		E2E_RESET="${E2E_RESET:-true}" \
		E2E_SKIP_EXTENDED="${E2E_SKIP_EXTENDED:-false}" \
		E2E_ONLY="${E2E_ONLY:-}" \
		bash /work/in-pod.sh
	;;
down)
	manifests | host delete --ignore-not-found -f -
	# What the virtual cluster created in the host cluster goes with it; this
	# removes what is left if it was killed first.
	host delete pods,services,secrets,configmaps,endpoints -l "vcluster.loft.sh/managed-by=${RELEASE}" --ignore-not-found
	;;
*)
	sed -n '2,23p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
