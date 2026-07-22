#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE=".env"
MODE="pull"

usage() {
  cat <<'EOF'
Usage: scripts/selfhost-deploy.sh [--env-file PATH] [--build]

Runs preflight checks, updates the Compose stack, and verifies the frontend,
backend, and automatic database migrations. Use --build to build images from
the current checkout; the default pulls the configured release images.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      ENV_FILE="$2"
      shift 2
      ;;
    --build)
      MODE="build"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
done

"$ROOT_DIR/scripts/selfhost-preflight.sh" "$ENV_FILE"

compose=(docker compose --env-file "$ENV_FILE" -f docker-compose.selfhost.yml)
if [[ "$MODE" == "build" ]]; then
  compose+=(-f docker-compose.selfhost.build.yml)
  echo "==> Building and starting Multica..."
  "${compose[@]}" up -d --build
else
  echo "==> Pulling configured Multica images..."
  "${compose[@]}" pull
  echo "==> Starting Multica..."
  "${compose[@]}" up -d
fi

echo "==> Waiting for the backend entrypoint to finish automatic migrations..."
for _ in $(seq 1 30); do
  if "$ROOT_DIR/scripts/selfhost-verify.sh" "$ENV_FILE" >/dev/null 2>&1; then
    "$ROOT_DIR/scripts/selfhost-verify.sh" "$ENV_FILE"
    exit 0
  fi
  sleep 2
done

echo "Deployment verification did not complete. Inspect service logs with:" >&2
printf '  docker compose --env-file %q -f docker-compose.selfhost.yml logs\n' "$ENV_FILE" >&2
exit 1
