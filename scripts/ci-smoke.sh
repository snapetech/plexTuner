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

assert_file_prefix() {
  local path="$1" prefix="$2"
  python3 - "$path" "$prefix" <<'PY'
import pathlib
import sys

data = pathlib.Path(sys.argv[1]).read_bytes()
prefix = sys.argv[2].encode()
if not data.startswith(prefix):
    raise SystemExit(f"{sys.argv[1]} missing prefix {sys.argv[2]!r}: got {data[:32]!r}")
PY
}

PIDS=()

# The smoke fixtures intentionally use tiny finite HLS playlists. Production
# retry settings make sense for real providers, but they turn these fixtures into
# multi-minute waits after the first bytes are proven.
export IPTV_TUNERR_UPSTREAM_RETRY_LIMIT="${IPTV_TUNERR_UPSTREAM_RETRY_LIMIT:-0}"

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

cat >"$TMP_DIR/catalog-shared.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": [
    {
      "channel_id": "shared1",
      "guide_number": "111",
      "guide_name": "Shared Relay",
      "group_title": "News",
      "stream_url": "REPLACE_SHARED_HLS_URL",
      "stream_urls": ["REPLACE_SHARED_HLS_URL"],
      "epg_linked": true,
      "tvg_id": "shared.relay"
    }
  ]
}
JSON

cat >"$TMP_DIR/catalog-accounts.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": [
    {
      "channel_id": "acct1",
      "guide_number": "211",
      "guide_name": "Account Relay 1",
      "group_title": "News",
      "stream_urls": [
        "REPLACE_ACCOUNT_U1_CH1",
        "REPLACE_ACCOUNT_U2_CH1",
        "REPLACE_ACCOUNT_U3_CH1"
      ],
      "epg_linked": true,
      "tvg_id": "account.relay.1"
    },
    {
      "channel_id": "acct2",
      "guide_number": "212",
      "guide_name": "Account Relay 2",
      "group_title": "News",
      "stream_urls": [
        "REPLACE_ACCOUNT_U1_CH2",
        "REPLACE_ACCOUNT_U2_CH2",
        "REPLACE_ACCOUNT_U3_CH2"
      ],
      "epg_linked": true,
      "tvg_id": "account.relay.2"
    },
    {
      "channel_id": "acct3",
      "guide_number": "213",
      "guide_name": "Account Relay 3",
      "group_title": "News",
      "stream_urls": [
        "REPLACE_ACCOUNT_U1_CH3",
        "REPLACE_ACCOUNT_U2_CH3",
        "REPLACE_ACCOUNT_U3_CH3"
      ],
      "epg_linked": true,
      "tvg_id": "account.relay.3"
    }
  ]
}
JSON

cat >"$TMP_DIR/catalog-remux.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": [
    {
      "channel_id": "remux1",
      "guide_number": "311",
      "guide_name": "Remux Fallback",
      "group_title": "News",
      "stream_url": "REPLACE_REMUX_HLS_URL",
      "stream_urls": ["REPLACE_REMUX_HLS_URL"],
      "epg_linked": true,
      "tvg_id": "remux.fallback"
    }
  ]
}
JSON

