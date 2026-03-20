#!/usr/bin/env bash
# Optional soak: hit Tunerr ?mux=hls in a loop (requires ffplay or curl).
# Usage: HLS_MUX_URL='http://127.0.0.1:5004/stream/0?mux=hls' ./scripts/hls-mux-soak.sh
set -euo pipefail
URL="${HLS_MUX_URL:-http://127.0.0.1:5004/stream/0?mux=hls}"
CONCURRENCY="${HLS_MUX_CONCURRENCY:-4}"
DURATION_SEC="${HLS_MUX_DURATION:-30}"

echo "Soak: URL=$URL concurrency=$CONCURRENCY duration=${DURATION_SEC}s"

end=$((SECONDS + DURATION_SEC))
worker() {
  local id="$1"
  while (( SECONDS < end )); do
    if command -v curl >/dev/null 2>&1; then
      curl -sS -m 5 -o /dev/null "$URL" || echo "worker $id: curl fail"
    fi
    sleep 0.2
  done
}

for ((i = 0; i < CONCURRENCY; i++)); do
  worker "$i" &
done
wait
echo "Soak done."
