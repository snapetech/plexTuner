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

assert_contains() {
  local haystack="$1" needle="$2" context="${3:-}"
  [[ "$haystack" == *"$needle"* ]] || fail "expected output to contain '$needle' ${context:+($context)}"
}

PIDS=()

cat >"$TMP_DIR/catalog-full.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": [
    {
      "channel_id": "ch1",
      "dna_id": "dna-news",
      "guide_number": "101",
      "guide_name": "Smoke One",
      "group_title": "News",
      "stream_url": "http://example.invalid/stream-1.ts",
      "stream_urls": ["http://example.invalid/stream-1.ts"],
      "epg_linked": true,
      "tvg_id": "smoke.one"
    },
    {
      "channel_id": "ch2",
      "guide_number": "102",
      "guide_name": "Smoke Two",
      "group_title": "Sports",
      "stream_url": "http://example.invalid/stream-2.ts",
      "stream_urls": ["http://example.invalid/stream-2.ts"],
      "epg_linked": true,
      "tvg_id": "smoke.two"
    },
    {
      "channel_id": "ch3",
      "dna_id": "dna-news",
      "guide_number": "1001",
      "guide_name": "Smoke One",
      "group_title": "DirecTV",
      "source_tag": "directv",
      "stream_url": "http://example.invalid/stream-3.ts",
      "stream_urls": ["http://example.invalid/stream-3.ts"],
      "epg_linked": true,
      "tvg_id": "smoke.one"
    }
  ]
}
JSON

cat >"$TMP_DIR/programming.json" <<'JSON'
{
  "selected_categories": ["news"],
  "order_mode": "source"
}
JSON

cat >"$TMP_DIR/catalog-empty.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": []
}
JSON

cat >"$TMP_DIR/catalog-vod.json" <<'JSON'
{
  "movies": [
    {
      "id": "m1",
      "title": "Smoke Movie",
      "year": 2024,
      "stream_url": "http://example.invalid/movie.mp4"
    }
  ],
  "series": [
    {
      "id": "s1",
      "title": "Smoke Show",
      "year": 2023,
      "seasons": [
        {
          "number": 1,
          "episodes": [
            {
              "id": "e1",
              "season_num": 1,
              "episode_num": 1,
              "title": "Pilot",
              "stream_url": "http://example.invalid/show-s01e01.mp4"
            }
          ]
        }
      ]
    }
  ],
  "live_channels": []
}
JSON

log "building binary"
go build -o "$BIN" ./cmd/iptv-tunerr

darwin_hint="$("$BIN" vod-webdav-mount-hint -os darwin -addr 127.0.0.1:58188 2>&1)"
assert_contains "$darwin_hint" "mount_webdav" "darwin mount hint"

windows_hint="$("$BIN" vod-webdav-mount-hint -os windows -addr 127.0.0.1:58188 2>&1)"
assert_contains "$windows_hint" "net use" "windows mount hint"

run_serve() {
  local catalog="$1" port="$2"
  IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
  IPTV_TUNERR_XMLTV_URL= \
  IPTV_TUNERR_WEBUI_DISABLED=1 \
  IPTV_TUNERR_PROGRAMMING_RECIPE_FILE="$TMP_DIR/programming.json" \
  "$BIN" serve -catalog "$catalog" -addr ":$port" -base-url "http://127.0.0.1:$port" \
    >"$TMP_DIR/serve-$port.log" 2>&1 &
  PIDS+=("$!")
}

