---
id: expl-epg-long-tail-strategies
type: explanation
status: current
tags: [epg, xmltv, gracenote, long-tail, fast, iptv, dvb, coverage]
---

# EPG Long-Tail: Strategies and Implementation Status

What the ~40k unlinked channels actually are, which strategies are now implemented,
and what the realistic coverage ceiling looks like.

## The actual breakdown of the unlinked pool (pre-enrichment)

```
41,130  no tvg-id in provider feed
  ├── 10,140 (24.7%)  ᴿᴬᵂ re-encodes          — alternate-bitrate copies
  ├──  3,646  (8.9%)  4K/UHD re-encodes        — alternate-resolution copies
  ├──  4,549 (11.1%)  PPV events               — per-event, no persistent identity
  ├──    631  (1.5%)  Filler/placeholder       — ##-prefixed padding entries
  └── 22,160 (53.9%)  Real channels, no EPG    — actual channels, just no XMLTV data
```

Of the 22,160 "real channel" pool, the largest segment is US (11,077), followed by AR,
UK, DE, NL, FR, ASIA, SE, ES, AL, CA — 50+ countries represented.

---

## Implementation status

### ✅ Implemented: Re-encode inheritance (~246 channels, growing)

Re-encodes (`ᴿᴬᵂ`/`ᵁᴴᴰ`/`4K`/`⁶⁰ᶠᵖˢ`) inherit the tvg-id from their base channel
after stripping quality markers from the display name.

Implemented in `internal/indexer/m3u.go` as `InheritTVGIDs`.  Also sets `Quality`
tier on every channel (UHD=2, HD=1, SD=0, RAW=-1) for best-stream selection.

**Live result:** 246 channels inherited tvg-id in first pass.

---

### ✅ Implemented: FAST/AVOD EPG merge

US FAST/AVOD bundles (Sling Free, Roku, Pluto/GO, Tubi) have public community XMLTV
sources.  The `iptv-m3u-updater.py` now merges extra XMLTV files before the prune step
via `EPG_URL_2`…`EPG_URL_5` or `EPG_EXTRA_URLS`.

**Deployed:** Pluto/Sling/Tubi/Roku iptv-org EPG URLs active on all instances.

---

### ✅ Implemented: Gracenote enrichment (~530 channels)

Harvests the Plex.tv Gracenote EPG API and builds a local DB of ~16k stations.  For
unlinked channels, attempts callSign/gridKey matching.

