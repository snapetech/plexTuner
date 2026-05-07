#!/usr/bin/env bash
set -euo pipefail

proxy_url="${PROXY_URL:?set PROXY_URL, e.g. https://media.example.com}"
owner_token="${OWNER_TOKEN:-${IPTV_TUNERR_PMS_OWNER_TOKEN:-${IPTV_TUNERR_PMS_TOKEN:-${PLEX_TOKEN:-}}}}"
user_token="${USER_TOKEN:-}"
fake_token="${FAKE_TOKEN:-iptv-tunerr-fake-shared-user-token}"

if [[ -z "$owner_token" ]]; then
  echo "missing OWNER_TOKEN/IPTV_TUNERR_PMS_OWNER_TOKEN/IPTV_TUNERR_PMS_TOKEN/PLEX_TOKEN" >&2
  exit 2
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

check() {
  local label="$1"
  local url="$2"
  local want="$3"
  local out="$tmp/${label//[^a-zA-Z0-9]/_}.out"
  local code
  code=$(curl -ksS --compressed -o "$out" -w '%{http_code}' "$url" || true)
  printf '%-28s code=%s want=%s bytes=%s\n' "$label" "$code" "$want" "$(wc -c <"$out")"
  if [[ "$code" != "$want" ]]; then
    echo "unexpected status for $label" >&2
    head -c 300 "$out" >&2 || true
    echo >&2
    exit 1
  fi
}

check "owner identity" "$proxy_url/identity?X-Plex-Token=$owner_token" 200
check "fake libraries not elevated" "$proxy_url/library/sections?X-Plex-Token=$fake_token" 401
check "fake Live TV DVRs elevated" "$proxy_url/livetv/dvrs?X-Plex-Token=$fake_token" 200
check "fake media providers elevated" "$proxy_url/media/providers?X-Plex-Token=$fake_token" 200

if [[ -n "$user_token" ]]; then
  check "real user Live TV DVRs" "$proxy_url/livetv/dvrs?X-Plex-Token=$user_token" 200
  check "real user media providers" "$proxy_url/media/providers?X-Plex-Token=$user_token" 200
fi

echo "proxy validation passed: normal library requests stayed user-scoped, Live TV requests were elevated"
