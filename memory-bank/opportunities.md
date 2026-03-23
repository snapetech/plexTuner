# Opportunities (Continuous Improvement Backlog)

This is a lightweight backlog for improvements discovered during other work.
It exists to encourage quality gains without derailing the current task.

**Consolidated “what’s left” index:** For a single-page map that links this file, LTV/LP epics, **`known_issues`**, **`docs-gaps`**, and intentional limits, see **[docs/explanations/project-backlog.md](../docs/explanations/project-backlog.md)**. Update that index when you add or close major themes; keep **evidence and scope** in the dated entries below.

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

- Date: 2026-03-22
  Category: other
  Title: Grow Tunerr into a full general-purpose library janitor
  Context: Migration work is expanding from Live TV into library definitions, catch-up publishing, parity audit, and now identity/account cutover. The user explicitly wants the broader direction on the backlog too: use Tunerr as the operator control plane for cross-server library hygiene, not just one-shot migration.
  Why it matters: Large Plex/Emby/Jellyfin estates end up needing continuous reconciliation for stale libraries, path drift, unwanted leftovers, scan lag, parity gaps, and clean shutdown of old platform-specific state. That is adjacent to the migration lane but materially larger than the current scope.
  Evidence: Current repo now has `internal/livetvbundle`, migration audit/reporting, and generated-library registration flows, but no general-purpose janitor/rule engine for ongoing cleanup and reconciliation.
  Suggested fix: Treat this as a dedicated future epic: build a rule-driven janitor surface with preview/diff/apply, exclusions, safe delete/archive policies, scan/parity repair actions, and operator-visible reports across Plex/Emby/Jellyfin.
  Risk/Scope: high | fits current scope? no
  User decision needed?: yes
  If yes: 1) Janitor design spike only (Recommended), 2) MVP cleanup/reconciliation rules for generated libraries only, 3) Full cross-server janitor roadmap. If no answer: keep as backlog and continue identity/account migration plus overlap-sync work.

- Date: 2026-03-21
  Category: reliability
  Title: Adaptive per-account contract learning for provider pools
  Context: The gateway now has explicit provider-account leasing and can enforce `IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT`, but that cap is still operator-tuned. Different panels can allow different concurrent counts per credential set, and those limits may need to be learned independently per account rather than assumed globally.
  Status: **Shipped 2026-03-21** — Tunerr now learns tighter per-account caps from upstream concurrency-limit signals, persists them with `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE`, expires stale learning with `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS`, restores them on startup, and exposes the full state on `/provider/profile.json` plus `/debug/runtime.json`.
  Why it matters: Multi-account concurrency is now both more truthful at runtime and durable across process restarts without becoming permanent stickiness.
  Evidence: `internal/tuner/gateway_accounts.go`, `internal/tuner/account_limit_store.go`, `internal/tuner/server.go`, `/provider/profile.json`.
  Suggested fix: Reopen only if a provider needs smarter decay than the current TTL model or if operators need per-account manual override controls.
  Risk/Scope: n/a
  User decision needed?: no

- Date: 2026-03-21
  Category: reliability
  Title: Tighten startup registration contract beyond placeholder `guide.xml`
  Context: Security/control-plane hardening fixed empty-guide caching and made placeholders visibly provisional, but `run` still binds early and serves `200` placeholder XMLTV while lineup/catalog work is still underway.
  Status: **Shipped 2026-03-21** — `/guide.xml` now returns `503` + `Retry-After: 5` + `X-IptvTunerr-Guide-State: loading` while only the placeholder body exists, and HDHR discovery/lineup surfaces add `X-IptvTunerr-Startup-State: loading` before lineup load. Regression lane added to `memory-bank/commands.yml` as `release_smoke`.
  Risk/Scope: n/a
  User decision needed?: no

- Date: 2026-03-21
  Category: operability
  Title: Promote stream-compare/channel-diff/evidence intake from scripts into first-class operator workflows
  Context: Programming Manager now has live preview and detail cards, but the strongest failure-isolation tooling still lives in shell/python helpers (`stream-compare-harness`, `channel-diff-harness`, `evidence-intake`, `analyze-bundle`) rather than the deck.
  Status: **Partially shipped 2026-03-21** — `/ops/workflows/diagnostics.json` now gives operators a good-vs-bad capture workflow with recent attempt suggestions and latest `.diag/` families, `/ops/actions/evidence-intake-start` scaffolds `.diag/evidence/<case-id>/` directly from the app, the deck now surfaces that workflow, and `scripts/ci-smoke.sh` covers it. **Remaining:** trigger harnesses from the deck and summarize `stream-compare` / `channel-diff` results in-card instead of requiring shell execution and manual report inspection.
  Why it matters: Faster triage for intermittent channel-class failures without dropping straight to shell/python every time.
  Evidence: `internal/tuner/server.go`, `internal/webui/deck.js`, `scripts/ci-smoke.sh`.
  Suggested fix: Add operator actions that can launch safe local harness runs with bounded inputs, then surface report summaries and latest diff verdicts directly in the diagnostics lane.
  Risk/Scope: med | fits current scope? no
  User decision needed?: no

