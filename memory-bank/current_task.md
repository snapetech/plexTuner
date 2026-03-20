# Current task

<!-- Update at session start and when focus changes. -->

**Latest (2026-03-20):** **Release cut `v0.1.18`:** package the lineup-integrity logs, first-run mapping repair, guide-force-lineup-match mode, and new **`/guide/lineup-match.json`** debug surface into the next patch build. **`./scripts/verify`** OK; next is pushing the release tag and watching the workflow.

**Latest (2026-03-20):** **Guide debug payload enrichment:** **`/guide/lineup-match.json`** sampled missing rows now include observed **`channel_id`** and **`tvg_id`** in addition to **`guide_number`** / **`guide_name`** / URL so tester reports show real upstream linkage state instead of only lineup labels.

**Latest (2026-03-20):** **Post-release regression + test-cost fix:** the exact-URL guide-input hardening regressed first-run automatic channel mapping when provider/XMLTV refs were supplied at runtime instead of env; internal callers now pass their exact trusted refs explicitly into **`guideinput`**, restoring runtime EPG repair / guide-health flows without reopening generic remote fetches. Also cut the worst HLS relay test from ~12s wall-clock to ~1s by overriding the relay stall/sleep hooks in-test only. **`./scripts/verify`** OK.

**Latest (2026-03-20):** **Live shard validation + lineup integrity logs:** swept 18 live ports (**5004**, **5006–5013**, **5101–5103**, **5201–5206**) and every sampled **`lineup.json`** matched **`guide.xml`** exactly with zero malformed rows or duplicate guide numbers. Added a concise **`UpdateChannels`** integrity summary log (**channels / epg_linked / with_tvg / with_stream / missing_core / duplicate_guide_numbers / duplicate_channel_ids**) so future tester reports identify bad generated shards immediately.

**Latest (2026-03-20):** **Guide match guarantee mode:** added an explicit XMLTV emission mode so **`IPTV_TUNERR_EPG_FORCE_LINEUP_MATCH=1`** keeps every lineup channel represented in **`guide.xml`** even when **`IPTV_TUNERR_EPG_PRUNE_UNLINKED=1`** is enabled. This is aimed at Plex first-run matching: unmatched channels keep placeholder guide rows instead of disappearing from the guide output.

**Latest (2026-03-20):** **Guide-to-lineup debug surface:** added **`/guide/lineup-match.json`** so operators can see current lineup count, guide count, exact-name match coverage, duplicate guide names/numbers, and a sample of unmatched lineup rows without scraping XML manually.

**Latest (2026-03-20):** **INT-005/INT-010 bridge:** Autopilot now supports a JSON **host policy file** (**`IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE`**) with **preferred** and **blocked** hosts. Preferred hosts merge with **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`**; blocked hosts are skipped in **`reorderStreamURLs`** only when backups remain. Runtime/report/docs/tests updated.

**Latest (2026-03-20):** **INT-008 follow-up:** Ghost Hunter now has first-class operator actions: **`POST /ops/actions/ghost-visible-stop`** and **`POST /ops/actions/ghost-hidden-recover?mode=dry-run|restart`**, reusing the guarded helper path (**`IPTV_TUNERR_GHOST_HUNTER_RECOVERY_HELPER`** override). Control-deck actions/workflows, tests, and docs updated.

**Latest (2026-03-20):** **Docs:** **[project-backlog.md](../docs/explanations/project-backlog.md)** audit — **§1 shipped** vs **§2 open**; **opportunities** statuses (HLS toolkit doc, hidden-grab partial). **`./scripts/verify`** OK.

**Latest (2026-03-20):** **Docs:** **[docs/explanations/project-backlog.md](../docs/explanations/project-backlog.md)** — canonical “open work” index (links epics, **opportunities**, **known_issues**, **docs-gaps**, **README** / **AGENTS** / **repo_map** / **EPIC** See also). **`./scripts/verify`** OK.

**Latest (2026-03-19):** **Control Deck:** **`deck.js`** provider summary + Watch/Routing for **host quarantine**; **EPIC-live-tv-intelligence** observability line; **CHANGELOG** / **features**. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **INT-010:** **`upstream_quarantine_skips_total`** on **`/provider/profile.json`** (cumulative; mirrors Prometheus counter). **`./scripts/verify`** OK.

**Latest (2026-03-19):** **INT-010 follow-up:** **`iptv_tunerr_upstream_quarantine_skips_total`** (Prometheus when **`IPTV_TUNERR_METRICS_ENABLE`**), **`promRegisterUpstreamMetrics`** on tuner start; extra **`filterQuarantinedUpstreams`** / **`ServeHTTP`** tests; docs (**cli-and-env**, **CHANGELOG**, **features**, **observability-prometheus-and-otel**). **`go mod tidy` + `go mod vendor`** ( **`prometheus/testutil`** ). **`./scripts/verify`** OK.

**Latest (2026-03-19):** **LTV epic code:** **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`** + optional **`IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE`** (**`walkStreamUpstreams`**, **`/provider/profile.json`**, **`/debug/runtime.json`**). **`./scripts/verify`** OK.

**Latest (2026-03-20):** **INT-010 next slice:** promote provider intelligence into **active remediation** with optional runtime **host quarantine**. Scope: repeated host failures can temporarily quarantine a bad upstream host (with threshold + cooldown env knobs), stream selection skips quarantined hosts when backups exist, provider profile surfaces active quarantine state, tests/docs updated.

**Latest (2026-03-19):** **Closed last docs gap:** [architecture.md](../docs/explanations/architecture.md) **Visual (Mermaid)**; [docs-gaps.md](../docs/docs-gaps.md) has no open High/Medium/Low rows. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **Backlog validation:** [docs-gaps.md](../docs/docs-gaps.md) stale rows removed (cli-and-env + existing docs cover prior “gaps”); [EPIC-live-tv-intelligence](../docs/epics/EPIC-live-tv-intelligence.md) / [EPIC-lineup-parity](../docs/epics/EPIC-lineup-parity.md) + [work_breakdown](work_breakdown.md) + [opportunities](opportunities.md) (guide-health opportunity partially superseded). **CHANGELOG** doc bullet.

**Latest (2026-03-19):** **LTV `INT-006` — hot-start by M3U `group_title`:** **`IPTV_TUNERR_HOT_START_GROUP_TITLES`**; **`/debug/runtime.json`** **`tuner.hot_start_*`**; tests **`gateway_hotstart_test.go`**. **`Gateway`**: removed dead **`hlsPackager*`** fields (undefined type). **`./scripts/verify`** OK.

**Latest (2026-03-20):** **LP-010 / LP-011:** named profiles can now prefer **`output_mux: "hls"`** for **ffmpeg-packaged HLS**. Tunerr starts a short-lived packager, keeps a background tuner hold while it runs, serves packaged playlist/segment files back through Tunerr session URLs, and leaves explicit **`?mux=hls`** on the existing native rewrite/proxy path. Docs/tests updated; **`./scripts/verify`** OK.

