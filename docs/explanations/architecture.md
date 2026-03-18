---
id: architecture
type: explanation
status: stable
tags: [explanations, architecture, design]
---

# Architecture

How IPTV Tunerr is structured and how the pieces fit together.

---

## Overview

IPTV Tunerr sits between an IPTV provider and a media server (Plex, Emby, Jellyfin). It translates the provider's protocols (Xtream API, M3U playlists) into the HDHomeRun tuner interface that media servers natively understand.

```
IPTV Provider
  (Xtream API / M3U)
        │
        ▼
  ┌─────────────────────────────────────┐
  │          IPTV Tunerr                │
  │                                     │
  │  Indexer → Catalog → Tuner Server  │
  │                │                    │
  │           Stream Gateway            │
  └─────────────────────────────────────┘
        │
        ▼
  Media Server
  (Plex / Emby / Jellyfin)
```

---

## Data flow

### 1. Indexing (catalog build)

`iptv-tunerr index` (or the index phase of `run`) fetches from the provider:

- **Xtream API** (`player_api.php`): structured live channels, VOD movies, series, EPG data
- **M3U** (`get.php` or a direct URL): channel list with EXTINF attributes

The indexer normalizes this into a **catalog** (`catalog.json`): a unified JSON document containing `live_channels`, `movies`, and `series`. Each channel entry includes all available stream URLs (for failover), the `tvg-id` (EPG link), group, logo, and codec hints.

Provider failover runs at index time: if multiple hosts are configured (`IPTV_TUNERR_PROVIDER_URLS`), the indexer probes each one and uses the first that responds successfully. Cloudflare-proxied endpoints can be filtered out (`IPTV_TUNERR_STRIP_STREAM_HOSTS`).

### 2. Serving (tuner runtime)

`iptv-tunerr serve` loads the catalog and starts an HTTP server that emulates an HDHomeRun tuner:

| Endpoint | What it returns |
|----------|-----------------|
| `GET /discover.json` | Device identity (name, device ID, lineup URL) |
| `GET /lineup_status.json` | Lineup scan status |
| `GET /lineup.json` | Channel list with guide numbers and stream URLs |
| `GET /guide.xml` | XMLTV EPG (placeholder or remapped from external source) |
| `GET /stream/{id}` | Proxied or transcoded live stream |
| `GET /live.m3u` | M3U export for VLC or other players |
| `GET /healthz` | Health check (channel count, last refresh) |

### 3. Stream gateway

When Plex requests `/stream/{id}`, the gateway:

1. Looks up the channel in the catalog; tries stream URLs in ranked order
2. Optionally transcodes via ffmpeg (`IPTV_TUNERR_STREAM_TRANSCODE`)
3. Applies startup race hardening if configured (bootstrap TS, PAT+PMT keepalive, startup gate)
4. Streams MPEG-TS data to the client

If the primary URL fails, it falls back to the next URL automatically. Cloudflare abuse-page detection aborts immediately rather than passing garbage bytes to Plex.

### 4. Guide handling

`/guide.xml` is served in XMLTV format. Two modes:

- **Placeholder (default):** A minimal XMLTV document listing channel stubs with no programme data. Plex can still discover channels; the guide shows as empty.
- **External XMLTV:** If `IPTV_TUNERR_XMLTV_URL` is set, the app fetches the upstream XMLTV, filters it to channels present in the catalog (by `tvg-id`), remaps channel IDs to local guide numbers, optionally normalizes language/script, and caches the result. This is what Plex uses to populate guide listings.

---

## Process modes

### Single instance

One `iptv-tunerr` process on one port. Suitable for a single HDHomeRun-style tuner in Plex.

```
iptv-tunerr run -addr :5004
```

### Supervisor (multi-instance)

`iptv-tunerr supervise` spawns multiple child `iptv-tunerr run` processes from a JSON config. Each child runs independently on its own port with its own provider, lineup, guide, and device identity.

Used for:
- **Category DVR fleets**: one child per content category (sports, news, broadcast US, etc.), each registered as a separate DVR in Plex
- **Combined HDHR + injection**: one child serves the wizard-compatible HDHR lane; others serve injected category DVRs

The supervisor manages process lifecycle (restart on crash, startup delays) and optionally runs a top-level Emby/Jellyfin watchdog.

```
supervise
 ├─ child: hdhr-main  (:5004)  — wizard-compatible, capped at 480
 ├─ child: bcastus    (:5101)  — broadcast US category
 ├─ child: newsus     (:5102)  — news category
 └─ child: sports     (:5103)  — sports category
```

---

## Internal package structure

| Package | Responsibility |
|---------|---------------|
| `cmd/iptv-tunerr` | CLI dispatcher; wires together all packages |
| `internal/config` | Environment variable parsing and validation |
| `internal/indexer` | M3U and Xtream API fetching and parsing |
| `internal/catalog` | Catalog data model and JSON persistence |
| `internal/provider` | Multi-host probing, Cloudflare detection, stream URL ranking |
| `internal/tuner` | HDHR HTTP endpoints, XMLTV serving, stream gateway, Plex session reaper, SSDP discovery |
| `internal/supervisor` | Child process lifecycle management |
| `internal/plex` | Programmatic Plex DVR registration (API + SQLite-assisted) |
| `internal/emby` | Emby and Jellyfin registration and watchdog |
| `internal/vodfs` | FUSE filesystem mount for VOD browsing (Linux only) |
| `internal/materializer` | Pluggable stream backends for VOD playback (direct, HLS relay, download cache) |
| `internal/hdhomerun` | Native HDHomeRun UDP/TCP protocol (optional network mode) |
| `internal/epglink` | EPG match reporting (tvg-id / alias / name-based coverage analysis) |
| `internal/health` | Provider health check |
| `internal/cache` | Smoketest probe result persistence |
| `internal/httpclient` | Retry logic, timeouts, user-agent handling |

---

## Key design decisions

**No web UI.** Configuration is entirely via environment variables and CLI flags. This keeps the binary small, makes containerization simple, and eliminates a whole class of auth/state management problems.

**Catalog as the source of truth.** All runtime decisions (lineup, guide, streaming) read from `catalog.json`. Index and serve are decoupled: you can schedule indexing separately from serving, pre-warm a catalog, or inspect it directly.

**HDHomeRun emulation over native API.** By emulating an HDHomeRun device, IPTV Tunerr works with the standard Plex/Emby/Jellyfin wizard with no special plugins or integrations required. The DVR injection path builds on top of this foundation.

**Provider failover at multiple layers.** At index time (choose a working host), at stream time (try next URL on failure), and at the ffmpeg level (HLS reconnect options). This makes the tuner resilient to CDN outages without user intervention.

**Startup race hardening.** Plex's DASH packager has a timing dependency: it needs valid MPEG-TS program structure (PAT+PMT) before the first IDR frame arrives, or it fails silently. The startup gate and PAT+PMT keepalive features address this without requiring changes to Plex.

See also
--------
- [ADR index](../adr/index.md) — decision records for specific choices
- [Glossary](../glossary.md) — term definitions
- [Features](../features.md) — full feature list
