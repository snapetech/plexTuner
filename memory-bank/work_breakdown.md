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

- **Goal (2–5 sentences):** Make IptvTunerr the "home run" Plex IPTV bridge by outperforming common Threadfin/xTeVe setups on the things users actually feel: stream correctness, client compatibility, lineup/EPG hygiene, resilience, and low overhead. Success means a direct IptvTunerr deployment can be dropped into Plex with minimal manual cleanup, stable playback across Plex Web and TV clients, and reliable recordings under normal upstream instability. This epic is the durable product-hardening track; bugfixes should map back to one of these pillars.
- **Non-goals (scope fence):** Building a full UI, replacing Plex's DVR/channel UX, supporting every IPTV provider edge case in one pass, or introducing placeholder/disabled fallback behavior that cannot be exercised in real Plex tests.

### Active epic overlay (2026-03-18)

- **Goal (2–5 sentences):** Consolidate the new intelligence layer so it feeds runtime behavior instead of staying report-only, while also paying down the biggest structural hotspots created by rapid feature growth. Success means guide quality can drive lineup and catch-up decisions, shared file/URL loading is consistent across runtime and tooling, and the next refactors can land on smaller, cleaner seams.
- **Non-goals (scope fence):** Shipping a full UI, building a real timeshift recorder in one pass, or pretending near-live catch-up is already true replay without an actual replay source.

### Active epic overlay (recorder track, 2026-03-19)

- **Goal (2–5 sentences):** Turn the catch-up/recording path from a one-shot operator tool into a policy-driven subsystem that can automatically capture current and upcoming programmes across as many supported feeds as the provider/system budget allows. Success means Tunerr can schedule and persist recordings headlessly, dedupe duplicate channel variants, and produce real recorded assets instead of only launcher/replay links for non-replay sources.
- **Non-goals (scope fence):** Shipping a full archive/DVR product in one patch, implementing every future retention/publisher heuristic immediately, or claiming gapless perfect mid-stream failover across all providers without proving it.

### Active epic overlay (Lineup-app parity, 2026-03-19) — user approved “yes to all”

