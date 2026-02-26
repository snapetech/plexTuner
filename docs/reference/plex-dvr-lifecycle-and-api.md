---
id: ref-plex-dvr-lifecycle-api
type: reference
status: draft
tags: [reference, plex, dvr, livetv, api, wizard]
---

# Plex DVR lifecycle and API operations

Reference for Plex-side Live TV / DVR manipulation used by Plex Tuner testing and recovery workflows.

This doc covers:
- HDHR wizard-equivalent API flow (create DVR from tuner)
- Injected DVR flow (Plex DB / API registration path)
- DVR removal / cleanup
- guide refresh and channelmap activation
- common Plex metadata/cache gotchas that make UI behavior look wrong

This is intentionally focused on Plex behavior around Plex Tuner, not general Plex administration.

## Mental model (important)

Plex Live TV has multiple layers that are easy to conflate:

- **HDHR device identity** (what Plex discovers from `/discover.json`)
- **DVR row** (`/livetv/dvrs`)
- **provider rows** (`media_provider_resources`, including `tv.plex.providers.epg.xmltv:<id>`)
- **client UI source presentation** (tabs/source labels in Plex Web/TV)

Key consequences:
- A new DVR row does **not** always produce a new configured tuner card in UI if Plex considers it the same device identity.
- UI labels can be driven by Plex provider metadata (for example Plex server name) instead of tuner `FriendlyName`.
- Guide weirdness can be a provider-row mismatch even when tuner feeds are healthy.

## Supported workflows in this repo

### 1. HDHR wizard path (UI or wizard-equivalent API)

Use when you want Plex to treat Plex Tuner like a discovered HDHomeRun device and perform normal Live TV setup behavior.

Typical Plex Tuner endpoints:
- `GET /discover.json`
- `GET /lineup_status.json`
- `GET /lineup.json`
- `GET /guide.xml`

Notes:
- Plex UI wizard is device-centric.
- Provider matching/EPG suggestions are lineup-sensitive.
- The wizard protocol does not let the tuner pre-check only some channels in a larger list. If you want `479`, the tuner must serve `479`.

### 2. Injected DVR path (programmatic registration)

Use when you want to bypass the wizard and create/update DVRs directly (for example 13-category injected DVR setup).

In this repo that is done by:
- `plex-tuner` API registration path (`-register-plex`)
- helper scripts / k8s jobs (category activation, guide reloads, channelmap save)

Notes:
- This is how multi-category DVR fleets are created quickly.
- It is ideal for testing Plex Tuner behavior independent of Plex wizard/provider matching.

## HDHR wizard-equivalent API flow (what "wizard inject" really means)

Plex can be driven through the same backend endpoints the wizard uses, without clicking through the UI.

Practical flow:

1. **Create/select HDHR device identity**
- Device identity comes from tuner `discover.json` (`DeviceID`, `FriendlyName`, etc.)
- If you reuse the same `DeviceID`, Plex may treat it as the same configured tuner
- To create a second configured tuner, use a distinct `DeviceID` (for example `hdhrbcast2`)

2. **Create DVR**
- Plex backend creates a new DVR row tied to the device identity

3. **Set lineup/guide**
- Use tuner `lineup.json` + `guide.xml`
- Plex may still not surface it cleanly in UI if provider metadata is incomplete/inconsistent

4. **Reload guide**
- Trigger Plex guide refresh so it fetches the current `guide.xml`

5. **Activate channelmap**
- Replay/save the Plex channelmap step so mapped channels become active

Important:
- Creating a DVR row alone is not enough for user-visible success.
- Validate in the actual Plex UI screen the user uses (configured tuners / Live TV settings), not only `/livetv/dvrs`.

## Injected DVR lifecycle (category DVRs)

Recommended for category-based Plex Tuner testing:

1. Start per-category tuners (or supervisor children) with distinct:
- `PLEX_TUNER_DEVICE_ID`
- `PLEX_TUNER_FRIENDLY_NAME`
- `PLEX_TUNER_GUIDE_NUMBER_OFFSET` (prevents cross-DVR guide collisions in Plex clients)

