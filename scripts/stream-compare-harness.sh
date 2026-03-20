#!/usr/bin/env bash
#
# Compare a direct upstream playback path against the equivalent IPTV Tunerr
# path. Collect ffprobe/ffplay logs, curl headers/body previews, and optional
# tcpdump output so operators can diff "direct works" vs "Tunerr fails" runs.
#
# Typical usage:
#   DIRECT_URL='https://upstream.example/path/playlist.m3u8' \
#   TUNERR_BASE_URL='http://127.0.0.1:5004' CHANNEL_ID='espn.us' \
#   USE_TCPDUMP=true ./scripts/stream-compare-harness.sh
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_ROOT="${OUT_ROOT:-$ROOT/.diag/stream-compare}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="$OUT_ROOT/$RUN_ID"
RUN_SECONDS="${RUN_SECONDS:-20}"
USE_CURL="${USE_CURL:-true}"
USE_FFPROBE="${USE_FFPROBE:-true}"
USE_FFPLAY="${USE_FFPLAY:-true}"
USE_TCPDUMP="${USE_TCPDUMP:-false}"
TCPDUMP_IFACE="${TCPDUMP_IFACE:-any}"
TCPDUMP_USE_SUDO="${TCPDUMP_USE_SUDO:-auto}" # auto|true|false
TCPDUMP_FILTER="${TCPDUMP_FILTER:-}"
FFPLAY_BIN="${FFPLAY_BIN:-ffplay}"
FFPROBE_BIN="${FFPROBE_BIN:-ffprobe}"
COMMON_HEADERS_FILE="${COMMON_HEADERS_FILE:-}"
DIRECT_HEADERS_FILE="${DIRECT_HEADERS_FILE:-}"
TUNERR_HEADERS_FILE="${TUNERR_HEADERS_FILE:-}"
DIRECT_URL="${DIRECT_URL:-}"
TUNERR_URL="${TUNERR_URL:-}"
TUNERR_BASE_URL="${TUNERR_BASE_URL:-}"
CHANNEL_ID="${CHANNEL_ID:-}"
FFPROBE_ANALYZE_DURATION_US="${FFPROBE_ANALYZE_DURATION_US:-5000000}"
FFPROBE_PROBE_SIZE="${FFPROBE_PROBE_SIZE:-5000000}"
FFPLAY_LOGLEVEL="${FFPLAY_LOGLEVEL:-verbose}"
FFPLAY_NODISP="${FFPLAY_NODISP:-true}"
FFPLAY_AUTOEXIT="${FFPLAY_AUTOEXIT:-true}"
FFPLAY_INFBUF="${FFPLAY_INFBUF:-false}"
FETCH_TUNERR_ATTEMPTS="${FETCH_TUNERR_ATTEMPTS:-true}"
ANALYZE_MANIFESTS="${ANALYZE_MANIFESTS:-true}"
MANIFEST_REF_LIMIT="${MANIFEST_REF_LIMIT:-40}"

PIDS=()
TCPDUMP_PID=""