- Date: 2026-03-19
  Category: operability
  Title: (partially addressed 2026-03-19) Autopilot **provider-level** host policy beyond per-DNA JSON
  Context: [EPIC-live-tv-intelligence](../../docs/epics/EPIC-live-tv-intelligence.md) “next” listed multi-host policy memory; operators needed a no-state-file knob for trusted CDN hostnames.
  Status: **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`** shipped; **`IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE`** (preferred + blocked hosts, JSON) shipped — **`reorderStreamURLs`**, **`/autopilot/report.json`**, **`/debug/runtime.json`**. **Remaining:** UI-driven policy, automatic strip/cap from **`remediation_hints`**, etc.
  Risk/Scope: n/a
  User decision needed?: no

- Date: 2026-03-22
  Category: operability
  Title: Unified **`.diag/`** harness index (optional static `index.html` or `scripts/harness-index.py`)
  Context: **§7** / **§9** / **§10** now have matching [how-to docs](../../docs/how-to/) (`live-race`, `stream-compare`, `multi-stream`), but operators still dig through timestamped folders by hand.
  Status: **MVP shipped** — **`scripts/harness-index.py`** lists newest runs per family (**`--json`**); linked from harness how-tos + **`commands.yml`** **`harness_index`**. Optional future: HTML index or **`--open`** — file under [opportunities](../../memory-bank/opportunities.md) if needed.
  Why it matters: Faster triage when multiple harness runs exist under **`.diag/live-race`**, **`.diag/stream-compare`**, **`.diag/multi-stream`**.
  Evidence: Runbooks + how-tos; no single directory browser.
  Suggested fix: Small script that lists latest N runs per family + embeds **`report.json`** / **`summary.txt`** excerpts; optional **`--open`** for file manager. Keep out of **`scripts/verify`** (not CI-critical).
  Risk/Scope: low | fits current scope? yes (isolated script + short doc link)
  User decision needed?: no

- Date: 2026-03-22
  Category: operability
  Title: **Probe** results — “what to do next” how-to (closes [docs-gaps.md](../../docs/docs-gaps.md) high row)
  Context: **`iptv-tunerr probe`** output is clear to maintainers but not always actionable for new operators (CF vs auth vs DNS vs empty categories).
  Status: **Shipped** — [docs/how-to/interpreting-probe-results.md](../../docs/how-to/interpreting-probe-results.md); README **`probe`** row; runbook §4 link; [docs-gaps.md](../../docs/docs-gaps.md) **Resolved** row.
  Why it matters: Reduces chat loops after “probe says …”.
  Evidence: [docs-gaps.md](../../docs/docs-gaps.md) § High; **`cmd`** probe + [cli-and-env-reference](../../docs/reference/cli-and-env-reference.md).
  Suggested fix: **`docs/how-to/interpreting-probe-results.md`** with decision table (symptom → env/flag → next command); link from README **Quick Start** and runbook §4.
  Risk/Scope: low | fits current scope? yes (docs-only)
  User decision needed?: no

- Date: 2026-03-22
  Category: maintainability
  Title: **Plex connect** step-by-step (UI wizard vs `-register-plex` vs API) — [docs-gaps.md](../../docs/docs-gaps.md) high row
  Context: README covers modes at high level; **480-channel wizard limit**, channelmap activation, and “headless DVR exists but guide empty” still generate support churn.
  Status: **Shipped** — [docs/how-to/connect-plex-to-iptv-tunerr.md](../../docs/how-to/connect-plex-to-iptv-tunerr.md); README **How-To**; **`docs/how-to/index`**, **`docs/index`**, **`reference/index`**; **`docs-gaps.md`** **Resolved**; **`features.md`** §8 reference + §14 metrics cross-link.
  Why it matters: Onboarding and category-DVR fleets need one honest checklist.
  Evidence: [docs-gaps.md](../../docs/docs-gaps.md); [plex-dvr-lifecycle-and-api.md](../../docs/reference/plex-dvr-lifecycle-and-api.md); [ADR 0001](../../docs/adr/0001-zero-touch-plex-lineup.md).
  Suggested fix: **`docs/how-to/connect-plex-to-iptv-tunerr.md`** with flows A/B/C + troubleshooting links; README + docs index.
  Risk/Scope: med (UX wording) | fits current scope? yes as docs-only epic slice
  User decision needed?: no (safe defaults: document all three paths honestly)

- Date: 2026-03-19
  Category: maintainability
  Title: (superseded 2026-03-19) Optional — smaller pieces inside **`gateway_stream_upstream.go`**
  Context: Earlier audit flagged **`walkStreamUpstreams`** as the remaining merge hotspot after the first gateway split.
  Status: Non-OK upstream handling + success relay branches now live in **`gateway_stream_response.go`**; **`gateway_stream_upstream.go`** is back to the orchestration loop. Reopen only if churn concentrates again inside the new helper file.
  Risk/Scope: n/a (historical)
  User decision needed?: no

- Date: 2026-03-19
  Category: operability
  Title: HLS mux — consolidated operator toolkit + enhancement backlog doc
  Context: User asked for a single place listing many future HLS-mux-style improvements (diagnostics, protocol edge cases, security, scale) without opening N separate backlog tickets.
  Status: **Reference doc shipped** — [docs/reference/hls-mux-toolkit.md](../docs/reference/hls-mux-toolkit.md). **Remaining:** implement or decline individual candidate rows in that doc (tracked there and in [project-backlog §2–3](../docs/explanations/project-backlog.md)); this opportunities row is **closed** for “single place exists.”
  Why it matters: Keeps gateway/HLS work discoverable for humans and agents; items graduate into scoped PRs with evidence when picked up.
  Evidence: New reference page [docs/reference/hls-mux-toolkit.md](../docs/reference/hls-mux-toolkit.md) (tables, `curl` recipes, categorized candidates).
  Suggested fix: When implementing a slice from the toolkit, add ADR/test link in that doc or move the row to `known_issues` if it becomes a user-facing limitation.
  Risk/Scope: low | fits current scope? yes (docs-only)
  User decision needed?: no

- Date: 2026-03-19
  Category: performance
  Title: Provider incremental XMLTV fetch (avoid full xmltv.php each refresh)
  Context: `fetchProviderXMLTV` in `internal/tuner/epg_pipeline.go` pulls the whole provider XMLTV when HTTP conditional GET cannot short-circuit; SQLite `GlobalMaxStopUnix` could bound a smaller window **if** the provider supports date-range parameters (non-standard; panel-specific).
  Why it matters: Large provider guides waste bandwidth/CPU on every cache tick.
  Evidence: Epic PR-6 notes “incremental-only ingest” as future; Tunerr SQLite sync remains full replace per refresh.
  Suggested fix: **`IPTV_TUNERR_PROVIDER_EPG_DISK_CACHE`** (2026-03-19) implements on-disk body + **`If-None-Match`** / **`If-Modified-Since`** when upstream sends validators. **`IPTV_TUNERR_PROVIDER_EPG_INCREMENTAL`** + suffix tokens + **`IPTV_TUNERR_EPG_SQLITE_INCREMENTAL_UPSERT`** reduce full-table churn when panels support window params / overlap sync. Remaining: provider-specific param names still manual via suffix; true ffmpeg-only incremental **xmltv.php** pulls not applicable server-side without provider contract.
  Risk/Scope: high | fits current scope? no
  User decision needed?: yes (provider contract / compatibility)

- Date: 2026-03-19
  Category: operability
  Title: Optional PostgreSQL backend for shared multi-writer EPG
  Context: Lineup parity track uses SQLite file (`internal/epgstore`) by default; operators with HA / multiple Tunerr instances may ask for Postgres.
  Why it matters: SQLite fits single-writer + local file; clustered deployments may want a networked DB (see [ADR 0003](../docs/adr/0003-epg-sqlite-vs-postgres.md)).
  Evidence: ADR documents when to reconsider; no Postgres code path exists today.
  Suggested fix: Only if product requires multi-instance shared EPG — add explicit epic, connection config, and migration story; do not “accidentally” grow Postgres for a single-binary bridge.
  Risk/Scope: high | fits current scope? no
  User decision needed?: yes (only if multi-writer EPG becomes a requirement)

- Date: 2026-03-18
  Category: other
  Title: Build an always-on recorder daemon for non-replay catch-up
  Context: User asked what a full recorder-backed future feature would be, beyond current `catchup-publish`, replay templates, and `catchup-record`.
  Why it matters: The repo now has strong guide-driven catch-up packaging, but non-replay sources still lack a continuous background capture layer. An always-on recorder daemon would turn short-lived catch-up libraries into real recorded assets instead of only launcher/replay surfaces.
  Evidence: Current shipped stack includes `catchup-publish`, `catchup-record`, replay-template support, capsule curation, and media-server publishing, but no continuous scheduler/worker/retention subsystem.
  Suggested fix: Build a future `catchup-daemon` subsystem with a scheduler, recording workers, persistent state, publisher integration, and retention sweeps. Start with a small MVP that records `in_progress` / `starting_soon` items for selected lanes.
  Risk/Scope: high | fits current scope? no
  User decision needed?: yes
  If yes: 1) design/spec only (Recommended), 2) MVP single-process daemon for one or two lanes, 3) full multi-worker recorder/publisher architecture. If no answer: keep as a documented future feature and continue current catch-up tooling evolution.

- Date: 2026-03-19
  Category: maintainability
  Title: (superseded 2026-03-19) Old audit bullets — `main.go` monolith, giant `gateway.go`, guide **`DefaultClient`**
  Context: March 2026 audit assumed **`cmd/iptv-tunerr/main.go`** ~900+ lines, **`internal/tuner/gateway.go`** ~3000+ lines, and guide paths on **`http.DefaultClient`**.
  Status: **INT-005** slimmed **`main.go`** to a dispatcher; **INT-006** split gateway into many **`gateway_*.go`** files (**`gateway_servehttp`**, **`gateway_stream_upstream`**, **`gateway_upstream_cf`**, …); **INT-001** + **`internal/refio`** + **`internal/httpclient`** rewired shared loading and clients. Re-audit with `wc -l` and `rg http.DefaultClient` before reopening.
  Risk/Scope: n/a (historical)
  User decision needed?: no

- Date: 2026-03-18
  Category: operability
  Title: Feed guide-health and channel-intelligence results back into runtime publishing and lineup decisions
  Context: Fresh whole-project audit of the new intelligence layer.
  Status: **Partially superseded (2026-03-19)** — **`IPTV_TUNERR_GUIDE_POLICY`** / **`IPTV_TUNERR_CATCHUP_GUIDE_POLICY`**, **`GET /guide/policy.json`**, **`UpdateChannels`** guide pruning, **`catchup-publish -guide-policy`**, **`catchup-capsules`** with policy, **`IPTV_TUNERR_REGISTER_RECIPE=healthy`** are shipped. **Remaining:** optional deeper wiring (e.g. more registration paths keyed solely off live **`guide-health`** scores) if product wants stricter defaults everywhere.
  Why it matters: the app can now diagnose weak guide coverage very well; tightening every remaining path is product scope.
  Evidence (historical): older audit predated policy flags; see **`guide_policy.go`**, **`FilterCatchupCapsulesByGuidePolicy`**, server **`UpdateChannels`**.
  Suggested fix: Re-audit only if a concrete path still ignores guide quality after **`IPTV_TUNERR_*_POLICY`**.
  Risk/Scope: med | fits current scope? no (unless new requirement)
  User decision needed?: only for new default strictness

- Date: 2026-03-18
  Category: other
  Title: Catch-up publishing currently produces near-live launchers, not program-bound playback
  Context: Fresh whole-project audit of the catch-up subsystem.
  Why it matters: the feature is useful today, but users may assume each published item replays a specific programme window. Right now it launches the current live channel stream, so the library object is metadata-rich but not a true time-shifted asset.
  Evidence: `internal/tuner/catchup_publish.go:97` writes `.strm` files pointing to `streamBaseURL + \"/stream/\" + capsule.ChannelID`; no time window, DVR buffer, or recording path is encoded.
  Suggested fix: Keep documenting the current behavior honestly, and consider a future "true catch-up" mode backed by provider timeshift/replay URLs or a short rolling recorder.
  Risk/Scope: med-high | fits current scope? no
  User decision needed?: yes
  If yes: 2–3 options + recommended default + what you will do if no answer
  Recommended default: keep the current near-live publisher, label it clearly as a launcher, and only pursue true replay once there is a real time-shift source.

