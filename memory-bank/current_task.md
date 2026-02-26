# Current task

<!-- Update at session start and when focus changes. -->

**Goal:** Restore actual Plex Live TV playback (not just tune success) on the pure PlexTuner injected DVR path and document the true root cause/fix. Current focus has shifted from PlexTuner stream-format hypotheses to Plex Media Server Live TV internal callback authorization behavior after proving the first-stage recorder was healthy.

**Scope:** In: live validation/triage only (k3s + Plex API/logs), minimal process restarts inside `plextuner-build` pod, runtime-only tuner env/catalog experiments (WebSafe/Trial), minimal tuner-code instrumentation needed for live triage, and documenting operational findings (ffmpeg DNS/startup-gate behavior, hidden Plex capture-session reuse). Out: Plex pod restarts, Threadfin pipeline changes, infra/firewall persistence, and unrelated code changes (another agent is modifying `internal/hdhomerun/*`).

**Last updated:** 2026-02-26

**Breakthrough (2026-02-25 late):**
- Reused the existing `k3s/plex/scripts/plex-websafe-pcap-repro.sh` harness on pure `DVR 218` (`FOX WEATHER`, helper AB4 `:5009`) and finally captured the missing signal: PMS first-stage `Lavf` `/video/:/transcode/session/.../manifest` callbacks were hitting `127.0.0.1:32400` and receiving repeated HTTP `403` responses (visible in localhost pcap), while Plex logs only showed `buildLiveM3U8: no segment info available`.
- Root cause is Plex-side callback auth, not PlexTuner TS formatting: first-stage `ssegment` was posting valid CSV segment rows, but PMS rejected the callback updates, so `/livetv/sessions/.../index.m3u8` had no segment info.
- Applied a Plex runtime workaround by adding `allowedNetworks="127.0.0.1/8,::1/128,192.168.50.0/24"` to PMS `Preferences.xml` and restarting `deploy/plex`.
- Post-fix validation:
  - pcap harness rerun: first-stage callback responses flipped from `403` to `200`; PMS internal `/livetv/sessions/.../index.m3u8` returned `200` with real HLS entries; logs changed from `buildLiveM3U8: no segment info available` to healthy `buildLiveM3U8: min ... max ...`.
  - Plex Web probe path (`DVR 218`, `FOX WEATHER`) now reaches immediate `decision` + `start.mpd` success and returns DASH init headers and first segments (`/0/header`, `/0/0.m4s`, `/1/header`, `/1/0.m4s` all with bytes).
- Full probe succeeded after patching the external probe script decode bug (binary DASH segment fetches caused `UnicodeDecodeError` in the harness, not playback failure).

**Follow-on fixes (2026-02-25 night):**
- User reported Plex Web/Chrome video-without-audio while TV clients worked, plus lingering Live TV sessions when LG/webOS input is switched without stopping playback.
- Verified the lingering HLS pulls are Plex client/session lifecycle behavior (PMS keeps pulling while the LG app remains "playing" in the background), not PlexTuner streaming independently after a client disconnect.
- Found the immediate Chrome-audio blocker on injected category DVRs was runtime drift: the 13 category `plextuner-*` deployments were running shell-less `plex-tuner:hdhr-test` images without `ffmpeg`, and with `PLEX_TUNER_STREAM_TRANSCODE=off`, so PlexTuner relayed raw HLS (HE-AAC source audio) to Plex.
- Durable repo fixes landed:
  - `Dockerfile` and `Dockerfile.static` now install `ffmpeg`
  - `internal/tuner/gateway.go` logs explicit warnings when transcode was requested but `ffmpeg` is unavailable
  - added `scripts/plex-live-session-drain.py` for manual Plex Live TV session cleanup (no max-live TTL behavior)
- Found and fixed a real app regression during rollout: `cmd/plex-tuner` `run -mode=easy` (`fetchCatalog`) ignored configured `PLEX_TUNER_M3U_URL` / built M3U URLs unless `-m3u` was passed explicitly; patched it to honor `cfg.M3UURLsOrBuild()` first.
- Runtime rollout completed on `kspls0` (all 13 category pods):
  - built/imported ffmpeg-enabled `plex-tuner:hdhr-test` into k3s containerd on-node
  - restarted all 13 category deployments successfully and verified `ffmpeg` exists in category pods
  - set `PLEX_TUNER_STREAM_TRANSCODE=on` across the 13 category deployments for immediate web audio normalization (client-adapt optimization can follow later)

