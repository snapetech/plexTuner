# Opportunities (Continuous Improvement Backlog)

This is a lightweight backlog for improvements discovered during other work.
It exists to encourage quality gains without derailing the current task.

## Rules
- Prefer evidence: link to code, test output, perf numbers, or a specific risk.
- Do NOT expand scope mid-task unless it is small, low-risk, and clearly aligned.
- If an item needs a product/UX decision or significant effort, raise it to the user.

## Entry template
- Date: YYYY-MM-DD
  Category: security | performance | reliability | maintainability | operability | other
  Title: <short>
  Context: <where you noticed it>
  Why it matters: <impact + who it affects>
  Evidence: <link/snippet/metric>
  Suggested fix: <concrete next step>
  Risk/Scope: <low/med/high> | <fits current scope? yes/no>
  User decision needed?: yes/no
  If yes: 2–3 options + recommended default + what you will do if no answer

## Entries

- Date: 2026-02-26
  Category: reliability
  Title: Add in-app/operator command to detect and clear Plex hidden Live TV "active grabs" without full Plex restart
  Context: Post guide-number-offset remap validation for 15 DVRs; Plex Web clicks did nothing until Plex restart.
  Why it matters: After large guide/channel remaps, Plex can wedge tunes (`Waiting for media grab to start`) even when `/status/sessions` is empty. Restarting Plex works but is heavy-handed and interrupts all clients.
  Evidence: `Plex Media Server.5.log` during `POST /livetv/dvrs/218/channels/2001/tune` showed `There are 2 active grabs at the end.` and `Waiting for media grab to start.`; same channel tuned normally after `deploy/plex` restart.
  Suggested fix: Investigate PMS APIs or DB/state paths for stale grab cleanup (or add an operator helper that detects the log pattern and recommends/executes a targeted restart only when no active sessions exist).
  Risk/Scope: med | fits current scope: no (Plex internals / ops tooling)
  User decision needed?: no

- Date: 2026-02-26
  Category: maintainability
  Title: Add official command/config reference pages for new envs and supervisor mode (beyond testing doc)
  Context: Added `docs/reference/testing-and-supervisor-config.md` as a practical tester reference.
  Why it matters: Recent features (`supervise`, guide offsets, reaper, XMLTV language normalization, HDHR shaping) now exist but are not yet in a canonical exhaustive CLI/env reference.
  Evidence: `docs/reference/index.md` was effectively empty before this session; README lists only core envs.
  Suggested fix: Add a proper generated or hand-maintained CLI + env reference page (all commands/flags/envs), and cross-link from README and `docs/index.md`.
  Risk/Scope: med | fits current scope: no (docs expansion)
  User decision needed?: no

- Date: 2026-02-25
  Category: reliability
  Title: Postvalidate CDN rate-limiting causes false-positive stream drops
  Context: ../k3s/plex/iptv-m3u-server-split.yaml POSTVALIDATE_WORKERS=12, sequential DVR files
  Why it matters: 12 concurrent ffprobe workers testing streams sequentially per DVR exhaust CDN capacity by mid-run. newsus/sportsb/moviesprem/ukie/eusouth all dropped to 0 channels (100% false-fail). bcastus passed 136/136 (ran first, CDN not yet limited).
  Evidence: postvalidate run 2026-02-25: bcastus=136/136 (no drops), newsus=0/44, sportsb=0/281, moviesprem=0/253, ukie=0/112, eusouth=0/52 — all "Connection refused" in that order.
  Suggested fix: (a) Reduce POSTVALIDATE_WORKERS to 3-4 with random jitter, (b) add per-host rate limit delay, (c) skip postvalidate for EU buckets if cluster is US-based (geo-block), or (d) disable postvalidate entirely and rely on EPG prune + FALLBACK_RUN_GUARD.
  Risk/Scope: low code change | user decision needed on approach
  User decision needed?: yes

