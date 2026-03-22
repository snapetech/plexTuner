---
id: ref-cli-env-reference
type: reference
status: draft
tags: [reference, cli, env, config]
---

# CLI and env reference

Reference for primary commands, key flags, and commonly used environment variables.

This is focused on practical operation/testing. For tester bundles and supervisor-specific lab knobs, also see:
- [testing-and-supervisor-config](testing-and-supervisor-config.md)

## Commands

## `iptv-tunerr run`

One-shot workflow:
- refresh catalog (unless skipped)
- health-check provider (unless skipped)
- start tuner server

Common flags:
- `-catalog`
- `-addr`
- `-base-url`
- `-device-id`
- `-friendly-name`
- `-mode` (`easy` or `full`)
- `-skip-index`
- `-skip-health`
- `-register-plex`
- `-register-only`

Use for:
- systemd/Docker runtime
- most single-binary deployments

## `iptv-tunerr serve`

Serve tuner endpoints from an existing catalog.

Common flags:
- `-catalog`
- `-addr`
- `-base-url`
- `-device-id`
- `-friendly-name`
- `-mode`

Use for:
- split workflows (external indexing)
- local endpoint tests

## `iptv-tunerr index`

Fetch provider M3U/API and write catalog JSON.

Common flags:
- `-m3u`
- `-catalog`

Use for:
- scheduled indexing
- catalog debugging without starting the server

## `iptv-tunerr mount`

Mount VODFS from the catalog.

Common flags:
- `-mount`
- `-catalog`
- `-cache`

Notes:
- Linux-only (`FUSE`)

## `iptv-tunerr vod-webdav`

Serve the VOD catalog over a read-only WebDAV surface so macOS and Windows can
mount the same synthetic `Movies/` / `TV/` tree without the Linux FUSE path.

Common flags:
- `-addr`
- `-catalog`
- `-cache`

Notes:
- Cross-platform (`Linux`, `macOS`, `Windows`)
- The WebDAV server is read-only.
- Directory scans work without `-cache`, but actual file reads need a working materializer/cache path.

## `iptv-tunerr vod-webdav-mount-hint`

Print a platform-specific mount hint and example command for the read-only VOD
WebDAV surface.

Common flags:
- `-addr`
- `-os`
- `-target`

## `iptv-tunerr plex-vod-register`

Create or reuse Plex libraries for a mounted VODFS tree.

Default library names:
- `VOD` -> `<mount>/TV` (Plex TV library)
- `VOD-Movies` -> `<mount>/Movies` (Plex Movie library)

Common flags:
- `-mount`
- `-plex-url`
- `-token`
- `-shows-name`
- `-movies-name`
- `-vod-safe-preset` (default `true`)
- `-refresh`

Env fallbacks:
- `IPTV_TUNERR_PMS_URL` (or `PLEX_HOST` -> `http://<host>:32400`)
- `IPTV_TUNERR_PMS_TOKEN` (or `PLEX_TOKEN`)
- `IPTV_TUNERR_MOUNT`

Notes:
- Requires the VODFS mount path to be visible to the Plex server host/container.
- Creates/reuses sections idempotently by section name + path.
- If the same section name exists with a different path/type, the command returns an error instead of mutating it.
- By default, applies a per-library VOD-safe Plex preset to disable expensive analysis jobs (credits, intro/chapter/preview thumbnails, ad/voice analysis) on these virtual catch-up libraries only.

## `iptv-tunerr vod-split`

Split a VOD catalog into multiple category/region lane catalogs for separate
VODFS mounts/libraries.

Built-in lane names (current):
- `bcastUS`
- `sports`
- `news`
- `kids`
- `music`
- `euroUK`
- `mena`
- `movies`
- `tv`
- `intl`

Common flags:
- `-catalog`
- `-out-dir` (required)

Output:
- `<out-dir>/<lane>.json`
- `<out-dir>/manifest.json` (lane counts + source catalog)

Use for:
- smaller category-scoped Plex VOD libraries
- reduced scan scope / faster targeted rescans
- operational isolation of high-churn catch-up lanes

## `iptv-tunerr epg-link-report`

Generate a deterministic EPG-link coverage report for `live_channels` in a
catalog against an XMLTV source. This is the Phase 1 workflow for improving the
long-tail unlinked channel set without changing runtime playback behavior.

Match tiers (current):
- `tvg-id` exact
- alias override exact
- normalized channel-name exact (unique only)

Common flags:
- `-catalog`
- `-xmltv` (required; file path or `http(s)` URL)
- `-aliases` (optional JSON alias override file)
- `-out` (optional JSON full report; default is stdout)
- `-unmatched-out` (optional JSON unmatched-only list)

Alias override JSON shape:

```json
{
  "name_to_xmltv_id": {
    "Nick Junior Canada": "nickjr.ca",
    "Fox News Channel US": "foxnews.us"
  }
}
```

Use for:
- measuring current XMLTV coverage before changing lineups
- generating a review queue for the unlinked tail
- iterating alias mappings safely (report-only, no runtime mutation)

## `iptv-tunerr channel-report`

Generate a channel intelligence report for the current lineup.

The report scores each channel on:
- guide confidence
- stream resilience
- backup stream depth
- actionable next steps

Optional XMLTV enrichment adds EPG match provenance:
- exact `tvg-id`
- alias override
- normalized exact-name repair
- unmatched

Common flags:
- `-catalog`
- `-xmltv` (optional file path or `http(s)` URL)
- `-aliases` (optional JSON alias override file)
- `-out` (optional JSON output file; otherwise stdout)

Also available live over HTTP:
- `GET /channels/report.json`
- `GET /provider/profile.json` — runtime provider profile including learned tuner caps, HLS instability, Cloudflare hits, penalized upstream hosts, and **`remediation_hints`** (advisory heuristic suggestions with optional related **`IPTV_TUNERR_*`** env names)

Use for:
- spotting channels that are present but operationally weak
- confirming whether EPG success is coming from exact `tvg-id` matches or repairs
- building a prioritized cleanup queue for aliases, backup streams, and stable guide numbers

Notes:
- each reported channel now includes a persisted `dna_id`
- the current Channel DNA foundation prefers real/repaired `TVGID`, then falls back to normalized channel identity inputs

## `iptv-tunerr channel-leaderboard`

Generate the short-form hall-of-fame / hall-of-shame view for the lineup.

This is the fast operator surface when you do not want to read the full channel report first.

Common flags:
- `-catalog`
- `-xmltv` (optional file path or `http(s)` URL)
- `-aliases` (optional JSON alias override file)
- `-limit` (rows per bucket; default `10`)
- `-out` (optional JSON output file; otherwise stdout)

Also available live over HTTP:
- `GET /channels/leaderboard.json`

Buckets:
- `hall_of_fame`
- `hall_of_shame`
- `guide_risks`
- `stream_risks`

## `iptv-tunerr guide-health`

Generate a guide-health report for the actual merged guide output.

This answers a different question than `epg-link-report`:
- not only "did the channel match XMLTV?"
- but also "did real programme rows make it into the served guide?"

The report classifies channels by:
- real programme coverage
- placeholder-only fallback
- no programme rows
- optional XMLTV match provenance

Common flags:
- `-catalog`
- `-guide` (required; file path or `http(s)` URL, usually `/guide.xml`)
- `-xmltv` (optional source XMLTV for deterministic match provenance)
- `-aliases` (optional JSON alias override file)
- `-out` (optional JSON output file; otherwise stdout)

Live endpoint:
- `GET /guide/health.json`

Use for:
- proving that guide data contains real show blocks, not only channel-name placeholders
- identifying channels that are guide-linked but still have no programme coverage
- debugging tester reports like "channel names appear, but no actual what's-on data"

## `iptv-tunerr epg-doctor`

Run the combined EPG diagnostic workflow in one report.

This is the recommended top-level operator tool when you want one answer to:
- did the channel match XMLTV?
- did real programme rows make it into the served guide?
- is the channel only surviving on placeholders?
- what should I fix first?

Common flags:
- `-catalog`
- `-guide` (required; file path or `http(s)` URL, usually `/guide.xml`)
- `-xmltv` (optional source XMLTV for deterministic match provenance)
- `-aliases` (optional JSON alias override file)
- `-out` (optional JSON output file; otherwise stdout)
- `-write-aliases` (optional JSON output file containing suggested `name_to_xmltv_id` overrides from healthy normalized-name matches)

Live endpoint:
- `GET /guide/doctor.json`
- `GET /guide/aliases.json`

Use for:
- one-shot EPG triage instead of manually comparing `epg-link-report` and `guide-health`
- prioritizing whether the real problem is matching, programme coverage, or placeholder fallback
- exporting reviewable alias overrides once a repaired match has proven it carries real programme blocks

## `iptv-tunerr channel-dna-report`

Export grouped live-channel identity clusters from a catalog.

Common flags:
- `-catalog`
- `-out`

Live endpoint:
- `GET /channels/dna.json`

## `iptv-tunerr ghost-hunter`

Observe Plex Live TV sessions over a short window, classify visible stalls with the same idle/lease heuristics as the built-in reaper, and optionally stop stale visible transcode sessions.

Common flags:
- `-pms-url`
- `-token`
- `-observe`
- `-poll`
- `-stop`
- `-recover-hidden dry-run|restart`
- `-machine-id`
- `-player-ip`

## `iptv-tunerr autopilot-report`

Export remembered Autopilot decisions and the hottest channels by hit count.

Common flags:
- `-state-file`
- `-limit`

Also available live over HTTP:
- `GET /autopilot/report.json`

Live endpoint:
- `GET /plex/ghost-report.json`
  - supports `?stop=true` to apply the same stale-visible-session stop mode as the CLI
- `POST /ops/actions/ghost-visible-stop`
  - runs the same stale-visible-session stop pass for the localhost/LAN operator UI
- `POST /ops/actions/ghost-hidden-recover?mode=dry-run|restart`
  - runs the guarded hidden-grab helper for the localhost/LAN operator UI

Query params:
- `observe=4s`
- `poll=1s`

Limit:
- hidden Plex grabs that never appear in `/status/sessions` are not visible to Ghost Hunter; use the recovery runbook for those cases.
- when Ghost Hunter observes zero visible sessions, it now returns:
  - `hidden_grab_suspected=true`
  - `recommended_action`
  - `recovery_command`
  - `runbook`
- `IPTV_TUNERR_GHOST_HUNTER_RECOVERY_HELPER` optionally overrides the helper script path used by the CLI `-recover-hidden` hook and the operator action endpoint (default `./scripts/plex-hidden-grab-recover.sh`).

## Provider behavior profile endpoint

Runtime-only provider intelligence surface:
- `GET /provider/profile.json`

