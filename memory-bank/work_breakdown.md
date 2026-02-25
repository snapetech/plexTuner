# Work breakdown (multi-PR work only)

**Use this file only when the work is bigger than one PR.** Otherwise use [current_task.md](current_task.md) only.

Create or update this file when **any** of these are true:

- Will take **>1 PR**
- Touches **>~5 modules/areas**
- Multiple stakeholders / UX choices
- Non-trivial migration or rollout
- Meaningful security or perf implications

**Rule:** If the task is multi-PR, you must create/update this (or [docs/epics/EPIC-*.md](../docs/epics/EPIC-template.md)) and **only work on items listed there**. Every task/PR must reference a story ID. Park out-of-scope ideas in [opportunities.md](opportunities.md).

---

## When active: fill the sections below (minimal, high-control)

### North star

- **Goal (2–5 sentences):** Make PlexTuner the "home run" Plex IPTV bridge by outperforming common Threadfin/xTeVe setups on the things users actually feel: stream correctness, client compatibility, lineup/EPG hygiene, resilience, and low overhead. Success means a direct PlexTuner deployment can be dropped into Plex with minimal manual cleanup, stable playback across Plex Web and TV clients, and reliable recordings under normal upstream instability. This epic is the durable product-hardening track; bugfixes should map back to one of these pillars.
- **Non-goals (scope fence):** Building a full UI, replacing Plex's DVR/channel UX, supporting every IPTV provider edge case in one pass, or introducing placeholder/disabled fallback behavior that cannot be exercised in real Plex tests.

### Milestones (vertical slices)

| Milestone | Done = verifiable outcomes |
|-----------|----------------------------|
| M1: Plex Web startup correctness + hygiene defaults | WebSafe path no longer times out on `start.mpd` for the test channel set in Plex Web; built-in EPG-linked/dedupe/stable-ID hygiene defaults produce a low-noise lineup without external preprocessing. |
| M2: Cross-client compatibility + adaptive routing | Tier-1 client matrix (Plex Web, Android TV, Apple TV, at least one additional TV client if available) passes with sticky adaptation and automatic fallback behavior documented and verified. |
| M3: Resilience + recording reliability + overhead policy | Recording soak and upstream flap tests pass with documented retry/backoff behavior; remux-first/per-channel normalization policy is implemented and measured so transcode is used only when required. |

### Story list (granular, scoped)

For each story:

