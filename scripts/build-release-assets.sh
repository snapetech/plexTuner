#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[build-release-assets] missing required command: $1" >&2
    exit 1
  }
}

require go
require tar
require zip
require sha256sum
require python3

VERSION="${VERSION:-${1:-${GITHUB_REF_NAME:-}}}"
OUT_DIR="${OUT_DIR:-${2:-dist}}"

if [[ -z "$VERSION" ]]; then
  echo "usage: VERSION=vX.Y.Z $0 [version] [out-dir]" >&2
  exit 1
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z]+)*$ ]]; then
  echo "[build-release-assets] version must look like vX.Y.Z; got '$VERSION'" >&2
  exit 1
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/.stage"

LDFLAGS="-s -w -X main.Version=${VERSION}"
PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "linux/arm/v7"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

asset_suffix() {
  local os="$1" arch="$2" arm="$3"
  if [[ "$arch" == "arm" && -n "$arm" ]]; then
    printf '%s-armv%s' "$os" "$arm"
  else
    printf '%s-%s' "$os" "$arch"
  fi
}

copy_context_files() {
  local target="$1"
  cp README.md LICENSE "$target/"
  mkdir -p "$target/docs/how-to" "$target/docs/reference"
  cp docs/how-to/deployment.md "$target/docs/how-to/"
  cp docs/reference/cli-and-env-reference.md "$target/docs/reference/"
}

for spec in "${PLATFORMS[@]}"; do
  IFS='/' read -r goos goarch goarm_rest <<<"$spec"
  goarm=""
  if [[ "$goarch" == "arm" ]]; then
    goarm="${goarm_rest#v}"
  fi

  suffix="$(asset_suffix "$goos" "$goarch" "$goarm")"
  base="iptv-tunerr-${VERSION}-${suffix}"
  stage="$OUT_DIR/.stage/$base"
  mkdir -p "$stage"

  bin_name="iptv-tunerr"
  raw_asset="$base"
  if [[ "$goos" == "windows" ]]; then
    bin_name="iptv-tunerr.exe"
    raw_asset="${base}.exe"
  fi

  echo "[build-release-assets] building $spec"
  env \
    CGO_ENABLED=0 \
    GOOS="$goos" \
    GOARCH="$goarch" \
    GOARM="${goarm:-}" \
    go build -mod=vendor -trimpath -ldflags="$LDFLAGS" -o "$stage/$bin_name" ./cmd/iptv-tunerr

  chmod 0755 "$stage/$bin_name"
  cp "$stage/$bin_name" "$OUT_DIR/$raw_asset"
  copy_context_files "$stage"

  if [[ "$goos" == "windows" ]]; then
    (cd "$OUT_DIR/.stage" && zip -qr "../${base}.zip" "$base")
  else
    (cd "$OUT_DIR/.stage" && tar -czf "../${base}.tar.gz" "$base")
  fi
done

(
  cd "$OUT_DIR"
  sha256sum iptv-tunerr-* > SHA256SUMS.txt
)

OUT_DIR="$OUT_DIR" VERSION="$VERSION" python3 - <<'PY'
import hashlib
import json
import os
from pathlib import Path

out = Path(os.environ["OUT_DIR"])
version = os.environ["VERSION"]
assets = []
for path in sorted(out.iterdir()):
    if not path.is_file() or path.name in {"SHA256SUMS.txt", "release-manifest.json", "release-notes.md"}:
        continue
    assets.append({
        "name": path.name,
        "size": path.stat().st_size,
        "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
    })

(out / "release-manifest.json").write_text(
    json.dumps({"version": version, "assets": assets}, indent=2) + "\n",
    encoding="utf-8",
)
PY

echo "[build-release-assets] wrote $OUT_DIR"
ls -1 "$OUT_DIR" | sed 's/^/  /'
