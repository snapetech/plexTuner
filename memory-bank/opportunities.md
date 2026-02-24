# Opportunities (Continuous Improvement Backlog)

This is a lightweight backlog for improvements discovered during other work.
It exists to encourage quality gains without derailing the current task.

## Rules
- Prefer evidence: link to code, test output, perf numbers, or a specific risk.
- Do NOT expand scope mid-task unless it is small, low-risk, and clearly aligned.
- If an item needs a product/UX decision or significant effort, raise it to the user.

## Entry template
- Date: YYYY-MM-DD
  Category: security | performance | reliability | maintainability | operability | other
  Title: <short>
  Context: <where you noticed it>
  Why it matters: <impact + who it affects>
  Evidence: <link/snippet/metric>
  Suggested fix: <concrete next step>
  Risk/Scope: <low/med/high> | <fits current scope? yes/no>
  User decision needed?: yes/no
  If yes: 2–3 options + recommended default + what you will do if no answer

## Entries

- Date: 2025-02-23
  Category: maintainability
  Title: Add or document internal/indexer dependency
  Context: README/docs pass; build fails without indexer.
  Why it matters: New clones cannot build; unclear whether indexer is external or missing.
  Evidence: `go build ./cmd/plex-tuner` → "no required module provides package .../internal/indexer".
  Suggested fix: Either add the indexer package to the repo (from another branch/repo) or document the dependency and build steps in README/reference.
  Risk/Scope: low | fits current scope: no (documented in docs-gaps).
  User decision needed?: yes (whether indexer lives in-repo or separate).

- Date: 2026-02-24
  Category: performance
  Title: Cache remapped external XMLTV for `/guide.xml` (and fast-fallback on timeout)
  Context: Live Plex integration testing against `plextuner-websafe` in k3s (`plex.home`).
  Why it matters: `guide.xml` is fetched by Plex metadata flows; external XMLTV remap currently runs per request and took ~45s, which stalls Plex DVR channel metadata APIs.
  Evidence: `internal/tuner/xmltv.go` fetches external XMLTV every request (no cache); live measurement from Plex pod: `guide.xml` ~45.15s with external XMLTV enabled, ~0.19s with placeholder guide (XMLTV disabled).
  Suggested fix: Add in-memory/on-disk XMLTV cache with TTL + stale-while-revalidate; on timeout/error serve last good cached remap immediately, otherwise placeholder as fallback.
  Risk/Scope: med | fits current scope: no (code + behavior design)
  User decision needed?: yes (cache TTL/size and whether stale guide is preferred over placeholder on source failures).

- Date: 2026-02-24
  Category: operability
  Title: Add guidance and tooling for Plex-safe lineup sizing (WebSafe had 41k+ channels)
  Context: Live Plex API testing on DVR `138` (`plextunerWebsafe`) in k3s.
  Why it matters: Plex could tune a known channel, but channel metadata enumeration (`.../lineups/dvr/channels`) was too slow with ~41,116 channels, making mapping/diagnostics painful.
  Evidence: `lineup.json` ~5.3 MB / ~41,116 channels; Plex `tune` for channel `11141` succeeded, but channel list API did not return within 15s during tests.
  Suggested fix: Document and/or add tooling for pre-serve channel pruning (EPG-linked only, category includes/excludes, max-channel cap) and provide recommended profiles for Plex.
  Risk/Scope: med | fits current scope: no (behavior/config product choices)
  User decision needed?: yes (preferred pruning strategy for your Plex setup; default recommendation: EPG-linked + curated categories).

- Date: 2026-02-24
  Category: operability
  Title: Instrument source->EPG coverage in the 13-category split pipeline
  Context: Live Threadfin/Plex multi-DVR validation in k3s after rerunning the IPTV split + Threadfin refresh jobs.
  Why it matters: The pipeline can "work" while producing unexpectedly tiny outputs; without counts at each stage it looks like Threadfin/Plex is broken when the real constraint is feed/XMLTV linkage.
  Evidence: Observed ~41,116 source channels -> 188 EPG-linked (`tvg-id` found in XMLTV) -> 91 deduped -> 91 total across 13 `dvr-*.m3u`; many Threadfin buckets and Plex DVRs were empty by design.
  Suggested fix: Log and persist stage counts (`all`, `with_tvg_id`, `in_xmltv`, `deduped`, per-bucket totals) in the split/update jobs and optionally warn/fail if totals drop below a configurable threshold.
  Risk/Scope: low | fits current scope: no (k3s/job tooling change, not PlexTuner code)
  User decision needed?: no

- Date: 2026-02-24
  Category: reliability
  Title: Make `plex-activate-dvr-lineups.py` skip empty DVRs instead of crashing
  Context: Activating newly created Threadfin DVRs in Plex after 13-way split refresh.
  Why it matters: Empty category buckets are expected when source/EPG coverage is sparse, but the activation helper aborts on the first empty DVR and prevents activation of later non-empty DVRs.
  Evidence: `ValueError: No valid ChannelMapping entries found` on DVR `141` (`threadfin-newsus`, 0 channels); rerunning the script only for non-empty DVRs succeeded and mapped all 91 channels.
  Suggested fix: Catch the empty-mapping case in `plex/scripts/plex-activate-dvr-lineups.py`, log `SKIP_EMPTY`, and continue processing remaining DVRs.
  Risk/Scope: low | fits current scope: no (external k3s repo script)
  User decision needed?: no