- Date: 2026-03-19
  Category: maintainability
  Title: (superseded) **`repo_map`** remotes + under-described modules; stale **`architecture.md`**
  Context: 2026-03-18 audit claimed **`repo_map`** still told users to push **`plex`** and skipped intelligence/catch-up; **`architecture.md`** allegedly lacked layered view.
  Status: **`repo_map`** documents **`origin`-only** push + **`channelreport`/`channeldna`** + **`webui`** + HTTP client map + **`architecture.md`** link. **`docs/explanations/architecture.md`** has three-layer overview + **`channeldna`/`channelreport`** + catch-up publishing + post-**INT-005** CLI note. Re-open only after a real regression audit.
  Risk/Scope: n/a
  User decision needed?: no

- Date: 2026-03-19
  Category: maintainability
  Title: (superseded) Split monolithic **`cmd/iptv-tunerr/main.go`**
  Context: Pre-**INT-005** note assumed **`main.go`** owned all command flags and handlers.
  Status: **`main.go`** is a thin dispatcher (**~100** lines); **`cmd_registry.go`** + **`cmd_*.go`** (25+ modules) own flags and **`Run`** handlers (**`cmd_util.go`** helpers). Further splits are optional polish only.
  Risk/Scope: n/a
  User decision needed?: no

- Date: 2026-02-26
  Category: operability
  Title: Add near-live catch-up library mode with category-split Plex libraries and targeted scans
  Context: User product-direction discussion during VODFS/Plex import debugging; Live TV sharing constraints and large-library scan pain both surfaced.
  Why it matters: Plex Live TV sharing is limited (Plex Home-focused) and giant hot libraries are expensive to scan/update. A near-live catch-up model using ordinary Plex libraries could improve remote sharing, UX, and scanner performance if designed around program-bounded assets + targeted scans.
  Evidence: Current VOD tests show full-library scale (~157k movies / ~41k series) makes validation and scan feedback slow; subset libraries (`VOD-SUBSET`, `VOD-SUBSET-Movies`) provide much faster, clearer signal. User provided detailed architecture/cadence proposal (EPG backend metadata; published assets as Plex items; collections/recommendation rows as UX veneer).
  Suggested fix: Design and implement a first-class catch-up mode with: (1) program-bounded finalized assets only, (2) event-driven publish + path-specific section scans, (3) hourly-ish retention sweeps, (4) category-split Plex libraries (e.g. `bcastUS`, `sports`, `news`, regional buckets), and (5) optional collection/shelf curator on a 10–15 minute cadence.
  Risk/Scope: high | fits current scope: no (new product mode / multi-worker architecture)
  User decision needed?: yes
  If yes: 1) Spike only (taxonomy + schema + worker plan) (Recommended), 2) MVP implementation for one category library + publisher/scan worker, 3) Full catch-up product roadmap/epic. If no answer: keep as documented opportunity and continue current VODFS/library import stabilization.

