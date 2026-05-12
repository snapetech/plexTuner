#!/usr/bin/env bash
set -euo pipefail

proxy_url="${PROXY_URL:?set PROXY_URL, e.g. https://media.example.com}"
owner_token="${OWNER_TOKEN:-${IPTV_TUNERR_PMS_OWNER_TOKEN:-${IPTV_TUNERR_PMS_TOKEN:-${PLEX_TOKEN:-}}}}"
user_token="${USER_TOKEN:-}"
fake_token="${FAKE_TOKEN:-iptv-tunerr-fake-shared-user-token}"
expect_block="${EXPECT_BLOCK:-1}"
bad_source="${BAD_SOURCE:-198.51.100.99}"

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
  printf '%-36s code=%s want=%s bytes=%s\n' "$label" "$code" "$want" "$(wc -c <"$out")"
  if [[ "$code" != "$want" ]]; then
    echo "unexpected status for $label" >&2
    head -c 300 "$out" >&2 || true
    echo >&2
    exit 1
  fi
}

check_header() {
  local label="$1"
  local url="$2"
  local want="$3"
  local out="$tmp/${label//[^a-zA-Z0-9]/_}.out"
  local code
  code=$(curl -ksS --compressed -H "X-Forwarded-For: $bad_source" -o "$out" -w '%{http_code}' "$url" || true)
  printf '%-36s code=%s want=%s bytes=%s\n' "$label" "$code" "$want" "$(wc -c <"$out")"
  if [[ "$code" != "$want" ]]; then
    echo "unexpected status for $label" >&2
    head -c 300 "$out" >&2 || true
    echo >&2
    exit 1
  fi
}

check "public identity no token" "$proxy_url/identity" 200
check "owner identity" "$proxy_url/identity?X-Plex-Token=$owner_token" 200
check "owner Live TV DVRs" "$proxy_url/livetv/dvrs?X-Plex-Token=$owner_token" 200
check "fake libraries not elevated" "$proxy_url/library/sections?X-Plex-Token=$fake_token" 401
check "fake Live TV denied" "$proxy_url/livetv/dvrs?X-Plex-Token=$fake_token" 403
check "missing Live TV denied" "$proxy_url/livetv/dvrs" 403

if [[ -n "$user_token" ]]; then
  check "real user libraries" "$proxy_url/library/sections?X-Plex-Token=$user_token" 200
  check "real user Live TV DVRs" "$proxy_url/livetv/dvrs?X-Plex-Token=$user_token" 200
  check "real user media providers" "$proxy_url/media/providers?X-Plex-Token=$user_token" 200
else
  echo "USER_TOKEN not set; skipping live shared-user proof and relying on automated proxy tests for that path"
fi

if [[ "$expect_block" == "1" ]]; then
  for i in 1 2 3 4 5; do
    check_header "bad Live TV attempt $i" "$proxy_url/livetv/dvrs?X-Plex-Token=${fake_token}-${i}" 403
  done
  check_header "bad Live TV blocked" "$proxy_url/livetv/dvrs?X-Plex-Token=${fake_token}-blocked" 429
fi

echo "proxy validation passed: Live TV entitlement works for authorized tokens, random/no-token callers are denied, and repeated bad attempts are blocked"
