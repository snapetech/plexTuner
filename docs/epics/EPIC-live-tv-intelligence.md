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
  - **`INT-005` Autopilot**: JSON state file, remembered transcode/profile/**upstream host** preference, **hot-start** hints (**`IPTV_TUNERR_HOT_START_CHANNELS`**, Autopilot hit counts, **`IPTV_TUNERR_HOT_START_GROUP_TITLES`** vs M3U **`group_title`**), mux **seg** slot bonus, **`/autopilot/report.json`**
  - Ghost Hunter: `ghost-hunter` CLI and `/plex/ghost-report.json`
  - Provider behavior profile: **`/provider/profile.json`** (tuner limits, CF/mux counters, penalized hosts, last mux outcomes)
  - **Operator surfacing (2026-03):** **`GET /provider/profile.json`** includes **`intelligence.autopilot`** (enabled, state file, decision count, top hot channels) beside provider-runtime fields so the deck and scripts see LTV signals in one JSON fetch. Stream-investigate workflow actions link **`/autopilot/report.json`** and **`autopilot-reset`**.
  - **Advisory remediation hints (2026-03):** **`remediation_hints`** on **`/provider/profile.json`** — stable **`code`** / **`severity`** / **`message`** / optional **`env`** rows derived from CF blocks, penalized hosts, concurrency signals, and mux error counters (not automatic config changes).
  - **Autopilot consensus host (2026-03):** optional **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST`** — aggregate **`preferred_host`** agreement across multiple **`dna_id`** rows can steer **`StreamURLs`** for channels without per-DNA memory (thresholds via **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_DNA`** / **`_MIN_HIT_SUM`**); reported on **`/autopilot/report.json`** and **`intelligence.autopilot`**.
- Next recommended slices (still open — validated 2026-03-19):
  - Richer **Channel DNA** graph (cross-provider relationships, long-lived match provenance store beyond current **`dna_id`** + reports)
  - **Autopilot**: provider-level / multi-host **policy** memory — **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`** (comma hostnames; **`reorderStreamURLs`** after per-DNA memory, before consensus) **shipped**; richer cross-provider **policy file** / automatic host strips beyond **`STRIP_STREAM_HOSTS`** remain future
  - **Ghost Hunter**: deeper product loop is now partially shipped via localhost/LAN operator actions (**`ghost-visible-stop`**, guarded hidden-grab dry-run/restart helper); remaining depth is richer evidence correlation and stronger automated recovery policy
  - **Provider profile → active remediation:** optional **host quarantine** (**`IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE`**) skips repeatedly failing upstreams when backups exist. **Observability shipped:** **`upstream_quarantine_skips_total`** on **`/provider/profile.json`**, Prometheus **`iptv_tunerr_upstream_quarantine_skips_total`** (with **`IPTV_TUNERR_METRICS_ENABLE`**), and Control Deck (**`internal/webui/deck.js`**) summary + watch/Routing meta. **Further work:** automatic strip / cap beyond quarantine (policy); **`remediation_hints`** remain advisory read-only.
  - **Always-on recorder** extensions for non-replay sources beyond **[catchup-daemon](../explanations/always-on-recorder-daemon.md)** MVP (**`catchup-daemon`** already schedules + records + publish hooks)

See also
--------
- [Project backlog index](../explanations/project-backlog.md) (open work across epics, opportunities, known issues)
- [features](../features.md)
- [cli-and-env-reference](../reference/cli-and-env-reference.md)
