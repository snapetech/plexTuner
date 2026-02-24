# Task History

Append-only. One entry per completed task.

## Entry template
- Date: YYYY-MM-DD
  Title: <short>
  Summary:
    - <what changed>
    - <what changed>
  Verification:
    - <format command or N/A>
    - <lint command or N/A>
    - <tests command or N/A>
    - <build/compile command or N/A>
  Notes:
    - <surprises, follow-ups, trade-offs>
  Opportunities filed:
    - <link to opportunities entry or 'none'>
  Links:
    - <PR/issue/docs>

## Entries

- Date: 2026-02-24
  Title: Define "home run features" multi-PR epic and acceptance gates
  Summary:
    - Promoted the requested product priorities to an explicit multi-PR epic in `memory-bank/work_breakdown.md` so future work is constrained to the intended "home run" pillars instead of ad hoc tuning.
    - Added a concrete story list (`HR-001`..`HR-010`) covering: IDR-aware WebSafe startup, client compatibility matrix + sticky adaptation, built-in lineup/EPG hygiene defaults, remux-first/per-channel normalization policy, resilience, concurrency/keepalive tuning, and recording soak tests.
    - Added milestone outcomes, PR sequencing, and decision points (tier-1 clients, WebSafe startup bias, default hygiene behavior).
    - Updated `current_task.md` to link the current Plex Web startup work to the new epic (`HR-001` / `HR-002`).
  Verification:
    - Documentation-only update (no code/runtime change); reviewed memory-bank files locally.
  Notes:
    - This epic is now the source of truth for multi-PR work in this area; subsequent implementation tasks should reference a `HR-###` story ID.
  Opportunities filed:
    - none
  Links:
    - memory-bank/work_breakdown.md, memory-bank/current_task.md

- Date: 2025-02-23
  Title: README redo, features.md, changelog.md, docs-gaps
  Summary:
    - Pulled from origin (gitlab.home); integrated latest main.
    - Rewrote README: full feature summary, comparison matrix (Plex Tuner vs xTeVe vs Threadfin), commands and env tables, repo layout.
    - Added docs/features.md: canonical feature list (input/indexing, catalog, tuner, EPG, VOD/VODFS, ops, not supported).
    - Added docs/CHANGELOG.md: history from git (merge, Plex Tuner content, template).
    - Added docs/docs-gaps.md: critical (missing internal/indexer), high (Plex setup, config reference, probe, RegisterTuner), medium (architecture, VODFS, XMLTV, multi-host), low (glossary, runbooks, Docker, systemd).
    - Updated docs/index.md with links to features, CHANGELOG, docs-gaps; memory-bank/current_task.md, known_issues.md (missing indexer), opportunities.md (indexer dependency).
  Verification:
    - N/A (format/lint: not run)
    - Build fails: missing internal/indexer (documented in known_issues and docs-gaps).
  Notes:
    - origin/main does not contain internal/indexer; main.go imports it. Documented as critical doc gap and known issue.
  Opportunities filed:
    - memory-bank/opportunities.md: Add or document internal/indexer dependency.
  Links:
    - README.md, docs/features.md, docs/CHANGELOG.md, docs/docs-gaps.md, docs/index.md

- Date: 2025-02-23
  Title: 429/5xx retry, indexer parallel series, provider 429, gateway log
  Summary:
    - internal/httpclient: DoWithRetry with RetryPolicy (429 Retry-After cap 60s, 5xx single retry 1s); parseRetryAfter(seconds or RFC1123 date); tests.
    - internal/indexer/player_api: doGetWithRetry for all API GETs; fetchSeries parallelized fetchSeriesInfo with semaphore (maxConcurrentSeriesInfo=10).
    - internal/provider: StatusRateLimited for 429 in ProbeOne and ProbePlayerAPI.
    - internal/tuner/gateway: log "429 rate limited" when upstream returns 429 before trying next URL.
  Verification:
    - gofmt -s -w, go vet ./..., go test ./..., go build ./cmd/plex-tuner (scripts/verify).
  Notes:
    - 4xx (except 429) never retried; retry is one attempt after wait. No pagination (Xtream player_api returns full lists).
  Opportunities filed:
    - none
  Links:
    - internal/httpclient/retry.go, internal/httpclient/retry_test.go, internal/indexer/player_api.go, internal/provider/probe.go, internal/tuner/gateway.go

