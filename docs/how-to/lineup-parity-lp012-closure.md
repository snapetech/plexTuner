---
id: lineup-parity-lp012-closure
type: how-to
status: stable
tags: [lp-012, lineup-parity, operator]
---

# LP-012 — lineup parity operator checklist (closure)

[EPIC-lineup-parity](../epics/EPIC-lineup-parity.md) **LP-012** is the ongoing “docs + runbook + env sweep” story. Use this checklist after substantive changes (profiles, HDHR merge, mux, readiness).

## Hybrid HDHR + IPTV

- [ ] [hybrid-hdhr-iptv](hybrid-hdhr-iptv.md) — lineup URL at **index**, guide URL at **serve**, SQLite optional
- [ ] **`IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS`** if **`hdhr-scan`** sees nothing via global broadcast ([cli-and-env](../reference/cli-and-env-reference.md))
- [ ] ADR links still valid: [0002](../adr/0002-hdhr-hardware-iptv-merge.md), [0004](../adr/0004-hdhr-guide-epg-merge.md)

## Profiles and native mux

- [ ] [transcode-profiles](../reference/transcode-profiles.md) — **`IPTV_TUNERR_STREAM_PROFILES_FILE`**, **`?mux=`**, **`X-IptvTunerr-Native-Mux`**
- [ ] [hls-mux-toolkit](../reference/hls-mux-toolkit.md) — operator diagnostics and **`curl`** recipes

## Ops readiness

- [ ] **`/readyz`** / **`/healthz`** in runbooks and k8s examples ([troubleshooting runbook](../runbooks/iptvtunerr-troubleshooting.md) §8, [k8s/README](../../k8s/README.md))

## Live TV intelligence (LTV)

- [ ] [EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md) — deck URLs in **`/debug/runtime.json`**
- [ ] **`/provider/profile.json`** includes **`intelligence.autopilot`** when Autopilot state file is configured

## Verify

- [ ] `./scripts/verify`
- [ ] Spot-check **`GET /discover.json`**, **`/lineup.json`**, **`/guide.xml`**, **`/provider/profile.json`**

See also
--------
- [Docs index](../index.md)
- [features](../features.md)