**Takeover note (2026-02-25):** Taking over live Plex/PlexTuner DVR-delivery triage after another agent stalled in repeat probe loops. Immediate priority is to re-validate the current runtime state (Plex reachability, active PlexTuner WebSafe/Trial services, DVR mappings) and reproduce with fresh channels/sessions only, following the hidden `CaptureBuffer` reuse loop guardrails.

**Takeover progress (2026-02-25):**
- Root cause for the immediate "DVRs not delivering" state was operational drift, not the previously investigated Plex packager issue: the `plextuner-trial` / `plextuner-websafe` services still existed but had **no endpoints** because the `app=plextuner-build` pod was gone, and Plex DVR devices `135` / `138` had also drifted to the wrong URI (`http://plextuner-otherworld.plex.svc:5004`).
- Temporary runtime recovery applied (no Plex restart): recreated a lightweight `plextuner-build` deployment (helper pod) in `plex`, copied a fresh static `plex-tuner` binary into `/workspace`, regenerated shared live catalogs from provider API creds (`PLEX_TUNER_PROVIDER_*`, `LiveOnly`, `LiveEPGOnly`), and started Trial (`:5004`) + WebSafe (`:5005`) processes with `PLEX_TUNER_LINEUP_MAX_CHANNELS=-1`.
- Plex device URIs were repaired in-place via `/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=...` for `plextuner-trial.plex.svc:5004` and `plextuner-websafe.plex.svc:5005`; Plex then immediately resumed `GET /discover.json` and `GET /lineup_status.json` to both tuners (confirmed in tuner logs).
- Current follow-on blocker for "fully healthy" direct DVRs in this temporary runtime is guide refresh latency: Plex `reloadGuide` hits both tuners, but external XMLTV fetches timed out at ~45s and PlexTuner fell back to placeholder `guide.xml`, which also made `plex-activate-dvr-lineups.py` / random stream probes stall on guide/channel metadata calls.
- Revalidated the current helper runtime from code + live logs and corrected stale assumptions: direct Trial/WebSafe now run from local `iptv-m3u-server` feeds (`live.m3u` + `xmltv.xml`) with fast real guide responses (~1.4–2.5s, ~70 MB XML), and Plex `reloadGuide` does trigger tuner `/guide.xml` fetches again.
- Found a new operational regression in the ad hoc helper pod: WebSafe was running without `ffmpeg`, so `STREAM_TRANSCODE=true` silently degraded to the Go raw HLS relay (`hls-relay` logs only). Installed `ffmpeg` in the helper pod (`apt-get install -y ffmpeg`) and restarted only the WebSafe `serve` process with `PLEX_TUNER_FFMPEG_PATH=/usr/bin/ffmpeg`.
- Fresh browser-path probe after restoring ffmpeg (`DVR 138`, channel `108`) still fails `startmpd1_0`, but now with confirmed WebSafe ffmpeg output (`ffmpeg-transcode`, startup gate `idr=true`, `aac=true`, first bytes in ~4.1s), which strengthens the Plex-internal packaging diagnosis.
- User-directed pivot completed: restored and validated the **13-category injected DVR path using PlexTuner only** (no Threadfin in device or lineup URLs). Recreated DVRs `218,220,222,224,226,228,230,232,234,236,238,240,242` with devices `http://plextuner-<bucket>.plex.svc:5004` and lineups `lineup://.../http://plextuner-<bucket>.plex.svc:5004/guide.xml#plextuner-<bucket>`.
- Root cause of earlier empty 13-bucket category tuners was not PlexTuner indexing: `iptv-m3u-server` postvalidation had zeroed many generated `dvr-*.m3u` files after probe failures. Rerunning only the splitter (skipping postvalidate) restored non-empty category M3Us; all 13 `plextuner-*` deployments then loaded live channels and exposed service endpoints.
- Pure-app channel activation completed successfully for all 13 injected DVRs (`plex-activate-dvr-lineups.py ... --dvr 218 ... 242`): final status `OK` with mapped counts `44,136,308,307,257,206,212,111,465,52,479,273,404` (total `3254` mapped channels).
- Pure-app playback proof (category DVR): `plex-web-livetv-probe.py --dvr 218` tuned `US: NEWS 12 BROOKLYN` (`POST /livetv/dvrs/218/channels/39/tune -> 200`), PlexTuner `plextuner-newsus` logged `/stream/News12Brooklyn.us` startup + HLS playlist relay, but Plex probe still failed `startmpd1_0` after ~35s.
- Smart TV spin proof from Plex logs (client `192.168.50.148`): Plex starts first-stage grabber, reads from PlexTuner stream URLs, receives `progress/streamDetail`, then its own internal `GET /livetv/sessions/.../index.m3u8` returns `500` with `buildLiveM3U8: no segment info available`, while client `start.mpd` requests complete ~100–125s later or after stop.
- Repo hygiene pass completed for this concern: removed non-essential "Threadfin-style" wording from Plex API registration code/logs and stale k8s helper comments; remaining `threadfin` references in this repo are comparison docs, historical memory-bank notes, or explicit legacy secret-name context.
- Plex cleanup completed: deleted all stale Threadfin-era DVRs (`141,144,147,150,153,156,159,162,165,168,171,174,177`). Current DVR inventory is now only the 2 direct test DVRs (`135`, `138`) plus the 13 pure `plextuner-*` injected DVRs (`218..242`) with no `threadfin-*` entries left.
- Category A/B test completed on `DVR 218` (`plextuner-newsus`): temporarily switched the `plextuner-newsus` deployment to WebSafe-style settings (`STREAM_TRANSCODE=true`, `PROFILE=plexsafe`, `CLIENT_ADAPT=false`, `FFMPEG_PATH=/usr/bin/ffmpeg`), reran Plex Web probe, then rolled back the deployment to original `STREAM_TRANSCODE=off`.
- A/B result: no playback improvement. The `DVR 218` probe still failed `startmpd1_0` (~37s), and `plextuner-newsus` logs still showed HLS relay (`hls-playlist ... relaying as ts`) rather than `ffmpeg-transcode`, so the category `plex-tuner:hdhr-test` runtime did not exercise a true ffmpeg WebSafe path in this test.
- PMS evidence for the A/B session (`live=798fc0ae-...`, client session `19baaba...`) matches the existing pattern: Plex started the grabber against `http://plextuner-newsus.../stream/FoxBusiness.us`, received `progress/streamDetail`, the client timed out/stopped, and PMS only completed `decision`/`start.mpd` ~95s later. Extra `connection refused` errors appeared afterward because the A/B pod was intentionally restarted for rollback while PMS still had the background grabber open.
- Helper-pod ffmpeg A/Bs on `DVR 218` now prove the category path can run a real WebSafe ffmpeg stream when Plex is repointed to helper services (`:5006+`), and this surfaced two distinct problems instead of one:
  - `:5006` (`plexsafe`, bootstrap enabled, old binary): Plex first-stage recorder failed almost immediately with repeated `AAC bitstream not in ADTS format and extradata missing`, then `Recording failed. Please check your tuner or antenna.` while PlexTuner showed `bootstrap-ts` followed by `ffmpeg-transcode` bytes.
  - `:5007` (`plexsafe`, bootstrap disabled) and `:5008` (`aaccfr`, bootstrap disabled): Plex recorder stayed healthy for the full probe window (continuous `progress/streamDetail`, no recorder crash), but Plex Web still failed `startmpd1_0`.