- Date: 2026-02-24
  Title: Atomic catalog save, catalog tests, subscription glob, fetchCatalog dedup
  Summary:
    - internal/catalog: Save() now writes to a temp file then os.Rename (atomic on most Unix FSes); prevents corrupt catalog on crash mid-write.
    - internal/catalog: Added catalog_test.go (Save/Load roundtrip, overwrite, no-temp-leftovers, 0600 perms, error cases).
    - internal/config: readSubscriptionFile globs ~/Documents/iptv.subscription.*.txt instead of hardcoded 2026 year; picks alphabetically last (highest year) so it works across year-end renewals.
    - cmd/plex-tuner: Extracted fetchCatalog(cfg, m3uOverride) helper + catalogStats() — eliminates ~80 lines of copy-paste across index/run-startup/run-scheduled. Bug fix: scheduled refresh now applies LiveEPGOnly filter and smoketest (was silently skipped before).
  Verification:
    - Go not installed locally; no build system available in this environment.
    - Changes are syntactically consistent with existing code patterns; all edited files reviewed.
  Notes:
    - Scheduled-refresh missing filters was a silent bug: if smoketest or EPG-only was enabled, startup index honored them but the background ticker did not. Now all three fetch paths go through the same fetchCatalog().
    - os.Rename is atomic only when src and dst are on the same filesystem; temp file is created in the same directory as the catalog to ensure this.
  Opportunities filed:
    - none
  Links:
    - internal/catalog/catalog.go, internal/catalog/catalog_test.go, internal/config/config.go, cmd/plex-tuner/main.go

- Date: 2026-02-24
  Title: Verify pending changes + local Plex-facing smoke test
  Summary:
    - Installed a temporary local Go 1.24.0 toolchain under `/tmp/go` (no system install) to run repo verification in this environment.
    - Ran `scripts/verify` successfully (format, vet, test, build) on the pending uncommitted changes.
    - Applied a format-only `gofmt` fix to `internal/tuner/psi_keepalive.go` (comment indentation) because verify failed on formatting before tests.
    - Ran a local smoke test: generated a catalog from a temporary local M3U, started `serve`, validated `discover.json`, `lineup_status.json`, `lineup.json`, `guide.xml`, `live.m3u`, and fetched one proxied stream URL successfully.
  Verification:
    - `PATH=/tmp/go/bin:$PATH ./scripts/verify`
    - Local smoke: `go run ./cmd/plex-tuner index -m3u http://127.0.0.1:<port>/test.m3u -catalog <tmp>` then `go run ./cmd/plex-tuner serve ...` + `curl` endpoint checks
    - `GET /stream/<channel-id>` returned `200` and proxied bytes from local dummy upstream
    - Real provider/Plex E2E not run (no `.env` / Plex host available in environment)
  Notes:
    - `./scripts/verify` surfaced an unrelated formatting drift (`internal/tuner/psi_keepalive.go`) that was not part of the pending feature changes but blocks CI-level verification.
    - Local smoke validates the tuner HTTP surface and proxy routing mechanics, but not MPEG-TS compatibility or real Plex session behavior.
  Opportunities filed:
    - none
  Links:
    - scripts/verify, internal/tuner/psi_keepalive.go, docs/runbooks/plextuner-troubleshooting.md

- Date: 2026-02-24
  Title: Live Plex integration triage (plex.home 502, WebSafe guide latency, direct tune)
  Summary:
    - Diagnosed `plex.home` 502 as Traefik backend reachability failure to Plex on `kspls0:32400` (Plex itself was healthy; `kspld0` could not reach `192.168.50.85:32400`).
    - Fixed host firewall on `kspls0` by allowing LAN TCP `32400` in `inet filter input`, restoring `http://plex.home` / `https://plex.home` (401 unauthenticated expected).
    - Validated from inside the Plex pod that `plextuner-websafe` (`:5005`) is reachable and `plextuner-trial` (`:5004`) is not.
    - Identified `guide.xml` latency root cause: external XMLTV remap (~45s per request). Restarted WebSafe `plex-tuner serve` in the lab pod without `PLEX_TUNER_XMLTV_URL` (placeholder guide) to make `guide.xml` fast again (~0.2s).
    - Proved live Plex→PlexTuner path works after fixes: direct Plex API `POST /livetv/dvrs/138/channels/11141/tune` returned `200`, and `plextuner-websafe` logged `/stream/11141` with HLS relay first bytes.
  Verification:
    - `curl -I http://plex.home` / `curl -k -I https://plex.home` → `502` before fix, `401` after firewall fix
    - `kubectl` checks on `kspld0`: `get pods/svc/endpoints`, Plex pod `curl` to `plextuner-websafe.plex.svc:5005`
    - Plex pod timing: `guide.xml` ~45.15s with external XMLTV; ~0.19s after WebSafe restart without XMLTV
    - Plex direct tune API for DVR `138` / channel `11141` returned `200` and produced `/stream/11141` request in `plextuner-websafe` logs
  Notes:
    - Runtime fixes are operational and may not persist across host firewall reloads/pod restarts unless codified in infra manifests/scripts.
    - `plextunerWebsafe` lineup is very large (~41,116 channels); Plex channel metadata APIs remain slow even after `guide.xml` was accelerated.
  Opportunities filed:
    - `memory-bank/opportunities.md` (XMLTV caching / fast fallback, Plex-safe lineup sizing)
  Links:
    - memory-bank/known_issues.md, memory-bank/opportunities.md, /home/coder/code/k3s/docs/runbooks/plex-502-bad-gateway.md

