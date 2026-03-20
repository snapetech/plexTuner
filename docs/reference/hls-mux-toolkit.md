---
id: hls-mux-toolkit
type: reference
status: stable
tags: [hls, dash, gateway, operators, diagnostics, mux]
---

# Native mux operator toolkit (`?mux=hls` / `?mux=dash`)

Dense reference for **Tunerr-native HLS** and **experimental DASH MPD** proxying (playlist/MPD rewrite + `seg=` relay). Setup: [HLS mux how-to](../how-to/hls-mux-proxy.md).

## Shipped behavior (quick map)

| Area | Behavior |
|------|-----------|
| Entry URL (HLS) | `GET /stream/<channel>?mux=hls` — rewritten **M3U8**; media uses `?mux=hls&seg=<url-encoded-upstream>` |
| Entry URL (DASH) | `GET /stream/<channel>?mux=dash` on **DASH** upstream — rewritten **MPD** (`application/dash+xml`); **`media=`** / **`initialization=`** / **`sourceURL`** / **`segmentURL`** (and related) accept **double- or single-quoted** values; absolute **http(s)** and **plain relative** URLs resolve with the **`<BaseURL>`** chain + manifest URL → `?mux=dash&seg=` (**experimental**). **`$`** template identifiers stay usable in **`seg=`** (including **`$Number%0Nd$`** / **`$Time%0Nd$`**: **`%`** is preserved through query encoding). Optional **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE`** expands **`SegmentTemplate`** to **`SegmentList`**: uniform **`duration`** + **`$Number$`** (self-closing or paired tags); **`SegmentTimeline`** with **`$Time$`** / **`$Number$`**; **`$Number%0Nd$`** padding; capped by **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_MAX_SEGMENTS`** |
| Upstream auth | Same **`Cookie`**, **`Referer`**, **`Origin`**, **`Authorization`**, correlation IDs as **`/stream`** |
| Playlist `URI="..."` / **`URI='...'`** | Rewritten on HLS tag lines (keys, maps, variants, media, **`EXT-X-PART`** `URI=`, case-insensitive **`uri=`**); non-standard **single-quoted** `URI='https://…'` is normalized to a proxied URL inside the same quote style |
| **Range / If-Range** | Forwarded; **206** + **Content-Range** for binary |
| **304** | Forwarded for conditional segment fetches |
| Non-**http(s)** `seg=` | **400** + **`unsupported_target_scheme`** |
| CDN **4xx/5xx** on `seg=` | Pass-through + diagnostic **`upstream_http_<status>`** + bounded body |
| Transport / URL build | **502** **`Native mux target failed`**; **redirect** policy rejection → **403** (blocked hop) or **502** + **`redirect_rejected`** |
| Redirect chain | Each **3xx** hop re-checks scheme + literal/resolved private policy (max **10** hops); see `internal/tuner/mux_http_client.go` |
| Concurrency | **`seg=`** for **both** mux kinds shares **`hls_mux_seg_*`** slot counter / limits |
| CORS | **`IPTV_TUNERR_HLS_MUX_CORS`** applies to **`mux=hls`** and **`mux=dash`** (**`OPTIONS`** preflight for both) |
| Absolute client URLs | **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`** |
| Rate limit | Optional **`IPTV_TUNERR_HLS_MUX_SEG_RPS_PER_IP`** per source IP (**429** + **`seg_rate_limited`**) |
| DNS-assisted SSRF guard | **`IPTV_TUNERR_HLS_MUX_DENY_RESOLVED_PRIVATE_UPSTREAM`** — **A/AAAA** lookup; block if any addr is private/link-local/loopback (**fail-open** on DNS errors; combine with literal deny) |
| gzip | Go’s **`http.Transport`** negotiates **gzip** and **decompresses** unless disabled — segments/playlists are usually already decoded when handlers run |
| Brotli | Optional **`IPTV_TUNERR_HTTP_ACCEPT_BROTLI`**: **`Accept-Encoding`** includes **`br`** and **`Content-Encoding: br`** responses are decompressed (`internal/httpclient`) |
| LL-HLS-style tags | **`URI="..."`** on **`#EXT-X-PRELOAD-HINT`**, **`#EXT-X-RENDITION-REPORT`**, **`#EXT-X-PART`**, etc.; optional same-line **`#EXTINF`** segment URI (conservative); non-standard **`#EXTINF:...,BYTERANGE=...`** on one line is split into **`#EXTINF`** + **`#EXT-X-BYTERANGE`** — see [hls-mux-ll-hls-tags](hls-mux-ll-hls-tags.md) |
| UTF-8 BOM | Leading **`EF BB BF`** is stripped before HLS / DASH manifest rewrite so **`#EXTM3U`** / **`<MPD`** still match at line/start |
| **`SegmentTimeline` `S` elements** | Quote-aware **`<S …/>`** / **`<S …>…</S>`** scan ( **`>`** inside quoted attrs OK); nested **`<S>`** balanced (invalid MPD: outer **`<S>`** row only) |
| Metrics | **`iptv_tunerr_mux_seg_request_duration_seconds`** histogram (when **`IPTV_TUNERR_METRICS_ENABLE`**); optional per-channel counter/histogram labels (**`IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS`**, high cardinality) |
| Autopilot | Optional **`IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS`**: extra **`seg=`** slots for channels whose **`dna_id`** has hot Autopilot memory (see env table) |

