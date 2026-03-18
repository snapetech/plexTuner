---
id: features
type: reference
status: stable
tags: [features, reference]
---

# IPTV Tunerr — Feature list

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
| **Multi-subscription merge** | Numbered env suffix (`_2`, `_3`, ...) to pull from separate provider accounts and merge into one catalog; channels with duplicate `tvg-id` are deduplicated with all stream URLs available as fallbacks. |
| **Subscription-file credentials** | Load `Username:` / `Password:` from a subscription file when env vars are not set. |
| **Live-only / EPG-only** | Filter catalog generation to live-only or EPG-linked channels only. |
| **Stream smoketest (optional)** | Post-index stream validation: probe each channel's primary URL (Range/HEAD for MPEG-TS, playlist GET for HLS), drop channels that fail. Persistent cache avoids re-probing fresh URLs on subsequent index runs (`IPTV_TUNERR_SMOKETEST_CACHE_FILE`). |
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
| **`guide.xml`** | Layered XMLTV guide output (provider `xmltv.php` > external XMLTV > placeholder fallback). |
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

The guide pipeline merges three sources in priority order per channel: provider XMLTV > external XMLTV > placeholder. External gap-fills provider. Cache is pre-warmed at startup; stale data served on fetch error.

| Feature | Description |
|---------|-------------|
| **Provider EPG via `xmltv.php`** | Fetches EPG directly from Xtream provider using existing credentials (`IPTV_TUNERR_PROVIDER_EPG_ENABLED`). No third-party EPG source required for Xtream providers. |
| **External XMLTV fetch/remap** | Fetch external XMLTV, filter to current lineup, remap programme channel IDs to local guide numbers. Gap-fills provider EPG for uncovered time windows. |
| **Deterministic runtime EPG repair** | During catalog build, channels can have missing/wrong `TVGID`s repaired from provider/external XMLTV channel metadata before `LIVE_EPG_ONLY` filtering. |
| **Alias override source** | Optional alias file/URL (`IPTV_TUNERR_XMLTV_ALIASES`) supplies manual channel-name to XMLTV-ID mappings for long-tail repairs. |
| **Placeholder XMLTV** | Valid XMLTV output always available — per-channel fallback when neither provider nor external has data. |
| **Background refresh** | Guide cache refreshed by background goroutine on TTL tick. First refresh is synchronous (cache warm before server starts). Stale cache preserved on error. |
| **Language preference normalization** | Prefer selected language variants (for example `en,eng`) across all sources. |
| **Latin-script preference** | Prefer Latin-script title/desc variants where available. |
| **Non-Latin title fallback** | Optional fallback to channel name when title text is non-Latin and no usable variant exists. |
| **Guide number offsets** | Per-instance channel/guide number offsets to avoid Plex multi-DVR guide collisions. |

## 6. Channel intelligence

| Feature | Description |
|---------|-------------|
| **Channel health report** | Per-channel score/tier showing guide confidence, stream resilience, strengths, risks, and next actions (`channel-report` or `/channels/report.json`). |
| **EPG match provenance** | When an XMLTV source is supplied to the report command, channels show whether they matched by exact `tvg-id`, alias override, normalized-name repair, or not at all. |
| **Top opportunity summary** | Report summary highlights the highest-frequency fixes across the lineup (for example missing `TVGID`, no backup streams, or alias repair candidates). |
| **Lineup recipes** | Intelligence-driven lineup shaping with `IPTV_TUNERR_LINEUP_RECIPE=high_confidence|balanced|guide_first|resilient`. |
| **Channel DNA foundation** | Live channels now carry a persisted `dna_id` derived from repaired `TVGID` or normalized channel identity inputs, forming the basis for future cross-provider identity stability. |
| **Autopilot decision memory foundation** | Optional JSON-backed memory file remembers winning playback decisions by `dna_id + client_class`, so repeated Plex Web/native/internal requests can reuse a known-good transcode/profile choice. |
| **Ghost Hunter foundation** | `ghost-hunter` CLI and `/plex/ghost-report.json` observe Plex Live TV sessions over time, classify visible stalls with reaper heuristics, optionally stop stale visible transcode sessions, and escalate hidden-grab suspicion when Plex exposes no visible sessions. |
| **Provider behavior profile foundation** | `/provider/profile.json` exposes learned provider/runtime quirks such as effective tuner cap, recent concurrency-limit signals, Cloudflare-abuse hits, auth-context forwarding posture, and whether HLS reconnect has been auto-armed after observed instability. |
| **Guide highlights foundation** | `/guide/highlights.json` repackages the cached merged guide into immediate user-facing lanes: `current`, `starting_soon`, `sports_now`, and `movies_starting_soon`. |
| **Catch-up capsule preview foundation** | `/guide/capsules.json` turns real guide rows into previewable near-live capsule candidates with lane, publish, and expiry metadata, ready for future library publishing flows. |
| **Live TV intelligence roadmap** | Product roadmap documented as an epic: Channel DNA, Autopilot, lineup recipes, Ghost Hunter, and catch-up capsules. |

## 7. Lineup shaping for HDHR wizard / provider matching

