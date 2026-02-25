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