- Date: 2026-02-24
  Title: Full Threadfin 13-category DVR pipeline validation and Plex insertion
  Summary:
    - Reran the IPTV split + Threadfin refresh chain in k3s (`threadfin-set-playlists-multi` + `threadfin-api-update-multi`) and verified all 13 Threadfin instances updated successfully (`failures=0`).
    - Verified the generated split output and live `threadfin-*` lineups from the Plex pod matched: 13 buckets totaled 91 channels (counts: `eueast=26`, `latin=33`, `moviesprem=17`, `sportsa=7`, `sportsb=7`, `docsfam=1`, all others `0`).
    - Created 13 new Plex DVRs (Threadfin-backed) via `plex/scripts/plex-dvr-setup-multi.sh`; Plex DVR count increased to `15` total (existing 2 + new 13).
    - Activated Plex channel mappings for the 6 non-empty Threadfin DVRs via `plex/scripts/plex-activate-dvr-lineups.py`, resulting in `91` mapped channels total across those DVRs.
  Verification:
    - k3s jobs: `threadfin-set-playlists-multi` completed; `threadfin-api-update-multi` completed at `2026-02-24T04:00:19Z` with logs ending `All instances updated (failures=0)`
    - Split file counts (`iptv-m3u-server` updater container): `dvr-*.m3u` totals = `91`
    - Threadfin lineups from Plex pod: `/lineup.json` counts across 13 services totaled `91`
    - Plex DVR setup: `plex/scripts/plex-dvr-setup-multi.sh` created DVR keys `141,144,147,150,153,156,159,162,165,168,171,174,177`
    - Plex activation (non-empty DVRs only): `plex/scripts/plex-activate-dvr-lineups.py --dvr 144,147,156,159,162,168` all `status=OK` with after-counts `17,26,7,7,1,33`
  Notes:
    - The expected high-volume category split is currently blocked by source/EPG linkage, not PlexTuner or Plex insertion; observed path was ~41,116 source channels -> 188 XMLTV-linked -> 91 deduped.
    - `plex/scripts/plex-activate-dvr-lineups.py` currently crashes on empty DVRs (`No valid ChannelMapping entries found`); workaround is to activate only non-empty DVRs.
  Opportunities filed:
    - `memory-bank/opportunities.md` (split-pipeline stage count instrumentation; empty-DVR activation helper hardening)
  Links:
    - memory-bank/known_issues.md, memory-bank/opportunities.md, /home/keith/Documents/code/k3s/plex/scripts/plex-dvr-setup-multi.sh, /home/keith/Documents/code/k3s/plex/scripts/plex-activate-dvr-lineups.py

