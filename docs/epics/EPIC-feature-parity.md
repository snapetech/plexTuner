---
id: epic-feature-parity
type: explanation
status: draft
tags: [epic, parity, roadmap, events, dvr, xtream, multi-user]
---

# EPIC-feature-parity

Close the most important product gaps between IPTV Tunerr and the wider IPTV /
live-TV tooling field without diluting the single-binary/operator-first stance.

## Why this exists

The parity audit surfaced that Tunerr is already strong at ingest, HDHR/Plex
alignment, programming curation, diagnostics, and provider resilience, but it
still trails the best of the field in a few durable product areas:

- event/webhook automation
- shared stream fanout / upstream reuse
- full DVR rule/history experience
- richer client-facing outputs
- multi-user / entitlement models
- virtual channels from owned media
- operator-grade live analytics and control

These should be treated as one coordinated epic so the foundations are shared
instead of reimplemented piecemeal.

## Product stance

- Keep the single-binary deployment model.
- Prefer durable server-side capabilities over thin UI-only tricks.
- Build enabling substrates first, then stack product slices on top.
- Do not claim parity by documentation alone; each slice needs tests and
  operator visibility.

## Story list

| ID | Goal | Acceptance criteria |
|----|------|---------------------|
| PAR-001 | Event/webhook substrate | File-backed webhook config, async delivery, recent-delivery debug surface, and real stream/lineup lifecycle events. |
| PAR-002 | Shared stream fanout foundation | Server can describe and eventually reuse one upstream session for multiple local consumers instead of treating every viewer as a fresh upstream walk. |
| PAR-003 | Live DVR rule model | Server-backed recording rules/history model exists beyond one-shot catch-up actions. |
| PAR-004 | Xtream-compatible output | Tunerr can publish an Xtream-like downstream surface for curated lineups and guide exposure. |
| PAR-005 | Multi-user / entitlement model | Distinct operator/consumer identities and lineup access scopes exist server-side. |
| PAR-006 | Virtual channels from VOD/media | Operators can define synthetic channels from owned media/catalog sources with schedule rules. |
| PAR-007 | Richer live analytics/control | Active-stream state, recent failures, and intervention controls are exposed cleanly to the operator plane. |

## Milestone shape

1. Foundation
   - `PAR-001`
2. Product expansion
   - `PAR-002`
   - `PAR-003`
   - `PAR-007`
3. Publishing and audience model
   - `PAR-004`
   - `PAR-005`
   - `PAR-006`

## Current status

- **2026-03-21:** `PAR-001` started.
  - This is the highest-leverage foundation slice because it unlocks webhook
    automation, richer operator observability, and future recording / output /
    multi-user lifecycle integrations without duplicating transport code.
- **2026-03-21:** `PAR-001` through `PAR-005` and `PAR-007` now have real in-tree slices.
  - Event webhooks, shared HLS relay reuse, recording rules/history, downstream Xtream output, downstream entitlements, and active-stream control all now exist as server-backed features with release-gate coverage.
- **2026-03-21:** `PAR-004` expanded again.
  - `player_api.php` now answers `get_short_epg` and `get_simple_data_table` for both real live channels and virtual channels.
  - Those compact listings come from Tunerr's existing guide and virtual-channel schedule pipeline instead of a separate export path.
- **2026-03-21:** `PAR-004` expanded again.
  - `get.php` now publishes a user-scoped M3U export and `xmltv.php` now publishes a user-scoped XMLTV guide for the same entitled live lineup.
  - This makes the Xtream starter a more complete publishing surface instead of only a `player_api.php` + proxy path.
- **2026-03-21:** `PAR-006` restarted with a durable virtual-channel substrate.
  - `IPTV_TUNERR_VIRTUAL_CHANNELS_FILE` now stores file-backed virtual-channel rules.
  - `/virtual-channels/rules.json` and `/virtual-channels/preview.json` provide durable rules + preview schedules over catalog movies/episodes.
  - This is intentionally a starter slice: preview/schedule design is in, but real published/playable synthetic channels are still the next depth.

## Out of scope

- pretending every parity gap is complete in one patch
- replacing Plex/Emby/Jellyfin UX wholesale
- building a public SaaS control plane
- shipping native role-based auth for every surface before the event and state
  model exists

## See also

- [EPIC-live-tv-intelligence](EPIC-live-tv-intelligence.md)
- [EPIC-lineup-parity](EPIC-lineup-parity.md)
- [EPIC-programming-manager](EPIC-programming-manager.md)
- [project-backlog](../explanations/project-backlog.md)
