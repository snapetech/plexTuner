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
- load cached catalog immediately if one exists on disk (serves Plex without delay)
- refresh catalog in background (or blocking if no cache exists)
- health-check provider (unless skipped)
- start tuner server

**Cached startup**: if a `catalog.json` already exists on disk, `run` starts serving it to Plex right away and queues an immediate background refresh.  Clients see no gap in the guide after a restart.  If no cache exists (first run), the initial fetch blocks as before.

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

## `plex-tuner plex-iptvorg-harvest`

Downloads the [iptv-org](https://github.com/iptv-org/iptv) community channel database
(~39k channels) and saves it as a local JSON file for in-app enrichment.

```bash
plex-tuner plex-iptvorg-harvest -out /path/to/iptvorg.json
# Override source URL:
plex-tuner plex-iptvorg-harvest -out /path/to/iptvorg.json -url https://iptv-org.github.io/api/channels.json
```

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | *(required)* | Output path for the DB JSON |
| `-url` | iptv-org.github.io default | Override channels.json URL |

Set `PLEX_TUNER_IPTVORG_DB=/path/to/iptvorg.json` to enable enrichment at runtime.
Re-run monthly. The DB is ~3 MB and covers ~39k channels across 250+ territories.

**See also:** [EPG Long-Tail Strategies](../explanations/epg-long-tail-strategies.md)

---

## `plex-tuner plex-gracenote-harvest`

Harvest the plex.tv Gracenote channel database for all world regions and persist it
as a local JSON file.  The result feeds the Gracenote enrichment tier in
`fetchCatalog` and `epg-link-report`.

```bash
plex-tuner plex-gracenote-harvest \
  -token "$PLEX_TOKEN" \
  -out /var/lib/plextuner/gracenote.json

# merge new channels into an existing DB
plex-tuner plex-gracenote-harvest \
  -token "$PLEX_TOKEN" \
  -out /var/lib/plextuner/gracenote.json \
  -merge

# harvest English+French only for reduced DB size
plex-tuner plex-gracenote-harvest \
  -token "$PLEX_TOKEN" \
  -out /var/lib/plextuner/gracenote.json \
  -lang en,fr

# harvest a specific region only
plex-tuner plex-gracenote-harvest \
  -token "$PLEX_TOKEN" \
  -out /var/lib/plextuner/gracenote.json \
  -regions "North America — Canada"
```

Flags:
- `-token` — plex.tv auth token (also read from `PLEX_TOKEN` env)
- `-out` — output DB path (required)
- `-merge` — merge into existing DB instead of overwriting
- `-lang` — comma-separated language codes to keep (empty = all)
- `-regions` — comma-separated region names to harvest (empty = all)

The output JSON is a flat `{"channels":[...]}` array.  Each channel has
`gridKey`, `callSign`, `title`, `language`, `isHd`.

The world regions covered (those confirmed to return lineups from
`epg.provider.plex.tv`) include US, Canada, Mexico, select Latin American
countries, most Western/Nordic EU countries, Poland, Australia/NZ, and India.

After harvesting, point the app at the DB with:
```
PLEX_TUNER_GRACENOTE_DB=/var/lib/plextuner/gracenote.json
```

The `plex-gracenote-harvest` subcommand supersedes the old `scripts/plex-wizard-epg-harvest.py`.

## `plex-tuner epg-link-report`

Generate a deterministic EPG-link coverage report for `live_channels` in a
catalog against an XMLTV source. This is the Phase 1 workflow for improving the
long-tail unlinked channel set without changing runtime playback behavior.

Match tiers (current):
- `tvg-id` exact
- alias override exact
- **Gracenote callSign/gridKey** (tier 1c; active when `PLEX_TUNER_GRACENOTE_DB` is set)
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
- `PLEX_TUNER_FETCH_STATE` — path for the resilient fetch checkpoint file (ETags, per-category progress, stream hashes). Auto-derived as `<CATALOG_STEM>.fetchstate.json` when not set. Set to `""` (empty string) to disable state persistence (disables conditional GETs and crash-resume).

## Resilient fetch engine

`internal/indexer/fetch` is used by default for all `index` and `run` fetches when `PLEX_TUNER_FETCH_STATE` is non-empty (the default). It provides:

- **Conditional GET** — sends `If-None-Match` / `If-Modified-Since` on every request. A `304 Not Modified` skips all parsing and catalog writes for that run.
- **Content-hash fallback** — even when the provider doesn't honour ETags, a SHA-256 hash of the raw body is compared; no-op if unchanged.
- **Category-parallel Xtream fetch** — `get_live_streams?category_id=<id>` fetched in parallel (default 8 concurrent). Each category is checkpointed individually so a crash mid-run resumes cleanly.
- **Stream-hash diff** — per-stream SHA-256(stream_id + name + epg_id + url). Tracks new/changed/unchanged counts per run for logging.
- **Crash-safe state** — `FetchState` is written atomically (temp-file-then-rename) after each category completes. A resumed run skips already-complete categories.
- **Cloudflare detection** — samples up to `PLEX_TUNER_FETCH_STREAM_SAMPLE_SIZE` stream URLs via HEAD; rejects the entire catalog if CF headers are detected (configurable).
- **Force full refresh** — on demand via `PLEX_TUNER_FETCH_FORCE_REFRESH=1` (wipes all cached ETags and completion flags).
- **M3U streaming parse** — body is parsed line-by-line while downloading; ETag + content hash cached across runs.
- **Ranked multi-base failover** — provider ranking and stream-URL backfill unchanged from legacy path.

Env tuning:

| Variable | Default | Purpose |
|---|---|---|
| `PLEX_TUNER_FETCH_STATE` | `<catalog>.fetchstate.json` | Checkpoint path; `""` = disable |
| `PLEX_TUNER_FETCH_CATEGORY_CONCURRENCY` | `8` | Parallel Xtream categories |
| `PLEX_TUNER_FETCH_CF_REJECT` | `true` | Hard-fail on Cloudflare-proxied streams |
| `PLEX_TUNER_FETCH_STREAM_SAMPLE_SIZE` | `5` | URLs probed for CF detection; `0` = skip |

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

## EPG enrichment pipeline

The enrichment pipeline runs during `fetchCatalog` in this order, before `LIVE_EPG_ONLY` filtering:

1. **Re-encode inheritance** — channels labelled `ᴿᴬᵂ`/`4K`/`ᵁᴴᴰ` with no tvg-id inherit
   the tvg-id from their base channel (same name after stripping quality markers). Quality
   tier is also set: UHD=2, HD=1, SD=0, RAW=-1.

2. **Gracenote enrichment** (`PLEX_TUNER_GRACENOTE_DB`) — callSign/gridKey matching via the
   Gracenote DB harvested from plex.tv.

3. **iptv-org enrichment** (`PLEX_TUNER_IPTVORG_DB`) — name/shortcode matching via the
   iptv-org community channel DB (~39k channels).

4. **SDT name propagation** — if a channel's display name looks like garbage (numeric ID,
   UUID, etc.) and the background SDT probe has already stored a `service_name`, the display
   name is replaced so downstream tiers can match it.