- Date: 2026-02-25
  Category: reliability
  Title: plex-reload-guides-batched.py uses wget (not in Plex container)
  Context: k3s/plex/scripts/plex-reload-guides-batched.py was fixed wget→curl this session but the configmap version may still use wget if re-applied
  Why it matters: Script will fail if Plex container only has curl
  Suggested fix: File is local-only (not a configmap); already fixed this session. No action needed unless file is re-applied from a pre-fix copy.
  Risk/Scope: low | fits current scope: done
  User decision needed?: no

- Date: 2026-02-24
  Category: security
  Title: Replace committed provider credentials in `k8s/plextuner-hdhr-test.yaml`
  Context: While adding one-shot deploy automation, the tracked test manifest currently contains concrete provider-looking values in the ConfigMap.
  Why it matters: Even if test-only, committed credentials/URLs increase secret leakage risk and normalize unsafe workflow.
  Evidence: `k8s/plextuner-hdhr-test.yaml` ConfigMap `plextuner-test-env` has explicit `PLEX_TUNER_PROVIDER_*` and `PLEX_TUNER_M3U_URL` values.
  Suggested fix: Replace with placeholders (or sample values), keep one-shot script/Secret flow as the recommended path, and rotate any real credentials if they were valid.
  Risk/Scope: low | fits current scope: no (logged only).
  User decision needed?: no

- Date: 2025-02-23
  Category: maintainability
  Title: Add or document internal/indexer dependency
  Context: README/docs pass; build fails without indexer.
  Why it matters: New clones cannot build; unclear whether indexer is external or missing.
  Evidence: `go build ./cmd/plex-tuner` → "no required module provides package .../internal/indexer".
  Suggested fix: Either add the indexer package to the repo (from another branch/repo) or document the dependency and build steps in README/reference.
  Risk/Scope: low | fits current scope: no (documented in docs-gaps).
  User decision needed?: yes (whether indexer lives in-repo or separate).

- Date: 2026-02-24
  Category: performance
  Title: Cache remapped external XMLTV for `/guide.xml` (and fast-fallback on timeout)
  Context: Live Plex integration testing against `plextuner-websafe` in k3s (`plex.home`).
  Why it matters: `guide.xml` is fetched by Plex metadata flows; external XMLTV remap currently runs per request and took ~45s, which stalls Plex DVR channel metadata APIs.
  Evidence: `internal/tuner/xmltv.go` fetches external XMLTV every request (no cache); live measurement from Plex pod: `guide.xml` ~45.15s with external XMLTV enabled, ~0.19s with placeholder guide (XMLTV disabled).
  Suggested fix: Add in-memory/on-disk XMLTV cache with TTL + stale-while-revalidate; on timeout/error serve last good cached remap immediately, otherwise placeholder as fallback.
  Risk/Scope: med | fits current scope: no (code + behavior design)
  User decision needed?: yes (cache TTL/size and whether stale guide is preferred over placeholder on source failures).

- Date: 2026-02-24
  Category: operability
  Title: Add guidance and tooling for Plex-safe lineup sizing (WebSafe had 41k+ channels)
  Context: Live Plex API testing on DVR `138` (`plextunerWebsafe`) in k3s.
  Why it matters: Plex could tune a known channel, but channel metadata enumeration (`.../lineups/dvr/channels`) was too slow with ~41,116 channels, making mapping/diagnostics painful.
  Evidence: `lineup.json` ~5.3 MB / ~41,116 channels; Plex `tune` for channel `11141` succeeded, but channel list API did not return within 15s during tests.
  Suggested fix: Document and/or add tooling for pre-serve channel pruning (EPG-linked only, category includes/excludes, max-channel cap) and provide recommended profiles for Plex.
  Risk/Scope: med | fits current scope: no (behavior/config product choices)
  User decision needed?: yes (preferred pruning strategy for your Plex setup; default recommendation: EPG-linked + curated categories).

