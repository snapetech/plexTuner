#!/usr/bin/env bash
#
# Capture and compare one "good" and one "bad" channel so intermittent
# playback reports become a channel-class diff instead of a single anecdote.
#
# Typical usage:
#   TUNERR_BASE_URL='http://127.0.0.1:5004' \
#   GOOD_CHANNEL_ID='325860' \
#   BAD_CHANNEL_ID='325778' \
#   ./scripts/channel-diff-harness.sh
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_ROOT="${OUT_ROOT:-$ROOT/.diag/channel-diff}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="$OUT_ROOT/$RUN_ID"
RUNS_DIR="$OUT_DIR/runs"

TUNERR_BASE_URL="${TUNERR_BASE_URL:-}"
GOOD_CHANNEL_ID="${GOOD_CHANNEL_ID:-}"
BAD_CHANNEL_ID="${BAD_CHANNEL_ID:-}"
GOOD_DIRECT_URL="${GOOD_DIRECT_URL:-}"
BAD_DIRECT_URL="${BAD_DIRECT_URL:-}"
RUN_SECONDS="${RUN_SECONDS:-20}"
SEED_SECONDS="${SEED_SECONDS:-8}"
ATTEMPT_LIMIT="${ATTEMPT_LIMIT:-40}"

USE_CURL="${USE_CURL:-true}"
USE_FFPROBE="${USE_FFPROBE:-true}"
USE_FFPLAY="${USE_FFPLAY:-true}"
USE_TCPDUMP="${USE_TCPDUMP:-false}"
FFPLAY_LOGLEVEL="${FFPLAY_LOGLEVEL:-verbose}"
ANALYZE_MANIFESTS="${ANALYZE_MANIFESTS:-true}"