2. Register/create DVR rows in Plex for each category tuner

3. Refresh guide for each DVR

4. Activate/save channelmaps for each DVR

5. Validate per-DVR provider channel counts and first channel IDs

Why guide offsets matter:
- Without offsets, multiple DVRs starting at channel `1` can collide in Plex provider/client caches, causing “same guide in different tabs” symptoms.

## Remove / cleanup operations

Use removal when cleaning stale test devices, helper tuners, or legacy direct DVRs that confuse the UI.

Safe targets to remove:
- old helper DVRs pointing to temporary ports/hosts
- legacy test DVRs no longer used
- orphan device/provider rows left behind after failed tests

Be careful:
- Plex UI can cache device/provider candidates
- deleting a DVR may not remove every orphan provider/device row
- verify both:
  - `/livetv/dvrs`
  - `/media/grabbers/devices`

After cleanup, restart or fully refresh Plex clients if they still show stale candidates.

## Guide refresh / EPG operations

### Reload guide

Use `reloadGuide` when:
- tuner `guide.xml` changed
- `guide.xml` URI was repaired
- XMLTV content changed (language normalization, remap, feed switch)

Validation:
- tuner logs should show Plex fetching `/guide.xml`
- Plex provider endpoints should reflect updated channel IDs / counts

### Channelmap activation (“save” replay)

Guide refresh alone is not enough if channel mappings are stale or missing.

Use channelmap activation after:
- creating a new DVR
- changing lineup IDs / guide-number offsets
- major feed reshaping

Symptom if skipped:
- DVR exists but channels are missing or playback clicks do nothing

## Common Plex-side gotchas (important)

### 1. Device-centric UI vs DVR-centric backend

You can create a new DVR row and still not see a new configured tuner card if Plex thinks it is the same HDHR device identity.

Fix:
- use a distinct `DeviceID`/device identity for the second tuner

### 2. Provider metadata drift

Plex can keep stale/wrong provider `uri` values (for example wrong `guide.xml` source) in provider rows.

Symptoms:
- guide content from the wrong source
- duplicate-looking tabs/sources
- spinning/blank guide after deleting old providers

Fix:
- verify and repair provider rows, then refresh guides and channelmaps

### 3. Client cache / stale provider references

TV apps can cache provider IDs and continue requesting old providers even after backend fixes.

Symptoms:
- guide spinner
- source switch appears stuck on one provider

Fixes:
- force-stop app / reopen
- sign out/in
- clear app cache/data (client-specific)

### 4. Hidden Plex “active grabs” wedge

Plex can retain hidden active Live TV grabs and block new tunes (`Waiting for media grab to start`) even when `/status/sessions` shows no playback.

Operational recovery:
- see [plex-hidden-live-grab-recovery](../runbooks/plex-hidden-live-grab-recovery.md)

## Validation checklist (backend + UI)

Do not accept success on backend rows alone.

Validate all of:

1. Tuner endpoints
- `/discover.json`, `/lineup_status.json`, `/lineup.json`, `/guide.xml` return `200`

2. Plex backend objects
- `/livetv/dvrs` has expected DVRs
- `/media/grabbers/devices` has expected devices (and no stale helpers)
- provider endpoints return expected channel counts

3. Plex UI / client behavior
- configured tuner appears where expected
- guide loads
- switching sources changes guide content
- channel tune starts playback

## Related tools in this repo

- `scripts/plex-live-session-drain.py`
  - stale Plex live-session cleanup helper (external script)
- `scripts/plex-hidden-grab-recover.sh`
  - operational recovery for hidden Plex grabs
- `scripts/plex-generate-stream-overrides.py`
  - analyze channels for profile/transcode override candidates
- `scripts/plex-supervisor-cutover-map.py`
  - map category service URIs during supervisor cutover

See also
--------
- [testing-and-supervisor-config](testing-and-supervisor-config.md)
- [plex-hidden-live-grab-recovery](../runbooks/plex-hidden-live-grab-recovery.md)
- [deploy-and-connect-plex-home](../how-to/deploy-and-connect-plex-home.md)
- [k8s/README](../../k8s/README.md)