- Date: 2026-02-24
  Category: operability
  Title: Instrument source->EPG coverage in the 13-category split pipeline
  Context: Live Threadfin/Plex multi-DVR validation in k3s after rerunning the IPTV split + Threadfin refresh jobs.
  Why it matters: The pipeline can "work" while producing unexpectedly tiny outputs; without counts at each stage it looks like Threadfin/Plex is broken when the real constraint is feed/XMLTV linkage.
  Evidence: Observed ~41,116 source channels -> 188 EPG-linked (`tvg-id` found in XMLTV) -> 91 deduped -> 91 total across 13 `dvr-*.m3u`; many Threadfin buckets and Plex DVRs were empty by design.
  Suggested fix: Log and persist stage counts (`all`, `with_tvg_id`, `in_xmltv`, `deduped`, per-bucket totals) in the split/update jobs and optionally warn/fail if totals drop below a configurable threshold.
  Risk/Scope: low | fits current scope: no (k3s/job tooling change, not PlexTuner code)
  User decision needed?: no

- Date: 2026-02-24
  Category: reliability
  Title: Make `plex-activate-dvr-lineups.py` skip empty DVRs instead of crashing
  Context: Activating newly created Threadfin DVRs in Plex after 13-way split refresh.
  Why it matters: Empty category buckets are expected when source/EPG coverage is sparse, but the activation helper aborts on the first empty DVR and prevents activation of later non-empty DVRs.
  Evidence: `ValueError: No valid ChannelMapping entries found` on DVR `141` (`threadfin-newsus`, 0 channels); rerunning the script only for non-empty DVRs succeeded and mapped all 91 channels.
  Suggested fix: Catch the empty-mapping case in `plex/scripts/plex-activate-dvr-lineups.py`, log `SKIP_EMPTY`, and continue processing remaining DVRs.
  Risk/Scope: low | fits current scope: no (external k3s repo script)
  User decision needed?: no

- Date: 2026-02-24
  Category: reliability
  Title: Add built-in direct-catalog dedupe/alignment for XMLTV-remapped Plex lineups
  Context: Direct PlexTuner (no Threadfin) WebSafe testing with real XMLTV on `plextuner-websafe`.
  Why it matters: Plex guide UX can show many "Unavailable Airings" even when streaming works if `lineup.json` contains duplicate `tvg-id` rows while XMLTV remap dedupes guide channels/programmes by `tvg-id`.
  Evidence: Live direct WebSafe test observed `188` lineup channels vs `91` guide channels after XMLTV remap; deduping `catalog.live_channels` by `tvg_id` before `serve` fixed the mismatch (`91/91`) and removed the guide/linkage mismatch.
  Suggested fix: Add a built-in catalog/live-channel dedupe option (e.g., by `tvg-id`) and emit counts/logs for dropped duplicates when XMLTV remap is enabled, so direct Plex mode stays lineup/guide-aligned without an external preprocessing step.
  Risk/Scope: med | fits current scope: no (new config/behavior choice)
  User decision needed?: yes (default behavior for duplicates: keep first, prefer highest-priority source, or make it opt-in; recommended default: opt-in first, then consider enabling automatically when XMLTV remap is active).

- Date: 2026-02-24
  Category: reliability
  Title: Canonicalize/resolve HLS input hosts before ffmpeg (k3s short service hostname compatibility)
  Context: Live WebSafe ffmpeg testing in `plextuner-build` pod with `PLEX_TUNER_FFMPEG_PATH=/workspace/ffmpeg-static`.
  Why it matters: ffmpeg can fail on Kubernetes short service hostnames (for example `iptv-hlsfix.plex.svc`) even though Go HTTP fetches work, causing PlexTuner to silently fall back to the raw relay path that Plex Web cannot parse reliably.
  Evidence: WebSafe logs showed `ffmpeg-transcode failed (falling back to go relay)` with stderr `Failed to resolve hostname iptv-hlsfix.plex.svc`; replacing the hostname with the numeric service IP in a catalog copy allowed ffmpeg to start.
  Suggested fix: Before invoking ffmpeg on HLS URLs, canonicalize/resolve the URL host in Go (for example resolve to IP or rewrite k8s `.svc` hosts to a form ffmpeg can resolve) and log the rewritten input host.
  Risk/Scope: med | fits current scope: no (runtime behavior change + tests)
  User decision needed?: no

