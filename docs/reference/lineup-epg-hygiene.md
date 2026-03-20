---
id: lineup-epg-hygiene
type: reference
status: stable
tags: [reference, lineup, epg, catalog, hr-005]
---

# Lineup and EPG hygiene (built-in)

IPTV Tunerr applies **catalog-time** and **serve-time** hygiene so lineup and guide stay aligned without a separate preprocessor. This maps [work breakdown HR-005](../../memory-bank/work_breakdown.md) to concrete knobs and code paths.

## Index / `catalog.json` build (`index`, `iptv-tunerr` catalog refresh)

| Step | What it does | Where |
|------|----------------|-------|
| **tvg-id dedupe** | Merges rows that share the same **`tvg_id`**, unioning **`stream_urls`** (and stream auths). Runs after each M3U/player_api path and **once more after** free-source + HDHR hardware merges. | `cmd/iptv-tunerr/cmd_catalog.go` — **`dedupeByTVGID`**, **`maybeDedupeByTVGID`** |
| **Dedupe opt-out** | Set **`IPTV_TUNERR_DEDUPE_BY_TVG_ID=false`** to skip all tvg-id merging (niche debugging). Default **on**. | env |
| **Strip blocked hosts** | Drops stream URLs whose host matches **`IPTV_TUNERR_STRIP_STREAM_HOSTS`**; drops channels with no URLs left. | **`stripStreamHosts`** |
| **Stable order** | **`catalog.ReplaceWithLive`** sorts **`live_channels`** by **`channel_id`** (HR-006). | **`internal/catalog/catalog.go`** |
| **EPG-linked only** | **`IPTV_TUNERR_LIVE_EPG_ONLY=1`** keeps channels with **`epg_linked`** after XMLTV match / repairs. | **`fetchCatalog`** |
| **Smoketest filter** | Optional live URL probe cache to drop dead streams before save. | **`indexer.FilterLiveBySmoketestWithCache`** |
| **DNA assign** | Stable **`dna_id`** for duplicate variants. | **`channeldna.Assign`** |

## Serve / guide alignment (`serve`)

| Step | What it does | Where |
|------|----------------|-------|
| **Guide-quality policy** | **`IPTV_TUNERR_GUIDE_POLICY`** `healthy` \| `strict` drops weak guide rows from the **in-memory lineup** once guide-health is cached (defer if guide not ready). | **`internal/tuner/guide_policy.go`**, **`Server.UpdateChannels`** |
| **Prune unlinked in XMLTV** | **`IPTV_TUNERR_EPG_PRUNE_UNLINKED`** removes unmatched `<channel>` / programmes from emitted **`guide.xml`** when enabled. | XMLTV pipeline |
| **Catch-up / recorder** | Optional **`IPTV_TUNERR_CATCHUP_GUIDE_POLICY`** (and flags) filter capsules/records the same way. | catch-up commands |

## Logos and `lineup.json`

HDHomeRun-shaped **`lineup.json`** exposes **GuideNumber**, **GuideName**, and **stream URL** only — no per-channel logo field. Plex usually takes **channel artwork from `guide.xml`** (`<icon src="..."/>`) when the merged XMLTV includes icons. There is no separate “logo sanitizer” in Tunerr today; broken provider icons are a guide-source concern.

## See also

- [cli-and-env-reference](cli-and-env-reference.md) — env index.
- [epg-linking-pipeline](epg-linking-pipeline.md) — matching and confidence.
- [plex-livetv-http-tuning](plex-livetv-http-tuning.md) — adjacent Plex ops notes.
