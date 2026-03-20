---
id: transcode-profiles
type: reference
status: stable
tags: [transcode, ffmpeg, gateway, hdhomerun]
---

# Transcode profiles (MPEG-TS, packaged HLS, fMP4)

IPTV Tunerr’s gateway can **transcode** live HLS/MPEG-TS to a Plex-friendly **MPEG-TS** output using ffmpeg. Profiles are **names** that select bitrate, resolution caps, and audio codec policy (`internal/tuner/gateway_profiles.go`).

**MPEG-TS** is the default output for Plex/HDHR compatibility. **Fragmented MP4** is optional for ffmpeg HLS paths: add **`?mux=fmp4`** on `/stream/…` when **transcoding** is active (experimental). Named profiles can also prefer **ffmpeg-packaged HLS** with **`"output_mux": "hls"`**; that path spins up a short-lived ffmpeg HLS packager, returns a Tunerr-served playlist, and serves packaged segment files back through Tunerr.

**Tunerr-native manifest proxies:** **`?mux=hls`** for **M3U8** upstreams and **`?mux=dash`** (experimental) for **MPD** upstreams on `/stream/…`. Diagnostics: [hls-mux-toolkit](hls-mux-toolkit.md). HLS returns a rewritten **M3U8** whose media lines loop through Tunerr (`?mux=hls&seg=…`); tags with **`URI="…"`** (keys, **`EXT-X-PART`**, maps, variants, etc.) rewrite the same way for **http(s)** targets. DASH rewrites absolute segment **`media=`** / **`initialization=`** and **`BaseURL`** text to **`?mux=dash&seg=…`**. Optional **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`** forces **absolute** Tunerr URLs. See [hls-mux-proxy how-to](../how-to/hls-mux-proxy.md). This native mux is still **proxy + rewrite only**; the separate ffmpeg-packaged-HLS path comes from named profiles, not from explicit **`?mux=hls`**.

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

Your panel may use different spellings; extend mappings via code or **per-channel profile overrides** (`IPTV_TUNERR_PROFILE_OVERRIDES_FILE` in [cli-and-env-reference](cli-and-env-reference.md)). Pair with **`IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`** when you need per-channel **remux vs transcode** (see [plex-livetv-http-tuning](plex-livetv-http-tuning.md)).

## Named profile matrix (optional)

**`IPTV_TUNERR_STREAM_PROFILES_FILE`** is a JSON object of **operator-defined** profile keys. Each entry selects a **built-in** `base_profile` and can override **transcode on/off** and ffmpeg **output mux** (`mpegts`, packaged `hls`, or fragmented `fmp4`) without patching code. This is a small JSON map, not a full codec DSL, and it is useful when panels expect labels Tunerr does not map yet (Lineup parity **LP-010 / LP-011**).

Example:

```json
{
  "ISP-1080p": {
    "base_profile": "dashfast",
    "transcode": true,
    "output_mux": "mpegts",
    "description": "Panel-specific label → dashfast preset"
  },
  "ISP-web": {
    "base_profile": "aaccfr",
    "transcode": true,
    "output_mux": "fmp4"
  },
  "ISP-hls": {
    "base_profile": "dashfast",
    "transcode": true,
    "output_mux": "hls",
    "description": "ffmpeg-packaged HLS playlist + segments served through Tunerr"
  }
}
```

Use **`?profile=ISP-1080p`** or reference those names from **`IPTV_TUNERR_PROFILE_OVERRIDES_FILE`**.

Notes:
- `base_profile` must be one of the built-in profiles or HDHR-style aliases listed above.
- `output_mux` is only a preferred default. An explicit request like `?mux=mpegts` still wins.
- `output_mux: "hls"` is a **profile-selected** ffmpeg packager path. It does **not** change explicit **`?mux=hls`**, which still means Tunerr-native playlist rewrite/proxy.
- Packaged HLS uses short-lived session URLs under `/stream/<id>?mux=hlspkg&sid=...&file=...`; Tunerr serves the playlist and segment files while ffmpeg keeps packaging in the background.
- `IPTV_TUNERR_PROFILE`, `IPTV_TUNERR_PROFILE_OVERRIDES_FILE`, and `?profile=<name>` can all reference these custom names once loaded.
- Runtime snapshot echoes the file path under `tuner.stream_profiles_file`.

## Selecting a profile

- **Query string:** `GET /stream/<id>?profile=<name>` — forces adaptation using built-in or loaded named profiles (e.g. `?profile=internet360` → `aaccfr`, or `?profile=ISP-web` from the matrix file).
- **Environment:** `IPTV_TUNERR_PROFILE`, `IPTV_TUNERR_PLEX_SAFE`, profile overrides file, named profile matrix file.
- **Autopilot:** remembered per `dna_id` + client class when enabled.

## See also

- [plex-client-compatibility-matrix](plex-client-compatibility-matrix.md) — which Plex clients should get **`plexsafe`** vs native path when **`CLIENT_ADAPT`** is on (**HR-003**).
- [cli-and-env-reference](cli-and-env-reference.md) — env vars and stream behavior.
- [EPIC-lineup-parity](../epics/EPIC-lineup-parity.md) — product roadmap for named HLS/fMP4 profiles.
