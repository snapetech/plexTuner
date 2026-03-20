---
id: docs-index
type: reference
status: stable
tags: [docs, index]
---

# Docs index

Where to find what. This repo uses the [Diátaxis](https://diataxis.fr/) split by reader need.

**Quick entrypoints:** [README](../README.md) (product overview + doc map) · [CHANGELOG](CHANGELOG.md) (**[Unreleased]** = current engineering slices) · [features.md](features.md) (canonical capability table) · [cli-and-env-reference](reference/cli-and-env-reference.md).

| Section | Purpose |
|--------|--------|
| **[tutorials/](tutorials/index.md)** | Learning-oriented: get started, first run. |
| **[how-to/](how-to/index.md)** | Task-oriented: first push, add remote. |
| **[reference/](reference/index.md)** | Facts: commands, config. |
| **[explanations/](explanations/index.md)** | Why and concepts. Add project docs here. |
| **[adr/](adr/index.md)** | Decision log: architecture decision records. |
| **[runbooks/](runbooks/index.md)** | Operational procedures ([troubleshooting](runbooks/iptvtunerr-troubleshooting.md) §8: **`/healthz`**, **`/readyz`**). |
| **[how-to/deployment.md](how-to/deployment.md)** | Deploy IPTV Tunerr (binary, Docker, systemd, local test script). |
| **[how-to/plex-ops-patterns.md](how-to/plex-ops-patterns.md)** | Advanced Plex-only operating patterns: zero-touch, category DVR fleets, injected DVRs. |
| **[how-to/package-test-builds.md](how-to/package-test-builds.md)** | Build cross-platform test bundles for testers (Linux/macOS/Windows). |
| **[how-to/tester-handoff-checklist.md](how-to/tester-handoff-checklist.md)** | Tester handoff checklist (bundle contents, platform expectations, bug report capture). |
| **[how-to/tester-release-notes-draft.md](how-to/tester-release-notes-draft.md)** | Draft tester-facing release notes for current validation builds. |
| **[how-to/cloudflare-bypass.md](how-to/cloudflare-bypass.md)** | Cloudflare-protected providers: UA cycling, cookies, headers, troubleshooting. |
| **[how-to/debug-bundle.md](how-to/debug-bundle.md)** | Collect a shareable `debug-bundle` and analyze with `scripts/analyze-bundle.py`. |
| **[how-to/stream-compare-harness.md](how-to/stream-compare-harness.md)** | Stream-compare: direct vs Tunerr + **`stream-compare-report.py`** (**runbook §9**). |
| **[how-to/live-race-harness.md](how-to/live-race-harness.md)** | Live-race harness: synthetic/replay + **`live-race-harness-report.py`** (**runbook §7**). |
| **[how-to/multi-stream-harness.md](how-to/multi-stream-harness.md)** | Two-stream collapse: **`scripts/multi-stream-harness.sh`** + report (**runbook §10**). |
| **[how-to/hybrid-hdhr-iptv.md](how-to/hybrid-hdhr-iptv.md)** | Merge HDHR `lineup.json` + `guide.xml` with IPTV index and `/guide.xml`. |
| **[how-to/lineup-parity-lp012-closure.md](how-to/lineup-parity-lp012-closure.md)** | **LP-012** operator checklist (hybrid HDHR, mux, readiness, LTV links). |
| **[how-to/hls-mux-proxy.md](how-to/hls-mux-proxy.md)** | HLS/DASH-through-Tunerr: `?mux=hls` / experimental `?mux=dash`, optional **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`**, CORS, web demo. |
| **[reference/hls-mux-toolkit.md](reference/hls-mux-toolkit.md)** | Native mux diagnostics (**HLS** + **DASH**), operator **`curl`** snippets, metrics, DNS SSRF options, remaining backlog. |
| **[reference/hls-mux-ll-hls-tags.md](reference/hls-mux-ll-hls-tags.md)** | LL-HLS / low-latency tag coverage for **`?mux=hls`** playlist rewrite. |
| **[reference/cli-and-env-reference.md](reference/cli-and-env-reference.md)** | Commands, flags, and key env vars (including supervisor/testing knobs). |
| **[reference/plex-livetv-http-tuning.md](reference/plex-livetv-http-tuning.md)** | Plex/Lavf parallel HTTP vs Tunerr’s shared **`httpclient`** pool; mux concurrency; live-path failover (**work breakdown HR-008 / HR-010**). |
| **[potential_fixes.md](potential_fixes.md)** | Draft: Plex Live TV startup-race mitigations; code pointers follow **`gateway_*`** layout (**HR-001** context). |
| **[reference/plex-client-compatibility-matrix.md](reference/plex-client-compatibility-matrix.md)** | Tier-1 Plex Live TV clients, **`CLIENT_ADAPT`** classes, automated + manual validation (**HR-003**). |
| **[reference/lineup-epg-hygiene.md](reference/lineup-epg-hygiene.md)** | Built-in lineup/EPG hygiene defaults: dedupe, strip hosts, guide policy, stable catalog order (**HR-005** / **HR-006**). |
| **[reference/transcode-profiles.md](reference/transcode-profiles.md)** | Gateway ffmpeg profile names, HDHR-style aliases, `?profile=`, `?mux=fmp4` / `?mux=hls` / `?mux=dash`. |
| **[explanations/media-server-integration-modes.md](explanations/media-server-integration-modes.md)** | Where the common tuner story ends and Plex-heavy ops begin. |
| **[explanations/observability-prometheus-and-otel.md](explanations/observability-prometheus-and-otel.md)** | **`/metrics`** and bridging to OpenTelemetry via collector scrape. |
| **[explanations/always-on-recorder-daemon.md](explanations/always-on-recorder-daemon.md)** | Rolling catch-up recorder daemon (`catchup-daemon`): MVP shipped, extensions noted. |
| **[k8s/README.md](../k8s/README.md)** | Kubernetes deployment (HDHR in cluster). |
| **[epics/](epics/EPIC-template.md)** | Multi-PR epic template. Use with [memory-bank/work_breakdown.md](../memory-bank/work_breakdown.md). |
| **[epics/EPIC-lineup-parity.md](epics/EPIC-lineup-parity.md)** | Optional **Lineup-app-style** track: real HDHomeRun client, web dashboard, SQLite EPG, HLS/fMP4 profiles ([inspiration](https://github.com/am385/lineup)). |
| **product/** · **stories/** | [PRD template](product/PRD-template.md), [STORY template](stories/STORY-template.md). |
| **[features.md](features.md)** | Full feature list. |
| **[CHANGELOG.md](CHANGELOG.md)** | Version and change history. |
| **[docs-gaps.md](docs-gaps.md)** | Known documentation gaps and suggested fixes. |

**Conventions:** [Linking and formatting](_meta/linking.md) · [Glossary](glossary.md).

See also
--------
- [AGENTS.md](../AGENTS.md) and [memory-bank/](../memory-bank/) (**[repo_map.md](../memory-bank/repo_map.md)** for code navigation).
- [README](../README.md) (Kubernetes probe notes, **Recent Changes**, full documentation map).
