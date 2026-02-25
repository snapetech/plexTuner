#!/usr/bin/env bash
# Stand up Plex Tuner HDHR in the cluster and verify endpoints (discover, lineup).
# Run from repo root on a host with kubectl and (optional) Docker. Exits 0 only if tuner is up and returning 200.
#
# Prerequisites: Plex (see docs/runbooks/plex-in-cluster.md if missing). Threadfin or M3U URL in manifest.
# Optional: TUNER_BASE_URL=http://plextuner-hdhr.plex.home  (default) or http://<node-ip>:30004 for NodePort.
# If kubectl requires root: sudo ./k8s/standup-and-verify.sh

set -e
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if ! kubectl cluster-info &>/dev/null; then
  echo "[standup] kubectl cannot reach cluster. Run from a host with KUBECONFIG and cluster access (e.g. sudo ./k8s/standup-and-verify.sh if kubectl needs root)." >&2
  exit 1
fi

echo "[standup] Deploying Plex Tuner HDHR ..."
./k8s/deploy.sh "$@"

# Prefer Ingress hostname; fallback NodePort if TUNER_BASE_URL not set and we have a node
BASE="${TUNER_BASE_URL:-http://plextuner-hdhr.plex.home}"
if [[ "$BASE" == "http://plextuner-hdhr.plex.home" ]]; then
  # If user only has NodePort, they must set TUNER_BASE_URL=http://<node-ip>:30004
  NODE_IP=$(kubectl -n plex get svc plextuner-hdhr-test -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)
  if [[ -z "$NODE_IP" ]]; then
    NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || true)
  fi
  if [[ -n "$NODE_IP" ]]; then
    echo "[standup] Ingress may not be ready; you can also verify with: curl -s -o /dev/null -w '%{http_code}' http://${NODE_IP}:30004/discover.json"
  fi
fi

echo "[standup] Verifying tuner endpoints ..."
DISCOVER=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "$BASE/discover.json" 2>/dev/null || echo "000")
LINEUP=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "$BASE/lineup.json" 2>/dev/null || echo "000")

if [[ "$DISCOVER" != "200" || "$LINEUP" != "200" ]]; then
  echo "[standup] Verification failed: discover.json=$DISCOVER lineup.json=$LINEUP (expected 200)." >&2
  echo "[standup] Ensure DNS/host resolves $BASE to the tuner (Ingress or set TUNER_BASE_URL=http://<node-ip>:30004)." >&2
  exit 1
fi

echo "[standup] OK — discover.json and lineup.json returned 200."
echo ""
echo "--- HDHR tuner is up and verified ---"
echo "  Base URL: $BASE"
echo "  Plex: if -register-plex was used and Plex uses the same DB path, open plex.home and Live TV should already be configured."
echo "  Otherwise: Settings → Live TV & DVR → Set up → Base URL: $BASE, Guide: $BASE/guide.xml"
echo ""
