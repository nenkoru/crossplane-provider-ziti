#!/usr/bin/env bash
# End-to-end test of provider-ziti.
#
# The provider runs out-of-cluster against a kind cluster and manages an
# OpenZiti controller started from ziti-docker-compose.yml. The test applies
# the manifests in examples/, checks through the Ziti Edge Management API that
# the entities exist as declared, changes, tampers with and deletes them
# behind the provider's back, and checks that deleting the managed resources
# deletes the entities. docs/testing.md lists what is checked for every kind.
#
# Usage:
#   test/e2e/e2e.sh up          # start OpenZiti, a kind cluster and the provider
#   test/e2e/e2e.sh test        # run the test against what "up" started
#   test/e2e/e2e.sh logs        # print diagnostics
#   test/e2e/e2e.sh down        # stop everything "up" started
#   test/e2e/e2e.sh all         # up, test, down
#   test/e2e/e2e.sh crossplane  # install Crossplane into the cluster of the test
#
# E2E_ONLY runs some stages of the test instead of all of them, for example
# E2E_ONLY="lifecycle scenarios". The stages are core, drift, extended,
# posture_checks, authentication, renewal, lifecycle, scenarios, examples and
# composition.
#
# "test" alone runs the checks against a provider that is already running:
# put the kubeconfig of its cluster in $E2E_WORK_DIR/kubeconfig and set
# ZITI_URL, ZITI_USER and ZITI_PWD for the test controller. That is how
# test/e2e/vcluster runs the test inside a virtual cluster.
#
# The Composition of examples/composition is tested when Crossplane is
# installed in the cluster. "up" installs it if E2E_CROSSPLANE is true.
#
# Requires docker (with compose), kind, kubectl, go, curl, jq and openssl, and
# helm to install Crossplane.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="${E2E_WORK_DIR:-${ROOT}/_output/e2e}"

CLUSTER="${E2E_CLUSTER:-provider-ziti-e2e}"
COMPOSE_FILE="${ROOT}/ziti-docker-compose.yml"
COMPOSE_PROJECT="provider-ziti-e2e"

# The OpenZiti release "up" starts. The compose file itself follows latest.
export ZITI_VERSION="${ZITI_VERSION:-2.0.6}"

ZITI_URL="${ZITI_URL:-https://localhost:1280}"
ZITI_USER="${ZITI_USER:-admin}"
ZITI_PWD="${ZITI_PWD:-admin}"
ZITI_API="${ZITI_URL}/edge/management/v1"

# How long to wait for the provider to converge, in seconds.
TIMEOUT="${E2E_TIMEOUT:-180}"
# How often the provider checks for drift.
POLL="${E2E_POLL:-10s}"
# How long entities must stay untouched to prove the provider does not update
# them without a reason, in seconds. Must span several polls.
SETTLE="${E2E_SETTLE:-35}"

# The Crossplane release "crossplane" installs.
CROSSPLANE_VERSION="${CROSSPLANE_VERSION:-2.4.2}"

GROUP="ziti.crossplane.io"

# The resources every user of the provider needs.
CORE_EXAMPLES=(
	examples/service/service.yaml
	examples/identity/identity.yaml
	examples/servicepolicy/servicepolicy.yaml
)

# The remaining kinds.
EXTENDED_EXAMPLES=(
	examples/edgerouter/edgerouter.yaml
	examples/edgerouterpolicy/edgerouterpolicy.yaml
	examples/serviceedgerouterpolicy/serviceedgerouterpolicy.yaml
	examples/posturecheck/os.yaml
	examples/posturecheck/mfa.yaml
	examples/authpolicy/authpolicy.yaml
	examples/service/hostv2.yaml
	examples/identity/ca.yaml
	examples/identity/updb.yaml
	examples/identity/none.yaml
)

# The posture checks that tell devices apart by their Windows domain, their
# MAC addresses and the processes they run.
POSTURE_CHECK_EXAMPLES=(
	examples/posturecheck/domain.yaml
	examples/posturecheck/mac.yaml
	examples/posturecheck/process.yaml
	examples/posturecheck/multiprocess.yaml
)

# The certificate authority, whose manifest gets a certificate that is made
# for the test, see apply_ca.
CA_EXAMPLE=examples/certificateauthority/certificateauthority.yaml

# The external JWT signer, and what refers to it and to the certificate
# authority by name.
AUTHENTICATION_EXAMPLES=(
	examples/externaljwtsigner/externaljwtsigner.yaml
	examples/authpolicy/jwt.yaml
	examples/identity/ca.yaml
)

# The scenarios: resources that are applied together.
SCENARIO_EXAMPLES=(
	examples/scenarios/publish-service.yaml
	examples/scenarios/edge-router.yaml
	examples/scenarios/configs.yaml
)

# The Composition with its function and its definition, and a composite
# resource for it.
COMPOSITION_EXAMPLES=(
	examples/composition/function.yaml
	examples/composition/definition.yaml
	examples/composition/composition.yaml
)
COMPOSITE_EXAMPLE=examples/composition/publishedservice.yaml
COMPOSITE="publishedservices.platform.example.org"

export KUBECONFIG="${WORK}/kubeconfig"

step() { printf '\n>>> %s\n' "$*"; }
ok() { printf '  [ok] %s\n' "$*"; }
fail() {
	printf '  [FAIL] %s\n' "$*" >&2
	exit 1
}

# ------------------------------------------------------------------------------
# Ziti Edge Management API

ZITI_TOKEN=""

ziti_login() {
	ZITI_TOKEN="$(curl -sk --fail -X POST -H 'Content-Type: application/json' \
		-d "{\"username\": \"${ZITI_USER}\", \"password\": \"${ZITI_PWD}\"}" \
		"${ZITI_API}/authenticate?method=password" | jq -r '.data.token')"
	[ -n "${ZITI_TOKEN}" ] && [ "${ZITI_TOKEN}" != "null" ] || fail "cannot log in to ${ZITI_URL}"
}

# ziti_get <collection> <id> prints the entity.
ziti_get() {
	curl -sk --fail -H "zt-session: ${ZITI_TOKEN}" "${ZITI_API}/$1/$2" | jq '.data'
}

# ziti_post <collection> <json> creates an entity behind the provider's back
# and prints its ID.
ziti_post() {
	curl -sk --fail -X POST -H "zt-session: ${ZITI_TOKEN}" -H 'Content-Type: application/json' \
		-d "$2" "${ZITI_API}/$1" | jq -er '.data.id'
}

# ziti_refresh <enrollment id> <seconds> has Ziti give an enrollment a new
# token behind the provider's back, one that expires so many seconds from now.
ziti_refresh() {
	curl -sk --fail -X POST -H "zt-session: ${ZITI_TOKEN}" -H 'Content-Type: application/json' \
		-d "$(jq -n --argjson seconds "$2" '{expiresAt: (now + $seconds | todate)}')" \
		"${ZITI_API}/enrollments/$1/refresh" >/dev/null
}

# ziti_delete <collection> <id> deletes an entity behind the provider's back.
ziti_delete() {
	curl -sk --fail -X DELETE -H "zt-session: ${ZITI_TOKEN}" "${ZITI_API}/$1/$2" >/dev/null
}

# ziti_patch <collection> <id> <json> changes the entity behind the provider's back.
ziti_patch() {
	curl -sk --fail -X PATCH -H "zt-session: ${ZITI_TOKEN}" -H 'Content-Type: application/json' \
		-d "$3" "${ZITI_API}/$1/$2" >/dev/null
}

# ziti_id <collection> <name> prints the ID of the entity with that name, if any.
ziti_id() {
	curl -sk --fail -H "zt-session: ${ZITI_TOKEN}" --get \
		--data-urlencode "filter=name=\"$2\"" "${ZITI_API}/$1" | jq -r '.data[0].id // empty'
}

# ------------------------------------------------------------------------------
# Assertions

# expect <description> <actual> <expected>
expect() {
	if [ "$2" = "$3" ]; then
		ok "$1"
	else
		fail "$1: want '$3', got '$2'"
	fi
}

# expect_gone <description> <collection> <name> checks that no entity has that name.
expect_gone() {
	local id
	id="$(ziti_id "$2" "$3")" || fail "$1: cannot query Ziti"
	expect "$1" "${id}" ""
}

# eventually <description> <command...> retries until the command succeeds.
eventually() {
	local description="$1"
	shift
	local deadline=$((SECONDS + TIMEOUT))
	until "$@" >/dev/null 2>&1; do
		if [ "${SECONDS}" -ge "${deadline}" ]; then
			"$@" || true
			fail "${description} (gave up after ${TIMEOUT}s)"
		fi
		sleep 2
	done
	ok "${description}"
}

# entity_matches <collection> <id> <jq filter> succeeds if the filter is true for the entity.
entity_matches() {
	ziti_get "$1" "$2" | jq -e "$3"
}

# sync_error_has <plural> <name> <text> succeeds if the managed resource
# reports an error that contains the text.
sync_error_has() {
	kubectl get "$1.${GROUP}" "$2" -o jsonpath='{.status.conditions[?(@.type=="Synced")].message}' | grep -qF "$3"
}

# patch <plural> <name> <json merge patch of spec.forProvider> changes the spec.
patch() {
	kubectl patch "$1.${GROUP}" "$2" --type merge -p "{\"spec\": {\"forProvider\": $3}}" >/dev/null
}

# mr_id <plural> <name> prints the Ziti ID recorded in the managed resource.
mr_id() {
	kubectl get "$1.${GROUP}" "$2" -o jsonpath='{.status.atProvider.id}'
}

# check_entity <plural> <name> <collection> <jq filter> checks that the managed
# resource points at a Ziti entity with the same name that satisfies the filter.
check_entity() {
	local plural="$1" name="$2" collection="$3" filter="$4" id external
	id="$(mr_id "${plural}" "${name}")"
	external="$(kubectl get "${plural}.${GROUP}" "${name}" -o jsonpath='{.metadata.annotations.crossplane\.io/external-name}')"
	[ -n "${id}" ] || fail "${plural}/${name} has no status.atProvider.id"
	expect "${plural}/${name}: external name is the Ziti ID" "${external}" "${id}"
	expect "${plural}/${name}: found in Ziti by name" "$(ziti_id "${collection}" "${name}")" "${id}"
	if entity_matches "${collection}" "${id}" "${filter}" >/dev/null; then
		ok "${plural}/${name}: Ziti entity is as declared"
	else
		# Enrollment and verification tokens are left out of the dump.
		ziti_get "${collection}" "${id}" | jq 'del(.enrollment, .enrollmentJwt, .enrollmentToken, .verificationToken)' >&2
		fail "${plural}/${name}: Ziti entity does not satisfy: ${filter}"
	fi
}

# check_token <secret> <collection> <id> <jq path of the token> checks that the
# connection secret holds the enrollment token Ziti reports.
check_token() {
	local secret="$1" collection="$2" id="$3" path="$4" published reported
	published="$(kubectl get secret "${secret}" -o jsonpath='{.data.enrollmentToken}' | base64 -d)"
	reported="$(ziti_get "${collection}" "${id}" | jq -r "${path}")"
	[ -n "${published}" ] || fail "secret/${secret} has no enrollmentToken"
	# Never print the token itself.
	if [ "${published}" = "${reported}" ]; then
		ok "secret/${secret}: holds the enrollment token"
	else
		fail "secret/${secret}: enrollmentToken differs from the token Ziti reports"
	fi
	case "${published}" in
	*.*.*) ok "secret/${secret}: enrollment token is a JWT" ;;
	*) fail "secret/${secret}: enrollment token is not a JWT" ;;
	esac
}

# token_digest <secret> prints a digest of the enrollment token in the
# connection secret. It tells tokens apart without showing them.
token_digest() {
	kubectl get secret "$1" -o jsonpath='{.data.enrollmentToken}' | base64 -d | openssl dgst -sha256 -r | cut -d' ' -f1
}

# token_is_current <secret> <collection> <id> <jq path of the token> succeeds
# if the connection secret holds the enrollment token Ziti reports.
token_is_current() {
	local published reported
	published="$(kubectl get secret "$1" -o jsonpath='{.data.enrollmentToken}' | base64 -d)" || return 1
	reported="$(ziti_get "$2" "$3" | jq -er "$4")" || return 1
	[ "${published}" = "${reported}" ]
}

# expiry_is_current <plural> <name> <collection> <jq path of the expiry>
# succeeds if the status of the managed resource says when the enrollment
# token Ziti reports expires.
expiry_is_current() {
	local shown reported
	shown="$(kubectl get "$1.${GROUP}" "$2" -o jsonpath='{.status.atProvider.enrollmentExpiresAt}')" || return 1
	reported="$(ziti_get "$3" "$(mr_id "$1" "$2")" | jq -er "$4")" || return 1
	[ "${shown}" = "${reported}" ]
}

# expires_in <collection> <id> <jq path of the expiry> <min> <max> succeeds
# if the time lies more than min and at most max seconds ahead.
expires_in() {
	ziti_get "$1" "$2" | jq -e --argjson min "$4" --argjson max "$5" \
		"($3 | sub(\"[.][0-9]+Z\$\"; \"Z\") | fromdate) - now | . > \$min and . <= \$max"
}

# expect_renewed <plural> <name> <collection> <jq path of the token> <jq path
# of the expiry> <digest of the old token> checks that the connection secret
# <name>-enrollment holds a new enrollment token, the one Ziti reports, and
# that the status says when it expires.
expect_renewed() {
	local plural="$1" name="$2" collection="$3" token="$4" expiry="$5" old="$6" secret="$2-enrollment" id digest
	id="$(mr_id "${plural}" "${name}")"
	eventually "secret/${secret}: follows the token Ziti reports" token_is_current "${secret}" "${collection}" "${id}" "${token}"
	check_token "${secret}" "${collection}" "${id}" "${token}"
	digest="$(token_digest "${secret}")" || fail "cannot read secret/${secret}"
	[ "${digest}" != "${old}" ] || fail "secret/${secret}: still holds the old enrollment token"
	ok "secret/${secret}: the enrollment token is a new one"
	eventually "${plural}/${name}: status says when the new token expires" expiry_is_current "${plural}" "${name}" "${collection}" "${expiry}"
}

# stamps <collection> <id>... prints the last update time of each entity.
stamps() {
	local collection="$1" id
	shift
	for id in "$@"; do
		printf '%s/%s %s\n' "${collection}" "${id}" "$(ziti_get "${collection}" "${id}" | jq -r '.updatedAt')"
	done
}

