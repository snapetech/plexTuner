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

## `iptv-tunerr run`

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

## `iptv-tunerr serve`

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

## `iptv-tunerr index`

Fetch provider M3U/API and write catalog JSON.

Common flags:
- `-m3u`
- `-catalog`

Use for:
- scheduled indexing
- catalog debugging without starting the server

## `iptv-tunerr mount`

Mount VODFS from the catalog.

Common flags:
- `-mount`
- `-catalog`
- `-cache`

Notes:
- Linux-only (`FUSE`)

## `iptv-tunerr plex-vod-register`

Create or reuse Plex libraries for a mounted VODFS tree.

Default library names:
- `VOD` -> `<mount>/TV` (Plex TV library)
- `VOD-Movies` -> `<mount>/Movies` (Plex Movie library)

Common flags:
- `-mount`
- `-plex-url`
- `-token`
- `-shows-name`
- `-movies-name`
- `-vod-safe-preset` (default `true`)
- `-refresh`

Env fallbacks:
- `IPTV_TUNERR_PMS_URL` (or `PLEX_HOST` -> `http://<host>:32400`)
- `IPTV_TUNERR_PMS_TOKEN` (or `PLEX_TOKEN`)
- `IPTV_TUNERR_MOUNT`

Notes:
- Requires the VODFS mount path to be visible to the Plex server host/container.
- Creates/reuses sections idempotently by section name + path.
- If the same section name exists with a different path/type, the command returns an error instead of mutating it.
- By default, applies a per-library VOD-safe Plex preset to disable expensive analysis jobs (credits, intro/chapter/preview thumbnails, ad/voice analysis) on these virtual catch-up libraries only.

## `iptv-tunerr vod-split`

Split a VOD catalog into multiple category/region lane catalogs for separate
VODFS mounts/libraries.

Built-in lane names (current):
- `bcastUS`
- `sports`
- `news`
- `kids`
- `music`
- `euroUK`
- `mena`
- `movies`
- `tv`
- `intl`

Common flags:
- `-catalog`
- `-out-dir` (required)

Output:
- `<out-dir>/<lane>.json`
- `<out-dir>/manifest.json` (lane counts + source catalog)

Use for:
- smaller category-scoped Plex VOD libraries
- reduced scan scope / faster targeted rescans
- operational isolation of high-churn catch-up lanes

## `iptv-tunerr epg-link-report`

Generate a deterministic EPG-link coverage report for `live_channels` in a
catalog against an XMLTV source. This is the Phase 1 workflow for improving the
long-tail unlinked channel set without changing runtime playback behavior.

Match tiers (current):
- `tvg-id` exact
- alias override exact
- normalized channel-name exact (unique only)

Common flags:
- `-catalog`
- `-xmltv` (required; file path or `http(s)` URL)
- `-aliases` (optional JSON alias override file)
- `-out` (optional JSON full report)
- `-unmatched-out` (optional JSON unmatched-only list)

Alias override JSON shape:

```json
{
  "name_to_xmltv_id": {
    "Nick Junior Canada": "nickjr.ca",
    "Fox News Channel US": "foxnews.us"
  }
}
```

Use for:
- measuring current XMLTV coverage before changing lineups
- generating a review queue for the unlinked tail
- iterating alias mappings safely (report-only, no runtime mutation)

## `iptv-tunerr probe`

Probe provider URLs and print ranked results (best host first).

Common flags:
- `-urls`

Use for:
- provider host failover validation
- diagnosing Cloudflare/proxy failures

## `iptv-tunerr plex-epg-oracle`

Probe Plex's wizard-equivalent HDHR registration/guide/channelmap flow across one
or more tuner base URLs and report what Plex maps.

This is an in-app tool for using Plex as a provider/EPG matching oracle during
EPG-linking experiments (for example different lineup sizes/orderings for a region).

Common flags:
- `-plex-url`
- `-token`
- `-base-urls` (comma-separated tuner URLs to test)
- `-base-url-template` + `-caps` (expand `{cap}` into multiple URLs)
- `-reload-guide` (default `true`)
- `-activate` (default `false`; report/probe only unless enabled)
- `-out` (JSON report)

Notes:
- Creates/registers Plex DVR/device rows as part of the probe flow.
- Best used in a lab/test Plex instance.
- Intended to harvest mapping outcomes, not as a runtime dependency.

## `iptv-tunerr plex-epg-oracle-cleanup`

Clean up DVR/device rows created during oracle experiments.

Default behavior is **dry-run** (prints matching DVR/device rows without deleting).

Common flags:
- `-plex-url`
- `-token`
- `-lineup-prefix` (default `oracle-`)
- `-device-uri-substr` (optional extra filter)
- `-do` (actually delete)

Typical flow:
1. Dry-run inspect:
   - `iptv-tunerr plex-epg-oracle-cleanup -plex-url ... -token ...`