cat >"$TMP_DIR/stream-profiles.json" <<'JSON'
{
  "shared-hls": {
    "base_profile": "dashfast",
    "transcode": true,
    "output_mux": "hls",
    "description": "binary smoke packaged HLS shared-session profile"
  },
  "shared-fmp4": {
    "base_profile": "lowbitrate",
    "transcode": true,
    "output_mux": "fmp4",
    "description": "binary smoke ffmpeg fMP4 shared-session profile"
  }
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

run_with_webui() {
  local catalog="$1" port="$2" webui_port="$3"
  IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
  IPTV_TUNERR_XMLTV_URL= \
  IPTV_TUNERR_WEBUI_DISABLED=0 \
  IPTV_TUNERR_WEBUI_PORT="$webui_port" \
  IPTV_TUNERR_WEBUI_USER=deck \
  IPTV_TUNERR_WEBUI_PASS=secret \
  IPTV_TUNERR_XTREAM_USER=demo \
  IPTV_TUNERR_XTREAM_PASS=secret \
  IPTV_TUNERR_XTREAM_USERS_FILE="$TMP_DIR/xtream-users.json" \
  IPTV_TUNERR_PROGRAMMING_RECIPE_FILE="$TMP_DIR/programming.json" \
  IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE="$TMP_DIR/lineup-harvest.json" \
  IPTV_TUNERR_VIRTUAL_CHANNELS_FILE="$TMP_DIR/virtual-channels.json" \
  IPTV_TUNERR_RECORDING_RULES_FILE="$TMP_DIR/recording-rules.json" \
  IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE="$TMP_DIR/recorder-state.json" \
  "$BIN" run -catalog "$catalog" -addr ":$port" -base-url "http://127.0.0.1:$port" -skip-index -skip-health \
    >"$TMP_DIR/run-webui-$port.log" 2>&1 &
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

run_slow_hls_server() {
  local port="$1"
  HLS_PORT="$port" HLS_ROOT="$TMP_DIR" python3 - <<'PY' >"$TMP_DIR/slow-hls-$port.log" 2>&1 &
import os
import socketserver
import time
from http.server import BaseHTTPRequestHandler
from urllib.parse import urlparse

PORT = int(os.environ["HLS_PORT"])

SEGMENT = bytes([0x47]) + b"\x00" * 187
SEGMENT = SEGMENT * 2000

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        parsed = urlparse(self.path)
        path = parsed.path
        if path.endswith(".m3u8"):
            base = path.rsplit("/", 1)[0]
            body = (
                "#EXTM3U\n"
                "#EXT-X-VERSION:3\n"
                "#EXT-X-TARGETDURATION:2\n"
                "#EXT-X-MEDIA-SEQUENCE:1\n"
                "#EXTINF:2.0,\n"
                f"{base}/seg1.ts\n"
                "#EXTINF:2.0,\n"
                f"{base}/seg2.ts\n"
            ).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/x-mpegURL")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if path.endswith("/seg1.ts") or path.endswith("/seg2.ts"):
            self.send_response(200)
            self.send_header("Content-Type", "video/mp2t")
            self.send_header("Content-Length", str(len(SEGMENT)))
            self.end_headers()
            chunk = 188 * 50
            for i in range(0, len(SEGMENT), chunk):
                self.wfile.write(SEGMENT[i:i+chunk])
                self.wfile.flush()
                time.sleep(0.02)
            return
        self.send_response(404)
        self.end_headers()

    def log_message(self, fmt, *args):
        return

with socketserver.TCPServer(("127.0.0.1", PORT), Handler) as httpd:
    httpd.serve_forever()
PY
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

port_hls="$(pick_port)"
run_slow_hls_server "$port_hls"
wait_http_code "http://127.0.0.1:$port_hls/shared.m3u8" "200" || fail "slow hls server not ready"
sed -i "s|REPLACE_SHARED_HLS_URL|http://127.0.0.1:$port_hls/shared.m3u8|g" "$TMP_DIR/catalog-shared.json"
sed -i "s|REPLACE_ACCOUNT_U1_CH1|http://127.0.0.1:$port_hls/live/alpha01/pass1/ch1.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_ACCOUNT_U2_CH1|http://127.0.0.1:$port_hls/live/bravo02/pass2/ch1.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_ACCOUNT_U3_CH1|http://127.0.0.1:$port_hls/live/charly3/pass3/ch1.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_ACCOUNT_U1_CH2|http://127.0.0.1:$port_hls/live/alpha01/pass1/ch2.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_ACCOUNT_U2_CH2|http://127.0.0.1:$port_hls/live/bravo02/pass2/ch2.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_ACCOUNT_U3_CH2|http://127.0.0.1:$port_hls/live/charly3/pass3/ch2.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_ACCOUNT_U1_CH3|http://127.0.0.1:$port_hls/live/alpha01/pass1/ch3.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_ACCOUNT_U2_CH3|http://127.0.0.1:$port_hls/live/bravo02/pass2/ch3.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_ACCOUNT_U3_CH3|http://127.0.0.1:$port_hls/live/charly3/pass3/ch3.m3u8|g" "$TMP_DIR/catalog-accounts.json"
sed -i "s|REPLACE_REMUX_HLS_URL|http://127.0.0.1:$port_hls/remux/channel1.m3u8|g" "$TMP_DIR/catalog-remux.json"

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
grep -q '"name": "diagnostics_capture"' <(curl -sS "http://127.0.0.1:$port_full/ops/workflows/diagnostics.json") || fail "diagnostics workflow missing name"
grep -q '"resolved_name": "Movie One"' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/preview.json?per_channel=2") || fail "virtual channel preview missing movie slot"
grep -q '"slots": \[' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/schedule.json?horizon=3h") || fail "virtual channel schedule missing slots"
grep -q '"resolved_now"' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/channel-detail.json?channel_id=vc-news&limit=2&horizon=3h") || fail "virtual channel detail missing resolved_now"
grep -q '<channel id="virtual.vc-news">' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/guide.xml?horizon=3h") || fail "virtual channel guide missing channel id"
grep -q '/virtual-channels/stream/vc-news.mp4' <(curl -sS "http://127.0.0.1:$port_full/virtual-channels/live.m3u") || fail "virtual channel m3u missing stream url"
virtual_stream_body="$(curl -sS "http://127.0.0.1:$port_full/virtual-channels/stream/vc-news.mp4")"
[[ "$virtual_stream_body" == "movie-bytes" || "$virtual_stream_body" == "episode-bytes" ]] || fail "virtual channel stream missing playable bytes"
grep -q '"alternative_sources"' <(curl -sS "http://127.0.0.1:$port_full/programming/channel-detail.json?channel_id=ch1") || fail "programming channel detail missing alternatives section"
evidence_code="$(curl -sS -X POST -H 'Content-Type: application/json' --data '{"case_id":"smoke-case"}' -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_full/ops/actions/evidence-intake-start" || true)"
[[ "$evidence_code" == "200" ]] || fail "evidence intake action status=$evidence_code body=$(cat "$body_file" 2>/dev/null)"
[[ -f ".diag/evidence/smoke-case/notes.md" ]] || fail "evidence intake action did not create notes.md"
grep -q '"stream_type":"live"' <(curl -sS "http://127.0.0.1:$port_full/player_api.php?username=demo&password=secret&action=get_live_streams") || fail "xtream live streams endpoint missing live row"
grep -q '#EXTM3U url-tvg="http://127.0.0.1:'"$port_full"'/xmltv.php?username=demo&password=secret"' <(curl -sS "http://127.0.0.1:$port_full/get.php?username=demo&password=secret&type=m3u_plus&output=ts") || fail "xtream get.php missing xmltv url"
grep -q '<channel id="ch1">' <(curl -sS "http://127.0.0.1:$port_full/xmltv.php?username=demo&password=secret") || fail "xtream xmltv export missing entitled channel"
limited_live="$(curl -sS "http://127.0.0.1:$port_full/player_api.php?username=limited&password=pw&action=get_live_streams")"
grep -q '"stream_id":"ch1"' <<<"$limited_live" || fail "limited xtream live view missing allowed channel"
! grep -q '"stream_id":"ch2"' <<<"$limited_live" || fail "limited xtream live view leaked disallowed channel"
limited_m3u="$(curl -sS "http://127.0.0.1:$port_full/get.php?username=limited&password=pw&type=m3u_plus&output=ts")"
grep -q '/live/limited/pw/ch1.ts' <<<"$limited_m3u" || fail "limited xtream get.php missing allowed channel"
! grep -q '/live/limited/pw/ch2.ts' <<<"$limited_m3u" || fail "limited xtream get.php leaked denied channel"
limited_xmltv="$(curl -sS "http://127.0.0.1:$port_full/xmltv.php?username=limited&password=pw")"
grep -q '<channel id="ch1">' <<<"$limited_xmltv" || fail "limited xtream xmltv missing allowed channel"
! grep -q '<channel id="ch2">' <<<"$limited_xmltv" || fail "limited xtream xmltv leaked denied channel"
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
backup_prefer_code="$(curl -sS -X POST -H 'Content-Type: application/json' --data '{"action":"prefer","channel_id":"ch3"}' -o "$body_file" -w '%{http_code}' "http://127.0.0.1:$port_full/programming/backups.json" || true)"
[[ "$backup_prefer_code" == "200" ]] || fail "programming backup prefer status=$backup_prefer_code body=$(cat "$body_file" 2>/dev/null)"
grep -q '"primary_channel_id": "ch3"' <(curl -sS "http://127.0.0.1:$port_full/programming/backups.json") || fail "programming backups missing preferred primary"
grep -q '"channel_id": "ch3"' <(curl -sS "http://127.0.0.1:$port_full/programming/preview.json") || fail "programming preview missing preferred backup primary"

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

port_webui_tuner="$(pick_port)"
port_webui="$(pick_port)"
run_with_webui "$TMP_DIR/catalog-full.json" "$port_webui_tuner" "$port_webui"
wait_http_code "http://127.0.0.1:$port_webui_tuner/discover.json" "200" || fail "webui run discover.json not ready"
wait_http_code "http://127.0.0.1:$port_webui/login" "200" || fail "webui login not ready"
login_headers="$TMP_DIR/webui-login.headers"
login_code="$(curl -sS -D "$login_headers" -o /dev/null -w '%{http_code}' -X POST \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data 'username=deck&password=secret' \
  "http://127.0.0.1:$port_webui/login" || true)"
[[ "$login_code" == "303" ]] || fail "webui login status=$login_code"
cookie="$(awk '/^Set-Cookie:/ { sub(/\r$/, "", $0); split($2, parts, ";"); print parts[1]; exit }' "$login_headers")"
[[ -n "$cookie" ]] || fail "webui login missing session cookie"
home_html="$(curl -sS -H "Cookie: $cookie" "http://127.0.0.1:$port_webui/")"
assert_contains "$home_html" "deck-bootstrap" "webui home bootstrap"
csrf_token="$(HTML="$home_html" python3 -c 'import json, os, re; html=os.environ["HTML"]; m=re.search(r"<script id=\"deck-bootstrap\" type=\"application/json\">(.*?)</script>", html, re.S); assert m; print(json.loads(m.group(1)).get("csrfToken",""))')"
[[ -n "$csrf_token" ]] || fail "webui home missing csrf token"
runtime_body="$(curl -sS -H "Cookie: $cookie" "http://127.0.0.1:$port_webui/api/debug/runtime.json")"
assert_contains "$runtime_body" '"webui"' "webui runtime proxy"
settings_code="$(curl -sS -o "$body_file" -w '%{http_code}' -X POST \
  -H "Cookie: $cookie" \
  -H "Content-Type: application/json" \
  -H "X-IPTVTunerr-Deck-CSRF: $csrf_token" \
  --data '{"default_refresh_sec":45}' \
  "http://127.0.0.1:$port_webui/deck/settings.json" || true)"
[[ "$settings_code" == "200" ]] || fail "webui settings save status=$settings_code body=$(cat "$body_file" 2>/dev/null)"
replay_code="$(curl -sS -o "$body_file" -w '%{http_code}' -X POST \
  -H "Cookie: $cookie" \
  -H "Content-Type: application/json" \
  -H "X-IPTVTunerr-Deck-CSRF: $csrf_token" \
  --data '{"shared_relay_replay_bytes":131072}' \
  "http://127.0.0.1:$port_webui/api/ops/actions/shared-relay-replay" || true)"
[[ "$replay_code" == "200" ]] || fail "shared replay update status=$replay_code body=$(cat "$body_file" 2>/dev/null)"
runtime_body="$(curl -sS -H "Cookie: $cookie" "http://127.0.0.1:$port_webui/api/debug/runtime.json")"
assert_contains "$runtime_body" '"shared_relay_replay_bytes": "131072"' "webui runtime replay update"
action_status_body="$(curl -sS -H "Cookie: $cookie" "http://127.0.0.1:$port_webui/api/ops/actions/status.json")"
assert_contains "$action_status_body" '"/ops/actions/shared-relay-replay"' "webui action status replay endpoint"
diagnostics_body="$(curl -sS -H "Cookie: $cookie" "http://127.0.0.1:$port_webui/api/ops/workflows/diagnostics.json")"
assert_contains "$diagnostics_body" '"suggested_good_channel_id"' "webui diagnostics workflow"

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

port_shared="$(pick_port)"
IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
IPTV_TUNERR_XMLTV_URL= \
IPTV_TUNERR_WEBUI_DISABLED=1 \
IPTV_TUNERR_FFMPEG_DISABLED=1 \
IPTV_TUNERR_XTREAM_USER=demo \
IPTV_TUNERR_XTREAM_PASS=secret \
IPTV_TUNERR_PROGRAMMING_RECIPE_FILE="$TMP_DIR/programming.json" \
"$BIN" serve -catalog "$TMP_DIR/catalog-shared.json" -addr ":$port_shared" -base-url "http://127.0.0.1:$port_shared" \
  >"$TMP_DIR/serve-shared-$port_shared.log" 2>&1 &
PIDS+=("$!")
wait_http_code "http://127.0.0.1:$port_shared/discover.json" "200" || fail "shared relay discover.json not ready"
curl -sS "http://127.0.0.1:$port_shared/stream/shared1" -o "$TMP_DIR/shared-first.out" &
first_stream_pid=$!
sleep 0.25
shared_headers="$TMP_DIR/shared-second.headers"
curl -sS -D "$shared_headers" "http://127.0.0.1:$port_shared/stream/shared1" -o "$TMP_DIR/shared-second.out" &
second_stream_pid=$!
sleep 0.25
grep -q '"count": 1' <(curl -sS "http://127.0.0.1:$port_shared/debug/shared-relays.json") || fail "shared relay report missing active relay"
grep -q '"subscriber_count": 1' <(curl -sS "http://127.0.0.1:$port_shared/debug/shared-relays.json") || fail "shared relay report missing joined subscriber"
wait "$first_stream_pid"
wait "$second_stream_pid"
grep -qi '^X-IptvTunerr-Shared-Upstream: hls_go' "$shared_headers" || fail "shared relay second consumer missing shared upstream header"
[[ -s "$TMP_DIR/shared-first.out" ]] || fail "shared relay first consumer got no bytes"
[[ -s "$TMP_DIR/shared-second.out" ]] || fail "shared relay second consumer got no bytes"

fake_packager_ffmpeg="$TMP_DIR/fake-packager-ffmpeg.sh"
cat >"$fake_packager_ffmpeg" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
playlist=""
segpat=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "seg" ]]; then
    segpat="$arg"
    prev=""
    continue
  fi
  if [[ "$arg" == "-hls_segment_filename" ]]; then
    prev="seg"
    continue
  fi
  playlist="$arg"
