---
id: howto-tester-release-notes-draft
type: how-to
status: draft
tags: [how-to, release-notes, testing]
---

# Tester release notes (draft)

Use this as a handoff note for testers validating the recent Plex Live TV/DVR fixes and packaging changes.

## Summary

This build significantly improves Plex Live TV/DVR behavior for multi-DVR IPTV setups and adds a packaged test workflow for Linux/macOS/Windows.

Highlights:
- fixed multi-DVR guide collisions in Plex clients (distinct guides per DVR)
- restored stable playback after guide remap/channelmap rebuilds
- added single-app supervisor mode (many tuners in one process/container)
- added built-in Plex stale-session reaper (optional)
- improved HDHR wizard-lane shaping controls and HDHR metadata signaling
- added cross-platform tester package + handoff bundle scripts

## What testers should focus on

### Live TV / DVR behavior

- Can Plex load distinct guides for different DVR sources (no duplicate-guide/tab collisions)?
- Can you tune channels reliably after guide refreshes/remaps?
- Does Plex Web playback start and keep audio/video in sync on common channels?
- Do closed tabs / switched-away clients leave stale sessions running too long?

### Multi-DVR / supervisor behavior

If testing a supervisor build (single app managing multiple child tuners):
- Are all expected DVRs present in Plex?
- Do category DVRs stay distinct (lineup, guide, playback)?
- Does the HDHR wizard lane coexist with injected DVRs?

### Packaging / platform behavior

- Does the binary start on your OS without extra dependencies?
- Do `run`, `serve`, `probe`, and `supervise` work?
- Linux only: does `mount` / VODFS work if FUSE is installed?
- Windows/macOS: confirm core tuner behavior; report HDHR network discovery results if tested natively

## Notable changes in this test build

### Plex behavior / playback

- Added per-instance guide-number offsets to avoid cross-DVR guide collisions in Plex clients.
- Added tooling and runbooks for Plex stale session / hidden-grab recovery.
- Added optional built-in Plex session reaper in the app (Plex-side session based, not raw socket based).

### HDHR and lineup behavior

- Added richer HDHR `discover.json` metadata controls for wizard lanes.
- Added `ScanPossible` control so category tuners can be de-emphasized in HDHR setup flows.
- Added lineup shaping/filtering controls (wizard-safe caps, music/radio drops, region/profile ordering).

### XMLTV / guide behavior

- Added optional XMLTV language/Latin preference normalization and non-Latin title fallback.

### Packaging and docs

- Added cross-platform package builder and staged tester handoff bundle builder.
- Added CLI/env reference and Plex DVR lifecycle/API reference docs.

## Known limitations (current)

- `mount` / VODFS is Linux-only.
- Plex wizard path cannot be forced to pre-check only a subset of channels in a larger HDHR lineup; serve the desired subset directly.
- Windows HDHR network mode compiles again, but native Windows validation is still preferred over `wine` smoke tests.

## What to include in bug reports

Please include:
- OS + architecture (e.g. Linux amd64, macOS arm64, Windows amd64)
- Plex client type (Web/Chrome, Firefox, LG/webOS, Apple TV, etc.)
- Exact channel and approximate timestamp
- Whether this was HDHR wizard lane or injected DVR lane
- Relevant env/config snippets (redact secrets)
- Logs if available (`plex-tuner` logs and Plex server logs)

See also:
- [tester-handoff-checklist](tester-handoff-checklist.md)
- [cli-and-env-reference](../reference/cli-and-env-reference.md)
- [plex-dvr-lifecycle-and-api](../reference/plex-dvr-lifecycle-and-api.md)
