#!/usr/bin/env bash
#
# Live race diagnostics harness for PlexTuner / Plex Live TV startup issues.
# Runs a synthetic HLS source, an optional replay HLS source, plex-tuner serve,
# concurrent client probes, and (optionally) tcpdump / PMS log snapshots.
#
# Goal: collect evidence for these hypotheses in one run:
#  1) synthetic source (no provider) stability
#  2) replay source (recorded input) stability
#  3) first-seconds wire capture
#  4) PMS log correlation snapshots
#  5) concurrent request behavior / tuner contention traces
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_ROOT="${OUT_ROOT:-$ROOT/.diag/live-race}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="$OUT_ROOT/$RUN_ID"
WORK_DIR="$OUT_DIR/work"
SYN_DIR="$WORK_DIR/synth"
REPLAY_DIR="$WORK_DIR/replay"
CATALOG="$WORK_DIR/diag-catalog.json"
PLEX_LOG="$OUT_DIR/plex-tuner.log"
SYN_LOG="$OUT_DIR/synth-ffmpeg.log"
REPLAY_LOG="$OUT_DIR/replay-ffmpeg.log"
CURL_LOG="$OUT_DIR/curl.log"
SUMMARY="$OUT_DIR/summary.txt"
TCPDUMP_PCAP="$OUT_DIR/tuner-loopback.pcap"
PMS_SNAPSHOT="$OUT_DIR/pms-logs"

PORT_ALIAS="${PORT:-}"
TUNER_HOST="${TUNER_HOST:-127.0.0.1}"
TUNER_PORT="${TUNER_PORT:-${PORT_ALIAS:-5504}}"
TUNER_ADDR="${TUNER_ADDR:-${TUNER_HOST}:${TUNER_PORT}}"
TUNER_BASE_URL="${TUNER_BASE_URL:-http://${TUNER_HOST}:${TUNER_PORT}}"

RUN_SECONDS="${RUN_SECONDS:-25}"
CONCURRENCY="${CONCURRENCY:-4}"
CLIENT_READ_SECS="${CLIENT_READ_SECS:-8}"
CLIENT_STAGGER_MS="${CLIENT_STAGGER_MS:-200}"
USE_TCPDUMP="${USE_TCPDUMP:-false}"
TCPDUMP_IFACE="${TCPDUMP_IFACE:-lo}"
TCPDUMP_USE_SUDO="${TCPDUMP_USE_SUDO:-auto}" # auto|true|false
PMS_LOG_DIR="${PMS_LOG_DIR:-}"
SYNTH_TS_FILE="${SYNTH_TS_FILE:-}"
REPLAY_TS_FILE="${REPLAY_TS_FILE:-}"
STATIC_HLS_FROM_TS="${STATIC_HLS_FROM_TS:-false}"
STATIC_HLS_EXTINF="${STATIC_HLS_EXTINF:-8.0}"
STATIC_HLS_REPEAT="${STATIC_HLS_REPEAT:-20}"
KEEP_WORKDIR="${KEEP_WORKDIR:-false}"
HARNESS_FFMPEG_BIN="${HARNESS_FFMPEG_BIN:-${PLEX_TUNER_FFMPEG_PATH:-ffmpeg}}"

