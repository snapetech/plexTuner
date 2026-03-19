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

- Date: 2026-03-19
  Title: Integrate tester gateway compatibility fixes
  Summary:
    - Integrated the tester fork's redirect-safe HLS playlist handling so playlist refreshes and nested relative segment paths keep using the effective post-redirect URL.
    - Added upstream request controls for custom headers, custom User-Agent, optional `Sec-Fetch-*`, ffmpeg disable, and ffmpeg DNS-rewrite disable so stricter providers/CDNs can be matched without forking the relay path.
    - Reworked the persistent cookie-jar contribution so newly learned cookies are tracked through `SetCookies` and really survive restarts, then documented the new operator knobs.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - Credit preserved in the commit footer for RK Davies because the landed change is based on his forked gateway fix set, but not a verbatim cherry-pick.
  Opportunities filed:
    - none
  Links:
    - https://github.com/rkdavies/iptvtunerr, internal/tuner/gateway.go, internal/tuner/gateway_upstream.go, internal/tuner/gateway_cookiejar.go

- Date: 2026-03-19
  Title: Assess tester fork fixes for upstream integration
  Summary:
    - Fetched `rkdavies/iptvtunerr` `main` and compared its single ahead commit against local `origin/main`.
    - Reviewed the tuner/gateway patch set, ran `go test ./internal/tuner/...` in a detached worktree at the fork tip, and classified the changes into recommended, conditional, and not-ready buckets.
    - Recorded that the redirected-HLS effective-URL handling looks worth integrating, while the persistent cookie-jar path needs a follow-up fix before merge.
  Verification:
    - `git diff --stat HEAD...FETCH_HEAD`
    - `git diff HEAD...FETCH_HEAD -- internal/tuner/... .env.example`
    - `go test ./internal/tuner/...` (run in detached worktree at `15d7cff`)
    - N/A (no local code change)
  Notes:
    - The fork patch is a single commit: `15d7cff It finally works... Codec issues in web player`.
    - The persistent cookie jar only saves domains already present in the on-disk snapshot, so a fresh jar will not persist newly learned cookies.
  Opportunities filed:
    - none
  Links:
    - https://github.com/rkdavies/iptvtunerr, memory-bank/current_task.md

- Date: 2026-03-18
  Title: Document always-on recorder daemon as a future feature
  Summary:
    - Added a future-feature explainer describing what an always-on recorder daemon would do, why it matters, and how it would fit into the existing catch-up system.
    - Linked the new explainer from the docs index and noted it in the Live TV intelligence epic as a future evaluation path.
    - Recorded the daemon as a high-scope opportunity in the memory bank so future work can pick it up cleanly.
  Verification:
    - N/A (docs-only)
  Notes:
    - This is design/backlog documentation only; no runtime behavior changed.
  Opportunities filed:
    - `memory-bank/opportunities.md` always-on recorder daemon entry
  Links:
    - docs/explanations/always-on-recorder-daemon.md, docs/index.md, docs/epics/EPIC-live-tv-intelligence.md

- Date: 2026-03-18
  Title: Add catch-up recorder and Autopilot failure memory
  Summary:
    - Added `catchup-record`, which records current in-progress capsules to local TS files plus `record-manifest.json` for non-replay sources.
    - Extended Autopilot decisions with failure counts/streaks so stale remembered paths stop being reused automatically after repeated misses.
    - Added a Ghost Hunter CLI recovery hook so hidden-grab suspicion can invoke the guarded helper directly with `-recover-hidden dry-run|restart`.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - `catchup-record` is the recorder-backed path for current in-progress capsules; it does not pretend to be a full always-on DVR daemon.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/catchup_record.go, internal/tuner/autopilot.go, cmd/iptv-tunerr/cmd_reports.go, docs/reference/cli-and-env-reference.md

- Date: 2026-03-18
  Title: Add DNA provider preference, catch-up curation, and Ghost Hunter action guidance
  Summary:
    - Added `IPTV_TUNERR_DNA_PREFERRED_HOSTS` so duplicate DNA winners can bias trusted provider/CDN authorities before score-based tie-breaking.
    - Curated catch-up capsule generation so duplicate programme rows that share the same `dna_id + start + title` collapse to the richer candidate before export and publishing.
    - Improved Ghost Hunter so visible stale sessions and hidden-grab suspicion now produce different recommended next actions, and the live endpoint accepts `?stop=true`.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Catch-up curation prefers richer metadata and better state priority while keeping the existing lane model intact.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/dna_policy.go, internal/tuner/xmltv.go, internal/tuner/ghost_hunter.go, internal/tuner/server.go

- Date: 2026-03-18
  Title: Add provider host-penalty autotune
  Summary:
    - Added host-level failure tracking so repeated request/status/empty-body failures penalize specific upstream hosts instead of only incrementing global instability counters.
    - Updated stream ordering so healthier hosts/CDNs are preferred before penalized ones, while still preserving normal fallback behavior.
    - Exposed penalized upstream hosts in the provider profile and updated README, features, reference docs, and changelog.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - A successful stream on a host clears its penalty, so the steering remains self-healing instead of permanently blacklisting one CDN.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_adapt.go, internal/tuner/gateway_provider_profile.go, internal/tuner/gateway_test.go

- Date: 2026-03-18
  Title: Add registration intent-preset parity
  Summary:
    - Extended `IPTV_TUNERR_REGISTER_RECIPE` so Plex/Emby/Jellyfin registration can use `sports_now`, `kids_safe`, and `locals_first` in addition to the score-based recipes.
    - Reused the runtime lineup recipe logic so registration and live lineup shaping do not drift into separate heuristics.
    - Updated README, features, reference docs, changelog, and env examples for the expanded registration recipe surface.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - `sports_now` and `kids_safe` filter the registered set; `locals_first` keeps the full set and only changes ordering.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_runtime_register.go, cmd/iptv-tunerr/cmd_runtime_register_test.go, internal/tuner/server.go, docs/reference/cli-and-env-reference.md

- Date: 2026-03-18
  Title: Add Autopilot upstream URL memory
  Summary:
    - Extended remembered Autopilot decisions to persist the last known-good upstream URL and host alongside transcode/profile choices.
    - Updated the gateway to prefer the remembered stream path first on later requests for the same `dna_id + client_class`.
    - Exposed the preferred host in the Autopilot report and updated README, features, reference docs, changelog, and env examples.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The gateway still falls back across the remaining stream URLs normally if the remembered upstream stops working.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/autopilot.go, internal/tuner/gateway_adapt.go, internal/tuner/gateway.go, internal/tuner/gateway_test.go

- Date: 2026-03-18
  Title: Apply Channel DNA policy to lineup and registration
  Summary:
    - Added `IPTV_TUNERR_DNA_POLICY=off|prefer_best|prefer_resilient` so duplicate channels that share a `dna_id` can collapse to a single preferred winner.
    - Applied the policy in runtime lineup shaping and registration flows so Channel DNA affects real server behavior instead of only powering reports.
    - Updated README, features, reference docs, changelog, and env examples for the new policy surface.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - `prefer_best` favors overall intelligence score and guide quality; `prefer_resilient` favors backup depth and stream resilience first.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/dna_policy.go, internal/tuner/server.go, cmd/iptv-tunerr/cmd_runtime_register.go, docs/reference/cli-and-env-reference.md

- Date: 2026-03-18
  Title: Add Autopilot hot-start and reporting
  Summary:
    - Added `autopilot-report` plus `/autopilot/report.json` so operators can inspect remembered decisions and the hottest channels by hit count.
    - Added hot-start tuning for favorite or high-hit channels on the ffmpeg HLS path, using more aggressive startup thresholds and keepalive/bootstrap settings.
    - Updated README, features, reference docs, changelog, env example, and current-task tracking for the new Autopilot surfaces.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Hot-start remains explicit and evidence-driven: it only activates from `IPTV_TUNERR_HOT_START_CHANNELS` or remembered Autopilot hit counts.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/autopilot.go, internal/tuner/gateway_hotstart.go, internal/tuner/gateway_relay.go, internal/tuner/server.go, cmd/iptv-tunerr/cmd_reports.go

- Date: 2026-03-18
  Title: Add intent lineup recipes
  Summary:
    - Extended `IPTV_TUNERR_LINEUP_RECIPE` with `sports_now`, `kids_safe`, and `locals_first` so operators can expose intent-focused lineups instead of only score-sorted ones.
    - Reused explicit catalog/name/TVGID heuristics and existing lineup-shape logic rather than pretending the app already has a full semantic channel taxonomy.
    - Updated README, features, reference docs, changelog, and current-task tracking for the new recipes.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - `sports_now` and `kids_safe` are filter recipes; `locals_first` is a reorder recipe that piggybacks on the current North-American lineup-shape scoring.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/server.go, internal/tuner/server_test.go, README.md, docs/reference/cli-and-env-reference.md

- Date: 2026-03-18
  Title: Add source-backed catch-up replay mode
  Summary:
    - Added replay-aware capsule previews and publishing so `catchup-capsules`, `/guide/capsules.json`, and `catchup-publish` can render real programme-window replay URLs when `IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE` is configured.
    - Kept the boundary honest: without a replay template, capsules and published libraries remain launcher-mode and point back at the live stream path.
    - Updated README, features, reference docs, changelog, env example, and current-task tracking for the new replay-mode behavior and template tokens.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The replay implementation is provider-agnostic on purpose; the app does not guess a provider timeshift URL shape and only enters replay mode when the operator supplies a source-backed template.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/xmltv.go, internal/tuner/catchup_publish.go, internal/tuner/catchup_replay_test.go, cmd/iptv-tunerr/cmd_reports.go, cmd/iptv-tunerr/cmd_catchup_publish.go

- Date: 2026-03-18
  Title: Ship the remaining product-facing intelligence surfaces
  Summary:
    - Added `epg-doctor -write-aliases` plus `/guide/aliases.json` so healthy normalized-name repairs can be exported as reviewable `name_to_xmltv_id` overrides.
    - Added `channel-leaderboard` plus `/channels/leaderboard.json` for hall-of-fame, hall-of-shame, guide-risk, and stream-risk snapshots.
    - Added `IPTV_TUNERR_REGISTER_RECIPE` / `run -register-recipe` so Plex, Emby, and Jellyfin registration can reuse channel-intelligence scoring instead of blindly registering catalog order.
    - Updated README, features, reference docs, changelog, env example, and current-task tracking for the new operator surfaces.
  Verification:
    - `go test ./internal/epgdoctor ./internal/channelreport ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Registration recipes are intentionally catalog/intelligence-score based; they improve ordering and optional pruning for registration flows without waiting on a heavier runtime guide-health prewarm redesign.
  Opportunities filed:
    - none
  Links:
    - internal/epgdoctor/epgdoctor.go, internal/channelreport/report.go, internal/tuner/server.go, cmd/iptv-tunerr/cmd_guide_reports.go, cmd/iptv-tunerr/cmd_reports.go, cmd/iptv-tunerr/cmd_runtime_register.go

- Date: 2026-03-18
  Title: Split shared report input helpers into a support file
  Summary:
    - Added `cmd/iptv-tunerr/cmd_report_support.go` for shared live-catalog loading and optional XMLTV match-report loading.
    - Removed duplicate report-input plumbing from `cmd_reports.go` and `cmd_guide_reports.go`.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is structural only; report behavior was preserved while consolidating the shared input path.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_reports.go, cmd/iptv-tunerr/cmd_guide_reports.go, cmd/iptv-tunerr/cmd_report_support.go

- Date: 2026-03-18
  Title: Split VOD commands out of cmd_core.go
  Summary:
    - Moved `mount`, `plex-vod-register`, and `vod-split` into `cmd/iptv-tunerr/cmd_vod.go`.
    - Kept `cmd_core.go` focused on core live-TV commands (`run`, `serve`, `index`, `probe`, `supervise`).
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is structural only; VOD command flags and behavior were preserved.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_core.go, cmd/iptv-tunerr/cmd_vod.go, cmd/iptv-tunerr/main.go

- Date: 2026-03-18
  Title: Split shared runtime server helpers out of cmd_runtime.go
  Summary:
    - Added `cmd/iptv-tunerr/cmd_runtime_server.go` for shared live-channel load/repair/DNA setup and shared `tuner.Server` construction.
    - Reduced `cmd_runtime.go` to the actual serve/run lifecycle differences instead of rebuilding the same runtime setup twice.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is structural only; serve/run behavior was preserved.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_runtime.go, cmd/iptv-tunerr/cmd_runtime_server.go

- Date: 2026-03-18
  Title: Split generic gateway support helpers into a focused file
  Summary:
    - Moved request-id, env parsing, disconnect detection, and path parsing helpers into `internal/tuner/gateway_support.go`.
    - Kept `internal/tuner/gateway.go` focused on `ServeHTTP` and top-level request orchestration.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This is structural only; runtime gateway behavior was preserved.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_support.go

- Date: 2026-03-18
  Title: Split runtime registration logic out of cmd_runtime.go
  Summary:
    - Moved Plex/Emby/Jellyfin registration and watchdog helpers into `cmd/iptv-tunerr/cmd_runtime_register.go`.
    - Kept `cmd_runtime.go` focused on the serve/run lifecycle, catalog loading, and runtime server setup.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is structural only; run/serve behavior and registration flows were preserved.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_runtime.go, cmd/iptv-tunerr/cmd_runtime_register.go

- Date: 2026-03-18
  Title: Split catch-up publish command out of cmd_ops.go
  Summary:
    - Moved `catchup-publish` into `cmd/iptv-tunerr/cmd_catchup_publish.go`.
    - Kept `cmd_ops.go` focused on supervisor and VOD operational helpers instead of mixing them with Guide/EPG publishing.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is structural only; command flags and publish behavior were preserved.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_ops.go, cmd/iptv-tunerr/cmd_catchup_publish.go, cmd/iptv-tunerr/main.go

- Date: 2026-03-18
  Title: Split gateway relay implementations into a focused file
  Summary:
    - Moved the raw TS ffmpeg normalizer and the HLS relay implementations into `internal/tuner/gateway_relay.go`.
    - Reduced `internal/tuner/gateway.go` to the request entrypoint and upstream-selection/orchestration path.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This is structural only; runtime relay behavior was preserved while isolating the remaining ffmpeg/HLS engine from the top-level gateway request path.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_relay.go

- Date: 2026-03-18
  Title: Fix player_api probe shape handling and restore direct index fallback
  Summary:
    - Updated `internal/provider/ProbePlayerAPI` to treat `server_info`-only HTTP 200 JSON as a valid Xtream-style auth response instead of misclassifying it as `bad_status`.
    - Restored the older direct `IndexFromPlayerAPI` retry path in `fetchCatalog` when ranked provider probes return no OK host, before falling back to `get.php`.
    - Added regression coverage for both the `server_info` probe case and the no-ranked-host direct-index fallback case.
  Verification:
    - `go test ./internal/provider ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This fixes a real tester regression where `run` could fail on usable Xtream panels whose top-level auth response shape differed from the probe's stricter expectations.
  Opportunities filed:
    - none
  Links:
    - internal/provider/probe.go, internal/provider/probe_test.go, cmd/iptv-tunerr/cmd_catalog.go, cmd/iptv-tunerr/main_test.go

- Date: 2026-03-18
  Title: Split Plex oracle lab commands out of cmd_ops.go
  Summary:
    - Moved `plex-epg-oracle` and `plex-epg-oracle-cleanup` into `cmd/iptv-tunerr/cmd_oracle_ops.go`.
    - Kept `cmd_ops.go` focused on catch-up publishing and shared VOD/supervisor operational helpers.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This continues the CLI cleanup by separating experimental Plex lab tooling from the day-to-day operational commands.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_ops.go, cmd/iptv-tunerr/cmd_oracle_ops.go, cmd/iptv-tunerr/main.go

- Date: 2026-03-18
  Title: Split guide-diagnostics commands out of cmd_reports.go
  Summary:
    - Moved `epg-link-report`, `guide-health`, and `epg-doctor` into `cmd/iptv-tunerr/cmd_guide_reports.go`.
    - Added shared catalog/XMLTV loading helpers for the guide-diagnostics path and kept `cmd_reports.go` focused on channel, Ghost Hunter, and capsule reporting.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is structural cleanup plus a small duplication reduction; command behavior and flags were preserved.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_reports.go, cmd/iptv-tunerr/cmd_guide_reports.go, cmd/iptv-tunerr/main.go

- Date: 2026-03-18
  Title: Split CLI runtime handlers out of cmd_core.go
  Summary:
    - Moved `handleServe` and `handleRun` into `cmd/iptv-tunerr/cmd_runtime.go`.
    - Reduced `cmd/iptv-tunerr/cmd_core.go` to the remaining core non-runtime commands while preserving command behavior.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This continues the CLI cleanup by aligning the runtime-serving path with its own file instead of keeping it inside the broader core command bucket.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_core.go, cmd/iptv-tunerr/cmd_runtime.go

- Date: 2026-03-18
  Title: Split CLI media-server helpers out of main.go
  Summary:
    - Moved Plex/Emby/Jellyfin catch-up library registration helpers into `cmd/iptv-tunerr/cmd_media_servers.go`.
    - Reduced `cmd/iptv-tunerr/main.go` to bootstrap, usage output, and a few tiny shared helpers.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This keeps the CLI cleanup moving without changing command behavior.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/cmd_media_servers.go

- Date: 2026-03-18
  Title: Split CLI catalog and EPG-repair helpers out of main.go
  Summary:
    - Moved catalog ingest, direct-M3U/provider fallback handling, stream-host filtering, runtime EPG repair helpers, and catch-up preview loading into `cmd/iptv-tunerr/cmd_catalog.go`.
    - Reduced `cmd/iptv-tunerr/main.go` to bootstrap and generic media-server helper code.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This continues the command-entrypoint cleanup without changing command behavior.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/cmd_catalog.go

- Date: 2026-03-18
  Title: Split gateway upstream request helpers into a focused file
  Summary:
    - Moved upstream request/header application, ffmpeg header block generation, response preview reading, and concurrency-preview parsing into `internal/tuner/gateway_upstream.go`.
    - Reduced `gateway.go` further so upstream helper code no longer sits inline with the relay flow.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - Structural only; no intended runtime behavior change.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_upstream.go

- Date: 2026-03-18
  Title: Restore provider player_api fallback and fix XMLTV reader cancellation
  Summary:
    - Changed `fetchCatalog` so only explicit direct M3U configuration uses the M3U-only branch; provider-configured runs now continue through the `player_api`-first path with `get.php` fallback as before.
    - Added a regression test for the `884`/built-`get.php` case.
    - Fixed `internal/refio.Open` so timeout contexts are canceled when the caller closes the response body, not immediately when `Open` returns.
    - Added a regression test to keep remote XMLTV readers usable after `Open`.
  Verification:
    - `go test ./cmd/iptv-tunerr ./internal/refio ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This addresses a real runtime bug on `main`, not a side effect of the in-progress gateway refactors.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/main_test.go, internal/refio/refio.go, internal/refio/refio_test.go

- Date: 2026-03-18
  Title: Split gateway debug helpers into a focused file
  Summary:
    - Moved stream-debug env parsing, header logging, capped tee-file helpers, and the wrapped debug response writer into `internal/tuner/gateway_debug.go`.
    - Further reduced `gateway.go` so observability utilities no longer sit inline with the request/relay control path.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - Another structural-only `INT-006` slice; request handling behavior was preserved.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_debug.go

- Date: 2026-03-18
  Title: Split gateway stream mechanics into a focused helper file
  Summary:
    - Moved TS discontinuity splice helpers, startup-signal detection, adaptive stream buffering, and null-TS keepalive support into `internal/tuner/gateway_stream_helpers.go`.
    - Left `gateway.go` focused more tightly on request lifecycle and relay orchestration.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This continues the structural-only gateway decomposition without changing request handling or tuning policy.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_stream_helpers.go

- Date: 2026-03-18
  Title: Split gateway ffmpeg relay helpers into a focused file
  Summary:
    - Moved ffmpeg relay output writer types, stdin-normalizer support, and bootstrap TS generation out of `internal/tuner/gateway.go` into `gateway_ffmpeg_relay.go`.
    - Kept relay decision-making and orchestration in `gateway.go` while further shrinking the monolith.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This is another structural-only `INT-006` slice; runtime policy and transcoding choices were preserved.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_ffmpeg_relay.go

- Date: 2026-03-18
  Title: Split gateway profile/ffmpeg and HLS helpers into focused files
  Summary:
    - Moved profile selection, override loading, ffmpeg codec argument building, bootstrap audio helpers, and ffmpeg input URL canonicalization into `internal/tuner/gateway_profiles.go`.
    - Moved HLS playlist refresh, rewrite, segment fetch, and playlist parsing helpers into `internal/tuner/gateway_hls.go`.
    - Preserved runtime behavior while reducing `internal/tuner/gateway.go` to more of the request/relay orchestration layer.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This continues the `INT-006` gateway decomposition without changing runtime policy or transcoding decisions.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_profiles.go, internal/tuner/gateway_hls.go

- Date: 2026-03-18
  Title: Split gateway provider-profile and adaptation helpers into focused files
  Summary:
    - Moved provider behavior profile/autotune reporting out of `internal/tuner/gateway.go` into `gateway_provider_profile.go`.
    - Moved Plex client adaptation, request hint parsing, session resolution, and Autopilot helper methods into `gateway_adapt.go`.
    - Preserved runtime behavior while shrinking the core gateway file and giving the next decomposition slices cleaner seams.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This is structural only; the relay/transcode path remains in `gateway.go` for now.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_adapt.go, internal/tuner/gateway_provider_profile.go

- Date: 2026-03-18
  Title: Split CLI command registration out of main.go
  Summary:
    - Moved command flag wiring and summaries into concern-specific registry builders in `cmd_core.go`, `cmd_reports.go`, and `cmd_ops.go`.
    - Added a small shared command-spec type so `main.go` now only handles version, usage rendering, command lookup, and dispatch.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is a structural refactor only; command names and runtime behavior were preserved.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/cmd_registry.go, cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/cmd_core.go, cmd/iptv-tunerr/cmd_reports.go, cmd/iptv-tunerr/cmd_ops.go

- Date: 2026-03-18
  Title: Cross-wire guide-health into lineup and catch-up policy
  Summary:
    - Added a shared local-file/URL loader under `internal/refio` and switched report/guide tooling away from duplicated `http.DefaultClient` helper paths.
    - Cached guide-health alongside the merged XMLTV cache and added opt-in guide-quality policies so runtime lineup refreshes and catch-up capsule preview/publish flows can suppress placeholder-only or no-programme channels.
    - Added guide-policy tests for lineup refresh and catch-up capsule filtering, plus env/docs/changelog updates for the new behavior.
  Verification:
    - `go test ./internal/refio ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Runtime lineup filtering depends on cached guide-health being available, so the first startup refresh remains permissive until the merged guide cache exists.
  Opportunities filed:
    - none
  Links:
    - internal/refio/refio.go, internal/tuner/guide_policy.go, internal/tuner/epg_pipeline.go, internal/tuner/server.go, cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/cmd_reports.go, cmd/iptv-tunerr/cmd_ops.go

