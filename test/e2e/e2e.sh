#!/usr/bin/env bash
# End-to-end test of provider-ziti.
#
# The provider runs out-of-cluster against a kind cluster and manages an
# OpenZiti controller started from ziti-docker-compose.yml. The test applies
# the manifests in examples/, checks through the Ziti Edge Management API that
# the entities exist as declared, changes and tampers with them, and checks
# that deleting the managed resources deletes the entities.
#
# Usage:
#   test/e2e/e2e.sh up      # start OpenZiti, a kind cluster and the provider
#   test/e2e/e2e.sh test    # run the test against what "up" started
#   test/e2e/e2e.sh logs    # print diagnostics
#   test/e2e/e2e.sh down    # stop everything "up" started
#   test/e2e/e2e.sh all     # up, test, down
#
# "test" alone runs the checks against a provider that is already running:
# put the kubeconfig of its cluster in $E2E_WORK_DIR/kubeconfig and set
# ZITI_URL, ZITI_USER and ZITI_PWD for the test controller.
#
# Requires docker (with compose), kind, kubectl, go, curl, jq and openssl.

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

# mr_gone <plural> <name> succeeds if the managed resource no longer exists.
mr_gone() {
	local found
	found="$(kubectl get "$1.${GROUP}" "$2" --ignore-not-found -o name)" || return 1
	[ -z "${found}" ]
}

# ------------------------------------------------------------------------------
# Commands

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

test_core() {
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

	local ca signer policy sensor fingerprint token refusal
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
	# A PATCH of a certificate authority also takes its external ID claim away.
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

run_tests() {
	ziti_login
	test_core
	if [ "${E2E_SKIP_EXTENDED:-false}" != "true" ]; then
		test_extended
		test_authentication
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
	docker compose -p "${COMPOSE_PROJECT}" -f "${COMPOSE_FILE}" logs --tail 50 ziti-controller 2>&1 | redact || true
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
all)
	trap 'logs; down' EXIT
	up
	run_tests
	;;
*)
	sed -n '2,21p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
