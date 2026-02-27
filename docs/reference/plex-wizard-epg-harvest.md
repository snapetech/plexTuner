---
id: ref-plex-wizard-epg-harvest
type: reference
status: active
tags: [reference, epg, gracenote, xmltv, wizard, oracle, harvest, scripts]
---

# Plex Wizard EPG Harvest

`scripts/plex-wizard-epg-harvest.py` — Simulates the Plex DVR setup wizard's
"suggest a guide" step to discover Gracenote EPG lineups and channels for any
postal code, aggregate unique channels across all lineups, cross-match against
the existing XMLTV source, and emit XMLTV `<channel>` stubs.

See also: [EPG Linking Pipeline](epg-linking-pipeline.md), [Testing and Supervisor Config](testing-and-supervisor-config.md)

---

## Background: How the Plex DVR Wizard Works

When you add an HDHomeRun device in the Plex UI, the wizard calls:

```
GET https://epg.provider.plex.tv/lineups
    ?country=CA
    &postalCode=S4P3Y2
    &X-Plex-Token=<token>
```

Plex returns a list of cable/OTA providers for that postal code (sourced from
Gracenote).  You pick a provider, Plex fetches its channel list, and maps your
tuner's channels against the Gracenote guide by callSign/gridKey.

### Key finding: channel count does not matter

The oracle probe (run via `--probe-counts`) confirms that advertising 100, 200,
300, 479, 600, 800, or 1000 channels in your HDHomeRun discover/lineup JSON
produces **identical lineup suggestions**.  The suggestions are purely postal-code
driven.  There is no "trick" to get more guide options by inflating the channel
count.

For Regina, Saskatchewan (S4P 3Y2), plex.tv returns **9 lineups** regardless:

| Type  | Provider                          | Declared ch | Actual ch |
|-------|-----------------------------------|:-----------:|:---------:|
| cable | Access Communications             | 494         | 488       |
| cable | Access NexTV                      | 396         | 396       |
| cable | Access Ngv                        | 514         | 514       |
| cable | Access Now                        | 11          | 11        |
| cable | Comwave                           | 318         | 312       |
| cable | Hospitality Network               | 80          | 79        |
| ota   | Local Broadcast Listings          | 4           | 4         |
| cable | SaskTel Max                       | 594         | 586       |
| cable | VMedia                            | 282         | 279       |

**993 unique channels** (by Gracenote `gridKey`) across all 9 lineups.

Language breakdown: 800 en, 96 fr, 28 hi, 10 ar, 7 ur/pa/ru, 6 es/it, ...

---

## Cross-match results against our XMLTV

Our `iptv-m3u-server/xmltv.xml` (6,192 channels / 3,232 unique IDs) covers
**95 of 993** Gracenote channels (**9.6% match rate**).

This is expected: our XMLTV comes from an IPTV provider whose channel IDs use
`CallSign.ca` format (e.g. `CBKT.ca`, `TSN1.ca`), while Gracenote uses
callSign codes with HD/DT suffixes (`CBKTDT`, `TSN1HD`).  The script normalises
both sides (strips `.ca`/`.us`/`.uk` and `HD`/`DT`/`HBO` suffixes) before comparing.

All 95 matches go through the **fuzzy-normalised** path.  The remaining 898
unmatched channels (718 English) represent gaps where our IPTV source has no
Gracenote-compatible guide data.  These are primarily:
- Regional Canadian community channels (Access, CKCK-area locals)
- Canadian specialty: AHCC, Adult Swim Canada, NHL Plus
- US Detroit-market channels (WDIV, WXYZ, WWJ, WTVS)
- South Asian, Arabic, Filipino language channels

---

## Usage

### Prerequisites

```bash
pip install  # no external dependencies — stdlib only
```

### Basic harvest (write XMLTV stubs to stdout)

```bash
python3 scripts/plex-wizard-epg-harvest.py \
    --token "$PLEX_TOKEN" \
    --postal S4P3Y2 --country CA
```

### Harvest with cross-match + file output

```bash
python3 scripts/plex-wizard-epg-harvest.py \
    --token "$PLEX_TOKEN" \
    --postal S4P3Y2 --country CA \
    --xmltv http://iptv-m3u-server.plex.svc/xmltv.xml \
    --out /tmp/gracenote-stubs.xml \
    --report /tmp/harvest-report.json \
    --verbose
```

