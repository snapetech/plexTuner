#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
OUT_ROOT="${OUT_ROOT:-$ROOT/.diag/windows-baremetal-package}"
OUT_DIR="$OUT_ROOT/$RUN_ID"
PKG_DIR="$OUT_DIR/package"

log() { printf '[windows-baremetal-package] %s\n' "$*"; }
fail() { printf '[windows-baremetal-package] ERROR: %s\n' "$*" >&2; exit 1; }

mkdir -p "$PKG_DIR"

log "cross-building windows/amd64 binary"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$PKG_DIR/iptv-tunerr.exe" ./cmd/iptv-tunerr

cp scripts/windows-baremetal-smoke.ps1 "$PKG_DIR/"

cat >"$PKG_DIR/README.txt" <<'TXT'
Windows bare-metal smoke package
================================

Contents:
- iptv-tunerr.exe
- windows-baremetal-smoke.ps1

Usage on a Windows host:

  powershell -ExecutionPolicy Bypass -File .\windows-baremetal-smoke.ps1

Artifacts will be written under:

  .diag\windows-baremetal\<run-id>\

This smoke covers:
- serve startup contract
- web UI login surface
- empty-catalog loading contract
- vod-webdav OPTIONS / PROPFIND / HEAD / range / read-only rejection
TXT

log "package written to $PKG_DIR"
printf '%s\n' "$PKG_DIR"
