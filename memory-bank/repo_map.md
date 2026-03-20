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
| **`cmd/iptv-tunerr/`** | CLI entrypoint and command handlers for run/serve/index/supervise, reports, registration, and catch-up publishing. **`main.go`** dispatches via **`cmd_registry.go`**; shared helpers **`cmd_util.go`**. Catalog **`index`** path: **`cmd_catalog.go`** (**tvg-id** dedupe, strip hosts, free/HDHR merge) — see **`docs/reference/lineup-epg-hygiene.md`**. |
| **`internal/indexer/`** | M3U stream parsing, player_api (auth, live, VOD, series with parallel fetch). Optional **smoketest** disk cache (**`smoketest_cache.go`**, **`IPTV_TUNERR_SMOKETEST_CACHE_FILE`**, **`IPTV_TUNERR_SMOKETEST_CACHE_TTL`**) skips fresh re-probes on **`index`**. |
| **`internal/catalog/`** | Movie/Series/LiveChannel types; Save (snapshot then encode), Load. **`ReplaceWithLive`** sorts **`live_channels`** by **`channel_id`** for stable on-disk order (**HR-006**). |
| **`internal/tuner/`** | HDHR endpoints, stream gateway, XMLTV/guide pipeline, Autopilot, Ghost Hunter, provider profile, catch-up publishing. |
| **`internal/webui/`** | Dedicated operator dashboard on port `48879` (`0xBEEF` by default); reverse-proxies tuner JSON/debug endpoints under `/api/*` and now drives safe operator actions/workflows on top of those surfaces. |
| **`internal/epgstore/`** | Optional SQLite EPG file (`IPTV_TUNERR_EPG_SQLITE_PATH`): migrations, `SyncMergedGuideXML` (optional retain-past prune), max-stop queries; `/guide/epg-store.json`. |
| **HDHR hardware EPG** | Optional `IPTV_TUNERR_HDHR_GUIDE_URL` merges device `guide.xml` in `internal/tuner/epg_pipeline.go` ([ADR 0004](../docs/adr/0004-hdhr-guide-epg-merge.md)). |
| **EPG SQLite** | `internal/epgstore/` — optional `VACUUM`, **max file bytes** (`EnforceMaxDBBytes`); `/guide/epg-store.json`. |
| **HDHR lineup import** | `IPTV_TUNERR_HDHR_LINEUP_URL` at **index** → `cmd_catalog.go` + `hdhomerun.LiveChannelsFromLineupDoc`. **Physical HDHR:** `iptv-tunerr hdhr-scan` + `hdhomerun.DiscoverLAN` / HTTP **`discover.json`** (see **LP-001** in [EPIC-lineup-parity](../docs/epics/EPIC-lineup-parity.md)). |
| **`internal/channelreport/`** | Channel intelligence scoring and report building. |
| **`internal/channeldna/`** | Stable per-channel identity (`dna_id`) and grouping/report surfaces. |
| **`internal/emby/`** | Emby/Jellyfin tuner registration plus catch-up library registration helpers. |
| **`internal/vodfs/`** | FUSE: root, Movies/TV dirs, virtual files (NodeOpener, keep FD). |
| **`AGENTS.md`** | Agent instructions; **`memory-bank/`** = state + process. |
| **`docs/index.md`** | Doc map (Diátaxis). |
| **`docs/explanations/project-backlog.md`** | Open-work index (epics + **opportunities** + **known_issues** + gaps + limits). |
| **`docs/explanations/architecture.md`** | Layered overview: core runtime vs intelligence vs publishing; **`cmd_*`** / **`gateway_*`** entry pointers. |

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

