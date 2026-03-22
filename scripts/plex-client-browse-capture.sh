#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

OUT_ROOT="${OUT_ROOT:-$ROOT_DIR/.diag/plex-client-browse}"
POLL_SECS="${POLL_SECS:-2}"
CLIENT_ID="${CLIENT_ID:-iptvtunerr-plex-client-browse-capture}"
ACTIVE_FILE="$OUT_ROOT/.active-run"

usage() {
  cat <<'EOF'
Usage:
  scripts/plex-client-browse-capture.sh start [options]
  scripts/plex-client-browse-capture.sh stop
  scripts/plex-client-browse-capture.sh status

Capture PMS + plex.tv evidence while a real client browses Live TV.

Start options:
  -id <run-id>            Run identifier. Default: tv-browse-YYYYmmdd-HHMMSS
  -out <dir>              Exact output directory. Overrides -id.
  -plex-data-dir <dir>    Plex data dir containing Preferences.xml and Logs/
  -pms-url <url>          PMS base URL. Default: from env or http://127.0.0.1:32400
  -token <token>          Plex owner token. Default: from env or Preferences.xml
  -machine-id <id>        Processed machine identifier. Default: from Preferences.xml
  -poll-secs <n>          Poll interval. Default: 2

Examples:
  scripts/plex-client-browse-capture.sh start -print
  scripts/plex-client-browse-capture.sh stop
EOF
}

err() {
  echo "[plex-client-browse-capture] ERROR: $*" >&2
  exit 1
}

log() {
  echo "[plex-client-browse-capture] $*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "missing command: $1"
}

guess_plex_data_dir() {
  local candidates=(
    "/var/lib/plex-standby-config/Library/Application Support/Plex Media Server"
    "/var/lib/plexmediaserver/Library/Application Support/Plex Media Server"
    "$HOME/Library/Application Support/Plex Media Server"
  )
  local dir
  for dir in "${candidates[@]}"; do
    if [[ -f "$dir/Preferences.xml" && -d "$dir/Logs" ]]; then
      printf '%s\n' "$dir"
      return 0
    fi
  done
  return 1
}

read_pref_attr() {
  local pref_file="$1"
  local attr="$2"
  python3 - "$pref_file" "$attr" <<'PY'
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
print(root.attrib.get(sys.argv[2], ""))
PY
}

record_log_offsets() {
  local log_dir="$1"
  local out_file="$2"
  : >"$out_file"
  local file size
  while IFS= read -r -d '' file; do
    size="$(wc -c <"$file" | tr -d '[:space:]')"
    printf '%s\t%s\n' "$file" "$size" >>"$out_file"
  done < <(find "$log_dir" -maxdepth 1 -type f -name 'Plex Media Server*.log' -print0 | sort -z)
}

capture_new_log_bytes() {
  local offsets_file="$1"
  local out_dir="$2"
  mkdir -p "$out_dir"
  local path offset current start base
  while IFS=$'\t' read -r path offset; do
    [[ -n "${path:-}" ]] || continue
    [[ -f "$path" ]] || continue
    current="$(wc -c <"$path" | tr -d '[:space:]')"
    start=$((offset + 1))
    if (( current < start )); then
      start=1
    fi
    base="$(basename "$path")"
    tail -c +"$start" "$path" >"$out_dir/$base.slice.log"
  done <"$offsets_file"
}

snapshot_requests() {
  local target_dir="$1"
  local pms_url="$2"
  local token="$3"
  local machine_id="$4"
  local plex_data_dir="$5"
  local ts="$6"

  mkdir -p "$target_dir"

  go run ./cmd/iptv-tunerr plex-api-inspect \
    -plex-url "$pms_url" \
    -token "$token" \
    -include-probes=true \
    -out "$target_dir/pms-api-inspect-$ts.json" >/dev/null

  go run ./cmd/iptv-tunerr plex-api-request \
    -base-url "$pms_url" \
    -token "$token" \
    -method GET \
    -path /status/sessions \
    -out "$target_dir/pms-status-sessions-$ts.json" >/dev/null || true

  go run ./cmd/iptv-tunerr plex-api-request \
    -base-url https://plex.tv \
    -token "$token" \
    -method GET \
    -path /api/users \
    -headers "X-Plex-Client-Identifier: $CLIENT_ID" \
    -out "$target_dir/plextv-users-$ts.json" >/dev/null

  go run ./cmd/iptv-tunerr plex-api-request \
    -base-url https://plex.tv \
    -token "$token" \
    -method GET \
    -path /api/home/users \
    -headers "X-Plex-Client-Identifier: $CLIENT_ID" \
    -out "$target_dir/plextv-home-users-$ts.json" >/dev/null

  go run ./cmd/iptv-tunerr plex-api-request \
    -base-url https://plex.tv \
    -token "$token" \
    -method GET \
    -path "/api/servers/$machine_id/shared_servers" \
    -headers "X-Plex-Client-Identifier: $CLIENT_ID" \
    -out "$target_dir/plextv-shared-servers-$ts.json" >/dev/null

  go run ./cmd/iptv-tunerr plex-db-inspect \
    -plex-data-dir "$plex_data_dir" \
    -out "$target_dir/plex-db-inspect-$ts.json" >/dev/null || true
}

