# Plex Tuner

Plex Tuner bridges IPTV feeds into Plex Live TV & DVR.

It supports two major integration modes with Plex:
- **HDHR mode**: act like an HDHomeRun tuner (`discover.json`, `lineup.json`, `guide.xml`, `/stream/...`) so Plex can use the normal Live TV wizard.
- **DVR injection mode**: programmatic / headless Plex DVR creation and updates (including multi-DVR/category setups), plus guide refresh and channelmap activation workflows.

It also supports running many virtual tuners from one process (`supervise`) and optional VOD filesystem mounting on Linux.

No web UI. Configuration is CLI + env vars.

Source: <https://github.com/snapetech/plexTuner>

## What This App Does

- Indexes IPTV sources (M3U and Xtream `player_api`)
- Exposes a Plex-compatible tuner interface (HDHR-compatible HTTP endpoints)
- Proxies/transcodes streams for Plex playback
- Serves XMLTV guide data (placeholder or external XMLTV remap)
- Supports both Plex wizard-based setup and programmatic DVR/injection workflows
- Supports multi-DVR setups (for example category DVRs) in one app via `supervise`
- Supports deterministic EPG-link coverage reporting for long-tail unlinked channels (`epg-link-report`)

## Two Plex Integration Paths (Important)

## 1. HDHR Functions (Wizard-Compatible)

Use this when you want Plex to discover/add Plex Tuner as if it were an HDHomeRun tuner.

Plex Tuner provides:
- `GET /discover.json`
- `GET /lineup.json`
- `GET /lineup_status.json`
- `GET /guide.xml`
- `GET /stream/<id>`

This path is used for:
- normal Plex “Add DVR / Set up” wizard flows
- HDHR-style manual URL entry
- HDHR lane testing in parallel with injected DVRs

Related features for this path:
- lineup shaping / wizard-safe caps (for provider matching behavior)
- HDHR metadata controls (`Manufacturer`, `ModelNumber`, etc.)
- `ScanPossible` control (so category tuners can be de-emphasized in wizard selection)

## 2. DVR Injection Functions (Programmatic / Headless)

Use this when you want Plex DVRs created/managed without relying on the wizard UI.

This path supports:
- programmatic DVR registration flows (API/DB-assisted)
- multi-DVR category setups (for example 13 injected DVRs)
- guide reload workflows
- channelmap activation / replay workflows
- repeatable cutover/update operations
- overflow category buckets via lineup sharding (`category2`, `category3`, ...)

This is the path used for:
- category DVR fleets
- headless Plex lab/test setups
- fast rebuilds after guide/channel remaps

Reference (authoritative for Plex-side manipulation details):
- [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md)

## Core App Features

- **Inputs**: M3U URL or Xtream `player_api` (live, VOD, series)
- **Provider failover**: multi-host probing + ranked backup stream URLs
- **Streaming gateway**: HLS relay/transcode to Plex-friendly MPEG-TS paths
- **Guide handling**: placeholder XMLTV or external XMLTV fetch/filter/remap
- **XMLTV normalization**: language/script preference options (optional)
- **Multi-DVR safety**: per-instance guide number offsets (avoid Plex guide collisions)
- **Built-in Plex stale-session reaper**: optional background worker (poll + SSE + lease)
- **Supervisor mode**: many child tuner instances from one JSON config (`plex-tuner supervise`)
- **VODFS (optional)**: Linux-only FUSE mount for VOD catalog browsing

Feature reference:
- [`docs/features.md`](docs/features.md)

## Supported Runtime Modes

## 1. Single Tuner (Simple)

Run one Plex Tuner instance for a single HDHR-style tuner endpoint.

```bash
plex-tuner run
# or
plex-tuner serve -addr :5004
```

## 2. Multi-Instance Supervisor (Single App / Single Container)

Run many virtual tuners (for example category DVR children + one HDHR wizard child) from one process:

```bash
plex-tuner supervise -config /path/to/supervisor.json
```

Examples:
- [`k8s/plextuner-supervisor-multi.example.json`](k8s/plextuner-supervisor-multi.example.json)
- [`k8s/plextuner-supervisor-singlepod.example.yaml`](k8s/plextuner-supervisor-singlepod.example.yaml)

## Quick Start (HDHR / Wizard Path)

1. Build

```bash
go build -o plex-tuner ./cmd/plex-tuner
```

2. Configure (`.env`)

Set at least:
- `PLEX_TUNER_PROVIDER_USER` / `PLEX_TUNER_PROVIDER_PASS` (or subscription file)
- `PLEX_TUNER_PROVIDER_URL` or `PLEX_TUNER_PROVIDER_URLS`
- `PLEX_TUNER_BASE_URL=http://<host>:5004`

3. Run

```bash
./plex-tuner run
```

4. Add in Plex (wizard)

Plex -> Settings -> Live TV & DVR -> Add DVR / Set up
- Device URL: your base URL
- Guide URL: `<base>/guide.xml` (if Plex asks for XMLTV)

For local binary/Docker/systemd setups:
- [`docs/how-to/run-without-kubernetes.md`](docs/how-to/run-without-kubernetes.md)

## Quick Start (DVR Injection / Headless Path)

Use the programmatic registration/injection path when you want multiple DVRs or repeatable headless setup.

