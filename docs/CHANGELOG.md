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

- **Channel intelligence foundation**: added `channel-report` plus `/channels/report.json` to score channels by guide confidence, stream resilience, and next-step fixes.
- **EPG match provenance in reports**: when XMLTV is supplied, channel reports now show whether a channel matched by exact `tvg-id`, alias override, normalized-name repair, or not at all.
- **Intelligence-driven lineup recipes**: added `IPTV_TUNERR_LINEUP_RECIPE` with `high_confidence`, `balanced`, `guide_first`, and `resilient` lineup shaping modes.
- **Channel DNA foundation**: live channels now persist a `dna_id` derived from repaired `TVGID` or normalized channel identity inputs, creating a stable identity substrate for future cross-provider intelligence.
- **Autopilot memory foundation**: added optional JSON-backed remembered playback decisions keyed by `dna_id + client_class`, allowing successful stream transcode/profile choices to be reused on later requests.
- **Ghost Hunter foundation**: added `ghost-hunter` plus `/plex/ghost-report.json` to observe visible Plex Live TV sessions, classify stalls with reaper heuristics, and optionally stop stale visible transcode sessions.
- **Provider behavior profile foundation**: added `/provider/profile.json` to expose learned effective tuner cap, recent upstream concurrency-limit signals, Cloudflare-abuse hits, and current auth-context forwarding posture.
- **Provider autotune foundation**: when `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` is not explicitly set, Tunerr can now auto-arm ffmpeg HLS reconnect after it has actually observed HLS playlist/segment instability at runtime.
- **Guide highlights foundation**: added `/guide/highlights.json`, which repackages the cached merged guide into `current`, `starting_soon`, `sports_now`, and `movies_starting_soon` lanes.
- **Ghost Hunter escalation**: when Plex exposes zero visible live sessions, Ghost Hunter now flags the hidden-grab pattern explicitly and returns the guarded recovery helper command and runbook path.
- **Catch-up capsule preview foundation**: added `/guide/capsules.json`, which turns real guide rows into near-live capsule candidates with lane, publish, and expiry metadata for future library publishing.
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