What it exposes:
- configured tuner limit
- learned/effective tuner limit after upstream concurrency-cap signals
- forwarded auth-context / fetch headers (`Cookie`, `Referer`, `Origin`, `Range`, `If-Range`)
- whether provider basic auth is configured
- whether `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` and `IPTV_TUNERR_FETCH_CF_REJECT` are active
- whether provider autotune is enabled and whether HLS reconnect has been auto-armed
- count and last-seen details for provider concurrency-limit signals
- count and last-seen details for Cloudflare-abuse block hits
- count and last-seen details for HLS playlist/segment instability

Related env:
- `IPTV_TUNERR_PROVIDER_AUTOTUNE` — default `true`; enables conservative provider-aware runtime tuning when the operator has not explicitly set the relevant knob
- `IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE` — when `true`/`1`/`on` **and** autotune is on, upstream hosts that exceed **`IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER`** consecutive failure signals are **skipped** in **`walkStreamUpstreams`** while at least one non-quarantined backup URL remains (per-host cooldown **`IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_SEC`**, default **900**). Surfaced on **`/provider/profile.json`** as **`auto_host_quarantine`**, **`upstream_quarantine_skips_total`** (cumulative), **`penalized_hosts[].quarantined_until`**, **`quarantined_hosts`**, and **`remediation_hints`** (`host_quarantine_active`). With **`IPTV_TUNERR_METRICS_ENABLE`**, Prometheus **`iptv_tunerr_upstream_quarantine_skips_total`** matches the same events.
- `IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT` — optional per-provider-account concurrent-stream cap for deduplicated multi-account channels. When set to a positive integer, live stream ordering prefers less-loaded credential sets and rejects new tunes with HDHR-style **805** / HTTP **503** when every distinct provider account for that channel is already at the cap. If unset, Tunerr still uses account-aware spreading for channels that carry multiple distinct credential sets, and now learns tighter per-account caps from upstream concurrency-limit signals when a specific credential set starts returning `423` / `458` / `509` / similar limit responses.
- `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE` — optional JSON state file used to persist learned per-account concurrency caps across restarts. If unset but `IPTV_TUNERR_COOKIE_JAR_FILE` is configured, Tunerr derives `provider-account-limits.json` in that same directory automatically.
- `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS` — TTL for persisted learned per-account concurrency caps. Defaults to `24`; values below `1` are clamped up to `1`.
- `/provider/profile.json` now includes `account_learned_limits[]` so operators can see which credential set has learned a tighter cap, how many contention signals were seen, and whether that account currently has leased streams.
- `/provider/profile.json` and `/debug/runtime.json` now also expose the learned-limit state file path and TTL so the persistence/decay policy is visible at runtime.
- `IPTV_TUNERR_PROGRAMMING_RECIPE_FILE` — optional JSON file storing the server-side Programming Manager recipe. When set, Tunerr applies the saved category/channel selection, manual/custom order, and optional exact-backup collapse after guide/DNA intelligence and before final lineup exposure. Surfaced via `/programming/categories.json`, `/programming/channels.json`, `/programming/order.json`, `/programming/backups.json`, `/programming/recipe.json`, `/programming/preview.json`, and `/debug/runtime.json`.
- `IPTV_TUNERR_RECORDING_RULES_FILE` — optional JSON file storing durable server-side recording rules. Surfaced via `/recordings/rules.json`, `/recordings/rules/preview.json`, `/recordings/history.json`, and `/debug/runtime.json`.

## Programming Manager foundation endpoints

Server-backed lineup-curation primitives for the upcoming Programming Manager UI:
- `GET /programming/categories.json`
- `POST /programming/categories.json`
- `GET /programming/categories.json?category=<id>`
- `GET /programming/channels.json`
- `POST /programming/channels.json`
- `GET /programming/order.json`
- `POST /programming/order.json`
- `GET /programming/backups.json`
- `GET /programming/recipe.json`
- `POST /programming/recipe.json`
- `GET /programming/preview.json`

What they expose:
- stable category inventory built from the raw post-intelligence lineup (`group_title` / `source_tag`)
- optional per-category member listing
- bulk category include/exclude/remove mutations
- exact channel include/exclude/remove mutations
- the durable saved recipe (`selected_categories`, `included_channel_ids`, `excluded_channel_ids`, `order_mode`, `custom_order`, `collapse_exact_backups`)
- manual order mutations (`prepend`, `append`, `before`, `after`, `remove`) through `/programming/order.json`
- exact-match backup grouping reports through `/programming/backups.json`
- a preview of the currently curated lineup after the recipe is applied, including taxonomy bucket counts

Supported `order_mode` values:
- `source` — keep source order after existing lineup intelligence and filters
- `custom` — use `custom_order` first, then preserve the remaining source order
- `recommended` — classify channels into the server taxonomy buckets (`local_broadcast`, `general_entertainment`, `news_info`, `sports`, `lifestyle_home`, `documentary_history`, `children_family`, `reality_specialized`, `premium_networks`, `regional_sports`, `religious`, `international`) and sort by bucket, then by saved `custom_order`, then by guide number/name
- `collapse_exact_backups: true` — collapse strong exact sibling rows (same `tvg_id`, else same `dna_id`) into one visible lineup row with merged `stream_urls`; inspect those candidate groups via `/programming/backups.json`

Notes:
- `POST /programming/recipe.json` is localhost/LAN-operator guarded with the same policy as other tuner-side operator mutation endpoints.
- A configured `IPTV_TUNERR_PROGRAMMING_RECIPE_FILE` is required for durable writes; without it, the recipe endpoint is read-only and reports that no writable file is configured.

## Guide highlights endpoint

User-facing guide packaging surface built from the cached merged `/guide.xml`:
- `GET /guide/highlights.json`

Query params:
- `soon=30m` — future window for `starting_soon` / `movies_starting_soon`
- `limit=12` — max items per lane

Returned lanes:
- `current`
- `starting_soon`
- `sports_now`
- `movies_starting_soon`

## Catch-up capsule preview endpoint

Preview/feed of future publishable near-live capsule candidates built from the cached merged guide:
- `GET /guide/capsules.json`

Query params:
- `horizon=3h` — how far ahead to include candidate programme windows
- `limit=20` — max capsules returned
- `policy=healthy|strict` — optional guide-quality filter; when omitted, falls back to `IPTV_TUNERR_CATCHUP_GUIDE_POLICY`

Returned fields include:
- `capsule_id`
- `dna_id`
- `lane`
- `state`
- `publish_at`
- `expires_at`

Current states:
- `in_progress`
- `starting_soon`

This endpoint is the preview/input layer for the `catchup-publish` command.

## Catch-up recorder report endpoint

Summarized view of the persistent recorder state written by `catchup-daemon`:
- `GET /recordings/recorder.json`

Query params:
- `limit=10` — max items returned from each of `active`, `completed`, and `failed`

Requirements:
- server must know the recorder state path via `IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE`

Returned fields include:
- `statistics`
- `published_count`
- `interrupted_count`
- `lanes`
- `active`
- `completed`
- `failed`

## `iptv-tunerr catchup-capsules`

Export the same capsule preview model to JSON from a catalog plus a guide/XMLTV source.

Common flags:
- `-catalog`
- `-xmltv` — required; local file or `http(s)` URL, including your own `/guide.xml`
- `-horizon`
- `-limit`
- `-out`
- `-layout-dir` — optional lane-split output directory; writes `<lane>.json` files plus `manifest.json`
- `-guide-policy` — optional `off|healthy|strict`; filters capsules using real guide-health before export
- `-replay-url-template` — optional source-backed replay URL template; when set, capsules include rendered replay URLs and `replay_mode=replay`

## `iptv-tunerr catchup-publish`

Publish near-live guide capsules as media-server-ingestible `.strm + .nfo` libraries.

Common flags:
- `-catalog`
- `-xmltv` — required; local file or `http(s)` URL, including your own `/guide.xml`
- `-horizon`
- `-limit`
- `-out-dir` — required; root output directory
- `-stream-base-url` — required unless `IPTV_TUNERR_BASE_URL` is set; used inside generated `.strm` files
- `-replay-url-template` — optional source-backed replay URL template; when set, `.strm` files point at rendered replay URLs instead of `/stream/<channel>`
- `-library-prefix` — default `Catchup`
- `-guide-policy` — optional `off|healthy|strict`; filters capsules using real guide-health before publish
- `-manifest-out`
- `-register-plex`
- `-register-emby`
- `-register-jellyfin`
- `-refresh`

Output shape:
- `<out-dir>/sports/...`
- `<out-dir>/movies/...`
- `<out-dir>/general/...`
- one folder per capsule, containing:
  - `<name>.strm`
  - `<name>.nfo`
- `publish-manifest.json`

Registration behavior:
- Plex: creates/reuses one movie library per lane and applies the same VOD-safe library preset used by `plex-vod-register`
- Emby/Jellyfin: creates/reuses one movie library per lane via `/Library/VirtualFolders`, then triggers a library refresh scan when `-refresh=true`

Operational note:
- without a replay template, published items are near-live launchers and each `.strm` points back to `IPTV_TUNERR_BASE_URL/stream/<channel>`
- with a replay template, published items become source-backed replay launchers for the programme window
- rerun the publisher on a schedule to keep the lane libraries current

## `iptv-tunerr catchup-record`

Record current in-progress capsules to local TS files for sources that do not already provide replay URLs.

Common flags:
- `-catalog`
- `-xmltv`
- `-horizon`
- `-limit`
- `-out-dir`
- `-stream-base-url`
- `-max-duration`
- `-guide-policy`
- `-replay-url-template`

Output:
- one `.ts` file per recorded in-progress capsule (written as `<lane>/<sanitized-capsule-id>.partial.ts` first, then renamed to `.ts` when the transfer completes cleanly)
- `record-manifest.json`

Replay template variables:
- `{capsule_id}`
- `{dna_id}`
- `{channel_id}`
- `{guide_number}`
- `{channel_name}` / `{channel_name_query}`
- `{title}` / `{title_query}`
- `{start_rfc3339}` / `{stop_rfc3339}`
- `{start_unix}` / `{stop_unix}`
- `{duration_mins}`
- `{start_ymd}`
- `{start_hm}`
- `{start_xtream}` / `{stop_xtream}` (`YYYY-MM-DD:HH-MM`)

## `iptv-tunerr catchup-daemon`

Continuously scan guide-derived capsules and record eligible programmes headlessly with a persistent state file.

This is the first recorder-daemon MVP:
- records `in_progress` capsules immediately
- can schedule `starting_soon` capsules within a configurable lead window
- records multiple items concurrently up to a configured limit
- persists `active`, `completed`, and `failed` items in `recorder-state.json`
- supports replay URLs when `-replay-url-template` is configured, otherwise records from `/stream/<channel>`

