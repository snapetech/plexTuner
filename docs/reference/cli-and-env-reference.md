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

## `plex-tuner plex-vod-register`

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
- `PLEX_TUNER_PMS_URL` (or `PLEX_HOST` -> `http://<host>:32400`)
- `PLEX_TUNER_PMS_TOKEN` (or `PLEX_TOKEN`)
- `PLEX_TUNER_MOUNT`

Notes:
- Requires the VODFS mount path to be visible to the Plex server host/container.
- Creates/reuses sections idempotently by section name + path.
- If the same section name exists with a different path/type, the command returns an error instead of mutating it.
- By default, applies a per-library VOD-safe Plex preset to disable expensive analysis jobs (credits, intro/chapter/preview thumbnails, ad/voice analysis) on these virtual catch-up libraries only.

## `plex-tuner vod-split`

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

## `plex-tuner epg-link-report`

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
- `-oracle-report` (optional `plex-epg-oracle` JSON output; generates alias suggestions for unmatched channels)
- `-suggest-out` (optional path to write oracle-derived alias suggestions; the output is `name_to_xmltv_id`-compatible and can be passed to the next run via `-aliases`)
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

### Oracle-assisted alias suggestion workflow

1. Run `plex-epg-oracle -out oracle.json` against a test tuner to capture Plex's channelmap decisions.
2. Run `epg-link-report -xmltv xmltv.xml -oracle-report oracle.json -suggest-out suggestions.json` to produce suggested aliases for unmatched channels.
3. Review `suggestions.json`, prune false positives, then pass it to the next report run via `-aliases suggestions.json` to measure the coverage lift.

Use for:
- measuring current XMLTV coverage before changing lineups
- generating a review queue for the unlinked tail
- iterating alias mappings safely (report-only, no runtime mutation)
- harvesting Plex oracle channelmap hints to improve match rate

## `plex-tuner probe`

Probe provider URLs and print ranked results (best host first).

Common flags:
- `-urls`

Use for:
- provider host failover validation
- diagnosing Cloudflare/proxy failures

## `plex-tuner plex-epg-oracle`

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
- Output now includes full per-channel mapping rows (`channels[]`) with `guide_name`, `guide_number`, `tvg_id`, and `lineup_identifier` (the XMLTV channel ID Plex oracle matched). This is the input for `epg-link-report -oracle-report`.

## `plex-tuner plex-epg-oracle-cleanup`

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
   - `plex-tuner plex-epg-oracle-cleanup -plex-url ... -token ...`
2. Apply cleanup:
   - `plex-tuner plex-epg-oracle-cleanup -plex-url ... -token ... -do`

## `plex-tuner supervise`

Run multiple child `plex-tuner` instances from one JSON config.

Common flags:
- `-config`

Use for:
- single-app / multi-DVR category deployments
- combined injected DVR + HDHR wizard lanes

## Core env vars

## Provider / input

### Primary source

- `PLEX_TUNER_PROVIDER_URL` — single Xtream provider base URL
- `PLEX_TUNER_PROVIDER_URLS` — comma-separated list of provider base URLs (ranked failover; first success wins)
- `PLEX_TUNER_PROVIDER_USER`
- `PLEX_TUNER_PROVIDER_PASS`
- `PLEX_TUNER_SUBSCRIPTION_FILE` — path to a `Username: / Password:` file (auto-detects `~/Documents/iptv.subscription.*.txt`)
- `PLEX_TUNER_M3U_URL` — direct M3U URL (bypasses player_api/get.php construction)

### Second provider (live-channel merge)

When set, live channels from the second provider are **merged** into the primary catalog after the primary fetch. Deduplication is by `tvg-id` (when present) or normalized stream-URL hostname+path (credential query-strings are stripped). Merged-in channels are tagged with `source_tag: "provider2"` in the catalog. VOD from the second provider is not merged (live only).

- `PLEX_TUNER_M3U_URL_2` — direct M3U URL for the second provider (highest priority)
- `PLEX_TUNER_PROVIDER_URL_2` — Xtream base URL for the second provider (used to build `get.php` URL when `M3U_URL_2` is absent)
- `PLEX_TUNER_PROVIDER_USER_2`
- `PLEX_TUNER_PROVIDER_PASS_2`

