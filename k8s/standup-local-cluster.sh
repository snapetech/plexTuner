#!/usr/bin/env bash
# Deploy Plex Tuner into your local cluster, connect to Plex at plex.home, populate Live TV.
# Run from repo root. Uses .env for provider creds. Tuner will index at startup, then -register-plex into /var/lib/plex.
#
# Prereqs:
#   - kubectl pointing at your local cluster (kind, k3d, etc.)
#   - Plex Media Server in the cluster (or on a node) with data at /var/lib/plex on the node
#   - DNS: plextuner-hdhr.plex.home → Ingress (or set TUNER_BASE_URL for NodePort)
#   - If Plex runs on a specific node, set PLEX_NODE_NAME and uncomment nodeSelector in k8s/plextuner-hdhr-test.yaml
#
# Usage:
#   ./k8s/standup-local-cluster.sh [--static]
#   TUNER_BASE_URL=http://<node-ip>:30004 ./k8s/standup-local-cluster.sh   # if no Ingress

set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [[ ! -f .env ]]; then
  echo "[standup-local] ERROR: .env not found. Copy .env.example to .env and set PLEX_TUNER_PROVIDER_* (and PLEX_TUNER_BASE_URL if needed)." >&2
  exit 1
fi

echo "[standup-local] Deploying Plex Tuner to local cluster (base URL: plextuner-hdhr.plex.home, -register-plex to populate Plex Live TV) ..."
./k8s/deploy-hdhr-one-shot.sh "$@"

echo "[standup-local] Verifying tuner endpoints ..."
BASE="${TUNER_BASE_URL:-http://plextuner-hdhr.plex.home}"
if curl -sS -o /dev/null -w "%{http_code}" "$BASE/discover.json" | grep -q 200; then
  echo "[standup-local] discover.json OK"
else
  echo "[standup-local] WARN: $BASE/discover.json not 200 (is DNS/Ingress set up?)" >&2
fi
if curl -sS -o /dev/null -w "%{http_code}" "$BASE/lineup.json" | grep -q 200; then
  echo "[standup-local] lineup.json OK"
else
  echo "[standup-local] WARN: $BASE/lineup.json not 200" >&2
fi

echo ""
echo "--- Plex Tuner is up in the cluster ---"
echo "  Tuner URL:  $BASE"
echo "  Plex: open plex.home → Live TV should already be populated (we wrote to Plex DB at -register-plex=/var/lib/plex)."
echo "  If Plex is on a specific node, ensure that node has /var/lib/plex and set nodeSelector in k8s/plextuner-hdhr-test.yaml to that node."
echo "  Logs: kubectl -n plex logs -l app=plextuner-hdhr-test -f"
echo ""
