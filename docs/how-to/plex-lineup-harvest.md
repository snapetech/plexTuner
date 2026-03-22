---
id: plex-lineup-harvest
type: how-to
status: draft
tags: [plex, lineup, guide, programming-manager]
---

# Harvest Plex lineup candidates

Use `iptv-tunerr plex-lineup-harvest` to probe Plex's HDHR guide-matching flow
across several tuner lineup variants and capture which lineup titles Plex
actually offers back.

This is the productized version of the older oracle experiment path. It is
useful when you want to try several lineup caps or shapes, then decide which
one should feed a wizard-safe tuner or a Programming Manager recipe.

## What it does

For each target tuner URL, Tunerr:

1. registers a temporary HDHR device in Plex
2. creates a DVR entry
3. optionally reloads the guide
4. polls Plex channel-map results for a bounded time
5. captures the resolved lineup title, lineup URL, and channel-map count

The JSON report also dedupes discovered lineup titles into a summary list so
you can quickly see which candidates look strongest.

## Example: cap sweep

```bash
iptv-tunerr plex-lineup-harvest \
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
  -plex-url http://plex.example:32400 \
  -token "$PLEX_TOKEN" \
  -base-urls http://tuner-a:5004,http://tuner-b:5004 \
  -wait 30s
```

## Common flags

- `-plex-url`
- `-token`
- `-base-urls`
- `-base-url-template`
- `-caps`
- `-friendly-name-prefix`
- `-wait`
- `-poll`
- `-reload-guide`
- `-activate`
- `-out`

## Output shape

The report includes:

- `results[]`
  - one row per tested tuner target
  - device/DVR ids
  - discovered `lineup_title`
  - `channelmap_rows`
  - any per-target error
- `lineups[]`
  - deduped summary by lineup title
  - strongest `channelmap_rows`
  - which targets/friendly names hit that lineup

## Notes

- This creates real Plex DVR/device rows. Use it against a test Plex instance or
  clean up after experiments.
- For cleanup, use `iptv-tunerr plex-epg-oracle-cleanup`.
- This does not yet import the harvested results directly into Programming
  Manager, but that is the intended next bridge.

## See also

- [EPIC-lineup-harvest](../epics/EPIC-lineup-harvest.md)
- [EPIC-programming-manager](../epics/EPIC-programming-manager.md)
