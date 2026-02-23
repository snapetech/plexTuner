# Implementation stories

Stories mapped from [DESIGN.md](DESIGN.md) phased plan. Implement in order within each phase; Phase 1 is the skeleton.

**Stack:** Go (FUSE via `bazil.org/fuse` or `github.com/hanwen/go-fuse`, stdlib HTTP, low-resource).

---

## Phase 1 — Skeleton

| ID   | Story | Deliverable | Done |
|------|--------|-------------|------|
| P1.1 | **Project skeleton** | Go module, dir layout (`cmd/`, `internal/`), config struct (provider URL, credentials, mount path, cache dir), `go.mod`/deps, optional `docker-compose.yml` placeholder | ☑ |
| P1.2 | **VOD catalog data model** | Types: `Movie`, `Series`, `Season`, `Episode` with stable IDs, title, year, artwork URL, stream URL. In-memory catalog that can be loaded/saved (e.g. JSON). | ☑ |
| P1.3 | **VOD indexer** | Ingest from M3U (parse movie/series sections) and/or Xtream-style API stub (JSON) → normalize into catalog. CLI or library to “refresh catalog”. | ☑ |
| P1.4 | **VODFS (FUSE)** | Mount that exposes `Movies/` and `TV/` from catalog. Plex naming: `Movies/MovieName (Year)/MovieName (Year).mp4`, `TV/Show Name (Year)/Season 01/Show Name - s01e01 - Title.mp4`. Fast dir listing, stable paths; files can be 0-byte stubs or placeholder. | ☑ |
| P1.5 | **Materializer stub** | Interface: e.g. `Materialize(ctx, assetID) (path string, err)`. No-op or placeholder impl that returns “not ready” or a 0-byte path. VODFS calls it; when stub says “no file”, VODFS can still show entry (e.g. 0 size) or hide until materialized. | ☑ |

**Phase 1 exit:** Run indexer → catalog populated; mount VODFS → see Movies/TV tree with correct names; materializer is a stub.

---

## Phase 2 — Materializer v1 (direct-file VOD)

| ID   | Story | Deliverable | Done |
|------|--------|-------------|------|
| P2.1 | **Cache layout** | Cache dir: `cache/vod/<id>.mp4`. Stable paths. | ☑ |
| P2.2 | **Range-aware download** | Direct-file: range requests to cache. | ☑ |
| P2.3 | **Materializer v1** | DirectFile + Cache: download to cache, return path. VODFS serves from cache. | ☑ |
| P2.4 | **VODFS ↔ cache** | Materialize(assetID, streamURL); if path returned, expose size/mtime and read from cache. | ☑ |

---

## Phase 3 — HLS VOD support

| ID   | Story | Deliverable | Done |
|------|--------|-------------|------|
| P3.1 | **Stream type probe** | HEAD/small GET; detect HLS vs TS vs direct MP4. | ☑ |
| P3.2 | **HLS fetch** | ffmpeg -i &lt;hls_url&gt; (ffmpeg does fetch). | ☑ |
| P3.3 | **Remux to MP4** | ffmpeg -c copy -bsf:a aac_adtstoasc -movflags +faststart. | ☑ |
| P3.4 | **Partial → final** | Write .partial; rename to .mp4 on success. Cache materializer does direct + HLS. | ☑ |

---

## Phase 4 — Live TV parity

| ID   | Story | Deliverable | Done |
|------|--------|-------------|------|
| P4.1 | **HDHR emulator** | `GET /discover.json`, `GET /lineup_status.json`, `GET /lineup.json`. | ☑ |
| P4.2 | **XMLTV** | `GET /guide.xml` (minimal XMLTV from channel list). | ☑ |
| P4.3 | **Stream gateway** | `GET /stream/&lt;n&gt;` proxies to provider with auth; tuner concurrency limit. | ☑ |

---

## Phase 5 — Kitchen sink

| ID   | Story | Deliverable | Done |
|------|--------|-------------|------|
| P5.1 | Artwork sidecars / thumbs | ☐ |
| P5.2 | Collections (genre/metadata) | ☐ |
| P5.3 | Continue-watching (stable IDs/paths) | ☐ |
| P5.4 | Health dashboards, token refresh, fallbacks | ☐ |

---

## Cross-cutting

- **Config:** One place (env + file) for provider base URL, user/pass, mount point, cache dir, tuner count.
- **Logging:** Structured or simple; no silent failures.
- **Recurring loops:** When hitting a painful pattern, add to `memory-bank/recurring_loops.md`.