# expect_settled <description> <command printing stamps> checks that no entity
# is updated while the provider keeps polling: an update without a spec change
# means the provider sees a difference that is not there.
expect_settled() {
	local description="$1" before after
	shift
	before="$("$@")" || fail "${description}: cannot query Ziti"
	sleep "${SETTLE}"
	after="$("$@")" || fail "${description}: cannot query Ziti"
	if [ "${before}" = "${after}" ]; then
		ok "${description}"
	else
		diff <(echo "${before}") <(echo "${after}") >&2 || true
		fail "${description}: entities were updated within ${SETTLE}s without a spec change"
	fi
}

# redact masks tokens and passwords, so that none ends up in a build log:
# JWTs, values that are labelled as a secret, credentials of an Authorization
# header and UUIDs, which is what a Ziti API session token is.
redact() {
	sed -E \
		-e 's/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/<redacted JWT>/g' \
		-e 's/("?(token|zt-session|password|secret)"?[=:] ?"?)[^" ,}]+/\1<redacted>/g' \
		-e 's/((Bearer|Basic) +)[A-Za-z0-9._~+\/=-]+/\1<redacted>/g' \
		-e 's/[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}/<redacted UUID>/g'
}

apply() {
	local args=()
	for f in "$@"; do args+=(-f "${ROOT}/${f}"); done
	kubectl apply "${args[@]}"
}

wait_ready() {
	local args=()
	for f in "$@"; do args+=(-f "${ROOT}/${f}"); done
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "${args[@]}"
}

delete() {
	local args=()
	for f in "$@"; do args+=(-f "${ROOT}/${f}"); done
	kubectl delete --wait --timeout="${TIMEOUT}s" "${args[@]}"
}

# apply_ca <certificate file> applies the certificate authority example with
# that certificate in place of its placeholder. The certificate is made when
# the test runs: one in the repository would expire some day, and the test
# needs its private key, which must not be published.
apply_ca() {
	kubectl create --dry-run=client -o json -f "${ROOT}/${CA_EXAMPLE}" |
		jq --rawfile pem "$1" '.spec.forProvider.certPem = $pem' |
		kubectl apply -f -
}

# status_is <plural> <name> <field> <value> succeeds if the managed resource
# reports that value in the field of status.atProvider.
status_is() {
	[ "$(kubectl get "$1.${GROUP}" "$2" -o jsonpath="{.status.atProvider.$3}")" = "$4" ]
}

# mr_gone <plural> <name> succeeds if the managed resource no longer exists.
mr_gone() {
	local found
	found="$(kubectl get "$1.${GROUP}" "$2" --ignore-not-found -o name)" || return 1
	[ -z "${found}" ]
}

# named_matches <collection> <name> <jq filter> succeeds if an entity has that
# name and the filter is true for it.
named_matches() {
	local id
	id="$(ziti_id "$1" "$2")" || return 1
	[ -n "${id}" ] && entity_matches "$1" "${id}" "$3"
}

# listed <path of a list> <name> succeeds if the list has an entity with that
# name.
listed() {
	curl -sk --fail -H "zt-session: ${ZITI_TOKEN}" --get --data-urlencode "filter=name=\"$2\"" "${ZITI_API}/$1" |
		jq -e '.data[0].id != null'
}

# ziti_gone <collection> <name> succeeds if no entity has that name.
ziti_gone() {
	local id
	id="$(ziti_id "$1" "$2")" || return 1
	[ -z "${id}" ]
}

# has_crossplane succeeds if Crossplane is installed in the cluster.
has_crossplane() {
	kubectl get crd compositions.apiextensions.crossplane.io >/dev/null 2>&1
}

# ------------------------------------------------------------------------------
# Commands

crossplane() {
	step "Installing Crossplane ${CROSSPLANE_VERSION}"
	helm upgrade --install crossplane crossplane --repo https://charts.crossplane.io/stable \
		--version "${CROSSPLANE_VERSION}" --namespace crossplane-system --create-namespace --wait --timeout 300s
	# Crossplane grants itself access to the kinds of the providers it
	# installs. The provider under test runs outside the cluster, so the test
	# grants the access.
	kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: provider-ziti-e2e:aggregate-to-crossplane
  labels:
    rbac.crossplane.io/aggregate-to-crossplane: "true"
rules:
  - apiGroups: ["${GROUP}"]
    resources: ["*"]
    verbs: ["*"]
EOF
}

up() {
	mkdir -p "${WORK}"

	step "Starting OpenZiti from ${COMPOSE_FILE##*/}"
	docker compose -p "${COMPOSE_PROJECT}" -f "${COMPOSE_FILE}" up -d ziti-controller
	local deadline=$((SECONDS + 300))
	until curl -sk --fail -m 2 "${ZITI_URL}/edge/client/v1/version" >/dev/null 2>&1; do
		[ "${SECONDS}" -lt "${deadline}" ] || fail "the Ziti controller did not come up at ${ZITI_URL}"
		sleep 2
	done
	ok "Ziti controller $(curl -sk "${ZITI_URL}/edge/client/v1/version" | jq -r '.data.version') answers at ${ZITI_URL}"

	step "Creating kind cluster ${CLUSTER}"
	kind create cluster --name "${CLUSTER}" --kubeconfig "${KUBECONFIG}" --wait 120s

	step "Installing CRDs"
	kubectl apply -f "${ROOT}/package/crds"
	kubectl wait --for=condition=Established --timeout=60s -f "${ROOT}/package/crds"

	if [ "${E2E_CROSSPLANE:-false}" = "true" ]; then
		crossplane
	fi

	step "Starting the provider"
	(cd "${ROOT}" && go build -o "${WORK}/provider" ./cmd/provider)
	"${WORK}/provider" --debug --poll="${POLL}" >"${WORK}/provider.log" 2>&1 &
	echo $! >"${WORK}/provider.pid"
	sleep 3
	kill -0 "$(cat "${WORK}/provider.pid")" 2>/dev/null || {
		cat "${WORK}/provider.log" >&2
		fail "the provider exited"
	}
	ok "provider is running (pid $(cat "${WORK}/provider.pid"), log ${WORK}/provider.log)"
}

configure() {
	step "Configuring the provider"
	kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: ziti-credentials
  namespace: default
type: Opaque
stringData:
  username: "${ZITI_USER}"
  password: "${ZITI_PWD}"
---
apiVersion: ${GROUP}/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: default
spec:
  host: "${ZITI_URL}"
  # The quickstart controller has a throwaway private PKI.
  insecureSkipTLSVerify: true
  credentials:
    source: Secret
    secretRef:
      name: ziti-credentials
EOF
}

test_core() {
	step "Creating the core resources"
	apply "${CORE_EXAMPLES[@]}"
	wait_ready "${CORE_EXAMPLES[@]}"

	local host intercept service client server
	host="$(mr_id confighostv1s web-service-host)"
	intercept="$(mr_id configinterceptv1s web-service-intercept)"
	service="$(mr_id services web-service)"
	client="$(mr_id identities web-client)"
	server="$(mr_id identities web-server)"

	step "Checking the core resources in Ziti"
	check_entity confighostv1s web-service-host configs \
		'.data.address == "web-service.local" and .data.port == 8080 and .data.protocol == "tcp" and .data.listenOptions.precedence == "required"'
	check_entity configinterceptv1s web-service-intercept configs \
		'.data.addresses == ["web.example.com", "api.example.com"] and (.data.portRanges | length) == 2 and .data.dialOptions.connectTimeoutSeconds == 30'
	check_entity services web-service services \
		"(.configs | sort) == ([\"${host}\", \"${intercept}\"] | sort) and .encryptionRequired == true and .terminatorStrategy == \"smartrouting\" and (.roleAttributes | sort) == [\"role:api\", \"role:web\"] and .tags.env == \"production\""
	check_entity identities web-client identities \
		'.roleAttributes == ["web-clients"] and .isAdmin == false and .tags.team == "platform"'
	check_entity identities web-server identities '.roleAttributes == ["web-servers"]'
	check_entity servicepolicies web-service-dial service-policies \
		".type == \"Dial\" and .semantic == \"AnyOf\" and .serviceRoles == [\"@${service}\"] and .identityRoles == [\"#web-clients\"]"
	check_entity servicepolicies web-service-bind service-policies \
		".type == \"Bind\" and .serviceRoles == [\"#role:web\"] and .identityRoles == [\"@${server}\"]"
	check_token web-client-enrollment identities "${client}" '.enrollment.ott.jwt'
	check_token web-server-enrollment identities "${server}" '.enrollment.ott.jwt'

	step "Updating a managed resource updates Ziti"
	patch services web-service \
		'{"roleAttributes": ["role:web", "role:e2e"], "encryptionRequired": false, "terminatorStrategy": "weighted", "maxIdleTimeMillis": 30000, "tags": {"env": "e2e", "team": null}}'
	eventually "service is updated in Ziti" entity_matches services "${service}" \
		'(.roleAttributes | sort) == ["role:e2e", "role:web"] and .encryptionRequired == false and .terminatorStrategy == "weighted" and .maxIdleTimeMillis == 30000 and .tags == {"env": "e2e"}'
	expect "service keeps its Ziti ID" "$(mr_id services web-service)" "${service}"
	patch confighostv1s web-service-host '{"port": 8443, "listenOptions": {"precedence": "default"}}'
	eventually "host config is updated in Ziti" entity_matches configs "${host}" \
		'.data.port == 8443 and .data.listenOptions.precedence == "default"'
	patch configinterceptv1s web-service-intercept '{"addresses": ["web.example.com"], "dialOptions": null}'
	eventually "intercept config is updated in Ziti" entity_matches configs "${intercept}" \
		'.data.addresses == ["web.example.com"] and (.data | has("dialOptions") | not)'
	patch identities web-server \
		'{"roleAttributes": ["web-servers", "e2e"], "tags": {"env": "e2e"}, "defaultHostingCost": 10, "defaultHostingPrecedence": "required", "serviceHostingCosts": {"web-service": 50}, "serviceHostingPrecedences": {"web-service": "failed"}, "appData": {"site": "berlin"}}'
	eventually "identity is updated in Ziti" entity_matches identities "${server}" \
		"(.roleAttributes | sort) == [\"e2e\", \"web-servers\"] and .tags == {\"env\": \"e2e\"} and .defaultHostingCost == 10 and .defaultHostingPrecedence == \"required\" and .serviceHostingCosts == {\"${service}\": 50} and .serviceHostingPrecedences == {\"${service}\": \"failed\"} and .appData == {\"site\": \"berlin\"}"
	patch servicepolicies web-service-dial '{"semantic": "AllOf", "identityRoles": ["#web-clients", "@web-server"], "tags": {"env": "e2e"}}'
	eventually "service policy is updated in Ziti" entity_matches service-policies "$(mr_id servicepolicies web-service-dial)" \
		".semantic == \"AllOf\" and (.identityRoles | sort) == ([\"#web-clients\", \"@${server}\"] | sort) and .tags == {\"env\": \"e2e\"}"

	step "Changes made in Ziti behind the provider's back are reverted"
	ziti_patch identities "${client}" '{"roleAttributes": ["tampered"]}'
	entity_matches identities "${client}" '.roleAttributes == ["tampered"]' >/dev/null || fail "could not tamper with the identity"
	eventually "identity role attributes are restored" entity_matches identities "${client}" '.roleAttributes == ["web-clients"]'

	step "A creation that was interrupted before the Ziti ID was saved is recovered"
	local started left_behind foreign
	# Someone else holds the name of the second resource below, from well
	# before its creation starts.
	foreign="$(ziti_post services '{"name": "e2e-taken", "encryptionRequired": true}')" || fail "cannot create a service in Ziti"
	started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	left_behind="$(ziti_post services '{"name": "e2e-interrupted", "encryptionRequired": true}')" || fail "cannot create a service in Ziti"
	kubectl apply -f - <<EOF
apiVersion: ${GROUP}/v1alpha1
kind: Service
metadata:
  name: e2e-interrupted
  namespace: default
  annotations:
    crossplane.io/external-create-pending: "${started}"
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  forProvider:
    name: e2e-interrupted
    roleAttributes:
      - e2e
EOF
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "services.${GROUP}/e2e-interrupted"
	expect "service takes over the entity its creation left behind" "$(mr_id services e2e-interrupted)" "${left_behind}"
	eventually "the entity follows the spec" entity_matches services "${left_behind}" '.roleAttributes == ["e2e"]'
	kubectl delete --wait --timeout="${TIMEOUT}s" "services.${GROUP}/e2e-interrupted"
	expect_gone "recovered service is gone" services e2e-interrupted

	step "A creation that was interrupted while its name was taken leaves the other entity alone"
	sleep 10
	kubectl apply -f - <<EOF
apiVersion: ${GROUP}/v1alpha1
kind: Service
metadata:
  name: e2e-taken
  namespace: default
  annotations:
    crossplane.io/external-create-pending: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  forProvider:
    name: e2e-taken
    roleAttributes:
      - e2e
EOF
	eventually "service reports that Ziti refuses its name" sync_error_has services e2e-taken 'ziti API error 4'
	expect "service does not take over the entity of someone else" "$(kubectl get "services.${GROUP}" e2e-taken -o jsonpath='{.metadata.annotations.crossplane\.io/external-name}')" ""
	kubectl delete --wait --timeout="${TIMEOUT}s" "services.${GROUP}/e2e-taken"
	entity_matches services "${foreign}" '.name == "e2e-taken" and (.roleAttributes | length) == 0' >/dev/null ||
		fail "the entity of someone else was changed or deleted"
	ok "the entity of someone else is untouched"
	ziti_delete services "${foreign}" || fail "cannot delete the service in Ziti"

	step "Nothing is updated without a spec change"
	core_stamps() {
		stamps configs "${host}" "${intercept}"
		stamps services "${service}"
		stamps identities "${client}" "${server}"
		stamps service-policies "$(mr_id servicepolicies web-service-dial)" "$(mr_id servicepolicies web-service-bind)"
	}
	expect_settled "core entities stay untouched" core_stamps

	step "Deleting the managed resources deletes the Ziti entities"
	delete "${CORE_EXAMPLES[@]}"
	expect_gone "service is gone" services web-service
	expect_gone "host config is gone" configs web-service-host
	expect_gone "intercept config is gone" configs web-service-intercept
	expect_gone "client identity is gone" identities web-client
	expect_gone "server identity is gone" identities web-server
	expect_gone "dial policy is gone" service-policies web-service-dial
	expect_gone "bind policy is gone" service-policies web-service-bind
}

# recreated <plural> <name> <collection> <old id> succeeds once the managed
# resource points at an entity with its name that is not the old one.
recreated() {
	local id
	id="$(mr_id "$1" "$2")" || return 1
	[ -n "${id}" ] && [ "${id}" != "$4" ] || return 1
	[ "$(ziti_id "$3" "$2")" = "${id}" ]
}