- Root-cause isolation from those helper A/Bs: the WebSafe `bootstrap-ts` path was emitting a fixed H264/AAC bootstrap even when the active profile output audio was MP3/MP2 (`plexsafe`/`pmsxcode`), creating a mid-stream audio codec switch that can break Plex's recorder.
- Code fix implemented in `internal/tuner/gateway.go`: WebSafe `bootstrap-ts` audio codec now matches the active output profile (`plexsafe`=MP3, `pmsxcode`=MP2, `videoonly`=no audio, otherwise AAC) and bootstrap logs now include `profile=...`.
- Live validation of the code fix using a patched helper binary (`:5009`, `plexsafe`, bootstrap enabled) succeeded for the recorder-crash case:
  - PlexTuner logs show `bootstrap-ts ... profile=plexsafe`
  - PMS no longer logs the previous AAC/ADTS recorder failure
  - PMS first-stage `progress/streamDetail` reports `codec=mp3` and keeps recording alive
  - Plex Web probe still fails `startmpd1_0` (remaining PMS packager/startup issue unchanged)
- New focused `DVR 218` / helper `:5009` (`dashfast`, `realtime`, patched binary) long-wait probes on **2026-02-25** confirm the failure is deeper than the browser's 35s timeout:
  - With extended probe timeouts (`HTTP_MAX_TIME=130`, `DASH_READY_WAIT_S=140`), Plex delays the first `start.mpd` response ~`100–125s`.
  - A normal concurrent probe (`decision` + `start.mpd`) can still induce a second-stage transcode self-kill race, but a **serialized/no-decision** probe reproduces the same end result, so the race is not the root cause.
  - After the delayed `start.mpd`, Plex returns an MPD shell and exposes a DASH session ID, but repeated `GET /video/:/transcode/universal/session/<session>/0/header` stays `404` for ~2 minutes (`dash_init_404`).
  - PMS logs for the serialized run show the second-stage DASH transcode starts (`Req#7b280`) and then fails with `TranscodeSession: timed out waiting to find duration for live session` -> `Failed to start session.` -> `Recording failed. Please check your tuner or antenna.`
  - Concurrent TS inspector capture on the same Fox Weather run (`PLEX_TUNER_TS_INSPECT_MAX_PACKETS=120000`) shows ~63s of clean PlexTuner ffmpeg TS output (`sync_losses=0`, monotonic PCR/PTS, no media-PID CC errors, no discontinuities), strengthening the case that PlexTuner output is not the immediate trigger.

