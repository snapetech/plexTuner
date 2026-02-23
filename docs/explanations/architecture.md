---
id: architecture
type: explanation
status: stable
tags: [architecture, design, vodfs, plex, components]
---

# Plex IPTV Tuner — Design & architecture

Full design for an IPTV Tuner that gives Plex both Live TV (tuner-style) and VOD as real library paths.

---

## Approach: VOD as virtual filesystem

With **VOD Approach #2 (virtual filesystem)**, the split is:

- **Live TV** stays "tuner-style": HDHomeRun-ish HTTP endpoints + XMLTV guide so Plex Live TV/DVR works as usual.
- **VOD** becomes **real-looking files** (Movies/TV libraries) via a **FUSE mount**, backed by provider streams and a local cache.

Result: Plex gets the UX you want (real Movies/Series libraries, metadata agents, scanning) while the source remains IPTV.

---

## What we can infer about Strong8k (enough to design around)

- They support **M3U + EPG + Xtream Codes–style login**. Their guide walks through extracting URL/username/password from an M3U pattern and notes the player UI separates **Live TV / Movies / Series**. [^strong8k]
- Community reports suggest some setups toggle "output format" between **MPEG-TS and HLS**, consistent with Xtream-based services. [^reddit]
- Xtream-style APIs often expose **categories and items by type (Live/Movie/Series)** via JSON, not only M3U. [^xtream]

So we **auto-detect** per asset whether the stream is TS, HLS, direct MP4, etc.

---

# 1) Components

## A) HDHR Tuner Emulator (Live TV)

Implements the endpoints Plex expects from a tuner:

- `GET /discover.json`
- `GET /lineup_status.json`
- `GET /lineup.json`

HDHomeRun's HTTP API documents `lineup.json` and related endpoints. [^hdhr]  
Plex hits `discover.json` and `lineup_status.json` and uses fields such as `BaseURL`, `LineupURL`, `TunerCount`, etc. [^plex-forum]

## B) XMLTV Service (Live guide)

- Serve `GET /guide.xml` (XMLTV).
- Plex supports XMLTV guide import for Live TV & DVR. [^plex-xmltv]

## C) Stream Gateway (bytes path for Live + optional VOD fallback)

- Header injection / cookies / referer / User-Agent
- HLS playlist rewrite + segment proxy (if needed)
- Token refresh hooks
- Concurrency control (tuner semantics)

## D) VOD Catalog Indexer

Ingests the provider's VOD listing (M3U sections, Xtream-style JSON, etc.) and normalizes into:

- **Movies:** title, year, IDs, artwork URLs
- **Series → Seasons → Episodes:** SxxEyy, airdate, etc.
- Stable IDs

## E) VODFS (FUSE virtual filesystem)

Mounts a directory tree that Plex can add as normal library paths:

- `/vodfs/Movies/...`
- `/vodfs/TV/...`

Plex scans folders/subfolders for new media [^plex-scan] and expects a mounted path/share that stays available. [^plex-mount]

## F) Materializer (what makes VODFS actually work)

Plex is **byte-range and seek-heavy**. A "pure streaming concatenation" FUSE is painful when VOD is HLS segments.

The materializer makes VODFS **file-semantic** by mapping each "file" to a **real cached file** on disk:

- If upstream is a direct file (e.g. MP4/MKV over HTTP with Range): **proxy-with-cache**.
- If upstream is HLS/segments: **remux (copy, no transcode)** into a local MP4/MKV cache file, then serve byte ranges from local disk.

That keeps **CPU low** (no transcoding), makes seeking reliable, and turns throughput into "read from disk."

---

# 2) What Plex sees — naming rules to implement

## Movies (Plex naming)

Paths like:

```
/vodfs/Movies/MovieName (Year)/MovieName (Year).mp4
```

[^plex-movies]

## TV (Plex naming)

Paths like:

```
/vodfs/TV/Show Name (Year)/Season 01/Show Name (Year) - s01e01 - Episode Title.mp4
```

[^plex-tv]

Get naming right and Plex's normal metadata agents handle the rest.

---

# 3) The VODFS contract (what we must guarantee)

VODFS must behave like a normal filesystem so Plex is happy.

### Must-haves

- **Fast directory listing** — don't block on upstream calls.
- **Stable inodes/paths** — files don't "rename" on every refresh.
- **Correct `stat()`:** `size`, `mtime`.
- **Correct `read(offset, length)`** — Plex seeks by offset.

### The "size problem" and the solution

For HLS VOD you often **can't cheaply know** final byte size without doing work.

So: **only present a file as "ready" once it has a known size** (materialized or at least indexed).

Practical pattern:

1. File appears with a **`.partial`** suffix while materializing.
2. When the cache file is complete (or "complete enough" with a stable size strategy), **rename to final `.mp4`**.
3. Plex scans only final files; partials are ignored or live outside the library root.