# test_drift deletes and changes entities of the core kinds in Ziti behind the
# provider's back, and checks that the provider puts them right: a change is
# reverted, a deleted entity is created anew under a new ID, what refers to
# it follows the new ID, and a new enrollment token reaches the connection
# secret.
test_drift() {
	step "Creating the core resources again"
	apply "${CORE_EXAMPLES[@]}"
	wait_ready "${CORE_EXAMPLES[@]}"

	local host service dial client old token
	host="$(mr_id confighostv1s web-service-host)"
	service="$(mr_id services web-service)"
	dial="$(mr_id servicepolicies web-service-dial)"
	client="$(mr_id identities web-client)"
	[ -n "${host}" ] && [ -n "${service}" ] && [ -n "${dial}" ] && [ -n "${client}" ] || fail "a core resource has no Ziti ID"

	step "Changes made in Ziti to every family of fields are reverted"
	ziti_patch services "${service}" '{"roleAttributes": ["tampered"], "tags": {"tampered": "yes"}}' || fail "cannot change the service in Ziti"
	entity_matches services "${service}" '.roleAttributes == ["tampered"]' >/dev/null || fail "could not tamper with the service"
	ziti_patch configs "${host}" '{"data": {"address": "tampered.local", "port": 9999, "protocol": "udp"}}' || fail "cannot change the host config in Ziti"
	entity_matches configs "${host}" '.data.address == "tampered.local"' >/dev/null || fail "could not tamper with the host config"
	ziti_patch service-policies "${dial}" '{"semantic": "AllOf", "identityRoles": ["#tampered"]}' || fail "cannot change the dial policy in Ziti"
	entity_matches service-policies "${dial}" '.semantic == "AllOf" and .identityRoles == ["#tampered"]' >/dev/null || fail "could not tamper with the dial policy"
	ziti_patch identities "${client}" '{"appData": {"tampered": "yes"}, "tags": {}}' || fail "cannot change the identity in Ziti"
	entity_matches identities "${client}" '.appData.tampered == "yes"' >/dev/null || fail "could not tamper with the identity"
	eventually "service role attributes and tags are restored" entity_matches services "${service}" \
		'(.roleAttributes | sort) == ["role:api", "role:web"] and .tags == {"env": "production", "team": "platform"}'
	eventually "host config data is restored" entity_matches configs "${host}" \
		'.data.address == "web-service.local" and .data.port == 8080 and .data.protocol == "tcp" and .data.listenOptions.precedence == "required"'
	eventually "dial policy semantic and roles are restored" entity_matches service-policies "${dial}" \
		'.semantic == "AnyOf" and .identityRoles == ["#web-clients"]'
	eventually "identity application data and tags are restored" entity_matches identities "${client}" \
		'(.appData | length) == 0 and .tags == {"team": "platform"}'

	step "Entities deleted in Ziti are created anew"
	ziti_delete configs "${host}" || fail "cannot delete the host config in Ziti"
	ziti_delete services "${service}" || fail "cannot delete the service in Ziti"
	ziti_delete service-policies "${dial}" || fail "cannot delete the dial policy in Ziti"
	expect_gone "host config is deleted in Ziti" configs web-service-host
	expect_gone "service is deleted in Ziti" services web-service
	expect_gone "dial policy is deleted in Ziti" service-policies web-service-dial
	eventually "host config is created anew" recreated confighostv1s web-service-host configs "${host}"
	eventually "service is created anew" recreated services web-service services "${service}"
	eventually "dial policy is created anew" recreated servicepolicies web-service-dial service-policies "${dial}"
	host="$(mr_id confighostv1s web-service-host)"
	service="$(mr_id services web-service)"
	dial="$(mr_id servicepolicies web-service-dial)"
	check_entity confighostv1s web-service-host configs \
		'.data.address == "web-service.local" and .data.port == 8080 and .data.protocol == "tcp" and .data.listenOptions.precedence == "required"'
	eventually "service refers to the new host config" entity_matches services "${service}" \
		"(.configs | sort) == ([\"${host}\", \"$(mr_id configinterceptv1s web-service-intercept)\"] | sort)"
	check_entity services web-service services \
		'.encryptionRequired == true and .terminatorStrategy == "smartrouting" and (.roleAttributes | sort) == ["role:api", "role:web"]'
	check_entity servicepolicies web-service-dial service-policies \
		".type == \"Dial\" and .semantic == \"AnyOf\" and .serviceRoles == [\"@${service}\"] and .identityRoles == [\"#web-clients\"]"
	wait_ready "${CORE_EXAMPLES[@]}"

	step "An identity deleted in Ziti is created anew with a new enrollment token"
	old="$(token_digest web-client-enrollment)" || fail "cannot read secret/web-client-enrollment"
	ziti_delete identities "${client}" || fail "cannot delete the identity in Ziti"
	expect_gone "identity is deleted in Ziti" identities web-client
	eventually "identity is created anew" recreated identities web-client identities "${client}"
	client="$(mr_id identities web-client)"
	check_entity identities web-client identities '.roleAttributes == ["web-clients"] and .tags.team == "platform"'
	expect_renewed identities web-client identities '.enrollment.ott.jwt' '.enrollment.ott.expiresAt' "${old}"

	step "Lists that name an item twice settle"
	# Ziti stores lists of strings as sets, without duplicates.
	patch services web-service '{"roleAttributes": ["role:web", "role:api", "role:web"]}'
	patch servicepolicies web-service-dial '{"identityRoles": ["#web-clients", "#web-clients"]}'
	drift_stamps() {
		stamps configs "$(mr_id confighostv1s web-service-host)"
		stamps services "$(mr_id services web-service)"
		stamps identities "$(mr_id identities web-client)"
		stamps service-policies "$(mr_id servicepolicies web-service-dial)"
	}
	token="$(token_digest web-client-enrollment)" || fail "cannot read secret/web-client-enrollment"
	expect_settled "entities with duplicates in their spec and entities created anew stay untouched" drift_stamps
	old="${token}"
	token="$(token_digest web-client-enrollment)" || fail "cannot read secret/web-client-enrollment"
	expect "secret/web-client-enrollment: the token is not replaced again" "${token}" "${old}"

	step "Deleting the managed resources deletes the entities created anew"
	delete "${CORE_EXAMPLES[@]}"
	expect_gone "service is gone" services web-service
	expect_gone "host config is gone" configs web-service-host
	expect_gone "client identity is gone" identities web-client
	expect_gone "dial policy is gone" service-policies web-service-dial
}

test_extended() {
	step "Creating the certificate authority of the identity that enrolls with its certificates"
	local ca
	openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=provider-ziti e2e CA' \
		-keyout "${WORK}/ca.key" -out "${WORK}/ca.crt" 2>/dev/null || fail "cannot create a CA certificate"
	apply_ca "${WORK}/ca.crt"
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "certificateauthorities.${GROUP}/device-ca"
	ca="$(mr_id certificateauthorities device-ca)"

	step "Creating the remaining kinds"
	apply "${EXTENDED_EXAMPLES[@]}"
	wait_ready "${EXTENDED_EXAMPLES[@]}"

	step "Checking the remaining kinds in Ziti"
	check_entity edgerouters public-router edge-routers '.roleAttributes == ["public"] and .isTunnelerEnabled == false'
	check_token public-router-enrollment edge-routers "$(mr_id edgerouters public-router)" '.enrollmentJwt'
	check_entity edgerouterpolicies web-clients-public-routers edge-router-policies \
		'.semantic == "AnyOf" and .edgeRouterRoles == ["#public"] and (.identityRoles | sort) == ["#web-clients", "#web-servers"]'
	check_entity serviceedgerouterpolicies web-services-public-routers service-edge-router-policies \
		'.semantic == "AnyOf" and .serviceRoles == ["#role:web"] and .edgeRouterRoles == ["#public"]'
	check_entity posturecheckoses supported-os posture-checks '.typeId == "OS" and (.operatingSystems | length) == 2'
	check_entity posturecheckmfas mfa posture-checks '.typeId == "MFA" and .timeoutSeconds == 3600 and .promptOnWake == true and .promptOnUnlock == true'
	check_entity authpolicies certificates-and-totp auth-policies \
		'.primary.cert.allowed == true and .primary.updb.allowed == false and .primary.extJwt.allowed == false and .secondary.requireTotp == true and .tags.team == "platform"'

	local hosts sensor operator
	hosts="$(mr_id confighostv2s web-hosts)"
	sensor="$(mr_id identitycas sensor)"
	operator="$(mr_id identityupdbs operator)"
	check_entity confighostv2s web-hosts configs \
		'.configTypeId == "host.v2" and (.data.terminators | length) == 2 and .data.terminators[0].address == "web-1.internal" and .data.terminators[0].listenOptions.precedence == "required" and .data.terminators[1].address == "web-2.internal" and .tags.team == "platform"'
	check_entity services web-pair services "(.configs == [\"${hosts}\"]) and .roleAttributes == [\"role:web\"]"
	check_entity identitycas sensor identities \
		".roleAttributes == [\"sensors\"] and .enrollment.ottca.caId == \"${ca}\" and (.enrollment | has(\"ott\") | not)"
	check_token sensor-enrollment identities "${sensor}" '.enrollment.ottca.jwt'
	check_entity identityupdbs operator identities \
		'.roleAttributes == ["operators"] and (.enrollment | has("updb")) and (.enrollment | has("ott") | not)'
	check_token operator-enrollment identities "${operator}" '.enrollment.updb.jwt'
	check_entity identitynones sso-user identities \
		'.externalId == "sso-user@example.com" and .authPolicyId == "default" and .roleAttributes == ["web-clients"] and .appData == {"department": "engineering"} and ([.enrollment[]?] | length) == 0'

	step "Updating the remaining kinds updates Ziti"
	patch edgerouters public-router '{"cost": 10, "noTraversal": true, "isTunnelerEnabled": true, "roleAttributes": ["public", "e2e"]}'
	eventually "edge router is updated in Ziti" entity_matches edge-routers "$(mr_id edgerouters public-router)" \
		'.cost == 10 and .noTraversal == true and .isTunnelerEnabled == true and (.roleAttributes | sort) == ["e2e", "public"]'
	patch edgerouterpolicies web-clients-public-routers '{"semantic": "AllOf", "identityRoles": ["#web-clients"]}'
	eventually "edge router policy is updated in Ziti" entity_matches edge-router-policies "$(mr_id edgerouterpolicies web-clients-public-routers)" \
		'.semantic == "AllOf" and .identityRoles == ["#web-clients"]'
	patch serviceedgerouterpolicies web-services-public-routers '{"edgeRouterRoles": ["#all"], "tags": {"env": "e2e"}}'
	eventually "service edge router policy is updated in Ziti" entity_matches service-edge-router-policies "$(mr_id serviceedgerouterpolicies web-services-public-routers)" \
		'.edgeRouterRoles == ["#all"] and .tags == {"env": "e2e"}'
	patch posturecheckoses supported-os '{"operatingSystems": [{"type": "Linux", "versions": [">=6.0.0"]}], "roleAttributes": ["managed-devices", "e2e"]}'
	eventually "OS posture check is updated in Ziti" entity_matches posture-checks "$(mr_id posturecheckoses supported-os)" \
		'(.operatingSystems | length) == 1 and .operatingSystems[0].versions == [">=6.0.0"] and (.roleAttributes | sort) == ["e2e", "managed-devices"]'
	patch posturecheckmfas mfa '{"timeoutSeconds": 600, "promptOnWake": false, "ignoreLegacyEndpoints": true}'
	eventually "MFA posture check is updated in Ziti" entity_matches posture-checks "$(mr_id posturecheckmfas mfa)" \
		'.timeoutSeconds == 600 and (.promptOnWake // false) == false and .promptOnUnlock == true and .ignoreLegacyEndpoints == true'
	patch authpolicies certificates-and-totp \
		'{"primary": {"cert": {"allowExpiredCerts": true}, "updb": {"allowed": true, "minPasswordLength": 12, "requireNumberChar": true}}, "secondary": {"requireTotp": false}}'
	eventually "auth policy is updated in Ziti" entity_matches auth-policies "$(mr_id authpolicies certificates-and-totp)" \
		'.primary.cert.allowExpiredCerts == true and .primary.updb.allowed == true and .primary.updb.minPasswordLength == 12 and .primary.updb.requireNumberChar == true and (.secondary.requireTotp // false) == false'

	patch confighostv2s web-hosts '{"terminators": [{"address": "web-1.internal", "port": 8443, "protocol": "tcp"}], "tags": null}'
	eventually "host.v2 config is updated in Ziti" entity_matches configs "${hosts}" \
		'(.data.terminators | length) == 1 and .data.terminators[0].port == 8443 and (.tags | length) == 0'
	patch identitycas sensor '{"roleAttributes": ["sensors", "e2e"], "defaultHostingCost": 5}'
	eventually "CA identity is updated in Ziti" entity_matches identities "${sensor}" \
		'(.roleAttributes | sort) == ["e2e", "sensors"] and .defaultHostingCost == 5'
	patch identityupdbs operator '{"isAdmin": true, "tags": {"env": "e2e"}}'
	eventually "password identity is updated in Ziti" entity_matches identities "${operator}" \
		'.isAdmin == true and .tags == {"env": "e2e"}'
	patch identitynones sso-user '{"externalId": "sso-user@example.org", "appData": null}'
	eventually "identity without an enrollment is updated in Ziti" entity_matches identities "$(mr_id identitynones sso-user)" \
		'.externalId == "sso-user@example.org" and (.appData | length) == 0'

	step "Nothing is updated without a spec change"
	extended_stamps() {
		stamps configs "${hosts}"
		stamps services "$(mr_id services web-pair)"
		stamps identities "${sensor}" "${operator}" "$(mr_id identitynones sso-user)"
		stamps edge-routers "$(mr_id edgerouters public-router)"
		stamps edge-router-policies "$(mr_id edgerouterpolicies web-clients-public-routers)"
		stamps service-edge-router-policies "$(mr_id serviceedgerouterpolicies web-services-public-routers)"
		stamps posture-checks "$(mr_id posturecheckoses supported-os)" "$(mr_id posturecheckmfas mfa)"
		stamps auth-policies "$(mr_id authpolicies certificates-and-totp)"
	}
	expect_settled "remaining entities stay untouched" extended_stamps

	step "Deleting the remaining kinds"
	# The certificate authority goes at the same time as the identity that
	# refers to it.
	kubectl delete --wait=false "certificateauthorities.${GROUP}/device-ca"
	delete "${EXTENDED_EXAMPLES[@]}"
	eventually "certificate authority is deleted along with the identity that refers to it" mr_gone certificateauthorities device-ca
	expect_gone "edge router is gone" edge-routers public-router
	expect_gone "edge router policy is gone" edge-router-policies web-clients-public-routers
	expect_gone "service edge router policy is gone" service-edge-router-policies web-services-public-routers
	expect_gone "OS posture check is gone" posture-checks supported-os
	expect_gone "MFA posture check is gone" posture-checks mfa
	expect_gone "auth policy is gone" auth-policies certificates-and-totp
	expect_gone "host.v2 config is gone" configs web-hosts
	expect_gone "service with the host.v2 config is gone" services web-pair
	expect_gone "CA identity is gone" identities sensor
	expect_gone "password identity is gone" identities operator
	expect_gone "identity without an enrollment is gone" identities sso-user
	expect_gone "certificate authority is gone" cas device-ca
}

