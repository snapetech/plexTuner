---
id: hls-mux-proxy
type: how-to
status: stable
tags: [hls, gateway, streaming, operators]
---

# Use Tunerr as an HLS playlist proxy (`?mux=hls`)

Some clients work best when they fetch an **HLS master or media playlist** from Tunerr itself, while Tunerr still pulls **segments and nested playlists** from the provider with the same auth, cookies, and upstream headers as a normal `/stream/<id>` session.

This mode is **not** ffmpeg segment packaging: Tunerr **rewrites** playlist lines to point back through Tunerr and **proxies** each requested URL. Default playback remains **MPEG-TS** when you omit `mux` or use the usual relay/transcode path.

## When to use it

- You want **M3U8 in / M3U8 out** through Tunerr (e.g. testing with `ffplay` or an HLS-aware player) without transcoding to TS first.
- A client mishandles **relative** URLs inside playlists: set **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`** so media lines use **absolute** Tunerr URLs.
- A **browser** or **devtools** page loads the playlist from a different origin than Tunerr: set **`IPTV_TUNERR_HLS_MUX_CORS`**. Tunerr then adds permissive CORS headers on **`?mux=hls`** playlist and segment responses and answers **`OPTIONS`** preflight for the same query pattern.

## Preconditions

- Channel’s `stream_url` / `stream_urls` point at an **HLS** endpoint (playlist), same as today.
- Tunerr can reach the upstream (network, cookies, CF clearance if applicable).

## Steps

1. Note your tuner base URL (same idea as **`IPTV_TUNERR_BASE_URL`**), e.g. `http://192.168.1.10:5004`.
2. For a channel id or guide index `N`, open:

   `GET /stream/<channel>?mux=hls`

   Example: `http://192.168.1.10:5004/stream/42?mux=hls`

3. The response is an **`application/vnd.apple.mpegurl`** playlist. Each media line becomes a Tunerr URL of the form:

   `/stream/<channel>?mux=hls&seg=<url-encoded-upstream-target>`

4. Optional: set **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`** to `http://192.168.1.10:5004` (no trailing slash) so those lines are **absolute** (`http://192.168.1.10:5004/stream/...`).

5. Optional: set **`IPTV_TUNERR_HLS_MUX_CORS=true`** when a web client needs CORS on the playlist and segment URLs.

6. Inspect effective settings at **`/debug/runtime.json`** (`tuner.stream_public_base_url`, **`tuner.hls_mux_cors`**) and SQLite/EPG flags at **`/guide/epg-store.json`** when using incremental provider suffixes.

Byte-range HLS (**`#EXT-X-BYTERANGE`**) requires the player’s **`Range`** request to reach the CDN: Tunerr forwards **`Range`** / **`If-Range`** on upstream fetches and returns **`206 Partial Content`** with **`Content-Range`** when the CDN responds that way.

## Verify

- `curl -sI 'http://127.0.0.1:5004/stream/<id>?mux=hls'` → `200` and `Content-Type` containing `mpegurl`.
- Playlist body contains `mux=hls&seg=` for segment or sub-playlist lines.

## Caveats

- This path **does not** replace Plex’s usual **MPEG-TS** HDHR stream expectation; use it where HLS-through-Tunerr is intentional.
- **AES-128 / init map:** Tunerr rewrites **`URI="..."`** on common HLS tags (including **`#EXT-X-KEY`**, **`#EXT-X-MAP`**, variant **`#EXT-X-STREAM-INF`**) so keys and fMP4 init segments are fetched through the same **`?mux=hls&seg=`** proxy. **Widevine / FairPlay** and other DRM systems are not handled here—test with your client.

See also
--------

- [Transcode profiles](../reference/transcode-profiles.md) — `?mux=fmp4`, `?profile=`, TS defaults.
- [CLI and env reference](../reference/cli-and-env-reference.md) — `IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`, `IPTV_TUNERR_HLS_MUX_CORS`, streaming envs.
- [Features](../features.md) — gateway overview.
