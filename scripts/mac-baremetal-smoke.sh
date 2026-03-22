#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MAC_HOST="${MAC_HOST:-192.168.50.108}"
MAC_USER="${MAC_USER:-keith}"
MAC_SSH_KEY="${MAC_SSH_KEY:-$HOME/.ssh/id_ed25519}"
MAC_WOL_BROADCAST="${MAC_WOL_BROADCAST:-192.168.50.255}"
MAC_WOL_PORT="${MAC_WOL_PORT:-9}"
MAC_WOL_MACS="${MAC_WOL_MACS:-ae:8d:76:ae:19:e8,1c:f6:4c:a1:39:1d,f6:88:d8:da:12:47,f6:88:d8:da:12:48}"
WAKE_MACBOOK="${WAKE_MACBOOK:-true}"
WAKE_WAIT_SEC="${WAKE_WAIT_SEC:-90}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
OUT_ROOT="${OUT_ROOT:-$ROOT/.diag/mac-baremetal}"
OUT_DIR="$OUT_ROOT/$RUN_ID"
TMP_DIR="$(mktemp -d)"
REMOTE_ROOT="/Users/${MAC_USER}/iptvtunerr-mac-smoke-${RUN_ID}"

SSH_OPTS=(
  -i "$MAC_SSH_KEY"
  -o BatchMode=yes
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
)

PIDS=()

log() { printf '[mac-baremetal-smoke] %s\n' "$*"; }
fail() { printf '[mac-baremetal-smoke] ERROR: %s\n' "$*" >&2; exit 1; }

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

ssh_ok() {
  ssh "${SSH_OPTS[@]}" "${MAC_USER}@${MAC_HOST}" 'printf ready' >/dev/null 2>&1
}

send_magic_packets() {
  local macs_csv="$1"
  python3 - "$macs_csv" "$MAC_WOL_BROADCAST" "$MAC_WOL_PORT" <<'PY'
import socket
import sys
import time

macs = [m.strip() for m in sys.argv[1].split(",") if m.strip()]
broadcast = sys.argv[2]
port = int(sys.argv[3])

def packet(mac: str) -> bytes:
    cleaned = mac.replace(":", "").replace("-", "").lower()
    if len(cleaned) != 12:
        raise SystemExit(f"bad mac: {mac}")
    data = bytes.fromhex(cleaned)
    return b"\xff" * 6 + data * 16

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
for _ in range(3):
    for mac in macs:
        sock.sendto(packet(mac), (broadcast, port))
    time.sleep(0.25)
sock.close()
PY
}

wait_for_ssh() {
  local deadline=$((SECONDS + WAKE_WAIT_SEC))
  while (( SECONDS < deadline )); do
    if ssh_ok; then
      return 0
    fi
    sleep 2
  done
  return 1
}

mkdir -p "$OUT_DIR"

if ! ssh_ok; then
  if [[ "$WAKE_MACBOOK" != "true" ]]; then
    fail "Mac is not reachable over SSH and WAKE_MACBOOK=false"
  fi
  log "ssh unavailable; sending Wake-on-LAN magic packets to $MAC_HOST"
  send_magic_packets "$MAC_WOL_MACS"
  wait_for_ssh || fail "Mac did not become reachable after Wake-on-LAN"
fi

log "cross-building darwin/arm64 binary"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o "$TMP_DIR/iptv-tunerr-darwin-arm64" ./cmd/iptv-tunerr

log "staging binary and helper scripts on macOS host"
ssh "${SSH_OPTS[@]}" "${MAC_USER}@${MAC_HOST}" "mkdir -p '$REMOTE_ROOT/scripts' '$REMOTE_ROOT/out'"
scp "${SSH_OPTS[@]}" "$TMP_DIR/iptv-tunerr-darwin-arm64" "${MAC_USER}@${MAC_HOST}:$REMOTE_ROOT/iptv-tunerr"
scp "${SSH_OPTS[@]}" \
  scripts/vod-webdav-client-harness.sh \
  scripts/vod-webdav-client-report.py \
  scripts/vod-webdav-client-diff.py \
  "${MAC_USER}@${MAC_HOST}:$REMOTE_ROOT/scripts/"

