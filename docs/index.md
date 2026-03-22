---
id: docs-index
type: reference
status: stable
tags: [docs, index]
---

# Docs index

Where to find what. This repo uses the [Diátaxis](https://diataxis.fr/) split by reader need.

**Quick entrypoints:** [README](../README.md) (product overview + doc map) · [CHANGELOG](CHANGELOG.md) (**[Unreleased]** = current engineering slices) · [features.md](features.md) (canonical capability table) · [cli-and-env-reference](reference/cli-and-env-reference.md) · [release-readiness-matrix](explanations/release-readiness-matrix.md) (what is actually proven before a tag) · [project-backlog](explanations/project-backlog.md) (open work index: epics, opportunities, constraints).

| Section | Purpose |
|--------|--------|
| **[tutorials/](tutorials/index.md)** | Learning-oriented: get started, first run. |
| **[how-to/](how-to/index.md)** | Task-oriented: first push, add remote. |
| **[reference/](reference/index.md)** | Facts: commands, config. |
| **[explanations/](explanations/index.md)** | Why and concepts. Includes [release-readiness-matrix](explanations/release-readiness-matrix.md) for current proof/gate coverage. |
| **[adr/](adr/index.md)** | Decision log: architecture decision records. |
| **[runbooks/](runbooks/index.md)** | Operational procedures ([troubleshooting](runbooks/iptvtunerr-troubleshooting.md) §8: **`/healthz`**, **`/readyz`**). |
| **[how-to/deployment.md](how-to/deployment.md)** | Deploy IPTV Tunerr (binary, Docker, systemd, local test script). |
| **[how-to/mac-baremetal-smoke.md](how-to/mac-baremetal-smoke.md)** | Cross-build, wake, SSH, and prove the app on a real macOS host. |
| **[how-to/windows-baremetal-smoke.md](how-to/windows-baremetal-smoke.md)** | Prepare and run the Windows bare-metal smoke lane when the host/VM is available. |
| **[how-to/vod-webdav-client-harness.md](how-to/vod-webdav-client-harness.md)** | Replay macOS/Windows WebDAV client request shapes against `vod-webdav`. |
| **[how-to/plex-lineup-harvest.md](how-to/plex-lineup-harvest.md)** | Probe Plex lineup variants, capture harvest bundles, and feed Programming Manager import/assist flows. |
| **[how-to/plex-ops-patterns.md](how-to/plex-ops-patterns.md)** | Advanced Plex-only operating patterns: zero-touch, category DVR fleets, injected DVRs. |
| **[how-to/reverse-engineer-plex-livetv-access.md](how-to/reverse-engineer-plex-livetv-access.md)** | Prove where Live TV is inserted, mine undocumented PMS endpoints from logs, and test plex.tv tuner-access gating. |
| **[how-to/package-test-builds.md](how-to/package-test-builds.md)** | Build cross-platform test bundles for testers (Linux/macOS/Windows). |
| **[how-to/tester-handoff-checklist.md](how-to/tester-handoff-checklist.md)** | Tester handoff checklist (bundle contents, platform expectations, bug report capture). |
| **[how-to/tester-release-notes-draft.md](how-to/tester-release-notes-draft.md)** | Draft tester-facing release notes for current validation builds. |
| **[how-to/cloudflare-bypass.md](how-to/cloudflare-bypass.md)** | Cloudflare-protected providers: UA cycling, cookies, headers, troubleshooting. |
| **[how-to/debug-bundle.md](how-to/debug-bundle.md)** | Collect a shareable `debug-bundle` and analyze with `scripts/analyze-bundle.py`. |
| **[how-to/evidence-intake.md](how-to/evidence-intake.md)** | Standardize real working-vs-failing tester cases under `.diag/evidence/<case-id>/`. |
| **[how-to/connect-plex-to-iptv-tunerr.md](how-to/connect-plex-to-iptv-tunerr.md)** | Plex UI wizard vs **`-register-plex`** vs API; channelmap, **480** limit, empty guide. |
| **[how-to/interpreting-probe-results.md](how-to/interpreting-probe-results.md)** | **`iptv-tunerr probe`**: status strings, **`get.php`** vs **`player_api`**, what to do next. |
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
| **[explanations/architecture.md](explanations/architecture.md)** | Layers, ASCII + Mermaid flow, package map, CLI split (**`cmd_registry`**). |
| **[explanations/project-backlog.md](explanations/project-backlog.md)** | **Open work index:** links epics, **`memory-bank/opportunities.md`**, **`known_issues`**, **`docs-gaps`**, features limits — single entry point for “what’s left.” |
| **[explanations/media-server-integration-modes.md](explanations/media-server-integration-modes.md)** | Where the common tuner story ends and Plex-heavy ops begin. |
| **[explanations/observability-prometheus-and-otel.md](explanations/observability-prometheus-and-otel.md)** | **`/metrics`** and bridging to OpenTelemetry via collector scrape. |
| **[explanations/always-on-recorder-daemon.md](explanations/always-on-recorder-daemon.md)** | Rolling catch-up recorder daemon (`catchup-daemon`): MVP shipped, extensions noted. |
| **[explanations/release-readiness-matrix.md](explanations/release-readiness-matrix.md)** | Feature-family proof table for `./scripts/release-readiness.sh`, binary smoke, and host lanes. |
| **[k8s/README.md](../k8s/README.md)** | Kubernetes deployment (HDHR in cluster). |
| **[epics/](epics/EPIC-template.md)** | Multi-PR epic template. Use with [memory-bank/work_breakdown.md](../memory-bank/work_breakdown.md). |
| **[epics/EPIC-lineup-parity.md](epics/EPIC-lineup-parity.md)** | Optional **Lineup-app-style** track: real HDHomeRun client, web dashboard, SQLite EPG, HLS/fMP4 profiles ([inspiration](https://github.com/am385/lineup)). |
| **[epics/EPIC-operator-completion.md](epics/EPIC-operator-completion.md)** | Umbrella completion lane for all non-admin, non-Postgres follow-through across LP/PM/LH/PAR/ACC/HR/INT/REC/VODX. |
| **[epics/EPIC-programming-manager.md](epics/EPIC-programming-manager.md)** | Category-driven lineup curation, backup groups, quick-add flows, and operator preview UX. |
| **[epics/EPIC-lineup-harvest.md](epics/EPIC-lineup-harvest.md)** | Harvest Plex local-market lineups and bridge them into Programming Manager. |
| **[epics/EPIC-feature-parity.md](epics/EPIC-feature-parity.md)** | Broader ecosystem parity roadmap: downstream outputs, operator workflows, users, and publishing. |
| **product/** · **stories/** | [PRD template](product/PRD-template.md), [STORY template](stories/STORY-template.md). |
| **[features.md](features.md)** | Full feature list. |
| **[CHANGELOG.md](CHANGELOG.md)** | Version and change history. |
| **[docs-gaps.md](docs-gaps.md)** | Known documentation gaps and suggested fixes. |

**Conventions:** [Linking and formatting](_meta/linking.md) · [Glossary](glossary.md).

See also
--------
- [AGENTS.md](../AGENTS.md) and [memory-bank/](../memory-bank/) (**[repo_map.md](../memory-bank/repo_map.md)** for code navigation).
- [README](../README.md) (Kubernetes probe notes, **Recent Changes**, full documentation map).
