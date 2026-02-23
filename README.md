# Plex Tuner

**IPTV-to-Plex bridge.** Index M3U or Xtream Codes (player_api) sources, build a catalog of live channels (and optional VOD/series), and expose them as an HDHomeRun-style tuner so Plex Live TV & DVR can use it. Optional FUSE mount for VOD as Movies/TV directories.

- **Mirrored:** [GitLab](https://gitlab.home/keith/plexTuner) · [GitHub](https://github.com/snapetech/plexTuner) (same codebase).

---

## Features (summary)

| Area | What we do |
|------|------------|
| **Input** | M3U URL (e.g. provider `get.php`) or Xtream **player_api** (live + VOD movies + series). Multi-host: first working host wins (same idea as xtream-to-m3u.js). |
| **Catalog** | JSON catalog: live channels, movies, series. Snapshot-then-encode for safe writes. Optional filters: EPG-linked only, live-only (skip VOD), stream smoketest. |
| **Tuner** | HDHomeRun emulator: `/discover.json`, `/lineup.json`, `/lineup_status.json`, `/guide.xml`, `/live.m3u`, `/stream/<id>`. Configurable tuner count; stream gateway with auth and backup URLs. |
| **EPG** | Built-in placeholder XMLTV or **external XMLTV** URL: fetch, filter to catalog channels, remap to local guide numbers. EPG prune: only channels with `tvg-id` in guide/M3U. |
| **VOD** | Optional **FUSE (VODFS)**: mount catalog as `Movies/` and `TV/`; optional on-demand download cache. |
| **Ops** | **run** (one-shot: refresh catalog → health check → serve), **index** / **serve** / **mount** / **probe** subcommands. Optional Plex DB registration (`-register-plex`). Env-based config + subscription file fallback. |

See **[docs/features.md](docs/features.md)** for a full feature list and **[docs/CHANGELOG.md](docs/CHANGELOG.md)** for version history.

---

## Comparison: Plex Tuner vs alternatives

We derive design and behavior from the same IPTV→Plex use case as **xTeVe** and **Threadfin** (and from in-house stacks like k3s IPTV + xtream-to-m3u.js). The table below is a feature matrix: ✓ = supported, — = not supported or N/A.

| Feature | xTeVe | Threadfin | **Plex Tuner** |
|--------|-------|-----------|----------------|
| M3U input | ✓ | ✓ | ✓ |
| HDHomeRun emulation (Plex DVR) | ✓ | ✓ | ✓ |
| XMLTV / EPG | ✓ | ✓ | ✓ (placeholder + external URL) |
| Web UI | ✓ | ✓ | **—** (CLI + env only) |
| Stream buffering / transcoding | ✓ | ✓ (HLS buffer) | **—** (proxy only) |
| Channel mapping/filtering (UI) | ✓ | ✓ | ✓ (EPG-only, smoketest, live-only via env) |
| **Xtream player_api** (live+VOD+series) | — | — | **✓** (first-class; same as xtream-to-m3u.js) |
| **Multi-host probe** (first OK wins) | — | — | **✓** (provider URLs + `probe` command) |
| **VOD as filesystem (FUSE)** | — | — | **✓** (VODFS: Movies/TV + optional cache) |
| **Plex DB registration** (headless) | — | — | **✓** (optional `-register-plex`) |
| **Subscription file** creds | — | — | **✓** (e.g. `iptv.subscription.2026.txt`) |
| **Stream smoketest** at index | — | — | **✓** (drop failing channels) |
| **External XMLTV remap** | ✓ (mapping) | ✓ | **✓** (filter + remap to lineup) |
| **run / serve / index split** | — | — | **✓** (systemd-friendly one-run) |
| Single binary, no runtime deps | — | — | **✓** (Go; optional FUSE) |

**Unique to Plex Tuner:** No web UI (config via `.env` and CLI); no built-in buffer/transcode (we proxy). We focus on **player_api-first** indexing, **multi-host resilience**, **VODFS**, **headless Plex registration**, and **run-once** operation for systemd/Docker.

---

## Quick start

```bash
# Build (requires internal/indexer package if present)
go build -o plex-tuner ./cmd/plex-tuner

# Run one-shot: refresh catalog, health check, then serve (for systemd)
./plex-tuner run

# Or run steps separately
./plex-tuner index                    # fetch M3U/API, save catalog
./plex-tuner serve -addr :5004        # serve tuner only (no index)
./plex-tuner mount -mount /mnt/vodfs  # mount VOD as Movies/TV (optional -cache for download)
./plex-tuner probe -urls "http://h1,http://h2"  # probe provider hosts
```

Copy `.env.example` to `.env` and set provider credentials and `PLEX_TUNER_BASE_URL` (e.g. `http://192.168.1.10:5004`). In Plex: add an **HDHomeRun**-compatible tuner with lineup URL `http://<this-host>:5004/lineup.json` and guide `http://<this-host>:5004/guide.xml`.

---

## Commands

| Command | Purpose |
|---------|---------|
| **run** | One-run: refresh catalog (unless `-skip-index`), provider health check (unless `-skip-health`), then serve. Use with systemd. Optional `-refresh=6h`, `-register-plex=/path`. |
| **index** | Fetch M3U or player_api, apply filters (EPG-only, smoketest), save catalog. |
| **serve** | Run HDHomeRun + XMLTV + stream gateway only; no indexing. |
| **mount** | Load catalog and mount VODFS at `-mount`; use `-cache` for on-demand download. |
| **probe** | Cycle through provider URLs, report get.php and player_api status (OK / Cloudflare / fail). |

---

## Configuration (env)

| Env | Purpose |
|-----|---------|
| `PLEX_TUNER_PROVIDER_URL` / `PLEX_TUNER_PROVIDER_URLS` | Base URL(s); comma-separated for multi-host. |
| `PLEX_TUNER_PROVIDER_USER` / `PLEX_TUNER_PROVIDER_PASS` | API credentials (or use `PLEX_TUNER_SUBSCRIPTION_FILE`). |
| `PLEX_TUNER_M3U_URL` | Full M3U URL if not using provider URL + get.php. |
| `PLEX_TUNER_CATALOG` | Catalog JSON path (default `./catalog.json`). |
| `PLEX_TUNER_BASE_URL` | Base URL for Plex (e.g. `http://192.168.1.10:5004`). |
| `PLEX_TUNER_TUNER_COUNT` | Number of concurrent streams (default 2). |
| `PLEX_TUNER_LIVE_EPG_ONLY` | Only include channels with EPG (tvg-id). |
| `PLEX_TUNER_EPG_PRUNE_UNLINKED` | Guide and M3U export only include channels with tvg-id. |
| `PLEX_TUNER_SMOKETEST_ENABLED` | At index, probe each channel stream and drop failures. |
| `PLEX_TUNER_XMLTV_URL` | External XMLTV feed; we fetch, filter, and remap to lineup. |
| `PLEX_TUNER_MOUNT` / `PLEX_TUNER_CACHE` | VODFS mount point and optional cache dir. |

See `.env.example` for the full list.

---

## Docker

```bash
docker compose up -d
# Serve on :5004; override command for run/index/mount/serve. Copy .env.example to .env.
```

See `Dockerfile` and `docker-compose.yml`.

---

## Repo layout

| Path | Purpose |
|------|---------|
| **cmd/plex-tuner/** | Main entrypoint (run, index, mount, serve, probe). |
| **internal/catalog** | Catalog types, Save/Load. |
| **internal/config** | Config from env + subscription file. |
| **internal/tuner** | HDHR, XMLTV, M3U export, stream gateway. |
| **internal/plex** | Optional Plex DB registration. |
| **internal/provider** | Multi-host probe, FirstWorkingPlayerAPI. |
| **internal/materializer** | VOD download (cache, direct file, HLS stub). |
| **internal/vodfs** | FUSE VOD (Movies/TV). |
| **internal/health** | Provider health check. |

---

## For agents / template

This repo uses the **agentic template** workflow: [AGENTS.md](AGENTS.md) and [memory-bank/](memory-bank/) (including `repo_map.md`, `recurring_loops.md`) are the source of truth for commands and process. Run `./scripts/verify` for format/lint/test/build; see `memory-bank/commands.yml`. Documentation gaps are tracked in [docs/docs-gaps.md](docs/docs-gaps.md).