test_authentication() {
	step "Creating a certificate authority, an external JWT signer and what refers to them"
	openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=provider-ziti e2e device CA' \
		-keyout "${WORK}/device-ca.key" -out "${WORK}/device-ca.crt" 2>/dev/null || fail "cannot create a CA certificate"
	# The auth policy and the identity are created along with what they refer
	# to by name, and are retried until that exists.
	apply_ca "${WORK}/device-ca.crt"
	apply "${AUTHENTICATION_EXAMPLES[@]}"
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "certificateauthorities.${GROUP}/device-ca"
	wait_ready "${AUTHENTICATION_EXAMPLES[@]}"

	local ca signer policy sensor fingerprint token refusal enrollment published
	ca="$(mr_id certificateauthorities device-ca)"
	signer="$(mr_id externaljwtsigners corporate-sso)"
	policy="$(mr_id authpolicies corporate-sso-only)"
	sensor="$(mr_id identitycas sensor)"
	# Ziti identifies a certificate by its SHA-1 fingerprint in lower case.
	fingerprint="$(openssl x509 -in "${WORK}/device-ca.crt" -noout -fingerprint -sha1 | sed -e 's/.*=//' -e 's/://g' | tr 'A-F' 'a-f')"
	[ -n "${fingerprint}" ] || fail "cannot compute the fingerprint of the CA certificate"

	step "Checking them in Ziti"
	check_entity certificateauthorities device-ca cas \
		".fingerprint == \"${fingerprint}\" and .isVerified == false and .isAuthEnabled == true and .isOttCaEnrollmentEnabled == true and .isAutoCaEnrollmentEnabled == false and .identityNameFormat == \"[caName]-[commonName]\" and (.identityRoles | length) == 0 and .externalIdClaim == null and .tags.team == \"platform\""
	expect "certificate authority has the certificate of the test" \
		"$(ziti_get cas "${ca}" | jq -r '.certPem')" "$(cat "${WORK}/device-ca.crt")"
	# What the owner of the certificate authority needs to verify it.
	expect "certificateauthorities/device-ca: status has the fingerprint" \
		"$(kubectl get "certificateauthorities.${GROUP}" device-ca -o jsonpath='{.status.atProvider.fingerprint}')" "${fingerprint}"
	expect "certificateauthorities/device-ca: status says it is not verified" \
		"$(kubectl get "certificateauthorities.${GROUP}" device-ca -o jsonpath='{.status.atProvider.isVerified}')" ""
	token="$(kubectl get "certificateauthorities.${GROUP}" device-ca -o jsonpath='{.status.atProvider.verificationToken}')"
	# Never print the token itself.
	if [ -n "${token}" ] && [ "${token}" = "$(ziti_get cas "${ca}" | jq -r '.verificationToken')" ]; then
		ok "certificateauthorities/device-ca: status has the verification token"
	else
		fail "certificateauthorities/device-ca: status.atProvider.verificationToken is not the token Ziti reports"
	fi
	check_entity externaljwtsigners corporate-sso external-jwt-signers \
		'.issuer == "https://sso.example.com/realms/corporate" and .audience == "ziti" and .enabled == true and .jwksEndpoint == "https://sso.example.com/realms/corporate/protocol/openid-connect/certs" and .certPem == null and .kid == null and .claimsProperty == "email" and .useExternalId == true and .externalAuthUrl == "https://sso.example.com/realms/corporate" and .clientId == "ziti-clients" and (.scopes | sort) == ["email", "profile"] and .targetToken == "ID" and .tags == {"team": "platform"}'
	check_entity authpolicies corporate-sso-only auth-policies \
		".primary.extJwt.allowed == true and .primary.extJwt.allowedSigners == [\"${signer}\"] and .primary.cert.allowed == false and .primary.updb.allowed == false and .secondary.requireExtJwtSigner == null"
	check_entity identitycas sensor identities ".enrollment.ottca.caId == \"${ca}\""

	step "Updating them updates Ziti"
	patch certificateauthorities device-ca \
		'{"isAuthEnabled": false, "isAutoCaEnrollmentEnabled": true, "identityRoles": ["sensors", "e2e"], "identityNameFormat": "[caName]-[requestedName]", "externalIdClaim": {"location": "SAN_URI", "matcher": "SCHEME", "matcherCriteria": "spiffe", "parser": "SPLIT", "parserCriteria": "/", "index": 2}, "tags": {"env": "e2e", "team": null}}'
	eventually "certificate authority is updated in Ziti" entity_matches cas "${ca}" \
		'.isAuthEnabled == false and .isAutoCaEnrollmentEnabled == true and .isOttCaEnrollmentEnabled == true and (.identityRoles | sort) == ["e2e", "sensors"] and .identityNameFormat == "[caName]-[requestedName]" and .externalIdClaim == {"location": "SAN_URI", "matcher": "SCHEME", "matcherCriteria": "spiffe", "parser": "SPLIT", "parserCriteria": "/", "index": 2} and .tags == {"env": "e2e"}'
	expect "certificate authority keeps its Ziti ID" "$(mr_id certificateauthorities device-ca)" "${ca}"
	expect "certificate authority keeps its certificate" "$(ziti_get cas "${ca}" | jq -r '.fingerprint')" "${fingerprint}"
	refusal="$(patch certificateauthorities device-ca '{"certPem": "another certificate"}' 2>&1)" &&
		fail "the certificate of a certificate authority was changed"
	case "${refusal}" in
	*"certPem cannot be changed after creation"*) ok "the certificate of a certificate authority cannot be changed" ;;
	*) fail "changing the certificate of a certificate authority: ${refusal}" ;;
	esac
	# The signer changes from a JWKS endpoint to a certificate, for which any
	# certificate will do, and loses its client ID.
	patch externaljwtsigners corporate-sso "$(jq -n --rawfile pem "${WORK}/device-ca.crt" \
		'{audience: "ziti-edge", jwksEndpoint: null, certPem: $pem, kid: "e2e-key", clientId: null, scopes: ["openid"], targetToken: "ACCESS", enrollNameClaimsSelector: "preferred_username", enrollAttributeClaimsSelector: "groups", tags: {env: "e2e", team: null}}')"
	eventually "external JWT signer is updated in Ziti" entity_matches external-jwt-signers "${signer}" \
		'.audience == "ziti-edge" and .jwksEndpoint == null and .commonName == "provider-ziti e2e device CA" and .kid == "e2e-key" and .clientId == null and .scopes == ["openid"] and .targetToken == "ACCESS" and .enrollNameClaimsSelector == "preferred_username" and .enrollAttributeClaimsSelector == "groups" and .claimsProperty == "email" and .enabled == true and .tags == {"env": "e2e"}'
	expect "external JWT signer has the certificate of the test" \
		"$(ziti_get external-jwt-signers "${signer}" | jq -r '.certPem')" "$(cat "${WORK}/device-ca.crt")"
	expect "external JWT signer keeps its Ziti ID" "$(mr_id externaljwtsigners corporate-sso)" "${signer}"

	step "Changes made in Ziti behind the provider's back are reverted"
	ziti_patch cas "${ca}" '{"isOttCaEnrollmentEnabled": false, "identityRoles": ["tampered"]}'
	entity_matches cas "${ca}" '.identityRoles == ["tampered"]' >/dev/null || fail "could not tamper with the certificate authority"
	# Which is why the provider replaces a certificate authority instead.
	entity_matches cas "${ca}" '.externalIdClaim == null' >/dev/null ||
		fail "a PATCH without the external ID claim of a certificate authority no longer takes the claim away"
	ok "a PATCH of a certificate authority took its external ID claim away"
	eventually "certificate authority is restored" entity_matches cas "${ca}" \
		'.isOttCaEnrollmentEnabled == true and (.identityRoles | sort) == ["e2e", "sensors"] and .externalIdClaim.parserCriteria == "/" and .externalIdClaim.index == 2'
	ziti_patch external-jwt-signers "${signer}" '{"audience": "tampered", "enabled": false}'
	entity_matches external-jwt-signers "${signer}" '.audience == "tampered"' >/dev/null || fail "could not tamper with the external JWT signer"
	eventually "external JWT signer is restored" entity_matches external-jwt-signers "${signer}" '.audience == "ziti-edge" and .enabled == true'
	ziti_patch auth-policies "${policy}" '{"primary": {"extJwt": {"allowedSigners": []}}}'
	entity_matches auth-policies "${policy}" '(.primary.extJwt.allowedSigners | length) == 0' >/dev/null || fail "could not tamper with the auth policy"
	eventually "auth policy refers to its signer again" entity_matches auth-policies "${policy}" ".primary.extJwt.allowedSigners == [\"${signer}\"]"

	step "A setting that is taken out of the spec is removed in Ziti"
	patch certificateauthorities device-ca '{"externalIdClaim": null}'
	eventually "external ID claim is removed in Ziti" entity_matches cas "${ca}" '.externalIdClaim == null'

	step "The owner of the certificate authority verifies it"
	# The proof is a certificate with the verification token as its common
	# name, signed with the key of the certificate authority. Neither openssl
	# nor curl may print the token.
	openssl req -new -newkey rsa:2048 -nodes -subj "/CN=${token}" \
		-keyout "${WORK}/verify.key" -out "${WORK}/verify.csr" 2>/dev/null || fail "cannot create the proof"
	openssl x509 -req -in "${WORK}/verify.csr" -CA "${WORK}/device-ca.crt" -CAkey "${WORK}/device-ca.key" -CAcreateserial \
		-days 1 -out "${WORK}/verify.crt" 2>/dev/null || fail "cannot sign the proof"
	curl -sk --fail -X POST -H "zt-session: ${ZITI_TOKEN}" -H 'Content-Type: text/plain' \
		--data-binary "@${WORK}/verify.crt" "${ZITI_API}/cas/${ca}/verify" >/dev/null || fail "Ziti does not accept the proof"
	eventually "certificate authority reports that it is verified" status_is certificateauthorities device-ca isVerified true
	patch certificateauthorities device-ca '{"isAuthEnabled": true}'
	eventually "certificate authority stays verified when it is updated" entity_matches cas "${ca}" '.isAuthEnabled == true and .isVerified == true'

	step "The identity enrolls with a certificate of the certificate authority"
	openssl req -new -newkey rsa:2048 -nodes -subj '/CN=sensor' \
		-keyout "${WORK}/sensor.key" -out "${WORK}/sensor.csr" 2>/dev/null || fail "cannot create a client certificate request"
	printf 'extendedKeyUsage=clientAuth\n' >"${WORK}/sensor.ext"
	openssl x509 -req -in "${WORK}/sensor.csr" -CA "${WORK}/device-ca.crt" -CAkey "${WORK}/device-ca.key" -CAcreateserial \
		-days 1 -extfile "${WORK}/sensor.ext" -out "${WORK}/sensor.crt" 2>/dev/null || fail "cannot sign the client certificate"
	check_token sensor-enrollment identities "${sensor}" '.enrollment.ottca.jwt'
	published="$(kubectl get secret sensor-enrollment -o jsonpath='{.data.enrollmentToken}')"
	enrollment="$(ziti_get identities "${sensor}" | jq -er '.enrollment.ottca.token')" || fail "the identity has no enrollment token"
	curl -sk --fail -X POST --cert "${WORK}/sensor.crt" --key "${WORK}/sensor.key" -H 'Content-Type: application/json' \
		-d "{\"token\": \"${enrollment}\"}" "${ZITI_URL}/edge/client/v1/enroll/ottca" >/dev/null ||
		fail "the identity cannot enroll with its token and its client certificate"
	entity_matches identities "${sensor}" \
		"([.enrollment[]?] | length) == 0 and .authenticators.cert.fingerprint == \"$(openssl x509 -in "${WORK}/sensor.crt" -noout -fingerprint -sha1 | sed -e 's/.*=//' -e 's/://g' | tr 'A-F' 'a-f')\"" >/dev/null ||
		fail "Ziti does not know the identity by its client certificate"
	ok "identity is enrolled in Ziti with its client certificate"
	eventually "identity reports that it is enrolled" status_is identitycas sensor enrolled true
	expect "identity no longer reports a pending enrollment" \
		"$(kubectl get "identitycas.${GROUP}" sensor -o jsonpath='{.status.atProvider.enrollmentExpiresAt}')" ""
	# The connection secret keeps the token that was used, or loses it, but
	# never gets another one. The token itself is never printed.
	case "$(kubectl get secret sensor-enrollment -o jsonpath='{.data.enrollmentToken}')" in
	"" | "${published}") ok "no enrollment token is published for an identity that has enrolled" ;;
	*) fail "secret/sensor-enrollment got a new enrollment token after the identity enrolled" ;;
	esac

	step "Nothing is updated without a spec change"
	authentication_stamps() {
		stamps cas "${ca}"
		stamps external-jwt-signers "${signer}"
		stamps auth-policies "${policy}"
		stamps identities "${sensor}"
	}
	expect_settled "authentication entities stay untouched" authentication_stamps

	step "An external JWT signer outlives the auth policy that refers to it"
	kubectl delete --wait=false "externaljwtsigners.${GROUP}/corporate-sso"
	eventually "signer reports that Ziti refuses to delete it" sync_error_has externaljwtsigners corporate-sso 'CAN_NOT_DELETE_REFERENCED_ENTITY'
	expect "signer is still in Ziti" "$(ziti_id external-jwt-signers corporate-sso)" "${signer}"
	delete examples/authpolicy/jwt.yaml
	expect_gone "auth policy is gone" auth-policies corporate-sso-only
	eventually "signer is deleted once no auth policy refers to it" mr_gone externaljwtsigners corporate-sso
	expect_gone "external JWT signer is gone" external-jwt-signers corporate-sso

	step "An identity is deleted before its certificate authority"
	delete examples/identity/ca.yaml
	expect_gone "identity is gone" identities sensor
	expect "certificate authority is still in Ziti" "$(ziti_id cas device-ca)" "${ca}"

	step "A certificate authority is deleted before the identity that refers to it"
	apply examples/identity/ca.yaml
	wait_ready examples/identity/ca.yaml
	sensor="$(mr_id identitycas sensor)"
	check_entity identitycas sensor identities ".enrollment.ottca.caId == \"${ca}\""
	kubectl delete --wait --timeout="${TIMEOUT}s" "certificateauthorities.${GROUP}/device-ca"
	expect_gone "certificate authority is gone" cas device-ca
	# Ziti deletes the pending enrollments of a certificate authority with it.
	entity_matches identities "${sensor}" '.name == "sensor" and ([.enrollment[]?] | length) == 0' >/dev/null ||
		fail "the identity is gone or kept an enrollment with a certificate authority that is gone"
	ok "identity is still in Ziti, without its enrollment"
	delete examples/identity/ca.yaml
	expect_gone "identity is gone" identities sensor
}