log "running macOS bare-metal smoke"
ssh "${SSH_OPTS[@]}" "${MAC_USER}@${MAC_HOST}" \
  "chmod +x '$REMOTE_ROOT/iptv-tunerr' '$REMOTE_ROOT/scripts/vod-webdav-client-harness.sh' && REMOTE_ROOT='$REMOTE_ROOT' bash -s" <<'REMOTE'
set -euo pipefail

ROOT="$REMOTE_ROOT"
WORKDIR="$ROOT/work"
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR/assets" "$WORKDIR/cache" "$ROOT/out"

log() { printf '[mac-remote] %s\n' "$*"; }
fail() { printf '[mac-remote] ERROR: %s\n' "$*" >&2; exit 1; }

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
  local url="$1" want="$2" attempts="${3:-60}"
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

body_file="$WORKDIR/body.out"

assert_status() {
  local url="$1" want="$2"
  local code
  code="$(curl -sS -o "$body_file" -w '%{http_code}' "$url" || true)"
  [[ "$code" == "$want" ]] || fail "$url status=$code want=$want body=$(cat "$body_file" 2>/dev/null)"
}

assert_header() {
  local url="$1" header="$2" want="$3"
  local got
  got="$(curl -sSI "$url" | awk -F': ' -v key="$header" 'tolower($1) == tolower(key) { sub(/\r$/, "", $2); print $2; exit }')"
  [[ "$got" == "$want" ]] || fail "$url header $header=$got want=$want"
}

cleanup() {
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT INT TERM
PIDS=()

asset_port="$(pick_port)"
serve_port="$(pick_port)"
empty_port="$(pick_port)"
webui_port="$(pick_port)"
vod_port="$(pick_port)"

printf 'movie-bytes' >"$WORKDIR/assets/movie.bin"
printf 'episode-bytes' >"$WORKDIR/assets/episode.bin"

cat >"$WORKDIR/catalog-vod.json" <<JSON
{
  "movies": [
    {
      "id": "m1",
      "title": "Smoke Movie",
      "year": 2024,
      "stream_url": "http://127.0.0.1:${asset_port}/movie.bin"
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
              "stream_url": "http://127.0.0.1:${asset_port}/episode.bin"
            }
          ]
        }
      ]
    }
  ],
  "live_channels": []
}
JSON

cat >"$WORKDIR/programming.json" <<'JSON'
{
  "selected_categories": ["news"],
  "order_mode": "source"
}
JSON

cat >"$WORKDIR/catalog-full.json" <<'JSON'
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

cat >"$WORKDIR/catalog-empty.json" <<'JSON'
{
  "movies": [],
  "series": [],
  "live_channels": []
}
JSON

cd "$WORKDIR/assets"
python3 -m http.server "$asset_port" --bind 127.0.0.1 >"$ROOT/out/assets.log" 2>&1 &
PIDS+=("$!")
wait_http_code "http://127.0.0.1:${asset_port}/movie.bin" "200" || fail "asset server not ready"

cd "$ROOT"
IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
IPTV_TUNERR_XMLTV_URL= \
IPTV_TUNERR_PROGRAMMING_RECIPE_FILE="$WORKDIR/programming.json" \
IPTV_TUNERR_WEBUI_DISABLED=0 \
IPTV_TUNERR_WEBUI_USER=admin \
IPTV_TUNERR_WEBUI_PASS=admin \
IPTV_TUNERR_WEBUI_PORT="$webui_port" \
./iptv-tunerr serve -catalog "$WORKDIR/catalog-full.json" -addr ":$serve_port" -base-url "http://127.0.0.1:$serve_port" \
  >"$ROOT/out/serve-full.log" 2>&1 &
FULL_PID=$!
PIDS+=("$FULL_PID")

