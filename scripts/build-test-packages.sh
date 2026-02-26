#!/usr/bin/env bash
set -euo pipefail

# Build cross-platform test bundles for plex-tuner (CLI + supervisor examples).
#
# Output:
#   dist/test-packages/<version>/
#     plex-tuner_<version>_source.zip
#     plex-tuner_<version>_<os>_<arch>.zip
#     SHA256SUMS.txt
#
# Usage:
#   ./scripts/build-test-packages.sh
#   VERSION=v0.0.0-test ./scripts/build-test-packages.sh
#   PLATFORMS="linux/amd64 linux/arm64 darwin/arm64" ./scripts/build-test-packages.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require go
require zip
require sha256sum
require git

DEFAULT_PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "linux/arm/v7"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

readarray -t PLATFORMS_ARR < <(
  if [[ -n "${PLATFORMS:-}" ]]; then
    tr ' ' '\n' <<<"${PLATFORMS}" | sed '/^$/d'
  else
    printf '%s\n' "${DEFAULT_PLATFORMS[@]}"
  fi
)

VERSION="${VERSION:-}"
if [[ -z "$VERSION" ]]; then
  if git describe --tags --dirty --always >/dev/null 2>&1; then
    VERSION="$(git describe --tags --dirty --always)"
  else
    VERSION="dev-$(date -u +%Y%m%d%H%M%S)"
  fi
fi
VERSION="${VERSION//\//-}"

DIST_DIR="$ROOT_DIR/dist/test-packages/$VERSION"
STAGE_ROOT="$DIST_DIR/.stage"
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR" "$STAGE_ROOT"

echo "Building plex-tuner test packages"
echo "  version: $VERSION"
echo "  out:     $DIST_DIR"

pkg_name() {
  local os="$1" arch="$2" arm="$3"
  local suffix="$arch"
  if [[ "$arch" == "arm" && -n "$arm" ]]; then
    suffix="armv${arm}"
  fi
  printf 'plex-tuner_%s_%s_%s' "$VERSION" "$os" "$suffix"
}

copy_bundle_files() {
  local target_dir="$1"
  cp README.md "$target_dir/"
  mkdir -p "$target_dir/docs/how-to" "$target_dir/docs/reference" "$target_dir/k8s" "$target_dir/scripts"
  cp docs/how-to/run-without-kubernetes.md "$target_dir/docs/how-to/"
  cp docs/how-to/package-test-builds.md "$target_dir/docs/how-to/"
  cp docs/reference/testing-and-supervisor-config.md "$target_dir/docs/reference/"
  cp k8s/plextuner-supervisor-multi.example.json "$target_dir/k8s/"
  cp k8s/plextuner-supervisor-singlepod.example.yaml "$target_dir/k8s/"
  cp scripts/plex-live-session-drain.py "$target_dir/scripts/"
}

build_source_zip() {
  local out="$1"
  git archive --format=zip --output "$out" --prefix="plex-tuner_${VERSION}_source/" HEAD
}

build_source_zip "$DIST_DIR/plex-tuner_${VERSION}_source.zip"

for spec in "${PLATFORMS_ARR[@]}"; do
  IFS='/' read -r GOOS GOARCH GOARM_REST <<<"$spec"
  GOARM=""
  if [[ "${GOARCH}" == "arm" ]]; then
    GOARM="${GOARM_REST#v}"
    if [[ -z "$GOARM" || "$GOARM" == "$GOARM_REST" && "$GOARM_REST" == "$spec" ]]; then
      GOARM="7"
    fi
  fi

  base="$(pkg_name "$GOOS" "$GOARCH" "$GOARM")"
  stage_dir="$STAGE_ROOT/$base"
  rm -rf "$stage_dir"
  mkdir -p "$stage_dir"

  bin_name="plex-tuner"
  if [[ "$GOOS" == "windows" ]]; then
    bin_name="plex-tuner.exe"
  fi

  echo "  -> $spec"
  env \
    CGO_ENABLED=0 \
    GOOS="$GOOS" \
    GOARCH="$GOARCH" \
    GOARM="${GOARM:-}" \
    go build \
      -trimpath \
      -ldflags="-s -w" \
      -o "$stage_dir/$bin_name" \
      ./cmd/plex-tuner

  copy_bundle_files "$stage_dir"

  (cd "$STAGE_ROOT" && zip -qr "$DIST_DIR/$base.zip" "$base")
done

(cd "$DIST_DIR" && sha256sum ./*.zip > SHA256SUMS.txt)

echo
echo "Done. Generated packages:"
ls -1 "$DIST_DIR" | sed 's/^/  /'