test_renewal() {
	step "Creating identities and edge routers to take the enrollment from"
	local ca
	openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=provider-ziti e2e renewal CA' \
		-keyout "${WORK}/renewal-ca.key" -out "${WORK}/renewal-ca.crt" 2>/dev/null || fail "cannot create a CA certificate"
	ca="$(ziti_post cas "$(jq -n --rawfile pem "${WORK}/renewal-ca.crt" \
		'{name: "e2e-renewal-ca", certPem: $pem, isAuthEnabled: true, isAutoCaEnrollmentEnabled: false, isOttCaEnrollmentEnabled: true, identityRoles: []}')")" ||
		fail "cannot create a certificate authority in Ziti"

	# declare_resource <kind> <name> [setting] creates a managed resource that
	# writes its enrollment token to the secret <name>-enrollment.
	declare_resource() {
		kubectl apply -f - <<EOF
apiVersion: ${GROUP}/v1alpha1
kind: $1
metadata:
  name: $2
  namespace: default
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  writeConnectionSecretToRef:
    name: $2-enrollment
  forProvider:
    name: $2
    ${3:-}
EOF
	}
	# Only a token that replaces another one is valid for the 10 minutes.
	declare_resource Identity e2e-expired 'enrollmentDuration: 10m'
	declare_resource Identity e2e-deleted
	declare_resource Identity e2e-enrolled
	declare_resource Identity e2e-external 'externalId: e2e-external@example.org'
	declare_resource Identity e2e-by-id
	declare_resource IdentityCA e2e-deleted-ca 'ottca: e2e-renewal-ca'
	declare_resource IdentityUPDB e2e-deleted-updb 'updbUsername: e2e-deleted-updb'
	declare_resource EdgeRouter e2e-deleted-router
	declare_resource EdgeRouter e2e-enrolled-router
	local resources=(
		"identities.${GROUP}/e2e-expired" "identities.${GROUP}/e2e-deleted" "identities.${GROUP}/e2e-enrolled"
		"identities.${GROUP}/e2e-external" "identities.${GROUP}/e2e-by-id"
		"identitycas.${GROUP}/e2e-deleted-ca" "identityupdbs.${GROUP}/e2e-deleted-updb"
		"edgerouters.${GROUP}/e2e-deleted-router" "edgerouters.${GROUP}/e2e-enrolled-router"
	)
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "${resources[@]}"

	local expired deleted with_ca with_password router enrolled enrolled_router external by_id
	expired="$(mr_id identities e2e-expired)"
	deleted="$(mr_id identities e2e-deleted)"
	with_ca="$(mr_id identitycas e2e-deleted-ca)"
	with_password="$(mr_id identityupdbs e2e-deleted-updb)"
	router="$(mr_id edgerouters e2e-deleted-router)"
	enrolled="$(mr_id identities e2e-enrolled)"
	enrolled_router="$(mr_id edgerouters e2e-enrolled-router)"
	external="$(mr_id identities e2e-external)"
	by_id="$(mr_id identities e2e-by-id)"
	check_token e2e-expired-enrollment identities "${expired}" '.enrollment.ott.jwt'
	check_token e2e-deleted-enrollment identities "${deleted}" '.enrollment.ott.jwt'
	check_token e2e-deleted-ca-enrollment identities "${with_ca}" '.enrollment.ottca.jwt'
	check_token e2e-deleted-updb-enrollment identities "${with_password}" '.enrollment.updb.jwt'
	check_token e2e-deleted-router-enrollment edge-routers "${router}" '.enrollmentJwt'
	check_token e2e-enrolled-enrollment identities "${enrolled}" '.enrollment.ott.jwt'
	check_token e2e-enrolled-router-enrollment edge-routers "${enrolled_router}" '.enrollmentJwt'
	check_token e2e-external-enrollment identities "${external}" '.enrollment.ott.jwt'
	check_token e2e-by-id-enrollment identities "${by_id}" '.enrollment.ott.jwt'
	eventually "edgerouters/e2e-deleted-router: status says when the token expires" \
		expiry_is_current edgerouters e2e-deleted-router edge-routers '.enrollmentExpiresAt'

	step "An enrollment token that expired is replaced"
	local old enrollment
	old="$(token_digest e2e-expired-enrollment)" || fail "cannot read secret/e2e-expired-enrollment"
	enrollment="$(ziti_get identities "${expired}" | jq -er '.enrollment.ott.id')" || fail "e2e-expired has no enrollment"
	# The first token is valid for hours. Ziti is asked for one that expires
	# in a moment instead.
	ziti_refresh "${enrollment}" 5 || fail "cannot refresh the enrollment of e2e-expired in Ziti"
	expires_in identities "${expired}" '.enrollment.ott.expiresAt' -60 10 >/dev/null || fail "could not make the enrollment of e2e-expired expire"
	eventually "identities/e2e-expired: the enrollment is valid again, for the 10 minutes the resource asks for" \
		expires_in identities "${expired}" '.enrollment.ott.expiresAt' 300 600
	expect "identities/e2e-expired: the enrollment is the one that expired" \
		"$(ziti_get identities "${expired}" | jq -r '.enrollment.ott.id')" "${enrollment}"
	expect_renewed identities e2e-expired identities '.enrollment.ott.jwt' '.enrollment.ott.expiresAt' "${old}"

	step "An enrollment that is gone is created anew"
	# enrollment_recreated <plural> <name> <method> deletes the enrollment of an identity
	# in Ziti and checks that the identity gets another one.
	enrollment_recreated() {
		local plural="$1" name="$2" method="$3" id old enrollment
		id="$(mr_id "${plural}" "${name}")"
		old="$(token_digest "${name}-enrollment")" || fail "cannot read secret/${name}-enrollment"
		enrollment="$(ziti_get identities "${id}" | jq -er ".enrollment.${method}.id")" || fail "${name} has no enrollment"
		ziti_delete enrollments "${enrollment}" || fail "cannot delete the enrollment of ${name} in Ziti"
		eventually "${plural}/${name}: has an enrollment again" entity_matches identities "${id}" \
			".enrollment.${method}.id != null and .enrollment.${method}.id != \"${enrollment}\""
		expect_renewed "${plural}" "${name}" identities ".enrollment.${method}.jwt" ".enrollment.${method}.expiresAt" "${old}"
	}
	enrollment_recreated identities e2e-deleted ott
	enrollment_recreated identitycas e2e-deleted-ca ottca
	enrollment_recreated identityupdbs e2e-deleted-updb updb
	expires_in identities "${deleted}" '.enrollment.ott.expiresAt' 10200 10800 >/dev/null ||
		fail "the new enrollment of e2e-deleted is not valid for 180 minutes"
	ok "identities/e2e-deleted: the new token is valid for 180 minutes"
	entity_matches identities "${with_ca}" ".enrollment.ottca.caId == \"${ca}\" and (.enrollment | has(\"ott\") | not)" >/dev/null ||
		fail "the new enrollment of e2e-deleted-ca is not one for its certificate authority"
	ok "identitycas/e2e-deleted-ca: the new enrollment is one for its certificate authority"
	entity_matches identities "${with_password}" '(.enrollment | has("updb")) and (.enrollment | has("ott") | not)' >/dev/null ||
		fail "the new enrollment of e2e-deleted-updb is not one for a password"
	ok "identityupdbs/e2e-deleted-updb: the new enrollment is one for a password"

	step "An edge router whose enrollment is gone is enrolled anew"
	local old_expiry
	old="$(token_digest e2e-deleted-router-enrollment)" || fail "cannot read secret/e2e-deleted-router-enrollment"
	old_expiry="$(ziti_get edge-routers "${router}" | jq -er '.enrollmentExpiresAt')" || fail "e2e-deleted-router has no enrollment"
	enrollment="$(curl -sk --fail -H "zt-session: ${ZITI_TOKEN}" --get --data-urlencode "filter=edgeRouter=\"${router}\"" \
		"${ZITI_API}/enrollments" | jq -er '.data[0].id')" || fail "cannot find the enrollment of e2e-deleted-router"
	ziti_delete enrollments "${enrollment}" || fail "cannot delete the enrollment of e2e-deleted-router in Ziti"
	eventually "edgerouters/e2e-deleted-router: has an enrollment again" entity_matches edge-routers "${router}" \
		".isVerified == false and .enrollmentJwt != null and .enrollmentExpiresAt != \"${old_expiry}\""
	expect_renewed edgerouters e2e-deleted-router edge-routers '.enrollmentJwt' '.enrollmentExpiresAt' "${old}"

	step "A new token can be used, and what has enrolled is left alone"
	local token
	# Ziti only shows the username of a password enrollment once it is used.
	token="$(ziti_get identities "${with_password}" | jq -er '.enrollment.updb.token')" || fail "e2e-deleted-updb has no enrollment"
	curl -sk --fail -X POST -H 'Content-Type: application/json' -H 'Accept: application/json' \
		-d "$(jq -n --arg password "$(openssl rand -hex 16)" '{password: $password}')" \
		"${ZITI_URL}/edge/client/v1/enroll?method=updb&token=${token}" >/dev/null ||
		fail "cannot enroll e2e-deleted-updb with its new token"
	entity_matches identities "${with_password}" '.authenticators.updb.username == "e2e-deleted-updb" and ([.enrollment[]?] | length) == 0' >/dev/null ||
		fail "e2e-deleted-updb has not enrolled with the username of its resource"
	ok "identityupdbs/e2e-deleted-updb: enrolled with its new token and the username of its resource"

	# The identity enrolls like a Ziti SDK does: with a certificate request
	# and the token its enrollment JWT stands for.
	openssl req -new -newkey rsa:2048 -nodes -subj '/CN=e2e-enrolled' \
		-keyout "${WORK}/enrolled.key" -out "${WORK}/enrolled.csr" 2>/dev/null || fail "cannot create a certificate request"
	token="$(ziti_get identities "${enrolled}" | jq -er '.enrollment.ott.token')" || fail "e2e-enrolled has no enrollment"
	curl -sk --fail -X POST -H 'Content-Type: application/x-pem-file' -H 'Accept: application/json' \
		--data-binary "@${WORK}/enrolled.csr" "${ZITI_URL}/edge/client/v1/enroll?method=ott&token=${token}" >/dev/null ||
		fail "cannot enroll e2e-enrolled"
	# An edge router asks for two certificates. The one it connects to the
	# controller with must name its ID.
	openssl req -new -newkey rsa:2048 -nodes -subj "/CN=${enrolled_router}" \
		-keyout "${WORK}/enrolled-router.key" -out "${WORK}/enrolled-router.csr" 2>/dev/null || fail "cannot create a certificate request"
	openssl req -new -key "${WORK}/enrolled-router.key" -subj '/CN=e2e-enrolled-router' \
		-out "${WORK}/enrolled-router-server.csr" 2>/dev/null || fail "cannot create a certificate request"
	token="$(ziti_get edge-routers "${enrolled_router}" | jq -er '.enrollmentToken')" || fail "e2e-enrolled-router has no enrollment"
	curl -sk --fail -X POST -H 'Content-Type: application/json' -H 'Accept: application/json' \
		-d "$(jq -n --rawfile client "${WORK}/enrolled-router.csr" --rawfile server "${WORK}/enrolled-router-server.csr" \
			'{certCsr: $client, serverCertCsr: $server}')" \
		"${ZITI_URL}/edge/client/v1/enroll?method=erott&token=${token}" >/dev/null ||
		fail "cannot enroll e2e-enrolled-router"

	# has_enrolled succeeds if both have enrolled and have no enrollment.
	has_enrolled() {
		entity_matches identities "${enrolled}" '.authenticators.cert.id != null and ([.enrollment[]?] | length) == 0' &&
			entity_matches edge-routers "${enrolled_router}" '.isVerified == true and .fingerprint != "" and .enrollmentJwt == null'
	}
	has_enrolled >/dev/null || fail "e2e-enrolled or e2e-enrolled-router has not enrolled"
	ok "identities/e2e-enrolled and edgerouters/e2e-enrolled-router: enrolled with their tokens"
	reports_enrolled() {
		[ "$(kubectl get "identities.${GROUP}" e2e-enrolled -o jsonpath='{.status.atProvider.enrolled}')" = "true" ] &&
			[ "$(kubectl get "edgerouters.${GROUP}" e2e-enrolled-router -o jsonpath='{.status.atProvider.isVerified}')" = "true" ]
	}
	eventually "identities/e2e-enrolled and edgerouters/e2e-enrolled-router: report that they have enrolled" reports_enrolled

	# The certificates of the enrolled entities, every enrollment without
	# its token, and the last update of every entity.
	enrollment_stamps() {
		ziti_get identities "${enrolled}" | jq -r '"\(.id) \(.updatedAt) \(.authenticators.cert.fingerprint) \([.enrollment[] | .id, .expiresAt])"'
		ziti_get edge-routers "${enrolled_router}" | jq -r '"\(.id) \(.updatedAt) \(.isVerified) \(.fingerprint) \(.enrollmentExpiresAt)"'
		local id
		for id in "${expired}" "${deleted}" "${with_ca}" "${with_password}"; do
			ziti_get identities "${id}" | jq -r '"\(.id) \(.updatedAt) \([.enrollment[] | .id, .expiresAt])"'
		done
		ziti_get edge-routers "${router}" | jq -r '"\(.id) \(.updatedAt) \(.isVerified) \(.enrollmentExpiresAt)"'
	}
	expect_settled "enrolled entities are left alone and no token is replaced twice" enrollment_stamps
	has_enrolled >/dev/null || fail "e2e-enrolled or e2e-enrolled-router was given an enrollment after it had enrolled"
	ok "identities/e2e-enrolled and edgerouters/e2e-enrolled-router: have no enrollment and keep their certificates"
	local resource
	for resource in "${resources[@]}"; do
		expect "${resource}: is in sync" "$(kubectl get "${resource}" -o jsonpath='{.status.conditions[?(@.type=="Synced")].status}')" "True"
	done

	step "An identity that can sign in without an authenticator gets no enrollment"
	# Ziti reports no authenticator for an identity that signs in with an
	# external JWT, or with a certificate of a certificate authority that
	# carries its external ID. One with an external ID can do so, and so can
	# every identity whose auth policy allows a signer that looks identities
	# up by their ID, which the default policy does for every signer.
	entity_matches auth-policies default '.primary.extJwt.allowed == true and (.primary.extJwt.allowedSigners | length) == 0' >/dev/null ||
		fail "the default auth policy does not allow every external JWT signer"
	openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=provider-ziti e2e renewal signer' \
		-keyout "${WORK}/renewal-signer.key" -out "${WORK}/renewal-signer.crt" 2>/dev/null || fail "cannot create a signer certificate"
	local signer id
	signer="$(ziti_post external-jwt-signers "$(jq -n --rawfile pem "${WORK}/renewal-signer.crt" \
		'{name: "e2e-renewal-by-id", issuer: "https://e2e-renewal.example.org", audience: "ziti", certPem: $pem, enabled: true, useExternalId: false}')")" ||
		fail "cannot create an external JWT signer in Ziti"
	for id in "${external}" "${by_id}"; do
		enrollment="$(ziti_get identities "${id}" | jq -er '.enrollment.ott.id')" || fail "identity ${id} has no enrollment"
		ziti_delete enrollments "${enrollment}" || fail "cannot delete the enrollment of identity ${id} in Ziti"
	done
	# The provider polls several times meanwhile.
	sleep "${SETTLE}"
	entity_matches identities "${external}" '.externalId == "e2e-external@example.org" and ([.enrollment[]?] | length) == 0' >/dev/null ||
		fail "identities/e2e-external got an enrollment although it can sign in with its external ID"
	ok "identities/e2e-external: gets no enrollment, it can sign in with its external ID"
	entity_matches identities "${by_id}" '([.enrollment[]?] | length) == 0' >/dev/null ||
		fail "identities/e2e-by-id got an enrollment although it can sign in with a JWT that names its ID"
	ok "identities/e2e-by-id: gets no enrollment while a signer looks identities up by their ID"
	ziti_delete external-jwt-signers "${signer}" || fail "cannot delete the external JWT signer"
	eventually "identities/e2e-by-id: has an enrollment again once no signer looks it up by its ID" \
		entity_matches identities "${by_id}" '.enrollment.ott.id != null'
	entity_matches identities "${external}" '([.enrollment[]?] | length) == 0' >/dev/null ||
		fail "identities/e2e-external got an enrollment although it can sign in with its external ID"
	ok "identities/e2e-external: still has no enrollment"

	step "Deleting the identities and edge routers"
	kubectl delete --wait --timeout="${TIMEOUT}s" "${resources[@]}"
	local name
	for name in e2e-expired e2e-deleted e2e-enrolled e2e-external e2e-by-id e2e-deleted-ca e2e-deleted-updb; do
		expect_gone "identity ${name} is gone" identities "${name}"
	done
	expect_gone "edge router e2e-deleted-router is gone" edge-routers e2e-deleted-router
	expect_gone "edge router e2e-enrolled-router is gone" edge-routers e2e-enrolled-router
	ziti_delete cas "${ca}" || fail "cannot delete the certificate authority"
}

