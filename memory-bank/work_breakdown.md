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
- **LP progress (2026-03-21):** **LP-012** [lineup-parity-lp012-closure](../docs/how-to/lineup-parity-lp012-closure.md) checklist + doc index wiring; **LP-001** slice: **`IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS`** for directed UDP discovery (**IPv6 literal targets + UDP6** merged with IPv4). **LP-010/011** now include named-profile **ffmpeg-packaged HLS** (`output_mux: "hls"`) in addition to `mpegts` / `fmp4`. **LTV slice:** Autopilot **normalized URL** match (trailing slash / default port / scheme·host casing) for **`preferred_url`** vs catalog. **Next depth:** packager tuning/soak, not missing product mode. **Park:** Postgres, **catchup-daemon**-scale “always-on” extensions beyond MVP, provider **`xmltv.php`** bandwidth when panels omit HTTP validators (disk cache + incremental SQLite already shipped) — [opportunities.md](opportunities.md). **Docs (2026-03-19):** [docs-gaps.md](../docs/docs-gaps.md) high/medium rows cleared after validation; optional Mermaid in [architecture.md](../docs/explanations/architecture.md) remains nice-to-have.

### Active epic overlay (Account-aware concurrency + VOD ergonomics, 2026-03-21)

- **Goal (2–5 sentences):** Turn multi-provider catalogs into real concurrent capacity by leasing streams against provider-account identities instead of treating extra accounts as passive failover only, and finish the first operator-usable non-Linux VOD parity path with mount ergonomics. Success means operators can point several accounts at Tunerr and get predictable spread across them, while macOS/Windows users have a documented, mountable `vod-webdav` workflow from the same binary.
- **Non-goals (scope fence):** Solving every provider-specific concurrency contract in one pass, rewriting the entire gateway selection engine, or shipping native macFUSE/WinFsp backends before the shared VOD/WebDAV story is solid.
- **Story IDs:** `ACC-001` … `ACC-003`, `VODX-003`
- **ACC progress (2026-03-21):** `ACC-001`, `ACC-002`, and `ACC-003` are now in: the gateway tracks active provider-account leases, prefers lower-load accounts during URL ordering, can reject locally when every distinct account for a channel is already at `IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT`, learns tighter per-account caps from real upstream `423` / `458` / `509`-style signals, persists those learned caps across restarts via `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE`, and expires stale learning with `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS`. `/provider/profile.json` and `/debug/runtime.json` now surface both the learned-account state and the persistence knobs.

### Active epic overlay (Programming Manager planning, 2026-03-21)

- **Doc:** [docs/epics/EPIC-programming-manager.md](../docs/epics/EPIC-programming-manager.md)
- **Goal:** Build a real server-backed channel-builder / programming-manager surface with category-first selection, per-channel include/exclude, saved lineup order, and exact-match backup grouping.
- **Non-goals:** Replacing Plex’s own channel UI, doing every fuzzy match heuristic in one pass, or shipping a public internet-facing admin portal.
- **Story IDs:** `PM-001` … `PM-009`
- **Planning status (2026-03-21):** Epic defined from tester requirements plus external UX patterns (Channels collections and xTeVe/XEPG mapping/order control).
- **Progress (2026-03-21):** `PM-001` and `PM-002` foundation slice shipped. `internal/programming` now builds category inventory from the raw post-intelligence lineup and persists a durable recipe file (`IPTV_TUNERR_PROGRAMMING_RECIPE_FILE`). Tuner endpoints `/programming/categories.json`, `/programming/recipe.json`, and `/programming/preview.json` are live, and `Server.UpdateChannels` now applies the programming recipe before final lineup shaping/cap logic.
- **Progress (2026-03-21):** `PM-003`, `PM-004`, and the first visible `PM-005` slice are now in. `/programming/categories.json` supports bulk category include/exclude/remove mutations, `/programming/channels.json` supports exact channel include/exclude/remove mutations, and `order_mode: "recommended"` now applies the requested server taxonomy order. Next depth: `PM-006` manual drag/order UX refinement, `PM-007` exact-match backup grouping, then `PM-008` deck UI.
- **Progress (2026-03-21):** `PM-006` and `PM-007` backend slice are now in. `/programming/order.json` supports durable server-side manual order mutations, `/programming/backups.json` reports strong exact-match sibling groups, and `collapse_exact_backups: true` can collapse those same-channel siblings into one visible row with merged backup streams before final lineup exposure. Next depth: `PM-008` deck UI and `PM-009` broader release-grade regression coverage around refresh survival/operator flows.
- **Progress (2026-03-21):** `PM-008` deck UI is now in. The dedicated `internal/webui` control deck has a real Programming lane with category inventory cards, exact include/exclude controls, manual order nudges from the preview lineup, backup-group inspection, and recipe/order/collapse toggles wired to the existing server APIs. `./scripts/verify` and the binary smoke lane are green. Next depth is `PM-009`: broader regression coverage around refresh survival and end-to-end operator flows.
- **Progress (2026-03-21):** first visible `PM-009` coverage is now in. Tuner tests prove saved recipe mutations survive `UpdateChannels` refresh churn, and `scripts/ci-smoke.sh` now restarts `serve` against a reshuffled catalog while reusing the same recipe file so curated lineup shape, custom order, and backup collapse are asserted across process restarts too. Remaining depth for `PM-009` is higher-level operator/browser flow automation, not missing refresh persistence coverage.