log() { printf '[stream-compare] %s\n' "$*"; }
warn() { printf '[stream-compare] WARN: %s\n' "$*" >&2; }
die() { printf '[stream-compare] ERROR: %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

maybe_cmd() {
  command -v "$1" >/dev/null 2>&1
}

cleanup() {
  set +e
  if [[ -n "$TCPDUMP_PID" ]] && kill -0 "$TCPDUMP_PID" 2>/dev/null; then
    kill "$TCPDUMP_PID" 2>/dev/null || sudo -n kill "$TCPDUMP_PID" 2>/dev/null || true
    wait "$TCPDUMP_PID" 2>/dev/null || true
  fi
  for pid in "${PIDS[@]:-}"; do
    if [[ -n "${pid:-}" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT INT TERM

resolve_tunerr_url() {
  if [[ -n "$TUNERR_URL" ]]; then
    printf '%s\n' "$TUNERR_URL"
    return 0
  fi
  if [[ -n "$TUNERR_BASE_URL" && -n "$CHANNEL_ID" ]]; then
    printf '%s/stream/%s\n' "${TUNERR_BASE_URL%/}" "$CHANNEL_ID"
    return 0
  fi
  return 1
}

resolve_tunerr_base_url() {
  if [[ -n "$TUNERR_BASE_URL" ]]; then
    printf '%s\n' "${TUNERR_BASE_URL%/}"
    return 0
  fi
  if [[ -n "$TUNERR_URL" ]]; then
    python3 - "$TUNERR_URL" <<'PY'
import sys
from urllib.parse import urlparse
u = urlparse(sys.argv[1])
if not u.scheme or not u.netloc:
    print("")
else:
    print(f"{u.scheme}://{u.netloc}")
PY
    return 0
  fi
  return 1
}

ensure_target_url() {
  local name="$1" value="$2"
  [[ -n "$value" ]] || die "$name is required"
}

build_header_array() {
  local file="$1"
  local arr_name="$2"
  local -n arr_ref="$arr_name"
  arr_ref=()
  [[ -n "$file" ]] || return 0
  [[ -f "$file" ]] || die "headers file not found: $file"
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -n "$line" ]] || continue
    [[ "$line" =~ ^# ]] && continue
    arr_ref+=("$line")
  done <"$file"
}

build_ff_headers_blob() {
  local output="$1"
  shift
  : >"$output"
  if [[ $# -eq 0 ]]; then
    return 0
  fi
  python3 - "$output" "$@" <<'PY'
import sys
out = sys.argv[1]
headers = sys.argv[2:]
with open(out, "w", encoding="utf-8") as fh:
    for line in headers:
        fh.write(line.rstrip("\r\n"))
        fh.write("\r\n")
PY
}

write_target_meta() {
  local label="$1" url="$2" headers_file="$3" artifact_dir="$4"
  python3 - "$label" "$url" "$headers_file" "$artifact_dir/meta.json" <<'PY'
import json
import sys
payload = {
    "label": sys.argv[1],
    "url": sys.argv[2],
    "headers_file": sys.argv[3],
}
with open(sys.argv[4], "w", encoding="utf-8") as fh:
    json.dump(payload, fh, indent=2, sort_keys=True)
    fh.write("\n")
PY
}

resolve_capture_filter() {
  if [[ -n "$TCPDUMP_FILTER" ]]; then
    printf '%s\n' "$TCPDUMP_FILTER"
    return 0
  fi
  python3 - "$DIRECT_URL" "$RESOLVED_TUNERR_URL" <<'PY'
import ipaddress
import socket
import sys
from urllib.parse import urlparse

ips = []
seen = set()
for raw in sys.argv[1:]:
    if not raw:
        continue
    host = urlparse(raw).hostname
    if not host:
        continue
    try:
        ip = ipaddress.ip_address(host)
        if str(ip) not in seen:
            ips.append(str(ip))
            seen.add(str(ip))
        continue
    except ValueError:
        pass
    try:
        infos = socket.getaddrinfo(host, None, type=socket.SOCK_STREAM)
    except OSError:
        continue
    for info in infos:
        ip = info[4][0]
        if ip not in seen:
            ips.append(ip)
            seen.add(ip)
if not ips:
    print("")
else:
    print(" or ".join(f"host {ip}" for ip in ips))
PY
}

can_sudo_nopasswd() {
  sudo -n true >/dev/null 2>&1
}

start_tcpdump() {
  [[ "$USE_TCPDUMP" == "true" ]] || return 0
  maybe_cmd tcpdump || { warn "tcpdump not found; skipping capture"; return 0; }
  local filter use_sudo="false"
  filter="$(resolve_capture_filter)"
  case "$TCPDUMP_USE_SUDO" in
    true) use_sudo="true" ;;
    false) use_sudo="false" ;;
    auto)
      if can_sudo_nopasswd; then
        use_sudo="true"
      fi
      ;;
    *)
      warn "invalid TCPDUMP_USE_SUDO=$TCPDUMP_USE_SUDO (expected auto|true|false); using auto"
      if can_sudo_nopasswd; then
        use_sudo="true"
      fi
      ;;
  esac
  local pcap="$OUT_DIR/compare.pcap"
  log "Starting tcpdump on $TCPDUMP_IFACE${filter:+ filter=\"$filter\"}"
  if [[ "$use_sudo" == "true" ]]; then
    if [[ -n "$filter" ]]; then
      sudo -n tcpdump -Z "$(id -un)" -i "$TCPDUMP_IFACE" -s 0 -U -w "$pcap" "$filter" >"$OUT_DIR/tcpdump.log" 2>&1 &
    else
      sudo -n tcpdump -Z "$(id -un)" -i "$TCPDUMP_IFACE" -s 0 -U -w "$pcap" >"$OUT_DIR/tcpdump.log" 2>&1 &
    fi
  else
    if [[ -n "$filter" ]]; then
      tcpdump -i "$TCPDUMP_IFACE" -s 0 -U -w "$pcap" "$filter" >"$OUT_DIR/tcpdump.log" 2>&1 &
    else
      tcpdump -i "$TCPDUMP_IFACE" -s 0 -U -w "$pcap" >"$OUT_DIR/tcpdump.log" 2>&1 &
    fi
  fi
  TCPDUMP_PID="$!"
}

run_curl_probe() {
  local label="$1" url="$2" header_blob="$3"
  local artifact_dir="$OUT_DIR/$label"
  local headers_out="$artifact_dir/curl.headers"
  local body_out="$artifact_dir/curl.body"
  local preview_out="$artifact_dir/curl.preview.txt"
  local meta_out="$artifact_dir/curl.meta.json"
  local exit_code=0
  local -a curl_args
  curl_args=(-L -D "$headers_out" --max-time "$RUN_SECONDS" --output "$body_out" --silent --show-error --write-out '%{http_code}\n%{content_type}\n%{size_download}\n%{url_effective}\n')
  if [[ -s "$header_blob" ]]; then
    while IFS= read -r line || [[ -n "$line" ]]; do
      curl_args+=(-H "$line")
    done <"$header_blob"
  fi
  (
    cd "$artifact_dir"
    curl "${curl_args[@]}" "$url"
  ) >"$artifact_dir/curl.writeout" 2>"$artifact_dir/curl.stderr" || exit_code=$?
  python3 - "$body_out" "$preview_out" <<'PY'
import pathlib
import sys
body = pathlib.Path(sys.argv[1]).read_bytes() if pathlib.Path(sys.argv[1]).exists() else b""
sample = body[:4096]
try:
    text = sample.decode("utf-8")
    if all((ch == "\n" or ch == "\r" or ch == "\t" or 32 <= ord(ch) < 127) for ch in text):
        pathlib.Path(sys.argv[2]).write_text(text, encoding="utf-8")
    else:
        raise ValueError
except Exception:
    lines = [" ".join(f"{b:02x}" for b in sample[i:i+16]) for i in range(0, len(sample), 16)]
    pathlib.Path(sys.argv[2]).write_text("\n".join(lines) + ("\n" if lines else ""), encoding="utf-8")
PY
  python3 - "$artifact_dir/curl.writeout" "$exit_code" "$meta_out" <<'PY'
import json
import pathlib
import sys
lines = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8", errors="replace").splitlines()
payload = {
    "exit_code": int(sys.argv[2]),
    "http_code": lines[0] if len(lines) > 0 else "",
    "content_type": lines[1] if len(lines) > 1 else "",
    "size_download": lines[2] if len(lines) > 2 else "",
    "url_effective": lines[3] if len(lines) > 3 else "",
}

analyze_manifest_artifact() {
  local label="$1"
  local artifact_dir="$OUT_DIR/$label"
  [[ "$ANALYZE_MANIFESTS" == "true" ]] || return 0
  [[ -f "$artifact_dir/curl.body" ]] || return 0
  [[ -f "$artifact_dir/curl.meta.json" ]] || return 0
  [[ -f "$artifact_dir/meta.json" ]] || return 0
  python3 "$ROOT/scripts/stream-compare-manifest.py" \
    --body "$artifact_dir/curl.body" \
    --meta "$artifact_dir/meta.json" \
    --curl-meta "$artifact_dir/curl.meta.json" \
    --out "$artifact_dir/manifest.json" \
    --ref-limit "$MANIFEST_REF_LIMIT"
}
with open(sys.argv[3], "w", encoding="utf-8") as fh:
    json.dump(payload, fh, indent=2, sort_keys=True)
    fh.write("\n")
PY
}

run_ffprobe_probe() {
  local label="$1" url="$2" header_blob="$3"
  local artifact_dir="$OUT_DIR/$label"
  local exit_code=0
  local -a args
  args=(-hide_banner -v info -show_error -show_format -show_streams -print_format json -rw_timeout "$FFPROBE_ANALYZE_DURATION_US" -analyzeduration "$FFPROBE_ANALYZE_DURATION_US" -probesize "$FFPROBE_PROBE_SIZE")
  if [[ -s "$header_blob" ]]; then
    args+=(-headers "$(cat "$header_blob")")
  fi
  args+=("$url")
  "$FFPROBE_BIN" "${args[@]}" >"$artifact_dir/ffprobe.json" 2>"$artifact_dir/ffprobe.stderr" || exit_code=$?
  printf '%s\n' "$exit_code" >"$artifact_dir/ffprobe.exit"
}

run_ffplay_probe() {
  local label="$1" url="$2" header_blob="$3"
  local artifact_dir="$OUT_DIR/$label"
  local exit_code=0
  local -a args
  args=(-hide_banner -loglevel "$FFPLAY_LOGLEVEL")
  if [[ "$FFPLAY_AUTOEXIT" == "true" ]]; then
    args+=(-autoexit)
  fi
  if [[ "$FFPLAY_NODISP" == "true" ]]; then
    args+=(-nodisp)
  fi
  if [[ "$FFPLAY_INFBUF" == "true" ]]; then
    args+=(-infbuf)
  fi
  args+=(-t "$RUN_SECONDS")
  if [[ -s "$header_blob" ]]; then
    args+=(-headers "$(cat "$header_blob")")
  fi
  args+=(-i "$url")
  "$FFPLAY_BIN" "${args[@]}" >"$artifact_dir/ffplay.stdout" 2>"$artifact_dir/ffplay.stderr" || exit_code=$?
  printf '%s\n' "$exit_code" >"$artifact_dir/ffplay.exit"
}

run_target() {
  local label="$1" url="$2" headers_file="$3"
  local artifact_dir="$OUT_DIR/$label"
  mkdir -p "$artifact_dir"
  write_target_meta "$label" "$url" "$headers_file" "$artifact_dir"

  local -a common_headers target_headers combined_headers
  build_header_array "$COMMON_HEADERS_FILE" common_headers
  build_header_array "$headers_file" target_headers
  combined_headers=("${common_headers[@]}" "${target_headers[@]}")
  build_ff_headers_blob "$artifact_dir/http.headers" "${combined_headers[@]}"

  if [[ "$USE_CURL" == "true" ]]; then
    log "[$label] curl $url"
    run_curl_probe "$label" "$url" "$artifact_dir/http.headers"
    analyze_manifest_artifact "$label"
  fi
  if [[ "$USE_FFPROBE" == "true" ]]; then
    log "[$label] ffprobe $url"
    run_ffprobe_probe "$label" "$url" "$artifact_dir/http.headers"
  fi
  if [[ "$USE_FFPLAY" == "true" ]]; then
    log "[$label] ffplay $url"
    run_ffplay_probe "$label" "$url" "$artifact_dir/http.headers"
  fi
}

write_summary() {
  local summary="$OUT_DIR/summary.txt"
  {
    echo "Run ID: $RUN_ID"
    echo "Out Dir: $OUT_DIR"
    echo "Date: $(date -Is)"
    echo
    echo "Targets:"
    echo "  direct: $DIRECT_URL"
    echo "  tunerr: $RESOLVED_TUNERR_URL"
    echo
    echo "Commands enabled:"
    echo "  curl:    $USE_CURL"
    echo "  ffprobe: $USE_FFPROBE"
    echo "  ffplay:  $USE_FFPLAY"
    echo "  tcpdump: $USE_TCPDUMP"
    echo
    echo "Artifacts:"
    echo "  direct/: curl, ffprobe, ffplay logs"
    echo "  tunerr/: curl, ffprobe, ffplay logs"
    if [[ "$ANALYZE_MANIFESTS" == "true" ]]; then
      echo "  */manifest.json: parsed M3U8/MPD references, including decoded Tunerr seg targets when present"
    fi
    [[ -f "$OUT_DIR/compare.pcap" ]] && echo "  compare.pcap: open in Wireshark/tshark"
    echo "  report.txt / report.json: compact comparison summary"
    echo
    echo "Suggested next steps:"
    echo "  1) Compare direct/curl.meta.json vs tunerr/curl.meta.json"
    echo "  2) Compare direct/ffplay.stderr vs tunerr/ffplay.stderr"
    echo "  3) Inspect */manifest.json when the body is HLS or DASH; it decodes Tunerr seg targets into redacted upstream URLs"
    echo "  4) If packet capture is enabled, open compare.pcap in Wireshark and filter on the direct or Tunerr host"
    echo "  5) If Tunerr fails but direct succeeds, correlate with iptv-tunerr logs from the same wall-clock window"
  } >"$summary"
}

fetch_tunerr_attempts() {
  [[ "$FETCH_TUNERR_ATTEMPTS" == "true" ]] || return 0
  local base
  base="$(resolve_tunerr_base_url || true)"
  [[ -n "$base" ]] || return 0
  curl -fsS "$base/debug/stream-attempts.json?limit=10" >"$OUT_DIR/tunerr/stream-attempts.json" 2>"$OUT_DIR/tunerr/stream-attempts.stderr" || {
    warn "failed to fetch Tunerr stream attempts from $base/debug/stream-attempts.json"
    return 0
  }
}

main() {
  need_cmd bash
  need_cmd python3
  need_cmd curl
  [[ "$USE_FFPROBE" == "true" ]] && need_cmd "$FFPROBE_BIN"
  [[ "$USE_FFPLAY" == "true" ]] && need_cmd "$FFPLAY_BIN"

  RESOLVED_TUNERR_URL="$(resolve_tunerr_url || true)"
  ensure_target_url "DIRECT_URL" "$DIRECT_URL"
  ensure_target_url "Tunerr URL (set TUNERR_URL or TUNERR_BASE_URL + CHANNEL_ID)" "$RESOLVED_TUNERR_URL"

  mkdir -p "$OUT_DIR"
  log "Output dir: $OUT_DIR"
  start_tcpdump
  run_target "direct" "$DIRECT_URL" "$DIRECT_HEADERS_FILE"
  run_target "tunerr" "$RESOLVED_TUNERR_URL" "$TUNERR_HEADERS_FILE"
  fetch_tunerr_attempts
  if [[ -n "$TCPDUMP_PID" ]]; then
    sleep 1
  fi
  write_summary
  python3 "$ROOT/scripts/stream-compare-report.py" --dir "$OUT_DIR" >"$OUT_DIR/report.txt"
  python3 "$ROOT/scripts/stream-compare-report.py" --dir "$OUT_DIR" --json >"$OUT_DIR/report.json"
  log "Done. Summary: $OUT_DIR/summary.txt"
  log "Report: $OUT_DIR/report.txt"
}

main "$@"
