#!/usr/bin/env bash
#
# Exercise the read-only vod-webdav surface using request shapes that mimic
# macOS Finder/WebDAVFS and Windows WebDAV MiniRedir clients.
#
# Default mode is self-contained:
#   - builds a temp iptv-tunerr binary
#   - serves temp movie/episode assets over local HTTP
#   - starts `iptv-tunerr vod-webdav`
#   - runs the request matrix and writes a report bundle under .diag/
#
# External mode:
#   BASE_URL=http://127.0.0.1:58188 ./scripts/vod-webdav-client-harness.sh
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_ROOT="${OUT_ROOT:-$ROOT/.diag/vod-webdav-client}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="$OUT_ROOT/$RUN_ID"
TMP_DIR="$OUT_DIR/work"
BIN="$TMP_DIR/iptv-tunerr"
BASE_URL="${BASE_URL:-}"
KEEP_WORKDIR="${KEEP_WORKDIR:-false}"

PIDS=()

log() { printf '[vod-webdav-harness] %s\n' "$*"; }
warn() { printf '[vod-webdav-harness] WARN: %s\n' "$*" >&2; }
die() { printf '[vod-webdav-harness] ERROR: %s\n' "$*" >&2; exit 1; }

pick_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

wait_http() {
  local url="$1" want="${2:-200}" attempts="${3:-50}"
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

cleanup() {
  set +e
  for pid in "${PIDS[@]:-}"; do
    if [[ -n "${pid:-}" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  if [[ "$KEEP_WORKDIR" != "true" ]]; then
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT INT TERM

mkdir -p "$OUT_DIR" "$TMP_DIR/assets" "$OUT_DIR/steps"

movie_path='/Movies/Live:%20Smoke%20Movie%20%282024%29/Live:%20Smoke%20Movie%20%282024%29.mp4'
episode_path='/TV/Live:%20Smoke%20Show%20%282023%29/Season%2001/Live:%20Smoke%20Show%20%282023%29%20-%20s01e01%20-%20Pilot.mp4'

if [[ -z "$BASE_URL" ]]; then
  log "building temp binary"
  go build -o "$BIN" ./cmd/iptv-tunerr

  printf 'movie-bytes' >"$TMP_DIR/assets/movie.bin"
  printf 'episode-bytes' >"$TMP_DIR/assets/episode.bin"

  asset_port="$(pick_port)"
  cat >"$TMP_DIR/catalog-vod.json" <<JSON
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

  python3 -m http.server "$asset_port" --bind 127.0.0.1 --directory "$TMP_DIR/assets" >"$OUT_DIR/assets.log" 2>&1 &
  PIDS+=("$!")
  wait_http "http://127.0.0.1:${asset_port}/movie.bin" 200 || die "asset server failed to start"

  vod_port="$(pick_port)"
  "$BIN" vod-webdav -catalog "$TMP_DIR/catalog-vod.json" -addr "127.0.0.1:${vod_port}" -cache "$TMP_DIR/cache" >"$OUT_DIR/vod-webdav.log" 2>&1 &
  PIDS+=("$!")
  BASE_URL="http://127.0.0.1:${vod_port}"
  wait_http "${BASE_URL}/" 405 || die "vod-webdav failed to start"
fi

BASE_URL="${BASE_URL%/}"
log "capturing against ${BASE_URL}"

cat >"$TMP_DIR/matrix.json" <<JSON
[
  {
    "id": "darwin_options_root",
    "method": "OPTIONS",
    "path": "/",
    "user_agent": "WebDAVFS/3.0 (03008000) Darwin/24.0.0",
    "expect_status": 200
  },
  {
    "id": "darwin_propfind_root",
    "method": "PROPFIND",
    "path": "/",
    "user_agent": "WebDAVFS/3.0 (03008000) Darwin/24.0.0",
    "content_type": "text/xml",
    "depth": "1",
    "body": "<propfind xmlns=\"DAV:\"><allprop/></propfind>",
    "expect_status": 207
  },
  {
    "id": "windows_propfind_movies",
    "method": "PROPFIND",
    "path": "/Movies",
    "user_agent": "Microsoft-WebDAV-MiniRedir/10.0.19045",
    "content_type": "text/xml",
    "depth": "1",
    "body": "<a:propfind xmlns:a=\"DAV:\"><a:allprop/></a:propfind>",
    "expect_status": 207
  },
  {
    "id": "darwin_propfind_episode_file",
    "method": "PROPFIND",
    "path": "${episode_path}",
    "user_agent": "WebDAVFS/3.0 (03008000) Darwin/24.0.0",
    "content_type": "text/xml",
    "depth": "0",
    "body": "<a:propfind xmlns:a=\"DAV:\"><a:allprop/></a:propfind>",
    "expect_status": 207
  },
  {
    "id": "darwin_head_movie",
    "method": "HEAD",
    "path": "${movie_path}",
    "user_agent": "WebDAVFS/3.0 (03008000) Darwin/24.0.0",
    "expect_status": 200
  },
  {
    "id": "windows_range_movie",
    "method": "GET",
    "path": "${movie_path}",
    "user_agent": "Microsoft-WebDAV-MiniRedir/10.0.19045",
    "range": "bytes=0-4",
    "expect_status": 206
  },
  {
    "id": "windows_head_episode",
    "method": "HEAD",
    "path": "${episode_path}",
    "user_agent": "Microsoft-WebDAV-MiniRedir/10.0.19045",
    "expect_status": 200
  },
  {
    "id": "windows_range_episode",
    "method": "GET",
    "path": "${episode_path}",
    "user_agent": "Microsoft-WebDAV-MiniRedir/10.0.19045",
    "range": "bytes=0-6",
    "expect_status": 206
  },
  {
    "id": "windows_put_rejected",
    "method": "PUT",
    "path": "${movie_path}",
    "user_agent": "Microsoft-WebDAV-MiniRedir/10.0.19045",
    "content_type": "application/octet-stream",
    "body": "bad",
    "expect_status": 405
  }
]
JSON

python3 - "$BASE_URL" "$TMP_DIR/matrix.json" "$OUT_DIR" <<'PY'
import json
import pathlib
import subprocess
import sys

base_url = sys.argv[1]
matrix_path = pathlib.Path(sys.argv[2])
out_dir = pathlib.Path(sys.argv[3])
steps_dir = out_dir / "steps"
steps_dir.mkdir(parents=True, exist_ok=True)

matrix = json.loads(matrix_path.read_text(encoding="utf-8"))
results = []

for step in matrix:
    step_id = step["id"]
    body_path = steps_dir / f"{step_id}.body"
    headers_path = steps_dir / f"{step_id}.headers"
    cmd = [
        "curl", "-sS", "--max-time", "15",
        "-D", str(headers_path),
        "-o", str(body_path),
        "-w", "%{http_code}",
        f"{base_url}{step['path']}",
    ]
    if step["method"] == "HEAD":
        cmd.append("--head")
    else:
        cmd.extend(["-X", step["method"]])
    if step.get("user_agent"):
        cmd.extend(["-H", f"User-Agent: {step['user_agent']}"])
    if step.get("depth"):
        cmd.extend(["-H", f"Depth: {step['depth']}"])
    if step.get("content_type"):
        cmd.extend(["-H", f"Content-Type: {step['content_type']}"])
    if step.get("range"):
        cmd.extend(["-H", f"Range: {step['range']}"])
    if "body" in step:
        cmd.extend(["--data-binary", step["body"]])

    proc = subprocess.run(cmd, capture_output=True, text=True)
    status = proc.stdout.strip() if proc.stdout else ""
    step_result = {
        "id": step_id,
        "method": step["method"],
        "path": step["path"],
        "expected_status": step["expect_status"],
        "status": int(status) if status.isdigit() else None,
        "ok": status.isdigit() and int(status) == int(step["expect_status"]),
        "stderr": proc.stderr.strip(),
        "headers_file": str(headers_path.relative_to(out_dir)),
        "body_file": str(body_path.relative_to(out_dir)),
    }
    results.append(step_result)

report = {
    "base_url": base_url,
    "step_count": len(results),
    "ok": all(item["ok"] for item in results),
    "results": results,
}
(out_dir / "report.json").write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY

python3 scripts/vod-webdav-client-report.py --dir "$OUT_DIR" --print >"$OUT_DIR/report.txt"
log "artifacts written to $OUT_DIR"
cat "$OUT_DIR/report.txt"
