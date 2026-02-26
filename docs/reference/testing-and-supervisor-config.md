---
id: ref-testing-supervisor-config
type: reference
status: draft
tags: [reference, config, supervisor, live-tv]
---

# Testing and supervisor config

Reference for the recent test/lab features used in the single-app supervisor flow.

## CLI: supervisor mode

```bash
plex-tuner supervise -config /path/to/supervisor.json
```

`supervisor.json` contains a list of child `plex-tuner` instances (commands + env).

Example:
- [`k8s/plextuner-supervisor-multi.example.json`](../../k8s/plextuner-supervisor-multi.example.json)

## Env vars (recently added / important for testing)

### Per-child guide ID ranges (fixes multi-DVR guide collisions)

- `PLEX_TUNER_GUIDE_NUMBER_OFFSET`
  - Adds an integer offset to exposed channel `GuideNumber` values.
  - Use unique ranges per DVR child (example: `newsus=2000`, `bcastus=1000`, `otherworld=13000`).
  - Purpose: avoid Plex client/provider cache collisions when many DVRs all start at channel `1`.

### Per-child lineup sharding (overflow buckets)

- `PLEX_TUNER_LINEUP_SKIP`
- `PLEX_TUNER_LINEUP_TAKE`

Behavior:
- Applied **after** pre-cap filtering/shaping (EPG-linked-only, music-drop, wizard shaping)
- Applied **before** `PLEX_TUNER_LINEUP_MAX_CHANNELS` final cap

Use:
- Split a large category into multiple injected DVR children that all point at the same
  source M3U/XMLTV but expose different channel slices (`cat`, `cat2`, `cat3`, ...)
- Example:
  - shard 1: `TAKE=479`
  - shard 2: `SKIP=479 TAKE=479`
  - shard 3: `SKIP=958 TAKE=479`

### Built-in Plex stale-session reaper (cross-platform, no Python)

Required:
- `PLEX_TUNER_PMS_URL`
- `PLEX_TUNER_PMS_TOKEN`

Enable and tune:
- `PLEX_TUNER_PLEX_SESSION_REAPER=1`
- `PLEX_TUNER_PLEX_SESSION_REAPER_POLL_S=2`
- `PLEX_TUNER_PLEX_SESSION_REAPER_IDLE_S=15`
- `PLEX_TUNER_PLEX_SESSION_REAPER_RENEW_LEASE_S=20`
- `PLEX_TUNER_PLEX_SESSION_REAPER_HARD_LEASE_S=1800`
- `PLEX_TUNER_PLEX_SESSION_REAPER_SSE=1`

Notes:
- `IDLE_S` is the practical stale-session prune timer.
- `HARD_LEASE_S` is a backstop, not the primary timer.

### XMLTV language normalization

- `PLEX_TUNER_XMLTV_PREFER_LANGS=en,eng`
- `PLEX_TUNER_XMLTV_PREFER_LATIN=true`
- `PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK=channel`

Purpose:
- prefer English variants when upstream XMLTV provides them
- reduce non-Latin titles when the feed is multilingual

Limit:
- cannot translate feeds that only provide one non-English variant

### HDHR wizard-lane shaping / behavior

- `PLEX_TUNER_LINEUP_MAX_CHANNELS=479`
- `PLEX_TUNER_LINEUP_DROP_MUSIC=true`
- `PLEX_TUNER_LINEUP_SHAPE=na_en`
- `PLEX_TUNER_LINEUP_REGION_PROFILE=ca_west` (example; make runtime-configurable)
- `PLEX_TUNER_HDHR_SCAN_POSSIBLE=true|false`

Use:
- `true` for the dedicated HDHR wizard lane
- `false` for category/injected DVR children to reduce wizard mis-selection

## Recommended single-app test shape

- `13` injected DVR category children
- `1` HDHR wizard child (broad feed + cap/shaping)
- optional `1` second HDHR child if testing multiple wizard devices

Important:
- only one child should enable HDHR network mode on default ports unless custom HDHR ports are assigned.
- if you create overflow category shards, assign unique `PLEX_TUNER_GUIDE_NUMBER_OFFSET` ranges per shard to avoid cross-provider channel collisions.

### Supervisor manifest generator overflow support

`scripts/generate-k3s-supervisor-manifests.py` can now auto-create overflow
category children from a confirmed linked-channel count file.

Flags:
- `--category-counts-json <path>` (JSON map of base category -> linked count)
- `--category-cap 479` (overflow threshold and shard take size)

It expands categories into:
- `<category>`
- `<category>2`
- `<category>3`
...as needed, and injects per-child `PLEX_TUNER_LINEUP_SKIP/TAKE`.

## Platform notes

- `VODFS` mount (`plex-tuner mount`) is Linux-only.
- HDHR network mode now compiles on Linux/macOS/Windows, but Windows runtime validation in this repo was smoke-tested under `wine` (which can fail UDP/TCP socket operations even when the app path is correct).

## Known Plex-side caveats (affect testing, not packaging)

- Plex may label multiple Live TV sources with the same server-level name (`plexKube`) in some clients.
- Plex can retain hidden "active grabs" that block new tunes until a Plex restart, even when `/status/sessions` shows no playback.

See also
--------
- [package-test-builds](../how-to/package-test-builds.md)
- [run-without-kubernetes](../how-to/run-without-kubernetes.md)
- [k8s/README](../../k8s/README.md)