wait_http_code "http://127.0.0.1:$serve_port/discover.json" "200" || fail "full serve discover.json not ready"
wait_http_code "http://127.0.0.1:$webui_port/login" "200" || fail "webui login not ready"
assert_status "http://127.0.0.1:$serve_port/readyz" "200"
assert_status "http://127.0.0.1:$serve_port/guide.xml" "200"
assert_header "http://127.0.0.1:$serve_port/guide.xml" "X-IptvTunerr-Guide-State" "ready"
assert_status "http://127.0.0.1:$serve_port/lineup.json" "200"
grep -q '"GuideNumber":"101"' <(curl -sS "http://127.0.0.1:$serve_port/lineup.json") || fail "lineup missing expected news row"
! grep -q '"GuideNumber":"102"' <(curl -sS "http://127.0.0.1:$serve_port/lineup.json") || fail "programming recipe did not filter sports row"
grep -q '"raw_channels": 3' <(curl -sS "http://127.0.0.1:$serve_port/programming/preview.json") || fail "programming preview missing raw count"
grep -q '"curated_channels": 1' <(curl -sS "http://127.0.0.1:$serve_port/programming/preview.json") || fail "programming preview missing curated count"
grep -q 'login' <(curl -sS "http://127.0.0.1:$webui_port/login") || fail "webui login page body unexpected"

kill "$FULL_PID" 2>/dev/null || true
wait "$FULL_PID" 2>/dev/null || true

IPTV_TUNERR_PROVIDER_EPG_ENABLED=false \
IPTV_TUNERR_XMLTV_URL= \
IPTV_TUNERR_WEBUI_DISABLED=1 \
./iptv-tunerr serve -catalog "$WORKDIR/catalog-empty.json" -addr ":$empty_port" -base-url "http://127.0.0.1:$empty_port" \
  >"$ROOT/out/serve-empty.log" 2>&1 &
EMPTY_PID=$!
PIDS+=("$EMPTY_PID")

wait_http_code "http://127.0.0.1:$empty_port/discover.json" "200" || fail "empty serve discover.json not ready"
assert_status "http://127.0.0.1:$empty_port/readyz" "503"
assert_status "http://127.0.0.1:$empty_port/guide.xml" "503"
assert_header "http://127.0.0.1:$empty_port/guide.xml" "X-IptvTunerr-Guide-State" "loading"
assert_header "http://127.0.0.1:$empty_port/guide.xml" "Retry-After" "5"
assert_header "http://127.0.0.1:$empty_port/discover.json" "X-IptvTunerr-Startup-State" "loading"
assert_header "http://127.0.0.1:$empty_port/lineup.json" "X-IptvTunerr-Startup-State" "loading"

./iptv-tunerr vod-webdav -catalog "$WORKDIR/catalog-vod.json" -addr "127.0.0.1:${vod_port}" -cache "$WORKDIR/cache" \
  >"$ROOT/out/vod-webdav.log" 2>&1 &
PIDS+=("$!")
wait_http_code "http://127.0.0.1:${vod_port}/" "405" || fail "vod-webdav root not ready"

BASE_URL="http://127.0.0.1:${vod_port}" \
OUT_ROOT="$ROOT/out/vod-webdav-client" \
RUN_ID="mac-selfhost" \
KEEP_WORKDIR=true \
"$ROOT/scripts/vod-webdav-client-harness.sh"

{
  echo "mac bare-metal smoke: PASS"
  echo "serve_port=$serve_port"
  echo "webui_port=$webui_port"
  echo "empty_port=$empty_port"
  echo "vod_port=$vod_port"
  echo
  echo "--- vod-webdav report ---"
  cat "$ROOT/out/vod-webdav-client/mac-selfhost/report.txt"
} >"$ROOT/out/summary.txt"
REMOTE

log "collecting artifacts"
scp -r "${SSH_OPTS[@]}" "${MAC_USER}@${MAC_HOST}:$REMOTE_ROOT/out" "$OUT_DIR/"

if [[ -d "$ROOT/.diag/vod-webdav-client" ]] && [[ -f "$OUT_DIR/out/vod-webdav-client/mac-selfhost/report.json" ]]; then
  baseline="$(ls -1dt "$ROOT"/.diag/vod-webdav-client/* 2>/dev/null | head -n1 || true)"
  if [[ -n "$baseline" ]]; then
    python3 scripts/vod-webdav-client-diff.py \
      --left "$baseline" \
      --right "$OUT_DIR/out/vod-webdav-client/mac-selfhost" \
      --left-label baseline \
      --right-label macos \
      --print >"$OUT_DIR/header-diff.txt"
  fi
fi

cat "$OUT_DIR/out/summary.txt"
log "artifacts written to $OUT_DIR"