### Active epic overlay (Plex lineup harvest, 2026-03-21)

- **Doc:** [docs/epics/EPIC-lineup-harvest.md](../docs/epics/EPIC-lineup-harvest.md)
- **Goal:** Turn the old Plex wizard-oracle experiments into a real operator feature for sweeping tuner lineup caps/shapes, discovering which lineup titles Plex maps back, and eventually feeding those results into Programming Manager decisions.
- **Non-goals:** scraping Plex UI, pretending lineup-market guessing is fully automatic, or leaving the feature as ad hoc lab output only.
- **Story IDs:** `LH-001` … `LH-006`
- **Progress (2026-03-21):** `LH-001` through `LH-003` starter shipped. `internal/plexharvest` now owns reusable target expansion, bounded channelmap polling, per-target result capture, and deduped lineup summaries; `iptv-tunerr plex-lineup-harvest` is the named CLI surface; and docs/how-to plus CLI reference are in place. Next depth: persist harvested reports and bridge them into Programming Manager or deck workflows.

### Active epic overlay (Feature parity, 2026-03-21)

- **Doc:** [docs/epics/EPIC-feature-parity.md](../docs/epics/EPIC-feature-parity.md)
- **Goal:** Close the biggest remaining product gaps versus the wider IPTV / DVR / virtual-channel tool field without abandoning Tunerr’s single-binary/operator-first shape.
- **Non-goals:** Pretending every parity gap is complete in one patch, replacing Plex outright, or shipping a public SaaS control plane.
- **Story IDs:** `PAR-001` … `PAR-007`
- **Progress (2026-03-21):** `PAR-001` foundation slice shipped: event/webhook substrate, lifecycle events, and debug/runtime exposure are now in. `IPTV_TUNERR_EVENT_WEBHOOKS_FILE` loads a JSON hook list, Tunerr emits lineup/stream lifecycle events, and `/debug/event-hooks.json` plus `/debug/runtime.json` expose the current state.
- **Progress (2026-03-21):** first visible `PAR-002` slice shipped too: same-channel duplicate consumers can now attach to one live HLS Go-relay session instead of always starting another upstream walk, and `/debug/shared-relays.json` exposes the current shared sessions plus subscriber counts.
- **Progress (2026-03-21):** first visible `PAR-007` slices shipped too: `/debug/active-streams.json` now exposes in-flight stream sessions, `/ops/actions/stream-stop` can cancel them by request ID or channel ID, and account pooling now falls back to Xtream path credentials when per-stream auth metadata is missing so multi-account rollover works against real `/live/<user>/<pass>/...` URLs instead of collapsing to the default account.
- **Progress (2026-03-21):** visible `PAR-004` slices shipped too: optional read-only downstream Xtream output now exists via `IPTV_TUNERR_XTREAM_USER` / `IPTV_TUNERR_XTREAM_PASS`, serving live + VOD + series `player_api.php` actions and Tunerr-owned `/live/`, `/movie/`, and `/series/` proxy paths on top of the curated lineup and catalog.
- **Progress (2026-03-21):** first visible `PAR-005` slice shipped too: `IPTV_TUNERR_XTREAM_USERS_FILE` now enables file-backed downstream users with scoped live/VOD/series access, `/entitlements.json` exposes or updates that ruleset from the operator plane, and both `player_api.php` and `/live|movie|series/...` now filter or deny output by authenticated user instead of treating Xtream output as one global catalog.
- **Progress (2026-03-21):** Programming Manager follow-up for tester tooling shipped too: `/programming/channel-detail.json` now gives category-first or curses-style clients a focused channel view with taxonomy/category metadata, exact-match backup alternatives, and a 3-hour upcoming-programme preview; release smoke asserts both the new detail endpoint and the expanded Xtream VOD/series surface.
- **Progress (2026-03-21):** first visible `PAR-003` slice shipped too: `IPTV_TUNERR_RECORDING_RULES_FILE` now enables durable server-side recording rules, `/recordings/rules.json` CRUD, `/recordings/rules/preview.json` over current catch-up capsules, and `/recordings/history.json` classification of recorder-state activity against the current ruleset. `scripts/ci-smoke.sh` now validates recorder-rule mutation and history output in the release gate.

