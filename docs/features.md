---
id: features
type: reference
status: stable
tags: [features, reference]
---

# Plex Tuner — Feature list

Canonical list of features. See [README](../README.md) for quick start and [comparison matrix](../README.md#comparison-plex-tuner-vs-alternatives) vs xTeVe/Threadfin.

---

## 1. Input and indexing

| Feature | Description |
|---------|-------------|
| **M3U URL** | Fetch a single M3U via URL (e.g. provider `get.php?username=...&password=...&type=m3u_plus&output=ts`). Parsed for live channels and optional VOD/series when present. |
| **Xtream player_api** | First-class indexing from `player_api.php`: live, VOD movies, series. Same strategy as xtream-to-m3u.js: auth → server_info → prefer non-Cloudflare host for stream URLs, `.m3u8` for playback. |
| **Multi-host (first OK wins)** | Multiple provider base URLs (e.g. `PLEX_TUNER_PROVIDER_URLS=http://h1,http://h2`). We try player_api on each; first HTTP 200 wins. Fallback: get.php on each host. |
| **Subscription file** | Credentials can be read from a file (e.g. `iptv.subscription.2026.txt`) with `Username:` and `Password:` lines. Env `PLEX_TUNER_SUBSCRIPTION_FILE` or default `~/Documents/iptv.subscription.2026.txt`. |
| **Live-only mode** | `PLEX_TUNER_LIVE_ONLY=true`: only fetch live channels from API (skip VOD and series); faster indexing. |
| **EPG-linked only** | `PLEX_TUNER_LIVE_EPG_ONLY=true`: only include channels that have `tvg-id` (EPG link) in the catalog. |
| **Stream smoketest** | `PLEX_TUNER_SMOKETEST_ENABLED=true`: at index time, probe each channel’s primary stream URL (HLS playlist + segment); drop channels that fail. Configurable timeout and concurrency. |

---

## 2. Catalog

| Feature | Description |
|---------|-------------|
| **JSON catalog** | Single file (default `catalog.json`) holding live channels, movies, and series. |
| **Snapshot-then-encode** | Safe concurrent writes: snapshot in memory, then encode and write so readers never see partial state. |
| **Backup stream URLs** | Live channels can have multiple `StreamURLs`; gateway tries in order on failure. |
| **EPG metadata** | Per-channel: name, tvg-id, tvg-logo, group; used for lineup and XMLTV. |

---

## 3. Tuner (HDHomeRun emulation)

| Feature | Description |
|---------|-------------|
| **discover.json** | HDHomeRun discovery endpoint so Plex can find the tuner. |
| **lineup.json** | Channel lineup for Plex DVR. |
| **lineup_status.json** | Status endpoint. |
| **guide.xml** | XMLTV guide. Default: placeholder. Optional: fetch external XMLTV URL, filter to catalog channels, remap programme channel IDs to local guide numbers. |
| **live.m3u** | M3U export of live channels (for external use or debugging). |
| **/stream/<id>** | Stream gateway: proxy to provider URL with optional basic auth; tuner count limit; fallback to backup URLs. |
| **Tuner count** | Configurable concurrent stream limit (`PLEX_TUNER_TUNER_COUNT`, default 2). Returns HDHomeRun error 805 when all tuners in use. |
| **Base URL** | Configurable base URL for discover/lineup/guide so Plex can reach this host (`PLEX_TUNER_BASE_URL` or `-base-url`). |

---

## 4. EPG / XMLTV

| Feature | Description |
|---------|-------------|
| **Placeholder guide** | Default `/guide.xml`: minimal valid XMLTV with channel entries and no programmes (Plex still shows channels). |
| **External XMLTV** | `PLEX_TUNER_XMLTV_URL`: fetch external feed, keep only channels present in live catalog, remap programme channel IDs to our lineup numbers. Timeout configurable. |
| **EPG prune** | `PLEX_TUNER_EPG_PRUNE_UNLINKED=true`: guide.xml and live.m3u only include channels that have tvg-id set (reduces noise). |

---

## 5. VOD and VODFS

| Feature | Description |
|---------|-------------|
| **VOD in catalog** | Movies and series from player_api (or M3U when present) stored in catalog. |
| **FUSE mount (VODFS)** | Mount catalog as a filesystem: `Movies/` and `TV/` (series) with virtual files. Plex or other tools can scan the mount. |
| **Optional cache** | With `-cache` / `PLEX_TUNER_CACHE`, direct-file URLs are downloaded on demand and served from cache; HLS remains pass-through. |

---

## 6. Operations and deployment

| Feature | Description |
|---------|-------------|
| **Subcommands** | `run`, `index`, `serve`, `mount`, `probe` so indexing, serving, and mounting can be run separately or together. |
| **run (one-shot)** | For systemd/Docker: refresh catalog (unless `-skip-index`), provider health check (unless `-skip-health`), then serve. Optional scheduled refresh (`-refresh=6h`). |
| **probe** | Cycle through provider URLs; report get.php and player_api status (OK / Cloudflare / fail) and latency. Use to choose which host to use. |
| **Health check** | Before serve in `run`, optional HTTP check of provider player_api URL; exit non-zero if down. |
| **Plex DB registration** | Optional `-register-plex=/path/to/Plex/Media/Server`: update Plex’s SQLite DB so DVR and XMLTV point to this tuner (stop Plex first; backup DB). |
| **Config** | Env vars and `.env` file; no web UI. Subscription file fallback for credentials. |

---

## 7. Not supported (by design)

- **Web UI** — configuration is CLI and env only.
- **Stream buffering / transcoding** — we proxy; no ffmpeg buffer or transcode.
- **Channel mapping UI** — filtering is via env (EPG-only, smoketest, live-only).
- **Plex API DVR creation** — we do not create DVRs via Plex HTTP API or perform channelmap activation; we only have DB-based `RegisterTuner` to point existing provider rows at our URLs.