# Harness knobs for plex-tuner (race-focused defaults)
export PLEX_TUNER_STREAM_BUFFER_BYTES="${PLEX_TUNER_STREAM_BUFFER_BYTES:-0}"
export PLEX_TUNER_STREAM_TRANSCODE="${PLEX_TUNER_STREAM_TRANSCODE:-on}"
export PLEX_TUNER_WEBSAFE_BOOTSTRAP="${PLEX_TUNER_WEBSAFE_BOOTSTRAP:-true}"
export PLEX_TUNER_WEBSAFE_BOOTSTRAP_ALL="${PLEX_TUNER_WEBSAFE_BOOTSTRAP_ALL:-true}"
export PLEX_TUNER_WEBSAFE_BOOTSTRAP_SECONDS="${PLEX_TUNER_WEBSAFE_BOOTSTRAP_SECONDS:-0.35}"
export PLEX_TUNER_WEBSAFE_STARTUP_MIN_BYTES="${PLEX_TUNER_WEBSAFE_STARTUP_MIN_BYTES:-65536}"
export PLEX_TUNER_WEBSAFE_STARTUP_MAX_BYTES="${PLEX_TUNER_WEBSAFE_STARTUP_MAX_BYTES:-524288}"
export PLEX_TUNER_WEBSAFE_STARTUP_TIMEOUT_MS="${PLEX_TUNER_WEBSAFE_STARTUP_TIMEOUT_MS:-30000}"
export PLEX_TUNER_WEBSAFE_REQUIRE_GOOD_START="${PLEX_TUNER_WEBSAFE_REQUIRE_GOOD_START:-false}"
export PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE="${PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE:-true}"
export PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE_MS="${PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE_MS:-100}"
export PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE_PACKETS="${PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE_PACKETS:-1}"
export PLEX_TUNER_TUNER_COUNT="${PLEX_TUNER_TUNER_COUNT:-16}"

PIDS=()
CLIENT_PIDS=()