### Account-aware concurrency story list (2026-03-21)

| ID | Acceptance criteria | Files/areas (expected) | Verification | Risk flags |
|----|---------------------|------------------------|--------------|------------|
| ACC-001 | Gateway derives a stable provider-account identity per stream URL and tracks active leases for successful live sessions. | `internal/tuner/gateway*.go`, tests | targeted gateway tests + `./scripts/verify` | runtime regression |
| ACC-002 | URL ordering/account selection prefers less-loaded accounts and can enforce a per-account concurrent-stream cap without breaking single-account behavior. | `internal/tuner/gateway*.go`, docs/runtime/profile surfaces | targeted gateway tests + `./scripts/verify` | behavior / compatibility |
| ACC-003 | Operator/runtime surfaces expose account-pool state clearly enough to debug “10 accounts should mean 10 viewers”. | `internal/tuner/server.go`, `cmd_runtime_server.go`, docs/tests | targeted tests + `./scripts/verify` | operability |

### Active epic overlay (Cross-platform VOD parity, 2026-03-21)

- **Goal (2–5 sentences):** Replicate Linux VODFS behavior on macOS and Windows without insisting on the same kernel/filesystem stack. Success means Tunerr can expose the VOD catalog as a mountable `Movies/` / `TV/` tree on all supported desktop/server OSes, with on-demand file reads and Plex-scannable paths, while Linux keeps the existing `go-fuse` mount path.
- **Non-goals (scope fence):** Rewriting the current Linux VODFS stack in one shot, shipping cgo-heavy macFUSE/WinFsp backends before there is a stable shared VOD tree layer, or pretending WebDAV is identical to kernel-native FUSE semantics in every corner case.
- **Story IDs:** `VODX-001` … `VODX-005`
- **Initial implementation stance:** Linux stays on `internal/vodfs` `go-fuse`; macOS/Windows parity starts with a cross-platform WebDAV mount surface and native mount helpers, then native per-OS backends can layer later if still needed.
- **Progress (2026-03-21):** `VODX-001` and the first visible `VODX-002` slice are in: naming/tree logic now lives in cross-platform files under `internal/vodfs`, Linux `Root` uses the shared `Tree`, and `internal/vodwebdav` + `iptv-tunerr vod-webdav` now expose the same synthetic `Movies/` / `TV/` tree over read-only WebDAV for non-Linux parity. `VODX-003` has its first operator slice too: `iptv-tunerr vod-webdav-mount-hint` prints platform-specific mount commands and `vod-webdav` now logs concrete mount commands at startup. **Validation depth landed:** unit tests now cover `OPTIONS`, directory/file `PROPFIND`, file `HEAD`, byte-range `GET`, and clean `405` rejection for mutation methods; binary smoke exercises live WebDAV `OPTIONS`, root `PROPFIND`, `PROPFIND /Movies`, movie + episode `HEAD`, movie + episode range reads, and read-only `PUT` rejection through a real cached materializer path backed by a local HTTP asset server; `scripts/vod-webdav-client-harness.sh` replays Finder/WebDAVFS and Windows MiniRedir request sequences into `.diag/vod-webdav-client/` bundles with a report; `scripts/vod-webdav-client-diff.py` now compares both status and key response headers; `k8s/vod-webdav-client-macair-job.yaml` is ready as a node-targeted run path once `macair-m4` is `Ready`; a real macOS host run passed with no status or header drift versus the local baseline; `scripts/mac-baremetal-smoke.sh` automates the whole darwin cross-build + Wake-on-LAN + remote `serve`/web UI/VOD smoke loop from Linux; and `scripts/windows-baremetal-package.sh` now prepares a matching Windows package with a PowerShell smoke runner. Next depth: actual Windows host runs plus fixes for any true client-specific drift they expose.

