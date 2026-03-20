---
id: adr-index
type: reference
status: stable
tags: [adr, decisions, index]
---

# Architecture decision records (ADR)

Decision log. One file per decision; number by sequence.

| Doc | Description |
|-----|-------------|
| [0001-zero-touch-plex-lineup](0001-zero-touch-plex-lineup.md) | Zero-touch Plex setup: programmatic lineup injection so no wizard, no 480 cap; full channel count when using `-register-plex`. |
| [0002-hdhr-hardware-iptv-merge](0002-hdhr-hardware-iptv-merge.md) | Physical HDHomeRun vs IPTV catalog: tag sources, prefer separate instances until explicit merged-catalog design. |
| [0003-epg-sqlite-vs-postgres](0003-epg-sqlite-vs-postgres.md) | EPG persistence: SQLite file for Tunerr-local store; Postgres only if shared/multi-writer requirements are explicit. |
| [0004-hdhr-guide-epg-merge](0004-hdhr-guide-epg-merge.md) | Optional hardware `guide.xml` URL merged after provider + external gap-fill; overlap rules and `tvg-id` identity. |

See also
--------
- [Explanations: architecture](../explanations/architecture.md).
- [Linking rules](_meta/linking.md).