---

## Assumptions & questions (only if uncertainty matters)
Assumptions (safe defaults you are proceeding with):
- Local environment may not have Go installed; OK to use a temporary local Go toolchain (non-system install) only for verification.
- k3s/Plex troubleshooting changes on remote hosts may be temporary runtime fixes unless later codified in infra manifests or host firewall config.
- Existing WebSafe/Trial pod processes and DVR IDs noted below may have drifted since 2026-02-24; all IDs/URIs must be rechecked before interpreting probe results.

Questions (ONLY if blocked or high-risk ambiguity):
- Q: None currently blocking for this patch-sized change.
- Q: None currently blocking. User confirmed initial tier-1 client matrix for `HR-003`: LG webOS, Plex Web (Firefox/Chrome), iPhone iOS, and NVIDIA Shield TV (Android TV/Google target coverage).

## Opportunity radar (don't derail)
- If you notice out-of-scope improvements, record them in `memory-bank/opportunities.md` and raise to the user in your summary.

## Parallel agent tracking
- **Agent 2 (this session):** HDHR k8s standup: Ingress, run-mode deployment, BaseURL=http://plextuner-hdhr.plex.home, k8s/README.md.

## Self-check (quality bar — fill before claiming done)
- **Correctness:** ✅ Pure PlexTuner injected DVR path remains active (`218..242`), and Plex Web playback on `DVR 218` (`FOX WEATHER`) is now working after the PMS `allowedNetworks` callback-auth workaround. Root cause for the prior `buildLiveM3U8`/`start.mpd` failures was PMS rejecting its own first-stage `/manifest` callbacks (`403`), not a PlexTuner stream/HLS selection issue.
- **Tests:** ✅ Reproduced and fixed with before/after pcap + PMS-log evidence on `DVR 218` helper AB4 (`:5009`), then verified browser-path success with `plex-web-livetv-probe.py` (post-fix probe returns `OK`; DASH init + first media segments fetched for video/audio). ⚠️ The external probe harness needed a binary-safe decode patch (`errors="replace"`) to avoid false `UnicodeDecodeError` failures once playback actually started working.
- **Risk:** med-high (runtime state in Plex/k3s can drift after Plex restarts, hidden Plex capture/transcode reuse can invalidate probe results, and current tuner env/catalog experiments are temporary)
- **Performance impact:** current direct helper runtime serves a much larger catalog (~6,207 live channels) but local-feed guide fetches remain fast enough (~1.4–2.5s `guide.xml` from Plex requests, ~70 MB payload). The current browser blocker remains a Plex startup/packager-readiness issue, not raw tuner throughput or ffmpeg startup.
- **Security impact:** none (token used in-container only; not printed)

