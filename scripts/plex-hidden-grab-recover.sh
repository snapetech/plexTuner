#!/usr/bin/env bash
set -euo pipefail

# Detect a Plex hidden Live TV "active grab" wedge and optionally restart Plex.
#
# Why:
#   Plex can get stuck with hidden active grabs after guide/channel remap work.
#   Symptom in Plex logs:
#     "Subscription: Waiting for media grab to start."
#   while /status/sessions shows no active videos. Tunes then hang / do nothing.
#
# This helper checks for that pattern in Plex logs and only restarts Plex when:
#   1) the hidden-grab log pattern was seen recently
#   2) /status/sessions reports 0 active videos
#
# Usage:
#   ./scripts/plex-hidden-grab-recover.sh --namespace plex --deployment plex --dry-run
#   ./scripts/plex-hidden-grab-recover.sh --restart

NAMESPACE="plex"
DEPLOYMENT="plex"
CONTAINER="plex"
LOOKBACK_MIN=10
DRY_RUN=0
DO_RESTART=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace) NAMESPACE="$2"; shift 2 ;;
    --deployment) DEPLOYMENT="$2"; shift 2 ;;
    --container) CONTAINER="$2"; shift 2 ;;
    --lookback-min) LOOKBACK_MIN="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --restart) DO_RESTART=1; shift ;;
    -h|--help)
      sed -n '1,120p' "$0"
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 2
      ;;
  esac
done

if [[ "$DO_RESTART" -eq 0 && "$DRY_RUN" -eq 0 ]]; then
  DRY_RUN=1
fi

require() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing command: $1" >&2; exit 1; }
}
require kubectl
require python3

pod_exec() {
  kubectl -n "$NAMESPACE" exec "deploy/$DEPLOYMENT" -c "$CONTAINER" -- "$@"
}

PMS_LOG='/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log'

echo "Checking Plex hidden-grab wedge"
echo "  namespace: $NAMESPACE"
echo "  deployment: $DEPLOYMENT"
echo "  lookback: ${LOOKBACK_MIN}m"
echo "  mode: $([[ $DO_RESTART -eq 1 ]] && echo restart || echo dry-run)"

# Read token from Preferences.xml inside Plex container.
TOKEN="$(
  pod_exec sh -lc "sed -n 's/.*PlexOnlineToken=\"\\([^\"]*\\)\".*/\\1/p' \"/config/Library/Application Support/Plex Media Server/Preferences.xml\"" \
    | tr -d '\r\n'
)"
if [[ -z "$TOKEN" ]]; then
  echo "Could not read Plex token from Preferences.xml" >&2
  exit 1
fi

# Count active videos in /status/sessions.
ACTIVE_VIDEOS="$(
  pod_exec curl -fsS "http://127.0.0.1:32400/status/sessions?X-Plex-Token=${TOKEN}" \
    | python3 - <<'PY'
import sys, xml.etree.ElementTree as ET
raw = sys.stdin.read()
root = ET.fromstring(raw)
print(len(root.findall("Video")))
PY
)"

# Detect recent hidden-grab signature in any rotated Plex logs.
HIDDEN_MATCHES="$(
  pod_exec sh -lc "grep -R -n 'Subscription: Waiting for media grab to start\\|There are [0-9][0-9]* active grabs at the end' \
    \"/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server\"* 2>/dev/null | tail -n 40" || true
)"

echo "  active_videos: ${ACTIVE_VIDEOS}"
if [[ -n "$HIDDEN_MATCHES" ]]; then
  echo "  hidden_grab_log_pattern: yes"
else
  echo "  hidden_grab_log_pattern: no"
fi

if [[ -z "$HIDDEN_MATCHES" ]]; then
  echo "No hidden-grab signature found in recent Plex logs."
  exit 0
fi

if [[ "$ACTIVE_VIDEOS" != "0" ]]; then
  echo "Active playback sessions exist; refusing restart."
  echo "$HIDDEN_MATCHES" | tail -n 10
  exit 3
fi

echo "Hidden-grab wedge pattern detected and no active sessions are visible."
echo "$HIDDEN_MATCHES" | tail -n 10

if [[ "$DRY_RUN" -eq 1 && "$DO_RESTART" -eq 0 ]]; then
  echo "Dry run only. Re-run with --restart to restart deploy/$DEPLOYMENT."
  exit 0
fi

echo "Restarting deploy/$DEPLOYMENT..."
kubectl -n "$NAMESPACE" rollout restart "deploy/$DEPLOYMENT"
kubectl -n "$NAMESPACE" rollout status "deploy/$DEPLOYMENT" --timeout=300s
echo "Restart complete."

