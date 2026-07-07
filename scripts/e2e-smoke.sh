#!/usr/bin/env bash
set -euo pipefail

api_base="${BALLAST_API_BASE:-http://localhost:8080}"
admin_token="${BALLAST_ADMIN_TOKEN:-ballast-dev-admin-token}"
web_origin="${BALLAST_WEB_ORIGIN:-http://localhost:3000}"
cookie_jar="$(mktemp)"
response_file="$(mktemp)"
headers_file="$(mktemp)"

cleanup() {
  rm -f "${cookie_jar}" "${response_file}" "${headers_file}"
}
trap cleanup EXIT

status_code() {
  curl -sS -o "${response_file}" -w '%{http_code}' "$@"
}

json_field() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

wait_for_status() {
  local session_id="$1"
  local wanted="$2"
  local deadline=$((SECONDS + 30))
  while (( SECONDS < deadline )); do
    local body
    body="$(curl -fsS -b "${cookie_jar}" "${api_base}/api/sessions/${session_id}")"
    if [[ "$(printf '%s' "${body}" | json_field status)" == "${wanted}" ]]; then
      return 0
    fi
    sleep 0.25
  done
  echo "session ${session_id} did not reach ${wanted}" >&2
  return 1
}

curl -fsS "${api_base}/healthz" >/dev/null

code="$(status_code "${api_base}/api/sessions")"
[[ "${code}" == "401" ]]

code="$(status_code -H 'Authorization: Bearer wrong' -X POST \
  -H 'Content-Type: application/json' -d '{}' \
  "${api_base}/api/internal/harness/event")"
[[ "${code}" == "401" ]]

code="$(curl -sS -D "${headers_file}" -o /dev/null -w '%{http_code}' \
  -X OPTIONS -H "Origin: ${web_origin}" \
  -H 'Access-Control-Request-Method: POST' \
  "${api_base}/api/auth/login")"
[[ "${code}" == "204" ]]
grep -qi "^Access-Control-Allow-Origin: ${web_origin}" "${headers_file}"

curl -fsS -c "${cookie_jar}" \
  -H "Origin: ${web_origin}" \
  -H 'Content-Type: application/json' \
  -d "{\"token\":\"${admin_token}\"}" \
  "${api_base}/api/auth/login" >/dev/null

session_json="$(curl -fsS -b "${cookie_jar}" \
  -H 'Content-Type: application/json' \
  -d '{"title":"e2e CrashLoopBackOff triage"}' \
  "${api_base}/api/sessions")"
session_id="$(printf '%s' "${session_json}" | json_field session_id)"

wait_for_status "${session_id}" "SUSPENDED"

sandbox_security="$(docker inspect -f \
  '{{.Config.User}}|{{.HostConfig.ReadonlyRootfs}}|{{json .HostConfig.CapDrop}}|{{json .HostConfig.SecurityOpt}}' \
  "ballast-sbx-${session_id}")"
[[ "${sandbox_security}" == 'ballast|true|["ALL"]|["no-new-privileges"]' ]]

audit_before="$(docker compose exec -T postgres psql -U ballast -d ballast -Atc \
  "SELECT policy_decision || ':' || count(*) FROM ballast_audit_logs WHERE session_id='${session_id}' GROUP BY policy_decision ORDER BY policy_decision")"
grep -qx 'APPROVE:2' <<<"${audit_before}"
grep -qx 'SUSPEND:1' <<<"${audit_before}"

curl -fsS -b "${cookie_jar}" -X POST \
  "${api_base}/api/sessions/${session_id}/approve" >/dev/null
wait_for_status "${session_id}" "SUCCESS"

deadline=$((SECONDS + 15))
while docker ps -a --format '{{.Names}}' | grep -qx "ballast-sbx-${session_id}"; do
  if (( SECONDS >= deadline )); then
    echo "sandbox ballast-sbx-${session_id} was not removed" >&2
    exit 1
  fi
  sleep 0.25
done

audit_after="$(docker compose exec -T postgres psql -U ballast -d ballast -Atc \
  "SELECT policy_decision || ':' || count(*) FROM ballast_audit_logs WHERE session_id='${session_id}' GROUP BY policy_decision ORDER BY policy_decision")"
grep -qx 'APPROVE:3' <<<"${audit_after}"
grep -qx 'SUSPEND:1' <<<"${audit_after}"

code="$(status_code -b "${cookie_jar}" -X POST \
  "${api_base}/api/sessions/${session_id}/approve")"
[[ "${code}" == "409" ]]

echo "Ballast e2e smoke passed for ${session_id}"