Common flags:
- `-catalog`
- `-xmltv`
- `-horizon`
- `-limit`
- `-out-dir`
- `-publish-dir`
- `-library-prefix`
- `-stream-base-url`
- `-poll-interval`
- `-lead-time`
- `-max-duration`
- `-max-concurrency`
- `-state-file`
- `-retain-completed`
- `-retain-failed`
- `-retain-completed-per-lane`
- `-retain-failed-per-lane`
- `-budget-bytes-per-lane`
- `-guide-policy`
- `-replay-url-template`
- `-lanes`
- `-exclude-lanes`
- `-channels`
- `-exclude-channels`
- `-register-plex`
- `-register-emby`
- `-register-jellyfin`
- `-refresh`
- `-defer-library-refresh` — with `-register-*` and `-refresh`, defer the library scan until after `recorded-publish-manifest.json` is written for each successful completion
- `-record-max-attempts` — max capture tries per programme when failures look transient (default `1`)
- `-record-retry-backoff` — initial backoff between transient retries (default `5s`)
- `-record-retry-backoff-max` — max backoff between transient retries (default `2m`)
- `-record-resume-partial` — after transient mid-stream failures, retry with HTTP `Range` against the same `.partial.ts` spool when the server supports partial responses (default `true`)
- `-record-upstream-fallback` — build an ordered URL list from Tunerr `/stream/<id>` plus catalog `stream_url` / `stream_urls` so capture can switch upstream after failures (default `true`)
- `-retain-completed-max-age` — drop completed recordings whose `StoppedAt` is older than this duration (`72h`, `7d`, etc.); empty means off
- `-retain-completed-max-age-per-lane` — per-lane max age for completed items (e.g. `sports=72h,general=24h`)
- `-once`
- `-run-for`

Output/state:
- recorded `.ts` files under `<out-dir>/<lane>/` (each capture uses a `.partial.ts` spool path until the transfer finishes, then renames to `.ts`)
- optional published media-server-friendly layout under `-publish-dir` with linked/copied `.ts` plus `.nfo`
- persistent recorder state JSON at `<out-dir>/recorder-state.json` unless overridden with `-state-file`
- `recorded-publish-manifest.json` under `-publish-dir` when publishing is enabled

State file model:
- `active` — currently scheduled or recording items
- `completed` — finished recordings
- `failed` — interrupted or failed recordings
- `statistics.lane_storage` — optional per-lane `used_bytes` plus `budget_bytes` / `headroom_bytes` when byte budgets are configured
- `statistics.sum_capture_http_attempts`, `sum_capture_transient_retries`, `sum_capture_bytes_resumed`, `sum_capture_upstream_switches` — aggregate capture churn, bytes appended via HTTP Range resume, and catalog upstream advances
- per completed/failed item: optional `capture_http_attempts`, `capture_transient_retries`, `capture_bytes_resumed`, `capture_upstream_switches` for that programme’s capture path

Operational notes:
- this MVP dedupes by `capsule_id`, which already collapses duplicate programme variants built from the same `dna_id + start + title`
- the daemon also suppresses duplicate recordings by programme identity (`dna_id` or channel fallback + start + normalized title), so duplicate provider variants do not both record if they leak into the scheduler input
- `-channels` / `-exclude-channels` match exact `channel_id`, `guide_number`, `dna_id`, or `channel_name`
- `-retain-completed-per-lane` and `-retain-failed-per-lane` apply newer-first retention within each lane before the global completed/failed caps are enforced
- `-budget-bytes-per-lane` applies newer-first completed-item pruning within each lane using `BytesRecorded` or on-disk file sizes, with units like `MiB`, `GiB`, or raw bytes
- interrupted active items are preserved as failed `status=interrupted` records on startup, annotated with `recovery_reason=daemon_restart`, partial byte counts when available, and automatically retried if the same programme window is still eligible
- `-once` is useful for cron-style “scan, record what is live/starting now, then exit”
- without `-once`, the command keeps polling until interrupted or until `-run-for` elapses
- completed and failed recorder state is pruned by retention count, and expired completed items are deleted automatically based on capsule expiry
- when `-publish-dir` is combined with media-server registration flags, each completed recording can create/reuse the matching lane library and trigger a targeted refresh for that lane (or use `-defer-library-refresh` to refresh once after the publish manifest updates)
- daemon publish-time registration reuses the same lane naming as `catchup-publish` (`<library-prefix> Sports`, `<library-prefix> Movies`, etc.)

Relevant streaming env knobs for tricky HLS/CDN paths:
- `IPTV_TUNERR_FFMPEG_HLS_HTTP_PERSISTENT` (`true|false`, default `true`) — ask ffmpeg/libavformat to reuse HTTP connections across HLS fetches
- `IPTV_TUNERR_FFMPEG_HLS_MULTIPLE_REQUESTS` (`true|false`, default `true`) — allow multiple HTTP requests on a persistent connection for HLS input
- `IPTV_TUNERR_HLS_PLAYLIST_RETRY_LIMIT` (default `2`) — extra retries for playlist refreshes that fail with a learned/concurrency-style upstream limit (`423`, `429`, `458`, `509`, or matching body text)
- `IPTV_TUNERR_HLS_PLAYLIST_RETRY_BACKOFF_MS` (default `1000`) — base backoff for those retries; attempts use `1x`, `2x`, `4x`
- `IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE` (`true|false`, default `true`) — for non-transcode HLS, prefer the Go relay over ffmpeg remux when Tunerr has seen **provider concurrency pressure** *or* the current stream host has a **non-zero penalty** from **`IPTV_TUNERR_PROVIDER_AUTOTUNE`** failure accounting (e.g. a recent **`ffmpeg_hls_failed`** on that host). Set `false` to disable this entire branch (concurrency + host-penalty); use **`IPTV_TUNERR_HLS_RELAY_PREFER_GO`** to force Go relay regardless.
- `IPTV_TUNERR_HLS_RELAY_PREFER_GO` (`true|false`, default `false`) — force Go-relay preference even when the pressure/penalty signals above are absent

These are legitimate transport-parity knobs, not Cloudflare bypasses.

## `iptv-tunerr catchup-recorder-report`

Summarize the persistent recorder state file without starting the daemon.

Common flags:
- `-state-file` — required unless `IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE` is set
- `-limit`
- `-out`

Returned fields include:
- aggregate recorder `statistics`
- `published_count`
- `interrupted_count`
- per-lane counts
- recent `active`, `completed`, and `failed` items

## `iptv-tunerr import-cookies`

Import upstream cookies (typically `cf_clearance`) from a browser export into Tunerr’s persistent cookie jar.

Three input formats accepted:

**Inline cookie string:**
```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -cookie "cf_clearance=<value>" \
  -domain provider.example.com
```

**Netscape/Cookie-Editor export:**
```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -netscape /tmp/cookies.txt
```

**HAR file (DevTools "Save all as HAR with content"):**
```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -har /tmp/provider-session.har
```
HAR import deduplicates cookies by name+domain+path and derives domain from the `Host` request header when the cookie domain field is empty.

Common flags:
- `-jar` — required; path to cookie jar JSON file
- `-cookie` — inline `name=value` cookie string (use with `-domain`)
- `-domain` — domain for `-cookie` input
- `-netscape` — path to Netscape-format cookie file
- `-har` — path to HAR file from browser DevTools
- `-ttl` — cookie lifetime in seconds when no expiry is set in the source (default: 24 hours)

See also: [cloudflare-bypass.md](../how-to/cloudflare-bypass.md)

## `iptv-tunerr cf-status`

Offline view of per-host Cloudflare state. No running server required — reads directly from the cookie jar and `cf-learned.json`.

Shows per host: CF-tagged flag, `cf_clearance` presence and time-to-expiry, working UA learned by cycling.

Common flags:
- `-jar` — cookie jar JSON path (default: `IPTV_TUNERR_COOKIE_JAR_FILE`)
- `-learned` — CF learned JSON path (default: `IPTV_TUNERR_CF_LEARNED_FILE` or `cf-learned.json` beside the jar)
- `-json` — machine-readable output

Example:
```bash
iptv-tunerr cf-status
iptv-tunerr cf-status -json | jq
```

See also: [cloudflare-bypass.md](../how-to/cloudflare-bypass.md)

## `iptv-tunerr debug-bundle`

Collect Tunerr-side diagnostic state into a bundle directory or `.tar.gz` for sharing with maintainers or feeding into `scripts/analyze-bundle.py`.

What is collected:
- `stream-attempts.json` — last 500 stream attempts from `/debug/stream-attempts.json`
- `provider-profile.json` — autopilot state from `/provider/profile.json`
- `cf-learned.json` — per-host CF state (working UA, CF-tagged flag)
- `cookie-meta.json` — cookie names/domains/expiry, **no cookie values** (safe to share)
- `env.json` — all `IPTV_TUNERR_*` vars, secrets redacted by default
- `bundle-info.json` — timestamp, version, collection summary

Common flags:
- `-url` — base URL of running server (default `http://localhost:5004`)
- `-out` — output directory (default `debug-scratch/`)
- `--tar` — also write `tunerr-debug-TIMESTAMP.tar.gz`
- `--redact` — redact secrets from env dump (default `true`)
- `--no-server` — skip live server fetch, collect only local state files

Example:
```bash
iptv-tunerr debug-bundle --out ./debug-scratch --tar
```

See also: [debug-bundle.md](../how-to/debug-bundle.md)

## `iptv-tunerr free-sources`

Fetch and inspect free public IPTV channels from configured sources without affecting the running catalog. Useful for exploring feeds before enabling them in production.

| Flag | Default | Meaning |
|------|---------|---------|
| `-by-group` | false | Print channel count summary grouped by `group-title` |
| `-catalog` | — | Path to a catalog JSON file; prints what would be added (channels not already present) |
| `-probe` | false | Run a live HTTP probe pass on fetched channels |
| `-probe-concurrency` | 10 | Parallel probe workers |
| `-probe-timeout` | 8s | Per-channel probe timeout |
| `-probe-max` | 0 | Cap number of channels probed (0 = all) |
| `-require-tvgid` | false | Only include channels that have a `tvg-id` |
| `-limit` | 0 | Print only first N results (0 = all) |
| `-json` | false | JSON output for scripting |

Examples:

```sh
# What groups are in the iptv-org US feed?
IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES=us \
  iptv-tunerr free-sources -by-group

# What would be added to an existing catalog?
IPTV_TUNERR_FREE_SOURCES=https://m3u.prigoana.com/all.m3u \
  iptv-tunerr free-sources -catalog ./catalog.json

# Probe a sample of 50 channels to check live pass rate
IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL=true \
  iptv-tunerr free-sources -probe -probe-max 50
```

## `iptv-tunerr hdhr-scan`

Discover **physical** SiliconDust HDHomeRun tuners on the local network, or query a device by HTTP only.

- **UDP (default):** broadcast discovery on port `65001`, collect `discover` replies (device id, base URL, tuner count).
- **HTTP:** `-addr http://<device-ip>` skips UDP and loads `discover.json` (and optionally `lineup.json`).

Flags:

| Flag | Meaning |
|------|---------|
| `-timeout` | UDP listen window (default `3s`; ignored with `-addr`) |
| `-addr` | Base URL of the device (e.g. `http://192.168.1.100`) — HTTP-only mode |
| `-lineup` | Also GET `lineup.json` and print channel count / metadata |
| `-json` | JSON output for scripting |
| `-guide-xml` | GET `guide.xml` (XMLTV) from each device base; prints byte size and counts `<channel>` / `<programme>` elements (does **not** merge into Tunerr) |

Merge semantics for HDHR + IPTV catalogs: [adr/0002-hdhr-hardware-iptv-merge.md](../adr/0002-hdhr-hardware-iptv-merge.md).

### Operator web UI (`serve` / `run`)

| Env | Meaning |
|-----|---------|
| `IPTV_TUNERR_WEBUI_DISABLED` | If `1`, disable the dedicated dashboard on port `48879` (`0xBEEF`). |
| `IPTV_TUNERR_WEBUI_PORT` | Dedicated dashboard port (default `48879`). |
| `IPTV_TUNERR_WEBUI_ALLOW_LAN` | If `1`, allow non-loopback clients to open the dedicated dashboard (default: **localhost only**). |
| `IPTV_TUNERR_WEBUI_STATE_FILE` | Optional JSON state file for server-derived deck activity plus non-secret deck preferences across process restarts. |
| `IPTV_TUNERR_WEBUI_USER` | Dedicated deck HTTP Basic auth username (defaults to `admin` only when unset at startup). |
| `IPTV_TUNERR_WEBUI_PASS` | Dedicated deck HTTP Basic auth password. When unset, Tunerr generates a one-time startup password instead of using `admin/admin`. That generated password is logged once at startup and shown on the localhost login page until you pin a real password. |
| `IPTV_TUNERR_EVENT_WEBHOOKS_FILE` | Optional JSON file that configures outbound event webhooks. Each hook can declare `name`, `url`, optional `events`, optional extra `headers`, and optional per-hook `timeout` duration. |
| `IPTV_TUNERR_RECORDING_RULES_FILE` | Optional JSON file for durable server-side recording rules used by `/recordings/rules.json`, `/recordings/rules/preview.json`, and `/recordings/history.json`. |
| `IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE` | Optional JSON file storing the latest persisted Plex lineup-harvest report, surfaced via `/programming/harvest.json` and `/programming/preview.json`. |
| `IPTV_TUNERR_VIRTUAL_CHANNELS_FILE` | Optional JSON file for file-backed virtual-channel rules used by `/virtual-channels/rules.json`, `/virtual-channels/preview.json`, `/virtual-channels/live.m3u`, and `/virtual-channels/stream/<id>.mp4`. |
| `IPTV_TUNERR_XTREAM_USER` | Optional username for the read-only downstream Xtream-compatible live output. Requires `IPTV_TUNERR_XTREAM_PASS`. |
| `IPTV_TUNERR_XTREAM_PASS` | Optional password for the read-only downstream Xtream-compatible live output. Requires `IPTV_TUNERR_XTREAM_USER`. |
| `IPTV_TUNERR_UI_DISABLED` | If `1`, `/ui/` is not served. |
| `IPTV_TUNERR_UI_ALLOW_LAN` | If `1`, allow non-loopback clients to open `/ui/` (default: **localhost only**). |

Browser URLs:
- Dedicated deck: `http://127.0.0.1:48879/` by default. It reverse-proxies tuner endpoints under `/api/*`, surfaces runtime settings from `/api/debug/runtime.json`, opens on a login page with a cookie-backed session, accepts direct HTTP Basic auth for scriptable/API access without minting browser sessions, generates a one-time startup password when `IPTV_TUNERR_WEBUI_PASS` is unset, and exposes read-only deck telemetry under `/deck/telemetry.json` plus server-derived operator activity under `/deck/activity.json`.
- Event hooks: `/debug/event-hooks.json` reports configured hooks and recent delivery attempts. Lifecycle events currently include `lineup.updated`, `stream.requested`, `stream.rejected`, and `stream.finished`.
- Xtream output (expanded starter): when `IPTV_TUNERR_XTREAM_USER` and `IPTV_TUNERR_XTREAM_PASS` are set, Tunerr exposes a read-only downstream Xtream-compatible surface at `/player_api.php` (`get_live_streams`, `get_live_categories`, `get_vod_categories`, `get_vod_streams`, `get_series_categories`, `get_series`, `get_series_info`) plus `/live/<user>/<pass>/<channel>.ts`, `/movie/<user>/<pass>/<id>.mp4`, and `/series/<user>/<pass>/<episode>.mp4`.
- Xtream entitlements (starter): when `IPTV_TUNERR_XTREAM_USERS_FILE` is set, Tunerr loads file-backed downstream users with per-user live/VOD/series access scopes, filters `player_api.php` results for those users, gates `/live|movie|series/...` playback by the same rules, and exposes the current ruleset at `/entitlements.json`.
- Recording rules (starter): when `IPTV_TUNERR_RECORDING_RULES_FILE` is set, Tunerr exposes durable recorder-rule CRUD at `/recordings/rules.json`, live capsule matching at `/recordings/rules/preview.json`, and recorder-state classification at `/recordings/history.json`.
- Programming harvest bridge: when `IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE` is set, Tunerr reloads the saved harvest report and exposes it at `/programming/harvest.json`; `/programming/preview.json` also includes `harvest_ready` plus deduped `harvest_lineups` so the Programming lane can surface harvested candidate lineups alongside recipe state.
- Virtual channels (starter): when `IPTV_TUNERR_VIRTUAL_CHANNELS_FILE` is set, Tunerr exposes `/virtual-channels/rules.json` for durable file-backed rules, `/virtual-channels/preview.json` for schedule previews over catalog movies/episodes, `/virtual-channels/live.m3u` for a publishable synthetic-channel export, and `/virtual-channels/stream/<id>.mp4` for the current scheduled asset proxy.
- Active stream intervention: `/debug/active-streams.json` shows live request IDs and `/ops/actions/stream-stop` accepts `{"request_id":"..."}` or `{"channel_id":"..."}` to cancel matching active stream contexts from the localhost operator plane.
- Shared relay visibility: `/debug/shared-relays.json` shows current same-channel shared HLS Go-relay sessions and subscriber counts when duplicate consumers are attached to one upstream relay.
- Programming Manager detail view: `/programming/channel-detail.json?channel_id=<id>&horizon=3h&limit=6` returns focused channel metadata, exact-match backup alternatives, and upcoming programme capsules for category-first channel-builder tools.
- Guide/operator endpoints include `/guide/lineup-match.json`, which reports whether current `lineup.json` rows have exact-name counterparts in emitted `guide.xml`, plus duplicate-name/number signals and a sample of unmatched rows including `channel_id`, `guide_number`, `guide_name`, and observed `tvg_id`.
- Startup contract: until the first real merged guide is cached, `/guide.xml` returns `503 Service Unavailable` with `Retry-After: 5`, `X-IptvTunerr-Guide-State: loading`, and a visible placeholder XMLTV body. HDHR discovery/lineup endpoints stay `200`, but emit `X-IptvTunerr-Startup-State: loading` while no lineup channels are loaded yet; `/lineup_status.json` reports `ScanInProgress=1` and `LineupReady=false` during that startup window, and an empty `/lineup.json` adds `Retry-After: 5`.
- Legacy pages on the tuner port: `http://127.0.0.1:<port>/ui/` (home), `/ui/guide/` (merged guide preview from cache), `/ui/guide-preview.json` (JSON; optional `?limit=`).

Transcode profile names, HDHomeRun-style aliases, and `?profile=` on `/stream/<id>`: [transcode-profiles.md](transcode-profiles.md).

## `iptv-tunerr probe`

Probe provider URLs and print ranked results (best host first).

Common flags:
- `-urls`

Use for:
- provider host failover validation
- diagnosing Cloudflare/proxy failures

## `iptv-tunerr plex-lineup-harvest`

Probe Plex's HDHR guide/channelmap flow across several tuner lineup variants and
emit a structured report of the discovered lineup titles.

This is the productized successor to the older oracle-only flow when you want
to sweep lineup caps or tuner shapes and see which market/provider lineup Plex
matches back.

Common flags:
- `-plex-url`
- `-token`
- `-base-urls`
- `-base-url-template`
- `-caps`
- `-friendly-name-prefix`
- `-wait`
- `-poll`
- `-reload-guide`
- `-activate`
- `-out`

Notes:
- Creates/registers real Plex DVR/device rows during the probe flow.
- Polls channel-map results for a bounded time instead of fetching only once.
- Emits both per-target results and a deduped `lineups[]` summary in JSON.
- See [plex-lineup-harvest](../how-to/plex-lineup-harvest.md).

## `iptv-tunerr plex-epg-oracle`

Probe Plex's wizard-equivalent HDHR registration/guide/channelmap flow across one
or more tuner base URLs and report what Plex maps.

This is an in-app tool for using Plex as a provider/EPG matching oracle during
EPG-linking experiments (for example different lineup sizes/orderings for a region).

Common flags:
- `-plex-url`
- `-token`
- `-base-urls` (comma-separated tuner URLs to test)
- `-base-url-template` + `-caps` (expand `{cap}` into multiple URLs)
- `-reload-guide` (default `true`)
- `-activate` (default `false`; report/probe only unless enabled)
- `-out` (JSON report)

Notes:
- Creates/registers Plex DVR/device rows as part of the probe flow.
- Best used in a lab/test Plex instance.
- Intended to harvest mapping outcomes, not as a runtime dependency.

## `iptv-tunerr plex-epg-oracle-cleanup`

Clean up DVR/device rows created during oracle experiments.

Default behavior is **dry-run** (prints matching DVR/device rows without deleting).

Common flags:
- `-plex-url`
- `-token`
- `-lineup-prefix` (default `oracle-`)
- `-device-uri-substr` (optional extra filter)
- `-do` (actually delete)

Typical flow:
1. Dry-run inspect:
   - `iptv-tunerr plex-epg-oracle-cleanup -plex-url ... -token ...`
2. Apply cleanup:
   - `iptv-tunerr plex-epg-oracle-cleanup -plex-url ... -token ... -do`

## `iptv-tunerr supervise`

Run multiple child `iptv-tunerr` instances from one JSON config.

Common flags:
- `-config`

Use for:
- single-app / multi-DVR category deployments
- combined injected DVR + HDHR wizard lanes

## Core env vars

## Provider / input

Multi-host failover (one subscription, multiple CDN endpoints):
- `IPTV_TUNERR_PROVIDER_URLS` — comma-separated list of provider base URLs; all are probed at startup, fastest/healthiest wins for indexing, rest become per-channel stream URL fallbacks

Single provider:
- `IPTV_TUNERR_PROVIDER_URL`
- `IPTV_TUNERR_PROVIDER_USER`
- `IPTV_TUNERR_PROVIDER_PASS`

