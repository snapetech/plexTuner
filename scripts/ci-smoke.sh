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
  "movies": [
    {
      "id": "m1",
      "title": "Movie One",
      "year": 2024,
      "stream_url": "http://example.invalid/movie-1.mp4"
    }
  ],
  "series": [
    {
      "id": "s1",
      "title": "Series One",
      "seasons": [
        {
          "number": 1,
          "episodes": [
            {
              "id": "e1",
              "season_num": 1,
              "episode_num": 1,
              "title": "Pilot",
              "stream_url": "http://example.invalid/series-1.mp4"
            }
          ]
        }
      ]
    }
  ],
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

cat >"$TMP_DIR/recording-rules.json" <<'JSON'
{
  "rules": [
    {
      "id": "news-live",
      "name": "News Live",
      "enabled": true,
      "include_channel_ids": ["ch1"]
    }
  ]
}
JSON

cat >"$TMP_DIR/xtream-users.json" <<'JSON'
{
  "users": [
    {
      "username": "limited",
      "password": "pw",
      "allow_live": true,
      "allow_movies": true,
      "allow_series": false,
      "allowed_channel_ids": ["ch1"],
      "allowed_movie_ids": ["m1"],
      "allowed_category_ids": ["news"]
    }
  ]
}
JSON

cat >"$TMP_DIR/lineup-harvest.json" <<'JSON'
{
  "plex_url": "plex.example:32400",
  "results": [
    {
      "base_url": "http://oracle-100:5004",
      "cap": "100",
      "friendly_name": "harvest-100",
      "lineup_title": "Rogers West",
      "channelmap_rows": 420,
      "channels": [
        { "guide_number": "101", "guide_name": "Smoke One", "tvg_id": "smoke.one" },
        { "guide_number": "102", "guide_name": "Smoke Two", "tvg_id": "smoke.two" }
      ]
    }
  ]
}
JSON

cat >"$TMP_DIR/virtual-channels.json" <<'JSON'
{
  "channels": [
    {
      "id": "vc-news",
      "name": "News Loop",
      "guide_number": "9001",
      "enabled": true,
      "loop_daily_utc": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 },
        { "type": "episode", "series_id": "s1", "episode_id": "e1", "duration_mins": 30 }
      ]
    }
  ]
}
JSON

cat >"$TMP_DIR/recorder-state.json" <<'JSON'
{
  "completed": [
    {
      "capsule_id": "done-1",
      "channel_id": "ch1",
      "guide_number": "101",
      "channel_name": "Smoke One",
      "title": "Smoke Recording",
      "lane": "general",
      "status": "recorded",
      "published_path": "/tmp/smoke-recording.ts"
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

cat >"$TMP_DIR/catalog-full-shuffled.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": [
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
      "channel_id": "ch1",
      "dna_id": "dna-news",
      "guide_number": "101",
      "guide_name": "Smoke One",
      "group_title": "News",
      "stream_url": "http://example.invalid/stream-1.ts",
      "stream_urls": ["http://example.invalid/stream-1.ts"],
      "epg_linked": true,
      "tvg_id": "smoke.one"
    }
  ]
}
JSON

cat >"$TMP_DIR/catalog-vod.json" <<'JSON'
{
  "movies": [
    {
      "id": "m1",
      "title": "Smoke Movie",
      "year": 2024,
      "stream_url": "REPLACE_MOVIE_URL"
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
              "stream_url": "REPLACE_EPISODE_URL"
            }
          ]
        }
      ]
    }
  ],
  "live_channels": []
}
JSON

mkdir -p "$TMP_DIR/assets"
printf 'movie-bytes' >"$TMP_DIR/assets/movie.bin"
printf 'episode-bytes' >"$TMP_DIR/assets/episode.bin"

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
  IPTV_TUNERR_XTREAM_USER=demo \
  IPTV_TUNERR_XTREAM_PASS=secret \
  IPTV_TUNERR_XTREAM_USERS_FILE="$TMP_DIR/xtream-users.json" \
  IPTV_TUNERR_PROGRAMMING_RECIPE_FILE="$TMP_DIR/programming.json" \
  IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE="$TMP_DIR/lineup-harvest.json" \
  IPTV_TUNERR_VIRTUAL_CHANNELS_FILE="$TMP_DIR/virtual-channels.json" \
  IPTV_TUNERR_RECORDING_RULES_FILE="$TMP_DIR/recording-rules.json" \
  IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE="$TMP_DIR/recorder-state.json" \
  "$BIN" serve -catalog "$catalog" -addr ":$port" -base-url "http://127.0.0.1:$port" \
    >"$TMP_DIR/serve-$port.log" 2>&1 &
  PIDS+=("$!")
}

