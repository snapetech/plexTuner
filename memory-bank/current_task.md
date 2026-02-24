# Current task

<!-- Update at session start and when focus changes. -->

**Goal:** Continue live Plex integration testing on the direct PlexTuner path (no Threadfin) without restarting Plex, while preserving other agents' work. Direct Trial/WebSafe tuners remain re-established and mapped at 91 channels, and the WebSafe ffmpeg path now starts/streams real payload again (DNS + HLS reconnect-loop issues fixed), but Plex Web browser playback is still blocked by Plex's internal DASH packaging (`start.mpd` timeout / `CaptureBuffer`). New focus is the remaining startup/packager gap in Plex itself: first-stage Plex recorder sessions write many TS segment files and report stream metadata, but Plex's internal live HLS endpoint (`/livetv/sessions/<live>/<client>/index.m3u8`) can return zero bytes for minutes while `buildLiveM3U8` repeatedly logs `no segment info available`. This reproduces across WebSafe output profiles (`aaccfr`, `plexsafe`, and forced `pmsxcode`) and is now the leading blocker for `HR-001` / `HR-002`.

**Scope:** In: live validation/triage only (k3s + Plex API/logs), minimal process restarts inside `plextuner-build` pod, runtime-only tuner env/catalog experiments (WebSafe/Trial), and documenting operational findings (ffmpeg DNS/startup-gate behavior, hidden Plex capture-session reuse). Out: Plex pod restarts, Threadfin pipeline changes, infra/firewall persistence, and unrelated code changes (another agent is modifying `internal/hdhomerun/*`).

**Last updated:** 2026-02-24

---

## Assumptions & questions (only if uncertainty matters)
Assumptions (safe defaults you are proceeding with):
- Local environment may not have Go installed; OK to use a temporary local Go toolchain (non-system install) only for verification.
- k3s/Plex troubleshooting changes on remote hosts may be temporary runtime fixes unless later codified in infra manifests or host firewall config.

Questions (ONLY if blocked or high-risk ambiguity):
- Q: None currently blocking for this patch-sized change.
- Q: None currently blocking. User confirmed initial tier-1 client matrix for `HR-003`: LG webOS, Plex Web (Firefox/Chrome), iPhone iOS, and NVIDIA Shield TV (Android TV/Google target coverage).

## Opportunity radar (don't derail)
- If you notice out-of-scope improvements, record them in `memory-bank/opportunities.md` and raise to the user in your summary.

## Self-check (quality bar — fill before claiming done)
- **Correctness:** ✅ WebSafe (`:5005`) is currently running in the existing `plextuner-build` pod from `/workspace/plex-tuner-websafe-fix` (clean build of `HEAD + internal/tuner/gateway.go`) with real XMLTV and the 93-channel alias test catalog (`catalog-websafe-dedup-alias112.json`). Runtime is restored to the baseline direct-test profile (`aaccfr` default + client adaptation enabled) with explicit ffmpeg path, HLS host canonicalization, and HLS reconnect-default fix (`PLEX_TUNER_FFMPEG_HLS_RECONNECT=false`). Trial (`:5004`) was observed down during this cycle and was intentionally left untouched while focusing on the WebSafe browser blocker.
- **Tests:** ⚠️ Fresh WebSafe browser probes still fail (`startmpd1_0`) on channels `103`, `104`, `107`, and `109`, but triage advanced materially: (1) the `103` and `104` sessions proved Plex eventually returns `decision` and `start.mpd` after ~100s (probe times out at ~35s first); (2) for the same sessions, Plex's first-stage recorder wrote many `media-*.ts` segments and accepted `progress/stream` + `progress/streamDetail` callbacks, yet internal `GET /livetv/sessions/<live>/<client>/index.m3u8` returned **0 bytes** during repeated in-container polls and PMS logged repeated `buildLiveM3U8: no segment info available`; (3) changing WebSafe output profile did not fix the startup window (`plexsafe` via client adaptation and forced `pmsxcode` with `client_adapt=false` both reproduced the ~35s Web timeout); (4) the forced `pmsxcode` run confirms the first-stage codec/path actually changed (`mpeg2video` + `mp2` in Plex progress streamDetail) while the browser timeout behavior stayed the same. Local code verification from earlier patch cycle remains green: `go test ./internal/tuner`.
- **Risk:** med-high (runtime state in Plex/k3s can drift after Plex restarts, hidden Plex capture/transcode reuse can invalidate probe results, and current tuner env/catalog experiments are temporary)
- **Ops note (2026-02-24 side quest):** `kspls0` was rebooted and recovered (root Btrfs back `rw`, `k3s` active, node `Ready`), normal Plex Service routing was restored, and `https://plex.home` is back (`401` unauth expected). The persistent LAN nftables allow rules in `/etc/nftables.conf` plus pinned NFS auxiliary RPC ports in `/etc/nfs.conf` survived reboot; `kspld0 -> kspls0` NFS RPC (`rpcinfo`/`showmount`) is working.
- **Performance impact:** direct guide path remains fast enough at 91 channels (~1s `guide.xml`); the current browser blocker is now a startup/packager-readiness issue (early video/IDR timing + Plex capture behavior), not raw throughput or ffmpeg startup.
- **Security impact:** none (token used in-container only; not printed)

## Decisions (single source of truth)
- If you make a **durable** decision, promote it to **ADR** (`docs/adr/`) or **memory-bank**.
- If you're **unsure whether it's durable**, don't promote yet — note it in Assumptions.

## Docs (done = doc update when behavior changes)
- If you changed **behavior, interfaces, or config:** update or create **one** doc in `docs/`; add cross-links. Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md). (Memory-bank updates are in scope for this patch; broader docs can follow if this behavior is promoted.)
- If you **noticed doc gaps** but it's out of scope: file in `memory-bank/opportunities.md`.
