---
id: virtual-channel-stations
type: reference
status: stable
tags: [reference, virtual-channels, stations]
---

# Virtual channel station metadata

`IPTV_TUNERR_VIRTUAL_CHANNELS_FILE` can now describe a virtual channel as a
station definition, not just a flat loop of entries.

## Channel fields

Each channel may include:

| Field | Purpose |
|-------|---------|
| `description` | Operator-facing description of the station. |
| `branding.logo_url` | Logo used in exported M3U (`tvg-logo`) and synthetic XMLTV (`<icon>`). |
| `branding.bug_text` | On-screen bug text used by the branded stream/slate surfaces. |
| `branding.bug_image_url` | On-screen bug image used by the branded stream/slate surfaces. |
| `branding.bug_position` | One of `bottom-right`, `bottom-left`, `top-right`, `top-left`. Invalid values normalize to `bottom-right`. |
| `branding.banner_text` | Channel banner/slate text used by branded stream/slate surfaces. |
| `branding.theme_color` | Theme hint used by the rendered slate surface and deck/operator views. |
| `branding.stream_mode` | Optional publish override: `plain`, `branded`, or empty/`auto`. `plain` forces `/virtual-channels/stream/<id>.mp4`, `branded` forces `/virtual-channels/branded-stream/<id>.ts`, and `auto` defers to the process-wide default. |
| `recovery.mode` | `filler` or empty/`none`. Current runtime only supports filler-style cutover. |
| `recovery.black_screen_seconds` | Threshold in seconds before filler/recovery should trigger. Defaults to `2`. |
| `recovery.fallback_entries` | Ordered replacement entries used by the runtime recovery lane. Uses the same `movie` / `episode` entry schema as main channel programming. |
| `slots[]` | Optional daily UTC slot schedule. When present, preview/current-slot/schedule surfaces use these slots instead of the older plain loop-entry order. |

## Slot fields

Each `slots[]` row may include:

| Field | Purpose |
|-------|---------|
| `start_hhmm` | Daily UTC start time in `HH:MM` 24-hour format. |
| `duration_mins` | Slot duration in minutes. Defaults to `30`. |
| `label` | Optional operator-facing title override for the slot. |
| `entry` | The movie/episode entry played in that slot. |

## Current publish surfaces

These fields are already visible in:

- `/virtual-channels/rules.json`
- `/virtual-channels/channel-detail.json`
- `/virtual-channels/report.json`
- `/virtual-channels/live.m3u`
- `/virtual-channels/guide.xml`
- `/virtual-channels/schedule.json`

Current behavior:

- `branding.logo_url` is exported today.
- `/virtual-channels/recovery-report.json` now exposes recent runtime recovery
  events so filler cutovers and probe-triggered recoveries are inspectable
  without scraping headers or logs.
- `channel-detail.json` now also reports `published_stream_url`, which is the
  actual stream URL currently being published for that virtual channel.
  - recovery metadata is now partly executed on the virtual-channel playback path:
  when `recovery.mode` is `filler` and the scheduled asset has no source URL or
  the upstream fetch fails, Tunerr will try the first resolvable
  `recovery.fallback_entries[]` item as a filler fallback.
- the same fallback now also triggers for obviously bad upstream responses on
  the virtual-channel path, such as HTML/JSON/text bodies or empty first-read
  payloads where a media response was expected.
- `recovery.black_screen_seconds` now also acts as a startup dead-air timeout on
  the virtual-channel path: if response headers arrive but no usable first bytes
  appear within that window, Tunerr will switch to filler instead of waiting on
  the stalled source indefinitely.
- the virtual-channel path can now also perform repeated midstream filler
  cutovers across the ordered fallback chain when the active upstream stalls or
  hard-errors after startup, instead of only deciding before the session begins.
- the live recovery relay can now also perform repeated rolling probes over
  sampled in-session media bytes after startup and trigger filler when a later
  sampled window probes as black or silent, instead of only reacting to startup
  samples or transport stalls.
