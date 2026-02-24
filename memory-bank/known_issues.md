# Known issues

<!-- Add bugs, limitations, and design tradeoffs as they are discovered or fixed. -->

## Security

- **Credentials:** Secrets must live only in `.env` (or environment). `.env` is in `.gitignore`. Never commit `.env` or log secrets. Use `.env.example` as a template (no real values).

## Plex / Integration

- **`plex.home` can fail with `503` when the Plex node (`kspls0`) goes `NotReady` even if the host Plex process is still running:** Observed on 2026-02-24 when `kspls0` root Btrfs was remounted read-only (`/` mounted `ro`), which caused `k3s` on `kspls0` to crash at startup (`kine.sock ... read-only file system`). The Plex pod on `kspls0` became stuck `Terminating`, and the replacement pod on `kspld0` stayed `Init:0/1` because its NFS mounts from `192.168.50.85` were unreachable.
  - Symptom: `https://plex.home` returns `503`, `kubectl -n plex get endpoints plex` is empty, and `kspls0` shows `NotReady`, while direct `curl http://192.168.50.85:32400` may still return Plex `401`.
  - Temporary workaround used (no Plex restart): make Service `plex` selectorless and attach a manual `EndpointSlice` pointing to `192.168.50.85:32400`, then delete the stale auto-managed slice. This restored `https://plex.home` to `401` (unauth expected).
  - Permanent fix required: recover `kspls0` host filesystem (root Btrfs `rw` again) and restart `k3s` on `kspls0`; then restore the normal `plex` Service selector (`app=plex`) and remove manual endpoint overrides.

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
  - Follow-up confirmation (same day): the same failure occurs on both `plextunerWebsafe` (`DVR 138`) and `plextunerTrial` (`DVR 135`) after Trial was fixed and remapped.
  - Plex log evidence: `GET /video/:/transcode/universal/decision` and `GET /video/:/transcode/universal/start.mpd` complete only after long waits (~100s and ~125s), then PMS logs `Failed to start session.` even while PlexTuner logs show `/stream/...` bytes relayed.
  - Impact: direct PlexTuner path is working for tune/relay and DVR mapping, but browser-based Plex Web playback remains blocked by Plex internal Live TV packaging/session startup behavior.

- **WebSafe ffmpeg HLS input can fail on Kubernetes short service hostnames (`*.svc`) even when Go HTTP requests succeed:** In live k3s testing on 2026-02-24, WebSafe with `PLEX_TUNER_FFMPEG_PATH=/workspace/ffmpeg-static` attempted the ffmpeg transcode path, but ffmpeg failed to resolve `iptv-hlsfix.plex.svc` and PlexTuner fell back to the Go raw HLS relay.
  - Symptom: WebSafe logs show `ffmpeg-transcode failed (falling back to go relay)` with stderr containing `Failed to resolve hostname iptv-hlsfix.plex.svc: System error`.
  - Workaround used in runtime testing: run WebSafe/Trial against a copy of the deduped catalog with HLSFix stream URLs rewritten from `iptv-hlsfix.plex.svc:8080` to the numeric service IP (`10.43.210.255:8080`) so ffmpeg receives a numeric host.
  - Impact: The intended WebSafe ffmpeg/PAT+PMT startup path silently degrades to the raw relay path in k3s, which preserves the Plex Web `start.mpd` failure mode.