- Date: 2026-03-18
  Title: Add operator how-to for fixing guide data with EPG Doctor
  Summary:
    - Added a dedicated how-to for the new `epg-doctor` workflow so operators can diagnose placeholder-only guide rows, missing programme blocks, unmatched XMLTV channels, and bad `TVGID` linkage from one document.
    - Linked the new guide-fix workflow from both the how-to index and the runbooks index so guide troubleshooting now routes to the same documented path.
  Verification:
    - N/A (docs-only)
  Notes:
    - This does not add new runtime behavior; it documents the already-shipped `guide-health` and `epg-doctor` surfaces in an operator-facing format.
  Opportunities filed:
    - none
  Links:
    - docs/how-to/fix-guide-data-with-epg-doctor.md, docs/how-to/index.md, docs/runbooks/index.md

- Date: 2026-03-18
  Title: Start Live TV Intelligence with channel health and EPG provenance reports
  Summary:
    - Added a new channel intelligence foundation: `iptv-tunerr channel-report` and `/channels/report.json` now score channels by guide confidence, stream resilience, and actionable next steps.
    - Wired optional XMLTV enrichment into the report so operators can see whether guide success comes from exact `tvg-id` matches, alias overrides, normalized-name repairs, or no deterministic match at all.
    - Added early intelligence-driven lineup recipes (`high_confidence`, `balanced`, `guide_first`, `resilient`), a persisted Channel DNA foundation (`dna_id`), an Autopilot decision-memory foundation (`IPTV_TUNERR_AUTOPILOT_STATE_FILE`), a Ghost Hunter visible-session foundation (`ghost-hunter`, `/plex/ghost-report.json`), and a provider behavior profile foundation (`/provider/profile.json`).
  Verification:
    - `./scripts/verify`
    - `go test ./internal/channelreport ./internal/tuner ./cmd/iptv-tunerr`
  Notes:
    - This is still a foundation slice only. Catch-up capsules, active provider self-tuning defaults, hidden-grab Ghost Hunter automation, and a fuller cross-provider Channel DNA graph remain explicitly planned multi-PR work.
  Opportunities filed:
    - none
  Links:
    - internal/channelreport/report.go, internal/tuner/autopilot.go, internal/tuner/ghost_hunter.go, internal/tuner/gateway.go, internal/tuner/server.go, cmd/iptv-tunerr/main.go, docs/epics/EPIC-live-tv-intelligence.md, README.md

- Date: 2026-03-18
  Title: Expand Docker image matrix to linux armv7
  Summary:
    - Extended `.github/workflows/docker.yml` so registry publishes now target `linux/amd64`, `linux/arm64`, and `linux/arm/v7`.
    - Updated `Dockerfile` to translate BuildKit `TARGETVARIANT` into `GOARM`, which is required for correct Go builds on `linux/arm/v7`.
    - Aligned the packaging/platform docs with the widened Linux container platform set.
  Verification:
    - `./scripts/verify`
  Notes:
    - Container publishing remains Linux-only. Windows and macOS continue to be binary-release targets, not container targets.
  Opportunities filed:
    - none
  Links:
    - Dockerfile, .github/workflows/docker.yml, docs/how-to/package-test-builds.md, docs/how-to/platform-requirements.md

- Date: 2026-03-18
  Title: Expand tagged GitHub Release binaries to linux armv7 and windows arm64
  Summary:
    - Extended `.github/workflows/release.yml` so tagged releases now publish `linux/arm/v7` and `windows/arm64` binaries in addition to the existing amd64/arm64 targets.
    - Updated the release build helper to carry `GOARM` through to artifact naming so the Linux 32-bit ARM build is emitted as a distinct `linux-armv7` release asset.
    - Aligned the packaging and platform docs with the actual published artifact matrix.
  Verification:
    - `./scripts/verify`
  Notes:
    - This change expands GitHub Release binary artifacts only; container images remain `linux/amd64` and `linux/arm64`.
  Opportunities filed:
    - none
  Links:
    - .github/workflows/release.yml, docs/how-to/package-test-builds.md, docs/how-to/platform-requirements.md

- Date: 2026-03-18
  Title: Automate GitHub Release notes from repo changelog and tag commit range
  Summary:
    - Added `scripts/generate-release-notes.sh` so release pages are generated from repository data instead of GitHub's generic auto-notes.
    - Wired `.github/workflows/release.yml` to fetch full tag history and publish `body_path` from generated notes; the generator prefers the matching changelog tag section, then `Unreleased`, then the exact tagged commit range.
    - Updated `tester-bundles.yml`, packaging docs, and recurring-loop guidance so future release jobs stop reintroducing generic note generation and the current `v0.1.7` release can be backfilled with generated notes.
  Verification:
    - `bash -n scripts/generate-release-notes.sh .github/workflows/release.yml .github/workflows/tester-bundles.yml`
    - `bash ./scripts/generate-release-notes.sh v0.1.7 dist/release-notes-v0.1.7.md`
    - `./scripts/verify`
  Notes:
    - If old tags are intentionally pruned, the generator falls back to the tagged commit itself when no previous tag is available, which keeps notes bounded instead of dumping full repo history.
  Opportunities filed:
    - none
  Links:
    - scripts/generate-release-notes.sh, .github/workflows/release.yml, .github/workflows/tester-bundles.yml, docs/how-to/package-test-builds.md, memory-bank/recurring_loops.md

- Date: 2026-03-18
  Title: Release v0.1.7 for multi-credential direct-M3U indexing and prune old release tags
  Summary:
    - Added numbered direct-M3U config support so `IPTV_TUNERR_M3U_URL`, `_2`, `_3`, and higher are loaded together when operators split channels across multiple credentialed playlist URLs.
    - Changed direct-M3U catalog indexing to merge all successful configured M3U feeds before dedupe/filtering instead of stopping at the first successful fetch, fixing incomplete indexes when providers require multiple credential sets.
    - Added targeted config/indexing tests, released the change as `v0.1.7`, and removed superseded git tags and GitHub releases so only the current repo tag remains.
  Verification:
    - `go test -count=1 ./cmd/iptv-tunerr ./internal/config`
    - `./scripts/verify`
  Notes:
    - Git cleanup completed: local and remote git tags older than `v0.1.7` were deleted, and old GitHub releases were deleted.
    - Registry artifact cleanup could not be completed from this environment: GHCR package APIs returned `403` without package scopes, and Docker Hub had no authenticated credentials configured for delete operations.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/main_test.go, internal/config/config.go, internal/config/config_test.go, memory-bank/current_task.md

- Date: 2026-03-18
  Title: Runtime EPG repair from provider/external XMLTV metadata
  Summary:
    - Integrated deterministic EPG matching into catalog build so live channels can have missing or incorrect `TVGID`s repaired from provider `xmltv.php` metadata first and external XMLTV metadata second, before `LIVE_EPG_ONLY` filtering runs.
    - Added `IPTV_TUNERR_XMLTV_ALIASES` and `IPTV_TUNERR_XMLTV_MATCH_ENABLE`, plus an example alias JSON file and updated env/k8s examples/docs to make the external XMLTV + alias source explicit.
    - Fixed the `run` path to carry forward the provider entry actually used during indexing so the server's provider EPG fetch can stay aligned with the chosen provider source instead of blindly falling back to the primary config entry.
    - Added tests for deterministic repair, external repair behavior, provider-over-external precedence, and config parsing; updated docs that still described guide handling as "placeholder vs external only."
  Verification:
    - `./scripts/verify`
  Notes:
    - Runtime repair remains deterministic only (`tvg-id`, alias exact, normalized exact-name). Fuzzy matching and persistent match storage are still not implemented.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/main_test.go, internal/epglink/epglink.go, internal/tuner/epg_pipeline.go, .env.example, k8s/xmltv-aliases.example.json

- Date: 2026-03-18
  Title: Learn provider concurrency caps from 423/429/458 live-stream failures
  Summary:
    - Updated the tuner gateway to classify upstream `423`, `429`, and `458` playlist failures as provider concurrency-limit errors instead of collapsing them into a generic `502`.
    - Added bounded upstream error-body capture to logs and a parser for numeric caps in phrases like `maximum 1 connections allowed`; when present, the gateway now learns that lower concurrency cap and clamps the effective local tuner limit for the current process.
    - Added tuner tests for `458` translation, advertised-cap parsing, and local rejection after learning a lower upstream limit. Updated troubleshooting and known-issues docs to tell operators to persist the learned value via `IPTV_TUNERR_TUNER_COUNT`.
  Verification:
    - `./scripts/verify`
  Notes:
    - Learned caps only reduce the effective limit and only for the running process; they do not overwrite config or attempt to raise limits automatically.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_test.go, docs/runbooks/iptvtunerr-troubleshooting.md, memory-bank/known_issues.md

- Date: 2026-03-18
  Title: Release v0.1.4 with Cloudflare/CDN playback fixes
  Summary:
    - Forwarded selected upstream request context (`Cookie`, `Referer`, `Origin`, and passthrough `Authorization` when provider basic auth is not configured) into both Go relay fetches and ffmpeg HLS input headers so CDN-backed playlists and segments retain caller auth context.
    - Changed ffmpeg input host canonicalization to prefer resolved IPv4 addresses when available, avoiding unroutable IPv6 selection on dual-stack CDN hosts.
    - Raised the default ffmpeg HLS read/write timeout and websafe startup timeout to 60s for slower CDN-backed live starts, and documented the new defaults in the troubleshooting runbook.
    - Added tuner unit coverage for ffmpeg header construction and IPv4 resolution preference, then prepared the patch for release as `v0.1.4`.
  Verification:
    - `./scripts/verify`
    - `VERSION=v0.1.4 ./scripts/build-test-packages.sh`
  Notes:
    - Generated package archives under `dist/test-packages/v0.1.4/` for local verification only; they were not committed.
    - This checkout only had `origin` configured for the IPTV Tunerr repo, so the release push used the remotes actually present in this workspace.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_test.go, docs/runbooks/iptvtunerr-troubleshooting.md, memory-bank/current_task.md

- Date: 2026-03-13
  Title: Fix iptv-m3u-server splitter: emit all stream URLs per channel in dvr-*.m3u
  Summary:
    - Updated k3s/plex iptv-m3u-splitter-configmap.yaml (split-m3u.py): Channel now has urls: list[str]; parse_m3u_channels collects all URL lines after each EXTINF; dedupe_by_tvg_id merges URLs from all duplicates into the winner; write_bucket_file writes one EXTINF per channel then all URLs. So category DVR files get every CDN/host variant per channel and IptvTunerr strip keeps non-CF.
    - Applied ConfigMap and restarted deployment/iptv-m3u-server. New pod will use updated script on next fetch→split cycle. After that cycle, restart iptvtunerr-supervisor so category instances reload catalogs.
  Verification:
    - ConfigMap applied; rollout restarted. Next split run will produce dvr-*.m3u with multiple URLs per channel when source has duplicate tvg-ids.
  Notes:
    - Implemented in sibling k3s/plex; known_issues and task_history updated in iptvTunerr.
  Opportunities filed:
    - none
  Links:
    - k3s/plex/iptv-m3u-splitter-configmap.yaml, memory-bank/known_issues.md (Category DVRs — fixed)

- Date: 2026-03-12
  Title: Root cause: category DVRs empty — dvr-*.m3u single-URL from iptv-m3u-server
  Summary:
    - Identified why bcastus, newsus, generalent (and similar category tuners) end up with 0 channels and "no live channels available": per-category M3U files (dvr-bcastus.m3u etc.) from iptv-m3u-server contain only one stream URL per channel, and that URL is always cf.like-cdn.com. IptvTunerr stripStreamHosts then drops every channel. Main HDHR uses live.m3u which has multiple URLs per channel, so after dedupe+strip many channels remain.
    - Documented root cause and required upstream fix in known_issues.md; added runbook §10 and reference docs/upstream-m3u-split-requirement.md; updated repo_map (category DVR feeds). Fix: iptv-m3u-server split step must emit all stream URLs per channel in dvr-*.m3u (same format as live.m3u).
  Verification:
    - Confirmed from cluster: curl dvr-bcastus.m3u → 133 channels, all URLs cf.like-cdn.com; live.m3u has multiple hosts. N/A for code (docs only).
  Notes:
    - Fix is in sibling k3s/plex repo (iptv-m3u-server split script), not in IptvTunerr.
  Opportunities filed:
    - none
  Links:
    - memory-bank/known_issues.md, docs/runbooks/iptvtunerr-troubleshooting.md §10, docs/reference/upstream-m3u-split-requirement.md, memory-bank/repo_map.md

- Date: 2026-03-12
  Title: Single-pod consolidation: merge oracle into main supervisor
  Summary:
    - Merged oracle-cap instances (hdhrcap100…hdhrcap600) into the main supervisor config so one pod runs all tuner instances (main + categories + oracle). No separate iptvtunerr-oracle-supervisor deployment.
    - Updated ConfigMap iptvtunerr-supervisor-config with merged instances (28 total). Patched deployment iptvtunerr-supervisor to expose container ports 5201–5206. Patched Service iptvtunerr-oracle-hdhr to select app=iptvtunerr-supervisor. Scaled deployment iptvtunerr-oracle-supervisor to 0.
    - Repo: k8s/iptvtunerr-oracle-supervisor.yaml is now Service-only (selector app=iptvtunerr-supervisor). Added k8s/oracle-instances.json with the 6 instance definitions (including IPTV_TUNERR_STRIP_STREAM_HOSTS) for reference when generating merged configs. Updated repo_map, known_issues, and rollout instructions.
  Verification:
    - iptvtunerr-supervisor pod Running/Ready; iptvtunerr-oracle-hdhr endpoints point at main pod; curl from inside pod to 127.0.0.1:5201/discover.json → 200.
  Notes:
    - Oracle data dir was hostPath /srv/iptvtunerr-oracle-data on the old deployment; merged pod uses the main supervisor's /data volume, so oracle instance catalogs live under /data/hdhrcap100 etc. on the same volume.
  Opportunities filed:
    - none
  Links:
    - memory-bank/repo_map.md (single-pod consolidation), k8s/oracle-instances.json, k8s/iptvtunerr-oracle-supervisor.yaml

- Date: 2026-03-12
  Title: Restore plex.home and iptvtunerr-hdhr.plex.home with kspls0 NotReady
  Summary:
    - Node kspls0 (media=plex) was NotReady (kubelet stopped posting status); Plex and plex-label-proxy pods could not schedule; plex.home returned 503; iptvtunerr-hdhr.plex.home returned 404 (Ingress pointed at non-existent service iptvtunerr-hdhr-test).
    - Force-deleted Terminating pods (plex, plex-label-proxy, db-sync, threadfin, hidden-grab-recover). Removed unreachable taints from kspls0, then cordoned kspls0; labeled kspld0 with media=plex but kspld0 was at pod limit (110/110) so Plex pod still could not schedule.
    - Applied manual Endpoints workaround: removed selector from Service `plex`, added `k8s/plex-endpoints-manual.yaml` with 192.168.50.85:32400 (Plex responding on kspls0 host). plex.home returned 200.
    - Patched Ingress `iptvtunerr-hdhr` to use backend service `iptvtunerr-hdhr` instead of `iptvtunerr-hdhr-test`. iptvtunerr-hdhr.plex.home discover.json, lineup.json, guide.xml, and stream/0 now return 200; stream delivered ~10MB in 8s.
  Verification:
    - `curl -sk https://plex.home/identity` → 200
    - `curl -s http://iptvtunerr-hdhr.plex.home/discover.json` → 200; lineup.json and guide.xml → 200; `/stream/0` → 200 with MPEG-TS bytes
  Notes:
    - plex-label-proxy remains Pending (nodeSelector media=plex; kspls0 cordoned, kspld0 at pod limit). /media/providers route may not rewrite labels until that pod runs or node recovers.
    - When kspls0 is back: uncordon kspls0, restore Service plex selector (`app=plex`), delete manual Endpoints, delete k8s/plex-endpoints-manual.yaml apply or leave file for future outages.
  Opportunities filed:
    - none
  Links:
    - memory-bank/known_issues.md (plex.home 503 workaround), k8s/plex-endpoints-manual.yaml