**Latest (2026-03-19):** **Closure — Prometheus Autopilot consensus metrics + Plex onboarding doc:** **`internal/tuner/prometheus_autopilot.go`** + **`prometheus_autopilot_test.go`**; **`server.go`** registers when **`IPTV_TUNERR_METRICS_ENABLE`**. **Docs:** [how-to/connect-plex-to-iptv-tunerr.md](../docs/how-to/connect-plex-to-iptv-tunerr.md); **README** / **`docs/index`** / **`how-to/index`** / **`reference/index`**; **`docs-gaps`** Resolved + high table trim; **`cli-and-env`** **`METRICS_ENABLE`**; **`features.md`**; **CHANGELOG**; **`opportunities.md`** Plex connect → **Shipped**. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **Control Deck + Autopilot consensus:** **`deck.js`** **`summarizeProviderProfile`** + **`formatAutopilotConsensusMeta`**; **Watch** / **wins** / **Operations** Autopilot card; endpoint catalog. **CHANGELOG** + **features**. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **LTV Autopilot consensus host:** **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST`** (opt-in) + **`_MIN_DNA`** / **`_MIN_HIT_SUM`**; **`consensusPreferredHost`** in **`autopilot.go`**; **`autopilotPreferredStreamURL`** fallback; **`AutopilotReport`** + **`intelligence.autopilot`** fields; tests **`TestAutopilot_consensusPreferredHost`**, **`TestGateway_reorderStreamURLs_autopilotConsensusHost`**. Docs: **CHANGELOG**, **cli-and-env**, **.env.example**, **features**, **EPIC-live-tv-intelligence**. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **LP-004 Control Deck:** **`deck.js`** **`summarizeProviderProfile`** + **`remediationHintsFromProfile`** — Overview/Routing provider cards use real **`/provider/profile.json`** fields; **`remediation_hints`** on incidents, watch list, decision board, routing. **CHANGELOG** + **`current_task`**. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **LP-001 / HDHR:** **`IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS`** supports **literal IPv6** targets (plus **`[addr]:port`**, **`fe80::1%eth0:65001`**-style); **`DiscoverLAN`** uses UDP6 + merges with IPv4 broadcast. Docs: **cli-and-env**, **hybrid-hdhr**, **CHANGELOG**, **EPIC-lineup-parity**, **work_breakdown**. **`./scripts/verify`** OK; **`task_history`** entry.

**Latest (2026-03-19):** **LTV:** **`GET /provider/profile.json`** now includes **`remediation_hints`** — stable **`code`** / **`severity`** / **`message`** / optional **`env`** suggestions from live counters (CF blocks, penalized hosts, concurrency, mux 502/503/rate-limit). Advisory only. **`./scripts/verify`** OK; **`task_history`** entry.

**Latest (2026-03-22):** **Run startup visibility:** `run` now binds the tuner before long catalog/guide warm-up completes, so `/healthz` and `/readyz` expose `loading` / `not_ready` during startup. Added catalog phase timing logs plus `IndexFromPlayerAPI` substep timings so provider stalls identify the exact slow phase.

**Latest (2026-03-22):** **Backlog → shipped:** [how-to/interpreting-probe-results.md](../docs/how-to/interpreting-probe-results.md); **`scripts/harness-index.py`**; README/runbook/**features**/**docs-gaps Resolved**/**commands.yml** **`harness_index`**; **`opportunities.md`** probe + harness-index entries marked shipped. **`./scripts/verify`** OK.

**Latest (2026-03-22):** **Docs + backlog:** [how-to/stream-compare-harness.md](../docs/how-to/stream-compare-harness.md) (**§9** lead-in, harness trilogy cross-links, **`features`**, **`docs-gaps` Resolved**); **`memory-bank/opportunities.md`** — harness index helper, **probe** interpretation how-to, **Plex connect** how-to. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Docs:** [how-to/live-race-harness.md](../docs/how-to/live-race-harness.md) (parity with multi-stream); **runbook §7** lead-in; **`commands.yml`** **`live_race_harness`**; **multi-stream** related-harness link **§6→§7** fix. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Docs:** [how-to/multi-stream-harness.md](../docs/how-to/multi-stream-harness.md) + index/README/runbook §10/repo_map cross-links (**two-stream collapse** harness). **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Verify hygiene:** **`scripts/verify-steps.sh`** runs **`bash -n`** on **`scripts/*.sh`** and **`python3 -m py_compile`** on **`scripts/*.py`** ( **`commands.yml`** **`verify_steps`**, **`repo_map`**). **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Two-stream collapse harness:** adding **`scripts/multi-stream-harness.sh`** + **`scripts/multi-stream-harness-report.py`** so “load one channel, start another, first dies” reports turn into a reproducible bundle with staggered live pulls, provider/runtime/attempt snapshots, optional Plex session evidence, and a compact sustained-vs-premature report.

**Latest (2026-03-21):** **Test hardening:** **`TestGateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry`** uses completion signal (no **1.5s** sleep); **`TestGateway_shouldPreferGoRelayForHLSRemux_hostPenalty`** **`autotune_off`** subtest. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **HLS go-relay + tests (restored):** **`shouldPreferGoRelayForHLSRemux(streamURL)`** considers **`hostPenalty`** for flaky hosts; **`TestGateway_shouldPreferGoRelayForHLSRemux_hostPenalty`**, **`TestGateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry`**; **CHANGELOG** + **`recurring_loops`** note: do not **`git restore`** unrelated dirty files (multi-agent WIP). **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Handoff / doc closure:** Confirmed gateway integration test **`TestGateway_stream_prefersAutopilotRememberedURL_normalizedTrailingSlash`** lives on **`origin/main`** (not part of the LP/LTV feature commit); expanded **`streamURLsSemanticallyEqual`** godoc + **`known_issues.md`** **Gateway / Autopilot** row so “by design” URL-match limits are single-source in code + memory bank. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Close-by-design slice:** Autopilot **URL normalization**; **`IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS`**; **LP-012** checklist **`lineup-parity-lp012-closure.md`** + indexes; **cli-and-env**, **.env.example**, **CHANGELOG**, epics, **hdhr-scan** summary. **`./scripts/verify`** OK.

**Latest (2026-03-20):** **LP / LTV slice:** **`/provider/profile.json`** → **`intelligence.autopilot`**; **stream-investigate** workflow actions; **EPIC-lineup-parity** implementation status sync; **EPIC-live-tv-intelligence** current status; **hybrid-hdhr** §6 LTV table; **features** + **CHANGELOG** + **work_breakdown**. **`./scripts/verify`** OK.

**Latest (2026-03-20):** Doc sync: **README** documentation map + k8s probes + recent bullets; **`docs/features.md`** (**`/readyz`**, native mux header, profiles, harness); **`docs/index`**, **`reference/index`**, **`runbooks/index`**, **`how-to/index`** cross-links; **CHANGELOG** [Unreleased] **Documentation** section.

**Latest (2026-03-20):** Closed test gaps: **`internal/probe/probe_test.go`** (~92% stmts) + **`internal/materializer/materializer_test.go`** (~71% stmts; HLS/ffmpeg still integration-only); **`CHANGELOG`**, **`commands.yml`** note; **`./scripts/verify`** OK.

**Latest (2026-03-20):** Documented **`GET /readyz`** (already in **`server.go`**); **k8s** examples use **`/readyz`** for **`readinessProbe`**; runbook §8, architecture, static UI, **`CHANGELOG`**, **LP-012**, opportunities superseded row, **`work_breakdown`**. **`./scripts/verify`** OK.

**Latest (2026-03-20):** Superseded **smoketest disk cache** opportunity (already shipped); **plex-livetv-http-tuning** + **hybrid-hdhr** + **k8s/README** cross-links; **repo_map** indexer smoketest note. **`./scripts/verify`** OK.

**Latest (2026-03-19):** Priority sweep **4→1**: native mux **`X-IptvTunerr-Native-Mux`** + toolkit/runbook; **HR-002** checklist in troubleshooting; superseded ops opportunities (**Save/SIGHUP/healthz**); k8s readiness **`/healthz`**; **EPIC-lineup-parity** **LP-010** = **`STREAM_PROFILES_FILE`**. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **`opportunities.md`**: superseded duplicate **XMLTV `/guide.xml` cache** backlog items (**2026-02-24** / **2026-02-25**) — behavior is **`xmltv.go`** merged-guide **`cachedXML`** + TTL + **`TestXMLTV_cacheHit`**. **CHANGELOG** note. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **`gateway_profiles_test.go`** expanded: **`loadNamedProfilesFile`** + **`resolveProfileSelection`** coverage for **`STREAM_PROFILES_FILE`**; **`opportunities.md`** wget item superseded. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **`IPTV_TUNERR_STREAM_PROFILES_FILE`** documented (named profile matrix, **LP-010**); **`potential_fixes.md`** link + docs index; **CHANGELOG** restore **`HTTP_*`** scope line; **`repo_map`** → **`gateway_profiles.go`**. **`gofmt`** on **`gateway_profiles.go`**. **`./scripts/verify`** OK.

**Latest (2026-03-19, low-overlap follow-up):** Continued on the non-epic lane alongside parallel product work. **`gateway_stream_upstream.go`** is slimmer again: non-OK upstream handling + success relay branches moved into **`gateway_stream_response.go`**. Native mux operability also improved: **`/provider_profile.json`** now exposes **`last_hls_mux_outcome`** / **`last_dash_mux_outcome`** with redacted target URLs + timestamps so operators can see the latest mux failure/success reason without scraping logs. **`./scripts/verify`** OK.

**Latest (2026-03-19, HR-002 harnessing):** Bridging the repo-local proof gap for Plex Web startup validation. Current slice wires the optional external **`plex-web-livetv-probe.py`** into **`scripts/live-race-harness.sh`** via **`PWPROBE_SCRIPT`** / **`PWPROBE_ARGS`**, captures probe JSON/log/exit code in the harness bundle, and teaches **`live-race-harness-report.py`** to summarize those artifacts when present.

**Latest (2026-03-19):** **architecture.md** + **reference/index.md** aligned with **`cmd_*`** / **`gateway_*`** layout; **`opportunities.md`** clears two obsolete doc/indexer tickets. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **cli-and-env** documents **`IPTV_TUNERR_HTTP_*`** scope across subsystems; **`opportunities.md`** drops duplicate **`main.go`** split ticket (superseded by **INT-005**). **`./scripts/verify`** OK.

**Latest (2026-03-19):** **HR-010** — **`plex-livetv-http-tuning`** documents full **`httpclient`** footprint; **`gateway_test`** mux scheme test uses **`httpclient.Default()`**. **`./scripts/verify`** OK.

**Latest (2026-03-19):** **HR-010** — **`internal/plex`** (dvr + library), **`internal/provider/probe`**, **`internal/emby`** now use **`httpclient.WithTimeout`** (same timeouts as before). **`./scripts/verify`** OK.

**Latest (2026-03-19):** **HR-010** continued: **`httpClientOrDefault`** (EPG pipeline), **`internal/health`**, **`internal/probe`** use **`httpclient.WithTimeout`**; architecture doc tuner pointers updated. **`./scripts/verify`** OK.

**Latest (2026-03-19):** Lineup-parity **documentation + hygiene slice:** **`gateway_upstream_cf.go`** (**`tryRecoverCFUpstream`**); **`internal/hdhomerun`** + **`hdhr-scan`** on **`httpclient`**; [EPIC-lineup-parity](docs/epics/EPIC-lineup-parity.md) **implementation status**; **`work_breakdown`** LP progress; **`opportunities.md`** superseded stale audit rows; **hls-mux-toolkit** related-code paths updated. **`./scripts/verify`** OK. *Deferred (multi-PR):* SQLite guide **LP-007–009**, Postgres, incremental XMLTV contract, always-on recorder.

**Latest (2026-03-19):** **INT-006** follow-up: upstream URL loop + stream dispatch extracted to **`gateway_stream_upstream.go`** (**`walkStreamUpstreams`**); **`gateway_servehttp.go`** is tuner slot + **`ServeHTTP`** wiring only. **INT-001 tail** + prior **`gateway_*`** splits unchanged. **`./scripts/verify`** OK.

**Latest (2026-03-19, work breakdown begin→end):** Working the intelligence cross-wiring epic from the **front** while another agent works the **back**. Current slice is **`INT-001`**: new shared **`internal/guideinput`** helpers centralize provider XMLTV URL generation plus local-file / URL loading for guide XML, XMLTV channels, alias overrides, and match reports on the repo’s shared HTTP path. Report tooling, catch-up preview helpers, and tuner guide-health callers are rewired; next is full **`./scripts/verify`**, then landing **`INT-001`** cleanly before moving into the first real **`INT-002`** gap.

**Latest (2026-03-19, work breakdown begin→end, INT-002):** Guide policy is being promoted from a hidden boolean filter into a reusable decision surface. Current code adds **`GuidePolicySummary`** / **`GuidePolicyReport`**, richer policy-application logging, **`/guide/policy.json`**, and catch-up preview metadata that shows what the active guide policy kept or dropped and why.

**Latest (2026-03-19, work breakdown begin→end, INT-003/INT-004 audit):** The runtime lineup and catch-up paths were already consuming guide policy from the earlier cross-wiring work; this pass confirmed that **`UpdateChannels`**, **`/guide/capsules.json`**, **`catchup-capsules`**, and **`catchup-publish`** were already on the policy path. The real missing piece was inspectability, which **`INT-002`** now supplies through **`/guide/policy.json`** and catch-up preview policy summaries. Current next step: land the small **`INT-005`** CLI registry cleanup so command aggregation/indexing is owned by **`cmd_registry.go`** with tests instead of remaining ad hoc in **`main.go`**.

**Latest (2026-03-19, work breakdown HR-006):** **`catalog.ReplaceWithLive`** sorts live rows by **`channel_id`** for stable **`catalog.json`** / lineup order when M3U order drifts. **`./scripts/verify`** OK.

**Latest (2026-03-19, work breakdown HR-007):** **`TRANSCODE_OVERRIDES_FILE`** merges with **`STREAM_TRANSCODE`** **`off`/`on`/`auto`** (per-channel remux/transcode vs global); policy logs + **`gateway_policy_test.go`**; runtime paths in **`/debug/runtime.json`**. **`./scripts/verify`** OK.

**Latest (2026-03-19, work breakdown end→begin):** **HR-010**: shared HTTP idle pool env + **`plex-livetv-http-tuning`** ref + runtime echo. **HR-009**: DVR recording soak checklist in runbook §9. **HR-008**: live-path failover vs **`seg=`** diagnostics documented. **`./scripts/verify`** OK.

**Latest (2026-03-19, mux regression-fixture closure):** Promoted synthetic stream-compare captures into committed HLS and DASH goldens under **`internal/tuner/testdata/`** and finished the native mux follow-up around them. HLS rewrite now strips a leading **UTF-8 BOM**, rewrites non-standard **`URI='...'`**, and keeps strict golden bodies. DASH rewrite/expansion now covers single-quoted URL attrs, quote-aware **`SegmentTimeline`** **`<S>`** scanning, paired **`SegmentTemplate`**, **`$Time$`** / padded **`$Number%0Nd$`**, and a DASH stream-compare golden that intentionally runs with **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE=1`** so expected output is fully expanded **`SegmentList`** + Tunerr proxy URLs. Docs, testdata README, `.gitignore` for **`.diag/`**, and runbook guidance for promoting captures to fixtures are all aligned. **`./scripts/verify`** OK.

**Latest (2026-03-19, mux toolkit — continuation):** **`SegmentTimeline`** **`<S></S>`** (empty paired) + **UTF-8 BOM** strip on HLS/DASH rewrite (**`stripLeadingUTF8BOM`** in **`gateway_support.go`**). Docs/tests/CHANGELOG. **`./scripts/verify`** OK.

**Latest (2026-03-19, mux toolkit — scope completion):** DASH **single-quoted** URL attrs; **`dashSegQueryEscape`** restores **`$Number%05d$`** / **`$Time%…$`**; **`SegmentTemplate`** expand: **paired** tags, **`SegmentTimeline`** + **`$Time$`**, **`$Number%0Nd$`**, skip nested self-close inside paired; HLS **`URI='...'`** rewrite. Docs + tests + fuzz. **`./scripts/verify`** OK.

**Latest (2026-03-19, mux toolkit follow-up):** **`/debug/runtime.json`** echoes **`hls_mux_dash_expand_*`**; fuzz corpus seeds for merged **EXTINF/BYTERANGE** + **SegmentTemplate** MPD; **`hls-mux-proxy` how-to** + **`repo_map`** pointer to **`gateway_dash_expand.go`**. **`./scripts/verify`** OK.

**Latest (2026-03-19, mux toolkit backlog):** **DASH** optional **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE`** expands uniform self-closing **`SegmentTemplate`** → **`SegmentList`** (**`gateway_dash_expand.go`**, wired in **`rewriteDASHManifestToGatewayProxy`**). **HLS** splits non-standard **`#EXTINF:...,BYTERANGE=...`** into **`#EXTINF`** + **`#EXT-X-BYTERANGE`**. Docs (toolkit, LL-HLS tags, CLI/env, **`.env.example`**, CHANGELOG), unit tests. **`./scripts/verify`** OK.

**Latest (2026-03-19, mux nice-to-haves):** **DASH** **`$Number$`** preserved in **`seg=`** (**`dashSegQueryEscape`** / **`gatewayDashMuxProxyURL`**); **LL-HLS** **`URI=`** tags + conservative same-line **`#EXTINF`** (**`docs/reference/hls-mux-ll-hls-tags.md`**); **`IPTV_TUNERR_HTTP_ACCEPT_BROTLI`**; Prometheus **`iptv_tunerr_mux_seg_request_duration_seconds`** + optional **`IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS`**; **Autopilot** **`IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS`**; runtime snapshot keys; **andybalholm/brotli** vendored. **`./scripts/verify`** OK.

**Latest (2026-03-19, native mux closure):** **Redirect-hop** validation on **`seg=`** (`mux_http_client` + **`safeurl.ValidateMuxSegTarget`**), richer **DASH** rewrite (relative **`media=`** / **`init=`** / **`<BaseURL>`** chain; skip **`$`** templates), **`IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO`** adaptive bonus, **`IPTV_TUNERR_HLS_MUX_ACCESS_LOG`**, golden **`testdata/hls_mux_small_playlist.golden`**, integration tests (**302→private**, chunked), **ADR 0005** (no disk packager), **OTEL** explanation doc (Prometheus scrape via collector). **`./scripts/verify`** OK.

**Latest (2026-03-19, native mux epic):** Shipped **`?mux=dash`** (experimental MPD rewrite), DNS **`IPTV_TUNERR_HLS_MUX_DENY_RESOLVED_PRIVATE_UPSTREAM`**, per-IP **`IPTV_TUNERR_HLS_MUX_SEG_RPS_PER_IP`**, Prometheus **`/metrics`** + **`iptv_tunerr_mux_seg_outcomes_total`**, **`hls_mux_diag`** logs, **`POST /ops/actions/mux-seg-decode`**, **`/debug/hls-mux-demo.html`**, **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST`**, fuzz/soak/docs/vendor; webui tests aligned with **`deckSession`** + POST logout.

**Latest (2026-03-19, HLS mux):** Implemented toolkit backlog slice: **`seg=`** length cap, optional literal-private IP block, JSON mux errors, line splitting without scanner limits, CORS allow **correlation** headers, upstream **HEAD** for **`seg=`**, **`hls_mux_seg_*`** counters on **`provider_profile.json`** + runtime env echo, **`safeurl`** tests; fixed **`internal/webui`** **`go vet`**/test break (**`logout`**, **`strconv`**, test **`Server`** setup). **`./scripts/verify`** OK.

**Latest (2026-03-19, docs):** Added **`docs/reference/hls-mux-toolkit.md`** — operator quick map, **`X-IptvTunerr-Hls-Mux-Error`**, stream-attempt statuses, **`curl`** recipes, and a **large categorized enhancement backlog** (LL-HLS, SSRF, metrics, DRM policy, tests, …); linked from index, how-to, transcode ref, **repo_map**, CHANGELOG **[Unreleased]**; **opportunities.md** meta entry.

**Latest (2026-03-19, gateway):** HLS mux: **`IPTV_TUNERR_HLS_MUX_CORS`**, segment concurrency caps, SAMPLE-AES/SESSION-KEY rewrite hardening, **400** + **`hls_mux_unsupported_target_scheme`** for non-http(s) **`seg=`** (e.g. **`skd://`**) before acquiring seg slots; tests + docs + **`./scripts/verify`** OK.

**Latest (2026-03-20):** Continuing the web UI from “sticky operator cockpit” toward shared operator memory. Current slice adds a server-backed deck telemetry endpoint in `internal/webui` so trend cards can use shared in-process history across reloads and browsers hitting the same deck, while leaving per-user UI prefs in client-side `localStorage`. In the same cleanup pass, the gateway HLS mux path now has explicit browser/CORS hooks and bounded segment-proxy concurrency knobs instead of treating every `?mux=hls&seg=` request as unbounded.

**Latest (2026-03-20, auth + persistence):** The dedicated deck now gates the whole `internal/webui` origin behind HTTP Basic auth, defaulting to `admin` / `admin` unless `IPTV_TUNERR_WEBUI_USER` / `IPTV_TUNERR_WEBUI_PASS` override it. Shared deck telemetry/history can also persist across web UI restarts with `IPTV_TUNERR_WEBUI_STATE_FILE`, and the runtime snapshot/UI now explicitly call out whether the deck is still on default creds and whether memory is durable or only process-local.

**Latest (2026-03-20, session UX):** Replaced the bare browser auth prompt with a dedicated deck login page and cookie-backed session flow on the `internal/webui` origin, while keeping direct HTTP Basic auth as a fallback for scripts and API clients. The deck now has an explicit sign-out control and redirects back to `/login` if the session expires during live use, so the front door finally feels like product UX instead of infra chrome.

**Latest (2026-03-20, operator activity):** Added shared deck activity memory on the dedicated web UI side so the control plane records operator behavior, not only system state. The deck now exposes `/deck/activity.json`, persists that activity alongside deck telemetry when `IPTV_TUNERR_WEBUI_STATE_FILE` is configured, records login/logout/memory-clear/action events, and surfaces the shared activity timeline in overview + ops.

**Latest (2026-03-20, deck productization pass):** Continuing the dedicated web UI from “credible internal control deck” toward a safer, fuller operator console. Current slice is deliberately not another cosmetic pass: it adds CSRF/session hardening for state-changing deck flows and expands the Settings lane into a more comprehensive control surface using the existing runtime/action/endpoint data instead of leaving it as a thin summary list.

**Goal:** Start the new Live TV Intelligence product track: map the multi-PR roadmap, then ship the first visible foundation feature so IPTV Tunerr feels like an intelligent control plane instead of only a tuner bridge.

**Approved epic (2026-03-19):** User confirmed **all four** tracks in [docs/epics/EPIC-lineup-parity.md](../docs/epics/EPIC-lineup-parity.md) — real HDHomeRun **client**, **web dashboard**, **SQLite EPG** model, **HLS/fMP4 profiles** (see stories `LP-001`–`LP-012`). Implementation is multi-PR; do not scope-creep unrelated refactors.

**Shipped 2026-03-19 (recorder + CF ops bundle):** Same-spool HTTP Range resume for `catchup-daemon`, Retry-After + status-aware transient backoff, capture metrics on items and in `recorder-state.json` statistics; CF learned persistence, `cf-status`, host UA override, CF bootstrap header parity + freshness monitor. Docs: `docs/CHANGELOG.md` [Unreleased], `docs/features.md`, `docs/reference/cli-and-env-reference.md`; history: `memory-bank/task_history.md`.

**Shipped 2026-03-19 (recorder gaps):** Multi-upstream capture failover (Tunerr URL + catalog `stream_url`/`stream_urls`), catalog `preferred_ua` on capture requests, time-based completed retention (`-retain-completed-max-age*`), `scripts/recorder-daemon-soak.sh`, metrics `capture_upstream_switches` / `sum_capture_upstream_switches`.

**Release v0.1.14 (2026-03-19):** `debug-bundle` CLI, `analyze-bundle.py`, CF + debug how-tos, README updates, `IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS` for capture fallback ordering; tag `v0.1.14` (v0.1.13 already existed on remote).

**Scope:** In: roadmap/epic documentation, channel intelligence reporting (`channel-report` + `/channels/report.json`), EPG match provenance visibility, lineup recipes, Channel DNA foundation, Autopilot decision-memory foundation, Ghost Hunter visible-session foundation, provider behavior profile foundation, README/features/reference/changelog updates, memory-bank updates, local verification. Out: catch-up capsules, active provider self-tuning defaults, hidden-grab Ghost Hunter automation, and a complete cross-provider identity graph in one patch.

