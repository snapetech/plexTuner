---
id: platform-requirements
type: how-to
status: stable
tags: [how-to, install, windows, macos, linux, ffmpeg]
---

# Platform requirements and installation

iptvTunerr cross-compiles and runs on Linux, macOS, and Windows. The core tuner (`run`, `supervise`, `index`, `probe`) works on all three. A few optional features have per-platform dependencies.

---

## Supported platforms

| Platform | Architecture | Core tuner | `mount` command | SIGHUP reload |
|----------|-------------|------------|-----------------|---------------|
| Linux | amd64, arm64 | ✓ | ✓ (requires libfuse) | ✓ |
| macOS | amd64, arm64 (Apple Silicon) | ✓ | ✓ (requires macFUSE) | ✓ |
| Windows | amd64 | ✓ | ✗ (no FUSE driver) | no-op (signal never fires; use `-refresh` flag for scheduled reload instead) |

---

## FFmpeg (strongly recommended)

FFmpeg is used for:
- MPEG-TS normalization — fixes raw streams that Plex/Emby/Jellyfin reject
- HLS-to-MPEG-TS remux — required for HLS streams
- Transcoding (`IPTV_TUNERR_STREAM_TRANSCODE=on`)

The app falls back to a raw Go relay if `ffmpeg` is not found in `PATH`, but HLS streams will not work and raw TS streams may be rejected by media clients. Install ffmpeg for a reliable experience.

### Linux

```bash
# Debian / Ubuntu
sudo apt-get install ffmpeg

# Fedora / RHEL
sudo dnf install ffmpeg

# Alpine (inside Docker)
apk add ffmpeg
```

### macOS

```bash
brew install ffmpeg
```

### Windows

1. Download a Windows build from [ffmpeg.org/download.html](https://ffmpeg.org/download.html) (e.g. the gyan.dev release).
2. Extract to a folder, e.g. `C:\ffmpeg\bin`.
3. Add that folder to your system `PATH`:
   - Search "Edit the system environment variables" → Environment Variables → Path → New → `C:\ffmpeg\bin`
4. Verify: open a new terminal and run `ffmpeg -version`.

Alternatively, set `IPTV_TUNERR_FFMPEG_PATH=C:\ffmpeg\bin\ffmpeg.exe` to point directly at the binary without modifying `PATH`.

---

## Getting the binary

### Build from source (all platforms)

Requires Go 1.22+.

```bash
# Linux / macOS
go build -o iptv-tunerr ./cmd/iptv-tunerr

# Windows (PowerShell)
go build -o iptv-tunerr.exe ./cmd/iptv-tunerr
```

### Cross-compile

```bash
# Linux ARM64 (e.g. Raspberry Pi, Oracle Ampere)
GOOS=linux GOARCH=arm64 go build -o iptv-tunerr-linux-arm64 ./cmd/iptv-tunerr

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o iptv-tunerr-darwin-arm64 ./cmd/iptv-tunerr

# Windows
GOOS=windows GOARCH=amd64 go build -o iptv-tunerr-windows-amd64.exe ./cmd/iptv-tunerr
```

### Docker (Linux, multi-arch)

```bash
# Build and run locally
docker build --network=host -t iptv-tunerr:local .
docker run -p 5004:5004 --env-file .env iptv-tunerr:local

# Multi-arch image (linux/amd64 + linux/arm64)
docker buildx build --platform linux/amd64,linux/arm64 -t iptv-tunerr:latest --push .
```

---

## `mount` command (FUSE-based VOD filesystem)

The `mount` command exposes the VOD catalog as a local filesystem via FUSE.

### Linux

```bash
# Debian / Ubuntu
sudo apt-get install fuse3

# Fedora / RHEL
sudo dnf install fuse3
```

### macOS

Install [macFUSE](https://osxfuse.github.io/) (free, open-source):
```bash
brew install --cask macfuse
```
Note: macFUSE requires a kernel extension. On Apple Silicon (M1/M2/M3) you must enable it in System Settings → Privacy & Security after the first install.

### Windows

FUSE is not natively available on Windows. The `mount` command will fail at runtime with an unsupported error. Use the tuner's HTTP endpoints (`/lineup.json`, `/stream/`) directly instead.

---

## Platform-specific notes

### Windows path separators

All flag values that take file paths (e.g. `-catalog`, `-emby-state-file`) accept either forward or back slashes. Forward slashes work in PowerShell and cmd.exe:

```powershell
.\iptv-tunerr.exe run -addr :5004 -catalog C:/data/catalog.json
```

### Windows: catalog refresh without SIGHUP

On Linux/macOS you can send `SIGHUP` to reload the catalog without restarting. On Windows, use the `-refresh` flag for scheduled reload instead:

```powershell
.\iptv-tunerr.exe run -addr :5004 -refresh 6h
```

### macOS: network binding

macOS restricts binding to ports below 1024. Port 5004 is above that threshold and works without `sudo`. If you need a different port, no special configuration is required as long as it is above 1023.

### Linux: firewall

Open port 5004 (TCP) so Plex/Emby/Jellyfin can reach the tuner:

```bash
# ufw
sudo ufw allow 5004/tcp

# firewalld
sudo firewall-cmd --add-port=5004/tcp --permanent && sudo firewall-cmd --reload
```

If HDHR LAN discovery is enabled (`IPTV_TUNERR_HDHR_NETWORK_MODE=true`), also open UDP/TCP 65001:

```bash
sudo ufw allow 65001/tcp
sudo ufw allow 65001/udp
```

---

## See also

- [Deployment guide](deployment.md) — binary, Docker, and systemd setup
- [CLI and env reference](../reference/cli-and-env-reference.md) — all flags and environment variables
- [Emby and Jellyfin support](../emby-jellyfin-support.md) — media server registration
