#!/usr/bin/env bash
set -euo pipefail

# Run from the tag checkout, never from a branch substituted by the publisher.
tag="${1:?Usage: check-release-origin.sh vX.Y.Z}"
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || {
  echo 'Only stable vX.Y.Z tags can publish this fork.' >&2
  exit 1
}
[[ "${GITHUB_REPOSITORY:-}" == 'quboqin/multica' ]] || {
  echo 'Publishing is restricted to quboqin/multica.' >&2
  exit 1
}
git fetch --no-tags origin qqb_main:refs/remotes/origin/qqb_main
commit="$(git rev-parse HEAD)"
[[ "$(git rev-parse "refs/tags/$tag^{commit}")" == "$commit" ]] || {
  echo 'The checkout does not match the release tag.' >&2
  exit 1
}
git merge-base --is-ancestor "$commit" origin/qqb_main || {
  echo 'The release commit must be merged into qqb_main.' >&2
  exit 1
}
echo "Verified $tag at $commit on qqb_main."
