---
id: adr-0005-hls-mux-no-disk-packager
type: reference
status: accepted
tags: [adr, hls, dash, mux, packaging]
---

# ADR 0005: Native mux stays in-process — no built-in disk HLS packager

## Status

Accepted.

## Context

- Tunerr’s **native mux** (`?mux=hls` / `?mux=dash`) rewrites manifests and **proxies** bytes through **`seg=`**. Operators sometimes ask for a **server-side packager** that writes **fMP4 / LL-HLS** to disk for static serving or CDN offload.
- A real packager implies **persistent storage layout**, **key rotation**, **cleanup**, **disk quotas**, **concurrent writers**, and **codec/legal** concerns — a different product surface than a live relay.
- **Goal:** Be explicit that the core daemon does **not** ship an on-disk packager, while still allowing **observability** (metrics, optional JSONL access lines) for mux traffic.

## Decision

1. **No in-repo disk packager** as part of `iptv-tunerr`: mux remains an **HTTP relay + rewrite** path only.
2. **Substitute for “packager” asks:** use **external** tooling (ffmpeg, shaka-packager, etc.) fed from Tunerr’s proxied URLs if a site needs packaged outputs; document that this is **out of process**.
3. **Optional audit trail:** **`IPTV_TUNERR_HLS_MUX_ACCESS_LOG`** appends **one JSON line per successful `seg=` completion** (redacted upstream URL) for operators who want a lightweight “what was fetched” log without packaging.

## Consequences

- Scope stays bounded; security and support load do not expand into storage lifecycle.
- Users who need packaged VOD/live-to-file workflows compose Tunerr with **their** packager of choice.
- Future work that truly belongs in Tunerr would be a **separate command** or sidecar with its own ADR, not a silent growth of the gateway.

## References

- `internal/tuner/gateway_hls.go` — `serveNativeMuxTarget`, access log hook
- [HLS mux how-to](../how-to/hls-mux-proxy.md)
- [Native mux toolkit](../reference/hls-mux-toolkit.md)

See also
--------

- [Observability: Prometheus and OpenTelemetry](../explanations/observability-prometheus-and-otel.md)