## Decisions (single source of truth)
- If you make a **durable** decision, promote it to **ADR** (`docs/adr/`) or **memory-bank**.
- If you're **unsure whether it's durable**, don't promote yet — note it in Assumptions.

## Docs (done = doc update when behavior changes)
- If you changed **behavior, interfaces, or config:** update or create **one** doc in `docs/`; add cross-links. Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md). (Memory-bank updates are in scope for this patch; broader docs can follow if this behavior is promoted.)
- If you **noticed doc gaps** but it's out of scope: file in `memory-bank/opportunities.md`.

---

## Parallel threads (2026-02-24)

- **agent1:** Live Plex Web packaging/`start.mpd` triage on direct PlexTuner (WebSafe/Trial) via k3s/PMS logs; avoid Plex restarts and preserve current runtime state.
- **agent2:** Non-HDHR validation lane for main PlexTuner functionality: local automated tests + live-race harness (synthetic/replay), VOD/FUSE virtual-file smoke check, and non-disruptive direct Plex API probe loop against `https://plex.home` using existing preconfigured DVRs only (no re-registration/restart).

**Live session cleanup follow-on (2026-02-26):** Added a multi-layer Plex-side stale-session reaper path to `scripts/plex-live-session-drain.py` to address lingering Live TV streams after browser tab close / LG input switch. The script now supports (1) polling-based stale detection using `/status/sessions` + PMS request activity, (2) optional Plex SSE notifications as fast rescan triggers, and (3) optional lease TTL backstop. Live dry-run validation against an active Chrome session confirmed no false idle kill after wiring SSE activity into the idle timer.

**Feed criteria / override tooling (2026-02-26):** Added `scripts/plex-generate-stream-overrides.py` to probe a tuner `lineup.json` and generate criteria-based channel overrides for `PLEX_TUNER_PROFILE_OVERRIDES_FILE` / `PLEX_TUNER_TRANSCODE_OVERRIDES_FILE`. It reuses the existing override path and supports `--replace-url-prefix` for port-forwarded category tuners whose lineup URLs contain cluster-internal hostnames. Validation on `ctvwinnipeg.ca` (the Chrome rebuffer case) correctly produced no flag, reinforcing that this case is a PMS transcode-throughput issue rather than an obvious feed-format problem.

**Built-in Plex session reaper (2026-02-26):** Ported the stale-session watchdog into the Go app as an optional background worker started by `tuner.Server.Run` (no Python dependency required for packaged builds). It uses Plex `/status/sessions` polling and optional Plex SSE notifications for fast wake-ups, with configurable idle timeout, renewable lease timeout, and hard lease backstop. Enable with `PLEX_TUNER_PLEX_SESSION_REAPER=1` plus existing `PLEX_TUNER_PMS_URL` / `PLEX_TUNER_PMS_TOKEN`.

**XMLTV language normalization (2026-02-26):** Added in-app guide text normalization for remapped external XMLTV feeds. New env-controlled policy can prefer `lang=` variants (e.g. `en,eng`), prefer Latin-script variants among repeated programme nodes, and optionally replace mostly non-Latin programme titles with the channel name (`PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK=channel`). This addresses the user-reported Plex guide text showing Cyrillic/Arabic-like titles when upstream XMLTV is multilingual or non-English.

**Single-app supervisor mode (2026-02-26):** Added `plex-tuner supervise -config <json>` to run multiple child `plex-tuner` instances in one container/process supervisor for packaged "one pod runs many DVR buckets" deployments. First-pass design uses child processes (not in-process goroutine multiplexing) for lower risk and code reuse. Important constraint: HDHR network mode (UDP/TCP 65001) should be enabled on only one child unless custom HDHR ports are assigned.

