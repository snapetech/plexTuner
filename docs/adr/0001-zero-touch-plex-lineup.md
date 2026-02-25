# ADR 0001: Zero-touch Plex setup — programmatic lineup injection

## Status

Accepted.

## Context

- Plex’s **Live TV & DVR** “Set up” wizard fetches our `lineup.json` and `guide.xml`, then **saves** the channel lineup. That save path enforces a **~480-channel limit**; above that, Plex shows “failed to save channel lineup” and the user gets nothing.
- Our tooling can index and serve **thousands** of channels (e.g. ~6000). Forcing users to add the tuner via the wizard caps them at 480 and requires manual steps.
- **Goal:** Zero human interaction. No wizard. We register the tuner and **inject the full lineup (and optionally guide) into Plex’s database** so that when Plex starts, it already has our channels and never runs the wizard path. That bypasses the 480 limit and gives users the full catalog our tooling provides.

## Decision

1. **We do full programmatic setup:** Register tuner URIs (existing `RegisterTuner`) **and** sync the channel lineup (and where possible EPG) into Plex’s SQLite DB (“splice ours in”) so Plex does not need to fetch-and-save via the wizard.
2. **When `-register-plex` is used with lineup sync:** We write DVR + XMLTV URIs and inject our channel list (guide number, name, stream URL) into the tables Plex uses for Live TV channels. No cap at 480 for this path; we sync whatever the catalog has.
3. **Wizard path remains available** for users who can’t use DB access (e.g. Plex in the cloud, no filesystem access). In that path we still cap at 480 so “Add tuner” succeeds.
4. **Schema discovery:** Plex’s `com.plexapp.plugins.library.db` schema is version-dependent and not fully public. We implement lineup sync by probing for known table names and/or documenting how to discover the correct table (e.g. `sqlite3 path/to/library.db .schema`) and adding support for that schema.

## Consequences

- **Zero-touch:** One run with `-register-plex=/path/to/Plex` (and optional lineup sync) configures Plex with no UI steps; users get full channel count (e.g. ~6000) when we write the lineup into the DB.
- **Implementation:** New `SyncLineupToPlex` (or equivalent) in `internal/plex` that writes channel rows; called after `RegisterTuner` when sync is enabled. Lineup cap is skipped when we perform full sync.
- **Docs:** Document that “no wizard” and “full channel count” require `-register-plex` with lineup sync and Plex stopped; wizard path remains 480-capped.

## References

- [internal/plex/dvr.go](../../internal/plex/dvr.go) — existing RegisterTuner
- [docs/features.md](../features.md) — “Plex API DVR creation” not supported; we use DB-based approach
- [memory-bank/known_issues.md](../../memory-bank/known_issues.md) — 480-channel limit
