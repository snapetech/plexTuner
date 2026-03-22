---
id: epic-lineup-harvest
type: explanation
status: draft
tags: [epic, plex, lineup, guide, programming-manager]
---

# EPIC-lineup-harvest

Turn the existing Plex "oracle" experiments into a real operator feature for
discovering useful cable/market guide lineups and feeding them back into
Programming Manager decisions.

## Why this exists

Tunerr already knows how to:

- register synthetic HDHR tuners into Plex
- vary lineup caps/shapes
- force Plex to resolve a guide lineup
- read back channel-map results

Right now that power is buried in ad hoc lab commands. The tester request is to
productize it:

- probe many lineup variants quickly
- discover which market/provider lineups Plex offers for a given location
- surface the good candidates instead of hiding them in logs
- eventually reuse those results when building curated/local-heavy lineups

## Product stance

- Reuse the existing Plex API and DVR registration primitives.
- Treat lineup harvest as an operator tool first, not a magical background
  feature.
- Capture structured reports, not just console logs.
- Keep harvested results composable with Programming Manager and lineup recipes.

## Story list

| ID | Goal | Acceptance criteria |
|----|------|---------------------|
| LH-001 | Reusable harvest engine | Existing oracle logic is extracted into a reusable package with structured results and tests. |
| LH-002 | Named CLI feature | `plex-lineup-harvest` exists as a first-class command with cap/template expansion, polling, and JSON report output. |
| LH-003 | Lineup summary view | Harvest results dedupe discovered lineup titles and summarize the strongest matches by channel-map count. |
| LH-004 | Persistent report / recipe bridge | Operators can save harvest results and later pick them up from Programming Manager or other tooling. |
| LH-005 | Control-deck lane | The dedicated web UI can launch/view harvest reports and show candidate lineups by title/strength. |
| LH-006 | Local-market recipe assist | Harvested lineup/title/channel-map hints can seed lineup recipes geared toward local broadcast or region-specific bundles. |

## Status

- **2026-03-21:** `LH-001` through `LH-003` starter shipped in-tree.
  - `internal/plexharvest` now owns reusable target expansion, polling, per-target result capture, and deduped lineup summaries.
  - `iptv-tunerr plex-lineup-harvest` is now the first named feature surface for this flow, instead of leaving it under the old oracle-only lab command.
  - Reports now capture lineup titles, lineup URLs, channel-map counts, activation counts, and per-target failures in structured JSON.
- **2026-03-21:** `LH-004` and the first visible `LH-005` slice shipped in-tree.
  - Harvest reports can now be saved and reloaded from `IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE`.
  - `/programming/harvest.json` exposes the saved report to operator tooling, and `/programming/preview.json` now embeds `harvest_ready`, `harvest_file`, and deduped harvested lineup summaries.
  - The dedicated deck Programming lane now surfaces those harvested candidate lineups instead of leaving the data as one-shot CLI output only.
- **2026-03-21:** `LH-006` starter shipped in-tree.
  - Harvest probe results now capture the harvested `lineup.json` rows alongside lineup titles/URLs.
  - `/programming/harvest-import.json` can preview or apply a chosen harvested lineup as a real Programming Manager recipe by matching harvested lineup rows back onto the current raw channel set.
  - This is the first real bridge from “Plex found a useful market lineup” to “Tunerr can turn that lineup into a saved curation rule.”

## Out of scope

- claiming every market can be guessed automatically without operator input
- scraping Plex web UI
- replacing Programming Manager with harvested lineups

## See also

- [EPIC-programming-manager](EPIC-programming-manager.md)
- [EPIC-lineup-parity](EPIC-lineup-parity.md)