- when `ffmpeg` is available, the virtual-channel path can now probe both:
  - the source URL before playback, and
  - sampled response bytes from the actual upstream response body
  and switch to filler when the start of the source appears fully black
  (`blackdetect`) or silent (`silencedetect`).
- bug/banner/image metadata are now rendered by the branded stream/slate
  surfaces, while the plain `/virtual-channels/stream/` path remains the
  unbranded source-preserving option.

## Mutation helpers

The first operator-friendly mutation helpers now exist:

- `POST /virtual-channels/channel-detail.json`
  - updates station metadata for an existing channel
  - accepts `channel_id` plus any of:
    - `name`
    - `guide_number`
    - `group_title`
    - `description`
    - `enabled`
    - `branding`
    - `branding_clear`
    - `recovery`
    - `recovery_clear`
  - branding updates now merge into the existing branding object instead of
    replacing it wholesale, so partial updates such as only changing
    `branding.stream_mode` do not drop existing logo/bug/banner fields
  - recovery updates now also merge into the existing recovery object instead of
    replacing it wholesale, so changing `recovery.mode` or
    `recovery.black_screen_seconds` does not drop existing
    `recovery.fallback_entries`
- `POST /virtual-channels/schedule.json`
  - updates schedule entries for an existing channel
  - supported actions:
    - `append_entry`
    - `replace_entries`
    - `append_movies`
    - `append_episodes`
    - `remove_entries`
    - `append_slot`
    - `replace_slots`
    - `remove_slots`

These helpers are intentionally foundation-grade. They reduce manual JSON editing,
but they are not yet a full slot-builder UI/workflow.

## Daypart helper

`POST /virtual-channels/schedule.json` also now supports `fill_daypart`:

- required:
  - `channel_id`
  - `daypart_start_hhmm`
  - `daypart_end_hhmm`
- supply content using one of:
  - `movie_ids`
  - `series_id` + `episode_ids`
  - `entries`
  - `entry`
- optional:
  - `duration_mins`
  - `label_prefix`

The helper expands the provided content into explicit daily `slots[]` inside the
requested time window. Existing slots in that same window are replaced; slots
outside the window are retained.

Two collection-aware helpers now also exist:

- `fill_movie_category`
  - auto-builds a daypart from all indexed movies whose `category` matches the
    requested `category`
- `fill_series`
  - auto-builds a daypart from all episodes in the requested `series_id`, or a
    bounded subset if `episode_ids` are supplied

## Slate rendering

Virtual channels now also have a rendered station-slate surface:

- `GET /virtual-channels/slate/<channel-id>.svg`
- `GET /virtual-channels/branded-stream/<channel-id>.ts`
- `GET /virtual-channels/recovery-report.json`

The SVG slate uses current branding metadata:

- `branding.logo_url`
- `branding.bug_text`
- `branding.bug_position`
- `branding.banner_text`
- `branding.theme_color`

This is the first real rendered output for the branding layer. It is a station
slate surface, not full live-video overlay compositing yet.

The branded stream endpoint is the first real composited playback surface for
branding metadata:

- it uses `ffmpeg` when available
- it can burn `bug_text` and `banner_text` into a MPEG-TS output
- it can also overlay `branding.bug_image_url` or, if that is empty,
  `branding.logo_url` as the corner image input
- it is intentionally separate from the existing plain `/virtual-channels/stream/`
  path so the unbranded source-preserving path remains available

Operators can now also opt into publishing that branded stream by default:

- `IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT=true`
- when enabled, virtual channels with branding metadata are published through
  `/virtual-channels/branded-stream/<channel-id>.ts` in the generated
  `/virtual-channels/live.m3u`
- channels without branding metadata continue to publish the plain
  `/virtual-channels/stream/<channel-id>.mp4` path

Per-channel `branding.stream_mode` can override that process default:

- `plain` always publishes the plain stream path
- `branded` always publishes the branded stream path
- empty / `auto` uses the process-wide default plus branding presence

The deck Programming lane now exposes first-step controls for this field through
the virtual station report cards:

- force `plain`
- force `branded`
- reset to `auto`

