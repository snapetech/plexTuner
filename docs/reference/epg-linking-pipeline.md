---
id: ref-epg-linking-pipeline
type: reference
status: draft
tags: [reference, epg, xmltv, matching, channels, providers]
---

# EPG Linking Pipeline (Multi-Provider + Confidence Matching)

Concrete plan for improving EPG coverage across large IPTV lineups, including the
high-volume unlinked-channel tail.

This is designed for Plex Tuner’s channel/guide workflows:
- better XMLTV matches for Live TV
- safer rollout of low-confidence matches
- support for multiple IPTV providers and multiple EPG sources

## Goals

1. Aggregate multiple IPTV providers into one normalized channel catalog
2. Merge multiple EPG/XMLTV sources
3. Increase EPG linkage for the long tail of unlinked channels
4. Keep false-positive matches low
5. Preserve manual overrides and operator review decisions

## Current implementation status (Phase 1)

Implemented in-app (report-only, no runtime guide mutation):
- `plex-tuner epg-link-report`
  - parses XMLTV channel ids / display names
  - matches against catalog `live_channels`
  - deterministic tiers only:
    - `tvg-id` exact
    - alias override exact
    - normalized channel-name exact (unique only)
  - writes JSON report + unmatched queue exports

Not implemented yet:
- persistent match store / DB tables
- automatic application of medium-confidence matches
- fuzzy/schedule-fingerprint matching
- multi-EPG merge resolver

## Non-goals

- Perfectly link all channels in one pass
- Replace curated high-confidence category feeds
- Depend on one provider’s naming quality

## Data model (minimum)

### Canonical channel registry

`canonical_channels`
- `id`
- `canonical_name`
- `brand`
- `country_code`
- `language_codes`
- `region`
- `channel_family` (news, sports, kids, etc.)
- `aliases_json`
- `logo_url`
- `priority_weight`

### Provider channels

`provider_channels`
- `id`
- `provider_id`
- `provider_channel_id`
- `name_raw`
- `name_normalized`
- `group_title`
- `tvg_id`
- `tvg_name`
- `tvg_logo`
- `country_hint`
- `language_hint`
- `stream_url_hash`
- `status` (active/dead/blocked)

### EPG channels (XMLTV)

`epg_channels`
- `id`
- `epg_source_id`
- `xmltv_channel_id`
- `display_names_json`
- `name_normalized`
- `icon_url`
- `country_hint`
- `language_hint`

### Match table (provider -> EPG)

`channel_epg_matches`
- `provider_channel_id`
- `epg_channel_id`
- `score`
- `confidence` (`high|medium|low`)
- `match_method` (`tvg_id_exact`, `alias_exact`, `name_fuzzy`, `schedule_fingerprint`, ...)
- `is_manual_override`
- `approved_by`
- `approved_at`
- `last_verified_at`

## Multi-provider aggregation model

Treat channels as a canonical entity with one-or-more provider stream candidates.

Per canonical channel:
- `primary_stream` (preferred provider)
- `backup_streams[]`
- `epg_binding` (canonical EPG channel or per-region variants)

Selection policy (example):
1. explicit operator pin
2. high-confidence match + healthy stream
3. best quality (resolution/fps/bitrate) within preferred region
4. lowest recent failure rate

## Matching strategy (ordered, confidence-based)

### Tier 1: Deterministic exact matches (high confidence)

1. `tvg-id` exact match
- provider `tvg_id` == XMLTV channel id
- normalize case/whitespace and common suffixes before compare

2. Manual alias exact match
- provider normalized name maps to alias table entry for a canonical channel / EPG channel

3. Provider-supplied explicit XMLTV id
- if provider metadata contains a direct XMLTV channel key

### Tier 2: Strong normalized name matches (medium-high)

Use normalized channel names with country/language constraints.