log() { printf '[harness] %s\n' "$*"; }
warn() { printf '[harness] WARN: %s\n' "$*" >&2; }
die() { printf '[harness] ERROR: %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

maybe_cmd() {
  command -v "$1" >/dev/null 2>&1
}

resolve_ffmpeg_bin() {
  command -v "$HARNESS_FFMPEG_BIN" 2>/dev/null || true
}

can_sudo_nopasswd() {
  sudo -n true >/dev/null 2>&1
}

port_in_use() {
  local port="$1"
  if maybe_cmd ss; then
    ss -ltn "( sport = :$port )" 2>/dev/null | grep -q ":$port"
    return
  fi
  if maybe_cmd netstat; then
    netstat -ltn 2>/dev/null | grep -q "[.:]$port[[:space:]]"
    return
  fi
  return 1
}

cleanup() {
  set +e
  for pid in "${PIDS[@]:-}"; do
    if [[ -n "${pid:-}" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || sudo -n kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  if [[ "$KEEP_WORKDIR" != "true" ]]; then
    rm -rf "$WORK_DIR" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

json_escape() {
  python3 - "$1" <<'PY'
import json, sys
print(json.dumps(sys.argv[1]))
PY
}

write_catalog() {
  local synth_url replay_url
  synth_url="http://127.0.0.1:18080/synth/playlist.m3u8"
  replay_url="http://127.0.0.1:18080/replay/playlist.m3u8"
  mkdir -p "$(dirname "$CATALOG")"
  cat >"$CATALOG" <<EOF
{
  "movies": [],
  "series": [],
  "live_channels": [
    {
      "channel_id": "synth",
      "guide_number": "9001",
      "guide_name": "DIAG Synth",
      "stream_url": "$synth_url",
      "stream_urls": ["$synth_url"],
      "epg_linked": false
    },
    {
      "channel_id": "replay",
      "guide_number": "9002",
      "guide_name": "DIAG Replay",
      "stream_url": "$replay_url",
      "stream_urls": ["$replay_url"],
      "epg_linked": false
    }
  ]
}
EOF
}

write_hls_server() {
  cat >"$WORK_DIR/hls_server.py" <<'PY'
#!/usr/bin/env python3
import os
import posixpath
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

ROOT = os.environ.get("HLS_ROOT")
PORT = int(os.environ.get("HLS_PORT", "18080"))

class H(SimpleHTTPRequestHandler):
    def translate_path(self, path):
        path = path.split('?',1)[0].split('#',1)[0]
        path = posixpath.normpath(path)
        rel = path.lstrip("/")
        return os.path.join(ROOT, rel)
    def log_message(self, fmt, *args):
        print("hls-http: " + (fmt % args), flush=True)

if __name__ == "__main__":
    os.chdir(ROOT)
    ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
PY
  chmod +x "$WORK_DIR/hls_server.py"
}

write_static_hls_playlist() {
  local dir="$1" seg_name="$2"
  local i
  mkdir -p "$dir"
  {
    echo '#EXTM3U'
    echo '#EXT-X-VERSION:3'
    echo "#EXT-X-TARGETDURATION:${STATIC_HLS_EXTINF%.*}"
    echo '#EXT-X-MEDIA-SEQUENCE:0'
    for ((i=0; i<STATIC_HLS_REPEAT; i++)); do
      echo "#EXTINF:${STATIC_HLS_EXTINF},"
      echo "$seg_name"
    done
    # Intentionally omit ENDLIST so it looks live-ish.
  } >"$dir/playlist.m3u8"
}

gen_replay_ts_if_needed() {
  if [[ -n "$REPLAY_TS_FILE" ]]; then
    [[ -f "$REPLAY_TS_FILE" ]] || die "REPLAY_TS_FILE not found: $REPLAY_TS_FILE"
    cp -f "$REPLAY_TS_FILE" "$WORK_DIR/replay-input.ts"
    return
  fi
  log "Generating local replay TS sample (used when REPLAY_TS_FILE is unset)"
  "$FFMPEG_BIN" -hide_banner -loglevel error -nostdin \
    -f lavfi -i "testsrc2=size=1280x720:rate=30000/1001" \
    -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
    -t 30 \
    -map 0:v:0 -map 1:a:0 \
    -c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p \
    -g 30 -keyint_min 30 -sc_threshold 0 \
    -c:a aac -b:a 128k -ar 48000 -ac 2 \
    -f mpegts "$WORK_DIR/replay-input.ts"
}

start_synth_hls() {
  mkdir -p "$SYN_DIR"
  if [[ "$STATIC_HLS_FROM_TS" == "true" ]]; then
    [[ -n "$SYNTH_TS_FILE" ]] || die "STATIC_HLS_FROM_TS=true requires SYNTH_TS_FILE"
    [[ -f "$SYNTH_TS_FILE" ]] || die "SYNTH_TS_FILE not found: $SYNTH_TS_FILE"
    log "Using static HLS source for synth from $SYNTH_TS_FILE"
    cp -f "$SYNTH_TS_FILE" "$SYN_DIR/seg-static.ts"
    write_static_hls_playlist "$SYN_DIR" "seg-static.ts"
    : >"$SYN_LOG"
    return 0
  fi
  log "Starting synthetic live HLS generator"
  "$FFMPEG_BIN" -hide_banner -nostdin -loglevel warning \
    -f lavfi -re -i "testsrc2=size=1280x720:rate=30000/1001" \
    -f lavfi -re -i "anullsrc=r=48000:cl=stereo" \
    -shortest \
    -map 0:v:0 -map 1:a:0 \
    -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p \
    -g 30 -keyint_min 30 -sc_threshold 0 \
    -c:a aac -b:a 96k -ar 48000 -ac 2 \
    -f hls \
    -hls_time 1 \
    -hls_list_size 6 \
    -hls_flags delete_segments+append_list+omit_endlist+independent_segments \
    -hls_segment_filename "$SYN_DIR/seg-%06d.ts" \
    "$SYN_DIR/playlist.m3u8" \
    >"$SYN_LOG" 2>&1 &
  PIDS+=("$!")
}

start_replay_hls() {
  mkdir -p "$REPLAY_DIR"
  if [[ "$STATIC_HLS_FROM_TS" == "true" ]]; then
    [[ -n "$REPLAY_TS_FILE" ]] || die "STATIC_HLS_FROM_TS=true requires REPLAY_TS_FILE"
    [[ -f "$REPLAY_TS_FILE" ]] || die "REPLAY_TS_FILE not found: $REPLAY_TS_FILE"
    log "Using static HLS source for replay from $REPLAY_TS_FILE"
    cp -f "$REPLAY_TS_FILE" "$REPLAY_DIR/seg-static.ts"
    write_static_hls_playlist "$REPLAY_DIR" "seg-static.ts"
    : >"$REPLAY_LOG"
    return 0
  fi
  gen_replay_ts_if_needed
  log "Starting replay HLS generator from $WORK_DIR/replay-input.ts"
  "$FFMPEG_BIN" -hide_banner -nostdin -loglevel warning \
    -re -stream_loop -1 -i "$WORK_DIR/replay-input.ts" \
    -map 0:v:0 -map 0:a? \
    -c copy \
    -f hls \
    -hls_time 1 \
    -hls_list_size 6 \
    -hls_flags delete_segments+append_list+omit_endlist+independent_segments \
    -hls_segment_filename "$REPLAY_DIR/seg-%06d.ts" \
    "$REPLAY_DIR/playlist.m3u8" \
    >"$REPLAY_LOG" 2>&1 &
  PIDS+=("$!")
}

wait_for_file() {
  local file="$1" timeout_s="${2:-15}" i
  for ((i=0; i<timeout_s*10; i++)); do
    [[ -s "$file" ]] && return 0
    sleep 0.1
  done
  return 1
}

start_hls_http_server() {
  write_hls_server
  log "Starting local HLS HTTP server on 127.0.0.1:18080"
  HLS_ROOT="$WORK_DIR" HLS_PORT=18080 python3 "$WORK_DIR/hls_server.py" \
    >"$OUT_DIR/hls-server.log" 2>&1 &
  PIDS+=("$!")
}

start_tcpdump() {
  [[ "$USE_TCPDUMP" == "true" ]] || return 0
  if ! maybe_cmd tcpdump; then
    warn "tcpdump not found; skipping wire capture"
    return 0
  fi
  local use_sudo="false"
  case "$TCPDUMP_USE_SUDO" in
    true) use_sudo="true" ;;
    false) use_sudo="false" ;;
    auto)
      if sudo -n true >/dev/null 2>&1; then
        use_sudo="true"
      fi
      ;;
    *)
      warn "invalid TCPDUMP_USE_SUDO=$TCPDUMP_USE_SUDO (expected auto|true|false); using auto"
      if sudo -n true >/dev/null 2>&1; then
        use_sudo="true"
      fi
      ;;
  esac
  if [[ "$use_sudo" == "true" ]]; then
    log "Starting tcpdump with sudo on ${TCPDUMP_IFACE} for port ${TUNER_PORT}"
    sudo -n tcpdump -Z "$(id -un)" -i "$TCPDUMP_IFACE" -s 0 -U -w "$TCPDUMP_PCAP" "tcp port ${TUNER_PORT}" \
      >"$OUT_DIR/tcpdump.log" 2>&1 &
    PIDS+=("$!")
    return 0
  fi
  log "Starting tcpdump on ${TCPDUMP_IFACE} for port ${TUNER_PORT}"
  tcpdump -i "$TCPDUMP_IFACE" -s 0 -U -w "$TCPDUMP_PCAP" "tcp port ${TUNER_PORT}" \
    >"$OUT_DIR/tcpdump.log" 2>&1 &
  PIDS+=("$!")
}