done
mkdir -p "$(dirname "$playlist")"
printf '#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\nseg-000001.ts\n' >"$playlist"
segfile="$(printf "$segpat" 1)"
printf 'segment-bytes' >"$segfile"
sleep 5
SH
chmod +x "$fake_packager_ffmpeg"

port_packager="$(pick_port)"
IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
IPTV_TUNERR_XMLTV_URL= \
IPTV_TUNERR_WEBUI_DISABLED=1 \
IPTV_TUNERR_FFMPEG_DISABLED=0 \
IPTV_TUNERR_FFMPEG_PATH="$fake_packager_ffmpeg" \
IPTV_TUNERR_STREAM_PROFILES_FILE="$TMP_DIR/stream-profiles.json" \
IPTV_TUNERR_TUNER_COUNT=1 \
"$BIN" serve -catalog "$TMP_DIR/catalog-remux.json" -addr ":$port_packager" -base-url "http://127.0.0.1:$port_packager" \
  >"$TMP_DIR/serve-packager-$port_packager.log" 2>&1 &
PIDS+=("$!")
wait_http_code "http://127.0.0.1:$port_packager/discover.json" "200" || fail "packaged hls discover.json not ready"
packager_first="$TMP_DIR/packager-first.m3u8"
curl -sS "http://127.0.0.1:$port_packager/stream/remux1?profile=shared-hls" -o "$packager_first"
packager_second="$TMP_DIR/packager-second.m3u8"
packager_second_headers="$TMP_DIR/packager-second.headers"
curl -sS -D "$packager_second_headers" "http://127.0.0.1:$port_packager/stream/remux1?profile=shared-hls" -o "$packager_second"
grep -qi '^X-IptvTunerr-Shared-Upstream: ffmpeg_hls_packager' "$packager_second_headers" || fail "packaged hls second consumer missing shared upstream header"
python3 - "$packager_first" "$packager_second" <<'PY'
import pathlib
import sys
from urllib.parse import parse_qs, urlparse