### Cross-platform VOD parity story list (2026-03-21)

| ID | Acceptance criteria | Files/areas (expected) | Verification | Risk flags |
|----|---------------------|------------------------|--------------|------------|
| VODX-001 | Shared VOD naming/tree generation is no longer trapped behind Linux build tags and can feed more than one backend. | `internal/vodfs/*`, new shared package if needed, tests | `go test ./internal/vodfs ./...`; `./scripts/verify` | maintainability / regression |
| VODX-002 | A cross-platform WebDAV VOD surface exposes the same `Movies/` and `TV/` tree with on-demand reads from the materializer. | new `internal/vodwebdav/*`, `cmd/iptv-tunerr/*`, tests/docs | targeted tests + `./scripts/verify` | behavior / interoperability |
| VODX-003 | macOS and Windows mount helper flows exist so operators can mount the WebDAV surface natively from the same binary/operator docs. | `cmd/iptv-tunerr/*`, docs/how-to/reference, tests where practical | targeted tests + docs + `./scripts/verify` | operability |
| VODX-004 | Linux `mount` and current VODFS behavior remain intact while sharing the extracted tree/naming layer. | `internal/vodfs/*`, tests | `go test ./internal/vodfs`; `./scripts/verify` | regression |
| VODX-005 | Docs, features matrix, and platform requirements clearly describe Linux FUSE vs macOS/Windows WebDAV parity and operator tradeoffs. | `README.md`, `docs/*`, memory-bank | docs review + `./scripts/verify` | docs drift |

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

### Progress notes

#### Integration / intelligence + recorder — status audit (2026-03-19)

Verification: repo scan + `./scripts/verify` green. Treat stories below as **met unless you reopen scope** (e.g. stricter “no `http.DefaultClient` anywhere”).

