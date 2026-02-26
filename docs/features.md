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
| **External helper (Python)** | `scripts/plex-live-session-drain.py` remains available for lab/k8s workflows. |

## 10. VOD and VODFS

| Feature | Description |
|---------|-------------|
| **VOD cataloging** | Movies/series stored in catalog from provider feeds/APIs. |
| **VODFS mount** | FUSE-based filesystem exposing `Movies/` and `TV/`. |
| **On-demand cache** | Optional cache materialization for direct-file VOD paths. |
| **Plex library registration helper** | `plex-vod-register` creates/reuses Plex TV/Movie libraries for a VODFS mount, with optional VOD-safe library prefs. |
| **One-sided VOD registration** | `plex-vod-register --shows-only` / `--movies-only` for lane-specific library creation without unwanted companion sections. |
| **Platform scope** | `mount` / VODFS is Linux-only. Non-Linux builds provide a stub. |

## 11. Packaging, testing, and ops tooling

| Feature | Description |
|---------|-------------|
| **Cross-platform tester package builder** | `scripts/build-test-packages.sh` builds archives + checksums for Linux/macOS/Windows. |
| **Tester handoff bundle builder** | `scripts/build-tester-release.sh` stages packages + docs + examples + manifest for distribution. |
| **Tester handoff docs** | Packaging and tester checklists are documented under `docs/how-to/`. |
| **CI tester bundles** | GitHub Actions workflow builds tester bundles on demand/tag. |
| **Hidden Plex grab recovery** | `scripts/plex-hidden-grab-recover.sh` + runbook for Plex Live TV hidden active-grab wedges. |
| **Plex stream override analysis helper** | `scripts/plex-generate-stream-overrides.py` for feed/profile override candidate generation. |
| **Live TV provider label rewrite proxy** | `scripts/plex-media-providers-label-proxy.py` + k8s deploy helper to rewrite `/media/providers` labels (client-dependent effect). |

## 12. Platform support summary

| Platform | Core app (`run/serve/index/probe/supervise`) | HDHR HTTP endpoints | HDHR network mode | `mount` / VODFS |
|----------|----------------------------------------------|---------------------|-------------------|-----------------|
| Linux | Yes | Yes | Yes | Yes |
| macOS | Yes | Yes | Compiles (runtime validation depends on environment) | No |
| Windows | Yes | Yes | Compiles (native validation recommended; `wine` smoke not authoritative) | No |

## 13. Not supported / limits (current)

- **Web UI** (by design; CLI/env only)
- **Plex wizard checkbox preselection for >479 channels** (HDHR protocol/wizard limitation; serve only the channels you want selectable)
- **VODFS on non-Linux** (current platform scope)