- **`internal/httpclient`** — Shared tuned HTTP client; used by indexer, gateway, materializer, vodfs, **`hdhomerun`**, **`plex`** (DVR/library API), **`emby`** registration, **`provider`** probe, **`tuner/epg_pipeline`** (**`httpClientOrDefault`**), **`health`**, **`probe`** (nil client). Idle pool env: **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST`** (default 16), **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS`** (100), **`IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC`** (90). See **`docs/reference/plex-livetv-http-tuning.md`** (**HR-010**).
- **`internal/materializer`** — Download: single GET or range (16 MiB, 206 when off>0); env `IPTV_TUNERR_RANGE_DOWNLOAD=1`.
- **`internal/tuner/gateway.go`** — **`Gateway`** struct + request context keys. **`gateway_servehttp.go`** — **`ServeHTTP`**, tuner acquire/release, call into upstream walk, aggregate failure responses. **`gateway_stream_upstream.go`** — **`walkStreamUpstreams`** (URL loop, DASH/HLS/raw dispatch). **`gateway_upstream_cf.go`** — **`tryRecoverCFUpstream`** (UA cycle + CF bootstrap). **`gateway_mux_ratelimit.go`** — mux segment per-IP rate limit + outcome accounting. **`gateway_profiles.go`** — ffmpeg transcode presets, HDHR-style alias mapping, optional **`IPTV_TUNERR_STREAM_PROFILES_FILE`** named profile matrix (**LP-010**). **`gateway_adapt.go`** / **`gateway_adapt_sticky.go`** — Plex client adaptation + **HR-004** session-scoped WebSafe sticky after hard failures. **`gateway_relay.go`** / **`gateway_stream_helpers.go`** — WebSafe ffmpeg prefetch + **HR-001** IDR/AAC sliding window + **`release=`** log. **HR-003** / **HR-002** tier-1 + Plex Web regression template: **`docs/reference/plex-client-compatibility-matrix.md`**.
- **`internal/tuner/gateway_hls.go`** — HLS helpers: playlist rewrite, **`serveNativeMuxTarget`** ( **`?mux=hls`** + shared **`seg=`** with DASH). **`internal/tuner/gateway_dash.go`** — experimental **MPD** rewrite for **`?mux=dash`**. **`internal/tuner/gateway_dash_expand.go`** — optional **`SegmentTemplate`** → **`SegmentList`** expansion (**`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE`**). **`prometheus_mux.go`** — **`iptv_tunerr_mux_seg_outcomes_total`** when **`IPTV_TUNERR_METRICS_ENABLE`**. See **`docs/reference/hls-mux-toolkit.md`**.
- **`internal/tuner/xmltv.go` + `internal/tuner/epg_pipeline.go`** — Layered guide builder: provider XMLTV (optional **`IPTV_TUNERR_PROVIDER_EPG_DISK_CACHE`** + HTTP 304), external XMLTV, placeholder fallback, highlights, capsules. **`XMLTV`** serves **`/guide.xml`** from a **merged-guide TTL cache** (**`cachedXML`**, **`IPTV_TUNERR_XMLTV_CACHE_TTL`**); see **`TestXMLTV_cacheHit`**.
- **`internal/webui/webui.go`** — Dedicated dashboard listener + `/api/*` reverse proxy; main page is embedded from `internal/webui/index.html`.
- **`internal/tuner/server.go` + `internal/tuner/operator_ui.go`** — Operator/report JSON surfaces plus safe action/workflow endpoints (`/ops/actions/*`, `/ops/workflows/*`) guarded by the localhost/LAN UI policy.
- **`internal/channelreport`** — Channel scoring, guide confidence, resilience summaries.
- **`internal/channeldna`** — Stable identity layer for merged-provider channels.
- **`internal/plex` / `internal/emby`** — DVR/tuner registration and media-server integration flows.
- **`internal/probe`** — Lineup (lineup.json), discovery (device.xml).

## No-go zones

- **`.env`** — Never commit; never log or echo. Credentials live only in env.
- **Generated/vendor** — Don't edit unless the task explicitly requires it.
- **Weakening tests** — Don't "fix" by loosening assertions; fix root cause.

## Verification and QA

- **`scripts/verify`** — Full check: format (gofmt) → vet → **`bash -n` / `py_compile` on `scripts/*.sh` + `scripts/*.py`** → test → build. Fail fast, same as CI.
- **`scripts/quick-check.sh`** — Tests only; use for short feedback when iterating.
- **Troubleshooting:** [docs/runbooks/iptvtunerr-troubleshooting.md](docs/runbooks/iptvtunerr-troubleshooting.md) — fail-fast checklist, probe (**§4** → [interpreting-probe-results.md](docs/how-to/interpreting-probe-results.md)), logs, common failures; **§7** live-race → [how-to/live-race-harness.md](docs/how-to/live-race-harness.md); **§9** stream-compare → [how-to/stream-compare-harness.md](docs/how-to/stream-compare-harness.md); **§10** two-stream collapse → [how-to/multi-stream-harness.md](docs/how-to/multi-stream-harness.md). **`scripts/harness-index.py`** — newest **`.diag/*`** harness runs.
- CI runs only `scripts/verify`.
