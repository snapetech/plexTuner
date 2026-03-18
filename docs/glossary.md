---
id: glossary
type: reference
status: stable
tags: [reference, glossary, vocabulary]
---

# Glossary

Key terms used across the IPTV Tunerr docs.

| Term | Definition |
|------|------------|
| **catalog** | The local JSON file (`catalog.json`) that indexes all live channels, VOD movies, and series fetched from the provider. Built by `iptv-tunerr index`; read by `serve`/`run`. |
| **channelmap** | Plex's internal mapping of guide numbers to DVR channels. After DVR injection, Plex must activate a channelmap before guide data is associated with channels. |
| **DVR injection** | Programmatic creation of Plex DVR device and provider rows via Plex's internal API or database, bypassing the wizard UI. Removes the 480-channel limit and enables repeatable headless setup. |
| **EPG** | Electronic Programme Guide. Schedule/metadata for what is airing on a channel, typically sourced from XMLTV. |
| **EPG-linked** | A channel that has a `tvg-id` matching an entry in the XMLTV source. Unlinked channels appear in Plex without guide data. |
| **guide number** | The channel number assigned to a live channel in the lineup. Plex uses guide numbers for DVR scheduling and guide display. |
| **guide number offset** | `IPTV_TUNERR_GUIDE_NUMBER_OFFSET` — a per-instance integer added to all guide numbers. Used to prevent guide ID collisions between multiple DVR instances in Plex. |
| **HDHR / HDHomeRun** | A physical network tuner device made by SiliconDust. IPTV Tunerr emulates the HDHomeRun HTTP API so Plex/Emby/Jellyfin see it as a network tuner without needing real hardware. |
| **HDHR network mode** | An optional mode (`IPTV_TUNERR_HDHR_NETWORK_MODE`) that also implements the native HDHomeRun UDP/TCP protocol on port 65001, enabling LAN broadcast discovery by Plex clients. |
| **lineup** | The ordered list of channels returned by `/lineup.json`. Plex reads this to populate its DVR channel list. Capped at 480 channels in wizard mode. |
| **M3U / M3U+** | A playlist format used by IPTV providers. Each entry has an `#EXTINF` metadata line followed by a stream URL. `m3u_plus` adds `tvg-id`, `tvg-name`, `tvg-logo`, and `group-title` attributes. |
| **player_api** | Xtream Codes API endpoint (`player_api.php`) used to fetch live channels, VOD movies, series, and EPG data from Xtream-compatible providers. |
| **SSRF (protection)** | Server-Side Request Forgery protection built into IPTV Tunerr's stream gateway. Rejects stream URLs pointing to private network ranges so the tuner cannot be used as a proxy to internal services. |
| **startup gate** | A built-in buffer in the stream gateway that holds the initial TS bytes until a minimum threshold is met before passing data to Plex. Reduces `dash_init_404` startup race failures. |
| **supervisor mode** | Running multiple `iptv-tunerr` child instances from a single parent process (`iptv-tunerr supervise`). Each child has independent args, env, port, and provider config. Used for multi-DVR category deployments. |
| **transcode** | Converting a video stream's codec/container with ffmpeg before passing it to Plex. `IPTV_TUNERR_STREAM_TRANSCODE=auto` uses ffprobe to detect the codec and transcodes only if Plex can't play it natively (e.g. HEVC, VP9). |
| **tvg-id** | The EPG identifier attribute on an M3U channel entry (`tvg-id="..."`). Used to match channels in the M3U/catalog against XMLTV programme data. Channels without a `tvg-id` are unlinked. |
| **VODFS** | VOD Filesystem — a Linux-only FUSE mount that exposes the VOD catalog (movies and series) as browsable directories so Plex can scan and index them as library sections. |
| **websafe** | A streaming mode that applies browser-compatible codec settings (MP3 audio) and startup race hardening (bootstrap TS, PAT+PMT keepalive) for Plex Web/browser clients. |
| **XMLTV** | An XML format for TV programme guide data. IPTV Tunerr can fetch an external XMLTV file, filter it to the live catalog channels, remap channel IDs, and serve it from `/guide.xml`. |
| **Xtream / Xtream Codes** | A popular IPTV panel/API standard. Providers expose `player_api.php` for structured channel/VOD access and `get.php` for M3U playlist generation. |
| **zero-touch** | A setup flow where IPTV Tunerr writes DVR/guide configuration directly to Plex's data directory (via `-register-plex`) so no Plex wizard interaction is required. |

See also
--------
- [Docs index](index.md)
- [CLI and env reference](reference/cli-and-env-reference.md)
- [Features](features.md)
