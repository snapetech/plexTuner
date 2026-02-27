---
id: features
type: reference
status: stable
tags: [features, reference]
---

# Plex Tuner — Feature list

Canonical feature list for the current app.

See also:
- [README](../README.md)
- [cli-and-env-reference](reference/cli-and-env-reference.md)
- [plex-dvr-lifecycle-and-api](reference/plex-dvr-lifecycle-and-api.md)

## 1. Input and indexing

| Feature | Description |
|---------|-------------|
| **M3U URL** | Fetch and parse IPTV M3U lineups (live, and VOD/series when present). |
| **Xtream `player_api`** | First-class indexing for live, movies, and series. |
| **Multi-host probing + ranking** | Probe all provider URLs, rank by health/latency, index from the best host, and store backup stream URLs for failover. |
| **Subscription-file credentials** | Load `Username:` / `Password:` from a subscription file when env vars are not set. |
| **Live-only / EPG-only** | Filter catalog generation to live-only or EPG-linked channels only. |
| **Stream smoketest (optional)** | Drop channels that fail probe checks during indexing. |
| **EPG-link report (Phase 1)** | Deterministic coverage/unmatched report for live channels vs XMLTV (`epg-link-report`). |

## 2. Catalog and stream source handling

| Feature | Description |
|---------|-------------|
| **JSON catalog** | Single catalog file for live channels, movies, and series. |
| **Atomic-ish write behavior** | Snapshot-then-write flow to avoid partial read state. |
| **Per-channel backup stream URLs** | Gateway can fail over to secondary/tertiary provider URLs. |
| **EPG metadata retention** | Keeps tvg-id/name/logo/group metadata for lineup and XMLTV mapping. |
| **Cached startup** | On restart, the tuner immediately serves the last saved catalog to Plex while a background refresh updates the lineup. Clients see no gap in the guide. |
| **Second-provider merge** | Live channels from a second M3U/Xtream provider merged and deduplicated into the primary catalog. Merged channels tagged `source_tag: "provider2"`. |

## 3. HDHomeRun-compatible tuner service

| Feature | Description |
|---------|-------------|
| **`discover.json`** | HDHR discovery endpoint (Plex-compatible). |
| **`lineup.json` / `lineup_status.json`** | Tuner channel lineup and status endpoints. |
| **`guide.xml`** | Placeholder or remapped external XMLTV guide output. |
| **`live.m3u`** | Live channel M3U export. |
| **`/stream/<id>`** | Stream gateway with provider auth/failover and tuner count limiting. |
| **Tuner count limit** | Configurable concurrent streams, HDHR-style “all tuners in use” behavior. |
| **HDHR metadata controls** | Per-instance discover metadata (manufacturer/model/fw/device auth) and `ScanPossible` control for wizard-lane vs category tuners. |

## 4. Stream gateway and transcoding

| Feature | Description |
|---------|-------------|
| **HLS -> MPEG-TS relay** | Native relay path for HLS inputs to Plex-facing TS output. |
| **ffmpeg transcode path** | Optional ffmpeg-based remux/transcode for web-safe / Plex-friendly output. |
| **Transcode modes** | `off`, `on`, `auto` (codec/probe-driven behavior depending config/runtime). |
| **Startup/bootstrap controls** | Web-safe bootstrap and startup-gate tuning for Plex web playback behavior. |
| **Client disconnect handling** | Better classification of downstream disconnects to avoid false relay errors. |
| **Optional realtime ffmpeg pacing** | HLS ffmpeg `-re` option (env-controlled) for startup pacing experiments. |

## 5. XMLTV / EPG behavior

| Feature | Description |
|---------|-------------|
| **Placeholder XMLTV** | Valid XMLTV output even without external guide source. |
| **External XMLTV fetch/remap** | Fetch XMLTV, filter to current lineup, remap programme channel IDs to local guide numbers. |
| **Language preference normalization** | Prefer selected language variants (for example `en,eng`) when multiple programme nodes exist. |
| **Latin-script preference** | Prefer Latin-script title/desc variants where available. |
| **Non-Latin title fallback** | Optional fallback to channel name when title text is non-Latin and no usable variant exists. |
| **Guide number offsets** | Per-instance channel/guide number offsets to avoid Plex multi-DVR guide collisions. |
| **Dummy guide fallback** | When `PLEX_TUNER_DUMMY_GUIDE=true`, appends 24 × 6-hour placeholder programme blocks for every channel with no real EPG programmes, preventing Plex from hiding or deactivating those channels. |

## 5a. EPG enrichment pipeline

An 8-tier automatic enrichment pipeline runs during `fetchCatalog` before `LIVE_EPG_ONLY` filtering.

