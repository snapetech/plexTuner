# Plex Tuner

**Your IPTV subscription, inside Plex Live TV & DVR.** One binary: point it at your provider, get an HDHomeRun-compatible tuner and (optionally) a VOD filesystem. No web UI—just `.env`, CLI, and a single Go binary that does the job.

**Source:** [GitHub](https://github.com/snapetech/plexTuner) (mirrored on GitLab).

---

## What it does

Plex Tuner sits between your IPTV provider and Plex. It **indexes** your provider (M3U or Xtream Codes `player_api`), keeps a **catalog** of live channels (and optionally movies/series), and **serves** that as an HDHomeRun-style tuner so Plex can do Live TV and DVR. Streams are proxied (and, when you want, buffered or transcoded) so Plex sees clean MPEG-TS. No browser, no dashboard—configure with env vars and subcommands, run it under systemd or Docker, and forget it.

**Why use it?** You already use xTeVe or Threadfin, or you run something like xtream-to-m3u.js and want a single process that does indexing + tuner + optional VOD-as-filesystem. Plex Tuner is **player_api-first**, **multi-host ranked** (we probe every provider URL, rank by latency, use best for indexing and **fill each channel with 2nd/3rd as stream backups** so when the best fails the gateway tries the next instead of dying), and **headless**: optional Plex DB registration, subscription file for creds, stream smoketest at index time, and a `run` command that refreshes catalog, health-checks the provider, then serves. One binary. One config. One way to get IPTV into Plex without a web UI.

---

## Features at a glance

| Area | What you get |
|------|----------------|
| **Input** | M3U URL or Xtream **player_api** (live + VOD + series). Multiple provider URLs: we **probe all** (one request per host), rank OK-by-latency, use best for indexing and set each channel’s **backup URLs to 2nd, 3rd, …** so streams fail over instead of dying. |
| **Catalog** | One JSON file: live channels, movies, series. Snapshot-then-write so nothing gets half-updated. Filters: EPG-only, live-only, and stream smoketest (drop dead channels at index). |
| **Tuner** | Full HDHomeRun emulation: discover, lineup, guide, and `/stream/<id>`. Tuner count limit, backup stream URLs, optional auth to the provider. **Streams:** remux or transcode (off / on / **auto** via ffprobe). **Buffer:** off, fixed size, or **adaptive** (grows when the client is slow, shrinks when it keeps up). |
| **EPG** | Placeholder guide out of the box. Or point at an external XMLTV URL—we fetch it, keep only channels in the catalog, and remap IDs so Plex gets a clean lineup. EPG prune: only channels with `tvg-id` in guide and M3U. |
| **VOD** | Optional **FUSE (VODFS)**: mount the catalog as `Movies/` and `TV/`. Optional on-demand cache so files are downloaded when Plex (or anything else) opens them. |
| **Ops** | **run** = refresh catalog → health check → serve (one shot for systemd). **index** / **serve** / **mount** / **probe** for split workflows. **probe** = hit every provider URL, report get.php + player_api (OK / Cloudflare / fail) and **print ranked order** (best first; index uses #1, stream failover uses #2, #3, …). Optional **-register-plex** to write tuner/XMLTV URLs into Plex’s DB. |

Full list: **[docs/features.md](docs/features.md)**. History: **[docs/CHANGELOG.md](docs/CHANGELOG.md)**.

---

## Plex Tuner vs xTeVe / Threadfin

Same goal—IPTV into Plex—different tradeoffs. We took cues from xTeVe, Threadfin, and in-house stacks (e.g. k3s IPTV + xtream-to-m3u.js). The table is a straight feature matrix: ✓ = supported, — = not or N/A.

| Feature | xTeVe | Threadfin | **Plex Tuner** |
|--------|-------|-----------|----------------|
| M3U input | ✓ | ✓ | ✓ |
| HDHomeRun (Plex DVR) | ✓ | ✓ | ✓ |
| XMLTV / EPG | ✓ | ✓ | ✓ (placeholder + external) |
| Web UI | ✓ | ✓ | **—** (CLI + env only) |
| Stream buffering / transcoding | ✓ | ✓ (HLS buffer) | **✓** (adaptive buffer; transcode off/on/auto) |
| Channel mapping/filtering (UI) | ✓ | ✓ | ✓ (EPG-only, smoketest, live-only via env) |
| **Xtream player_api** (live+VOD+series) | — | — | **✓** (first-class) |
| **Multi-host ranked** (probe all, best + backups) | — | — | **✓** (probe every URL, rank by latency; index from best, stream failover to 2nd/3rd) |
| **VOD as filesystem (FUSE)** | — | — | **✓** |
| **Plex DB registration** (headless) | — | — | **✓** |
| **Subscription file** creds | — | — | **✓** |
| **Stream smoketest** at index | — | — | **✓** |
| **run / serve / index split** | — | — | **✓** (systemd-friendly) |
| Single binary, minimal deps | — | — | **✓** (Go; optional FUSE/ffmpeg) |

**In short:** No web UI and no built-in “mapping” UI—you configure with env and CLI. We double down on **player_api**, **ranked multi-host** (probe all, best + stream backups), **VODFS**, **headless Plex setup**, and **one-shot run** for systemd/Docker.

---

## Running without a cluster

On a single host (no Kubernetes): use the **binary**, **Docker**, or **systemd**. Full steps and a **local QA/smoke script** are in **[docs/how-to/run-without-kubernetes.md](docs/how-to/run-without-kubernetes.md)**.

| Method | Summary |
|--------|---------|
| **Binary** | `go build -o plex-tuner ./cmd/plex-tuner` then `./plex-tuner run -addr :5004`. |
| **Docker** | `cp .env.example .env` (edit it), then `docker compose up -d`. Or `docker build -t plex-tuner:local .` and `docker run -p 5004:5004 --env-file .env plex-tuner:local`. |
| **systemd** | Copy [docs/systemd/plextuner.service.example](docs/systemd/plextuner.service.example) to `/etc/systemd/system/plextuner.service`, set WorkingDirectory and EnvironmentFile, then `systemctl enable --now plextuner`. |
| **Local test** | `./scripts/plextuner-local-test.sh` — QA (vet, test), serve/run, and smoke-check endpoints (options: `qa`, `serve`, `run`, `smoke`, `all`). |

Cross-platform tester bundles (Linux/macOS/Windows, amd64/arm64 + Linux armv7): **[docs/how-to/package-test-builds.md](docs/how-to/package-test-builds.md)** via `./scripts/build-test-packages.sh` or staged handoff bundles via `./scripts/build-tester-release.sh` (see **[docs/how-to/tester-handoff-checklist.md](docs/how-to/tester-handoff-checklist.md)**).

---

## Quick start

**1. Build**

```bash
go build -o plex-tuner ./cmd/plex-tuner
```

**2. Configure**

Copy `.env.example` to `.env`. Set at least:

- `PLEX_TUNER_PROVIDER_USER` and `PLEX_TUNER_PROVIDER_PASS` (or a subscription file)
- `PLEX_TUNER_PROVIDER_URL` (or `PLEX_TUNER_PROVIDER_URLS` for several hosts—we probe all, rank by latency, use best for index and 2nd/3rd as stream backups)
- `PLEX_TUNER_BASE_URL` = the URL Plex will use to reach this machine (e.g. `http://192.168.1.10:5004`)

**3. Run**

One-shot (refresh catalog, health check, then serve)—ideal for systemd:

```bash
./plex-tuner run
```

Or do it in steps:

```bash
./plex-tuner index                    # fetch M3U/API, write catalog
./plex-tuner serve -addr :5004        # tuner only (no index)
./plex-tuner probe -urls "http://h1,http://h2"   # see which provider URLs work
./plex-tuner mount -mount /mnt/vodfs  # VOD as Movies/TV (optional -cache)
```

**4. Add tuner in Plex**

In Plex: **Settings → Live TV & DVR → Set up**. Use your **Base URL** as the device URL. Lineup: `http://<this-host>:5004/lineup.json`, guide: `http://<this-host>:5004/guide.xml`. Or run with **-register-plex=/path/to/Plex/Media/Server** (stop Plex first, backup its DB) and we write those URLs into Plex for you.

---

## Commands

| Command | What it does |
|---------|----------------|
| **run** | Refresh catalog (unless `-skip-index`), health-check provider (unless `-skip-health`), then serve. Optional `-refresh=6h` to re-index on a schedule, `-register-plex=...` to poke Plex’s DB. |
| **index** | Fetch M3U or player_api, apply EPG-only / smoketest / live-only if set, save catalog. |
| **serve** | Run the tuner (discover, lineup, guide, stream gateway) only. No indexing. |
| **mount** | Load catalog and mount VODFS at `-mount`. Use `-cache` for on-demand download. |
| **probe** | Hit every provider URL (or `-urls=...`), report get.php and player_api (OK / Cloudflare / fail) and latency, and **print ranked order** (best first; index uses #1, stream failover tries #2, #3, …). |

---

## Configuration (env)

**Get going:** provider URL(s), user/pass (or subscription file), and base URL for Plex. Everything else has defaults.

| Env | Purpose |
|-----|---------|
| `PLEX_TUNER_PROVIDER_URL` / `PLEX_TUNER_PROVIDER_URLS` | Provider base URL(s); comma-separated. We probe all, rank by latency, use best for index and set each channel’s backup URLs to 2nd, 3rd, … for stream failover. |
| `PLEX_TUNER_PROVIDER_USER` / `PLEX_TUNER_PROVIDER_PASS` | Credentials (or `PLEX_TUNER_SUBSCRIPTION_FILE`). |
| `PLEX_TUNER_M3U_URL` | Full M3U URL if you don’t want URL + get.php. |
| `PLEX_TUNER_BASE_URL` | URL Plex uses to reach this tuner (e.g. `http://192.168.1.10:5004`). |
| `PLEX_TUNER_CATALOG` | Catalog path (default `./catalog.json`). |

**Tuner and streams:** tuner count, buffer, transcode, EPG.

| Env | Purpose |
|-----|---------|
| `PLEX_TUNER_TUNER_COUNT` | Max concurrent streams (default 2). |
| `PLEX_TUNER_STREAM_BUFFER_BYTES` | `0` = off, `auto` = adaptive when transcoding, or fixed bytes (e.g. `2097152`). Default `auto`. |
| `PLEX_TUNER_STREAM_TRANSCODE` | `off` = remux only, `on` = always transcode, `auto` = transcode only when codec isn’t Plex-friendly. |
| `PLEX_TUNER_LIVE_EPG_ONLY` | Only include channels with EPG (tvg-id). |
| `PLEX_TUNER_EPG_PRUNE_UNLINKED` | Guide and M3U only include channels with tvg-id. |
| `PLEX_TUNER_SMOKETEST_ENABLED` | At index, probe each channel’s stream and drop failures. |
| `PLEX_TUNER_XMLTV_URL` | External XMLTV; we fetch, filter, remap. |
| `PLEX_TUNER_MOUNT` / `PLEX_TUNER_CACHE` | VODFS mount and optional cache dir. |

Full list and comments: **`.env.example`**.

---

## Docker

```bash
cp .env.example .env
# Edit .env: set PLEX_TUNER_PROVIDER_USER, PLEX_TUNER_PROVIDER_PASS, PLEX_TUNER_PROVIDER_URL,
#            and PLEX_TUNER_BASE_URL=http://YOUR_HOST_IP:5004 (so Plex can reach the tuner)
docker compose up -d
curl -s -o /dev/null -w "%{http_code}" http://localhost:5004/discover.json   # expect 200
```

Serves on port 5004. The image defaults to `run -addr :5004`. To override (e.g. serve only): `docker compose run --rm plextuner serve -addr :5004`. See [docs/how-to/run-without-kubernetes.md](docs/how-to/run-without-kubernetes.md) for more.

## Kubernetes (HDHR in cluster)

Deploy Plex Tuner as an HDHomeRun-compatible tuner in Kubernetes: one script to build, apply, and verify endpoints. See **[k8s/README.md](k8s/README.md)** for prerequisites, provider credentials (manifest or one-shot script), and Plex setup. From a host with `kubectl`:

```bash
./k8s/standup-and-verify.sh
# Or: sudo ./k8s/standup-and-verify.sh   # if kubectl requires root
```

Optional: `TUNER_BASE_URL=http://<node-ip>:30004` for NodePort-only; `--static` for offline Docker build.

---

## Repo layout

| Path | Purpose |
|------|---------|
| **cmd/plex-tuner/** | Main: run, index, mount, serve, probe. |
| **internal/catalog** | Catalog types, Save/Load. |
| **internal/config** | Env + subscription file. |
| **internal/tuner** | HDHR, XMLTV, M3U export, stream gateway (buffer/transcode). |
| **internal/plex** | Optional Plex DB registration. |
| **internal/provider** | Multi-host probe: RankedPlayerAPI (probe all, rank OK-by-latency), FirstWorkingPlayerAPI. |
| **internal/materializer** | VOD download (cache, direct, HLS). |
| **internal/vodfs** | FUSE VOD (Movies/TV). |
| **internal/health** | Provider health check. |
| **k8s/** | Kubernetes manifest and deploy scripts (HDHR tuner in cluster). See [k8s/README.md](k8s/README.md). |

---

## Development and QA

- **Before push:** `./scripts/verify` (format → vet → test → build). Same as CI.
- **Quick tests:** `./scripts/quick-check.sh` (tests only).
- **Cross-platform test bundles:** `./scripts/build-test-packages.sh` (archives + checksums for tester handoff).
- **When something breaks:** [docs/runbooks/plextuner-troubleshooting.md](docs/runbooks/plextuner-troubleshooting.md)—fail-fast checklist, probe, log patterns, common failures.
- **Push to both remotes:** `git push origin main && git push plex main` (see `memory-bank/repo_map.md` for remote names).

---

## For agents / template

This repo follows the **agentic template** workflow: [AGENTS.md](AGENTS.md) and [memory-bank/](memory-bank/) (e.g. `repo_map.md`, `recurring_loops.md`) define commands and process. Run `./scripts/verify`; see `memory-bank/commands.yml`. Doc gaps: [docs/docs-gaps.md](docs/docs-gaps.md).
