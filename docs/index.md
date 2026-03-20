---
id: docs-index
type: reference
status: stable
tags: [docs, index]
---

# Docs index

Where to find what. This repo uses the [Diátaxis](https://diataxis.fr/) split by reader need.

| Section | Purpose |
|--------|--------|
| **[tutorials/](tutorials/index.md)** | Learning-oriented: get started, first run. |
| **[how-to/](how-to/index.md)** | Task-oriented: first push, add remote. |
| **[reference/](reference/index.md)** | Facts: commands, config. |
| **[explanations/](explanations/index.md)** | Why and concepts. Add project docs here. |
| **[adr/](adr/index.md)** | Decision log: architecture decision records. |
| **[runbooks/](runbooks/index.md)** | Operational procedures. |
| **[how-to/deployment.md](how-to/deployment.md)** | Deploy IPTV Tunerr (binary, Docker, systemd, local test script). |
| **[how-to/plex-ops-patterns.md](how-to/plex-ops-patterns.md)** | Advanced Plex-only operating patterns: zero-touch, category DVR fleets, injected DVRs. |
| **[how-to/package-test-builds.md](how-to/package-test-builds.md)** | Build cross-platform test bundles for testers (Linux/macOS/Windows). |
| **[how-to/tester-handoff-checklist.md](how-to/tester-handoff-checklist.md)** | Tester handoff checklist (bundle contents, platform expectations, bug report capture). |
| **[how-to/tester-release-notes-draft.md](how-to/tester-release-notes-draft.md)** | Draft tester-facing release notes for current validation builds. |
| **[how-to/cloudflare-bypass.md](how-to/cloudflare-bypass.md)** | Cloudflare-protected providers: UA cycling, cookies, headers, troubleshooting. |
| **[how-to/debug-bundle.md](how-to/debug-bundle.md)** | Collect a shareable `debug-bundle` and analyze with `scripts/analyze-bundle.py`. |
| **[how-to/hybrid-hdhr-iptv.md](how-to/hybrid-hdhr-iptv.md)** | Merge HDHR `lineup.json` + `guide.xml` with IPTV index and `/guide.xml`. |
| **[reference/cli-and-env-reference.md](reference/cli-and-env-reference.md)** | Commands, flags, and key env vars (including supervisor/testing knobs). |
| **[reference/transcode-profiles.md](reference/transcode-profiles.md)** | Gateway ffmpeg profile names, HDHR-style aliases, `?profile=` usage. |
| **[explanations/media-server-integration-modes.md](explanations/media-server-integration-modes.md)** | Where the common tuner story ends and Plex-heavy ops begin. |
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
- [AGENTS.md](../AGENTS.md) and [memory-bank/](../memory-bank/).
- [README](../README.md).