2. Apply cleanup:
   - `iptv-tunerr plex-epg-oracle-cleanup -plex-url ... -token ... -do`

## `iptv-tunerr supervise`

Run multiple child `iptv-tunerr` instances from one JSON config.

Common flags:
- `-config`

Use for:
- single-app / multi-DVR category deployments
- combined injected DVR + HDHR wizard lanes

## Core env vars

## Provider / input

- `IPTV_TUNERR_PROVIDER_URL`
- `IPTV_TUNERR_PROVIDER_URLS`
- `IPTV_TUNERR_PROVIDER_USER`
- `IPTV_TUNERR_PROVIDER_PASS`
- `IPTV_TUNERR_SUBSCRIPTION_FILE`
- `IPTV_TUNERR_M3U_URL`

## Paths

- `IPTV_TUNERR_CATALOG`
- `IPTV_TUNERR_MOUNT`
- `IPTV_TUNERR_CACHE`

## Tuner identity / lineup

- `IPTV_TUNERR_BASE_URL`
- `IPTV_TUNERR_DEVICE_ID`
- `IPTV_TUNERR_FRIENDLY_NAME`
- `IPTV_TUNERR_TUNER_COUNT`
- `IPTV_TUNERR_LINEUP_MAX_CHANNELS`
- `IPTV_TUNERR_GUIDE_NUMBER_OFFSET`

`IPTV_TUNERR_GUIDE_NUMBER_OFFSET`:
- adds a per-instance channel/guide ID offset
- useful for many DVRs in Plex to avoid guide cache collisions

## Stream behavior

- `IPTV_TUNERR_STREAM_TRANSCODE` (`off|on|auto`)
- `IPTV_TUNERR_STREAM_BUFFER_BYTES` (`0|auto|<bytes>`)
- `IPTV_TUNERR_FFMPEG_PATH`
- `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` (advanced ffmpeg/HLS behavior)
- `IPTV_TUNERR_CLIENT_ADAPT` — when true, resolve Plex client from session and force websafe (transcode+plexsafe) for web/browser clients and for internal fetcher (Lavf/PMS) so Chrome and Firefox both get compatible audio.
- `IPTV_TUNERR_FORCE_WEBSAFE` — when true, always transcode with plexsafe (MP3) regardless of client; use if Chrome has no audio after a Plex/server update and client detection misclassifies.
- `IPTV_TUNERR_STRIP_STREAM_HOSTS` — comma-separated hostnames (e.g. `cf.like-cdn.com,like-cdn.com`) whose stream URLs are removed at catalog build; channels with only those hosts are dropped so the tuner never uses CF-blocked endpoints.

## Guide / XMLTV

- `IPTV_TUNERR_XMLTV_URL`
- `IPTV_TUNERR_XMLTV_TIMEOUT`
- `IPTV_TUNERR_XMLTV_CACHE_TTL`
- `IPTV_TUNERR_LIVE_EPG_ONLY`
- `IPTV_TUNERR_EPG_PRUNE_UNLINKED`

XMLTV language normalization:
- `IPTV_TUNERR_XMLTV_PREFER_LANGS`
- `IPTV_TUNERR_XMLTV_PREFER_LATIN`
- `IPTV_TUNERR_XMLTV_NON_LATIN_TITLE_FALLBACK`

## HDHR network mode

- `IPTV_TUNERR_HDHR_NETWORK_MODE`
- `IPTV_TUNERR_HDHR_DEVICE_ID`
- `IPTV_TUNERR_HDHR_TUNER_COUNT`
- `IPTV_TUNERR_HDHR_FRIENDLY_NAME`
- `IPTV_TUNERR_HDHR_SCAN_POSSIBLE`
- `IPTV_TUNERR_HDHR_MANUFACTURER`
- `IPTV_TUNERR_HDHR_MODEL_NUMBER`
- `IPTV_TUNERR_HDHR_FIRMWARE_NAME`
- `IPTV_TUNERR_HDHR_FIRMWARE_VERSION`
- `IPTV_TUNERR_HDHR_DEVICE_AUTH`

## Plex session reaper (built-in)

Required:
- `IPTV_TUNERR_PMS_URL`
- `IPTV_TUNERR_PMS_TOKEN`

Enable/tune:
- `IPTV_TUNERR_PLEX_SESSION_REAPER`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_POLL_S`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_IDLE_S`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_RENEW_LEASE_S`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_HARD_LEASE_S`
- `IPTV_TUNERR_PLEX_SESSION_REAPER_SSE`

## HDHR wizard-lane shaping (optional)

- `IPTV_TUNERR_LINEUP_DROP_MUSIC`
- `IPTV_TUNERR_LINEUP_SHAPE`
- `IPTV_TUNERR_LINEUP_REGION_PROFILE`

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