---

# 4) Auto-detect stream type

We don't know upstream format in advance. For each asset URL, the gateway/materializer does a cheap probe:

1. **HEAD** or small **GET**:
   - Body starts with `#EXTM3U` or `#EXT-X-...` → **HLS playlist**
   - `Content-Type: video/MP2T` or first bytes look like MPEG-TS sync (`0x47` cadence) → **TS**
   - `Content-Type: video/mp4` and Range works → **direct file mode**

2. **Choose pipeline:**
   - **Direct file:** stream + cache (range passthrough).
   - **HLS:** fetch playlist → download segments → **remux-copy** to local cache → serve from local file.
   - **TS:** for live, treat as live stream; for VOD TS, remux-copy to MP4 cache.

(Live can support both TS and HLS; VOD should always end up as a local file for sane Plex behavior.)

---

# 5) Deployment — low-resource and easy

## Default install (easy)

**Docker Compose** with:

- `tuner` (emulator + XMLTV + UI + indexer)
- `gateway`
- `vodfs` (FUSE) + `cache` volume

User steps:

1. Run compose.
2. Add tuner in Plex Live TV (point to tuner IP).
3. Add XMLTV URL. [^plex-xmltv]
4. Add Plex Movies/TV libraries pointing to `/vodfs/Movies` and `/vodfs/TV`.

## Kubernetes reality check (FUSE)

FUSE needs `/dev/fuse` and elevated caps. In k8s that usually means:

- Privileged pod, or
- Run VODFS on the **host** and mount/export into k8s (e.g. bind mount into Plex container, or NFS export).

If "easy for end users" is a goal, **don't make k8s the default path.**

---

# 6) Phased build plan (VODFS as centerpiece)

### Phase 1 — Skeleton

- VOD indexer builds a catalog.
- VODFS exposes directories/files with correct Plex naming.
- Materializer stubs (placeholders only).

### Phase 2 — Materializer v1 (direct-file VOD)

- Range-aware download + disk cache.
- Serve local cached file through FUSE.

### Phase 3 — HLS VOD support (the big one)

- HLS fetch + segment download.
- Remux-copy to MP4/MKV cache.
- "Partial → final" file lifecycle.

### Phase 4 — Live TV parity

- HDHR endpoints + lineup + tuner semantics. [^hdhr]
- XMLTV generation/import. [^plex-xmltv]
- Gateway rewrite/proxy for Live (TS/HLS).

### Phase 5 — Kitchen sink

- Artwork sidecars.
- Collections (genre folders or Plex collections via metadata).
- Continue-watching friendliness (stable file paths, stable IDs).
- Health dashboards, per-channel failure reasons, token refresh, fallback URLs.

---

# 7) The main trade (VODFS)

We're trading:

- **"Pure streaming pointers"** (simpler)  
  for  
- **"Filesystem semantics"** (harder) — which unlocks the **best Plex UX**.

Keeping it **low-resource**: **no transcoding, only remux-copy**, and only do that work **on demand** and **cached**.

A separate doc can pin down **cache policy + concurrency policy** (e.g. how many materializations at once, how to avoid stampedes when Plex scans 50k VOD entries) for "fast + low overhead" goals.

---

## References

[^strong8k]: [Strong 8K IPTV — installation guide](https://strong8k.com/installation-guide-2/)
[^reddit]: [r/Strong_8K](https://www.reddit.com/r/Strong_8K/)
[^xtream]: [Xtream Code API implementation (Fermata)](https://github.com/AndreyPavlenko/Fermata/discussions/434)
[^hdhr]: [HDHomeRun HTTP API](https://info.hdhomerun.com/info/http_api)
[^plex-forum]: [Plex Forum — PMS can't find HDHomeRun](https://forums.plex.tv/t/pms-cant-find-hdhomerun/676047)
[^plex-xmltv]: [Plex Support — Using XMLTV for guide data](https://support.plex.tv/articles/using-an-xmltv-guide/)
[^plex-scan]: [Plex Support — Scanning vs Refreshing a Library](https://support.plex.tv/articles/200289306-scanning-vs-refreshing-a-library/)
[^plex-mount]: [Plex Support — Mounting Network Resources](https://support.plex.tv/articles/201122318-mounting-network-resources/)
[^plex-movies]: [Plex Support — Naming and organizing your Movie files](https://support.plex.tv/articles/naming-and-organizing-your-movie-media-files/)
[^plex-tv]: [Plex Support — Naming and Organizing Your TV Show files](https://support.plex.tv/articles/naming-and-organizing-your-tv-show-files/)

See also
--------
- [Reference: implementation stories](../reference/implementation-stories.md)
- [Glossary](../glossary.md)

Related ADRs
------------
- *(none yet)*

Related runbooks
----------------
- *(none yet)*
