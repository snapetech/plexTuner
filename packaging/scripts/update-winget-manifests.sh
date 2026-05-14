#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-}"
WIN_X64_URL="${2:-}"
WIN_X64_SHA="${3:-}"
RELEASE_TAG="${4:-$VERSION}"

if [[ -z "$VERSION" || -z "$WIN_X64_URL" || -z "$WIN_X64_SHA" ]]; then
  echo "Usage: $0 <version> <win-x64-url> <win-x64-sha> [release-tag]" >&2
  exit 1
fi

WINGET_VERSION="${VERSION#v}"
WINGET_VERSION="${WINGET_VERSION//-/.}"
MANIFEST_DIR="packaging/winget"
FILE_BASENAME="snapetech.iptvtunerr"
INSTALLER_FILE="$MANIFEST_DIR/${FILE_BASENAME}.installer.yaml"
LOCALE_FILE="$MANIFEST_DIR/${FILE_BASENAME}.locale.en-US.yaml"
VERSION_FILE="$MANIFEST_DIR/${FILE_BASENAME}.yaml"
RELEASE_DATE="$(date -u +%Y-%m-%d)"

mkdir -p "$MANIFEST_DIR"

cat > "$INSTALLER_FILE" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.installer.1.6.0.schema.json

PackageIdentifier: snapetech.iptvtunerr
PackageVersion: "$WINGET_VERSION"
InstallerType: zip
ReleaseDate: $RELEASE_DATE
NestedInstallerType: portable
NestedInstallerFiles:
- RelativeFilePath: iptv-tunerr-${RELEASE_TAG}-windows-amd64/iptv-tunerr.exe
  PortableCommandAlias: iptv-tunerr
Commands:
- iptv-tunerr
Installers:
- Architecture: x64
  InstallerUrl: $WIN_X64_URL
  InstallerSha256: $WIN_X64_SHA
ManifestType: installer
ManifestVersion: 1.6.0
EOF

cat > "$LOCALE_FILE" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.defaultLocale.1.6.0.schema.json

PackageIdentifier: snapetech.iptvtunerr
PackageVersion: "$WINGET_VERSION"
PackageLocale: en-US
Publisher: snapetech
PublisherUrl: https://github.com/snapetech
PublisherSupportUrl: https://github.com/snapetech/iptvtunerr/issues
PackageName: IPTV Tunerr
PackageUrl: https://github.com/snapetech/iptvtunerr
License: AGPL-3.0-or-later
LicenseUrl: https://github.com/snapetech/iptvtunerr/blob/main/LICENSE
ShortDescription: IPTV to Plex, Emby, and Jellyfin bridge
Description: |-
  IPTV Tunerr turns IPTV sources into a stable HDHomeRun-style tuner and XMLTV
  guide surface for Plex, Emby, and Jellyfin. It supports live TV, guide shaping,
  stream adaptation, provider failover, and read-only WebDAV VOD access.
Moniker: iptvtunerr
Tags:
  - iptv
  - plex
  - emby
  - jellyfin
  - hdhomerun
  - xmltv
  - live-tv
ReleaseNotes: |-
  Release $VERSION

  See https://github.com/snapetech/iptvtunerr/releases/tag/$RELEASE_TAG for details.
ReleaseNotesUrl: https://github.com/snapetech/iptvtunerr/releases/tag/$RELEASE_TAG
ManifestType: defaultLocale
ManifestVersion: 1.6.0
EOF

cat > "$VERSION_FILE" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.version.1.6.0.schema.json

PackageIdentifier: snapetech.iptvtunerr
PackageVersion: "$WINGET_VERSION"
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
EOF

echo "Updated Winget manifests:"
echo "  - $VERSION_FILE"
echo "  - $INSTALLER_FILE"
echo "  - $LOCALE_FILE"