**Last updated:** 2026-03-19 (named profiles doc + **`potential_fixes`** hygiene)

**Recorder follow-on slices (2026-03-19):**
- Transient capture retries: `recordCatchupCapsuleWithRetries` + `IsTransientRecordError`, CLI `-record-max-attempts` / `-record-retry-backoff` / `-record-retry-backoff-max` (defaults 1 / 5s / 2m).
- Budget visibility: `statistics.lane_storage` on `recorder-state.json` with `used_bytes` and optional `budget_bytes` / `headroom_bytes`.
- Publish ops: `-defer-library-refresh` + `OnManifestSaved` runs full-manifest library refresh after `recorded-publish-manifest.json` write; `LoadRecordedCatchupPublishManifest` for the hook path.
- Docs/changelog/features updated; tests for retry classification, backoff, daemon retry integration, lane stats, manifest load, and CLI hooks.

**Recorder spool/finalize (2026-03-19):**
- `RecordCatchupCapsule` streams to `<lane>/<sanitized-capsule-id>.partial.ts`, removes stale spool, then `Rename`s to `.ts` only after HTTP 200, successful `io.Copy`, and a clean context (no more half-written “final” assets on failure).
- Exported `CatchupRecordArtifactPaths`; `catchup-daemon` sets active `output_path` to the spool file while recording so restarts and pruning align with on-disk bytes.
- Tests cover path derivation, successful finalize (spool removed), and deadline/cancel leaving a spool artifact without a final `.ts`.
- Docs: `docs/CHANGELOG.md` [Unreleased], `docs/features.md`, `docs/reference/cli-and-env-reference.md`.

**Current focus shift (direct-vs-Tunerr stream debug harness, 2026-03-19):**
- User asked to build out a real troubleshooting harness for the remaining provider/CDN weirdness, explicitly including tools like `ffplay` and packet capture so direct upstream pulls can be compared against Tunerr pulls.
- This pass covers:
  1. add a reproducible comparison harness that can run direct URL and Tunerr URL fetch/playback attempts side by side
  2. capture the evidence operators actually need (`ffprobe`/`ffplay` logs, optional `tcpdump` pcaps, headers, byte samples, summary)
  3. document the workflow in the troubleshooting/runbook path so future CF/CDN debugging uses one standard lane instead of ad hoc shell history
- Assumptions:
  1. Wireshark itself does not need to be embedded; generating `.pcap` artifacts and analysis hints is the useful part
  2. the harness should work against an already running tuner or a hand-supplied direct upstream URL without requiring Kubernetes/Plex
- Result:
  1. added `scripts/stream-compare-harness.sh` plus `scripts/stream-compare-report.py` for direct-vs-Tunerr `curl`/`ffprobe`/`ffplay` comparison with optional `tcpdump`
  2. added app-side structured debug export at `/debug/stream-attempts.json` so the harness can pull Tunerr's own per-upstream decisions instead of only external tool logs
  3. documented the workflow in `docs/runbooks/iptvtunerr-troubleshooting.md` and `memory-bank/commands.yml`
  4. verified the harness with a clean-cwd local smoke against a synthetic HLS source plus local `iptv-tunerr serve`, including automatic fetch of `tunerr/stream-attempts.json`
  5. documented the recurring `.env` contamination trap for synthetic/local harnesses in `memory-bank/recurring_loops.md`

**Current focus shift (recorder daemon MVP, 2026-03-19):**
- User asked to implement the future recording feature seriously, with support for recording across as many feeds as the app can support.
- This is now tracked in `memory-bank/work_breakdown.md` under `REC-001` through `REC-003`.
- Current PR-sized slice:
  1. `REC-001` policy-driven recorder daemon MVP
  2. use existing catch-up capsule/recording primitives instead of inventing a second unrelated recording path
  3. persist enough scheduling/recording state to survive restarts and make later retention/publishing work possible
- Result:
  1. added `iptv-tunerr catchup-daemon`, which continuously scans guide capsules, schedules eligible `in_progress` / `starting_soon` recordings, and records multiple items concurrently
  2. added persistent recorder state with `active` / `completed` / `failed` buckets in `recorder-state.json`
  3. refactored catch-up recording so the daemon and one-shot `catchup-record` share the same single-capsule record helper
  4. added optional publish layout for completed recordings plus `.nfo` sidecars and `recorded-publish-manifest.json`
  5. added expiry/retention pruning for completed/failed recorder items
  6. improved ffmpeg HLS parity for legitimate CDN/HLS cases by forwarding effective UA/referer/cookies more faithfully and enabling persistent/multi-request HTTP input by default
  7. added publish-time media-server automation so daemon-completed recordings can now create/reuse and refresh matching Plex, Emby, and Jellyfin lane libraries via the same workflow as `catchup-publish`
  8. added recorder-policy refinement with channel-level allow/deny filters and duplicate suppression by programme identity (`dna_id`/channel + start + title), so duplicate provider variants do not record twice even if they slip into the scheduler input
  9. added recorder observability via `catchup-recorder-report` and `/recordings/recorder.json`, backed by a shared state-file summary loader with lane counts and recent active/completed/failed items
  10. added lane-specific retention and storage-budget controls, plus a fix for stale duplicate indexes after pruning so expired/trimmed recordings do not block future rerecords indefinitely
  11. improved restart recovery semantics so interrupted active items are preserved as explicit partial failures with recovery metadata and can be retried automatically when the same programme window is still eligible
  12. documented the MVP boundary honestly: scheduler/state/concurrency/publish/retention/initial media-server automation, first policy controls, first observability surfaces, basic per-lane quota controls, and first restart-recovery semantics are in; deeper budget intelligence and broader recorder heuristics remain future `REC-*` slices

**Current focus shift (tester fork assessment, 2026-03-19):**
- User asked for a review of the tester fork at `https://github.com/rkdavies/iptvtunerr` to decide which submitted fixes should be integrated upstream.
- Review scope:
  1. fetch the fork tip and compare it against `origin/main`
  2. classify the changes into safe-to-integrate vs useful-but-needs-adjustment vs do-not-merge-yet
  3. record any material risks discovered during review
- Landed result:
  1. integrated the redirected-HLS effective-URL rewrite so nested playlists and relative segments keep resolving correctly after upstream redirects
  2. integrated upstream header / User-Agent overrides, plus optional `Sec-Fetch-*` headers, with proper Go `req.Host` handling instead of header-only pseudo-overrides
  3. integrated persistent upstream cookie storage, but rewrote it so newly learned cookies are actually tracked and saved across restarts
  4. added regression tests, updated env/docs/changelog, and ran full `scripts/verify`

**Current focus shift (audit follow-up, 2026-03-19):**
- Follow-on from the broad repo audit after `v0.1.11`:
  1. fix the missing top-level `help` alias so `iptv-tunerr help` prints usage instead of erroring
  2. restore the executable bit on `scripts/quick-check.sh` so the documented shortcut actually runs
  3. rerun the original failing checks plus full `scripts/verify` as a second pass
- Result:
  1. `iptv-tunerr help` now prints the same usage surface as the no-arg path
  2. `./scripts/quick-check.sh` now executes successfully
  3. second-pass verification is green

**Current focus shift (audit follow-up round 2, 2026-03-19):**
- User asked to keep going after the first audit follow-up landed.
- This pass covers:
  1. make `iptv-tunerr help` return success (`0`) instead of an error exit code
  2. fix `scripts/iptvtunerr-local-test.sh` so explicit caller-supplied `IPTV_TUNERR_BASE_URL` / `IPTV_TUNERR_ADDR` are not overridden by `.env`
  3. rerun the exact repro paths (`help`, `verify`, and local smoke with explicit loopback override)
- Result:
  1. `go run ./cmd/iptv-tunerr help` now exits `0`
  2. `IPTV_TUNERR_BASE_URL=http://127.0.0.1:5015 IPTV_TUNERR_ADDR=:5015 ./scripts/iptvtunerr-local-test.sh all` now succeeds
  3. full `./scripts/verify` still passes

**Current focus shift (continued local audit hardening, 2026-03-19):**
- Continuing beyond the second-pass fixes with more local end-to-end proof and UX cleanup.
- This pass covers:
  1. make the local smoke harness deterministic by default instead of depending on remote provider/XMLTV guide fetches from `.env`
  2. exercise guide-backed commands against a real local `guide.xml`
  3. normalize `epg-link-report` so it writes JSON to stdout by default like the other report commands
- Result:
  1. `scripts/iptvtunerr-local-test.sh all` now disables remote guide fetches unless `IPTV_TUNERR_LOCAL_TEST_FETCH_GUIDE=true` is set
  2. local loopback smoke on `127.0.0.1:5019` succeeded consistently
  3. `guide-health`, `epg-doctor`, `catchup-capsules`, and `epg-link-report` all ran end-to-end against the served local `guide.xml`

**Current focus shift (Cloudflare / credential-rolling finish line, 2026-03-19):**
- User reported newer tester feedback from RK Davies / phantasm: Cloudflare handling was improved but still not complete, and multi-account credential rolling still broke when fallback URLs crossed provider entries.
- This pass covers:
  1. evaluate the public fork state versus our current branch and confirm the remaining gaps are not fully represented in the public fork tip
  2. preserve provider-specific auth alongside fallback stream URLs so merged/deduped channels do not lose credential affinity
  3. make ffmpeg HLS inputs inherit both per-stream auth and cookie-jar cookies so CF-cleared sessions survive the handoff from Go fetches to ffmpeg
  4. add regression tests and rerun repo-wide verification before push
- Result:
  1. confirmed the public `rkdavies/iptvtunerr` fork still only exposes the older `15d7cff` tip, while the remaining work was to finish the behavior already partially integrated upstream
  2. added `LiveChannel.StreamAuths` and threaded per-stream auth selection through catalog enrichment, duplicate-channel merging, host stripping, gateway upstream requests, and ffmpeg header generation
  3. ffmpeg relay inputs now include learned cookie-jar cookies for the actual playlist URL, which closes the Cloudflare clearance gap between Go HTTP and ffmpeg
  4. added regression tests for auth-preserving dedupe/strip, per-provider auth assignment, gateway per-stream auth selection, and ffmpeg cookie forwarding
  5. verification passed with `go test ./...` and `./scripts/verify`

**Current focus shift (real provider validation follow-up, 2026-03-19):**
- User asked to test the fix against the real configured providers from local `.env`.
- This pass covered:
  1. validate both configured provider accounts directly without exposing secrets
  2. verify that `run` / live catalog generation actually preserve multi-provider backups and per-stream auth rules in the real environment
  3. verify that the gateway advances to backup URLs when the primary `.m3u8` response is HTML/empty instead of a usable playlist
  4. fix any provider-tested regressions discovered during that run
- Result:
  1. proved both configured providers return `200` for direct `player_api` auth and `get_live_streams` requests, even though `probe` still classifies them as `bad_status`
  2. fixed `handleProbe` so it now inspects numbered provider entries (`_2`, `_3`, …) instead of only the primary provider URL
  3. fixed the no-ranked direct `player_api` fallback so the real provider catalog now keeps backups/auth rules (`51641` live channels, all with `2` stream URLs and `2` stream auth rules in the tested env)
  4. fixed gateway HLS failover so `.m3u8` responses that are HTML/empty now count as `invalid-hls-playlist` and the gateway tries the next fallback URL
  5. fixed `safeurl.RedactURL` so Xtream path-embedded credentials are redacted from logs
  6. real-provider stream test now fails over correctly from provider-2 HTML `200` to provider-1 `513` and returns a clean `502` instead of stalling on the first bogus playlist
  7. verification passed with `go test ./internal/safeurl ./internal/tuner ./cmd/iptv-tunerr` and `./scripts/verify`

**Current focus shift (probe false-negative fix, 2026-03-19):**
- Follow-on from the real provider validation: the remaining gap was that `probe` still reported `player_api bad_status HTTP 200` for the same Cloudflare-fronted providers that direct requests and `run` already proved valid.
- Root cause:
  1. `ProbePlayerAPI` treated any `Server: cloudflare` response as a challenge-inspection path
  2. on `200 application/json` responses, it consumed the first chunk of the body before JSON decode
  3. the later decoder then saw a truncated stream and returned `bad_status`
- Result:
  1. fixed `ProbePlayerAPI` to read the body once, inspect a preview for CF challenge text, and unmarshal the full JSON body afterward
  2. added regression coverage for `Server: cloudflare` + `200 application/json`
  3. reran real-provider `probe` and both configured providers now report `player_api ok HTTP 200`
  4. full `./scripts/verify` passed after the fix

**Current focus shift (release-confidence smoke, 2026-03-19):**
- User asked for a short curated real-provider smoke before declaring release confidence.
- Result:
  1. fresh loopback `run -skip-health` now succeeds on the ranked-provider path too after teaching `fetchCatalog` to try the next ranked provider when the best-ranked host cannot actually index live streams
  2. the sampled lineup slice (first 5 exposed channels) still failed upstream, but now in a clean and diagnosable way:
     - some URLs returned HTML instead of HLS playlists and were rejected as `invalid-hls-playlist`
     - backup URLs were attempted afterward
     - remaining failures were upstream `513` or request-timeout/context-cancel outcomes, returned to the client as clean `502`
  3. release conclusion: app-side fixes are landed and validated; the remaining risk is provider/channel quality, not IPTV Tunerr logic

**Current focus shift (post-release audit follow-up, 2026-03-19):**
- User asked for another audit specifically looking for bugs, mistakes, logic errors, and gaps after the provider work landed.
- Findings addressed in this pass:
  1. `get.php` fallback still collapsed multi-provider mode to the first successful provider instead of merging feeds and preserving duplicate-channel backups
  2. `probe` log output only redacted the primary provider password and could leak numbered-provider credentials
  3. `probe` ranking output ignored `IPTV_TUNERR_BLOCK_CF_PROVIDERS`, so it could recommend hosts that runtime ingest would reject
- Result:
  1. `get.php` fallback now merges all successful provider feeds, dedupes by `tvg-id`, and preserves multi-provider backup URLs in fallback mode too
  2. `probe` now logs provider URLs through `safeurl.RedactURL`, so numbered-provider usernames/passwords are not exposed
  3. `probe` ranking now uses the same Cloudflare-blocking policy as runtime ingest
  4. added regression coverage for merged `get.php` fallback providers
  5. full `./scripts/verify` passed after the fixes

**Current focus shift (intelligence cross-wiring epic, 2026-03-18):**
- User requested the full next wave from the audit: structural cleanup plus runtime cross-wiring so the newer intelligence/reporting work actually changes behavior.
- This is now tracked as a multi-PR epic in `memory-bank/work_breakdown.md` under `INT-001` through `INT-007`.
- Current PR-sized slice:
  1. `INT-001` shared file/URL loader cleanup
  2. `INT-002` cached guide-quality policy foundation
  3. `INT-003` lineup shaping hooks for healthy-guide filtering
  4. `INT-004` catch-up publishing hooks for healthy-guide filtering
  5. docs/changelog/memory-bank updates plus verification

**Current focus shift (CLI command-registry split, 2026-03-18):**
- Follow-on structural cleanup after the guide-policy slice: stop keeping all CLI flag wiring in one giant `main.go`.
- This pass covers:
  1. move command registration/flag ownership into concern-specific files
  2. make `main.go` a thin usage + dispatch layer
  3. preserve command names/help/behavior while reducing the size and coupling of the top-level entrypoint

**Current focus shift (gateway decomposition, 2026-03-18):**
- Next structural slice after the CLI registry split: reduce `internal/tuner/gateway.go` by moving the cleanest concern seams out first.
- This pass covers:
  1. move provider-profile/autotune reporting into a dedicated file
  2. move Plex client adaptation and Autopilot helper logic into a dedicated file
  3. preserve all runtime behavior and tests while shrinking the monolith