- Date: 2026-02-24
  Category: operability
  Title: Strengthen Plex test helpers to detect/clear hidden Live TV `CaptureBuffer` state
  Context: Browser probe iteration on direct WebSafe/Trial DVRs while tuning ffmpeg startup behavior.
  Why it matters: Repeated probes on the same channel can reuse hidden Plex `CaptureBuffer`/transcode state that is not visible in `/status/sessions` or `/transcode/sessions`, causing false test results and making tuner changes appear ineffective.
  Evidence: `start.mpd` debug XML repeatedly returned the same `TranscodeSession` key for a channel, with no new `/stream/...` request in PlexTuner logs, while `/status/sessions` and `/transcode/sessions` both reported `size=0`; `universal/stop?session=<id>` returned `404`.
  Suggested fix: Extend the probe/drain tooling to detect stale-session reuse (same transcode key + no new tuner stream request), optionally rotate to a fresh channel automatically, and record a "stale probe" outcome instead of treating it as a tuner regression.
  Risk/Scope: low-med | fits current scope: no (external k3s helper scripts)
  User decision needed?: no

- Date: 2026-02-24
  Category: reliability
  Title: Add IDR-aware live HLS startup strategy for WebSafe ffmpeg path
  Context: Direct WebSafe Plex Web probe triage after fixing ffmpeg DNS and HLS reconnect-loop behavior.
  Why it matters: Plex Web `start.mpd` still times out even when WebSafe ffmpeg now streams healthy TS payload for >1 minute; startup-gate diagnostics show audio but no early video IDR (`idr=false`) in the initial buffered output.
  Evidence: Live WebSafe logs with `reconnect=false` show `startup-gate-ready`, `first-bytes`, and `ffmpeg-transcode bytes=11275676` / `client-done bytes=18996512`, but probes still fail `startmpd1_0`; startup buffers remained `idr=false aac=true` at both `32768` and `524288` bytes.
  Suggested fix: Add an IDR-aware startup policy for HLS transcode inputs (for example adaptive `live_start_index` fallback, larger/conditional prefetch when no IDR is seen, or a source-keyframe warmup path before releasing bytes to Plex) and log when the gate releases without video IDR.
  Risk/Scope: med-high | fits current scope: no (behavior/latency tradeoffs + more live testing)
  User decision needed?: yes (startup latency vs reliability tradeoff for WebSafe; recommended default: prefer reliability in WebSafe and allow a faster opt-in profile).

- Date: 2026-02-24
  Category: operability
  Title: Add TS timing/continuity debug capture for the first seconds of WebSafe output
  Context: Direct Plex Web startup triage now shows Plex first-stage recorder writes TS segments and accepts stream metadata, but Plex's internal live `index.m3u8` stays empty (`buildLiveM3U8 no segment info`) across `aaccfr`, `plexsafe`, and `pmsxcode`.
  Why it matters: We need source-side evidence (PCR/PTS/DTS continuity, discontinuity markers, segment timing characteristics) to determine why Plex's `ssegment` stage is not producing usable segment info, and current tuner logs do not expose that level of detail.
  Evidence: In-container `curl` to `http://127.0.0.1:32400/livetv/sessions/<live>/<client>/index.m3u8` timed out with 0 bytes while Plex recorded many `media-*.ts` files and logged repeated `buildLiveM3U8: no segment info available`; changing WebSafe output profile did not change the symptom.
  Suggested fix: Add a temporary/debug-only TS introspection mode in PlexTuner (or a helper script) that samples the first N packets/seconds from the emitted MPEG-TS and logs parsed PCR/PTS/DTS continuity + discontinuity indicators for one request ID.
  Risk/Scope: med | fits current scope: no (new debug tooling/format)
  User decision needed?: no

