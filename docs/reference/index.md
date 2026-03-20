---
id: reference-index
type: reference
status: stable
tags: [reference, index]
---

# Reference (facts, API, config)

Dense and factual. Add CLI reference, env vars, API docs as needed.

| Doc | Description |
|-----|-------------|
| [cli-and-env-reference](cli-and-env-reference.md) | Canonical hand-maintained commands, flags, and environment variables (multi-DVR, mux, HTTP pool, web UI, recorder, …). |
| [transcode-profiles](transcode-profiles.md) | Gateway transcode profile names, HDHR-style aliases, `?profile=`, optional `?mux=fmp4` / `?mux=hls`. |
| [hls-mux-toolkit](hls-mux-toolkit.md) | Native **`?mux=hls`** / **`?mux=dash`** mux: diagnostics, Prometheus, DNS SSRF options, operator **`curl`**, remaining backlog. |
| [hls-mux-ll-hls-tags](hls-mux-ll-hls-tags.md) | LL-HLS / low-latency HLS tag rewrite coverage for **`?mux=hls`**. |
| [plex-livetv-http-tuning](plex-livetv-http-tuning.md) | Plex/Lavf parallel HTTP, shared client idle pool, tuner vs **`seg=`** caps (**HR-008** / **HR-010**). |
| [plex-client-compatibility-matrix](plex-client-compatibility-matrix.md) | Tier-1 Plex clients, adaptation classes, QA procedure (**HR-003**). |
| [lineup-epg-hygiene](lineup-epg-hygiene.md) | Built-in catalog + serve hygiene: tvg-id dedupe, strip hosts, guide policy, EPG-only (**HR-005**). |
| [plex-dvr-lifecycle-and-api](plex-dvr-lifecycle-and-api.md) | Plex Live TV/DVR lifecycle reference: wizard-equivalent API flow, injected DVRs, remove/refresh/channelmap, and UI/backend gotchas. |
| [epg-linking-pipeline](epg-linking-pipeline.md) | Multi-provider channel/EPG linking pipeline: normalization, alias/override DB, confidence scoring, and rollout strategy for large unlinked channel sets. |
| [upstream-m3u-split-requirement](upstream-m3u-split-requirement.md) | Required behavior for iptv-m3u-server when generating category `dvr-*.m3u`: emit all stream URLs per channel so strip + dedupe keep non-CF. |
| [testing-and-supervisor-config](testing-and-supervisor-config.md) | Supervisor mode and recent test/lab env vars (guide offsets, reaper, HDHR shaping, XMLTV normalization). |
| [memory-bank/commands.yml](../../memory-bank/commands.yml) | Verification commands. |

See also
--------
- [Docs index](../index.md).
- [Explanations](../explanations/index.md).