def last_sid(path: str) -> str:
    text = pathlib.Path(path).read_text()
    lines = [line.strip() for line in text.splitlines() if line.strip() and not line.startswith("#")]
    if not lines:
        raise SystemExit(f"no playlist URL lines in {path}")
    return parse_qs(urlparse(lines[-1]).query).get("sid", [""])[0]

sid1 = last_sid(sys.argv[1])
sid2 = last_sid(sys.argv[2])
if not sid1 or not sid2:
    raise SystemExit(f"missing sid sid1={sid1!r} sid2={sid2!r}")
if sid1 != sid2:
    raise SystemExit(f"expected shared packaged sid, got {sid1!r} vs {sid2!r}")
PY
packager_seg_rel="$(python3 - "$packager_second" <<'PY'
import pathlib
import sys

text = pathlib.Path(sys.argv[1]).read_text()
lines = [line.strip() for line in text.splitlines() if line.strip() and not line.startswith("#")]
if not lines:
    raise SystemExit("no playlist URL lines")
print(lines[-1])
PY
)"
packager_seg_body="$(curl -sS "http://127.0.0.1:$port_packager$packager_seg_rel")"
[[ "$packager_seg_body" == "segment-bytes" ]] || fail "packaged hls segment body unexpected"

fake_shared_ffmpeg="$TMP_DIR/fake-shared-ffmpeg.sh"
cat >"$fake_shared_ffmpeg" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'shared-'
sleep 1
printf 'ffmpeg'
SH
chmod +x "$fake_shared_ffmpeg"

