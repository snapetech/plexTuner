---
id: epic-programming-manager
type: explanation
status: draft
tags: [epic, lineup, webui, programming-manager]
---

# EPIC-programming-manager

Build a real "Programming Manager" surface for curating the live channel lineup:
category-first bulk selection, per-channel include/exclude, deliberate custom
ordering, and exact-EPG-match backup grouping.

## Why this exists

The current product can ingest, dedupe, score, and expose channels, but it does
not yet give operators a first-class curation workflow. The tester request is
not just "another settings page"; it is a product surface for deciding:

- which provider categories belong in the lineup
- which exact channels inside those categories stay or go
- what order those channels should appear in
- which exact-EPG sibling channels should be retained as backup sources behind
  one visible lineup row

That should become a server-backed, operator-friendly control surface, not a
collection of ad hoc env vars and one-off recipes.

## External patterns worth copying

These are the useful patterns, not literal UI clones.

1. Channels DVR collections: manual curation plus explicit sort modes and custom
   drag ordering.
   Source: <https://getchannels.com/docs/channels-dvr-server/how-to/library-collections/>
2. xTeVe/XEPG style channel mapping: one canonical visible channel row with
   stream + EPG mapping control and explicit channel order control.
   Source: <https://github.com/xteve-project/xTeVe>

What those tools get right:

- bulk-add from a grouped source, then refine
- one visible lineup row can still hide richer mapping underneath
- sorting is deliberate, not just "whatever the source emitted"
- server-side saved order matters more than client-side temporary filtering

## Product stance

- Category selection should be the fast path.
- Per-channel inclusion/exclusion should be the refinement path.
- Custom order should be saved server-side and replayable across refreshes.
- Backup sources should be grouped automatically when there is a trustworthy
  exact EPG/channel identity match, but the operator should be able to inspect
  and override that grouping.

## UX shape

The Programming Manager should have four lanes:

1. **Sources**
   - provider category groups (`Sling`, `DirecTV`, `NBC`, `ABC`, `FOX`, etc.)
   - counts, confidence, and whether a group is already included
2. **Selection**
   - bulk include/exclude category
   - drill into category and toggle specific channels
3. **Order**
   - preset taxonomy order
   - manual drag-and-drop custom order
   - preview of final `lineup.json` order
4. **Backups**
   - show channels grouped by exact EPG/channel identity
   - one primary visible row
   - zero or more backup sources behind it

## Default taxonomy order

When the operator chooses "recommended order", the server should sort by this
bucket sequence first, then by saved manual rank if present, then by guide
number/name:

1. Local Broadcast
2. General Entertainment
3. News & Info
4. Sports
5. Lifestyle & Home
6. Documentary & History
7. Children & Family
8. Reality & Specialized
9. Premium Networks
10. Regional Sports
11. Religious
12. International

This needs a stable, testable classifier rather than a hand-wavy UI-only label.

## Story list

| ID | Goal | Acceptance criteria |
|----|------|---------------------|
| PM-001 | Source category inventory | Server can build a category inventory from provider/XMLTV/group-title inputs with counts and stable IDs. |
| PM-002 | Saved lineup recipe model | Persist selected categories, per-channel include/exclude, and saved ordering in a durable JSON or SQLite-backed model. |
| PM-003 | Category-first selection API | Add API endpoints to bulk include/exclude categories and inspect category members. |
| PM-004 | Per-channel selection API | Add API endpoints to include/exclude exact channels and preview the resulting curated lineup. |
| PM-005 | Recommended taxonomy ordering | Server can classify channels into the requested bucket order and produce a deterministic recommended sort. |
| PM-006 | Manual order persistence | Operators can save a custom order; refresh/index runs preserve it unless explicitly rebuilt. |
| PM-007 | Backup grouping by exact match | Exact EPG/channel matches across markets/providers can be retained as backup stream sources behind one visible row. |
| PM-008 | Control-deck UI | Add a Programming Manager lane to the web UI with category picker, detail drawer, and order view. |
| PM-009 | Release-grade testing | Unit tests, API tests, and smoke coverage prove category selection, saved order, and backup grouping survive refreshes. |

## Status