run_vod_webdav() {
  local catalog="$1" port="$2"
  "$BIN" vod-webdav -catalog "$catalog" -addr "127.0.0.1:$port" -cache "$TMP_DIR/vod-cache" \
    >"$TMP_DIR/vod-webdav-$port.log" 2>&1 &
  PIDS+=("$!")
}

run_asset_server() {
  local port="$1"
  python3 -m http.server "$port" --bind 127.0.0.1 --directory "$TMP_DIR/assets" \
    >"$TMP_DIR/assets-$port.log" 2>&1 &
  PIDS+=("$!")
}

port_assets="$(pick_port)"
run_asset_server "$port_assets"
wait_http_code "http://127.0.0.1:$port_assets/movie.bin" "200" || fail "asset server not ready"
for catalog_file in "$TMP_DIR/catalog-full.json" "$TMP_DIR/catalog-full-shuffled.json" "$TMP_DIR/catalog-vod.json"; do
  sed -i "s|REPLACE_MOVIE_URL|http://127.0.0.1:$port_assets/movie.bin|g" "$catalog_file" 2>/dev/null || true
  sed -i "s|REPLACE_EPISODE_URL|http://127.0.0.1:$port_assets/episode.bin|g" "$catalog_file" 2>/dev/null || true
done
sed -i "s|http://example.invalid/movie-1.mp4|http://127.0.0.1:$port_assets/movie.bin|g" "$TMP_DIR/catalog-full.json" "$TMP_DIR/catalog-full-shuffled.json"
sed -i "s|http://example.invalid/series-1.mp4|http://127.0.0.1:$port_assets/episode.bin|g" "$TMP_DIR/catalog-full.json" "$TMP_DIR/catalog-full-shuffled.json"

