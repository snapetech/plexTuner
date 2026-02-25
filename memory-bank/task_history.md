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
    - kubectl must use KUBECONFIG=/home/keith/.kube/config (not default k3s /etc/rancher/k3s/k3s.yaml which is root-only)
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
    - cmd/plex-tuner/main.go: -mode=easy|full on run and serve. easy => LineupMaxChannels=479, no smoketest at index, no -register-plex; full => -register-plex uses NoLineupCap. Stderr hints updated.
    - internal/tuner/server_test.go: TestUpdateChannels easy-mode cap at 479.
    - docs/features.md: new section 6 "Two flows (easy vs full DVR builder)"; Operations renumbered to 7, Not supported to 8.
  Verification:
    - ./scripts/verify (format, vet, test, build) OK.
  Notes:
    - Easy = add tuner in Plex wizard, pick suggested guide; full = index + smoketest + optional zero-touch with -register-plex.
  Opportunities filed:
    - none
  Links:
    - docs/features.md, cmd/plex-tuner/main.go, internal/tuner/server.go

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
    - docs/adr/0001-zero-touch-plex-lineup.md, internal/plex/lineup.go, cmd/plex-tuner/main.go, memory-bank/known_issues.md, docs/features.md, docs/adr/index.md

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
    - Default M3U URL generation assumes Xtream-style `get.php`; pass `PLEX_TUNER_M3U_URL` or `--m3u-url` to override.
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
    - User must set real provider credentials in k8s/plextuner-hdhr-test.yaml (or use a Secret) before ./k8s/deploy.sh.
    - DNS: plextuner-hdhr.plex.home → Ingress; then Plex at plex.home can add DVR with Base URL and guide.xml.
  Opportunities filed:
    - none
  Links:
    - k8s/plextuner-hdhr-test.yaml, k8s/deploy.sh, k8s/README.md

- Date: 2026-02-24
  Title: HDHR k8s standup for plex.home (Agent 2)
  Summary:
    - Updated k8s/plextuner-hdhr-test.yaml: run-mode (index at startup), emptyDir catalog, BaseURL=http://plextuner-hdhr.plex.home.
    - Added Ingress for plextuner-hdhr.plex.home → plextuner-hdhr-test:5004 (ingressClassName: nginx).
    - Removed static catalog ConfigMap; run indexes from provider at startup.
    - Added k8s/README.md: build, deploy, verify, Plex setup, customization.
  Verification:
    - ./scripts/verify ✅
    - docker build (blocked: sandbox network)
    - kubectl apply (blocked: kubeconfig permission)
  Notes:
    - User must: build image, load into cluster, apply manifests, ensure DNS for plextuner-hdhr.plex.home.
    - NodePort 30004 fallback if Ingress not used.
  Opportunities filed:
    - none
  Links:
    - k8s/plextuner-hdhr-test.yaml, k8s/README.md, docs/index.md, docs/runbooks/index.md

- Date: 2026-02-24
  Title: SSDP discovery URL hardening for Plex auto-discovery (sandbox-tested)
  Summary:
    - Patched `internal/tuner/ssdp.go` to build/validate `DeviceXMLURL` from `BaseURL` instead of blindly emitting `LOCATION: /device.xml` when `BaseURL` is unset.
    - Disabled SSDP startup when `BaseURL` is empty/invalid and added a log message so operators know Plex auto-discovery requires a reachable `-base-url` / `PLEX_TUNER_BASE_URL`.
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
    - `GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gocache go test -count=1 -run '^$' ...` ⚠️ partial compile-only smoke: core packages + `internal/tuner` compile; `cmd/plex-tuner` and `internal/vodfs` blocked by sandbox DNS/socket while downloading `modernc.org/sqlite` and `github.com/hanwen/go-fuse/v2`
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
    - memory-bank/current_task.md, docs/runbooks/plextuner-troubleshooting.md, k8s/plextuner-hdhr-test.yaml

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

- Date: 2026-02-24
  Title: Instrument first-seconds WebSafe TS output and confirm clean startup TS on a fresh failing Plex Web probe
  Summary:
    - Added a TS inspector (`internal/tuner/ts_inspector.go`) and hooked it into the ffmpeg relay output path in `internal/tuner/gateway.go` so PlexTuner can log first-packet TS timing/continuity (PAT/PMT/PCR/PTS/DTS/CC/discontinuity) for targeted probe requests.
    - Built an instrumented binary locally (`/tmp/plex-tuner-tsinspect`), copied it into the existing `plextuner-build` pod, and restarted only the WebSafe process (`:5005`) with the same runtime env plus `PLEX_TUNER_TS_INSPECT=true` and `PLEX_TUNER_TS_INSPECT_CHANNEL=111`.
    - Ran a fresh Plex Web probe (`plex-web-livetv-probe.py --dvr 138 --channel 111`) and reproduced the browser failure (`detail=startmpd1_0`, ~39s elapsed) without restarting Plex.
    - Captured the new TS inspector summary for the failing probe (`req=r000001`, channel `111` / `skysportnews.de`): first 12,000 TS packets had `sync_losses=0`, PAT/PMT repeated (`175` each), `pcr_pid=0x100`, monotonic PCR/PTS on H264 video PID `0x100`, monotonic PTS on audio PID `0x101`, `idr=true` at startup gate, and no continuity errors on media PIDs (only null PID `0x1FFF` duplicate CCs).
    - Correlated PMS logs for the same live session (`c5a1eca7-f15b-4b84-b22a-fac76d1e5391` / client `157b3117a4354af68c19d075`): first-stage recorder session started in ~3.1s, Plex accepted `progress/stream` + `progress/streamDetail` (H264 + MP3), but `decision` and `start.mpd` still completed only after ~100s, when PMS finally launched the second-stage DASH transcode reading `http://127.0.0.1:32400/livetv/sessions/.../index.m3u8`.
  Verification:
    - `PATH=/tmp/go/bin:$PATH /tmp/go/bin/go test ./internal/tuner -run '^$' -count=1` (compile-only pass)
    - `python3 /home/coder/code/k3s/plex/scripts/plex-web-livetv-probe.py --dvr 138 --channel 111 --hold 3 --json-out /tmp/probe-138-111.json` (expected fail: `startmpd1_0`)
    - `kubectl -n plex exec deploy/plextuner-build -- tail/grep /tmp/plextuner-websafe-tsinspect.log`
    - `kubectl -n plex exec -c plex deploy/plex -- grep ... \"Plex Media Server.log\"` (session `c5a1eca7-...`)
  Notes:
    - No Plex restart. Only the WebSafe `plex-tuner serve` process inside `plextuner-build` was restarted.
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
    - This strengthens the hypothesis that the remaining Plex Web blocker is in Plex's internal segment-info/manifest path (between first-stage `ssegment` output files and `/livetv/sessions/.../index.m3u8` readiness), not in PlexTuner stream startup.
    - The WebSafe process remains instrumented, but TS inspection is still scoped to channel match `111`; the `108` probe did not add TS-inspector log noise.
  Opportunities filed:
    - none
  Links:
    - memory-bank/current_task.md, memory-bank/known_issues.md, /tmp/probe-138-108.json
