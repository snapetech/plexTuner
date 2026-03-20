---
id: adr-0002-hdhr-hardware-iptv-merge
type: reference
status: accepted
tags: [adr, hdhomerun, iptv, catalog]
---

# ADR 0002: Physical HDHomeRun vs IPTV catalog merge semantics

## Context

IPTV Tunerr’s primary catalog is built from **M3U / Xtream `player_api`** streams. Some deployments also have **SiliconDust HDHomeRun** hardware on the LAN with its own `lineup.json` and MPEG-TS stream URLs.

We are adding **client-side** discovery and HTTP fetch (`hdhr-scan`, `internal/hdhomerun` client) without silently mixing sources.

## Decision

1. **Tag sources, don’t merge blindly**  
   Hardware channels and IPTV channels must remain distinguishable in any future merged catalog. Prefer explicit **source tags** (e.g. `hdhr` vs `iptv`) and stable **namespacing** for IDs so Plex/Emby remaps and `tvg-id` repair do not collide.

2. **Default = separate instances**  
   Until a merge design is implemented and tested, operators should run **separate `iptv-tunerr` instances** (or separate supervisor children): one for IPTV, one pointed at HDHR-derived input when that path exists. This matches how many users already split “OTA” vs “IPTV” tuners in Plex.

3. **Future merged catalog (opt-in)**  
   A single merged catalog may be introduced behind an explicit flag or config section, with:
   - deterministic precedence rules (e.g. IPTV vs HDHR when both claim the same guide number),
   - duplicate detection on `GuideNumber` + source,
   - documentation and migration notes.

4. **EPG**  
   HDHR EPG from Silicondust APIs is **not** assumed to be interchangeable with provider XMLTV; merge rules for guide windows will be a **separate ADR** once ingest exists.

## Consequences

- `hdhr-scan` and HTTP helpers are **observability / import spiking** tools, not automatic catalog mutation.
- Future work (`LP-002`+) implements optional import or sidecar merge following this ADR.

## Status

Accepted (2026-03-19).

See also
--------
- [EPIC-lineup-parity.md](../epics/EPIC-lineup-parity.md)
- [internal/hdhomerun/client.go](../../internal/hdhomerun/client.go)