test_posture_checks() {
	# The examples give hashes and fingerprints in upper case and with
	# separators, MAC addresses in two notations and lists out of order. Ziti
	# stores hexadecimal values in lower case and without separators, and
	# lists sorted. These are the values of the examples as Ziti stores them.
	local agent_hash="33679befb5996cabe53923efc754e19fc0cac7a63d0ceddb14e7ff7e5f5eba644fe833615d0580c2b8e68d481e1d547a34d422a9b8e93159b65e3295aec92d3d"
	local linux_hash_1="085b1d2647e1800b17809504600d01839f13fef57e647609b8c4a79b24c924b5281a01ae321606b1b820dc951a11470058b222400e5017e2144b5083d2617a97"
	local linux_hash_2="a7fe5c886fad6bc932ada51ac4d35faa43a2f0a29bfd3122e1b28c1d4a1e4e5f33eb5f61b21ffe12994411445e855563266ff093d853279acc240ee426d60ab3"
	local windows_signer="6b5ada26eecb0e04309cf0425ceb0744f0f454b9"
	local macos_signer="f5b98b6fb446f07438f2fa72f55d60edd8b7e361"

	step "Creating the posture checks"
	apply "${POSTURE_CHECK_EXAMPLES[@]}"
	# Ziti returns the operating systems of an OS check in the order of their
	# type, and their versions sorted.
	kubectl apply -f - <<EOF
apiVersion: ${GROUP}/v1alpha1
kind: PostureCheckOS
metadata:
  name: e2e-os-out-of-order
  namespace: default
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  forProvider:
    name: e2e-os-out-of-order
    operatingSystems:
      - type: macOS
        versions: [">=14.0.0", ">=13.0.0"]
      - type: Windows
        versions: [">=10.0.19045"]
      - type: Linux
        versions: [">=6.0.0", ">=5.10.0", ">=6.0.0"]
EOF
	wait_ready "${POSTURE_CHECK_EXAMPLES[@]}"
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "posturecheckoses.${GROUP}/e2e-os-out-of-order"

	local domain mac process multi os
	domain="$(mr_id posturecheckdomains corporate-domain)"
	mac="$(mr_id posturecheckmacs registered-devices)"
	process="$(mr_id posturecheckprocesses windows-agent)"
	multi="$(mr_id posturecheckmultiprocesses endpoint-agents)"
	os="$(mr_id posturecheckoses e2e-os-out-of-order)"

	step "Checking the posture checks in Ziti"
	check_entity posturecheckdomains corporate-domain posture-checks \
		'.typeId == "DOMAIN" and .domains == ["branch.example.com", "corp.example.com"] and .roleAttributes == ["managed-devices"]'
	check_entity posturecheckmacs registered-devices posture-checks \
		'.typeId == "MAC" and .macAddresses == ["001a2b3c4d5e", "0a1b2c3d4e5f"] and .roleAttributes == ["registered-devices"]'
	check_entity posturecheckprocesses windows-agent posture-checks \
		".typeId == \"PROCESS\" and .process.osType == \"Windows\" and .process.path == \"C:\\\\Program Files\\\\Endpoint Agent\\\\agent.exe\" and .process.hashes == [\"${agent_hash}\"] and .process.signerFingerprint == \"${windows_signer}\" and .roleAttributes == [\"managed-devices\"]"
	check_entity posturecheckmultiprocesses endpoint-agents posture-checks \
		".typeId == \"PROCESS_MULTI\" and .semantic == \"AnyOf\" and (.processes | length) == 2 and .processes[0] == {osType: \"Linux\", path: \"/opt/endpoint-agent/bin/agent\", hashes: [\"${linux_hash_1}\", \"${linux_hash_2}\"], signerFingerprints: []} and .processes[1] == {osType: \"macOS\", path: \"/Library/Endpoint Agent/agent\", hashes: [], signerFingerprints: [\"${macos_signer}\"]}"
	check_entity posturecheckoses e2e-os-out-of-order posture-checks \
		'.typeId == "OS" and .operatingSystems == [{type: "Linux", versions: [">=5.10.0", ">=6.0.0"]}, {type: "Windows", versions: [">=10.0.19045"]}, {type: "macOS", versions: [">=13.0.0", ">=14.0.0"]}]'

	step "Updating the posture checks updates Ziti"
	# The new values are again not in the form Ziti stores them in, so that the
	# check for updates without a reason below is run on such values. The
	# process check loses its fingerprint and keeps its hash. The Linux
	# process of the multi process check loses its hashes, and a Windows
	# process with a fingerprint takes the place of the macOS one.
	local domain_updated mac_updated process_updated multi_updated
	domain_updated='.domains == ["corp.example.com", "emea.example.com"] and (.roleAttributes | sort) == ["e2e", "managed-devices"]'
	mac_updated='.macAddresses == ["001a2b3c4d5e", "0a1b2c3d4e60"] and .tags == {"env": "e2e"}'
	process_updated=".process.osType == \"Windows\" and .process.path == \"C:\\\\Program Files\\\\Endpoint Agent\\\\agent64.exe\" and .process.hashes == [\"${agent_hash}\"] and (.process.signerFingerprint // \"\") == \"\" and (.roleAttributes | sort) == [\"e2e\", \"managed-devices\"]"
	multi_updated=".semantic == \"AllOf\" and (.processes | length) == 2 and .processes[0] == {osType: \"Linux\", path: \"/opt/endpoint-agent/bin/agent\", hashes: [], signerFingerprints: []} and .processes[1] == {osType: \"Windows\", path: \"C:\\\\Program Files\\\\Endpoint Agent\\\\agent.exe\", hashes: [], signerFingerprints: [\"${windows_signer}\"]}"
	patch posturecheckdomains corporate-domain '{"domains": ["emea.example.com", "corp.example.com"], "roleAttributes": ["managed-devices", "e2e"]}'
	eventually "domain posture check is updated in Ziti" entity_matches posture-checks "${domain}" "${domain_updated}"
	patch posturecheckmacs registered-devices '{"macAddresses": ["0A1B.2C3D.4E60", "00:1A:2B:3C:4D:5E"], "tags": {"env": "e2e"}}'
	eventually "MAC address posture check is updated in Ziti" entity_matches posture-checks "${mac}" "${mac_updated}"
	patch posturecheckprocesses windows-agent \
		'{"process": {"path": "C:\\Program Files\\Endpoint Agent\\agent64.exe", "signerFingerprint": null}, "roleAttributes": ["managed-devices", "e2e"]}'
	eventually "process posture check is updated in Ziti" entity_matches posture-checks "${process}" "${process_updated}"
	patch posturecheckmultiprocesses endpoint-agents \
		'{"semantic": "AllOf", "processes": [{"osType": "Windows", "path": "C:\\Program Files\\Endpoint Agent\\agent.exe", "signerFingerprints": ["6B:5A:DA:26:EE:CB:0E:04:30:9C:F0:42:5C:EB:07:44:F0:F4:54:B9"]}, {"osType": "Linux", "path": "/opt/endpoint-agent/bin/agent"}]}'
	eventually "multi process posture check is updated in Ziti" entity_matches posture-checks "${multi}" "${multi_updated}"

	step "Changes made to the posture checks in Ziti behind the provider's back are reverted"
	ziti_patch posture-checks "${domain}" '{"typeId": "DOMAIN", "domains": ["tampered.example.com"]}'
	entity_matches posture-checks "${domain}" '.domains == ["tampered.example.com"]' >/dev/null || fail "could not tamper with the domain posture check"
	ziti_patch posture-checks "${mac}" '{"typeId": "MAC", "macAddresses": ["ff:ff:ff:ff:ff:ff"]}'
	entity_matches posture-checks "${mac}" '.macAddresses == ["ffffffffffff"]' >/dev/null || fail "could not tamper with the MAC address posture check"
	ziti_patch posture-checks "${process}" '{"typeId": "PROCESS", "process": {"osType": "Linux", "path": "/tampered", "hashes": ["ff"], "signerFingerprint": "ff"}}'
	entity_matches posture-checks "${process}" '.process.path == "/tampered"' >/dev/null || fail "could not tamper with the process posture check"
	ziti_patch posture-checks "${multi}" '{"typeId": "PROCESS_MULTI", "semantic": "AnyOf", "processes": [{"osType": "Linux", "path": "/tampered", "hashes": ["ff"], "signerFingerprints": ["ff"]}]}'
	entity_matches posture-checks "${multi}" '.semantic == "AnyOf" and .processes[0].path == "/tampered"' >/dev/null || fail "could not tamper with the multi process posture check"
	eventually "domain posture check is restored" entity_matches posture-checks "${domain}" "${domain_updated}"
	eventually "MAC address posture check is restored" entity_matches posture-checks "${mac}" "${mac_updated}"
	eventually "process posture check is restored" entity_matches posture-checks "${process}" "${process_updated}"
	eventually "multi process posture check is restored" entity_matches posture-checks "${multi}" "${multi_updated}"

	step "A service policy refers to a posture check by name"
	local policy
	kubectl apply -f - <<EOF
apiVersion: ${GROUP}/v1alpha1
kind: ServicePolicy
metadata:
  name: e2e-posture-dial
  namespace: default
spec:
  providerConfigRef:
    kind: ProviderConfig
    name: default
  forProvider:
    name: e2e-posture-dial
    type: Dial
    semantic: AnyOf
    serviceRoles:
      - "#role:web"
    identityRoles:
      - "#web-clients"
    postureCheckRoles:
      - "@corporate-domain"
      - "#managed-devices"
EOF
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "servicepolicies.${GROUP}/e2e-posture-dial"
	policy="$(mr_id servicepolicies e2e-posture-dial)"
	check_entity servicepolicies e2e-posture-dial service-policies \
		"(.postureCheckRoles | sort) == ([\"@${domain}\", \"#managed-devices\"] | sort)"

	step "No posture check is updated without a spec change"
	posture_check_stamps() {
		stamps posture-checks "${domain}" "${mac}" "${process}" "${multi}" "${os}"
		stamps service-policies "${policy}"
	}
	expect_settled "posture checks stay untouched" posture_check_stamps

	step "Deleting the posture checks"
	kubectl delete --wait --timeout="${TIMEOUT}s" "servicepolicies.${GROUP}/e2e-posture-dial"
	expect_gone "service policy with posture checks is gone" service-policies e2e-posture-dial
	delete "${POSTURE_CHECK_EXAMPLES[@]}"
	kubectl delete --wait --timeout="${TIMEOUT}s" "posturecheckoses.${GROUP}/e2e-os-out-of-order"
	expect_gone "domain posture check is gone" posture-checks corporate-domain
	expect_gone "MAC address posture check is gone" posture-checks registered-devices
	expect_gone "process posture check is gone" posture-checks windows-agent
	expect_gone "multi process posture check is gone" posture-checks endpoint-agents
	expect_gone "OS posture check listed out of order is gone" posture-checks e2e-os-out-of-order
}