log() { printf '[channel-diff] %s\n' "$*"; }
die() { printf '[channel-diff] ERROR: %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

seed_channel_attempt() {
  local label="$1" channel_id="$2"
  local body="$OUT_DIR/${label}-seed.body"
  local headers="$OUT_DIR/${label}-seed.headers"
  local stderr="$OUT_DIR/${label}-seed.stderr"
  local url="${TUNERR_BASE_URL%/}/stream/${channel_id}"
  log "[$label] seeding Tunerr attempt from $url"
  curl -L --silent --show-error --max-time "$SEED_SECONDS" -D "$headers" --output "$body" "$url" \
    >"$OUT_DIR/${label}-seed.writeout" 2>"$stderr" || true
}

snapshot_attempts() {
  local out="$1"
  curl -fsS "${TUNERR_BASE_URL%/}/debug/stream-attempts.json?limit=${ATTEMPT_LIMIT}" -o "$out"
}

infer_attempt_context() {
  local label="$1" channel_id="$2" attempts_json="$3" direct_out="$4" header_out="$5" meta_out="$6"
  python3 - "$label" "$channel_id" "$attempts_json" "$direct_out" "$header_out" "$meta_out" <<'PY'
import json
import sys
from pathlib import Path

label, channel_id, attempts_path, direct_out, header_out, meta_out = sys.argv[1:]
payload = json.loads(Path(attempts_path).read_text(encoding="utf-8", errors="replace"))
rows = payload.get("attempts") if isinstance(payload, dict) else []
match = None
for row in rows or []:
    if str(row.get("channel_id", "")) == channel_id:
        match = row
        break
if match is None:
    raise SystemExit(f"no stream attempt found for channel_id={channel_id}")

upstreams = match.get("upstreams") or []
first = upstreams[0] if upstreams else {}
direct_url = str(match.get("effective_url") or first.get("url") or "").strip()
if not direct_url:
    raise SystemExit(f"matched attempt for channel_id={channel_id} has no effective_url/upstream url")

headers = []
for line in first.get("request_headers") or []:
    line = str(line).strip()
    if not line or ":" not in line:
        continue
    key = line.split(":", 1)[0].strip().lower()
    if key in {"host", "content-length"}:
        continue
    headers.append(line)

Path(direct_out).write_text(direct_url + "\n", encoding="utf-8")
Path(header_out).write_text(("\n".join(headers) + "\n") if headers else "", encoding="utf-8")
Path(meta_out).write_text(json.dumps({
    "label": label,
    "channel_id": channel_id,
    "effective_url": match.get("effective_url"),
    "final_status": match.get("final_status"),
    "final_mode": match.get("final_mode"),
    "duration_ms": match.get("duration_ms"),
    "direct_url": direct_url,
    "header_count": len(headers),
    "upstream_count": len(upstreams),
}, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

run_stream_compare() {
  local label="$1" channel_id="$2" direct_url="$3" direct_headers="$4"
  mkdir -p "$RUNS_DIR"
  log "[$label] running stream-compare against channel_id=$channel_id"
  OUT_ROOT="$RUNS_DIR" \
  RUN_ID="$label" \
  DIRECT_URL="$direct_url" \
  DIRECT_HEADERS_FILE="$direct_headers" \
  TUNERR_BASE_URL="$TUNERR_BASE_URL" \
  CHANNEL_ID="$channel_id" \
  RUN_SECONDS="$RUN_SECONDS" \
  USE_CURL="$USE_CURL" \
  USE_FFPROBE="$USE_FFPROBE" \
  USE_FFPLAY="$USE_FFPLAY" \
  USE_TCPDUMP="$USE_TCPDUMP" \
  FFPLAY_LOGLEVEL="$FFPLAY_LOGLEVEL" \
  ANALYZE_MANIFESTS="$ANALYZE_MANIFESTS" \
  "$ROOT/scripts/stream-compare-harness.sh"
}

write_summary() {
  cat >"$OUT_DIR/summary.txt" <<EOF
Run ID: $RUN_ID
Out Dir: $OUT_DIR
Base URL: $TUNERR_BASE_URL
Good Channel: $GOOD_CHANNEL_ID
Bad Channel: $BAD_CHANNEL_ID
Good Run: $RUNS_DIR/good
Bad Run: $RUNS_DIR/bad

Report:
  python3 "$ROOT/scripts/channel-diff-report.py" --good "$RUNS_DIR/good" --bad "$RUNS_DIR/bad" --print
EOF
}

main() {
  need_cmd bash
  need_cmd curl
  need_cmd python3
  [[ -n "$TUNERR_BASE_URL" ]] || die "TUNERR_BASE_URL is required"
  [[ -n "$GOOD_CHANNEL_ID" ]] || die "GOOD_CHANNEL_ID is required"
  [[ -n "$BAD_CHANNEL_ID" ]] || die "BAD_CHANNEL_ID is required"

  mkdir -p "$OUT_DIR"

  local good_direct="$GOOD_DIRECT_URL"
  local bad_direct="$BAD_DIRECT_URL"

  if [[ -z "$good_direct" ]]; then
    seed_channel_attempt "good" "$GOOD_CHANNEL_ID"
    snapshot_attempts "$OUT_DIR/good-attempts.json"
    infer_attempt_context "good" "$GOOD_CHANNEL_ID" "$OUT_DIR/good-attempts.json" \
      "$OUT_DIR/good-direct.url" "$OUT_DIR/good-direct.headers" "$OUT_DIR/good-attempt.meta.json"
    good_direct="$(tr -d '\r\n' <"$OUT_DIR/good-direct.url")"
  else
    : >"$OUT_DIR/good-direct.headers"
  fi

  if [[ -z "$bad_direct" ]]; then
    seed_channel_attempt "bad" "$BAD_CHANNEL_ID"
    snapshot_attempts "$OUT_DIR/bad-attempts.json"
    infer_attempt_context "bad" "$BAD_CHANNEL_ID" "$OUT_DIR/bad-attempts.json" \
      "$OUT_DIR/bad-direct.url" "$OUT_DIR/bad-direct.headers" "$OUT_DIR/bad-attempt.meta.json"
    bad_direct="$(tr -d '\r\n' <"$OUT_DIR/bad-direct.url")"
  else
    : >"$OUT_DIR/bad-direct.headers"
  fi

  run_stream_compare "good" "$GOOD_CHANNEL_ID" "$good_direct" "$OUT_DIR/good-direct.headers"
  run_stream_compare "bad" "$BAD_CHANNEL_ID" "$bad_direct" "$OUT_DIR/bad-direct.headers"

  python3 "$ROOT/scripts/channel-diff-report.py" \
    --good "$RUNS_DIR/good" \
    --bad "$RUNS_DIR/bad" \
    --out-dir "$OUT_DIR" \
    --print >"$OUT_DIR/report.txt"
  python3 "$ROOT/scripts/channel-diff-report.py" \
    --good "$RUNS_DIR/good" \
    --bad "$RUNS_DIR/bad" \
    --out-dir "$OUT_DIR" \
    --json >"$OUT_DIR/report.json"
  write_summary
  log "Done. Summary: $OUT_DIR/summary.txt"
  log "Report: $OUT_DIR/report.txt"
}

main "$@"
