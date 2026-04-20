#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

HEAD_SHA="${TARGET_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="${TARGET_SHORT_SHA:-${HEAD_SHA:0:7}}"
VERSION="${TARGET_VERSION:-dev-${SHORT_SHA}}"
IMAGE_REPO="${IPTV_TUNERR_CLUSTER_IMAGE_REPO:-localhost/iptvtunerr}"
IMAGE_TAG="${IPTV_TUNERR_CLUSTER_IMAGE_TAG:-cluster-${SHORT_SHA}}"
IMAGE="${IMAGE_REPO}:${IMAGE_TAG}"

echo "[deploy-cluster] sha=${HEAD_SHA} version=${VERSION} image=${IMAGE}"

docker build \
  --build-arg VERSION="${VERSION}" \
  -t "${IMAGE}" \
  .

docker save "${IMAGE}" | sudo k3s ctr -n k8s.io images import -

sudo kubectl apply \
  -f deploy/cluster/plex/iptvtunerr-deployment.yaml \
  -f deploy/cluster/plex/iptvtunerr-sports-deployment.yaml

sudo kubectl -n plex set image deployment/iptvtunerr iptvtunerr="${IMAGE}"
sudo kubectl -n plex set image deployment/iptvtunerr-sports iptvtunerr="${IMAGE}"

sudo kubectl -n plex rollout status deployment/iptvtunerr --timeout=5m
sudo kubectl -n plex rollout status deployment/iptvtunerr-sports --timeout=5m

sudo kubectl -n plex exec deployment/iptvtunerr -- wget -qO- http://127.0.0.1:5004/readyz
sudo kubectl -n plex exec deployment/iptvtunerr-sports -- wget -qO- http://127.0.0.1:5004/readyz
