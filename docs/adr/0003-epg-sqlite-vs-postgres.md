---
id: adr-0003
type: adr
status: accepted
tags: [epg, sqlite, postgres, persistence]
---

# ADR 0003: EPG persistence — SQLite first; Postgres only if multi-writer / shared state is required

## Context

The [Lineup parity epic](../epics/EPIC-lineup-parity.md) adds a **durable EPG store** (incremental fetch, retention, cleanup) alongside today’s **in-memory merged XMLTV cache** in `internal/tuner/xmltv.go`.

Operators sometimes ask whether **PostgreSQL** is a better default than **SQLite** for “real” deployments.

The repo already ships **`modernc.org/sqlite`** (e.g. read-only browser cookie DBs in `internal/tuner/cookie_browser.go`), so an embedded SQLite file adds **no new driver dependency** for the EPG path.

## Decision

1. **Use SQLite** for the Tunerr **EPG programme store** when `IPTV_TUNERR_EPG_SQLITE_PATH` is set: single file, WAL mode, one primary writer (the tuner process), range-friendly indexes for “max airtime per channel” queries (LP-008).

2. **Do not introduce PostgreSQL** for this layer **unless** a concrete requirement appears for:
   - multiple Tunerr instances **writing the same guide** concurrently, or
   - centralized EPG shared across hosts **without** a shared filesystem, or
   - org-mandated backup/HA patterns that exclude file-backed stores.

   Those are **different products** (networked DB ops, connection strings, secrets, migrations in CI for Postgres). They belong behind an explicit epic/ADR, not as the default for a single-binary bridge.

3. **Do not unify all app state into SQLite** in one sweep. Today the project intentionally uses **purpose-specific persistence** (e.g. JSON state files for recorder/autopilot, catalog JSON, optional SQLite for EPG). Moving **every** subsystem into one DB is a large coupling and migration cost; the EPG store should remain **bounded** to guide/programme retention until a future decision revisits a “single store” architecture.

## Consequences

- **Pros:** Simple deployment (file path + permissions), matches Lineup-style local cache, aligns with existing `modernc.org/sqlite` usage, no extra network dependency for guide.
- **Cons:** Multi-instance HA is “one file on shared storage” or “one writer per file”, not Postgres-style active/active without careful design.

## See also

- [EPIC-lineup-parity.md](../epics/EPIC-lineup-parity.md) — `LP-007`–`LP-009`
- [0002-hdhr-hardware-iptv-merge.md](./0002-hdhr-hardware-iptv-merge.md) — catalog merge semantics (orthogonal to EPG engine choice)
