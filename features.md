# Plex Tuner — Features

Short feature overview. Full reference: **[docs/features.md](docs/features.md)**.

---

## Input and indexing

- **M3U URL** — Single M3U via URL (e.g. provider `get.php`). Live channels and optional VOD/series.
- **Xtream player_api** — First-class: live, VOD movies, series from `player_api.php`. Same approach as xtream-to-m3u.js; we prefer non-Cloudflare hosts for stream URLs and use `.m3u8` for playback.
- **Multi-host** — `PLEX_TUNER_PROVIDER_URLS`: try each URL; first HTTP 200 wins. Fallback to get.php per host.
- **Subscription file** — Creds from a file (`Username:` / `Password:`). Env or default `~/Documents/iptv.subscription.2026.txt`.
- **Live-only / EPG-only / Smoketest** — Live-only skips VOD/series; EPG-only keeps channels with tvg-id; smoketest drops channels whose stream fails at index time.

---

## Catalog

- **JSON catalog** — One file (default `catalog.json`): live channels, movies, series. Snapshot-then-write so readers never see half-updated data.
- **Backup stream URLs** — Per channel: multiple URLs; gateway tries in order on failure.
- **EPG metadata** — name, tvg-id, tvg-logo, group for lineup and XMLTV.

---

## Tuner (HDHomeRun)

- **Endpoints** — `discover.json`, `lineup.json`, `lineup_status.json`, `guide.xml`, `live.m3u`, `/stream/<id>`.
- **Stream gateway** — Proxy to provider with auth, tuner count limit, backup URLs. HLS → remux or transcode to MPEG-TS (ffmpeg when available).
- **Stream buffering** — `PLEX_TUNER_STREAM_BUFFER_BYTES`: `0` = off, `auto` = **adaptive** when transcoding (64 KiB–2 MiB from backpressure), or fixed bytes. Default `auto`.
- **Stream transcoding** — `PLEX_TUNER_STREAM_TRANSCODE`: `off` = remux only, `on` = always transcode (libx264/aac), `auto` = transcode only when codec isn’t Plex-friendly (ffprobe).
- **Tuner count / Base URL** — Configurable concurrent streams; base URL is what Plex uses to reach this host.

---

## EPG / XMLTV

- **Placeholder guide** — Default `/guide.xml`: valid XMLTV, channel entries only (no programmes).
- **External XMLTV** — `PLEX_TUNER_XMLTV_URL`: we fetch, filter to catalog channels, remap IDs to our lineup.
- **EPG prune** — Guide and M3U only include channels with tvg-id.

---

## VOD and VODFS

- **VOD in catalog** — Movies and series from player_api (or M3U) stored in catalog.
- **FUSE (VODFS)** — Mount catalog as `Movies/` and `TV/`. Virtual files; Plex (or anything else) can scan the mount.
- **Optional cache** — On-demand download when a file is opened; HLS stays pass-through.

---

## Operations

- **Subcommands** — `run`, `index`, `serve`, `mount`, `probe`.
- **run** — One-shot: refresh catalog → health check → serve. Optional `-refresh=6h`, `-register-plex=...`.
- **probe** — Hit every provider URL; report get.php and player_api (OK / Cloudflare / fail) and latency.
- **Plex DB registration** — `-register-plex=/path`: write tuner and XMLTV URLs into Plex’s DB (stop Plex first; backup DB).
- **Config** — Env and `.env`; no web UI.

---

## Not supported (by design)

- **Web UI** — CLI and env only.
- **Channel mapping UI** — Filtering via env (EPG-only, smoketest, live-only).
- **Plex API DVR creation** — We only have DB-based `RegisterTuner`; we don’t create DVRs via Plex’s HTTP API.

---

**Full list and details:** [docs/features.md](docs/features.md) · **Quick start:** [README.md](README.md)