port_ffmpeg_shared="$(pick_port)"
IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
IPTV_TUNERR_XMLTV_URL= \
IPTV_TUNERR_WEBUI_DISABLED=1 \
IPTV_TUNERR_FFMPEG_DISABLED=0 \
IPTV_TUNERR_FFMPEG_PATH="$fake_shared_ffmpeg" \
IPTV_TUNERR_TUNER_COUNT=1 \
"$BIN" serve -catalog "$TMP_DIR/catalog-remux.json" -addr ":$port_ffmpeg_shared" -base-url "http://127.0.0.1:$port_ffmpeg_shared" \
  >"$TMP_DIR/serve-ffmpeg-shared-$port_ffmpeg_shared.log" 2>&1 &
PIDS+=("$!")
wait_http_code "http://127.0.0.1:$port_ffmpeg_shared/discover.json" "200" || fail "ffmpeg shared discover.json not ready"
curl -sS "http://127.0.0.1:$port_ffmpeg_shared/stream/remux1" -o "$TMP_DIR/ffmpeg-shared-first.out" &
ffmpeg_shared_first_pid=$!
sleep 0.25
ffmpeg_shared_headers="$TMP_DIR/ffmpeg-shared-second.headers"
curl -sS -D "$ffmpeg_shared_headers" "http://127.0.0.1:$port_ffmpeg_shared/stream/remux1" -o "$TMP_DIR/ffmpeg-shared-second.out" &
ffmpeg_shared_second_pid=$!
sleep 0.25
grep -q '"count": 1' <(curl -sS "http://127.0.0.1:$port_ffmpeg_shared/debug/shared-relays.json") || fail "ffmpeg shared relay report missing active relay"
grep -q '"shared_upstream": "hls_ffmpeg"' <(curl -sS "http://127.0.0.1:$port_ffmpeg_shared/debug/shared-relays.json") || fail "ffmpeg shared relay report missing upstream label"
wait "$ffmpeg_shared_first_pid"
wait "$ffmpeg_shared_second_pid"
grep -qi '^X-IptvTunerr-Shared-Upstream: hls_ffmpeg' "$ffmpeg_shared_headers" || fail "ffmpeg shared second consumer missing shared upstream header"
[[ -s "$TMP_DIR/ffmpeg-shared-first.out" ]] || fail "ffmpeg shared first consumer got no bytes"
[[ -s "$TMP_DIR/ffmpeg-shared-second.out" ]] || fail "ffmpeg shared second consumer got no bytes"
assert_file_prefix "$TMP_DIR/ffmpeg-shared-second.out" "shared-"

