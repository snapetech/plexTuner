---
id: glossary
type: reference
status: stable
tags: [reference, glossary, vocabulary]
---

# Glossary

Key terms used across the docs. Link to this from other docs to keep vocabulary consistent.

| Term | Definition |
|------|------------|
| **VODFS** | Virtual filesystem (FUSE) that exposes Movies/TV trees so Plex can add them as libraries. Files are backed by provider streams and a local cache; see [VODFS contract](explanations/architecture.md#3-the-vodfs-contract-what-we-must-guarantee). From repo root: `docs/explanations/architecture.md`. |
| **Materializer** | Component that turns a provider stream (direct file or HLS) into a real cached file on disk. Remux-copy only (no transcoding). VODFS serves byte ranges from the cache. |
| **HDHR** | HDHomeRun. Plex expects tuner endpoints: `discover.json`, `lineup_status.json`, `lineup.json`. We emulate this so Plex treats us as an HDHomeRun-style tuner. |
| **XMLTV** | XML format for TV guide data. We serve `GET /guide.xml`; Plex imports it for Live TV/DVR. |
| **FUSE** | Filesystem in Userspace. How we expose VODFS as a mountable directory. |
| **VOD** | Video on demand (movies/series), as opposed to live channels. |
| **Xtream / Xtream Codes** | Provider API style: `player_api.php`, M3U, EPG. Many IPTV providers use it. |
| **M3U** | Playlist format; often used by providers to list live channels and sometimes VOD. |
| **EPG** | Electronic programme guide. We use XMLTV as the EPG for Live TV. |
| **Remux** | Copy streams (video/audio) without re-encoding. We use ffmpeg remux (e.g. HLS → MP4) for cache; no transcoding. |

See also
--------
- [Docs index](index.md)
- [Explanations: architecture](explanations/architecture.md)

Related ADRs
------------
- *(none)*

Related runbooks
----------------
- *(none)*