snapshot_pms_logs() {
  local src="${PMS_LOG_DIR:-}"
  if [[ -z "$src" ]]; then
    src="$(detect_pms_log_dir || true)"
    if [[ -n "$src" ]]; then
      log "Auto-detected PMS logs: $src"
    fi
  fi
  [[ -n "$src" ]] || return 0

  mkdir -p "$PMS_SNAPSHOT"
  if [[ -d "$src" ]]; then
    cp -a "$src"/. "$PMS_SNAPSHOT"/ 2>/dev/null || warn "PMS log snapshot copy had errors"
    return 0
  fi
  if can_sudo_nopasswd; then
    sudo -n test -d "$src" 2>/dev/null || {
      warn "PMS_LOG_DIR does not exist: $src"
      return 0
    }
    sudo -n cp -a "$src"/. "$PMS_SNAPSHOT"/ 2>/dev/null || warn "PMS log snapshot copy had errors (sudo)"
    return 0
  fi
  warn "PMS_LOG_DIR not readable and sudo unavailable: $src"
}

detect_pms_log_dir() {
  local d pid
  for d in \
    "/var/lib/plex/Plex Media Server/Logs" \
    "/var/lib/plexmediaserver/Library/Application Support/Plex Media Server/Logs" \
    "$HOME/Library/Application Support/Plex Media Server/Logs"
  do
    [[ -d "$d" ]] && { printf '%s\n' "$d"; return 0; }
  done

  if maybe_cmd pgrep; then
    pid="$(pgrep -xo 'Plex Media Server' 2>/dev/null || true)"
    if [[ -n "$pid" ]]; then
      d="/proc/$pid/root/config/Library/Application Support/Plex Media Server/Logs"
      if [[ -d "$d" ]]; then
        printf '%s\n' "$d"
        return 0
      fi
      if can_sudo_nopasswd && sudo -n test -d "$d" 2>/dev/null; then
        printf '%s\n' "$d"
        return 0
      fi
    fi
  fi
  return 1
}