port_ffmpeg_fmp4="$(pick_port)"
IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
IPTV_TUNERR_XMLTV_URL= \
IPTV_TUNERR_WEBUI_DISABLED=1 \
IPTV_TUNERR_FFMPEG_DISABLED=0 \
IPTV_TUNERR_FFMPEG_PATH="$fake_shared_ffmpeg" \
IPTV_TUNERR_STREAM_PROFILES_FILE="$TMP_DIR/stream-profiles.json" \
IPTV_TUNERR_TUNER_COUNT=1 \
"$BIN" serve -catalog "$TMP_DIR/catalog-remux.json" -addr ":$port_ffmpeg_fmp4" -base-url "http://127.0.0.1:$port_ffmpeg_fmp4" \
  >"$TMP_DIR/serve-ffmpeg-fmp4-$port_ffmpeg_fmp4.log" 2>&1 &
PIDS+=("$!")
wait_http_code "http://127.0.0.1:$port_ffmpeg_fmp4/discover.json" "200" || fail "ffmpeg fmp4 shared discover.json not ready"
curl -sS "http://127.0.0.1:$port_ffmpeg_fmp4/stream/remux1?profile=shared-fmp4" -o "$TMP_DIR/ffmpeg-fmp4-first.out" &
ffmpeg_fmp4_first_pid=$!
sleep 0.25
ffmpeg_fmp4_headers="$TMP_DIR/ffmpeg-fmp4-second.headers"
curl -sS -D "$ffmpeg_fmp4_headers" "http://127.0.0.1:$port_ffmpeg_fmp4/stream/remux1?profile=shared-fmp4" -o "$TMP_DIR/ffmpeg-fmp4-second.out" &
ffmpeg_fmp4_second_pid=$!
sleep 0.25
grep -q '"content_type": "video/mp4"' <(curl -sS "http://127.0.0.1:$port_ffmpeg_fmp4/debug/shared-relays.json") || fail "ffmpeg fmp4 shared relay report missing mp4 content type"
wait "$ffmpeg_fmp4_first_pid"
wait "$ffmpeg_fmp4_second_pid"
grep -qi '^X-IptvTunerr-Shared-Upstream: hls_ffmpeg' "$ffmpeg_fmp4_headers" || fail "ffmpeg fmp4 second consumer missing shared upstream header"
grep -qi '^Content-Type: video/mp4' "$ffmpeg_fmp4_headers" || fail "ffmpeg fmp4 second consumer missing video/mp4 content type"
[[ -s "$TMP_DIR/ffmpeg-fmp4-first.out" ]] || fail "ffmpeg fmp4 first consumer got no bytes"
[[ -s "$TMP_DIR/ffmpeg-fmp4-second.out" ]] || fail "ffmpeg fmp4 second consumer got no bytes"
assert_file_prefix "$TMP_DIR/ffmpeg-fmp4-second.out" "shared-"

