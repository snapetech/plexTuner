# IPTV Tunerr

Bridge IPTV feeds into Plex, Emby, and Jellyfin as an HDHomeRun-compatible tuner.

**Source:** https://github.com/snapetech/iptvtunerr

---

## What It Does

- Indexes IPTV providers (M3U and Xtream `player_api`)
- Emulates an HDHomeRun tuner (`/discover.json`, `/lineup.json`, `/guide.xml`, `/stream/...`)
- Proxies and optionally transcodes live streams
- Serves XMLTV guide data (placeholder or remapped from an external source)
- Supports single-tuner and multi-DVR supervisor deployments
- Optionally mounts a VOD filesystem (Linux-only, FUSE)

No web UI. Configuration is CLI flags and environment variables.

---

## Two Integration Paths

### HDHR / Wizard Path

IPTV Tunerr appears as an HDHomeRun device. Add it through the normal Live TV wizard in Plex, Emby, or Jellyfin.

Best for: single-tuner setups, manual wizard flows, parallel HDHR lane testing.

### DVR Injection / Headless Path

Programmatic DVR creation and management without touching the UI. Supports multi-DVR category fleets, guide reloads, channelmap activation, and repeatable cutover operations.

Best for: multi-DVR setups, headless lab environments, fast rebuilds after guide/channel remaps.

Reference: [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md)

---

## Quick Start

### Build

```bash
go build -o iptv-tunerr ./cmd/iptv-tunerr
```

### Minimum Configuration

```bash
IPTV_TUNERR_PROVIDER_URL=https://your-provider.com
IPTV_TUNERR_PROVIDER_USER=username
IPTV_TUNERR_PROVIDER_PASS=password
IPTV_TUNERR_BASE_URL=http://<this-host>:5004
```

### Run

```bash
./iptv-tunerr run
```

This fetches the catalog, checks provider health, and starts the tuner server on `:5004`.

### Add to Plex (Wizard)

Plex → Settings → Live TV & DVR → Set up
Device URL: `http://<this-host>:5004`
Guide URL: `http://<this-host>:5004/guide.xml`

For Docker, systemd, and bare-metal setups: [`docs/how-to/deployment.md`](docs/how-to/deployment.md)

---

## Supervisor Mode (Multi-DVR)

Run multiple virtual tuner instances from a single process — each on its own port, with independent provider credentials, lineup, and guide configuration.

```bash
iptv-tunerr supervise -config /path/to/supervisor.json
```

Examples:
- [`k8s/iptvtunerr-supervisor-multi.example.json`](k8s/iptvtunerr-supervisor-multi.example.json)
- [`k8s/iptvtunerr-supervisor-singlepod.example.yaml`](k8s/iptvtunerr-supervisor-singlepod.example.yaml)

Config reference: [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md)

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `run` | Index + health check + serve (use for systemd/containers) |
| `serve` | Tuner server only (requires existing catalog) |
| `index` | Fetch provider data and write `catalog.json` |
| `probe` | Test and rank provider hosts |
| `supervise` | Run multiple child tuner instances from a JSON config |
| `epg-link-report` | Report EPG coverage and unmatched channels |
| `mount` | Mount VODFS (Linux only) |
| `plex-vod-register` | Create or reuse Plex VOD libraries for a VODFS mount |

Full reference: [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md)

---

## Key Environment Variables