start_poller() {
  local poll_dir="$1"
  local pms_url="$2"
  local token="$3"
  local machine_id="$4"
  local poll_log="$poll_dir/poller.log"
  mkdir -p "$poll_dir"
  (
    while true; do
      local ts
      ts="$(date -u +%Y%m%dT%H%M%SZ)"
      go run ./cmd/iptv-tunerr plex-api-request \
        -base-url "$pms_url" \
        -token "$token" \
        -method GET \
        -path /status/sessions \
        -out "$poll_dir/pms-status-sessions-$ts.json" >/dev/null || true
      go run ./cmd/iptv-tunerr plex-api-request \
        -base-url "$pms_url" \
        -token "$token" \
        -method GET \
        -path /livetv/dvrs \
        -out "$poll_dir/pms-dvrs-$ts.json" >/dev/null || true
      go run ./cmd/iptv-tunerr plex-api-request \
        -base-url "$pms_url" \
        -token "$token" \
        -method GET \
        -path /media/providers \
        -out "$poll_dir/pms-media-providers-$ts.json" >/dev/null || true
      go run ./cmd/iptv-tunerr plex-api-request \
        -base-url https://plex.tv \
        -token "$token" \
        -method GET \
        -path "/api/servers/$machine_id/shared_servers" \
        -headers "X-Plex-Client-Identifier: $CLIENT_ID" \
        -out "$poll_dir/plextv-shared-servers-$ts.json" >/dev/null || true
      sleep "$POLL_SECS"
    done
  ) >>"$poll_log" 2>&1 </dev/null &
  STARTED_POLLER_PID="$!"
}