- Date: 2026-02-26
  Category: reliability
  Title: Add in-app/operator command to detect and clear Plex hidden Live TV "active grabs" without full Plex restart
  Context: Post guide-number-offset remap validation for 15 DVRs; Plex Web clicks did nothing until Plex restart.
  Status: **Partially addressed (2026-03)** — Tunerr exposes localhost/LAN **`POST /ops/actions/ghost-visible-stop`** and **`POST /ops/actions/ghost-hidden-recover`** (guarded helper; see [cli-and-env](../docs/reference/cli-and-env-reference.md) Ghost Hunter section). **Remaining:** Plex-side hidden state may still require PMS restart in some cases; full “no restart” cleanup is not guaranteed from Tunerr alone.
  Why it matters: After large guide/channel remaps, Plex can wedge tunes (`Waiting for media grab to start`) even when `/status/sessions` is empty. Restarting Plex works but is heavy-handed and interrupts all clients.
  Evidence: `Plex Media Server.5.log` during `POST /livetv/dvrs/218/channels/2001/tune` showed `There are 2 active grabs at the end.` and `Waiting for media grab to start.`; same channel tuned normally after `deploy/plex` restart.
  Suggested fix: Investigate PMS APIs or DB/state paths for stale grab cleanup (or add an operator helper that detects the log pattern and recommends/executes a targeted restart only when no active sessions exist).
  Risk/Scope: med | fits current scope: no (Plex internals / ops tooling)
  User decision needed?: no