Normalization rules:
- lowercase
- strip punctuation
- strip common noise tokens:
  - `hd`, `uhd`, `4k`, `fhd`, `sd`
  - region noise (`us`, `uk`, `ca`, provider suffixes)
  - quality tags
- normalize brand variants:
  - `fox news channel` -> `foxnews`
  - `nick jr` / `nickjr` -> `nickjr`

Scoring boosts:
- same country
- same language
- same brand group
- same category/family (`news`, `sports`, etc.)

### Tier 3: Fuzzy string + metadata constraints (medium)

Use fuzzy matching only within a constrained candidate set.

Constrain by:
- language
- country/region
- channel family
- provider group title

Features for score:
- token overlap
- normalized Levenshtein/Jaro-Winkler
- acronym similarity (`bbc one` / `bbcone`)

### Tier 4: Schedule fingerprinting (low-medium, expensive)

For channels still unmatched:
- compare a short time window of program titles/descriptions against candidate EPG channels
- score overlap patterns over 3–12 hours

Use only when:
- channel has stable schedule
- a candidate set can be region/language constrained

This is powerful but expensive and error-prone for:
- news channels
- sports channels
- sparse/incorrect EPG feeds

## Confidence policy

### High confidence
- auto-apply to Plex Tuner guide linking
- include in normal lineup

Examples:
- `tvg_id_exact`
- manual alias exact
- exact normalized match with country/language agreement and no conflicts

### Medium confidence
- apply only if no conflicting candidate above threshold
- mark for review
- include optionally behind a flag

### Low confidence
- do not auto-apply
- keep in review queue

## Manual override system (required)

This is the key to making long-tail coverage improve over time.

Store:
- provider-channel -> epg-channel overrides
- alias additions
- deny rules (never match X to Y)

Operator actions:
- approve proposed match
- reject match
- promote alias
- mark channel as `no-epg` / `radio` / `junk`

## Review queue (high leverage)

Sort by:
1. viewership/popularity
2. frequency in scans
3. confidence (medium first)
4. unresolved duplicates/conflicts

This gives the biggest real-world gain quickly without trying to “solve all 42k”.

## XMLTV source merging

Support multiple EPG sources and merge into a single candidate set.

Merge order:
1. local/curated source
2. regional source
3. broad/global source

Deduping keys:
- xmltv channel id
- normalized display name + country/language

Store source provenance:
- which XMLTV source supplied the winning channel/program data

## Channel classes to exclude early

Don’t waste matching cycles on channels unlikely to benefit:
- radio/audio-only
- duplicate mirrors with identical URLs
- dead streams
- test channels
- PPV placeholders
- adult (if excluded by product policy)

## Rollout strategy (safe)

### Phase 1
- deterministic + alias matching only
- manual overrides DB
- metrics/reporting

### Phase 2
- constrained fuzzy matching
- review queue
- approval workflow

### Phase 3
- schedule fingerprinting for high-value unmatched channels
- multi-provider canonical stream selection/failover

## Metrics to track

- total provider channels
- linked channels (count and %)
- high/medium/low confidence counts
- manual override count
- false positive corrections
- top-viewed unlinked channels
- per-provider link coverage

## Integration points in Plex Tuner (future work)

Potential insertion points:
- indexer/catalog build stage (normalize + candidate generation)
- guide builder/remapper stage (final channel->EPG bindings)
- external review/export tool (CSV/JSON)

Recommended outputs:
- `aliases.yaml` (human-editable)
- `matches.json` / DB table (machine state)
- `coverage-report.json`

## Practical expectation

You will not get all ~42k channels linked cleanly.

You can, however, get a large and meaningful improvement by:
- merging EPG sources
- normalizing names properly
- adding aliases/overrides
- prioritizing the channels users actually use
- keeping low-confidence guesses out of production lineups

See also
--------
- [Plex DVR lifecycle and API operations](plex-dvr-lifecycle-and-api.md)
- [catchup-category-taxonomy.example.yaml](catchup-category-taxonomy.example.yaml)