- **2026-03-21:** `PM-001` and `PM-002` foundation slice shipped.
  - `internal/programming` now builds stable category inventory from the raw post-intelligence lineup and persists a durable JSON recipe file.
  - `Server.UpdateChannels` now preserves `raw catalog -> intelligence/dedupe` input separately from the final exposed lineup, then applies the saved programming recipe before existing lineup-shape/cap logic.
  - First backend endpoints are live:
    - `/programming/categories.json`
    - `/programming/channels.json`
    - `/programming/recipe.json`
    - `/programming/preview.json`
  - This is intentionally backend-first. The control-deck UI lane (`PM-008`) and explicit mutation conveniences (`PM-003` / `PM-004`) still follow.
- **2026-03-21:** `PM-003`, `PM-004`, and the first visible `PM-005` slice shipped.
  - `/programming/categories.json` now supports operator-guarded bulk include/exclude/remove mutations for category selection.
  - `/programming/channels.json` now supports operator-guarded include/exclude/remove mutations for exact channel overrides.
  - `order_mode: "recommended"` now classifies channels into the requested taxonomy buckets and sorts deterministically by bucket -> saved manual rank -> guide number/name.
  - `/programming/preview.json` now reports bucket counts so the output shape is inspectable without scraping the lineup by hand.
- **2026-03-21:** `PM-006` and `PM-007` backend slice shipped.
  - `/programming/order.json` now supports durable server-side manual order mutations (`prepend`, `append`, `before`, `after`, `remove`) and automatically flips the recipe into `order_mode: "custom"` when operators start pinning rows deliberately.
  - `/programming/backups.json` now reports exact-match sibling groups using strong identity only (`tvg_id` exact, else `dna_id` exact).
  - `collapse_exact_backups: true` on the saved recipe now collapses those exact sibling rows into one visible lineup channel with merged `stream_urls`, so “Sling SyFy” and “DirecTV SyFy” can become one visible row with backup sources behind it.
- **2026-03-21:** `PM-008` deck lane shipped.
  - The dedicated `internal/webui` control deck now has a real Programming lane instead of leaving the feature as backend JSON only.
  - Operators can bulk include/exclude categories, pin or block exact channels, nudge manual order from the curated preview (`prepend` / relative up/down / drop order), toggle `collapse_exact_backups`, and inspect exact backup groups.
  - The lane also exposes raw Programming payload drill-down so recipe, preview, categories, channels, order, and backups stay debuggable from the same control plane.
- **2026-03-21:** first visible `PM-009` release-grade coverage shipped.
  - Added tuner regression coverage proving saved programming recipe mutations survive `UpdateChannels` refresh churn instead of only passing in a static one-shot API flow.
  - Expanded the binary smoke lane so it restarts `iptv-tunerr serve` against a reshuffled catalog while reusing the same programming recipe file, then reasserts curated lineup shape, persisted custom order, and `collapse_exact_backups` behavior after the restart.
  - Remaining `PM-009` depth is richer operator/browser automation, not missing persistence coverage.

## Technical approach

### Data model

Add a new durable curation layer, separate from raw ingest:

- `programming_categories`
- `programming_recipe`
- `programming_overrides`
- `programming_order`
- `programming_backup_groups`

The raw catalog stays source truth for ingest. The programming layer becomes the
operator truth for final exposed lineup shape.

### Match strategy for backup sources

Only auto-group channels when the identity signal is strong:

- same exact `tvg_id`, or
- same `dna_id`, or
- same exact normalized guide identity with an explicit confidence threshold

Do not group fuzzy near-matches automatically.

### Runtime contract

The final exposed lineup should become:

`raw catalog -> intelligence/dedupe -> programming recipe -> final lineup`

That keeps curation deterministic and replayable after refreshes.

## Verification plan

- unit tests for category classification and taxonomy ordering
- API tests for category and channel selection mutations
- persistence tests for saved order surviving catalog refresh
- backup-group tests for exact-match grouping and operator override
- binary smoke for a minimal saved recipe being applied to `lineup.json`

## Out of scope

- replacing Plex's own channel UI
- free-form fuzzy matching for every possible market alias in one pass
- one-shot import of arbitrary third-party lineup-editor configs
- building a full public-facing admin app

## See also

- [EPIC-live-tv-intelligence](EPIC-live-tv-intelligence.md)
- [EPIC-lineup-parity](EPIC-lineup-parity.md)
- [project-backlog](../explanations/project-backlog.md)