### Provider / Input

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_PROVIDER_URL` | Xtream or M3U provider base URL |
| `IPTV_TUNERR_PROVIDER_URLS` | Comma-separated URLs for multi-host failover |
| `IPTV_TUNERR_PROVIDER_USER` | Provider username |
| `IPTV_TUNERR_PROVIDER_PASS` | Provider password |
| `IPTV_TUNERR_M3U_URL` | Full M3U URL (skips host+credentials assembly) |
| `IPTV_TUNERR_SUBSCRIPTION_FILE` | File with `Username:` / `Password:` lines |

### Tuner Identity & Lineup

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_BASE_URL` | URL the media server uses to reach this tuner (required) |
| `IPTV_TUNERR_DEVICE_ID` | Stable HDHomeRun device identifier |
| `IPTV_TUNERR_FRIENDLY_NAME` | Display name in media server UI |
| `IPTV_TUNERR_TUNER_COUNT` | Max concurrent streams |
| `IPTV_TUNERR_LINEUP_MAX_CHANNELS` | Max channels in lineup (default 480, Plex wizard cap) |
| `IPTV_TUNERR_GUIDE_NUMBER_OFFSET` | Channel number offset (prevents multi-DVR collisions) |
| `IPTV_TUNERR_LINEUP_SKIP` / `IPTV_TUNERR_LINEUP_TAKE` | Slice lineup for overflow DVR shards |
| `IPTV_TUNERR_LIVE_EPG_ONLY` | Only include channels with a `tvg-id` |

### Streaming

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_STREAM_TRANSCODE` | `off` \| `on` \| `auto` (probe codec, transcode if needed) |
| `IPTV_TUNERR_STREAM_BUFFER_BYTES` | `0` \| `auto` \| `<bytes>` |
| `IPTV_TUNERR_FFMPEG_PATH` | Custom ffmpeg binary path |
| `IPTV_TUNERR_CLIENT_ADAPT` | Detect Plex Web; apply browser-compatible codec automatically |
| `IPTV_TUNERR_FORCE_WEBSAFE` | Always transcode with MP3 audio |

### Guide / XMLTV

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_XMLTV_URL` | External XMLTV source to fetch, filter, and serve |
| `IPTV_TUNERR_XMLTV_CACHE_TTL` | How long to cache remapped XMLTV (default `10m`) |
| `IPTV_TUNERR_EPG_PRUNE_UNLINKED` | Exclude unlinked channels from guide and lineup |
| `IPTV_TUNERR_XMLTV_PREFER_LANGS` | Language preference for programme titles (e.g. `en,eng`) |
| `IPTV_TUNERR_XMLTV_PREFER_LATIN` | Prefer Latin script when multilingual data is available |

### Plex Session Reaper (Optional)

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_PMS_URL` | Plex Media Server URL |
| `IPTV_TUNERR_PMS_TOKEN` | Plex API token |
| `IPTV_TUNERR_PLEX_SESSION_REAPER` | Enable background stale-session cleanup |
| `IPTV_TUNERR_PLEX_SESSION_REAPER_IDLE_S` | Seconds idle before a session is pruned |

### Emby / Jellyfin

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_EMBY_HOST` | Emby server URL |
| `IPTV_TUNERR_EMBY_TOKEN` | Emby API key |
| `IPTV_TUNERR_JELLYFIN_HOST` | Jellyfin server URL |
| `IPTV_TUNERR_JELLYFIN_TOKEN` | Jellyfin API key |

Full variable reference: [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md)

---

## Platform Support

| Feature | Linux | macOS | Windows |
|---------|-------|-------|---------|
| `run`, `serve`, `index`, `probe`, `supervise` | ✓ | ✓ | ✓ |
| HDHR HTTP endpoints | ✓ | ✓ | ✓ |
| XMLTV remap / normalization | ✓ | ✓ | ✓ |
| Plex session reaper | ✓ | ✓ | ✓ |
| `mount` / VODFS (FUSE) | ✓ | — | — |

Platform requirements: [`docs/how-to/platform-requirements.md`](docs/how-to/platform-requirements.md)

---

## VOD Filesystem (Linux Only)

Mount IPTV VOD catalog as a browsable filesystem that Plex can index as libraries.

```bash
iptv-tunerr mount -catalog ./catalog.json -mount /srv/iptvtunerr-vodfs
iptv-tunerr plex-vod-register \
  -mount /srv/iptvtunerr-vodfs \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN"
```