# test_lifecycle takes a resource of every kind through what the tests above
# show for some of them: a change made to its entity in Ziti is reverted, an
# entity that is deleted in Ziti is created anew under a new ID, what refers
# to it by name follows the new ID, a new enrollment token reaches the
# connection secret, nothing is updated once it is settled, and deleting the
# resource deletes the entity.
test_lifecycle() {
	local plurals=() names=() collections=() changes=() filters=()
	# row <plural> <name> <collection> <change to make in Ziti> <jq filter that
	# is true for the entity as declared>
	row() {
		plurals+=("$1")
		names+=("$2")
		collections+=("$3")
		changes+=("$4")
		filters+=("$5")
	}
	row services web-service services '{"roleAttributes": ["tampered"]}' \
		'(.roleAttributes | sort) == ["role:api", "role:web"] and (.configs | length) == 2'
	row confighostv1s web-service-host configs '{"data": {"address": "tampered.local", "port": 9999, "protocol": "udp"}}' \
		'.data.address == "web-service.local" and .data.port == 8080 and .data.protocol == "tcp"'
	row configinterceptv1s web-service-intercept configs \
		'{"data": {"addresses": ["tampered.example.com"], "protocols": ["tcp"], "portRanges": [{"low": 1, "high": 1}]}}' \
		'.data.addresses == ["web.example.com", "api.example.com"] and (.data.portRanges | length) == 2'
	row confighostv2s web-hosts configs '{"data": {"terminators": [{"address": "tampered.internal", "port": 1, "protocol": "tcp"}]}}' \
		'(.data.terminators | length) == 2 and .data.terminators[0].address == "web-1.internal"'
	row servicepolicies web-service-dial service-policies '{"semantic": "AllOf", "identityRoles": ["#tampered"]}' \
		'.type == "Dial" and .semantic == "AnyOf" and .identityRoles == ["#web-clients"]'
	row servicepolicies web-service-bind service-policies '{"serviceRoles": ["#tampered"]}' \
		'.type == "Bind" and .serviceRoles == ["#role:web"]'
	row edgerouterpolicies web-clients-public-routers edge-router-policies '{"edgeRouterRoles": ["#tampered"]}' \
		'.edgeRouterRoles == ["#public"] and (.identityRoles | sort) == ["#web-clients", "#web-servers"]'
	row serviceedgerouterpolicies web-services-public-routers service-edge-router-policies '{"edgeRouterRoles": ["#tampered"]}' \
		'.edgeRouterRoles == ["#public"] and .serviceRoles == ["#role:web"]'
	row edgerouters public-router edge-routers '{"roleAttributes": ["tampered"]}' '.roleAttributes == ["public"]'
	row identities web-client identities '{"roleAttributes": ["tampered"]}' '.roleAttributes == ["web-clients"]'
	row identities web-server identities '{"roleAttributes": ["tampered"]}' '.roleAttributes == ["web-servers"]'
	row identitycas sensor identities '{"roleAttributes": ["tampered"]}' '.roleAttributes == ["sensors"]'
	row identityupdbs operator identities '{"roleAttributes": ["tampered"]}' '.roleAttributes == ["operators"]'
	row identitynones sso-user identities '{"roleAttributes": ["tampered"], "externalId": "tampered@example.com"}' \
		'.roleAttributes == ["web-clients"] and .externalId == "sso-user@example.com"'
	# Ziti wants the type of a posture check along with a change.
	row posturecheckoses supported-os posture-checks '{"typeId": "OS", "operatingSystems": [{"type": "Linux", "versions": [">=1.0.0"]}]}' \
		'.typeId == "OS" and (.operatingSystems | length) == 2'
	row posturecheckmfas mfa posture-checks '{"typeId": "MFA", "timeoutSeconds": 60}' \
		'.typeId == "MFA" and .timeoutSeconds == 3600 and .promptOnWake == true'
	row posturecheckdomains corporate-domain posture-checks '{"typeId": "DOMAIN", "domains": ["tampered.example.com"]}' \
		'.typeId == "DOMAIN" and .domains == ["branch.example.com", "corp.example.com"]'
	row posturecheckmacs registered-devices posture-checks '{"typeId": "MAC", "macAddresses": ["ff:ff:ff:ff:ff:ff"]}' \
		'.typeId == "MAC" and .macAddresses == ["001a2b3c4d5e", "0a1b2c3d4e5f"]'
	row posturecheckprocesses windows-agent posture-checks \
		'{"typeId": "PROCESS", "process": {"osType": "Linux", "path": "/tampered", "hashes": ["ff"], "signerFingerprint": "ff"}}' \
		'.typeId == "PROCESS" and .process.osType == "Windows" and (.process.hashes | length) == 1'
	row posturecheckmultiprocesses endpoint-agents posture-checks \
		'{"typeId": "PROCESS_MULTI", "semantic": "AllOf", "processes": [{"osType": "Linux", "path": "/tampered", "hashes": ["ff"], "signerFingerprints": ["ff"]}]}' \
		'.typeId == "PROCESS_MULTI" and .semantic == "AnyOf" and (.processes | length) == 2'
	row authpolicies certificates-and-totp auth-policies '{"tags": {"tampered": "yes"}}' \
		'.primary.cert.allowed == true and .secondary.requireTotp == true and .tags == {"team": "platform"}'
	# The auth policy comes right before the signer it refers to: Ziti keeps a
	# signer that an auth policy refers to.
	row authpolicies corporate-sso-only auth-policies '{"primary": {"extJwt": {"allowedSigners": []}}}' \
		'.primary.extJwt.allowed == true and (.primary.extJwt.allowedSigners | length) == 1'
	row externaljwtsigners corporate-sso external-jwt-signers '{"audience": "tampered"}' '.audience == "ziti" and .enabled == true'
	row certificateauthorities device-ca cas '{"isOttCaEnrollmentEnabled": false}' \
		'.isOttCaEnrollmentEnabled == true and .isAuthEnabled == true'

	local examples=(
		"${CORE_EXAMPLES[@]}" "${EXTENDED_EXAMPLES[@]}" "${POSTURE_CHECK_EXAMPLES[@]}"
		examples/externaljwtsigner/externaljwtsigner.yaml examples/authpolicy/jwt.yaml
	)

	step "Creating a resource of every kind"
	openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=provider-ziti e2e lifecycle CA' \
		-keyout "${WORK}/lifecycle-ca.key" -out "${WORK}/lifecycle-ca.crt" 2>/dev/null || fail "cannot create a CA certificate"
	apply_ca "${WORK}/lifecycle-ca.crt"
	apply "${examples[@]}"
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "certificateauthorities.${GROUP}/device-ca"
	wait_ready "${examples[@]}"

	local i id ids=()
	for i in "${!plurals[@]}"; do
		id="$(mr_id "${plurals[$i]}" "${names[$i]}")"
		[ -n "${id}" ] || fail "${plurals[$i]}/${names[$i]} has no Ziti ID"
		entity_matches "${collections[$i]}" "${id}" "${filters[$i]}" >/dev/null ||
			fail "${plurals[$i]}/${names[$i]}: Ziti entity does not satisfy: ${filters[$i]}"
		ids+=("${id}")
	done
	ok "${#ids[@]} resources of $(printf '%s\n' "${plurals[@]}" | sort -u | wc -l | tr -d ' ') kinds are in Ziti as declared"

	step "A change made in Ziti to an entity of every kind is reverted"
	for i in "${!plurals[@]}"; do
		ziti_patch "${collections[$i]}" "${ids[$i]}" "${changes[$i]}" || fail "cannot change ${plurals[$i]}/${names[$i]} in Ziti"
		if entity_matches "${collections[$i]}" "${ids[$i]}" "${filters[$i]}" >/dev/null; then
			fail "could not tamper with ${plurals[$i]}/${names[$i]}"
		fi
	done
	for i in "${!plurals[@]}"; do
		eventually "${plurals[$i]}/${names[$i]}: the change is reverted" entity_matches "${collections[$i]}" "${ids[$i]}" "${filters[$i]}"
		expect "${plurals[$i]}/${names[$i]}: keeps its Ziti ID" "$(mr_id "${plurals[$i]}" "${names[$i]}")" "${ids[$i]}"
	done

	step "An entity of every kind that is deleted in Ziti is created anew"
	local client server sensor operator router attempt
	client="$(token_digest web-client-enrollment)" || fail "cannot read secret/web-client-enrollment"
	server="$(token_digest web-server-enrollment)" || fail "cannot read secret/web-server-enrollment"
	sensor="$(token_digest sensor-enrollment)" || fail "cannot read secret/sensor-enrollment"
	operator="$(token_digest operator-enrollment)" || fail "cannot read secret/operator-enrollment"
	router="$(token_digest public-router-enrollment)" || fail "cannot read secret/public-router-enrollment"
	for i in "${!plurals[@]}"; do
		attempt=0
		until ziti_delete "${collections[$i]}" "${ids[$i]}"; do
			# Only the signer may be kept: the provider was quick enough to
			# create the auth policy anew, which refers to it again.
			attempt=$((attempt + 1))
			[ "${plurals[$i]}" = externaljwtsigners ] && [ "${attempt}" -le 5 ] ||
				fail "cannot delete ${plurals[$i]}/${names[$i]} in Ziti"
			ziti_delete auth-policies "$(ziti_id auth-policies corporate-sso-only)" || true
		done
	done
	for i in "${!plurals[@]}"; do
		eventually "${plurals[$i]}/${names[$i]}: created anew under a new ID" \
			recreated "${plurals[$i]}" "${names[$i]}" "${collections[$i]}" "${ids[$i]}"
	done
	# What refers to an entity that is created later settles a poll later.
	for i in "${!plurals[@]}"; do
		ids[i]="$(mr_id "${plurals[$i]}" "${names[$i]}")"
		eventually "${plurals[$i]}/${names[$i]}: the new entity is as declared" \
			entity_matches "${collections[$i]}" "${ids[$i]}" "${filters[$i]}"
	done
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" "certificateauthorities.${GROUP}/device-ca"
	wait_ready "${examples[@]}"

	step "What refers to the entities by name follows their new IDs"
	local host intercept hosts service signer ca
	host="$(mr_id confighostv1s web-service-host)"
	intercept="$(mr_id configinterceptv1s web-service-intercept)"
	hosts="$(mr_id confighostv2s web-hosts)"
	service="$(mr_id services web-service)"
	signer="$(mr_id externaljwtsigners corporate-sso)"
	ca="$(mr_id certificateauthorities device-ca)"
	eventually "service refers to its new configs" entity_matches services "${service}" \
		"(.configs | sort) == ([\"${host}\", \"${intercept}\"] | sort)"
	eventually "service refers to its new host.v2 config" entity_matches services "$(mr_id services web-pair)" ".configs == [\"${hosts}\"]"
	eventually "dial policy refers to the new service" entity_matches service-policies "$(mr_id servicepolicies web-service-dial)" \
		".serviceRoles == [\"@${service}\"]"
	eventually "bind policy refers to the new identity" entity_matches service-policies "$(mr_id servicepolicies web-service-bind)" \
		".identityRoles == [\"@$(mr_id identities web-server)\"]"
	eventually "auth policy refers to the new signer" entity_matches auth-policies "$(mr_id authpolicies corporate-sso-only)" \
		".primary.extJwt.allowedSigners == [\"${signer}\"]"
	eventually "identity enrolls with the new certificate authority" entity_matches identities "$(mr_id identitycas sensor)" \
		".enrollment.ottca.caId == \"${ca}\""

	step "The entities created anew have new enrollment tokens"
	expect_renewed identities web-client identities '.enrollment.ott.jwt' '.enrollment.ott.expiresAt' "${client}"
	expect_renewed identities web-server identities '.enrollment.ott.jwt' '.enrollment.ott.expiresAt' "${server}"
	expect_renewed identitycas sensor identities '.enrollment.ottca.jwt' '.enrollment.ottca.expiresAt' "${sensor}"
	expect_renewed identityupdbs operator identities '.enrollment.updb.jwt' '.enrollment.updb.expiresAt' "${operator}"
	expect_renewed edgerouters public-router edge-routers '.enrollmentJwt' '.enrollmentExpiresAt' "${router}"
	entity_matches identities "$(mr_id identitynones sso-user)" '([.enrollment[]?] | length) == 0' >/dev/null ||
		fail "identitynones/sso-user was given an enrollment"
	ok "identitynones/sso-user: the new identity has no enrollment"

	step "Nothing is updated without a spec change"
	lifecycle_stamps() {
		local i
		for i in "${!plurals[@]}"; do
			stamps "${collections[$i]}" "${ids[$i]}"
		done
		stamps services "$(mr_id services web-pair)"
	}
	for i in "${!plurals[@]}"; do
		ids[i]="$(mr_id "${plurals[$i]}" "${names[$i]}")"
	done
	expect_settled "the entities created anew stay untouched" lifecycle_stamps

	step "Deleting the resources of every kind deletes the entities"
	# The certificate authority goes at the same time as the identity that
	# refers to it.
	kubectl delete --wait=false "certificateauthorities.${GROUP}/device-ca"
	delete "${examples[@]}"
	eventually "certificate authority is deleted" mr_gone certificateauthorities device-ca
	for i in "${!plurals[@]}"; do
		expect_gone "${plurals[$i]}/${names[$i]}: the entity is gone" "${collections[$i]}" "${names[$i]}"
	done
}