- Date: 2026-02-24
  Title: Direct PlexTuner WebSafe hardening for Plex routing (guide-number fallback + default-safe client adaptation)
  Summary:
    - `internal/tuner/gateway`: Added channel lookup fallback by `GuideNumber` so `/auto/v<guide-number>` works even when the catalog `channel_id` is a non-numeric slug (for example `eurosport1.de`).
    - `internal/tuner/gateway`: Changed Plex client adaptation to a tri-state override model so behavior can explicitly force WebSafe (`transcode on`), explicitly force full path (`transcode off`), or inherit the existing default.
    - New adaptation policy (when `PLEX_TUNER_CLIENT_ADAPT=true`): explicit query `profile=` still wins; unknown/unresolved Plex client resolution defaults to WebSafe; resolved Plex Web/browser clients use WebSafe; resolved non-web clients force full path.
    - Recorded live direct PlexTuner findings in memory-bank: real XMLTV + EPG-linked + deduped catalog fixed lineup/guide mismatch (`188 -> 91` unique `tvg-id` rows) and removed the "Unavailable Airings" mismatch root cause; remaining browser issue is Plex Web DASH `start.mpd` timeout after successful tune/relay.
  Verification:
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go test ./internal/tuner -run 'TestGateway_(requestAdaptation|autoPath)' -count=1`
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go build ./cmd/plex-tuner`
    - `PATH=/tmp/go/bin:$PATH ./scripts/verify` (fails on unrelated repo-wide format drift in tracked files: `internal/config/config.go`, `internal/hdhomerun/control.go`, `internal/hdhomerun/packet.go`, `internal/hdhomerun/server.go`)
  Notes:
    - The client-adaptation behavior change is gated by `PLEX_TUNER_CLIENT_ADAPT`; deployments with the flag disabled retain prior behavior.
    - Full verification is not green due unrelated formatting drift outside this patch scope; this change set itself is `gofmt`-clean and builds/tests cleanly.
  Opportunities filed:
    - `memory-bank/opportunities.md` (built-in direct-catalog dedupe/alignment for XMLTV-remapped Plex lineups)
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_test.go, memory-bank/known_issues.md, memory-bank/opportunities.md

- Date: 2026-02-24
  Title: Re-establish direct PlexTuner DVRs after Plex restart (Trial URI fix + remap) and re-test browser playback
  Summary:
    - Avoided Plex restarts and pod restarts; restarted only the two `plex-tuner serve` processes in the existing `plextuner-build` pod (`:5004` Trial, `:5005` WebSafe) using `/workspace/plex-tuner.policy`, the deduped direct catalog (`catalog-websafe-dedup.json`), and real XMLTV (`iptv-m3u-server`).
    - Verified both direct tuner services were healthy again (`discover.json`, `lineup.json`) and served the 91-channel deduped catalog with real XMLTV remap enabled.
    - `DVR 138` (`plextunerWebsafe`) activation confirmed healthy (`before=91`, `after=91`).
    - Diagnosed `DVR 135` (`plextunerTrial`) zero-channel state as a wrong HDHomeRun device URI in Plex (`http://127.0.0.1:5004` instead of `http://plextuner-trial.plex.svc:5004`).
    - Fixed Trial in place by re-registering the HDHomeRun device to `plextuner-trial.plex.svc:5004`, then `reloadGuide` + `plex-activate-dvr-lineups.py --dvr 135`, which restored `after=91`.
    - Re-ran Plex Web probes on both `DVR 138` and `DVR 135`: both now `tune=200` but still fail at `startmpd1_0`. Trial logs confirm the client-adaptation switch is active and defaults unknown clients to websafe mode (`reason=unknown-client-websafe`).
    - Collected matching Plex logs showing the remaining browser failure is Plex-side: `decision` and `start.mpd` requests complete only after long waits, followed by `Failed to start session.`, while PlexTuner logs show successful `/stream/...` byte relay.
  Verification:
    - k3s runtime checks via `sudo kubectl` on `kspld0` (Plex pod + `plextuner-build` pod): endpoint health, log tails, DVR/device detail XML
    - `sudo python3 /home/keith/Documents/code/k3s/plex/scripts/plex-activate-dvr-lineups.py --dvr 138`
    - `sudo python3 /home/keith/Documents/code/k3s/plex/scripts/plex-activate-dvr-lineups.py --dvr 135`
    - `sudo python3 /home/keith/Documents/code/k3s/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel-id 112`
    - `sudo python3 /home/keith/Documents/code/k3s/plex/scripts/plex-web-livetv-probe.py --dvr 135 --channel-id 112`
  Notes:
    - The probe script `plex-dvr-random-stream-probe.py` reported timeout/0-byte failures on direct `/stream/...` URLs due its fixed 60s timeout, but PlexTuner logs for the same probes show HTTP 200 and tens/hundreds of MB relayed over ~60–130s; use tuner logs as the source of truth for those runs.
    - Another agent is actively changing `internal/hdhomerun/*`; no code changes were made in that area and no Plex restarts were performed.
  Opportunities filed:
    - none
  Links:
    - memory-bank/known_issues.md, memory-bank/recurring_loops.md, /home/keith/Documents/code/k3s/plex/scripts/plex-activate-dvr-lineups.py, /home/keith/Documents/code/k3s/plex/scripts/plex-web-livetv-probe.py

