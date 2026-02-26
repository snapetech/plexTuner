# Plex Tuner — Features

Short feature overview. Canonical list: [`docs/features.md`](docs/features.md).

## Highlights

- **HDHomeRun-compatible tuner** for Plex Live TV & DVR (`discover.json`, `lineup.json`, `guide.xml`, `/stream/...`)
- **IPTV indexing** from M3U and Xtream `player_api`
- **Provider failover** with multi-host probing and ranked backup stream URLs
- **Supervisor mode** (`plex-tuner supervise`) to run many virtual tuners in one app/container
- **Multi-DVR Plex support** (category/injected DVR fleets + HDHR wizard lane in parallel)
- **XMLTV remap + normalization** (English/Latin preference options)
- **Lineup shaping + wizard-safe caps** (for HDHR/provider matching workflows)
- **Injected DVR overflow sharding** (`PLEX_TUNER_LINEUP_SKIP/TAKE`, generator support for `category2/category3/...`)
- **Built-in Plex stale-session reaper** (optional)
- **Optional VODFS mount** (`mount`, Linux only)
- **VOD Plex library registration** (`plex-vod-register`, including `--shows-only` / `--movies-only`)
- **EPG linking report (Phase 1)** (`epg-link-report` deterministic coverage/unmatched reports)
- **Cross-platform test packaging** (Linux/macOS/Windows bundles)

## Commands

- `run`, `serve`, `index`, `probe`, `mount`, `plex-vod-register`, `epg-link-report`, `supervise`

## Plex workflows supported

- **HDHR wizard path** (manual setup in Plex)
- **Wizard-equivalent API flow** (programmatic creation/activation patterns)
- **Injected DVR path** (programmatic/DB-assisted category DVR fleets)
- **Guide refresh + channelmap activation** repeat workflows

See: [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md)

## Platform notes

- **Linux/macOS/Windows:** core tuner app (`run`, `serve`, `index`, `probe`, `supervise`)
- **Linux only:** `mount` / VODFS (FUSE)
- **Windows HDHR network mode:** build path enabled; native Windows validation recommended (`wine` smoke is not authoritative)

## Operations / packaging

- Tester package archives: `scripts/build-test-packages.sh`
- Staged tester handoff bundle: `scripts/build-tester-release.sh`
- Plex hidden-grab recovery helper: `scripts/plex-hidden-grab-recover.sh`
- Plex stale session drain helper (external): `scripts/plex-live-session-drain.py`
- Plex Live TV provider label rewrite proxy (experimental/client-dependent): `scripts/plex-media-providers-label-proxy.py`

Full details: [`docs/features.md`](docs/features.md)