- **WebSafe ffmpeg startup gate can time out and force a fallback to raw relay even after the ffmpeg DNS issue is removed:** In live k3s testing on 2026-02-24 (with numeric HLSFix URLs), WebSafe ffmpeg launched and PAT/PMT keepalive started, but ffmpeg produced no payload bytes before the startup gate deadline and PlexTuner killed ffmpeg, emitted timeout bootstrap TS, and fell back to the Go raw relay.
  - Symptom: Logs show `ffmpeg-transcode pat-pmt-keepalive start`, then `... stop=startup-gate-timeout`, `timeout-bootstrap emitted before relay fallback`, and `ffmpeg-transcode failed (falling back to go relay): startup-gate-timeout`.
  - Root cause follow-up (same day): a major contributor was ffmpeg's generic HTTP reconnect flags on live HLS (`-reconnect*`, especially `-reconnect_at_eof`) causing `.m3u8` EOF/reconnect loops and delayed/failed first-segment loads. Manual ffmpeg in the pod reproduced the loop (`Will reconnect at 1071 ... error=End of file`) and immediately succeeded when reconnect flags were removed.
  - Code/runtime follow-up (same day): `internal/tuner/gateway.go` was patched so `PLEX_TUNER_FFMPEG_HLS_RECONNECT` defaults to `false` for HLS ffmpeg input; a clean WebSafe runtime build was deployed and verified in live probes (`reconnect=false` in logs, `startup-gate-ready`, `first-bytes`, and multi-MB ffmpeg payload streamed).
  - Impact: The "no ffmpeg payload before startup gate" failure mode is significantly reduced after disabling HLS reconnect flags by default, but Plex Web can still fail later in `start.mpd` for other reasons (see next issue).

- **Plex Web can still fail `start.mpd` even when WebSafe ffmpeg is streaming healthy TS bytes if early startup output lacks video IDR:** Observed on 2026-02-24 after fixing WebSafe ffmpeg DNS + HLS reconnect behavior and redeploying the patched WebSafe binary.
  - Symptom: WebSafe logs show `reconnect=false`, `startup-gate-ready`, `ffmpeg-transcode first-bytes=...`, and long successful ffmpeg stream runs (`ffmpeg-transcode bytes=11275676` / `client-done bytes=18996512`), but `plex-web-livetv-probe.py` still fails `startmpd1_0`.
  - Startup-gate evidence: `startup-gate buffered=32768 ... idr=false aac=true` and, after stricter runtime tuning (`REQUIRE_GOOD_START=true`, `STARTUP_TIMEOUT_MS=12000`, larger max prefetch), `startup-gate buffered=524288 ... idr=false aac=true`.
  - Follow-up hypothesis from live testing: forcing `PLEX_TUNER_FFMPEG_HLS_LIVE_START_INDEX=-1` likely increases the chance of starting mid-GOP (audio arrives first, no early decodable video). Restoring `-3` is safer but did not immediately produce `idr=true` in the tested run.
  - Impact: WebSafe ffmpeg startup is no longer the primary blocker; remaining browser failure likely involves early video/keyframe readiness vs Plex live packaging timeout behavior.

- **Plex internal Live TV HLS manifest (`/livetv/sessions/<live>/<client>/index.m3u8`) can stay zero-byte while the first-stage recorder is healthy, causing repeated `buildLiveM3U8: no segment info available` and browser `start.mpd` timeouts:** Observed on 2026-02-24 during fresh WebSafe browser probes on channels `103`, `104`, and `109`.
  - Symptom: Plex `tune` succeeds (`200`), the first-stage recorder writes many `media-*.ts` files in the transcode session directory and reports `progress/stream` + `progress/streamDetail` (video/audio codecs and dimensions), but in-container `curl` to Plex's own `http://127.0.0.1:32400/livetv/sessions/<live>/<client>/index.m3u8?...` times out with **0 bytes** for tens of seconds to minutes.
  - Plex log evidence: repeated `buildLiveM3U8: no segment info available` for the same live session/client pair while the recorder continues writing and while the internal HLS endpoint still returns zero bytes. `decision` / `start.mpd` can complete only after ~100s (too slow for Plex Web startup), and `buildLiveM3U8` warnings may continue afterward.
  - Follow-up comparison (same day): the same behavior persists across multiple WebSafe output profiles (`aaccfr`, `plexsafe`, and forced `pmsxcode` with `client_adapt=false`), including a run where Plex's first-stage streamDetail reported `mpeg2video` + `mp2`.
  - TS timing/continuity follow-up (same day, fresh channel `111`): PlexTuner WebSafe ffmpeg output for the first 12,000 TS packets was structurally clean (`sync_losses=0`, PAT/PMT repeated, PCR PID + monotonic PCR, monotonic video/audio PTS, `idr=true` at startup, no CC errors on media PIDs). The only large continuity duplicate count was on PID `0x1FFF` (null packets), which is expected/benign. Despite this, Plex still delayed `decision`/`start.mpd` about `100s` and only then launched the second-stage DASH transcode from the internal `/livetv/sessions/.../index.m3u8` URL.
  - Session-cache follow-up (same day, fresh channel `108`, live session `dfeb3d9f-...`): during the browser timeout window, Plex's transcode cache directory `.../plex-transcode-dfeb3d9f-...` contained dozens of first-stage `media-*.ts` files with healthy non-zero sizes (plus the current in-progress segment at `0` bytes), while a concurrent in-container `curl -m 5` to the matching internal `/livetv/sessions/dfeb3d9f-.../ff10b85.../index.m3u8?...` still timed out with 0 bytes.
  - Logging follow-up (same day): PMS logs show the first-stage segmenter job command line includes `-segment_list http://127.0.0.1:32400/video/:/transcode/session/<live>/<id>/manifest?...` and show many `/progress` callback request/completed lines for the same first-stage transcode session, but no `/video/:/transcode/session/.../manifest` request lines are visible in `Plex Media Server.log` (unclear whether manifest callbacks are failing silently or simply not logged by PMS).
  - Impact: The remaining browser failure is not explained by missing tuner bytes or a specific WebSafe codec profile; the bottleneck is Plex's internal segment-info/manifest readiness for Live TV.

