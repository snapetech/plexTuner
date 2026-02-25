#!/usr/bin/env bash
# Deploy Plex Tuner HDHR to the cluster: build image, load (kind/k3d), apply, wait.
# Usage: from repo root, ./k8s/deploy.sh [--no-build] [--no-load] [--static]
# Optional env: MANIFEST=/path/to/manifest.yaml (defaults to k8s/plextuner-hdhr-test.yaml)
# Requires: docker, kubectl, cluster with plex namespace and (optionally) Ingress.

set -e
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
IMAGE="${IMAGE:-plex-tuner:hdhr-test}"
NAMESPACE="${NAMESPACE:-plex}"
MANIFEST="${MANIFEST:-k8s/plextuner-hdhr-test.yaml}"

do_build=1
do_load=1
use_static=0
for arg in "$@"; do
  case "$arg" in
    --no-build)  do_build=0 ;;
    --no-load)   do_load=0 ;;
    --static)    use_static=1 ;;
  esac
done

echo "[deploy] Image: $IMAGE  Namespace: $NAMESPACE"

if [ "$do_build" -eq 1 ]; then
  if [ "$use_static" -eq 1 ]; then
    echo "[deploy] Building binary and image (Dockerfile.static.scratch, no network in Docker) ..."
    CGO_ENABLED=0 go build -o plex-tuner ./cmd/plex-tuner || exit 1
    docker build -f Dockerfile.static.scratch -t "$IMAGE" .
  else
    echo "[deploy] Building Docker image ..."
    docker build -t "$IMAGE" .
  fi
fi

if [ "$do_load" -eq 1 ]; then
  ctx=$(kubectl config current-context 2>/dev/null || true)
  if [[ "$ctx" == kind-* ]]; then
    echo "[deploy] Loading image into kind ..."
    kind load docker-image "$IMAGE"
  elif [[ "$ctx" == k3d-* ]]; then
    echo "[deploy] Loading image into k3d ..."
    k3d image import "$IMAGE"
  else
    echo "[deploy] Skipping image load (context $ctx is not kind/k3d; push image to your registry if needed)"
  fi
fi

echo "[deploy] Ensuring namespace $NAMESPACE ..."
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

echo "[deploy] Applying $MANIFEST ..."
kubectl apply -f "$MANIFEST"

echo "[deploy] Waiting for deployment plextuner-hdhr-test ..."
kubectl -n "$NAMESPACE" rollout status deployment/plextuner-hdhr-test --timeout=300s

echo ""
echo "--- Plex Tuner HDHR is up ---"
echo "  Pods:   kubectl -n $NAMESPACE get pods -l app=plextuner-hdhr-test"
echo "  Logs:   kubectl -n $NAMESPACE logs -l app=plextuner-hdhr-test -f"
echo ""
echo "  If using Ingress (plextuner-hdhr.plex.home):"
echo "    curl -s -o /dev/null -w '%{http_code}' http://plextuner-hdhr.plex.home/discover.json   # expect 200"
echo "  NodePort fallback: <node-ip>:30004"
echo "    curl -s http://<node-ip>:30004/discover.json"
echo ""
echo "  Plex: Settings → Live TV & DVR → Set up"
echo "    Device/Base URL: http://plextuner-hdhr.plex.home"
echo "    XMLTV guide URL: http://plextuner-hdhr.plex.home/guide.xml"
echo ""
