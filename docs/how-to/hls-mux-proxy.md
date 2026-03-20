---
id: hls-mux-proxy
type: how-to
status: stable
tags: [hls, dash, gateway, streaming, operators]
---

# Use Tunerr as an HLS / DASH manifest proxy (`?mux=hls`, `?mux=dash`)

Some clients work best when they fetch an **HLS master or media playlist** from Tunerr itself, while Tunerr still pulls **segments and nested playlists** from the provider with the same auth, cookies, and upstream headers as a normal `/stream/<id>` session.

This mode is **not** ffmpeg segment packaging: Tunerr **rewrites** manifest lines to point back through Tunerr and **proxies** each requested URL. **`?mux=dash`** is **experimental** for **MPD** upstreams: absolute **http(s)** references are rewritten, and **plain relative** **`media=`** / **`initialization=`** / **`<BaseURL>`** text is resolved using the manifest URL plus a running **`<BaseURL>`** chain. URL attributes may use **single or double** quotes. **`SegmentTemplate`** values that still contain **`$`** placeholders are left as-is in **`seg=`** by default (including **`$Number%0Nd$`** width forms). Optional **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE`** expands **`SegmentTemplate`** (uniform duration, **`SegmentTimeline`**, paired tags, padded **`$Number`**) into **`SegmentList`** before URL rewrite (bounded by **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_MAX_SEGMENTS`**; see [hls-mux-toolkit](../reference/hls-mux-toolkit.md)). Default playback remains **MPEG-TS** when you omit `mux` or use the usual relay/transcode path.

## When to use it

- You want **M3U8 in / M3U8 out** through Tunerr (e.g. testing with `ffplay` or an HLS-aware player) without transcoding to TS first.
- A client mishandles **relative** URLs inside playlists: set **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`** so media lines use **absolute** Tunerr URLs.
- A **browser** or **devtools** page loads the manifest from a different origin than Tunerr: set **`IPTV_TUNERR_HLS_MUX_CORS`**. Tunerr then adds permissive CORS headers on **`?mux=hls`** and **`?mux=dash`** responses and answers **`OPTIONS`** preflight for both query patterns.
- Optional **`IPTV_TUNERR_HLS_MUX_WEB_DEMO`** exposes **`/debug/hls-mux-demo.html`** (uses **hls.js** from a CDN — still your responsibility to allow CORS to the tuner).

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

6. Inspect effective settings at **`/debug/runtime.json`** (`tuner.stream_public_base_url`, **`tuner.hls_mux_cors`**, **`tuner.hls_mux_dash_expand_segment_template`**, **`tuner.hls_mux_max_seg_param_bytes`**, **`tuner.hls_mux_deny_literal_private_upstream`**, etc.) and SQLite/EPG flags at **`/guide/epg-store.json`** when using incremental provider suffixes.

Byte-range HLS (**`#EXT-X-BYTERANGE`**) requires the player’s **`Range`** request to reach the CDN: Tunerr forwards **`Range`** / **`If-Range`** on upstream fetches and returns **`206 Partial Content`** with **`Content-Range`** when the CDN responds that way. If a packager emits non-standard **`#EXTINF:...,BYTERANGE=...`** on a single line, Tunerr splits that into separate **`#EXTINF`** and **`#EXT-X-BYTERANGE`** tags during rewrite. A leading **UTF-8 BOM** on the playlist or MPD body is removed before rewrite. Conditional cache validation (**`If-None-Match`** / **`If-Modified-Since`**) is forwarded; **`304 Not Modified`** is passed through for segment/sub-playlist responses.

**Concurrency:** each **`?mux=hls|dash&seg=`** request counts against the same cap (default **effective tuner limit × 8**, tunable with **`IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER`** or absolute **`IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT`**). Optional **`IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO`** temporarily raises the cap after recent **503** limit hits (see [hls-mux-toolkit](../reference/hls-mux-toolkit.md)). Inspect **`hls_mux_seg_in_use`** / **`hls_mux_seg_limit`** on the provider behavior profile endpoint. When the cap is hit, Tunerr returns **`503`** (same **`805`** tuner-style error header as main streams). **`IPTV_TUNERR_HLS_MUX_ACCESS_LOG`** can append one JSON line per successful **`seg=`** (redacted URL) for light auditing.

## Verify

- `curl -sI 'http://127.0.0.1:5004/stream/<id>?mux=hls'` → `200` and `Content-Type` containing `mpegurl`.
- Playlist body contains `mux=hls&seg=` for segment or sub-playlist lines.

Regression fixtures: upstream/Tunerr manifest captures from the stream-compare harness (or manual **`curl`**) can be dropped under **`internal/tuner/testdata/`** (see **`testdata/README.md`**) and wired into **`gateway_*_test.go`** so rewrites stay covered without re-fetching live CDNs.

## Caveats

- This path **does not** replace Plex’s usual **MPEG-TS** HDHR stream expectation; use it where HLS-through-Tunerr is intentional.
- **AES-128 / SAMPLE-AES / init map:** Tunerr rewrites **`URI="..."`** (case-insensitive **`uri="`**) on common HLS tags—including **`#EXT-X-KEY`** (**`METHOD=AES-128`** or **`METHOD=SAMPLE-AES`** with optional **`KEYFORMAT`**), **`#EXT-X-SESSION-KEY`** on master playlists, **`#EXT-X-MAP`**, **`#EXT-X-MEDIA`**, variant **`#EXT-X-STREAM-INF`**—so keys, init segments, and renditions use the same **`?mux=hls&seg=`** proxy and upstream cookies. Empty **`URI=""`** is left unchanged. **Widevine / FairPlay** (e.g. non-HTTP **`skd://`** key delivery), PlayReady, and other full DRM stacks are **not** implemented here—test with your client.
- For direct **`?mux=hls&seg=`** requests, non-HTTP target schemes are rejected early with **`400 Bad Request`** ("unsupported hls mux target URL scheme") instead of generic `502`. The response includes **`X-IptvTunerr-Hls-Mux-Error: unsupported_target_scheme`** for scripts and devtools; with **`IPTV_TUNERR_HLS_MUX_CORS`**, that header is exposed to browser JavaScript.
- When the **CDN** returns an error for a **`seg=`** URL (**403**, **404**, **5xx**, …), Tunerr **forwards that status** (and a short upstream body preview) so players and **`curl`** see the real failure mode, with **`X-IptvTunerr-Hls-Mux-Error: upstream_http_<status>`**. Local request-build and network errors still surface as **`502`** (**`HLS mux target failed`**).

See also
--------

- [LL-HLS tag coverage (reference)](../reference/hls-mux-ll-hls-tags.md) — which **`#EXT-*`** lines get **`URI=`** rewrite and same-line **`#EXTINF`** handling.
- [HLS mux toolkit (reference)](../reference/hls-mux-toolkit.md) — diagnostics, **`curl`** snippets, enhancement backlog.
- [Transcode profiles](../reference/transcode-profiles.md) — `?mux=fmp4`, `?profile=`, TS defaults.
- [CLI and env reference](../reference/cli-and-env-reference.md) — `IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`, `IPTV_TUNERR_HLS_MUX_CORS`, streaming envs.
- [Observability: Prometheus and OpenTelemetry](../explanations/observability-prometheus-and-otel.md) — **`/metrics`** and collector scrape.
- [Features](../features.md) — gateway overview.