### English-only stubs (for iptv-m3u-server injection)

```bash
python3 scripts/plex-wizard-epg-harvest.py \
    --token "$PLEX_TOKEN" \
    --postal S4P3Y2 --country CA \
    --lang-filter en \
    --out /tmp/gracenote-en-stubs.xml
```

### Oracle probe: does channel count change guide suggestions?

```bash
python3 scripts/plex-wizard-epg-harvest.py \
    --token "$PLEX_TOKEN" \
    --postal S4P3Y2 --country CA \
    --probe-counts 100,200,300,400,479,500,600,700,800,1000
```

---

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--token` | `$PLEX_TOKEN` | Plex auth token |
| `--postal` | `S4P3Y2` | Postal/ZIP code (no spaces) |
| `--country` | `CA` | ISO country code |
| `--epg-base` | `https://epg.provider.plex.tv` | plex.tv EPG base URL |
| `--xmltv` | _(none)_ | XMLTV URL or file to cross-match against |
| `--out` | stdout | Write XMLTV stubs here |
| `--report` | _(none)_ | Write full JSON report here |
| `--probe-counts` | _(none)_ | Comma-separated channel counts for oracle test |
| `--lang-filter` | _(all)_ | Only emit channels with this language |
| `--verbose` / `-v` | off | Per-lineup progress |

Environment variables `PLEX_TOKEN`, `PLEX_POSTAL`, `PLEX_COUNTRY`, `EPG_API_BASE`
override the defaults.

---

## Output: XMLTV stubs

The `--out` file is a valid XMLTV `<tv>` document with one `<channel>` per unique
Gracenote `gridKey`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<tv source-info-name="Gracenote via plex.tv (CA/S4P3Y2)">
  <channel id="5fc705daa05ef8002e61581c">
    <display-name lang="en">CBC Saskatchewan HD</display-name>
    <display-name>CBKTDT</display-name>
  </channel>
  ...
</tv>
```

The `id` attribute is the Gracenote `gridKey` (a 24-hex-char Mongo-style ID).
The first `<display-name>` is the human title; the second is the callSign.

**These stubs alone do not provide guide data** (no `<programme>` elements).  They
serve two purposes:

1. **Gap analysis** — compare against your XMLTV source to find channels where
   Gracenote knows about a station that your IPTV provider doesn't have guide data for.
2. **iptv-m3u-server injection** — a future enhancement could feed these stubs
   into iptv-m3u-server so that Plex's guide matcher can find the channel even if
   your IPTV M3U uses a non-standard tvg-id.

---

## Output: JSON report

```json
{
  "postal": "S4P3Y2",
  "country": "CA",
  "lineups": [ { "id": "...", "title": "SaskTel Max (594 channels)", "type": "cable", ... } ],
  "totalUniqueChannels": 993,
  "allChannels": [
    { "gridKey": "5fc705daa05ef8002e61581c", "callSign": "CBKTDT",
      "title": "CBC Saskatchewan HD", "vcn": "004",
      "isHd": true, "language": "en", "lineupCount": 4 }
  ],
  "matchResult": {
    "total_gracenote": 993,
    "matched": 95,
    "unmatched": 898,
    "match_pct": 9.6,
    "match_methods": { "fuzzy_normalised": 95 },
    "lang_unmatched": { "en": 718, "fr": 84, ... },
    "matched_channels": [ { ..., "xmltvId": "CBKT.ca" } ],
    "unmatched_channels": [ ... ]
  }
}
```

---

## Matching logic

Cross-match runs in three priority tiers:

1. **Exact gridKey** — `gridKey == xmltv_id` (currently 0 hits; our XMLTV doesn't use Gracenote IDs)
2. **Exact callSign** — `callSign == xmltv_id`
3. **Fuzzy normalised** — strip country suffix (`.ca`, `.us`, `.uk`) from XMLTV IDs, strip
   `HD`/`DT`/`HBO`/`H` suffix from Gracenote callSigns, lowercase both.
   Also tries stripping trailing digits (`TSN1` → `TSN`) as a last resort.

---

## API reference: plex.tv EPG endpoints used

| Method | URL | Description |
|--------|-----|-------------|
| `GET` | `https://epg.provider.plex.tv/lineups?country=CA&postalCode=S4P3Y2` | List guide providers for a postal code |
| `GET` | `https://epg.provider.plex.tv/lineups/{id}/channels` | All channels in one lineup |

