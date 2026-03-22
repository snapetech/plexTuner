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
- [Docs index](index.md) · [CHANGELOG](CHANGELOG.md) (version history + **[Unreleased]** slices)
- [cli-and-env-reference](reference/cli-and-env-reference.md)
- [hls-mux-toolkit](reference/hls-mux-toolkit.md) · [plex-livetv-http-tuning](reference/plex-livetv-http-tuning.md)
- [plex-dvr-lifecycle-and-api](reference/plex-dvr-lifecycle-and-api.md)

## 1. Input and indexing

| Feature | Description |
|---------|-------------|
| **M3U URL** | Fetch and parse IPTV M3U lineups (live, and VOD/series when present). |
| **Xtream `player_api`** | First-class indexing for live, movies, and series. |
| **Multi-host probing + ranking** | Probe all provider URLs, rank by health/latency, index from the best host, and store backup stream URLs for failover. |
| **Multi-subscription merge** | Numbered env suffix (`_2`, `_3`, ...) to pull from separate provider accounts and merge into one catalog; channels with duplicate `tvg-id` are deduplicated with all stream URLs available as fallbacks, and live playback can now spread active streams across distinct provider-account credential sets. Provider-account contention is no longer static only: Tunerr can learn tighter per-account concurrency caps from real upstream limit signals, persist them across restarts with TTL decay, and expose them on `/provider/profile.json`. |
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
| **`/healthz` / `/readyz`** | Catalog readiness for operators and Kubernetes: **`/readyz`** returns JSON **`ready`** / **`not_ready`** (**503** until **`UpdateChannels`** has live rows); **`/healthz`** returns **`ok`** / **`loading`** plus **`source_ready`**, **`channels`**, **`last_refresh`**. Example **`k8s/`** readiness probes use **`/readyz`**; **`/discover.json`** is often a better **liveness** target during long cold starts. See [runbook §8](runbooks/iptvtunerr-troubleshooting.md#8-tuner-endpoints-sanity-check). |
| **`guide.xml`** | Layered XMLTV guide output (provider `xmltv.php` > external XMLTV > optional HDHR device `guide.xml` gap-fill > placeholder). While the real merged guide is still building, Tunerr returns **`503`** with a visible placeholder body plus **`Retry-After: 5`** and **`X-IptvTunerr-Guide-State: loading`** instead of a misleading success response. |
| **`live.m3u`** | Live channel M3U export. |
| **`/stream/<id>`** | Stream gateway with provider auth/failover and tuner count limiting. |
| **Tuner count limit** | Configurable concurrent streams, HDHR-style “all tuners in use” behavior. |
| **HDHR metadata controls** | Per-instance discover metadata (manufacturer/model/fw/device auth) and `ScanPossible` control for wizard-lane vs category tuners. |
| **Physical HDHomeRun client (spike)** | `hdhr-scan` discovers real tuners on the LAN (UDP, global IPv4 broadcast + optional **`IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS`** IPv4 and **literal IPv6** targets on UDP6) or fetches `discover.json` / `lineup.json` via HTTP; catalog merge is **not** automatic — see [ADR 0002](adr/0002-hdhr-hardware-iptv-merge.md). |
| **HDHR `guide.xml`** | `hdhr-scan -guide-xml` probes device XMLTV (counts). **`IPTV_TUNERR_HDHR_GUIDE_URL`** merges that feed into `/guide.xml` when `tvg-id` values match ([ADR 0004](adr/0004-hdhr-guide-epg-merge.md)). |
| **HDHR `lineup.json` → catalog** | **`IPTV_TUNERR_HDHR_LINEUP_URL`** merges hardware channels during **`iptv-tunerr index`** ([how-to](how-to/hybrid-hdhr-iptv.md)). |
| **Dedicated web UI (`:48879` / `0xBEEF`)** | Separate operator dashboard on `serve`/`run` with a single-origin proxy over tuner JSON/debug endpoints plus a runtime-settings snapshot (`/debug/runtime.json`). Default: `http://127.0.0.1:48879/`; opens on a dedicated login page with cookie-backed deck sessions, while direct HTTP Basic auth still works for scripts; if `IPTV_TUNERR_WEBUI_PASS` is unset, Tunerr generates a one-time startup password instead of defaulting to `admin/admin`; localhost-only unless `IPTV_TUNERR_WEBUI_ALLOW_LAN=1`; optional `IPTV_TUNERR_WEBUI_STATE_FILE` persists server-derived operator activity and non-secret deck preferences across web UI restarts. |
| **Deck actions / workflows / memory** | The deck exposes safe operator actions (`/ops/actions/*`), workflow/playbook endpoints (`/ops/workflows/*`), a read-only deck telemetry surface (`/deck/telemetry.json`), server-derived operator activity (`/deck/activity.json`), and refresh-preference controls (`/deck/settings.json`) with grouped endpoint indexing, trends, and a runtime-backed settings lane. **Provider profile** cards summarize flat **`/provider/profile.json`** (tuner limits, penalties, **host quarantine** skips + quarantined-host counts when relevant, mux/autopilot breadcrumbs, **Autopilot consensus** when computed) and surface **`remediation_hints`** on Overview/Routing (incidents + decision board when warn-level hints exist). |
| **Deck session hardening** | Cookie-backed deck sessions use a session-bound `X-IPTVTunerr-Deck-CSRF` token for state-changing requests, plus login failure rate limiting and explicit POST sign-out. |
| **Event webhooks** | File-backed webhook dispatch (`IPTV_TUNERR_EVENT_WEBHOOKS_FILE`) can emit `lineup.updated`, `stream.requested`, `stream.rejected`, and `stream.finished` as JSON POSTs, with recent-delivery visibility under `/debug/event-hooks.json`. |
| **Active stream view + stop control** | `/debug/active-streams.json` exposes currently in-flight stream sessions with request IDs, channels, client UA, durations, tuner occupancy, and whether the session is cancelable. Operators can request a live cancellation via `/ops/actions/stream-stop`, which cancels matching active stream contexts by request ID or channel ID. |
| **Xtream-compatible output (expanded starter)** | Optional read-only downstream Xtream surface: `player_api.php` now serves `get_live_streams`, `get_live_categories`, `get_vod_categories`, `get_vod_streams`, `get_series_categories`, `get_series`, and `get_series_info`; `/live/<user>/<pass>/<channel>.ts`, `/movie/<user>/<pass>/<id>.mp4`, and `/series/<user>/<pass>/<episode>.mp4` proxy through Tunerr-owned URLs instead of exposing raw upstreams directly. A file-backed multi-user entitlement layer (`IPTV_TUNERR_XTREAM_USERS_FILE`, `/entitlements.json`) can now scope live/VOD/series access per downstream user instead of treating the Xtream output as one global view. |
| **Recording rules + history (starter)** | `IPTV_TUNERR_RECORDING_RULES_FILE` enables durable server-side recording rules. `/recordings/rules.json` stores and mutates rule definitions, `/recordings/rules/preview.json` shows which current/upcoming catch-up capsules would match each rule, and `/recordings/history.json` classifies recorder state against the current rules so operators can see which rules are actually producing completed or failed work. |
| **Programming Manager** | Server-backed lineup curation primitives now exist and are operable from the dedicated control deck. `/programming/categories.json` builds grouped source/category inventory and supports bulk category include/exclude, `/programming/channels.json` handles exact channel include/exclude overrides, `/programming/channel-detail.json` gives focused channel metadata plus a 3-hour upcoming-programme preview and exact-match alternative sources, `/programming/order.json` persists manual server-side order changes, `/programming/backups.json` reports exact-match sibling groups, `/programming/recipe.json` stores the durable recipe, and `/programming/preview.json` shows the curated lineup plus taxonomy bucket counts after the recipe is applied. `order_mode: "recommended"` now produces a deterministic Local/News/Sports/etc. ordering on the server, `collapse_exact_backups: true` can collapse strong same-channel siblings into one visible row with merged fallback streams, and the web UI Programming lane lets operators drive those decisions without hand-posting JSON. |
| **Plex lineup harvest** | `iptv-tunerr plex-lineup-harvest` productizes the old wizard-oracle flow: it can probe several tuner lineup variants against Plex, poll channel-map results, capture discovered lineup titles/URLs, and emit a structured report plus deduped lineup summary for later Programming Manager or wizard-lane decisions. |
| **Shared HLS relay reuse (foundation)** | Same-channel duplicate consumers can now attach to one live HLS Go-relay session instead of always starting another upstream walk. The foundation is intentionally bounded to the native HLS Go-relay path, and `/debug/shared-relays.json` exposes current shared sessions and subscriber counts. |
| **Legacy `/ui/` shell** | Embedded lightweight pages remain on the tuner port: home (`/ui/`) links to health and JSON reports; **`/ui/guide/`** shows a read-only preview of the merged cached guide; localhost-only unless `IPTV_TUNERR_UI_ALLOW_LAN=1`. |

## 4. Stream gateway and transcoding

| Feature | Description |
|---------|-------------|
| **HLS -> MPEG-TS relay** | Native relay path for HLS inputs to Plex-facing TS output. |
| **Tunerr-native HLS / DASH manifest proxy** | **`?mux=hls`** returns a rewritten **M3U8**; experimental **`?mux=dash`** rewrites **MPD** absolutes to **`?mux=dash&seg=…`**. Successful native mux responses may set response header **`X-IptvTunerr-Native-Mux: hls`** or **`dash`** (entry playlist/MPD, **`seg=`** relay incl. **304**, internal mux target paths); listed in **`Access-Control-Expose-Headers`** when **`IPTV_TUNERR_HLS_MUX_CORS`** is on. Optional **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`**, **`IPTV_TUNERR_HLS_MUX_CORS`**, concurrency caps, literal + **DNS-resolved** private blocking, per-IP **`seg=`** rate limit, Prometheus **`/metrics`**, demo page, **`POST .../mux-seg-decode`**. Counters: **`hls_mux_seg_*`** and **`dash_mux_seg_*`** + **`last_*_mux_outcome`** breadcrumbs on **`provider_profile.json`**. See [hls-mux-toolkit](reference/hls-mux-toolkit.md). |
| **Named stream profile matrix** | Optional **`IPTV_TUNERR_STREAM_PROFILES_FILE`** JSON maps operator profile names → **`base_profile`**, **`transcode`**, preferred **`output_mux`** (`mpegts` / `hls` / `fmp4`), used with **`?profile=`**, env defaults, and **`IPTV_TUNERR_PROFILE_OVERRIDES_FILE`** (**LP-010** / **LP-011**) — [transcode-profiles](reference/transcode-profiles.md). |
| **ffmpeg transcode path** | Optional ffmpeg remux/transcode; MPEG-TS default; experimental **`?mux=fmp4`** fragmented MP4 when transcoding HLS; named profiles can also prefer **ffmpeg-packaged HLS** (`output_mux: "hls"`) with Tunerr-served playlist and segment session URLs — [transcode-profiles](reference/transcode-profiles.md). |
| **Transcode modes** | `off`, `on`, `auto` (codec/probe-driven behavior depending config/runtime). |
| **Startup/bootstrap controls** | Web-safe bootstrap and startup-gate tuning for Plex web playback behavior. |
| **Client disconnect handling** | Better classification of downstream disconnects to avoid false relay errors. |
| **Optional realtime ffmpeg pacing** | HLS ffmpeg `-re` option (env-controlled) for startup pacing experiments. |

## 5. XMLTV / EPG behavior

The guide pipeline merges three sources in priority order per channel: provider XMLTV > external XMLTV > placeholder. External gap-fills provider. Cache is pre-warmed at startup; stale data served on fetch error.

Optional **SQLite EPG file** (`IPTV_TUNERR_EPG_SQLITE_PATH`) stores merged guide rows after each refresh, exposes max programme end times for incremental fetch windows, and `/guide/epg-store.json` for inspection ([ADR 0003](adr/0003-epg-sqlite-vs-postgres.md)). Optional **`IPTV_TUNERR_EPG_SQLITE_RETAIN_PAST_HOURS`** drops older programmes from the SQLite file after each sync; optional **`IPTV_TUNERR_EPG_SQLITE_VACUUM`** runs `VACUUM` after such pruning; optional **`IPTV_TUNERR_EPG_SQLITE_MAX_BYTES`** / **`IPTV_TUNERR_EPG_SQLITE_MAX_MB`** cap on-disk file size after sync. Optional **`IPTV_TUNERR_EPG_SQLITE_INCREMENTAL_UPSERT`** syncs merged guide data with **overlap-window upsert** instead of truncating the full programme table each refresh. **`IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX`** can append query params to provider `xmltv.php` when your panel documents them; with **`IPTV_TUNERR_PROVIDER_EPG_INCREMENTAL`**, suffix strings can include horizon tokens (`{from_unix}`, `{to_unix}`, `{from_ymd}`, `{to_ymd}`). Optional **`IPTV_TUNERR_PROVIDER_EPG_DISK_CACHE`** stores the last provider `xmltv.php` download and uses HTTP conditional GET (`If-None-Match` / `If-Modified-Since`) when the upstream sends **`ETag`** / **`Last-Modified`** — **HTTP 304** skips re-download (many panels omit validators; then each refresh still pulls the full feed).

| Feature | Description |
|---------|-------------|
| **Provider EPG via `xmltv.php`** | Fetches EPG directly from Xtream provider using existing credentials (`IPTV_TUNERR_PROVIDER_EPG_ENABLED`). No third-party EPG source required for Xtream providers. |
| **External XMLTV fetch/remap** | Fetch external XMLTV, filter to current lineup, remap programme channel IDs to local guide numbers. Gap-fills provider EPG for uncovered time windows. |
| **HDHR hardware EPG (optional)** | When `IPTV_TUNERR_HDHR_GUIDE_URL` is set, fetch device `guide.xml` and add non-overlapping programmes per `tvg-id` after provider + external. |
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
| **Guide health report** | `guide-health` and `/guide/health.json` inspect the actual merged guide output and classify channels by real programme coverage, placeholder-only fallback, or no guide rows at all. |
| **EPG doctor workflow** | `epg-doctor` and `/guide/doctor.json` combine deterministic XMLTV match analysis with real merged-guide coverage so operators get one diagnosis instead of stitching together multiple reports. |
| **EPG auto-fixer** | `epg-doctor -write-aliases` and `/guide/aliases.json` export reviewable alias overrides from healthy normalized-name matches so good repairs can be persisted. |
| **EPG match provenance** | When an XMLTV source is supplied to the report command, channels show whether they matched by exact `tvg-id`, alias override, normalized-name repair, or not at all. |
| **Top opportunity summary** | Report summary highlights the highest-frequency fixes across the lineup (for example missing `TVGID`, no backup streams, or alias repair candidates). |
| **Channel leaderboard** | `channel-leaderboard` and `/channels/leaderboard.json` expose hall-of-fame, hall-of-shame, guide-risk, and stream-risk slices so weak channels stand out immediately. |
| **Lineup recipes** | Intelligence-driven lineup shaping with `IPTV_TUNERR_LINEUP_RECIPE=high_confidence|balanced|guide_first|resilient|sports_now|kids_safe|locals_first`. |
| **Guide-quality lineup policy** | Optional runtime policy (`IPTV_TUNERR_GUIDE_POLICY=healthy|strict`) can suppress channels that only produce placeholder rows or no real programme coverage once guide-health cache is available. |
| **Guide-policy decision surface** | `/guide/policy.json?policy=healthy|strict` turns cached guide-health into an explicit keep/drop report with counts for healthy, placeholder-only, no-programme, and missing-`TVGID` decisions, so lineup and catch-up policy behavior is inspectable instead of implicit. |
| **Registration recipes** | `IPTV_TUNERR_REGISTER_RECIPE=healthy|balanced|high_confidence|guide_first|resilient|sports_now|kids_safe|locals_first` lets Plex/Emby/Jellyfin registration reuse channel-intelligence scoring or the same intent-oriented lineup recipes instead of blindly registering catalog order. |
| **Channel DNA foundation** | Live channels now carry a persisted `dna_id` derived from repaired `TVGID` or normalized channel identity inputs, `/channels/dna.json` / `channel-dna-report` group channels by shared identity, and `IPTV_TUNERR_DNA_POLICY=prefer_best|prefer_resilient` can now collapse duplicate variants to a preferred winner in lineup and registration flows. |
| **Channel DNA provider preference** | `IPTV_TUNERR_DNA_PREFERRED_HOSTS` lets duplicate-variant selection bias trusted provider/CDN authorities before score-based tie-breaking, so the winning variant can match operator preference as well as guide/stream quality. |
| **Autopilot decision memory foundation** | Optional JSON-backed memory file remembers winning playback decisions by `dna_id + client_class`, including the preferred upstream URL/host that last succeeded. **URL matching** treats trailing-slash paths, default **:80**/**:443**, and scheme/host case drift as equivalent when reordering **`StreamURLs`**. **Consensus host** (opt-in **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST`**) can prefer a hostname that multiple other channels already agree on when this channel has no memory or stale URLs. `autopilot-report` and `/autopilot/report.json` expose the hottest remembered channels plus consensus fields; `/provider/profile.json` includes **`intelligence.autopilot`**. Hot-start rules can use those hits for high-traffic channels; **`IPTV_TUNERR_HOT_START_GROUP_TITLES`** matches M3U **`group_title`** substrings for category-wide favorites. **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`** lists hostnames to prefer for every channel (after per-DNA memory, before consensus), and **`IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE`** adds JSON-backed preferred/blocked host policy for cross-provider routing guardrails. |
| **Autopilot failure memory** | Remembered Autopilot decisions now track failure counts and failure streaks too, so stale remembered stream/profile choices automatically stop being reused after repeated misses. |
| **Ghost Hunter operator loop** | `ghost-hunter` CLI and `/plex/ghost-report.json` observe Plex Live TV sessions over time, classify visible stalls with reaper heuristics, optionally stop stale visible transcode sessions, recommend the next safe action automatically for visible-stale vs hidden-grab cases, the CLI can invoke the guarded hidden-grab helper directly, and the operator deck now exposes `ghost-visible-stop` plus guarded hidden-grab dry-run/restart actions. |
| **Provider behavior profile foundation** | `/provider/profile.json` exposes learned provider/runtime quirks (tuner cap, concurrency/CF signals, penalized hosts, optional **autotune host quarantine** (`quarantined_hosts`, `auto_host_quarantine`, **`upstream_quarantine_skips_total`**), mux counters, last **HLS/DASH** mux outcomes) plus **`intelligence.autopilot`** and advisory **`remediation_hints`** (heuristic **`code`**/**`severity`**/**`message`**/**`env`** suggestions) for one-fetch operator dashboards ([LTV epic](epics/EPIC-live-tv-intelligence.md)). |
| **Guide highlights foundation** | `/guide/highlights.json` repackages the cached merged guide into immediate user-facing lanes: `current`, `starting_soon`, `sports_now`, and `movies_starting_soon`. |
| **Catch-up capsule publishing** | `/guide/capsules.json` and `catchup-capsules` still expose the preview/feed layer, and `catchup-publish` now turns those capsules into real `.strm + .nfo` lane libraries plus `publish-manifest.json`, with optional Plex/Emby/Jellyfin library create+refresh. `IPTV_TUNERR_CATCHUP_GUIDE_POLICY=healthy|strict` can suppress weak placeholder-only channels from preview/publish output, and duplicate programme rows that share the same `dna_id + start + title` are now curated down to the richer capsule before export/publish. When `IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE` is set, capsules and published `.strm` files become source-backed replay launchers instead of plain live-channel launchers. Emby and Jellyfin were live-validated in-cluster; Jellyfin uses its current `GET /Library/VirtualFolders` + query-param `POST /Library/VirtualFolders` API shape. |
| **Catch-up recording** | `catchup-record` records current in-progress capsules to local TS files plus `record-manifest.json`, and `catchup-daemon` adds the first policy-driven recorder loop for scanning guide capsules, scheduling eligible captures, filtering by lane or channel identity, suppressing duplicate `dna_id` programme variants, persisting active/completed/failed state across runs, annotating interrupted partial recordings on restart and retrying still-eligible programme windows, supporting lane-specific retention and storage budgets with headroom surfaced in state JSON, optional max-age pruning of completed items (global and per-lane), transient capture retries with exponential backoff that also respects `Retry-After` and applies status-aware multipliers for typical overload/rate-limit responses, optional same-spool HTTP `Range` resume against `.partial.ts` when upstreams support partial content, optional multi-upstream failover using catalog stream URLs after the Tunerr relay URL, per-item and aggregate capture metrics (HTTP attempts, transient retries, bytes resumed, upstream switches), spool-then-finalize capture so interrupted runs do not masquerade as finished `.ts` assets, optionally publishing completed recordings into media-server-friendly lane folders, optionally creating/reusing Plex, Emby, or Jellyfin lane libraries as recordings land (including optional deferred full-manifest library refresh), and exposing recorder status through `catchup-recorder-report` and `/recordings/recorder.json`. |
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
- [connect-plex-to-iptv-tunerr](how-to/connect-plex-to-iptv-tunerr.md) — wizard vs **`-register-plex`** vs API; channelmap and limits.
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
| **WebDAV VOD surface** | Read-only WebDAV export of the same synthetic `Movies/` / `TV/` tree for native macOS/Windows mounting, with explicit `405` rejection for mutation methods and binary smoke coverage for `OPTIONS`, directory/file `PROPFIND`, file `HEAD`, and byte-range reads through the real cached materializer path. |
| **On-demand cache** | Optional cache materialization for direct-file (and HLS→MP4 via ffmpeg) VOD paths (`internal/materializer`, `internal/probe` for stream-type sniff). |
| **Plex library registration helper** | `plex-vod-register` creates/reuses Plex TV/Movie libraries for a VODFS mount, with optional VOD-safe library prefs. |
| **One-sided VOD registration** | `plex-vod-register --shows-only` / `--movies-only` for lane-specific library creation without unwanted companion sections. |
| **Platform scope** | Linux keeps `mount` / VODFS; macOS and Windows use `vod-webdav` for parity. |

## 12. Cloudflare resilience

Operator-oriented summary; full guide: [how-to/cloudflare-bypass.md](how-to/cloudflare-bypass.md).

| Feature | Description |
|---------|-------------|
| **UA cycling** | On CF block signals, cycles Lavf (ffmpeg-detected), VLC, mpv, Kodi, Firefox, Chrome until one works. |
| **Browser header profiles** | Matching `Accept`, `Accept-Language`, `Sec-Ch-Ua`, etc., when a browser UA is selected. |
| **Learned UA persistence** | Per-host working UA in `cf-learned.json` (with cookie jar); restored at startup. |
| **HLS segment CF detection** | Detects CF failures on `.ts` segment fetches, not only playlists. |
| **Clearance freshness monitor** | Optional proactive re-bootstrap before `cf_clearance` expiry. |
| **Cookie import** | `import-cookies` from HAR, Netscape, or inline (`cf_clearance`). |
| **`cf-status`** | CLI view of per-host CF state, clearance TTL, working UA. |

## 13. Free public source integration

Supplement or enrich the paid catalog with public M3U feeds fetched at index time. No redistribution — sources are fetched fresh on each catalog build.

| Feature | Description |
|---------|-------------|
| **Custom M3U URLs** | `IPTV_TUNERR_FREE_SOURCES=url1,url2` — any public M3U URL is fetched and merged. |
| **iptv-org preset** | Built-in support for `iptv-org/iptv` per-country/per-category feeds via `IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES=us,gb`, `_CATEGORIES=news,sports`, or `_ALL=true`. |
| **Merge modes** | `supplement` (default) — add channels absent from paid lineup; `merge` — append free URLs as paid-channel fallbacks; `full` — deduplicate combined catalog, paid takes precedence. |
| **M3U content caching** | Downloaded playlists cached on disk (default 6 h TTL via `IPTV_TUNERR_FREE_SOURCE_CACHE_TTL`). Avoids re-downloading large feeds on every index run. |
| **Guide number collision fix** | Free channels are re-numbered starting at `maxPaidGuideNumber + 1` so they never collide with paid lineup numbers in Plex/Emby/Jellyfin. |
| **iptv-org blocklist filter** | Fetches `iptv-org/api` `blocklist.json` at index time; channels flagged nsfw/legal/geo are dropped or tagged. Also cached. |
| **iptv-org NSFW tagging** | `IPTV_TUNERR_FREE_SOURCE_FILTER_NSFW=false` keeps NSFW channels but prefixes `GroupTitle` with `[NSFW] <category>` for supervisor-based routing to an adult lineup. |
| **Closed channel filter** | `IPTV_TUNERR_FREE_SOURCE_FILTER_CLOSED=true` (default) drops channels with a closure date in `channels.json`. |
| **Require tvg-id** | `IPTV_TUNERR_FREE_SOURCE_REQUIRE_TVG_ID=true` limits free channels to those with a EPG-linkable `tvg-id`. |
| **Smoketest integration** | `IPTV_TUNERR_FREE_SOURCE_SMOKETEST=true` runs a probe pass on free channels; reuses `IPTV_TUNERR_SMOKETEST_CACHE_FILE` so results persist across runs. |
| **`free-sources` CLI** | `iptv-tunerr free-sources` — fetch, probe, by-group summary, catalog diff, JSON output. |

## 14. Diagnostics and debug tooling

| Feature | Description |
|---------|-------------|
| **`debug-bundle`** | `iptv-tunerr debug-bundle` collects stream attempts, provider profile, CF learned, cookie metadata, env (redacted). See [how-to/debug-bundle.md](how-to/debug-bundle.md). |
| **`analyze-bundle.py`** | Correlates bundle artifacts with PMS.log, Tunerr stdout, and optional pcap. |
| **HLS mux toolkit docs** | [reference/hls-mux-toolkit](reference/hls-mux-toolkit.md) plus [observability-prometheus-and-otel](explanations/observability-prometheus-and-otel.md) document mux diagnostics, soak helpers, `/metrics`, and OTEL bridge posture. |
| **Autopilot consensus + upstream metrics** | With **`IPTV_TUNERR_METRICS_ENABLE`**, **`GET /metrics`** exposes **`iptv_tunerr_autopilot_consensus_*`** gauges (DNA count, hit sum, runtime flag) and **`iptv_tunerr_upstream_quarantine_skips_total`** when host quarantine drops bad upstreams. See [cli-and-env-reference](reference/cli-and-env-reference.md). |
| **Runtime snapshot URL map** | **`/debug/runtime.json`** (tuner **`URLs`**) includes **`health`** → **`/healthz`** and **`ready`** → **`/readyz`** for automation. |

## 15. Packaging, testing, and ops tooling

| Feature | Description |
|---------|-------------|
| **Cross-platform tester package builder** | `scripts/build-test-packages.sh` builds archives + checksums for Linux/macOS/Windows. |
| **Tester handoff bundle builder** | `scripts/build-tester-release.sh` stages packages + docs + examples + manifest for distribution. |
| **Tester handoff docs** | Packaging and tester checklists are documented under `docs/how-to/`. |
| **CI tester bundles** | GitHub Actions workflow builds tester bundles on demand/tag. |
| **Hidden Plex grab recovery** | `scripts/plex-hidden-grab-recover.sh` + runbook for Plex Live TV hidden active-grab wedges. |
| **Plex stream override analysis helper** | `scripts/plex-generate-stream-overrides.py` for feed/profile override candidate generation. |
| **Live TV provider label rewrite proxy** | `scripts/plex-media-providers-label-proxy.py` + k8s deploy helper to rewrite `/media/providers` labels (client-dependent effect). |
| **Provider `probe` interpretation** | **`iptv-tunerr probe`** exercises **`get.php`** + **`player_api.php`** per host; how-to: [interpreting-probe-results.md](how-to/interpreting-probe-results.md). |
| **Stream-compare harness** | **`scripts/stream-compare-harness.sh`** diffs direct upstream vs Tunerr (`curl`/`ffprobe`/optional `ffplay`, optional pcap, **`manifest.json`**, **`stream-compare-report.py`**). How-to: [stream-compare-harness.md](how-to/stream-compare-harness.md). |
| **Live-race harness + Plex sessions** | With **`PMS_URL`** + **`PMS_TOKEN`** (or Tunerr/Plex env aliases), **`scripts/live-race-harness.sh`** can snapshot Plex **`/status/sessions`** into the bundle; **`live-race-harness-report.py`** summarizes players/products for **HR-002** / **HR-003** correlation. How-to: [live-race-harness.md](how-to/live-race-harness.md). |
| **Multi-stream contention harness** | **`scripts/multi-stream-harness.sh`** runs 2+ staggered live pulls against a real tuner, samples **`/provider/profile.json`** + **`/debug/stream-attempts.json`** + **`/debug/runtime.json`**, optionally snapshots Plex sessions, and emits **`report.txt`** / **`report.json`** for “second stream starts, first dies” regressions. How-to: [multi-stream-harness.md](how-to/multi-stream-harness.md). |

## 16. Platform support summary

| Platform | Core app (`run/serve/index/probe/supervise`) | HDHR HTTP endpoints | HDHR network mode | Linux `mount` / VODFS | `vod-webdav` |
|----------|----------------------------------------------|---------------------|-------------------|------------------------|--------------|
| Linux | Yes | Yes | Yes | Yes | Yes |
| macOS | Yes | Yes | Compiles (runtime validation depends on environment) | No | Yes |
| Windows | Yes | Yes | Compiles (native validation recommended; `wine` smoke not authoritative) | No | Yes |

## 17. Not supported / limits (current)

- **Public-grade multi-user admin plane** (the dedicated deck is a strong operator console, but it is not a hardened internet-facing admin product with roles/SSO/audit persistence)
- **Plex wizard checkbox preselection for >479 channels** (HDHR protocol/wizard limitation; serve only the channels you want selectable)
- **Native macFUSE / WinFsp backends** (not shipped yet; current non-Linux parity path is `vod-webdav`)