port_full="$(pick_port)"
run_serve "$TMP_DIR/catalog-full.json" "$port_full"
full_pid="${PIDS[${#PIDS[@]}-1]}"
wait_http_code "http://127.0.0.1:$port_full/discover.json" "200" || fail "full catalog discover.json not ready"
assert_status "http://127.0.0.1:$port_full/readyz" "200"
assert_status "http://127.0.0.1:$port_full/guide.xml" "200"
assert_header "http://127.0.0.1:$port_full/guide.xml" "X-IptvTunerr-Guide-State" "ready"
assert_status "http://127.0.0.1:$port_full/lineup.json" "200"
grep -q '"GuideNumber":"101"' <(curl -sS "http://127.0.0.1:$port_full/lineup.json") || fail "full catalog lineup missing expected guide number"
! grep -q '"GuideNumber":"102"' <(curl -sS "http://127.0.0.1:$port_full/lineup.json") || fail "programming recipe did not filter second category"
grep -q '"raw_channels": 3' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing raw channel count"
grep -q '"name": "News Live"' <(curl -sS "http://127.0.0.1:$port_full/recordings/rules.json") || fail "recording rules endpoint missing seeded rule"
curl -sS -X POST -H 'Content-Type: application/json' --data '{"action":"upsert","rule":{"id":"sports-live","name":"Sports Live","enabled":true,"include_channel_ids":["ch2"]}}' "http://127.0.0.1:$port_full/recordings/rules.json" >/dev/null
grep -q '"sports-live"' <"$TMP_DIR/recording-rules.json" || fail "recording rules mutation did not persist to file"
grep -q '"completed_count": 1' <(curl -sS "http://127.0.0.1:$port_full/recordings/history.json") || fail "recording history missing completed recorder item"
grep -q '"curated_channels": 1' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing curated channel count"
grep -q '"id": "news"' <(curl -sS "http://127.0.0.1:$port_full/programming/categories.json") || fail "programming categories missing News"
grep -q '"id": "sports"' <(curl -sS "http://127.0.0.1:$port_full/programming/categories.json") || fail "programming categories missing Sports"
grep -q '"group_count": 0' <(curl -sS "http://127.0.0.1:$port_full/programming/backups.json") || fail "programming backups unexpected initial group"
grep -q '"lineup_title": "Rogers West"' <(curl -sS "http://127.0.0.1:$port_full/programming/harvest.json") || fail "programming harvest missing seeded lineup title"
grep -q '"harvest_ready": true' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing harvest readiness"
grep -q '"matched_channels":' <(curl -sS "http://127.0.0.1:$port_full/programming/harvest-import.json?lineup_title=Rogers%20West&replace=1") || fail "programming harvest import preview missing matched rows"
grep -q '"match_strategies":' <(curl -sS "http://127.0.0.1:$port_full/programming/harvest-import.json?lineup_title=Rogers%20West&replace=1") || fail "programming harvest import preview missing strategy summary"
grep -q '"recommended": true' <(curl -sS "http://127.0.0.1:$port_full/programming/harvest-assist.json") || fail "programming harvest assist missing recommended lineup"
grep -q '"resolved_name": "Movie One"' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/preview.json?per_channel=2") || fail "virtual channel preview missing movie slot"
grep -q '"slots": \[' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/schedule.json?horizon=3h") || fail "virtual channel schedule missing slots"
grep -q '"resolved_now"' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/channel-detail.json?channel_id=vc-news&limit=2&horizon=3h") || fail "virtual channel detail missing resolved_now"
grep -q '<channel id="virtual.vc-news">' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/guide.xml?horizon=3h") || fail "virtual channel guide missing channel id"
grep -q '/virtual-channels/stream/vc-news.mp4' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/live.m3u") || fail "virtual channel m3u missing stream url"
virtual_stream_body="$(curl -sS "http://127.0.0.1:$port_full/virtual-channels/stream/vc-news.mp4")"
[[ "$virtual_stream_body" == "movie-bytes" || "$virtual_stream_body" == "episode-bytes" ]] || fail "virtual channel stream missing playable bytes"
grep -q '"alternative_sources"' <(curl -sS "http://127.0.0.1:$port_full/programming/channel-detail.json?channel_id=ch1") || fail "programming channel detail missing alternatives section"
grep -q '"stream_type":"live"' <(curl -sS "http://127.0.0.1:$port_full/player_api.php?username=demo&password=secret&action=get_live_streams") || fail "xtream live streams endpoint missing live row"
limited_live="$(curl -sS "http://127.0.0.1:$port_full/player_api.php?username=limited&password=pw&action=get_live_streams")"
grep -q '"stream_id":"ch1"' <<<"$limited_live" || fail "limited xtream live view missing allowed channel"
! grep -q '"stream_id":"ch2"' <<<"$limited_live" || fail "limited xtream live view leaked disallowed channel"
limited_denied_code="$(curl -sS -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_full/live/limited/pw/ch2.ts" || true)"
[[ "$limited_denied_code" == "404" ]] || fail "limited xtream live proxy status=$limited_denied_code body=$(cat "$body_file" 2>/dev/null)"
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