- **INT-001 (done):** **`internal/guideinput`** + **`refio`** back guide/XMLTV load paths; wired from **`cmd_catalog`**, **`cmd_guide_reports`**, **`cmd_report_support`**, **`internal/tuner/guide_health.go`**. **`internal/materializer`** and **`Server`** loopback stream self-fetch use **`internal/httpclient`** when no injected client. Residual **`http.DefaultClient`**: mostly tests / intentional mux negative cases (e.g. **`gateway_test.go`**).
- **INT-002 (done):** **`internal/tuner/guide_policy.go`**, **`GET /guide/policy.json`**, **`GuidePolicySummary`** on catch-up preview, tests (**`TestServer_UpdateChannelsGuidePolicy`**, **`TestServer_catchupCapsulesGuidePolicy`**, etc.).
- **INT-003 / INT-004 (done):** **`UpdateChannels`** → **`applyGuidePolicyToChannels`** via **`IPTV_TUNERR_GUIDE_POLICY`**; catch-up CLI/publish/daemon use **`IPTV_TUNERR_CATCHUP_GUIDE_POLICY`** / **`-guide-policy`** with **`FilterCatchupCapsulesByGuidePolicy`**.
- **INT-005 (done):** **`main.go`** ~100 lines (dispatcher only); **`cmd_registry.go`** aggregates **`coreCommands`**, **`reportCommands`**, **`guideReportCommands`**, **`catchupOpsCommands`**, …; **`cmd_util.go`** shared helpers; **`TestAllCommandsUniqueAndSectioned`**.
- **INT-006 (done):** **`gateway.go`** = **`Gateway`** struct + keys; **`gateway_servehttp.go`** = **`ServeHTTP`** + tuner slot + failure surfacing; **`gateway_stream_upstream.go`** = **`walkStreamUpstreams`** (upstream loop + dispatch); **`gateway_mux_ratelimit.go`** = mux segment rate + outcome counters helpers. Other **`gateway_*.go`** peers unchanged (relay, HLS/DASH, adapt, …). Optional: split inside **`gateway_stream_upstream.go`** if merge conflicts concentrate there.
- **INT-007 (done — acceptance):** **`IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE`** / **`-replay-url-template`**; **`ReplayURL`** on capsules; publish/record/daemon honor replay vs launcher; tests **`catchup_replay_test.go`**, server capsule tests. Further “true replay” product depth = new scope.

### Recorder track — status audit (2026-03-19)

- **REC-001 (done — MVP):** **`iptv-tunerr catchup-daemon`** + **`internal/tuner/catchup_daemon.go`**, state file, concurrency, lane/channel filters, retry/resume flags, tests **`catchup_daemon_test.go`**.
- **REC-002 (done — MVP):** lane allow/deny, channel allow/deny (`channel_id`, `guide_number`, `dna_id`, name), replay honesty via **`ReplayURL`** vs live **`/stream/`**; resilient record tests.
- **REC-003 (done — MVP):** **`recorded-publish-manifest.json`** (**`catchup_record_publish.go`**), publish dir layout, optional Plex/Emby/Jellyfin registration hooks on daemon.

