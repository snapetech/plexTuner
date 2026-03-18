---
id: epic-live-tv-intelligence
type: explanation
status: draft
tags: [epic, product, live-tv, intelligence]
---

# Epic: Live TV Intelligence Layer

IPTV Tunerr already solves hard IPTV-to-Plex problems: provider failover, guide repair, lineup shaping, concurrency handling, and client-specific playback workarounds.

This epic turns those capabilities into a product surface users can feel.

## North star

Make IPTV Tunerr feel less like a tuner bridge and more like a live-TV intelligence layer:
- picks the best stream path
- explains guide confidence
- exposes channel quality clearly
- self-heals common provider/Plex failure modes
- packages channels around intent, not just source ordering

## Non-goals

- Building a full web UI in the first phase
- Replacing Plex's native Live TV UX
- Introducing fuzzy/opaque matching logic with no operator visibility

## Milestones

| Milestone | Done = verifiable outcome |
|-----------|---------------------------|
| M1: Channel intelligence | Operators can export/view channel health, guide confidence, backup-stream depth, and EPG match provenance. |
| M2: Channel DNA | Channel identity survives provider swaps and bad upstream metadata; favorites/remaps churn less across refreshes. |
| M3: Autopilot | Tunerr learns preferred stream/profile/fallback choices by channel and client class. |
| M4: Intent packaging | Dynamic lineups / packs / catch-up capsules turn raw channels into usable experiences. |
| M5: Recovery magic | Plex hidden grabs, upstream caps, and guide degradation are detected and recovered with clear operator output. |

## Story list

| ID | Story | Acceptance |
|----|-------|------------|
| INT-001 | Channel health report | CLI + HTTP report shows score, tier, guide confidence, stream resilience, and actionable next steps per channel. |
| INT-002 | EPG match provenance | Report shows exact `tvg-id` matches vs alias/name repairs vs unmatched channels. |
| INT-003 | Guide confidence policy | Runtime guide quality is surfaced as confidence bands and used by lineup recipes / pruning. |
| INT-004 | Channel DNA store | Stable identity links channels across provider accounts and metadata churn. |
| INT-005 | Autopilot decision memory | Persist best-known stream/profile/fallback choice per channel and client class. |
| INT-006 | Hot-start favorites | Favorite/high-traffic channels get tune-latency optimization. |
| INT-007 | Saved lineup recipes | Built-in recipes like `high-confidence`, `sports-now`, `kids-safe`, `locals-first`. |
| INT-008 | Ghost Hunter | First-class stuck-grab detection and safe recovery for Plex Live TV. |
| INT-009 | Catch-up capsules | Near-live finalized recordings published into Plex libraries as short-lived smart shelves. |
| INT-010 | Provider behavior profiles | Tunerr learns and records provider quirks: concurrency caps, auth context needs, flaky HLS behavior. |

## Suggested PR plan

| PR | Scope |
|----|-------|
| PR-1 | `INT-001` + `INT-002` |
| PR-2 | `INT-003` + early lineup recipe hooks |
| PR-3 | `INT-004` Channel DNA foundations |
| PR-4 | `INT-005` + `INT-006` Autopilot and hot-start experiments |
| PR-5 | `INT-007` intent-packaged lineups |
| PR-6 | `INT-008` Ghost Hunter |
| PR-7 | `INT-009` catch-up capsules |
| PR-8 | `INT-010` provider intelligence and defaults |

## Current status

- Shipped now:
  - `INT-001` channel health report foundation
  - `INT-002` EPG match provenance in the report when XMLTV is supplied
  - early lineup recipes driven by channel intelligence scores (`high_confidence`, `balanced`, `guide_first`, `resilient`)
  - Channel DNA foundation: persisted `dna_id` derived from repaired `TVGID` or normalized channel identity inputs
  - `INT-005` Autopilot memory foundation: optional JSON-backed remembered decisions keyed by `dna_id + client_class`, with successful stream choices reused before generic client adaptation
- Next recommended slices:
  - persist richer match provenance and long-lived cross-provider relationships into a fuller Channel DNA store
  - expand Autopilot from profile/transcode memory into fallback URL/provider selection and hot-start behavior

See also
--------
- [features](../features.md)
- [cli-and-env-reference](../reference/cli-and-env-reference.md)
