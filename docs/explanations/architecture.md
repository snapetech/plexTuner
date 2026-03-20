---
id: architecture
type: explanation
status: stable
tags: [explanations, architecture, design]
---

# Architecture

How IPTV Tunerr is structured now that it is more than a tuner shim.

---

## Overview

IPTV Tunerr has three active layers:

1. **Core runtime**
   Ingest provider data, build a catalog, serve HDHomeRun-compatible tuner endpoints, proxy streams, and generate `/guide.xml`.
2. **Intelligence layer**
   Repair bad guide IDs, score channels, group duplicates by stable identity, remember working playback choices, and expose provider/session diagnostics.
3. **Publishing and registration layer**
   Register tuners and libraries with Plex, Emby, and Jellyfin; publish near-live catch-up libraries as `.strm + .nfo`.

That means the app is no longer just:

`Indexer -> Catalog -> Tuner Server`

It is closer to:

```
Provider inputs
  (Xtream API / M3U / XMLTV)
        |
        v
Catalog build and repair
  - provider probe/ranking
  - live/VOD/series indexing
  - TVGID repair
  - dedupe/fallback URL merge
        |
        +-----------------------------+
        |                             |
        v                             v
Core tuner runtime              Intelligence surfaces
  - HDHR endpoints                - channel report
  - stream gateway                - Channel DNA
  - guide.xml                     - Ghost Hunter
  - live.m3u                      - provider profile
                                  - lineup recipes
                                  - guide highlights
                                  - catch-up capsules
        |
        v
Registration / publishing
  - Plex DVR + lineup + guide sync
  - Emby/Jellyfin tuner registration
  - catch-up library publishing
```

---

## Layer 1: Core Runtime

This is the original product foundation and it is still the backbone.

### Ingest and catalog build

`iptv-tunerr index` and the index phase of `run` still:
- fetch live, movie, and series data from Xtream or M3U sources
- probe provider hosts and rank them
- merge backup stream URLs
- dedupe duplicate live channels
- write the catalog as the runtime source of truth

Primary code:
- [main.go](../../cmd/iptv-tunerr/main.go) (dispatcher); [cmd_registry.go](../../cmd/iptv-tunerr/cmd_registry.go); handlers such as [cmd_catalog.go](../../cmd/iptv-tunerr/cmd_catalog.go)
- [indexer](../../internal/indexer)
- [catalog](../../internal/catalog)
- [provider](../../internal/provider)

### Tuner runtime

`iptv-tunerr serve` and `run` still expose the HDHomeRun-compatible surface:
- `/discover.json`
- `/lineup.json`
- `/lineup_status.json`
- `/stream/{id}`
- `/guide.xml`
- `/live.m3u`
- `/healthz`

Primary code:
- [server.go](../../internal/tuner/server.go)
- [gateway_servehttp.go](../../internal/tuner/gateway_servehttp.go) (+ [`gateway_*.go`](../../internal/tuner/) — stream, HLS/DASH, adaptation, relay)
- [xmltv.go](../../internal/tuner/xmltv.go)

### Lineup shaping and category fleets

The older lineup/category logic is still active:
- wizard-safe channel caps
- language/drop/exclude filters
- region/profile shaping
- skip/take sharding
- multi-instance supervisor deployments
- category DVR fleets

This matters especially for Plex-heavy multi-DVR setups.

Primary code:
- [server.go](../../internal/tuner/server.go)
- [cmd/iptv-tunerr/](../../cmd/iptv-tunerr/) (`main`, `cmd_*` for flags shaping)
- [supervisor](../../internal/supervisor)

---

## Layer 2: Guide And Intelligence

This is the major post-integration expansion.

### Guide pipeline

The guide path now has a real layered merge:

1. provider XMLTV (`xmltv.php`)
2. external XMLTV
3. placeholder fallback

Provider data wins where present; external XMLTV gap-fills it; placeholder only survives where neither source has programme rows.

Before `LIVE_EPG_ONLY` pruning, IPTV Tunerr can also repair bad or missing `TVGID`s using deterministic matching:
- exact `tvg-id`
- alias override
- normalized exact-name match

