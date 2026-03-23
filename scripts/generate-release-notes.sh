#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

err() {
  echo "[generate-release-notes] ERROR: $*" >&2
  exit 1
}

trim_blank_edges() {
  awk '
    { lines[NR] = $0 }
    END {
      start = 1
      while (start <= NR && lines[start] ~ /^[[:space:]]*$/) start++
      end = NR
      while (end >= start && lines[end] ~ /^[[:space:]]*$/) end--
      for (i = start; i <= end; i++) print lines[i]
    }
  '
}

extract_changelog_section() {
  local heading="$1"
  awk -v heading="$heading" '
    $0 == heading { in_section = 1; next }
    in_section && /^## \[/ { exit }
    in_section && /^---$/ { next }
    in_section { print }
  ' docs/CHANGELOG.md | trim_blank_edges
}

normalize_remote_url() {
  local raw="$1"
  case "$raw" in
    https://github.com/*)
      printf '%s\n' "${raw%.git}"
      ;;
    git@github.com:*)
      raw="${raw#git@github.com:}"
      printf 'https://github.com/%s\n' "${raw%.git}"
      ;;
    *)
      printf '%s\n' "${raw%.git}"
      ;;
  esac
}

subject_highlight() {
  local subject="$1"
  subject="$(printf '%s' "$subject" | sed -E 's/^[a-z]+(\([^)]+\))?:[[:space:]]*//')"
  printf '%s' "$subject" | awk '
    {
      line = $0
      if (length(line) == 0) next
      first = substr(line, 1, 1)
      rest = substr(line, 2)
      line = toupper(first) rest
      if (line !~ /[.!?]$/) line = line "."
      print line
    }
  '
}

TAG="${1:-${GITHUB_REF_NAME:-}}"
OUT_PATH="${2:-$ROOT_DIR/dist/release-notes.md}"

[[ -n "$TAG" ]] || err "usage: $0 <tag> [output-path]"
git rev-parse -q --verify "refs/tags/$TAG" >/dev/null || err "tag '$TAG' not found"

mkdir -p "$(dirname "$OUT_PATH")"

REMOTE_URL="$(git config --get remote.origin.url || true)"
REPO_URL="$(normalize_remote_url "$REMOTE_URL")"
RELEASE_DATE="$(git log -1 --format=%cs "$TAG")"
PREV_TAG="$(git tag --list 'v*' --sort=-v:refname | awk -v cur="$TAG" '$0 != cur { print; exit }')"

TAG_SECTION="$(extract_changelog_section "## [$TAG] — $RELEASE_DATE" || true)"
if [[ -z "$TAG_SECTION" ]]; then
  TAG_SECTION="$(awk -v tag="$TAG" '
    $0 ~ "^## \\[" tag "\\]" { in_section = 1; next }
    in_section && /^## \[/ { exit }
    in_section { print }
  ' docs/CHANGELOG.md | trim_blank_edges)"
fi
UNRELEASED_SECTION="$(extract_changelog_section "## [Unreleased]" || true)"

if [[ "$UNRELEASED_SECTION" == "- *(none)*" ]]; then
  UNRELEASED_SECTION=""
fi

if [[ -n "$PREV_TAG" ]]; then
  RANGE="${PREV_TAG}..${TAG}"
  COMPARE_URL="$REPO_URL/compare/${PREV_TAG}...${TAG}"
else
  RANGE="${TAG}^!"
  COMPARE_URL=""
fi

COMMITS="$(git log --reverse --no-merges --format='%H%x09%s' "$RANGE")"
if [[ -z "$COMMITS" ]]; then
  COMMITS="$(git log --reverse --format='%H%x09%s' "$RANGE")"
fi

{
  printf '# IPTV Tunerr %s\n\n' "$TAG"
  printf 'Released: %s\n\n' "$RELEASE_DATE"

  if [[ -n "$PREV_TAG" && -n "$REPO_URL" ]]; then
    printf 'Compare: [%s...%s](%s)\n\n' "$PREV_TAG" "$TAG" "$COMPARE_URL"
  elif [[ -n "$PREV_TAG" ]]; then
    printf 'Compare: `%s...%s`\n\n' "$PREV_TAG" "$TAG"
  fi

  printf '## What Changed\n\n'

  if [[ -n "$TAG_SECTION" ]]; then
    printf '%s\n\n' "$TAG_SECTION"
    printf '_Source: `docs/CHANGELOG.md` section for `%s`._\n\n' "$TAG"
  elif [[ -n "$UNRELEASED_SECTION" ]]; then
    printf '%s\n\n' "$UNRELEASED_SECTION"
    printf '_Source: `docs/CHANGELOG.md` `Unreleased` section at tag time._\n\n'
  else
    if [[ -z "$COMMITS" ]]; then
      printf '%s\n\n' "- No recorded changes found for \`$TAG\`."
    else
      while IFS=$'\t' read -r sha subject; do
        highlight="$(subject_highlight "$subject")"
        if [[ -n "$highlight" ]]; then
          printf -- '- %s\n' "$highlight"
        fi
      done <<<"$COMMITS"
      printf '\n'
    fi
  fi

  printf '## Included Commits\n\n'
  if [[ -z "$COMMITS" ]]; then
    printf '%s\n' "- No commits found for \`$TAG\`."
  else
    while IFS=$'\t' read -r sha subject; do
      short_sha="${sha:0:7}"
      if [[ -n "$REPO_URL" ]]; then
        printf -- '- `%s` %s ([commit](%s/commit/%s))\n' "$short_sha" "$subject" "$REPO_URL" "$sha"
      else
        printf -- '- `%s` %s\n' "$short_sha" "$subject"
      fi
    done <<<"$COMMITS"
  fi
} >"$OUT_PATH"

printf 'Wrote %s\n' "$OUT_PATH"