**Current focus shift (gateway decomposition follow-on, 2026-03-18):**
- Continuing the gateway breakup after the first adaptation/provider-profile split.
- This pass covers:
  1. move profile selection, override loading, and ffmpeg codec/bootstrap helpers into `gateway_profiles.go`
  2. move HLS playlist/segment fetch and rewrite helpers into `gateway_hls.go`
  3. keep the remaining `gateway.go` focused on request orchestration / relay control flow
  4. run focused tuner tests, then full verify, then push the refactor

**Current focus shift (gateway relay helper split, 2026-03-18):**
- Continuing the same decomposition track with the next relay-mechanics block.
- This pass covers:
  1. move ffmpeg relay output writers and stdin normalizer types into a dedicated file
  2. move bootstrap TS generation there as well
  3. preserve the orchestration in `relayHLSWithFFmpeg` / `relayHLSAsTS`
  4. verify and push if green

**Current focus shift (gateway stream helper split, 2026-03-18):**
- Continuing `INT-006` with the lower-level stream mechanics block.
- This pass covers:
  1. move TS discontinuity splice helpers into a dedicated file
  2. move startup-signal / adaptive-buffer helpers there too
  3. keep request handling and relay orchestration in `gateway.go`
  4. verify and push if green

**Current focus shift (gateway debug helper split, 2026-03-18):**
- Continuing the same decomposition with the observability/debug block.
- This pass covers:
  1. move debug header logging and tee-file helpers into a dedicated file
  2. move the wrapped debug response writer there as well
  3. leave live request routing and stream decisions in `gateway.go`
  4. verify and push if green

**Current focus shift (catalog fallback + EPG repair hotfix, 2026-03-18):**
- Tester reported that current `iptv-tunerr` still fails on provider `884`/M3U errors because `fetchCatalog` can terminate on the M3U path before trying the older `player_api` route.
- The same validation run also exposed a separate EPG repair failure: provider XMLTV channel parsing logging `context canceled`.
- This pass covers:
  1. restore old behavior so only explicit `IPTV_TUNERR_M3U_URL[_N]` uses direct M3U mode
  2. keep provider-configured runs on the `player_api` first, `get.php` fallback path
  3. fix `refio.Open` so timed URL readers are not canceled immediately on return
  4. add regression tests and verify before pushing

**Current focus shift (gateway upstream helper split, 2026-03-18):**
- Back on the gateway decomposition after landing the ingest/EPG hotfix.
- This pass covers:
  1. move upstream request/header helpers into a dedicated file
  2. move upstream concurrency-preview parsing there as well
  3. keep `gateway.go` focused on request lifecycle and relay logic
  4. verify and push if green

**Current focus shift (CLI catalog helper split, 2026-03-18):**
- With `gateway.go` mostly down to orchestration, the next hotspot is `cmd/iptv-tunerr/main.go`.
- This pass covers:
  1. move catalog ingest helpers out of `main.go`
  2. move runtime EPG-repair helpers and catch-up preview helper alongside them
  3. keep `main.go` as bootstrap + generic media-server helpers
  4. verify and push if green

**Current focus shift (CLI media-server helper split, 2026-03-18):**
- Continuing the same entrypoint cleanup now that catalog helpers are out.
- This pass covers:
  1. move Plex/Emby/Jellyfin catch-up library registration helpers out of `main.go`
  2. keep `main.go` down to bootstrap, usage, and tiny generic helpers
  3. verify and push if green

**Current focus shift (CLI runtime helper split, 2026-03-18):**
- Continuing the CLI decomposition after the catalog and media-server helper splits.
- This pass covers:
  1. move `handleServe` and `handleRun` out of `cmd_core.go` into a dedicated runtime file
  2. leave `cmd_core.go` focused on the remaining core non-runtime commands
  3. preserve all command behavior while shrinking the remaining hotspot
  4. verify and push if green

**Current focus shift (guide-report command split, 2026-03-18):**
- Continuing the CLI decomposition with the `Guide/EPG` command family.
- This pass covers:
  1. move `epg-link-report`, `guide-health`, and `epg-doctor` into a dedicated guide-report file
  2. keep `cmd_reports.go` focused on channel, Ghost Hunter, and capsule reporting
  3. consolidate duplicated catalog/XMLTV loading helpers for the guide-diagnostics path
  4. verify and push if green

**Current focus shift (oracle ops split, 2026-03-18):**
- Continuing the CLI decomposition with the `Lab/ops` command family.
- This pass covers:
  1. move Plex oracle experiment and cleanup commands into a dedicated oracle-ops file
  2. keep `cmd_ops.go` focused on catch-up publishing plus VOD/supervisor helpers
  3. preserve command behavior while reducing the last mixed-purpose CLI file
  4. verify and push if green

**Current focus shift (player_api probe/direct-index regression, 2026-03-18):**
- Tester reported that some Xtream panels still index successfully but `probe` shows `player_api bad_status HTTP 200`, after which `run` can fail with `no player_api OK and no get.php OK on any provider`.
- This pass covers:
  1. relax `player_api` probe success to accept `server_info`-only Xtream auth responses
  2. restore the old direct `IndexFromPlayerAPI` fallback when ranked probes return no OK host
  3. add regression tests for both cases
  4. verify and push before returning to the structural cleanup track

**Current focus shift (gateway relay split, 2026-03-18):**
- Returning to the structural cleanup after the player_api regression hotfix.
- This pass covers:
  1. move the FFmpeg/raw TS/HLS relay implementations out of `internal/tuner/gateway.go`
  2. keep `gateway.go` focused on request entry, channel lookup, and upstream selection/orchestration
  3. preserve runtime behavior while shrinking the last major tuner hotspot
  4. verify and push if green

**Current focus shift (catch-up publish command split, 2026-03-18):**
- Continuing the CLI decomposition after the relay split.
- This pass covers:
  1. move `catchup-publish` into a dedicated command file
  2. keep `cmd_ops.go` focused on supervisor/VOD operational helpers
  3. preserve command behavior while separating Guide/EPG publishing from VOD ops
  4. verify and push if green

**Current focus shift (runtime registration split, 2026-03-18):**
- Continuing the runtime cleanup after the catch-up publish split.
- This pass covers:
  1. move the Plex/Emby/Jellyfin registration and watchdog logic out of `cmd_runtime.go`
  2. keep `cmd_runtime.go` focused on serve/run lifecycle and catalog/runtime setup
  3. preserve runtime behavior while separating media-server integration from core run flow
  4. verify and push if green

**Current focus shift (gateway support helper split, 2026-03-18):**
- Finishing the remaining obvious gateway cleanup after the relay split.
- This pass covers:
  1. move request-id/env/disconnect/path helpers out of `internal/tuner/gateway.go`
  2. keep `gateway.go` focused on `ServeHTTP` and request dispatch/orchestration
  3. preserve runtime behavior while shrinking the last mixed helper block in the gateway entrypoint
  4. verify and push if green

**Current focus shift (runtime server helper split, 2026-03-18):**
- Continuing the cleanup after the gateway helper split.
- This pass covers:
  1. extract shared live-channel load/repair/DNA setup for `serve` and `run`
  2. extract shared `tuner.Server` construction
  3. keep `cmd_runtime.go` focused on the real differences between serve and run flows
  4. verify and push if green

**Current focus shift (VOD command split, 2026-03-18):**
- Finishing the remaining mechanical CLI family cleanup.
- This pass covers:
  1. move `mount`, `plex-vod-register`, and `vod-split` out of `cmd_core.go`
  2. keep `cmd_core.go` focused on core live-TV commands only
  3. preserve command behavior while giving VOD its own command file
  4. verify and push if green

**Current focus shift (report support consolidation, 2026-03-18):**
- Finishing the smaller report-path cleanup after the command-family splits.
- This pass covers:
  1. move shared report catalog/XMLTV loader helpers into a dedicated support file
  2. keep `cmd_reports.go` and `cmd_guide_reports.go` focused on report behavior, not shared input plumbing
  3. preserve report behavior while removing duplicated loading logic
  4. verify and push if green

**Current focus shift (EPG doctor operator docs, 2026-03-18):**
- Follow-on docs cleanup after shipping `guide-health` and `epg-doctor`: make the new guide-diagnostics workflow discoverable from the how-to and runbook indexes so operators have one documented path from symptom to fix.
- This pass adds:
  1. a practical how-to for "channel names but no what's on" and other weak-guide symptoms
  2. links from the how-to index
  3. links from the runbooks index so troubleshooting flows route to the same doctor workflow

**Current focus shift (architecture cleanup + command split, 2026-03-18):**
- User asked for the follow-on work after the architecture review: map the active layers clearly, identify improvement opportunities, then execute the cleanup.
- This pass covers:
  1. rewrite architecture docs around core runtime vs intelligence vs publishing
  2. fix stale repo navigation/remotes guidance
  3. split the oversized `cmd/iptv-tunerr/main.go` command execution paths into command-specific files without changing behavior
  4. verify and record the cleanup

**Current focus shift (docs audience split, 2026-03-18):**
- Follow-on cleanup after the architecture refactor: separate the general deployment/integration story from the Plex-heavy operational patterns so the docs are clearer for Emby/Jellyfin and basic Plex users.
- This pass adds:
  1. a media-server integration explainer
  2. a Plex-only ops-patterns how-to
  3. routing links from the deployment page and docs index

**Current focus shift (guide health / EPG doctor surface, 2026-03-18):**
- Next cleanup/productivity step after the architecture pass: unify guide diagnostics into one operator-facing report instead of leaving them split across `epg-link-report`, `channel-report`, and raw `/guide.xml` inspection.
- This pass adds:
  1. `iptv-tunerr guide-health`
  2. `GET /guide/health.json`
  3. real merged-guide coverage checks: actual programme blocks vs placeholder-only rows vs no guide rows
  4. optional XMLTV match provenance in the same report
  5. `iptv-tunerr epg-doctor`
  6. `GET /guide/doctor.json`
  7. cached live match-provenance reuse so repeated guide diagnostics do not rebuild the same source-XMLTV match analysis on every request

**Current focus shift (README feature-story rewrite, 2026-03-18):**

**Current focus shift (Channel DNA runtime policy, 2026-03-18):**
- Continuing the documented backlog after the intelligence/reporting and Autopilot slices: make `dna_id` affect real runtime decisions instead of only powering reports.
- This pass covers:
  1. add `IPTV_TUNERR_DNA_POLICY=off|prefer_best|prefer_resilient`
  2. apply the policy in runtime lineup shaping so duplicate variants can collapse to one preferred winner
  3. apply the same policy in media-server registration so Plex/Emby/Jellyfin sync a cleaner lineup
  4. update docs/changelog/env examples and verify before pushing

**Current focus shift (Autopilot upstream preference memory, 2026-03-18):**
- Continuing the same backlog after the DNA policy slice: make Autopilot remember which upstream URL/host actually worked, not just the transcode/profile decision.
- This pass covers:
  1. persist preferred upstream URL/host in the Autopilot state file
  2. prefer that known-good stream path first on later requests for the same `dna_id + client_class`
  3. expose the preferred host in Autopilot reports
  4. update docs/changelog/env examples and verify before pushing

**Current focus shift (registration intent parity, 2026-03-18):**
- Continuing the backlog after the Autopilot upstream-memory slice: registration flows should understand the same intent-oriented presets as runtime lineups.
- This pass covers:
  1. let `IPTV_TUNERR_REGISTER_RECIPE` accept `sports_now`, `kids_safe`, and `locals_first`
  2. reuse the lineup recipe logic instead of inventing a second registration-only heuristic set
  3. add regression coverage plus docs/changelog/env updates
  4. verify before pushing

**Current focus shift (provider host penalties, 2026-03-18):**
- Continuing the backlog after registration-intent parity: provider autotune should react to repeated failures on specific upstream hosts, not just generic instability counters.
- This pass covers:
  1. track repeated host-level upstream failures in the gateway/provider profile
  2. automatically prefer healthier hosts/CDNs before retrying penalized ones
  3. expose penalized hosts through the provider profile surface
  4. add regression coverage plus docs/changelog updates and verify before pushing

**Current focus shift (backlog consolidation pass, 2026-03-18):**
- User asked to continue the remaining backlog in one combined pass instead of more tiny follow-ups.
- This pass covers:
  1. add preferred-provider hints for duplicate DNA winners via `IPTV_TUNERR_DNA_PREFERRED_HOSTS`
  2. curate catch-up capsules so duplicate programme rows collapse to the richer candidate before export/publish
  3. improve Ghost Hunter output so visible-stale vs hidden-grab cases recommend different next safe actions
  4. keep docs/changelog/env/memory-bank aligned and verify before pushing

**Current focus shift (final backlog hard-boundary pass, 2026-03-18):**
- User explicitly asked to finish the remaining big-ticket backlog now rather than leave them as future notes.
- This pass covers:
  1. add a real recorder-backed catch-up command for non-replay sources
  2. extend Autopilot memory so failures are tracked and stale remembered decisions stop being reused
  3. add a CLI recovery hook so Ghost Hunter can invoke the guarded hidden-grab helper directly
  4. keep docs/changelog/env/memory-bank aligned and verify before pushing

**Current focus shift (future-feature documentation, 2026-03-18):**
- User asked to document the always-on recorder daemon concept for future work.
- This pass covers:
  1. write a future-feature explainer under `docs/explanations/`
  2. link it from the docs index and the Live TV intelligence epic
  3. record it in `memory-bank/opportunities.md` so it stays visible as backlog, not hallway lore
- README was rewritten so the front page explains why the features matter operationally, not just that they exist.

**Current focus shift (remaining product-facing intelligence surfaces, 2026-03-18):**
- User called out that the remaining work from the earlier audit/product list still needed to land, not just structural cleanup.
- This pass covers:
  1. `epg-doctor` alias-export auto-fixer output (`-write-aliases` plus a live endpoint)
  2. channel leaderboard / hall-of-fame / hall-of-shame surfaces
  3. a registration recipe so Plex/Emby/Jellyfin registration can reuse channel-intelligence scoring
  4. docs/changelog/env/memory-bank updates plus full verify

**Current focus shift (source-backed catch-up replay mode, 2026-03-18):**
- User explicitly asked to finish the documented backlog instead of stopping after the audit cleanup slices.
- This pass covers the next backlog item from `INT-007`:
  1. add explicit replay-mode support for catch-up capsules and publishing
  2. require a real operator-provided replay source template instead of faking replay with the live stream URL
  3. expose replay-vs-launcher mode in capsule previews and publish manifests
  4. update docs/changelog/env/memory-bank and verify before push

**Current focus shift (intent lineup recipes, 2026-03-18):**
- Continuing the documented backlog immediately after replay mode.
- This pass covers the next operator-visible slice from the lineup recipe epic:
  1. extend `IPTV_TUNERR_LINEUP_RECIPE` beyond score-only modes
  2. add built-in `sports_now`, `kids_safe`, and `locals_first` recipes
  3. verify the filters/reordering with tuner tests
  4. update docs/changelog/memory-bank and push if green

**Current focus shift (Autopilot hot-start + report, 2026-03-18):**
- Continuing the backlog after the lineup presets.
- This pass covers the next Autopilot slice:
  1. expose remembered decisions and hottest channels via CLI + HTTP
  2. let favorite/high-hit channels trigger a more aggressive HLS startup profile
  3. keep the hot-start logic evidence-based (explicit favorites or remembered hits), not opaque
  4. update docs/changelog/env/memory-bank and verify before push
- User called out that the README was listing features without explaining why an operator should care.
- This docs pass rewrites the front-page README around:
  1. real IPTV pain points
  2. what IPTV Tunerr changes operationally
  3. why the new intelligence/catch-up features matter in practice
  4. clearer value framing for tuner, EPG, Ghost Hunter, provider profile, and catch-up publishing

