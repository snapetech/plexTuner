---
id: release-readiness-matrix
type: explanation
status: draft
tags: [release, testing, readiness, matrix]
---

# Release readiness matrix

This is the honest answer to "can we prove the shipped features work before a
release?".

The repo now has three proof tiers:

- **Code proof**: unit and focused regression tests
- **Binary proof**: `scripts/ci-smoke.sh` against a real temporary binary
- **Host proof**: bare-metal/platform runs such as macOS WebDAV validation

Not every feature can be host-proven in every environment on every release, but
every release should at least make those gaps explicit.

## Readiness commands

- Baseline release gate:
  - `./scripts/release-readiness.sh`
- Include real macOS host proof:
  - `./scripts/release-readiness.sh --include-mac`
- Include Windows package/build handoff proof:
  - `./scripts/release-readiness.sh --include-windows-package`

## Matrix

| Surface | Unit / focused | Binary smoke | Host / live proof | Current stance |
|---|---|---|---|---|
| Startup contract (`/healthz`, `/readyz`, `guide.xml`, `lineup.json`) | Yes | Yes | Partial | Release-gated |
| HDHR discovery / lineup exposure | Yes | Yes | Partial | Release-gated |
| HLS relay / mux fallback / provider-pressure logic | Yes | Yes | Partial live evidence | Release-gated with stronger fallback proof and known live variance |
| Provider account pooling / rollover | Yes | Yes | Partial live evidence | Release-gated with stronger binary proof |
| Shared HLS relay reuse | Yes | Yes | Not broadly host-proven | Release-gated with stronger binary proof |
| Xtream live / VOD / series output | Yes | Yes | Partial | Release-gated |
| Xtream `get.php` / `xmltv.php` / short EPG | Yes | Yes | Partial | Release-gated |
| Programming Manager recipe / order / backups | Yes | Yes | N/A | Release-gated |
| Programming backup preference | Yes | Yes | N/A | Release-gated |
| Harvest import / assist flows | Yes | Yes | N/A | Release-gated |
| Diagnostics workflows / bounded harness launchers | Yes | Yes | N/A | Release-gated |
| Virtual channels preview / schedule / playback / guide | Yes | Yes | Partial via downstream output | Release-gated |
| WebDAV VOD protocol contract | Yes | Yes | Yes on macOS | Strong |
| macOS VOD/WebDAV client behavior | Harness + diff | N/A | Yes | Host-proven |
| Windows VOD/WebDAV client behavior | Harness shape only | Package prep only | No | Not host-proven |
| Web UI programming / diagnostics lane behavior | Yes | Yes | Partial macOS host proof | Release-gated, still not browser-exhaustive |

## What “good enough to release” means here

A release is in good shape when:

1. `./scripts/release-readiness.sh` passes.
2. Any changed platform-specific surface gets the matching optional host proof
   when the host exists.
3. Known non-host-proven surfaces are called out in release notes instead of
   implied as fully validated.

## What this does not claim

- It does **not** claim every provider/channel/client combination is proven.
- It does **not** claim Windows host validation exists before a real Windows run.
- It does **not** replace the live diagnostics harnesses when a tester reports a
  provider-specific split.

## See also

- [features](../features.md)
- [project-backlog](project-backlog.md)
- [tester-handoff-checklist](../how-to/tester-handoff-checklist.md)
- [mac-baremetal-smoke](../how-to/mac-baremetal-smoke.md)
- [windows-baremetal-smoke](../how-to/windows-baremetal-smoke.md)
