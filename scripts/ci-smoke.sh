#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d)"
BIN="$TMP_DIR/iptv-tunerr"

cleanup() {
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

log() {
  printf '[ci-smoke] %s\n' "$*"
}

fail() {
  printf '[ci-smoke] ERROR: %s\n' "$*" >&2
  exit 1
}

pick_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

wait_http_code() {
  local url="$1" want="$2" attempts="${3:-50}"
  local code
  for _ in $(seq 1 "$attempts"); do
    code="$(curl -sS -o /dev/null -w '%{http_code}' "$url" || true)"
    if [[ "$code" == "$want" ]]; then
      return 0
    fi
    sleep 0.2
  done
  return 1
}

header_value() {
  local url="$1" header="$2"
  curl -sSI "$url" | awk -F': ' -v key="$header" 'tolower($1) == tolower(key) { sub(/\r$/, "", $2); print $2; exit }'
}

body_file="$TMP_DIR/body.out"

assert_status() {
  local url="$1" want="$2"
  local code
  code="$(curl -sS -o "$body_file" -w '%{http_code}' "$url" || true)"
  [[ "$code" == "$want" ]] || fail "$url status=$code want $want body=$(cat "$body_file" 2>/dev/null)"
}

assert_header() {
  local url="$1" header="$2" want="$3"
  local got
  got="$(header_value "$url" "$header")"
  [[ "$got" == "$want" ]] || fail "$url header $header=$got want $want"
}

PIDS=()

cat >"$TMP_DIR/catalog-full.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": [
    {
      "channel_id": "ch1",
      "guide_number": "101",
      "guide_name": "Smoke One",
      "stream_url": "http://example.invalid/stream-1.ts",
      "stream_urls": ["http://example.invalid/stream-1.ts"],
      "epg_linked": true,
      "tvg_id": "smoke.one"
    }
  ]
}
JSON

cat >"$TMP_DIR/catalog-empty.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": []
}
JSON

log "building binary"
go build -o "$BIN" ./cmd/iptv-tunerr

run_serve() {
  local catalog="$1" port="$2"
  IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
  IPTV_TUNERR_XMLTV_URL= \
  IPTV_TUNERR_WEBUI_DISABLED=1 \
  "$BIN" serve -catalog "$catalog" -addr ":$port" -base-url "http://127.0.0.1:$port" \
    >"$TMP_DIR/serve-$port.log" 2>&1 &
  PIDS+=("$!")
}

port_full="$(pick_port)"
run_serve "$TMP_DIR/catalog-full.json" "$port_full"
wait_http_code "http://127.0.0.1:$port_full/discover.json" "200" || fail "full catalog discover.json not ready"
assert_status "http://127.0.0.1:$port_full/readyz" "200"
assert_status "http://127.0.0.1:$port_full/guide.xml" "200"
assert_header "http://127.0.0.1:$port_full/guide.xml" "X-IptvTunerr-Guide-State" "ready"
assert_status "http://127.0.0.1:$port_full/lineup.json" "200"
grep -q '"GuideNumber":"101"' <(curl -sS "http://127.0.0.1:$port_full/lineup.json") || fail "full catalog lineup missing expected guide number"

port_empty="$(pick_port)"
run_serve "$TMP_DIR/catalog-empty.json" "$port_empty"
wait_http_code "http://127.0.0.1:$port_empty/discover.json" "200" || fail "empty catalog discover.json not ready"
assert_status "http://127.0.0.1:$port_empty/readyz" "503"
assert_status "http://127.0.0.1:$port_empty/guide.xml" "503"
assert_header "http://127.0.0.1:$port_empty/guide.xml" "X-IptvTunerr-Guide-State" "loading"
assert_header "http://127.0.0.1:$port_empty/guide.xml" "Retry-After" "5"
assert_status "http://127.0.0.1:$port_empty/lineup.json" "200"
assert_header "http://127.0.0.1:$port_empty/discover.json" "X-IptvTunerr-Startup-State" "loading"
assert_header "http://127.0.0.1:$port_empty/lineup.json" "X-IptvTunerr-Startup-State" "loading"

log "smoke checks passed"
