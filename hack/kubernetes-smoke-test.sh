#!/usr/bin/env sh
set -eu

KUBECTL=${KUBECTL:-kubectl}
RECORDER_NAMESPACE=${KBB_NAMESPACE:-kube-blackbox}
RECORDER_DEPLOYMENT=${KBB_DEPLOYMENT:-kube-blackbox}
RUN_ID=$(date +%s)
TEST_NAMESPACE=${KBB_TEST_NAMESPACE:-kbb-smoke-${RUN_ID}}
CONFIGMAP_NAME=kbb-probe
SECRET_MARKER=kbb-must-not-persist-${RUN_ID}
CREATED_TEST_NAMESPACE=false

cleanup() {
	if [ "${CREATED_TEST_NAMESPACE}" = true ]; then
		${KUBECTL} delete namespace "${TEST_NAMESPACE}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
	fi
}
trap cleanup EXIT INT TERM

fail() {
	printf 'FAIL: %s\n' "$1" >&2
	exit 1
}

records() {
	${KUBECTL} -n "${RECORDER_NAMESPACE}" exec "deployment/${RECORDER_DEPLOYMENT}" -- \
		/kube-blackbox timeline \
		--data-dir=/var/lib/kube-blackbox \
		--namespace="${TEST_NAMESPACE}" \
		--kind=ConfigMap \
		--name="${CONFIGMAP_NAME}" \
		--output=records \
		--limit=100
}

wait_for_action() {
	action=$1
	attempt=0
	while [ "${attempt}" -lt 30 ]; do
		output=$(records 2>/dev/null || true)
		if printf '%s\n' "${output}" | grep -q "\"action\": \"${action}\""; then
			printf '%s\n' "${output}"
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 1
	done
	return 1
}

printf 'Waiting for recorder rollout...\n'
${KUBECTL} -n "${RECORDER_NAMESPACE}" rollout status "deployment/${RECORDER_DEPLOYMENT}" --timeout=120s

printf 'Checking read-only RBAC...\n'
SUBJECT="system:serviceaccount:${RECORDER_NAMESPACE}:kube-blackbox"
${KUBECTL} auth can-i list pods --all-namespaces --as="${SUBJECT}" | grep -qx yes || fail 'ServiceAccount cannot list Pods'
${KUBECTL} auth can-i watch networkpolicies.networking.k8s.io --all-namespaces --as="${SUBJECT}" | grep -qx yes || fail 'ServiceAccount cannot watch NetworkPolicies'
if ${KUBECTL} auth can-i get secrets --all-namespaces --as="${SUBJECT}" | grep -qx yes; then
	fail 'ServiceAccount unexpectedly has access to Secrets'
fi
if ${KUBECTL} auth can-i create pods --all-namespaces --as="${SUBJECT}" | grep -qx yes; then
	fail 'ServiceAccount unexpectedly has write access to Pods'
fi

printf 'Waiting for all informer snapshots to finish...\n'
attempt=0
while [ "${attempt}" -lt 60 ]; do
	if ${KUBECTL} -n "${RECORDER_NAMESPACE}" logs "deployment/${RECORDER_DEPLOYMENT}" | grep -q 'initial Kubernetes snapshots persisted'; then
		break
	fi
	attempt=$((attempt + 1))
	sleep 1
done
[ "${attempt}" -lt 60 ] || fail 'recorder did not finish its initial Kubernetes snapshots'

printf 'Creating, updating, and deleting a probe ConfigMap...\n'
if ${KUBECTL} get namespace "${TEST_NAMESPACE}" >/dev/null 2>&1; then
	fail "test namespace ${TEST_NAMESPACE} already exists; choose another KBB_TEST_NAMESPACE"
fi
${KUBECTL} create namespace "${TEST_NAMESPACE}" >/dev/null
CREATED_TEST_NAMESPACE=true
${KUBECTL} -n "${TEST_NAMESPACE}" create configmap "${CONFIGMAP_NAME}" --from-literal=password="${SECRET_MARKER}" >/dev/null
wait_for_action ADD >/dev/null || fail 'ADD record was not persisted within 30 seconds'

${KUBECTL} -n "${TEST_NAMESPACE}" label configmap "${CONFIGMAP_NAME}" smoke-phase=updated >/dev/null
wait_for_action UPDATE >/dev/null || fail 'UPDATE record was not persisted within 30 seconds'

${KUBECTL} -n "${TEST_NAMESPACE}" delete configmap "${CONFIGMAP_NAME}" --wait=true >/dev/null
FINAL_RECORDS=$(wait_for_action DELETE) || fail 'DELETE record was not persisted within 30 seconds'

printf 'Checking retained evidence and redaction...\n'
for action in ADD UPDATE DELETE; do
	printf '%s\n' "${FINAL_RECORDS}" | grep -q "\"action\": \"${action}\"" || fail "${action} record is missing from retained evidence"
done
if printf '%s\n' "${FINAL_RECORDS}" | grep -Fq "${SECRET_MARKER}"; then
	fail 'ConfigMap payload was persisted'
fi
if printf '%s\n' "${FINAL_RECORDS}" | grep -Eq '"(data|binaryData)":'; then
	fail 'ConfigMap data field was persisted'
fi

printf 'PASS: RBAC, watch delivery, persistence, deletion history, and ConfigMap redaction are working.\n'
