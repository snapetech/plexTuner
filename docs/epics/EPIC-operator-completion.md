---
id: epic-operator-completion
type: explanation
status: in_progress
tags: [epic, roadmap, hardening, parity, operator]
---

# EPIC: Operator Completion (except public admin plane and Postgres)

## Why this exists

Tunerr is now well beyond MVP: HDHomeRun, Programming Manager, stream hardening,
provider intelligence, and parity surfaces are all in-progress or shipped.  
This epic consolidates every remaining non-excluded operator-facing gap so execution
doesn’t fragment across LP/PAR/PM/LH/HR tracks anymore.

Primary tracking policy:

- Use this epic as the single backlog owner for all in-scope, non-excluded operator
  work until the end of this completion tranche.
- Track every active follow-through item here as `CMP-*` slices (`LP-*`/`PAR-*`/`PM-*`/`LH-*`/`HR-*`/`INT-*`/`ACC-*`/`REC-*`/`VODX-*` lineage) so PR-level references stay consistent.

This is the "all-in completion lane." It is not a replacement for epics that already
ship stable pieces; it is the set of follow-through slices needed to turn those pieces
into a finished operator product.

## Explicit scope fence

- **Included:** all active backlog from existing product epics that is not already shipped,
  plus adjacent reliability/ops proof gaps needed to make it dependable.
- **Excluded by design:** public admin SaaS/control-plane productization and Postgres/shared
  writer migration (unless a hard requirement for multi-instance architecture emerges).

## Goal

Ship a consistent completion bundle where:
- stream resilience is predictable under real provider/client stress,
- lineup/guide/identity is stable across refreshes and multi-source merges,
- parity outputs are complete and coherent for operator and downstream clients,
- and every meaningful behavior is covered by verifiable unit/binary/host checks.

## Completion map

This epic intentionally maps to existing tracks:

- `LP-*` and `ACC-*` stories that are already shipped stay as prerequisites.
- The follow-up slices below continue `PAR-*`, `PM-*`, `LH-*`, `HR-*`, `INT-*`,
  and `VODX-*` where they are still incomplete for day-to-day operator confidence.
- Open operational risks from `known_issues.md` are pulled in when they are product-impacting
  and testable.

## Milestones

| Milestone | Definition of done |
|---|---|
| M1 — Runtime behavior closure | All remaining reliability and compatibility defects from HR/known-issues have automated proof and a verified fallback path. |
| M2 — Parity completion | Remaining Parity/Programming/Lineup follow-on stories are closed so there are no silent “mostly done” dead-ends. |
| M3 — Ops automation closure | Harnesses move from shell-first to in-app actionable workflows with evidence summaries. |
| M4 — Platform proof & release proof | macOS/Windows proof lanes and release packaging remain green with explicit gaps called out (not hidden). |
| M5 — Data quality guardrails | Identity, guide-linking, and recipe behavior are stable under merges, refresh churn, and provider-account variation. |

## Story list

