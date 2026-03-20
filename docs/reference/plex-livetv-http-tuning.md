---
id: plex-livetv-http-tuning
type: reference
status: stable
tags: [reference, plex, http, tuning, hr-010, hr-009, hr-008, hr-007, hr-006, hr-005, hr-004, hr-003, hr-002, hr-001]
---

# Plex Live TV: HTTP pools, mux concurrency, and upstream failover

Plex Media Server uses **ffmpeg / Lavf** for many Live TV pulls. HLS in particular tends to open **multiple parallel HTTP requests** (playlist refresh, segments, variants). Tunerr sits in the middle: **PMS → Tunerr → upstream CDN**.

This page ties **work breakdown** stories **HR-010** (connection pooling), **HR-008** (live-path retries), **HR-001** (WebSafe startup / IDR gate), and related items to concrete knobs; see [memory-bank/work_breakdown.md](../../memory-bank/work_breakdown.md).

## Shared upstream HTTP client (`internal/httpclient`)

The same tuned `http.Transport` (idle pool + optional brotli) backs **timeout-scoped** clients via `httpclient.WithTimeout` and the global `httpclient.Default()` / `ForStreaming()` entrypoints. Besides the **indexer**, **stream gateway**, **materializer**, and **VOD FUSE**, integration code paths include:

- **Plex** — DVR + library HTTP (`internal/plex`)
- **Emby / Jellyfin** — registration (`internal/emby`)
- **Provider ranking** — M3U probes (`internal/provider`)
- **Physical HDHomeRun** — `discover.json` / `lineup.json` / `guide.xml` (`internal/hdhomerun`) and `iptv-tunerr hdhr-scan`
- **Guide / EPG fetches** — `httpClientOrDefault` in `internal/tuner/epg_pipeline.go`
- **Health checks** — `internal/health`
- **Stream sniff** — `internal/probe.Probe` when `client == nil`

Mux **`seg=`** still builds a dedicated `http.Client` with redirect validation (`internal/tuner/mux_http_client.go`).

Process-start environment variables:

| Variable | Default | Role |
|----------|---------|------|
| `IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST` | **16** | Idle connections kept **per upstream host** (parallel segment fetches). |
| `IPTV_TUNERR_HTTP_MAX_IDLE_CONNS` | **100** | Global idle connection cap across hosts. |
| `IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC` | **90** | How long an idle connection may sit in the pool before close. |
| `IPTV_TUNERR_HTTP_ACCEPT_BROTLI` | off | Optional `br` decoding (see [cli-and-env-reference](cli-and-env-reference.md)). |

Unset or invalid values fall back to defaults. These are read **once at process start** (same as today’s `MAX_IDLE_CONNS_PER_HOST` behavior).

**When to raise `MAX_IDLE_CONNS_PER_HOST`:** many `connection reset` / `EOF` spikes under load with **one** busy HLS upstream, or PMS opening more parallel segment connections than the default pool comfortably serves.

**When to adjust idle timeout:** long gaps between requests to the same host with finicky CDNs sometimes benefit from a **longer** idle lifetime; conversely, very large values can hold sockets open longer than you want on constrained hosts.

## Tuner count vs `?mux=*&seg=` concurrency

HDHR semantics cap **concurrent full transcodes/streams** with **`TunerCount`** (and optional learned upstream limits). Native **HLS/DASH mux** uses separate **`seg=`** concurrency: default **`effective_tuner_limit × IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER`** (default multiplier **8**), with optional **`IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT`** override. See [hls-mux-toolkit](hls-mux-toolkit.md) and [cli-and-env-reference](cli-and-env-reference.md).

