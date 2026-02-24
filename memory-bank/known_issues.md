# Known issues

<!-- Add bugs, limitations, and design tradeoffs as they are discovered or fixed. -->

## Security

- **Credentials:** Secrets must live only in `.env` (or environment). `.env` is in `.gitignore`. Never commit `.env` or log secrets. Use `.env.example` as a template (no real values).

## Plex / Integration

- **External XMLTV on `/guide.xml` can block Plex metadata flows:** `internal/tuner/xmltv.go` fetches/remaps the external XMLTV on every `GET /guide.xml` request (no cache). In live k3s testing on 2026-02-24, `plextuner-websafe` `guide.xml` took ~45s with `PLEX_TUNER_XMLTV_URL` enabled, causing Plex DVR metadata/channel APIs to hang or time out.
  - Workaround used in ops testing: restart `plex-tuner serve` for WebSafe without `PLEX_TUNER_XMLTV_URL` (placeholder guide), which reduced `guide.xml` to ~0.2s.
  - See also: `memory-bank/opportunities.md` (XMLTV caching + faster fallback).

- **Very large live lineups can make Plex DVR channel metadata very slow:** In live k3s testing on 2026-02-24, `plextuner-websafe` served ~41,116 live channels (`lineup.json` ~5.3 MB). Plex could tune known channels, but `/tv.plex.providers.epg.xmltv:138/lineups/dvr/channels` did not return within 15s.
  - Symptom: Plex API/probe helpers appear to hang while enumerating mapped channels, even when direct tune and stream playback path works.
  - Impact: channel management/mapping UX in Plex is slow; playback may still work for already-known channels.

- **13-way Threadfin DVR split can collapse to tiny counts when source feed lacks `tvg-id` coverage:** In live k3s testing on 2026-02-24, the upstream M3U had ~41,116 channels, but only 188 had `tvg-id` values present in XMLTV and only 91 remained after dedupe by `tvg-id`, so the 13 bucket files totaled 91 channels (many buckets empty).
  - Symptom: user expects a large split (e.g., "48k -> 6k"), but most `threadfin-*` `lineup.json` endpoints expose `0` channels and Plex DVRs created from them are empty.
  - Impact: Plex multi-DVR/category setup works technically, but channel volume is constrained by source feed + XMLTV linkage, not PlexTuner/Threadfin/Plex insertion logic.