**Current focus shift (catch-up library publishing + media-server parity, 2026-03-18):**
- User asked to close the remaining catch-up gap and extend the new intelligence/capsule work to Emby and Jellyfin too, not just Plex.
- Implemented in this session:
  1. Added real catch-up publishing via `iptv-tunerr catchup-publish`.
  2. Publisher now writes media-server-ingestible `.strm + .nfo` items plus `publish-manifest.json`.
  3. Output is lane-based (`sports`, `movies`, `general`) and uses one movie-style library per lane.
  4. Added Emby/Jellyfin library list/create/refresh helpers via `/Library/VirtualFolders` so catch-up publishing can create/reuse matching libraries there too.
  5. Reused the existing Plex library-registration path and VOD-safe preset so Plex gets the same library automation.
  6. Updated README/features/reference/emby-jellyfin docs and changelog to reflect that catch-up publishing is now a real cross-server workflow, not only a preview/export surface.

**Current focus shift (Live TV Intelligence foundation, 2026-03-18):**
- User asked for the “Pop” pass: identify the biggest user-wowing opportunities, map them, and start implementation immediately.
- Product direction captured in `docs/epics/EPIC-live-tv-intelligence.md`.
- Shipping foundation in this session:
  1. `channel-report` CLI for per-channel score/tier/action reporting.
  2. `/channels/report.json` live endpoint for the same intelligence over HTTP.
  3. Report summary/opportunity rollups so weak channels are actionable, not just present.
  4. Optional XMLTV-enriched provenance so tester feedback like “no tvg-id/xmltv matches” is visible as exact match vs alias/name repair vs unmatched.
  5. Intelligence-driven lineup recipes via `IPTV_TUNERR_LINEUP_RECIPE=high_confidence|balanced|guide_first|resilient`.
  6. Channel DNA foundation via persisted `dna_id` on live channels.
  7. Autopilot decision-memory foundation via optional JSON-backed remembered choices keyed by `dna_id + client_class`.
  8. Ghost Hunter visible-session foundation via `ghost-hunter` and `/plex/ghost-report.json`.
  9. Provider behavior profile foundation via `/provider/profile.json`.
  10. README/features/reference/changelog updates so this becomes part of the product story, not just an internal tool.

**Current focus shift (Docker image matrix expansion, 2026-03-18):**
- Binary releases were expanded first, but container images were still limited to `linux/amd64` and `linux/arm64`.
- Implemented in this session:
  1. Added `linux/arm/v7` to `.github/workflows/docker.yml`.
  2. Updated `Dockerfile` to honor `TARGETVARIANT` and pass `GOARM` for armv7 builds.
  3. Updated packaging/platform docs so the published Docker platform set is explicit.
  4. Planned release step: tag the next patch release so the Docker workflow publishes the widened matrix.

**Current focus shift (release asset matrix expansion, 2026-03-18):**
- The repo already packaged `linux/arm/v7` and `windows/arm64` in test bundles, but `.github/workflows/release.yml` still published only `linux/amd64`, `linux/arm64`, `darwin/*`, and `windows/amd64`.
- Implemented in this session:
  1. Extended the tagged release workflow build helper to understand `GOARM` suffixes and publish `linux-armv7` tarballs.
  2. Added `windows/arm64` to the tagged GitHub Release artifact matrix.
  3. Updated platform/package docs so the documented support table and release artifacts match.
  4. Re-ran `./scripts/verify` before pushing.

**Current focus shift (release notes automation, 2026-03-18):**
- GitHub Releases were still using `generate_release_notes: true`, which produced vague/empty notes and required manual cleanup after each tag.
- Implemented in this session:
  1. Added `scripts/generate-release-notes.sh` to generate release notes from the repo itself.
  2. Release notes now prefer the matching `docs/CHANGELOG.md` tag section, then `Unreleased`, then fall back to the exact tagged commit range.
  3. Updated `.github/workflows/release.yml` to fetch full tag history and publish `body_path` from the generated notes instead of GitHub auto-notes.
  4. Updated `.github/workflows/tester-bundles.yml` to stop appending a second set of generic auto-notes when uploading tester assets.
  5. Documented the release-note source in `docs/how-to/package-test-builds.md` and added the recurring-loop note so future agents do not reintroduce GitHub auto-notes.
  6. Validated with `./scripts/verify` and a generated `v0.1.7` notes file before preparing to republish the current release page.

**Current focus shift (M3U multi-credential follow-up, 2026-03-18):**
- Tester confirmed a separate root cause on their side: the index build did not include multiple credentialed M3U URLs.
- Verified in code: direct-M3U mode accepted only one `IPTV_TUNERR_M3U_URL` and catalog build stopped after the first successful M3U fetch.
- Implemented in this session:
  1. Added numbered `IPTV_TUNERR_M3U_URL_2/_3/...` support.
  2. Changed direct-M3U catalog build to merge all successful configured M3U feeds before dedupe/filtering.
  3. Added config and catalog-build tests for the multi-M3U merge path.
  4. Re-ran `scripts/verify`.
  5. Released commit `49ddf3d` as tag `v0.1.7` and pushed `main` + tag to `origin`.
  6. Deleted superseded git tags locally/remotely and deleted old GitHub releases, leaving git tag `v0.1.7` as the only remaining repo tag.
  7. Confirmed registry cleanup is only partially possible from this environment: GHCR deletion is blocked by missing `read:packages`/package-delete scope, and Docker Hub deletion is blocked by missing Docker Hub auth.

**Current focus shift (EPG hardening, 2026-03-18):**
- Review found that runtime guide quality still depended mainly on source-provided `TVGID`s: if a channel had a non-empty but wrong ID, it survived `LIVE_EPG_ONLY` yet still fell through to placeholder programme entries. The deterministic linker existed only as `epg-link-report`, not as a runtime repair path.
- Implemented in this session:
  1. Deterministic EPG repair now runs during catalog build using provider XMLTV channel metadata first, then external XMLTV channel metadata.
  2. Incorrect existing `TVGID`s can now be repaired, not just empty ones.
  3. Added `IPTV_TUNERR_XMLTV_ALIASES` and `IPTV_TUNERR_XMLTV_MATCH_ENABLE` config support plus example alias JSON.
  4. `run` now carries forward the provider entry actually used for indexing so guide `xmltv.php` fetches can stay aligned with the chosen provider source.
  5. Updated architecture/reference/examples to reflect the actual three-layer guide pipeline and runtime repair behavior.
  6. Added end-to-end guide-output assertions proving repaired channels emit real programme blocks with `start`/`stop`, title, and description instead of placeholder channel-name rows.

**Current focus shift (release build, 2026-03-18 late):**
- The provider-capacity follow-up patch is implemented in the working tree. Remaining work is release hygiene: verify, commit, tag, and push the next patch release so CI publishes binaries and container images.
- This checkout still only has `origin` configured for the IPTV Tunerr repo, so the release push will use that configured remote.
- Planned release steps in this session:
  1. Run `scripts/verify`.
  2. Commit the provider concurrency-limit fix set.
  3. Create and push tag `v0.1.5`.

**Current focus shift (tester follow-up, 2026-03-18):**
- New tester report from `phantasm`: a second concurrent tune from another device fails with `gateway: ... upstream[1/1] status=458 ... .m3u8`.
- Working hypothesis: the provider is enforcing a per-account live-stream cap and IptvTunerr is currently surfacing that as a generic upstream failure (`502`) instead of the HDHR-style capacity signal Plex expects (`805` / service unavailable). This is likely distinct from the just-shipped header/IPv4/startup fixes.
- Planned fix in this session:
  1. Inspect gateway handling for upstream non-200 statuses, especially one-URL live streams.
  2. Add a targeted regression test for upstream `458` capacity errors.
  3. Translate provider concurrency-limit responses into a clearer local capacity response and document that operators should align `IPTV_TUNERR_TUNER_COUNT` with the provider's actual concurrent-stream allowance.

**Current focus shift (release build, 2026-03-18):**
- The playback patch is already implemented in the working tree (`internal/tuner/gateway.go`, tests, troubleshooting doc). The remaining work is release hygiene: verify locally, package once, then commit/tag/push so GitHub Actions can publish the build artifacts and container images.
- This checkout does not currently have the `plex` remote described in `repo_map.md`; `origin` points at `https://github.com/snapetech/iptvtunerr.git`. For this session, push the release through the remotes actually configured in this checkout.
- Planned release steps in this session:
  1. Run `scripts/verify`.
  2. Run the packaging script once locally against the planned version tag.
  3. Commit the playback fix set.
  4. Create and push the next patch tag so release workflows publish artifacts.

**Current focus shift (Cloudflare playback triage, 2026-03-18):**
- Tester report from `phantasm`: Cloudflare-backed playback improved when local startup wait was raised from ~15s to 60s, but streams still fail later with likely missing auth context and ffmpeg logs show fallback to unroutable IPv6 (`2606:4700::/32`, `No route to host`).
- Working hypothesis from code inspection: ffmpeg currently receives only Basic auth, and the Go HLS relay forwards only Basic auth + fixed UA; neither path preserves request cookies/referer/origin needed by some CDN-backed playlists/segments. Separately, ffmpeg input URL canonicalization picks the first resolver answer, which can be IPv6 even when the node has no usable IPv6 route.
- Planned fix in this session:
  1. Forward selected upstream auth headers (`Cookie`, `Referer`, `Origin`, plus non-conflicting auth) from the incoming request into upstream playlist/segment fetches and ffmpeg `-headers`.
  2. Prefer IPv4 when rewriting ffmpeg input hosts after DNS resolution.
  3. Raise the default websafe startup gate timeout so slow CDN-backed HLS starts do not fail over before first bytes arrive.
  4. Add unit coverage for header forwarding and IPv4 preference, then run `scripts/verify`.

**OpenBao rollout + credential migration (2026-02-27):**
- Found correct unseal keys in `~/Documents/code/k3s/openbao/openbao-init-output.txt` (the two other init files, `~/Documents/openbao-init-output.txt` and `~/Documents/k3s-secrets/openbao/openbao-init-output.json`, were stale/bad — deleted).
- OpenBao was sealed with stale raft leader entry (dead pod IP 10.42.0.101). Generated new root token (`s.nSyHYZUvm5RZB4jJMv69Hhkk`) via `generate-root` ceremony using the 3 working keys; stored in `secret/data/iptvtunerr.openbao_root_token`.
- Updated `secret/data/iptv` in Bao: replaced `provider1_host=http://cf.supergaminghub.xyz` → `http://pod17546.cdngold.me`.
- Enabled Kubernetes auth in Bao, configured in-cluster k8s host, created `iptvtunerr` policy (read `secret/data/iptv` + `secret/data/iptvtunerr`) and k8s auth role bound to `plex` namespace SA `iptvtunerr`.
- Created `iptvtunerr` ServiceAccount in `plex` namespace.
- Replaced `iptvtunerr-test-env` ConfigMap — stripped all credentials, kept only non-secret config (`PLEX_HOST`, `IPTV_TUNERR_DEVICE_ID`, etc.).
- Deleted `plex-iptv-creds` Secret.
- Patched `iptvtunerr-supervisor` deployment: image → `iptv-tunerr:latest`, SA → `iptvtunerr`, added Bao agent injector annotations to render `/vault/secrets/iptv.env` and `/vault/secrets/plex.env`.
- Added `envFiles` field to supervisor `Config` struct + `loadEnvFile()` function: sources `export KEY=VALUE` files into supervisor process env before starting children. Children inherit all Bao-injected credentials automatically. Also added `startDelay` to `Instance` struct (was in live ConfigMap, caused immediate crash).
- Updated live supervisor ConfigMap: added `envFiles: [/vault/secrets/iptv.env, /vault/secrets/plex.env]`, removed hardcoded `IPTV_TUNERR_M3U_URL`/`IPTV_TUNERR_PROVIDER_*` from instance envs.
- Rebuilt image (`iptv-tunerr:latest`) and pushed to kspld0 via `ctr import`.
- `scripts/unseal-openbao.sh` rewritten: all 5 keys documented, `validate` subcommand seals → tests each key individually via API → unseals → verifies root token. All 5 keys confirmed VALID. Bad files deleted.

**Multi-provider per-credential support (2026-02-27):**
- Added `ProviderEntry` struct to `internal/config/config.go` and a `ProviderEntries()` method that reads `IPTV_TUNERR_PROVIDER_URL_N` / `_USER_N` / `_PASS_N` for N=2,3,… (stops at first gap). Each entry carries its own credentials; if `_USER_N`/`_PASS_N` are absent the primary creds are inherited.
- Added `provider.Entry` / `provider.EntryResult` / `provider.RankedEntries()` to `internal/provider/probe.go` — the multi-credential parallel probe equivalent of `RankedPlayerAPI`. CF blocking and logging behave identically.
- Replaced the single-credential `player_api` path in `fetchCatalog` with a `ProviderEntries()` call that feeds `RankedEntries()`, then uses the winning entry's credentials for `IndexFromPlayerAPI`. `get.php` fallback iterates all entries with their correct credentials.
- `.env` now uses `IPTV_TUNERR_PROVIDER_URL_2` / `_USER_2` / `_PASS_2` directly (removed orphan `M3U_URL_2`).
- 7 new tests added (4 config, 3 provider); all pass; `scripts/verify` green.

**HDHR auto-scaling (2026-02-27): generate-k3s-supervisor-manifests.py now auto-shards HDHR DVRs:**
- Root cause: `build_supervisor_json()` hardcoded exactly one `hdhr-main` HDHR instance. With ~3,513 EPG-linked channels and a 479-channel Plex DVR cap, only the first 479 channels were exposed.
- Fix: Added `--hdhr-total-channels` and `--hdhr-plex-host` CLI args to the generator. `build_supervisor_json()` now computes `n_shards = ceil(hdhr_total_channels / hdhr_lineup_max)` and generates that many HDHR instances (`hdhr-main`, `hdhr-main2`, ..., `hdhr-mainN`). Each extra shard gets unique port, device ID, guide number offset, and `LINEUP_SKIP`/`LINEUP_TAKE` to cover a distinct slice of the channel pool. Services section updated to emit `iptvtunerr-hdhr-test`, `iptvtunerr-hdhr-test2`, ..., `iptvtunerr-hdhr-testN` Services.
- Live config patched: 9 HDHR shards now running (hdhr-main + hdhr-main2..9), covering up to 8×479=3,832 channel slots across 3,513 EPG-linked channels.
- K8s Services created: `iptvtunerr-hdhr-test3` through `iptvtunerr-hdhr-test9`.
- Firewall updated: kspld0 and kspls0 nftables port range expanded from `5006` to `5006-5013`.
- New DVRs will self-register in Plex after each shard's catalog fetch completes (~10 min due to upstream 503s from provider).

**Network fix (2026-02-27): iptvtunerr ports now reachable from Plex pod:**
- Root cause of persistent "No route to host" for `kspls0 -> kspld0:5004/5101-5126` was `kspld0`'s `table inet filter` (`/etc/nftables.conf`, priority 0) dropping packets AFTER `table inet host-firewall` (`/etc/nftables/kspld0-host-firewall.conf`, priority -400) had accepted them. In nftables, multiple base chains at the same hook all run independently; an accept in a lower-priority chain does NOT prevent a higher-priority chain from dropping the packet.
- Fix: added `ip saddr 192.168.50.0/24 tcp dport { 5004, 5006, 5101-5126 } accept` to `/etc/nftables.conf` on kspld0.
- All 15 DVRs now registered in Plex Live TV, EPG/guide.xml confirmed flowing from Plex pod, and `iptvtunerr-supervisor` pod healthy (`1/1 Running`).

**Current focus shift (EPG long-tail, 2026-02-26):**
- Began Phase 1 implementation of the documented EPG-linking pipeline (`docs/reference/epg-linking-pipeline.md`) with a **report-only** in-app CLI:
  - `iptv-tunerr epg-link-report`
