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
  - `INT-001` channel health report foundation (`channel-report`, `/channels/report.json`)
  - `INT-002` EPG match provenance in reports when XMLTV is supplied
  - `INT-003` guide-quality policy: **`IPTV_TUNERR_GUIDE_POLICY`**, **`IPTV_TUNERR_CATCHUP_GUIDE_POLICY`**, **`GET /guide/policy.json`**, catch-up preview summaries
  - Lineup / registration recipes driven by channel intelligence (`high_confidence`, `balanced`, `guide_first`, `resilient`, …)
  - Channel DNA: persisted **`dna_id`**, `/channels/dna.json`, duplicate collapse policies
  - **`INT-005` Autopilot**: JSON state file, remembered transcode/profile/**upstream host** preference, **hot-start** hints, mux **seg** slot bonus, **`/autopilot/report.json`**
  - Ghost Hunter: `ghost-hunter` CLI and `/plex/ghost-report.json`
  - Provider behavior profile: **`/provider/profile.json`** (tuner limits, CF/mux counters, penalized hosts, last mux outcomes)
  - **Operator surfacing (2026-03):** **`GET /provider/profile.json`** includes **`intelligence.autopilot`** (enabled, state file, decision count, top hot channels) beside provider-runtime fields so the deck and scripts see LTV signals in one JSON fetch. Stream-investigate workflow actions link **`/autopilot/report.json`** and **`autopilot-reset`**.
  - **Advisory remediation hints (2026-03):** **`remediation_hints`** on **`/provider/profile.json`** — stable **`code`** / **`severity`** / **`message`** / optional **`env`** rows derived from CF blocks, penalized hosts, concurrency signals, and mux error counters (not automatic config changes).
  - **Autopilot consensus host (2026-03):** optional **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST`** — aggregate **`preferred_host`** agreement across multiple **`dna_id`** rows can steer **`StreamURLs`** for channels without per-DNA memory (thresholds via **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_DNA`** / **`_MIN_HIT_SUM`**); reported on **`/autopilot/report.json`** and **`intelligence.autopilot`**.
- Next recommended slices:
  - Richer **Channel DNA** graph (cross-provider relationships, long-lived match provenance store)
  - **Autopilot**: provider-level / multi-host **policy** memory beyond per-channel **`preferred_url`** / host reranking + consensus host (exact + normalized URL matching for catalog drift is **shipped**)
  - **Ghost Hunter** + **hidden-grab** runbook automation (already scripts/runbooks — tighter product loop)
  - **Provider profile → active remediation** (auto cap / strip hosts beyond today’s autotune hooks and read-only **`remediation_hints`**)
  - **Always-on recorder** for non-replay sources ([catchup-daemon](../explanations/always-on-recorder-daemon.md) extensions)

See also
--------
- [features](../features.md)
- [cli-and-env-reference](../reference/cli-and-env-reference.md)
