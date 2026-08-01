#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${1:-.env}"

fail() {
  echo "Deployment preflight failed: $*" >&2
  exit 1
}

warn() {
  echo "Deployment preflight warning: $*" >&2
}

[[ -f "$ENV_FILE" ]] || fail "missing env file $ENV_FILE"
command -v docker >/dev/null 2>&1 || fail "docker is not installed"
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required"
docker info >/dev/null 2>&1 || fail "the Docker daemon is not reachable"

set -a
# Deployment env files are operator-owned configuration, matching the existing
# self-host scripts' loading behavior.
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

if [[ -z "${JWT_SECRET:-}" || "${JWT_SECRET}" == "change-me-in-production" ]]; then
  fail "set a non-default JWT_SECRET in $ENV_FILE"
fi

if [[ -z "${BROKER_STATE_KEY:-}" ]]; then
  fail "set BROKER_STATE_KEY in $ENV_FILE"
fi

if [[ -n "${MULTICA_PUBLIC_URL:-}" && ! "${MULTICA_PUBLIC_URL}" =~ ^https:// ]]; then
  fail "MULTICA_PUBLIC_URL must use https in a public deployment"
fi

if [[ -n "${MULTICA_DEV_VERIFICATION_CODE:-}" ]]; then
  warn "MULTICA_DEV_VERIFICATION_CODE is set; Compose production mode ignores it, but remove it before exposing the service"
fi

if [[ -z "${RESEND_API_KEY:-}" && -z "${SMTP_HOST:-}" ]]; then
  warn "no email provider is configured; login codes will be available only in backend logs"
fi

docker compose --env-file "$ENV_FILE" -f docker-compose.selfhost.yml config --quiet

echo "Self-host deployment preflight passed."