- The command reads `catalog.json` live channels + XMLTV, applies deterministic matching tiers (`tvg-id` exact, alias exact, normalized-name exact unique), and emits coverage/unmatched reports for operator review.
- This is intentionally non-invasive: it does **not** mutate runtime guide linkage yet.
- Next phase would add a persistent alias/override store and optional application of high-confidence matches during indexing.
- Added an in-app Plex wizard-oracle command (`plex-epg-oracle`) to automate HDHR registration + DVR create + guide reload + channelmap retrieval across multiple tuner base URLs (or a `{cap}` URL template with `-caps`) for EPG-linking experiments. This is report/probe tooling and can create DVR rows in Plex, so use on a test Plex instance.

**Live category capacity follow-up (2026-02-26):**
- Added runtime lineup sharding envs in tuner pre-cap path:
  - `IPTV_TUNERR_LINEUP_SKIP`
  - `IPTV_TUNERR_LINEUP_TAKE`
- Sharding is applied after pre-cap EPG/music/shaping filters and before final lineup cap, so overflow DVR buckets are based on the **confirmed filtered/linkable lineup**, not raw source order.
- Updated `scripts/generate-k3s-supervisor-manifests.py` to support optional auto-overflow child creation from confirmed per-category linked counts:
  - `--category-counts-json`
  - `--category-cap` (default `479`)
- Generator now emits `category2`, `category3`, ... children (as needed) that reuse the same base category M3U/XMLTV but set `IPTV_TUNERR_LINEUP_SKIP/TAKE`.

**Current status (VOD work, 2026-02-26):**
- There was no in-app equivalent of Live TV DVR injection for standard Plex Movies/TV libraries; VOD support existed only as `iptv-tunerr mount` (Linux FUSE/VODFS) + manual Plex library creation.
- Added new CLI command `plex-vod-register` that creates/reuses Plex library sections for a VODFS mount:
  - `VOD` -> `<mount>/TV` (show library)
  - `VOD-Movies` -> `<mount>/Movies` (movie library)
  - idempotent by library `name + path`, with optional refresh (default on)
- Live-validated the command against the running test PMS API inside the Plex pod using temporary section names (`PTVODTEST`, `PTVODTEST-Movies`) with successful create + reuse + refresh behavior.
- Remaining blocker for "IPTV VOD libraries running in k8s Plex" is mount placement, not Plex API registration:
  - the Plex pod has no `/dev/fuse`, so VODFS cannot be mounted inside it as-is
  - a VODFS mount in a separate helper pod will not automatically be visible to the Plex pod (separate mount namespaces / no mount propagation)
  - the real VODFS mount must exist on a filesystem path visible to the Plex server process (host-level/systemd on the Plex node or an equivalent privileged mount-propagation setup)
- Live k3s host-mount path is now in place and Plex libraries `VOD` / `VOD-Movies` exist, but imports remain blocked after scan:
  - Plex file logs show both scanners traversing `/media/iptv-vodfs/TV` and `/media/iptv-vodfs/Movies`
  - section counts still report `size=0`
- VODFS traversal blockers fixed in code during live bring-up:
  - invalid `/` in titles causing bad FUSE names / `readdir` failures
  - duplicate top-level movie/show names causing entry collisions
- Additional import blocker fixed in code (likely Plex-specific):
  - file `Lookup()` attrs for movie/episode entries were still returning `EntryOut.Size=0` even after `Getattr()` was patched to expose a non-zero placeholder size
  - movie/episode lookup paths now return the same placeholder size as `Getattr()`
- Additional VODFS correctness fixes proven on host mount (2026-02-26):
  - `VirtualFileNode` now implements `NodeOpener` (file opens no longer fail with `Errno 95 / EOPNOTSUPP`)
  - VOD probe/materializer now accepts direct non-MP4 files such as `.mkv` (`StreamDirectFile`)
  - direct sample VOD file on host now reaches materializer and starts downloading into cache (`.partial`)
- Newly proven root cause for Plex VOD import/scanner pain:
  - `VirtualFileNode.Read()` blocks until `Materialize()` completes a full download/remux and renames the final cache file
  - for large VOD assets, Plex's first read/probe can stall for a long time waiting for the entire file, which likely causes scan/import failures or "failed quickly" UI behavior
  - evidence: sample `.mkv` asset `1750487` reached `materializer: download direct ...` and wrote a large `.partial` file (~551 MB) while the first `read()` remained blocked with no bytes returned yet
- Progressive-read VODFS fix is now live/proven on host (2026-02-26):
  - VODFS now returns early bytes from `.partial` cache files during the first read instead of waiting for full materialization
  - sample asset `1750487` returned a real Matroska header (`READ 256 ... matroska`) via `vodfs: progressive read ... using=.partial`
- New blocker after progressive-read fix:
  - background/direct materialization for the sample asset later failed with `context deadline exceeded (Client.Timeout or context cancellation while reading body)`
  - the current shared HTTP client timeout appears too short for large VOD downloads during scanner-triggered materialization, which can still prevent successful full cache completion/import
- Operational note: huge top-level `Movies` / `TV` shell listings can hang for a long time on the current catalog size (~157k movies / ~41k series); use Plex scanner logs or nested known paths instead of repeated top-level `ls/find` probes.
- VOD subset proof path established to avoid waiting on huge full-library scans:
  - created temporary Plex libraries `VOD-SUBSET` (TV, section `9`) and `VOD-SUBSET-Movies` (Movies, section `10`) backed by a separate host-mounted subset VODFS (`/media/iptv-vodfs-subset`)
  - subset movie import is now proven working (non-zero item counts and active metadata updates in Plex)
- Root cause for subset TV remaining empty was **not Plex/VODFS at that point**:
  - the subset `catalog.json` had `series` rows with empty `seasons` (show folders existed but were empty)
  - confirmed by inspecting both the subset catalog JSON and mounted TV show directories
- Found likely upstream parser bug causing empty TV seasons in Xtream-derived catalogs:
  - `internal/indexer/player_api.go` `get_series_info` parsing handled flat episode arrays and map-of-episode objects, but missed the common Xtream shape `episodes: { "<season>": [episode, ...] }`
  - patched parser and added regression tests (`internal/indexer/player_api_test.go`)
- Rebuilt the subset TV series data directly from provider `get_series_info` calls on the Plex node and remounted subset VODFS:
  - subset catalog now contains `50` series with seasons and `528` total episodes
  - mounted TV tree now shows real season folders and episode files (e.g. `4K-NF - 13 Reasons Why (US) (2017)/Season 01/...`)
- Current wait state:
  - `VOD-SUBSET-Movies` scan is still occupying the Plex scanner in observed polls (movie subset count increasing)
  - need a fresh/complete `VOD-SUBSET` TV scan pass after movie scan clears to confirm TV import rises above `0`

**User product-direction note (capture before loss, 2026-02-26):**
- User is considering a broader "near-live catch-up libraries" model (program-bounded assets + targeted scans + collections/shelves) as a distribution strategy for remote/non-Plex-Home sharing and better UX than raw Live TV/EPG.
- Important architectural implication for Plex ingest/perf: **prefer multiple smaller category libraries over one giant hot library** when churn is high (for example `bcastUS`, `sports`, `news`, `movies`, regional/world buckets), because Plex scan/update work is section-scoped and targeted path scans are easier/cheaper when sections are narrower.
- Keep this in scope as a design/documentation follow-on after current VODFS import validation is complete.

**Breakthrough (2026-02-25 late):**
- Reused the existing `k3s/plex/scripts/plex-websafe-pcap-repro.sh` harness on pure `DVR 218` (`FOX WEATHER`, helper AB4 `:5009`) and finally captured the missing signal: PMS first-stage `Lavf` `/video/:/transcode/session/.../manifest` callbacks were hitting `127.0.0.1:32400` and receiving repeated HTTP `403` responses (visible in localhost pcap), while Plex logs only showed `buildLiveM3U8: no segment info available`.
- Root cause is Plex-side callback auth, not IptvTunerr TS formatting: first-stage `ssegment` was posting valid CSV segment rows, but PMS rejected the callback updates, so `/livetv/sessions/.../index.m3u8` had no segment info.
- Applied a Plex runtime workaround by adding `allowedNetworks="127.0.0.1/8,::1/128,<lan-cidr>"` to PMS `Preferences.xml` and restarting `deploy/plex`.
- Post-fix validation:
  - pcap harness rerun: first-stage callback responses flipped from `403` to `200`; PMS internal `/livetv/sessions/.../index.m3u8` returned `200` with real HLS entries; logs changed from `buildLiveM3U8: no segment info available` to healthy `buildLiveM3U8: min ... max ...`.
  - Plex Web probe path (`DVR 218`, `FOX WEATHER`) now reaches immediate `decision` + `start.mpd` success and returns DASH init headers and first segments (`/0/header`, `/0/0.m4s`, `/1/header`, `/1/0.m4s` all with bytes).
- Full probe succeeded after patching the external probe script decode bug (binary DASH segment fetches caused `UnicodeDecodeError` in the harness, not playback failure).

**Follow-on fixes (2026-02-25 night):**
- User reported Plex Web/Chrome video-without-audio while TV clients worked, plus lingering Live TV sessions when LG/webOS input is switched without stopping playback.
- Verified the lingering HLS pulls are Plex client/session lifecycle behavior (PMS keeps pulling while the LG app remains "playing" in the background), not IptvTunerr streaming independently after a client disconnect.
- Found the immediate Chrome-audio blocker on injected category DVRs was runtime drift: the 13 category `iptvtunerr-*` deployments were running shell-less `iptv-tunerr:hdhr-test` images without `ffmpeg`, and with `IPTV_TUNERR_STREAM_TRANSCODE=off`, so IptvTunerr relayed raw HLS (HE-AAC source audio) to Plex.
- Durable repo fixes landed:
  - `Dockerfile` and `Dockerfile.static` now install `ffmpeg`
  - `internal/tuner/gateway.go` logs explicit warnings when transcode was requested but `ffmpeg` is unavailable
  - added `scripts/plex-live-session-drain.py` for manual Plex Live TV session cleanup (no max-live TTL behavior)
- Found and fixed a real app regression during rollout: `cmd/iptv-tunerr` `run -mode=easy` (`fetchCatalog`) ignored configured `IPTV_TUNERR_M3U_URL` / built M3U URLs unless `-m3u` was passed explicitly; patched it to honor `cfg.M3UURLsOrBuild()` first.
- Runtime rollout completed on `<plex-node>` (all 13 category pods):
  - built/imported ffmpeg-enabled `iptv-tunerr:hdhr-test` into k3s containerd on-node
  - restarted all 13 category deployments successfully and verified `ffmpeg` exists in category pods
  - set `IPTV_TUNERR_STREAM_TRANSCODE=on` across the 13 category deployments for immediate web audio normalization (client-adapt optimization can follow later)

**Takeover note (2026-02-25):** Taking over live Plex/IptvTunerr DVR-delivery triage after another agent stalled in repeat probe loops. Immediate priority is to re-validate the current runtime state (Plex reachability, active IptvTunerr WebSafe/Trial services, DVR mappings) and reproduce with fresh channels/sessions only, following the hidden `CaptureBuffer` reuse loop guardrails.

**Takeover progress (2026-02-25):**
- Root cause for the immediate "DVRs not delivering" state was operational drift, not the previously investigated Plex packager issue: the `iptvtunerr-trial` / `iptvtunerr-websafe` services still existed but had **no endpoints** because the `app=iptvtunerr-build` pod was gone, and Plex DVR devices `135` / `138` had also drifted to the wrong URI (`http://iptvtunerr-otherworld.plex.svc:5004`).
- Temporary runtime recovery applied (no Plex restart): recreated a lightweight `iptvtunerr-build` deployment (helper pod) in `plex`, copied a fresh static `iptv-tunerr` binary into `/workspace`, regenerated shared live catalogs from provider API creds (`IPTV_TUNERR_PROVIDER_*`, `LiveOnly`, `LiveEPGOnly`), and started Trial (`:5004`) + WebSafe (`:5005`) processes with `IPTV_TUNERR_LINEUP_MAX_CHANNELS=-1`.
- Plex device URIs were repaired in-place via `/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=...` for `iptvtunerr-trial.plex.svc:5004` and `iptvtunerr-websafe.plex.svc:5005`; Plex then immediately resumed `GET /discover.json` and `GET /lineup_status.json` to both tuners (confirmed in tuner logs).
- Current follow-on blocker for "fully healthy" direct DVRs in this temporary runtime is guide refresh latency: Plex `reloadGuide` hits both tuners, but external XMLTV fetches timed out at ~45s and IptvTunerr fell back to placeholder `guide.xml`, which also made `plex-activate-dvr-lineups.py` / random stream probes stall on guide/channel metadata calls.
- Revalidated the current helper runtime from code + live logs and corrected stale assumptions: direct Trial/WebSafe now run from local `iptv-m3u-server` feeds (`live.m3u` + `xmltv.xml`) with fast real guide responses (~1.4–2.5s, ~70 MB XML), and Plex `reloadGuide` does trigger tuner `/guide.xml` fetches again.
- Found a new operational regression in the ad hoc helper pod: WebSafe was running without `ffmpeg`, so `STREAM_TRANSCODE=true` silently degraded to the Go raw HLS relay (`hls-relay` logs only). Installed `ffmpeg` in the helper pod (`apt-get install -y ffmpeg`) and restarted only the WebSafe `serve` process with `IPTV_TUNERR_FFMPEG_PATH=/usr/bin/ffmpeg`.
- Fresh browser-path probe after restoring ffmpeg (`DVR 138`, channel `108`) still fails `startmpd1_0`, but now with confirmed WebSafe ffmpeg output (`ffmpeg-transcode`, startup gate `idr=true`, `aac=true`, first bytes in ~4.1s), which strengthens the Plex-internal packaging diagnosis.
- User-directed pivot completed: restored and validated the **13-category injected DVR path using IptvTunerr only** (no Threadfin in device or lineup URLs). Recreated DVRs `218,220,222,224,226,228,230,232,234,236,238,240,242` with devices `http://iptvtunerr-<bucket>.plex.svc:5004` and lineups `lineup://.../http://iptvtunerr-<bucket>.plex.svc:5004/guide.xml#iptvtunerr-<bucket>`.
- Root cause of earlier empty 13-bucket category tuners was not IptvTunerr indexing: `iptv-m3u-server` postvalidation had zeroed many generated `dvr-*.m3u` files after probe failures. Rerunning only the splitter (skipping postvalidate) restored non-empty category M3Us; all 13 `iptvtunerr-*` deployments then loaded live channels and exposed service endpoints.
- Pure-app channel activation completed successfully for all 13 injected DVRs (`plex-activate-dvr-lineups.py ... --dvr 218 ... 242`): final status `OK` with mapped counts `44,136,308,307,257,206,212,111,465,52,479,273,404` (total `3254` mapped channels).
- Pure-app playback proof (category DVR): `plex-web-livetv-probe.py --dvr 218` tuned `US: NEWS 12 BROOKLYN` (`POST /livetv/dvrs/218/channels/39/tune -> 200`), IptvTunerr `iptvtunerr-newsus` logged `/stream/News12Brooklyn.us` startup + HLS playlist relay, but Plex probe still failed `startmpd1_0` after ~35s.
- Smart TV spin proof from Plex logs (client `<client-ip-a>`): Plex starts first-stage grabber, reads from IptvTunerr stream URLs, receives `progress/streamDetail`, then its own internal `GET /livetv/sessions/.../index.m3u8` returns `500` with `buildLiveM3U8: no segment info available`, while client `start.mpd` requests complete ~100–125s later or after stop.
- Repo hygiene pass completed for this concern: removed non-essential "Threadfin-style" wording from Plex API registration code/logs and stale k8s helper comments; remaining `threadfin` references in this repo are comparison docs, historical memory-bank notes, or explicit legacy secret-name context.
- Plex cleanup completed: deleted all stale Threadfin-era DVRs (`141,144,147,150,153,156,159,162,165,168,171,174,177`). Current DVR inventory is now only the 2 direct test DVRs (`135`, `138`) plus the 13 pure `iptvtunerr-*` injected DVRs (`218..242`) with no `threadfin-*` entries left.
- Category A/B test completed on `DVR 218` (`iptvtunerr-newsus`): temporarily switched the `iptvtunerr-newsus` deployment to WebSafe-style settings (`STREAM_TRANSCODE=true`, `PROFILE=plexsafe`, `CLIENT_ADAPT=false`, `FFMPEG_PATH=/usr/bin/ffmpeg`), reran Plex Web probe, then rolled back the deployment to original `STREAM_TRANSCODE=off`.
- A/B result: no playback improvement. The `DVR 218` probe still failed `startmpd1_0` (~37s), and `iptvtunerr-newsus` logs still showed HLS relay (`hls-playlist ... relaying as ts`) rather than `ffmpeg-transcode`, so the category `iptv-tunerr:hdhr-test` runtime did not exercise a true ffmpeg WebSafe path in this test.
- PMS evidence for the A/B session (`live=798fc0ae-...`, client session `19baaba...`) matches the existing pattern: Plex started the grabber against `http://iptvtunerr-newsus.../stream/FoxBusiness.us`, received `progress/streamDetail`, the client timed out/stopped, and PMS only completed `decision`/`start.mpd` ~95s later. Extra `connection refused` errors appeared afterward because the A/B pod was intentionally restarted for rollback while PMS still had the background grabber open.
- Helper-pod ffmpeg A/Bs on `DVR 218` now prove the category path can run a real WebSafe ffmpeg stream when Plex is repointed to helper services (`:5006+`), and this surfaced two distinct problems instead of one:
  - `:5006` (`plexsafe`, bootstrap enabled, old binary): Plex first-stage recorder failed almost immediately with repeated `AAC bitstream not in ADTS format and extradata missing`, then `Recording failed. Please check your tuner or antenna.` while IptvTunerr showed `bootstrap-ts` followed by `ffmpeg-transcode` bytes.
  - `:5007` (`plexsafe`, bootstrap disabled) and `:5008` (`aaccfr`, bootstrap disabled): Plex recorder stayed healthy for the full probe window (continuous `progress/streamDetail`, no recorder crash), but Plex Web still failed `startmpd1_0`.
