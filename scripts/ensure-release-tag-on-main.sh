#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TAG="${1:-${GITHUB_REF_NAME:-}}"
MAIN_REF="${MAIN_REF:-origin/main}"

err() {
  echo "[ensure-release-tag-on-main] ERROR: $*" >&2
  exit 1
}

[[ -n "$TAG" ]] || err "usage: $0 <tag>"
git rev-parse -q --verify "refs/tags/$TAG" >/dev/null || err "tag '$TAG' not found"

if git remote get-url origin >/dev/null 2>&1; then
  git fetch --quiet origin main --tags
fi

git rev-parse -q --verify "$MAIN_REF" >/dev/null || err "main ref '$MAIN_REF' not found"

TAG_COMMIT="$(git rev-list -n 1 "$TAG")"
MAIN_COMMIT="$(git rev-parse "$MAIN_REF")"

if [[ "$TAG_COMMIT" != "$MAIN_COMMIT" ]]; then
  err "release tag '$TAG' points to $TAG_COMMIT, but $MAIN_REF is $MAIN_COMMIT; release tags must point at current main"
fi

echo "[ensure-release-tag-on-main] $TAG is on current main ($MAIN_COMMIT)"
