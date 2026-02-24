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