- Root-cause isolation from those helper A/Bs: the WebSafe `bootstrap-ts` path was emitting a fixed H264/AAC bootstrap even when the active profile output audio was MP3/MP2 (`plexsafe`/`pmsxcode`), creating a mid-stream audio codec switch that can break Plex's recorder.
- Code fix implemented in `internal/tuner/gateway.go`: WebSafe `bootstrap-ts` audio codec now matches the active output profile (`plexsafe`=MP3, `pmsxcode`=MP2, `videoonly`=no audio, otherwise AAC) and bootstrap logs now include `profile=...`.
- Live validation of the code fix using a patched helper binary (`:5009`, `plexsafe`, bootstrap enabled) succeeded for the recorder-crash case:
  - IptvTunerr logs show `bootstrap-ts ... profile=plexsafe`
  - PMS no longer logs the previous AAC/ADTS recorder failure
  - PMS first-stage `progress/streamDetail` reports `codec=mp3` and keeps recording alive
  - Plex Web probe still fails `startmpd1_0` (remaining PMS packager/startup issue unchanged)
- New focused `DVR 218` / helper `:5009` (`dashfast`, `realtime`, patched binary) long-wait probes on **2026-02-25** confirm the failure is deeper than the browser's 35s timeout:
  - With extended probe timeouts (`HTTP_MAX_TIME=130`, `DASH_READY_WAIT_S=140`), Plex delays the first `start.mpd` response ~`100–125s`.
  - A normal concurrent probe (`decision` + `start.mpd`) can still induce a second-stage transcode self-kill race, but a **serialized/no-decision** probe reproduces the same end result, so the race is not the root cause.
  - After the delayed `start.mpd`, Plex returns an MPD shell and exposes a DASH session ID, but repeated `GET /video/:/transcode/universal/session/<session>/0/header` stays `404` for ~2 minutes (`dash_init_404`).
  - PMS logs for the serialized run show the second-stage DASH transcode starts (`Req#7b280`) and then fails with `TranscodeSession: timed out waiting to find duration for live session` -> `Failed to start session.` -> `Recording failed. Please check your tuner or antenna.`
  - Concurrent TS inspector capture on the same Fox Weather run (`IPTV_TUNERR_TS_INSPECT_MAX_PACKETS=120000`) shows ~63s of clean IptvTunerr ffmpeg TS output (`sync_losses=0`, monotonic PCR/PTS, no media-PID CC errors, no discontinuities), strengthening the case that IptvTunerr output is not the immediate trigger.

---

## Assumptions & questions (only if uncertainty matters)
Assumptions (safe defaults you are proceeding with):
- Next release version for this direct-M3U follow-up patch is `v0.1.7` (latest existing tag is `v0.1.6`).
- Next release version for this follow-up patch build is `v0.1.5` (latest existing tag is `v0.1.4`).
- Next release version for this patch-only build is `v0.1.4` (latest existing tag is `v0.1.3`).
- Local environment may not have Go installed; OK to use a temporary local Go toolchain (non-system install) only for verification.
- k3s/Plex troubleshooting changes on remote hosts may be temporary runtime fixes unless later codified in infra manifests or host firewall config.
- Existing WebSafe/Trial pod processes and DVR IDs noted below may have drifted since 2026-02-24; all IDs/URIs must be rechecked before interpreting probe results.
- Incoming stream requests may already carry CDN session state via normal HTTP headers from the caller/proxy layer; forwarding a narrow allowlist upstream is lower risk than inventing provider-specific auth handling.

Questions (ONLY if blocked or high-risk ambiguity):
- Q: None currently blocking for this patch-sized change.
- Q: None currently blocking. User confirmed initial tier-1 client matrix for `HR-003`: LG webOS, Plex Web (Firefox/Chrome), iPhone iOS, and NVIDIA Shield TV (Android TV/Google target coverage).

## Opportunity radar (don't derail)
- If you notice out-of-scope improvements, record them in `memory-bank/opportunities.md` and raise to the user in your summary.

## Parallel agent tracking
- **Agent 2 (this session):** HDHR k8s standup: Ingress, run-mode deployment, BaseURL=http://iptvtunerr-hdhr.plex.home, k8s/README.md.

## Self-check (quality bar — fill before claiming done)
- **Correctness:** ✅ Pure IptvTunerr injected DVR path remains active (`218..242`), and Plex Web playback on `DVR 218` (`FOX WEATHER`) is now working after the PMS `allowedNetworks` callback-auth workaround. Root cause for the prior `buildLiveM3U8`/`start.mpd` failures was PMS rejecting its own first-stage `/manifest` callbacks (`403`), not a IptvTunerr stream/HLS selection issue.
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

- **agent1:** Live Plex Web packaging/`start.mpd` triage on direct IptvTunerr (WebSafe/Trial) via k3s/PMS logs; avoid Plex restarts and preserve current runtime state.
- **agent2:** Non-HDHR validation lane for main IptvTunerr functionality: local automated tests + live-race harness (synthetic/replay), VOD/FUSE virtual-file smoke check, and non-disruptive direct Plex API probe loop against `https://plex.home` using existing preconfigured DVRs only (no re-registration/restart).

**Live session cleanup follow-on (2026-02-26):** Added a multi-layer Plex-side stale-session reaper path to `scripts/plex-live-session-drain.py` to address lingering Live TV streams after browser tab close / LG input switch. The script now supports (1) polling-based stale detection using `/status/sessions` + PMS request activity, (2) optional Plex SSE notifications as fast rescan triggers, and (3) optional lease TTL backstop. Live dry-run validation against an active Chrome session confirmed no false idle kill after wiring SSE activity into the idle timer.

**Feed criteria / override tooling (2026-02-26):** Added `scripts/plex-generate-stream-overrides.py` to probe a tuner `lineup.json` and generate criteria-based channel overrides for `IPTV_TUNERR_PROFILE_OVERRIDES_FILE` / `IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`. It reuses the existing override path and supports `--replace-url-prefix` for port-forwarded category tuners whose lineup URLs contain cluster-internal hostnames. Validation on `ctvwinnipeg.ca` (the Chrome rebuffer case) correctly produced no flag, reinforcing that this case is a PMS transcode-throughput issue rather than an obvious feed-format problem.

**Built-in Plex session reaper (2026-02-26):** Ported the stale-session watchdog into the Go app as an optional background worker started by `tuner.Server.Run` (no Python dependency required for packaged builds). It uses Plex `/status/sessions` polling and optional Plex SSE notifications for fast wake-ups, with configurable idle timeout, renewable lease timeout, and hard lease backstop. Enable with `IPTV_TUNERR_PLEX_SESSION_REAPER=1` plus existing `IPTV_TUNERR_PMS_URL` / `IPTV_TUNERR_PMS_TOKEN`.

**XMLTV language normalization (2026-02-26):** Added in-app guide text normalization for remapped external XMLTV feeds. New env-controlled policy can prefer `lang=` variants (e.g. `en,eng`), prefer Latin-script variants among repeated programme nodes, and optionally replace mostly non-Latin programme titles with the channel name (`IPTV_TUNERR_XMLTV_NON_LATIN_TITLE_FALLBACK=channel`). This addresses the user-reported Plex guide text showing Cyrillic/Arabic-like titles when upstream XMLTV is multilingual or non-English.

**Single-app supervisor mode (2026-02-26):** Added `iptv-tunerr supervise -config <json>` to run multiple child `iptv-tunerr` instances in one container/process supervisor for packaged "one pod runs many DVR buckets" deployments. First-pass design uses child processes (not in-process goroutine multiplexing) for lower risk and code reuse. Important constraint: HDHR network mode (UDP/TCP 65001) should be enabled on only one child unless custom HDHR ports are assigned.

**Single-pod supervisor example assembled (2026-02-26):** Added a concrete `k8s/iptvtunerr-supervisor-multi.example.json` with 14 children (`13` category DVR insertion instances + `1` big-feed HDHR wizard instance) and `k8s/iptvtunerr-supervisor-singlepod.example.yaml` showing a host-networked single-pod deployment with a multi-port Service for category HTTP ports. The HDHR child alone enables `IPTV_TUNERR_HDHR_NETWORK_MODE=true`; category children use HTTP-only ports `5101..5113` on `iptvtunerr-supervisor.plex.svc`.

**Single-pod supervisor live cutover completed (2026-02-26 late):**
- Regenerated real supervisor artifacts with timezone-guided HDHR preset selection (`na_en`) and updated the HDHR child to use the broad feed (`live.m3u`) with in-app filtering/cap:
  - `IPTV_TUNERR_LINEUP_DROP_MUSIC=true`
  - `IPTV_TUNERR_LINEUP_MAX_CHANNELS=479`
  - XMLTV English-first normalization envs enabled
- Reapplied only the generated supervisor `ConfigMap` + `Deployment` in `k3s/plex`, then patched the deployment image back to the custom locally imported tag (`iptv-tunerr:supervisor-cutover-20260225223451`) on `<plex-node>` to retain the new `supervise` binary.
- Verified the supervisor pod is healthy (`1/1`) and all 14 child instances start, with category children serving bare category identities (`FriendlyName`/`DeviceID` = `newsus`, `generalent`, etc.) and the HDHR child advertising `BaseURL=http://iptvtunerr-hdhr.plex.home`.
- Verified HDHR child behavior inside the supervisor pod:
  - `Loaded 6207 live channels`
  - `Lineup pre-cap filter: dropped 72 music/radio channels`
  - `/lineup.json` count = `479`
- Applied only the generated Service documents and confirmed category/HDHR Services now route to the supervisor pod endpoints (`<plex-host-ip>:510x` / `:5004`), then scaled the old 13 category deployments to `0/0`.
- Sample post-cutover validation from inside the Plex pod:
  - `http://iptvtunerr-newsus.plex.svc:5004/discover.json` reports `FriendlyName=newsus`
  - `http://iptvtunerr-hdhr-test.plex.svc:5004/lineup.json` returns `479` entries

**HDHR wizard noise reduction follow-up (2026-02-26 late):**
- Plex's "hardware we recognize" list is driven by `/media/grabbers/devices` (and cached DB rows in `media_provider_resources`), so active injected category DVR devices still appear there as known HDHR devices (e.g. `otherworld`) even though they are not the intended wizard lane.
- Added in-app `IPTV_TUNERR_HDHR_SCAN_POSSIBLE` support (`/lineup_status.json`) and regenerated the supervisor config so:
  - category children return `{"ScanPossible":0}`
  - the dedicated HDHR child returns `{"ScanPossible":1}`
- Live-verified on the running supervisor pod and via the Plex pod:
  - `iptvtunerr-otherworld` -> `ScanPossible=0`
  - `iptvtunerr-hdhr-test` -> `ScanPossible=1`
- Cleaned the stale helper cache row (`newsus-websafeab5:5010`) from Plex's `media_provider_resources`; it no longer appears in `/media/grabbers/devices`.
- Important operational gotcha rediscovered: image imports must happen on the actual scheduled node (`<plex-node>`) runtime, not the local `k3s` runtime on `<work-node>`, or kubelet will keep reporting `ErrImageNeverPull` even when local `crictl` on the wrong host shows the image.

**Plex TV UI / provider metadata follow-up (2026-02-26 late):**
- User-reported TV symptom ("all tabs labelled `plexKube`" and identical-looking guides) is **not** caused by flattened tuner feeds. Verified live tuner outputs remain distinct after the supervisor cutover:
  - `newsus=44`, `bcastus=136`, `otherworld=404`, `hdhr1=479`, `hdhr2=479` (`/lineup.json` counts).
- Verified Plex backend provider endpoints are also distinct per DVR:
  - `/tv.plex.providers.epg.xmltv:<id>/lineups/dvr/channels` returns different sizes (for example `218=44`, `220=136`, `242=404`, `247=308`, `250=308`).
- Found and repaired Plex DB metadata drift in `media_provider_resources`:
  - direct provider child rows `136` (`DVR 135`) and `139` (`DVR 138`) had `uri=http://iptvtunerr-otherworld.../guide.xml`
  - most injected/HDHR provider child rows (`type=3`) had blank `uri`
  - `DVR 218` device row `179` still pointed to helper A/B URI `http://iptvtunerr-newsus-websafeab4.plex.svc:5009`
- Applied a DB patch (with file backup first) setting `type=3` provider child `uri` values to each DVR's actual `.../guide.xml` and repaired row `179` to `http://iptvtunerr-newsus.plex.svc:5004`; `/livetv/dvrs/218` now reflects the correct device URI again.
- Remaining evidence points to Plex client/UI presentation behavior:
  - `/media/providers` still emits every Live TV `MediaProvider` with `friendlyName="plexKube"` and `title="Live TV & DVR"` (Plex-generated), which likely explains the repeated tab labels on TV clients.
  - Need live LG/webOS request capture to confirm whether the TV app is actually requesting distinct `tv.plex.providers.epg.xmltv:<id>` grids when switching tabs.

**LG TV guide-path capture + cleanup (2026-02-26 late):**
- File-level Plex logs (`Plex Media Server.log`, not `kubectl logs`) finally captured the LG client (`<client-ip-b>`) guide requests.
- Root cause for the wrong TV guide behavior in the captured session: the LG was requesting **only provider `tv.plex.providers.epg.xmltv:135`** (`DVR 135` / legacy direct `iptvtunerrTrial`) for:
  - `/lineups/dvr/channels`
  - `/grid?...`
  - `/hubs/discover?...`
  while also sending playback/timeline traffic (`context=source:content.dvr.guide`).