- **Doc:** [docs/epics/EPIC-lineup-parity.md](../docs/epics/EPIC-lineup-parity.md) (inspired by [am385/lineup](https://github.com/am385/lineup)).
- **Goal:** Add optional **real HDHomeRun client** ingestion, **operator web dashboard**, **SQLite-backed EPG** persistence with incremental fetch/cleanup, and **named HLS/fMP4 transcoding profiles** — without replacing Tunerr’s IPTV core or single-binary model.
- **Non-goals:** Rewriting in .NET, full upstream Lineup feature parity in one PR, or replacing Plex Live TV UX.
- **Story IDs:** `LP-001` … `LP-012` — work only on stories listed in that epic file; park extras in [opportunities.md](opportunities.md).

### Active story list (2026-03-18)

| ID | Acceptance criteria | Files/areas (expected) | Verification | Risk flags |
|----|---------------------|------------------------|--------------|------------|
| INT-001 | Shared file/URL loading lives in one internal helper and report/guide tooling stop using ad-hoc `http.DefaultClient` code paths. | `internal/*`, `cmd/iptv-tunerr/*`, `internal/tuner/*` | targeted `go test` + `./scripts/verify` | maintainability / operability |
| INT-002 | Guide quality policy exists as a reusable runtime decision surface that can classify channels as healthy/placeholder-only/no-programme and feed other flows from cached guide state. | `internal/tuner/*`, new `internal/*` helper package if needed | `go test ./internal/tuner ...`; `./scripts/verify` | reliability / behavior |
| INT-003 | Lineup shaping can optionally use guide-quality policy so operators can prefer or require channels with real programme coverage. | `internal/tuner/server.go`, config/docs/tests | `go test ./internal/tuner`; `./scripts/verify` | behavior / migration |
| INT-004 | Catch-up publishing can optionally suppress weak channels/capsules using guide-quality policy instead of publishing every preview row. | `internal/tuner/*`, `cmd/iptv-tunerr/*`, docs/tests | targeted tests + `./scripts/verify` | product behavior |
| INT-005 | CLI flag construction is no longer centralized in one 900+ line `main.go`; command registration follows the same concern split as execution. | `cmd/iptv-tunerr/*` | `go test ./cmd/iptv-tunerr`; `./scripts/verify` | maintainability |
| INT-006 | `internal/tuner/gateway.go` is decomposed into smaller concern-focused files without changing public behavior. | `internal/tuner/gateway*.go` | `go test ./internal/tuner`; `./scripts/verify` | maintainability / regression |
| INT-007 | Catch-up publishing distinguishes clearly between near-live launchers and true replay, and a real replay mode is only introduced behind an explicit source-backed path. | `internal/tuner/*`, docs/tests | targeted tests + docs updates | product / scope |
| REC-001 | A new recorder command can continuously scan guide capsules, schedule eligible captures, dedupe duplicate variants, record multiple items concurrently up to a configured limit, and persist a JSON state file. | `cmd/iptv-tunerr/*`, `internal/tuner/*`, docs/tests | targeted tests + `./scripts/verify` | behavior / storage / long-running |
| REC-002 | Recorder policy can filter by lane/channel and avoid duplicate recordings across `dna_id` variants while staying honest about replay-vs-recorded assets. | `internal/tuner/*`, docs/tests | targeted tests + docs updates | product behavior |
| REC-003 | Recorder output can be finalized/published cleanly with retention-ready manifests and enough metadata to feed later media-server automation. | `internal/tuner/*`, docs/tests | targeted tests + `./scripts/verify` | operability / scope |

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
| HR-002 | **Plex Web startup regression closed for tier-1 sample**: Plex Web probe succeeds (`start.mpd` no timeout) on agreed test channels in direct WebSafe path with real XMLTV and deduped lineup. | `internal/tuner/*`, runtime env docs/memory-bank notes | live probe scripts + IptvTunerr logs + PMS log correlation | client-compat / operability |
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

### Active PR plan (2026-03-18)

| PR | Scope |
|----|--------|
| PR-A | `INT-001` + `INT-002` + `INT-003` + `INT-004` (shared loader + guide-quality policy + lineup/catch-up integration) |
| PR-B | `INT-005` (CLI flag/registration split) |
| PR-C | `INT-006` (gateway decomposition) |
| PR-D | `INT-007` (explicit replay-mode boundary and any source-backed true replay work) |
| PR-E | `REC-001` (policy-driven recorder daemon MVP) |
| PR-F | `REC-002` + `REC-003` (policy refinement + finalize/publish/retention wiring) |

### Progress notes (2026-03-18)

- **INT-001 (in progress, begin→end lane):** Added shared **`internal/guideinput`** helpers for provider XMLTV URL generation plus local-file / URL loading of guide XML, XMLTV channels, alias overrides, and optional match reports using the repo’s shared transport defaults. Current callers rewired: guide report tooling, catch-up preview helpers, and tuner guide-health/report paths.
- **HR-010 (slice):** Shared **`internal/httpclient`** transport now documents Plex/Lavf parallel-segment behavior; new env **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS`**, **`IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC`**; **`parseSharedTransportEnv`** unit tests; **`/debug/runtime.json`** echoes **`tuner.http_max_idle_conns`** + **`tuner.http_idle_conn_timeout_sec`**. Reference: **`docs/reference/plex-livetv-http-tuning.md`**.
- **HR-009 (slice):** Runbook **§9** adds a **DVR recording soak baseline** checklist (short record, size, playback, logs).
- **HR-008 (slice):** Runbook + **`plex-livetv-http-tuning`** document live-path **primary→backup** failover (no hot-path backoff) vs **`seg=`** diagnostics.
- **HR-007 (slice):** **`IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`** now layers on **`off`/`on`/`auto`** (per-channel remux or transcode vs global mode), not only **`auto_cached`**; policy decisions logged as **`gateway: transcode policy ...`**; tests in **`gateway_policy_test.go`**; **`/debug/runtime.json`** includes override file paths; docs **cli-and-env**, **plex-livetv-http-tuning**, **`.env.example`**, **README**.
- **HR-006 (slice):** **`catalog.ReplaceWithLive`** sorts **`live_channels`** in place by **`channel_id`** (then **guide_number**, **guide_name**) so catalog saves and lineup iteration stay deterministic when M3U order drifts.

- `INT-007` now has a shipped first slice:
  - catch-up capsules/publishing accept `IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE`
  - replay mode is explicit in preview/publish outputs
  - without a replay template, the system stays in launcher mode instead of pretending live URLs are historical recordings

### Decision points (needs user input)

- Decision (resolved 2026-02-24): Initial tier-1 client matrix = **LG webOS (Plex app)**, **Plex Web (Firefox + Chrome)**, **iPhone iOS (Plex app)**, **NVIDIA Shield TV (Plex app; covers major Google/Android TV behavior)**. Matrix remains extensible for Apple TV / Roku later.
- Decision: Tier-1 clients for the compatibility matrix → Options: (A) Plex Web + Android TV + Apple TV + Roku/Fire TV if available, (B) Plex Web + whichever devices are physically available now, (C) Web-only first then TVs later → Default if no answer: **B** (test what is available now, keep matrix extensible).
- Decision: WebSafe startup bias (latency vs reliability) → Options: (A) Prefer reliability, wait longer for video IDR before releasing bytes (recommended), (B) Prefer fastest possible startup with bounded fallback even if Web is less reliable, (C) Per-profile tunable defaults → Default if no answer: **A** for WebSafe.
- Decision: Built-in hygiene defaults enabled by default? → Options: (A) EPG-linked + `tvg-id` dedupe on by default when XMLTV remap is enabled (recommended), (B) opt-in flags only, (C) mixed (warn + suggested flags) → Default if no answer: **A** for WebSafe profile, opt-in for Trial/full path.

---

## When not active

Leave this file as-is. Use **current_task.md** as the single plan for PR-sized work.
