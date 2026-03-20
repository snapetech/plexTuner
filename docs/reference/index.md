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
| [cli-and-env-reference](cli-and-env-reference.md) | Commands, common flags, and key environment variables (including multi-DVR/testing options). |
| [transcode-profiles](transcode-profiles.md) | Gateway transcode profile names, HDHomeRun-style aliases, `?profile=`, optional `?mux=fmp4` / `?mux=hls`. |
| [plex-dvr-lifecycle-and-api](plex-dvr-lifecycle-and-api.md) | Plex Live TV/DVR lifecycle reference: wizard-equivalent API flow, injected DVRs, remove/refresh/channelmap, and UI/backend gotchas. |
| [epg-linking-pipeline](epg-linking-pipeline.md) | Multi-provider channel/EPG linking pipeline: normalization, alias/override DB, confidence scoring, and rollout strategy for large unlinked channel sets. |
| [upstream-m3u-split-requirement](upstream-m3u-split-requirement.md) | Required behavior for iptv-m3u-server when generating category `dvr-*.m3u`: emit all stream URLs per channel so strip + dedupe keep non-CF. |
| [testing-and-supervisor-config](testing-and-supervisor-config.md) | Supervisor mode and recent test/lab env vars (guide offsets, reaper, HDHR shaping, XMLTV normalization). |
| [memory-bank/commands.yml](../../memory-bank/commands.yml) | Verification commands. |

See also
--------
- [Docs index](../index.md).
- [Explanations](../explanations/index.md).