| Tier | Feature | Description |
|------|---------|-------------|
| 1 | **Re-encode inheritance** | Channels labelled `ᴿᴬᵂ`/`4K`/`ᵁᴴᴰ` with no tvg-id inherit the base channel's tvg-id. Quality tier set: UHD=2, HD=1, SD=0, RAW=-1. |
| 2 | **Gracenote enrichment** | callSign/gridKey matching via the Gracenote DB harvested from plex.tv (`PLEX_TUNER_GRACENOTE_DB`). |
| 3 | **iptv-org enrichment** | Name/shortcode matching via the iptv-org community channel DB (~39k channels, `PLEX_TUNER_IPTVORG_DB`). |
| 4 | **SDT name propagation** | If a channel display name looks like garbage (numeric ID, UUID, etc.) and the background SDT probe has stored a `service_name`, the display name is replaced for downstream matching. |
| 5 | **Schedules Direct enrichment** | callSign/station-name matching via a local SD station DB (`PLEX_TUNER_SD_DB`). Produces `SD-<stationID>` tvg-ids compatible with SD XMLTV grabbers. |
| 6 | **DVB DB enrichment** | For channels with a probed DVB triplet (ONID+TSID+SID), looks up broadcaster name and optional tvg-id in the DVB services DB (`PLEX_TUNER_DVB_DB`). |
| 7 | **Brand-group inheritance** | Second-pass sweep clustering regional/quality variants (`ABC East`, `ABC HD`, `ABC 2`) under a canonical brand tvg-id. |
| 8 | **Best-stream selection** | For each tvg-id, keeps only the highest-quality stream (UHD > HD > SD > RAW). Removes duplicate lower-quality encodes. |

Background (post-catalog): **SDT background prober** (see §12 below).