Multiple subscriptions (numbered suffix, merge into one catalog):
- `IPTV_TUNERR_PROVIDER_URL_2`, `IPTV_TUNERR_PROVIDER_USER_2`, `IPTV_TUNERR_PROVIDER_PASS_2`
- `IPTV_TUNERR_PROVIDER_URL_3`, `IPTV_TUNERR_PROVIDER_USER_3`, `IPTV_TUNERR_PROVIDER_PASS_3`
- (pattern continues for `_4`, `_5`, ...)
- Channels with duplicate `tvg-id` values across providers are deduplicated — one lineup entry, all stream URLs merged as fallbacks

Other:
- `IPTV_TUNERR_SUBSCRIPTION_FILE`
- `IPTV_TUNERR_M3U_URL`

## Post-index stream validation (smoketest)

Optional: probe each channel's primary stream URL at index time and drop channels that fail. Eliminates dead channels before they ever appear in the lineup.

- `IPTV_TUNERR_SMOKETEST_ENABLED` (`false`) — enable the probe pass
- `IPTV_TUNERR_SMOKETEST_TIMEOUT` (`8s`) — per-channel probe timeout
- `IPTV_TUNERR_SMOKETEST_CONCURRENCY` (`10`) — parallel probe workers
- `IPTV_TUNERR_SMOKETEST_MAX_CHANNELS` (`0` = unlimited) — random sample cap; 0 probes all channels
- `IPTV_TUNERR_SMOKETEST_MAX_DURATION` (`5m`) — wall-clock cap for the full probe pass
- `IPTV_TUNERR_SMOKETEST_CACHE_FILE` — path to persistent per-URL result cache; skips re-probing fresh entries on subsequent runs
- `IPTV_TUNERR_SMOKETEST_CACHE_TTL` (`4h`) — how long a cached result is considered fresh

Probe method:
- MPEG-TS: HTTP Range request for first 4 KB (avoids pulling full streams); 200 or 206 = pass
- HLS (`.m3u8`): GET playlist; validates `#EXTM3U` / `#EXTINF` or a non-comment segment URI

## Free public sources

Supplement or enrich the paid catalog with public M3U feeds at index time. No redistribution — sources are fetched fresh per catalog build.

### Source selection

| Env | Default | Meaning |
|-----|---------|---------|
| `IPTV_TUNERR_FREE_SOURCES` | — | Comma-separated public M3U URLs |
| `IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES` | — | Country codes (`us,gb,ca`) — uses iptv-org per-country feeds |
| `IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES` | — | Category slugs (`news,sports`) — uses iptv-org per-category feeds |
| `IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL` | `false` | `true` — use iptv-org combined all-channels feed |

### Merge mode

| Env | Default | Meaning |
|-----|---------|---------|
| `IPTV_TUNERR_FREE_SOURCE_MODE` | `supplement` | `supplement` — add channels absent from paid lineup; `merge` — append free URLs as paid-channel fallbacks; `full` — deduplicate combined catalog, paid takes precedence |

### Content cache

| Env | Default | Meaning |
|-----|---------|---------|
| `IPTV_TUNERR_FREE_SOURCE_CACHE_TTL` | `6h` | How long downloaded M3U and iptv-org API files are cached on disk |
| `IPTV_TUNERR_FREE_SOURCE_CACHE_DIR` | `<CacheDir>/free-sources` | Override disk cache directory |

### Safety filters

| Env | Default | Meaning |
|-----|---------|---------|
| `IPTV_TUNERR_FREE_SOURCE_FILTER_NSFW` | `true` | Drop NSFW channels (from iptv-org blocklist/channels.json); `false` = keep but tag `GroupTitle` with `[NSFW] <category>` |
| `IPTV_TUNERR_FREE_SOURCE_FILTER_CLOSED` | `true` | Drop channels with a closure date in iptv-org `channels.json` |
| `IPTV_TUNERR_FREE_SOURCE_REQUIRE_TVG_ID` | `false` | Drop channels without a `tvg-id` (EPG-linkable channels only) |
| `IPTV_TUNERR_FREE_SOURCE_SMOKETEST` | `false` | Probe free channels at index time; reuses `IPTV_TUNERR_SMOKETEST_CACHE_FILE` |

Quick-start examples:

```sh
# Add US/GB news & sports to paid lineup (supplement mode)
IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES=us,gb
IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES=news,sports

# Use a custom public feed, keep NSFW but route via supervisor
IPTV_TUNERR_FREE_SOURCES=https://m3u.prigoana.com/all.m3u
IPTV_TUNERR_FREE_SOURCE_FILTER_NSFW=false

# Append free URLs as fallbacks behind paid channels
IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL=true
IPTV_TUNERR_FREE_SOURCE_MODE=merge
```

## Paths

- `IPTV_TUNERR_CATALOG`
- `IPTV_TUNERR_MOUNT`
- `IPTV_TUNERR_CACHE`

## Tuner identity / lineup

- `IPTV_TUNERR_BASE_URL`
- `IPTV_TUNERR_DEVICE_ID`
- `IPTV_TUNERR_FRIENDLY_NAME`
- `IPTV_TUNERR_TUNER_COUNT`
- `IPTV_TUNERR_LINEUP_MAX_CHANNELS`
- `IPTV_TUNERR_GUIDE_NUMBER_OFFSET`
- `IPTV_TUNERR_LINEUP_RECIPE` — intelligence-driven lineup shaping:
  - `high_confidence` = keep only channels with strong guide-confidence signals
  - `balanced` = rank by combined guide + stream score
  - `guide_first` = rank by guide confidence before stream resilience
  - `resilient` = rank by backup-stream resilience before guide score
  - `sports_now` = keep sports-heavy channels only
  - `kids_safe` = keep kid/family channels while excluding obvious unsafe/adult/news matches
  - `locals_first` = bubble likely local/regional channels to the top using the same North-American lineup-shape heuristics
- `IPTV_TUNERR_DNA_POLICY` — optional duplicate-variant policy keyed by `dna_id`:
  - `off` = keep all variants
  - `prefer_best` = keep the strongest duplicate by combined channel-intelligence score
  - `prefer_resilient` = keep the most backup-stream-resilient duplicate first
- `IPTV_TUNERR_DNA_PREFERRED_HOSTS` — optional comma-separated preferred provider/CDN authorities (for example `preferred.example,backup.example:8080`) used as a tie-breaker when duplicate variants share the same `dna_id`
- `IPTV_TUNERR_GUIDE_POLICY` — optional runtime guide-quality policy:
  - `off` = current permissive behavior
  - `healthy` = keep only channels with real programme rows once cached guide-health is available
  - `strict` = same as `healthy`, plus require a non-empty `TVGID`
- `IPTV_TUNERR_REGISTER_RECIPE` — optional media-server registration recipe:
  - `off` = register channels in current catalog order
  - `balanced` = rank by combined guide + stream score
  - `high_confidence` = bias registration order toward stronger guide-confidence channels
  - `guide_first` = prefer guide confidence, then stream resilience
  - `resilient` = prefer backup-stream resilience, then guide confidence
  - `healthy` = like `high_confidence`, but also drops poor-tier channels before Plex/Emby/Jellyfin registration
  - `sports_now` = register sports-heavy channels only
  - `kids_safe` = register kid/family-safe channels only
  - `locals_first` = keep the full set, but bubble likely locals/regionals to the top

`IPTV_TUNERR_GUIDE_NUMBER_OFFSET`:
- adds a per-instance channel/guide ID offset
- useful for many DVRs in Plex to avoid guide cache collisions

## Stream behavior