| ID | Area | Acceptance criteria | Files / areas (expected) | Verification |
|---|---|---|---|---|
| CMP-001 | Stream failure resilience | Provider/playlist errors from quota/backoff (423/429/458/509) are retried with bounded exponential fallback before full stream abort, and no single failing URL can cause global stalls. | `internal/tuner/*`, `internal/tuner/gateway*.go`, tests | `go test ./internal/tuner`; fault-injection harness + binary smoke |
| CMP-002 | Client compatibility closure | Tier-1 Plex paths (`Web`, `Android TV`, `Apple TV`/`TV` variant) remain demonstrably stable or have explicit, documented fallback behavior with no blind acceptance. | `docs/reference/plex-client-compatibility-matrix.md`, `internal/tuner/*`, `scripts/live-race-harness.sh` | matrix runbook + harness report + reproducible live probe corpus |
| CMP-003 | Startup/hidden-state resilience | Hidden grab/pending-session edge cases include an operator-visible, auditable workflow; no manual Plex restart needed for every failure class. | `internal/tuner/server.go`, `/ops/workflows`, `internal/tuner/gateway_*.go`, `internal/webui/deck.js` | unit smoke + command integration + regression transcript |
| CMP-004 | Identity/lineup stability | Channel identity remains stable across merges and refreshes (`tvg_id`/`dna_id`/provider identity dedupe); lineup and guide mapping do not collapse unrelated variants (especially same provider `tvg_id` collisions). | `internal/catalog/*`, `internal/programming/*`, `internal/tuner/*`, `internal/tuner/gateway_mux_ratelimit.go` | unit tests + catalog/lineup diff smoke + regression fixtures |
| CMP-005 | Virtual channel maturation | `PAR-006` is completed for production usage: virtual channels move from preview-only capability to durable playback/publish behavior with predictable scheduling and restart semantics. | `internal/tuner/*`, `internal/tuner/catchup_*`, `internal/programming/*`, `internal/webui/deck.js` | acceptance tests + end-to-end stream smoke + documentation examples |
| CMP-006 | Recorder semantics boundary | Catch-up publication behavior is explicitly labeled as launcher-vs-replay where true source-backed replay is unavailable; UI/docs and emitted outputs match behavior. | `internal/tuner/catchup_*`, `internal/tuner/catchup_record_publish.go`, docs | unit tests + release smoke fixtures |
| CMP-007 | Autonomy-aware recorder policy | Extend catch-up/recording behavior from manual-only paths to policy-led lanes, queue controls, and retention defaults that operators can tune safely. | `internal/tuner/catchup_daemon.go`, `internal/tuner/catchup_record_publish.go`, docs | recorder tests + smoke + policy validation |
| CMP-008 | Lineup/guide quality-by-default | Guide health and lineup policy defaults are consistently applied to both active playlist publishing and helper outputs where no explicit override exists. | `internal/tuner/guide_policy.go`, `internal/tuner/server.go`, `internal/catalog/*`, `docs/reference/lineup-epg-hygiene.md` | regression tests + catalog/guide diff checks |
| CMP-009 | Diagnostics-to-operator bridge | In-app workflows can trigger bounded `stream-compare` / `channel-diff` and ingest `.diag` bundles with a latest-verdict summary card. | `internal/webui/*`, `internal/tuner/*`, `scripts/*.sh`, `/ops/actions` | deck workflow tests + harness + verify |
| CMP-010 | HDHR+provider merge hardening | Merge/precedence logic for hybrid lineup and provider catalogs has deterministic conflict policy under edge cases (dup names, same ID, same source family). | `internal/hdhomerun/*`, `internal/catalog/*`, `internal/tuner/server.go` | merge fixtures + integration tests + k8s/manual smoke |
| CMP-011 | HLS mux edge cases | Close first-priority mux/documented cases in `hls-mux-toolkit` (non-URI attributes, protocol-edge handling, security behavior) that materially affect production reliability. | `internal/tuner/gateway_hls.go`, `docs/reference/hls-mux-toolkit.md`, tests | fixture matrix + replay stream assertions |
| CMP-012 | Provider XMLTV optimization | Reduce full refresh pain when provider offers partial/incremental contracts: configurable bounded/suffix refresh still avoids churn and keeps guide freshness. | `internal/tuner/epg_pipeline.go`, `internal/tuner/server.go`, `docs/reference/cli-and-env-reference.md` | cache probe tests + runtime profile |
| CMP-013 | Cross-platform VOD parity closure | macOS and Windows WebDAV parity remain production-safe; known differences are documented, tested, and included in release-readiness checklists. | `internal/vodwebdav/*`, `scripts/vod-webdav-client-*`, docs | host runs + harness diff + release readiness run |
| CMP-014 | Windows-host parity for streaming/clients | Add Windows host streaming + integration lane for native `serve` and critical live playback parity (no `wine`-only false positives). | `.github/*`, `scripts/windows-baremetal-*`, release notes, `docs/how-to/windows-baremetal-smoke.md` | Windows host-run evidence + CI package checks |
| CMP-015 | Emby/Jellyfin registration readiness | Registration attempts do not race server readiness; registration failures are actionable and recover automatically. | `internal/register*`, `cmd/iptv-tunerr/*` | unit + integration + release smoke regression |
| CMP-016 | Per-account control and visibility | Operator can reason about account limits (learned + manual caps), inspect them in `/provider/profile.json`, and validate rollover under contention without guesswork. | `internal/tuner/*`, `cmd_runtime.go`, docs | account-contest tests + binary race smoke |
| CMP-017 | Catalog/lineup churn safeguards | Refresh/cascade reordering doesn’t silently churn visible lineup; stable IDs and collapsed-source behavior remain deterministic between restarts. | `internal/catalog/catalog.go`, `internal/programming/*`, `internal/tuner/server.go` | deterministic fixture diff tests + restart smoke |
| CMP-018 | Release/readiness hardening | Release matrix includes every currently implemented surface, marks non-proof lanes explicitly, and runs pre-tag with no manual exception drift. | `memory-bank/commands.yml`, `scripts/release-readiness.sh`, `scripts/verify` | command-level assertions + release audit |

## PR plan

| PR | Scope |
|---|---|
| PR-1 | `CMP-001` + `CMP-002` (failure resilience + client matrix closure) |
| PR-2 | `CMP-003` + `CMP-004` + `CMP-009` (ops diagnostics + identity stability) |
| PR-3 | `CMP-005` + `CMP-006` + `CMP-007` (virtual channels + catch-up semantics/policy) |
| PR-4 | `CMP-008` + `CMP-010` + `CMP-017` (guide/lineup-policy hardening + merge stability) |
| PR-5 | `CMP-011` + `CMP-012` (mux edge + XMLTV optimization) |
| PR-6 | `CMP-013` + `CMP-014` + `CMP-015` (cross-platform and host proof) |
| PR-7 | `CMP-018` + `CMP-016` (release automation + account policy observability) |

## Success criteria before close

- No known "partially done" or "mostly done" slices remain in the core roadmap with user-facing ambiguity.
- Every remaining story here has acceptance tests and at least one binary or host proof lane.
- `docs/explanations/project-backlog.md` and `memory-bank/*` are aligned to this epic as the source of the completion plan.
- Public admin-plane and Postgres remain explicitly on backlog and not pulled in without explicit decision.

## Decision log

- **Postgres shared EPG:** defer to backlog unless shared write-path architecture is required.
- **Public admin plane:** defer; deck + local operator workflows are in scope only.
- **Windows host proof:** required before release claims for Windows parity; if unavailable, claims must remain explicit and not over-broad.

## Cross links

- [EPIC-lineup-parity](EPIC-lineup-parity.md)
- [EPIC-programming-manager](EPIC-programming-manager.md)
- [EPIC-lineup-harvest](EPIC-lineup-harvest.md)
- [EPIC-feature-parity](EPIC-feature-parity.md)
- [EPIC-live-tv-intelligence](EPIC-live-tv-intelligence.md)
- [project backlog](../explanations/project-backlog.md)