**Single-pod supervisor example assembled (2026-02-26):** Added a concrete `k8s/plextuner-supervisor-multi.example.json` with 14 children (`13` category DVR insertion instances + `1` big-feed HDHR wizard instance) and `k8s/plextuner-supervisor-singlepod.example.yaml` showing a host-networked single-pod deployment with a multi-port Service for category HTTP ports. The HDHR child alone enables `PLEX_TUNER_HDHR_NETWORK_MODE=true`; category children use HTTP-only ports `5101..5113` on `plextuner-supervisor.plex.svc`.

**Single-pod supervisor live cutover completed (2026-02-26 late):**
- Regenerated real supervisor artifacts with timezone-guided HDHR preset selection (`na_en`) and updated the HDHR child to use the broad feed (`live.m3u`) with in-app filtering/cap:
  - `PLEX_TUNER_LINEUP_DROP_MUSIC=true`
  - `PLEX_TUNER_LINEUP_MAX_CHANNELS=479`
  - XMLTV English-first normalization envs enabled
- Reapplied only the generated supervisor `ConfigMap` + `Deployment` in `k3s/plex`, then patched the deployment image back to the custom locally imported tag (`plex-tuner:supervisor-cutover-20260225223451`) on `kspls0` to retain the new `supervise` binary.
- Verified the supervisor pod is healthy (`1/1`) and all 14 child instances start, with category children serving bare category identities (`FriendlyName`/`DeviceID` = `newsus`, `generalent`, etc.) and the HDHR child advertising `BaseURL=http://plextuner-hdhr.plex.home`.
- Verified HDHR child behavior inside the supervisor pod:
  - `Loaded 6207 live channels`
  - `Lineup pre-cap filter: dropped 72 music/radio channels`
  - `/lineup.json` count = `479`
- Applied only the generated Service documents and confirmed category/HDHR Services now route to the supervisor pod endpoints (`192.168.50.85:510x` / `:5004`), then scaled the old 13 category deployments to `0/0`.
- Sample post-cutover validation from inside the Plex pod:
  - `http://plextuner-newsus.plex.svc:5004/discover.json` reports `FriendlyName=newsus`
  - `http://plextuner-hdhr-test.plex.svc:5004/lineup.json` returns `479` entries

**HDHR wizard noise reduction follow-up (2026-02-26 late):**
- Plex's "hardware we recognize" list is driven by `/media/grabbers/devices` (and cached DB rows in `media_provider_resources`), so active injected category DVR devices still appear there as known HDHR devices (e.g. `otherworld`) even though they are not the intended wizard lane.
- Added in-app `PLEX_TUNER_HDHR_SCAN_POSSIBLE` support (`/lineup_status.json`) and regenerated the supervisor config so:
  - category children return `{"ScanPossible":0}`
  - the dedicated HDHR child returns `{"ScanPossible":1}`
- Live-verified on the running supervisor pod and via the Plex pod:
  - `plextuner-otherworld` -> `ScanPossible=0`
  - `plextuner-hdhr-test` -> `ScanPossible=1`
- Cleaned the stale helper cache row (`newsus-websafeab5:5010`) from Plex's `media_provider_resources`; it no longer appears in `/media/grabbers/devices`.
- Important operational gotcha rediscovered: image imports must happen on the actual scheduled node (`kspls0`) runtime, not the local `k3s` runtime on `kspld0`, or kubelet will keep reporting `ErrImageNeverPull` even when local `crictl` on the wrong host shows the image.

**Plex TV UI / provider metadata follow-up (2026-02-26 late):**
- User-reported TV symptom ("all tabs labelled `plexKube`" and identical-looking guides) is **not** caused by flattened tuner feeds. Verified live tuner outputs remain distinct after the supervisor cutover:
  - `newsus=44`, `bcastus=136`, `otherworld=404`, `hdhr1=479`, `hdhr2=479` (`/lineup.json` counts).
- Verified Plex backend provider endpoints are also distinct per DVR:
  - `/tv.plex.providers.epg.xmltv:<id>/lineups/dvr/channels` returns different sizes (for example `218=44`, `220=136`, `242=404`, `247=308`, `250=308`).
