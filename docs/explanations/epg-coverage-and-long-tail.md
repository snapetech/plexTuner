---
id: expl-epg-coverage-long-tail
type: explanation
status: current
tags: [epg, xmltv, gracenote, iptv, coverage, matching]
---

# EPG Coverage, the Long Tail, and Gracenote Enrichment

Why your 48k-channel feed produces only ~3,200 Plex DVR channels, what the
"long tail" actually is, and what Gracenote enrichment does and doesn't fix.

## The pipeline in numbers

```
Raw Xtream API feed              48,899  channels
   │
   ├─ Provider XMLTV (before prune)  ~7,109  channels with EPG data
   │    ├─ dropped: no programmes      874
   │    └─ dropped: all unavailable     43
   ├─ Provider XMLTV (after prune)   6,192  channels
   │
   ├─ M3U entries dropped by EPG prune 1,572  (dupes of pruned ids)
   │
   ├─ Deduplicated by tvg-id          3,232  unique EPG-linked channels
   │    └─ Split across 13 DVR buckets (all at 100% XMLTV match rate)
   │
   └─ No EPG data from provider      ~40,135  ← the long tail
```

The **short answer** to "why only 3,232?":

1. Your IPTV provider carries ~49k streams, but their own XMLTV feed only covers
   ~7,100 of them with actual programme data.
2. Of those 7,100, ~1,600 are duplicates of the same channel (multiple resolutions,
   mirror hosts, etc.) — after deduplication by `tvg-id` you're left with 3,232 unique
   identities.
3. The `split-m3u.py` splitter then keeps only EPG-linked channels (`EPG_PRUNE_DROP_FROM_M3U=1`),
   so all 13 DVR buckets are fed exclusively from that 3,232 pool.

This is **not** a Canada/English filter. The XMLTV covers 68 countries and 3,232 unique
channel IDs spanning US, EU, MENA, SA, APAC, and more.

## What the XMLTV actually covers

The provider XMLTV (`/xmltv.php?username=...`) serves a flat XML file with channel
entries + programme schedules. Breakdown by country TLD (top 15 as of 2026-02-27):

| TLD | Count | TLD | Count | TLD | Count |
|-----|-------|-----|-------|-----|-------|
| .us | 344   | .pt | 120   | .gr |  82  |
| .in | 213   | .ru | 102   | .se |  75  |
| .fr | 189   | .nl |  90   | .hu |  75  |
| .uk | 180   | .it |  87   | .no |  69  |
| .pl | 163   | .rs |  86   | ... | ...  |
| .ca | 163   | .ro |  82   |     |      |

Total: 68 country TLDs represented. The feed is already global; what's missing is
**programme data for the other ~42,000 channels** — those streams simply don't have
an EPG entry in the provider's XMLTV at all.

## The long tail: what it is

The ~40,000 channels with no EPG are not a matching problem — they are channels for
which no programme schedule data exists in any source we query:

- **RAW/re-encoded streams** (channels labelled `ᴿᴬᵂ`, `⁶⁰ᶠᵖˢ`, `ᵁᴴᴰ`) — these are
  alternate encodes of existing channels, not distinct channels in any EPG database
- **PPV / event feeds** — per-event streams that have no persistent channel identity
- **Low-tier IPTV-only content** — regional/niche channels that no EPG aggregator
  tracks (particularly the 499 Portuguese channels identified in Gracenote enrichment)
- **Duplicate streams** — the same channel at different bitrates or from different CDN
  hosts, all sharing one `tvg-id` after dedup

These channels are in the provider's stream catalog but not in Gracenote's 15,998-channel
canonical database either.

## What Gracenote enrichment does

The `PLEX_TUNER_GRACENOTE_DB` enrichment step runs during `fetchCatalog` on the `hdhr-main`
and `hdhr-main2` instances (the broad, unfiltered feeds). It looks at channels that have
**no tvg-id** and attempts to assign a Gracenote `gridKey` by matching the channel's
display name or callSign against the 15,998-channel Gracenote DB.

Current results (as of 2026-02-27):

```
hdhr-main / hdhr-main2:
  47,327 channels scanned
  542 enriched  (blank tvg-id → Gracenote gridKey assigned)
  6,191 already had a matching XMLTV id
  548 unfixable: either already a gridKey with no XMLTV coverage, or no match anywhere
```

