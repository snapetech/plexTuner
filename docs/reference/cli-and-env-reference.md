---
id: ref-cli-env-reference
type: reference
status: draft
tags: [reference, cli, env, config]
---

# CLI and env reference

Reference for primary commands, key flags, and commonly used environment variables.

This is focused on practical operation/testing. For tester bundles and supervisor-specific lab knobs, also see:
- [testing-and-supervisor-config](testing-and-supervisor-config.md)

## Commands

## `plex-tuner run`

One-shot workflow:
- refresh catalog (unless skipped)
- health-check provider (unless skipped)
- start tuner server

Common flags:
- `-catalog`
- `-addr`
- `-base-url`
- `-device-id`
- `-friendly-name`
- `-mode` (`easy` or `full`)
- `-skip-index`
- `-skip-health`
- `-register-plex`
- `-register-only`

Use for:
- systemd/Docker runtime
- most single-binary deployments

## `plex-tuner serve`

Serve tuner endpoints from an existing catalog.

Common flags:
- `-catalog`
- `-addr`
- `-base-url`
- `-device-id`
- `-friendly-name`
- `-mode`

Use for:
- split workflows (external indexing)
- local endpoint tests

## `plex-tuner index`

Fetch provider M3U/API and write catalog JSON.

Common flags:
- `-m3u`
- `-catalog`

Use for:
- scheduled indexing
- catalog debugging without starting the server

## `plex-tuner mount`

Mount VODFS from the catalog.

Common flags:
- `-mount`
- `-catalog`
- `-cache`

Notes:
- Linux-only (`FUSE`)

## `plex-tuner probe`

Probe provider URLs and print ranked results (best host first).

Common flags:
- `-urls`

Use for:
- provider host failover validation
- diagnosing Cloudflare/proxy failures

## `plex-tuner supervise`

Run multiple child `plex-tuner` instances from one JSON config.

Common flags:
- `-config`

Use for:
- single-app / multi-DVR category deployments
- combined injected DVR + HDHR wizard lanes

## Core env vars

## Provider / input

- `PLEX_TUNER_PROVIDER_URL`
- `PLEX_TUNER_PROVIDER_URLS`
- `PLEX_TUNER_PROVIDER_USER`
- `PLEX_TUNER_PROVIDER_PASS`
- `PLEX_TUNER_SUBSCRIPTION_FILE`
- `PLEX_TUNER_M3U_URL`

## Paths

- `PLEX_TUNER_CATALOG`
- `PLEX_TUNER_MOUNT`
- `PLEX_TUNER_CACHE`

## Tuner identity / lineup

- `PLEX_TUNER_BASE_URL`
- `PLEX_TUNER_DEVICE_ID`
- `PLEX_TUNER_FRIENDLY_NAME`
- `PLEX_TUNER_TUNER_COUNT`
- `PLEX_TUNER_LINEUP_MAX_CHANNELS`
- `PLEX_TUNER_GUIDE_NUMBER_OFFSET`

`PLEX_TUNER_GUIDE_NUMBER_OFFSET`:
- adds a per-instance channel/guide ID offset
- useful for many DVRs in Plex to avoid guide cache collisions

## Stream behavior

- `PLEX_TUNER_STREAM_TRANSCODE` (`off|on|auto`)
- `PLEX_TUNER_STREAM_BUFFER_BYTES` (`0|auto|<bytes>`)
- `PLEX_TUNER_FFMPEG_PATH`
- `PLEX_TUNER_FFMPEG_HLS_RECONNECT` (advanced ffmpeg/HLS behavior)

## Guide / XMLTV

- `PLEX_TUNER_XMLTV_URL`
- `PLEX_TUNER_XMLTV_TIMEOUT`
- `PLEX_TUNER_XMLTV_CACHE_TTL`
- `PLEX_TUNER_LIVE_EPG_ONLY`
- `PLEX_TUNER_EPG_PRUNE_UNLINKED`

XMLTV language normalization:
- `PLEX_TUNER_XMLTV_PREFER_LANGS`
- `PLEX_TUNER_XMLTV_PREFER_LATIN`
- `PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK`

## HDHR network mode

- `PLEX_TUNER_HDHR_NETWORK_MODE`
- `PLEX_TUNER_HDHR_DEVICE_ID`
- `PLEX_TUNER_HDHR_TUNER_COUNT`
- `PLEX_TUNER_HDHR_FRIENDLY_NAME`
- `PLEX_TUNER_HDHR_SCAN_POSSIBLE`
- `PLEX_TUNER_HDHR_MANUFACTURER`
- `PLEX_TUNER_HDHR_MODEL_NUMBER`
- `PLEX_TUNER_HDHR_FIRMWARE_NAME`
- `PLEX_TUNER_HDHR_FIRMWARE_VERSION`
- `PLEX_TUNER_HDHR_DEVICE_AUTH`

## Plex session reaper (built-in)

Required:
- `PLEX_TUNER_PMS_URL`
- `PLEX_TUNER_PMS_TOKEN`

Enable/tune:
- `PLEX_TUNER_PLEX_SESSION_REAPER`
- `PLEX_TUNER_PLEX_SESSION_REAPER_POLL_S`
- `PLEX_TUNER_PLEX_SESSION_REAPER_IDLE_S`
- `PLEX_TUNER_PLEX_SESSION_REAPER_RENEW_LEASE_S`
- `PLEX_TUNER_PLEX_SESSION_REAPER_HARD_LEASE_S`
- `PLEX_TUNER_PLEX_SESSION_REAPER_SSE`

## HDHR wizard-lane shaping (optional)

- `PLEX_TUNER_LINEUP_DROP_MUSIC`
- `PLEX_TUNER_LINEUP_SHAPE`
- `PLEX_TUNER_LINEUP_REGION_PROFILE`

Typical use:
- broad feed HDHR lane capped to `479`
- category DVR lanes use separate M3U inputs and no shaping

## Platform notes

- `mount` / VODFS is Linux-only
- Core tuner paths (`run`, `serve`, `supervise`) are cross-platform
- HDHR network mode compiles on Linux/macOS/Windows; validate native Windows networking on a real Windows host (not `wine`)

## Verification helpers

- `./scripts/verify`
- `./scripts/build-test-packages.sh`
- `./scripts/build-tester-release.sh`
- `./scripts/plex-hidden-grab-recover.sh --dry-run`

See also
--------
- [testing-and-supervisor-config](testing-and-supervisor-config.md)
- [package-test-builds](../how-to/package-test-builds.md)
- [tester-handoff-checklist](../how-to/tester-handoff-checklist.md)
- [memory-bank/commands.yml](../../memory-bank/commands.yml)

