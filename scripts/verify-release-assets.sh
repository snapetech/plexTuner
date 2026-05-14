#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION="${VERSION:-${1:-${GITHUB_REF_NAME:-}}}"
DIST_DIR="${DIST_DIR:-${2:-dist}}"

err() {
  echo "[verify-release-assets] ERROR: $*" >&2
  exit 1
}

[[ -n "$VERSION" ]] || err "usage: $0 <version> [dist-dir]"
[[ -d "$DIST_DIR" ]] || err "dist dir not found: $DIST_DIR"
[[ -f "$DIST_DIR/SHA256SUMS.txt" ]] || err "missing SHA256SUMS.txt"
[[ -f "$DIST_DIR/release-manifest.json" ]] || err "missing release-manifest.json"

expected=(
  "iptv-tunerr-${VERSION}-linux-amd64"
  "iptv-tunerr-${VERSION}-linux-amd64.tar.gz"
  "iptv-tunerr-${VERSION}-linux-arm64"
  "iptv-tunerr-${VERSION}-linux-arm64.tar.gz"
  "iptv-tunerr-${VERSION}-linux-armv7"
  "iptv-tunerr-${VERSION}-linux-armv7.tar.gz"
  "iptv-tunerr-${VERSION}-darwin-amd64"
  "iptv-tunerr-${VERSION}-darwin-amd64.tar.gz"
  "iptv-tunerr-${VERSION}-darwin-arm64"
  "iptv-tunerr-${VERSION}-darwin-arm64.tar.gz"
  "iptv-tunerr-${VERSION}-windows-amd64.exe"
  "iptv-tunerr-${VERSION}-windows-amd64.zip"
  "iptv-tunerr-${VERSION}-windows-arm64.exe"
  "iptv-tunerr-${VERSION}-windows-arm64.zip"
)

for asset in "${expected[@]}"; do
  [[ -f "$DIST_DIR/$asset" ]] || err "missing release asset: $asset"
  grep -Eq "  ${asset//./\\.}$" "$DIST_DIR/SHA256SUMS.txt" || err "missing checksum line for $asset"
done

(cd "$DIST_DIR" && sha256sum -c SHA256SUMS.txt)

tar -tzf "$DIST_DIR/iptv-tunerr-${VERSION}-linux-amd64.tar.gz" | grep -Fx "iptv-tunerr-${VERSION}-linux-amd64/iptv-tunerr" >/dev/null \
  || err "linux tarball does not contain expected binary path"

for arch in amd64 arm64; do
  zip_asset="$DIST_DIR/iptv-tunerr-${VERSION}-windows-${arch}.zip"
  nested_exe="iptv-tunerr-${VERSION}-windows-${arch}/iptv-tunerr.exe"
  unzip -l "$zip_asset" "$nested_exe" >/dev/null 2>&1 \
    || err "windows zip does not contain expected binary path: $nested_exe"
done

for optional_asset in "$DIST_DIR"/iptvtunerr_*.deb "$DIST_DIR"/*.rpm; do
  [[ -e "$optional_asset" ]] || continue
  optional_name="$(basename "$optional_asset")"
  grep -Eq "  ${optional_name//./\\.}$" "$DIST_DIR/SHA256SUMS.txt" || err "missing checksum line for $optional_name"
done

python3 - "$DIST_DIR/release-manifest.json" "${#expected[@]}" <<'PY'
import json
import sys
from pathlib import Path

manifest = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
minimum_count = int(sys.argv[2])
assets = manifest.get("assets", [])
if len(assets) < minimum_count:
    raise SystemExit(f"manifest asset count {len(assets)} < expected minimum {minimum_count}")
for asset in assets:
    if not asset.get("name") or not asset.get("sha256") or not asset.get("size"):
        raise SystemExit(f"manifest asset missing required fields: {asset}")
PY

echo "[verify-release-assets] OK"
