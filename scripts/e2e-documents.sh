#!/usr/bin/env bash
# Run in a dedicated checkout: this restarts its registered API/Web environment.
set -euo pipefail
cd "$(dirname "$0")/.."

# A healthy existing API may have started with the flag off. Restart it rather
# than assuming an environment variable in the Playwright process changes it.
if make status >/dev/null 2>&1; then
  make down ARGS='--components api,web'
fi
trap 'make down ARGS="--components api,web"' EXIT
FF_CORTEX_DOCS=true make up C=api,web ARGS=--ephemeral
make env-exec ARGS='-- pnpm exec playwright test e2e/document-table.spec.ts --reporter=line'
