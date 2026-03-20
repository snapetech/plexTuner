---
id: howto-connect-plex
type: how-to
status: current
tags: [how-to, plex, livetv, dvr, registration]
---

# Connect IPTV Tunerr to Plex

IPTV Tunerr speaks **HDHomeRun-style** HTTP (`/discover.json`, `/lineup.json`, `/guide.xml`, `/stream/…`). Plex discovers it like a network tuner. The confusing part is **which Plex path** you use and what still has to happen **after** the DVR row exists.

See also: [plex-dvr-lifecycle-and-api](../reference/plex-dvr-lifecycle-and-api.md) (API detail), [deploy-and-connect-plex-home](deploy-and-connect-plex-home.md) (in-cluster home example), [troubleshooting runbook](../runbooks/iptvtunerr-troubleshooting.md).

## Choose a path

| Path | Best when | You handle |
|------|-----------|------------|
| **A. Plex UI wizard** | One household Plex, normal setup | Click through Live TV & DVR; map channels to guide |
| **B. `iptv-tunerr run -register-plex`** | You want Tunerr to call Plex’s registration API after startup | Same Plex requirements as API path; see flags in [CLI reference](../reference/cli-and-env-reference.md) |
| **C. Scripted / API-only** | Many DVRs, automation, category fleets | Programmatic DVR create + EPG + channelmap (see lifecycle doc) |

All paths need: **Tunerr reachable from Plex**, **catalog + guide** in good enough shape for Plex to match channels to EPG.

## Preconditions (all paths)

1. Tunerr is listening (e.g. `iptv-tunerr run` or `serve`) with a stable **`-base-url`** Plex will use (the URL Plex puts in the DVR).
2. **`GET /discover.json`** returns sane `DeviceID` / `FriendlyName` (Plex keys off device identity).
3. **`GET /lineup.json`** lists the channels you expect; **`GET /guide.xml`** has programme data for linked channels (`tvg-id` / repairs as per your hygiene settings).
4. Firewall: Plex Server can reach Tunerr’s HTTP port.

Quick checks from the Plex host:

```bash
curl -fsS "$TUNERR/discover.json" | head
curl -fsS "$TUNERR/guide.xml" | head
```

Use your real base URL for `$TUNERR` (same idea as [interpreting-probe-results](interpreting-probe-results.md) for provider health).

## Path A — Plex UI wizard (recommended for simple setups)

1. In Plex: **Settings → Live TV & DVR → Set up Plex DVR** (wording varies by version).
2. Let Plex **discover** network devices; pick the tuner that matches Tunerr’s **FriendlyName** / IP.
3. Complete the flow: lineup, guide source, **channel map** (map lineup rows to guide channels).
4. **Activate** the lineup/channel map if Plex leaves the DVR in a “pending” state — without activation, the guide can stay empty even though `/guide.xml` is fine.

**480-channel limit:** Plex’s wizard is built around a bounded channel count. If your catalog is larger, use Tunerr’s lineup hygiene / recipes ([lineup-epg-hygiene](../reference/lineup-epg-hygiene.md), `IPTV_TUNERR_GUIDE_POLICY`, registration recipes) *before* wizard, or use path B/C for programmatic fleets.

## Path B — `-register-plex` on `run`

`iptv-tunerr run … -register-plex` asks Tunerr to register with Plex using configured Plex URL/token (see [CLI reference](../reference/cli-and-env-reference.md)). It is a **convenience** for the injected DVR style, not a substitute for:

- Plex having a valid **token** and reaching Tunerr
- **Channelmap / EPG association** if your Plex version still needs a follow-up in the UI or API

If registration succeeds but the guide is empty, see **Troubleshooting** below and the lifecycle doc’s “guide refresh / channelmap” section.

## Path C — API / headless

Use the same backend steps the wizard uses, without the UI: create/select device identity, create DVR, attach guide, save channelmap. Details and caveats (duplicate `DeviceID`, provider rows, tab labels) are in [plex-dvr-lifecycle-and-api](../reference/plex-dvr-lifecycle-and-api.md).

## Troubleshooting (short)

| Symptom | Common cause |
|---------|----------------|
| DVR exists, **guide empty** | Channel map not activated; `tvg-id` mismatch; wrong provider row — see lifecycle doc § guide refresh |
| **Duplicate** tuners / wrong card | Reused `DeviceID` — use distinct `IPTV_TUNERR_DEVICE_ID` per logical tuner |
| Streams fail only in Plex Web | Client adaptation / WebSafe — [plex-livetv-http-tuning](../reference/plex-livetv-http-tuning.md), [compatibility matrix](../reference/plex-client-compatibility-matrix.md) |
| Unsure provider is healthy | `iptv-tunerr probe` — [interpreting-probe-results](interpreting-probe-results.md) |

## See also

- [features](../features.md) — Plex-facing capabilities
- [runbooks](../runbooks/index.md) — operational drill-downs