port_accounts="$(pick_port)"
IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
IPTV_TUNERR_XMLTV_URL= \
IPTV_TUNERR_WEBUI_DISABLED=1 \
IPTV_TUNERR_FFMPEG_DISABLED=1 \
IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT=1 \
IPTV_TUNERR_TUNER_COUNT=4 \
"$BIN" serve -catalog "$TMP_DIR/catalog-accounts.json" -addr ":$port_accounts" -base-url "http://127.0.0.1:$port_accounts" \
  >"$TMP_DIR/serve-accounts-$port_accounts.log" 2>&1 &
PIDS+=("$!")
wait_http_code "http://127.0.0.1:$port_accounts/discover.json" "200" || fail "account pool discover.json not ready"
curl -sS "http://127.0.0.1:$port_accounts/stream/acct1" -o "$TMP_DIR/account-first.out" &
account_stream_1=$!
sleep 0.15
curl -sS "http://127.0.0.1:$port_accounts/stream/acct2" -o "$TMP_DIR/account-second.out" &
account_stream_2=$!
sleep 0.15
curl -sS "http://127.0.0.1:$port_accounts/stream/acct3" -o "$TMP_DIR/account-third.out" &
account_stream_3=$!
sleep 0.3
python3 - "$port_accounts" <<'PY'
import json
import sys
from urllib.request import urlopen

