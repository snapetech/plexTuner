# Repo map (navigation)

First place to look before editing. Keeps agents from thrashing.

**This folder is the IPTV Tunerr project.** In this workspace, `origin` is the authoritative remote used for pushes. Older notes about a separate `plex` remote may still appear in historical docs/logs; check `git remote -v` before assuming it exists.

## Remotes (do not cross-push)

| Remote    | Repo         | Host   | Use from this folder      |
|-----------|--------------|--------|----------------------------|
| **origin** | iptvTunerr    | GitHub | ✓ Push IPTV Tunerr here    |
| **github** | repoTemplate | GitHub | ✗ Do not push from here   |
| **template** | repoTemplate | GitLab | ✗ Do not push from here   |

Normal push path from this checkout: `git push origin main`. Never `git push github` or `git push template` from this folder.

## Main entrypoints

| Path | Purpose |
|------|--------|
| **`cmd/iptv-tunerr/`** | CLI entrypoint and command handlers for run/serve/index/supervise, reports, registration, and catch-up publishing. |
| **`internal/indexer/`** | M3U stream parsing, player_api (auth, live, VOD, series with parallel fetch). |
| **`internal/catalog/`** | Movie/Series/LiveChannel types; Save (snapshot then encode), Load. |
| **`internal/tuner/`** | HDHR endpoints, stream gateway, XMLTV/guide pipeline, Autopilot, Ghost Hunter, provider profile, catch-up publishing. |
| **`internal/epgstore/`** | Optional SQLite EPG file (`IPTV_TUNERR_EPG_SQLITE_PATH`): migrations, `SyncMergedGuideXML` (optional retain-past prune), max-stop queries; `/guide/epg-store.json`. |
| **HDHR hardware EPG** | Optional `IPTV_TUNERR_HDHR_GUIDE_URL` merges device `guide.xml` in `internal/tuner/epg_pipeline.go` ([ADR 0004](../docs/adr/0004-hdhr-guide-epg-merge.md)). |
| **`internal/channelreport/`** | Channel intelligence scoring and report building. |
| **`internal/channeldna/`** | Stable per-channel identity (`dna_id`) and grouping/report surfaces. |
| **`internal/emby/`** | Emby/Jellyfin tuner registration plus catch-up library registration helpers. |
| **`internal/vodfs/`** | FUSE: root, Movies/TV dirs, virtual files (NodeOpener, keep FD). |
| **`AGENTS.md`** | Agent instructions; **`memory-bank/`** = state + process. |
| **`docs/index.md`** | Doc map (Diátaxis). |

## Single binary (supervisor vs oracle)

**There is only one application:** `iptv-tunerr` (one binary, one build). All modes are subcommands of that binary:

- `run`, `serve`, `index` — single tuner or catalog refresh
- `supervise` — read a JSON config and spawn N child processes (each child is the same binary, e.g. `iptv-tunerr run -addr=:5004 ...`)
- `plex-epg-oracle` — CLI to probe Plex HDHR wizard/channelmap and write reports (one-shot or cron)
- `probe`, `mount`, `vod-split`, `plex-vod-register`, `epg-link-report` — core ops subcommands
- `channel-report`, `channel-dna-report`, `ghost-hunter` — intelligence/diagnostic subcommands
- `catchup-capsules`, `catchup-publish` — guide-derived publishing subcommands

**Single-pod consolidation (done):** Main and oracle instances run in **one** supervisor pod. The main supervisor config (ConfigMap `iptvtunerr-supervisor-config`) includes both the main instances (hdhr-main, categories, …) and the oracle-cap instances (hdhrcap100…hdhrcap600). Service `iptvtunerr-oracle-hdhr` selects `app=iptvtunerr-supervisor` and exposes ports 5201–5206. There is no separate `iptvtunerr-oracle-supervisor` deployment. Oracle instance definitions for merging into a generated config: `k8s/oracle-instances.json`. Windows/macOS: one `go build`; no extra binaries.

**Category DVR feeds (dvr-*.m3u):** Category instances (bcastus, newsus, generalent, …) use M3U URLs like `http://iptv-m3u-server.plex.svc/dvr-bcastus.m3u`. Those files are produced by **iptv-m3u-server** (split step) in the sibling `k3s/plex` repo. They must emit **all** stream URLs per channel (not just one), or after `IPTV_TUNERR_STRIP_STREAM_HOSTS` every channel is dropped and guides show "no live channels available". See known_issues.md (Category DVRs … 0 channels) and runbook §10.

## Key modules

- **`internal/httpclient`** — Shared tuned HTTP client; used by indexer, gateway, materializer, vodfs.
- **`internal/materializer`** — Download: single GET or range (16 MiB, 206 when off>0); env `IPTV_TUNERR_RANGE_DOWNLOAD=1`.
- **`internal/tuner/gateway.go`** — Stream gateway with fallback URLs, provider-cap learning, auth-context forwarding, and autotune hooks.
- **`internal/tuner/xmltv.go` + `internal/tuner/epg_pipeline.go`** — Layered guide builder: provider XMLTV, external XMLTV, placeholder fallback, highlights, capsules.
- **`internal/channelreport`** — Channel scoring, guide confidence, resilience summaries.
- **`internal/channeldna`** — Stable identity layer for merged-provider channels.
- **`internal/plex` / `internal/emby`** — DVR/tuner registration and media-server integration flows.
- **`internal/probe`** — Lineup (lineup.json), discovery (device.xml).

## No-go zones

- **`.env`** — Never commit; never log or echo. Credentials live only in env.
- **Generated/vendor** — Don't edit unless the task explicitly requires it.
- **Weakening tests** — Don't "fix" by loosening assertions; fix root cause.

## Verification and QA

- **`scripts/verify`** — Full check: format (gofmt) → vet → test → build. Fail fast, same as CI.
- **`scripts/quick-check.sh`** — Tests only; use for short feedback when iterating.
- **Troubleshooting:** [docs/runbooks/iptvtunerr-troubleshooting.md](docs/runbooks/iptvtunerr-troubleshooting.md) — fail-fast checklist, probe, logs, common failures.
- CI runs only `scripts/verify`.