- Date: 2026-03-19
  Category: maintainability
  Title: (superseded) “Official” CLI/env reference page missing
  Context: Older note predates [cli-and-env-reference](../docs/reference/cli-and-env-reference.md) and [reference index](../docs/reference/index.md).
  Status: **`docs/reference/cli-and-env-reference.md`** is the canonical env/flag map; **`testing-and-supervisor-config.md`** remains lab/supervisor-focused. Optional future: auto-generate fragments from code — not required for operators today.
  Risk/Scope: n/a
  User decision needed?: no

- Date: 2026-02-25
  Category: reliability
  Title: Postvalidate CDN rate-limiting causes false-positive stream drops
  Context: ../k3s/plex/iptv-m3u-server-split.yaml POSTVALIDATE_WORKERS=12, sequential DVR files
  Why it matters: 12 concurrent ffprobe workers testing streams sequentially per DVR exhaust CDN capacity by mid-run. newsus/sportsb/moviesprem/ukie/eusouth all dropped to 0 channels (100% false-fail). bcastus passed 136/136 (ran first, CDN not yet limited).
  Evidence: postvalidate run 2026-02-25: bcastus=136/136 (no drops), newsus=0/44, sportsb=0/281, moviesprem=0/253, ukie=0/112, eusouth=0/52 — all "Connection refused" in that order.
  Suggested fix: (a) Reduce POSTVALIDATE_WORKERS to 3-4 with random jitter, (b) add per-host rate limit delay, (c) skip postvalidate for EU buckets if cluster is US-based (geo-block), or (d) disable postvalidate entirely and rely on EPG prune + FALLBACK_RUN_GUARD.