Successful native mux responses include **`X-IptvTunerr-Native-Mux: hls`** or **`dash`** (playlist/MPD rewrite, **`seg=`** relay, **304**/**206**) so operators can tell Tunerr-native proxying apart from TS relay without reading full bodies — details in [hls-mux-toolkit](hls-mux-toolkit.md).

## Stable live channel ordering (HR-006)

On **`index` / catalog refresh**, **`live_channels`** are stored in **sorted order** by **`channel_id`** (then **guide_number**, **guide_name**) inside **`catalog.ReplaceWithLive`**. That keeps **`catalog.json`** and **`lineup.json`** iteration from reshuffling when the upstream playlist order changes, which reduces noisy diffs and unnecessary Plex channel-map churn while **`channel_id`** values themselves stay provider-derived.

## Remux-first transcode overrides (HR-007)

**Global mode** (`IPTV_TUNERR_STREAM_TRANSCODE`):

- **`off`** — prefer remux (copy) through ffmpeg when on the HLS path; **`on`** — always normalize/transcode; **`auto`** — remux when ffprobe says video/audio are already Plex-friendly, otherwise transcode.
- **`auto_cached`** — strict remux-first: only channels listed in **`IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`** change behavior; there is **no** per-request ffprobe (good for cutting probe load).

Provider-pressure follow-on:
- once Tunerr has already observed upstream concurrency pressure, **`IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE`** (default **on**) skips the non-transcode **ffmpeg remux** attempt and goes straight to the Go HLS relay. This avoids spending extra provider/CDN request budget on a remux path that is already failing or destabilizing parallel playback.
- playlist refreshes on the Go HLS relay use bounded retries when the provider answers with concurrency-style failures; tune with **`IPTV_TUNERR_HLS_PLAYLIST_RETRY_LIMIT`** and **`IPTV_TUNERR_HLS_PLAYLIST_RETRY_BACKOFF_MS`**.

**Per-channel file** (`IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`): JSON map; each key is matched against **`channel_id`**, then **`guide_number`**, then **`tvg_id`**. For **`off` / `on` / `auto`**, the file **overrides** the global mode for hits (e.g. force remux for one hot channel while `on` everywhere else, or force transcode for a single bad feed under `off`). Logs: `gateway: transcode policy mode=...` when an override **differs** from the computed base (and always for `auto_cached` hits/misses).

**Later precedence:** **`requestAdaptation`** (Plex client class / `?profile=` / Autopilot) can still force transcode + profile **after** this policy — see `internal/tuner/gateway_adapt.go` (and policy computation in `gateway_policy.go`).

## Sticky WebSafe fallback after adaptation (HR-004)

When **`IPTV_TUNERR_CLIENT_ADAPT`** is on, Tunerr may choose the **non-WebSafe** path for resolved native/TV clients (remux / full transcode off). If that tune then ends with **`all_upstreams_failed`** or **`upstream_concurrency_limited`**, Tunerr records a **session-scoped** sticky entry: the **next** request for the same **channel** and **Plex session/client identifiers** uses **WebSafe** (`plexsafe` transcode) until TTL, without flip-flopping on every retry.

| Variable | Default | Role |
|----------|---------|------|
| `IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK` | on (`true` / `1` / `yes`) | Master switch; set `false` to disable sticky fallback. |
| `IPTV_TUNERR_CLIENT_ADAPT_STICKY_TTL_SEC` | **14400** (4h) | Sticky lifetime; clamped to **120** … **604800** (7d). |
| `IPTV_TUNERR_CLIENT_ADAPT_STICKY_LOG` | off | Set `1` to log internal sticky map keys (with channel id). |

Sticky keys require at least one of **`X-Plex-Session-Identifier`** or **`X-Plex-Client-Identifier`** (or equivalent query aliases Tunerr already accepts). If both are missing, nothing is recorded or honored — avoids pinning WebSafe for all anonymous clients on a channel.

**`/debug/runtime.json`** echoes raw env strings: **`tuner.client_adapt_sticky_fallback`**, **`tuner.client_adapt_sticky_ttl_sec`**.

## WebSafe ffmpeg startup gate + IDR awareness (HR-001)

On **transcoding** ffmpeg relay paths with **`IPTV_TUNERR_WEBSAFE_STARTUP_MIN_BYTES` > 0**, Tunerr **prefetches** ffmpeg’s MPEG-TS stdout before streaming the body. When **`IPTV_TUNERR_WEBSAFE_REQUIRE_GOOD_START`** is **on** (default), the gate waits for **H.264 Annex B IDR** *or* **HEVC Annex B IRAP** (NAL types **16–21**) plus **AAC ADTS** in the scanned TS (see `looksLikeGoodTSStart` / `containsAnnexBVideoKeyframe` in `internal/tuner/gateway_stream_helpers.go`) **and** at least **`STARTUP_MIN_BYTES`**, while bounding memory with a **sliding window** of **`IPTV_TUNERR_WEBSAFE_STARTUP_MAX_BYTES`** — it no longer flushes solely because the max byte count was reached without a detected video keyframe + AAC.

**Logs:** the **`startup-gate buffered=`** line includes **`release=`** — e.g. **`min-bytes-idr-aac-ready`**, **`max-bytes-no-signal-required`** (when **`REQUIRE_GOOD_START`** is off), **`max-bytes-without-idr-fallback`** (only if **`IPTV_TUNERR_WEBSAFE_STARTUP_MAX_FALLBACK_WITHOUT_IDR=true`**), or **`read-ended-partial-without-idr-aac`** on early EOF. The **`idr=`** field is **true** when either **H.264 IDR** or **HEVC IRAP** was seen (name is historical).

**`/debug/runtime.json`:** **`tuner.websafe_require_good_start`**, **`tuner.websafe_startup_max_fallback_without_idr`**, **`tuner.websafe_startup_{min,max}_bytes`**, **`tuner.websafe_startup_timeout_ms`** echo raw env strings.

**Other codecs:** **VP9**, **AV1**, etc. are not scanned here — use **`IPTV_TUNERR_WEBSAFE_STARTUP_MAX_FALLBACK_WITHOUT_IDR`**, relax **`REQUIRE_GOOD_START`**, or extend **`containsAnnexBVideoKeyframe`** in code.

**Plex Web validation bundle:** see **[plex-client-compatibility-matrix](plex-client-compatibility-matrix.md)** (**HR-002** probe + evidence checklist).

## Upstream flap on the **live** gateway path (HR-008)

On **`/stream/<channel>`**, Tunerr walks **primary then backup** stream URLs from the catalog. It does **not** add exponential backoff between attempts on the hot path (that would block throughput). **429 / 423** are not retried here by design.

For **short `seg=` relays**, failures surface as HTTP status + **`X-IptvTunerr-Hls-Mux-Error`**; use **`IPTV_TUNERR_HLS_MUX_ACCESS_LOG`**, **`/metrics`**, and **`/debug/stream-attempts.json`** for evidence.

## See also

- [plex-client-compatibility-matrix](plex-client-compatibility-matrix.md) — tier-1 clients, adaptation classes, QA procedure (**HR-003**).
- [lineup-epg-hygiene](lineup-epg-hygiene.md) — catalog + guide hygiene defaults (**HR-005**).
- [cli-and-env-reference](cli-and-env-reference.md) — full env list.
- [hls-mux-toolkit](hls-mux-toolkit.md) — mux diagnostics and caps.
- [iptvtunerr-troubleshooting](../runbooks/iptvtunerr-troubleshooting.md) — stream-compare harness, Plex HTTP / DVR soak notes.