## Diagnostic header: `X-IptvTunerr-Hls-Mux-Error`

| Value | Meaning |
|-------|--------|
| `unsupported_target_scheme` | **`seg=`** is not **http** / **https** |
| `seg_param_too_large` | Decoded **`seg=`** over max |
| `blocked_private_upstream` | Literal or resolved host blocked (**403**) (initial **`seg=`** or **redirect** hop) |
| `redirect_rejected` | Redirect hop failed policy (non-private), too many hops, or unsupported scheme (**502**) |
| `seg_rate_limited` | Per-IP RPS cap (**429**) |
| `upstream_http_<status>` | Upstream HTTP status |

**Logs:** error responses include **`hls_mux_diag=<token>`** (and upstream pass-through lines log the same token).

## Stream attempt `finalStatus` patterns

| Pattern | Meaning |
|---------|--------|
| `ok` + `hls_mux_target` / `dash_mux_target` | Successful **`seg=`** |
| `<mux>_mux_unsupported_target_scheme` | **400** scheme |
| `<mux>_mux_seg_param_too_large` | **400** length |
| `<mux>_mux_blocked_private_upstream` | **403** |
| `<mux>_mux_seg_rate_limited` | **429** |
| `<mux>_mux_upstream_http_<n>` | Upstream **n** |
| `<mux>_mux_target_failed` | **502** |
| `<mux>_mux_redirect_rejected` | **502** redirect policy (**403** if hop blocked as private) |
| `<mux>_mux_seg_limit` | **503** / **805** |

`<mux>` is **`hls`** or **`dash`**.

## Environment variables (native mux)

| Variable | Role |
|----------|------|
| `IPTV_TUNERR_STREAM_PUBLIC_BASE_URL` | Absolute base for rewritten lines |
| `IPTV_TUNERR_HLS_MUX_CORS` | CORS + **`OPTIONS`** for **`hls`** and **`dash`** |
| `IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT` | Cap concurrent **`seg=`** |
| `IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER` | × effective tuner limit (**8** default) |
| `IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO` | When **true**, add **temporary** bonus slots from recent **503** seg-limit hits (ignored if **`IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT`** is set) |
| `IPTV_TUNERR_HLS_MUX_SEG_AUTO_WINDOW_SEC` | Rolling window for auto bonus (**5**–**600**; default **60**) |
| `IPTV_TUNERR_HLS_MUX_SEG_AUTO_BONUS_PER_HIT` | Bonus slots per reject in window (default **4**) |
| `IPTV_TUNERR_HLS_MUX_SEG_AUTO_BONUS_CAP` | Max bonus slots (default **64**) |
| `IPTV_TUNERR_HLS_MUX_ACCESS_LOG` | Append **one JSON line** per successful **`seg=`** (redacted URL, duration); see [ADR 0005](../adr/0005-hls-mux-no-disk-packager.md) |
| `IPTV_TUNERR_HLS_MUX_MAX_SEG_PARAM_BYTES` | Max decoded **`seg=`** |
| `IPTV_TUNERR_HLS_MUX_DENY_LITERAL_PRIVATE_UPSTREAM` | Block literal RFC1918 / loopback / link-local IPs in URL host |
| `IPTV_TUNERR_HLS_MUX_DENY_RESOLVED_PRIVATE_UPSTREAM` | DNS lookup + block if any resolved IP is unsafe |
| `IPTV_TUNERR_HLS_MUX_UPSTREAM_ERR_BODY_MAX` | Upstream error body cap |
| `IPTV_TUNERR_HLS_MUX_SEG_RPS_PER_IP` | Token-bucket rate / IP (**0** = off) |
| `IPTV_TUNERR_HLS_MUX_WEB_DEMO` | Serves **`/debug/hls-mux-demo.html`** (hls.js sample) when **true** |
| `IPTV_TUNERR_METRICS_ENABLE` | Exposes Prometheus **`GET /metrics`** + mux counters + **`iptv_tunerr_mux_seg_request_duration_seconds`** histogram |
| `IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS` | Add **`channel_id`** to mux counter/histogram labels (**high cardinality**; default off) |
| `IPTV_TUNERR_HTTP_ACCEPT_BROTLI` | Accept **br** and decompress brotli upstream bodies on the shared HTTP stack |
| `IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST` | Tunable **`MaxIdleConnsPerHost`** on shared HTTP client (default **16**) |
| `IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS` | Extra concurrent **`seg=`** slots from Autopilot hit counts (**`MAX_CONCURRENT`** disables) |
| `IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_MIN_HITS` | Minimum Autopilot **Hits** (best row per **`dna_id`**) before bonus applies (default **3**) |
| `IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS_PER_STEP` | Bonus slots × (**maxHits − minHits + 1**), capped (default **4** per step) |
| `IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS_CAP` | Max autopilot bonus slots (default **32**) |
| `IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE` | When **true**, expand uniform **`SegmentTemplate`** → **`SegmentList`** before MPD URL rewrite (default **off**) |
| `IPTV_TUNERR_HLS_MUX_DASH_EXPAND_MAX_SEGMENTS` | Max segments emitted per expanded template (default **10000**, hard cap **500000**) |