Primary code:
- [epg_pipeline.go](../../internal/tuner/epg_pipeline.go)
- [xmltv.go](../../internal/tuner/xmltv.go)
- [epglink.go](../../internal/epglink/epglink.go)

### Channel intelligence

The intelligence layer makes the runtime explain itself:
- `channel-report`
- `/channels/report.json`
- lineup recipes
- guide-confidence scoring
- stream-resilience scoring

Primary code:
- [report.go](../../internal/channelreport/report.go)
- [server.go](../../internal/tuner/server.go)

### Stable channel identity

Channel DNA gives live channels a stable `dna_id` so the system can reason about:
- duplicates across merged providers
- repaired guide matches
- future playback-memory and routing decisions

Primary code:
- [dna.go](../../internal/channeldna/dna.go)
- [report.go](../../internal/channeldna/report.go)

### Runtime learning and diagnostics

Several operator-facing surfaces now sit on top of the runtime:
- **Autopilot memory** for successful playback decisions
- **Ghost Hunter** for stale/hidden-grab investigation
- **Provider profile** for learned caps and instability signals
- **Guide highlights** and **catch-up capsules**

These are layered additions. They do not replace the tuner/gateway/guide core.

Primary code:
- [autopilot.go](../../internal/tuner/autopilot.go)
- [ghost_hunter.go](../../internal/tuner/ghost_hunter.go)
- [gateway.go](../../internal/tuner/gateway.go) (**Gateway** struct); live **`ServeHTTP`**: [gateway_servehttp.go](../../internal/tuner/gateway_servehttp.go); provider profile / caps: [gateway_provider_profile.go](../../internal/tuner/gateway_provider_profile.go)
- [xmltv.go](../../internal/tuner/xmltv.go)

---

## Layer 3: Registration And Publishing

This layer turns the runtime into a full media-server control plane.

### Tuner and DVR registration

The app still supports:
- HDHR wizard flows
- Plex DVR/API/DB registration
- Emby tuner registration
- Jellyfin tuner registration
- watchdog repair loops

Primary code:
- [cmd/iptv-tunerr/](../../cmd/iptv-tunerr/) (registration subcommands)
- [dvr.go](../../internal/plex/dvr.go)
- [emby](../../internal/emby)

### Catch-up publishing

The catch-up path is now a real publishing subsystem:
- derive capsules from guide windows
- export preview/layout JSON
- write `.strm + .nfo`
- write `publish-manifest.json`
- optionally create/reuse and refresh libraries in Plex, Emby, and Jellyfin

Primary code:
- [catchup_publish.go](../../internal/tuner/catchup_publish.go)
- [catchup_capsules_export.go](../../internal/tuner/catchup_capsules_export.go)
- [cmd/iptv-tunerr/](../../cmd/iptv-tunerr/) (`catchup-publish`, `catchup-capsules`, …)

---

## What Is Still Foundational

These older flows are still first-class, not abandoned:
- lineup sorting and shaping
- category DVR lanes
- guide-number offsets
- wizard-safe caps
- supervisor mode
- Plex DVR registration
- EPG link reporting

The newer intelligence work depends on these layers. It did not replace them.

---

## Current design tension

The product spans **ingest**, **tuner/guide runtime**, **intelligence**, and **publishing**, so the primary tension is **keeping seams clear** as features accumulate. The Go runtime remains layered by package; **CLI** concerns are split across **`cmd_registry.go`** and many **`cmd_*.go`** files under **[cmd/iptv-tunerr](../../cmd/iptv-tunerr/)** (no longer a single oversized `main.go`). Ongoing work is incremental: navigation docs (e.g. [repo map](../../memory-bank/repo_map.md)), reference pages, and tests around the hottest paths.

See also
--------
- [Features](../features.md)
- [CLI and env reference](../reference/cli-and-env-reference.md)
- [Plex DVR lifecycle and API](../reference/plex-dvr-lifecycle-and-api.md)
- [Live TV Intelligence epic](../epics/EPIC-live-tv-intelligence.md)
