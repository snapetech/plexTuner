---
id: changelog
type: reference
status: stable
tags: [changelog, reference]
---

# Changelog

All notable changes to IPTV Tunerr are documented here. Repo: [github.com/snapetech/iptvtunerr](https://github.com/snapetech/iptvtunerr).

---

## [Unreleased]

### Hardware HDHomeRun (client spike)
- **`hdhr-scan`**: UDP discovery for physical SiliconDust tuners (or `-addr` for HTTP-only `discover.json` / optional `lineup.json`). Implemented in `internal/hdhomerun` (`DiscoverLAN`, `FetchDiscoverJSON`, `FetchLineupJSON`).
- **`hdhr-scan -guide-xml`**: fetch device `guide.xml`, count XMLTV `channel` / `programme` elements (`internal/hdhomerun/guide.go`); still no merge into Tunerr EPG (see ADR 0002).
- **Operator `/ui/`**: minimal embedded HTML dashboard (`internal/tuner/static/ui/`, `IPTV_TUNERR_UI_*`); localhost-only by default.
- **ADR 0002** ([docs/adr/0002-hdhr-hardware-iptv-merge.md](adr/0002-hdhr-hardware-iptv-merge.md)): how HDHR hardware lineups relate to IPTV catalogs (tag sources; separate instances until explicit merge).

---

## [v0.1.14] — 2026-03-19

### Documentation & diagnostics
- **Cloudflare operator guide**: added [how-to/cloudflare-bypass.md](how-to/cloudflare-bypass.md) (automatic UA cycling, header profiles, cookies, `cf-status`, env knobs).
- **Debug bundle workflow**: added `iptv-tunerr debug-bundle` plus [how-to/debug-bundle.md](how-to/debug-bundle.md) and `scripts/analyze-bundle.py` for correlating stream attempts, logs, and pcaps.
- **README**: expanded Cloudflare troubleshooting section and cross-links to the new how-to guides.

### QA / diagnostics
- **Direct-vs-Tunerr comparison harness**: added `scripts/stream-compare-harness.sh` and `scripts/stream-compare-report.py` to capture `ffprobe`, `ffplay`, `curl`, and optional `tcpdump` evidence for a direct upstream URL versus the equivalent Tunerr stream URL in one reproducible bundle.
- **Structured stream-attempt export**: added `/debug/stream-attempts.json`, which exposes recent gateway decisions, per-upstream outcomes, effective URLs, and redacted request/ffmpeg header summaries for debugging direct-vs-Tunerr mismatches.
- **Troubleshooting workflow update**: the runbook now documents the new comparison harness, including header-file inputs, pcap generation, and how to inspect the resulting artifacts in Wireshark or `tshark`.

### Catch-up recording
- **Recorder daemon MVP**: added `iptv-tunerr catchup-daemon`, which continuously scans guide-derived capsules, records eligible `in_progress` / `starting_soon` items, dedupes by capsule identity, enforces a max-concurrency budget, and persists `active` / `completed` / `failed` state to JSON.
- **Recorder publish/retention hooks**: completed daemon recordings can now be published into a media-server-friendly directory layout with `.nfo` sidecars, and expired or over-retained recordings are pruned automatically.
- **Recorder publish-time library registration**: `catchup-daemon` can now reuse the same lane library workflow as `catchup-publish`, creating/reusing Plex, Emby, and Jellyfin libraries and triggering targeted refreshes as completed recordings land under `-publish-dir`.
- **Recorder policy filters and duplicate suppression**: `catchup-daemon` now supports channel-level allow/deny filters (`-channels`, `-exclude-channels`) and suppresses duplicate recordings by programme identity (`dna_id`/channel + start + title), not only by exact `capsule_id`.
- **Recorder status/reporting surface**: added `catchup-recorder-report` plus `/recordings/recorder.json`, which summarize recorder state, per-lane counts, published item totals, and recent active/completed/failed items from the persistent daemon state file.
- **Lane-specific retention and storage budgets**: `catchup-daemon` now supports per-lane completed/failed retention counts and per-lane completed-item storage budgets, pruning older items first within each lane before global retention limits are applied.
- **Interrupted-recording recovery semantics**: daemon restarts now preserve unfinished recordings as explicit failed `status=interrupted` items with recovery metadata and partial byte counts when available, and the scheduler can automatically retry the same programme window if it is still eligible after restart.
- **Recorder spool/finalize**: `catchup-record` / `catchup-daemon` capture streams to `<lane>/<sanitized-capsule-id>.partial.ts` first and rename to the final `.ts` only after a clean HTTP 200 + body copy; interrupted or failed runs no longer leave a finished-looking `.ts` on disk.
- **Recorder transient retry/backoff**: `catchup-daemon` can retry a capture when errors look transient (typical 5xx/429/408-style HTTP failures, timeouts, connection resets) with exponential backoff capped by `-record-retry-backoff-max`, up to `-record-max-attempts`.
- **Recorder same-spool HTTP Range resume**: after transient mid-stream failures, `catchup-daemon` can retry with `Range` against the existing `.partial.ts` spool when the upstream supports partial content (206), avoiding a full re-download when possible (`-record-resume-partial`, default on).
- **Recorder smarter backoff**: transient retries honor `Retry-After` when present and apply HTTP-status-aware backoff multipliers (e.g. 429/502/503/504) on top of exponential backoff.
- **Recorder capture observability**: per-item fields `capture_http_attempts`, `capture_transient_retries`, and `capture_bytes_resumed`, plus aggregate `sum_*` fields in `recorder-state.json` statistics, summarize HTTP attempts, retry churn, and bytes appended via resume.
- **Recorder multi-upstream failover**: catalog `stream_url` / `stream_urls` are merged (after the Tunerr `/stream/<id>` URL) into `record_source_urls` when `-record-upstream-fallback` is enabled (default on for `catchup-daemon` / `catchup-record`); capture advances to the next URL after non-transient failures or exhausted transient retries, with `capture_upstream_switches` / `sum_capture_upstream_switches` metrics.
- **Recorder catalog UA on capture**: `preferred_ua` from the live channel is sent as `User-Agent` on capture HTTP requests when present.
- **Recorder time-based completed retention**: `-retain-completed-max-age` and `-retain-completed-max-age-per-lane` (e.g. `sports=72h`, `7d`) prune old completed items from state and delete associated files.
- **Recorder soak helper**: `scripts/recorder-daemon-soak.sh` wraps `catchup-daemon -run-for` for bounded soak runs.
- **Recorder fallback URL ordering**: `IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS` (comma-separated hosts) moves matching catalog fallbacks after healthier URLs; the Tunerr `/stream/<id>` URL stays first.

### Upstream / Cloudflare hardening
- **`cf-status` CLI**: inspect per-host Cloudflare state from the cookie jar and persisted learned file (`cf_clearance` freshness, working UA, CF-tagged flag); JSON output supported.
- **CF learned persistence**: Tunerr persists per-host working UA and CF-tagged flags to `cf-learned.json` (path via `IPTV_TUNERR_CF_LEARNED_FILE` or auto-derived next to the cookie jar), and restores them on startup.
- **Per-host UA override**: `IPTV_TUNERR_HOST_UA=host:preset,...` pins a resolved User-Agent preset per upstream host without waiting for cycling.
- **CF bootstrap**: browser-style header profiles accompany browser UAs during probe cycling; optional background freshness refresh reduces mid-session expiry surprises for `cf_clearance`.
- **Recorder lane budget visibility**: `recorder-state.json` statistics now include `lane_storage` with per-lane `used_bytes` and optional `budget_bytes` / `headroom_bytes` when `-budget-bytes-per-lane` is set.
- **Deferred library refresh**: with `-register-*` and `-refresh`, `-defer-library-refresh` registers/reuses libraries per recording but runs the media-server library scan once after `recorded-publish-manifest.json` is updated for that completion.
- **Better ffmpeg HLS request parity**: ffmpeg relay inputs now inherit the effective upstream `User-Agent`, `Referer`, and cookie-jar cookies more faithfully, and enable persistent/multi-request HLS HTTP input by default to better match successful direct `ffplay` behavior on legitimate CDN/HLS paths.

---

## [v0.1.12] — 2026-03-19

### Streaming
- **Provider/CDN compatibility controls**: added `IPTV_TUNERR_UPSTREAM_HEADERS`, `IPTV_TUNERR_UPSTREAM_ADD_SEC_FETCH`, `IPTV_TUNERR_UPSTREAM_USER_AGENT`, `IPTV_TUNERR_COOKIE_JAR_FILE`, `IPTV_TUNERR_FFMPEG_DISABLED`, and `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE` so operators can match stricter upstream header/cookie expectations and disable ffmpeg-side host rewriting when necessary.
- **Redirect-safe HLS relay**: HLS playlist rewriting and refresh now track the effective post-redirect playlist URL, so relative segment or nested playlist paths keep resolving correctly after CDN redirects.
- **Credential-aware fallback stream routing**: multi-provider fallback URLs now keep per-stream auth metadata through catalog dedupe and host filtering, so channel changes and second-session failover do not silently reuse provider-1 credentials against provider-2 URLs.
- **FFmpeg Cloudflare cookie forwarding**: ffmpeg HLS relay inputs now inherit the same per-stream credentials and learned upstream cookies as the Go gateway client, which closes the remaining gap where Cloudflare-cleared playlists still failed once ffmpeg took over segment fetches.
- **Direct player_api fallback now preserves multi-provider backups**: when probe ranking returns no provider as `OK` but direct `player_api` indexing still works, the catalog now keeps multi-provider fallback URLs and per-stream auth rules instead of collapsing back to a single provider path.
- **Invalid HLS playlists now fail over**: `.m3u8` responses that come back as empty or HTML are now treated as upstream failures and the gateway advances to the next fallback URL instead of stalling on a useless `200`.

### Guide / intelligence
- **Guide health report**: added `guide-health` plus `/guide/health.json` to combine XMLTV match status with actual merged-guide coverage, including detection of placeholder-only channel rows versus real programme blocks.
- **EPG doctor workflow**: added `epg-doctor` plus `/guide/doctor.json` as the combined top-level diagnosis path, and cached live guide match provenance so repeated guide diagnostics do not rebuild the same match analysis on every request.
- **EPG auto-fixer**: `epg-doctor -write-aliases` and `/guide/aliases.json` can now export reviewable `name_to_xmltv_id` suggestions from healthy normalized-name matches so repaired guide links can be persisted.
- **Channel leaderboard**: added `channel-leaderboard` plus `/channels/leaderboard.json` for hall-of-fame, hall-of-shame, guide-risk, and stream-risk snapshots of the lineup.
- **Guide-quality policy hooks**: added shared guide-health caching plus `IPTV_TUNERR_GUIDE_POLICY` / `IPTV_TUNERR_CATCHUP_GUIDE_POLICY` so runtime lineup shaping and catch-up capsule output can optionally suppress placeholder-only or no-programme channels.
- **Intent lineup recipes**: `IPTV_TUNERR_LINEUP_RECIPE` now includes `sports_now`, `kids_safe`, and `locals_first` in addition to the earlier score-based recipes.
- **Registration recipes**: added `IPTV_TUNERR_REGISTER_RECIPE` / `run -register-recipe` so Plex, Emby, and Jellyfin registration can now reuse channel-intelligence scoring instead of blindly syncing catalog order.
- **Registration intent presets**: media-server registration now also accepts `sports_now`, `kids_safe`, and `locals_first`, matching the runtime lineup recipe presets.
- **Source-backed catch-up replay mode**: `catchup-capsules`, `/guide/capsules.json`, and `catchup-publish` now support `IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE`, which renders programme-window replay URLs when a real replay-capable source exists instead of pretending the live launcher is a recording.
- **Autopilot hot-start**: added `autopilot-report` plus `/autopilot/report.json`, and hot-start tuning now lets favorite or high-hit channels use more aggressive ffmpeg startup thresholds/keepalive on the HLS path.
- **Autopilot upstream memory**: remembered playback decisions now also keep the last known-good upstream URL/host, so repeat requests can prefer the working stream path first on duplicate or multi-CDN channels.
- **Provider host penalties**: provider autotune now tracks repeated host-level upstream failures and automatically prefers healthier stream hosts/CDNs before retrying known-bad ones.
- **Channel DNA preference policy**: added `IPTV_TUNERR_DNA_POLICY=prefer_best|prefer_resilient` so lineup and registration flows can now collapse duplicate `dna_id` variants to a preferred winner instead of only reporting the group.
- **Channel DNA preferred hosts**: added `IPTV_TUNERR_DNA_PREFERRED_HOSTS` so duplicate-variant selection can bias trusted provider/CDN authorities before falling back to score-based tie-breaking.
- **Ghost Hunter action recommendations**: visible stale sessions and hidden-grab suspicion now produce different recommended next actions and recovery commands, and the live endpoint supports `?stop=true`.
- **Catch-up capsule curation**: duplicate programme rows that share the same `dna_id + start + title` are now curated down to the richer capsule candidate before export/publish.
- **Autopilot failure memory**: remembered Autopilot decisions now track failure counts/streaks too, so stale remembered paths stop being reused automatically after repeated misses.
- **Ghost Hunter recovery hook**: the CLI can now run the guarded hidden-grab helper directly with `-recover-hidden dry-run|restart`.
- **Catch-up recorder**: added `catchup-record`, which records current in-progress capsules to local TS files plus `record-manifest.json` for non-replay sources.
- **Shared ref loader**: report and guide tooling now use one shared local-file/URL loader with the repo HTTP client defaults instead of duplicated `http.DefaultClient` code paths.

### Ingest / probe
- **Server-info Xtream auth probes**: `player_api.php` probes now treat `server_info`-only JSON responses as valid Xtream-style auth success, matching panels that index correctly even when they do not return `user_info`.
- **Direct player_api fallback restored**: when no provider host ranks as probe-OK, catalog refresh now retries direct `IndexFromPlayerAPI` before falling through to `get.php`, restoring the older behavior that kept indexing alive on panels with probe-only response-shape quirks.
- **Multi-entry probe coverage**: `iptv-tunerr probe` now inspects numbered provider entries (`IPTV_TUNERR_PROVIDER_URL_2`, `_3`, etc.) instead of only the primary provider URL.

### Security
- **Xtream path credential redaction**: URL logging now redacts provider credentials embedded in Xtream-style stream paths (`/live/<user>/<pass>/...`, `/movie/...`, `/series/...`, `/timeshift/...`) instead of only stripping query parameters.

---

## [v0.1.10] — 2026-03-18

### Live TV intelligence
- **Channel intelligence foundation**: added `channel-report` plus `/channels/report.json` to score channels by guide confidence, stream resilience, and next-step fixes.
- **EPG match provenance in reports**: when XMLTV is supplied, channel reports now show whether a channel matched by exact `tvg-id`, alias override, normalized-name repair, or not at all.
- **Intelligence-driven lineup recipes**: added `IPTV_TUNERR_LINEUP_RECIPE` with `high_confidence`, `balanced`, `guide_first`, and `resilient` lineup shaping modes.
- **Channel DNA foundation**: live channels now persist a `dna_id` derived from repaired `TVGID` or normalized channel identity inputs, creating a stable identity substrate for future cross-provider intelligence.
- **Channel DNA grouping surface**: added `/channels/dna.json` and `iptv-tunerr channel-dna-report` to group live channels by shared stable identity instead of exposing `dna_id` only as a per-row field.
- **Autopilot memory foundation**: added optional JSON-backed remembered playback decisions keyed by `dna_id + client_class`, allowing successful stream transcode/profile choices to be reused on later requests.
- **Ghost Hunter foundation**: added `ghost-hunter` plus `/plex/ghost-report.json` to observe visible Plex Live TV sessions, classify stalls with reaper heuristics, and optionally stop stale visible transcode sessions.
- **Ghost Hunter escalation**: when Plex exposes zero visible live sessions, Ghost Hunter now flags the hidden-grab pattern explicitly and returns the guarded recovery helper command and runbook path.
- **Provider behavior profile foundation**: added `/provider/profile.json` to expose learned effective tuner cap, recent upstream concurrency-limit signals, Cloudflare-abuse hits, and current auth-context forwarding posture.
- **Provider autotune foundation**: when `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` is not explicitly set, Tunerr can now auto-arm ffmpeg HLS reconnect after it has actually observed HLS playlist/segment instability at runtime.
- **Guide highlights foundation**: added `/guide/highlights.json`, which repackages the cached merged guide into `current`, `starting_soon`, `sports_now`, and `movies_starting_soon` lanes.

### Catch-up publishing
- **Catch-up capsule preview foundation**: added `/guide/capsules.json`, which turns real guide rows into near-live capsule candidates with lane, publish, and expiry metadata for future library publishing.
- **Catch-up capsule export**: added `iptv-tunerr catchup-capsules` to export the capsule preview model to JSON from a catalog plus guide/XMLTV input.
- **Catch-up capsule layout export**: `catchup-capsules -layout-dir` now writes deterministic lane-split JSON files plus `manifest.json` for downstream publisher automation.
- **Catch-up capsule publishing**: added `iptv-tunerr catchup-publish`, which turns capsule rows into `.strm + .nfo` lane libraries plus `publish-manifest.json`, and can now create/reuse matching Plex, Emby, and Jellyfin libraries in one pass.
- **Jellyfin catch-up library compatibility**: catch-up publishing now uses Jellyfin's current `/Library/VirtualFolders` API shape (list via `GET /Library/VirtualFolders`, create with query params) instead of assuming Emby's older `/Query` behavior.
- **Live server validation**: Emby and Jellyfin catch-up publishing were proven live in-cluster against real server PVC paths and created lane libraries plus `.strm + .nfo` output successfully.

### Docs
- **Product roadmap**: documented the Live TV Intelligence epic (Channel DNA, Autopilot, lineup recipes, Ghost Hunter, catch-up capsules).

---

## [v0.1.9] — 2026-03-18

### Build / release
- **Expanded Docker image matrix**: registry publishes now target `linux/amd64`, `linux/arm64`, and `linux/arm/v7`.
- **Correct armv7 Docker cross-builds**: the Docker build path now translates BuildKit's `TARGETVARIANT` into `GOARM`, which is required for correct Go builds on `linux/arm/v7`.

### Docs
- **Container platform alignment**: Docker and packaging docs now match the actual Linux image platforms shipped by the workflow.

---

## [v0.1.8] — 2026-03-18

### Build / release
- **Expanded tagged release binaries**: GitHub Releases now publish `linux/arm/v7` and `windows/arm64` artifacts in addition to the existing `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64` builds.
- **Cleaner release pages**: release notes are generated from repo data instead of generic GitHub auto-notes. When a changelog section exists for the tag, it is used directly on the release page.

### Docs
- **Platform support alignment**: packaging and platform docs now match the actual published binary matrix so operators can see which targets are shipped on tagged releases.

---

## [v0.1.2] — 2026-03-18

### Features
- **Layered EPG pipeline**: guide data now comes from three sources merged by priority — provider XMLTV (`xmltv.php`) > external XMLTV (`IPTV_TUNERR_XMLTV_URL`) > placeholder. External gap-fills provider for any time windows the provider EPG doesn't cover. Placeholder is always the final fallback per channel.
- **Provider EPG via `xmltv.php`**: fetches the Xtream-standard EPG endpoint using existing provider credentials. No additional configuration required for Xtream providers. Produces real programme schedule data without any third-party EPG source.
- **Background refresh**: guide cache is pre-warmed synchronously at startup (first `/guide.xml` request is never cold), then refreshed by a background goroutine on the TTL tick. Stale cache is preserved on fetch error — no guide outage on transient provider failures.

### New env vars
- `IPTV_TUNERR_PROVIDER_EPG_ENABLED` (default `true`) — disable provider `xmltv.php` fetch if not needed
- `IPTV_TUNERR_PROVIDER_EPG_TIMEOUT` (default `90s`) — per-fetch timeout (provider XMLTV can be large)
- `IPTV_TUNERR_PROVIDER_EPG_CACHE_TTL` (default `10m`) — refresh interval; overrides `XMLTV_CACHE_TTL` when set

### Fixes
- **HDHR tuner count integer overflow**: `uint8(tunerCount)` with no bounds check would silently truncate values > 255 in the HDHR discovery packet. Now clamped to [0, 255]. (CodeQL alert #5)

---

## [v0.1.1] — 2026-03-18

- CI: use `GHCR_TOKEN` secret for GHCR registry login; `GITHUB_TOKEN` cannot create new container packages.
- CI: add `release.yml` workflow — creates a GitHub Release with auto-generated notes on every `v*` tag push. `tester-bundles.yml` is now manual-only (`workflow_dispatch`).

---

## [v0.1.0] — 2026-03-17

First tagged release. Covers all features developed through the pre-release testing cycle.

### Features
- **IPTV indexing**: M3U and Xtream `player_api` (live channels, VOD movies, series) with multi-host failover and Cloudflare detection
- **HDHomeRun emulation**: `/discover.json`, `/lineup.json`, `/lineup_status.json`, `/guide.xml`, `/stream/{id}`, `/live.m3u`, `/healthz`
- **Optional native HDHR network mode**: UDP/TCP 65001 for LAN broadcast discovery
- **Stream gateway**: direct MPEG-TS proxy, HLS-to-TS relay, optional ffmpeg transcode (`off`/`on`/`auto`); adaptive buffer; client detection for browser-compatible codec
- **Live TV startup race hardening**: bootstrap TS burst, startup gate, null-TS and PAT+PMT keepalive to prevent Plex `dash_init_404`
- **XMLTV guide**: placeholder or external XMLTV fetch/filter/remap; language/script normalization; TTL cache
- **Supervisor mode**: `iptv-tunerr supervise` runs many child tuner instances from one JSON config for multi-DVR category deployments
- **Plex DVR injection**: programmatic DVR/guide registration via Plex internal API and SQLite (`-register-plex`), bypassing 480-channel wizard limit
- **Emby and Jellyfin support**: tuner registration, idempotent state file, watchdog auto-recovery on server restart
- **VOD filesystem (Linux)**: FUSE mount exposing VOD catalog as directories for Plex library scanning (`iptv-tunerr mount` / `plex-vod-register`)
- **EPG link report**: deterministic coverage report (tvg-id / alias / name match tiers) for improving unlinked channel tail
- **Plex stale-session reaper**: built-in background worker with poll + SSE, configurable idle/lease timeouts
- **Smoketest**: optional per-channel stream probe at index time with persistent cache
- **Lineup shaping**: wizard-safe cap (479), drop-music, region profile, overflow shards (`LINEUP_SKIP`/`LINEUP_TAKE`) for category DVR buckets

### Security
- SSRF prevention: stream gateway validates URLs as HTTP/HTTPS before any fetch
- Credentials redacted from all logs via `safeurl.RedactURL()`
- No TLS verification bypass

### Build / ops
- Single static binary (CGO disabled), Alpine Docker image with ffmpeg
- CI: `go test ./...`, `go vet`, `gofmt` on every push/PR
- Docker: multi-arch (`linux/amd64`, `linux/arm64`), GHCR image on tag push
- Tester bundle workflow: per-platform ZIPs + SHA256SUMS attached to GitHub Release on tag push
- Version embedded at build time via `-ldflags "-X main.Version=..."`; `iptv-tunerr --version` prints it

---

## History (from git)

### Merge and integration (current main)

- **Merge remote-tracking branch origin/main** — Integrate GitHub template updates and restore Plex tuner runtime. Single codebase with agentic template (memory-bank, verify, Diátaxis docs).
- **repo_map:** Document remotes so iptvTunerr only pushes to `origin` and `plex`; do not push from this folder to `github` or `template`.
- **README:** Fix mirror link to iptvTunerr GitHub (not repoTemplate).

### IPTV Tunerr content and docs

- **Fix README and repo docs for IPTV Tunerr** — Align README and docs with actual IPTV Tunerr behavior (IPTV bridge, catalog, tuner, VODFS).
- **Strip all iptvTunerr content from template** — Template repo stripped to generic agentic template; IPTV Tunerr lives in this repo only.
- **Add IPTV Tunerr: IPTV indexer, catalog, VODFS, gateway, and tests** — Initial IPTV Tunerr implementation: index from M3U or player_api, catalog (movies/series/live), HDHomeRun emulator, XMLTV, stream gateway, optional VODFS mount, materializer (cache, direct file, HLS), config from env, health check, Plex DB registration, provider probe. Subcommands: run, index, serve, mount, probe.
- **Learnings from k3s IPTV, HLS smoketest, config/gateway/VODFS and scripts** — Document k3s IPTV stack (Threadfin, M3U server, Plex EPG), what we reuse (player_api first, multi-host, EPG-linked, smoketest), and optional future work (Plex API DVR, 480-channel split, EPG prune). Add systemd example and LEARNINGS-FROM-K3S-IPTV.md.

### Template and agentic workflow

- **Language-agnostic template** — Any language, not just Go.
- **Harden .gitignore for reusable Go template.**
- **Strip to generic agentic Go template** — Remove iptv-tunerr, k3s, all project examples.
- **Template: decision log, definition of done, dangerous ops, repro-first, runbook, scope guard, repo orientation, link check.**
- **Add performance & resource-respect skill, Git-first workflow skill.**
- **Add curly-quotes/special-chars loop + copy/paste-safe doc policy.**
- **Template: agentic repo v4** — Memory bank, Diátaxis docs, CI, work breakdown.

### Initial commits

- **Merge GitLab initial repo with iptv-tunerr.**
- **Initial commit: iptv-tunerr Live TV/VOD catalog and HDHomeRun tuner for Plex.**

---

## Versioning

Currently no semantic version tags; releases are identified by commit. When tagging releases, use [Semantic Versioning](https://semver.org/) (e.g. `v0.1.0`).