See [cli-and-env-reference](cli-and-env-reference.md#stream-behavior).

## Operator endpoints

| Path | Role |
|------|------|
| `/debug/hls-mux-demo.html` | Static demo player (**`IPTV_TUNERR_HLS_MUX_WEB_DEMO`**) |
| `/metrics` | Prometheus scrape (**`IPTV_TUNERR_METRICS_ENABLE`**) |
| `/ops/actions/mux-seg-decode` | **POST** `{"seg_b64":"..."}` → **`redacted_url`**, **`http_ok`** (localhost / **UI_ALLOW_LAN** policy) |

## Copy-paste checks

```bash
curl -sSI 'http://127.0.0.1:5004/stream/<id>?mux=hls'
curl -sS 'http://127.0.0.1:5004/stream/<id>?mux=hls' -o /tmp/tunerr.m3u8
grep -E 'mux=hls&seg=' /tmp/tunerr.m3u8 | head -1

# DASH (if upstream is MPD)
curl -sS 'http://127.0.0.1:5004/stream/<id>?mux=dash' | head

curl -sS 'http://127.0.0.1:5004/provider_profile.json' | jq '.hls_mux_seg_success, .dash_mux_seg_success, .forwarded_headers'

# Soak helper
# HLS_MUX_URL='http://127.0.0.1:5004/stream/0?mux=hls' ./scripts/hls-mux-soak.sh
```

## Enhancement backlog (remaining)

- **HLS:** tags that carry HTTP(S) targets only in non-**`URI`** attributes (no `URI=` / `uri=`) or exotic quoting/escapes (open an issue with a sample). A parallel effort is extending the stream-compare harness to capture failing M3U8/MPD samples as fixtures for regression tests.
- **DASH:** **`BaseURL`** only as a single-quoted attribute (unusual); **`SegmentTemplate`** with **`$Number$`** inside non-**`media`** attributes only. **`SegmentTimeline`** **`<S>`** parsing is quote-aware and balances nested **`<S>…</S>`** (invalid per ISO, but tolerated: only the **outer** **`<S>`** contributes a segment row).

## Related code

- `internal/tuner/gateway_hls.go` — HLS rewrite, **`serveNativeMuxTarget`**, CORS, diagnostics, access log
- `internal/tuner/mux_http_client.go` — **`seg=`** HTTP client with redirect validation
- `internal/tuner/gateway_dash.go` — MPD rewrite
- `internal/tuner/gateway_dash_expand.go` — optional **`SegmentTemplate`** → **`SegmentList`** expansion
- `internal/tuner/gateway.go` — **`seg=`** policy, **`dash` / `hls`** modes, main loop **`dash_native_mux`**
- `internal/tuner/gateway_policy.go` — **`effectiveHLSMuxSegLimitLocked`**, adaptive bonus
- `internal/tuner/prometheus_mux.go` — Prometheus counters
- `internal/safeurl/privateresolve.go` — DNS-assisted private check
- `internal/safeurl/mux_target.go` — shared **`ValidateMuxSegTarget`** for initial URL + redirects

See also
--------

- [LL-HLS tag coverage](hls-mux-ll-hls-tags.md)
- [HLS mux how-to](../how-to/hls-mux-proxy.md)
- [Transcode profiles](transcode-profiles.md) — **`?mux=hls`** / **`?mux=dash`** vs ffmpeg **`?mux=fmp4`**
- [CLI and env reference](cli-and-env-reference.md)
- [CHANGELOG](../CHANGELOG.md)
