---
id: observability-prometheus-otel
type: explanation
status: stable
tags: [observability, prometheus, opentelemetry, metrics]
---

# Prometheus metrics and OpenTelemetry

IPTV Tunerr exposes **Prometheus** text exposition when **`IPTV_TUNERR_METRICS_ENABLE`** is set: **`GET /metrics`** on the tuner HTTP server. Native mux outcomes are recorded as **`iptv_tunerr_mux_seg_outcomes_total`** (labels **`mux`**, **`outcome`**, and optionally **`channel_id`** when **`IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS`** is enabled — **warning:** per-channel series multiply cardinality). Completed **`seg=`** requests also populate **`iptv_tunerr_mux_seg_request_duration_seconds`** (histogram; **`mux`** and optional **`channel_id`**).

## OpenTelemetry

There is **no in-process OTLP exporter** in the daemon today. The supported bridge is **vendor-neutral**:

1. Run an **OpenTelemetry Collector** (or Grafana Agent in “collector” mode) with a **Prometheus receiver** that scrapes Tunerr’s **`/metrics`** endpoint.
2. Export from the collector to your backend (**OTLP**, **Prometheus remote write**, vendor sinks, etc.).

This avoids pulling a large metrics SDK into the main binary while still fitting standard **OTel** deployment patterns.

See also
--------

- [Native mux toolkit](../reference/hls-mux-toolkit.md) — mux metrics and diagnostics
- [CLI and env reference](../reference/cli-and-env-reference.md)