- Date: 2026-02-25
  Title: 13-DVR pipeline end-to-end: M3U fetch → EPG prune → split → Threadfin → Plex DVR activation
  Summary:
    - Deployed iptv-m3u-server (M3U updater + nginx) and all 13 Threadfin instances to k8s plex namespace
    - Disabled STREAM_SMOKETEST_ENABLED (was causing 99.6% false-fail due to CDN rate limits with 48 threads)
    - Increased POSTVALIDATE_TIMEOUT_SECS from 6→12 to reduce rc_124 false drops
    - Full run: 48903 streams fetched from cf.supergaminghub.xyz in 2s, 6108 EPG-linked, 3173 split across 13 DVRs
    - Registered 13 Threadfin devices + 13 DVRs in Plex via NodePort API (bypassing plex-dvr-setup-multi.sh which uses wget)
    - Fixed plex-activate-dvr-lineups.py: wget→curl + --globoff for [] in query params + empty-DVR ValueError→graceful skip
    - Fixed plex-reload-guides-batched.py: wget→curl for both GET and POST
    - Activated channels in Plex: 8 of 13 DVRs with channels (1316 total)
    - 5 DVRs wiped to 0 by postvalidate CDN rate-limiting (newsus/sportsb/moviesprem/ukie/eusouth)
  Verification:
    - EPG counts: bcastus=136, docsfam=189, eunordics=173, eueast=336, latin=218, otherworld=220, sportsa=22, generalent=22
    - Plex has 13 DVRs registered, 8 with channels in EPG, guide reloads completed
  Notes:
    - kubectl must use KUBECONFIG=<user-kubeconfig> (not default k3s /etc/rancher/k3s/k3s.yaml which is root-only)
    - Plex container has curl but NOT wget; all scripts must use curl
    - Plex device URI format: IP:port (no http://) when registering via POST query param
    - DVR activation PUT needs --globoff for literal [] in channelMappingByKey[id]=id query params
    - Postvalidate CDN rate-limit causes false-positive drops (see opportunities.md)
  Opportunities filed:
    - Postvalidate CDN rate-limiting → opportunities.md 2026-02-25

- Date: 2026-02-25
  Title: Two flows: easy (HDHR 479 cap) vs full (DVR builder, max feeds)
  Summary:
    - internal/tuner/server.go: PlexDVRWizardSafeMax = 479; easy mode strips lineup from end to fit Plex wizard (e.g. Rogers West Canada ~680 ch).
    - cmd/iptv-tunerr/main.go: -mode=easy|full on run and serve. easy => LineupMaxChannels=479, no smoketest at index, no -register-plex; full => -register-plex uses NoLineupCap. Stderr hints updated.
    - internal/tuner/server_test.go: TestUpdateChannels easy-mode cap at 479.
    - docs/features.md: new section 6 "Two flows (easy vs full DVR builder)"; Operations renumbered to 7, Not supported to 8.
  Verification:
    - ./scripts/verify (format, vet, test, build) OK.
  Notes:
    - Easy = add tuner in Plex wizard, pick suggested guide; full = index + smoketest + optional zero-touch with -register-plex.
  Opportunities filed:
    - none
  Links:
    - docs/features.md, cmd/iptv-tunerr/main.go, internal/tuner/server.go

- Date: 2026-02-24
  Title: Zero-touch Plex lineup (programmatic sync, no wizard, no 480 cap)
  Summary:
    - ADR docs/adr/0001-zero-touch-plex-lineup.md: goal = zero human interaction; inject full lineup into Plex DB so wizard not used and 480 limit bypassed.
    - internal/plex/lineup.go: LineupChannel, SyncLineupToPlex(plexDataDir, channels) — discovers channel table in Plex DB, INSERTs in batches of 500; ErrLineupSchemaUnknown if no suitable table.
    - main (run): when -register-plex set, use tuner.NoLineupCap; after RegisterTuner build lineup from live (URL = baseURL + /stream/ + channelID), call SyncLineupToPlex; on schema unknown log skip + ADR; on success log "Lineup synced to Plex: N channels (no wizard needed)".
    - internal/tuner/server.go: PlexDVRMaxChannels=480, NoLineupCap=-1; UpdateChannels caps at LineupMaxChannels unless NoLineupCap; config LineupMaxChannels from env (default 480).
    - Docs: known_issues (480 = wizard path; -register-plex = zero-touch + full sync), features (programmatic lineup sync), adr/index (0001).
  Verification:
    - ./scripts/verify (format, vet, test, build) ✅
    - internal/plex/lineup_test.go (TestSyncLineupToPlex_noSchema, TestSyncLineupToPlex_emptyChannels), internal/tuner/server_test.go (TestUpdateChannels_capsLineup).
  Notes:
    - Schema discovery is heuristic (tables/columns with channel/livetv/lineup, guide_number/guide_name/url). If user's Plex version uses different schema, sync skips; next step: get real Plex DB schema and extend discoverChannelTable or add env override.
  Opportunities filed:
    - none
  Links:
    - docs/adr/0001-zero-touch-plex-lineup.md, internal/plex/lineup.go, cmd/iptv-tunerr/main.go, memory-bank/known_issues.md, docs/features.md, docs/adr/index.md

- Date: 2026-02-24
  Title: Plex in cluster runbook + standup-and-verify (HDHR no-setup flow)
  Summary:
    - Added docs/runbooks/plex-in-cluster.md: check if Plex is in cluster; why missing (not in this repo); where it went (k3s stripped/external); how to restore (k3s repo, Helm, or on-node); verify after restore; full standup (section 6) for no manual setup in Plex.
    - Added k8s/standup-and-verify.sh: deploy via deploy.sh then verify discover.json and lineup.json return 200; exits 1 if kubectl unreachable or endpoints fail.
    - Updated k8s/README.md: prerequisites note Plex (link to runbook); one-command deploy and verify with standup-and-verify.sh; NodePort TUNER_BASE_URL hint.
    - Updated docs/runbooks/index.md, memory-bank/known_issues.md with Plex-not-in-repo and runbook link.
  Verification:
    - bash -n k8s/standup-and-verify.sh ✅
    - kubectl/deploy/curl not run (kubeconfig permission denied in env).
  Notes:
    - Full no-setup flow: Plex data at /var/lib/plex → run Plex once → stop Plex → ./k8s/standup-and-verify.sh → start Plex; then Live TV already configured.
  Opportunities filed:
    - none
  Links:
    - docs/runbooks/plex-in-cluster.md, k8s/standup-and-verify.sh, k8s/README.md, docs/runbooks/index.md, memory-bank/known_issues.md

- Date: 2026-02-24
  Title: Single-script HDHR k8s deploy wrapper (no manifest edits)
  Summary:
    - Added `k8s/deploy-hdhr-one-shot.sh` to inject provider env values into a temporary manifest and run `k8s/deploy.sh`.
    - Updated `k8s/deploy.sh` to accept `MANIFEST=/path/to/file` so wrappers can deploy generated manifests safely.
    - Updated `k8s/README.md` with one-shot script usage and env-based credential injection.
  Verification:
    - `bash -n k8s/deploy.sh k8s/deploy-hdhr-one-shot.sh` ✅
    - `./k8s/deploy-hdhr-one-shot.sh --help` ✅
    - Cluster deploy run is user-side (not run in sandbox).
    - Full `scripts/verify` not run (k8s shell-script/docs scoped change).
  Notes:
    - Wrapper redacts most of the M3U query string in logs and cleans up the temp manifest on exit.
    - Default M3U URL generation assumes Xtream-style `get.php`; pass `IPTV_TUNERR_M3U_URL` or `--m3u-url` to override.
  Opportunities filed:
    - `memory-bank/opportunities.md` (committed provider credentials in tracked k8s manifest)
  Links:
    - k8s/deploy-hdhr-one-shot.sh, k8s/deploy.sh, k8s/README.md

- Date: 2026-02-24
  Title: HDHR k8s standup — deploy script, readiness, Plex setup (Agent 2)
  Summary:
    - Added readinessProbe on /discover.json (initialDelaySeconds 90) so Ingress doesn’t 502 during catalog index.
    - Added k8s/deploy.sh: build image, load into kind/k3d, apply manifest, rollout status; prints verify and Plex setup.
    - Replaced ConfigMap provider creds with placeholders; README documents editing manifest or using a Secret.
    - Expanded k8s/README.md: one-command deploy, provider credentials, DNS/Ingress, step-by-step Plex connect for TV/browser.
  Verification:
    - scripts/verify (format, vet, test, build).
    - deploy.sh is executable; manual kubectl/docker run remains user-side.
  Notes:
    - User must set real provider credentials in k8s/iptvtunerr-hdhr-test.yaml (or use a Secret) before ./k8s/deploy.sh.
    - DNS: iptvtunerr-hdhr.plex.home → Ingress; then Plex at plex.home can add DVR with Base URL and guide.xml.
  Opportunities filed:
    - none
  Links:
    - k8s/iptvtunerr-hdhr-test.yaml, k8s/deploy.sh, k8s/README.md

- Date: 2026-02-24
  Title: HDHR k8s standup for plex.home (Agent 2)
  Summary:
    - Updated k8s/iptvtunerr-hdhr-test.yaml: run-mode (index at startup), emptyDir catalog, BaseURL=http://iptvtunerr-hdhr.plex.home.
    - Added Ingress for iptvtunerr-hdhr.plex.home → iptvtunerr-hdhr-test:5004 (ingressClassName: nginx).
    - Removed static catalog ConfigMap; run indexes from provider at startup.
    - Added k8s/README.md: build, deploy, verify, Plex setup, customization.
  Verification:
    - ./scripts/verify ✅
    - docker build (blocked: sandbox network)
    - kubectl apply (blocked: kubeconfig permission)
  Notes:
    - User must: build image, load into cluster, apply manifests, ensure DNS for iptvtunerr-hdhr.plex.home.
    - NodePort 30004 fallback if Ingress not used.
  Opportunities filed:
    - none
  Links:
    - k8s/iptvtunerr-hdhr-test.yaml, k8s/README.md, docs/index.md, docs/runbooks/index.md

- Date: 2026-02-24
  Title: SSDP discovery URL hardening for Plex auto-discovery (sandbox-tested)
  Summary:
    - Patched `internal/tuner/ssdp.go` to build/validate `DeviceXMLURL` from `BaseURL` instead of blindly emitting `LOCATION: /device.xml` when `BaseURL` is unset.
    - Disabled SSDP startup when `BaseURL` is empty/invalid and added a log message so operators know Plex auto-discovery requires a reachable `-base-url` / `IPTV_TUNERR_BASE_URL`.
    - Added socket-free unit tests for SSDP response formatting, device.xml URL joining, and `/device.xml` handler output.
  Verification:
    - `gofmt -s -w internal/tuner/ssdp.go internal/tuner/ssdp_test.go` ✅
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -list . ./internal/tuner` ✅ (enumerated tests to build package under sandbox)
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -count=1 -run '^(Test(HDHR_|M3UServe_|JoinDeviceXMLURL|SSDP_searchResponse|Server_deviceXML|XMLTV_(serve|404|epgPruneUnlinked)|AdaptiveWriter_|StreamWriter_))' ./internal/tuner` ✅
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -count=1 ./internal/tuner/...` ❌ blocked by sandbox socket policy (`httptest.NewServer` listener in gateway/xmltv tests)
  Notes:
    - This improves real-world Plex discovery behavior when operators forget to set a reachable Base URL; Plex will no longer receive an invalid SSDP `LOCATION`.
    - Live Plex/TV/browser validation still must be run outside this sandbox because local socket binds and network access are denied here.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/ssdp.go, internal/tuner/ssdp_test.go, internal/tuner/server.go
- Date: 2026-02-24
  Title: Core functionality test session (sandbox-constrained; cluster Plex blocked)
  Summary:
    - Resumed testing with scope limited to core/non-HDHR functionality because another agent is actively testing HDHR in the same repo.
    - Read memory-bank state, commands, troubleshooting runbook, and local `k8s/` test manifest to align on expected QA flow and cluster namespace/service usage.
    - Ran a core package test matrix and targeted subtests that avoid socket listeners where possible; documented exact sandbox blockers for cluster access and socket-based tests.
    - Updated `memory-bank/current_task.md` with scope, assumptions, and self-check results for handoff.
  Verification:
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -count=1 ./internal/cache ./internal/config ./internal/indexer` ✅
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -count=1 -run 'TestCheckProvider_emptyURL$' ./internal/health` ✅
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -count=1 -run 'TestParseRetryAfter$' ./internal/httpclient` ✅
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -count=1 -run '^$' ./internal/provider` ✅ (compile-only; no tests run)
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -count=1 -run '^$' ...` ⚠️ partial compile-only smoke: core packages + `internal/tuner` compile; `cmd/iptv-tunerr` and `internal/vodfs` blocked by sandbox DNS/socket while downloading `modernc.org/sqlite` and `github.com/hanwen/go-fuse/v2`
    - `gofmt -s -l internal/tuner` ✅ (no formatting drift)
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go vet ./internal/tuner/...` ✅
    - `kubectl --kubeconfig ~/.kube/config ...` ❌ blocked: sandbox socket policy (`operation not permitted`) to k8s API
    - `go test ... ./internal/plex` ❌ blocked: network/DNS denied while downloading `modernc.org/sqlite`
    - `go test ... ./internal/health ./internal/httpclient ./internal/provider` (full) ❌ blocked: `httptest.NewServer` cannot bind listener (`socket: operation not permitted`)
  Notes:
    - Sandbox cannot perform the requested cluster-side Plex validation from this session; use the same commands outside the sandbox or in a less restricted runner.
    - Even compile-only repo smoke can be incomplete in this sandbox when dependencies are not already cached locally (DNS/socket denied for `proxy.golang.org`).
    - Avoided modifying shared `k8s/` resources to prevent overlap with the concurrent HDHR test session.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, docs/runbooks/iptvtunerr-troubleshooting.md, k8s/iptvtunerr-hdhr-test.yaml

- Date: 2026-02-24
  Title: HDHomeRun emulation tests + SSDP/UDP discovery
  Summary:
    - Added 5 new tests for HDHomeRun emulation in internal/tuner/hdhr_test.go:
      - TestHDHR_discover_defaults: verifies default BaseURL, DeviceID, TunerCount when empty
      - TestHDHR_lineup_explicit_channel_id: verifies explicit ChannelID is used in stream URL
      - TestHDHR_lineup_multiple_channels: verifies multiple channels with mix of explicit ChannelID and fallback to index
      - TestHDHR_lineup_empty: verifies empty channels array returns empty JSON array
      - TestHDHR_not_found: verifies unknown routes return 404
    - Added SSDP/UDP discovery in internal/tuner/ssdp.go:
      - Listens on UDP port 1900 for M-SEARCH requests
      - Responds to ssdp:all, urn:schemas-upnp-org:device:MediaServer, and urn:schemas-upnp-org:device:Basic:1 search types
      - Returns device XML location in LOCATION header
    - Added /device.xml endpoint to Server for UPnP device discovery
  Verification:
    - go vet ./internal/tuner/... ✅
    - go test ./internal/tuner/... ✅
    - Manual SSDP test: responds to M-SEARCH with proper HTTP/UDP response
    - HTTP endpoints: /discover.json, /device.xml, /lineup.json all return 200
  Notes:
    - Plex can now auto-discover the tuner via SSDP (M-SEARCH on port 1900)
    - If SSDP doesn't work on your network (multicast issues), users can manually configure the Base URL
  Opportunities filed:
    - none
  Links:
    - internal/tuner/hdhr_test.go, internal/tuner/ssdp.go, internal/tuner/server.go

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
    - Rewrote README: full feature summary, comparison matrix (IPTV Tunerr vs xTeVe vs Threadfin), commands and env tables, repo layout.
    - Added docs/features.md: canonical feature list (input/indexing, catalog, tuner, EPG, VOD/VODFS, ops, not supported).
    - Added docs/CHANGELOG.md: history from git (merge, IPTV Tunerr content, template).
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
    - gofmt -s -w, go vet ./..., go test ./..., go build ./cmd/iptv-tunerr (scripts/verify).
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
    - cmd/iptv-tunerr: Extracted fetchCatalog(cfg, m3uOverride) helper + catalogStats() — eliminates ~80 lines of copy-paste across index/run-startup/run-scheduled. Bug fix: scheduled refresh now applies LiveEPGOnly filter and smoketest (was silently skipped before).
  Verification:
    - Go not installed locally; no build system available in this environment.
    - Changes are syntactically consistent with existing code patterns; all edited files reviewed.
  Notes:
    - Scheduled-refresh missing filters was a silent bug: if smoketest or EPG-only was enabled, startup index honored them but the background ticker did not. Now all three fetch paths go through the same fetchCatalog().
    - os.Rename is atomic only when src and dst are on the same filesystem; temp file is created in the same directory as the catalog to ensure this.
  Opportunities filed:
    - none
  Links:
    - internal/catalog/catalog.go, internal/catalog/catalog_test.go, internal/config/config.go, cmd/iptv-tunerr/main.go

- Date: 2026-02-24
  Title: Verify pending changes + local Plex-facing smoke test
  Summary:
    - Installed a temporary local Go 1.24.0 toolchain under `/tmp/go` (no system install) to run repo verification in this environment.
    - Ran `scripts/verify` successfully (format, vet, test, build) on the pending uncommitted changes.
    - Applied a format-only `gofmt` fix to `internal/tuner/psi_keepalive.go` (comment indentation) because verify failed on formatting before tests.
    - Ran a local smoke test: generated a catalog from a temporary local M3U, started `serve`, validated `discover.json`, `lineup_status.json`, `lineup.json`, `guide.xml`, `live.m3u`, and fetched one proxied stream URL successfully.
  Verification:
    - `PATH=/tmp/go/bin:$PATH ./scripts/verify`
    - Local smoke: `go run ./cmd/iptv-tunerr index -m3u http://127.0.0.1:<port>/test.m3u -catalog <tmp>` then `go run ./cmd/iptv-tunerr serve ...` + `curl` endpoint checks
    - `GET /stream/<channel-id>` returned `200` and proxied bytes from local dummy upstream
    - Real provider/Plex E2E not run (no `.env` / Plex host available in environment)
  Notes:
    - `./scripts/verify` surfaced an unrelated formatting drift (`internal/tuner/psi_keepalive.go`) that was not part of the pending feature changes but blocks CI-level verification.
    - Local smoke validates the tuner HTTP surface and proxy routing mechanics, but not MPEG-TS compatibility or real Plex session behavior.
  Opportunities filed:
    - none
  Links:
    - scripts/verify, internal/tuner/psi_keepalive.go, docs/runbooks/iptvtunerr-troubleshooting.md

- Date: 2026-02-24
  Title: Live Plex integration triage (plex.home 502, WebSafe guide latency, direct tune)
  Summary:
    - Diagnosed `plex.home` 502 as Traefik backend reachability failure to Plex on `<plex-node>:32400` (Plex itself was healthy; `<work-node>` could not reach `<plex-host-ip>:32400`).
    - Fixed host firewall on `<plex-node>` by allowing LAN TCP `32400` in `inet filter input`, restoring `http://plex.home` / `https://plex.home` (401 unauthenticated expected).
    - Validated from inside the Plex pod that `iptvtunerr-websafe` (`:5005`) is reachable and `iptvtunerr-trial` (`:5004`) is not.
    - Identified `guide.xml` latency root cause: external XMLTV remap (~45s per request). Restarted WebSafe `iptv-tunerr serve` in the lab pod without `IPTV_TUNERR_XMLTV_URL` (placeholder guide) to make `guide.xml` fast again (~0.2s).
    - Proved live Plex→IptvTunerr path works after fixes: direct Plex API `POST /livetv/dvrs/138/channels/11141/tune` returned `200`, and `iptvtunerr-websafe` logged `/stream/11141` with HLS relay first bytes.
  Verification:
    - `curl -I http://plex.home` / `curl -k -I https://plex.home` → `502` before fix, `401` after firewall fix
    - `kubectl` checks on `<work-node>`: `get pods/svc/endpoints`, Plex pod `curl` to `iptvtunerr-websafe.plex.svc:5005`
    - Plex pod timing: `guide.xml` ~45.15s with external XMLTV; ~0.19s after WebSafe restart without XMLTV
    - Plex direct tune API for DVR `138` / channel `11141` returned `200` and produced `/stream/11141` request in `iptvtunerr-websafe` logs
  Notes:
    - Runtime fixes are operational and may not persist across host firewall reloads/pod restarts unless codified in infra manifests/scripts.
    - `iptvtunerrWebsafe` lineup is very large (~41,116 channels); Plex channel metadata APIs remain slow even after `guide.xml` was accelerated.
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
    - The expected high-volume category split is currently blocked by source/EPG linkage, not IptvTunerr or Plex insertion; observed path was ~41,116 source channels -> 188 XMLTV-linked -> 91 deduped.
    - `plex/scripts/plex-activate-dvr-lineups.py` currently crashes on empty DVRs (`No valid ChannelMapping entries found`); workaround is to activate only non-empty DVRs.
  Opportunities filed:
    - `memory-bank/opportunities.md` (split-pipeline stage count instrumentation; empty-DVR activation helper hardening)
  Links:
    - memory-bank/known_issues.md, memory-bank/opportunities.md, <sibling-k3s-repo>/plex/scripts/plex-dvr-setup-multi.sh, <sibling-k3s-repo>/plex/scripts/plex-activate-dvr-lineups.py

- Date: 2026-02-24
  Title: Direct IptvTunerr WebSafe hardening for Plex routing (guide-number fallback + default-safe client adaptation)
  Summary:
    - `internal/tuner/gateway`: Added channel lookup fallback by `GuideNumber` so `/auto/v<guide-number>` works even when the catalog `channel_id` is a non-numeric slug (for example `eurosport1.de`).
    - `internal/tuner/gateway`: Changed Plex client adaptation to a tri-state override model so behavior can explicitly force WebSafe (`transcode on`), explicitly force full path (`transcode off`), or inherit the existing default.
    - New adaptation policy (when `IPTV_TUNERR_CLIENT_ADAPT=true`): explicit query `profile=` still wins; unknown/unresolved Plex client resolution defaults to WebSafe; resolved Plex Web/browser clients use WebSafe; resolved non-web clients force full path.
    - Recorded live direct IptvTunerr findings in memory-bank: real XMLTV + EPG-linked + deduped catalog fixed lineup/guide mismatch (`188 -> 91` unique `tvg-id` rows) and removed the "Unavailable Airings" mismatch root cause; remaining browser issue is Plex Web DASH `start.mpd` timeout after successful tune/relay.
  Verification:
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go test ./internal/tuner -run 'TestGateway_(requestAdaptation|autoPath)' -count=1`
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go build ./cmd/iptv-tunerr`
    - `PATH=/tmp/go/bin:$PATH ./scripts/verify` (fails on unrelated repo-wide format drift in tracked files: `internal/config/config.go`, `internal/hdhomerun/control.go`, `internal/hdhomerun/packet.go`, `internal/hdhomerun/server.go`)
  Notes:
    - The client-adaptation behavior change is gated by `IPTV_TUNERR_CLIENT_ADAPT`; deployments with the flag disabled retain prior behavior.
    - Full verification is not green due unrelated formatting drift outside this patch scope; this change set itself is `gofmt`-clean and builds/tests cleanly.
  Opportunities filed:
    - `memory-bank/opportunities.md` (built-in direct-catalog dedupe/alignment for XMLTV-remapped Plex lineups)
  Links:
    - internal/tuner/gateway.go, internal/tuner/gateway_test.go, memory-bank/known_issues.md, memory-bank/opportunities.md

- Date: 2026-02-24
  Title: Re-establish direct IptvTunerr DVRs after Plex restart (Trial URI fix + remap) and re-test browser playback
  Summary:
    - Avoided Plex restarts and pod restarts; restarted only the two `iptv-tunerr serve` processes in the existing `iptvtunerr-build` pod (`:5004` Trial, `:5005` WebSafe) using `/workspace/iptv-tunerr.policy`, the deduped direct catalog (`catalog-websafe-dedup.json`), and real XMLTV (`iptv-m3u-server`).
    - Verified both direct tuner services were healthy again (`discover.json`, `lineup.json`) and served the 91-channel deduped catalog with real XMLTV remap enabled.
    - `DVR 138` (`iptvtunerrWebsafe`) activation confirmed healthy (`before=91`, `after=91`).
    - Diagnosed `DVR 135` (`iptvtunerrTrial`) zero-channel state as a wrong HDHomeRun device URI in Plex (`http://127.0.0.1:5004` instead of `http://iptvtunerr-trial.plex.svc:5004`).
    - Fixed Trial in place by re-registering the HDHomeRun device to `iptvtunerr-trial.plex.svc:5004`, then `reloadGuide` + `plex-activate-dvr-lineups.py --dvr 135`, which restored `after=91`.
    - Re-ran Plex Web probes on both `DVR 138` and `DVR 135`: both now `tune=200` but still fail at `startmpd1_0`. Trial logs confirm the client-adaptation switch is active and defaults unknown clients to websafe mode (`reason=unknown-client-websafe`).
    - Collected matching Plex logs showing the remaining browser failure is Plex-side: `decision` and `start.mpd` requests complete only after long waits, followed by `Failed to start session.`, while IptvTunerr logs show successful `/stream/...` byte relay.
  Verification:
    - k3s runtime checks via `sudo kubectl` on `<work-node>` (Plex pod + `iptvtunerr-build` pod): endpoint health, log tails, DVR/device detail XML
    - `sudo python3 <sibling-k3s-repo>/plex/scripts/plex-activate-dvr-lineups.py --dvr 138`
    - `sudo python3 <sibling-k3s-repo>/plex/scripts/plex-activate-dvr-lineups.py --dvr 135`
    - `sudo python3 <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel-id 112`
    - `sudo python3 <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py --dvr 135 --channel-id 112`
  Notes:
    - The probe script `plex-dvr-random-stream-probe.py` reported timeout/0-byte failures on direct `/stream/...` URLs due its fixed 60s timeout, but IptvTunerr logs for the same probes show HTTP 200 and tens/hundreds of MB relayed over ~60–130s; use tuner logs as the source of truth for those runs.
    - Another agent is actively changing `internal/hdhomerun/*`; no code changes were made in that area and no Plex restarts were performed.
  Opportunities filed:
    - none
  Links:
    - memory-bank/known_issues.md, memory-bank/recurring_loops.md, <sibling-k3s-repo>/plex/scripts/plex-activate-dvr-lineups.py, <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py

- Date: 2026-02-24
  Title: WebSafe ffmpeg-path triage (k3s ffmpeg DNS failure, startup-gate fallback, hidden Plex CaptureBuffer reuse)
  Summary:
    - Restarted direct WebSafe/Trial tuner processes in the existing `iptvtunerr-build` pod using `/workspace/iptv-tunerr-9514357` (no Plex restart), real XMLTV, and the 91-channel deduped direct catalog.
    - Confirmed the fresh WebSafe runtime binary now logs ffmpeg/websafe diagnostics; browser probes still fail with `startmpd1_0` and `start.mpd` debug XML continues to return `CaptureBuffer` with empty `sourceVideoCodec/sourceAudioCodec`.
    - Found a concrete WebSafe blocker: ffmpeg transcode startup failed on HLS input URLs that use the k3s short service hostname (`iptv-hlsfix.plex.svc`), causing IptvTunerr to fall back to the Go raw relay path.
    - Verified the ffmpeg DNS issue is specific to the ffmpeg HLS input path (Go HTTP fetches to the same hostname work). Runtime workaround: created `/workspace/catalog-websafe-dedup-ip.json` with HLSFix hostnames rewritten to the numeric service IP (`10.43.210.255:8080`) and restarted WebSafe on that catalog.
    - After the numeric-host workaround, ffmpeg + PAT/PMT keepalive started successfully, but the WebSafe ffmpeg startup gate timed out (no ffmpeg payload before timeout), emitted timeout bootstrap TS, then still fell back to the Go raw relay.
    - Tuned WebSafe ffmpeg/startup envs (`FFMPEG_HLS_*`, startup timeout 30s, smaller startup min bytes) and restarted WebSafe again for follow-up testing; hidden Plex `CaptureBuffer` session reuse on repeated channels limited clean validation of the tuned path.
    - Found a second major test-loop blocker: Plex can reuse hidden `CaptureBuffer`/transcode state not visible in `/status/sessions` or `/transcode/sessions`. `plex-live-session-drain.py --all-live` can report clean, but repeated probes on the same channel reuse the same `TranscodeSession` and do not hit IptvTunerr `/stream/...` again.
    - Confirmed `universal/stop?session=<id>` returns `404` for those hidden reused `TranscodeSession` IDs (examples: `8af250...`, `24b5e1...`, `07b8aa...`).
    - Restarted Trial with client-adapt enabled plus `IPTV_TUNERR_HLS_RELAY_FFMPEG_STDIN_NORMALIZE=true`, explicit ffmpeg path, numeric HLSFix catalog, and the same tuned ffmpeg/startup envs to set up a second DVR for fresh-channel browser tests.
  Verification:
    - `sudo kubectl -n plex exec pod/iptvtunerr-build-... -- ...` process restarts/checks for `:5004` and `:5005`
    - `sudo env PWPROBE_DEBUG_MPD=1 python3 .../plex-web-livetv-probe.py` on DVRs `138` and `135` (channels `112`, `111`, `108`, `109`, `107`, `104`, `103`, `26289`)
    - WebSafe/Trial tuner log correlation (`/tmp/iptvtunerr-websafe.log`, `/tmp/iptvtunerr-trial.log`) including `ffmpeg-transcode`, `pat-pmt-keepalive`, fallback reasons, and `/stream/...` presence/absence
    - Plex API checks from helper snippets: `/status/sessions`, `/transcode/sessions`, and explicit `universal/stop?session=<id>` attempts for hidden reused session IDs
  Notes:
    - Runtime-only test changes are not durable: WebSafe/Trial envs were changed in-process, and the numeric-host catalog copy (`catalog-websafe-dedup-ip.json`) exists only in the pod filesystem.
    - Hidden Plex `CaptureBuffer` reuse can invalidate repeated probe runs on the same channel; only probes that generate a new tuner `/stream/...` request should be used to judge tuner runtime changes.
    - No Plex pod restart was performed.
  Opportunities filed:
    - `memory-bank/opportunities.md` (ffmpeg HLS host canonicalization before ffmpeg; stronger stale-session detection in Plex probe/drain helpers)
  Links:
    - memory-bank/known_issues.md, memory-bank/recurring_loops.md, memory-bank/opportunities.md, <sibling-k3s-repo>/plex/scripts/plex-live-session-drain.py, <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py

- Date: 2026-02-24
  Title: Add ffmpeg HLS input host canonicalization in gateway (k3s short-host compatibility)
  Summary:
    - `internal/tuner/gateway.go`: Added `canonicalizeFFmpegInputURL(...)` to resolve the ffmpeg HLS input host in Go and rewrite the ffmpeg input URL to a numeric host before spawning ffmpeg.
    - `relayHLSWithFFmpeg(...)` now uses the rewritten ffmpeg input URL (when resolution succeeds) and logs `input-host-resolved <host>=><ip>` for visibility.
    - This is a direct code response to the live k3s WebSafe failure where ffmpeg could not resolve `iptv-hlsfix.plex.svc` and IptvTunerr fell back to the raw relay path.
  Verification:
    - `/tmp/go/bin/gofmt -w internal/tuner/gateway.go`
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go test ./internal/tuner -count=1`
  Notes:
    - The patch is currently local-only and not yet rebuilt/deployed into the `iptvtunerr-build` pod runtime.
    - Runtime validation still needs a fresh Plex browser probe that actually reaches a new tuner `/stream/...` request (hidden `CaptureBuffer` reuse can mask the change).
  Opportunities filed:
    - none (covered by existing ffmpeg host canonicalization + probe-helper entries)
  Links:
    - internal/tuner/gateway.go, memory-bank/current_task.md, memory-bank/known_issues.md

- Date: 2026-02-24
  Title: Fix WebSafe ffmpeg HLS reconnect-loop startup failure and re-validate live payload path
  Summary:
    - Continued direct IptvTunerr WebSafe browser-path triage without restarting Plex; restarted only the WebSafe `iptv-tunerr serve` process in `iptvtunerr-build` multiple times for env/runtime experiments.
    - Reproduced the ffmpeg startup stall manually inside the pod using `/workspace/ffmpeg-static` against the HLSFix live playlist and found the real blocker: generic ffmpeg HTTP reconnect flags on live HLS (`-reconnect*`) caused repeated `.m3u8` EOF reconnect loops (`Will reconnect at 1071 ... End of file`) and delayed/failed first-segment loading.
    - Confirmed the same manual ffmpeg command succeeds immediately when reconnect flags are removed (opens HLS segment, writes valid MPEG-TS file, exits cleanly).
    - Patched `internal/tuner/gateway.go` so `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` defaults to `false` for HLS ffmpeg inputs (env override still supported); this preserves the earlier ffmpeg host-canonicalization fix and avoids the live playlist reconnect loop by default.
    - Built a clean temporary runtime binary from `HEAD` plus only `internal/tuner/gateway.go` (to avoid including another agent's HDHomeRun WIP), deployed it into the `iptvtunerr-build` pod as `/workspace/iptv-tunerr-websafe-fix`, and restarted only WebSafe (`:5005`) in place.
    - Re-ran Plex Web probe on `DVR 138` / channel `106`: probe still fails `startmpd1_0`, but WebSafe logs now show the ffmpeg path is genuinely working (`reconnect=false`, `startup-gate-ready`, `first-bytes`, and long ffmpeg stream runs with multi-MB payload).
    - Additional WebSafe runtime tuning (`REQUIRE_GOOD_START=true`, larger startup timeout/prefetch, and later `HLS_LIVE_START_INDEX=-3`) still showed startup-gate buffers with `idr=false aac=true`; browser probes continued to fail `startmpd1_0`, shifting the main blocker from ffmpeg startup to early video/keyframe readiness vs Plex's live packager timeout.
    - Hit an unrelated k3s control-plane issue during later probe retries: `kubectl exec` to the Plex pod intermittently returned `502 Bad Gateway`, which temporarily blocked the probe helper's token-read step.
  Verification:
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go test ./internal/tuner -count=1`
    - Manual ffmpeg repro (inside `iptvtunerr-build` pod) with reconnect flags enabled: repeated playlist EOF reconnect loop (`Will reconnect at 1071 ...`)
    - Manual ffmpeg control (same pod/channel) without reconnect flags: opened HLS segment and wrote valid TS (`/tmp/manual106.ts`, ~3.9 MB in ~6s)
    - `python3 <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel 106` (via temporary `kubectl` wrapper to `sudo k3s kubectl`) before and after runtime tuning
    - WebSafe log correlation in `/tmp/iptvtunerr-websafe.log` confirming `reconnect=false`, `startup-gate-ready`, `first-bytes`, and `ffmpeg-transcode bytes/client-done` payload sizes
  Notes:
    - No Plex restart was performed.
    - Trial process was left running and was not restarted during this cycle.
    - Late probe retries were partially blocked by transient k3s `kubectl exec` proxy `502` errors to the Plex pod.
  Opportunities filed:
    - `memory-bank/opportunities.md` (IDR-aware live HLS startup strategy for WebSafe ffmpeg path)
  Links:
    - internal/tuner/gateway.go, memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md, memory-bank/opportunities.md

- Date: 2026-02-24
  Title: Restore `plex.home` via manual endpoint slice during `<plex-node>` read-only-root outage (no Plex restart)
  Summary:
    - Investigated `https://plex.home` `503` and found the Plex host node `<plex-node>` was `NotReady`; the Plex pod on `<plex-node>` was stuck `Terminating` and the Service had no ready endpoints.
    - Confirmed the host Plex process itself was still alive on `<plex-host-ip>:32400` (direct HTTP returned Plex `401` unauth).
    - Diagnosed `k3s` startup failure on `<plex-node>`: root Btrfs (`/`) was mounted read-only, and foreground `k3s server` failed with `failed to bootstrap cluster data ... chmod kine.sock: read-only file system`.
    - Confirmed the replacement Plex pod on `<work-node>` could not start because NFS mounts from `<plex-host-ip>` failed (`Host is unreachable`), leaving the `EndpointSlice` endpoint `ready=false`.
    - Restored `plex.home` without restarting Plex by patching Service `plex` to be selectorless and attaching a manual `EndpointSlice` to `<plex-host-ip>:32400`; `https://plex.home` returned `401` afterward.
  Verification:
    - `curl -k -I https://plex.home` (before: `503`, after: `401`)
    - `ssh <work-node> 'sudo k3s kubectl get nodes -o wide'`
    - `ssh <work-node> 'sudo k3s kubectl -n plex get svc/endpoints/endpointslice ...'`
    - `ssh <user>@<plex-node> 'findmnt -no TARGET,SOURCE,FSTYPE,OPTIONS /'`
    - `ssh <user>@<plex-node> 'timeout 20s /usr/local/bin/k3s server ...'` (foreground capture of `kine.sock` read-only failure)
  Notes:
    - This is a temporary traffic-routing workaround only. `<plex-node>` still needs host-level filesystem recovery (root Btrfs back to `rw`) and `k3s` restart.
    - After host recovery, restore the normal `plex` Service selector (`app=plex`) and remove the manual `EndpointSlice`.
    - No Plex process restart was performed.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md

- Date: 2026-02-24
  Title: Persist `<plex-node>` LAN Plex/NFS firewall allows in boot-loaded nftables config and restore Plex after reboot
  Summary:
    - Rebooted `<plex-node>` to recover the root Btrfs `ro` remount condition; confirmed `/` returned `rw` and `postgresql` + `k3s` were active after boot.
    - Found the post-reboot regression was the same firewall persistence issue: `/etc/nftables/<plex-node>-host-firewall.conf` still contained Plex/NFS allows, but the later `table inet filter` base chain from `/etc/nftables.conf` dropped LAN Plex/NFS traffic.
    - Added temporary live `nft` rules to `inet filter input` to restore LAN access for NFS/Plex (`111/2049/20048/.../32400`) and re-established `<work-node> -> <plex-node>` NFS RPC connectivity.
    - Patched `/etc/nftables.conf` (the file loaded by `nftables.service`) to persist the LAN Plex/NFS allow rules in the actual `inet filter input` chain so they survive future reboot/reload.
    - Restored normal Plex service routing (selector-based Service, removed temporary manual `EndpointSlice`), deleted the stuck pending Plex pod, and verified a new Plex pod came up on `<plex-node>` and `https://plex.home` returned `401`.
  Verification:
    - `ssh <user>@<plex-node> 'findmnt -no OPTIONS /; systemctl is-active postgresql k3s'`
    - `ssh <user>@<plex-node> 'sudo nft -c -f /etc/nftables.conf'`
    - `ssh <work-node> 'rpcinfo -p <plex-host-ip> && showmount -e <plex-host-ip>'`
    - `ssh <work-node> 'sudo k3s kubectl -n plex get pod -o wide'`
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
    - Verified the post-reboot `<plex-node>` LAN access fixes are truly persistent: `/etc/nfs.conf` still pins `lockd`/`mountd`/`statd` ports, `inet filter input` still contains the matching NFS + Plex `32400` allow rules, and `<work-node> -> <plex-node>` `rpcinfo`/`showmount` succeeds.
    - Confirmed direct WebSafe service is up and resumed fresh browser probes without restarting Plex; Trial (`:5004`) was down during this cycle and intentionally left untouched to minimize disruption.
    - Reproduced the Web browser failure on fresh WebSafe channels `103` and `104` with new hidden Plex `CaptureBuffer` sessions (`startmpd1_0` at ~35s), while IptvTunerr logs showed healthy ffmpeg startup and real streamed bytes (`startup-gate` ready, `first-bytes`, `idr=true` in the `103/104` runs).
    - Demonstrated that Plex `decision` / `start.mpd` for the `103` and `104` sessions can complete only after ~100s (PMS logs), which is longer than the probe/browser startup timeout.
    - Captured the key blocker directly: Plex's internal `http://127.0.0.1:32400/livetv/sessions/<live>/<client>/index.m3u8` timed out with **0 bytes** during repeated in-container polls, even while the first-stage recorder wrote many `media-*.ts` segments and Plex accepted `progress/stream` + `progress/streamDetail` callbacks.
    - PMS logs for session `ebbb9949-...` (channel `104`) repeatedly logged `buildLiveM3U8: no segment info available` while the internal live `index.m3u8` remained empty, confirming the bottleneck is Plex's segment-info/manifest readiness, not tuner throughput.
    - Ran two profile-comparison experiments on WebSafe (runtime-only process restarts inside `iptvtunerr-build`, no Plex restart):
      - `plexsafe` (via client adaptation) on channel `107` still failed `startmpd1_0`.
      - Forced `pmsxcode` with `IPTV_TUNERR_CLIENT_ADAPT=false` on channel `109` also failed `startmpd1_0`; PMS first-stage progress confirmed the codec path really changed (`mpeg2video` + `mp2`), but the browser timeout remained and the internal live `index.m3u8` still timed out with 0 bytes.
    - Restored the WebSafe runtime to the baseline test profile afterward (`aaccfr` default + client adaptation enabled, explicit ffmpeg path, HLS reconnect=false, no bootstrap/keepalive), again without restarting Plex.
  Verification:
    - `ssh <user>@<plex-node> 'grep -n ... /etc/nfs.conf; sudo nft list chain inet filter input; rpcinfo -p localhost'`
    - `ssh <work-node> 'rpcinfo -p <plex-host-ip>; showmount -e <plex-host-ip>'`
    - `kubectl -n plex get pods -o wide`, `kubectl -n plex exec deploy/plex -c plex -- curl .../discover.json`
    - `sudo env PWPROBE_DEBUG_MPD=1 python3 <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel {103,104,107,109}`
    - `kubectl -n plex exec deploy/plex -c plex -- curl http://127.0.0.1:32400/livetv/sessions/<live>/<client>/index.m3u8?...` (in-container internal live-manifest polling)
    - PMS log correlation in `/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log` for `buildLiveM3U8`, recorder segment sessions, and delayed `decision`/`start.mpd`
    - WebSafe runtime log correlation in `/tmp/iptvtunerr-websafe.log` for effective profile (`aaccfr` / `plexsafe` / `pmsxcode`) and startup-gate readiness
  Notes:
    - Multiple WebSafe runtime restarts were process-only inside `iptvtunerr-build` (no pod restart, no Plex restart).
    - One experiment initially left duplicate WebSafe processes due pod shell/process-tooling quirks; runtime was restored and the log confirms the final baseline restart (`default=aaccfr`, client adaptation enabled).
    - The strongest current evidence is Plex-side: first-stage recorder healthy + internal live HLS manifest empty (`0 bytes`) + repeated `buildLiveM3U8 no segment info`.
  Opportunities filed:
    - `memory-bank/opportunities.md` (TS timing/continuity debug capture for first-seconds WebSafe output)
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md, memory-bank/opportunities.md, <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py

- Date: 2026-02-24
  Title: Instrument first-seconds WebSafe TS output and confirm clean startup TS on a fresh failing Plex Web probe
  Summary:
    - Added a TS inspector (`internal/tuner/ts_inspector.go`) and hooked it into the ffmpeg relay output path in `internal/tuner/gateway.go` so IptvTunerr can log first-packet TS timing/continuity (PAT/PMT/PCR/PTS/DTS/CC/discontinuity) for targeted probe requests.
    - Built an instrumented binary locally (`/tmp/iptv-tunerr-tsinspect`), copied it into the existing `iptvtunerr-build` pod, and restarted only the WebSafe process (`:5005`) with the same runtime env plus `IPTV_TUNERR_TS_INSPECT=true` and `IPTV_TUNERR_TS_INSPECT_CHANNEL=111`.
    - Ran a fresh Plex Web probe (`plex-web-livetv-probe.py --dvr 138 --channel 111`) and reproduced the browser failure (`detail=startmpd1_0`, ~39s elapsed) without restarting Plex.
    - Captured the new TS inspector summary for the failing probe (`req=r000001`, channel `111` / `skysportnews.de`): first 12,000 TS packets had `sync_losses=0`, PAT/PMT repeated (`175` each), `pcr_pid=0x100`, monotonic PCR/PTS on H264 video PID `0x100`, monotonic PTS on audio PID `0x101`, `idr=true` at startup gate, and no continuity errors on media PIDs (only null PID `0x1FFF` duplicate CCs).
    - Correlated PMS logs for the same live session (`c5a1eca7-f15b-4b84-b22a-fac76d1e5391` / client `157b3117a4354af68c19d075`): first-stage recorder session started in ~3.1s, Plex accepted `progress/stream` + `progress/streamDetail` (H264 + MP3), but `decision` and `start.mpd` still completed only after ~100s, when PMS finally launched the second-stage DASH transcode reading `http://127.0.0.1:32400/livetv/sessions/.../index.m3u8`.
  Verification:
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go test ./internal/tuner -run '^$' -count=1` (compile-only pass)
    - `python3 /home/coder/code/k3s/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel 111 --hold 3 --json-out /tmp/probe-138-111.json` (expected fail: `startmpd1_0`)
    - `kubectl -n plex exec deploy/iptvtunerr-build -- tail/grep /tmp/iptvtunerr-websafe-tsinspect.log`
    - `kubectl -n plex exec -c plex deploy/plex -- grep ... \"Plex Media Server.log\"` (session `c5a1eca7-...`)
  Notes:
    - No Plex restart. Only the WebSafe `iptv-tunerr serve` process inside `iptvtunerr-build` was restarted.
    - The instrumented WebSafe process is left running and TS logging is scoped to guide number/channel match `111` only.
    - Full `go test ./internal/tuner` currently fails due an unrelated pre-existing test (`TestLooksLikeGoodTSStartDetectsSplitIDRStartCodeAcrossPackets`); the new TS inspector code path compiles.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/ts_inspector.go, internal/tuner/gateway.go, memory-bank/current_task.md, memory-bank/known_issues.md

- Date: 2026-02-24
  Title: Confirm first-stage Plex `ssegment` cache fills while internal live `index.m3u8` stays empty on fresh channel `108`
  Summary:
    - Ran another fresh WebSafe browser probe (`DVR 138`, channel `108`) and reproduced the same browser failure (`startmpd1_0`, ~39s elapsed) without restarting Plex.
    - Captured the live first-stage PMS session IDs from logs during the probe: client/live session `ff10b85acd744371a37b94ff` and transcode cache session `dfeb3d9f-85b7-4d4e-beb6-149addd22d6f`.
    - While the probe was still failing, inspected the PMS transcode cache directory `.../plex-transcode-dfeb3d9f-...` and found dozens of generated `media-*.ts` segments with healthy non-zero sizes (through `media-00037.ts`) plus a current in-progress `media-00038.ts` at `0` bytes.
    - At the same time, an in-container `curl -m 5` to Plex's internal `http://127.0.0.1:32400/livetv/sessions/dfeb3d9f-.../ff10b85.../index.m3u8?...` timed out with `0 bytes`.
    - Checked PMS logs for the same first-stage session: the `Plex Transcoder` `ssegment` command includes the expected `-segment_list .../manifest?...` callback URL and PMS logs many `/progress` callbacks for that first-stage transcode session, but no visible `/video/:/transcode/session/.../manifest` request lines appear in `Plex Media Server.log`.
  Verification:
    - `python3 /home/coder/code/k3s/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel 108 --hold 3 --json-out /tmp/probe-138-108.json` (expected fail: `startmpd1_0`)
    - `kubectl -n plex exec -c plex deploy/plex -- grep \"Grabber/108-\" \".../Plex Media Server.log\"`
    - `kubectl -n plex exec -c plex deploy/plex -- ls -lah \".../plex-transcode-dfeb3d9f-...\"`
    - `kubectl -n plex exec -c plex deploy/plex -- curl -m 5 http://127.0.0.1:32400/livetv/sessions/dfeb3d9f-.../ff10b85.../index.m3u8?...`
    - `kubectl -n plex exec -c plex deploy/plex -- grep -E \".../manifest|.../progress\" \".../Plex Media Server.log\"`
  Notes:
    - This strengthens the hypothesis that the remaining Plex Web blocker is in Plex's internal segment-info/manifest path (between first-stage `ssegment` output files and `/livetv/sessions/.../index.m3u8` readiness), not in IptvTunerr stream startup.
    - The WebSafe process remains instrumented, but TS inspection is still scoped to channel match `111`; the `108` probe did not add TS-inspector log noise.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, /tmp/probe-138-108.json

- Date: 2026-02-25
  Title: Recover dead direct Trial/WebSafe DVR backends and repair Plex device URI drift (no Plex restart)
  Summary:
    - Took over after repeated Plex Web probe loops and re-validated the live state first.
    - Found the immediate direct-DVR outage was operational drift: `iptvtunerr-trial` and `iptvtunerr-websafe` services still existed but had no endpoints because the ad hoc `app=iptvtunerr-build` pod was gone.
    - Found both direct DVR devices in Plex (`135` Trial and `138` WebSafe) had also drifted to the wrong HDHomeRun URI (`http://iptvtunerr-otherworld.plex.svc:5004`) while their lineup URLs still pointed to the direct service guide URLs.
    - Recovered a temporary direct runtime without restarting Plex by creating a lightweight helper deployment `iptvtunerr-build` (label `app=iptvtunerr-build`) in the `plex` namespace, copying a fresh static `iptv-tunerr` binary into `/workspace`, generating a shared live catalog from provider API credentials, and launching Trial (`:5004`) + WebSafe (`:5005`) `serve` processes with `IPTV_TUNERR_LINEUP_MAX_CHANNELS=-1`.
    - Re-registered the direct HDHomeRun device URIs in-place via Plex API to `http://iptvtunerr-trial.plex.svc:5004` and `http://iptvtunerr-websafe.plex.svc:5005` (no DVR recreation).
    - Verified Plex resumed polling both direct tuners (`GET /discover.json` + `GET /lineup_status.json`) from `PlexMediaServer` immediately after the URI repair.
    - Identified the next blocker in this temporary recovered state: `reloadGuide` on both direct DVRs triggers slow `/guide.xml` fetches, and the large 7,764-channel catalog plus external XMLTV read timeouts (~45s) causes IptvTunerr to fall back to placeholder guide XML, which stalls guide/channelmap-heavy helper scripts.
  Verification:
    - `kubectl --kubeconfig ~/.kube/config -n plex get endpoints iptvtunerr-trial iptvtunerr-websafe -o wide` (before: `<none>`, after: helper pod IP with `:5004`/`:5005`)
    - `kubectl --kubeconfig ~/.kube/config -n plex exec deploy/plex -c plex -- wget -qO- http://iptvtunerr-{trial,websafe}.plex.svc:{5004,5005}/{discover.json,lineup_status.json}`
    - `curl -k -X POST https://plex.home/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=http://iptvtunerr-{trial,websafe}.plex.svc:{5004,5005}&X-Plex-Token=...`
    - `curl -k https://plex.home/livetv/dvrs/{135,138}?X-Plex-Token=...` (device URI updated in place)
    - Helper pod logs `/tmp/iptvtunerr-trial.log` and `/tmp/iptvtunerr-websafe.log` showing new `PlexMediaServer` requests after repair
  Notes:
    - Recovery is runtime-only and temporary; the recreated `iptvtunerr-build` deployment is a simple helper pod, not the prior instrumented `iptvtunerr-build` workflow.
    - The helper runtime currently serves a large EPG-linked catalog (`7,764` channels), not the earlier 91-channel dedup direct-test catalog, so direct DVR guide/metadata operations are slower and can hit XMLTV timeout fallbacks.
    - No Plex restart performed.
    - No code changes in this repo besides memory-bank updates.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md

- Date: 2026-02-25
  Title: Revalidate direct guide/tune path, restore WebSafe ffmpeg in helper pod, and patch relay/env parsing bugs
  Summary:
    - Re-read source (`internal/tuner/gateway.go`, `internal/tuner/xmltv.go`, `internal/config/config.go`) and revalidated live behavior from runtime logs instead of relying on stale notes.
    - Confirmed direct guide serving is currently using local `iptv-m3u-server` feeds and returns real XMLTV quickly (~70 MB in ~1.4–2.5s from Plex requests); `/guide.xml` no longer shows the earlier placeholder/timeout behavior in the current helper runtime.
    - Proved Plex Web "Failed to tune" is not a tune failure in the current state: `POST /livetv/dvrs/138/channels/108/tune` returns `200`, IptvTunerr receives `/stream/skysportsf1.uk`, and streams first bytes within a few seconds, but Plex Web probe still fails later at `startmpd1_0`.
    - Found a new operational regression in the ad hoc helper pod: WebSafe had no `ffmpeg`, so `IPTV_TUNERR_STREAM_TRANSCODE=true` silently degraded to the Go HLS relay path.
    - Installed `ffmpeg` in the helper pod (`apt-get install -y ffmpeg`) and restarted only the WebSafe `serve` process with `IPTV_TUNERR_FFMPEG_PATH=/usr/bin/ffmpeg`; confirmed `ffmpeg-transcode` logs with startup gate `idr=true aac=true`, but Plex Web still failed `startmpd1_0`, strengthening the Plex-internal packaging diagnosis.
    - Patched code:
      - `internal/tuner/gateway.go`: treat client disconnect write errors during HLS relay as `client-done` instead of propagating a false relay failure/`502`.
      - `internal/config/config.go`: normalize escaped `\\&` in URL env vars (`IPTV_TUNERR_M3U_URL`, `IPTV_TUNERR_XMLTV_URL`, `IPTV_TUNERR_PROVIDER_URL(S)`).
  Verification:
    - `kubectl --kubeconfig ~/.kube/config -n plex get svc,ep iptvtunerr-trial iptvtunerr-websafe iptv-m3u-server iptv-hlsfix`
    - `kubectl --kubeconfig ~/.kube/config -n plex exec deploy/iptvtunerr-build -- tail -n ... /tmp/iptvtunerr-{trial,websafe}.log`
    - `python3 <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py --namespace plex --target deploy/plex --container plex --dvr 138 --channel-id 108 --hold 4` (still fails `startmpd1_0`, but tune=200 + ffmpeg-transcode confirmed)
    - `go test ./internal/config`
    - `go test ./internal/tuner -run '^$'` (compile-only pass)
    - `go test ./internal/tuner ./internal/config` (known unrelated failure in `TestLooksLikeGoodTSStartDetectsSplitIDRStartCodeAcrossPackets`)
  Notes:
    - `POST /livetv/dvrs/138/reloadGuide` triggered a fresh `/guide.xml` fetch in WebSafe logs, but Plex `DVR 138` `refreshedAt` did not change immediately; this field is not reliable proof of guide fetch success.
    - Runtime changes in the helper pod (installing `ffmpeg`, restarting WebSafe) are temporary and not yet codified in manifests.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, internal/config/config.go, memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md

- Date: 2026-02-25
  Title: Restore 13-category pure IptvTunerr injected DVRs, activate all lineups, and prove Smart TV/browser failures are still Plex packager-side
  Summary:
    - Pivoted to the user-requested pure IptvTunerr injected DVR path (no Threadfin in active device/lineup URLs) and inspected runtime/code state directly instead of relying on prior notes.
    - Found the immediate cause of empty `iptvtunerr-*` category tuners was upstream generated `dvr-*.m3u` files being zeroed by the `iptv-m3u-server` postvalidation step; reran only the splitter to restore non-empty category M3Us, then restarted all 13 `iptvtunerr-*` deployments.
    - Deleted the earlier mixed-mode DVRs (IptvTunerr device + Threadfin lineup) and recreated 13 pure-app DVRs pointing both device and lineup/guide at `iptvtunerr-*` services: IDs `218,220,222,224,226,228,230,232,234,236,238,240,242`.
    - Ran `plex-activate-dvr-lineups.py` across all 13 new DVRs; activation finished `status=OK` with mapped channel counts: `218=44`, `220=136`, `222=308`, `224=307`, `226=257`, `228=206`, `230=212`, `232=111`, `234=465`, `236=52`, `238=479`, `240=273`, `242=404` (total `3254`).
    - Probed a pure category DVR (`218` / `iptvtunerr-newsus`) and confirmed the same failure class remains: `tune=200`, IptvTunerr serves `/stream/News12Brooklyn.us`, but Plex Web probe still fails `startmpd1_0`.
    - Pulled Smart TV/Plex logs (client `<client-ip-a>`) and confirmed the same sequence during user-visible spinning: Plex starts the grabber and reads a IptvTunerr stream successfully, then PMS internal `/livetv/sessions/.../index.m3u8` returns `500` with `buildLiveM3U8: no segment info available`, and the client reports `state=stopped`.
    - Removed non-essential `Threadfin` wording in this repo's code/log text and k8s helper comments (`internal/plex/dvr.go`, `cmd/iptv-tunerr/main.go`, `k8s/deploy-hdhr-one-shot.sh`, `k8s/standup-and-verify.sh`, `k8s/README.md`), leaving only comparison docs / historical/context references.
  Verification:
    - `KUBECONFIG=$HOME/.kube/config python3 <sibling-k3s-repo>/plex/scripts/plex-activate-dvr-lineups.py --namespace plex --target deploy/plex --container plex --dvr 218 --dvr 220 --dvr 222 --dvr 224 --dvr 226 --dvr 228 --dvr 230 --dvr 232 --dvr 234 --dvr 236 --dvr 238 --dvr 240 --dvr 242` (final `status=OK`)
    - `KUBECONFIG=$HOME/.kube/config python3 <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py --namespace plex --target deploy/plex --container plex --dvr 218 --per-dvr 1 --json-out /tmp/pure218-probe.json` (expected fail: `startmpd1_0`; tune success + IptvTunerr stream request observed)
    - `KUBECONFIG=$HOME/.kube/config kubectl -n plex logs deploy/iptvtunerr-newsus --since=5m` (shows `/stream/News12Brooklyn.us` startup during pure-app probe)
    - `KUBECONFIG=$HOME/.kube/config kubectl -n plex exec deploy/plex -c plex -- grep ... \"Plex Media Server*.log\"` (Smart TV and probe session logs showing `buildLiveM3U8` / delayed `start.mpd`)
    - `rg -ni --hidden --glob '!.git' 'threadfin' .` (post-cleanup scan; remaining refs are comparison docs, memory-bank history/context, or explicit legacy-secret context)
  Notes:
    - Old Threadfin-era DVRs (`141..177`) may still exist in Plex as separate historical entries and can confuse UI selection; they were not deleted in this pass.
    - The pure-app injected DVRs now point to `iptvtunerr-*.plex.svc:5004` and are channel-activated, but user-facing playback is still blocked by Plex internal Live TV packaging readiness.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md, cmd/iptv-tunerr/main.go, internal/plex/dvr.go, k8s/README.md

- Date: 2026-02-25
  Title: Remove stale Threadfin-era DVRs and run category WebSafe-style A/B on pure `DVR 218`
  Summary:
    - Deleted all stale Threadfin-era DVRs from Plex (`141,144,147,150,153,156,159,162,165,168,171,174,177`) so the UI/runtime now only contains the 2 direct test DVRs plus the 13 pure `iptvtunerr-*` injected DVRs.
    - Ran a category-specific A/B on `iptvtunerr-newsus` / `DVR 218`: temporarily enabled `STREAM_TRANSCODE=true`, forced `PROFILE=plexsafe`, disabled client adaptation, and restarted the deployment; then reran the browser-path probe and rolled the deployment back to `STREAM_TRANSCODE=off`.
    - A/B probe result remained a failure (`startmpd1_0` ~37s). `iptvtunerr-newsus` still logged HLS relay (`hls-playlist ... relaying as ts`) with no `ffmpeg-transcode`, so the category image did not provide a proven ffmpeg/WebSafe path in this test.
    - PMS logs for the same A/B session (`798fc0ae-...`) again showed successful first-stage grabber startup + `progress/streamDetail` callbacks from the IptvTunerr stream, while browser client playback stopped before PMS returned `decision`/`start.mpd` (~95s later).
    - Late `connection refused` PMS errors against `iptvtunerr-newsus:5004` were induced by the intentional rollback restart while PMS still held the background live grab; they are not a new root cause.
  Verification:
    - `DELETE /livetv/dvrs/<id>` for stale Threadfin IDs (all returned `200`; subsequent inventory shows no `threadfin-*`)
    - `KUBECONFIG=$HOME/.kube/config python3 <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py --namespace plex --target deploy/plex --container plex --dvr 218 --per-dvr 1 --json-out /tmp/pure218-websafeab-probe.json` (expected fail: `startmpd1_0`)
    - `kubectl -n plex logs deploy/iptvtunerr-newsus` (A/B run shows HLS relay, no `ffmpeg-transcode`)
    - `kubectl -n plex exec deploy/plex -c plex -- grep ... \"Plex Media Server*.log\"` (grabber/progress + delayed `decision`/`start.mpd` on A/B session)
  Notes:
    - `iptvtunerr-newsus` was restored to its original env (`IPTV_TUNERR_STREAM_TRANSCODE=off`) after the A/B probe.
    - Browser probe correlation helper still points at `/tmp/iptvtunerr-websafe.log` for non-direct DVRs and can produce stale correlation metadata; rely on explicit Plex/IptvTunerr logs for category probes.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, /tmp/pure218-websafeab-probe.json

- Date: 2026-02-25
  Title: Isolate helper WebSafe ffmpeg failures on `DVR 218`, split recorder-vs-packager issues, and patch bootstrap profile mismatch
  Summary:
    - Repointed `DVR 218` to helper-pod category ffmpeg A/B services (`:5006`, `:5007`, `:5008`) to force a true WebSafe ffmpeg path and validate behavior with fresh channels/sessions.
    - Proved real ffmpeg category streaming in helper A/Bs (`ffmpeg-transcode`, `startup-gate idr=true aac=true`) and surfaced two separate failure classes:
      - `plexsafe` + bootstrap enabled (`:5006`): PMS first-stage recorder failed immediately with repeated `AAC bitstream not in ADTS format and extradata missing` and `Recording failed...`
      - bootstrap disabled (`:5007` `plexsafe`, `:5008` `aaccfr`): recorder stayed healthy (`progress/streamDetail`, stable recording activity), but Plex Web still failed `startmpd1_0`
    - Identified root cause in app code: `writeBootstrapTS` always generated AAC bootstrap TS, which mismatched non-AAC profiles (`plexsafe`/`pmsxcode`) and could break Plex's recorder via mid-stream codec switch.
    - Patched `internal/tuner/gateway.go` so bootstrap audio matches the active profile (MP3/MP2/AAC/no-audio as appropriate) and added bootstrap profile logging.
    - Built a patched binary, ran helper `:5009` (`plexsafe`, bootstrap enabled), and live-validated the fix: no PMS AAC/ADTS recorder errors, PMS first-stage streamDetail shows `codec=mp3`, recorder remains healthy, but browser probe still times out at `startmpd1_0`.
  Verification:
    - `go test ./internal/tuner -run '^$'`
    - `go test ./internal/config -run '^$'`
    - `go build -o /tmp/iptv-tunerr-patched ./cmd/iptv-tunerr`
    - helper A/B probes:
      - `/tmp/dvr218-helperab-probe.json` (`:5006`, `dash_init_404`, recorder crash path)
      - `/tmp/dvr218-helperab2-probe.json` (`:5007`, bootstrap off, `startmpd1_0`)
      - `/tmp/dvr218-helperab3-probe.json` (`:5008`, `aaccfr`, bootstrap off, `startmpd1_0`)
      - `/tmp/dvr218-helperab4-probe.json` (`:5009`, patched `plexsafe` bootstrap enabled, `startmpd1_0` but no recorder crash)
    - PMS log checks for:
      - old `AAC bitstream not in ADTS format and extradata missing` on `:5006`
      - absence of that error + healthy `progress/streamDetail codec=mp3` on patched `:5009`
  Notes:
    - `DVR 218` currently points to helper `iptvtunerr-newsus-websafeab4.plex.svc:5009` (patched binary, `plexsafe`, bootstrap enabled) for continued live testing.
    - The remaining blocker is still Plex's internal `start.mpd`/Live packager readiness, now isolated from the bootstrap/recorder crash bug.
  Opportunities filed:
    - none
  Links:
    - internal/tuner/gateway.go, memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md

- Date: 2026-02-25
  Title: Prove `DVR 218` helper AB4 failure persists without probe race (serialized `start.mpd`) and capture clean long-window TS output
  Summary:
    - Revalidated helper AB4 runtime (`iptvtunerr-newsus-websafeab4:5009`) and ran extended-timeout Fox Weather probes on `DVR 218` to move past the browser-style 35s timeout.
    - Confirmed the known concurrent probe race (`decision` + `start.mpd`) can still self-kill Plex's second-stage DASH session after the long startup stall, but then created a temporary no-decision probe copy and reran the same channel serialized.
    - Proved the core failure persists without the race: serialized/no-decision probe waited ~125s for `start.mpd`, then the returned DASH session's init/header endpoint (`/video/:/transcode/universal/session/<id>/0/header`) stayed `404` until timeout (`dash_init_404`).
    - PMS logs for the no-decision run (`Req#7b280`, client session `1c314794...`) showed the second-stage DASH transcode was started successfully and then failed with `TranscodeSession: timed out waiting to find duration for live session` -> `Failed to start session.` -> `Recording failed. Please check your tuner or antenna.`
    - Enabled long-window TS inspection on the AB4 helper for Fox Weather (`IPTV_TUNERR_TS_INSPECT_MAX_PACKETS=120000`) and captured ~63s of clean ffmpeg MPEG-TS output (monotonic PCR/PTS, no media-PID CC errors, no discontinuities), which further narrows the issue to Plex's internal duration/segment readiness path rather than obvious TS corruption from IptvTunerr.
  Verification:
    - `PWPROBE_HTTP_MAX_TIME=130 PWPROBE_DASH_READY_WAIT_S=140 python3 .../plex-web-livetv-probe.py --dvr 218 --channel 'FOX WEATHER'` (long-wait concurrent probe; reproduces delayed `start.mpd`)
    - Temporary probe copy with `PWPROBE_NO_DECISION=1` (`/tmp/plex-web-livetv-probe-nodecision.py`) + same args (serialized no-decision run; `dash_init_404`)
    - `kubectl -n plex exec deploy/iptvtunerr-build -- grep ... /tmp/iptvtunerr-newsus-websafeab4.log` (TS inspector summary + per-PID stats on Fox Weather)
    - `kubectl -n plex exec deploy/plex -c plex -- sed/grep ... \"Plex Media Server.log\"` (no-decision second-stage timeout / `timed out waiting to find duration for live session`)
  Notes:
    - The no-decision probe copy is temporary (`/tmp/plex-web-livetv-probe-nodecision.py`) and was used only to remove the concurrent probe race as a confounder.
    - Probe `correlation` JSON remains unreliable for injected/category DVRs because it infers the wrong IptvTunerr log path (`trial/websafe` heuristic).
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md

- Date: 2026-02-25
  Title: Fix Plex Live TV playback by proving and correcting PMS first-stage `/manifest` callback auth (`403`) on pure `DVR 218`
  Summary:
    - Re-read and reused the existing `k3s/plex` diagnostics harness (`plex-websafe-pcap-repro.sh`) instead of ad hoc probes to revisit the already-trodden first-stage `ssegment`/manifest path on the pure IptvTunerr injected setup (`DVR 218`, `FOX WEATHER`, helper AB4 `:5009`).
    - Harness localhost pcap proved the hidden root cause: PMS first-stage `Lavf` repeatedly `POST`ed CSV segment updates to `/video/:/transcode/session/.../manifest`, but PMS responded `403` to those callback requests while `Plex Media Server.log` only showed downstream `buildLiveM3U8: no segment info available`.
    - Confirmed PMS callback rejection was the blocker (not IptvTunerr TS format) by applying a Plex runtime workaround: added `allowedNetworks="127.0.0.1/8,::1/128,<lan-cidr>"` to PMS `Preferences.xml` and restarted `deploy/plex`.
    - Post-fix pcap harness rerun showed the expected behavior flip: first-stage `/manifest` callback responses became `200`, PMS internal `/livetv/sessions/.../index.m3u8` returned `200` with real HLS entries, and PMS logs switched to healthy `buildLiveM3U8: min ... max ...`.
    - Verified browser-path recovery on the same channel: PMS logs now show fast `decision` + `start.mpd` completion and `GET /video/:/transcode/universal/session/.../0/header` returning `200` (previously `404`/timeout).
    - Patched the external probe harness (`<sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py`) to be binary-safe (`subprocess.run(..., errors="replace")`) because successful DASH init/media fetches were causing false `UnicodeDecodeError` failures.
    - Final probe validation succeeded (`SUMMARY ok=1/1`) for `DVR 218` / `FOX WEATHER`.
  Verification:
    - `bash <sibling-k3s-repo>/plex/scripts/plex-websafe-pcap-repro.sh` (before fix, `DVR=218`, `CH=14`, AB4 `:5009`): localhost pcap shows repeated `/manifest` callback POSTs + `403` responses and PMS `buildLiveM3U8: no segment info available`
    - `kubectl -n plex exec deploy/plex -c plex -- ... Preferences.xml` (add `allowedNetworks=...`) + `kubectl -n plex rollout restart deploy/plex`
    - `bash <sibling-k3s-repo>/plex/scripts/plex-websafe-pcap-repro.sh` (after fix, same args): PMS `/livetv/sessions/.../index.m3u8` returns `200`; logs show `buildLiveM3U8: min ... max ...`
    - `python3 <sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py --dvr 218 --channel 'FOX WEATHER'` (after binary-safe harness patch): `OK`, DASH init/media fetches succeed
    - PMS log checks for `decision`, `start.mpd`, `.../0/header` (`200`) on the post-fix session
  Notes:
    - This is a Plex-side runtime/auth workaround in the PMS pod (`Preferences.xml`), not a IptvTunerr code change.
    - The existing pcap harness report parser can still under-report manifest callback response codes (`<missing>`) due loopback response correlation quirks; inspect `pms-local-http-responses.tsv` directly when in doubt.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, memory-bank/recurring_loops.md
- 2026-02-25: Fixed category runtime/image for Plex Web audio path and added manual stale-session drain helper.
  - Rebuilt/imported ffmpeg-enabled `iptv-tunerr:hdhr-test` on `<plex-node>`, restarted all 13 category deployments, and set `IPTV_TUNERR_STREAM_TRANSCODE=on` for immediate web audio normalization.
  - Fixed `cmd/iptv-tunerr` `run -mode=easy` regression so `IPTV_TUNERR_M3U_URL` / configured M3U URLs are honored again in `fetchCatalog()`.
  - Added missing-ffmpeg fallback warnings in `internal/tuner/gateway.go` and a manual `scripts/plex-live-session-drain.py` helper (no TTL behavior).
  - Verification: `go test ./cmd/iptv-tunerr -run '^$'`, `go test ./internal/tuner -run '^$'`, `python -m py_compile scripts/plex-live-session-drain.py`, category deployments back to `1/1`, `ffmpeg` present in category pods.
- 2026-02-26
  - Title: Add multi-layer Plex Live TV stale-session reaper mode (poll + SSE trigger + lease backstop)
  - Summary:
    - Extended `scripts/plex-live-session-drain.py` from one-shot manual drain into an optional continuous watch/reaper mode.
    - Implemented polling-based stale detection using Plex `/status/sessions` plus PMS request-activity heuristics from recent Plex logs (`/livetv/sessions/...`, DASH transcode session paths, client `/:/timeline`/`start.mpd`).
    - Added Plex SSE (`/:/eventsource/notifications`) listener as a fast wake-up trigger for rescans (notifications are advisory only; polling remains the authoritative kill condition).
    - Added optional lease backstop (`--lease-seconds`) to guarantee eventual cleanup if activity detection is ambiguous.
    - Fixed a false-positive idle bug discovered during live testing by treating non-ping SSE events as positive activity and relaxing log path matching so live/transcode path hits do not require client-IP match.
  - Verification:
    - `python -m py_compile scripts/plex-live-session-drain.py`
    - Live dry-run watch against active Chrome Plex Web session (`--watch --dry-run --sse --idle-seconds 8 ...`): session remained `idle_ready=no` while active playback generated `activity`/`playing`/`transcodeSession.update` SSE events.
- 2026-02-26
  - Title: A/B inspect `ctvwinnipeg.ca` rebuffer case (feed vs IptvTunerr output)
  - Summary:
    - Investigated Chrome/Plex Web rebuffering on Live TV `Scrubs` (`ctvwinnipeg.ca`, `iptvtunerr-generalent`) after user reported intermittent buffering despite max playback quality.
    - Confirmed PMS-side bottleneck from `/status/sessions`: `videoDecision=transcode`, `audioDecision=copy`, and `TranscodeSession speed=0.5` (below realtime), which explains rebuffering independent of stale-session reaper work.
    - A/B inspected stream characteristics on the same channel inside `iptvtunerr-generalent` pod:
      - upstream HLS sample (`iptv-hlsfix ... 1148306.m3u8`) = progressive `1280x720` `29.97fps` `H.264 High@L3.1` + `AAC-LC`, ~`3.78 Mbps`
      - IptvTunerr output sample (`/stream/ctvwinnipeg.ca`) = progressive `1280x720` `29.97fps` `H.264 High@L3.1` + `AAC-LC`, ~`1.25 Mbps`
    - Conclusion: this case does not show an obvious feed-format/pathology trigger; IptvTunerr output is already normalized and web-friendly, so the immediate issue is PMS transcode throughput/decision behavior rather than a malformed feed.
  - Verification:
    - `ffprobe` on upstream HLS playlist and source sample TS inside `deploy/iptvtunerr-generalent`
    - `ffprobe` on short IptvTunerr output capture for `/stream/ctvwinnipeg.ca`
    - Plex `/status/sessions` XML inspection for `TranscodeSession speed` / decision fields
- 2026-02-26
  - Title: Add criteria-based stream override generator helper
  - Summary:
    - Added `scripts/plex-generate-stream-overrides.py` to probe channels from a tuner `lineup.json` with `ffprobe` and emit suggested per-channel `profile`/`transcode` overrides using the existing runtime override hooks (`IPTV_TUNERR_PROFILE_OVERRIDES_FILE`, `IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`).
    - Criteria currently flag likely Plex Web trouble signals (interlaced video, >30fps, HE-AAC/non-LC AAC, unsupported codecs, high bitrate, high H.264 level/B-frame count).
    - Added `--replace-url-prefix OLD=NEW` to support probing lineup JSONs that contain cluster-internal absolute URLs via local port-forward.
    - Validated against `iptvtunerr-generalent` / `ctvwinnipeg.ca` (the `Scrubs` rebuffer case): generator classified it `OK` / no flags, matching manual upstream-vs-output A/B analysis and confirming the issue is not an obvious feed-format mismatch.
  - Verification:
    - `python -m py_compile scripts/plex-generate-stream-overrides.py`
    - `kubectl -n plex port-forward deploy/iptvtunerr-generalent 15004:5004` + `python scripts/plex-generate-stream-overrides.py --lineup-json http://127.0.0.1:15004/lineup.json --channel-id ctvwinnipeg.ca --replace-url-prefix 'http://iptvtunerr-generalent.plex.svc:5004=http://127.0.0.1:15004'`
- 2026-02-26
  - Title: Integrate Plex Live session reaper into Go app (`serve`) for packaged builds
  - Summary:
    - Added `internal/tuner/plex_session_reaper.go` and wired it into `tuner.Server.Run` behind env flag `IPTV_TUNERR_PLEX_SESSION_REAPER`.
    - Reaper uses Plex `/status/sessions` to enumerate Live TV sessions and stop transcodes via `/video/:/transcode/universal/stop`, with configurable thresholds:
      - idle timeout (`IPTV_TUNERR_PLEX_SESSION_REAPER_IDLE_S`)
      - renewable lease timeout (`..._RENEW_LEASE_S`)
      - hard lease timeout (`..._HARD_LEASE_S`)
      - poll interval (`..._POLL_S`)
      - optional SSE wake-up listener (`..._SSE`, default on)
    - Implemented session activity tracking from `/status/sessions` transcode fields (`maxOffsetAvailable`, `timeStamp`) and added stop-attempt cooldown to avoid hammering Plex.
    - Intentionally uses SSE only as a scan wake trigger (not a global heartbeat renewal) to avoid cross-session false negatives when multiple clients are active.
    - Added unit test coverage for live-session XML parsing and filtering.
  - Verification:
    - `go test ./internal/tuner -run 'TestParsePlexLiveSessionRowsFiltersAndParses|^$'`
    - `go test ./cmd/iptv-tunerr -run '^$'`
- 2026-02-26
  - Title: Wire built-in Go reaper into example k8s manifest and standalone run docs
  - Summary:
    - Updated `k8s/iptvtunerr-hdhr-test.yaml` to enable the built-in Plex session reaper by default in the example deployment and map `IPTV_TUNERR_PMS_TOKEN` from the existing `PLEX_TOKEN` secret key (`plex-iptv-creds`).
    - Documented built-in reaper behavior and tuning knobs in `k8s/README.md` and `docs/how-to/run-without-kubernetes.md` (binary, Docker, systemd/package-friendly usage).
  - Verification:
    - YAML patch inspection
    - Go compile/tests already green after integrated reaper changes (`go test ./internal/tuner ./cmd/iptv-tunerr -run '^$'`)

## 2026-02-26 — In-app XMLTV language normalization + single-app supervisor mode (first pass)
- Added `iptv-tunerr supervise -config <json>` (child-process supervisor) for self-contained multi-DVR deployments in one container/app, including config loader, restart loop, prefixed log fan-in, and tests (`internal/supervisor/*`, `cmd/iptv-tunerr/main.go`).
- Added in-app XMLTV programme text normalization in the XMLTV remapper (`internal/tuner/xmltv.go`) with env knobs:
  - `IPTV_TUNERR_XMLTV_PREFER_LANGS`
  - `IPTV_TUNERR_XMLTV_PREFER_LATIN`
  - `IPTV_TUNERR_XMLTV_NON_LATIN_TITLE_FALLBACK`
- Added tests covering preferred `lang=` pruning and non-Latin title fallback (`internal/tuner/xmltv_test.go`).
- Documented supervisor mode, HDHR networking constraints in k8s, and XMLTV language normalization in `k8s/README.md` and `docs/how-to/run-without-kubernetes.md`.
- Verified targeted tests: `go test ./internal/tuner ./internal/supervisor ./cmd/iptv-tunerr -run 'TestXMLTV_externalSourceRemap|TestXMLTV_externalSourceRemap_PrefersEnglishLang|TestXMLTV_externalSourceRemap_NonLatinTitleFallbackToChannel|TestLoadConfig|^$'` ✅
- Runtime note: reverted category `iptvtunerr-*` deployments back to working `iptv-tunerr:hdhr-test` after a temporary unique-tag rollout caused `ImagePullBackOff` (tag not present on node). No lasting runtime change from the supervisor work was applied to live category pods.
- Added concrete supervisor deployment examples for the intended production split: `13` category DVR insertion children + `1` big-feed HDHR child in one app/container (`k8s/iptvtunerr-supervisor-multi.example.json`, `k8s/iptvtunerr-supervisor-singlepod.example.yaml`). Validated JSON parses and contains 14 unique instances with exactly one HDHR-network-enabled child.
- Added cutover mapping artifacts for 13 injected DVRs when migrating to the single-pod supervisor: `scripts/plex-supervisor-cutover-map.py` + `k8s/iptvtunerr-supervisor-cutover-map.example.tsv`. The example preserves per-category injected DVR URIs (`iptvtunerr-<category>.plex.svc:5004`), so Plex DVR URI reinjection is usually unnecessary.
- Generated real single-pod supervisor migration artifacts in sibling `k3s/plex` from live manifests using `scripts/generate-k3s-supervisor-manifests.py`:
  - `iptvtunerr-supervisor-multi.generated.json` (14 children: 13 injected categories + 1 HDHR)
  - `iptvtunerr-supervisor-singlepod.generated.yaml` (single supervisor pod + per-category Services + HDHR service)
  - `iptvtunerr-supervisor-cutover-map.generated.tsv` (confirms 13 injected DVR URIs unchanged)
  Category child identity signals are bare categories (`device_id` / `friendly_name` = `newsus`, `sportsa`, etc.).
- 2026-02-26
  - Title: Complete live k3s cutover to single-pod supervisor (13 injected DVR children + 1 HDHR child)
  - Summary:
    - Regenerated supervisor artifacts with timezone-guided HDHR preset selection (`na_en`) after changing the HDHR child to use broad `live.m3u` plus in-app music/radio stripping and wizard-safe lineup cap (`479`).
    - Reapplied generated supervisor `ConfigMap` + `Deployment` in sibling `k3s/plex`, then re-patched the deployment image to the locally imported custom tag (`iptv-tunerr:supervisor-cutover-20260225223451`) because the generated YAML's default image (`iptv-tunerr:hdhr-test`) on `<plex-node>` lacked the new `supervise` command.
    - Verified supervisor pod startup on `<plex-node>` with all 14 children healthy and category children reporting bare category identities (`FriendlyName`/`DeviceID` without `iptvtunerr-` prefix).
    - Verified HDHR child loads broad feed (`6207` live channels), drops music/radio via pre-cap filter (`72` dropped), and serves exactly `479` channels on `lineup.json`.
    - Applied only the generated Service documents to cut category + HDHR HTTP routing over to the supervisor pod, then scaled the old 13 category deployments to `0/0`.
    - Post-cutover validation from Plex pod confirmed service responses (`iptvtunerr-newsus` discover identity and `iptvtunerr-hdhr-test` lineup count `479`).
  - Verification:
    - `python scripts/generate-k3s-supervisor-manifests.py --timezone 'America/Regina'` (generator does not echo timezone/postal)
    - `sudo kubectl -n plex apply -f /tmp/iptvtunerr-supervisor-bootstrap.yaml` (ConfigMap+Deployment only)
    - `docker save iptv-tunerr:supervisor-cutover-20260225223451 | ssh <plex-node> 'sudo k3s ctr -n k8s.io images import -'`
    - `sudo kubectl -n plex set image deploy/iptvtunerr-supervisor iptvtunerr=iptv-tunerr:supervisor-cutover-20260225223451`
    - `sudo kubectl -n plex rollout status deploy/iptvtunerr-supervisor`
    - `sudo kubectl -n plex apply -f /tmp/iptvtunerr-supervisor-services.yaml` (Services only)
    - `sudo kubectl -n plex get endpoints ...` + in-pod `wget` checks (`discover.json`, `lineup.json`)
## 2026-02-26 - HDHR wizard noise reduction + Plex cache verification

- Added in-app `/lineup_status.json` configurability for HDHR compatibility endpoint (`IPTV_TUNERR_HDHR_SCAN_POSSIBLE`) and updated the supervisor manifest generator to set category children `false` and the dedicated HDHR child `true`.
- Added/updated tests for HDHR lineup status scan-possible behavior.
- Regenerated supervisor manifests and rolled the patched supervisor binary to the actual node runtime (`<plex-node>`) after diagnosing image imports had been going to the wrong host runtime (`<work-node>`).
- Live-verified the running supervisor binary hash and endpoint behavior:
  - `iptvtunerr-otherworld` returns `ScanPossible=0`
  - `iptvtunerr-hdhr-test` returns `ScanPossible=1`
- Verified Plex-side device inventory via `/media/grabbers/devices`:
  - stale helper `newsus-websafeab5:5010` cache entry no longer present
  - active injected category devices still appear (expected; Plex lists registered HDHR devices)
- Removed an accidentally created standalone cached `newsus` device row (`key=245`) after a test re-register call, leaving only the active injected `DVR 218` row and the intended category/HDHR devices.

Verification:
- `go test ./internal/tuner -run 'TestHDHR_lineup_status|TestHDHR_lineup_status_scan_possible_false'`
- Live k8s endpoint checks from supervisor pod and Plex pod (`/lineup_status.json`)
- Plex `/media/grabbers/devices` API inspection

## 2026-02-26 - Plex provider metadata cleanup (guide URI drift) + backend/UI split proof

- Investigated user-reported TV symptom ("all tabs labelled `plexKube`" and identical-looking EPGs) after single-pod supervisor cutover.
- Proved tuner feeds were still distinct (`/lineup.json` counts differ across categories/HDHR) and Plex provider channel endpoints were also distinct (`/tv.plex.providers.epg.xmltv:<id>/lineups/dvr/channels` returned different sizes), so the issue is not a flattened IptvTunerr lineup.
- Found and patched real Plex DB metadata drift in `media_provider_resources` (inside Plex pod `com.plexapp.plugins.library.db`):
  - direct provider child rows for `DVR 135`/`138` (`id=136/139`, `type=3`) incorrectly pointed to `iptvtunerr-otherworld` guide URI
  - injected + HDHR provider child rows mostly had blank `type=3.uri`
  - `DVR 218` device row (`id=179`, `type=4`) still pointed to helper A/B URI `iptvtunerr-newsus-websafeab4:5009`
- Backed up the Plex DB file and patched all relevant `type=3.uri` rows to the correct per-DVR `.../guide.xml` plus repaired row `179` to `http://iptvtunerr-newsus.plex.svc:5004`.
- Verified `/livetv/dvrs/218` now reflects the correct device URI and DB rows are consistent with each DVR lineup.
- Confirmed `/media/providers` still reports all Live TV providers with `friendlyName=\"plexKube\"` and `title=\"Live TV & DVR\"`, which likely explains identical tab labels on Plex TV clients; remaining issue requires live client request capture to confirm provider-ID switching behavior.

Verification:
- `sqlite3` queries in Plex pod (`media_provider_resources` before/after patch)
- Plex API checks:
  - `/livetv/dvrs/<id>`
  - `/tv.plex.providers.epg.xmltv:<id>/lineups/dvr/channels`
  - `/media/providers`

## 2026-02-26 - LG TV guide-path capture proved legacy provider pinning; removed direct test DVRs

- Captured the LG TV (`<client-ip-b>`) guide path from the actual Plex log file (`Plex Media Server.log` inside the pod), not container stdout.
- Proved the TV guide flow was hitting only legacy provider `tv.plex.providers.epg.xmltv:135` (`DVR 135`, direct `iptvtunerrTrial`) for:
  - `/lineups/dvr/channels`
  - `/grid`
  - `/hubs/discover`
  while mixed with playback/timeline traffic (`context=source:content.dvr.guide`).
- Deleted legacy direct test DVRs `135` and `138` via Plex API (`DELETE /livetv/dvrs/<id>`) so the TV cannot keep defaulting to those providers.
- Deleted orphan HDHR device rows left behind by Plex (`media_provider_resources` ids `134`, `137`; `iptvtunerr01`, `iptvtunerrweb01`) after the DVR deletions, removing them from `/media/grabbers/devices`.
- Confirmed remaining DVR inventory is now only injected categories (`218..242`) plus the two HDHR wizard-path tuners (`247`, `250`).

Verification:
- File-log grep/tail on `Plex Media Server.log` inside Plex pod for `<client-ip-b>` and `tv.plex.providers.epg.xmltv:*`
- Plex API:
  - `/livetv/dvrs`
  - `/media/grabbers/devices`
- DB sanity:
  - `media_provider_resources` ids `134/137/135/138/136/139`

## 2026-02-26 - Fixed multi-DVR guide collisions with per-child guide-number offsets and rebuilt Plex mappings

- Root-caused "all tabs same guide but different channel names" to overlapping channel/guide IDs across DVRs (many children exposed `GuideNumber` starting at `1`).
- Added in-app `IPTV_TUNERR_GUIDE_NUMBER_OFFSET` support and wired it through `config` -> `tuner.Server.UpdateChannels`.
- Rolled a new supervisor image (`iptv-tunerr:supervisor-guideoffset-20260226001027`) plus offset-enabled supervisor config in live k3s (`<plex-node>`), assigning distinct channel ID ranges per category/HDHR child.
- Re-ran Plex guide reloads (`scripts/plex-reload-guides-batched.py`) and channelmap activation (`scripts/plex-activate-dvr-lineups.py`) for all 15 DVRs.
- Verified Plex provider channel lists now have non-overlapping IDs (examples: `newsus=2001+`, `bcastus=1001+`, `otherworld=13001+`, `HDHR2=103260+`) and user confirmed the first tabs now show distinct EPGs.
- Post-remap playback stall was traced to Plex hidden stale "active grabs" (`Waiting for media grab to start`) and cleared by restarting `deploy/plex`; same remapped channel tuned successfully afterward.

Verification:
- `go test ./internal/tuner -run 'TestUpdateChannels_appliesGuideNumberOffset|TestUpdateChannels_capsLineup'`
- Live k8s rollout + supervisor logs showing per-child offset application
- `scripts/plex-reload-guides-batched.py` (15 DVRs complete)
- `scripts/plex-activate-dvr-lineups.py` (15 DVRs `status=OK`)
- Plex provider channel inventory (`/tv.plex.providers.epg.xmltv:<id>/lineups/dvr/channels`)

## 2026-02-26 - Added cross-platform tester packaging workflow and docs (single-app supervisor ready)

- Added `scripts/build-test-packages.sh` to build cross-platform tester bundles (`.tar.gz`/`.zip`) and `SHA256SUMS.txt` under `dist/test-packages/<version>/`.
- Added packaging + supervisor testing docs:
  - `docs/how-to/package-test-builds.md`
  - `docs/reference/testing-and-supervisor-config.md`
- Linked the new docs from `README.md`, `docs/index.md`, `docs/how-to/index.md`, and `docs/reference/index.md`.
- Added OS build-gating/stubs so packaging compiles for non-Linux targets:
  - `internal/vodfs` Linux-only build tags + non-Linux stub `Mount`
  - `internal/hdhomerun` `!windows` build tags + Windows stub server (HDHR network mode unsupported on Windows builds)

Verification:
- `bash -n scripts/build-test-packages.sh`
- `PLATFORMS='linux/amd64 darwin/arm64 windows/amd64' VERSION=vtest-pack ./scripts/build-test-packages.sh`
- `go test ./cmd/iptv-tunerr -run '^$'`
- `go test ./internal/hdhomerun ./internal/vodfs -run '^$'`

## 2026-02-26 - Polished tester handoff workflow and added Plex hidden-grab recovery tooling

- Added `scripts/build-tester-release.sh` to stage a tester-ready bundle directory (`packages/`, `examples/`, `docs/`, `manifest.json`, `TESTER-README.txt`) on top of the cross-platform package archives.
- Added `docs/how-to/tester-handoff-checklist.md` for bundle validation and tester instructions per OS.
- Added `scripts/plex-hidden-grab-recover.sh` and `docs/runbooks/plex-hidden-live-grab-recovery.md` to detect and safely recover the Plex hidden "active grab" wedge (`Waiting for media grab to start`) by checking logs + `/status/sessions` before restarting Plex.
- Re-enabled real Windows HDHR network mode path by removing the temporary Windows stub and making `internal/hdhomerun` cross-platform (Windows/macOS/Linux compile); kept `VODFS` Linux-only stubs as intended.
- Updated docs and tester bundle metadata to reflect current platform support (Windows/macOS core tuner + HDHR path; `mount` remains Linux-only).

Verification:
- `bash -n scripts/plex-hidden-grab-recover.sh scripts/build-test-packages.sh scripts/build-tester-release.sh`
- `GOOS=windows GOARCH=amd64 go build -o /tmp/iptv-tunerr-win.exe ./cmd/iptv-tunerr`
- `go test ./internal/hdhomerun -run '^$'`
- `PLATFORMS='linux/amd64 windows/amd64' VERSION=vtest-final ./scripts/build-tester-release.sh`

## 2026-02-26 - Added CLI/env reference and CI automation for tester bundles

- Added `docs/reference/cli-and-env-reference.md` with practical command/flag/env coverage for `run`, `serve`, `index`, `mount`, `probe`, and `supervise`, including recent multi-DVR/testing envs (`IPTV_TUNERR_GUIDE_NUMBER_OFFSET`, Plex session reaper, HDHR shaping).
- Linked the new reference from `docs/reference/index.md` and `docs/index.md`.
- Added GitHub Actions workflow `.github/workflows/tester-bundles.yml`:
  - manual trigger (`workflow_dispatch`) with optional `version` / `platforms`
  - tag trigger (`v*`)
  - builds staged tester bundle via `scripts/build-tester-release.sh`
  - uploads artifact (`tester-bundle-<version>`)
- Updated packaging docs to document the CI artifact flow.

Verification:
- `bash -n scripts/build-test-packages.sh scripts/build-tester-release.sh scripts/plex-hidden-grab-recover.sh`
- Python YAML parse of `.github/workflows/tester-bundles.yml`

## 2026-02-26 - Added Plex DVR lifecycle/API reference doc for wizard/inject/remove/refresh flows

- Added `docs/reference/plex-dvr-lifecycle-and-api.md` as a single authoritative reference for Plex-side Live TV/DVR operations used in IPTV Tunerr testing:
  - HDHR wizard-equivalent API flow vs injected DVR flow
  - device identity vs DVR row vs provider row model
  - remove/cleanup guidance and stale device/provider caveats
  - guide reload and channelmap activation lifecycle
  - common Plex-side failure modes (provider drift, client cache, hidden grabs)
- Linked from `docs/reference/index.md`.

Verification:
- Manual doc review for coverage of wizard/API/inject/remove/refresh/channelmap + Plex UI/backend gotchas

## 2026-02-26 - Repo hygiene audit and root cleanup (secrets/path scan + cruft relocation)

- Audited tracked files for:
  - high-confidence secret patterns (tokens, private keys)
  - local paths/hostnames and personal identifiers (`<user>`, `/home/...`, `<plex-node>`, `<work-node>`)
  - agent/test artifacts unrelated to core app surface
- No high-confidence secrets found in tracked files.
- Cleaned root-level tracked cruft:
  - removed `iptvtunerr-main-fixed.zip`
  - moved ad hoc/manual test scripts into `scripts/legacy/`:
    - `test_hdhr.sh`
    - `test_hdhr_network.sh`
    - `<work-node>_plex_test.sh`
  - added `scripts/legacy/README.md` documenting legacy status

Verification:
- `rg` scans for secrets/path identifiers (tracked + untracked triage)
- `git status --short` confirms file moves/removal are tracked as rename/delete

## 2026-02-26 - Hardened release workflows (versioned Docker tags + GitHub Release tester bundles)

- Updated `.github/workflows/docker.yml` to:
  - set explicit GHCR publish permissions (`packages: write`)
  - generate tags via `docker/metadata-action` (tag refs, `latest` on `main`, and `sha-*` trace tags)
  - publish versioned image tags on `v*` pushes instead of only `latest`
- Updated `.github/workflows/tester-bundles.yml` to:
  - set `contents: write`
  - package the staged tester bundle directory as a `.tar.gz` on tag pushes
  - upload that archive to the GitHub Release (while still uploading the workflow artifact)

Verification:
- YAML parse (`python/yaml.safe_load`) for both workflow files

## 2026-02-26 - Fixed pre-existing startup-signal test helper regression and restored green verify

- Fixed `internal/tuner/gateway_startsignal_test.go` synthetic TS packet helper so short test payloads are emitted with adaptation-field stuffing instead of `0xff` bytes inside the payload region.
- This restores correct cross-packet boundary conditions for `TestLooksLikeGoodTSStartDetectsSplitIDRStartCodeAcrossPackets`.
- `./scripts/verify` now passes again (format, vet, test, build).

Verification:
- `go test ./internal/tuner -run TestLooksLikeGoodTSStartDetectsSplitIDRStartCodeAcrossPackets -count=1`
- `./scripts/verify`

## 2026-02-26 - Added in-app Plex VOD library registration (`plex-vod-register`) for VODFS mounts

- Added `internal/plex/library.go` with Plex library section APIs:
  - list sections
  - create section (`/library/sections`)
  - refresh section (`/library/sections/<key>/refresh`)
  - idempotent ensure-by-name+path
- Added new CLI command `plex-vod-register` to create/reuse:
  - `VOD` -> `<mount>/TV` (show library)
  - `VOD-Movies` -> `<mount>/Movies` (movie library)
  with Plex URL/token env fallbacks and optional refresh.
- Updated docs (`README.md`, `docs/reference/cli-and-env-reference.md`, `features.md`) to document the VODFS + Plex library registration workflow and the k8s mount-visibility caveat.
- Live validation against test Plex API (inside Plex pod) succeeded using temporary names (`PTVODTEST`, `PTVODTEST-Movies`): create + reuse + refresh behavior confirmed.

Verification:
- `go test ./cmd/iptv-tunerr ./internal/plex -run '^$'`
- `go build -o /tmp/iptv-tunerr-vodreg ./cmd/iptv-tunerr`
- Live Plex API smoke via in-pod binary execution: `plex-vod-register` create/reuse against `http://127.0.0.1:32400`
- 2026-02-26 (late): VODFS/Plex VOD TV imports unblocked by per-library Plex analysis suppression.
  - Proved `VOD-SUBSET` TV imports (`count` moved from `0` to `>0`, observed `6` and climbing) after disabling library-level credits/chapter-thumbnail/preview/ad/voice jobs and restarting Plex.
  - Added in-app Plex library prefs support (`internal/plex/library.go`) and wired `plex-vod-register` to apply a default VOD-safe preset.
  - Added in-app VOD taxonomy enrichment + deterministic sorting (`internal/catalog/vod_taxonomy.go`) and applied it during `fetchCatalog` for future category-split catch-up libraries.
  - Verification: `go test ./internal/plex ./internal/catalog ./cmd/iptv-tunerr -run '^$|TestApplyVODTaxonomy'` and live PMS prefs `PUT /library/sections/<id>/prefs` checks on sections `7/8/9/10`.

- 2026-02-26 (late): Added built-in VOD category-lane split tooling for post-backfill reruns.
  - New `iptv-tunerr vod-split` command emits per-lane catalogs (`bcastUS`, `sports`, `news`, `kids`, `music`, `euroUK`, `mena`, `movies`, `tv`, `intl`) plus `manifest.json`.
  - Added `internal/catalog` lane split logic + tests and wired taxonomy enrichment/sorting into `fetchCatalog`.
  - Added host-side helper `scripts/vod-seriesfixed-cutover.sh` for retry+swap+remount after `catalog.seriesfixed.json` backfill completes.

- 2026-02-26 (late): Switched tester release packaging to GitHub-style per-asset ZIPs.
  - `scripts/build-test-packages.sh` now emits ZIPs for every platform plus a source ZIP (`iptv-tunerr_<version>_source.zip`) and `SHA256SUMS.txt`.
  - `scripts/build-tester-release.sh` now stages only ZIP-based package assets and records source ZIP metadata in `manifest.json`.
  - `.github/workflows/tester-bundles.yml` now uploads individual ZIPs + `SHA256SUMS.txt` directly to GitHub Releases instead of a single combined bundle tarball.
  - Fixed cross-platform packaging regression by adding `MountWithAllowOther` to non-Linux VODFS stub (`internal/vodfs/mount_unsupported.go`).
  - Local verification: `v0.1.0-test2-rc1` package build + staged tester release contained source ZIP + 7 platform ZIPs + checksums.

- VODFS naming now prefixes presented movie/show/episode names with `Live: ` (idempotent) so Plex search/results make live-origin items obvious.
  - Implemented in `internal/vodfs/plexname.go` (`MovieDirName` / `ShowDirName` path generation) and verified with `internal/vodfs` naming tests.
- 2026-02-26 (morning): Added provider-category-aware VOD taxonomy hooks and Xtream indexer support for `category_id/category_name` on movies/series.
  - `catalog.Movie`/`catalog.Series` provider category fields are now populated by `internal/indexer/player_api.go` (via `get_vod_categories` / `get_series_categories`) for newly generated catalogs.
  - Tightened VOD taxonomy heuristics to avoid common title-substring false positives (`News of the World`, `The Sound of Music`, `Nickel Boys`, `Phantom Menace`, `The Newsroom`).
  - Re-ran `vod-split` on existing local catalog (which lacks provider categories): `sports/music/kids` lanes improved materially; `euroUK`/`mena` remain broad due to noisy source-tag region inference.
- 2026-02-26 (morning): Validated provider-category-driven VOD lane split quality using a fast merge into existing `catalog.json`.
  - Built `/tmp/catalog.providermerge.json` by fetching Xtream `get_vod_streams`/`get_series` + `get_vod_categories`/`get_series_categories` and merging `provider_category_id/name` into the current local catalog by ID (movies: `157321/157331` merged, series: `41391/41391` merged).
  - Re-ran `vod-split` on merged catalog: `sports`, `kids`, and `music` lanes became materially cleaner and larger (driven by provider categories instead of title substrings).
  - `euroUK`/`mena` remain broad by design/heuristics and need a second-pass taxonomy ruleset (sub-lanes or package-scoped region rules) if tighter segmentation is desired.
- 2026-02-26 (morning): Refined VOD lane model and `bcastUS` gating.
  - Split region-heavy lanes into explicit movie/TV lanes: `euroUKMovies`, `euroUKTV`, `menaMovies`, `menaTV`.
  - Tightened `bcastUS` to English US/CA TV-like provider categories (and common EN source tags), preventing dubbed/imported US/CA copies from crowding the broadcast lane.
  - Validation on provider-category-merged catalog: `bcastUS` reduced from `9631` to `2179` series; non-English/translated US/CA content moved to `tv`.

- Fixed recurring supervisor env leak: parent `IPTV_TUNERR_PLEX_SESSION_REAPER*` / `IPTV_TUNERR_PMS_*` vars are now stripped from child environments by default in `internal/supervisor` (children can still set explicit values in supervisor config).
- 2026-02-26 (morning): Phase A VOD lane rollout completed live (`sports`, `kids`, `music`) without replacing existing VOD libraries.
  - Created and mounted host VODFS lanes on Plex node: `/srv/iptvtunerr-vodfs-{sports,kids,music}` (plus separate cache/run dirs).
  - Patched Plex deployment to mount lane paths into pod: `/media/iptv-vodfs-{sports,kids,music}`.
  - Registered six new Plex libraries with in-app `plex-vod-register` and VOD-safe preset enabled by default:
    - shows: `sports` (11), `kids` (13), `music` (15)
    - movies: `sports-Movies` (12), `kids-Movies` (14), `music-Movies` (16)
  - Observed immediate import activity: `sports-Movies` scanning and `size>0` quickly.
  - Hit and repaired a host FUSE mount failure during rollout (`/srv/iptvtunerr-vodfs` transport endpoint disconnected) before recycling Plex.
- 2026-02-26 (morning): Completed Phase B + C VOD lane rollout in Plex.
  - Phase B movie-region lanes mounted and registered: `euroUK-Movies` (18), `mena-Movies` (20).
  - Phase C TV lanes mounted and registered with clean display names: `euroUK` (21), `mena` (23), `bcastUS` (25), `TV-Intl` (27).
  - Companion movie libraries were also created by the current `plex-vod-register` helper for TV lanes (`euroUKTV-Movies`, `menaTV-Movies`, `bcastUS-Movies`, `TV-Intl-Movies`) because the command always provisions both TV + Movies paths.
  - Verified live scan activity on new lane libraries (e.g. `Scanning euroUK`).

- 2026-02-26: Completed VOD lane Phase B/C Plex library rollout and cleanup; added `plex-vod-register` `-shows-only/-movies-only` flags. Live cleanup removed unintended companion lane libraries (`euroUKMovies`, `menaMovies`, `euroUKTV-Movies`, `menaTV-Movies`, `bcastUS-Movies`, `TV-Intl-Movies`). Performed a deliberate Plex library DB reverse-engineering pass on a copied `com.plexapp.plugins.library.db` (using `PRAGMA writable_schema=ON` workaround) and documented the core library table relationships (`library_sections`, `section_locations`, `metadata_items`, `media_items`, `media_parts`, `media_streams`) plus the Live TV provider chain (`media_provider_resources` type 1/3/4). Key finding: `media_provider_resources` has no per-provider friendly-name/title fields; `/media/providers` `friendlyName=plexKube` labels appear Plex-synthesized from server-level `friendlyName`, so the guide-tab title issue is not fixable via the DVR/provider URI DB patch path. Verification: live Plex API section list before/after cleanup, local DB schema/row inspection, `go test ./cmd/iptv-tunerr -run '^$'`.

- 2026-02-26: Reverse-engineered Plex Web Live TV source label logic in WebClient `main-*.js` (`function Zs` + module `50224`). Confirmed Plex Web chooses `serverFriendlyName` for multiple Live TV sources on a full-owned server, which is why tabs all showed `plexKube`. Patched running Plex Web bundle to inject a providerIdentifier->lineupTitle map (from `/livetv/dvrs`) so tab labels are per-provider (`newsus`, `bcastus`, ..., `iptvtunerrHDHR479`, `iptvtunerrHDHR479B`). This is a runtime bundle patch (survives until Plex update/image replacement); browser hard refresh required.

- 2026-02-26: Reverted the experimental Plex Web `main-*.js` bundle patch after it broke Web UI loading for the user. Implemented `scripts/plex-media-providers-label-proxy.py` instead: a server-side reverse proxy that rewrites `/media/providers` Live TV `MediaProvider` labels (`friendlyName`, `sourceTitle`, `title`, content root Directory title, watchnow title) using `/livetv/dvrs` lineup titles. Validated on captured `/media/providers` XML: all 15 `tv.plex.providers.epg.xmltv:<id>` providers rewrite to distinct labels (`newsus`, `bcastus`, ..., `iptvtunerrHDHR479B`). Caveat documented: current Plex Web version still hardcodes server-friendly-name labels for owned multi-LiveTV sources, so proxy primarily targets TV/native clients unless WebClient is separately patched.

- 2026-02-26: Deployed `plex-label-proxy` in k8s (`plex` namespace) and patched live `Ingress/plex` to route `Exact /media/providers` to `plex-label-proxy:33240` while leaving all other paths on `plex:32400`. Proxy is fed by ConfigMap from `scripts/plex-media-providers-label-proxy.py` and rewrites Live TV provider labels per DVR using `/livetv/dvrs`. Fixed gzip-compressed `/media/providers` handling after initial parse failures. End-to-end validation via `https://plex.home/media/providers` confirms rewritten labels for `tv.plex.providers.epg.xmltv:{218,220,247,250}` (`newsus`, `bcastus`, `iptvtunerrHDHR479`, `iptvtunerrHDHR479B`).
## 2026-02-26 — Phase 1 EPG-linking report CLI (deterministic, report-only)

- Added `internal/epglink` package for:
  - XMLTV channel extraction (`<channel id=...><display-name>...`)
  - conservative channel-name normalization
  - deterministic match tiers (`tvg-id` exact, alias exact, normalized-name exact unique)
  - JSON-friendly coverage/unmatched report structures
- Added `iptv-tunerr epg-link-report` CLI command:
  - `-catalog`
  - `-xmltv` (file or `http(s)` URL)
  - `-aliases` (JSON name->xmltv overrides)
  - `-out`, `-unmatched-out`
- Added tests for normalization, XMLTV parsing, and deterministic match tiers.
- Updated docs:
  - `docs/reference/cli-and-env-reference.md`
  - `docs/reference/epg-linking-pipeline.md` (Phase 1 status)

Verification:
- `go test ./internal/epglink ./internal/indexer ./internal/catalog -run '^Test'`
- `go test ./cmd/iptv-tunerr -run '^$'`
- CLI smoke test with synthetic catalog/XMLTV/alias files (`go run ./cmd/iptv-tunerr epg-link-report ...`)
## 2026-02-26 — Live category overflow bucket support (auto-sharded injected DVR children)

- Added runtime lineup sharding for live tuner children:
  - `IPTV_TUNERR_LINEUP_SKIP`
  - `IPTV_TUNERR_LINEUP_TAKE`
- Sharding is applied after pre-cap filters/shaping and before final lineup cap, so overflow buckets are sliced from the confirmed filtered lineup.
- Updated supervisor manifest generator (`scripts/generate-k3s-supervisor-manifests.py`) to auto-create overflow category children from a linked-count JSON:
  - `--category-counts-json`
  - `--category-cap` (default `479`)
- Overflow children are emitted as `<category>2`, `<category>3`, ... and get:
  - per-child `IPTV_TUNERR_LINEUP_SKIP/TAKE`
  - unique service/base URL identity (`iptvtunerr-<categoryN>`)
  - shard-adjusted `IPTV_TUNERR_GUIDE_NUMBER_OFFSET` when a base offset exists

Verification:
- `go test ./internal/tuner -run 'Test(ApplyLineupPreCapFilters_shardSkipTake|UpdateChannels_shardThenCap)$'`
- `python -m py_compile scripts/generate-k3s-supervisor-manifests.py`
- synthetic generator smoke run with counts (`newsus=1100`, `sportsa=500`) produced expected shards:
  - `newsus`, `newsus2`, `newsus3`
  - `sportsa`, `sportsa2`
## 2026-02-26 — In-app Plex wizard-oracle probe command (`plex-epg-oracle`)

- Added `iptv-tunerr plex-epg-oracle` to automate the wizard-equivalent Plex HDHR flow for one or more tuner base URLs:
  - register device (`/media/grabbers/.../devices`)
  - create DVR (`/livetv/dvrs`)
  - optional `reloadGuide`
  - fetch channelmap (`/livetv/epg/channelmap`)
  - optional activation
- Supports testing a URL matrix directly with:
  - `-base-urls`
  - or `-base-url-template` + `-caps` (template expansion for `{cap}`)
- Intended for EPG-linking experiments (using Plex as a mapping oracle), not runtime playback.

Verification:
- `go test ./cmd/iptv-tunerr -run '^$'`

Follow-up:
- Added `iptv-tunerr plex-epg-oracle-cleanup` (dry-run by default) to remove oracle-created DVR/device rows by lineup-title prefix (`oracle-`) and/or device URI substring.
- Added Plex API helper functions in `internal/plex` for listing/deleting DVRs/devices via HTTP API.

Verification:
- `go test ./internal/plex ./cmd/iptv-tunerr -run '^$'`

---

## 2026-02-27: Resolve iptvtunerr-supervisor EPG/DVR connectivity (firewall root cause)

**Goal:** Investigate why IptvTunerr was not providing an EPG to Plex.home. Route cause and fix the network connectivity between the Plex pod (kspls0) and the iptvtunerr-supervisor pod (kspld0).

**Summary:**
- The iptvtunerr-supervisor pod was running correctly and serving all 15 tuner instances (13 category DVRs + 2 HDHR instances).
- All 15 DVRs were already registered in Plex via the in-app `FullRegisterPlex` path.
- Root cause was a dual-nftables-table problem on kspld0: `table inet host-firewall` (priority -400) had `ip saddr 192.168.50.85 accept` at the top, but `table inet filter` (priority 0, in `/etc/nftables.conf`) had policy drop with no accept rule for ports 5004/5101-5126. In nftables, all base chains at the same hook run independently in priority order—an accept from the lower-priority chain does NOT stop the higher-priority chain from dropping the packet.
- Traced using iptables RAW PREROUTING LOG (confirmed SYN arrived at kspld0), mangle INPUT LOG (confirmed packet reached mangle INPUT), then identified it was not reaching filter INPUT (the `inet filter` chain at priority 0 was the culprit).

**Fix applied (persistent):**
- Added `ip saddr 192.168.50.0/24 tcp dport { 5004, 5006, 5101-5126 } accept comment "allow iptvtunerr ports from LAN"` to `/etc/nftables.conf` `inet filter input` chain on kspld0. Backup at `/etc/nftables.conf.bak-iptvtunerr-*`.
- Also confirmed prior session fixes remain: `kspls0` `/etc/nftables.conf` forward chain allows `ip daddr 192.168.50.148 tcp dport { 5004, 5006, 5101-5126 } accept`.

**Verification:**
- Direct TCP test: `bash -c 'echo > /dev/tcp/192.168.50.148/5004'` from kspls0 → exit=0 (previously EHOSTUNREACH)
- `curl http://192.168.50.148:5004/discover` from kspls0 → `404 page not found` (connected, route exists)
- `curl http://iptvtunerr-bcastus.plex.svc:5004/device.xml` from Plex pod → valid HDHR XML response
- `curl http://iptvtunerr-bcastus.plex.svc:5004/guide.xml` from Plex pod → valid XMLTV EPG with channel data
- `GET /livetv/dvrs` Plex API → 15 DVRs registered, all pointing to `iptvtunerr-*.plex.svc:5004`

**Opportunities filed:**
- None new; dual-table pattern already in recurring_loops.md (updated with iptvtunerr-specific trace path).

---

## 2026-02-27/28: Fix CF stream rejection, EPG path bug, oracle-supervisor BaseURL, k8s manifest

**Goal:** Investigate why IPTV feeds were not working in-cluster; identify root causes; fix in source and deploy.

**Root causes found (in-cluster log investigation):**

1. **Cloudflare CDN blocking .ts segments — `IPTV_TUNERR_FETCH_CF_REJECT` not implemented:** `IPTV_TUNERR_FETCH_CF_REJECT=true` was set on the supervisor Deployment but the binary ignored it (no code). When CF blocks a stream, it redirects each `.ts` segment to `cloudflare-terms-of-service-abuse.com`, producing 0-byte segments. The stream relays blank video silently for ~12 seconds then drops with `hls-relay ended no-new-segments`.

2. **Oracle-supervisor all 6 hdhrcap instances advertising `localhost:5004` as BaseURL:** The ConfigMap for `iptvtunerr-oracle-supervisor` had no `IPTV_TUNERR_BASE_URL` per instance. All instances fell back to the default `http://localhost:5004`, which is unreachable from Plex's pod. No oracle channels were visible in Plex. Also: no `restart/restartDelay/failFast` keys in the ConfigMap (supervisor treated children as no-restart).

3. **EPG path warning on every restart for all 13 category tuners:** When `FullRegisterPlex` fails (Plex returns "device is in use"), `apiRegistrationDone` stays false. The code then called `plex.SyncEPGToPlex(*runRegisterPlex, ...)` with `*runRegisterPlex="api"`, constructing the bogus filesystem path `api/Plug-in Support/Databases/tv.plex.providers.epg.xmltv-iptvtunerr-<name>.db`. This produced `EPG sync warning: EPG database not found: api/...` on every startup for all 13 children.

4. **`docker build` does not update k3s containerd image store:** After the code fix, `docker build -t iptv-tunerr:latest .` was run and `kubectl rollout restart` was issued, but the pods kept loading the old image. k3s/containerd has a separate image store from Docker. Without `docker save | k3s ctr images import -`, `imagePullPolicy: IfNotPresent` uses the old containerd-cached image.

**Immediate live fixes (same session, before code changes):**
- Patched supervisor Deployment: set `IPTV_TUNERR_FETCH_CF_REJECT` from `false` → `true` via `kubectl patch`.
- Patched oracle-supervisor ConfigMap: added `IPTV_TUNERR_BASE_URL` per instance (`:5201`–`:5206`) plus `restart/restartDelay/failFast` via Python patch script.

**Code changes (all in working tree, not yet committed):**
- `internal/config/config.go`: added `FetchCFReject bool` + `getEnvBool("IPTV_TUNERR_FETCH_CF_REJECT", false)`.
- `internal/tuner/gateway.go`: added `errCFBlock` sentinel error; `FetchCFReject bool` field on `Gateway`; CF domain detection in `fetchAndWriteSegment`; abort path in `relayHLSAsTS` segment loop.
- `internal/tuner/server.go`: added `FetchCFReject bool` to `Server` struct, wired to `Gateway{}`.
- `cmd/iptv-tunerr/main.go`: wired `cfg.FetchCFReject` to both `Server{}` literals (serve + run commands); added guard `if *runRegisterPlex == "api"` in the `!apiRegistrationDone` block to skip file-based EPG/lineup fallback.
- `k8s/iptvtunerr-supervisor-singlepod.example.yaml`: added `IPTV_TUNERR_FETCH_CF_REJECT: "true"` env entry.
- `scripts/generate-k3s-supervisor-manifests.py`: added `IPTV_TUNERR_FETCH_CF_REJECT: "true"` to `build_singlepod_manifest()` env list.
- `k8s/iptvtunerr-oracle-supervisor.yaml` (new file): ConfigMap + Deployment + Service for oracle-supervisor pod with all 6 hdhrcap instances and correct per-instance `IPTV_TUNERR_BASE_URL`.
- `Dockerfile`: added `COPY vendor/ vendor/` + `-mod=vendor` build flag (required because docker build environment has no internet access).

**Deploy:**
```bash
go mod vendor
docker build --network=host -t iptv-tunerr:latest .
docker save iptv-tunerr:latest | sudo k3s ctr images import -
kubectl apply -f k8s/iptvtunerr-oracle-supervisor.yaml
kubectl rollout restart deployment/iptvtunerr-supervisor deployment/iptvtunerr-oracle-supervisor -n plex
```

**Verification (post-deploy):**
- Zero `EPG database not found: api/...` errors in supervisor logs; all 13 category tuners log `[PLEX-REG] API registration failed; skipping file-based fallback` instead.
- `grep -ac "cloudflare-abuse-block" /usr/local/bin/iptv-tunerr` in pod → 1 (CF reject implemented).
- `grep -ac "skipping file-based" /usr/local/bin/iptv-tunerr` in pod → 1 (EPG guard implemented).
- All 13 category DVR instances listening, serving channels, responding 200 to kube-probe `/discover.json`.
- Oracle-supervisor 6 hdhrcap instances listing correct BaseURLs (`iptvtunerr-oracle-hdhr.plex.svc:520X`), all passing readiness probes.
- Plex DVR list (`GET /livetv/dvrs`): all 13 category DVRs registered with correct guide.xml URLs: `lineup://tv.plex.providers.epg.xmltv/http://iptvtunerr-<name>.plex.svc:5004/guide.xml#iptvtunerr-<name>`.

**Notes:**
- "Plex API registration failed: create DVR: no DVR in response" on every restart is **benign** — Plex returns HTTP 200 with `status="-1"` "device is in use with an existing DVR" body, which the code correctly treats as a non-fatal miss. DVRs are already registered from a prior run.
- The "falling back to DB registration" string in that log message is a misleading legacy string — it is immediately superseded by the "skipping file-based fallback" guard for `api` mode.
- SSDP `:1900` bind errors in oracle-supervisor are expected — multiple instances in one pod compete for the UDP port; all but the first fail to bind, which is harmless since they don't need SSDP for k8s routing.

**Opportunities filed:**
- Improve `CreateDVRViaAPI` to detect Plex's `status="-1"` "device is in use" response and treat it as a success-with-existing-DVR, avoiding the misleading error log on every restart.

---

- Date: 2026-02-28
  Title: Postvalidate CDN fix, DVR cleanup, credential hygiene, VODFS remount + VOD library re-registration
  Summary:
    - Reduced `POSTVALIDATE_WORKERS` from 12 to 3 and added per-probe jitter (`POSTVALIDATE_PROBE_JITTER_MAX_S=2.0`) in `k3s/plex/iptv-m3u-server-split.yaml` and `k3s/plex/iptv-m3u-postvalidate-configmap.yaml` to prevent CDN rate-limit false-fails.
    - Removed stale oracle-era HDHR DVRs 247 (`iptvtunerrHDHR479`) and 250 (`iptvtunerrHDHR479B`) from Plex via `plex-epg-oracle-cleanup`. The 13 category DVRs (218..242) preserved.
    - Cleaned `k8s/iptvtunerr-hdhr-test.yaml`: removed deleted `plex-iptv-creds` Secret references, added OpenBao/deploy-script credential guidance comments.
    - Fixed `scripts/verify-steps.sh` format check to exclude `vendor/` (was falsely failing on third-party Go files).
    - Restarted all 11 VODFS FUSE mount processes on kspls0 (all were dead after prior Plex pod restart). Restarted Plex pod (no active sessions), re-registered all 20 VOD/lane Plex library sections via plex-vod-register from inside the new pod.
    - Documented FUSE mount propagation root cause in `memory-bank/known_issues.md`: hostPath FUSE mounts started after pod init are invisible inside the container without `mountPropagation: HostToContainer`.
  Verification:
    - `scripts/verify` — all steps OK (format excl. vendor, vet, tests, build).
  Notes:
    - User direction on postvalidate: reduce to 3 workers first; reduce to 1 if still failing.
    - VODFS remount is runtime-only — not reflected in base deployment YAMLs (no `mountPropagation` on Plex hostPath volumes yet). Filed as next follow-up.
  Opportunities filed:
    - Add `mountPropagation: HostToContainer` to Plex deployment YAML VODFS hostPath volume mounts to prevent empty-mount-after-restart.
    - Add systemd services or a node-startup script on kspls0 for VODFS lane processes to survive host reboots.

---

- Date: 2026-03-18
  Title: Catch-up publisher and Plex/Emby/Jellyfin library parity
  Summary:
    - Added `iptv-tunerr catchup-publish`, which turns near-live guide capsules into `.strm + .nfo` lane libraries plus `publish-manifest.json`.
    - Added `internal/tuner/catchup_publish.go` and tests so capsule output is now media-server-ingestible instead of remaining JSON-only.
    - Added Emby/Jellyfin library list/create/refresh helpers in `internal/emby/library.go` using `/Library/VirtualFolders` and `/Library/Refresh`.
    - Wired optional catch-up library registration for Plex, Emby, and Jellyfin from the new publisher command.
    - Updated README, features, CLI/reference docs, Emby/Jellyfin support docs, changelog, and memory-bank notes.
  Verification:
    - `go test ./internal/emby ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Published capsules are near-live launchers backed by guide windows and live-channel `.strm` targets, not archived recordings.
    - Live cluster validation on 2026-03-18 proved:
      - Emby tuner registration recovered and indexed channels
      - Jellyfin tuner registration recovered and indexed channels
      - Emby catch-up library publishing created lane libraries and on-disk `.strm + .nfo` output on the server PVC
      - Jellyfin required an additional API compatibility follow-up (`GET /Library/VirtualFolders`, query-param create on `POST /Library/VirtualFolders`) before its catch-up library publishing path succeeded live too

---

- Date: 2026-03-18
  Title: README rewrite for user-facing feature value
  Summary:
    - Reworked `README.md` to explain why IPTV Tunerr matters operationally instead of listing internal feature names.
    - Reframed the intro around common IPTV failure modes: bad guide matches, dead provider hosts, client codec quirks, and media-server integration friction.
    - Expanded the core capability, channel intelligence, provider profile, Ghost Hunter, and catch-up publishing sections with problem/solution/value language.
    - Kept the newly shipped intelligence and catch-up features visible while making their operator benefit explicit.
  Verification:
    - Docs-only review of `README.md`; no code-path changes.

---

- Date: 2026-03-18
  Title: Architecture cleanup and command dispatcher split
  Summary:
    - Rewrote `docs/explanations/architecture.md` around the real current system: core runtime, intelligence layer, and registration/publishing layer.
    - Updated `memory-bank/repo_map.md` so remotes, entrypoints, and key modules match the current repo and product surfaces.
    - Split `cmd/iptv-tunerr/main.go` command execution into command-specific files:
      - `cmd_core.go`
      - `cmd_reports.go`
      - `cmd_ops.go`
    - Kept behavior unchanged while reducing the size and responsibility concentration of the main command switch.
    - Filed maintainability follow-ups in `memory-bank/opportunities.md` for continued doc/code cleanup.
  Verification:
    - `./scripts/verify`

---

- Date: 2026-03-18
  Title: Split general deployment docs from Plex-heavy ops patterns
  Summary:
    - Added a new explanation page describing the shared Plex/Emby/Jellyfin integration path versus Plex-only operational complexity.
    - Added a new how-to page for advanced Plex patterns: wizard lane, zero-touch registration, category DVR fleets, and injected DVR layouts.
    - Updated `docs/how-to/deployment.md` and `docs/index.md` to route operators toward the right doc path instead of mixing all audiences together.
  Verification:
    - Docs-only review of the new pages and updated links.

---

- Date: 2026-03-18
  Title: Guide health report and live endpoint
  Summary:
    - Added `internal/guidehealth` to classify actual merged-guide coverage by channel: real programme rows, placeholder-only fallback, or no guide rows.
    - Added `iptv-tunerr guide-health` so operators can inspect a catalog plus served `guide.xml` and optionally include source-XMLTV match provenance in the same report.
    - Added live endpoint `/guide/health.json` so running instances expose the same guide-health view over HTTP.
    - Updated README, features, CLI reference, and changelog so the new guide-diagnostics path is discoverable.
  Verification:
    - `go test ./internal/guidehealth ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`

---

- Date: 2026-03-18
  Title: EPG doctor workflow and cached live diagnostics
  Summary:
    - Added `internal/epgdoctor` plus `iptv-tunerr epg-doctor` to combine deterministic XMLTV matching and real merged-guide coverage into one operator-facing report.
    - Added live endpoint `/guide/doctor.json` for the same combined diagnosis on running instances.
    - Added cached reuse of live guide match-provenance analysis in `XMLTV`, keyed to the current guide cache generation plus alias source, so repeated guide-health/doctor requests do not rebuild the same source-XMLTV channel match report each time.
    - Updated README, features, CLI reference, and changelog so `epg-doctor` is the recommended top-level workflow and lower-level reports remain available as supporting tools.
  Verification:
    - `go test ./internal/epgdoctor ./internal/guidehealth ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`

---

- Date: 2026-03-18
  Title: Expand recorder-daemon explainer with Plex DVR comparison
  Summary:
    - Updated `docs/explanations/always-on-recorder-daemon.md` to explain how a future always-on recorder daemon differs from Plex DVR in operating model and purpose.
    - Added the headless concurrency angle so the doc now explicitly describes policy-driven recording up to provider and system limits instead of Plex rule limits.
    - Kept the recorder-daemon concept consolidated in one future-feature explainer rather than leaving key rationale only in chat history.
  Verification:
    - Docs-only review of the updated explanation page.