Reference: [EPG Long-Tail Strategies](explanations/epg-long-tail-strategies.md) | [cli-and-env-reference](reference/cli-and-env-reference.md#epg-enrichment-pipeline)

## 6. Lineup shaping for HDHR wizard / provider matching

| Feature | Description |
|---------|-------------|
| **Wizard-safe cap** | `PLEX_TUNER_LINEUP_MAX_CHANNELS` support (commonly `479`) for Plex wizard stability. |
| **Music/radio drop heuristic** | Optional pre-cap lineup filtering by name heuristic. |
| **Regex exclusions** | Optional pre-cap channel exclusion regex. |
| **Region/profile shaping** | Pre-cap channel ordering (`PLEX_TUNER_LINEUP_SHAPE`, `PLEX_TUNER_LINEUP_REGION_PROFILE`) to improve provider matching behavior. |
| **Lineup sharding (overflow buckets)** | Post-filter/pre-cap slicing with `PLEX_TUNER_LINEUP_SKIP` / `PLEX_TUNER_LINEUP_TAKE` for `category2/category3/...` injected DVR overflow children. |

## 7. Plex integration workflows

| Workflow | Supported | Notes |
|----------|-----------|-------|
| **HDHR wizard path** | Yes | Manual Plex setup using HDHR-compatible endpoints. |
| **Wizard-equivalent API flow** | Yes (tooling / operational flows) | Programmatic creation/activation patterns via Plex endpoints used by the wizard. |
| **Injected DVR path** | Yes | Programmatic/DB-assisted DVR fleets (for example category DVRs). |
| **Guide reload + channelmap replay** | Yes | Operational workflow for remaps, offsets, and feed updates. |
| **Plex DB registration helpers** | Yes | Existing DB-assisted registration/sync flows remain supported. |

Reference:
- [plex-dvr-lifecycle-and-api](reference/plex-dvr-lifecycle-and-api.md)

## 7a. Manual control endpoints

| Feature | Description |
|---------|-------------|
| **`POST /refresh`** | Immediately queues a full catalog re-fetch (equivalent to the normal periodic refresh, but on demand). Returns `202 Accepted`. |
| **`GET /refresh`** | Returns current catalog-refresh status. |
| **`POST /rescan`** | Triggers a forced full SDT rescan sweep, ignoring the per-channel cache TTL. All unlinked channels are re-probed. Returns `202 Accepted`. |
| **`GET /rescan`** | Returns current SDT rescan status. |

Both endpoints are available on the tuner's HTTP port (same as `/lineup.json`, etc.).

## 8. Multi-instance supervisor mode (single app / single pod)

| Feature | Description |
|---------|-------------|
| **`supervise` command** | Starts multiple child `plex-tuner` instances from one JSON config. |
| **Child env/args isolation** | Each child gets its own args/env/workdir while sharing one binary/container. |
| **Restart/fail-fast controls** | Supervisor-level restart and fail-fast behavior. |
| **Category + HDHR split** | Supports many injected DVR children plus one (or more) HDHR wizard children in parallel. |
| **k8s cutover examples** | Example JSON/manifests and URI cutover mapping included. |
| **Overflow shard generation** | Supervisor manifest generator can auto-create overflow category children from confirmed linked counts (`--category-counts-json`, `--category-cap`). |

## 9. Plex session cleanup / stale playback handling

| Feature | Description |
|---------|-------------|
| **Built-in reaper (Go)** | Optional background worker in app for Plex Live TV stale-session cleanup. |
| **Polling + SSE** | Polls Plex sessions and optionally listens to Plex SSE notifications for faster wakeups. |
| **Idle / renewable lease / hard lease** | Tunable timers for stale-session pruning and backstop cleanup. |
| **`plex-session-drain` subcommand** | One-shot or continuous watch-mode session drain via the Plex API. Replaces `scripts/plex-live-session-drain.py`. |

## 10. VOD and VODFS

| Feature | Description |
|---------|-------------|
| **VOD cataloging** | Movies/series stored in catalog from provider feeds/APIs. |
| **VODFS mount** | FUSE-based filesystem exposing `Movies/` and `TV/`. |
| **On-demand cache** | Optional cache materialization for direct-file VOD paths. |
| **Plex library registration helper** | `plex-vod-register` creates/reuses Plex TV/Movie libraries for a VODFS mount, with optional VOD-safe library prefs. |
| **One-sided VOD registration** | `plex-vod-register --shows-only` / `--movies-only` for lane-specific library creation without unwanted companion sections. |
| **Platform scope** | `mount` / VODFS is Linux-only. Non-Linux builds provide a stub. |

## 11. Ops tooling and ported subcommands

All previously Python-scripted ops helpers are now native Go subcommands in the `plex-tuner` binary. Python scripts have been removed.

| Feature | Description |
|---------|-------------|
| **`plex-session-drain`** | Drain Plex Live TV sessions via API (one-shot or watch mode). Replaces `scripts/plex-live-session-drain.py`. |
| **`plex-label-proxy`** | Reverse proxy rewriting `/media/providers` XML with per-DVR labels from Plex. Replaces `scripts/plex-media-providers-label-proxy.py`. |
| **`plex-probe-overrides`** | Probe lineup channel URLs with `ffprobe`; generate Plex profile + transcode override JSON files for streams with codec issues. Replaces `scripts/plex-generate-stream-overrides.py`. |
| **`generate-supervisor-config`** | Generate supervisor JSON, k8s singlepod YAML, and cutover TSV from the HDHR deployment template. Includes `--out-tsv` for cutover mapping (replaces `scripts/plex-supervisor-cutover-map.py`). |
| **`vod-backfill-series`** | Refetch per-series episode info from the Xtream API and rewrite catalog series seasons. Replaces `scripts/vod-backfill-series-catalog.py`. |
| **`plex-gracenote-harvest`** | Harvest plex.tv Gracenote channel DB for all world regions. Replaces `scripts/plex-wizard-epg-harvest.py`. |
| **Cross-platform tester package builder** | `scripts/build-test-packages.sh` builds archives + checksums for Linux/macOS/Windows. |
| **Tester handoff bundle builder** | `scripts/build-tester-release.sh` stages packages + docs + examples + manifest for distribution. |
| **Tester handoff docs** | Packaging and tester checklists are documented under `docs/how-to/`. |
| **CI tester bundles** | GitHub Actions workflow builds tester bundles on demand/tag. |
| **Hidden Plex grab recovery** | `scripts/plex-hidden-grab-recover.sh` + runbook for Plex Live TV hidden active-grab wedges. |

## 11a. SDT background prober

| Feature | Description |
|---------|-------------|
| **MPEG-TS SDT parsing** | Reads DVB Service Description Table (PID 0x0011) from live stream bytes; extracts full DVB identity bundle: ONID, TSID, SID, ProviderName, ServiceName, EIT schedule/present-following flags, now/next programme titles. |
| **Polite background sweep** | Auto-pauses when any IPTV stream is active; resumes after `PLEX_TUNER_SDT_PROBE_QUIET_WINDOW` (default 3 min) of idle. Configurable concurrency (default 2) and inter-probe delay (default 500 ms). |
| **Persistent cache** | Per-channel cache with TTL (default 7 days). Already-probed channels skipped until TTL expires. |
| **Forced rescan** | `POST /rescan` or `PLEX_TUNER_SDT_PROBE_RESCAN_INTERVAL` auto-monthly: ignores cache TTL and re-probes all unlinked channels. |
| **Catalog integration** | On `service_name` discovery, updates in-memory catalog and writes back to disk. Lineup served to Plex is refreshed immediately. |
| **Head-start delay** | Configurable via `PLEX_TUNER_SDT_PROBE_START_DELAY` (default 30 s) so the prober doesn't race against startup stream activity. |

## 12. Platform support summary

| Platform | Core app (`run/serve/index/probe/supervise`) | HDHR HTTP endpoints | HDHR network mode | `mount` / VODFS |
|----------|----------------------------------------------|---------------------|-------------------|-----------------|
| Linux | Yes | Yes | Yes | Yes |
| macOS | Yes | Yes | Compiles (runtime validation depends on environment) | No |
| Windows | Yes | Yes | Compiles (native validation recommended; `wine` smoke not authoritative) | No |

All ported subcommands (`plex-session-drain`, `plex-label-proxy`, `plex-probe-overrides`, `generate-supervisor-config`, `vod-backfill-series`, harvest commands) are cross-platform (Linux/macOS/Windows). `plex-probe-overrides` requires `ffprobe` to be on `PATH`.

## 13. Not supported / limits (current)

- **Web UI** (by design; CLI/env only)
- **Plex wizard checkbox preselection for >479 channels** (HDHR protocol/wizard limitation; serve only the channels you want selectable)
- **VODFS on non-Linux** (current platform scope)
- **SDT probing on non-MPEG-TS streams** (HLS-only providers without TS encapsulation will yield no SDT data)