Authentication: `X-Plex-Token` query param (same token as your local Plex server).

---

## World harvest (`--world`)

### Supported regions

| Region | Countries | Notes |
|--------|-----------|-------|
| North America — US | US | 34 cities; all major DMAs |
| North America — Canada | CA | 18 cities coast-to-coast |
| Latin America — Mexico & Central America | MX, CR, PA, PR | |
| Latin America — South America | CO, UY, EC, PE | BR/AR/CL/VE not supported by Gracenote |
| Europe — Western | DE, FR, IT, ES, NL, BE, CH, AT, PT, IE | **GB not supported** by plex.tv Gracenote |
| Europe — Nordic | SE, NO, DK, FI | |
| Europe — Eastern | PL, RU | CZ/HU/RO/GR/SK/HR/BG/RS/UA not supported |
| Asia-Pacific — India | IN (10 cities) | |
| Oceania | AU (8 cities), NZ (3 cities) | |

**Not supported by plex.tv/Gracenote:** GB, TR, JP, KR, SG, HK, PH, TH, MY, ID, TW, AE, SA, IL, ZA, EG, NG, KE, BR, AR, CL, VE and most of Eastern Europe.

### World harvest results (2026-02-27)

| Region | Lineups | New channels | Running total |
|--------|--------:|-------------:|--------------:|
| North America — US | 214 | 7,869 | 7,869 |
| North America — Canada | 163 | 2,490 | 10,359 |
| Latin America — Mexico & CA | 56 | 1,181 | 11,540 |
| Latin America — South America | 34 | 349 | 11,889 |
| Europe — Western | 153 | 4,354 | 16,243 |
| Europe — Nordic | 47 | 396 | 16,639 |
| Europe — Eastern | 85 | 1,363 | 18,002 |
| Asia-Pacific — India | 55 | 2,128 | 20,130 |
| Oceania | 44 | 644 | **20,774** |

**851 unique lineups → 20,774 unique Gracenote channels**

Language distribution: en 12,193 · es 2,278 · de 1,569 · ru 1,018 · fr 851 · it 336 · hi 326 · pl 320 · nl 278 · pt 265 · sv 189 …

XMLTV cross-match: **800 matched (3.9%)** — all 20,774 stubs in `/tmp/world-gracenote-stubs.xml` (3.2 MB).

The low match rate is expected: our XMLTV uses `callSign.cc` IDs while Gracenote uses hex `gridKey`s with callSign codes.  The 3.9% are the channels where our normalisation (`strip .ca/.us`, strip `HD`/`DT`) produces a collision.

### Scale comparison

| Scope | Unique channels |
|-------|----------------:|
| Regina SK only | 993 |
| All Canada | 2,917 |
| World (`--world`) | **20,774** |

### Usage

```bash
# Full world harvest
python3 scripts/plex-wizard-epg-harvest.py \
    --token "$PLEX_TOKEN" \
    --world \
    --xmltv /tmp/xmltv_full.xml \
    --out /tmp/world-stubs.xml \
    --report /tmp/world-report.json \
    --verbose

# Subset of regions only
python3 scripts/plex-wizard-epg-harvest.py \
    --token "$PLEX_TOKEN" \
    --world \
    --regions "Europe -- Western,Europe -- Nordic" \
    --out /tmp/eu-stubs.xml

# English channels only
python3 scripts/plex-wizard-epg-harvest.py \
    --token "$PLEX_TOKEN" \
    --world \
    --lang-filter en \
    --out /tmp/world-en-stubs.xml
```

---

## Integration with iptv-m3u-server

The unmatched stubs (`--lang-filter en`) could be added as a secondary XMLTV source
in iptv-m3u-server.  However, they contain no `<programme>` data — they are channel
shells only.  To get actual guide data for those 898 unmatched channels you would
need a Gracenote data feed or a supplementary XMLTV source that covers North American
cable (e.g. `schedulesdirect.org`).

A future `--fetch-programmes` flag could call Plex's guide data endpoint
(`/tv.plex.providers.epg.xmltv:<id>/sections/1/all`) to pull scheduled programming
for channels that ARE already registered in Plex and supplement the existing XML.
