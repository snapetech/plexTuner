---
id: expl-always-on-recorder-daemon
type: explanation
status: draft
tags: [catchup, recorder, daemon, future-feature]
---

# Future feature: always-on recorder daemon

The current catch-up stack gives IPTV Tunerr three useful modes:
- guide-derived capsule previews
- launcher/replay publishing into Plex, Emby, and Jellyfin
- on-demand `catchup-record` capture for current in-progress capsules

What it does **not** do yet is run a continuous background recorder that watches the guide, records programmes automatically, and turns them into short-lived replayable library items without operator intervention.

That future feature is the **always-on recorder daemon**.

## What problem it would solve

Some providers expose replay/timeshift URLs. For those, IPTV Tunerr can already build real replay launchers by rendering a replay URL template.

Other providers do not.

For those sources, catch-up is still limited unless IPTV Tunerr records the content itself. An always-on recorder daemon would provide:
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

One of the strategic reasons this future feature matters is that it would not depend on Plex's scheduling UI at all.

If built, IPTV Tunerr could record headlessly according to policy, limited by:
- the provider's real concurrent-stream allowance
- local bandwidth
- CPU if normalization/transcode is involved
- disk IO and storage budget
- the daemon's own max-concurrency policy

So yes, the daemon concept is explicitly about:
- headless rolling capture
- bounded by actual provider/system limits
- not by whether Plex currently has a recording rule for that content

## What the daemon would do

At a high level, it would:
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

## MVP shape

A realistic MVP would be intentionally smaller than a full DVR.

Suggested MVP:
- background scheduler loop every 30-60 seconds
- record only `in_progress` and `starting_soon` items
- record only selected lanes first
- one output file per programme
- small JSON state file or state directory
- finalize and publish on programme stop
- retention sweep on a timer
- targeted media-server refresh

That MVP would already be enough to make non-replay sources feel much more powerful.

## Why this is different from the current stack

Current catch-up behavior:
- capsules are guide-derived
- published items are launcher or replay URLs
- `catchup-record` is operator-invoked and limited to current in-progress capture

Always-on daemon behavior:
- recordings are scheduled and captured automatically
- finished programmes become actual recorded assets
- library items correspond to stored content, not just launchers
- retention is automatic instead of manual

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

## Likely operator surface

If implemented, it would probably want:
- a daemon/worker command, not just a flag on `run`
- a state directory
- a spool/output root
- max concurrent recordings
- lane/channel policy
- retention policy
- disk budget
- publish on/off and media-server target flags

Possible future command shape:

```bash
iptv-tunerr catchup-daemon \
  -catalog ./catalog.json \
  -xmltv http://127.0.0.1:5004/guide.xml \
  -state-dir ./catchup-state \
  -spool-dir ./catchup-spool \
  -publish-dir ./catchup-published \
  -stream-base-url http://127.0.0.1:5004 \
  -lanes sports,general \
  -max-recordings 4 \
  -retention sports=12h,general=24h,movies=72h
```

## Relationship to current features

This feature would build on top of:
- `dna_id`
- Autopilot stream-path knowledge
- provider host penalties
- catch-up capsule curation
- existing library publishing for Plex/Emby/Jellyfin

So it is not a separate product. It is the next deeper layer of the catch-up system.

## Recommended implementation order

If this is ever built, the recommended order is:
1. define recording state schema and spool/finalize layout
2. build a single-process scheduler + worker MVP
3. publish completed recordings through the existing catch-up publisher path
4. add retention sweeps
5. add upstream failover during recording
6. add smarter lane/channel policy

## Current status

Documented only.

The repo currently has:
- `catchup-publish`
- `catchup-record`
- replay-template support

It does **not** yet have:
- an always-on background recorder daemon
- automatic scheduling/finalization
- rolling retention-managed recorded catch-up

See also
--------
- [EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md)
- [features](../features.md)
- [cli-and-env-reference](../reference/cli-and-env-reference.md)