`plex-tuner plex-gracenote-harvest` — requires `PLEX_TOKEN`.
See [cli-and-env-reference.md](../reference/cli-and-env-reference.md#gracenote-epg-enrichment).

**Live result:** 530 channels enriched (mostly Portuguese and English).

---

### ✅ Implemented: iptv-org channel DB matching (~5,634 channels)

Downloads the [iptv-org/database](https://github.com/iptv-org/database) channel list
(~47k channels, 250 countries) and matches by normalised name and callSign short-code.

`plex-tuner plex-iptvorg-harvest` — no account needed.
Set `PLEX_TUNER_IPTVORG_DB=/path/to/iptvorg.json`.

**Live result:** 5,634 channels enriched in first pass.

---

### ✅ Implemented: SDT name propagation

After the background SDT probe has run, channels whose display name looks like garbage
(numeric stream IDs, UUIDs) have their name replaced with the broadcaster's own
`service_name` extracted from the MPEG-TS stream.  This feeds better data into
downstream enrichment tiers (Gracenote, iptv-org, Schedules Direct) on the next
catalog refresh.

Implemented in `internal/indexer/m3u.go` as `EnrichFromSDTMeta`.

---

### ✅ Implemented: Schedules Direct enrichment

Harvests the [Schedules Direct](https://schedulesdirect.org) SD-JSON API to build a
local station DB (~60k stations, strong US/Canada coverage).  Matches by callSign and
station name.  Produces `SD-<stationID>` tvg-ids compatible with any SD XMLTV grabber.

`plex-tuner plex-sd-harvest` — requires SD account (free 7-day trial, US$25/yr).
Set `PLEX_TUNER_SD_DB=/path/to/sd.json`.

---

### ✅ Implemented: DVB services DB + triplet enrichment

For channels where the background SDT probe has extracted a DVB triplet
(ONID+TSID+SID), the DVB services DB resolves it to a service name and optional tvg-id.
Also provides network identification from ONID alone (e.g. `0x233D` → "Sky UK").

**Data sources** (all free, no account needed):

| Source | Coverage | How obtained |
|---|---|---|
| Embedded ONID table | ~100 major broadcast networks worldwide | Built-in; instant |
| iptv-org channels CSV | ~47k name→tvg-id mappings | Auto-fetched |
| **e2se-seeds lamedb** | Full DVB triplets from community Enigma2 satellite receiver lists | Auto-fetched from GitHub |
| Enigma2 lamedb (local) | Any satellite receiver's full channel list | Your receiver / community forums |
| VDR channels.conf | Full triplets from VDR/w_scan2 scans | linuxtv.org forums, or run `w_scan2` |
| TvHeadend channel JSON | Full triplets from any TvHeadend instance | `/api/channel/grid` export |
| Community CSVs / JSON | Varies | KingOfSat, lyngsat community exports |

`plex-tuner plex-dvbdb-harvest` — no account needed for any source.
Set `PLEX_TUNER_DVB_DB=/path/to/dvbdb.json`.

---

### ✅ Implemented: Brand-group inheritance

A second-pass sweep clusters regional and quality variants of the same channel
(`ABC East`, `ABC HD`, `ABC 2`, `ABC (WABC)`) under the canonical brand's tvg-id.
Only applies when there is exactly one unambiguous linked channel for that brand token
(no false positives from ambiguous names).

Implemented in `internal/indexer/m3u.go` as `InheritTVGIDsByBrandGroup`.

---

### ✅ Implemented: Best-stream selection

For each tvg-id, only the highest-quality stream is kept (UHD > HD > SD > RAW).
Lower-quality duplicates are removed before the lineup is built.

Implemented in `internal/indexer/m3u.go` as `SelectBestStreams`.
**Live result:** 48,899 → 41,073 channels (7,826 lower-quality dupes removed).

---

### ✅ Implemented: SDT background prober

Reads the MPEG-TS Service Description Table (DVB SDT, PID 0x0011) from live streams
to extract the broadcaster's own `service_name`, provider name, DVB triplet, and
EIT now/next programme data.  Runs in the background after first catalog delivery,
pauses automatically when any IPTV stream is active.

Implemented in `internal/sdtprobe/`.
Enable with `PLEX_TUNER_SDT_PROBE_ENABLED=true`.
See [cli-and-env-reference.md](../reference/cli-and-env-reference.md#sdt-background-prober).

---

### ✅ Implemented: Dummy guide fallback

When `PLEX_TUNER_DUMMY_GUIDE=true`, the XMLTV handler injects 24 × 6-hour placeholder
programme blocks for every channel that has no real EPG data.  Prevents Plex from
hiding or deactivating unlinked channels while enrichment catches up.

---

## Strategies not yet implemented

### 🔲 Logo-hash inheritance

If two channels share the same logo URL/hash and high name similarity, inherit the EPG
mapping.  High ROI for channels that differ only in quality suffix.  Low risk since
logos are stable.  Filed in `memory-bank/opportunities.md`.

### 🔲 Schedule fingerprinting

Compare a short window of programme titles/descriptions against candidate EPG channels
to match channels where the name is ambiguous but the schedule is unique.  Powerful
but expensive; best reserved for high-value unmatched channels after all other tiers.

### 🔲 Scored multi-pass matcher with review queue

Automated confidence scoring across all match evidence (name, SDT service_name,
triplet, logo, schedule) with a human-review queue for medium-confidence candidates.
The architecture is documented in [epg-linking-pipeline.md](../reference/epg-linking-pipeline.md).

### 🔲 Video / logo frame fingerprinting

Extract a frame from the stream and compare against a channel logo database.  Very
high effort, computationally expensive; lower priority given the coverage already
achieved by other tiers.

---

## PPV channels (not solvable programmatically)

The ~4,500 PPV entries have no persistent channel identity.  Each event is a one-time
stream with an ad-hoc name; no EPG database tracks future PPV events.  These are
fundamentally not EPG-matchable.  The `LIVE_EPG_ONLY` filter already suppresses them
from DVR buckets.

---

## Current coverage results (live, post-enrichment)

```
Re-encode inheritance:       246 channels
Gracenote enrichment:        530 channels
iptv-org enrichment:       5,634 channels
Brand-group inheritance:   (running, results accumulating)
Schedules Direct:          (harvest pending)
DVB DB / triplet:          (harvest + SDT probe ongoing)
────────────────────────────────────────────────────────
Subtotal new links:       ~6,410+ channels
Best-stream dedup:        48,899 → 41,073 (7,826 removed)
```

With Schedules Direct harvest + continued SDT probing + DVB DB triplet matching,
the realistic ceiling for additional links is **8,000–15,000 more channels** on top
of the current 6,410, concentrated in US/CA/UK/EU broadcast channels.

The hard floor is ~5,000–10,000 channels that genuinely do not exist in any publicly
available EPG database (niche regional IPTV, PPV copies, dead feeds, pure foreign
channels with no English-language EPG coverage).

---

## Recommended next actions

1. **Run the DVB DB harvest** — zero cost, auto-fetches e2se-seeds + iptv-org:
   ```bash
   plex-tuner plex-dvbdb-harvest -out /plextuner-data/dvbdb.json
   # then: PLEX_TUNER_DVB_DB=/plextuner-data/dvbdb.json
   ```

2. **Sign up for Schedules Direct** (US$25/yr) and run the SD harvest for strong
   US/CA/UK station coverage:
   ```bash
   plex-tuner plex-sd-harvest -username U -password P -out /plextuner-data/sd.json
   # then: PLEX_TUNER_SD_DB=/plextuner-data/sd.json
   ```

3. **Enable the SDT prober** — it self-populates the triplet DB over time:
   ```bash
   PLEX_TUNER_SDT_PROBE_ENABLED=true
   PLEX_TUNER_SDT_PROBE_START_DELAY=0  # or 30s for production
   ```

4. **Supply local DVB channel lists** if you have satellite receiver hardware or
   can obtain community lamedb/VDR files:
   ```bash
   plex-tuner plex-dvbdb-harvest -out /plextuner-data/dvbdb.json \
     -lamedb /data/lamedb_astra28,/data/lamedb_hotbird \
     -vdr-channels /data/channels.conf
   ```

---

## See also

- [EPG Coverage and the Long Tail](epg-coverage-and-long-tail.md) — pipeline accounting
- [EPG Linking Pipeline](../reference/epg-linking-pipeline.md) — matching tier design
- [CLI and env reference](../reference/cli-and-env-reference.md) — all env vars and commands
- [iptv-org/epg](https://github.com/iptv-org/epg) — community FAST/AVOD EPG aggregator
- [e2se/e2se-seeds](https://github.com/e2se/e2se-seeds) — community Enigma2 lamedb
