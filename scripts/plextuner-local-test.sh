#!/usr/bin/env bash
# Local QA and smoke test for Plex Tuner (no Kubernetes).
# Run from repo root: ./scripts/plextuner-local-test.sh [qa|serve|run|smoke|all]
# See docs/how-to/run-without-kubernetes.md

set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

HOST_DEFAULT="$(hostname -s 2>/dev/null || hostname)"
BASE_URL="${PLEX_TUNER_BASE_URL:-http://${HOST_DEFAULT}:5004}"
ADDR="${PLEX_TUNER_ADDR:-:5004}"
CATALOG_PATH="${PLEX_TUNER_CATALOG_PATH:-./catalog.json}"
WAIT_SECS="${WAIT_SECS:-20}"

load_env() {
  if [[ -f .env ]]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
  fi
}

log() {
  printf '[plex-tuner-test] %s\n' "$*"
}

die() {
  printf '[plex-tuner-test] ERROR: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<EOF
Usage: $0 [qa|serve|run|smoke|zero-touch|all]

Commands:
  qa          Run module download + tuner vet/tests
  serve       Start tuner using existing catalog (foreground)
  run         Start full run mode (foreground; needs provider creds/.env)
  smoke       Smoke-check local endpoints at \$PLEX_TUNER_BASE_URL (default: ${BASE_URL})
  zero-touch  One-shot: run with -register-plex, sync lineup to Plex DB, then keep server up. Requires PLEX_DATA_DIR.
  all         Run qa, start serve/run in background, wait for readiness, smoke-check

Env overrides:
  PLEX_TUNER_BASE_URL     Default: ${BASE_URL}
  PLEX_TUNER_ADDR        Default: ${ADDR}
  PLEX_TUNER_CATALOG_PATH  Default: ${CATALOG_PATH}
  PLEX_DATA_DIR          Plex Media Server data root (required for zero-touch; stop Plex first)
  WAIT_SECS              Default: ${WAIT_SECS}
EOF
}

qa() {
  log "go mod download"
  go mod download

  log "go vet ./internal/tuner/..."
  go vet ./internal/tuner/...

  log "go test -count=1 ./internal/tuner/..."
  go test -count=1 ./internal/tuner/...
}

start_serve() {
  [[ -f "$CATALOG_PATH" ]] || die "catalog not found: $CATALOG_PATH"
  log "starting serve on $ADDR with base-url $BASE_URL (catalog=$CATALOG_PATH)"
  exec go run ./cmd/plex-tuner serve -catalog "$CATALOG_PATH" -addr "$ADDR" -base-url "$BASE_URL"
}

start_run() {
  load_env
  log "starting run on $ADDR with base-url $BASE_URL"
  exec go run ./cmd/plex-tuner run -addr "$ADDR" -base-url "$BASE_URL"
}

smoke() {
  local base="$BASE_URL"
  log "smoke checks against $base"
  local paths=(
    /discover.json
    /device.xml
    /lineup_status.json
    /lineup.json
    /guide.xml
    /live.m3u
  )

  for p in "${paths[@]}"; do
    code="$(curl -sS -o /tmp/plextuner-smoke.out -w '%{http_code}' "${base}${p}" 2>/dev/null || true)"
    printf '%s %s\n' "$code" "$p"
  done

  echo
  log "discover.json"
  curl -sS "${base}/discover.json" 2>/dev/null || true
  echo
}

wait_ready() {
  local base="$BASE_URL"
  local end=$((SECONDS + WAIT_SECS))
  while (( SECONDS < end )); do
    if curl -fsS "${base}/discover.json" >/dev/null 2>&1; then
      log "server is ready"
      return 0
    fi
    sleep 1
  done
  return 1
}

all() {
  qa
  load_env

  local mode
  if [[ -f "$CATALOG_PATH" ]]; then
    mode="serve"
  else
    mode="run"
  fi

  log "starting ${mode} in background"
  if [[ "$mode" == "serve" ]]; then
    go run ./cmd/plex-tuner serve -catalog "$CATALOG_PATH" -addr "$ADDR" -base-url "$BASE_URL" &
  else
    go run ./cmd/plex-tuner run -addr "$ADDR" -base-url "$BASE_URL" &
  fi
  local pid=$!
  trap 'kill "$pid" 2>/dev/null || true' EXIT

  if ! wait_ready; then
    log "server failed readiness check; recent process state:"
    ps -p "$pid" -o pid,ppid,stat,etime,cmd 2>/dev/null || true
    die "readiness timeout waiting for ${BASE_URL}/discover.json"
  fi

  smoke

  cat <<EOF

Plex/browser test target is running at: ${BASE_URL}
Leave this terminal open while testing.
Press Ctrl+C to stop the server.
EOF

  wait "$pid"
}

zero_touch() {
  load_env
  local plex_dir="${PLEX_DATA_DIR:-}"
  [[ -n "$plex_dir" ]] || die "PLEX_DATA_DIR is required for zero-touch (path to Plex Media Server data root; stop Plex first)"
  [[ -d "$plex_dir" ]] || die "PLEX_DATA_DIR is not a directory: $plex_dir"
  [[ -f "$CATALOG_PATH" ]] || die "catalog not found: $CATALOG_PATH (build one with 'run' first or set PLEX_TUNER_CATALOG_PATH)"

  log "zero-touch: register tuner + sync lineup to Plex at $plex_dir (stop Plex first)"
  log "starting run with -register-plex (skip-index, catalog=$CATALOG_PATH)"
  go run ./cmd/plex-tuner run -skip-index -catalog "$CATALOG_PATH" -addr "$ADDR" -base-url "$BASE_URL" -register-plex "$plex_dir" &
  local pid=$!
  trap 'kill "$pid" 2>/dev/null || true' EXIT

  if ! wait_ready; then
    log "server failed readiness; check logs above"
    ps -p "$pid" -o pid,stat,etime,cmd 2>/dev/null || true
    die "readiness timeout"
  fi

  smoke
  local lineup_count
  lineup_count="$(curl -sS "${BASE_URL}/lineup.json" 2>/dev/null | grep -c '"GuideNumber"' || true)"
  [[ -n "$lineup_count" ]] && [[ "$lineup_count" -gt 0 ]] && log "lineup.json has $lineup_count channels"

  cat <<EOF

Zero-touch done. Plex DB updated; lineup synced (no wizard, no 480 cap).
Start Plex and open Live TV. Tuner is at: ${BASE_URL}
Leave this terminal open. Ctrl+C stops the server.
EOF
  wait "$pid"
}

cmd="${1:-all}"
case "$cmd" in
  qa) qa ;;
  serve) start_serve ;;
  run) start_run ;;
  smoke) smoke ;;
  zero-touch) zero_touch ;;
  all) all ;;
  -h|--help|help) usage ;;
  *) usage; exit 2 ;;
esac
