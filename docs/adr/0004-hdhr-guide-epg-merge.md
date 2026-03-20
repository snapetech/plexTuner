---
id: adr-0004-hdhr-guide-epg-merge
type: reference
status: accepted
tags: [adr, hdhomerun, epg, xmltv]
---

# ADR 0004: HDHomeRun device `guide.xml` in the merged EPG pipeline

## Context

[ADR 0002](0002-hdhr-hardware-iptv-merge.md) defers **catalog** merge design. Operators still need a clear story for **EPG**: a physical SiliconDust tuner serves `guide.xml` over HTTP with programme data keyed by the device’s channel IDs.

IPTV Tunerr already builds `/guide.xml` from **provider** `xmltv.php`, **external** XMLTV (`IPTV_TUNERR_XMLTV_URL`), and **placeholders**.

## Decision

1. **Opt-in URL**  
   Optional env **`IPTV_TUNERR_HDHR_GUIDE_URL`** points at a full http(s) URL of a device `guide.xml` (typically `http://<hdhr-ip>/guide.xml`). Empty = no hardware EPG fetch.

2. **Precedence**  
   Per channel `tvg-id` (upstream XMLTV channel id), merge order is:
   - **Provider** programmes first (when provider EPG is enabled),
   - **External** gap-fill (same rules as today),
   - **HDHR** programmes that **do not overlap in time** with the union of provider+external windows for that `tvg-id`.

   When there is **no** provider or external data for a `tvg-id`, **HDHR-only** programmes are used if present; otherwise the usual wide placeholder applies.

3. **Identity**  
   Matching is **only** by `tvg-id` string equality between the Tunerr catalog and the hardware `guide.xml` `<programme channel="…">` attribute. Operators must align IDs (e.g. by importing HDHR lineup into the catalog or using consistent `tvg-id` repair). No fuzzy cross-source matching in this layer.

4. **Failure behavior**  
   If the HDHR URL fetch fails, the merge continues without hardware EPG (same pattern as external XMLTV fetch failure).

## Consequences

- Hybrid IPTV+OTA setups can layer OTA EPG for channels whose `tvg-id` matches the device guide, without replacing provider IPTV data where it exists.
- `hdhr-scan -guide-xml` remains a **diagnostic** tool; runtime merge is controlled by **`IPTV_TUNERR_HDHR_GUIDE_URL`** on `serve` / `run`.

See also
--------
- [EPIC-lineup-parity.md](../epics/EPIC-lineup-parity.md) — LP-003
- [cli-and-env reference](../reference/cli-and-env-reference.md)