The 542 enriched channels now have a Gracenote `gridKey` (24-char hex) as their
`tvg-id`. Whether Plex's EPG can resolve those gridKeys into programme data depends on
whether Plex's own guide source (Gracenote/Rovi) covers those channels — that's outside
our control.

### Why Gracenote doesn't fix the long tail

The 542 enriched channels are:

| Language | Count |
|----------|-------|
| Portuguese | 499 |
| English    |  28 |
| German     |   5 |
| Spanish    |   4 |
| Italian    |   4 |
| Other      |   2 |

These are channels whose display names in the raw feed matched a Gracenote callSign —
meaning Gracenote has a canonical record for them. However, our `iptv-m3u-server`
provider XMLTV does **not** contain entries for these gridKeys. So the gridKey is
correct, but the local XMLTV has no programme data for it.

The remaining ~40,000 long-tail channels didn't match Gracenote either — they are
truly off-the-grid streams.

## The alias/epg-link-report workflow

There is a second class of channels that are a better fit for the alias workflow:
channels that **have** a `tvg-id` but it's slightly wrong relative to what's in the
XMLTV. For example:

- `TSN1HD.ca` in the feed vs `TSN1.ca` in XMLTV
- `CBS.us` in feed vs `CBS` in XMLTV (missing TLD)

The normalization in `internal/epglink` already handles suffix stripping (`HD`, `DT`)
and TLD stripping, so most of these resolve automatically. The alias file
(`-aliases` flag to `epg-link-report`) is for cases where even normalized matching
fails and manual mapping is needed.

**Current state**: the normalized matching already captures all of these. Running
`epg-link-report` on the current `hdhr-main` catalog shows 0 channels in the
"has tvg-id but no XMLTV match" bucket (other than 6 known Albanian/Polish corner cases
that are genuinely absent from the XMLTV source).

## The 6 known unfixable channels

These 5 Albanian and 1 Polish channel are in the provider feed with `tvg-id` values
that don't match anything in the XMLTV, and Gracenote doesn't know them either:

| tvg-id        | Display name    | Why unfixable |
|---------------|-----------------|---------------|
| `kino1.al`    | GOLD: KINO 1    | No XMLTV entry for `.al` kino channels |
| `kino2.al`    | GOLD: KINO 2    | Same |
| `kino3.al`    | GOLD: KINO 3    | Same |
| `episode.al`  | GOLD: EPISODE   | No XMLTV entry |
| `primetv.al`  | GOLD: PRIME     | No XMLTV entry |
| `PL: TVN CZAS NA ŚLUB ᴴᴰ ◉` | (same as tvg-id) | tvg-id is actually a display name, not an id |

The Polish channel's tvg-id is its own display name with Unicode decorators — the M3U
source has a malformed entry where no proper `tvg-id` was set. This would need a manual
alias or a source fix.

## Summary: what actually improves EPG coverage

| Mechanism | What it fixes | Current state |
|-----------|--------------|---------------|
| Provider XMLTV | The 3,232 unique EPG-linked channels | ✅ Working |
| Gracenote enrichment | 542 previously-blank tvg-ids → gridKeys | ✅ Working (gridKeys set; Plex may resolve) |
| Alias overrides | tvg-id near-misses | ✅ Already captured by normalization; 0 gaps |
| Better XMLTV source | The ~40k long-tail channels | ❌ Would require a different EPG data provider |
| Gracenote for long tail | ~40k channels with no EPG | ❌ They're not in Gracenote's DB either |

The long tail is a **data gap in the IPTV provider's EPG** — it cannot be bridged by
matching logic alone. The streams exist; the schedule data does not.

## See also

- [EPG Linking Pipeline](epg-linking-pipeline.md) — matching tier design and alias workflow
- [Gracenote harvest CLI](cli-and-env-reference.md#plex-gracenote-harvest) — how to re-harvest
- [`PLEX_TUNER_GRACENOTE_DB`](cli-and-env-reference.md#plex_tuner_gracenote_db) — runtime enrichment config
- [Plex wizard EPG harvest](plex-wizard-epg-harvest.md) — how the Gracenote DB was built