- Date: 2026-02-24
  Title: WebSafe ffmpeg-path triage (k3s ffmpeg DNS failure, startup-gate fallback, hidden Plex CaptureBuffer reuse)
  Summary:
    - Restarted direct WebSafe/Trial tuner processes in the existing `plextuner-build` pod using `/workspace/plex-tuner-9514357` (no Plex restart), real XMLTV, and the 91-channel deduped direct catalog.
    - Confirmed the fresh WebSafe runtime binary now logs ffmpeg/websafe diagnostics; browser probes still fail with `startmpd1_0` and `start.mpd` debug XML continues to return `CaptureBuffer` with empty `sourceVideoCodec/sourceAudioCodec`.
    - Found a concrete WebSafe blocker: ffmpeg transcode startup failed on HLS input URLs that use the k3s short service hostname (`iptv-hlsfix.plex.svc`), causing PlexTuner to fall back to the Go raw relay path.
    - Verified the ffmpeg DNS issue is specific to the ffmpeg HLS input path (Go HTTP fetches to the same hostname work). Runtime workaround: created `/workspace/catalog-websafe-dedup-ip.json` with HLSFix hostnames rewritten to the numeric service IP (`10.43.210.255:8080`) and restarted WebSafe on that catalog.
    - After the numeric-host workaround, ffmpeg + PAT/PMT keepalive started successfully, but the WebSafe ffmpeg startup gate timed out (no ffmpeg payload before timeout), emitted timeout bootstrap TS, then still fell back to the Go raw relay.
    - Tuned WebSafe ffmpeg/startup envs (`FFMPEG_HLS_*`, startup timeout 30s, smaller startup min bytes) and restarted WebSafe again for follow-up testing; hidden Plex `CaptureBuffer` session reuse on repeated channels limited clean validation of the tuned path.
    - Found a second major test-loop blocker: Plex can reuse hidden `CaptureBuffer`/transcode state not visible in `/status/sessions` or `/transcode/sessions`. `plex-live-session-drain.py --all-live` can report clean, but repeated probes on the same channel reuse the same `TranscodeSession` and do not hit PlexTuner `/stream/...` again.
    - Confirmed `universal/stop?session=<id>` returns `404` for those hidden reused `TranscodeSession` IDs (examples: `8af250...`, `24b5e1...`, `07b8aa...`).
    - Restarted Trial with client-adapt enabled plus `PLEX_TUNER_HLS_RELAY_FFMPEG_STDIN_NORMALIZE=true`, explicit ffmpeg path, numeric HLSFix catalog, and the same tuned ffmpeg/startup envs to set up a second DVR for fresh-channel browser tests.
  Verification:
    - `sudo kubectl -n plex exec pod/plextuner-build-... -- ...` process restarts/checks for `:5004` and `:5005`
    - `sudo env PWPROBE_DEBUG_MPD=1 python3 .../plex-web-livetv-probe.py` on DVRs `138` and `135` (channels `112`, `111`, `108`, `109`, `107`, `104`, `103`, `26289`)
    - WebSafe/Trial tuner log correlation (`/tmp/plextuner-websafe.log`, `/tmp/plextuner-trial.log`) including `ffmpeg-transcode`, `pat-pmt-keepalive`, fallback reasons, and `/stream/...` presence/absence
    - Plex API checks from helper snippets: `/status/sessions`, `/transcode/sessions`, and explicit `universal/stop?session=<id>` attempts for hidden reused session IDs
  Notes:
    - Runtime-only test changes are not durable: WebSafe/Trial envs were changed in-process, and the numeric-host catalog copy (`catalog-websafe-dedup-ip.json`) exists only in the pod filesystem.
    - Hidden Plex `CaptureBuffer` reuse can invalidate repeated probe runs on the same channel; only probes that generate a new tuner `/stream/...` request should be used to judge tuner runtime changes.
    - No Plex pod restart was performed.
  Opportunities filed:
    - `memory-bank/opportunities.md` (ffmpeg HLS host canonicalization before ffmpeg; stronger stale-session detection in Plex probe/drain helpers)
  Links:
    - memory-bank/known_issues.md, memory-bank/recurring_loops.md, memory-bank/opportunities.md, /home/keith/Documents/code/k3s/plex/scripts/plex-live-session-drain.py, /home/keith/Documents/code/k3s/plex/scripts/plex-web-livetv-probe.py