start_plex_tuner() {
  log "Starting plex-tuner serve on $TUNER_ADDR using generated diag catalog"
  (
    cd "$ROOT"
    # The app itself loads .env on startup. Temporarily hide it so harness env overrides win.
    env_bak=""
    if [[ -f .env ]]; then
      env_bak=".env.harness.bak.$$"
      mv -f .env "$env_bak"
      trap '[[ -n "$env_bak" && -f "$env_bak" ]] && mv -f "$env_bak" .env' EXIT
    fi
    PLEX_TUNER_BASE_URL="${PLEX_TUNER_BASE_URL:-$TUNER_BASE_URL}" \
    go run ./cmd/plex-tuner serve -catalog "$CATALOG" -addr "$TUNER_ADDR" -base-url "$TUNER_BASE_URL"
  ) >"$PLEX_LOG" 2>&1 &
  PIDS+=("$!")
}

wait_for_tuner() {
  local url="$TUNER_BASE_URL/discover.json" lineup="$TUNER_BASE_URL/lineup.json" i
  for ((i=0; i<200; i++)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      if curl -fsS "$lineup" 2>/dev/null | grep -q 'DIAG Synth'; then
        return 0
      fi
    fi
    sleep 0.1
  done
  return 1
}

curl_probe() {
  local label="$1" path="$2"
  local url="$TUNER_BASE_URL$path"
  local body="$OUT_DIR/${label}.ts"
  local headers="$OUT_DIR/${label}.headers"
  {
    echo "=== $label $url $(date -Is) ==="
    curl -sS -D "$headers" --max-time "$CLIENT_READ_SECS" "$url" -o "$body" || true
    wc -c "$body" 2>/dev/null || true
    head -c 188 "$body" | od -An -tx1 -v 2>/dev/null | head -3 || true
  } >>"$CURL_LOG" 2>&1
}

run_concurrent_clients() {
  : >"$CURL_LOG"
  log "Launching $CONCURRENCY concurrent probe clients against /stream/synth and /stream/replay"
  local i label path sleep_s
  for ((i=1; i<=CONCURRENCY; i++)); do
    if (( i % 2 == 1 )); then
      path="/stream/synth"
      label="synth-c$(printf '%02d' "$i")"
    else
      path="/stream/replay"
      label="replay-c$(printf '%02d' "$i")"
    fi
    curl_probe "$label" "$path" &
    PIDS+=("$!")
    CLIENT_PIDS+=("$!")
    sleep_s="$(awk "BEGIN { printf \"%.3f\", ${CLIENT_STAGGER_MS}/1000 }")"
    sleep "$sleep_s"
  done
}

write_summary() {
  {
    echo "Run ID: $RUN_ID"
    echo "Out Dir: $OUT_DIR"
    echo "Date: $(date -Is)"
    echo
    echo "Experiments covered:"
    echo "  1) Synthetic local HLS source     -> /stream/synth"
    echo "  2) Replay HLS source (local TS)   -> /stream/replay"
    echo "  3) Wire capture (optional)        -> $TCPDUMP_PCAP"
    echo "  4) PMS log snapshot (optional)    -> $PMS_SNAPSHOT"
    echo "  5) Concurrent request probes      -> $CURL_LOG + req IDs in plex log"
    echo
    echo "Key files:"
    echo "  plex-tuner log: $PLEX_LOG"
    echo "  synth ffmpeg log: $SYN_LOG"
    echo "  replay ffmpeg log: $REPLAY_LOG"
    echo "  curl log: $CURL_LOG"
    [[ -f "$TCPDUMP_PCAP" ]] && echo "  tcpdump pcap: $TCPDUMP_PCAP"
    echo
    echo "Suggested grep:"
    echo "  grep -E 'req=|startup-gate|null-ts-keepalive|bootstrap-ts|first-bytes|all-tuners-in-use|acquire|release' \"$PLEX_LOG\""
    echo "  python3 \"$ROOT/scripts/live-race-harness-report.py\" --dir \"$OUT_DIR\" --print"
  } >"$SUMMARY"
}

main() {
  need_cmd bash
  need_cmd python3
  need_cmd curl
  need_cmd go
  FFMPEG_BIN="$(resolve_ffmpeg_bin)"
  [[ -n "$FFMPEG_BIN" ]] || die "ffmpeg binary not found: $HARNESS_FFMPEG_BIN"
  log "Using ffmpeg binary: $FFMPEG_BIN"

  if port_in_use "$TUNER_PORT"; then
    die "TUNER_PORT $TUNER_PORT is already in use (set TUNER_ADDR/TUNER_BASE_URL/TUNER_PORT to a free port)"
  fi

  mkdir -p "$OUT_DIR" "$WORK_DIR" "$SYN_DIR" "$REPLAY_DIR"
  log "Output dir: $OUT_DIR"
  log "Harness defaults: RUN_SECONDS=$RUN_SECONDS CONCURRENCY=$CONCURRENCY CLIENT_READ_SECS=$CLIENT_READ_SECS"

  write_catalog
  start_synth_hls
  start_replay_hls
  wait_for_file "$SYN_DIR/playlist.m3u8" 20 || die "synthetic playlist did not appear"
  wait_for_file "$REPLAY_DIR/playlist.m3u8" 20 || die "replay playlist did not appear"

  start_hls_http_server
  start_tcpdump
  snapshot_pms_logs
  start_plex_tuner
  wait_for_tuner || die "plex-tuner did not start on $TUNER_BASE_URL"

  run_concurrent_clients

  log "Harness running for ${RUN_SECONDS}s (you can also trigger Plex clients during this window)"
  sleep "$RUN_SECONDS"

  # Wait for client curls to finish before cleanup.
  for pid in "${CLIENT_PIDS[@]:-}"; do
    wait "$pid" 2>/dev/null || true
  done

  snapshot_pms_logs
  write_summary
  if [[ -x "$ROOT/scripts/live-race-harness-report.py" ]] || [[ -f "$ROOT/scripts/live-race-harness-report.py" ]]; then
    python3 "$ROOT/scripts/live-race-harness-report.py" --dir "$OUT_DIR" >"$OUT_DIR/report.run.log" 2>&1 || \
      warn "report parser failed (see $OUT_DIR/report.run.log)"
  fi

  log "Done. Summary: $SUMMARY"
  log "Quick grep:"
  echo "grep -E 'req=|startup-gate|null-ts-keepalive|bootstrap-ts|first-bytes|all-tuners-in-use|acquire|release' \"$PLEX_LOG\""
  echo "python3 \"$ROOT/scripts/live-race-harness-report.py\" --dir \"$OUT_DIR\" --print"
}

main "$@"
