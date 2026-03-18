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

- *(none)*

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