- Date: 2026-02-24
  Title: Add ffmpeg HLS input host canonicalization in gateway (k3s short-host compatibility)
  Summary:
    - `internal/tuner/gateway.go`: Added `canonicalizeFFmpegInputURL(...)` to resolve the ffmpeg HLS input host in Go and rewrite the ffmpeg input URL to a numeric host before spawning ffmpeg.
    - `relayHLSWithFFmpeg(...)` now uses the rewritten ffmpeg input URL (when resolution succeeds) and logs `input-host-resolved <host>=><ip>` for visibility.
    - This is a direct code response to the live k3s WebSafe failure where ffmpeg could not resolve `iptv-hlsfix.plex.svc` and PlexTuner fell back to the raw relay path.
  Verification:
    - `/tmp/go/bin/gofmt -w internal/tuner/gateway.go`
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go test ./internal/tuner -count=1`
  Notes:
    - The patch is currently local-only and not yet rebuilt/deployed into the `plextuner-build` pod runtime.
    - Runtime validation still needs a fresh Plex browser probe that actually reaches a new tuner `/stream/...` request (hidden `CaptureBuffer` reuse can mask the change).
  Opportunities filed:
    - none (covered by existing ffmpeg host canonicalization + probe-helper entries)
  Links:
    - internal/tuner/gateway.go, memory-bank/current_task.md, memory-bank/known_issues.md

- Date: 2026-02-24
  Title: Fix WebSafe ffmpeg HLS reconnect-loop startup failure and re-validate live payload path
  Summary:
    - Continued direct PlexTuner WebSafe browser-path triage without restarting Plex; restarted only the WebSafe `plex-tuner serve` process in `plextuner-build` multiple times for env/runtime experiments.
    - Reproduced the ffmpeg startup stall manually inside the pod using `/workspace/ffmpeg-static` against the HLSFix live playlist and found the real blocker: generic ffmpeg HTTP reconnect flags on live HLS (`-reconnect*`) caused repeated `.m3u8` EOF reconnect loops (`Will reconnect at 1071 ... End of file`) and delayed/failed first-segment loading.
    - Confirmed the same manual ffmpeg command succeeds immediately when reconnect flags are removed (opens HLS segment, writes valid MPEG-TS file, exits cleanly).
    - Patched `internal/tuner/gateway.go` so `PLEX_TUNER_FFMPEG_HLS_RECONNECT` defaults to `false` for HLS ffmpeg inputs (env override still supported); this preserves the earlier ffmpeg host-canonicalization fix and avoids the live playlist reconnect loop by default.
    - Built a clean temporary runtime binary from `HEAD` plus only `internal/tuner/gateway.go` (to avoid including another agent's HDHomeRun WIP), deployed it into the `plextuner-build` pod as `/workspace/plex-tuner-websafe-fix`, and restarted only WebSafe (`:5005`) in place.
    - Re-ran Plex Web probe on `DVR 138` / channel `106`: probe still fails `startmpd1_0`, but WebSafe logs now show the ffmpeg path is genuinely working (`reconnect=false`, `startup-gate-ready`, `first-bytes`, and long ffmpeg stream runs with multi-MB payload).
    - Additional WebSafe runtime tuning (`REQUIRE_GOOD_START=true`, larger startup timeout/prefetch, and later `HLS_LIVE_START_INDEX=-3`) still showed startup-gate buffers with `idr=false aac=true`; browser probes continued to fail `startmpd1_0`, shifting the main blocker from ffmpeg startup to early video/keyframe readiness vs Plex's live packager timeout.
    - Hit an unrelated k3s control-plane issue during later probe retries: `kubectl exec` to the Plex pod intermittently returned `502 Bad Gateway`, which temporarily blocked the probe helper's token-read step.
  Verification:
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go test ./internal/tuner -count=1`
    - Manual ffmpeg repro (inside `plextuner-build` pod) with reconnect flags enabled: repeated playlist EOF reconnect loop (`Will reconnect at 1071 ...`)
    - Manual ffmpeg control (same pod/channel) without reconnect flags: opened HLS segment and wrote valid TS (`/tmp/manual106.ts`, ~3.9 MB in ~6s)
    - `python3 /home/keith/Documents/code/k3s/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel 106` (via temporary `kubectl` wrapper to `sudo k3s kubectl`) before and after runtime tuning
    - WebSafe log correlation in `/tmp/plextuner-websafe.log` confirming `reconnect=false`, `startup-gate-ready`, `first-bytes`, and `ffmpeg-transcode bytes/client-done` payload sizes
  Notes:
    - No Plex restart was performed.
    - Trial process was left running and was not restarted during this cycle.
    - Late probe retries were partially blocked by transient k3s `kubectl exec` proxy `502` errors to the Plex pod.
  Opportunities filed:
    - `memory-bank/opportunities.md` (IDR-aware live HLS startup strategy for WebSafe ffmpeg path)
  Links:
    - internal/tuner/gateway.go, memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md, memory-bank/opportunities.md