By default this creates `VOD` (TV) and `VOD-Movies` libraries. Use `-shows-only` or `-movies-only` to register one at a time.

The mount path must be visible to the Plex server. In Kubernetes, VODFS mounts inside a helper container are not automatically visible to the Plex pod without host-level mounts or `MountPropagation`.

Guide: [`docs/how-to/mount-vodfs-and-register-plex-libraries.md`](docs/how-to/mount-vodfs-and-register-plex-libraries.md)

---

## Kubernetes

Single-command deployment:

```bash
./k8s/standup-local-cluster.sh
```

Or step-by-step with env-based credentials:

```bash
IPTV_TUNERR_PROVIDER_USER='user' \
IPTV_TUNERR_PROVIDER_PASS='pass' \
IPTV_TUNERR_PROVIDER_URL='https://provider.com' \
./k8s/deploy-hdhr-one-shot.sh --static
```

Full K8s guide: [`k8s/README.md`](k8s/README.md)

---

## Repo Layout

```
cmd/iptv-tunerr/      CLI entrypoint
internal/tuner/       HDHR endpoints, streaming gateway, XMLTV, Plex reaper
internal/supervisor/  Multi-instance supervisor runtime
internal/plex/        Plex registration helpers (API + DB-assisted)
internal/emby/        Emby / Jellyfin registration and watchdog
internal/provider/    Xtream / M3U probing and indexing
internal/catalog/     Normalized channel/VOD data model
internal/vodfs/       VOD filesystem mount (Linux only)
internal/epglink/     EPG match reporting
k8s/                  Manifests, supervisor examples, deploy scripts
scripts/              Packaging, Plex ops helpers, analysis tools
docs/                 Reference, how-to guides, runbooks
```

---

## Security Notes

**Endpoints are unauthenticated.** All HTTP endpoints (`/lineup.json`, `/stream/*`, `/guide.xml`, etc.) have no access control. This is by design — IPTV Tunerr is a LAN-internal service and the media server is the only expected consumer. Do not expose port 5004 directly to the internet. If you need public access, put a reverse proxy with authentication (Caddy, nginx) in front.

**`catalog.json` may contain provider credentials.** The catalog is built from Xtream API responses which embed credentials in stream URLs. Restrict permissions on the catalog file: `chmod 0600 catalog.json`. In supervisor deployments, use env file injection rather than baking credentials into the config JSON.

**Stream URLs are SSRF-validated.** The stream gateway only fetches `http://` and `https://` URLs. `file://`, `ftp://`, `data:`, and other schemes are rejected before any network request is made.

---

## Documentation

**Reference**
- [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md) — All CLI flags and environment variables
- [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md) — Plex DVR lifecycle, HDHR wizard flow, injection API
- [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md) — Supervisor config, guide offsets, overflow shards
- [`docs/reference/epg-linking-pipeline.md`](docs/reference/epg-linking-pipeline.md) — EPG match strategy

**How-To**
- [`docs/how-to/deployment.md`](docs/how-to/deployment.md) — Binary, Docker, systemd deployment
- [`docs/how-to/platform-requirements.md`](docs/how-to/platform-requirements.md) — FFmpeg, FUSE, platform notes
- [`docs/how-to/mount-vodfs-and-register-plex-libraries.md`](docs/how-to/mount-vodfs-and-register-plex-libraries.md) — VOD filesystem setup

**Runbooks**
- [`docs/runbooks/iptvtunerr-troubleshooting.md`](docs/runbooks/iptvtunerr-troubleshooting.md)
- [`docs/runbooks/plex-hidden-live-grab-recovery.md`](docs/runbooks/plex-hidden-live-grab-recovery.md)
- [`docs/runbooks/plex-in-cluster.md`](docs/runbooks/plex-in-cluster.md)

**Development**
- [`AGENTS.md`](AGENTS.md) — Agent/handoff workflow
- [`docs/features.md`](docs/features.md) — Full feature list

Verify the build:

```bash
./scripts/verify
```