| Feature | Description |
|---------|-------------|
| **Wizard-safe cap** | `IPTV_TUNERR_LINEUP_MAX_CHANNELS` support (commonly `479`) for Plex wizard stability. |
| **Music/radio drop heuristic** | Optional pre-cap lineup filtering by name heuristic. |
| **Regex exclusions** | Optional pre-cap channel exclusion regex. |
| **Region/profile shaping** | Pre-cap channel ordering (`IPTV_TUNERR_LINEUP_SHAPE`, `IPTV_TUNERR_LINEUP_REGION_PROFILE`) to improve provider matching behavior. |
| **Lineup sharding (overflow buckets)** | Post-filter/pre-cap slicing with `IPTV_TUNERR_LINEUP_SKIP` / `IPTV_TUNERR_LINEUP_TAKE` for `category2/category3/...` injected DVR overflow children. |

## 8. Plex integration workflows

| Workflow | Supported | Notes |
|----------|-----------|-------|
| **HDHR wizard path** | Yes | Manual Plex setup using HDHR-compatible endpoints. |
| **Wizard-equivalent API flow** | Yes (tooling / operational flows) | Programmatic creation/activation patterns via Plex endpoints used by the wizard. |
| **Injected DVR path** | Yes | Programmatic/DB-assisted DVR fleets (for example category DVRs). |
| **Guide reload + channelmap replay** | Yes | Operational workflow for remaps, offsets, and feed updates. |
| **Plex DB registration helpers** | Yes | Existing DB-assisted registration/sync flows remain supported. |

Reference:
- [plex-dvr-lifecycle-and-api](reference/plex-dvr-lifecycle-and-api.md)

## 9. Multi-instance supervisor mode (single app / single pod)

| Feature | Description |
|---------|-------------|
| **`supervise` command** | Starts multiple child `iptv-tunerr` instances from one JSON config. |
| **Child env/args isolation** | Each child gets its own args/env/workdir while sharing one binary/container. |
| **Restart/fail-fast controls** | Supervisor-level restart and fail-fast behavior. |
| **Category + HDHR split** | Supports many injected DVR children plus one (or more) HDHR wizard children in parallel. |
| **k8s cutover examples** | Example JSON/manifests and URI cutover mapping included. |
| **Overflow shard generation** | Supervisor manifest generator can auto-create overflow category children from confirmed linked counts (`--category-counts-json`, `--category-cap`). |

## 10. Plex session cleanup / stale playback handling

| Feature | Description |
|---------|-------------|
| **Built-in reaper (Go)** | Optional background worker in app for Plex Live TV stale-session cleanup. |
| **Polling + SSE** | Polls Plex sessions and optionally listens to Plex SSE notifications for faster wakeups. |
| **Idle / renewable lease / hard lease** | Tunable timers for stale-session pruning and backstop cleanup. |
| **External helper (Python)** | `scripts/plex-live-session-drain.py` remains available for lab/k8s workflows. |

## 11. VOD and VODFS

| Feature | Description |
|---------|-------------|
| **VOD cataloging** | Movies/series stored in catalog from provider feeds/APIs. |
| **VODFS mount** | FUSE-based filesystem exposing `Movies/` and `TV/`. |
| **On-demand cache** | Optional cache materialization for direct-file VOD paths. |
| **Plex library registration helper** | `plex-vod-register` creates/reuses Plex TV/Movie libraries for a VODFS mount, with optional VOD-safe library prefs. |
| **One-sided VOD registration** | `plex-vod-register --shows-only` / `--movies-only` for lane-specific library creation without unwanted companion sections. |
| **Platform scope** | `mount` / VODFS is Linux-only. Non-Linux builds provide a stub. |

## 12. Packaging, testing, and ops tooling

| Feature | Description |
|---------|-------------|
| **Cross-platform tester package builder** | `scripts/build-test-packages.sh` builds archives + checksums for Linux/macOS/Windows. |
| **Tester handoff bundle builder** | `scripts/build-tester-release.sh` stages packages + docs + examples + manifest for distribution. |
| **Tester handoff docs** | Packaging and tester checklists are documented under `docs/how-to/`. |
| **CI tester bundles** | GitHub Actions workflow builds tester bundles on demand/tag. |
| **Hidden Plex grab recovery** | `scripts/plex-hidden-grab-recover.sh` + runbook for Plex Live TV hidden active-grab wedges. |
| **Plex stream override analysis helper** | `scripts/plex-generate-stream-overrides.py` for feed/profile override candidate generation. |
| **Live TV provider label rewrite proxy** | `scripts/plex-media-providers-label-proxy.py` + k8s deploy helper to rewrite `/media/providers` labels (client-dependent effect). |

## 13. Platform support summary

| Platform | Core app (`run/serve/index/probe/supervise`) | HDHR HTTP endpoints | HDHR network mode | `mount` / VODFS |
|----------|----------------------------------------------|---------------------|-------------------|-----------------|
| Linux | Yes | Yes | Yes | Yes |
| macOS | Yes | Yes | Compiles (runtime validation depends on environment) | No |
| Windows | Yes | Yes | Compiles (native validation recommended; `wine` smoke not authoritative) | No |

## 14. Not supported / limits (current)

- **Web UI** (by design; CLI/env only)
- **Plex wizard checkbox preselection for >479 channels** (HDHR protocol/wizard limitation; serve only the channels you want selectable)
- **VODFS on non-Linux** (current platform scope)