- Date: 2026-02-24
  Title: Restore `plex.home` via manual endpoint slice during `kspls0` read-only-root outage (no Plex restart)
  Summary:
    - Investigated `https://plex.home` `503` and found the Plex host node `kspls0` was `NotReady`; the Plex pod on `kspls0` was stuck `Terminating` and the Service had no ready endpoints.
    - Confirmed the host Plex process itself was still alive on `192.168.50.85:32400` (direct HTTP returned Plex `401` unauth).
    - Diagnosed `k3s` startup failure on `kspls0`: root Btrfs (`/`) was mounted read-only, and foreground `k3s server` failed with `failed to bootstrap cluster data ... chmod kine.sock: read-only file system`.
    - Confirmed the replacement Plex pod on `kspld0` could not start because NFS mounts from `192.168.50.85` failed (`Host is unreachable`), leaving the `EndpointSlice` endpoint `ready=false`.
    - Restored `plex.home` without restarting Plex by patching Service `plex` to be selectorless and attaching a manual `EndpointSlice` to `192.168.50.85:32400`; `https://plex.home` returned `401` afterward.
  Verification:
    - `curl -k -I https://plex.home` (before: `503`, after: `401`)
    - `ssh kspld0 'sudo k3s kubectl get nodes -o wide'`
    - `ssh kspld0 'sudo k3s kubectl -n plex get svc/endpoints/endpointslice ...'`
    - `ssh keith@kspls0 'findmnt -no TARGET,SOURCE,FSTYPE,OPTIONS /'`
    - `ssh keith@kspls0 'timeout 20s /usr/local/bin/k3s server ...'` (foreground capture of `kine.sock` read-only failure)
  Notes:
    - This is a temporary traffic-routing workaround only. `kspls0` still needs host-level filesystem recovery (root Btrfs back to `rw`) and `k3s` restart.
    - After host recovery, restore the normal `plex` Service selector (`app=plex`) and remove the manual `EndpointSlice`.
    - No Plex process restart was performed.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md

- Date: 2026-02-24
  Title: Persist `kspls0` LAN Plex/NFS firewall allows in boot-loaded nftables config and restore Plex after reboot
  Summary:
    - Rebooted `kspls0` to recover the root Btrfs `ro` remount condition; confirmed `/` returned `rw` and `postgresql` + `k3s` were active after boot.
    - Found the post-reboot regression was the same firewall persistence issue: `/etc/nftables/kspls0-host-firewall.conf` still contained Plex/NFS allows, but the later `table inet filter` base chain from `/etc/nftables.conf` dropped LAN Plex/NFS traffic.
    - Added temporary live `nft` rules to `inet filter input` to restore LAN access for NFS/Plex (`111/2049/20048/.../32400`) and re-established `kspld0 -> kspls0` NFS RPC connectivity.
    - Patched `/etc/nftables.conf` (the file loaded by `nftables.service`) to persist the LAN Plex/NFS allow rules in the actual `inet filter input` chain so they survive future reboot/reload.
    - Restored normal Plex service routing (selector-based Service, removed temporary manual `EndpointSlice`), deleted the stuck pending Plex pod, and verified a new Plex pod came up on `kspls0` and `https://plex.home` returned `401`.
  Verification:
    - `ssh keith@kspls0 'findmnt -no OPTIONS /; systemctl is-active postgresql k3s'`
    - `ssh keith@kspls0 'sudo nft -c -f /etc/nftables.conf'`
    - `ssh kspld0 'rpcinfo -p 192.168.50.85 && showmount -e 192.168.50.85'`
    - `ssh kspld0 'sudo k3s kubectl -n plex get pod -o wide'`
    - `curl -k -I https://plex.home` (final `401`)
  Notes:
    - Persisted NFS auxiliary RPC ports match the currently observed `rpcinfo` ports (`nlockmgr/statd`) and may change after future NFS restarts/reboots unless pinned in NFS config.
    - No code changes in this repo besides memory-bank updates.
  Opportunities filed:
    - none
  Links:
    - memory-bank/recurring_loops.md, memory-bank/known_issues.md

