#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 || $# -gt 4 ]]; then
  echo "Usage: $0 <repo-dir> <aur-package-name> <commit-message> [branch]" >&2
  exit 1
fi

repo_dir="$1"
package_name="$2"
commit_message="$3"
branch="${4:-master}"

cd "$repo_dir"

git config user.name "IPTV Tunerr CI"
git config user.email "iptvtunerr@proton.me"

git commit -m "$commit_message" || echo "No AUR changes to commit for ${package_name}."

for attempt in 1 2 3 4 5; do
  if git push origin HEAD:"$branch"; then
    echo "Pushed to AUR: ${package_name}"
    exit 0
  fi

  if [[ "$attempt" -eq 5 ]]; then
    echo "ERROR: failed to push AUR repo ${package_name} after ${attempt} attempts." >&2
    exit 1
  fi

  sleep_seconds=$((attempt * 2))
  echo "Push failed for ${package_name}; rebasing and retrying in ${sleep_seconds}s..." >&2
  sleep "$sleep_seconds"
  git fetch origin "$branch" && git pull --rebase origin "$branch" || true
done