Example (two separate IPTV service accounts):
```
PLEX_TUNER_M3U_URL=http://provider1.example/get.php?username=u1&password=p1&type=m3u_plus
PLEX_TUNER_M3U_URL_2=http://provider2.example/get.php?username=u2&password=p2&type=m3u_plus
```

Or via Xtream creds:
```
PLEX_TUNER_PROVIDER_URL=http://provider1.example
PLEX_TUNER_PROVIDER_USER=u1
PLEX_TUNER_PROVIDER_PASS=p1
PLEX_TUNER_PROVIDER_URL_2=http://provider2.example
PLEX_TUNER_PROVIDER_USER_2=u2
PLEX_TUNER_PROVIDER_PASS_2=p2
```

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

## Lineup filtering and shaping

### Category filter (DVR injection lanes)

`PLEX_TUNER_LINEUP_CATEGORY` — filter the lineup to a named content/region bucket before the cap is applied. Accepts one or more comma-separated values (case-insensitive).

Content type values:
- `sports` — all sports channels (ESPN, TSN, DAZN, NFL, NBA, NHL, MLB, UFC, F1, etc.)
- `movies` — movie/premium channels (HBO, Showtime, Starz, Sky Cinema, etc.)
- `news` — news/weather/business channels
- `kids` — children's channels (Disney, Nickelodeon, PBS Kids, Treehouse, etc.)
- `music` — music/radio channels

Region values:
- `canada` or `ca` — Canadian channels (CBC, CTV, Global, Sportsnet, etc.)
- `us` — US channels (NBC, CBS, ABC, Fox, PBS, etc.)
- `na` — both Canada and US
- `uk` or `ukie` — UK and Irish channels
- `europe` — FR/DE/NL/BE/CH/AT/ES/PT + Nordics
- `nordics` — SE/NO/DK/FI specifically
- `eusouth` — IT/GR/CY/MT
- `eueast` — PL/RU/HU/RO/CZ/BG/HR/TR/UA/etc.
- `latam` — Latin America (AR/BR/MX/CO/CL/PE/CU)
- `intl` — everything not matched to a specific region

Classification is derived from the M3U `group-title` prefix (e.g. `US | ESPN HD` → prefix `US`) with name-keyword fallback. Channels that match either the content type or region component qualify.

Example supervisor child env:
```
PLEX_TUNER_LINEUP_CATEGORY=sports          # all sports channels
PLEX_TUNER_LINEUP_CATEGORY=canada          # Canadian general/news/bcast
PLEX_TUNER_LINEUP_CATEGORY=us             # US general/news/bcast
PLEX_TUNER_LINEUP_CATEGORY=canadamovies   # Canadian movie channels
PLEX_TUNER_LINEUP_CATEGORY=usmovies       # US movie channels
```

Category DVR children use the full live M3U/XMLTV feed — no pre-split per-category M3U files needed.

### Sharding (overflow buckets)

- `PLEX_TUNER_LINEUP_SKIP` — skip the first N channels after all pre-cap filters; used for overflow buckets (e.g. `sports2`)
- `PLEX_TUNER_LINEUP_TAKE` — take at most N channels after skip; use with `LINEUP_MAX_CHANNELS` for tight caps

### HDHR wizard-lane shaping

- `PLEX_TUNER_LINEUP_DROP_MUSIC` — drop music/radio channels by name heuristic (default off)
- `PLEX_TUNER_LINEUP_SHAPE` — wizard sort profile; currently `na_en` (North American English priority) or `off`
- `PLEX_TUNER_LINEUP_REGION_PROFILE` — regional sub-profile for wizard shape (e.g. `ca_west`, `ca_prairies`)
- `PLEX_TUNER_LINEUP_LANGUAGE` — keep only channels matching a language guess (e.g. `en`, `fr`, `es`)
- `PLEX_TUNER_LINEUP_EXCLUDE_REGEX` — drop channels whose `GuideName + TVGID` matches this regex

Typical use:
- HDHR wizard lane: broad feed, `na_en` shape, capped to `479`, music dropped
- Category DVR lanes: full feed, `PLEX_TUNER_LINEUP_CATEGORY` set, no shape needed

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