- Date: 2026-02-25
  Category: performance
  Title: XMLTV external fetch blocks every concurrent /guide.xml request for up to 45s
  Context: internal/tuner/xmltv.go serveExternalXMLTV — synchronous HTTP fetch on every request, no caching.
  Why it matters: Plex metadata refresh and DVR guide sync send concurrent /guide.xml requests. Each one blocks for up to 45s (SourceTimeout default). Under normal Plex usage this causes request pile-ups, server memory growth, and downstream API timeouts in Plex DVR channel enumeration.
  Evidence: xmltv.go:60-88 — no cache; new HTTP request created and awaited per handler call. Live measurement: ~45.15s with external XMLTV vs ~0.19s placeholder. Confirmed via existing opportunity entry 2026-02-24.
  Suggested fix: Add an in-memory XMLTV cache with a configurable TTL (e.g. 10m default via PLEX_TUNER_XMLTV_CACHE_TTL). Background goroutine refreshes asynchronously; requests return the last good cached bytes immediately. On first startup: block until first fetch completes or falls back to placeholder. Serialize with a sync.RWMutex — reads never block once cache is warm.
  Implementation notes: (1) Add `cachedXML []byte`, `cacheExpiry time.Time`, `mu sync.RWMutex` to XMLTV struct. (2) `ServeHTTP` acquires read lock; if cache hit, write cached bytes and return. (3) Cache miss or expiry: acquire write lock, re-check (double-checked locking), fetch, store, release. (4) New PLEX_TUNER_XMLTV_CACHE_TTL env (default 10m). (5) Unit test: inject two calls; assert only one fetch. No behavior change for placeholder path (remains per-request).
  Risk/Scope: low-med (adds concurrency; mutex must be held correctly) | fits current scope: no (separate PR)
  User decision needed?: yes (TTL default; stale-serve-on-error vs fallback-to-placeholder preference).

- Date: 2026-02-25
  Category: maintainability
  Title: hdhomerun package duplicates env-helper functions already in internal/config
  Context: internal/hdhomerun/server.go lines 129-166 define getEnvBool, getEnvInt, getEnvUint32 that are near-identical to helpers in internal/config/config.go.
  Why it matters: Duplicate logic means future env-parsing fixes (e.g. trimming whitespace, handling "yes"/"no") must be applied in two places. Already diverged: hdhomerun getEnvBool handles "on"/"off" while config does not; hdhomerun uses fmt.Sscanf while config uses strconv.Atoi.
  Evidence: internal/hdhomerun/server.go:129-166 vs internal/config/config.go:208-268.
  Suggested fix: Either (a) export config helpers to a shared internal/envutil package and import from both, or (b) load all HDHR env vars in internal/config/config.go and pass them via hdhomerun.Config already constructed in main.go (simpler: no new package). Option (b) is lower risk.
  Implementation notes: Option B — add HDHREnabled, HDHRDeviceID, HDHRTunerCount, HDHRDiscoverPort, HDHRControlPort fields to config.Config; populate in config.Load(); delete hdhomerun/server.go env helpers; hdhomerun.LoadConfig becomes a no-op or is deleted; main.go passes config fields. No behavior change. Add one config_test for new HDHR fields.
  Risk/Scope: low (pure refactor, no behavior change) | fits current scope: no (refactor PR)
  User decision needed?: no

