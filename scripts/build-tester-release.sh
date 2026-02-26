#!/usr/bin/env bash
set -euo pipefail

# Build and stage a tester handoff bundle from the cross-platform package archives.
#
# Produces:
#   dist/test-releases/<version>/
#     packages/...
#     examples/...
#     docs/...
#     manifest.json
#     TESTER-README.txt
#
# Environment:
#   VERSION    Optional version label (default: git describe)
#   PLATFORMS  Optional platform matrix passed through to build-test-packages.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require git
require python3

VERSION="${VERSION:-}"
if [[ -z "$VERSION" ]]; then
  if git describe --tags --dirty --always >/dev/null 2>&1; then
    VERSION="$(git describe --tags --dirty --always)"
  else
    VERSION="dev-$(date -u +%Y%m%d%H%M%S)"
  fi
fi
VERSION="${VERSION//\//-}"

COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE_UTC="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "Building tester release bundle"
echo "  version: $VERSION"
echo "  commit:  $COMMIT"
echo "  date:    $DATE_UTC"

VERSION="$VERSION" PLATFORMS="${PLATFORMS:-}" "$ROOT_DIR/scripts/build-test-packages.sh"

PKG_DIR="$ROOT_DIR/dist/test-packages/$VERSION"
REL_DIR="$ROOT_DIR/dist/test-releases/$VERSION"
rm -rf "$REL_DIR"
mkdir -p "$REL_DIR/packages" "$REL_DIR/examples" "$REL_DIR/docs"

cp "$PKG_DIR"/SHA256SUMS.txt "$REL_DIR/packages/"
  cp "$PKG_DIR"/*.zip "$REL_DIR/packages/" 2>/dev/null || true

cp k8s/plextuner-supervisor-multi.example.json "$REL_DIR/examples/"
cp k8s/plextuner-supervisor-singlepod.example.yaml "$REL_DIR/examples/"
cp docs/how-to/package-test-builds.md "$REL_DIR/docs/"
cp docs/reference/testing-and-supervisor-config.md "$REL_DIR/docs/"

cat >"$REL_DIR/TESTER-README.txt" <<EOF
Plex Tuner tester bundle
Version: $VERSION
Commit:  $COMMIT
Built:   $DATE_UTC

This bundle is for binary/supervisor testing.

Supported by packaged binaries (cross-platform):
  - Core tuner paths: run / serve / supervise
  - Plex session reaper (API/SSE mode)
  - XMLTV remap/normalization

Platform-specific limits:
  - Linux: full support (including VODFS mount and HDHR network mode)
  - macOS: no VODFS mount (Linux-only)
  - Windows: no VODFS mount (Linux-only)

Quick start (supervisor test):
  1. Extract a package for your platform from packages/
  2. Copy examples/plextuner-supervisor-multi.example.json beside the binary
  3. Edit child envs/URLs
  4. Run: plex-tuner supervise -config ./plextuner-supervisor-multi.example.json

Verify checksums:
  cd packages && sha256sum -c SHA256SUMS.txt

More docs:
  - docs/package-test-builds.md
  - docs/testing-and-supervisor-config.md
EOF

python3 - <<'PY' "$REL_DIR" "$VERSION" "$COMMIT" "$DATE_UTC"
import json, os, sys
from pathlib import Path

rel = Path(sys.argv[1])
version, commit, built_at = sys.argv[2:5]
packages_dir = rel / "packages"
sha_map = {}
sha_file = packages_dir / "SHA256SUMS.txt"
if sha_file.exists():
    for line in sha_file.read_text().splitlines():
        parts = line.strip().split()
        if len(parts) >= 2:
            sha_map[parts[-1].lstrip("./")] = parts[0]

platform_limits = {
    "linux": {"vodfs_mount": True, "hdhr_network_mode": True},
    "darwin": {"vodfs_mount": False, "hdhr_network_mode": True},
    "windows": {"vodfs_mount": False, "hdhr_network_mode": True},
}

packages = []
for p in sorted(packages_dir.iterdir()):
    if p.name == "SHA256SUMS.txt" or not p.is_file():
        continue
    name = p.name
    base = name[:-4] if name.endswith(".zip") else name
    parts = base.split("_")
    os_name = arch = "unknown"
    if name.endswith("_source.zip"):
        os_name = "source"
        arch = "src"
    elif len(parts) >= 4 and parts[0] == "plex-tuner":
        os_name = parts[-2]
        arch = parts[-1]
    packages.append({
        "file": name,
        "bytes": p.stat().st_size,
        "sha256": sha_map.get(name, ""),
        "os": os_name,
        "arch": arch,
        "feature_limits": platform_limits.get(os_name, {}),
    })

manifest = {
    "kind": "plex-tuner-tester-bundle",
    "version": version,
    "git_commit": commit,
    "built_at_utc": built_at,
    "packages_dir": "packages",
    "examples_dir": "examples",
    "docs_dir": "docs",
    "packages": packages,
    "common_features": [
        "run",
        "serve",
        "supervise",
        "xmltv-remap",
        "plex-session-reaper",
    ],
}
(rel / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
PY

echo
echo "Tester release bundle staged at:"
echo "  $REL_DIR"
echo "Contents:"
find "$REL_DIR" -maxdepth 2 -type f | sed "s#^$REL_DIR/#  #"
