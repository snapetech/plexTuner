---
id: hybrid-hdhr-iptv
type: how-to
status: stable
tags: [hdhomerun, iptv, epg, catalog]
---

# Hybrid HDHomeRun + IPTV with IPTV Tunerr

Use this when you run **both** a physical SiliconDust tuner (OTA) and **IPTV/Xtream** in one Tunerr instance. Catalog merge and EPG layering follow [ADR 0002](../adr/0002-hdhr-hardware-iptv-merge.md) and [ADR 0004](../adr/0004-hdhr-guide-epg-merge.md).

## 1. Discover the device

```sh
iptv-tunerr hdhr-scan
# or HTTP-only:
iptv-tunerr hdhr-scan -addr http://192.168.1.50 -lineup
```

Note the device **base URL** (e.g. `http://192.168.1.50`).

## 2. Merge hardware channels into the catalog (index)

Point **`IPTV_TUNERR_HDHR_LINEUP_URL`** at the device’s `lineup.json` when you run **`iptv-tunerr index`** (or your scheduled index):

```bash
export IPTV_TUNERR_HDHR_LINEUP_URL=http://192.168.1.50/lineup.json
export IPTV_TUNERR_HDHR_LINEUP_ID_PREFIX=hdhr   # optional; default hdhr
iptv-tunerr index
```

Hardware channels get `channel_id` like `hdhr:10`, **`tvg_id` = guide number** (string), `group_title` **HDHomeRun**, and `source_tag` matching the configured HDHR prefix (default `hdhr`). Exact `channel_id` duplicates are skipped, but `tvg_id` collisions are now kept as separate source-tagged rows so OTA and IPTV do not silently collapse into one channel.

## 3. Merge device EPG into `/guide.xml` (serve / run)

Set **`IPTV_TUNERR_HDHR_GUIDE_URL`** to the device **guide.xml** full URL. Tunerr merges it **after** provider + external XMLTV; programmes match on **`tvg-id`** equal to the device’s programme `channel` attribute (same as guide number for typical OTA lineups).

```bash
export IPTV_TUNERR_HDHR_GUIDE_URL=http://192.168.1.50/guide.xml
```

## 4. SQLite EPG file (optional)

See [ADR 0003](../adr/0003-epg-sqlite-vs-postgres.md). Use **`IPTV_TUNERR_EPG_SQLITE_PATH`** plus optional **`IPTV_TUNERR_EPG_SQLITE_RETAIN_PAST_HOURS`**, **`IPTV_TUNERR_EPG_SQLITE_VACUUM`**, and size caps **`IPTV_TUNERR_EPG_SQLITE_MAX_BYTES`** or **`IPTV_TUNERR_EPG_SQLITE_MAX_MB`**. Inspect **`GET /guide/epg-store.json`**.

## 5. Fragmented MP4 stream (optional, experimental)

For HLS inputs processed with ffmpeg, **`?mux=fmp4`** requests **fragmented MP4** instead of MPEG-TS. **Requires transcoding** (remux-only requests fall back to TS). Plex HDHR clients expect TS by default—use for lab/testing unless your player accepts `video/mp4`.

## 6. Live TV intelligence (hybrid ops)

Hardware + IPTV merges increase moving parts. Use the **LTV** JSON surfaces on the same tuner port:

| Endpoint | Use |
|----------|-----|
| **`GET /guide/health.json`** | Placeholder-only vs real programme coverage |
| **`GET /channels/report.json`** | Per-channel scores and backup-stream depth |
| **`GET /provider/profile.json`** | Concurrency/CF/mux counters + **`intelligence.autopilot`** (hot learned paths) |
| **`GET /autopilot/report.json`** | Full Autopilot memory sample |
| **`GET /plex/ghost-report.json`** | Plex visible-session stalls (optional **`stop`**) |

Cross-reference: [EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md). Deck **`/debug/runtime.json`** → **`URLs`** lists the same paths for copy/paste.

## See also

- [cli-and-env reference](../reference/cli-and-env-reference.md)
- [EPIC lineup parity](../epics/EPIC-lineup-parity.md)
- [EPIC live TV intelligence](../epics/EPIC-live-tv-intelligence.md)
- [features](../features.md)
- [hls-mux-toolkit](../reference/hls-mux-toolkit.md) — Tunerr-native **`?mux=hls|dash`** proxy headers and caps
- [Troubleshooting runbook](../runbooks/iptvtunerr-troubleshooting.md) — **`/healthz`**, **`/readyz`**, harnesses, **HR-*** checklists