- Found and repaired Plex DB metadata drift in `media_provider_resources`:
  - direct provider child rows `136` (`DVR 135`) and `139` (`DVR 138`) had `uri=http://plextuner-otherworld.../guide.xml`
  - most injected/HDHR provider child rows (`type=3`) had blank `uri`
  - `DVR 218` device row `179` still pointed to helper A/B URI `http://plextuner-newsus-websafeab4.plex.svc:5009`
- Applied a DB patch (with file backup first) setting `type=3` provider child `uri` values to each DVR's actual `.../guide.xml` and repaired row `179` to `http://plextuner-newsus.plex.svc:5004`; `/livetv/dvrs/218` now reflects the correct device URI again.
- Remaining evidence points to Plex client/UI presentation behavior:
  - `/media/providers` still emits every Live TV `MediaProvider` with `friendlyName="plexKube"` and `title="Live TV & DVR"` (Plex-generated), which likely explains the repeated tab labels on TV clients.
  - Need live LG/webOS request capture to confirm whether the TV app is actually requesting distinct `tv.plex.providers.epg.xmltv:<id>` grids when switching tabs.

**LG TV guide-path capture + cleanup (2026-02-26 late):**
- File-level Plex logs (`Plex Media Server.log`, not `kubectl logs`) finally captured the LG client (`192.168.50.225`) guide requests.
- Root cause for the wrong TV guide behavior in the captured session: the LG was requesting **only provider `tv.plex.providers.epg.xmltv:135`** (`DVR 135` / legacy direct `plextunerTrial`) for:
  - `/lineups/dvr/channels`
  - `/grid?...`
  - `/hubs/discover?...`
  while also sending playback/timeline traffic (`context=source:content.dvr.guide`).
- This explains why TV-side guide behavior could look wrong/duplicated even though injected category providers were distinct: the TV was on the old direct test provider, not a category provider.
- Cleanup applied:
  - deleted legacy direct test DVRs `135` and `138` via Plex API (`DELETE /livetv/dvrs/<id>`)
  - deleted orphan HDHR device rows `134` (`plextuner01`) and `137` (`plextunerweb01`) from `media_provider_resources` after API deletion left them in `/media/grabbers/devices`
- Post-cleanup validation:
  - `/livetv/dvrs` now contains only injected category DVRs (`218..242`) + HDHR wizard DVRs (`247`, `250`)
  - `/media/grabbers/devices` no longer lists `plextuner01` / `plextunerweb01`

**Guide-collision fix for injected DVR tabs (2026-02-26 late):**
- User confirmed Plex now shows the correct DVR count (`15`), but multiple tabs/sources in Plex Web appeared to show the same guide content while channel names differed.
- Root cause was **channel/guide ID collisions across DVRs**, not flattened feeds:
  - category tuners all exposed `GuideNumber` sequences starting at `1,2,3...`
  - Plex provider/UI layers could cache/reuse guide/grid content when multiple DVRs shared overlapping channel IDs.
- Implemented in-app `PLEX_TUNER_GUIDE_NUMBER_OFFSET` and wired it into `tuner.Server.UpdateChannels` so each child instance can expose a distinct channel/guide-number range.
- Rolled a new supervisor image (`plex-tuner:supervisor-guideoffset-20260226001027`) on `kspls0` and updated the live supervisor `ConfigMap` to assign offsets:
  - examples: `bcastus=1000`, `newsus=2000`, `sportsa=3000`, ..., `otherworld=13000`, `hdhr-main2=100000`
- Live validation from the Plex pod after rollout:
  - tuner `guide.xml` channel IDs are now distinct by source (`newsus:2001+`, `bcastus:1001+`, `sportsa:3001+`, `otherworld:13001+`)
  - Plex provider channel endpoints now expose non-overlapping first IDs:
    - `218/newsus -> first_id=2001`
    - `220/bcastus -> first_id=1001`
    - `242/otherworld -> first_id=13001`
    - `250/HDHR2 -> first_id=103260`
