#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

CHANGELOG="${CHANGELOG:-docs/CHANGELOG.md}"

err() {
  echo "[verify-changelog-entry] ERROR: $*" >&2
  exit 1
}

usage() {
  cat >&2 <<'EOF'
Usage:
  scripts/verify-changelog-entry.sh release <vX.Y.Z>
  scripts/verify-changelog-entry.sh range <base-ref> [head-ref]
  scripts/verify-changelog-entry.sh staged

Rules:
  - Release tags require a populated docs/CHANGELOG.md section for that tag.
  - Non-trivial code, workflow, script, packaging, or docs changes require
    docs/CHANGELOG.md to be changed in the same staged/range set.
EOF
  exit 2
}

changed_paths_range() {
  local base="$1" head="${2:-HEAD}"
  git diff --name-only "$base" "$head"
}

changed_paths_staged() {
  git diff --cached --name-only --diff-filter=ACMRT
}

requires_changelog() {
  awk '
    $0 == "docs/CHANGELOG.md" { next }
    $0 ~ /^memory-bank\// { next }
    $0 ~ /^dist\// { next }
    $0 ~ /^\.diag\// { next }
    $0 ~ /^docs\/CHANGELOG\.md$/ { next }
    $0 ~ /^cmd\// { found = 1 }
    $0 ~ /^internal\// { found = 1 }
    $0 ~ /^scripts\// { found = 1 }
    $0 ~ /^packaging\// { found = 1 }
    $0 ~ /^\.github\/workflows\// { found = 1 }
    $0 ~ /^docs\// { found = 1 }
    END { exit found ? 0 : 1 }
  '
}

has_changelog_change() {
  grep -Fxq "$CHANGELOG"
}

verify_changed_set() {
  local changed="$1"
  [[ -n "$changed" ]] || return 0

  if printf '%s\n' "$changed" | requires_changelog; then
    if ! printf '%s\n' "$changed" | has_changelog_change; then
      err "changes touch release-relevant files but do not update $CHANGELOG"
    fi
  fi
}

extract_release_section() {
  local tag="$1"
  awk -v tag="$tag" '
    $0 ~ "^## \\[" tag "\\]" { in_section = 1; next }
    in_section && /^## \[/ { exit }
    in_section { print }
  ' "$CHANGELOG"
}

verify_release() {
  local tag="$1"
  [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z]+)*$ ]] || err "release tag must look like vX.Y.Z; got '$tag'"

  local section
  section="$(extract_release_section "$tag")"
  [[ -n "$section" ]] || err "$CHANGELOG has no section for $tag"

  if ! grep -Eq '^## \[' <<<"$section"; then
    :
  fi
  if ! grep -Eq '^### ' <<<"$section"; then
    err "$CHANGELOG section for $tag must include at least one subsection heading"
  fi
  if ! grep -Eq '^- .+' <<<"$section"; then
    err "$CHANGELOG section for $tag must include at least one bullet"
  fi
  if grep -Eq '\*\(none\)\*|TBD|TODO|placeholder' <<<"$section"; then
    err "$CHANGELOG section for $tag still looks like a placeholder"
  fi

  echo "[verify-changelog-entry] $CHANGELOG has a populated section for $tag"
}

mode="${1:-}"
case "$mode" in
  release)
    [[ $# -eq 2 ]] || usage
    verify_release "$2"
    ;;
  range)
    [[ $# -ge 2 && $# -le 3 ]] || usage
    verify_changed_set "$(changed_paths_range "$2" "${3:-HEAD}")"
    ;;
  staged)
    [[ $# -eq 1 ]] || usage
    verify_changed_set "$(changed_paths_staged)"
    ;;
  *)
    usage
    ;;
esac
