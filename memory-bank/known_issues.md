# Known issues

<!-- Add bugs, limitations, and design tradeoffs as they are discovered or fixed. -->

## Security

- **Credentials:** Secrets must live only in `.env` (or environment). `.env` is in `.gitignore`. Never commit `.env` or log secrets. Use `.env.example` as a template (no real values).

## Plex / Integration

- **External XMLTV on `/guide.xml` can block Plex metadata flows:** `internal/tuner/xmltv.go` fetches/remaps the external XMLTV on every `GET /guide.xml` request (no cache). In live k3s testing on 2026-02-24, `plextuner-websafe` `guide.xml` took ~45s with `PLEX_TUNER_XMLTV_URL` enabled, causing Plex DVR metadata/channel APIs to hang or time out.
  - Workaround used in ops testing: restart `plex-tuner serve` for WebSafe without `PLEX_TUNER_XMLTV_URL` (placeholder guide), which reduced `guide.xml` to ~0.2s.
  - Follow-up finding (same day): with a direct PlexTuner catalog filtered to EPG-linked channels and deduped by `tvg-id` (91 channels), the same external XMLTV remap path served `guide.xml` in ~1.0–1.3s with real guide data. The no-cache design is still a scaling risk on large catalogs.
  - See also: `memory-bank/opportunities.md` (XMLTV caching + faster fallback).

- **Very large live lineups can make Plex DVR channel metadata very slow:** In live k3s testing on 2026-02-24, `plextuner-websafe` served ~41,116 live channels (`lineup.json` ~5.3 MB). Plex could tune known channels, but `/tv.plex.providers.epg.xmltv:138/lineups/dvr/channels` did not return within 15s.
  - Symptom: Plex API/probe helpers appear to hang while enumerating mapped channels, even when direct tune and stream playback path works.
  - Impact: channel management/mapping UX in Plex is slow; playback may still work for already-known channels.

- **13-way Threadfin DVR split can collapse to tiny counts when source feed lacks `tvg-id` coverage:** In live k3s testing on 2026-02-24, the upstream M3U had ~41,116 channels, but only 188 had `tvg-id` values present in XMLTV and only 91 remained after dedupe by `tvg-id`, so the 13 bucket files totaled 91 channels (many buckets empty).
  - Symptom: user expects a large split (e.g., "48k -> 6k"), but most `threadfin-*` `lineup.json` endpoints expose `0` channels and Plex DVRs created from them are empty.
  - Impact: Plex multi-DVR/category setup works technically, but channel volume is constrained by source feed + XMLTV linkage, not PlexTuner/Threadfin/Plex insertion logic.

- **Direct PlexTuner lineup/guide mismatch can produce “Unavailable Airings” when duplicate `tvg-id` rows remain in lineup:** The XMLTV remap path dedupes guide channels/programmes by `tvg-id`, but `lineup.json` will still expose every catalog row unless the catalog is deduped first.
  - Symptom (observed on 2026-02-24): direct WebSafe lineup had `188` channels while `guide.xml` only contained `91` unique channels/programme channels, causing many Plex guide entries to show no airings.
  - Workaround used in live testing: build the direct WebSafe catalog with `PLEX_TUNER_LIVE_EPG_ONLY=true`, then dedupe catalog `live_channels` by `tvg_id` before `serve`, resulting in matching lineup/guide counts (`91/91`).
  - Impact: direct PlexTuner mode can look broken in Plex guide UX even when streams work, unless lineup and XMLTV-remapped guide are aligned.

- **Plex Web/browser playback can still fail after successful tune and stream start (DASH `start.mpd` timeout):** In live probing against `plextunerWebsafe` (DVR `138`) on 2026-02-24, Plex `tune` succeeded (`200`) and PlexTuner relayed stream bytes, but Plex Web playback still failed later during DASH startup.
  - Symptom: `plex-web-livetv-probe.py` reports `start.mpd` timeout (`curl_exit=28`, `detail=startmpd1_0`) after a successful tune request.
  - Impact: direct PlexTuner path is working for tune/relay, but browser-based Plex Web playback remains unreliable pending DASH/transcode-path investigation.
