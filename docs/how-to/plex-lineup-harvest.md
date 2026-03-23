---
id: plex-lineup-harvest
type: how-to
status: draft
tags: [plex, lineup, guide, programming-manager]
---

# Harvest Plex lineup candidates

Use `iptv-tunerr plex-lineup-harvest` to harvest lineup candidates from Plex.

It has two modes:
- `oracle`
  - probes Plex's HDHR guide-matching flow across several tuner lineup variants
  - captures which lineup titles Plex maps back from those synthetic tuner shapes
- `provider`
  - queries Plex's real provider lineup catalog directly by country + postal code
  - captures real cable / satellite / OTA lineup titles and optional channel rows

Use `oracle` when you are shaping a wizard-safe XMLTV tuner and want to see
what Plex maps back. Use `provider` when you want real Plex provider titles in
the saved harvest report instead of synthetic `harvest-*` names.

## What it does

In `oracle` mode, Tunerr:

1. registers a temporary HDHR device in Plex
2. creates a DVR entry
3. optionally reloads the guide
4. polls Plex channel-map results for a bounded time
5. captures the resolved lineup title, lineup URL, and channel-map count

In `provider` mode, Tunerr:

1. queries Plex's provider EPG service for the configured country + postal code
2. optionally filters by lineup type and title substring
3. optionally fetches channel rows for each returned lineup
4. saves the real provider lineup titles and rows into the harvest report

The JSON report dedupes discovered lineup titles into a summary list so you can
quickly see which candidates look strongest.

## Example: cap sweep

```bash
iptv-tunerr plex-lineup-harvest \
  -mode oracle \
  -plex-url http://plex.example:32400 \
  -token "$PLEX_TOKEN" \
  -base-url-template http://iptvtunerr-hdhr-cap{cap}.plex.home \
  -caps 100,200,300,400,479,600,700 \
  -friendly-name-prefix harvest- \
  -wait 60s \
  -out /tmp/lineup-harvest.json
```

## Example: direct tuner URLs

```bash
iptv-tunerr plex-lineup-harvest \
  -mode oracle \
  -plex-url http://plex.example:32400 \
  -token "$PLEX_TOKEN" \
  -base-urls http://tuner-a:5004,http://tuner-b:5004 \
  -wait 30s
```

## Example: real provider lineup harvest

```bash
iptv-tunerr plex-lineup-harvest \
  -mode provider \
  -token "$PLEX_TOKEN" \
  -country CA \
  -postal-code "S4P 3X1" \
  -lineup-types cable,ota \
  -title-query access \
  -lineup-limit 12 \
  -include-channels \
  -out /tmp/provider-lineup-harvest.json
```

## Common flags

- `-plex-url`
- `-token`
- `-mode`
- `-base-urls`
- `-base-url-template`
- `-caps`
- `-friendly-name-prefix`
- `-country`
- `-postal-code`
- `-lineup-types`
- `-title-query`
- `-lineup-limit`
- `-include-channels`
- `-provider-base-url`
- `-provider-version`
- `-wait`
- `-poll`
- `-reload-guide`
- `-activate`
- `-out`

## Output shape

The report includes:

- `results[]`
  - one row per tested oracle target or provider lineup
  - device/DVR ids in `oracle` mode
  - provider lineup id/type/source in `provider` mode
  - discovered `lineup_title`
  - `channelmap_rows`
  - `channel_count`
  - any per-target error
- `lineups[]`
  - deduped summary by lineup title
  - strongest `channelmap_rows`
  - which targets/friendly names hit that lineup

## Notes

- `oracle` mode creates real Plex DVR/device rows. Use it against a test Plex
  instance or clean up after experiments.
- For cleanup, use `iptv-tunerr plex-epg-oracle-cleanup`.
- The saved report is available through `/programming/harvest.json`, can be
  requested live through `/programming/harvest-request.json`, and can feed
  Programming Manager via `/programming/harvest-import.json`.
- If Plex's provider EPG service returns an upstream error for a country/postal
  query, Tunerr now preserves that provider failure in the report instead of
  silently substituting synthetic `harvest-*` lineup titles.
- Canadian provider queries work with a full postal code such as `S4P3X1` or
  `S4P 3X1`. Tunerr normalizes lowercase country input like `ca` to `CA`
  before it hits Plex.
- When `-mode provider` is used without `-country` / `-postal-code`, Tunerr
  falls back to a built-in timezone lookup:
  - first an exact timezone map for many common zones such as
    `America/Regina -> CA / S4P 3X1`, `Europe/Berlin -> DE / 10115`,
    `Asia/Tokyo -> JP / 100-0001`
  - then a timezone-family approximation when there is no exact hit, for
    example `Africa/*`, `Europe/*`, `Asia/*`, `Pacific/*`
  - the lookup data now lives in `internal/plexharvest/provider_tz_defaults.json`
  Explicit flags and env vars still win.

## See also

- [EPIC-lineup-harvest](../epics/EPIC-lineup-harvest.md)
- [EPIC-programming-manager](../epics/EPIC-programming-manager.md)