#### Home run (HR) — slice log
- **HR-010 (slice):** Shared **`internal/httpclient`** transport now documents Plex/Lavf parallel-segment behavior; new env **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS`**, **`IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC`**; **`parseSharedTransportEnv`** unit tests; **`/debug/runtime.json`** echoes **`tuner.http_max_idle_conns`** + **`tuner.http_idle_conn_timeout_sec`**. Reference: **`docs/reference/plex-livetv-http-tuning.md`**.
- **HR-009 (slice):** Runbook **§9** adds a **DVR recording soak baseline** checklist (short record, size, playback, logs).
- **HR-008 (slice):** Runbook + **`plex-livetv-http-tuning`** document live-path **primary→backup** failover (no hot-path backoff) vs **`seg=`** diagnostics.
- **HR-007 (slice):** **`IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`** now layers on **`off`/`on`/`auto`** (per-channel remux or transcode vs global mode), not only **`auto_cached`**; policy decisions logged as **`gateway: transcode policy ...`**; tests in **`gateway_policy_test.go`**; **`/debug/runtime.json`** includes override file paths; docs **cli-and-env**, **plex-livetv-http-tuning**, **`.env.example`**, **README**.
- **HR-006 (slice):** **`catalog.ReplaceWithLive`** sorts **`live_channels`** in place by **`channel_id`** (then **guide_number**, **guide_name**) so catalog saves and lineup iteration stay deterministic when M3U order drifts.
- **HR-005 (slice):** **`docs/reference/lineup-epg-hygiene.md`** maps built-in hygiene; final **`dedupeByTVGID`** after free-source + HDHR merges; **`IPTV_TUNERR_DEDUPE_BY_TVG_ID`** opt-out; runtime **`tuner.dedupe_by_tvg_id`** echo; tests **`cmd_catalog_dedupe_test.go`**.
- **HR-004 (slice):** **`IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK`** + **`_TTL_SEC`** + **`_LOG`**; map **`channel_id` + Plex session/client** → WebSafe until expiry when adaptation used non-WebSafe and status is **`all_upstreams_failed`** or **`upstream_concurrency_limited`**; **`requestAdaptation`** reason **`sticky-fallback-websafe`**; code **`internal/tuner/gateway_adapt_sticky.go`** + defer hook in **`gateway.go`**; tests **`gateway_test.go`**; runtime **`tuner.client_adapt_sticky_*`**; docs **`plex-livetv-http-tuning`**, **cli-and-env**, **CHANGELOG**.
- **HR-003 (slice):** **`docs/reference/plex-client-compatibility-matrix.md`** — tier-1 clients (Web Firefox+Chrome, LG webOS, iOS, Shield/Android TV proxy), adaptation **client class** table vs expected **`gateway:`** reasons, **`go test -run 'TestGateway_requestAdaptation_|TestGateway_adaptSticky_'`** recipe, optional external **`plex-web-livetv-probe.py`** note, manual matrix table; **docs/index**, **reference/index**, **glossary** **`client class`**, **runbook §10**, **CHANGELOG**, **plex-livetv-http-tuning** see-also.
- **HR-002 (slice):** **HR-002** subsection in **`plex-client-compatibility-matrix.md`** — deployment-fillable DVR/channel template, probe pass criteria, Tunerr **`startup-gate … release=`** correlation, PMS log slice; cross-links **HR-001**. **`scripts/live-race-harness.sh`** now accepts optional **`PWPROBE_SCRIPT`** / **`PWPROBE_ARGS`** and bundles probe JSON/log/exit code; **`live-race-harness-report.py`** summarizes those artifacts when present.
- **HR-001 (slice):** WebSafe ffmpeg prefetch **sliding window** + **`release=`** log; **H.264 IDR** + **HEVC IRAP (NAL 16–21)** via **`containsAnnexBVideoKeyframe`**; **`trimTSHeadToMaxBytes`**; **`WEBSAFE_STARTUP_MAX_FALLBACK_WITHOUT_IDR`**; **`tuner.websafe_*`** runtime echo; tests (**trim**, **HEVC**, **H.264**); **cli** / **plex-livetv** / runbook §6; **`live-race-harness-report.py`**.
- **INT-005 (slice):** **`cmd_util.go`** shared helpers; thin **`main.go`** (see audit above).

### Decision points (needs user input)

- Decision (resolved 2026-02-24): Initial tier-1 client matrix = **LG webOS (Plex app)**, **Plex Web (Firefox + Chrome)**, **iPhone iOS (Plex app)**, **NVIDIA Shield TV (Plex app; covers major Google/Android TV behavior)**. Matrix remains extensible for Apple TV / Roku later.
- Decision: Tier-1 clients for the compatibility matrix → Options: (A) Plex Web + Android TV + Apple TV + Roku/Fire TV if available, (B) Plex Web + whichever devices are physically available now, (C) Web-only first then TVs later → Default if no answer: **B** (test what is available now, keep matrix extensible).
- Decision: WebSafe startup bias (latency vs reliability) → Options: (A) Prefer reliability, wait longer for video IDR before releasing bytes (recommended), (B) Prefer fastest possible startup with bounded fallback even if Web is less reliable, (C) Per-profile tunable defaults → Default if no answer: **A** for WebSafe.
- Decision: Built-in hygiene defaults enabled by default? → Options: (A) EPG-linked + `tvg-id` dedupe on by default when XMLTV remap is enabled (recommended), (B) opt-in flags only, (C) mixed (warn + suggested flags) → Default if no answer: **A** for WebSafe profile, opt-in for Trial/full path.

---

## When not active

Leave this file as-is. Use **current_task.md** as the single plan for PR-sized work.