Start with:
- Plex lifecycle/API reference: [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md)
- Supervisor/testing config reference: [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md)
- k8s deployment examples and cutover docs: [`k8s/README.md`](k8s/README.md)

Common headless operations include:
- create/update DVRs
- reload guides
- replay channelmaps
- clean stale devices/providers

## CLI Commands

- `run` — refresh catalog + health check + serve (systemd-friendly)
- `serve` — tuner only
- `index` — fetch provider data and write catalog
- `probe` — provider host probe / ranking
- `mount` — VODFS mount (Linux only)
- `plex-vod-register` — create/reuse Plex libraries for a VODFS mount (`VOD`, `VOD-Movies` by default)
- `epg-link-report` — deterministic EPG coverage + unmatched report for live channels vs XMLTV
- `supervise` — run multiple child tuner instances from one JSON config

Reference:
- [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md)
- [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md)

## Platform Support

### Linux / macOS / Windows (core app)

Supported:
- `run`, `serve`, `index`, `probe`, `supervise`
- HDHR-compatible HTTP tuner endpoints
- XMLTV remap/normalization
- built-in Plex session reaper

### Linux-only

- `mount` / `VODFS` (FUSE)

### VOD Libraries in Plex (VODFS)

VOD library injection is not the same as Live TV DVR injection.

Current supported flow:
1. mount VODFS (`plex-tuner mount`) on a path the Plex server can see
2. register libraries via `plex-vod-register`

By default, `plex-vod-register` creates/reuses:
- `VOD` -> `<mount>/TV`
- `VOD-Movies` -> `<mount>/Movies`

Optional one-sided registration:
- `-shows-only` (create/reuse only the TV library)
- `-movies-only` (create/reuse only the Movie library)

Example:

```bash
plex-tuner mount -catalog ./catalog.json -mount /srv/plextuner-vodfs
plex-tuner plex-vod-register \
  -mount /srv/plextuner-vodfs \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN"
```

Important:
- the VODFS mount path must be visible to Plex (same host / shared filesystem path)
- Kubernetes Plex pods usually need a host-level/systemd VODFS mount or a privileged mount-propagation setup; mounting VODFS in a separate helper pod does not automatically make it visible to the Plex pod

### Windows note (HDHR network mode)

Windows builds include the HDHR network-mode code path, but native Windows validation is still recommended (do not treat `wine` smoke tests as authoritative for discovery/network behavior).

## Recent Advanced Features / Ops Notes

- **EPG long-tail tooling (Phase 1):**
  - `plex-tuner epg-link-report` compares `catalog.json` live channels vs XMLTV and reports deterministic matches (`tvg-id`, alias exact, normalized-name exact unique)
  - intended for safely improving the large unlinked-channel tail before auto-applying matches
- **Injected DVR overflow buckets:**
  - runtime lineup sharding envs `PLEX_TUNER_LINEUP_SKIP` / `PLEX_TUNER_LINEUP_TAKE`
  - supervisor manifest generator can auto-create `category2/category3/...` children from confirmed linked counts (`--category-counts-json`)
- **Live TV provider label proxy tooling (experimental/client-dependent):**
  - server-side `/media/providers` and provider-endpoint rewrite proxy is available for per-source labels in clients that honor provider metadata
  - current Plex Web/TV clients may still display server-level labels (`plexKube`) in some source-tab UIs

## Packaging for Testers / Test Releases

Cross-platform tester bundles:

```bash
./scripts/build-test-packages.sh
./scripts/build-tester-release.sh
```

Tag-based test release flow (recommended):
- push `v*` tag -> versioned GHCR images + tester bundle GitHub Release asset

Docs:
- [`docs/how-to/package-test-builds.md`](docs/how-to/package-test-builds.md)
- [`docs/how-to/tester-handoff-checklist.md`](docs/how-to/tester-handoff-checklist.md)
- [`docs/how-to/tester-release-notes-draft.md`](docs/how-to/tester-release-notes-draft.md)

## Troubleshooting / Runbooks

- [`docs/runbooks/plextuner-troubleshooting.md`](docs/runbooks/plextuner-troubleshooting.md)
- [`docs/runbooks/plex-hidden-live-grab-recovery.md`](docs/runbooks/plex-hidden-live-grab-recovery.md)
- [`docs/runbooks/plex-in-cluster.md`](docs/runbooks/plex-in-cluster.md)

## Repo Layout (High-Value Paths)

- `cmd/plex-tuner/` — CLI entrypoint
- `internal/tuner/` — HDHR endpoints, XMLTV, streaming gateway, Plex reaper
- `internal/supervisor/` — multi-instance supervisor runtime
- `internal/plex/` — Plex registration helpers (API/DB-assisted flows)
- `internal/provider/` — Xtream/M3U probing + indexing support
- `internal/vodfs/` — VOD filesystem mount (Linux-only runtime)
- `k8s/` — manifests, supervisor examples, deploy scripts
- `scripts/` — packaging, Plex ops helpers, analysis tools
- `memory-bank/` — project state/history/recurring issue notes

## Development

Verification:

```bash
./scripts/verify
```

Agent/handoff workflow:
- [`AGENTS.md`](AGENTS.md)
- [`memory-bank/repo_map.md`](memory-bank/repo_map.md)
- [`memory-bank/commands.yml`](memory-bank/commands.yml)