port = sys.argv[1]
with urlopen(f"http://127.0.0.1:{port}/provider/profile.json", timeout=5) as resp:
    profile = json.load(resp)
leases = profile.get("account_leases", [])
if len(leases) != 3:
    raise SystemExit(f"expected 3 account leases, got {leases!r}")
labels = {lease.get("label") for lease in leases}
if len(labels) != 3:
    raise SystemExit(f"expected 3 distinct account labels, got {leases!r}")
if any(int(lease.get("in_use", 0)) != 1 for lease in leases):
    raise SystemExit(f"expected each account lease to be in_use=1, got {leases!r}")
PY
wait "$account_stream_1"
wait "$account_stream_2"
wait "$account_stream_3"
[[ -s "$TMP_DIR/account-first.out" ]] || fail "account pool first consumer got no bytes"
[[ -s "$TMP_DIR/account-second.out" ]] || fail "account pool second consumer got no bytes"
[[ -s "$TMP_DIR/account-third.out" ]] || fail "account pool third consumer got no bytes"

fake_ffmpeg="$TMP_DIR/fake-ffmpeg.sh"
cat >"$fake_ffmpeg" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
exec sleep 5
SH
chmod +x "$fake_ffmpeg"

port_remux="$(pick_port)"
IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
IPTV_TUNERR_XMLTV_URL= \
IPTV_TUNERR_WEBUI_DISABLED=1 \
IPTV_TUNERR_FFMPEG_DISABLED=0 \
IPTV_TUNERR_FFMPEG_PATH="$fake_ffmpeg" \
IPTV_TUNERR_FFMPEG_HLS_FIRST_BYTES_TIMEOUT_MS=100 \
"$BIN" serve -catalog "$TMP_DIR/catalog-remux.json" -addr ":$port_remux" -base-url "http://127.0.0.1:$port_remux" \
  >"$TMP_DIR/serve-remux-$port_remux.log" 2>&1 &
PIDS+=("$!")
wait_http_code "http://127.0.0.1:$port_remux/discover.json" "200" || fail "remux fallback discover.json not ready"
curl -sS "http://127.0.0.1:$port_remux/stream/remux1" -o "$TMP_DIR/remux-fallback.out"
[[ -s "$TMP_DIR/remux-fallback.out" ]] || fail "remux fallback produced no bytes"
python3 - "$port_remux" <<'PY'
import json
import sys
from urllib.request import urlopen

port = sys.argv[1]
with urlopen(f"http://127.0.0.1:{port}/debug/stream-attempts.json?limit=1", timeout=5) as resp:
    report = json.load(resp)
attempts = report.get("attempts", [])
if not attempts:
    raise SystemExit("expected at least one stream attempt")
attempt = attempts[0]
if attempt.get("channel_id") != "remux1":
    raise SystemExit(f"unexpected attempt payload {attempt!r}")
if attempt.get("final_mode") != "hls_go":
    raise SystemExit(f"expected final_mode=hls_go, got {attempt!r}")
if attempt.get("final_status") != "ok":
    raise SystemExit(f"expected final_status=ok, got {attempt!r}")
PY

log "smoke checks passed"
