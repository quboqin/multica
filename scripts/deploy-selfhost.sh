#!/usr/bin/env bash
set -euo pipefail
umask 077

# Upgrade an existing Compose installation. Bootstrap, .env provisioning and
# restore decisions belong to the operator; this script never creates secrets.
tag="${1:?Usage: deploy-selfhost.sh TAG COMMIT DEPLOY_PATH}"
commit="${2:?Missing commit}"
deploy_dir="${3:?Missing deployment path}"
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || exit 1
[[ "$commit" =~ ^[a-f0-9]{40}$ ]] || exit 1
[[ "$deploy_dir" =~ ^/[a-zA-Z0-9_./-]+$ && "$deploy_dir" != / ]] || exit 1
[[ -d "$deploy_dir" && -f "$deploy_dir/.env" ]] || {
  echo 'Provision an existing deployment with a production .env first.' >&2
  exit 1
}
source_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# CI transfers this script and the tagged Compose file into one directory.
compose_source="$source_dir/docker-compose.selfhost.yml"
[[ -f "$compose_source" ]] || compose_source="$source_dir/../docker-compose.selfhost.yml"
[[ -f "$compose_source" ]] || exit 1
mkdir "$deploy_dir/.deploy-lock" || {
  echo 'Another deployment holds .deploy-lock; inspect it before retrying.' >&2
  exit 1
}
trap 'rmdir "$deploy_dir/.deploy-lock"' EXIT

release_dir="$deploy_dir/.releases/$tag-$commit"
mkdir -p "$release_dir" "$deploy_dir/backups"
backup_dir="$(mktemp -d "$deploy_dir/backups/$(date -u +%Y%m%dT%H%M%SZ)-$tag.XXXXXXXX")"
cp "$compose_source" "$release_dir/compose.yml"
cp "$deploy_dir/.env" "$backup_dir/env"
if [[ -f "$deploy_dir/.current-release" ]]; then
  cp "$deploy_dir/.current-release" "$backup_dir/previous-release"
fi

# Explicit overrides prevent an inherited shell environment or old .env from
# deploying upstream images. Keep application secrets in the existing .env.
export MULTICA_BACKEND_IMAGE=ghcr.io/quboqin/multica-backend
export MULTICA_WEB_IMAGE=ghcr.io/quboqin/multica-web
export MULTICA_IMAGE_TAG="$tag"
dc() {
  docker compose --project-directory "$deploy_dir" --env-file "$deploy_dir/.env" -f "$release_dir/compose.yml" "$@"
}
dc config --quiet
[[ -n "$(dc ps -q postgres)" && -n "$(dc ps -q backend)" ]] || {
  echo 'Automatic upgrades require running postgres and backend containers.' >&2
  exit 1
}
dc pull backend frontend

# Quiesce writers before taking a matching database/uploads snapshot. Failure
# after this point deliberately leaves the deployment for operator recovery.
dc stop frontend backend
dc exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$backup_dir/database.dump"
dc run --rm --no-deps --entrypoint tar backend -C /app/data/uploads -czf - . > "$backup_dir/uploads.tar.gz"
[[ -s "$backup_dir/database.dump" && -s "$backup_dir/uploads.tar.gz" ]] || exit 1
dc up -d backend frontend

healthy=false
for ((attempt = 0; attempt < 60; attempt++)); do
  if dc exec -T frontend node -e '
    const options = { signal: AbortSignal.timeout(5000) };
    Promise.all([fetch("http://backend:8080/health", options), fetch("http://127.0.0.1:3000/", options)])
      .then(async ([api, web]) => {
        if (!api.ok || !web.ok || (await api.json()).commit !== process.argv[1]) process.exit(1);
      }).catch(() => process.exit(1));
  ' "$commit"; then
    healthy=true
    break
  fi
  sleep 5
done
if [[ "$healthy" != true ]]; then
  echo "Deployment failed health/version checks. Backup: $backup_dir. No automatic database rollback was attempted." >&2
  exit 1
fi
# Persist the successful image selection so an operator's later Compose command
# cannot silently return to latest or an upstream image. Preserve secret bytes.
awk -v tag="$tag" '
  !/^(MULTICA_BACKEND_IMAGE|MULTICA_WEB_IMAGE|MULTICA_IMAGE_TAG)=/ { print }
  END {
    print "MULTICA_BACKEND_IMAGE=ghcr.io/quboqin/multica-backend"
    print "MULTICA_WEB_IMAGE=ghcr.io/quboqin/multica-web"
    print "MULTICA_IMAGE_TAG=" tag
  }
' "$deploy_dir/.env" > "$deploy_dir/.env.next"
mv "$deploy_dir/.env.next" "$deploy_dir/.env"
cp "$release_dir/compose.yml" "$deploy_dir/docker-compose.selfhost.yml"
printf '%s %s\n' "$tag" "$commit" > "$deploy_dir/.current-release"
echo "Deployed $tag ($commit). Backup: $backup_dir"
