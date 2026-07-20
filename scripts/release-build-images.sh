#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TAG="${1:-}"
REPOSITORY_PREFIX="${MULTICA_IMAGE_REPOSITORY:-multica-ai}"

if [[ -z "$TAG" ]]; then
  echo "Usage: scripts/release-build-images.sh <image-tag>" >&2
  exit 2
fi

backend_image="${REPOSITORY_PREFIX}/multica-backend:${TAG}"
web_image="${REPOSITORY_PREFIX}/multica-web:${TAG}"
worker_image="${REPOSITORY_PREFIX}/multica-crawler-worker:${TAG}"

docker build --pull -t "$backend_image" -f Dockerfile .
docker build --pull -t "$web_image" -f Dockerfile.web .
docker build --pull -t "$worker_image" -f services/crawler-worker/Dockerfile .

docker push "$backend_image"
docker push "$web_image"
docker push "$worker_image"

printf 'Published release images with tag %s:\n  %s\n  %s\n  %s\n' \
  "$TAG" "$backend_image" "$web_image" "$worker_image"