- **Plex can reuse hidden Live TV `CaptureBuffer`/transcode sessions that are not visible in `/status/sessions` or `/transcode/sessions`, causing repeated probes to ignore tuner changes:** Observed on 2026-02-24 while iterating WebSafe/Trial ffmpeg settings.
  - Symptom: Re-probing the same channel reuses the same `TranscodeSession` key in `start.mpd` debug XML (`CaptureBuffer` response) and no new `/stream/...` request appears in PlexTuner logs, even after `plex-live-session-drain.py --all-live`.
  - Follow-up evidence: `/status/sessions` and `/transcode/sessions` both report `size=0`, and direct `POST /video/:/transcode/universal/stop?session=<id>` returns `404` for the hidden session IDs.
  - Impact: Probe runs can produce false negatives/false positives because changes to tuner env/config are not exercised unless a truly fresh channel/session is forced (for example by using an untested channel or changing Plex-visible channel identity).

- **k3s apiserver -> kubelet exec proxy to the Plex pod can intermittently return `502`, blocking probe helper scripts that read the Plex token from inside the pod:** Observed on 2026-02-24 while rerunning `plex-web-livetv-probe.py` from `kspld0`.
  - Symptom: The helper fails before running the probe with `proxy error ... dialing 192.168.50.85:10250, code 502: 502 Bad Gateway` when it calls `kubectl exec deploy/plex`.
  - Impact: Probe automation can fail transiently even when Plex and PlexTuner are healthy; use direct tuner logs and/or a cached/direct Plex token path when this occurs.

- **Direct Trial DVR can become unusable if Plex HDHomeRun device URI is registered as `127.0.0.1:5004` instead of the cluster service URI:** Observed on 2026-02-24 after a Plex restart; `DVR 135` (`plextunerTrial`) existed but had `0` mapped channels and the associated device (`key=134`) pointed to `uri="http://127.0.0.1:5004"`.
  - Symptom: `plex-activate-dvr-lineups.py --dvr 135` fails with `No valid ChannelMapping entries found` while `DVR 138` (WebSafe) activates normally.
  - Workaround/fix used: re-register the same HDHomeRun device to `plextuner-trial.plex.svc:5004` via Plex API (`/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=...`), which updates the existing device URI in place; then `reloadGuide` + `plex-activate-dvr-lineups.py --dvr 135` succeeds (`after=91`).
  - Impact: Trial DVR appears "set up" in Plex but is effectively dead/unmappable until the device URI is corrected.
