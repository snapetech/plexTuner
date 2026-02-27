# Plex Tuner — Features

Short feature overview. Canonical list: [`docs/features.md`](docs/features.md).

## Highlights

- **HDHomeRun-compatible tuner** for Plex Live TV & DVR (`discover.json`, `lineup.json`, `guide.xml`, `/stream/...`)
- **IPTV indexing** from M3U and Xtream `player_api` with resilient fetch engine (conditional GET, crash-resume, CF detection)
- **Multi-provider merge** — second provider live channels merged and deduped into primary catalog
- **Provider failover** with multi-host probing and ranked backup stream URLs
- **Supervisor mode** (`plex-tuner supervise`) to run many virtual tuners in one app/container
- **Multi-DVR Plex support** (category/injected DVR fleets + HDHR wizard lane in parallel)
- **EPG enrichment pipeline** — 8-tier automatic enrichment: re-encode inheritance → Gracenote → iptv-org → SDT name propagation → Schedules Direct → DVB DB → brand-group inheritance → best-stream selection
- **SDT background prober** — reads DVB Service Description Table from live streams; extracts full identity bundle (ONID/TSID/SID, provider, service name, EIT now/next); polite/auto-pausing; 1-week cache, monthly auto-rescan
- **Cached startup** — serves cached lineup to Plex immediately on restart; background refresh updates live lineup seamlessly
- **Manual refresh + rescan endpoints** — `POST /refresh` for immediate catalog re-fetch; `POST /rescan` for full SDT sweep ignoring cache
- **XMLTV remap + normalization** (English/Latin preference options, dummy-guide fallback)
- **Lineup shaping + wizard-safe caps**
- **Injected DVR overflow sharding** (`PLEX_TUNER_LINEUP_SKIP/TAKE`)
- **Built-in Plex stale-session reaper** (optional)
- **Optional VODFS mount** (`mount`, Linux only)
- **VOD Plex library registration** (`plex-vod-register`, including `--shows-only` / `--movies-only`)
- **EPG linking report** (`epg-link-report` coverage/unmatched reports + oracle alias workflow)
- **Cross-platform test packaging** (Linux/macOS/Windows bundles)

## Commands

Core: `run`, `serve`, `index`, `probe`, `mount`, `supervise`

EPG harvest: `plex-gracenote-harvest`, `plex-iptvorg-harvest`, `plex-sd-harvest`, `plex-dvbdb-harvest`

EPG analysis: `epg-link-report`, `plex-epg-oracle`, `plex-epg-oracle-cleanup`

Plex integration: `plex-dvr-sync`, `plex-vod-register`, `plex-session-drain`, `plex-label-proxy`

VOD: `vod-split`, `vod-backfill-series`

Ops/config: `generate-supervisor-config`, `plex-probe-overrides`

## Plex workflows supported

- **HDHR wizard path** (manual setup in Plex)
- **Wizard-equivalent API flow** (programmatic creation/activation patterns)
- **Injected DVR path** (programmatic/DB-assisted category DVR fleets)
- **Guide refresh + channelmap activation** repeat workflows

See: [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md)

## Platform notes

- **Linux/macOS/Windows:** core tuner app (`run`, `serve`, `index`, `probe`, `supervise`)
- **Linux only:** `mount` / VODFS (FUSE)
- **Windows HDHR network mode:** build path enabled; native Windows validation recommended

## Operations / packaging

- Tester package archives: `scripts/build-test-packages.sh`
- Staged tester handoff bundle: `scripts/build-tester-release.sh`
- Plex hidden-grab recovery helper: `scripts/plex-hidden-grab-recover.sh`

Full details: [`docs/features.md`](docs/features.md)
