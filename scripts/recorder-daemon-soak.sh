#!/usr/bin/env bash
# Soak-test helper: run catchup-daemon with -run-for <duration> plus any extra args.
# Configure catalog/XMLTV via flags or IPTV_TUNERR_BASE_URL / IPTV_TUNERR_CATALOG.
#
# Example:
#   ./scripts/recorder-daemon-soak.sh 45m \
#     -xmltv http://127.0.0.1:5004/guide.xml \
#     -out-dir /tmp/recorder-soak \
#     -stream-base-url http://127.0.0.1:5004 \
#     -once

set -euo pipefail
dur="${1:-30m}"
shift || true
exec iptv-tunerr catchup-daemon -run-for "$dur" "$@"