- Date: 2026-02-25
  Category: reliability
  Title: Catalog disk save failure leaves in-memory state ahead of persisted state
  Context: cmd/plex-tuner/main.go run command catalog refresh goroutine — calls server.UpdateChannels then c.Save.
  Why it matters: If c.Save fails (disk full, permissions, NFS hang), the server is serving the new channel list but the catalog on disk is stale. On next restart the process loads the old channels and re-indexes from scratch, causing a silent regression in channel availability between restart and next successful index.
  Evidence: main.go run refresh loop: UpdateChannels called before Save completes. Save error only logs and continues; no rollback of in-memory state.
  Suggested fix: Invert the order — Save to a temp file first (atomic rename), then call UpdateChannels only on success. Alternatively log a prominent warning that disk and memory are out of sync so operators know why post-restart state differs. The atomic-rename approach is the cleanest and has no user-visible behavior change when disk writes succeed (common case).
  Implementation notes: (1) catalog.Save should write to a .tmp file then os.Rename atomically. (2) In the refresh goroutine: call c.Save first; only if err == nil call server.UpdateChannels. (3) Add test: inject failing Save, assert UpdateChannels not called. Low blast radius — only changes success/failure ordering of two independent operations.
  Risk/Scope: low | fits current scope: no (correctness fix, separate PR)
  User decision needed?: no

- Date: 2026-02-25
  Category: operability
  Title: SIGHUP-triggered catalog reload without process restart
  Context: cmd/plex-tuner/main.go run command — catalog only refreshes on the built-in timer or full restart.
  Why it matters: Operators running in Docker/k8s often want to trigger an immediate lineup refresh (e.g. after provider maintenance) without a full pod restart, which resets streams and causes Plex to re-scan tuners. A SIGHUP reload is idiomatic Unix and expected by ops tooling.
  Evidence: main.go run command: refresh only in a background goroutine on a fixed interval. No signal handler.
  Suggested fix: Add a signal.NotifyContext or explicit signal channel for SIGHUP in the run command. On receipt, trigger a catalog refresh immediately (same logic as the periodic refresh goroutine). Log "SIGHUP received — reloading catalog". In k8s: kubectl exec kill -HUP <pid> or use a lifecycle hook.
  Implementation notes: (1) Add `sigHUP := make(chan os.Signal, 1); signal.Notify(sigHUP, syscall.SIGHUP)` in run. (2) Select on ticker and sigHUP in refresh loop. (3) No lock changes needed — same code path as periodic refresh. (4) Add a test that sends SIGHUP and asserts catalog was reloaded (or mock the fetch). Low risk: signal handling is additive and doesn't change normal operation.
  Risk/Scope: low | fits current scope: no (ops feature)
  User decision needed?: no

- Date: 2026-02-25
  Category: operability
  Title: Add dedicated /healthz or /ready endpoint for Kubernetes probes
  Context: k8s/plextuner-hdhr-test.yaml readinessProbe on /discover.json. internal/tuner/server.go — no /healthz route.
  Why it matters: /discover.json is an HDHomeRun protocol endpoint; its content and latency depend on catalog load state, not just server health. Using it as a readiness probe couples k8s readiness to HDHomeRun emulation correctness. A dedicated /healthz endpoint can return 200 immediately (liveness) or 200 once the catalog is loaded (readiness) with a JSON body including catalog size and last-refresh timestamp for ops visibility.
  Evidence: k8s/plextuner-hdhr-test.yaml readinessProbe.httpGet.path: /discover.json (initialDelaySeconds 90). No /healthz in server.go.
  Suggested fix: Add /healthz to the HTTP mux in Server.Run. Returns 200 + JSON `{"status":"ok","channels":<N>,"last_refresh":"<RFC3339>"}`. For readiness: 503 until first catalog load completes (channels > 0). Liveness: always 200 while HTTP server is up. Update k8s manifest readinessProbe to /healthz.
  Implementation notes: (1) Add lastRefresh time.Time and channelCount int64 (atomic) fields to Server, updated in UpdateChannels. (2) /healthz handler: if channels == 0, return 503 `{"status":"loading"}`; else 200. (3) Update k8s manifest. (4) Add test: new server returns 503; after UpdateChannels with channels, returns 200. No behavior change to existing endpoints.
  Risk/Scope: low | fits current scope: no (ops/k8s feature)
  User decision needed?: no