kill "$full_pid" 2>/dev/null || true
wait "$full_pid" 2>/dev/null || true
port_restart="$(pick_port)"
run_serve "$TMP_DIR/catalog-full-shuffled.json" "$port_restart"
wait_http_code "http://127.0.0.1:$port_restart/discover.json" "200" || fail "restart catalog discover.json not ready"
grep -q '"curated_channels": 2' <(curl -sS "http://127.0.0.1:$port_restart/programming/preview.json") || fail "restart preview missing curated count"
grep -q '"order_mode": "custom"' <(curl -sS "http://127.0.0.1:$port_restart/programming/order.json") || fail "restart order mode did not persist"
grep -q '"collapse_backups": true' <(curl -sS "http://127.0.0.1:$port_restart/programming/order.json") || fail "restart collapse flag missing"
grep -q '"GuideNumber":"102"' <(curl -sS "http://127.0.0.1:$port_restart/lineup.json") || fail "restart lineup missing pinned sports row"
grep -q '"GuideNumber":"1001"' <(curl -sS "http://127.0.0.1:$port_restart/lineup.json") || fail "restart lineup missing directv backup row"

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
head_headers="$TMP_DIR/movie.head"
head_code="$(curl -sS -I -D "$head_headers" -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port_vod/Movies/Live:%20Smoke%20Movie%20%282024%29/Live:%20Smoke%20Movie%20%282024%29.mp4" || true)"
[[ "$head_code" == "200" ]] || fail "vod-webdav movie HEAD status=$head_code"
grep -qi '^Accept-Ranges: bytes' "$head_headers" || fail "vod-webdav movie HEAD missing Accept-Ranges"
range_body="$TMP_DIR/movie.range"
range_code="$(curl -sS -H 'Range: bytes=0-4' -o "$range_body" -w '%{http_code}' "http://127.0.0.1:$port_vod/Movies/Live:%20Smoke%20Movie%20%282024%29/Live:%20Smoke%20Movie%20%282024%29.mp4" || true)"
[[ "$range_code" == "206" ]] || fail "vod-webdav movie range status=$range_code body=$(cat "$range_body" 2>/dev/null)"
grep -q '^movie$' "$range_body" || fail "vod-webdav movie range body unexpected"
episode_headers="$TMP_DIR/episode.head"
episode_code="$(curl -sS -I -D "$episode_headers" -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port_vod/TV/Live:%20Smoke%20Show%20%282023%29/Season%2001/Live:%20Smoke%20Show%20%282023%29%20-%20s01e01%20-%20Pilot.mp4" || true)"
[[ "$episode_code" == "200" ]] || fail "vod-webdav episode HEAD status=$episode_code"
grep -qi '^Accept-Ranges: bytes' "$episode_headers" || fail "vod-webdav episode HEAD missing Accept-Ranges"
episode_range="$TMP_DIR/episode.range"
episode_range_code="$(curl -sS -H 'Range: bytes=0-6' -o "$episode_range" -w '%{http_code}' "http://127.0.0.1:$port_vod/TV/Live:%20Smoke%20Show%20%282023%29/Season%2001/Live:%20Smoke%20Show%20%282023%29%20-%20s01e01%20-%20Pilot.mp4" || true)"
[[ "$episode_range_code" == "206" ]] || fail "vod-webdav episode range status=$episode_range_code body=$(cat "$episode_range" 2>/dev/null)"
grep -q '^episode$' "$episode_range" || fail "vod-webdav episode range body unexpected"
readonly_code="$(curl -sS -X PUT -H 'Content-Type: application/octet-stream' --data 'bad' -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_vod/Movies/Live:%20Smoke%20Movie%20%282024%29/Live:%20Smoke%20Movie%20%282024%29.mp4" || true)"
[[ "$readonly_code" == "405" ]] || fail "vod-webdav readonly PUT status=$readonly_code body=$(cat "$body_file" 2>/dev/null)"

port_xtream="$(pick_port)"
run_serve "$TMP_DIR/catalog-vod.json" "$port_xtream"
wait_http_code "http://127.0.0.1:$port_xtream/discover.json" "200" || fail "xtream catalog discover.json not ready"
grep -q '"stream_type":"movie"' <(curl -sS "http://127.0.0.1:$port_xtream/player_api.php?username=demo&password=secret&action=get_vod_streams") || fail "xtream vod streams endpoint missing movie row"
grep -q '"stream_type":"series"' <(curl -sS "http://127.0.0.1:$port_xtream/player_api.php?username=demo&password=secret&action=get_series") || fail "xtream series endpoint missing series row"
grep -q '"episodes":{"1":\[' <(curl -sS "http://127.0.0.1:$port_xtream/player_api.php?username=demo&password=secret&action=get_series_info&series_id=s1") || fail "xtream series info missing episode list"
movie_proxy_body="$(curl -sS "http://127.0.0.1:$port_xtream/movie/demo/secret/m1.mp4")"
[[ "$movie_proxy_body" == "movie-bytes" ]] || fail "xtream movie proxy body unexpected"
episode_proxy_body="$(curl -sS "http://127.0.0.1:$port_xtream/series/demo/secret/e1.mp4")"
[[ "$episode_proxy_body" == "episode-bytes" ]] || fail "xtream series proxy body unexpected"
limited_movie_body="$(curl -sS "http://127.0.0.1:$port_xtream/movie/limited/pw/m1.mp4")"
[[ "$limited_movie_body" == "movie-bytes" ]] || fail "limited xtream movie proxy body unexpected"
limited_series_code="$(curl -sS -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_xtream/series/limited/pw/e1.mp4" || true)"
[[ "$limited_series_code" == "404" ]] || fail "limited xtream series proxy status=$limited_series_code body=$(cat "$body_file" 2>/dev/null)"

log "smoke checks passed"