| ID | Acceptance criteria | Files/areas (expected) | Verification | Risk flags |
|----|---------------------|------------------------|--------------|------------|
| HR-001 | **IDR-aware WebSafe startup gate**: WebSafe startup does not release bytes to Plex Web until decodable video is present (or an explicit bounded fallback path is chosen/logged); startup logs include whether release happened with/without IDR and why. | `internal/tuner/gateway.go`, `internal/tuner/*_test.go` | `go test ./internal/tuner`; live `plex-web-livetv-probe.py` on DVR `138` with fresh channels and tuner-log correlation | perf / client-compat |
| HR-002 | **Plex Web startup regression closed for tier-1 sample**: Plex Web probe succeeds (`start.mpd` no timeout) on agreed test channels in direct WebSafe path with real XMLTV and deduped lineup. | `internal/tuner/*`, runtime env docs/memory-bank notes | live probe scripts + PlexTuner logs + PMS log correlation | client-compat / operability |
| HR-003 | **Client compatibility matrix + profiles**: Define tier-1 client profiles and expected stream paths (Web, Android TV, Apple TV, others available); matrix doc + reproducible test procedure exists. | `docs/` (reference/runbook), `internal/tuner/gateway.go` (if code changes), memory-bank | matrix run using real Plex clients; task_history evidence | operability / scope |
| HR-004 | **Sticky adaptation + auto-fallback**: Unknown clients default to WebSafe; known-good clients can use full path; failure on first startup can fall back once and stick per session without oscillation. | `internal/tuner/gateway.go`, `internal/tuner/gateway_test.go` | targeted unit tests + live trial/websafe comparison probes | client-compat / complexity |
| HR-005 | **Built-in lineup/EPG hygiene defaults**: EPG-linked filtering, `tvg-id` dedupe, stable channel IDs, and sane logo handling are available as built-in options (not external preprocessing) and produce aligned lineup/guide counts. | `internal/catalog/*`, `internal/tuner/*`, possibly config/docs | `go test ./...` (relevant pkgs); lineup/guide count checks in live serve; `scripts/verify` | behavior / migration |
| HR-006 | **Stable channel identity policy**: Channel IDs remain stable across refreshes when source ordering changes; Plex remaps do not churn unnecessarily. | catalog generation/normalization code, docs | deterministic fixture tests; compare catalog outputs across shuffled inputs | migration / user-impact |
| HR-007 | **Remux-first / normalization policy**: Per-channel decision policy prefers remux, escalates to normalization/transcode only when needed; policy and overrides are explicit and logged. | `internal/tuner/gateway.go`, config parsing, docs | unit tests on path selection + live sample channels (remux and transcode cases) | perf / client-compat |
| HR-008 | **Upstream flap resilience**: Backoff/retry/health checks keep channels recoverable under transient upstream HLS/HTTP failures without hammering sources; logs make failure mode obvious. | `internal/tuner/gateway.go`, retry helpers, docs | fault injection/manual flap tests; tuner logs show bounded retry/backoff | reliability / provider variance |
| HR-009 | **Recording soak harness + baseline**: At least one repeatable DVR recording soak scenario runs against Plex direct DVRs and captures pass/fail metrics (recording completes, duration sane, no corruption). | `docs/runbooks/*`, helper scripts (external repo refs), memory-bank | repeated Plex record jobs + playback/size checks + logs | operability / long-running |
| HR-010 | **Concurrency/keepalive tuned to Plex reality**: Tuner concurrency and keepalive settings are validated against Plex's actual parallel `Lavf` behavior (including duplicate stream requests) and documented defaults avoid starvation/fake failures. | `internal/tuner/*`, config docs, memory-bank | live probes + concurrent stream tests + log analysis | reliability / perf |

### PR plan

| PR | Scope |
|----|--------|
| PR-1 | `HR-001` + `HR-002` (close Plex Web startup blocker with IDR-aware WebSafe startup and prove via live probes) |
| PR-2 | `HR-003` + `HR-004` (compatibility matrix + sticky adaptation/auto-fallback) |
| PR-3 | `HR-005` + `HR-006` (built-in lineup/EPG hygiene + stable channel identity policy) |
| PR-4 | `HR-007` + `HR-010` (remux-first/per-channel normalization + concurrency/keepalive tuning) |
| PR-5 | `HR-008` + `HR-009` (resilience hardening + recording soak baseline and runbooks) |

### Decision points (needs user input)

- Decision (resolved 2026-02-24): Initial tier-1 client matrix = **LG webOS (Plex app)**, **Plex Web (Firefox + Chrome)**, **iPhone iOS (Plex app)**, **NVIDIA Shield TV (Plex app; covers major Google/Android TV behavior)**. Matrix remains extensible for Apple TV / Roku later.
- Decision: Tier-1 clients for the compatibility matrix → Options: (A) Plex Web + Android TV + Apple TV + Roku/Fire TV if available, (B) Plex Web + whichever devices are physically available now, (C) Web-only first then TVs later → Default if no answer: **B** (test what is available now, keep matrix extensible).
- Decision: WebSafe startup bias (latency vs reliability) → Options: (A) Prefer reliability, wait longer for video IDR before releasing bytes (recommended), (B) Prefer fastest possible startup with bounded fallback even if Web is less reliable, (C) Per-profile tunable defaults → Default if no answer: **A** for WebSafe.
- Decision: Built-in hygiene defaults enabled by default? → Options: (A) EPG-linked + `tvg-id` dedupe on by default when XMLTV remap is enabled (recommended), (B) opt-in flags only, (C) mixed (warn + suggested flags) → Default if no answer: **A** for WebSafe profile, opt-in for Trial/full path.

---

## When not active

Leave this file as-is. Use **current_task.md** as the single plan for PR-sized work.