- `IPTV_TUNERR_STREAM_TRANSCODE` — `off` remux only; `on` always transcode (libx264/AAC); `auto` run ffprobe on tune and transcode only if codecs are not Plex-friendly (e.g. HEVC, VP9); `auto_cached` (alias `cached_auto`) **remux-first** using **only** `IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE` (no ffprobe; channels not listed in the file stay on remux).
- `IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE` — JSON object mapping **channel_id**, **guide_number**, or **tvg_id** → `true`/`false` (or string `transcode`/`remux`). With `off`/`on`/`auto`, the file **overrides** the global decision for matching channels (remux-first escapes for `on`, or per-channel transcode under `off`). With `auto_cached`, the file **is** the policy. Precedence after this: **client adaptation** (`IPTV_TUNERR_CLIENT_ADAPT` / Autopilot) may still force websafe transcode for browser-class clients. See [plex-livetv-http-tuning](plex-livetv-http-tuning.md) (**HR-007**).
- `IPTV_TUNERR_PROFILE_OVERRIDES_FILE` — JSON per-channel ffmpeg profile names (same key order as transcode overrides). Used when transcoding is active. Values may be **built-in** / HDHR alias names or names defined in **`IPTV_TUNERR_STREAM_PROFILES_FILE`**.
- `IPTV_TUNERR_STREAM_PROFILES_FILE` — optional JSON object defining **custom profile names** for **`?profile=`** and per-channel overrides. Each value is an object: **`base_profile`** (built-in preset: `default`, `dashfast`, …), optional **`transcode`** (`true`/`false`, default **`true`** for custom names), optional **`output_mux`** (`mpegts`, packaged `hls`, or `fmp4` / `mp4` / `dash`), optional **`description`**. `output_mux: "hls"` starts a background ffmpeg HLS packager and serves the generated playlist/segments back through Tunerr. See [transcode-profiles](transcode-profiles.md).
- `IPTV_TUNERR_STREAM_BUFFER_BYTES` (`0|auto|<bytes>`) — `auto` enables adaptive buffering when transcoding; `0` disables; a fixed integer (e.g. `2097152`) sets a 2 MiB buffer.
- `IPTV_TUNERR_STREAM_PUBLIC_BASE_URL` — optional **no-trailing-slash** base URL (e.g. `http://192.168.1.10:5004`) prepended to **`?mux=hls`** playlist media lines so clients that mishandle relative URLs still resolve Tunerr. Empty = relative `/stream/...` lines only.
- `IPTV_TUNERR_HLS_MUX_CORS` — when `true`/`1`/`on`, add CORS headers on **`?mux=hls`** playlist and **`?mux=hls&seg=`** responses and handle **`OPTIONS`** preflight for those URLs (for browser-based players or devtools). Default off.
- `IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT` — optional absolute cap for concurrent **`?mux=hls|dash&seg=`** proxy requests. Default derives from effective tuner limit; when set, disables adaptive bonus below.
- `IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER` — multiplier for the default **`seg=`** concurrency cap (`effective_tuner_limit * slots_per_tuner`, default `8`).
- `IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO` — when enabled, add temporary bonus slots from recent **503** seg-limit rejections (requires **`MAX_CONCURRENT`** unset). Tunables: **`IPTV_TUNERR_HLS_MUX_SEG_AUTO_WINDOW_SEC`** (default **60**, **5**–**600**), **`IPTV_TUNERR_HLS_MUX_SEG_AUTO_BONUS_PER_HIT`** (default **4**), **`IPTV_TUNERR_HLS_MUX_SEG_AUTO_BONUS_CAP`** (default **64**).
- `IPTV_TUNERR_HLS_MUX_ACCESS_LOG` — append one JSON line per successful **`seg=`** (redacted upstream URL, duration).
- `IPTV_TUNERR_HLS_MUX_MAX_SEG_PARAM_BYTES` — max length of the **URL-decoded** **`seg=`** value (default **262144**; hard-capped). Over-limit **`400`** with **`X-IptvTunerr-Hls-Mux-Error: seg_param_too_large`**.
- `IPTV_TUNERR_HLS_MUX_DENY_LITERAL_PRIVATE_UPSTREAM` — when `true`/`1`/`on`, reject **`seg=`** URLs whose **host is a literal IP** that is loopback, RFC1918-private, link-local, or unspecified (**hostnames are not resolved**). Response **`403`** with **`blocked_private_upstream`** diagnostic.
- `IPTV_TUNERR_HLS_MUX_UPSTREAM_ERR_BODY_MAX` — max bytes of upstream error body to buffer when returning **`upstream_http_<status>`** for **`seg=`** (default **8192**, hard-capped at **1 MiB**).
- `IPTV_TUNERR_HLS_MUX_DENY_RESOLVED_PRIVATE_UPSTREAM` — when enabled, resolve **`seg=`** host (**A/AAAA**) and block if any IP is loopback/private/link-local/unspecified (**DNS failure** → allow, with a warning log).
- `IPTV_TUNERR_HLS_MUX_SEG_RPS_PER_IP` — optional per–source-IP rate limit (token bucket) for **`seg=`** on **`mux=hls`** and **`mux=dash`**; exceeded → **429** with diagnostic **`seg_rate_limited`**.
- `IPTV_TUNERR_HLS_MUX_WEB_DEMO` — when true, serve **`/debug/hls-mux-demo.html`** (static **hls.js** sample; still enable **`IPTV_TUNERR_HLS_MUX_CORS`** for cross-origin playlists).
- `IPTV_TUNERR_METRICS_ENABLE` — when true/`1`/on, register Prometheus mux counters + **`iptv_tunerr_mux_seg_request_duration_seconds`** histogram, **Autopilot consensus** gauges (**`iptv_tunerr_autopilot_consensus_*`**), **upstream host quarantine** counter (**`iptv_tunerr_upstream_quarantine_skips_total`**), and expose **`GET /metrics`**.
- `IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS` — when enabled, add **`channel_id`** to mux metric labels (**very high Prometheus cardinality**; default off).
- `IPTV_TUNERR_HTTP_ACCEPT_BROTLI` — when enabled, send **`br`** in **`Accept-Encoding`** and transparently decompress **`Content-Encoding: br`** on the shared HTTP client (including **`?mux=*&seg=`** fetches).
- `IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE` — when enabled, expand uniform self-closing DASH **`SegmentTemplate`** (**`duration`** / **`timescale`**, **`$Number$`** in **`media`**, no **`$Time$`**) into **`SegmentList`** before MPD URL rewrite (default off; large manifests possible).
- `IPTV_TUNERR_HLS_MUX_DASH_EXPAND_MAX_SEGMENTS` — max **`SegmentURL`** entries per expanded template (default **10000**, hard-capped at **500000**).
- `IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST` — optional override for the shared **`http.Transport`** **`MaxIdleConnsPerHost`** (default **16**), shared by **`internal/httpclient`** (process start only).
- `IPTV_TUNERR_HTTP_MAX_IDLE_CONNS` — optional override for **`MaxIdleConns`** on that transport (default **100**).
- `IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC` — optional idle connection lifetime in seconds (default **90**). See [plex-livetv-http-tuning](plex-livetv-http-tuning.md).

  These three knobs affect **every** call path that uses **`httpclient.Default()`**, **`httpclient.WithTimeout`**, or **`ForStreaming()`**’s transport clone — including **indexer** (M3U / Xtream), **stream gateway** upstream fetches, **materializer**, **vodfs**, **Plex** / **Emby** registration HTTP, **HDHR** `discover.json` / `lineup.json` / `guide.xml`, **provider** M3U probes, **`tuner` EPG pipeline** HTTP, **health** checks, and **`internal/probe`**. They do **not** reconfigure ad hoc clients that bypass **`internal/httpclient`** (rare); mux **`?mux=*&seg=`** uses a dedicated client with redirect validation (**`mux_http_client.go`**).
