---
title: macOS Bare-Metal Smoke
summary: Cross-build IPTV Tunerr, wake a Mac if needed, run real macOS serve/WebDAV smoke, and pull artifacts back.
---

# macOS bare-metal smoke

Use this when you want a real macOS host run without depending on the Kubernetes
Mac node being `Ready`.

The script:

- cross-builds a `darwin/arm64` `iptv-tunerr`
- SSHes to the Mac
- optionally sends Wake-on-LAN magic packets first
- runs a real `serve` startup/web UI smoke
- runs the real `vod-webdav` Finder/MiniRedir request matrix
- copies the resulting artifacts back under `.diag/mac-baremetal/`

## Run

```bash
./scripts/mac-baremetal-smoke.sh
```

Defaults:

- host: `192.168.50.108`
- user: `keith`
- ssh key: `~/.ssh/id_ed25519`
- Wake-on-LAN MAC set: current known MacBook Wi-Fi + Ethernet candidates

## Useful overrides

```bash
MAC_HOST=192.168.50.108 \
MAC_USER=keith \
MAC_SSH_KEY=~/.ssh/id_ed25519 \
WAKE_MACBOOK=true \
./scripts/mac-baremetal-smoke.sh
```

Disable wake attempts:

```bash
WAKE_MACBOOK=false ./scripts/mac-baremetal-smoke.sh
```

Override the candidate Wake-on-LAN MAC list:

```bash
MAC_WOL_MACS='aa:bb:cc:dd:ee:ff,11:22:33:44:55:66' ./scripts/mac-baremetal-smoke.sh
```

## Output

Artifacts land in:

```text
.diag/mac-baremetal/<run-id>/
```

Key files:

- `out/summary.txt`
- `out/serve-full.log`
- `out/serve-empty.log`
- `out/vod-webdav.log`
- `out/vod-webdav-client/mac-selfhost/report.txt`
- `header-diff.txt` when a local baseline bundle exists

## See also

- [VOD WebDAV client harness](./vod-webdav-client-harness.md)