run_vod_webdav() {
  local catalog="$1" port="$2"
  "$BIN" vod-webdav -catalog "$catalog" -addr "127.0.0.1:$port" \
    >"$TMP_DIR/vod-webdav-$port.log" 2>&1 &
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
! grep -q '"GuideNumber":"102"' <(curl -sS "http://127.0.0.1:$port_full/lineup.json") || fail "programming recipe did not filter second category"
grep -q '"raw_channels": 3' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing raw channel count"
grep -q '"curated_channels": 1' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing curated channel count"
grep -q '"id": "news"' <(curl -sS "http://127.0.0.1:$port_full/programming/categories.json") || fail "programming categories missing News"
grep -q '"id": "sports"' <(curl -sS "http://127.0.0.1:$port_full/programming/categories.json") || fail "programming categories missing Sports"
grep -q '"group_count": 0' <(curl -sS "http://127.0.0.1:$port_full/programming/backups.json") || fail "programming backups unexpected initial group"
category_mutate_code="$(curl -sS -X POST -H 'Content-Type: application/json' --data '{"action":"include","category_id":"sports"}' -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_full/programming/categories.json" || true)"
[[ "$category_mutate_code" == "200" ]] || fail "programming category mutation status=$category_mutate_code body=$(cat "$body_file" 2>/dev/null)"
channel_mutate_code="$(curl -sS -X POST -H 'Content-Type: application/json' --data '{"action":"exclude","channel_id":"ch1"}' -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_full/programming/channels.json" || true)"
[[ "$channel_mutate_code" == "200" ]] || fail "programming channel mutation status=$channel_mutate_code body=$(cat "$body_file" 2>/dev/null)"
order_mutate_code="$(curl -sS -X POST -H 'Content-Type: application/json' --data '{"action":"prepend","channel_id":"ch2"}' -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_full/programming/order.json" || true)"
[[ "$order_mutate_code" == "200" ]] || fail "programming order mutation status=$order_mutate_code body=$(cat "$body_file" 2>/dev/null)"
grep -q '"curated_channels": 1' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming mutation preview missing updated curated count"
grep -q '"sports": 1' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing sports bucket"
collapse_code="$(curl -sS -X POST -H 'Content-Type: application/json' --data '{"selected_categories":["news","directv","sports"],"order_mode":"custom","custom_order":["ch2","ch3"],"collapse_exact_backups":true}' -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_full/programming/recipe.json" || true)"
[[ "$collapse_code" == "200" ]] || fail "programming collapse recipe status=$collapse_code body=$(cat "$body_file" 2>/dev/null)"
grep -q '"curated_channels": 2' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing collapsed curated count"
grep -q '"stream_urls": \[' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing lineup stream urls"
grep -q '"group_count": 1' <(curl -sS "http://127.0.0.1:$port_full/programming/backups.json") || fail "programming backups missing grouped exact match"

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

port_vod="$(pick_port)"
run_vod_webdav "$TMP_DIR/catalog-vod.json" "$port_vod"
wait_http_code "http://127.0.0.1:$port_vod/" "405" || fail "vod-webdav root not ready"
options_headers="$TMP_DIR/options.headers"
options_code="$(curl -sS -X OPTIONS -D "$options_headers" -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port_vod/" || true)"
[[ "$options_code" == "200" ]] || fail "vod-webdav OPTIONS status=$options_code"
grep -qi '^DAV:' "$options_headers" || fail "vod-webdav OPTIONS missing DAV header"
propfind_body="$TMP_DIR/propfind.xml"
propfind_code="$(curl -sS -X PROPFIND -H 'Depth: 1' -H 'Content-Type: text/xml' --data '<propfind xmlns="DAV:"><allprop/></propfind>' -o "$propfind_body" -w '%{http_code}' "http://127.0.0.1:$port_vod/" || true)"
[[ "$propfind_code" == "207" ]] || fail "vod-webdav PROPFIND status=$propfind_code body=$(cat "$propfind_body" 2>/dev/null)"
grep -q "Movies" "$propfind_body" || fail "vod-webdav PROPFIND missing Movies"
grep -q "TV" "$propfind_body" || fail "vod-webdav PROPFIND missing TV"
movies_body="$TMP_DIR/propfind-movies.xml"
movies_code="$(curl -sS -X PROPFIND -H 'Depth: 1' -H 'Content-Type: text/xml' --data '<a:propfind xmlns:a="DAV:"><a:allprop/></a:propfind>' -o "$movies_body" -w '%{http_code}' "http://127.0.0.1:$port_vod/Movies" || true)"
[[ "$movies_code" == "207" ]] || fail "vod-webdav Movies PROPFIND status=$movies_code body=$(cat "$movies_body" 2>/dev/null)"
grep -q "Smoke Movie" "$movies_body" || fail "vod-webdav Movies PROPFIND missing movie directory"

log "smoke checks passed"