- Date: 2026-02-25
  Category: operability
  Title: Multi-arch (ARM64) Docker images for k8s clusters with ARM nodes
  Context: Dockerfile.static, Dockerfile.static.distroless, Dockerfile.static.scratch — all use standard Go cross-compile but don't declare platform targets.
  Why it matters: Home k8s clusters (Raspberry Pi, Apple Silicon VMs, cloud Graviton) often have ARM64 nodes. Without a multi-arch build, the image silently runs under QEMU emulation or fails to schedule. Static Go binaries cross-compile trivially with GOARCH=arm64 CGO_ENABLED=0.
  Evidence: Dockerfile.static uses `RUN go build` with no GOARCH override. k8s/plextuner-hdhr-test.yaml has no nodeSelector; would fail on ARM node without multi-arch image.
  Suggested fix: Use Docker Buildx with `--platform linux/amd64,linux/arm64` in CI. Add `ARG TARGETARCH` to Dockerfile.static; pass `GOARCH=$TARGETARCH` to `go build`. CI: `docker buildx build --platform linux/amd64,linux/arm64 --push`. No code changes to Go source needed; CGO_ENABLED=0 already required.
  Implementation notes: (1) Edit Dockerfile.static build stage: `ARG TARGETARCH` + `ENV GOARCH=$TARGETARCH CGO_ENABLED=0`. (2) Add `.github/workflows/docker.yml` or extend existing CI with `docker buildx` step. (3) No Go source changes. (4) Test: `docker run --platform linux/arm64 <image> /plex-tuner probe` should print help without QEMU error. Risk: none to existing amd64 behavior.
  Risk/Scope: low | fits current scope: no (CI/build feature)
  User decision needed?: no

- Date: 2026-02-25
  Category: reliability
  Title: Smoketest results not cached to disk — all channels re-probed on every restart/refresh
  Context: internal/indexer smoketest (PLEX_TUNER_SMOKETEST_ENABLED). Catalog save/load cycle in cmd/plex-tuner/main.go.
  Why it matters: With 500+ channels and SMOKETEST_TIMEOUT=8s at CONCURRENCY=10, a full smoketest takes ~400s (6.5 min) for a single refresh. On restart or -refresh this full cost is paid again. Channels that passed last time are very likely to pass again; re-probing all of them wastes provider bandwidth and slows startup.
  Evidence: filterLiveBySmoketest runs a full probe on every call (no state file). With 48 threads and CDN rate limits, false-fail rate was 99.6% (observed 2026-02-25 in 13-DVR pipeline — led to disabling smoketest entirely).
  Suggested fix: Persist smoketest pass/fail results to a sidecar file (e.g. catalog.smoketest.json, keyed by channel StreamURL hash). On next run: skip re-probe for channels whose last-passed timestamp is within a configurable staleness window (e.g. PLEX_TUNER_SMOKETEST_CACHE_TTL=4h default). Re-probe only new/changed channels and those whose cached result is stale or was a fail.
  Implementation notes: (1) New internal/indexer.SmoketestCache struct: `map[urlHash]{pass bool, ts time.Time}`. (2) Load from disk before probing; save after. (3) New PLEX_TUNER_SMOKETEST_CACHE_TTL env (default 4h). (4) filterLiveBySmoketest skips channels with valid cache hit. (5) Unit test: inject cache hits, assert those channels skip probe. No behavior change when cache file absent (falls back to full probe). Only risk: stale cache passes a now-dead URL — mitigated by TTL and full re-probe on miss.
  Risk/Scope: med | fits current scope: no (separate feature)
  User decision needed?: yes (cache TTL default and whether to re-probe past-failures immediately or also cache them with shorter TTL).
