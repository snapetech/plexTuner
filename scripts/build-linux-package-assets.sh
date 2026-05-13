#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION="${VERSION:-${1:-${GITHUB_REF_NAME:-}}}"
DIST_DIR="${DIST_DIR:-${2:-dist}}"

err() {
  echo "[build-linux-package-assets] ERROR: $*" >&2
  exit 1
}

require() {
  command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

[[ -n "$VERSION" ]] || err "usage: $0 <version> [dist-dir]"
[[ -d "$DIST_DIR" ]] || err "dist dir not found: $DIST_DIR"
DIST_DIR="$(cd "$DIST_DIR" && pwd)"

PKG_VERSION="${VERSION#v}"
require dpkg-deb
require rpmbuild
require sha256sum
require python3

DEB_ROOT="$DIST_DIR/.pkg/deb"
RPM_TOP="$DIST_DIR/.pkg/rpmbuild"
rm -rf "$DIST_DIR/.pkg"
mkdir -p "$DEB_ROOT/DEBIAN" \
  "$DEB_ROOT/usr/bin" \
  "$DEB_ROOT/lib/systemd/system" \
  "$DEB_ROOT/etc/iptvtunerr" \
  "$DEB_ROOT/usr/lib/sysusers.d" \
  "$DEB_ROOT/usr/lib/tmpfiles.d" \
  "$RPM_TOP"/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

install -Dm755 "$DIST_DIR/iptv-tunerr-${VERSION}-linux-amd64" "$DEB_ROOT/usr/bin/iptv-tunerr"
install -Dm644 packaging/aur/iptvtunerr.service "$DEB_ROOT/lib/systemd/system/iptvtunerr.service"
install -Dm644 packaging/aur/iptvtunerr.env "$DEB_ROOT/etc/iptvtunerr/iptvtunerr.env"
install -Dm644 packaging/aur/iptvtunerr.sysusers "$DEB_ROOT/usr/lib/sysusers.d/iptvtunerr.conf"
install -Dm644 packaging/aur/iptvtunerr.tmpfiles "$DEB_ROOT/usr/lib/tmpfiles.d/iptvtunerr.conf"
install -Dm755 packaging/debian/postinst "$DEB_ROOT/DEBIAN/postinst"

cat > "$DEB_ROOT/DEBIAN/control" <<EOF
Package: iptvtunerr
Version: ${PKG_VERSION}
Section: net
Priority: optional
Architecture: amd64
Maintainer: snapetech <iptvtunerr@proton.me>
Homepage: https://github.com/snapetech/iptvtunerr
Recommends: ffmpeg
Description: IPTV to Plex, Emby, and Jellyfin bridge
 IPTV Tunerr turns IPTV sources into a stable HDHomeRun-style tuner and XMLTV
 guide surface for Plex, Emby, and Jellyfin.
EOF

dpkg-deb --build --root-owner-group "$DEB_ROOT" "$DIST_DIR/iptvtunerr_${PKG_VERSION}_amd64.deb"

cp "$DIST_DIR/iptv-tunerr-${VERSION}-linux-amd64.tar.gz" "$RPM_TOP/SOURCES/"
cp packaging/aur/iptvtunerr.service "$RPM_TOP/SOURCES/"
cp packaging/aur/iptvtunerr.env "$RPM_TOP/SOURCES/"
cp packaging/aur/iptvtunerr.sysusers "$RPM_TOP/SOURCES/iptvtunerr.conf"
cp packaging/aur/iptvtunerr.tmpfiles "$RPM_TOP/SOURCES/"
cp packaging/rpm/iptvtunerr.spec "$RPM_TOP/SPECS/"
sed -i "s/^Version:.*/Version:        ${PKG_VERSION}/" "$RPM_TOP/SPECS/iptvtunerr.spec"
sed -i "s|^Source0:.*|Source0:        iptv-tunerr-${VERSION}-linux-amd64.tar.gz|" "$RPM_TOP/SPECS/iptvtunerr.spec"
rpmbuild -bb --nodeps "$RPM_TOP/SPECS/iptvtunerr.spec" \
  --define "_topdir $RPM_TOP" \
  --define "_unitdir /usr/lib/systemd/system" \
  --define "_sysusersdir /usr/lib/sysusers.d" \
  --define "_tmpfilesdir /usr/lib/tmpfiles.d" \
  --define "_sharedstatedir /var/lib"
find "$RPM_TOP/RPMS" -name '*.rpm' -exec cp {} "$DIST_DIR/" \;

(
  cd "$DIST_DIR"
  sha256sum iptv-tunerr-* iptvtunerr_*.deb *.rpm > SHA256SUMS.txt
)

DIST_DIR="$DIST_DIR" VERSION="$VERSION" python3 - <<'PY'
import hashlib
import json
import os
from pathlib import Path

out = Path(os.environ["DIST_DIR"])
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

echo "[build-linux-package-assets] added .deb/.rpm assets"
