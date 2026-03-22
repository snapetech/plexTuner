---
title: Windows Bare-Metal Smoke
summary: Build a Windows package from Linux and run a local PowerShell smoke on a Windows host.
---

# Windows bare-metal smoke

Use this when you have a real Windows VM or host and want to validate IPTV
Tunerr locally there without waiting for CI or a cluster runner.

## Build the package from Linux

```bash
./scripts/windows-baremetal-package.sh
```

That writes a package under:

```text
.diag/windows-baremetal-package/<run-id>/package/
```

Package contents:

- `iptv-tunerr.exe`
- `windows-baremetal-smoke.ps1`

## Run it on Windows

From PowerShell on the Windows host:

```powershell
powershell -ExecutionPolicy Bypass -File .\windows-baremetal-smoke.ps1
```

The script is self-contained. It:

- starts a local asset server
- runs a real `serve` smoke
- checks the web UI login surface
- verifies the empty-catalog startup contract
- runs a local `vod-webdav` smoke

Artifacts land under:

```text
.diag\windows-baremetal\<run-id>\
```

## Notes

- This path is prepared but not host-validated until it runs on a real Windows
  machine.
- It uses PowerShell-native HTTP requests instead of relying on WSL or curl.

## See also

- [macOS bare-metal smoke](./mac-baremetal-smoke.md)
- [VOD WebDAV client harness](./vod-webdav-client-harness.md)
