---
id: expl-always-on-recorder-daemon
type: explanation
status: stable
tags: [catchup, recorder, daemon, catchup-daemon]
---

# Always-on recorder daemon (MVP shipped)

This document started as a **future-feature** sketch. The first **policy-driven, headless** slice is now shipped as **`iptv-tunerr catchup-daemon`**: it polls the merged guide, schedules `in_progress` / `starting_soon` captures, records concurrently (within `-max-concurrency`), persists state to JSON, spool-then-finalizes `.ts` files, can publish into lane folders with optional Plex/Emby/Jellyfin registration, applies retention and per-lane storage budgets, retries transient failures with backoff (including HTTP `Range` resume on the same `.partial.ts` when supported), and exposes status via `catchup-recorder-report` and `/recordings/recorder.json`.

The catch-up stack still includes:
- guide-derived capsule previews
- launcher/replay publishing into Plex, Emby, and Jellyfin
- on-demand `catchup-record` for current in-progress capsules

**Always-on** here means: **continuous scheduling + capture without per-programme operator clicks**, not “every aspirational bullet below is implemented.” See [Current status](#current-status) for gaps.

## What problem it solves (and what “full vision” adds)

Some providers expose replay/timeshift URLs. For those, IPTV Tunerr can already build real replay launchers by rendering a replay URL template.

Other providers do not.

For those sources, catch-up is still limited unless IPTV Tunerr records the content itself. The daemon addresses that gap; a **full** always-on vision would also provide:
- real rolling catch-up for non-replay sources
- short-lived near-DVR libraries built from live TV automatically
- recent-playback availability after a programme ends
- sports/news/live-event preservation for a retention window

In short, it turns catch-up from:
- "smart packaging around live/replay URLs"

into:
- "guide-driven capture plus packaging"

## How this differs from Plex DVR

Plex DVR records from the Plex side.

An always-on recorder daemon would record from the IPTV Tunerr side.

That sounds similar on the surface, but it changes the operating model.

### Plex DVR

Plex DVR is:
- user-scheduled or rule-driven
- tightly coupled to Plex guide mapping and Plex DVR health
- mainly about "record this show" or "record this series"
- limited to the Plex server that owns the DVR workflow

Plex DVR is good at:
- familiar recording UX
- series rules
- user-managed recordings
- integrated playback/management inside Plex

### Always-on recorder daemon

An always-on recorder daemon would be:
- policy-driven instead of user-rule-driven
- independent of Plex scheduling
- able to record headlessly with no user recording rule
- able to publish the results to Plex, Emby, Jellyfin, or only to disk

It is mainly about:
- "keep recent live content available automatically"
- "build rolling catch-up from live TV"
- "capture according to lane/channel policy"

### Why that matters

The daemon could use IPTV Tunerr's own intelligence layer instead of relying only on media-server scheduling:
- `dna_id` duplicate collapse
- guide-health filtering
- Autopilot memory
- provider host preference
- provider concurrency knowledge
- upstream failover during recording

That means it could do things Plex DVR does not naturally do well:
- record by lane/category without creating explicit per-show rules
- keep a rolling recent-content window
- switch upstreams when one CDN dies mid-recording
- publish the same captured asset to multiple media servers
- operate headlessly up to the provider's real concurrent-stream limit

### Practical difference

Plex DVR says:
- record this programme because a user or rule asked for it

The recorder daemon says:
- continuously maintain recent replayable content for selected live-TV lanes

So the two features are complementary, not interchangeable:
- **Plex DVR** = intentional user-scheduled recording
- **always-on recorder daemon** = rolling catch-up infrastructure

## Headless concurrency model

The daemon does **not** depend on Plex's scheduling UI: it records headlessly according to CLI policy, limited by:
- the provider's real concurrent-stream allowance
- local bandwidth
- CPU if normalization/transcode is involved
- disk IO and storage budget
- the daemon's own max-concurrency policy

So yes, the daemon concept is explicitly about:
- headless rolling capture
- bounded by actual provider/system limits
- not by whether Plex currently has a recording rule for that content

## What the daemon does (high level)

At a high level, the MVP does:
1. watch the merged guide continuously
2. decide which programmes should be recorded
3. start recording before or at programme start
4. keep recording through the programme window
5. finalize metadata when the programme ends
6. publish or refresh media-server library items
7. enforce retention and storage limits
8. recover from stream/provider failures while recording

## What it would look like in the system

The daemon is best thought of as six cooperating pieces.

### 1. Scheduler

The scheduler would read the merged guide and decide which programme windows should become recordings.

Typical responsibilities:
- scan `guide.xml` / cached merged XMLTV on an interval
- detect upcoming, current, and ending programme windows
- match those rows to channel identity (`dna_id`)
- avoid duplicate recordings across duplicate channel variants
- apply recording policy by lane/category/channel

### 2. Recording worker

Each active programme recording would run as a worker.

Typical responsibilities:
- select the best live stream path for the programme
- fetch `/stream/<channel>` or a chosen upstream directly
- segment or write the recording to disk
- survive transient upstream failures
- switch upstreams when a provider/CDN fails mid-recording
- emit progress/state for supervision

### 3. State store

The daemon needs persistent state so it can resume safely after restart.

Likely stored state:
- scheduled recordings
- active recordings
- completed recordings
- failed recordings
- retries and failure reasons
- retention expiry
- published library paths / IDs

### 4. Publisher

Finished recordings need to become useful media-server items.

Responsibilities:
- finalize metadata
- write `.nfo` or richer sidecars
- move content from spool to published layout
- update manifests
- trigger targeted scans/refreshes in Plex, Emby, and Jellyfin

### 5. Retention sweeper

Because this is intended as rolling catch-up, not an infinite archive, storage policy matters.

Responsibilities:
- delete expired recordings
- enforce lane-specific retention
- enforce total disk budget
- prefer pruning oldest/lowest-priority items first

### 6. Policy engine

The daemon must not record everything blindly.

Likely policy inputs:
- lane/category (`sports`, `movies`, `general`, future lane sets)
- channel include/exclude lists
- guide-health quality
- `dna_id` duplicate collapse
- max simultaneous recordings
- disk budget
- sports/news priority
- keep windows like `6h`, `24h`, `72h` by lane

## MVP shape (largely implemented)

The shipped MVP is intentionally smaller than a full DVR. It includes:
- background scheduler loop (`-poll-interval`, commonly 30–60s)
- records `in_progress` and `starting_soon` (`-lead-time`) items
- lane allow/deny lists (`-lanes`, `-exclude-lanes`)
- one output file per programme (`.partial.ts` spool, then final `.ts`)
- JSON state file (`-state-file` or `<out-dir>/recorder-state.json`)
- finalize and optional publish on completion; interrupted items annotated on restart
- retention pruning (global and per-lane counts; per-lane byte budgets)
- optional targeted media-server library create/reuse + refresh (`-register-*`, `-refresh`, `-defer-library-refresh`)

See `iptv-tunerr catchup-daemon -h` and [cli-and-env-reference](../reference/cli-and-env-reference.md).

## Why this is different from the current stack

Catch-up *preview/publish* behavior:
- capsules are guide-derived
- published items are launcher or replay URLs (unless you recorded assets separately)

One-shot recording:
- `catchup-record` is operator-invoked and targets current in-progress capsules

Always-on daemon behavior (`catchup-daemon`):
- recordings are scheduled and captured automatically on a poll loop
- finished programmes become actual recorded assets on disk (and optionally under `-publish-dir`)
- library items can correspond to stored content when publishing + registration are enabled
- retention pruning and budgets run as part of daemon operation

## Hard parts

This is a real subsystem, not a small follow-up.

The hard parts are:
- stream failure recovery during recording
- duplicate suppression across same-`dna_id` variants
- choosing when to switch upstreams without corrupting output
- storage budgeting and expiry
- container/file strategy (`.ts`, fragmented MP4, sidecars, spool/finalize flow)
- avoiding constant full-library rescans
- handling live schedule drift when upstream guide data changes mid-event

## Operator surface

The CLI exposes (see `catchup-daemon`):
- `catchup-daemon` as the long-running worker (distinct from `iptv-tunerr run` / `serve`)
- `-out-dir` for recordings and default state path (`-state-file` overrides)
- `-max-concurrency`, `-poll-interval`, `-lead-time`, lane/channel filters
- retention via `-retain-*` and `-budget-bytes-per-lane`
- `-publish-dir` plus optional `-register-plex|emby|jellyfin` and `-refresh` / `-defer-library-refresh`

Example command shape (see `-h` for the full flag set):

```bash
iptv-tunerr catchup-daemon \
  -catalog ./catalog.json \
  -xmltv http://127.0.0.1:5004/guide.xml \
  -out-dir ./catchup-recordings \
  -publish-dir ./catchup-published \
  -stream-base-url http://127.0.0.1:5004 \
  -lanes sports,general \
  -max-concurrency 4 \
  -retain-completed-per-lane "sports=50,general=100" \
  -budget-bytes-per-lane "sports=20GiB,general=40GiB"
```

## Relationship to current features

This feature would build on top of:
- `dna_id`
- Autopilot stream-path knowledge
- provider host penalties
- catch-up capsule curation
- existing library publishing for Plex/Emby/Jellyfin

So it is not a separate product. It is the next deeper layer of the catch-up system.

## Implementation order (original plan vs today)

Original recommended order:
1. **Done** — recording state schema and spool/finalize layout
2. **Done** — single-process scheduler + worker MVP (`catchup-daemon`)
3. **Done** — publish completed recordings through the catch-up style publisher path (plus optional media-server registration)
4. **Done** — retention pruning and storage budgets (count- and byte-based)
5. **Open** — upstream failover *during* a single capture (multi-URL / provider switch without abandoning the window)
6. **Partially done** — lane/channel policy (`-lanes`, `-channels`, guide policy); richer prioritization remains future work

## Current status

**Implemented and covered by `go test ./...` (via `./scripts/verify`)** for the **recording pipeline** and **CLI wiring**: single-capsule capture (`RecordCatchupCapsule`), spool/finalize behavior, resilient retries and partial HTTP resume, publish-hook construction for the daemon, and related helpers. There is **no** dedicated long-running CI test that drives the full scheduler loop for hours; production confidence still comes from unit tests plus operator soak runs.

**Shipped in the product:**
- `iptv-tunerr catchup-daemon` — continuous guide scan, concurrency limits, persistent state, publish/retention/budgets, transient retries + optional same-spool `Range` resume, capture metrics in state JSON
- `iptv-tunerr catchup-recorder-report` and `/recordings/recorder.json` — observability
- shared primitives with `catchup-record` and replay-template-aware URLs when configured

**Still aspirational / partial relative to this doc’s “full vision”:**
- **Mid-recording upstream switching** (fail over to another provider URL mid-capture without restarting the programme window) — not the same as HTTP retry/`Range` resume on the **same** URL
- **Tight integration** with every intelligence signal listed below (Autopilot, host penalties) **during** capture selection — policy today is guide/capsule/lane/channel oriented
- **Time-based retention strings** like `sports=12h` in one flag — retention is count- and budget-oriented; expiry still ties to programme/capsule semantics
- **Soak/regression harness** for DVR-style multi-hour reliability (see work breakdown `HR-009`-style items)

See also
--------
- [EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md)
- [features](../features.md)
- [cli-and-env-reference](../reference/cli-and-env-reference.md)