The same deck cards now also offer first-step edits for:

- `branding.logo_url`
- `branding.bug_image_url`
- `branding.bug_text`
- `branding.bug_position`
- `branding.banner_text`
- `branding.theme_color`
- `recovery.mode` (`Disable Recovery` / `Enable Filler`)
- `recovery.black_screen_seconds`

Submitting an empty value from that UI clears the field instead of silently
leaving the old value in place.

The recovery report exposes recent in-memory runtime events with fields such as:

- `detected_at_utc`
- `channel_id`
- `entry_id`
- `reason`
- `surface`
- `fallback_entry_id`
- `fallback_url`

Some current recovery reasons include:

- `missing-source`
- `proxy-error`
- `upstream-status`
- `startup-timeout`
- `content-blackdetect`
- `content-blackdetect-bytes`
- `content-silencedetect-bytes`
- `live-stall-timeout`
- `live-stall-timeout-exhausted`
- `live-read-error`
- `live-read-error-exhausted`

Recovery sampling can be stretched slightly beyond the per-channel
`black_screen_seconds` threshold with:

- `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_WARMUP_SEC`
- `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_MIDSTREAM_PROBE_BYTES`
- `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC`

When set higher than `black_screen_seconds`, the startup response sampler keeps
watching up to that longer warmup window before deciding whether to fall
through to filler.

When `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC` is set, Tunerr also
enables a live-session stall watchdog. If the active upstream stops producing
bytes for that many seconds after startup, Tunerr attempts to switch the
session to the next healthy filler source instead of ending the stream
immediately.

If more than one `recovery.fallback_entries[]` item is configured, Tunerr now
walks them in order and skips broken fallback candidates until it finds one
that opens successfully. That applies both to startup-time recovery and the
live-session stall/error cutover path, and the same ordered chain can now be
walked again if an earlier fallback source later stalls or hard-errors too.
If the chain is exhausted, Tunerr records an explicit `*-exhausted` recovery
event so operators can distinguish “recovery ran out of options” from “recovery
never triggered”.

The deck Settings lane can now update this watchdog live for future sessions,
using the same localhost-only operator action pattern as other runtime knobs.

If `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_STATE_FILE` is set, those recovery
events are also persisted across restarts so the station report and recovery
report do not reset to empty on process start.

## Station report

The station layer now also exposes a condensed operator report:

- `GET /virtual-channels/report.json`

Each row includes:

- `channel_id`
- `name`
- `guide_number`
- `enabled`
- `stream_mode`
- `recovery_mode`
- `black_screen_seconds`
- `fallback_entries`
- `recovery_events`
- `recovery_exhausted`
- `last_recovery_reason`
- `published_stream_url`
- `slate_url`
- `branded_stream_url`
- `resolved_now`
- `recent_recovery`

## Example

```json
{
  "version": 1,
  "channels": [
    {
      "id": "vc-news",
      "name": "News Loop",
      "guide_number": "9001",
      "description": "A branded synthetic station built from local media.",
      "enabled": true,
      "loop_daily_utc": true,
      "branding": {
        "logo_url": "https://img.example/news.png",
        "bug_text": "NEWS",
        "bug_position": "top-left",
        "banner_text": "Breaking now",
        "theme_color": "#cc0000"
      },
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 3,
        "fallback_entries": [
          {
            "type": "movie",
            "movie_id": "filler-slate",
            "duration_mins": 5
          }
        ]
      },
      "entries": [
        {
          "type": "movie",
          "movie_id": "m1",
          "duration_mins": 60
        }
      ]
    }
  ]
}
```

## Notes

- This is the `STN-001` foundation slice from
  [EPIC-station-ops](../epics/EPIC-station-ops.md).
- Runtime black-screen detection, filler insertion, and overlay rendering are
  still future work.

See also
--------

- [EPIC-station-ops](../epics/EPIC-station-ops.md)
- [EPIC-programming-manager](../epics/EPIC-programming-manager.md)
- [cli-and-env-reference](cli-and-env-reference.md)
