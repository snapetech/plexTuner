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
