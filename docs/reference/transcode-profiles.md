---
id: transcode-profiles
type: reference
status: stable
tags: [transcode, ffmpeg, gateway, hdhomerun]
---

# Transcode profiles (MPEG-TS relay)

IPTV Tunerr’s gateway can **transcode** live HLS/MPEG-TS to a Plex-friendly **MPEG-TS** output using ffmpeg. Profiles are **names** that select bitrate, resolution caps, and audio codec policy (`internal/tuner/gateway_profiles.go`).

**MPEG-TS** is the default output for Plex/HDHR compatibility. **Fragmented MP4** is optional for ffmpeg HLS paths: add **`?mux=fmp4`** on `/stream/…` when **transcoding** is active (experimental). **HLS segment packaging** (multi-file `.m3u8` + `.ts` segments from Tunerr) is not implemented—use TS or fMP4 pass-through from ffmpeg.

## Canonical profile names

| Name | Internal id | Typical use |
|------|-------------|-------------|
| Default | `default` | Balanced x264 + AAC when transcoding |
| Plex Web–safe | `plexsafe` | MP3 audio, conservative video — good for Plex Web DASH |
| AAC CFR | `aaccfr` | Baseline-ish H.264 + AAC CFR |
| Video-only (fast) | `videoonlyfast` | Video transcode, no audio |
| Low bitrate | `lowbitrate` | Smaller video + AAC |
| DASH fast | `dashfast` | Tuned for faster Plex Web DASH startup |
| PMS xcode (diagnostic) | `pmsxcode` | MPEG-2 + MP2 — forces transcode off Plex’s copy path for debugging |

Aliases (e.g. `plex-safe`, `aac`, `video`) are accepted; see `normalizeProfileName` in code.

## HDHomeRun / SiliconDust–style aliases

Hardware tuners and Lineup-style apps often expose preset labels. Tunerr maps them to the closest **ffmpeg TS profile** above (Lineup parity **LP-010**):

| Alias | Maps to |
|-------|---------|
| `native`, `heavy`, `max`, `super` | `default` |
| `internet`, `internet720`, `internet1080`, `hd` | `dashfast` |
| `internet240`, `internet360`, `internet480` | `aaccfr` |
| `mobile`, `cell`, `light` | `lowbitrate` |

Hyphens and other punctuation are ignored for these hardware-style names (e.g. `Internet-1080` and `internet1080` both map to `dashfast`).

Your panel may use different spellings; extend mappings via code or **per-channel profile overrides** (`IPTV_TUNERR_PROFILE_OVERRIDES_FILE` in [cli-and-env-reference](cli-and-env-reference.md)).

## Selecting a profile

- **Query string:** `GET /stream/<id>?profile=<name>` — forces adaptation and transcode for known profiles (e.g. `?profile=internet360` → `aaccfr`).
- **Environment:** `IPTV_TUNERR_PROFILE`, `IPTV_TUNERR_PLEX_SAFE`, profile overrides file.
- **Autopilot:** remembered per `dna_id` + client class when enabled.

## See also

- [cli-and-env-reference](cli-and-env-reference.md) — env vars and stream behavior.
- [EPIC-lineup-parity](../epics/EPIC-lineup-parity.md) — product roadmap for named HLS/fMP4 profiles.
