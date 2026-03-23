---
id: epic-station-ops
type: explanation
status: draft
tags: [epic, virtual-channels, programming, stations, free-features]
---

# EPIC-station-ops

Build the "run your own TV station" lane as a first-class, fully free part of
IPTV Tunerr.

## Why this exists

Operators are not just asking for one more output format. They want to:

- build channels from local movies, seasons, specials, and episodes
- brand those channels with logos, bugs, and banners
- detect dead/black streams and switch to filler
- manage many stations and many backend servers from one tool
- do all of that without fragmenting the feature set behind artificial tiers

The right product shape is not "another IPTV panel". It is a station operations
layer on top of Tunerr's existing ingest, lineup, programming, virtual-channel,
and media-server integration foundations.

## Product stance

- All station operations features stay free.
- Virtual channels are the first execution substrate, not a sidecar toy.
- Programming Manager remains the curation plane for external live feeds.
- Station Ops grows the owned-media / synthetic-channel / presentation /
  resilience plane on top of that.

## Feature map

### 1. Station definition

Every virtual channel should be able to carry:

- station identity and description
- branding metadata:
  - logo
  - corner bug text/image + position
  - banner text/theme
- recovery metadata:
  - black-screen threshold
  - filler/replacement entries

### 2. Programming and schedule authoring

- choose movies, shows, seasons, specials, and episodes as channel entries
- support day-loop and slot-oriented authoring
- expose preview, current slot, and rolling schedule

### 3. Playback resilience

- detect black screen / dead-air conditions
- inject filler from configured replacement entries
- record the reason and recovery action for operator review

### 4. Presentation layer

- channel bug / overlay metadata
- channel banners / tune-in slates
- publishable logo/icon metadata in M3U/XMLTV/Xtream surfaces

### 5. Fleet operations

- manage many channels and many Tunerr backends
- audit drift between desired station definitions and live runtimes
- eventually coordinate rollout across multiple backend servers

## Story list

| ID | Goal | Acceptance criteria |
|----|------|---------------------|
| STN-001 | Station metadata foundation | Virtual-channel rules support branding/recovery metadata and publish it through rules/detail/export surfaces. |
| STN-002 | Station authoring API | Operators can mutate station branding/recovery policy without hand-editing JSON files. |
| STN-003 | Time-slot builder | Operators can build schedules from local content with slot-oriented helpers instead of raw entry arrays only. |
| STN-004 | Black-screen/filler runtime | The virtual-channel/live pipeline can detect a black/dead source and switch to configured filler content. |
| STN-005 | Presentation overlays | Stream/runtime can render bugs, channel banners, and simple slates using station metadata. |
| STN-006 | Multi-backend station rollout | Operators can inspect, diff, and roll out station definitions across multiple Tunerr backends. |

## Status

- **2026-03-22:** `STN-001` foundation slice shipped.
  - `internal/virtualchannels` now supports station-oriented metadata on each
    channel:
    - `description`
    - `branding.logo_url`
    - `branding.bug_text`
    - `branding.bug_image_url`
    - `branding.bug_position`
    - `branding.banner_text`
    - `branding.theme_color`
    - `recovery.mode`
    - `recovery.black_screen_seconds`
    - `recovery.fallback_entries`
  - Those fields are normalized on load/save, survive file round-trips, and
    now flow through existing virtual-channel publish surfaces:
    - `/virtual-channels/rules.json`
    - `/virtual-channels/channel-detail.json`
    - `/virtual-channels/live.m3u` (`tvg-logo`)
    - `/virtual-channels/guide.xml` (`<icon ...>`)
  - This is intentionally a foundation slice. Runtime black-screen detection,
    filler injection, and overlay rendering are still future `STN-*` stories.
- **2026-03-22:** first `STN-002` / `STN-003` operator-helper slice shipped.
  - Existing virtual-channel APIs no longer require full file replacement for
    basic station editing.
  - `POST /virtual-channels/channel-detail.json` can now update station
    metadata for an existing channel.
  - `POST /virtual-channels/schedule.json` now supports basic authoring helpers:
    `append_entry`, `replace_entries`, `append_movies`, `append_episodes`, and
    `remove_entries`.
  - This is still not the final slot-builder UX; it is the first server-backed
    mutation layer so future deck/UI work can build on something narrower than
    raw JSON file edits.
- **2026-03-22:** `STN-003` now has the first real daily-slot scheduling substrate.
  - Virtual channels can now carry `slots[]` with daily UTC `start_hhmm`,
    `duration_mins`, optional slot labels, and a scheduled entry.
  - Preview/current-slot/schedule surfaces now prefer those slots when present
    instead of only replaying the older raw looping-entry list.
  - `POST /virtual-channels/schedule.json` now supports `append_slot`,
    `replace_slots`, and `remove_slots`, giving the station lane a real
    daypart/time-placement foundation.
- **2026-03-22:** `STN-003` now also has the first daypart filler helper.
  - `POST /virtual-channels/schedule.json` supports `fill_daypart`, which turns
    a start/end window plus movies/episodes/entries into explicit daily slots.
  - This is the first server-backed “fill mornings / fill prime time” style
    helper rather than purely manual slot placement.
