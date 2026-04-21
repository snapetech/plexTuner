#!/usr/bin/env bash
#
# Real-tuner multi-stream contention harness.
# Starts 2+ live channel pulls against a running IPTV Tunerr instance, captures
# per-stream curl artifacts, periodic provider/attempt snapshots, and optional
# Plex /status/sessions snapshots so "second stream kills the first" failures
# can be reproduced and compared consistently.
#
# Typical usage:
#   TUNERR_BASE_URL='http://127.0.0.1:5004' \
#   CHANNEL_IDS='325824,123456' \
#   RUN_SECONDS=30 START_STAGGER_SECS=2 \
#   ./scripts/multi-stream-harness.sh
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_ROOT="${OUT_ROOT:-$ROOT/.diag/multi-stream}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="$OUT_ROOT/$RUN_ID"
SUMMARY="$OUT_DIR/summary.txt"
RUN_LOG="$OUT_DIR/harness.log"
PROVIDER_DIR="$OUT_DIR/provider-profile"
ATTEMPTS_DIR="$OUT_DIR/stream-attempts"
RUNTIME_DIR="$OUT_DIR/runtime"
PMS_SESSION_DIR="$OUT_DIR/pms-sessions"

TUNERR_BASE_URL="${TUNERR_BASE_URL:-}"
CHANNEL_IDS="${CHANNEL_IDS:-}"
CHANNEL_URLS_FILE="${CHANNEL_URLS_FILE:-}"
RUN_SECONDS="${RUN_SECONDS:-25}"
START_STAGGER_SECS="${START_STAGGER_SECS:-2}"
READ_TIMEOUT_SECS="${READ_TIMEOUT_SECS:-0}"
POLL_SECS="${POLL_SECS:-3}"
ATTEMPTS_LIMIT="${ATTEMPTS_LIMIT:-25}"
CURL_USER_AGENT="${CURL_USER_AGENT:-}"
DISCARD_BODY="${DISCARD_BODY:-false}"
PMS_URL="${PMS_URL:-${IPTV_TUNERR_PMS_URL:-}}"
PMS_TOKEN="${PMS_TOKEN:-${IPTV_TUNERR_PMS_TOKEN:-${PLEX_TOKEN:-}}}"

PIDS=()
CHANNEL_LABELS=()
CHANNEL_TARGETS=()