- Rebuilt Plex mappings after the offset change:
  - `scripts/plex-reload-guides-batched.py` completed for all `15` DVRs
  - `scripts/plex-activate-dvr-lineups.py` replayed channelmaps for all `15` DVRs (all `status=OK`; HDHR `247/250` remain `308` valid mappings due to Plex channelmap validity limits)
- User validation after remap:
  - first tabs now show distinct guides/EPGs (guide-collision symptom resolved)

**Post-remap playback stall root cause (2026-02-26 late):**
- Immediately after the successful remap, Plex Web channel clicks appeared to do nothing.
- Reprobed `DVR 218` / channel `2001` using the existing web probe harness:
  - `POST /livetv/dvrs/218/channels/2001/tune` hung ~35s and timed out
  - PlexTuner saw no `/stream/...` request
- File-log root cause in Plex (`Plex Media Server.5.log`):
  - `Subscription: There are 2 active grabs at the end.`
  - `Subscription: Waiting for media grab to start.`
  while `/status/sessions` showed no active playback (hidden stale-grab state).
- Restarted `deploy/plex` (no active sessions present) and re-probed the same channel:
  - `tune` returned `200` in ~`3.2s` again, confirming the guide remap did **not** break tuning.
- Remaining browser probe failure after the restart returned to the prior known Plex-side web packaging path (`dash_init_404`), not the guide/tab issue.

**Packaging + docs productization pass (2026-02-26 late):**
- Added cross-platform tester package builder:
  - `scripts/build-test-packages.sh`
  - builds archives + checksums under `dist/test-packages/<version>/`
  - default matrix includes Linux/macOS/Windows (`amd64/arm64`, plus Linux `armv7`)
- Added packaging/testing docs:
  - `docs/how-to/package-test-builds.md`
  - `docs/reference/testing-and-supervisor-config.md`
  - linked from `README.md`, `docs/index.md`, `docs/how-to/index.md`, `docs/reference/index.md`
- Added build-gating/stubs so cross-platform packaging compiles:
  - `internal/vodfs` marked Linux-only + non-Linux stub (`Mount` returns unsupported)
  - `internal/hdhomerun` package marked `!windows` + Windows stub server (HDHR network mode unsupported on Windows test builds)
- Smoke-validated package generation on a subset matrix:
  - `linux/amd64`, `darwin/arm64`, `windows/amd64`

**Productization follow-up polish (2026-02-26 late):**
- Added staged tester handoff bundle builder:
  - `scripts/build-tester-release.sh`
  - produces `dist/test-releases/<version>/` with `packages/`, `examples/`, `docs/`, `manifest.json`, and `TESTER-README.txt`
- Added tester handoff checklist:
  - `docs/how-to/tester-handoff-checklist.md`
- Added Plex hidden active-grab recovery helper + runbook:
  - `scripts/plex-hidden-grab-recover.sh` (detects hidden-grab log signature + checks `/status/sessions` before optional restart)
  - `docs/runbooks/plex-hidden-live-grab-recovery.md`
- Re-enabled real Windows HDHR network mode code path (removed temporary Windows HDHR stub):
  - `internal/hdhomerun` package now compiles on Windows/macOS/Linux
  - Windows smoke under `wine` shows real HDHR startup path is active (WinSock errors under `wine` are environment-related, not stub behavior)
- `VODFS` remains Linux-only (non-Linux stub kept intentionally).
- Added fuller reference + CI automation for tester bundles:
  - `docs/reference/cli-and-env-reference.md` (commands, flags, key envs including supervisor/reaper/guide-offset knobs)
  - `.github/workflows/tester-bundles.yml` (manual/tag-triggered tester bundle build + artifact upload)

**Docs completeness follow-up (2026-02-26 late):**
- Added a dedicated Plex-side lifecycle/API reference doc for Live TV & DVR manipulations:
  - `docs/reference/plex-dvr-lifecycle-and-api.md`
- Covers wizard-equivalent HDHR API flow, injected DVR lifecycle, remove/cleanup, guide reload + channelmap activation, and Plex UI/backend metadata gotchas (device-centric UI, provider drift, stale client cache, hidden grabs).
- Linked from `docs/reference/index.md` so future agents/users have one place for "wizard / inject / remove / refresh / EPG shenanigans" instead of scattered notes.