5. **Schedules Direct enrichment** (`PLEX_TUNER_SD_DB`) — callSign/station-name matching
   via the local SD station DB. Produces `SD-<stationID>` tvg-ids compatible with SD XMLTV.
   See [plex-sd-harvest](#plex-tuner-plex-sd-harvest).

6. **DVB DB enrichment** (`PLEX_TUNER_DVB_DB`) — for channels with a probed SDT triplet
   (ONID+TSID+SID), looks up the broadcaster name and optional tvg-id in the DVB services DB.
   Works with the embedded ONID table even without a harvest; richer with a full harvest.
   See [plex-dvbdb-harvest](#plex-tuner-plex-dvbdb-harvest).

7. **Brand-group inheritance** — a second-pass sweep that clusters regional/quality variants
   (`ABC East`, `ABC HD`, `ABC 2`) under a canonical brand tvg-id when there is exactly one
   unambiguous linked channel for that brand.

8. **Best-stream selection** — for each tvg-id, keep only the highest-quality stream
   (UHD > HD > SD > RAW). Removes duplicate lower-quality encodes of the same channel.

9. **SDT probe** (background, last resort) — see [SDT background prober](#sdt-background-prober).
   Runs *after* the first full catalog pass has been delivered to Plex, not during indexing.

10. **Dummy guide** (`PLEX_TUNER_DUMMY_GUIDE=true`) — when enabled, the XMLTV handler appends
    24 × 6-hour placeholder programme blocks for every channel that has no real EPG programmes
    in the upstream XMLTV.  Prevents Plex from hiding channels due to empty guide slots.

**Live results (2026-02-26, hdhr-main — pre-SD/DVB harvest):**
```
Re-encode inheritance:  246 channels inherited tvg-id
Gracenote enrichment:   530 channels enriched
iptv-org enrichment:  5,634 channels enriched
Best-stream selection: 48,899 → 41,073 channels (7,826 dupes removed)
```

## iptv-org EPG enrichment

`PLEX_TUNER_IPTVORG_DB` — path to a local iptv-org channel DB JSON file produced by
`plex-tuner plex-iptvorg-harvest`. Covers ~39k channels from the community iptv-org
project (US, EU, MENA, APAC, LATAM, etc.) with canonical channel IDs that map to
XMLTV sources at `epg.pw` and `iptv-org.github.io/epg`.

Matching tiers (in order):
1. Exact normalised name → single match.
2. Stripped name (minus country prefix and quality markers) → single match.
3. Short-code from tvg-id last segment (e.g. "cnn" from "cnn.us") → single match.

```bash
plex-tuner plex-iptvorg-harvest -out /var/lib/plextuner/iptvorg.json
export PLEX_TUNER_IPTVORG_DB=/var/lib/plextuner/iptvorg.json
# "iptv-org enrichment: 5634/48899 channels enriched (DB size: 39087)"
```

## Schedules Direct EPG enrichment

`PLEX_TUNER_SD_DB` — path to a local Schedules Direct station DB JSON file produced by
`plex-tuner plex-sd-harvest`.  Provides callSign and station-name matching for the
largest US/Canada cable, satellite, and OTA channel database (~60k stations).

Produces tvg-ids in the form `SD-<stationID>` (e.g. `SD-10137` for CNN) compatible with
any Schedules Direct XMLTV grabber endpoint.

```bash
plex-tuner plex-sd-harvest \
  -username mySDuser \
  -password mySDpass \
  -out /var/lib/plextuner/sd.json
export PLEX_TUNER_SD_DB=/var/lib/plextuner/sd.json
```

> **Sign up:** Free 7-day trial; US$25/year subscription at [schedulesdirect.org](https://schedulesdirect.org).

### `plex-tuner plex-sd-harvest`

Fetches station data from the Schedules Direct SD-JSON API and builds a local DB.

```
plex-sd-harvest -username U -password P -out /path/to/sd.json [flags]
```

Flags:
- `-username` — SD account username (or `SD_USERNAME` env)
- `-password` — SD account password (or `SD_PASSWORD` env)
- `-out` — output DB path (required)
- `-countries` — comma-separated SD country codes (default: `USA,CAN,GBR,AUS,DEU,FRA,ESP,ITA,NLD,MEX`)
- `-max-lineups` — max lineups to probe per country (default: `5`, limits API calls)

## DVB services DB enrichment

`PLEX_TUNER_DVB_DB` — path to a local DVB services DB JSON file.  Used for two purposes:

1. **Triplet lookup** — when the background SDT probe has stored an ONID+TSID+SID triplet for
   a channel, the DB can resolve it to a service name and optional tvg-id.
2. **Network identification** — maps ONID (Original Network ID) to a network name (e.g.
   `0x233D` → "Sky UK") for enriched logging.

Works immediately with an **embedded ONID table** (~100 major networks worldwide) — no harvest
required for basic network identification.  A full harvest adds triplet-level resolution.

```bash
# Recommended: zero-config harvest (fetches iptv-org CSV + e2se-seeds lamedb automatically):
plex-tuner plex-dvbdb-harvest -out /var/lib/plextuner/dvbdb.json

# With additional local files for deeper triplet coverage:
plex-tuner plex-dvbdb-harvest \
  -out /var/lib/plextuner/dvbdb.json \
  -lamedb /path/to/lamedb \
  -vdr-channels /path/to/channels.conf

export PLEX_TUNER_DVB_DB=/var/lib/plextuner/dvbdb.json
```

### `plex-tuner plex-dvbdb-harvest`

Populates the DVB services DB from multiple free community data sources.  The command is
**incremental** — re-running merges new entries without discarding existing ones.

```
plex-dvbdb-harvest -out /path/to/dvbdb.json [flags]
```

#### Automatic sources (run every time, no flags needed)

| Source | What it provides | Notes |
|---|---|---|
| **Embedded ONID table** | ~100 major broadcast network names | Always present; no network needed |
| **iptv-org channels CSV** | ~47k channel names → tvg-ids, country/language | Fetched from `github.com/iptv-org/database` |
| **e2se-seeds lamedb** | Full DVB triplets (ONID+TSID+SID+name+provider) from a community Enigma2 satellite receiver channel list | Fetched from `github.com/e2se/e2se-seeds`; disable with `-e2se-seeds=false` |

#### Optional local file sources

All file flags accept **comma-separated paths** for multiple files.

| Flag | Format | Where to get files |
|---|---|---|
| `-lamedb /path` | **Enigma2 lamedb** (v3/4/5) | Your satellite receiver's service list; community forum posts; any Enigma2/OpenATV/Vu+ community site |
| `-vdr-channels /path` | **VDR channels.conf** | Also accepts **w_scan2** output directly. Run `w_scan2` with a DVB card, or find on [linuxtv.org](https://www.linuxtv.org/wiki/) forums |
| `-tvheadend-json /path` | **TvHeadend channel export** | Your TvHeadend: `GET /api/channel/grid?limit=999999` or Web UI → Channels → Export JSON |
| `-dvbservices-csv /path` | **Triplet CSV** (ONID/TSID/SID/ServiceName) | Community exports from [kingofsat.net](https://en.kingofsat.net), lyngsat.com, or your own data |
| `-lyngsat-json /path` | **Lyngsat/KingOfSat JSON** | Community scraped exports |

#### Example: combining multiple sources

```bash
plex-tuner plex-dvbdb-harvest \
  -out /plextuner-data/dvbdb.json \
  -lamedb /data/lamedb_astra28,/data/lamedb_hotbird \
  -vdr-channels /data/channels-astra.conf \
  -tvheadend-json /data/tvh-channels.json
```

#### Note on dvbservices.com

`dvbservices.com` is the official DVB registration authority for broadcasters — it is a
**paid registration service for TV operators** and does not offer a public data download.
The `-dvbservices-csv` flag accepts any CSV in the ONID/TSID/SID column format from
community sources instead.

## Dummy guide fallback

`PLEX_TUNER_DUMMY_GUIDE=true` — when enabled, the XMLTV handler appends 24 × 6-hour
placeholder programme blocks for every channel that has no real EPG programmes in the
upstream XMLTV source.

**Use case:** prevents Plex DVR and xTeVe from hiding or deactivating channels whose
guide data is missing, while real enrichment (SDT probe, SD harvest, etc.) catches up.

```bash
export PLEX_TUNER_DUMMY_GUIDE=true
```

The dummy programmes use the channel's display name as the title ("No guide data" is
implied by the blank description).  Real programmes from the upstream XMLTV always take
precedence; dummy entries are only injected for channels with no programmes at all.

## SDT background prober

Reads the **MPEG-TS Service Description Table (DVB SDT, PID 0x0011)** from live streams to
extract the broadcaster's own `service_name`.  This is the programmatic "last resort" for
unlinked channels — it works for real broadcast streams carried inside an MPEG-TS container.

### Behaviour

- **Head-start delay** (`PLEX_TUNER_SDT_PROBE_START_DELAY`, default **30 s**): the worker
  waits this long after `Run` is called before beginning its first sweep.  Set to `0` to start
  immediately — useful for testing/debugging.
- **Pauses immediately** when any IPTV stream becomes active (`gateway.ActiveStreams() > 0`).
- **Resumes automatically** after streaming has been idle for `PLEX_TUNER_SDT_PROBE_QUIET_WINDOW`
  (default **3 minutes**).
- **Polite rate-limiting**: at most `PLEX_TUNER_SDT_PROBE_CONCURRENCY` (default **2**) concurrent
  fetches, with `PLEX_TUNER_SDT_PROBE_INTER_DELAY` (default **500 ms**) between probe starts.
  Between every probe the goroutine calls `runtime.Gosched()` so it voluntarily yields CPU
  to higher-priority goroutines (main serving path, stream gateway, etc.).
- **Reads at most 256 KB** per stream — a small Range request; providers that honour `Range`
  never send more.
- **Persistent cache** (`catalog.sdtcache.json` by default) survives restarts; already-probed
  channels are skipped until `PLEX_TUNER_SDT_PROBE_TTL` expires (default **7 days**).
- After a full sweep it sleeps **24 h** before re-probing stale entries.

### When a `service_name` is found

The full DVB identity bundle (`SDTMeta`: ONID, TSID, SID, ProviderName, ServiceName,
EIT flags, now/next titles) is written to the channel in the in-memory catalog **and**
saved back to the on-disk catalog JSON (`c.UpdateLiveSDTMeta` + `c.Save`).  The live
channel list served to Plex is also refreshed via `srv.UpdateChannels`.

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `PLEX_TUNER_SDT_PROBE_ENABLED` | `false` | Set to `1` or `true` to enable. |
| `PLEX_TUNER_SDT_PROBE_START_DELAY` | `30s` | Wait before first sweep. Set `0` to start immediately (testing). |
| `PLEX_TUNER_SDT_PROBE_CACHE` | `<catalog-stem>.sdtcache.json` | Path to persistent probe cache. |
| `PLEX_TUNER_SDT_PROBE_CONCURRENCY` | `2` | Max simultaneous stream fetches. |
| `PLEX_TUNER_SDT_PROBE_INTER_DELAY` | `500ms` | Minimum delay between probe starts. |
| `PLEX_TUNER_SDT_PROBE_TIMEOUT` | `12s` | Per-stream HTTP + read timeout. |
| `PLEX_TUNER_SDT_PROBE_TTL` | `168h` | Cache TTL (7 days). Normal sweeps skip channels with fresh entries. |
| `PLEX_TUNER_SDT_PROBE_QUIET_WINDOW` | `3m` | Idle period before probing resumes. |
| `PLEX_TUNER_SDT_PROBE_RESCAN_INTERVAL` | `720h` | Auto-monthly full rescan interval (ignores TTL). Set `0` to disable. |

### Enabling in Kubernetes

Add to the `plextuner-supervisor` deployment env block:
```yaml
- name: PLEX_TUNER_SDT_PROBE_ENABLED
  value: "true"
```
The cache file auto-derives from `PLEX_TUNER_CATALOG` (e.g. `/var/lib/plextuner/catalog.sdtcache.json`).

For immediate-start testing:
```yaml
- name: PLEX_TUNER_SDT_PROBE_START_DELAY
  value: "0"
```

### Forced / monthly full rescan

By default, the SDT prober skips channels whose cache entry has not yet expired (TTL = `PLEX_TUNER_SDT_PROBE_TTL`, default 7 days). A **forced rescan** ignores the TTL and re-probes all unlinked channels:

- **Manual:** `POST /rescan` on the tuner's HTTP port
- **Auto-monthly:** `PLEX_TUNER_SDT_PROBE_RESCAN_INTERVAL` (default `720h` / 30 days)

```bash
# Trigger a forced rescan right now:
curl -X POST http://localhost:5004/rescan

# Check rescan status:
curl http://localhost:5004/rescan

# Override auto-rescan interval (e.g. every 14 days):
PLEX_TUNER_SDT_PROBE_RESCAN_INTERVAL=336h
```

| Variable | Default | Description |
|---|---|---|
| `PLEX_TUNER_SDT_PROBE_RESCAN_INTERVAL` | `720h` | Interval for automatic full rescan (ignores cache TTL). Set `0` to disable automatic rescans. |

## Manual catalog refresh

`POST /refresh` — immediately queues a full catalog re-fetch (equivalent to the normal periodic refresh, but on demand).  Returns `202 Accepted` when the signal is queued.  `GET /refresh` returns current status.

```bash
# Trigger a catalog re-fetch right now:
curl -X POST http://localhost:5004/refresh

# Check status:
curl http://localhost:5004/refresh
```

This is equivalent to the normal periodic catalog refresh but fires immediately.  Useful after:
- updating provider credentials or M3U URL
- adding/removing a second provider
- forcing enrichment re-runs after a harvest (Gracenote, iptv-org, etc.)

## Gracenote EPG enrichment

`PLEX_TUNER_GRACENOTE_DB` — path to a local Gracenote channel DB JSON file
produced by `plex-tuner plex-gracenote-harvest`.  When set and the file exists,
the app applies a Gracenote-based tvg-id enrichment step during `fetchCatalog`
(between the M3U ingest and the `LIVE_EPG_ONLY` filter).  This bridges provider
callSign-style tvg-ids (e.g. `TSN1HD`) to Gracenote `gridKey` values which Plex
can resolve to guide data.

The enrichment also activates in `epg-link-report` as **match tier 1c**, placed
between alias matching and normalised-name matching.

Workflow:

```bash
# 1. Harvest once (or periodically to pick up new channels)
plex-tuner plex-gracenote-harvest -token "$PLEX_TOKEN" \
  -out /var/lib/plextuner/gracenote.json

# 2. Point app at DB
export PLEX_TUNER_GRACENOTE_DB=/var/lib/plextuner/gracenote.json

# 3. Restart / re-index — enrichment is logged at startup:
#    "Gracenote enrichment: 312/6000 channels enriched (DB size: 20774)"
```

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

## `plex-tuner plex-session-drain`

Drain active Plex Live TV sessions via the Plex API. One-shot or continuous watch mode.

```
plex-tuner plex-session-drain \
  --plex-url http://plex.plex.svc:32400 \
  --token "$PLEX_TOKEN" \
  [--machine-id <id>] [--player-ip <ip>] [--all-live] \
  [--dry-run] [--watch] [--sse] \
  [--idle-seconds 180] [--renew-lease-seconds 300] [--lease-seconds 1800] \
  [--poll 1.0] [--wait 15]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--plex-url` | `http://127.0.0.1:32400` | Plex PMS base URL |
| `--token` | `PLEX_TUNER_PMS_TOKEN` / `PLEX_TOKEN` | Plex auth token |
| `--machine-id` | — | Filter: only this player machineIdentifier |
| `--player-ip` | — | Filter: only this player IP |
| `--all-live` | auto | Act on all live sessions (default when no filter given) |
| `--watch` | false | Watch mode: continuously reap stale sessions |
| `--sse` | false | Subscribe to Plex SSE in watch mode for faster rescans |
| `--idle-seconds` | 0 | Stop sessions idle for this many seconds |
| `--renew-lease-seconds` | 0 | Stop if no activity for N seconds (renewable) |
| `--lease-seconds` | 0 | Hard backstop: stop after this session age |
| `--dry-run` | false | Print what would be stopped without stopping |
| `--poll` | 1.0 | Poll interval (seconds) |
| `--wait` | 15 | Seconds to wait for sessions to clear after stop |

Replaces `scripts/plex-live-session-drain.py`.

## `plex-tuner plex-label-proxy`

Reverse proxy that rewrites `/media/providers` XML to give each Live TV source a distinct label using DVR lineup titles from `/livetv/dvrs`.

```
plex-tuner plex-label-proxy \
  --listen 0.0.0.0:33240 \
  --upstream http://plex.plex.svc:32400 \
  --token "$PLEX_TOKEN" \
  [--strip-prefix plextuner-] \
  [--refresh-seconds 30]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `127.0.0.1:33240` | host:port to listen on |
| `--upstream` | (required) | Plex PMS URL |
| `--token` | `PLEX_TUNER_PMS_TOKEN` / `PLEX_TOKEN` | Plex auth token |
| `--strip-prefix` | `plextuner-` | Strip this prefix from lineup titles |
| `--refresh-seconds` | 30 | How often to refresh DVR label map |

Replaces `scripts/plex-media-providers-label-proxy.py`.

## `plex-tuner vod-backfill-series`

Refetch per-series episode info from the Xtream provider API and rewrite `series[].seasons` in a `catalog.json`. Useful for repairing catalogs where episode parsing missed the `{"episodes":{"1":[...]}}` shape.

```
plex-tuner vod-backfill-series \
  --catalog-in /data/catalog.json \
  --catalog-out /data/catalog.fixed.json \
  [--progress-out /tmp/progress.json] \
  [--workers 6] [--timeout 60] [--limit 0] \
  [--retry-failed-from /tmp/progress.json]
```

Credentials are auto-derived from the first movie stream URL in the catalog. Progress is printed as JSON lines to stdout. Replaces `scripts/vod-backfill-series-catalog.py`.

## `plex-tuner generate-supervisor-config`

Generate the supervisor `JSON` config, k8s singlepod `YAML` manifest, and cutover `TSV` from the HDHR deployment template in `k3s/plex/`.

```
plex-tuner generate-supervisor-config \
  --k3s-plex-dir ../k3s/plex \
  --out-json plextuner-supervisor-multi.generated.json \
  --out-yaml plextuner-supervisor-singlepod.generated.yaml \
  --out-tsv  plextuner-supervisor-cutover-map.generated.tsv \
  [--country CA] [--timezone America/Vancouver] \
  [--cat-m3u-url http://iptv-m3u-server.plex.svc/live.m3u] \
  [--cat-xmltv-url http://iptv-m3u-server.plex.svc/xmltv.xml] \
  [--category-counts-json counts.json] [--category-cap 479] \
  [--hdhr-lineup-max 479] [--hdhr-stream-transcode on]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--k3s-plex-dir` | `../k3s/plex` | Path to directory containing `plextuner-hdhr-test-deployment.yaml` |
| `--hdhr-region-profile` | `auto` | Preset: `auto` selects by country/timezone, or force `na_en` |
| `--cat-m3u-url` | `http://iptv-m3u-server.plex.svc/live.m3u` | M3U for category DVR children |
| `--cat-xmltv-url` | `http://iptv-m3u-server.plex.svc/xmltv.xml` | XMLTV for category DVR children |
| `--category-counts-json` | — | JSON file with confirmed linked counts; enables overflow sharding |
| `--category-cap` | 479 | Per-category cap before creating overflow shards |
| `--hdhr-lineup-max` | from preset | Override HDHR wizard child lineup max |
| `--hdhr-stream-transcode` | from preset | `on`/`off`/`auto`/`auto_cached` |

Replaces `scripts/generate-k3s-supervisor-manifests.py`. The cutover TSV (previously a separate `scripts/plex-supervisor-cutover-map.py`) is now emitted directly via `--out-tsv`.

## `plex-tuner plex-probe-overrides`

Probe lineup stream URLs with `ffprobe` to detect video/audio characteristics likely to require Plex-side transcoding (interlaced content, high fps, HE-AAC, unsupported codecs, high bitrate) and emit profile/transcode override JSON files compatible with `PLEX_TUNER_PROFILE_OVERRIDES_FILE` and `PLEX_TUNER_TRANSCODE_OVERRIDES_FILE`.

```
plex-tuner plex-probe-overrides \
  --lineup-json http://localhost:5004/lineup.json \
  --emit-profile-overrides   /data/profile-overrides.json \
  --emit-transcode-overrides /data/transcode-overrides.json \
  [--base-url http://localhost:5004] \
  [--replace-url-prefix "http://old-host=http://new-host"] \
  [--channel-id ch1,ch2] \
  [--limit 0] [--timeout 12] [--bitrate-threshold 5000000] \
  [--no-transcode-overrides] [--sleep-ms 0] [--ffprobe ffprobe]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--lineup-json` | (required) | Path or URL to `lineup.json` |
| `--base-url` | `""` | Base URL to resolve relative stream URLs in lineup |
| `--replace-url-prefix` | — | Comma-separated `OLD=NEW` prefix rewrites for stream URLs |
| `--channel-id` | — | Comma-separated channel IDs to probe (omit = all) |
| `--limit` | `0` | Probe at most N channels (0 = all) |
| `--timeout` | `12` | `ffprobe` timeout per channel (seconds) |
| `--bitrate-threshold` | `5000000` | Flag channels with bitrate above this bps (0 = disabled) |
| `--emit-profile-overrides` | `""` | Write profile overrides JSON to this path |
| `--emit-transcode-overrides` | `""` | Write transcode overrides JSON to this path |
| `--no-transcode-overrides` | `false` | Suppress `transcode=true` entries in the transcode overrides file |
| `--sleep-ms` | `0` | Sleep between probes in milliseconds (polite crawling) |
| `--ffprobe` | `ffprobe` | Path to `ffprobe` binary |

Profile values assigned: `aaccfr` for severe issues (interlace / high fps / high bitrate / non-LC AAC); `plexsafe` for codec compatibility mismatches; empty string (no override) if stream looks fine.

Replaces `scripts/plex-generate-stream-overrides.py`.

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
