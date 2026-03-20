---
id: how-to-index
type: reference
status: stable
tags: [how-to, index]
---

# How-to (task-oriented)

Goal → preconditions → steps → verify.

| Doc | Description |
|-----|-------------|
| [first-push](first-push.md) | Add remote and push (GitHub / GitLab / self-hosted). |
| [deployment](deployment.md) | Deploy IPTV Tunerr: binary, Docker, systemd; local QA/smoke script. |
| [fix-guide-data-with-epg-doctor](fix-guide-data-with-epg-doctor.md) | Diagnose and fix bad guide data, placeholder-only channels, and weak XMLTV matches. |
| [deploy-and-connect-plex-home](deploy-and-connect-plex-home.md) | Deploy IPTV Tunerr in-cluster and connect Plex at plex.home (one-command deploy, zero-touch or manual add). |
| [package-test-builds](package-test-builds.md) | Build cross-platform test bundles (Linux/macOS/Windows) for binary + supervisor testing. |
| [tester-handoff-checklist](tester-handoff-checklist.md) | Final handoff checklist for sending tester bundles and collecting useful bug reports. |
| [tester-release-notes-draft](tester-release-notes-draft.md) | Draft release notes / tester-facing summary for current validation builds. |
| [cloudflare-bypass](cloudflare-bypass.md) | Cloudflare-protected IPTV: UA cycling, cookies, clearance, operator knobs. |
| [debug-bundle](debug-bundle.md) | Collect `iptv-tunerr debug-bundle` and analyze with `scripts/analyze-bundle.py`. |
| [hybrid-hdhr-iptv](hybrid-hdhr-iptv.md) | Physical HDHomeRun + IPTV: lineup merge, guide.xml, optional SQLite caps. |
| [lineup-parity-lp012-closure](lineup-parity-lp012-closure.md) | **LP-012** operator checklist (parity epic doc sweep). |
| [multi-stream-harness](multi-stream-harness.md) | Reproduce two-stream collapse: staggered live pulls, snapshots, **`multi-stream-harness-report.py`**. |
| [hls-mux-proxy](hls-mux-proxy.md) | HLS playlist proxy through Tunerr (`?mux=hls`), absolute URL option. |

See also
--------
- [Docs index](../index.md) · [features](../features.md) · [CHANGELOG](../CHANGELOG.md).
- [Runbooks](../runbooks/index.md).
- [Reference](../reference/index.md).