# test_scenarios applies the manifests of examples/scenarios as a user would,
# all at once, and checks that what they describe is in Ziti.
test_scenarios() {
	step "Applying the scenarios"
	apply "${SCENARIO_EXAMPLES[@]}"
	wait_ready "${SCENARIO_EXAMPLES[@]}"

	step "Publishing a service"
	local service host intercept
	service="$(mr_id services orders-db)"
	host="$(mr_id confighostv1s orders-db-host)"
	intercept="$(mr_id configinterceptv1s orders-db-intercept)"
	check_entity confighostv1s orders-db-host configs '.data == {"address": "postgres.internal", "port": 5432, "protocol": "tcp"}'
	check_entity configinterceptv1s orders-db-intercept configs \
		'.data == {"addresses": ["orders-db.ziti"], "protocols": ["tcp"], "portRanges": [{"low": 5432, "high": 5432}]}'
	check_entity services orders-db services \
		"(.configs | sort) == ([\"${host}\", \"${intercept}\"] | sort) and .roleAttributes == [\"databases\"] and .encryptionRequired == true"
	check_entity servicepolicies orders-db-dial service-policies \
		".type == \"Dial\" and .serviceRoles == [\"@${service}\"] and .identityRoles == [\"#orders-db-clients\"]"
	check_entity servicepolicies orders-db-bind service-policies \
		".type == \"Bind\" and .serviceRoles == [\"@${service}\"] and .identityRoles == [\"#orders-db-hosts\"]"
	check_entity identities orders-api identities '.roleAttributes == ["orders-db-clients"]'
	check_entity identities orders-db-tunnel identities '.roleAttributes == ["orders-db-hosts"]'
	check_token orders-api-enrollment identities "$(mr_id identities orders-api)" '.enrollment.ott.jwt'
	check_token orders-db-tunnel-enrollment identities "$(mr_id identities orders-db-tunnel)" '.enrollment.ott.jwt'
	# What Ziti makes of the policies: who has access to the service.
	eventually "the client has access to the service" listed "identities/$(mr_id identities orders-api)/services" orders-db
	eventually "the tunneler has access to the service" listed "identities/$(mr_id identities orders-db-tunnel)/services" orders-db

	step "Adding an edge router"
	local router
	router="$(mr_id edgerouters site-a-router)"
	check_entity edgerouters site-a-router edge-routers '.roleAttributes == ["site-a"] and .isTunnelerEnabled == true'
	check_token site-a-router-enrollment edge-routers "${router}" '.enrollmentJwt'
	check_entity edgerouterpolicies all-identities-site-a edge-router-policies '.edgeRouterRoles == ["#site-a"] and .identityRoles == ["#all"]'
	check_entity serviceedgerouterpolicies databases-site-a service-edge-router-policies \
		'.serviceRoles == ["#databases"] and .edgeRouterRoles == ["#site-a"]'
	eventually "the service is available on the router" listed "services/${service}/edge-routers" site-a-router
	eventually "the client may use the router" listed "identities/$(mr_id identities orders-api)/edge-routers" site-a-router

	step "Intercept and host configs"
	check_entity configinterceptv1s app-intercept configs \
		'.data == {"addresses": ["app.ziti"], "protocols": ["tcp"], "portRanges": [{"low": 443, "high": 443}]}'
	check_entity confighostv1s app-host configs '.data == {"address": "10.0.0.10", "port": 8443, "protocol": "tcp"}'
	check_entity configinterceptv1s office-lan-intercept configs \
		'.data.addresses == ["10.20.0.0/16"] and (.data.protocols | sort) == ["tcp", "udp"] and .data.portRanges == [{"low": 1, "high": 65535}]'
	check_entity confighostv1s office-lan-host configs \
		'.data.forwardAddress == true and .data.allowedAddresses == ["10.20.0.0/16"] and .data.forwardPort == true and .data.allowedPortRanges == [{"low": 1, "high": 65535}] and .data.forwardProtocol == true and (.data.allowedProtocols | sort) == ["tcp", "udp"] and (.data | has("address") or has("port") or has("protocol") | not)'
	check_entity configinterceptv1s apps-intercept configs '.data.addresses == ["*.apps.ziti"] and (.data.portRanges | length) == 2'
	check_entity confighostv1s apps-host configs \
		'.data.address == "ingress.internal" and .data.forwardPort == true and (.data.allowedPortRanges | length) == 2 and .data.protocol == "tcp" and (.data.portChecks | length) == 1 and (.data.portChecks[0].actions | length) == 2'
	check_entity confighostv2s api-hosts configs \
		'.configTypeId == "host.v2" and (.data.terminators | length) == 2 and .data.terminators[0].listenOptions.precedence == "required"'
	local name
	for name in app office-lan apps api; do
		check_entity services "${name}" services "(.configs | length) == $([ "${name}" = api ] && echo 1 || echo 2)"
	done

	step "Nothing is updated without a spec change"
	scenario_stamps() {
		local name
		for name in orders-db-host app-host office-lan-host apps-host; do
			stamps configs "$(mr_id confighostv1s "${name}")"
		done
		for name in orders-db-intercept app-intercept office-lan-intercept apps-intercept; do
			stamps configs "$(mr_id configinterceptv1s "${name}")"
		done
		stamps configs "$(mr_id confighostv2s api-hosts)"
		for name in orders-db app office-lan apps api; do
			stamps services "$(mr_id services "${name}")"
		done
		stamps service-policies "$(mr_id servicepolicies orders-db-dial)" "$(mr_id servicepolicies orders-db-bind)"
		stamps identities "$(mr_id identities orders-api)" "$(mr_id identities orders-db-tunnel)"
		stamps edge-routers "${router}"
		stamps edge-router-policies "$(mr_id edgerouterpolicies all-identities-site-a)"
		stamps service-edge-router-policies "$(mr_id serviceedgerouterpolicies databases-site-a)"
	}
	expect_settled "the entities of the scenarios stay untouched" scenario_stamps

	step "Deleting the scenarios"
	delete "${SCENARIO_EXAMPLES[@]}"
	for name in orders-db app office-lan apps api; do
		expect_gone "service ${name} is gone" services "${name}"
	done
	for name in orders-db-host orders-db-intercept app-host app-intercept office-lan-host office-lan-intercept apps-host apps-intercept api-hosts; do
		expect_gone "config ${name} is gone" configs "${name}"
	done
	expect_gone "dial policy is gone" service-policies orders-db-dial
	expect_gone "bind policy is gone" service-policies orders-db-bind
	expect_gone "client identity is gone" identities orders-api
	expect_gone "tunneler identity is gone" identities orders-db-tunnel
	expect_gone "edge router is gone" edge-routers site-a-router
	expect_gone "edge router policy is gone" edge-router-policies all-identities-site-a
	expect_gone "service edge router policy is gone" service-edge-router-policies databases-site-a
}

# example_files prints every manifest of examples/ that is a resource of the
# provider. examples/provider connects the provider to a controller, which
# configure does for the test, and examples/composition needs Crossplane and
# has a stage of its own.
example_files() {
	(cd "${ROOT}" && find examples -name '*.yaml' ! -path 'examples/provider/*' ! -path 'examples/composition/*' | LC_ALL=C sort)
}

# test_examples applies every manifest of examples/ as it is in the
# repository, all at once, and checks that each resource becomes ready: Ziti
# refuses values the CRDs accept, and only a controller tells. The other
# stages apply most of the manifests too, but each names its own; this one
# takes whatever is in the directory, so that no example goes untested.
test_examples() {
	step "Applying every example"
	local files=() f
	while IFS= read -r f; do files+=("${f}"); done < <(example_files)
	[ "${#files[@]}" -gt 0 ] || fail "no manifests in examples/"
	for f in "${ROOT}"/examples/composition/*.yaml; do
		case " ${COMPOSITION_EXAMPLES[*]} ${COMPOSITE_EXAMPLE} " in
		*" examples/composition/${f##*/} "*) ;;
		*) fail "examples/composition/${f##*/} is not applied by the composition stage" ;;
		esac
	done
	apply "${files[@]}"
	for f in "${files[@]}"; do
		kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" -f "${ROOT}/${f}" >/dev/null ||
			fail "${f}: not every resource became ready"
		ok "${f}: ready"
	done

	step "Ziti stores what only it validates"
	check_entity services service-sticky services '.terminatorStrategy == "sticky"'
	check_entity services advanced-service services '.terminatorStrategy == "weighted" and (.configs | length) == 2'
	check_entity confighostv1s advanced-host configs \
		'.data.forwardAddress == true and .data.forwardAddressTranslations == [{"from": "10.1.0.0", "to": "192.168.0.0", "prefixLength": 16}]'
	check_entity services minimal-service services '(.configs | length) == 1'

	step "Deleting the examples"
	delete "${files[@]}"
	for name in service-sticky advanced-service minimal-service; do
		expect_gone "service ${name} is gone" services "${name}"
	done
	for name in advanced-host advanced-intercept minimal-host; do
		expect_gone "config ${name} is gone" configs "${name}"
	done
}

# test_composition has Crossplane compose the resources of the provider from
# the composite resource of examples/composition.
test_composition() {
	step "Installing the function, the definition and the Composition"
	apply "${COMPOSITION_EXAMPLES[@]}"
	# The active revision of the function says whether its pod answers.
	function_runs() {
		kubectl get functionrevisions.pkg.crossplane.io -l pkg.crossplane.io/package=function-patch-and-transform -o json |
			jq -e '[.items[] | select(.spec.desiredState == "Active") | .status.conditions[]? | select(.type == "RuntimeHealthy") | .status] == ["True"]'
	}
	eventually "the function of the Composition runs" function_runs
	kubectl wait --for=condition=Established --timeout="${TIMEOUT}s" "compositeresourcedefinitions.apiextensions.crossplane.io/${COMPOSITE}"
	eventually "the kind of the composite resource is served" kubectl get "${COMPOSITE}" -n default

	step "Creating a published service"
	apply "${COMPOSITE_EXAMPLE}"
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" -n default "${COMPOSITE}/wiki"
	local service host intercept
	service="$(ziti_id services wiki)"
	host="$(ziti_id configs wiki-host)"
	intercept="$(ziti_id configs wiki-intercept)"
	[ -n "${service}" ] && [ -n "${host}" ] && [ -n "${intercept}" ] || fail "the service or one of its configs is not in Ziti"
	entity_is() {
		if named_matches "$2" "$3" "$4" >/dev/null; then
			ok "$1"
		else
			fail "$1: no entity $3 in $2 satisfies: $4"
		fi
	}
	entity_is "host config is as composed" configs wiki-host '.data == {"address": "wiki.internal", "port": 8443, "protocol": "tcp"}'
	entity_is "intercept config is as composed" configs wiki-intercept \
		'.data == {"addresses": ["wiki.ziti"], "protocols": ["tcp"], "portRanges": [{"low": 443, "high": 443}]}'
	entity_is "service has both configs" services wiki "(.configs | sort) == ([\"${host}\", \"${intercept}\"] | sort) and .encryptionRequired == true"
	entity_is "dial policy is as composed" service-policies wiki-dial \
		".type == \"Dial\" and .serviceRoles == [\"@${service}\"] and .identityRoles == [\"#wiki-clients\"]"
	entity_is "bind policy is as composed" service-policies wiki-bind \
		".type == \"Bind\" and .serviceRoles == [\"@${service}\"] and .identityRoles == [\"#wiki-hosts\"]"
	entity_is "service edge router policy is as composed" service-edge-router-policies wiki-routers \
		".serviceRoles == [\"@${service}\"] and .edgeRouterRoles == [\"#all\"]"
	reports_service() {
		[ "$(kubectl get "${COMPOSITE}" wiki -n default -o jsonpath='{.status.serviceId}')" = "$1" ]
	}
	eventually "the published service reports the ID of its service" reports_service "${service}"

	step "Changing the published service changes Ziti"
	kubectl patch "${COMPOSITE}" wiki -n default --type merge \
		-p '{"spec": {"upstream": {"port": 9443}, "intercept": {"address": "wiki.example.ziti"}, "edgeRouterRole": "#public"}}' >/dev/null
	eventually "host config follows the published service" entity_matches configs "${host}" '.data.port == 9443'
	eventually "intercept config follows the published service" entity_matches configs "${intercept}" '.data.addresses == ["wiki.example.ziti"]'
	eventually "service edge router policy follows the published service" named_matches service-edge-router-policies wiki-routers \
		'.edgeRouterRoles == ["#public"]'

	step "A composed entity that is deleted in Ziti is created anew"
	ziti_delete services "${service}" || fail "cannot delete the service in Ziti"
	composed_anew() {
		local id
		id="$(ziti_id services wiki)" || return 1
		[ -n "${id}" ] && [ "${id}" != "${service}" ]
	}
	eventually "service is created anew" composed_anew
	service="$(ziti_id services wiki)"
	eventually "the published service reports the new ID" reports_service "${service}"
	eventually "dial policy refers to the new service" named_matches service-policies wiki-dial ".serviceRoles == [\"@${service}\"]"
	kubectl wait --for=condition=Ready --timeout="${TIMEOUT}s" -n default "${COMPOSITE}/wiki"

	step "Deleting the published service deletes what it is composed of"
	kubectl delete --wait --timeout="${TIMEOUT}s" -n default "${COMPOSITE}/wiki"
	eventually "service is gone" ziti_gone services wiki
	eventually "host config is gone" ziti_gone configs wiki-host
	eventually "intercept config is gone" ziti_gone configs wiki-intercept
	eventually "dial policy is gone" ziti_gone service-policies wiki-dial
	eventually "bind policy is gone" ziti_gone service-policies wiki-bind
	eventually "service edge router policy is gone" ziti_gone service-edge-router-policies wiki-routers
}

run_tests() {
	ziti_login
	configure
	if [ -n "${E2E_ONLY:-}" ]; then
		local stage
		for stage in ${E2E_ONLY}; do
			"test_${stage}"
		done
		step "The end-to-end checks of ${E2E_ONLY} passed"
		return
	fi
	test_core
	test_drift
	if [ "${E2E_SKIP_EXTENDED:-false}" != "true" ]; then
		test_extended
		test_posture_checks
		test_authentication
		test_renewal
		test_lifecycle
		test_scenarios
		test_examples
		if has_crossplane; then
			test_composition
		else
			step "Skipping the Composition: Crossplane is not installed"
		fi
	fi
	step "All end-to-end checks passed"
}

logs() {
	step "Managed resources"
	kubectl get managed -A -o wide 2>&1 || true
	step "Resources that are not ready"
	kubectl get managed -A -o json 2>/dev/null |
		jq -r '.items[] | select(([.status.conditions[]? | select(.type == "Ready" and .status == "True")] | length) == 0)
			| "\(.kind)/\(.metadata.name): " + ([.status.conditions[]? | "\(.type)=\(.status) \(.reason) \(.message // "")"] | join("; "))' || true
	step "Provider log (last 200 lines)"
	tail -n 200 "${WORK}/provider.log" 2>/dev/null | redact || true
	step "Ziti controller log (last 50 lines)"
	# Only "up" starts the controller with docker.
	if command -v docker >/dev/null 2>&1; then
		docker compose -p "${COMPOSE_PROJECT}" -f "${COMPOSE_FILE}" logs --tail 50 ziti-controller 2>&1 | redact || true
	fi
}

down() {
	if [ -f "${WORK}/provider.pid" ]; then
		kill "$(cat "${WORK}/provider.pid")" 2>/dev/null || true
		rm -f "${WORK}/provider.pid"
	fi
	kind delete cluster --name "${CLUSTER}" || true
	docker compose -p "${COMPOSE_PROJECT}" -f "${COMPOSE_FILE}" down -v || true
}

case "${1:-}" in
up) up ;;
test) run_tests ;;
logs) logs ;;
down) down ;;
crossplane) crossplane ;;
all)
	trap 'logs; down' EXIT
	up
	run_tests
	;;
*)
	sed -n '2,32p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