- `IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS` — when enabled, add extra concurrent **`seg=`** capacity for channels whose **`dna_id`** has “hot” Autopilot rows (best **`Hits`** across client classes); tunables **`IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_MIN_HITS`**, **`_BONUS_PER_STEP`**, **`_BONUS_CAP`**. Ignored when **`IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT`** is set.
- **HLS / DASH mux** — **`GET /stream/<channel>?mux=hls`** on an **HLS** upstream returns a rewritten **M3U8**; **`?mux=dash`** on a **DASH** upstream returns a rewritten **MPD** (experimental). HLS **`URI='...'`** (single-quoted) is rewritten like **`URI="..."`**. DASH **`media=`** / **`initialization=`** / similar accept **single- or double-quoted** values. **`?mux=dash&seg=`** preserves **`$Number%0Nd$`** / **`$Time%0Nd$`** through query encoding. Optional **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE`** expands **`SegmentTemplate`** (including **`SegmentTimeline`**, paired tags, padded **`$Number`**). Nested playlists/segments use **`?mux=<kind>&seg=<url>`**. Default stream behavior remains TS remux/transcode when `mux` is omitted. Direct **`seg=`** targets must be **http** or **https**; other schemes return **`400`** (JSON-capable clients may send **`Accept: application/json`**). **`X-IptvTunerr-Hls-Mux-Error`** documents failures; **4xx/5xx** from upstream are passed through with a bounded body preview. **`HEAD`** on **`seg=`** is forwarded as **HEAD** to the upstream. **`POST /ops/actions/mux-seg-decode`** (**localhost / UI LAN policy**) decodes base64 **`seg_b64`** for support tickets (**redacted** URL in JSON only).
- **Packaged HLS via named profiles** — when a named stream profile prefers **`output_mux: "hls"`**, Tunerr starts a short-lived ffmpeg HLS packager, returns a packaged playlist, and serves follow-up playlist/segment files through Tunerr under internal **`mux=hlspkg`** session URLs. This is separate from explicit **`?mux=hls`**, which still means native rewrite/proxy.
- **Byte-range / conditional segments:** client **`Range`** / **`If-Range`** / **`If-None-Match`** / **`If-Modified-Since`** are forwarded to upstream **`?mux=hls&seg=`** fetches; **`206`** + **`Content-Range`**, or **`304`**, are passed back when the CDN responds that way.
- `IPTV_TUNERR_FFMPEG_PATH` — override the ffmpeg binary path (e.g. `/opt/ffmpeg-static/current/ffmpeg`).
- `IPTV_TUNERR_FFMPEG_DISABLED` — disable ffmpeg entirely for HLS relay and stay on the Go playlist/segment fetch path. Useful when ffmpeg cannot satisfy provider header/cookie requirements.
- `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE` — keep the original ffmpeg input hostname instead of rewriting it to a resolved IP. Useful for CDNs that validate the hostname against `Host` or TLS state.
- `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` — when `true`, adds HLS reconnect flags to ffmpeg (`-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1`). Helps with providers whose HLS segment URLs expire mid-stream.
- `IPTV_TUNERR_FFMPEG_HLS_PACKAGER_DIR` — optional base directory for ffmpeg-packaged HLS session workdirs. Empty = system temp dir.
- `IPTV_TUNERR_FFMPEG_HLS_PACKAGER_STARTUP_TIMEOUT_MS` — wait this long for the first packaged playlist file before falling back to the normal relay path (default **8000**).
- `IPTV_TUNERR_FFMPEG_HLS_PACKAGER_FILE_WAIT_TIMEOUT_MS` — wait this long for a packaged playlist/segment file on follow-up requests (default **4000**).
- `IPTV_TUNERR_FFMPEG_HLS_PACKAGER_IDLE_SEC` — remove an inactive packaged-HLS session after this idle time (default **45**).
- `IPTV_TUNERR_FFMPEG_HLS_PACKAGER_MAX_AGE_SEC` — hard cap on packaged-HLS session lifetime before janitor cleanup (default **300**).
- `IPTV_TUNERR_FFMPEG_HLS_PACKAGER_LIST_SIZE` — ffmpeg HLS playlist window size for packaged-HLS mode (default **6**).
- `IPTV_TUNERR_FFMPEG_HLS_PACKAGER_SEGMENT_SECONDS` — ffmpeg HLS segment target duration for packaged-HLS mode (default **2**).
- `IPTV_TUNERR_FFMPEG_HLS_FIRST_BYTES_TIMEOUT_MS` — for non-transcode HLS ffmpeg-remux, wait this long for the first output bytes before aborting remux and falling back instead of letting the client sit on a dead remux attempt (default **4000**; `0` disables).
- `IPTV_TUNERR_HLS_PLAYLIST_RETRY_LIMIT` / `IPTV_TUNERR_HLS_PLAYLIST_RETRY_BACKOFF_MS` — bounded retry/backoff for playlist refreshes that hit provider concurrency/limit responses; intended for short-lived `509`/similar contention rather than permanent failures.
- `IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE` — skip non-transcode ffmpeg remux and go straight to the Go playlist/segment relay when Tunerr has **learned concurrency pressure** *or* the upstream host already has **autotune penalty** (same process; requires **`IPTV_TUNERR_PROVIDER_AUTOTUNE`** so failures are recorded). Turning this **off** disables both signals unless **`IPTV_TUNERR_HLS_RELAY_PREFER_GO`** is on.
- `IPTV_TUNERR_HLS_RELAY_PREFER_GO` — unconditional Go-relay preference (overrides the `false` branch above).
- `IPTV_TUNERR_HLS_RELAY_ALLOW_FFMPEG_CROSS_HOST` — default `false`; when disabled, non-transcode HLS playlists that reference media/key/map/variant URLs on a different host than the playlist itself skip ffmpeg remux and use the Go relay. This avoids one static ffmpeg request-header context being reused across cross-host HLS subrequests.
- `IPTV_TUNERR_CLIENT_ADAPT` — when `true`, resolve the Plex client from the active session and force websafe (transcode + MP3 audio) for web/browser clients and for internal fetchers (Lavf/PMS). Ensures Chrome and Firefox get compatible audio without transcoding non-browser clients.
- `IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK` — when enabled (default), if adaptation chose the **non-websafe** path and the tune ends with **`all_upstreams_failed`** or **`upstream_concurrency_limited`**, register a **session-scoped** WebSafe fallback for that channel + Plex session/client id until TTL (see [plex-livetv-http-tuning](plex-livetv-http-tuning.md) **HR-004**).
- `IPTV_TUNERR_CLIENT_ADAPT_STICKY_TTL_SEC` — sticky lifetime in seconds (default **14400**; clamped **120**–**604800**).
- `IPTV_TUNERR_CLIENT_ADAPT_STICKY_LOG` — set `1` to include internal sticky map keys in logs (with **`IPTV_TUNERR_STREAM_DEBUG`**).
- `IPTV_TUNERR_UPSTREAM_HEADERS` — comma-separated extra headers applied to upstream playlist and segment requests, for example `Referer`, `Origin`, or `Host`.
- `IPTV_TUNERR_UPSTREAM_ADD_SEC_FETCH` — add `Sec-Fetch-Site: cross-site` and `Sec-Fetch-Mode: cors` on upstream requests and ffmpeg inputs.
- `IPTV_TUNERR_UPSTREAM_USER_AGENT` — override the upstream `User-Agent` while leaving downstream client detection untouched. Accepts preset names (`lavf`, `vlc`, `mpv`, `kodi`, `firefox`) or a literal UA string. When set to a preset, the resolved string matches the installed ffmpeg version (for `lavf`) or a canonical media-player/browser value.
- `IPTV_TUNERR_COOKIE_JAR_FILE` — persist upstream cookies learned during playback so provider/CDN clearance tokens (`cf_clearance`) survive restarts. Required for CF auto-boot persistence and for `cf-status` / `import-cookies` to know where to read/write.
- `IPTV_TUNERR_CF_LEARNED_FILE` — optional explicit path for the per-host CF learned state file (`cf-learned.json`). When unset, Tunerr automatically derives the path next to `IPTV_TUNERR_COOKIE_JAR_FILE`. Stores: working UA found by cycling, CF-tagged flag, timestamp. Written atomically on every update. Read at startup to pre-populate `learnedUAByHost` so UA cycling does not repeat after restarts.
- `IPTV_TUNERR_HOST_UA` — comma-separated `host:preset` pairs to pin a resolved upstream User-Agent per hostname at startup, without waiting for automatic cycling. Preset names: `lavf`/`ffmpeg` (auto-detected ffmpeg version), `vlc`, `mpv`, `kodi`, `firefox`, or any literal UA string. Example: `IPTV_TUNERR_HOST_UA=provider.example.com:vlc,cdn2.example.com:lavf`. Pre-populates `learnedUAByHost`; does not prevent cycling from updating the value later if a CF block is observed.
- `IPTV_TUNERR_STREAM_ATTEMPT_LOG` — path to a JSONL file where each stream attempt is appended as a JSON record. Written asynchronously; does not block the stream path. The in-process ring buffer at `/debug/stream-attempts.json` resets on restart; this file persists across restarts for post-mortem analysis. Consumed by `scripts/analyze-bundle.py`. Example: `IPTV_TUNERR_STREAM_ATTEMPT_LOG=/var/log/tunerr-attempts.jsonl`.
- `IPTV_TUNERR_AUTOPILOT_STATE_FILE` — optional JSON file for remembered playback decisions keyed by `dna_id + client_class`; when enabled, successful stream choices can be reused on later requests before generic adaptation rules, including the last known-good upstream URL/host.
- `IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST` — when `true`/`1`/`on`, channels with **no** matching per-DNA Autopilot row (or stale URL memory) may prefer an upstream **hostname** that appears across multiple other channels’ remembered `preferred_host` values (aggregate agreement). Requires at least **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_DNA`** distinct `dna_id` rows (default **3**) and **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_HIT_SUM`** total `hits` across those rows for that host (default **15**). Skips hosts with autotune penalty. Reported on **`/autopilot/report.json`** and **`intelligence.autopilot`** on **`/provider/profile.json`**.
- `IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS` — comma-separated upstream **hostnames** (no `https://`; match is case-insensitive on URL host). When set, **`reorderStreamURLs`** prefers the first catalog **`StreamURLs`** entry whose host matches any listed host **after** per-channel Autopilot memory (URL/host) and **before** consensus host + penalty sort. Does not require **`IPTV_TUNERR_AUTOPILOT_STATE_FILE`**. Echoed on **`/autopilot/report.json`** (`global_preferred_hosts`), **`intelligence.autopilot`**, and **`/debug/runtime.json`** → **`tuner.autopilot_global_preferred_hosts`**.
- `IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE` — optional JSON file for global host policy. Supported keys: **`global_preferred_hosts`** / **`preferred_hosts`** and **`global_blocked_hosts`** / **`blocked_hosts`**. Preferred hosts merge with **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`**; blocked hosts are removed from **`reorderStreamURLs`** when at least one non-blocked backup remains. Surfaced on **`/autopilot/report.json`** (`host_policy_file`, `global_blocked_hosts`) and **`/debug/runtime.json`** → **`tuner.autopilot_host_policy_file`**.
- `IPTV_TUNERR_AUTOPILOT_MAX_FAILURE_STREAK` — maximum remembered failure streak before a stored Autopilot decision stops being reused automatically (default `2`)
- `IPTV_TUNERR_HOT_START_ENABLED` — enable hot-start tuning for favorite/high-hit channels (default `true`)
- `IPTV_TUNERR_HOT_START_CHANNELS` — comma-separated explicit favorites by `channel_id`, `dna_id`, `guide_number`, or exact `guide_name`
- `IPTV_TUNERR_HOT_START_GROUP_TITLES` — comma-separated substrings matched against each channel's M3U **`group_title`** (case-insensitive **substring**). When any needle matches, hot-start applies with reason **`group_title`**. Empty **`group_title`** never matches. Precedence: explicit **`HOT_START_CHANNELS`** first, then **`HOT_START_GROUP_TITLES`**, then Autopilot **`HOT_START_MIN_HITS`**. **`/debug/runtime.json`** → **`tuner.hot_start_*`** echoes enabled / min-hits / this env (when set).
- `IPTV_TUNERR_HOT_START_MIN_HITS` — minimum remembered Autopilot hits before a channel becomes hot automatically (default `3`)
- `IPTV_TUNERR_HOT_START_MIN_BYTES` — lower startup-gate byte threshold for hot channels (default `24576`)
- `IPTV_TUNERR_HOT_START_TIMEOUT_MS` — lower startup-gate timeout for hot channels (default `15000`)
- `IPTV_TUNERR_HOT_START_BOOTSTRAP_SECONDS` — bootstrap burst duration for hot channels (default `2.0`)
- `IPTV_TUNERR_HOT_START_PROGRAM_KEEPALIVE` — enable PAT/PMT keepalive automatically for hot channels (default `true`)
- `IPTV_TUNERR_FORCE_WEBSAFE` — when `true`, always transcode with MP3 audio regardless of client. Use if client detection misclassifies a browser client or after a Plex update changes the session UA.
- `IPTV_TUNERR_STRIP_STREAM_HOSTS` — comma-separated hostnames (e.g. `cf.like-cdn.com,like-cdn.com`) whose stream URLs are removed at catalog build time. Channels with only stripped hosts are dropped entirely so the tuner never attempts CF-blocked endpoints.
- `IPTV_TUNERR_DEDUPE_BY_TVG_ID` — when `true`/`1`/`on` (default), merge catalog rows that share the same **`tvg_id`** during **`index`** (including a **post-merge** pass after free sources + HDHR hardware lineup). Set `false`/`0`/`off` to disable (niche debugging).

## Live TV startup race hardening (websafe bootstrap)

These vars address the Plex `dash_init_404` / "Failed to find consumer" race where Plex accepts a session but its DASH packager doesn't receive usable MPEG-TS bytes fast enough to initialize.

- `IPTV_TUNERR_WEBSAFE_BOOTSTRAP` — when `true`, sends a short burst of TS bytes immediately on stream open to give Plex's packager a head start before the real stream arrives.
- `IPTV_TUNERR_WEBSAFE_BOOTSTRAP_ALL` — apply bootstrap to all stream types, not just HLS inputs.
- `IPTV_TUNERR_WEBSAFE_BOOTSTRAP_SECONDS` — duration of the bootstrap burst (default `0.35`).
- `IPTV_TUNERR_WEBSAFE_STARTUP_MIN_BYTES` — startup gate: minimum buffered bytes before releasing **main** ffmpeg TS to the client (default `65536`).
- `IPTV_TUNERR_WEBSAFE_STARTUP_MAX_BYTES` — max bytes of ffmpeg TS kept while scanning for a decodable start (default **`786432`**). With **`IPTV_TUNERR_WEBSAFE_REQUIRE_GOOD_START`**, the scan uses a **sliding window** of this size (**HR-001**) so the gate does not flush at max size without **H.264 IDR + AAC**.
- `IPTV_TUNERR_WEBSAFE_STARTUP_TIMEOUT_MS` — max wait for prefetch to finish (default **`60000`** ms in gateway code).
- `IPTV_TUNERR_WEBSAFE_REQUIRE_GOOD_START` — when `true` (default) on transcode/WebSafe ffmpeg relay, require **H.264 IDR** (Annex B) **or H.265/HEVC IRAP-family NAL** (types **16–21**, Annex B) plus **AAC ADTS** in the scanned TS (plus **`STARTUP_MIN_BYTES`**) before releasing; still aborts on timeout per strict mode. Logs include **`release=min-bytes-idr-aac-ready`** or other **`release=`** reasons on the **`startup-gate buffered=`** line (field **`idr=`** means “video keyframe / IRAP detected”, not H.264-only).
- `IPTV_TUNERR_WEBSAFE_STARTUP_MAX_FALLBACK_WITHOUT_IDR` — when `true`, allow the legacy **max-bytes** release without IDR/AAC (logged **`release=max-bytes-without-idr-fallback`**). Default **`false`**. Escape hatch for odd codecs; prefer larger **`STARTUP_MAX_BYTES`** or **`REQUIRE_GOOD_START=false`** for experiments.
- `IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE` — send null TS packets (PID `0x1FFF`) while the startup gate waits. Keeps the TCP connection alive but carries no program structure.
- `IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE_MS` — interval between null TS bursts (default `100`ms).
- `IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE_PACKETS` — null TS packets per burst (default `1`).
- `IPTV_TUNERR_WEBSAFE_PROGRAM_KEEPALIVE` — send real PAT+PMT packets while the startup gate waits. Stronger than null TS: delivers the program map (video+audio PIDs) so Plex's DASH packager can instantiate its consumer before the first IDR frame arrives. Use when null TS alone doesn't prevent `dash_init_404`.
- `IPTV_TUNERR_WEBSAFE_PROGRAM_KEEPALIVE_MS` — interval between PAT+PMT bursts (default `500`ms).

See [iptvtunerr-troubleshooting §6](../runbooks/iptvtunerr-troubleshooting.md#6-plex-live-tv-startup-race-session-opens-consumer-never-starts) for a recommended config profile.

## Guide / XMLTV

The guide pipeline serves the most complete data available, merging three sources in priority order (highest wins per channel):

```
placeholder  <  external XMLTV  <  provider XMLTV (xmltv.php)
```

External gap-fills provider for any time windows the provider EPG doesn't cover. The cache is pre-warmed synchronously at startup so the first request is never cold. On fetch failure, stale data is served — no guide outage on transient errors.

### Provider EPG (Xtream `xmltv.php`)

Fetches EPG directly from your IPTV provider using existing credentials. No separate EPG source needed for Xtream providers.

- `IPTV_TUNERR_PROVIDER_EPG_ENABLED` (`true`) — set `false` to disable provider EPG fetch
- `IPTV_TUNERR_PROVIDER_EPG_TIMEOUT` (`90s`) — fetch timeout; provider XMLTV can be large (10–50 MB)
- `IPTV_TUNERR_PROVIDER_EPG_CACHE_TTL` (`10m`) — how often to re-fetch; overrides `XMLTV_CACHE_TTL` when set
- `IPTV_TUNERR_PROVIDER_EPG_DISK_CACHE` — optional filesystem path to store the last downloaded provider `xmltv.php` body. When set, Tunerr persists **`ETag`** / **`Last-Modified`** in a sidecar `*.meta.json` and sends conditional request headers; **HTTP 304** responses skip re-download and parse the cached file. Many Xtream panels do **not** emit validators — in that case behavior matches an uncached fetch (full download each refresh). Create the parent directory before use.
- `IPTV_TUNERR_PROVIDER_EPG_INCREMENTAL` (`false`) — when `true`, apply token rendering on `IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX` from SQLite horizon (`GlobalMaxStopUnix`)
- `IPTV_TUNERR_PROVIDER_EPG_LOOKAHEAD_HOURS` (`72`) — window end offset for incremental suffix token rendering
- `IPTV_TUNERR_PROVIDER_EPG_BACKFILL_HOURS` (`6`) — start offset before known max stop for incremental suffix token rendering
- Suffix tokens: `{from_unix}`, `{to_unix}`, `{from_ymd}`, `{to_ymd}` (only meaningful when incremental is enabled and SQLite store has data)

### External XMLTV (tier 2)

- `IPTV_TUNERR_XMLTV_URL` — external XMLTV source URL; fetched, filtered to your channels, remapped to guide numbers
- `IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP` — allow private/loopback `http(s)` refs for external XMLTV / alias fetches. Default is **off** as a hardening measure; prefer filesystem paths for local files and enable this only when you intentionally fetch from localhost/LAN.
- `IPTV_TUNERR_GUIDE_INPUT_ALLOWED_URLS` — comma-separated extra exact remote XMLTV / alias URLs allowed beyond the configured provider, XMLTV, or HDHomeRun guide URLs. Use this only when those sources are intentional and stable.
- `IPTV_TUNERR_GUIDE_INPUT_ROOTS` — comma-separated safe root directories for local XMLTV / alias files. Relative paths resolve under the current working directory by default; absolute paths outside these roots are rejected.
- `IPTV_TUNERR_XMLTV_ALIASES` — optional file path or `http(s)` URL for alias overrides used in deterministic EPG repair
- `IPTV_TUNERR_CATCHUP_GUIDE_POLICY` — optional `off|healthy|strict`; applies guide-quality filtering to `/guide/capsules.json`, `catchup-capsules`, and `catchup-publish`
- `IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE` — optional source-backed replay URL template for capsules/publishing; when set, replay URLs are rendered with programme and channel tokens instead of falling back to live-channel launchers
- `IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS` — comma-separated hostnames; for `catchup-daemon` / `catchup-record` with `-record-upstream-fallback`, catalog capture fallbacks whose host matches (or is a subdomain of) these names are tried after other fallbacks (Tunerr `/stream/<id>` stays first)
- `IPTV_TUNERR_XMLTV_MATCH_ENABLE` — repair/assign channel `TVGID`s from provider/external XMLTV channel metadata during catalog build (default `true`)
- `IPTV_TUNERR_XMLTV_TIMEOUT` — fetch timeout (default `45s`)
- `IPTV_TUNERR_XMLTV_CACHE_TTL` — refresh interval when provider EPG cache TTL is not set (default `10m`)
- `IPTV_TUNERR_LIVE_EPG_ONLY` — at **catalog build**, keep only channels with **`epg_linked`** / a **`tvg-id`** (drops unlinked rows before save). See [lineup-epg-hygiene](lineup-epg-hygiene.md).
- `IPTV_TUNERR_EPG_PRUNE_UNLINKED` — exclude channels with no EPG match from emitted `guide.xml` and `live.m3u` by default
- `IPTV_TUNERR_EPG_FORCE_LINEUP_MATCH` — keep every lineup row represented in `guide.xml` even when `IPTV_TUNERR_EPG_PRUNE_UNLINKED=1`, using placeholder guide rows for unmatched channels so Plex can still map tuner channels to guide channels
- `IPTV_TUNERR_EPG_SQLITE_PATH` — optional filesystem path to a **SQLite** file for durable EPG storage (merged guide sync after each refresh; schema v2 includes `epg_meta`). Empty = disabled. Rationale: [ADR 0003](../adr/0003-epg-sqlite-vs-postgres.md).
- `IPTV_TUNERR_EPG_SQLITE_RETAIN_PAST_HOURS` — if `> 0`, after each sync delete SQLite programme rows whose **end time** is before `now - N hours`, then remove orphan `epg_channel` rows. `0` = keep the full merged snapshot in SQLite.
- `IPTV_TUNERR_EPG_SQLITE_VACUUM` — if `true`/`1`, run SQLite **`VACUUM`** after a retain-past prune that removed at least one row (optional; reclaims file space, may pause briefly on large DBs).
- `IPTV_TUNERR_EPG_SQLITE_MAX_BYTES` — optional post-sync cap on the **SQLite file size** (bytes); deletes programmes until the file fits (ended programmes first). `IPTV_TUNERR_EPG_SQLITE_MAX_MB` is a shorthand (mebibytes) if `MAX_BYTES` is unset.
- `IPTV_TUNERR_EPG_SQLITE_INCREMENTAL_UPSERT` — use overlap-window upsert sync mode for merged XMLTV (does not truncate all programme/channel rows each refresh)
- `IPTV_TUNERR_HDHR_LINEUP_URL` — optional `http(s)://device/lineup.json` merged into the catalog on **`iptv-tunerr index`** (LP-002). `IPTV_TUNERR_HDHR_LINEUP_ID_PREFIX` (default `hdhr`) prefixes generated `channel_id`s.
- `IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX` — optional string appended to provider `xmltv.php` as `&…` (e.g. panel-specific query params). **Not** part of stock Xtream; only use if your provider documents extra parameters.
- `IPTV_TUNERR_HDHR_GUIDE_URL` — optional http(s) URL to a **physical HDHomeRun-style** `guide.xml` (e.g. `http://192.168.1.50/guide.xml`). Merged **after** provider + external gap-fill; see [ADR 0004](../adr/0004-hdhr-guide-epg-merge.md).
- `IPTV_TUNERR_HDHR_GUIDE_TIMEOUT` — fetch timeout for the HDHR guide URL (default `90s`).
- `IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS` — optional comma-separated **literal** UDP targets for HDHR discovery (`hdhr-scan` without **`-addr`**, and **`DiscoverLAN`**). **IPv4** entries are subnet broadcasts (e.g. `192.168.1.255`, default port **65001**) or `host:port`. **IPv6** entries (e.g. `[::1]:65001`, `fe80::1%eth0:65001`, or `::1:65001` with a trailing port) use a separate UDP6 socket; link-local addresses should include a **zone** (`%eth0`) when required. Global IPv4 `255.255.255.255` is always tried first. Hostnames are not resolved (same as IPv4-only behavior).
  - Tunerr HTTP: `GET /guide/epg-store.json` — row counts, `last_sync_utc`, `global_max_stop_unix`, `retain_past_hours`, `db_file_bytes`, `db_file_modified_utc`, `vacuum_after_prune`; add `?detail=1` for `channel_max_stop_unix` (incremental fetch horizon).

