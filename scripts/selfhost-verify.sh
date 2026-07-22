#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${1:-.env}"

fail() {
  echo "Deployment verification failed: $*" >&2
  exit 1
}

[[ -f "$ENV_FILE" ]] || fail "missing env file $ENV_FILE"
command -v curl >/dev/null 2>&1 || fail "curl is required"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

BACKEND_PORT="${BACKEND_PORT:-${API_PORT:-${SERVER_PORT:-${PORT:-8080}}}}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"
BACKEND_URL="http://127.0.0.1:${BACKEND_PORT}"
FRONTEND_URL="http://127.0.0.1:${FRONTEND_PORT}"

health="$(curl --fail --silent --show-error --max-time 10 "${BACKEND_URL}/healthz")" \
  || fail "backend readiness endpoint is unavailable at ${BACKEND_URL}/healthz"

grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' <<<"$health" \
  || fail "backend did not report an ok status: $health"
grep -Eq '"migrations"[[:space:]]*:[[:space:]]*"ok"' <<<"$health" \
  || fail "database migrations are not healthy: $health"

curl --fail --silent --show-error --max-time 15 "${FRONTEND_URL}/login" >/dev/null \
  || fail "frontend login page is unavailable at ${FRONTEND_URL}/login"
curl --fail --silent --show-error --max-time 10 "${FRONTEND_URL}/api/config" >/dev/null \
  || fail "frontend cannot proxy the API at ${FRONTEND_URL}/api/config"

echo "Deployment verification passed."
echo "  Frontend: ${FRONTEND_URL}"
echo "  Backend:  ${BACKEND_URL}"
