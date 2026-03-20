---
id: epic-lineup-parity
type: explanation
status: draft
tags: [epic, product, hdhomerun, ui, epg, transcoding]
---

# Epic: Lineup-app parity (operator UX + HDHR hardware + media profiles)

**Inspiration:** [am385/lineup](https://github.com/am385/lineup) — a .NET app for **physical HDHomeRun** tuners with Blazor dashboard, TUI, SQLite EPG cache, incremental guide fetch, and ffmpeg **HLS/fMP4** transcoding profiles.

IPTV Tunerr’s mission stays **IPTV/Xtream → HDHR-shaped bridge for Plex/Emby/Jellyfin**. This epic adds optional **operator-grade surfaces** and **hardware-adjacent** paths that Lineup highlights, without replacing Tunerr’s core value.

## North star

- **Hardware path:** Optionally ingest **real SiliconDust HDHomeRun** lineups and EPG (LAN discovery + HTTP/API), alongside existing M3U/Xtream flows, so one binary can serve hybrid deployments.
- **Operator path:** Provide a **web dashboard** (settings, guide preview, health, stream smoke) so fewer tasks require CLI + logs alone.
- **Persistence path:** Move long-lived **EPG/guide** data toward a **SQLite** model (retention, incremental windows, expiry) while keeping current XMLTV outputs correct for media servers.
- **Playback path:** Offer **named transcoding profiles** (e.g. mobile / 720p / bandwidth tiers) producing **HLS and/or fMP4** where today the stack is TS/HLS-relay-centric.

## Non-goals

- Rewriting Tunerr in .NET or abandoning the single Go binary story.
- Matching Lineup feature-for-feature on day one (TUI, duplicate Blazor stack, etc.).
- Promising perfect feature parity with Silicondust firmware or every HDHR model before validation.
- Replacing Plex’s Live TV UI or becoming a full media server.

## Relationship to existing code

| Area | Already in repo | Epic extends |
|------|-----------------|--------------|
| HDHomeRun | `internal/hdhomerun/` — **virtual** discovery/server side for Plex compatibility | **Client** to real devices: discovery, lineup fetch, optional guide from device/API |
| Guide | `internal/tuner/xmltv.go`, `epg_pipeline.go`, caches | SQLite backing, incremental fetch policy, retention |
| Serve | `serve` HTTP, JSON reports | Embedded **web UI** + same APIs |
| Gateway | `internal/tuner/gateway.go`, ffmpeg paths | **Profile** matrix: HLS/fMP4 outputs, named presets |

## Milestones

| Milestone | Done = verifiable outcomes |
|-----------|----------------------------|
| **M1** | Real HDHR on LAN: discover + lineup + at least one EPG pull path documented and tested on reference hardware (e.g. FLEX-class). |
| **M2** | Thin **web UI**: read-only health + guide snippet + links to existing JSON endpoints; auth story documented. |
| **M3** | SQLite guide store: migration plan, schema, background cleanup; XMLTV output byte-stable for Plex. |
| **M4** | Two+ **transcode profiles** with stable names in config/env; HLS or fMP4 path exercised in `scripts/verify` or integration test. |

## Implementation status (repo snapshot)

- **LP-001 (MVP shipped):** `internal/hdhomerun` implements **`DiscoverLAN`** (UDP broadcast), **`ParseDiscoverReply`**, **`FetchDiscoverJSON`**, **`FetchLineupJSON`**, **`FetchGuideXML`**. Operators run **`iptv-tunerr hdhr-scan`** (LAN discovery or **`-addr http://device`**). Further work = polish (multicast subnets, IPv6) and continued **LP-002** merge ADR alignment.
- **LP-010 (partial):** Hardware-style profile aliases + optional **`IPTV_TUNERR_STREAM_PROFILES_FILE`** named matrix (**`base_profile`**, **`transcode`**, **`output_mux`**) in **`internal/tuner/gateway_profiles.go`**; see [transcode-profiles](../reference/transcode-profiles.md). Remaining LP-010/011 = more **ffmpeg HLS/fMP4 packaging** paths if product needs disk segment output beyond Tunerr-native **`?mux=hls|dash`** rewrite.
- **LP-004–LP-006:** Substantial dashboard work lives in **`internal/webui`** + tuner JSON APIs; treat epic rows as iterative hardening, not empty.
- **Not started here (multi-PR):** **LP-007–LP-009** (SQLite schema, incremental fetch, cleanup), **Postgres**, always-on recorder daemon — track in [memory-bank/work_breakdown.md](../../memory-bank/work_breakdown.md) when scheduled.

## Story list

| ID | Story | Acceptance (summary) |
|----|-------|----------------------|
| **LP-001** | HDHR **client** discovery | UDP discovery and/or HTTP probe finds devices; lists `DeviceID`, base URL, tuner count. |
| **LP-002** | HDHR **lineup import** | Pull `lineup.json` (or equivalent) from device; map into catalog or sidecar merge with IPTV sources (design ADR). |
| **LP-003** | HDHR **EPG ingest** | Fetch guide intervals from HDHR/API; normalize into internal guide model; merge priority documented. |
| **LP-004** | Web **shell** | `serve` exposes static dashboard route(s); build embedded via `embed.FS`; no secrets in JS. |
| **LP-005** | Dashboard **health** | Single page summarizing `/health`-class signals + tuner/catalog freshness + last guide refresh. |
| **LP-006** | Dashboard **guide preview** | Read-only grid or channel list from cached guide (pagination OK). |
| **LP-007** | SQLite **schema** | Tables for programmes/channels/metadata; migrations; file path via env. |
| **LP-008** | Guide **incremental fetch** | Safe window computation from SQLite max-airtime (like Lineup’s “avoid redundant API calls”). |
| **LP-009** | SQLite **cleanup** | Expired programme eviction; disk bounds; logging. |
| **LP-010** | **Profile** config model | Named profiles: bitrate caps, resolution, container (HLS/fMP4), codec policy. |
| **LP-011** | ffmpeg **profile execution** | Gateway or sub-handler selects profile; integration test or scripted probe. |
| **LP-012** | Docs + runbook | `docs/how-to/` for hybrid HDHR+IPTV; env reference updates; troubleshooting. |

## Suggested PR plan

| PR | Scope | Stories |
|----|--------|---------|
| **PR-1** | HDHR client MVP + ADR for merge semantics | LP-001, LP-002 (spike), LP-012 (partial) |
| **PR-2** | EPG from HDHR + pipeline integration | LP-003, tests, docs |
| **PR-3** | Web dashboard shell + health | LP-004, LP-005 |
| **PR-4** | Guide preview + auth hardening (if needed) | LP-006 |
| **PR-5** | SQLite schema + migration from current cache | LP-007 |
| **PR-6** | Incremental fetch + cleanup | LP-008, LP-009 |
| **PR-7** | Transcode profiles + ffmpeg wiring | LP-010, LP-011 |
| **PR-8** | Doc sweep + runbook | LP-012 |

## Decision points (defaults if silent)

| Topic | Options | Default |
|-------|---------|---------|
| HDHR + IPTV merge | Separate instances vs merged catalog | **ADR** in PR-1; default lean: **tagged source** so channels don’t collide blindly |
| Web auth | mTLS / token / localhost-only | **localhost-only first**, token optional |
| SQLite | Single file path vs per-instance | **`IPTV_TUNERR_EPG_SQLITE_PATH`**-style single file |
| Profiles | Env list vs YAML | **Env + JSON file** consistent with supervisor patterns |

## Current status

- **PR-1**: **`LP-001`** discovery; **`LP-002`** — **`IPTV_TUNERR_HDHR_LINEUP_URL`** merges device **`lineup.json`** at **index** time (`IPTV_TUNERR_HDHR_LINEUP_ID_PREFIX`); imported rows are source-tagged and no longer dropped just because an IPTV row already uses the same **`tvg_id`**; ADR 0002; [how-to hybrid HDHR+IPTV](../how-to/hybrid-hdhr-iptv.md).
- **PR-2**: **`LP-003`** — `hdhr-scan`, **`IPTV_TUNERR_HDHR_GUIDE_URL`** runtime merge into `/guide.xml` ([ADR 0004](../adr/0004-hdhr-guide-epg-merge.md)); `tvg-id` string match.
- **PR-3**: **`LP-004`/`LP-005`** — `/ui/`, health links, `IPTV_TUNERR_UI_*`, `AppVersion`.
- **PR-4**: **`LP-006`** — `/ui/guide/`, `guide-preview.json`, `GuidePreview`.
- **PR-5**: **`LP-007`** — `internal/epgstore`, [ADR 0003](../adr/0003-epg-sqlite-vs-postgres.md).
- **PR-6**: **`LP-008`/`LP-009`** — SQLite sync, retain/VACUUM/max-bytes, `/guide/epg-store.json`. Full **provider** incremental fetch (not pulling whole `xmltv.php`) is still a future optimization; Tunerr SQLite remains snapshot replace per refresh.
- **PR-7**: **`LP-010`/`LP-011`** — profiles + ffmpeg TS default; optional **`?mux=fmp4`** (transcode). **Update (2026-03):** Tunerr-native **HLS playlist proxy** via **`?mux=hls`** (rewrite + proxy; not ffmpeg segment packaging). *Optional* separate **ffmpeg-generated** multi-file HLS packaging remains future if product needs it.
- **`LP-012`**: [how-to/hybrid-hdhr-iptv.md](../how-to/hybrid-hdhr-iptv.md), env/docs sweep.

## Coordination with other epics

- **[EPIC-live-tv-intelligence.md](EPIC-live-tv-intelligence.md)** — Dashboard should surface channel intelligence and guide-confidence outputs where available (`INT-*`).
- **[memory-bank/work_breakdown.md](../../memory-bank/work_breakdown.md)** — Do not start `LP-*` work until stories are pulled into an active PR plan; keep `HR-*` / `INT-*` / `REC-*` scopes from thrashing.

See also
--------
- [features.md](../features.md)
- [repo map](../../memory-bank/repo_map.md) (`internal/hdhomerun/`, `internal/tuner/`)
- Lineup README (upstream): [github.com/am385/lineup](https://github.com/am385/lineup)
