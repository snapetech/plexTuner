# Plex Tuner

Plex Tuner bridges IPTV feeds into Plex Live TV & DVR.

It provides:
- an HDHomeRun-compatible tuner interface (`discover.json`, `lineup.json`, `guide.xml`, `/stream/...`)
- IPTV indexing (M3U and Xtream `player_api`)
- optional Plex registration paths (wizard-compatible HDHR lane, API/DB-assisted flows)
- optional VOD filesystem mount (`VODFS`, Linux only)
- a single-process supervisor mode to run many virtual tuners in one container/pod

No web UI. Configuration is CLI + env vars.

Source: <https://github.com/snapetech/plexTuner>

## What It Is Good At

- **Plex Live TV & DVR testing** with real IPTV feeds
- **Multi-DVR category setups** (for example 13 injected DVRs) in one app process via `supervise`
- **HDHR wizard lane** alongside injected DVRs (same app, separate child identity)
- **Headless/automated Plex workflows** for lineups, guide refreshes, and channelmap activation
- **Operational debugging** with Plex-specific helpers and runbooks

## Core Features

- **Inputs:** M3U URL or Xtream `player_api` (live, VOD, series)
- **Provider failover:** multi-host probing + ranked backup stream URLs
- **Tuner:** HDHomeRun-compatible endpoints + stream gateway
- **Guide:** placeholder XMLTV or external XMLTV fetch/filter/remap
- **XMLTV normalization:** English-preferred / Latin-preferred programme text selection and non-Latin title fallback (optional)
- **Lineup shaping:** pre-cap filtering/order for HDHR wizard/provider matching (music/radio drop, region profile hints)
- **Multi-DVR safety:** per-instance guide number offsets to avoid Plex guide cache collisions
- **Built-in Plex stale-session reaper:** optional background worker (poll + SSE + lease model)
- **Supervisor mode:** run many `plex-tuner` child instances from one JSON config (`plex-tuner supervise`)
- **VODFS (optional):** mount VOD catalog as `Movies/` and `TV/` (Linux only)

Feature details: [`docs/features.md`](docs/features.md)

## Supported Modes

### 1. Single tuner (simple)

Run one Plex Tuner instance and add it in Plex via the HDHR wizard.

```bash
plex-tuner run
# or
plex-tuner serve -addr :5004
```

### 2. Multi-instance supervisor (single app / single container)

Run multiple virtual tuners (for example category DVR children + one HDHR wizard child) from one process supervisor:

```bash
plex-tuner supervise -config /path/to/supervisor.json
```

Example config and k8s manifests:
- [`k8s/plextuner-supervisor-multi.example.json`](k8s/plextuner-supervisor-multi.example.json)
- [`k8s/plextuner-supervisor-singlepod.example.yaml`](k8s/plextuner-supervisor-singlepod.example.yaml)

### 3. Plex registration flows

Plex Tuner supports multiple ways to land in Plex:
- **HDHR wizard path** (manual or wizard-equivalent API flow)
- **Injected DVR path** (programmatic / DB-assisted registration for category DVR fleets)
- **Guide reload + channelmap activation** tooling for repeatable updates

Reference: [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md)

## Platform Support

### Works on Linux / macOS / Windows (core app)

- `run`
- `serve`
- `index`
- `probe`
- `supervise`
- HDHR-compatible HTTP tuner endpoints
- XMLTV handling and guide remap/normalization
- built-in Plex session reaper

### Linux-only

- `mount` (`VODFS` / FUSE)

### Windows note (HDHR network mode)

Windows builds include the HDHR network-mode code path again, but validation in this repo was only smoke-tested under `wine`.
Native Windows testing is still recommended for authoritative UDP/TCP discovery behavior.

## Quick Start (Single Host)

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

4. Add in Plex (wizard path)

Plex -> Settings -> Live TV & DVR -> Add DVR / Set up
- Device URL: your base URL
- Guide URL: `<base>/guide.xml` (if Plex asks for XMLTV)

More complete local options (binary / Docker / systemd):
- [`docs/how-to/run-without-kubernetes.md`](docs/how-to/run-without-kubernetes.md)

## Commands

- `run` — refresh catalog + health check + serve (systemd-friendly)
- `serve` — tuner only
- `index` — fetch provider data and write catalog
- `probe` — provider host probe / ranking
- `mount` — VODFS mount (Linux only)
- `supervise` — run multiple child tuner instances from one JSON config

Command and env reference:
- [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md)
- [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md)

## Packaging for Testers

Cross-platform test bundles (Linux/macOS/Windows):

```bash
./scripts/build-test-packages.sh
./scripts/build-tester-release.sh
```

Docs:
- [`docs/how-to/package-test-builds.md`](docs/how-to/package-test-builds.md)
- [`docs/how-to/tester-handoff-checklist.md`](docs/how-to/tester-handoff-checklist.md)
- [`docs/how-to/tester-release-notes-draft.md`](docs/how-to/tester-release-notes-draft.md)

## Troubleshooting / Runbooks

- [`docs/runbooks/plextuner-troubleshooting.md`](docs/runbooks/plextuner-troubleshooting.md)
- [`docs/runbooks/plex-hidden-live-grab-recovery.md`](docs/runbooks/plex-hidden-live-grab-recovery.md)
- [`docs/runbooks/plex-in-cluster.md`](docs/runbooks/plex-in-cluster.md)

## Repository Layout (high-value paths)

- `cmd/plex-tuner/` — CLI entrypoint
- `internal/tuner/` — HDHR endpoints, XMLTV, streaming gateway, Plex reaper
- `internal/supervisor/` — multi-instance supervisor runtime
- `internal/plex/` — Plex registration helpers
- `internal/provider/` — Xtream/M3U probing and indexing support
- `internal/vodfs/` — VOD filesystem mount (Linux-only runtime)
- `k8s/` — manifests, supervisor examples, deployment scripts
- `scripts/` — packaging, Plex ops helpers, analysis tools
- `memory-bank/` — current task state, known issues, recurring loops, history

## Development

Verification:

```bash
./scripts/verify
```

Agent/handoff workflow:
- [`AGENTS.md`](AGENTS.md)
- [`memory-bank/repo_map.md`](memory-bank/repo_map.md)
- [`memory-bank/commands.yml`](memory-bank/commands.yml)