log() { printf '[multi-stream] %s\n' "$*" | tee -a "$RUN_LOG" >&2; }
warn() { printf '[multi-stream] WARN: %s\n' "$*" | tee -a "$RUN_LOG" >&2; }
die() { printf '[multi-stream] ERROR: %s\n' "$*" | tee -a "$RUN_LOG" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

cleanup() {
  set +e
  for pid in "${PIDS[@]:-}"; do
    if [[ -n "${pid:-}" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT INT TERM

resolve_pms_url() {
  if [[ -n "$PMS_URL" ]]; then
    printf '%s\n' "${PMS_URL%/}"
    return 0
  fi
  if [[ -n "${PLEX_HOST:-}" ]]; then
    printf 'http://%s\n' "${PLEX_HOST%/}"
    return 0
  fi
  return 1
}

load_channels() {
  CHANNEL_LABELS=()
  CHANNEL_TARGETS=()
  if [[ -n "$CHANNEL_URLS_FILE" ]]; then
    [[ -f "$CHANNEL_URLS_FILE" ]] || die "CHANNEL_URLS_FILE not found: $CHANNEL_URLS_FILE"
    while IFS= read -r line || [[ -n "$line" ]]; do
      line="${line#"${line%%[![:space:]]*}"}"
      line="${line%"${line##*[![:space:]]}"}"
      [[ -n "$line" ]] || continue
      [[ "$line" =~ ^# ]] && continue
      local label target
      if [[ "$line" == *"="* ]]; then
        label="${line%%=*}"
        target="${line#*=}"
      else
        label="$line"
        target="${TUNERR_BASE_URL%/}/stream/$line"
      fi
      CHANNEL_LABELS+=("$label")
      CHANNEL_TARGETS+=("$target")
    done <"$CHANNEL_URLS_FILE"
  elif [[ -n "$CHANNEL_IDS" ]]; then
    [[ -n "$TUNERR_BASE_URL" ]] || die "TUNERR_BASE_URL is required when CHANNEL_IDS is used"
    IFS=',' read -r -a raw_ids <<<"$CHANNEL_IDS"
    for id in "${raw_ids[@]}"; do
      id="${id#"${id%%[![:space:]]*}"}"
      id="${id%"${id##*[![:space:]]}"}"
      [[ -n "$id" ]] || continue
      CHANNEL_LABELS+=("$id")
      CHANNEL_TARGETS+=("${TUNERR_BASE_URL%/}/stream/$id")
    done
  else
    die "set CHANNEL_IDS or CHANNEL_URLS_FILE"
  fi
  if (( ${#CHANNEL_TARGETS[@]} < 2 )); then
    die "need at least 2 channels/targets"
  fi
}

poll_tunerr_json() {
  local label="$1" url="$2" outdir="$3"
  mkdir -p "$outdir"
  local ts
  ts="$(date +%Y%m%d-%H%M%S)"
  curl -fsS "$url" -o "$outdir/$ts.json" 2>"$outdir/$ts.stderr" || true
}

poll_pms_sessions() {
  local base
  base="$(resolve_pms_url || true)"
  [[ -n "$base" && -n "$PMS_TOKEN" ]] || return 0
  mkdir -p "$PMS_SESSION_DIR"
  local ts
  ts="$(date +%Y%m%d-%H%M%S)"
  curl -fsS "${base}/status/sessions?X-Plex-Token=${PMS_TOKEN}" -o "$PMS_SESSION_DIR/$ts.xml" 2>"$PMS_SESSION_DIR/$ts.stderr" || true
}

poll_state_loop() {
  while true; do
    poll_tunerr_json "provider" "${TUNERR_BASE_URL%/}/provider/profile.json" "$PROVIDER_DIR"
    poll_tunerr_json "attempts" "${TUNERR_BASE_URL%/}/debug/stream-attempts.json?limit=${ATTEMPTS_LIMIT}" "$ATTEMPTS_DIR"
    poll_tunerr_json "runtime" "${TUNERR_BASE_URL%/}/debug/runtime.json" "$RUNTIME_DIR"
    poll_pms_sessions
    sleep "$POLL_SECS"
  done
}

run_channel_pull() {
  local idx="$1" label="$2" url="$3"
  local dir="$OUT_DIR/channel-$idx"
  mkdir -p "$dir"
  local body="$dir/body.ts"
  local output_path="$body"
  if [[ "$DISCARD_BODY" == "1" || "$DISCARD_BODY" == "true" ]]; then
    output_path="/dev/null"
  fi
  local headers="$dir/headers.txt"
  local meta="$dir/meta.json"
  local start_ts end_ts exit_code
  start_ts="$(date -Is)"
  exit_code=0
  local -a args
  args=(-D "$headers" --silent --show-error --location --output "$output_path")
  if [[ -n "$CURL_USER_AGENT" ]]; then
    args+=(-A "$CURL_USER_AGENT")
  fi
  if [[ "$READ_TIMEOUT_SECS" != "0" ]]; then
    args+=(--max-time "$READ_TIMEOUT_SECS")
  else
    args+=(--max-time "$RUN_SECONDS")
  fi
  {
    printf 'label=%s\nurl=%s\nstarted_at=%s\n' "$label" "$url" "$start_ts"
    curl "${args[@]}" --write-out 'http_code=%{http_code}\nsize_download=%{size_download}\ntime_total=%{time_total}\nurl_effective=%{url_effective}\n' "$url"
  } >"$dir/curl.meta.raw" 2>"$dir/curl.stderr" || exit_code=$?
  end_ts="$(date -Is)"
  python3 - "$label" "$url" "$start_ts" "$end_ts" "$exit_code" "$body" "$dir/curl.meta.raw" "$meta" "$DISCARD_BODY" <<'PY'
import json, os, sys
label, url, started_at, ended_at, exit_code, body_path, raw_path, out_path, discard_body = sys.argv[1:]
payload = {
    "label": label,
    "url": url,
    "started_at": started_at,
    "ended_at": ended_at,
    "exit_code": int(exit_code),
    "discard_body": discard_body.lower() in {"1", "true", "yes"},
    "bytes_written": os.path.getsize(body_path) if os.path.exists(body_path) else 0,
}
if os.path.exists(raw_path):
    with open(raw_path, "r", encoding="utf-8", errors="replace") as fh:
        for line in fh:
            line = line.strip()
            if "=" not in line:
                continue
            k, v = line.split("=", 1)
            if k in {"http_code", "size_download"}:
                try:
                    payload[k] = int(float(v))
                except ValueError:
                    payload[k] = v
            elif k == "time_total":
                try:
                    payload[k] = float(v)
                except ValueError:
                    payload[k] = v
            else:
                payload[k] = v
if payload["discard_body"] and "size_download" in payload:
    payload["bytes_written"] = payload["size_download"]
with open(out_path, "w", encoding="utf-8") as fh:
    json.dump(payload, fh, indent=2, sort_keys=True)
    fh.write("\n")
PY
}

write_summary() {
  {
    echo "Run ID: $RUN_ID"
    echo "Out Dir: $OUT_DIR"
    echo "Base URL: $TUNERR_BASE_URL"
    echo "Channels: ${CHANNEL_LABELS[*]}"
    echo "Run Seconds: $RUN_SECONDS"
    echo "Start Stagger Seconds: $START_STAGGER_SECS"
    echo "Poll Seconds: $POLL_SECS"
    echo "Curl User Agent: ${CURL_USER_AGENT:-<default>}"
    echo "Discard Body: $DISCARD_BODY"
    echo
    echo "Artifacts:"
    echo "  per-channel: $OUT_DIR/channel-*"
    echo "  provider snapshots: $PROVIDER_DIR"
    echo "  stream attempts: $ATTEMPTS_DIR"
    echo "  runtime snapshots: $RUNTIME_DIR"
    [[ -d "$PMS_SESSION_DIR" ]] && echo "  pms sessions: $PMS_SESSION_DIR"
    echo
    echo "Report:"
    echo "  python3 \"$ROOT/scripts/multi-stream-harness-report.py\" --dir \"$OUT_DIR\" --print"
  } >"$SUMMARY"
}

main() {
  need_cmd bash
  need_cmd curl
  need_cmd python3
  [[ -n "$TUNERR_BASE_URL" ]] || die "TUNERR_BASE_URL is required"
  mkdir -p "$OUT_DIR" "$PROVIDER_DIR" "$ATTEMPTS_DIR" "$RUNTIME_DIR"
  : >"$RUN_LOG"
  load_channels

  log "Output dir: $OUT_DIR"
  poll_tunerr_json "provider" "${TUNERR_BASE_URL%/}/provider/profile.json" "$PROVIDER_DIR"
  poll_tunerr_json "attempts" "${TUNERR_BASE_URL%/}/debug/stream-attempts.json?limit=${ATTEMPTS_LIMIT}" "$ATTEMPTS_DIR"
  poll_tunerr_json "runtime" "${TUNERR_BASE_URL%/}/debug/runtime.json" "$RUNTIME_DIR"
  poll_pms_sessions

  poll_state_loop &
  PIDS+=("$!")

  local idx=0
  for idx in "${!CHANNEL_TARGETS[@]}"; do
    run_channel_pull "$((idx+1))" "${CHANNEL_LABELS[$idx]}" "${CHANNEL_TARGETS[$idx]}" &
    PIDS+=("$!")
    if (( idx + 1 < ${#CHANNEL_TARGETS[@]} )); then
      sleep "$START_STAGGER_SECS"
    fi
  done

  local pid
  for pid in "${PIDS[@]:1}"; do
    wait "$pid" 2>/dev/null || true
  done

  poll_tunerr_json "provider" "${TUNERR_BASE_URL%/}/provider/profile.json" "$PROVIDER_DIR"
  poll_tunerr_json "attempts" "${TUNERR_BASE_URL%/}/debug/stream-attempts.json?limit=${ATTEMPTS_LIMIT}" "$ATTEMPTS_DIR"
  poll_tunerr_json "runtime" "${TUNERR_BASE_URL%/}/debug/runtime.json" "$RUNTIME_DIR"
  poll_pms_sessions

  write_summary
  if [[ -x "$ROOT/scripts/multi-stream-harness-report.py" ]] || [[ -f "$ROOT/scripts/multi-stream-harness-report.py" ]]; then
    python3 "$ROOT/scripts/multi-stream-harness-report.py" --dir "$OUT_DIR" >"$OUT_DIR/report.run.log" 2>&1 || \
      warn "report parser failed (see $OUT_DIR/report.run.log)"
  fi

  log "Done. Summary: $SUMMARY"
}

main "$@"