write_notes_template() {
  local out_dir="$1"
  cat >"$out_dir/notes.md" <<EOF
# TV Browse Notes

- Run id: $(basename "$out_dir")
- Started at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
- TV client:
- TV client version:
- Client account username:
- Expected result:
- Actual result:
- Exact browse path taken:
  - 
- Whether Live TV was visible:
- Whether guide rows appeared:
- Whether tune was attempted:
- Approximate wall-clock times:
  - 
- Next analysis:
  - Compare \`snapshots/pre\` vs \`snapshots/post\`
  - Inspect \`polls/\` for session and Live TV object changes
  - Inspect \`logs/plex/\` for new request lines from this browse window
EOF
}

start_capture() {
  local run_id="" out_dir="" plex_data_dir="" pms_url="" token="" machine_id=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -id) run_id="${2:-}"; shift 2 ;;
      -out) out_dir="${2:-}"; shift 2 ;;
      -plex-data-dir) plex_data_dir="${2:-}"; shift 2 ;;
      -pms-url) pms_url="${2:-}"; shift 2 ;;
      -token) token="${2:-}"; shift 2 ;;
      -machine-id) machine_id="${2:-}"; shift 2 ;;
      -poll-secs) POLL_SECS="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      *) err "unknown start argument: $1" ;;
    esac
  done

  [[ ! -f "$ACTIVE_FILE" ]] || err "an active run already exists; stop it first"

  if [[ -z "$plex_data_dir" ]]; then
    plex_data_dir="$(guess_plex_data_dir || true)"
  fi
  [[ -n "$plex_data_dir" ]] || err "set -plex-data-dir"
  [[ -f "$plex_data_dir/Preferences.xml" ]] || err "missing Preferences.xml under $plex_data_dir"
  [[ -d "$plex_data_dir/Logs" ]] || err "missing Logs/ under $plex_data_dir"

  if [[ -z "$out_dir" ]]; then
    if [[ -z "$run_id" ]]; then
      run_id="tv-browse-$(date +%Y%m%d-%H%M%S)"
    fi
    out_dir="$OUT_ROOT/$run_id"
  fi
  mkdir -p "$out_dir"/{snapshots/pre,snapshots/post,polls,logs/plex,state}

  if [[ -z "$token" ]]; then
    token="${IPTV_TUNERR_PMS_TOKEN:-${PLEX_TOKEN:-}}"
  fi
  if [[ -z "$token" ]]; then
    token="$(read_pref_attr "$plex_data_dir/Preferences.xml" "PlexOnlineToken")"
  fi
  [[ -n "$token" ]] || err "could not resolve Plex token"

  if [[ -z "$machine_id" ]]; then
    machine_id="$(read_pref_attr "$plex_data_dir/Preferences.xml" "ProcessedMachineIdentifier")"
  fi
  [[ -n "$machine_id" ]] || err "could not resolve processed machine identifier"

  if [[ -z "$pms_url" ]]; then
    pms_url="${IPTV_TUNERR_PMS_URL:-}"
  fi
  if [[ -z "$pms_url" && -n "${PLEX_HOST:-}" ]]; then
    pms_url="http://${PLEX_HOST}:32400"
  fi
  if [[ -z "$pms_url" ]]; then
    pms_url="http://127.0.0.1:32400"
  fi

  write_notes_template "$out_dir"
  record_log_offsets "$plex_data_dir/Logs" "$out_dir/state/log-offsets.tsv"
  snapshot_requests "$out_dir/snapshots/pre" "$pms_url" "$token" "$machine_id" "$plex_data_dir" "$(date -u +%Y%m%dT%H%M%SZ)"

  STARTED_POLLER_PID=""
  start_poller "$out_dir/polls" "$pms_url" "$token" "$machine_id"
  local poller_pid="$STARTED_POLLER_PID"

  cat >"$out_dir/state/run.env" <<EOF
OUT_DIR='$out_dir'
PLEX_DATA_DIR='$plex_data_dir'
PMS_URL='$pms_url'
TOKEN='$token'
MACHINE_ID='$machine_id'
POLLER_PID='$poller_pid'
POLL_SECS='$POLL_SECS'
STARTED_AT='$(date -u +"%Y-%m-%dT%H:%M:%SZ")'
EOF
  printf '%s\n' "$out_dir" >"$ACTIVE_FILE"

  log "started: $out_dir"
  log "browse on the TV now, then run: scripts/plex-client-browse-capture.sh stop"
}

stop_capture() {
  [[ -f "$ACTIVE_FILE" ]] || err "no active run"
  local out_dir
  out_dir="$(cat "$ACTIVE_FILE")"
  [[ -f "$out_dir/state/run.env" ]] || err "missing state file in $out_dir"
  # shellcheck disable=SC1090
  source "$out_dir/state/run.env"

  if [[ -n "${POLLER_PID:-}" ]] && kill -0 "$POLLER_PID" 2>/dev/null; then
    kill "$POLLER_PID" 2>/dev/null || true
    wait "$POLLER_PID" 2>/dev/null || true
  fi

  snapshot_requests "$OUT_DIR/snapshots/post" "$PMS_URL" "$TOKEN" "$MACHINE_ID" "$PLEX_DATA_DIR" "$(date -u +%Y%m%dT%H%M%SZ)"
  capture_new_log_bytes "$OUT_DIR/state/log-offsets.tsv" "$OUT_DIR/logs/plex"

  rm -f "$ACTIVE_FILE"
  log "stopped: $OUT_DIR"
  log "review: $OUT_DIR"
}

status_capture() {
  if [[ ! -f "$ACTIVE_FILE" ]]; then
    log "no active run"
    exit 0
  fi
  local out_dir
  out_dir="$(cat "$ACTIVE_FILE")"
  [[ -f "$out_dir/state/run.env" ]] || err "missing state file in $out_dir"
  # shellcheck disable=SC1090
  source "$out_dir/state/run.env"
  log "active run: $OUT_DIR"
  log "started: ${STARTED_AT:-unknown}"
  if [[ -n "${POLLER_PID:-}" ]] && kill -0 "$POLLER_PID" 2>/dev/null; then
    log "poller: running (pid=$POLLER_PID)"
  else
    log "poller: not running"
  fi
}

need_cmd go
need_cmd python3
need_cmd curl

cmd="${1:-}"
case "$cmd" in
  start)
    shift
    start_capture "$@"
    ;;
  stop)
    shift
    [[ $# -eq 0 ]] || err "stop takes no extra arguments"
    stop_capture
    ;;
  status)
    shift
    [[ $# -eq 0 ]] || err "status takes no extra arguments"
    status_capture
    ;;
  -h|--help|"")
    usage
    ;;
  *)
    err "unknown command: $cmd"
    ;;
esac