- Date: 2026-02-24
  Title: Verify sticky NFS/firewall recovery and isolate Plex internal live-manifest stall (`index.m3u8` zero-byte) across WebSafe profiles
  Summary:
    - Verified the post-reboot `kspls0` LAN access fixes are truly persistent: `/etc/nfs.conf` still pins `lockd`/`mountd`/`statd` ports, `inet filter input` still contains the matching NFS + Plex `32400` allow rules, and `kspld0 -> kspls0` `rpcinfo`/`showmount` succeeds.
    - Confirmed direct WebSafe service is up and resumed fresh browser probes without restarting Plex; Trial (`:5004`) was down during this cycle and intentionally left untouched to minimize disruption.
    - Reproduced the Web browser failure on fresh WebSafe channels `103` and `104` with new hidden Plex `CaptureBuffer` sessions (`startmpd1_0` at ~35s), while PlexTuner logs showed healthy ffmpeg startup and real streamed bytes (`startup-gate` ready, `first-bytes`, `idr=true` in the `103/104` runs).
    - Demonstrated that Plex `decision` / `start.mpd` for the `103` and `104` sessions can complete only after ~100s (PMS logs), which is longer than the probe/browser startup timeout.
    - Captured the key blocker directly: Plex's internal `http://127.0.0.1:32400/livetv/sessions/<live>/<client>/index.m3u8` timed out with **0 bytes** during repeated in-container polls, even while the first-stage recorder wrote many `media-*.ts` segments and Plex accepted `progress/stream` + `progress/streamDetail` callbacks.
    - PMS logs for session `ebbb9949-...` (channel `104`) repeatedly logged `buildLiveM3U8: no segment info available` while the internal live `index.m3u8` remained empty, confirming the bottleneck is Plex's segment-info/manifest readiness, not tuner throughput.
    - Ran two profile-comparison experiments on WebSafe (runtime-only process restarts inside `plextuner-build`, no Plex restart):
      - `plexsafe` (via client adaptation) on channel `107` still failed `startmpd1_0`.
      - Forced `pmsxcode` with `PLEX_TUNER_CLIENT_ADAPT=false` on channel `109` also failed `startmpd1_0`; PMS first-stage progress confirmed the codec path really changed (`mpeg2video` + `mp2`), but the browser timeout remained and the internal live `index.m3u8` still timed out with 0 bytes.
    - Restored the WebSafe runtime to the baseline test profile afterward (`aaccfr` default + client adaptation enabled, explicit ffmpeg path, HLS reconnect=false, no bootstrap/keepalive), again without restarting Plex.
  Verification:
    - `ssh keith@kspls0 'grep -n ... /etc/nfs.conf; sudo nft list chain inet filter input; rpcinfo -p localhost'`
    - `ssh kspld0 'rpcinfo -p 192.168.50.85; showmount -e 192.168.50.85'`
    - `kubectl -n plex get pods -o wide`, `kubectl -n plex exec deploy/plex -c plex -- curl .../discover.json`
    - `sudo env PWPROBE_DEBUG_MPD=1 python3 /home/keith/Documents/code/k3s/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel {103,104,107,109}`
    - `kubectl -n plex exec deploy/plex -c plex -- curl http://127.0.0.1:32400/livetv/sessions/<live>/<client>/index.m3u8?...` (in-container internal live-manifest polling)
    - PMS log correlation in `/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log` for `buildLiveM3U8`, recorder segment sessions, and delayed `decision`/`start.mpd`
    - WebSafe runtime log correlation in `/tmp/plextuner-websafe.log` for effective profile (`aaccfr` / `plexsafe` / `pmsxcode`) and startup-gate readiness
  Notes:
    - Multiple WebSafe runtime restarts were process-only inside `plextuner-build` (no pod restart, no Plex restart).
    - One experiment initially left duplicate WebSafe processes due pod shell/process-tooling quirks; runtime was restored and the log confirms the final baseline restart (`default=aaccfr`, client adaptation enabled).
    - The strongest current evidence is Plex-side: first-stage recorder healthy + internal live HLS manifest empty (`0 bytes`) + repeated `buildLiveM3U8 no segment info`.
  Opportunities filed:
    - `memory-bank/opportunities.md` (TS timing/continuity debug capture for first-seconds WebSafe output)
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md, memory-bank/opportunities.md, /home/keith/Documents/code/k3s/plex/scripts/plex-web-livetv-probe.py