### XMLTV language normalization

Applied to all sources (provider and external):
- `IPTV_TUNERR_XMLTV_PREFER_LANGS` — preferred language codes for programme titles/descriptions (e.g. `en,eng`)
- `IPTV_TUNERR_XMLTV_PREFER_LATIN` — prefer Latin-script variants where available
- `IPTV_TUNERR_XMLTV_NON_LATIN_TITLE_FALLBACK` — what to use when title text is non-Latin and no Latin variant exists (`channel` = use channel name)

## HDHR network mode

- `IPTV_TUNERR_HDHR_NETWORK_MODE`
- `IPTV_TUNERR_HDHR_DEVICE_ID`
- `IPTV_TUNERR_HDHR_TUNER_COUNT`
- `IPTV_TUNERR_HDHR_FRIENDLY_NAME`
- `IPTV_TUNERR_HDHR_SCAN_POSSIBLE`
- `IPTV_TUNERR_HDHR_MANUFACTURER`
- `IPTV_TUNERR_HDHR_MODEL_NUMBER`
- `IPTV_TUNERR_HDHR_FIRMWARE_NAME`
- `IPTV_TUNERR_HDHR_FIRMWARE_VERSION`
- `IPTV_TUNERR_HDHR_DEVICE_AUTH`

## Plex session reaper (built-in)

Required:
- `IPTV_TUNERR_PMS_URL`
- `IPTV_TUNERR_PMS_TOKEN`

Enable/tune:
- `IPTV_TUNERR_PLEX_SESSION_REAPER`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_POLL_S`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_IDLE_S`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_RENEW_LEASE_S`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_HARD_LEASE_S`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_SSE`

## HDHR wizard-lane shaping (optional)

- `IPTV_TUNERR_LINEUP_DROP_MUSIC`
- `IPTV_TUNERR_LINEUP_SHAPE`
- `IPTV_TUNERR_LINEUP_REGION_PROFILE`

Typical use:
- broad feed HDHR lane capped to `479`
- category DVR lanes use separate M3U inputs and no shaping

## Platform notes

- `mount` / VODFS is Linux-only
- `vod-webdav` is cross-platform and provides the non-Linux VOD parity path
- Core tuner paths (`run`, `serve`, `supervise`) are cross-platform
- HDHR network mode compiles on Linux/macOS/Windows; validate native Windows networking on a real Windows host (not `wine`)

## Verification helpers

- `./scripts/verify`
- `./scripts/build-test-packages.sh`
- `./scripts/build-tester-release.sh`
- `./scripts/plex-hidden-grab-recover.sh --dry-run`

See also
--------
- [testing-and-supervisor-config](testing-and-supervisor-config.md)
- [package-test-builds](../how-to/package-test-builds.md)
- [tester-handoff-checklist](../how-to/tester-handoff-checklist.md)
- [memory-bank/commands.yml](../../memory-bank/commands.yml)