- **2026-03-22:** `STN-003` gained the first collection-aware fillers, and `STN-004` has begun in the virtual playback path.
  - `fill_movie_category` can now build a daypart from all indexed movies in a
    chosen category.
  - `fill_series` can now build a daypart from all episodes in a chosen series
    (or a bounded episode subset).
  - Virtual-channel playback now honors `recovery.mode=filler` in one real
    runtime case: if the scheduled slot has no source URL, or the upstream
    fetch fails, Tunerr will attempt the first resolvable fallback entry instead
    of failing immediately.
  - That runtime guard is now slightly deeper too: obviously bad non-media
    responses (for example HTML/text bodies) can also trigger the filler
    fallback instead of being proxied straight through as fake playback.
  - `black_screen_seconds` now has a first runtime meaning too: it acts as a
    startup dead-air timeout for virtual playback, so header-only or stalled
    sources can fall through to filler instead of hanging until the client gives
    up.
- **2026-03-22:** all three requested directions now have a first concrete code path.
  - Content-aware detection: when `ffmpeg` is available, virtual playback can
    run a short `blackdetect` / `silencedetect` probe and switch to filler when
    the sampled source looks black or silent from the start.
  - Filler cutover: that content probe now feeds the same virtual playback
    recovery lane, so filler is no longer limited to missing URLs or transport
    failures.
  - Overlay/slates: virtual channels now expose a rendered
    `/virtual-channels/slate/<id>.svg` surface driven by branding metadata,
    giving the station layer its first real rendered output.
  - There is now also a first composited playback surface:
    `/virtual-channels/branded-stream/<id>.ts` uses ffmpeg to burn branding text
    into the output, while keeping the plain `/virtual-channels/stream/` path
    intact.
- **2026-03-22:** the first runtime-operator depth pass shipped on top of those initial paths.
  - `STN-004`: virtual playback recovery decisions are now inspectable via
    `/virtual-channels/recovery-report.json`, and per-channel detail now includes
    recent recovery events instead of forcing operators to infer fallback from
    response headers alone.
  - `STN-004`: the branded-stream path now uses the same filler/content-probe
    recovery logic as the plain virtual stream path, so branded playback no
    longer bypasses the resilience lane.
  - `STN-005`: branded playback now supports a first image-overlay lane using
    `branding.bug_image_url` or `branding.logo_url` in addition to text/banner
    draws.
- **2026-03-22:** the branded playback lane is now less isolated from normal publish surfaces.
  - `channel-detail.json` now reports `published_stream_url` so operators can
    see which virtual stream surface is actually being exported.
  - `IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT=true` now lets branded virtual
    channels publish `/virtual-channels/branded-stream/<id>.ts` directly in
    `/virtual-channels/live.m3u` instead of forcing the branded path to stay a
    hidden side URL.
- **2026-03-22:** the runtime recovery probe is now slightly closer to the real stream surface.
  - `STN-004`: virtual playback recovery no longer relies only on a separate
    ffmpeg probe against the source URL. The runtime now also samples bytes from
    the actual upstream response body and can trigger filler when those sampled
    bytes probe as black/silent at startup.
  - This is still startup-oriented, not a full continuous live-session media
    monitor, but it is a more faithful signal than URL-only preflight.

## Technical approach

The execution path should converge toward:

`catalog/owned media -> station definition -> schedule builder -> runtime playback/recovery -> publish surfaces`

That keeps authored station identity separate from both raw ingest and
client-specific runtime adaptation.

## Out of scope for this first slice

- actual black-screen detection logic
- actual filler insertion or dead-air replacement
- video overlay rendering
- backend fleet coordination

## See also

- [EPIC-programming-manager](EPIC-programming-manager.md)
- [EPIC-feature-parity](EPIC-feature-parity.md)
- [virtual-channel-stations](../reference/virtual-channel-stations.md)
- [project-backlog](../explanations/project-backlog.md)
- **2026-03-22:** `STN-004` / `STN-005` now have a release-grade first pass, not only metadata.
  - `POST /virtual-channels/channel-detail.json` now supports merge-safe
    `recovery` updates plus `recovery_clear`, so deck/operator changes do not
    wipe existing filler definitions.
  - `/virtual-channels/report.json` now includes recovery posture
    (`recovery_mode`, `black_screen_seconds`, fallback-entry counts), and the
    deck can edit recovery mode and startup threshold directly from station
    cards.
  - Virtual stream recovery now extends beyond startup probes: the plain and
    branded virtual stream paths can perform repeated midstream cutovers across
    the ordered filler chain when the active upstream stalls for the configured
    live-stall window (`IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC`)
    or hard-errors after startup.
  - Recovery now also walks multiple fallback entries in order instead of being
    limited to only the first filler candidate. A broken first filler no longer
    burns the only recovery attempt if a later fallback entry is healthy, and a
    later fallback can still be used if an earlier rescue source also degrades
    mid-session.
  - When the ordered chain is exhausted, the recovery report now records an
    explicit exhausted event instead of leaving operators with only a generic
    timeout symptom.
  - This is still not the end-state media-health engine. The current runtime
    supports repeated stall/error recovery across the configured fallback chain,
    not repeated decoded-video/audio analysis across the entire session.