- Date: 2026-02-25
  Category: reliability
  Title: (superseded 2026-03-19) plex-reload-guides-batched.py used wget (not in Plex container)
  Context: k3s/plex `plex-reload-guides-batched.py` historically used `wget`; Plex images typically ship `curl` only.
  Status: Tracked copy is **`curl`**-based; re-open only if an old ConfigMap/script reintroduces **`wget`**.
  Risk/Scope: n/a (historical)
  User decision needed?: no

- Date: 2026-02-24
  Category: security
  Title: Replace committed provider credentials in `k8s/iptvtunerr-hdhr-test.yaml`
  Context: While adding one-shot deploy automation, the tracked test manifest currently contains concrete provider-looking values in the ConfigMap.
  Why it matters: Even if test-only, committed credentials/URLs increase secret leakage risk and normalize unsafe workflow.
  Evidence: `k8s/iptvtunerr-hdhr-test.yaml` ConfigMap `iptvtunerr-test-env` has explicit `IPTV_TUNERR_PROVIDER_*` and `IPTV_TUNERR_M3U_URL` values.
  Suggested fix: Replace with placeholders (or sample values), keep one-shot script/Secret flow as the recommended path, and rotate any real credentials if they were valid.
  Risk/Scope: low | fits current scope: no (logged only).
  User decision needed?: no

- Date: 2026-03-19
  Category: maintainability
  Title: (superseded 2025 note) **`internal/indexer`** missing from clone
  Context: Historical opportunity when indexer lived outside tree or branch was incomplete.
  Status: **`internal/indexer`** is in-repo; **`go build ./cmd/iptv-tunerr`** succeeds on **main**.
  Risk/Scope: n/a
  User decision needed?: no

- Date: 2026-02-24
  Category: performance
  Title: (superseded 2026-03-19) `/guide.xml` external XMLTV was per-request / high latency
  Context: Feb 2026 k3s runs showed multi-second **`/guide.xml`** when the merged guide pipeline did work inline with Plex metadata traffic.
  Status: **`internal/tuner/xmltv.go`** caches the **merged** guide (**`cachedXML`** / **`cacheExp`**), honors **`IPTV_TUNERR_XMLTV_CACHE_TTL`** (default **10m**), refreshes in the background (**`StartRefresh`**), and keeps the last good bytes on fetch failure (no guide “outage” on transient upstream errors). **`TestXMLTV_cacheHit`** ( **`xmltv_test.go`** ) guards single-fetch behavior. Re-open only with evidence the **current** path still piles up concurrent pipeline runs.
  Risk/Scope: n/a (historical)
  User decision needed?: no