- This explains why TV-side guide behavior could look wrong/duplicated even though injected category providers were distinct: the TV was on the old direct test provider, not a category provider.
- Cleanup applied:
  - deleted legacy direct test DVRs `135` and `138` via Plex API (`DELETE /livetv/dvrs/<id>`)
  - deleted orphan HDHR device rows `134` (`iptvtunerr01`) and `137` (`iptvtunerrweb01`) from `media_provider_resources` after API deletion left them in `/media/grabbers/devices`
- Post-cleanup validation:
  - `/livetv/dvrs` now contains only injected category DVRs (`218..242`) + HDHR wizard DVRs (`247`, `250`)
  - `/media/grabbers/devices` no longer lists `iptvtunerr01` / `iptvtunerrweb01`

**Guide-collision fix for injected DVR tabs (2026-02-26 late):**
- User confirmed Plex now shows the correct DVR count (`15`), but multiple tabs/sources in Plex Web appeared to show the same guide content while channel names differed.
- Root cause was **channel/guide ID collisions across DVRs**, not flattened feeds:
  - category tuners all exposed `GuideNumber` sequences starting at `1,2,3...`
  - Plex provider/UI layers could cache/reuse guide/grid content when multiple DVRs shared overlapping channel IDs.
- Implemented in-app `IPTV_TUNERR_GUIDE_NUMBER_OFFSET` and wired it into `tuner.Server.UpdateChannels` so each child instance can expose a distinct channel/guide-number range.
- Rolled a new supervisor image (`iptv-tunerr:supervisor-guideoffset-20260226001027`) on `<plex-node>` and updated the live supervisor `ConfigMap` to assign offsets:
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
  - IptvTunerr saw no `/stream/...` request
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

**Docs/packaging polish follow-up (2026-02-26 late):**
- Rewrote `README.md` from the ground up to reflect the current app shape:
  - single-tuner mode + `supervise` mode
  - HDHR wizard and injected DVR flows
  - platform support/limits (`VODFS` Linux-only)
  - tester packaging and runbook references
- Updated both feature summaries:
  - `features.md` (short overview)
  - `docs/features.md` (canonical list) with current capabilities like supervisor mode, built-in Plex session reaper, XMLTV normalization, lineup shaping, and platform support matrix.
- Added `docs/how-to/tester-release-notes-draft.md` and linked it from `docs/how-to/index.md` and `docs/index.md` for tester handoffs.

**Repo hygiene audit + root cleanup (2026-02-26 late):**
- Audited tracked files for secrets, local hostnames/paths, and personal identifiers (`<user>`, `/home/...`, `<plex-node>`, `<work-node>`, `plex.home` examples).
- No high-confidence committed secrets/tokens/private keys found in tracked files (pattern scan).
- Cleaned root-level cruft:
  - deleted tracked archive artifact `iptvtunerr-main-fixed.zip`
  - moved ad hoc/manual test scripts from repo root to `scripts/legacy/`:
    - `test_hdhr.sh`
    - `test_hdhr_network.sh`
    - `<work-node>_plex_test.sh`
  - added `scripts/legacy/README.md` clarifying they are historical/manual helpers, not supported CLI surface.

**Release automation follow-up (2026-02-26 late):**
- Updated `.github/workflows/docker.yml`:
  - explicit GHCR permissions (`packages: write`)
  - versioned tags on `v*` pushes via `docker/metadata-action`
  - retained `latest` for `main`
  - added SHA tag output (`sha-<commit>`) for traceability
- Updated `.github/workflows/tester-bundles.yml`:
  - explicit `contents: write`
  - still uploads the tester bundle as a workflow artifact
  - now also packs the staged tester bundle directory and uploads it to the GitHub Release on tag pushes (`v*`)

**Verification unblock (2026-02-26 late):**
- Fixed the pre-existing failing `internal/tuner` startup-signal test (`TestLooksLikeGoodTSStartDetectsSplitIDRStartCodeAcrossPackets`) by correcting the synthetic TS packet helper in `gateway_startsignal_test.go` to use adaptation stuffing for short payloads instead of padding bytes in the payload region.
- This restores realistic packet-boundary semantics for the cross-packet Annex-B IDR detection test and makes `./scripts/verify` green again.
## Current Focus (2026-02-26 late, VODFS/Plex VOD bring-up)

- VODFS/Plex VOD import path is now largely fixed and **TV subset imports are confirmed working**.
- Root unblocker for Plex VOD TV scans was **per-library Plex analysis jobs** (credits/chapter thumbnails/etc.) consuming/scanning the virtual libraries poorly.
- `plex-vod-register` now applies a **VOD-safe per-library preset by default** (disable heavy analysis jobs only on the VODFS libraries).
- `VOD-SUBSET` TV section started importing immediately after applying that preset and restarting/refreshing (`count > 0`, observed climbing during scan).

### In progress

- Let subset scans continue while full catalog TV backfill (`catalog.seriesfixed.json`) runs on the Plex node.
- After backfill completes, swap main VOD TV mount catalog to the repaired file and rescan the real `VOD` TV library.
- Continue hardening VOD/catch-up category support (taxonomy + deterministic sort now in-app).
- New post-backfill category rerun path is now in-app:
  - `iptv-tunerr vod-split -catalog <repaired> -out-dir <lanes-dir>` writes per-lane catalogs (`bcastUS`, `sports`, `news`, `euroUK`, `mena`, `movies`, `tv`, etc.)
  - host-side helper `scripts/vod-seriesfixed-cutover.sh` can perform retry+swap+remount cleanly before running the lane split.

### New in-app work completed this pass

- `plex-vod-register` can now configure per-library Plex prefs for VODFS libraries (default-on VOD-safe preset).
- Added VOD taxonomy enrichment + deterministic sorting for catalog movies/series (`category`, `region`, `language`, `source_tag`) during `fetchCatalog`.
- Added `vod-split` CLI command to generate per-lane VOD catalogs for category-scoped VODFS mounts/libraries.

- VODFS presented file/folder names are now prefixed with `Live: ` (via VODFS name builders), which may require Plex library refresh/metadata refresh to reflect on already-imported items.
- VOD lane heuristic tuning improved obvious false positives (`news`, `music`, `kids`, `mena`) and added provider-category-aware classification hooks, but the current local `catalog.json` has **no provider_category_* fields populated** yet (`0/157331` movies, `0/41391` series), so lane quality is still limited by title/source-tag heuristics until the catalog is regenerated with the patched Xtream indexer.
- Provider-category-driven VOD lane classification is now wired and validated via a merged test catalog; next taxonomy tuning target is region-heavy lanes (`euroUK`, `mena`) and optional `bcastUS` narrowing (currently broad because many provider categories imply region/country but not content family).
- VOD lane model now uses `euroUKMovies/euroUKTV` and `menaMovies/menaTV` plus a stricter `bcastUS` series gate. Next tuning (optional) is sub-lanes within `menaTV`/`euroUKTV` (e.g. news/kids) if desired for UX/packageing.

- Supervisor now filters parent Plex reaper/PMS env vars before spawning children to avoid accidental per-child Plex polling/SSE storms.
- Phase A lane libraries (`sports`, `kids`, `music` + `-Movies`) are now live and scanning. Next steps are scan verification and Phase B region movie lanes (`euroUKMovies`, `menaMovies`) using the same host-mount + `plex-vod-register` pattern.
- Phase A/B/C VOD lane libraries are now mounted and registered in Plex (sports/kids/music + euroUK/mena movie+TV lanes + bcastUS + TV-Intl). Remaining VOD cleanup is optional removal of unwanted companion libraries for movie-only or TV-only lane mounts (current `plex-vod-register` creates both by design).


**VOD lane Phase B/C rollout + cleanup (2026-02-26):**
- Completed Phase B and Phase C live registration in Plex for split VOD lane libraries. Intended lanes now present:
  - `euroUK-Movies`, `mena-Movies`
  - `euroUK`, `mena`, `bcastUS`, `TV-Intl`
  - plus previously added `sports`, `sports-Movies`, `kids`, `kids-Movies`, `music`, `music-Movies`
- Removed unwanted auto-created companion lane libraries caused by current `plex-vod-register` behavior always creating both TV + Movies libraries:
  - deleted Plex sections `17` (`euroUKMovies`), `19` (`menaMovies`), `22` (`euroUKTV-Movies`), `24` (`menaTV-Movies`), `26` (`bcastUS-Movies`), `28` (`TV-Intl-Movies`).
- Added `plex-vod-register` flags to avoid recreating companion libraries in future lane rollouts:
  - `-shows-only` (register only `<mount>/TV`)
  - `-movies-only` (register only `<mount>/Movies`)

**Plex library DB reverse-engineering pass (2026-02-26):**
- Extracted and inspected a live copy of `com.plexapp.plugins.library.db` using `PRAGMA writable_schema=ON` (local sqlite workaround for Plex tokenizer schema entries).
- Confirmed VOD library core table relationships and schema used by current imports:
  - `library_sections` (section metadata / agent / scanner)
  - `section_locations` (section -> root path mapping)
  - `metadata_items` (movies/shows/seasons/episodes rows)
  - `media_items` (per-metadata media summary rows)
  - `media_parts` (file path rows)
  - `media_streams` (stream analysis rows; often empty until deeper analysis runs)
- Sample observations from lane libraries:
  - `sports-Movies` imported items currently have `metadata_items` rows and placeholder `media_items/media_parts` (`size=1`, empty codecs/container) due VODFS placeholder attr strategy and VOD-safe analysis settings
  - `VOD-SUBSET` TV section shows full hierarchy (`metadata_type` distribution `2=shows`, `3=seasons`, `4=episodes`) with episode `media_parts.file` paths pointing at the VODFS mount.
- Confirmed `media_provider_resources` schema for Live TV provider/device chain contains only IDs/URIs/protocol/status (`id,parent_id,type,identifier,protocol,uri,...`) and **does not contain per-provider friendly-name/title columns**.
- Combined with `/media/providers` API capture showing every Live TV provider emitted as `friendlyName="plexKube"`, this strongly indicates Plex synthesizes source-tab labels from the server-level `friendlyName`, not from per-DVR/provider DB rows.

- 2026-02-26: Reverse-engineered Plex Web Live TV source label logic in WebClient `main-*.js` (`function Zs` + module `50224`). Confirmed Plex Web chooses `serverFriendlyName` for multiple Live TV sources on a full-owned server, which is why tabs all showed `plexKube`. Patched running Plex Web bundle to inject a providerIdentifier->lineupTitle map (from `/livetv/dvrs`) so tab labels are per-provider (`newsus`, `bcastus`, ..., `iptvtunerrHDHR479`, `iptvtunerrHDHR479B`). This is a runtime bundle patch (survives until Plex update/image replacement); browser hard refresh required.

- 2026-02-26: Reverted the experimental Plex Web `main-*.js` bundle patch after it broke Web UI loading for the user. Implemented `scripts/plex-media-providers-label-proxy.py` instead: a server-side reverse proxy that rewrites `/media/providers` Live TV `MediaProvider` labels (`friendlyName`, `sourceTitle`, `title`, content root Directory title, watchnow title) using `/livetv/dvrs` lineup titles. Validated on captured `/media/providers` XML: all 15 `tv.plex.providers.epg.xmltv:<id>` providers rewrite to distinct labels (`newsus`, `bcastus`, ..., `iptvtunerrHDHR479B`). Caveat documented: current Plex Web version still hardcodes server-friendly-name labels for owned multi-LiveTV sources, so proxy primarily targets TV/native clients unless WebClient is separately patched.

- 2026-02-26: Deployed `plex-label-proxy` in k8s (`plex` namespace) and patched live `Ingress/plex` to route `Exact /media/providers` to `plex-label-proxy:33240` while leaving all other paths on `plex:32400`. Proxy is fed by ConfigMap from `scripts/plex-media-providers-label-proxy.py` and rewrites Live TV provider labels per DVR using `/livetv/dvrs`. Fixed gzip-compressed `/media/providers` handling after initial parse failures. End-to-end validation via `https://plex.home/media/providers` confirms rewritten labels for `tv.plex.providers.epg.xmltv:{218,220,247,250}` (`newsus`, `bcastus`, `iptvtunerrHDHR479`, `iptvtunerrHDHR479B`).

**Session 2026-02-28 (this session):**

- **Postvalidate CDN rate-limit fix:** Reduced `POSTVALIDATE_WORKERS` from 12 to 3 and added per-probe jitter (`POSTVALIDATE_PROBE_JITTER_MAX_S=2.0`, random sleep before each ffprobe) in `k3s/plex/iptv-m3u-server-split.yaml` and `k3s/plex/iptv-m3u-postvalidate-configmap.yaml`. Updated default in the script from 12 to 3. This directly addresses the CDN saturation false-fail pattern where the 13-way category split had newsus/sportsb/moviesprem/ukie/eusouth all drop to 0 channels mid-run (2026-02-25 evidence). If 3 still fails on further runs, reduce `POSTVALIDATE_WORKERS` to 1.

- **Stale DVR cleanup:** Removed oracle-era HDHR DVRs `247` (`iptvtunerrHDHR479`, device `iptvtunerr-hdhr-test.plex.svc:5004`) and `250` (`iptvtunerrHDHR479B`, device `iptvtunerr-hdhr-test2.plex.svc:5004`) from Plex via `plex-epg-oracle-cleanup -device-uri-substr iptvtunerr-hdhr-test -do`. The 13 active category DVRs (`218..242`) were preserved.

- **Credential hygiene in test YAML:** Updated `k8s/iptvtunerr-hdhr-test.yaml` to remove the deleted `plex-iptv-creds` Secret references (`secretRef` and `secretKeyRef`). The ConfigMap now has an explanatory comment pointing to OpenBao agent injection or a deploy-time Secret as the credential source. Deployments of this manifest must supply credentials via one of those paths.

- **Verify script fix:** `scripts/verify-steps.sh` format check now excludes `vendor/` (was failing on third-party files with `gofmt -s -l .`). Changed to `find . -name '*.go' -not -path './vendor/*' | xargs gofmt -s -l`.

- **VODFS remount + VOD library re-registration:** All 11 VODFS lane mount processes died when the Plex pod restarted (due to missing `mountPropagation: HostToContainer` on hostPath volumes). Restarted all processes on kspls0 with sudo, restarted Plex pod (no active sessions), then re-registered all VOD libraries from inside the new Plex pod. Libraries registered and scanning: VOD (key 29), VOD-Movies (30), VOD-SUBSET (31), VOD-SUBSET-Movies (32), sports/sports-Movies (33/34), kids/kids-Movies (35/36), music/music-Movies (37/38), bcastUS/bcastUS-Movies (39/40), euroUK-Movies (41), euroUK (42), mena-Movies (43), mena (44), TV-Intl/TV-Intl-Movies (45/46).
  - Root cause documented in `memory-bank/known_issues.md`: FUSE mounts started after pod start are invisible without `mountPropagation: HostToContainer`.
  - Recovery procedure for next time: start FUSE processes on plex node → confirm mounts → `kubectl rollout restart deployment/plex` → copy iptv-tunerr binary to new pod → re-run `plex-vod-register` per lane.

**Next focus:** Monitor VOD library scan counts. Fix `mountPropagation` on the Plex deployment YAML for durable VOD mount visibility (requires the live deployment YAML in k3s/plex to be patched). Consider systemd services for VODFS mounts on kspls0 for auto-restart on node reboot.
**Session 2026-03-18 (recorder-daemon docs follow-up):**

- Merged the later discussion about Plex DVR differences and headless provider-limited concurrency into `docs/explanations/always-on-recorder-daemon.md`.
- This keeps the future-feature explainer self-contained instead of splitting the concept across chat-only context.