- Date: 2026-02-24
  Category: operability
  Title: Add guidance and tooling for Plex-safe lineup sizing (WebSafe had 41k+ channels)
  Context: Live Plex API testing on DVR `138` (`iptvtunerrWebsafe`) in k3s.
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
  Risk/Scope: low | fits current scope: no (k3s/job tooling change, not IptvTunerr code)
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
  Context: Direct IptvTunerr (no Threadfin) WebSafe testing with real XMLTV on `iptvtunerr-websafe`.
  Why it matters: Plex guide UX can show many "Unavailable Airings" even when streaming works if `lineup.json` contains duplicate `tvg-id` rows while XMLTV remap dedupes guide channels/programmes by `tvg-id`.
  Evidence: Live direct WebSafe test observed `188` lineup channels vs `91` guide channels after XMLTV remap; deduping `catalog.live_channels` by `tvg_id` before `serve` fixed the mismatch (`91/91`) and removed the guide/linkage mismatch.
  Suggested fix: Add a built-in catalog/live-channel dedupe option (e.g., by `tvg-id`) and emit counts/logs for dropped duplicates when XMLTV remap is enabled, so direct Plex mode stays lineup/guide-aligned without an external preprocessing step.
  Risk/Scope: med | fits current scope: no (new config/behavior choice)
  User decision needed?: yes (default behavior for duplicates: keep first, prefer highest-priority source, or make it opt-in; recommended default: opt-in first, then consider enabling automatically when XMLTV remap is active).

- Date: 2026-02-24
  Category: reliability
  Title: Canonicalize/resolve HLS input hosts before ffmpeg (k3s short service hostname compatibility)
  Context: Live WebSafe ffmpeg testing in `iptvtunerr-build` pod with `IPTV_TUNERR_FFMPEG_PATH=/workspace/ffmpeg-static`.
  Why it matters: ffmpeg can fail on Kubernetes short service hostnames (for example `iptv-hlsfix.plex.svc`) even though Go HTTP fetches work, causing IptvTunerr to silently fall back to the raw relay path that Plex Web cannot parse reliably.
  Evidence: WebSafe logs showed `ffmpeg-transcode failed (falling back to go relay)` with stderr `Failed to resolve hostname iptv-hlsfix.plex.svc`; replacing the hostname with the numeric service IP in a catalog copy allowed ffmpeg to start.
  Suggested fix: Before invoking ffmpeg on HLS URLs, canonicalize/resolve the URL host in Go (for example resolve to IP or rewrite k8s `.svc` hosts to a form ffmpeg can resolve) and log the rewritten input host.
  Risk/Scope: med | fits current scope: no (runtime behavior change + tests)
  User decision needed?: no

- Date: 2026-02-24
  Category: operability
  Title: Strengthen Plex test helpers to detect/clear hidden Live TV `CaptureBuffer` state
  Context: Browser probe iteration on direct WebSafe/Trial DVRs while tuning ffmpeg startup behavior.
  Why it matters: Repeated probes on the same channel can reuse hidden Plex `CaptureBuffer`/transcode state that is not visible in `/status/sessions` or `/transcode/sessions`, causing false test results and making tuner changes appear ineffective.
  Evidence: `start.mpd` debug XML repeatedly returned the same `TranscodeSession` key for a channel, with no new `/stream/...` request in IptvTunerr logs, while `/status/sessions` and `/transcode/sessions` both reported `size=0`; `universal/stop?session=<id>` returned `404`.
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
  Suggested fix: Add a temporary/debug-only TS introspection mode in IptvTunerr (or a helper script) that samples the first N packets/seconds from the emitted MPEG-TS and logs parsed PCR/PTS/DTS continuity + discontinuity indicators for one request ID.
  Risk/Scope: med | fits current scope: no (new debug tooling/format)
  User decision needed?: no

- Date: 2026-02-25
  Category: performance
  Title: (superseded 2026-03-19 — duplicate) XMLTV fetch storm on concurrent `/guide.xml`
  Context: Same concern as **2026-02-24** opportunity (pre-merge-cache behavior).
  Status: Resolved with the same **`XMLTV`** merged-guide cache + TTL + background refresh described in the **2026-02-24** superseded row.
  Risk/Scope: n/a (historical)
  User decision needed?: no

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
  Title: (superseded 2026-03-19) Catalog refresh: Save vs **`UpdateChannels`** ordering
  Context: Older audit feared scheduled refresh called **`UpdateChannels`** before **`catalog.Save`**, or non-atomic writes.
  Status: **`catalog.Save`** uses **temp + rename** (`internal/catalog/catalog.go`). **`cmd/iptv-tunerr` `handleRun`** scheduled refresh: **`Save`** first, then **`channeldna.Assign`** + **`UpdateChannels`** only on success (`cmd_runtime.go`). Initial startup: save after index before serve when applicable.
  Risk/Scope: n/a (historical)
  User decision needed?: no

- Date: 2026-02-25
  Category: operability
  Title: (superseded 2026-03-19) **`SIGHUP`** catalog reload
  Context: Operators wanted reload without pod restart.
  Status: **`handleRun`** registers **`SIGHUP`** with the provider-credentials refresh goroutine; log line **`SIGHUP received — reloading catalog`** (`cmd_runtime.go`).
  Risk/Scope: n/a (historical)
  User decision needed?: no

- Date: 2026-02-25
  Category: operability
  Title: (superseded 2026-03-19) **`/healthz`** / **`/readyz`** for Kubernetes readiness
  Context: Examples used **`/discover.json`** for readiness only.
  Status: **`GET /healthz`** returns **503** `loading` until **`UpdateChannels`** has live rows, then **JSON** `ok` + **`source_ready`** + **`channels`** + **`last_refresh`**. **`GET /readyz`** uses the same gate with **`status`** **`ready`** / **`not_ready`** (`internal/tuner/server.go`, **`TestServer_healthz`**, **`TestServer_readyz`**). Example manifests: readiness MAY probe **`/readyz`** (or **`/healthz`**); **liveness** should stay something that stays **200** during long first catalog builds (often **`/discover.json`**).
  Risk/Scope: n/a (historical; update local manifests when convenient)
  User decision needed?: no

- Date: 2026-02-25
  Category: operability
  Title: Multi-arch (ARM64) Docker images for k8s clusters with ARM nodes
  Context: Dockerfile.static, Dockerfile.static.distroless, Dockerfile.static.scratch — all use standard Go cross-compile but don't declare platform targets.
  Why it matters: Home k8s clusters (Raspberry Pi, Apple Silicon VMs, cloud Graviton) often have ARM64 nodes. Without a multi-arch build, the image silently runs under QEMU emulation or fails to schedule. Static Go binaries cross-compile trivially with GOARCH=arm64 CGO_ENABLED=0.
  Evidence: Dockerfile.static uses `RUN go build` with no GOARCH override. k8s/iptvtunerr-hdhr-test.yaml has no nodeSelector; would fail on ARM node without multi-arch image.
  Suggested fix: Use Docker Buildx with `--platform linux/amd64,linux/arm64` in CI. Add `ARG TARGETARCH` to Dockerfile.static; pass `GOARCH=$TARGETARCH` to `go build`. CI: `docker buildx build --platform linux/amd64,linux/arm64 --push`. No code changes to Go source needed; CGO_ENABLED=0 already required.
  Implementation notes: (1) Edit Dockerfile.static build stage: `ARG TARGETARCH` + `ENV GOARCH=$TARGETARCH CGO_ENABLED=0`. (2) Add `.github/workflows/docker.yml` or extend existing CI with `docker buildx` step. (3) No Go source changes. (4) Test: `docker run --platform linux/arm64 <image> /iptv-tunerr probe` should print help without QEMU error. Risk: none to existing amd64 behavior.
  Risk/Scope: low | fits current scope: no (CI/build feature)
  User decision needed?: no

- Date: 2026-02-25
  Category: reliability
  Title: (superseded 2026-03-20) Smoketest persistent cache (**`IPTV_TUNERR_SMOKETEST_CACHE_FILE`**)
  Context: Full smoketest re-runs were expensive on large catalogs.
  Status: **`internal/indexer/smoketest_cache.go`** + **`FilterLiveBySmoketestWithCache`**; **`IPTV_TUNERR_SMOKETEST_CACHE_FILE`**, **`IPTV_TUNERR_SMOKETEST_CACHE_TTL`** (default **4h**); **`cmd_catalog`**, **`free_sources`**; atomic JSON save; **`smoketest_cache_test.go`**. **`/debug/runtime.json`** echoes cache keys when configured.
  Risk/Scope: n/a (historical)
  User decision needed?: no
# Opportunities

## 2026-03-21

- **Feature parity follow-ons after webhook substrate**
  - Context: the broad parity audit surfaced real missing capability families, but the first implementation slice is intentionally `PAR-001` (event/webhook substrate) because it unlocks later work without duplicating transport/state logic.
  - Next high-value follow-ons:
    - shared upstream stream fanout / session reuse
    - server-backed DVR rules/history/conflict model
    - Xtream-compatible downstream publishing
    - multi-user / entitlement scope model
    - virtual channels from owned media
    - richer active-stream analytics/control surfaces
