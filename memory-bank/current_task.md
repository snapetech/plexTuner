**Latest (2026-04-17):** **API-first zero-touch registration is now the documented default path for new users:** top-level help, `setup-doctor`, Plex connection docs, deployment docs, and the shipped supervisor examples now point new installs at `PLEX_HOST` + `PLEX_TOKEN` with `run -mode=full -register-plex=api` instead of the older DB-path-first wording. The app now also documents the validated two-DVR pattern (`general` + `sports_na`) with a shipped `k8s/iptvtunerr-supervisor-general-sports.example.json` example so the multi-guide flow is no longer tribal knowledge. Immediate next step is to commit/push this onboarding alignment together with the already-working sports DVR runtime changes.

**Latest (2026-04-17):** **A second sports-only Plex DVR is now live alongside the main lineup:** the repo now supports a strict `sports_na` lineup recipe, and the cluster sports bridge on `http://iptvtunerr-sports.plex.svc:5004` has been rebuilt/redeployed with that recipe. Live tuner validation shows `sports_na` keeping `109` channels from the provider catalog, resequenced and offset to guide numbers `10001+`, with Canadian and US sports brands first (`Sportsnet`, `TSN`, `NBC Sports`, `ESPN`, `NFL/NHL/NBA/MLB`, etc.). Plex standby registration completed cleanly via API using distinct identity `iptvtunerr-sports01`, producing device `759`, DVR `760`, and XMLTV provider `tv.plex.providers.epg.xmltv:760` while leaving the working primary DVR untouched (`757`). Immediate next step is client-side validation that Plex now surfaces both the primary guide and the sports guide cleanly and that the sports DVR plays through the same stabilized cluster playback path.


**Latest (2026-04-17):** **The six blank `plexKube` Live TV sources were not extra DVR rows at all; they came from Plex advertising six host-network connection URIs for the same PMS instance to plex.tv:** live canonical PMS state stayed clean throughout (`/livetv/dvrs` = one DVR `757`, `/media/providers` = one XMLTV provider `757`), but `https://plex.tv/api/resources` exposed `plexKube` with six connections (`enp17s0`, `wlan0`, `flannel.1`, `cni0`, `docker0`, WAN). That matched the blank-source count users saw in fresh Plex Web/Desktop sessions. The fix was server-side, not Tunerr-side: set `PreferredNetworkInterface=enp17s0` on standby PMS (live via `/:/prefs`, persisted in `../k3s/plex/deployment-kspld0.yaml` as `PLEX_PREFERENCE_6`) and restart Plex so plex.tv collapses the published graph to the canonical LAN+WAN pair. Verified after restart: plex.tv resources now show only `https://192-168-50-148...:32400` and `https://24-109-206-134...:55555`, while PMS still serves one DVR and `479` XMLTV channels. Immediate next step is client-side confirmation that the duplicate Live TV source strip collapses after refresh, not more DVR churn changes in Tunerr.

**Latest (2026-04-17):** **The cluster Plex internal-fetcher playback path is now materially faster and no longer depends on globally disabling ffmpeg:** the repo now honors `IPTV_TUNERR_HLS_RELAY_PREFER_GO=true` for transcode HLS requests too, so Plex internal `Lavf` fetchers can stay on the Go HLS relay path and still normalize through `IPTV_TUNERR_HLS_RELAY_FFMPEG_STDIN_NORMALIZE=true` instead of paying the slower direct ffmpeg-HLS-input startup cost. Live cluster validation on `iptvtunerr.plex.svc:5004` with the rebuilt image and the normal deployment envs (`HLS_RELAY_PREFER_GO=true`, stdin normalize on, internal-fetcher profile `plexsafehq`, no `IPTV_TUNERR_FFMPEG_DISABLED`) now shows `hls-relay-ffmpeg-stdin-transcode first-bytes=5600 startup=2.164s`, `first-feed-bytes ... startup=3.33s`, and a Plex-shaped `Lavf/60.16.100` fetch receiving ~64 MB in 20s with `time_starttransfer≈2.8s`. Immediate next step is real Plex-client validation against this live path rather than more tuner-side startup guessing.

**Latest (2026-04-17):** **Cluster Plex lineup and playback normalization are now aligned with the standby DVR path:** the live cluster deployment keeps the full curated lineup (`479` provider rows on standby DVR `755` after the duplicate-`TVGID` XMLTV fix), serves the localized Canadian-first order via guide-number resequencing, and now explicitly enables Plex client adaptation for the PMS internal `Lavf` fetcher lane. `../k3s/plex/iptvtunerr-deployment.yaml` now sets `IPTV_TUNERR_CLIENT_ADAPT=true`, `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY=websafe`, and `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE=copyvideomp3`, which moves standby Plex ingest off the raw Go-relay AAC path that produced `sampleRate=0` / `channels=0` in PMS. Verified live from the running cluster service: `Lavf/60.16.100` requests now log `adapt transcode=true profile="copyvideomp3" reason=ambiguous-internal-fetcher-websafe`, `ffmpeg-transcode first-bytes`, and sustained MPEG-TS output instead of raw pass-through. Remaining boundary: I proved the PMS ingest shape is corrected, but I did not complete a second browser/UI-driven `POST /livetv/dvrs/755/channels/c1/tune` proof from the same authenticated Plex client path in this session because direct manual replays of that endpoint are rejected by PMS (`400` externally, `403` anonymous loopback) unless they come from a real signed-in client flow.

# Current task

<!-- Update at session start and when focus changes. -->

**Latest (2026-04-17):** **Plex standby no longer fails at the old tuner ingest boundary, but Live TV can still spin because the playback consumer never latches onto PMS's rolling-sub session reliably:** live PMS logs now show both `c1` and `c4` tune attempts on DVR `755` successfully starting recorder/transcode sessions against `http://iptvtunerr.plex.svc:5004/stream/...`, with normalized `h264` + `mp3` streamDetail reported back from Plex Transcoder and hundreds of generated `media-%05d.ts` segments. The remaining failure is later in the PMS/client handoff: older `c1` attempts ended with `Client stopped playback`, and a newer `c4` attempt reached `Started session successfully` plus ~600 segment updates before PMS killed the rolling-sub recorder with `Recording failed. Please check your tuner or antenna.` and `Recorder: No more consumers, stopping.` I also A/B tested lower ffmpeg HLS analyze/probe/startup thresholds in the cluster deployment, but that did not materially reduce first media readiness (still ~10–12s on the internal `Lavf` lane), so that experiment was reverted instead of leaving unproven cluster drift behind. Immediate next step is to capture one fresh failing attempt from the actual Plex client and correlate the resulting playback-path requests (`index.m3u8`, DASH/decision, or client stop) against the already-corrected recorder ingest path.

**Latest (2026-04-17):** **Cluster Plex localization/playback path is now corrected end-to-end for the standby DVR:** the repo now defaults unsafe ffmpeg HLS flags `http_persistent` and `live_start_index` to opt-in instead of opt-out, the cluster deployment explicitly forces the fast Go HLS relay on the first request (`IPTV_TUNERR_HLS_RELAY_PREFER_GO=true`), and the lineup path now supports `IPTV_TUNERR_GUIDE_NUMBER_RESEQUENCE=true` so curated lineup order becomes the visible low-to-high channel-number order that Plex actually surfaces. The live `../k3s/plex/iptvtunerr-deployment.yaml` now runs `locals_first` + `na_en`/`ca_west`, resequences guide numbers from `1`, disables the unsupported ffmpeg HLS options, and prefers Go relay immediately. Verified live on Plex standby DVR `755`: `/tv.plex.providers.epg.xmltv:755/lineups/dvr/channels` now starts with Canadian locals in slots `1..21` instead of the old BEIN/Arabic block, and first-request stream startup on `CA| CTV 2 VICTORIA HD` now skips the dead remux attempt and returns stream bytes in about `3s` from the cluster service. Verified live on Plex standby DVR `755`: `/tv.plex.providers.epg.xmltv:755/lineups/dvr/channels` now starts with Canadian locals in slots `1..21` instead of the old BEIN/Arabic block, and the final XMLTV duplicate-row fix restored the full `479` provider rows instead of the earlier `382`/`97 skipped` partial import.

**Latest (2026-04-17):** **The post-release Gitleaks failure was a false positive in memory-bank wording and is now fixed:** GitHub Actions flagged `generic-api-key` on `memory-bank/current_task.md` line 24, but the match came from token-shaped phrasing in a structural note about `server.go`, not from any real credential. Reworded that line to plain English (`diagnostic and operator glue code`), pushed follow-up commit `051d192`, and confirmed the new Gitleaks run succeeded. No runtime/config behavior changed in this pass.

**Latest (2026-04-17):** **Cluster IPTV Tunerr is now fully imported into Plex standby with the full 463-channel provider view restored:** the live `iptvtunerr` deployment on `kspld0` is healthy, Plex standby DVR `755` / device `754` now activates successfully with a single full channel-map PUT, and `/tv.plex.providers.epg.xmltv:755/lineups/dvr/channels` now returns all `463` provider rows instead of the earlier `63`-row collapse. Root cause turned out to be two Plex-side constraints interacting with our previous behavior: PMS effectively treated batched mapping writes as replacement-like provider state, and it also rejected oversized full activation URLs until XMLTV `channelKey` values were shortened aggressively. The live fix is now: stable guide cache (`IPTV_TUNERR_PROVIDER_EPG_DISK_CACHE`), numeric guide display names in XMLTV, very short Plex-safe channel IDs (`c` + base36 guide numbers where possible), and one-shot channel-map activation in `internal/plex/dvr.go`. Verified live with `plex-dvr-repair` (`url_len=30571`, `status=200`), Plex DVR `755` showing `463` enabled mappings, provider channels endpoint returning `463` rows, and provider discover hub returning populated items again. Remaining follow-up is documentation/state cleanup only, not runtime repair.


**Latest (2026-04-17):** **Cluster rollout of IPTV Tunerr is complete and verified at the service layer:** the sibling `../k3s` repo now has a fresh `plex/iptvtunerr-deployment.yaml` + `plex/iptvtunerr-service.yaml`, `plex/README.md` documents the build/import/secret/apply flow, and `docs/DOCS-AUDIT.md` now lists `iptvtunerr` in the `plex` namespace. The live deployment is pinned to `kspld0`, overrides `IPTV_TUNERR_BASE_URL` to `http://iptvtunerr.plex.svc:5004`, exposes the dedicated deck at `http://iptvtunerr.plex.svc:48879`, and forces `IPTV_TUNERR_LIVE_ONLY=true` so readiness is not blocked by VOD/series indexing. Verified live via the deployed service: `discover.json` returns the cluster BaseURL/LineupURL, `lineup_status.json` is `LineupReady=true`, `readyz` reports `479` channels and `status=ready`, `lineup.json` returns `479` rows, `guide.xml` serves real XMLTV, and authenticated `deck/setup-doctor.json` reports the same cluster URLs. Follow-on fix in progress: the cluster deployment is being tightened to advertise a stable HDHomeRun identity (`iptvtunerr01` / `IPTV Tunerr`) before Plex registration, instead of leaking the pod name into discover.json. Remaining boundary: the configured Plex target from `.env` currently refuses TCP connections, and the in-cluster `deployment/plex` is still scaled to `0`, so a real DVR registration/provision proof against PMS could not be completed in this session.

**Latest (2026-04-17):** **The last direct advanced actions in simple mode are now gated too:** `internal/webui/deck.js` now routes the remaining diagnostics-history and Plex-harvest direct actions back through `Settings` unless advanced surfaces are enabled, so the simple deck path no longer has stray one-click escapes into advanced capture/harvest behavior. Guide and stream repair remain intentionally first-class; the advanced features still exist as demoted hints and opt-in actions. Verification is green via `go test ./internal/webui ./internal/tuner ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are now mostly product-policy decisions: whether any advanced summary cards should be removed entirely from simple mode now that they are visibly labeled as advanced, and whether any more `server.go` breakup is worth the review cost.

**Latest (2026-04-17):** **The default deck path now demotes advanced workflows even when their summary cards remain visible:** `internal/webui/deck.js` now keeps guide and stream repair first-class, but diagnostics capture, migration/OIDC cutover, Autopilot reset, Ghost Hunter recovery, and programming harvest workflow actions only render as direct workflow/action buttons when advanced surfaces are enabled. In the default mode those cards now point back to Settings instead of launching advanced workflows straight from the main journey. Verification is green via `go test ./internal/webui ./internal/tuner ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are now mostly product-policy decisions: whether these advanced workflow summary cards should disappear entirely in simple mode, and whether any more `server.go` breakup is worth the review cost.

**Latest (2026-04-17):** **The deck now enforces the advanced/simple split at the navigation layer, not just in settings copy and raw-endpoint visibility:** the `ops` lane is now marked as advanced in `internal/webui/index.html`, `internal/webui/deck.js` hides that nav button and section unless `Show Advanced Surfaces` is enabled, mode jumps to `ops` fall back to `settings` when advanced surfaces are hidden, and overview cards now send the user to Settings instead of a hidden advanced lane when appropriate. Regression coverage in `internal/webui/webui_test.go` now checks for the advanced-nav marker and the `ops`-mode gate in the deck bundle. Verification is green via `go test ./internal/webui ./internal/tuner ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are mostly product-policy decisions now: whether advanced workflows should stay in the shipped deck at all, and whether any more `server.go` breakup is worth the review cost.

**Latest (2026-04-15):** **The deck now genuinely hides the advanced raw/workflow surface by default instead of only renaming it:** `internal/webui/deck.js` now treats the raw endpoint atlas and workflow-heavy operator categories as opt-in, persists that preference locally, shows only the narrower first-run/runtime/guide/routing/programming endpoint subset by default, and replaces the old always-on advanced settings card with an explicit “Advanced surfaces hidden” card until the user enables them in Deck Preferences. Focused regression coverage in `internal/webui/webui_test.go` now checks for that toggle and hidden-default copy. Verification is green via `go test ./internal/webui ./internal/tuner ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are now mostly policy/packaging decisions rather than mechanical cleanup: whether any advanced surfaces should move out of the shipped default deck entirely, and whether more `server.go` decomposition is still worth the review cost.

**Latest (2026-04-14):** **The dedicated deck now presents a readiness-first default lane instead of leading with operator framing:** `internal/webui/index.html` and `internal/webui/deck.js` were updated so the nav, hero, overview, fast lanes, and settings copy now start with setup readiness, exact tuner/guide/deck URLs, and the shortest path to connecting Plex/Emby/Jellyfin. Advanced operator/runtime surfaces are still present, but they are explicitly demoted to advanced lanes and settings/index language instead of defining the product tone on first load. Verification is green via `go test ./internal/webui ./internal/tuner ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are still backlog: any deeper persona split beyond copy/lane ordering, and deciding whether more `server.go` decomposition is worth the review cost now that the obvious route clusters are gone.

**Latest (2026-04-14):** **The diagnostics and recorder/operator tail has now been split and verified:** diagnostic harness discovery/execution helpers plus recorder/history/rules endpoints, HLS mux demo helpers, mux segment decode action, and `device.xml` now live in `internal/tuner/server_diagnostics_recordings.go`. That leaves `internal/tuner/server.go` down to the core tuner runtime/bootstrap, lineup shaping, virtual-channel shared types/helpers, and the remaining non-route server glue instead of also carrying the operator diagnostics/recordings tail. Verification is green via `go test ./internal/tuner ./internal/webui ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are still backlog: deeper persona split across docs/deck/runtime, deciding whether any further `server.go` breakup is worth the review cost now that the obvious route clusters are gone, and any final `webui.go` cleanup if we want even smaller ownership slices.

**Latest (2026-04-14):** **The remaining virtual-channel transport block is now split and verified:** the slate endpoint, branded/plain virtual stream handlers, recovery relay logic, recovery-state persistence, and branding/render helpers now live in `internal/tuner/server_virtual_channel_streams.go`. That leaves `internal/tuner/server.go` focused on the tuner runtime/bootstrap, lineup shaping, the remaining diagnostic and operator glue code, and the non-virtual server helpers instead of also carrying the virtual playback transport stack. Verification is green via `go test ./internal/tuner ./internal/webui ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are still backlog: deeper persona split across docs/deck/runtime, any further non-virtual `server.go` decomposition if review pressure justifies it, and any final `webui.go` cleanup if we want even smaller ownership slices.

**Latest (2026-04-13):** **`internal/tuner/server.go` has been split again and verified:** the virtual-channel management/reporting surface now lives in `internal/tuner/server_virtual_channels.go`, including the rules, preview, schedule, detail, report, guide, and M3U handlers plus the mutation helpers that back them. That leaves `server.go` focused more tightly on runtime bootstrapping, lineup shaping, the remaining branded-stream/recovery transport pieces, and lower-level diagnostics helpers instead of also carrying the operator-facing virtual-channel control surface. Verification is green via `go test ./internal/tuner ./internal/webui ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are still backlog: deeper persona split across docs/deck/runtime, more decomposition in the remaining branded-stream/recovery portions of `server.go`, and any final `webui.go` cleanup if we want even smaller ownership slices.

**Latest (2026-04-13):** **`internal/tuner/server.go` has been split again and verified:** the programming-manager/report/import surfaces plus their harvest/import helper logic now live in `internal/tuner/server_programming.go`, joining the earlier `internal/tuner/server_status_reports.go` and `internal/tuner/server_operator_workflows.go` extractions. That leaves `server.go` focused more tightly on runtime bootstrapping, lineup shaping, the remaining virtual-channel/runtime handlers, and lower-level diagnostics helpers instead of also carrying the full programming-manager block. Verification is green via `go test ./internal/tuner ./internal/webui ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are still backlog: deeper persona split across docs/deck/runtime, more decomposition in the remaining virtual-channel/runtime sections of `server.go`, and any final `webui.go` cleanup if we want even smaller ownership slices.

**Latest (2026-04-13):** **`internal/tuner/server.go` has been split again and verified:** the operator workflow/report surfaces, safe operator action handlers, and programming-harvest request plumbing now live in `internal/tuner/server_operator_workflows.go`, alongside the earlier `internal/tuner/server_status_reports.go` split. That leaves `server.go` focused more tightly on lineup shaping, runtime bootstrapping, programming/virtual-channel endpoints, and lower-level diagnostics helpers instead of also carrying the whole operator control-plane layer. Verification is green via `go test ./internal/tuner ./internal/webui ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are still backlog: deeper persona split across docs/deck/runtime, more decomposition in the remaining programming/virtual-channel sections of `server.go`, and any final `webui.go` cleanup if we want even smaller ownership slices.

**Latest (2026-04-13):** **`internal/webui/webui.go` has now been split into route/concern slices and verified:** auth/session/security helpers now live in `internal/webui/webui_auth.go`, the migration/OIDC workflow handlers plus their reporting helpers now live in `internal/webui/webui_migration.go`, and the earlier setup lane remains in `internal/webui/webui_setup.go`. This keeps behavior unchanged but drops `webui.go` itself to the dashboard listener, deck state/settings, proxy/index/assets, and shared persistence/runtime replay helpers, which makes the remaining breakup work much more obvious. Verification is green via `go test ./internal/webui ./cmd/iptv-tunerr ./internal/tuner` and full `./scripts/verify`. Remaining larger audit themes are still backlog: deeper persona split across docs/deck/runtime, more `internal/tuner/server.go` decomposition, and a final `webui.go` split for proxy/assets/state if needed.

**Latest (2026-04-13):** **Initial `internal/tuner/server.go` breakup is now in place and verified:** the health/readiness handlers plus the read-only operator/report surfaces up through `/ops/actions/status.json` were moved out of the monolith into `internal/tuner/server_status_reports.go`. This does not change route behavior; it is the first maintainability slice from the audit’s “monolithic core files” concern and gives the product-surface cleanup a matching code-structure step. Combined with the earlier `internal/setupdoctor` extraction and `internal/webui/webui_setup.go` split, the repo now has concrete movement on both product boundary and handler ownership. Verification is green via `go test ./internal/tuner ./internal/webui ./cmd/iptv-tunerr` and full `./scripts/verify`. Remaining larger audit themes are still open backlog: deeper persona split across docs/deck/runtime, more `server.go` and `webui.go` decomposition, and broader deck simplification.

**Latest (2026-04-13):** **Second-phase product-surface cleanup is now landed and verified:** the first-run contract is no longer CLI-only. `internal/setupdoctor/` now owns the reusable setup report logic used by `iptv-tunerr setup-doctor`, the dedicated Control Deck exposes the same report at `/deck/setup-doctor.json`, and the deck overview/settings surfaces now show first-run readiness directly instead of only operator/runtime diagnostics. Top-level help no longer dumps the full `Lab/ops` surface by default; the default help now stays on `Core` / `Guide/EPG` / `VOD` and points advanced users to `iptv-tunerr --all-commands`. This pass also started breaking up the web UI monolith by moving the setup-doctor handler into `internal/webui/webui_setup.go`. Verification is green via `go test ./internal/setupdoctor ./cmd/iptv-tunerr ./internal/webui` and full `./scripts/verify`. Remaining larger audit themes are still backlog items: deeper persona splitting across docs/deck/runtime and broader `internal/tuner/server.go` / `internal/webui/webui.go` decomposition.

**Latest (2026-04-13):** **Reviewed onboarding cleanup is now merged into current `main` without rolling back newer repo work:** imported the user-provided audit scope from `~/Downloads/IPTVTunerr_audit_and_cleanup.md` and `~/Downloads/iptvtunerr-main-reviewed.zip`, confirmed this checkout was already in sync with `origin/main`, and merged only the concrete onboarding/product-surface slice instead of replacing the tree with the older reviewed snapshot. Shipped in this pass: new `setup-doctor` first-run command with focused tests, new `.env.minimal.example`, top-level CLI help that shows the actual first-run path, and onboarding doc updates in `README.md`, `.env.example`, and `docs/reference/cli-and-env-reference.md`. Out of scope for this pass: broad persona splitting, deck/UI redesign, or large structural file splits; those are now filed in `memory-bank/opportunities.md` from the audit notes. Verification is green via focused `go test ./cmd/iptv-tunerr -run 'Test(BuildSetupDoctorReportReady|BuildSetupDoctorReportNotReadyWithoutSourceOrBaseURL|BuildSetupDoctorReportWarnsOnLocalhostBaseURL|BuildSetupDoctorReportBaseURLOverrideDrivesDeckURL|HostLooksLocalOnly|UsageTextIncludesCommands)$'` and full `./scripts/verify`.

**Latest (2026-03-23):** **Release hardening pass closed the open Dependabot, CodeQL, and licensing items:** `.github/workflows/docker.yml` now uses `docker/setup-buildx-action@v4`; `internal/tuner/server.go` sanitizes evidence/diagnostics run IDs before filesystem joins so operator-supplied or env-supplied identifiers cannot escape the diagnostics root; and `internal/webui/webui.go` no longer derives fallback deck session tokens from configured credentials when `crypto/rand` fails. The repo licensing language is now explicitly dual-model via `LICENSE`, new `LICENSE-COMMERCIAL.md`, `README.md`, and `docs/CHANGELOG.md` as AGPL-3.0-or-later or commercial. Regression coverage was added in `internal/tuner/server_test.go`, and verification is green via `go test ./internal/tuner ./internal/webui` and full `./scripts/verify`. Immediate next step if we keep going on release prep: audit GitHub security alerts/workflow dependency drift again after CodeQL re-runs and decide whether to commit/tag this hardening pass separately or fold it into the next release batch.

**Latest (2026-03-22):** **The README opening is now being tightened into a real product intro instead of a migration-heavy wall of text:** the top section was rewritten again so it pulls custom EPG generation/repair, staged migration, and the owned-media / indie-broadcaster lane into a shorter narrative flow. This pass is specifically about discoverability at the top of the repo, not adding new runtime features. Immediate next step if we keep going on docs: tighten later README sections so they do not repeat the same migration/deck/station-ops story at lower detail.

**Latest (2026-03-22):** **Operator deck accessibility/affordance pass is now in place and documented:** `internal/webui/index.html`, `internal/webui/deck.css`, and `internal/webui/deck.js` now include skip-to-content, labeled controls, stronger focus-visible states, `aria-pressed` mode buttons, `aria-live` feedback/status surfaces, proper modal dialog semantics, focus restoration/trapping, and an in-deck modal editor for virtual-station branding/recovery fields instead of raw `window.prompt` popups. `internal/webui/webui_test.go` now pins the main accessibility affordances (`Skip to main content`, dialog semantics, modal editor presence, nav pressed-state updates). Docs were updated in `README.md` and `docs/features.md`. Verification: `node -c internal/webui/deck.js`, `go test ./internal/webui ./cmd/iptv-tunerr ./internal/tuner ./internal/virtualchannels`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`. Immediate next step if we keep going on this lane: broader UX cleanup on the legacy `/ui/` shell or deeper screen-reader audit of the dynamically generated cards/timelines.

**Latest (2026-03-22):** **Legacy `/ui/` audit is now closed and reflected in product behavior/docs:** the old tuner-port UI is still present but only as a compatibility shell (`/ui/`, `/ui/guide/`, `/ui/guide-preview.json`) implemented in `internal/tuner/operator_ui.go`; it is no longer treated as a parallel operator plane. The served HTML now explicitly says the dedicated Control Deck is primary and links back to it using the configured `IPTV_TUNERR_WEBUI_PORT`, with regression coverage in `internal/tuner/server_test.go`. `README.md` and `docs/features.md` now frame `/ui/` as compatibility/read-only instead of a first-class operator surface. Verification: `go test ./internal/webui ./cmd/iptv-tunerr ./internal/tuner ./internal/virtualchannels`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`. Immediate next step if we keep going on this lane: formal deprecation messaging in release notes or eventual removal once the deck fully supersedes the remaining guide-preview use case.

**Latest (2026-03-22):** **Jellyfin migration audit status semantics now match reality instead of overstating convergence:** after fixing the live `jellyfin-test` mount/path problem and successfully applying the missing library definitions, the remaining live state was no longer “missing libraries”; it was “libraries exist but are empty / lagging the Plex source.” `internal/livetvbundle/bundle.go` now treats `MissingLibraries`, `LaggingLibraries`, `TitleLaggingLibraries`, and `EmptyLibraries` as non-converged library parity, so `AuditBundleTargets(...)` and `migration-rollout-audit` only return `converged` when Live TV is indexed and the bundled library surface is actually present and populated enough to pass the current parity checks. `internal/livetvbundle/bundle_test.go` and `internal/webui/webui_test.go` were updated so the deck/CLI agree on the stricter status. Live rerun against the real Jellyfin target now reports `ready_to_apply` with explicit reasons (`count_lagging_libraries`, `title_lagging_libraries`, `empty_libraries`) instead of the old false `converged`. Verification: focused `go test ./internal/livetvbundle ./internal/emby`, affected-package `go test ./cmd/iptv-tunerr ./internal/webui ./internal/migrationident ./internal/keycloak`, live `migration-rollout-audit -targets jellyfin -summary`, and full `./scripts/verify`.

**Latest (2026-03-22):** **The two live-release migration blockers are now addressed against the real cluster:** Jellyfin Live TV audit no longer fails closed just because `10.11.x` lacks `GET /LiveTv/TunerHosts` and `GET /LiveTv/ListingProviders`. Live probing found the exact Jellyfin read path at `GET /System/Configuration/livetv`, so `internal/emby/register.go` and `internal/livetvbundle/bundle.go` now switch to that endpoint when the list endpoints return `405`, preserving exact tuner/listing parity instead of best-effort heuristics. On the IdP side, `internal/keycloak/keycloak.go`, `internal/migrationident/bundle.go`, `cmd/iptv-tunerr/cmd_identity_migration.go`, and `internal/webui/webui.go` now support Keycloak admin username/password credentials (`IPTV_TUNERR_KEYCLOAK_USER` / `IPTV_TUNERR_KEYCLOAK_PASSWORD`) and mint a fresh `admin-cli` token per diff/audit/apply run instead of depending on a brittle static bearer token. Live validation from the real k3s-backed `.env` now succeeds for:
- `migration-rollout-audit -targets jellyfin -summary`
- `identity-migration-oidc-audit -targets keycloak -summary`
- full `./scripts/verify`

**Follow-up live fix on the same lane:** the remaining Jellyfin “missing libraries” migration gap turned out to be a test-target mount problem, not another Tunerr bug. The migration bundle targeted `/media/...` library paths, but the `jellyfin-test` deployment had only `/config` and `/cache` mounted and the node did not have those `/media/...` directories. After creating the expected `/media/...` tree on the node, patching the `jellyfin-test` deployment to mount host `/media` into the container, and rerunning `library-migration-rollout -targets jellyfin -apply`, the live Jellyfin audit no longer reports missing bundled libraries. It now correctly reports `status: ready_to_apply` because the target still has content-parity gaps (`lagging` / `title_lagging` / `empty` libraries) even though the definitions are present.

**Residual caveat:** the Jellyfin exact-diff fix depends on `System/Configuration/livetv`, which is present on the tested `10.11.6` target. If Jellyfin changes or restricts that endpoint in a future release, Tunerr should fail loudly rather than guessing. Immediate next step if we keep going on this lane: decide whether content-parity signals should eventually split beyond the current `ready_to_apply` bucket into a stronger “definition-ready but content-not-synced” status, because the current audit is now truthful but still coarse.

**Latest (2026-03-22):** **Live cluster credential fill + real migration validation is now in place, and it exposed two release-relevant integration limits:** `.env` is now populated from the current k3s cluster for live Plex, Emby, Jellyfin, Authentik, and a disposable Keycloak test target, with bundle artifacts written under `.diag/live-migration/` (`migration-bundle.json`, `identity-bundle.json`, `identity-oidc-plan.json`). Real live results:
- provider/tuner smoke is healthy against the live `kspld0:5004` surface
- `migration-rollout-audit -in ... -targets emby -summary` succeeds against live Emby
- `identity-migration-audit -in ... -summary` succeeds against live Emby + Jellyfin and reports 8 missing users on each
- live OIDC audits succeed against Authentik and against Keycloak when a fresh admin token is minted inline
- fixed one real CLI bug on the way: `identity-migration-oidc-audit` was incorrectly reusing the Emby/Jelly target filter and rejecting `keycloak`; `cmd_identity_migration.go` now has an OIDC-specific target parser with regression coverage

**Live-tested limits found in this pass:**
- Jellyfin `10.11.6` returns `405 Method Not Allowed` on `GET /LiveTv/TunerHosts` and `GET /LiveTv/ListingProviders`, but its `GET /System/Configuration/livetv` endpoint exposes the same exact tuner/listing state and Tunerr now uses that path for audit
- static `IPTV_TUNERR_KEYCLOAK_TOKEN` values are still short-lived in the disposable test realm, but Tunerr now supports `IPTV_TUNERR_KEYCLOAK_USER` / `IPTV_TUNERR_KEYCLOAK_PASSWORD` and mints a fresh token per run when those credentials are configured

**Immediate next step if we keep going on this lane:** add one or two disposable-target live `apply` smokes so the release proof moves beyond audit-only validation.

**Latest (2026-03-22):** **Release audit pass: the deck now exposes shared live-session reuse directly and the canonical feature docs finally mention the migration lanes:** `internal/webui/deck.js` now surfaces `/debug/shared-relays.json` as a Routing card so shared-session reuse/replay is visible in the operator plane instead of only existing as a debug endpoint, and the asset guardrail in `internal/webui/webui_test.go` now pins that endpoint too. `docs/features.md` and `README.md` were updated to explicitly call out media-server migration, identity migration, OIDC/IdP workflows, and the shared-relay operator surface. Immediate next step if we keep polishing release docs: a tighter release-summary section that groups migration/operator changes without forcing readers through the long README intro.

**Latest (2026-03-22):** **Broad repo-confidence pass: low-coverage `cmd/iptv-tunerr` startup helpers and free-source merge paths now have direct tests instead of sitting at zero:** `cmd/iptv-tunerr/cmd_runtime_test.go` now covers `loadRuntimeLiveChannels`, `loadRuntimeCatalog`, and the newer runtime-snapshot/server propagation for virtual recovery state. `cmd/iptv-tunerr/cmd_util_test.go` now covers `parseCSV` and `hostPortFromBaseURL`, and `cmd/iptv-tunerr/free_sources_test.go` now covers `freeSourceCacheDir`, `urlCacheKey`, `maxPaidGuideNumber`, `assignFreeGuideNumbers`, plus supplement/merge/full free-source application behavior. The command package coverage moved from the low 31% range to `34.3%`, and the focused release suite plus build are green. Immediate next step if we keep pushing broad confidence: target one more cheap command-layer island such as `fetchRawCached` / `applyIptvOrgFilter`, rather than reopening stable station-ops/runtime code.

**Latest (2026-03-22):** **That command-layer confidence pass now covers the free-source cache/filter path too:** `cmd/iptv-tunerr/free_sources_test.go` now exercises `fetchRawCached` via a real cache hit, `loadIptvOrgFilter` via seeded on-disk cache files for the iptv-org blocklist/channels payloads, and `applyIptvOrgFilter` for both tag-only and drop modes. The `cmd/iptv-tunerr` package coverage moved again to `36.3%`, with `fetchRawCached`, `loadIptvOrgFilter`, and `applyIptvOrgFilter` no longer at zero. Immediate next step if we keep pushing repo-wide confidence: either cover `fetchFreeSources`/`applyFreeSources` dispatch directly or move to other 0%-heavy command handlers like `cmd_runtime_register.go`, not the already-stable station runtime.

**Latest (2026-03-22):** **The free-source fetch/dispatch island is covered now too, and the obvious pure helpers in `cmd_runtime_register.go` are no longer blind:** `cmd/iptv-tunerr/free_sources_test.go` now covers `fetchFreeSources` for the no-URL path and for a cached-filtered M3U ingest path, plus the `applyFreeSources` dispatcher. `cmd/iptv-tunerr/cmd_runtime_register_test.go` now covers `guideURLForBase`, `streamURLForBase`, `minInt`, and `maxInt`. The command package coverage moved again to `36.5%`, and the remaining big 0% areas are now heavier command handlers like `registerRunPlex`, `registerRunMediaServers`, and `handleRun`, not cheap helper islands. Immediate next step if we keep pushing broad confidence: decide whether to build heavier integration-style tests around `cmd_runtime_register.go` or stop at this safer boundary for release.

**Latest (2026-03-22):** **One more safe command-layer pass covered the remaining guard/policy branches before true integration work:** `cmd/iptv-tunerr/cmd_runtime_register_test.go` now covers `applyRegistrationRecipe` for off and healthy modes, the easy-mode early-return branch in `registerRunPlex`, and the missing-credentials branch in `registerRunMediaServers`. `cmd/iptv-tunerr/cmd_runtime_test.go` now also covers disabled-mode `maybeOpenEpgStore` and `startDedicatedWebUI`. The command package coverage moved to `37.7%`. Immediate next step if we keep pushing: only the genuinely heavier handlers remain (`handleServe`, `handleRun`, deeper Plex/Emby registration flows), so additional gains now mean integration-style test scaffolding rather than cheap helper coverage.

**Latest (2026-03-22):** **The command baseline moved again with real file-backed runtime behavior:** `cmd/iptv-tunerr/cmd_runtime_test.go` now opens a real temp EPG SQLite file through `maybeOpenEpgStore`, and `cmd/iptv-tunerr/cmd_runtime_register_test.go` now covers the `registerRunPlex` register-only/no-live branch. The command package coverage is now `38.2%`, with `maybeOpenEpgStore` at `80.0%` and `registerRunPlex` up to `33.8%`. Immediate next step if we keep pushing broad confidence: the remaining major gaps now truly require bigger harnesses around `handleServe`, `handleRun`, or VOD mount/WebDAV handlers rather than another batch of isolated unit tests.

**Latest (2026-03-22):** **The first subprocess-style integration harness is in, so blocking VOD command handlers are no longer untestable:** `cmd/iptv-tunerr/cmd_vod_integration_test.go` now spawns helper subprocesses to exercise `handleVODWebDAV` success/failure and `handleMount` failure paths without refactoring those `os.Exit`/blocking handlers. That moved `cmd/iptv-tunerr` to `38.5%`, with `handleMount` at `38.1%` and `handleVODWebDAV` at `27.3%`. Immediate next step if we keep pushing: use the same subprocess pattern for `handleServe`/`handleRun`, or stop here and treat the remaining gaps as a larger harness/refactor project rather than another “cheap confidence” pass.

**Latest (2026-03-22):** **That subprocess harness now reaches the real runtime entrypoints too:** `cmd/iptv-tunerr/cmd_runtime_integration_test.go` now uses helper subprocesses for `handleRun` register-only success/failure and `handleServe` startup against a real temp catalog. The `handleServe` helper now shuts down via `SIGTERM` so coverage counters flush cleanly, which moved `handleServe` to `52.4%` and the overall `cmd/iptv-tunerr` package to `40.0%`. Immediate next step if we keep pushing broad confidence: the remaining command gap is mostly deeper/long-lived runtime behavior inside `handleRun` and the `main()` dispatch path, which would need a larger subprocess/CLI harness rather than another small test file.

**Latest (2026-03-22):** **The CLI subprocess harness now reaches the actual `main()` dispatch and a deeper `handleRun` refresh path:** `cmd/iptv-tunerr/main_integration_test.go` now drives `main()` through the real `run` command in subprocess mode, both with an existing catalog and with direct-M3U refresh from env. `cmd/iptv-tunerr/cmd_runtime_integration_test.go` now also covers the `handleRun` refresh-from-M3U register-only path. That moved `handleRun` to `56.4%`, `main()` to `60.0%`, and the overall `cmd/iptv-tunerr` package to `41.1%`. Immediate next step if we keep pushing: the remaining command gaps are now the harder corners like long-lived serve/runtime loops, more registration variants, and low-value CLI error exits rather than the main success paths.

**Latest (2026-03-22):** **Top-level CLI dispatch is now fully covered too:** the `main()` subprocess harness in `cmd/iptv-tunerr/main_integration_test.go` now covers `run`, `index`, `version`, `--help`, no-args usage, and unknown-command exits. That moved `main()` to `100.0%` and the `cmd/iptv-tunerr` package to `41.6%`. Immediate next step if we keep pushing coverage further: the remaining gaps are mostly lower-value or more brittle paths such as long-lived serve/runtime loops after startup, deeper registration combinations, and OS-specific VOD mount success paths.

**Latest (2026-03-22):** **Docs/readme audit completed for the station-ops/runtime slice:** `README.md` now calls out station report/recovery surfaces, branded stream publishing, deck-side branding/recovery controls, fallback-chain recovery, and persisted virtual recovery history. `docs/reference/virtual-channel-stations.md` now describes `bug_*`, `banner_text`, and `theme_color` as active branded-stream/slate behavior instead of “planned future metadata”, and `docs/index.md` now frames that reference as station-operations/runtime behavior rather than only metadata schema. Immediate next step if we keep polishing docs: update changelog/tester release notes with the same station-ops language, but the main product/readme/reference surfaces are now aligned.

**Latest (2026-03-22):** **The deck now persists explicit target-specific OIDC apply status instead of making the modal infer failures from missing rows:** `internal/webui/webui.go` now records `target_statuses` for each requested IdP target on every deck-side OIDC apply, including `applied`, `failed`, `validation_failed`, and `not_reached` states plus failure phase/error when known. `internal/webui/deck.js` now reads that persisted structure for modal history rows, and `internal/webui/webui_test.go` now pins the target-status map and provider-failure behavior. Immediate next step if we keep polishing this lane: add provider-native onboarding/result URLs or user lists into the persisted target status for even deeper operator drill-down.

**Latest (2026-03-22):** **The deck OIDC workflow modal now drills into partial IdP runs instead of only showing one compact summary line:** `internal/webui/deck.js` now expands each recent OIDC apply entry into per-target outcome rows inside the modal history, so partial Keycloak/Authentik runs make it obvious which target was applied and which target was never reached before failure. `internal/webui/deck.css` adds a small target-row layout and `internal/webui/webui_test.go` now pins the modal target-detail strings/classes too. Immediate next step if we keep pushing this lane: persist richer target-specific failure reasons from the backend instead of inferring `not reached` only from missing result rows.

**Latest (2026-03-22):** **The deck OIDC workflow modal now exposes the same filtered success/failure history lane as the summary card:** `internal/webui/deck.js` now injects recent OIDC apply history directly into the OIDC workflow modal, reusing the existing `all / success / failed` filter buttons and success/failure badges instead of forcing operators back to the summary card to inspect IdP push history. `internal/webui/deck.css` now gives that modal history block an explicit grid wrapper, and the asset guardrail in `internal/webui/webui_test.go` now pins the modal-history strings/classes too. Immediate next step if we keep pushing this lane: surface more structured per-target failure detail inside the modal/history rows instead of only the compact summary string.

**Latest (2026-03-22):** **The deck OIDC workflow now has real success/failure history controls instead of one flat recent-runs line:** `internal/webui/deck.js` now adds `all / success / failed` filtering and visual badges to the `OIDC recent applies` card, with a small stylesheet extension in `internal/webui/deck.css` to make failed vs successful IdP runs scan quickly. Focused verification is green (`node -c internal/webui/deck.js`, `go test ./internal/webui ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: expose these same OIDC history controls inside the workflow modal and add per-target visual badges there too.

**Latest (2026-03-22):** **The deck OIDC workflow history now keeps failed apply attempts too, not just successes:** `internal/webui/webui.go` now records structured `oidc_migration_apply` activity entries for validation and provider-apply failures, reusing the same history lane as successful IdP pushes. `internal/webui/deck.js` now shows those failures in the `OIDC recent applies` summaries with `phase=` and `error=` context, so a bad Keycloak/Authentik run no longer disappears once the transient JSON error is dismissed. Focused verification is green (`go test ./internal/webui ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: add per-target success/failure badges and filtering in the deck workflow instead of only flat text summaries.

**Latest (2026-03-22):** **Virtual-channel live recovery now survives more than one bad source in the same session and reports chain exhaustion explicitly:** `internal/tuner/server.go` no longer treats the first successful filler cutover as the end of the recovery lane. The recoverable relay now walks the ordered fallback chain again if an earlier rescue source later stalls or hard-errors, closes the replaced body before swapping, and records the actual source-to-fallback hop for each recovery event. When the chain runs out, recovery now records explicit `live-stall-timeout-exhausted` / `live-read-error-exhausted` events instead of failing as an opaque timeout symptom. Focused verification is green (`go test ./internal/tuner -run 'TestServer_virtualChannelStream(FallsBackDuringLiveStall|LiveStallSkipsBrokenFirstFallback|FallsBackAgainAfterFallbackStalls|ReportsRecoveryExhaustion)$' -v`, `node -c internal/webui/deck.js`, plus the focused tuner/webui/virtualchannels/cmd suite and build); immediate next step if we keep pushing this lane: move beyond transport/stall recovery into repeated decoded-media health analysis instead of only walking the fallback chain on byte-level failure.

**Latest (2026-03-22):** **The virtual station report now summarizes recovery posture instead of only dumping raw recent events:** `internal/tuner/server.go` now adds `recovery_events`, `recovery_exhausted`, and `last_recovery_reason` to `/virtual-channels/report.json`, and `internal/webui/deck.js` uses that to make station cards distinguish normal recent recoveries from truly exhausted channels. Focused verification is green (`go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `node -c internal/webui/deck.js`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: add decoded-media health signals into that same summary instead of only transport/stall/exhaustion outcomes.

**Latest (2026-03-22):** **Virtual-channel recovery history can now survive process restarts:** `internal/tuner/server.go` now supports `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_STATE_FILE` as an optional persisted event log for recent virtual recovery events. The same recovery report and station report surfaces now reload that file on demand, so restart no longer wipes the operator’s recovery trail. Focused verification is green (`go test ./internal/tuner -run 'TestServer_(virtualRecoveryStatePersistsAcrossRestart|virtualChannelStreamReportsRecoveryExhaustion)$' -v`, plus the focused tuner/webui/virtualchannels/cmd suite and build); immediate next step if we keep pushing this lane: bring decoded-media health signals into the persisted summary instead of only transport/stall/exhaustion events.

**Latest (2026-03-22):** **The virtual recovery relay now does repeated rolling in-session media-content checks, not just one post-start sample:** `internal/tuner/server.go` now keeps probing rolling windows of active stream bytes after startup and can trigger filler when a later sampled window probes as black/silent, with a dedicated `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_MIDSTREAM_PROBE_BYTES` knob separate from the startup probe byte budget. Focused verification is green (`go test ./internal/tuner -run 'TestServer_virtualChannelStream(FallsBackOnMidstreamContentProbe|FallsBackOnLaterRollingMidstreamProbe)$' -v`, plus the focused tuner/webui/virtualchannels/cmd suite and build); immediate next step if we keep pushing this lane: move from rolling sampled windows to a fuller decoded-media health loop with stronger operator metrics if the user wants even more depth.

**Latest (2026-03-22):** **The deck OIDC workflow now keeps a short recent apply history, not only the latest run:** `internal/webui/webui.go` now exposes `summary.recent_applies` in `/deck/oidc-migration-audit.json` by reusing the persisted deck activity log, and `internal/webui/deck.js` now renders that as an `OIDC recent applies` card. Operators can now see a few recent Keycloak/Authentik cutover attempts plus their per-target delta counts from the workflow itself instead of bouncing between the modal and full activity log. Focused verification is green (`go test ./internal/webui ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: capture failure entries in the same structured history instead of only successful apply summaries.

**Latest (2026-03-22):** **The deck OIDC workflow now shows what the last IdP push changed, not just when it ran:** `internal/webui/webui.go` now stores compact per-target apply deltas in the persisted `oidc_migration_apply` activity detail and folds them back into `/deck/oidc-migration-audit.json`. `internal/webui/deck.js` now renders those deltas inside the `OIDC last apply` summary, so operators can see created-user/group counts, membership adds, metadata updates, and activation-pending counts for the last Keycloak/Authentik push from the workflow itself. Focused verification is green (`go test ./internal/webui ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: add failure/result history across more than one apply, not just the latest successful push.

**Latest (2026-03-22):** **The deck OIDC workflow now shows the most recent IdP apply result, not just readiness:** `internal/webui/webui.go` now folds the latest `oidc_migration_apply` activity entry back into `/deck/oidc-migration-audit.json` as `summary.last_apply`, and `internal/webui/deck.js` surfaces that as an `OIDC last apply` card in the workflow lane. That means the appliance can now answer both “is this IdP plan ready?” and “what did we last push?” from the same workflow surface. Focused verification is green (`go test ./internal/webui ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: add per-target apply success/failure deltas in the workflow summary instead of only the last raw apply detail.

**Latest (2026-03-22):** **The deck OIDC lane now reaches real provider onboarding options instead of a bare apply button:** `internal/webui/webui.go` now lets `/deck/oidc-migration-apply.json` accept Keycloak bootstrap-password, temporary-password, execute-actions-email, client/redirect/lifespan hints, plus Authentik bootstrap-password and recovery-email options so the request shape matches the practical CLI apply surface. `internal/webui/deck.js` now prompts for those provider-specific knobs before firing the deck apply action, so the OIDC workflow card is no longer a crippled wrapper over the backend migration code. Focused verification is green (`go test ./internal/webui ./internal/migrationident ./internal/keycloak ./internal/authentik ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: persist and surface the last IdP apply result in the OIDC workflow itself instead of only the transient modal/activity trail.

**Latest (2026-03-22):** **Virtual-channel recovery now has a bounded live-session cutover loop, not only startup decisions:** `internal/tuner/server.go` now wraps filler-enabled virtual stream bodies in a recoverable relay that can perform one midstream switch to the configured fallback source when the active upstream stalls or hard-errors after startup. The new operator/runtime knob is `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC`, surfaced in `/debug/runtime.json` and the deck Settings lane, while the deck/operator plane also gained merge-safe recovery controls so changing `recovery.mode` or `recovery.black_screen_seconds` no longer wipes fallback entries. Focused verification is green (`node -c internal/webui/deck.js`, `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: move from one-switch stall/error recovery to repeated decoded-media health analysis and smarter multi-cutover session handling.

**Latest (2026-03-22):** **That live-stall watchdog is now operable from the deck too, not just an env/runtime readout:** `internal/tuner/server.go` now exposes localhost-only `POST /ops/actions/virtual-channel-live-stall`, `/ops/actions/status.json` advertises the action plus current seconds, and the deck Settings lane can save `virtual_channel_recovery_live_stall_sec` live for future virtual-channel sessions alongside shared replay bytes. Focused verification is green (`node -c internal/webui/deck.js`, `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing release readiness: either add persistent server-side storage for these runtime knobs or keep advancing the recovery engine beyond one-switch stall/error handling.

**Latest (2026-03-22):** **Those live runtime knobs now survive restarts when the dedicated web UI state file is enabled:** `internal/webui` now persists `shared_relay_replay_bytes` and `virtual_channel_recovery_live_stall_sec` inside the existing `IPTV_TUNERR_WEBUI_STATE_FILE` settings payload, and replays both localhost operator actions against the tuner on startup. That closes the “saved in the deck but lost on restart” gap without inventing a second config subsystem. Focused verification is green (`go test ./internal/webui -run 'Test(PersistStateExcludesTelemetryAndAuthSecret|LoadStateRestoresRuntimeSettings|ApplyPersistedRuntimeAction)$' -v`, plus the focused tuner/webui/virtualchannels/cmd suite and build). Immediate next step if we keep pushing release readiness: persistent storage for additional live runtime knobs, or deeper recovery logic beyond the current one-switch stall/error path.

**Latest (2026-03-22):** **Virtual-station recovery settings are now operator-editable without wiping filler definitions:** `internal/tuner/server.go` now merges partial `recovery` updates the same way branding updates already worked, with `recovery_clear` support for resetting specific fields instead of replacing the whole object. `GET /virtual-channels/report.json` now includes `recovery_mode`, `black_screen_seconds`, and fallback-entry counts, and `internal/webui/deck.js` uses that to add station-card controls for `Disable Recovery` / `Enable Filler` plus `Black Sec` editing. Focused verification is green (`node -c internal/webui/deck.js`, `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: move beyond startup-sampled recovery by introducing longer-lived live-session monitoring/cutover instead of only making the startup knobs easier to edit.

**Latest (2026-03-22):** **The OIDC/IdP lane now has a real audit surface, not just raw diff/apply commands:** `internal/migrationident` now supports `AuditOIDCPlanTargets` plus `FormatOIDCAuditSummary`, the CLI exposes `identity-migration-oidc-audit`, and the deck exposes `/deck/oidc-migration-audit.json` when `IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE` and Keycloak/Authentik envs are configured. That means the provider-agnostic OIDC plan now has the same operator-grade readiness reporting shape as the Emby/Jellyfin identity lane: missing users, missing Tunerr migration groups, missing group membership, per-target status, and a compact summary. While landing this, Keycloak activation-pending detection was tightened so existing enabled users are no longer falsely reported as onboarding-pending just because they exist. Focused verification is green (`go test ./internal/migrationident ./internal/webui ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: unify IdP audit/apply visibility with deck actions or deepen provider parity for pre-existing IdP users instead of only create-time metadata.

**Latest (2026-03-22):** **The identity/OIDC lane now has a second live provider backend and real creation-time migration metadata:** `internal/authentik` now exists alongside `internal/keycloak`, and the CLI exposes `identity-migration-authentik-diff` plus `identity-migration-authentik-apply` on the same provider-agnostic OIDC plan. Current Authentik scope matches the migration-safe Keycloak slice: create missing users by preferred username, create missing Tunerr-owned migration groups, add missing membership, optionally set a bootstrap password, and optionally trigger recovery-email onboarding. New Keycloak/Authentik-created users also now get stable Tunerr migration metadata attributes at creation time (`subject_hint`, Plex ids/uuid, group hints) so later cutover/audit work can trace provenance instead of only seeing a username/group side effect. Focused verification is green (`go test ./internal/authentik ./internal/keycloak ./internal/migrationident ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: IdP-side audit/reporting in the deck or deeper provider parity for pre-existing users instead of only create-time metadata/bootstrap.

**Latest (2026-03-22):** **The Keycloak backend now covers basic onboarding bootstrap too, not just user/group creation:** `identity-migration-keycloak-apply` can now optionally set a bootstrap password through Keycloak admin `reset-password` and trigger `execute-actions-email` for OIDC-plan users with email addresses. This keeps the provider-agnostic OIDC plan intact while finally giving the first live IdP backend an onboarding path instead of only provisioning accounts and groups. Focused verification is green (`go test ./internal/keycloak ./internal/migrationident ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: richer Keycloak required-action and attribute sync, then a second IdP target once the contract settles.

**Latest (2026-03-22):** **The identity/OIDC lane now has its first live provider backend, not just neutral planning:** `internal/keycloak` now exists as the first real IdP integration, and the CLI exposes `identity-migration-keycloak-diff` plus `identity-migration-keycloak-apply` on top of the provider-agnostic OIDC plan. Current Keycloak scope is intentionally narrow and migration-safe: create missing users by preferred username, create missing Tunerr-owned migration groups, and add missing membership from the OIDC plan. It still does not set credentials or required actions. Focused verification is green (`go test ./internal/keycloak ./internal/migrationident ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: credentials / required-action handling for Keycloak, then a second IdP target once the neutral plan proves out.

**Latest (2026-03-22):** **Identity migration now has the first real OIDC foundation slice, even though provider-specific apply is still future work:** `internal/migrationident` can now build a provider-agnostic OIDC plan from the Plex user bundle, and the CLI exposes that as `identity-migration-oidc-plan`. The plan includes stable subject hints, preferred usernames, display names, email hints, and Tunerr-owned group claims derived from Plex share state such as `tunerr:live-tv`, `tunerr:sync`, and `tunerr:plex-shared`. This deliberately avoids hard-coding Authentik/Keycloak/Caddy API assumptions into the migration core while still giving the repo a concrete contract for later IdP integration. Focused verification is green (`go test ./internal/migrationident ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: pick the first real IdP target and implement diff/apply against that plan instead of only exporting it.

**Latest (2026-03-22):** **Station Ops now has the first operator-visible runtime loop instead of invisible fallback magic:** virtual-channel recovery decisions are now recorded in-memory and exposed through `/virtual-channels/recovery-report.json`, while per-channel detail reports include recent recovery events. The branded playback path also now shares the same filler/content-probe recovery logic as the plain virtual stream path, and the branded stream can overlay a real corner image from `branding.bug_image_url` or `branding.logo_url` in addition to text/banner draws. Focused verification is green (`go test ./internal/tuner ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: move from preflight-only black/silence checks toward longer-lived in-stream media analysis and make the deck/operator plane surface recovery history without raw JSON polling.

**Latest (2026-03-22):** **The deck/operator plane now sees the station-ops runtime too:** `internal/webui/deck.js` now pulls `/api/virtual-channels/recovery-report.json?limit=8`, adds a Programming-lane “Virtual recovery history” card, and includes recent virtual recovery context alongside the existing virtual schedule/programming detail surfaces. That means the new filler/probe behavior is no longer only visible via raw tuner JSON endpoints. Focused verification is green (`go test ./internal/webui ./internal/tuner ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: richer station-specific deck controls and eventually actual in-stream rather than startup-only recovery analysis.

**Latest (2026-03-22):** **Branded station playback can now become the published default instead of a hidden alternate URL:** virtual-channel detail reports now include `published_stream_url`, and `IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT=true` makes branded channels publish `/virtual-channels/branded-stream/<id>.ts` in `/virtual-channels/live.m3u` while leaving unbranded channels on the plain `/virtual-channels/stream/<id>.mp4` path. The same env is surfaced in `/debug/runtime.json` so the deck/runtime snapshot can show whether branded-default publishing is active. Focused verification is green (`go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: operator-side mutation/control for station branding mode and then deeper continuous recovery analysis.

**Latest (2026-03-22):** **Virtual recovery now samples actual upstream response bytes too, not only a separate source-URL probe:** `evaluateVirtualChannelResponseForRecovery` now buffers a bounded startup sample from the real upstream body, reconstructs the response, and lets ffmpeg run `blackdetect` / `silencedetect` against those sampled bytes before deciding whether to cut over to filler. That means recovery reasons can now distinguish response-byte probes like `content-blackdetect-bytes` from the older URL-preflight reasons. Focused verification is green (`go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: move from startup-sampled detection to continuous in-stream monitoring/cutover instead of stopping at the first buffered window.

**Latest (2026-03-22):** **Station publish mode is now controllable per channel, not only per process:** `internal/virtualchannels.Branding` now supports `stream_mode` (`plain`, `branded`, or empty/`auto`), and the published virtual stream URL respects that override before falling back to `IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT`. That gives station authors an actual per-channel publish knob for whether the exported lineup should use the plain or branded playback surface. Focused verification is green (`go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: expose mutation UX for `stream_mode` directly in the deck/programming lane and keep pushing toward continuous live-session recovery rather than startup-only checks.

**Latest (2026-03-22):** **Virtual stations now have a real report surface instead of only stitched-together detail/schedule/recovery payloads:** `GET /virtual-channels/report.json` now returns per-station rows with publish mode, published stream URL, current resolved slot, branded/slate URLs, and recent recovery history. The deck Programming lane now consumes that report directly for station posture cards/context instead of deriving everything from separate schedule and recovery payloads. Focused verification is green (`go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: add actual deck mutation controls for station publish mode/branding and keep pushing the runtime toward continuous recovery monitoring.

**Latest (2026-03-22):** **The deck can now mutate per-station publish mode directly:** the Programming lane’s virtual station cards now post partial `channel-detail.json` branding updates for `stream_mode` (`plain`, `branded`, `auto`), and the server now merges partial branding updates instead of replacing the whole branding object. That makes the new station publish-mode feature operable from the control plane without clobbering existing logo/bug/banner metadata. Focused verification is green (`go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: extend those controls to other station branding fields and keep pushing runtime recovery beyond startup-window analysis.

**Latest (2026-03-22):** **Both of those follow-ons have now moved another step:** the deck’s virtual-station cards can now edit `logo_url`, `bug_text`, and `banner_text` in addition to `stream_mode`, with empty submissions clearing those fields safely via `branding_clear`. On the runtime side, startup recovery sampling now also has a process-wide warmup extension knob, `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_WARMUP_SEC`, so operators can keep the response-byte monitor alive longer than the per-channel `black_screen_seconds` threshold when needed. Focused verification is green (`go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: richer deck-side editing for the remaining branding fields and then a true continuous live-session recovery loop rather than a longer startup monitor.

**Latest (2026-03-22):** **The remaining first-step branding controls are now in the deck too:** virtual-station cards can now edit `bug_image_url`, `bug_position`, and `theme_color`, while the Settings lane shows both `virtual_channel_branding_default` and `virtual_channel_recovery_warmup_sec` from the runtime snapshot. That gives operators a complete first-pass station-branding control surface without dropping to raw JSON for the most common fields. Focused verification is green (`node -c internal/webui/deck.js`, `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: stop adding UI knobs and tackle the real runtime gap, which is continuous live-session monitoring/cutover instead of warmup-only detection.

**Latest (2026-03-22):** **Identity migration audit now tracks activation readiness too, not just account existence and policy drift:** `internal/migrationident` now reports `activation_pending_users` alongside `missing_users` and `policy_update_users`, and `internal/emby` now classifies destination local users as not activation-ready when they still have no configured password or auto-login path. That means `identity-migration-diff`, `identity-migration-apply`, `identity-migration-audit`, and the deck workflow can now distinguish “user exists” from “user can actually sign in” without pretending to solve passwords or invite flows in-app. Focused verification is green (`go test ./internal/emby ./internal/migrationident ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: OIDC-provider integration plus richer destination permission mapping beyond the current safe additive subset.

**Latest (2026-03-22):** **Identity migration now does real additive access-policy parity, not just local-account bootstrap:** `internal/migrationident` plans/diffs/applies now carry the first safe destination policy subset that can be inferred from Plex share state, and `internal/emby` now preserves plus updates full user policy blobs through `/Users/{id}/Policy` instead of treating every tuner/share right as manual cleanup forever. The current automatic subset is: Live TV access from `AllowTuners`, sync/download access from `AllowSync`, all-library access when Plex exposes `AllLibraries`, and remote access for Plex-shared users. `identity-migration-diff`, `identity-migration-apply`, `identity-migration-audit`, and the deck workflow now expose policy-update drift separately from missing-account drift while still leaving folder-specific grants, invite/activation state, and OIDC/Caddy provisioning as follow-on work. Focused verification is green (`go test ./internal/emby ./internal/migrationident ./cmd/iptv-tunerr`); immediate next step if we keep pushing this lane: activation/invite state and then OIDC-provider integration on top of the same neutral identity bundle.

**Latest (2026-03-22):** **All three requested Station Ops directions now have a first concrete code path:** `docs/epics/EPIC-station-ops.md` owns the multi-PR plan for branded synthetic stations, filler/black-screen recovery, richer schedule authoring, and multi-backend rollout, with the explicit stance that these features stay free. Schedule authoring already includes collection-aware autofill (`fill_movie_category`, `fill_series`) plus slot/daypart helpers. Runtime recovery now covers missing sources, failed upstream requests, obviously bad non-media responses, stalled startup according to `black_screen_seconds`, and now also ffmpeg-based content probes: when `ffmpeg` is available, the virtual path can preflight with `blackdetect` / `silencedetect` and cut over to filler when the sampled source looks black or silent from the start. The branding layer also now has a first real rendered output via `/virtual-channels/slate/<id>.svg`, even though true live-video overlay compositing is still future work. Focused verification is green (`go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`, `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`). Immediate next step if we keep pushing this lane: real decoded-video black-frame/audio-silence analysis in-stream instead of only ffmpeg preflight heuristics, plus turning the slate/branding surface into actual live-video overlay compositing.

**Latest (2026-03-22):** **Identity migration now reaches the deck too, not just the CLI:** the built-in identity audit is now exposed at `/deck/identity-migration-audit.json` when `IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE` is set, and the deck shows an Identity Migration workflow card next to the existing overlap-migration workflow. That means account-cutover readiness, missing users, and manual follow-up signals are visible from the running appliance instead of only through CLI JSON/summary output. It is still intentionally local-user only: no password cloning, no OIDC/Caddy provisioning, and no permission/invite parity yet. The broader “general-purpose library janitor” direction remains backlogged in `memory-bank/opportunities.md`. Verification is green (`go test ./internal/webui ./internal/migrationident ./cmd/iptv-tunerr`, `./scripts/verify`). Immediate next step if we keep pushing this lane: actual destination permission/group parity plus activation/invite state, then OIDC-provider integration on top of the same neutral identity bundle.

**Latest (2026-03-22):** **The migration audit now has bounded title-level parity hints for reused libraries, not just count parity:** `internal/plex` can now sample source library item titles, `internal/emby` can sample destination library item titles, and `internal/livetvbundle` now carries those source titles through the bundle/plan so `migration-rollout-audit` can expose `title_synced_libraries` / `title_lagging_libraries` plus per-library `source_titles`, `existing_titles`, `missing_titles`, and `title_parity_status`. This is intentionally a bounded sample hint, not full metadata equivalence, but it finally distinguishes “destination count looks close” from “destination is still visibly missing known Plex items.” Focused verification is green (`go test ./internal/plex ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: shape the audit/CLI output into a more operator-readable sync report instead of only adding deeper nested parity fields.

**Latest (2026-03-22):** **The migration audit now has a built-in operator summary mode, not just nested JSON:** `cmd/iptv-tunerr/cmd_live_tv_bundle.go` now supports `iptv-tunerr migration-rollout-audit -summary`, which flattens the existing audit into a compact text rollout report with overall verdict, per-target status/reason, indexed Live TV count, and the main missing/lagging library hints. This does not change the machine-readable JSON contract; it gives operators a first-class sync/readiness report without ad hoc `jq` post-processing. Focused verification is green (`go test ./cmd/iptv-tunerr ./internal/livetvbundle`). Next depth if we keep pushing this lane: expose the same report shape in the deck/operator plane or add richer item-level parity reporting beyond bounded title samples.

**Latest (2026-03-22):** **The human-readable migration summary now includes bounded per-library missing-title hints, not just library names:** when `migration-rollout-audit -summary` sees reused libraries with `title_parity_status=sample_missing`, it now prints `title_missing[Library]: ...` lines with a bounded set of missing source sample titles. That makes the summary materially more useful for sync triage without forcing operators back into the nested JSON. Focused verification is green (`go test ./cmd/iptv-tunerr ./internal/livetvbundle`). Next depth if we keep pushing this lane: surface this same compact triage report in the operator/deck plane or add a dedicated structured item-parity report per library.

**Latest (2026-03-22):** **That migration audit is now surfaced in the dedicated deck too, not just the CLI:** `internal/webui` now serves `/deck/migration-audit.json`, backed by `IPTV_TUNERR_MIGRATION_BUNDLE_FILE` plus the existing Emby/Jellyfin host/token envs. The deck Operations lane now includes a Migration workflow card so operators can inspect overlap readiness, target status, and lagging-library signals from the running process. Focused verification is green (`go test ./internal/webui ./cmd/iptv-tunerr ./internal/livetvbundle ./internal/tuner`). Next depth if we keep pushing this lane: add a dedicated structured per-library parity drill-down instead of only workflow summary fields and report text.

**Latest (2026-03-22):** **An experimental AAC-based TV-safe profile now exists in code, but the live helper is intentionally still pinned to the known-good MP3-based `plexsafemax` lane:** after confirming `plexsafemax` worked for both TV and browser-backed sessions, a separate built-in profile `plexsafeaac` was added in `internal/tuner/gateway_profiles.go` so future A/B tests can compare the same high-quality video settings with AAC audio instead of MP3. It is not live yet. Current rationale: MP3 is still the only audio path conclusively proven to satisfy the LG/PMS compatibility branch, so AAC should be tested deliberately rather than swapped into production immediately. Focused verification is green:
- `go test ./internal/tuner -run 'Test(BuildFFmpegStreamCodecArgs_(plexsafeHQ|plexsafeMax|plexsafeAAC)|NormalizeProfileName_HDHRStyleAliases)$'`
- `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
Current live `:5005` helper remains foreground exec session `26611` with:
- `IPTV_TUNERR_PLEX_WEB_CLIENT_PROFILE=copyvideomp3`
- `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE=plexsafemax`
Immediate next step if we keep pushing quality: temporarily switch only the TV/internal lane to `plexsafeaac`, re-test the LG client, and compare whether Plex still keeps playback stable or falls back into the old session/decision failures.

**Latest (2026-03-22):** **The host-local TV-safe lane is now on a stronger transcode profile, `plexsafemax`, while browser playback stays on `copyvideomp3`:** after proving the LG TV only stabilized on the stricter full-transcode lane, but the resulting image could still look softer than desired, `internal/tuner/gateway_profiles.go` gained a new built-in profile `plexsafemax`. It preserves the same compatibility shape as `plexsafehq` (H.264 + MP3, `setsar=1`, no forced resize) but raises quality/perf ceilings:
- `preset=faster`
- `crf=16`
- `maxrate=30000k`
- `bufsize=60000k`
- `audio=MP3 256k`
- `mpegts muxrate=34000000`
Regression coverage in `internal/tuner/gateway_profiles_test.go` proves the exact ffmpeg args. During the same pass, an unrelated compile break in `internal/emby/register.go` (`TriggerGuideRefresh` had lost local `client/status` declarations) was repaired so `go build` could succeed again. Verification passed with:
- `go test ./internal/tuner ./cmd/iptv-tunerr`
- `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
- targeted profile tests for `plexsafehq` and `plexsafemax`
Current live `:5005` helper is foreground exec session `26611` with:
- `IPTV_TUNERR_PLEX_WEB_CLIENT_PROFILE=copyvideomp3`
- `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE=plexsafemax`
Runtime debug confirms the split on `http://127.0.0.1:5005/debug/runtime.json`. Immediate next step: retest TV playback quality and confirm the higher-quality lane still starts reliably before attempting riskier changes like AAC audio or deeper encoder changes.

**Latest (2026-03-22):** **The migration audit now exposes coarse library population visibility, not just library-definition presence:** `internal/emby` gained `GetLibraryItemCount(...)`, `internal/livetvbundle` now enriches reused library diff rows with `existing_item_count`, and the combined `migration-rollout-audit` now surfaces `populated_libraries` plus `empty_libraries` per target. This does not change readiness or convergence logic yet; it is an operator hint that distinguishes “library already exists” from “library exists but is still empty.” Focused verification is green (`go test ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: move from coarse item-count visibility into richer post-cutover scan/ingest progress rather than just empty-vs-populated library state.

**Latest (2026-03-22):** **The migration audit now exposes best-effort library scan progress too, not just library counts:** `internal/emby` now recognizes library-refresh scheduled tasks via `GetLibraryScanStatus(...)`, and the combined `migration-rollout-audit` surfaces `library_scan` per target when the destination exposes a recognizable scan task. This is still visibility-only and does not affect readiness/convergence, but it gives overlap migrations a coarse “still scanning vs idle” signal after apply. Focused verification is green (`go test ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: move from coarse scan-task visibility into richer ingest/index parity signals rather than only task status and item counts.

**Latest (2026-03-22):** **The migration audit now has real source-vs-destination library parity hints, not just destination-side counts:** source Plex library counts now flow through `BuildFromPlexAPI(...)` into the neutral bundle and library migration plans, `DiffLibraryPlan(...)` compares them with reused Emby/Jellyfin library item counts, and the combined audit now surfaces `synced_libraries` / `lagging_libraries` plus per-library `parity_status`. This still does not change readiness or convergence, but it finally distinguishes “reused library exists” from “reused library is materially caught up with Plex.” Focused verification is green (`go test ./internal/plex ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: move from coarse item-count parity into richer media-type/title or scan-completion parity instead of only aggregate counts.

**Latest (2026-03-22):** **The host-local `:5005` helper now splits Plex Web and ambiguous/internal PMS fetchers onto different compatibility profiles instead of forcing one shared WebSafe profile:** after confirming that Plex Web could recover on `copyvideomp3` while the LG TV still needed stricter full transcode to avoid the PMS segment-loop/spin behavior, `internal/tuner/gateway_adapt.go` was updated so the `websafe` branch no longer always uses `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE`. New env overrides:
- `IPTV_TUNERR_PLEX_WEB_CLIENT_PROFILE`
- `IPTV_TUNERR_PLEX_NATIVE_CLIENT_PROFILE`
- `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE`
The branch now resolves a client-class-specific profile first, then falls back to `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE`. Regression coverage in `internal/tuner/gateway_test.go` proves:
- resolved Plex Web can use `copyvideomp3` even when the general WebSafe profile is different
- resolved/ambiguous internal PMS fetchers can use `plexsafehq`
Runtime snapshot now exposes the new envs in `/debug/runtime.json`. Verification passed with:
- `go test ./internal/tuner ./cmd/iptv-tunerr`
- `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
- targeted adaptation tests for the new profile split
Current live `:5005` helper is foreground exec session `40620` with:
- `IPTV_TUNERR_PROFILE=copyclean`
- `IPTV_TUNERR_PLEX_UNKNOWN_CLIENT_POLICY=direct`
- `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY=websafe`
- `IPTV_TUNERR_PLEX_RESOLVE_ERROR_POLICY=direct`
- `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE=copyvideomp3`
- `IPTV_TUNERR_PLEX_WEB_CLIENT_PROFILE=copyvideomp3`
- `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE=plexsafehq`
Runtime debug confirms `stream_transcode="off"` globally, but browser-safe and TV-safe fallback profiles are now split. Immediate next step: re-test TV and browser against this exact runtime and inspect whether first-attempt browser playback stays on `copyvideomp3` while the TV no longer falls into the old “spin until retry” pattern.

**Latest (2026-03-22):** **The ambiguous Plex internal-fetcher lane is no longer relying only on sticky fallback; the host-local helper now deterministically infers the real active Plex client when PMS/Lavf forwards no session/client hints:** after confirming from PMS logs that both the LG TV and Plex Web could recover once the request shape flipped to `directStreamAudio=1` / safe-audio behavior, the adaptation path in `internal/tuner/gateway_adapt.go` was tightened so a no-hints request with `User-Agent: Lavf/...` or `PlexMediaServer/...` can still query `/status/sessions` and inherit the single plausible non-internal active client. If exactly one non-internal client is active, or exactly one web client is active with no native competitors, `requestAdaptation` now resolves that browser/native client immediately instead of returning `unknown-client-websafe` and waiting for quick-abort sticky fallback to learn it. New regression coverage in `internal/tuner/gateway_test.go` proves both branches:
- no-hints internal fetcher + single web session => `resolved-web-client` + `copyvideomp3`
- no-hints internal fetcher + single native session => `resolved-nonweb-client` + direct/remux
Verification passed with `go test ./internal/tuner ./cmd/iptv-tunerr` and `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`. The live `:5005` helper is now foreground exec session `3611` with:
- `IPTV_TUNERR_PROFILE=copyclean`
- `IPTV_TUNERR_CLIENT_ADAPT=true`
- all three Plex ambiguous-client policies still `direct`
- `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE=copyvideomp3`
Runtime debug confirms `count=4`, `stream_transcode="off"`, `client_adapt="true"`, and `force_websafe_profile="copyvideomp3"`. Immediate next step: re-test in Plex Web and the LG TV so the new deterministic no-hints inference can be validated against real traffic instead of the old sticky-after-failure path.

**Latest (2026-03-22):** **The remux-first retry reproduced the same LG/PMS decision failure, so the host-local helper has now moved to a narrower compatibility path: copy video, normalize audio, strip subtitle/data baggage:** live PMS evidence from the remux-first run proved the helper was healthy but Plex still failed later in `GET /video/:/transcode/universal/decision` for the TV. Examples:
- `15574` / `708385` (`CA| TELETOON FRENCH HD`): PMS tuned successfully and created session `243761fb-f480-4005-a52f-f00f02747df4`, but the TV then hit `GET .../universal/decision?...directStream=1...subtitles=none...` and PMS returned `500` at `2026-03-22 16:39:45`.
- `15578` (`CA| CBC EDMONTON HD`): PMS tuned successfully again, but the later TV request hit `GET .../universal/decision?...directStream=0...subtitles=burn...` and PMS returned `500` at `2026-03-22 16:39:56`.
The strongest signal inside the same PMS run was still bad early media metadata on the remux path (`selected subtitle stream has no codec`, plus progress reporting audio `channels=0` / `sampleRate=0`). To target that without going back to full video re-encode, a new built-in profile `copyvideomp3` was added in `internal/tuner/gateway_profiles.go`. It keeps source video (`-c:v copy`), disables subtitle/data mapping (`-sn -dn`), and re-encodes audio to clean `mp3 stereo 48 kHz 192k`. The live `:5005` helper is now running with `IPTV_TUNERR_CLIENT_ADAPT=true`, local PMS URL/token configured, and all three Plex-ambiguous adaptation policies back on `websafe`, but with `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE=copyvideomp3` instead of `plexsafehq`. Verified by direct probe on `15578`: Tunerr logs `adapt transcode=true profile="copyvideomp3" reason=unknown-client-websafe`, and `ffprobe` on the resulting TS shows `h264` video copy plus `mp3` stereo `48000` audio with sane aspect. Remaining gap: re-test on the TV to see whether this narrower compatibility path satisfies Plex `universal/decision` without the washed-out full-video transcode tradeoff.

**Latest (2026-03-22):** **The intended `copyvideomp3` runtime is now live on the host-local helper and verified on the real `:5005` process, not just in code/tests:** the old `15:43` helper process was still running and did not expose the newly added runtime fields, so the current build was rebuilt and the helper relaunched interactively. Current live session: foreground exec `75250`, command `./iptv-tunerr run -mode=easy -addr=:5005 ...`, env posture:
- `IPTV_TUNERR_CLIENT_ADAPT=true`
- `IPTV_TUNERR_PMS_URL=http://127.0.0.1:32400`
- `IPTV_TUNERR_PMS_TOKEN` set
- `IPTV_TUNERR_PLEX_UNKNOWN_CLIENT_POLICY=websafe`
- `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY=websafe`
- `IPTV_TUNERR_PLEX_RESOLVE_ERROR_POLICY=websafe`
- `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE=copyvideomp3`
- `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE=true`
- lineup shaping/music-drop still enabled for the 479-cap host-local lineup
Runtime debug now confirms the active posture directly via `/debug/runtime.json`:
- `stream_transcode: "off"`
- `client_adapt: "true"`
- `force_websafe_profile: "copyvideomp3"`
- all three Plex policies = `"websafe"`
Direct proof on channel `15578` (`CA| CBC EDMONTON HD`): helper logs `adapt transcode=true profile="copyvideomp3" reason=unknown-client-websafe`, then `ffmpeg-transcode profile=copyvideomp3`, and `ffprobe` on `/tmp/stream15578.ts` reports `h264` video (`1280x720`, `SAR 1:1`, `DAR 16:9`) plus `mp3` stereo `48 kHz`. Immediate next step: TV retest against this exact session, then inspect PMS `universal/decision` behavior again.

**Latest (2026-03-22):** **A true no-transcode candidate is now live for TV re-test: `copyclean` remux keeps both video and audio copied while stripping subtitle/data baggage and constraining to first video + first audio only:** after the stable `copyvideomp3` run proved that full video re-encode was not needed, a new built-in profile `copyclean` was added in `internal/tuner/gateway_profiles.go`. It is explicitly non-transcoding (`ForceTranscode=false`) and changes the non-transcode FFmpeg remux args from broad `-map 0:a?` to:
- `-map 0:v:0`
- `-map 0:a:0?`
- `-sn`
- `-dn`
- `-c copy`
This is meant to test the hypothesis that Plex/LG was reacting to stream-selection/subtitle/data noise rather than requiring audio transcode. Live host-local runtime now running in foreground exec session `11506` on `http://127.0.0.1:5005` with:
- `IPTV_TUNERR_PROFILE=copyclean`
- `IPTV_TUNERR_TUNER_COUNT=4`
- `IPTV_TUNERR_PLEX_UNKNOWN_CLIENT_POLICY=direct`
- `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY=direct`
- `IPTV_TUNERR_PLEX_RESOLVE_ERROR_POLICY=direct`
- `IPTV_TUNERR_CLIENT_ADAPT=true`
Runtime debug confirms `count=4`, `stream_transcode="off"`, and all Plex adaptation policies=`direct`. Direct probe on `15578` now logs `adapt transcode=false` and `ffmpeg-remux profile=copyclean`. `ffprobe` on `/tmp/stream15578-copyclean.ts` shows copied `h264` video plus copied `aac` stereo `48 kHz`, still with sane `SAR 1:1` / `DAR 16:9`. Immediate next step: TV retest against this exact `copyclean` session and compare PMS `universal/decision` behavior against the working `copyvideomp3` run.

**Latest (2026-03-22):** **The blank `plexkube` tab was a real PMS provider-state failure, not just a raw-Tunerr guide problem, and it is now repaired on DVR `723`:** after the TV reported a completely blank Live TV source, direct PMS inspection showed the exact client-facing provider endpoint `GET /tv.plex.providers.epg.xmltv:723/lineups/dvr/channels` was returning `<MediaContainer size="0">` even though Tunerr `http://127.0.0.1:5005/lineup.json` still had 479 channels and `guide.xml` was non-empty. Root cause: the DVR's enabled `ChannelMapping` set inside PMS had drifted away from the current channelmap generation. Evidence from local replay: current `/livetv/epg/channelmap?...device=device://tv.plex.grabbers.hdhomerun/hdhrlocal&lineup=...#hdhrlocal` returned 479 rows, but only 380 were fully valid (`channelKey` + `deviceIdentifier` + `lineupIdentifier`) and just **3** of those overlapped the DVR's then-current 475 enabled IDs. Reapplying activation against the exact current valid map for device `722` fixed the provider layer: `/tv.plex.providers.epg.xmltv:723/lineups/dvr/channels` now returns `size="80"` instead of `0`, and `/tv.plex.providers.epg.xmltv:723/hubs/discover` is populated with real items again. To keep this reproducible, a new repo command `plex-dvr-repair` is being added so we can rebuild one DVR's enabled set from the current Plex channelmap without hand-building PUT requests. Remaining gap: the provider endpoint is no longer blank, but it currently surfaces only 80 client-visible channels from the repaired/valid subset, so the next depth is understanding why 300+ otherwise-valid current channelmap rows are still not surfacing in the provider layer.

**Latest (2026-03-22):** **The host-local `:5005` runtime is now both non-forced and shaped toward a North American local-style 479 lineup:** after removing the universal `FORCE_WEBSAFE` override, the runtime was also switched to `IPTV_TUNERR_LINEUP_SHAPE=na_en` with `IPTV_TUNERR_LINEUP_REGION_PROFILE=ca_west`, while keeping `IPTV_TUNERR_LINEUP_DROP_MUSIC=true` and `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE=true`. The current live session is `64700` on `:5005`. Live logs now show:
`Lineup pre-cap filter: dropped 1609 music/radio channels ...`
`Lineup pre-cap shape: shape=na_en region=ca_west reordered 48448/48455 channels ...`
`Lineup updated: channels=479 epg_linked=466 ...`
and the exposed top of the lineup is now dominated by useful Canadian/NA rows such as `CTV 2 VICTORIA`, `GLOBAL SASKATOON`, `GLOBAL REGINA`, `CBC EDMONTON`, `CBC WINNIPEG`, `CITY TV VANCOUVER`, `CTV REGINA`, `ABC 18`, `NBC SN HD`, and `FOX NEWS HD`. For playback, the host-local path is back on the non-forced branch: probe logs on `1019880` show `hls-mode transcode=false mode="off"` followed by `ffmpeg-remux profile=default base_profile=default output_mux=mpegts`, proving the host-local tuner is no longer using the forced `plexsafehq` transcode workaround for that channel. Remaining gap: re-test on the TV to confirm that Plex is rendering the reshaped lineup and that the washed-out look is gone; if TV playback regresses again, the right next move is selective fallback/adaptation rather than restoring universal forced transcode.

**Latest (2026-03-22):** **Live TV migration is now a built-in build → convert → apply lane, not just a JSON experiment:** added neutral bundle/export logic in `internal/livetvbundle` plus CLI commands `live-tv-bundle-build`, `live-tv-bundle-convert`, and now `live-tv-bundle-apply`. The new apply slice can take a converted Emby/Jellyfin registration plan and register it directly against a live server through the existing `internal/emby` APIs, with optional persisted state-file cleanup/idempotence just like runtime registration. Docs now explicitly frame this as a gradual migration path: keep Plex online, pre-roll Emby/Jellyfin from the same Tunerr-backed tuner/guide identity, and move users over gradually instead of forcing a one-shot cutover. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`, `go test ./internal/plex -run 'Test(ActivateChannelsAPI_keepsFullEnabledSetAcrossBatches|RepairDVRChannelActivation.*)$'`). Next depth if we keep pushing this lane: add a dual-register/sync plan on top of the neutral bundle so the overlap workflow is explicit rather than implied.

**Latest (2026-03-22):** **The overlap migration path now has a built-in multi-target rollout command too:** added `RolloutPlan` / `RolloutApplyResult` support in `internal/livetvbundle` plus CLI `live-tv-bundle-rollout`, which can emit or apply one coordinated Emby+Jellyfin rollout from the same neutral Plex-derived bundle. This makes the “keep Plex live while pre-rolling both other hosts together” workflow explicit instead of forcing two unrelated manual applies. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep going here: carry the same shared-source idea beyond Live TV registration into reusable library/catch-up registration plans so dual-host migration covers more than tuner+guide state.

**Latest (2026-03-22):** **The migration lane now reaches shared library definitions too, not just Live TV:** `internal/livetvbundle` bundles can now optionally include Plex library sections and storage paths, `live-tv-bundle-build` has `-include-libraries`, and new commands `library-migration-convert` / `library-migration-apply` can turn those bundled Plex sections into Emby/Jellyfin library plans and apply them through the built-in media-server APIs. This is explicitly scoped to library definitions and shared paths, not vendor metadata DB conversion. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: add coordinated multi-server library rollout helpers and bring catch-up/library registration plans under the same neutral migration artifact.

**Latest (2026-03-22):** **Coordinated library rollout now mirrors the Live TV overlap workflow:** added `LibraryRolloutPlan` / `LibraryRolloutApplyResult` support in `internal/livetvbundle` and CLI `library-migration-rollout`, so the same bundled Plex library definitions can be emitted or applied across both Emby and Jellyfin in one step while Plex remains untouched. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep going: move generated catch-up library layouts under the same neutral migration artifact so the overlap lane covers shared server-facing surfaces end-to-end.

**Latest (2026-03-22):** **The migration bundle can now carry generated catch-up libraries too, not just Live TV plus shared Plex sections:** added `Bundle.Catchup` and `AttachCatchupManifest(...)` in `internal/livetvbundle`, plus CLI `live-tv-bundle-attach-catchup`, so a saved `catchup-publish` manifest can be folded into the same neutral migration artifact. `BuildLibraryPlan` now merges bundled Plex libraries and attached catch-up lanes, which means the existing library convert/apply/rollout commands can pre-roll Emby/Jellyfin catch-up libraries from the same bundle instead of requiring a separate side flow. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep going here: move from config/library migration into fuller shared-library synchronization and broader non-Live-TV overlap workflows.

**Latest (2026-03-22):** **Library migration now has a real dry-run diff stage against live Emby/Jellyfin state:** added `DiffLibraryPlan(...)` plus CLI `library-migration-diff`, which loads a target library plan, queries the live destination, and reports `reuse`, `create`, `conflict_type`, and `conflict_path` outcomes per library before apply. That closes the main operational blind spot in the overlap workflow: operators can now validate whether bundled Plex libraries and attached catch-up lanes will slot cleanly into the target server without mutating it first. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: extend the same diff/sync idea beyond library definitions into fuller overlap-state reporting instead of stopping at create/reuse checks.

**Latest (2026-03-22):** **The library diff stage now works across both non-Plex targets in one pass too:** added `DiffLibraryRolloutPlan(...)` and CLI `library-migration-rollout-diff`, which builds the same multi-target library rollout plan and then queries Emby and Jellyfin live state from that one neutral bundle. Operators can now see both targets' `reuse` / `create` / conflict results together before deciding whether the overlap migration is ready to apply. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: move from path-definition diffs toward fuller overlap-state sync/reporting for metadata and migration progress, while still keeping core functionality inside the Go binary.

**Latest (2026-03-22):** **The Live TV side now has the same diff symmetry as the library side:** added Emby/Jellyfin read helpers for existing tuner hosts and listing providers, plus `DiffEmbyPlan(...)` / `DiffRolloutPlan(...)` and CLI `live-tv-bundle-diff` / `live-tv-bundle-rollout-diff`. Live TV migration is now build → convert → diff or rollout-diff → apply, not build → convert → blind apply. Focused verification is green (`go test ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: broaden the overlap report from definition matching into real migration-progress and metadata/state-sync visibility, still inside the Go binary rather than sidecar scripts.

**Latest (2026-03-22):** **The overlap workflow now has one top-level audit command too, not just separate diff pieces:** added `AuditBundleTargets(...)` plus CLI `migration-rollout-audit`, which combines per-target Live TV diffing and optional library/catch-up diffing from the same neutral bundle into one report. That gives operators one answer to "is this target migration-ready?" instead of forcing them to manually correlate `live-tv-bundle-rollout-diff` and `library-migration-rollout-diff` output. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: add migration-progress and metadata/state visibility so the audit can say not only whether definitions match, but how far a destination has actually progressed after cutover.

**Latest (2026-03-22):** **The combined migration audit now emits an explicit readiness verdict too, not just raw diff sections:** `MigrationAuditResult` and `MigrationTargetAudit` now expose `ready_to_apply`, readiness by surface, and rolled-up conflict counts. That means the audit can act as a real pre-apply gate for overlap migration instead of leaving operators to infer readiness from nested JSON blocks. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: attach real migration-progress and metadata/state-sync visibility so readiness covers more than definition conflicts.

**Latest (2026-03-22):** **The migration audit now distinguishes “clean” from “actually converged”:** `LiveTVDiffResult` now records `indexed_channel_count`, and the combined audit computes `status` per target and overall (`blocked_conflicts`, `ready_to_apply`, `converged`). That gives a first real post-cutover visibility signal inside the same audit path: a target can be conflict-free yet still not have indexed channels, which is materially different from a target that is already exposing Live TV successfully. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: add metadata/library-state progress signals so convergence is not inferred only from Live TV channel indexing.

**Latest (2026-03-22):** **Convergence is now library-aware too, not just Live-TV-aware:** the combined audit now marks a target `converged` only when Live TV is indexed and any bundled libraries/catch-up lanes are already present. A target with indexed Live TV but still-missing bundle libraries now stays `ready_to_apply`, which is the correct state for a partial migration. Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: add richer post-cutover metadata/state progress signals inside the same audit instead of stopping at library presence.

**Latest (2026-03-22):** **The audit is now more actionable, not just more accurate:** each target report now includes `status_reason` plus `present_libraries` / `missing_libraries`, so partial migrations tell the operator exactly what remains absent instead of only saying “not yet converged.” Focused verification is green (`go test ./internal/livetvbundle ./cmd/iptv-tunerr`). Next depth if we keep pushing this lane: add metadata-scan or library-ingest progress signals so “missing” vs “present” is not the only post-cutover library state we can see.

**Latest (2026-03-22):** **The host-local `:5005` runtime now strips music/radio rows before the 479-channel Plex cap:** the arbitrary first-479 slice was still surfacing big `RADIO`/music blocks because `applyLineupBaseFilters` only drops those rows when `IPTV_TUNERR_LINEUP_DROP_MUSIC=true`. The live runtime has now been restarted as session `51513` on `:5005` with `IPTV_TUNERR_LINEUP_DROP_MUSIC=true`. Verified from live logs: `Lineup pre-cap filter: dropped 1609 music/radio channels by name heuristic (remaining 48455)` before the 479 cap, and the resulting lineup no longer begins with the Trinidad radio block. The first rows are now things like `CA| NBA TV`, `V SPORT`, `NEWS 12 LONG ISLAND`, `CANAL+ FOOT`, and `TNT SPORTS`. Remaining gap: the 479-cap is still a truncated slice of the full sorted catalog, not a local-market-ranked lineup, and Plex may continue showing cached older rows until it refreshes the DVR guide.

**Latest (2026-03-22):** **The host-local forced compatibility path now uses a higher-quality Plex-safe profile instead of the old low-bitrate cap:** added built-in profile `plexsafehq` in `internal/tuner/gateway_profiles.go` and a new env override `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE` in `internal/tuner/gateway_adapt.go`, so forced-websafe no longer has to hardcode `plexsafe`. `plexsafehq` keeps the compatibility-important MP3 audio path but adds `setsar=1`, raises the mux/video ceiling substantially (`crf=18`, `maxrate=16M`, `bufsize=32M`, `mpegts muxrate=18M`), and increases audio to `192k`. Targeted tests passed (`go test ./internal/tuner ./cmd/iptv-tunerr`) and the live local tuner has been restarted as session `49455` on `:5005` with `IPTV_TUNERR_FORCE_WEBSAFE=true`, `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE=plexsafehq`, and `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE=true`. Direct probe now proves the forced path is really using `plexsafehq` (`adapt ... profile="plexsafehq"`, `ffmpeg-transcode profile=plexsafehq`) and the resulting TS probe reports sane geometry/audio (`1280x720`, `SAR 1:1`, `DAR 16:9`, `30000/1001`, `MP3 stereo 48 kHz 192k`). Remaining gap: re-test on the TV to confirm whether the aspect/quality complaints are materially reduced under the new profile, and keep watching whether provider `509` limits still kill longer sessions later.

**Latest (2026-03-22):** **The smart-TV playback error and the later backend recorder failure are different bugs, and the host-local workaround now includes disabling FFmpeg's input-host IP rewrite:** PMS logs show the TV browse attempt on channel `45955` failed almost immediately in `/video/:/transcode/universal/decision` with `Got exception from request handler: Invalid argument` while building a Live TV session for client `192.168.50.225`. Separately, backend/manual tune tests on the same channel can run for tens of seconds and only fail later when upstream HLS refreshes hit provider `509` concurrency limits. Comparing live-session XML from `/livetv/dvrs/723/channels/45955/tune` and `/.../43784/tune` shows Plex still records suspicious stream metadata (`audioChannelLayout="0 channels"`, `samplingRate="0"`) even when steady-state `ffprobe` on `http://127.0.0.1:5005/stream/1002837` sees valid `aac stereo 48000`, which strongly points at a bad start-of-stream/early probe problem rather than permanently bad media. The current live runtime is session `33559` on `:5005` with both `IPTV_TUNERR_FORCE_WEBSAFE=true` and `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE=true`. That combination fixed a concrete FFmpeg startup failure: with DNS rewrite enabled, `relayHLSWithFFmpeg` died in the startup gate because FFmpeg was opening the Cloudflare-backed HLS input by numeric IP (`Invalid data found when processing input`). With DNS rewrite disabled, FFmpeg now keeps the original hostname, passes the startup gate (`idr=true aac=true`), emits `bootstrap-ts`, and serves real transcoded bytes on `http://127.0.0.1:5005/stream/1019880`. Remaining gap: the smart-TV client still needs to be re-tested against this improved runtime, and provider `509` concurrency limits can still kill long-running sessions later.

**Latest (2026-03-22):** **Short-EPG fallback now gives the host-local DVR some real programme blocks even though provider `xmltv.php` is blocked:** added an opt-in fallback in `internal/tuner/epg_pipeline.go` that calls upstream `player_api.php?action=get_short_epg` per channel when provider XMLTV fails. With `IPTV_TUNERR_PROVIDER_SHORT_EPG_FALLBACK=true` on the local `:5005` tuner, guide health improved from `0` real-programme channels to `31`, and channels like `15646` (`CA| NBA TV`) now show multiple real programme windows in `guide.xml`. Playback is also confirmed on another channel (`45955`, `IN-PREM| BOLLYWOOD 1`) with PMS creating a real Live TV session and continuously fetching `/stream/1019880`. Remaining gap: most of the 479-channel lineup is still placeholder-only because upstream short EPG is only available for a subset of channels, so the TV guide will still be mixed until a richer XMLTV source is restored.

**Latest (2026-03-22):** **Host-side local DVR tune path now works for real video channels after exposing Plex's `/auto/` compatibility path:** the rebuilt local tuner on `http://127.0.0.1:5005` now mounts `/auto/` as well as `/stream/`, which removed the dead `/auto/v<guide-number>` fallback that PMS used during manual tune. After restarting the local tuner and replaying `POST /livetv/dvrs/723/channels/<id>/tune`, PMS no longer falls back to an unreachable `.plex.svc` URL. For enabled video channel `43784` (`FR| CANAL+ FOOT FHD`), PMS now creates a live session, fetches `http://127.0.0.1:5005/stream/1002837`, returns a session payload with `videoDecision="copy"` / `audioDecision="copy"`, serves `/livetv/sessions/.../index.m3u8`, and returns playable `.ts` segments from that session. The previous radio-channel failure on `43401` remains a separate issue: PMS reaches the tuner, but the first-stage recorder/transcoder rejects that session after startup. Next depth: re-test on a real Plex client/TV against the fixed local DVR, then investigate why the recreated DVR `723` currently shows only 75 enabled `ChannelMapping` rows even though the cutover activation report recorded 475 activations.

**Latest (2026-03-22):** **The 475→75 DVR activation collapse was our batching bug, not Plex pruning, but the guide is still placeholder-only:** confirmed that `ActivateChannelsAPI` was sending `channelsEnabled` for only the current 100-row batch, and Plex treated each `PUT /media/grabbers/devices/:id/channelmap` as a full replacement. After patching it so every batch carries the full enabled set, live reactivation on device `722` restored `475` enabled mappings on DVR `723`. Separate finding: `http://127.0.0.1:5005/guide.xml` currently contains `475` channels and `475` long-span placeholder programmes (one per channel, no real schedule blocks), because provider XMLTV is still failing with `HTTP 403`. That explains the TV client showing a plain channel list instead of a timeline grid. Next depth: either restore a real provider/external EPG source or inject richer XMLTV for these channels, then retest on the TV with channels that are both enabled and have video playback history.

**Latest (2026-03-22):** **Reverse-engineering Plex Live TV host-side gating and tuner reachability:** current evidence now splits the problem into two layers. First, plex.tv still clamps non-Home shares back to `allowTuners=0` even through writable v2 share endpoints, so no bypass is proven there yet. Second, the current TV-side `Unavailable` state is concretely explained by dead injected HDHR device URIs: PMS is refreshing `http://plextuner-*.plex.svc:5004` registrations, cannot resolve them from the host, and marks those devices dead in `Plex Media Server.log`. Added operator tooling to capture client browse windows, audit registered device URIs from PMS's point of view, and dry-run/apply DVR URI cutovers from a TSV map via the same unsupported PMS APIs used for registration.

**Latest (2026-03-22):** **Host-side tuner cutover reached PMS scheduling, but tune still fails after channel activation:** brought up a self-consistent local tuner at `http://127.0.0.1:5005`, cut the main Plex DVR over to it, and activated 475 valid channels after fixing `GetChannelMap`/activation to skip malformed `ChannelMapping` rows returned by Plex. PMS now sees DVR `723` and device `722` as alive and fetches `/discover.json`, `/lineup_status.json`, `/guide.xml`, and `/lineup.json` from the new local tuner. Manual tune replay with a real `X-Plex-Session-Identifier` now progresses into Plex's rolling-subscription scheduler, but fails with `The device does not tune the required channel` before PMS ever requests `/stream/...` from Tunerr. Next depth: identify what Plex expects for the device-tunable channel identifier layer (likely a mismatch between Plex's `deviceIdentifier`/`channelKey` and the HDHR lineup stream URL identity), then adjust Tunerr's HDHR/control surfaces accordingly.

**Latest (2026-03-22):** **Reverse-engineering Plex Live TV / IPTV insertion + Plex Home gating:** current investigation is mapping exactly where IPTV Tunerr inserts Live TV into Plex (HTTP registration endpoints vs direct SQLite writes), which Plex-side objects are created or patched (`/media/grabbers/*`, `/livetv/dvrs`, `media_provider_resources`, lineup/EPG DBs), and whether the “Plex Home only” visibility rule can be bypassed through Plex DB or API automation. Goal: produce an evidence-based operator answer with confirmed repo code paths, confirmed Plex support/forum behavior for Home-user restrictions, and a concrete workaround matrix distinguishing “automatable via API”, “automatable via local DB edit”, and “not supported / likely enforced by plex.tv account state”. Assumptions: this is an investigation/report task, not a product change yet; any workaround recommendation must be labelled as confirmed vs inferred.

**Latest (2026-03-22):** **Shared-output reuse now covers `hls_go`, live FFmpeg HLS output, and packaged HLS, with bounded replay for late joiners:** the gateway now keys live ffmpeg HLS producer sessions by channel plus resolved output profile and reuses them before provider-account/tuner admission, not just the packaged-HLS path. A second viewer requesting the same channel with the same FFmpeg HLS output shape now attaches to the existing producer (`X-IptvTunerr-Shared-Upstream: hls_ffmpeg`), and shared sessions keep a bounded replay window so late subscribers can receive recent startup bytes instead of joining completely cold. The existing packaged-HLS path still reuses the packaged session (`X-IptvTunerr-Shared-Upstream: ffmpeg_hls_packager`). README, `docs/features.md`, the release-readiness matrix, `docs/reference/transcode-profiles.md`, and `docs/CHANGELOG.md` now describe the broader scope, and `scripts/ci-smoke.sh` now proves `hls_go`, live FFmpeg HLS TS/fMP4 sharing, and packaged-HLS sharing through a real temp binary in addition to focused gateway tests. Assumption: sharing is still intentionally bounded to identical output shape (same channel + resolved profile/mux contract), not arbitrary mixed mux/profile combinations or full rewind semantics. Next depth: if this lane keeps expanding, the next meaningful step is making replay duration/bytes a first-class operator control and integrating true near-replay/ring-buffer semantics rather than only a bounded startup replay.

**Latest (2026-03-22):** **Shared-output replay is now operator-visible and smoke-tested at the runtime/env layer too:** the CLI/env reference now documents `IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES` and clarifies that `/debug/shared-relays.json` reports generic shared-output sessions, not just the old Go HLS relay. The runtime snapshot in `cmd_runtime_server.go` now surfaces `shared_relay_replay_bytes`, and `scripts/ci-smoke.sh` was tightened so the real temp-binary smoke proves late-join replay bytes are observable on the deterministic FFmpeg TS and fMP4 shared-output lanes, not just that the second viewer receives some bytes. Verification is green through focused tuner relay tests, `bash ./scripts/ci-smoke.sh`, and full `./scripts/verify`. Next depth: if this expands further, expose richer operator controls/reporting for replay windows rather than only the raw env knob.

**Latest (2026-03-22):** **The deck now surfaces the shared replay window as part of the live transport posture:** `internal/webui/deck.js` now shows `Shared replay bytes` in the existing `Tuner + transport` settings card, using the runtime snapshot field already added in the previous pass. `internal/webui/webui_test.go` now locks that label/field into the served JS asset so the replay control does not disappear during future frontend churn. Verification is green through focused `internal/webui` tests, `bash ./scripts/ci-smoke.sh`, and full `./scripts/verify`. Next depth: if the operator surface keeps expanding, the next meaningful step is a read/write server setting or action for replay sizing rather than only passive runtime display.

**Latest (2026-03-22):** **Shared replay sizing is now a real live operator control, not just a displayed env/runtime value:** `internal/tuner/server.go` now exposes localhost-only `POST /ops/actions/shared-relay-replay` with `{"shared_relay_replay_bytes":<n>}`. The action updates `IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES` in-process and refreshes the runtime snapshot so new shared sessions pick up the new replay window immediately. `internal/webui/deck.js` now saves that value from the deck Settings panel in the same operator flow as refresh cadence, and `/ops/actions/status.json` advertises the action plus the current byte window. Docs (`docs/features.md`, `docs/reference/cli-and-env-reference.md`, `docs/CHANGELOG.md`) and regression coverage in `internal/tuner/server_test.go` / `internal/webui/webui_test.go` are updated, and verification is green through focused tuner/webui tests, `bash ./scripts/ci-smoke.sh`, and full `./scripts/verify`. Next depth: if this lane continues, the meaningful upgrade is per-profile/per-mux replay policy or bounded true near-replay controls rather than one global byte window.

**Latest (2026-03-22):** **The HDHomeRun guide helper still had the same empty-base validation gap as discover/lineup:** `internal/hdhomerun.GuideURLFromBase` would turn empty input into `/guide.xml`, and `FetchGuideXML` would then fail later at transport time instead of locally. Fixed both to fail closed on empty bases, with regression coverage in `internal/hdhomerun/guide_test.go`. Next depth: remaining defects should now be down to extremely narrow helper-level partial-init or normalization oddballs.

**Latest (2026-03-22):** **An SSDP discovery helper was still duplicating `device.xml` on already-complete URLs:** `internal/tuner.joinDeviceXMLURL` always appended `/device.xml`, so callers that already supplied a full device XML URL ended up with `/device.xml/device.xml`. Fixed it to preserve existing `device.xml` paths and added regression coverage in `internal/tuner/ssdp_test.go`. Next depth: remaining defects should now be extremely narrow helper-level normalization or nil-safety edges rather than discovery-surface drift.

**Latest (2026-03-22):** **A small HDHomeRun client validation gap was still letting empty bases degrade into path-only URLs:** `internal/hdhomerun` helper paths would synthesize `/discover.json` or `/lineup.json` for empty inputs, leaving callers to fail later with transport-layer noise instead of a local config error. Fixed `DiscoverURLFromBase`, `LineupURLFromBase`, `FetchDiscoverJSON`, and `FetchLineupJSON` to fail closed on empty bases, with regression coverage in `internal/hdhomerun/client_test.go`. Next depth: remaining defects should now be extremely narrow helper-level validation or nil-safety edges rather than surface-level contract issues.

**Latest (2026-03-22):** **A deck bootstrap helper was still rewriting hostname-only tuner targets to localhost:** `internal/webui.proxyBase` dropped hostnames when `TunerAddr` had no explicit port, so values like `localhost`, `tuner.internal`, or bare IPv6 literals silently became `127.0.0.1:5004`. Fixed it to preserve hostname-only and IPv6-without-port targets while still defaulting the port, with regression coverage in `internal/webui/webui_test.go`. Next depth: remaining defects should now be extremely narrow helper-level nil-safety or target-normalization edges rather than broader handler contract bugs.

**Latest (2026-03-22):** **A zero-value deck proxy edge was still failing open into the wrong error class:** `internal/webui.proxy` only rejected syntactically invalid `tunerBase` URLs, so an empty or partially initialized target fell through to a generic reverse-proxy `502 tuner unreachable` instead of failing locally as invalid configuration. Fixed it to require both scheme and host, with regression coverage in `internal/webui/webui_test.go`. Next depth: remaining defects should now be extremely narrow nil-safety or protocol-compatibility edges rather than local config validation drift.

**Latest (2026-03-22):** **A local-only access-control one-off was denying legitimate `localhost` requests:** the deck and operator loopback gates only recognized numeric loopback IPs, so requests arriving as `localhost:port` were treated as remote and rejected. Fixed `internal/webui/webui.go` and `internal/tuner/operator_ui.go` to treat the hostname `localhost` as local too, with regression coverage in `internal/webui/webui_test.go` and `internal/tuner/server_test.go`. Next depth: remaining defects should now be extremely narrow nil-safety or protocol-compatibility edges rather than access-control drift.

**Latest (2026-03-22):** **A deck auth-wrapper one-off was still returning the wrong surface contract under rate limiting:** `internal/webui.handleUnauthorized` always emitted JSON `429` when login attempts were blocked, even for browser page requests that normally stay in the login/redirect flow. Fixed it so scriptable `/api` and `/deck/*` paths remain JSON, while browser page requests preserve redirect semantics to `/login` with `Retry-After`, with regression coverage in `internal/webui/webui_test.go`. Next depth: remaining defects should now be down to very narrow nil-safety or protocol-compatibility one-offs rather than mixed browser/API contract drift.

**Latest (2026-03-22):** **The core gateway stream surface was still method-loose even after the wrapper cleanup:** `internal/tuner/gateway_servehttp.go` would still treat mutation verbs on `/stream/*` like normal reads instead of rejecting them, including `mux=hls` requests. Fixed the gateway to enforce `GET, HEAD` and to advertise `OPTIONS` as well when HLS mux CORS preflight support is enabled, with regression coverage in `internal/tuner/gateway_test.go`. Next depth: remaining defects should now be down to very narrow nil-safety or protocol-compatibility one-offs rather than lingering read-only method drift in the stream plane.

**Latest (2026-03-22):** **A gateway shim one-off was still open on the FFmpeg-packaged HLS target path:** `internal/tuner/gateway_hls_packager.go` served packaged playlists and segments under `mux=hls_ffmpeg_packager` but had no verb contract, so mutation verbs were treated like normal reads. Fixed the packaged target to enforce `GET, HEAD` with `Allow`, with direct regression coverage in `internal/tuner/gateway_hls_packager_test.go`. Next depth: remaining defects should now be down to very narrow nil-safety or wrapper-compatibility edge cases rather than read-only export shims.

**Latest (2026-03-22):** **Two more standalone export handlers were still method-loose, and one unrelated Plex compile drift surfaced behind them:** `internal/tuner/m3u.go` and `internal/tuner/xmltv.go` were still serving `live.m3u` and `guide.xml` as read-only exports without any verb gating, so mutation verbs were treated like normal reads. Fixed both to enforce `GET, HEAD` with `Allow`, added regression coverage in `internal/tuner/m3u_test.go` and `internal/tuner/xmltv_test.go`, and repaired unrelated `internal/plex/logs.go` compile drift by hoisting the log-aggregation helper type back to package scope so verification could build again. Next depth: remaining defects should now be down to even narrower compatibility or nil-safety one-offs rather than standalone export wrappers.

**Latest (2026-03-22):** **Reverse-engineering tooling + evidence capture for Plex Live TV access shipped:** added repo commands to inspect Plex SQLite (`plex-db-inspect`), PMS Live TV state (`plex-api-inspect`), arbitrary PMS/plex.tv requests (`plex-api-request`), PMS logs for discovered/undocumented Live TV endpoints (`plex-log-inspect`), and direct share-clamp reproduction (`plex-share-force-test`). Live evidence from a real Plex account/server currently shows: local DB injection controls tuner/DVR/provider rows, but non-Home Live TV visibility is enforced in plex.tv shared-server metadata (`allowTuners`) and plex.tv clamps non-Home share recreation back to `allowTuners=0` even when the request asks for `1`. Next step if this line continues: mine additional real endpoints from PMS logs / client traffic and test whether any other plex.tv or client-specific mutation path can alter non-Home tuner entitlement.

**Latest (2026-03-22):** **More plex.tv control-plane detail established:** `api/v2/shared_servers/<share-id>` is now confirmed as a richer row-level share object with `GET/POST/DELETE` semantics, while `PUT/PATCH` are rejected. Crucially, row-level `POST` is a real mutator for library membership/share shape, but it still leaves `allowTuners=0` for a non-Home invited user even when `settings.allowTuners=1` is requested. Separately, `api/v2/home/users` is read-only from this account (`allow: OPTIONS, GET, HEAD`) and per-user home rows expose only `DELETE`, so an obvious promote-to-Home write path has not yet surfaced. Next depth: keep future probes read-mostly unless necessary, and prioritize passive discovery from smart-TV/client traffic or PMS logs over additional share mutations.

**Latest (2026-03-22):** **Real-client browse capture harness added for the next reverse-engineering step:** `scripts/plex-client-browse-capture.sh` now wraps the TV browse window with pre/post PMS and plex.tv snapshots, periodic polls of `/status/sessions`, `/livetv/dvrs`, `/media/providers`, and plex.tv shared-server state, plus PMS log byte slicing so only the new browse-window log lines are kept. Immediate next step is operational, not code: start the capture, browse on the smart TV, stop the capture, then inspect the sliced PMS logs and poll deltas for the exact client-triggered request paths.

**Latest (2026-03-22):** **A leftover compatibility shim bug was still open in the standalone HDHR/probe handlers:** `internal/tuner/hdhr.go` and `internal/probe/probe.go` served `discover.json`, `lineup_status.json`, `lineup.json`, and `device.xml` as read-only surfaces but had no method gating at all, so mutation verbs were handled like normal reads. Fixed those wrappers to enforce `GET, HEAD` with `Allow`, and added direct regression coverage in `internal/tuner/hdhr_test.go` and `internal/probe/probe_test.go`. Next depth: remaining defects should now be down to even narrower compatibility or nil-safety one-offs rather than ungated read-only surfaces.

**Latest (2026-03-22):** **The last verification run exposed two separate tail items: one real tuner bug and one unrelated repo-drift repair:** fixed `internal/tuner/server.go` so the remaining runtime/event-hooks/active-streams/guide-lineup-match/history JSON endpoints use JSON `405` responses instead of plain text, and fixed unrelated `internal/plex` test harness drift by restoring the internal share-server helper path, making `plexHTTPClient` swappable in tests again, and adding the missing `encoding/xml` import in `inspect_test.go` so `./scripts/verify` could return green. Next depth: remaining defects should now be extremely sparse and likely limited to narrow compatibility quirks rather than more HTTP-contract cleanup.

**Latest (2026-03-22):** **Another tiny JSON `405` seam was still hiding in the dedicated guide diagnostics helpers:** `internal/tuner/guide_health.go` already returned JSON on success and ordinary failures, but its unsupported-method path still used the plain-text helper. Fixed `guide/health.json`, `guide/doctor.json`, and `guide/aliases.json` to return JSON `405` with `Allow`, with regression coverage in `internal/tuner/server_test.go`. Next depth: the remaining search space is now narrower than the last JSON 405 cluster and likely limited to isolated compatibility quirks.

**Latest (2026-03-22):** **One more machine-facing contract cluster was still open on `405` paths, not success/error payloads:** several operator JSON surfaces in `internal/tuner/server.go` already had the right method gates, but their unsupported-method path still used the plain-text helper. Fixed the remaining programming, virtual-channel, recording, report/debug, ghost-hunter, and operator-action JSON handlers to return JSON `405` with `Allow` instead of text/plain, with regression coverage in `internal/tuner/server_test.go`. Next depth: what remains now should be even narrower than the recent wrapper/protocol cleanup, because the leftover JSON 405 drift is largely burned down.

**Latest (2026-03-22):** **Another redirect-semantics bug was still present in deck auth redirects:** unauthenticated browser requests were still sent to `/login` with `303 See Other`, which can rewrite `HEAD` into `GET` even though the login page now supports `HEAD`. Fixed by preserving `GET, HEAD` with `307 Temporary Redirect` while keeping `303` for non-safe methods, with regression coverage in `internal/webui/webui_test.go`. Next depth: the remaining search space is now extremely sparse and likely limited to protocol nits or narrow compatibility assumptions.

**Latest (2026-03-22):** **The slash-canonical redirect wrappers had a subtle method-rewrite bug for `HEAD`:** bare `/api`, `/ui`, and `/ui/guide` were using `303 See Other`, which can turn `HEAD` follow-ups into `GET` even though those surfaces now explicitly allow `HEAD`. Fixed by switching those read-only redirect entrypoints to `307 Temporary Redirect`, with regression coverage in `internal/webui/webui_test.go` and `internal/tuner/server_test.go`. Next depth: the remaining defects are now likely even narrower than the protocol oddballs we just burned down.

**Latest (2026-03-22):** **Another small browser/protocol mismatch was in the deck login page:** `internal/webui.login` still rejected `HEAD` even though its read-side is just the HTML login form. Fixed by treating `HEAD` like `GET` and expanding the `405 Allow` contract to `GET, HEAD, POST`, with regression coverage in `internal/webui/webui_test.go`. Next depth: the remaining search space is now mostly tiny browser/protocol or compatibility oddballs, not missing whole surface contracts.

**Latest (2026-03-22):** **Another small protocol mismatch was in the operator HTML pages:** `internal/tuner/operator_ui.go` treated `/ui/` and `/ui/guide/` as GET-only even though they are read-only HTML surfaces like the deck root and other export pages, so `HEAD` unnecessarily failed there. Fixed by allowing `GET, HEAD` and updating the `405 Allow` contract accordingly, with regression coverage in `internal/tuner/server_test.go`. Next depth: the remaining search area is now down to tiny wrapper/protocol oddballs rather than anything clustered by subsystem.

**Latest (2026-03-22):** **A more substantive deck API wrapper bug turned up on bare `/api`:** the redirect entrypoint always issued `303 See Other` regardless of method, so `POST /api` could be silently rewritten into `GET /api/` instead of being rejected as an invalid API method/path combination. Fixed by moving `/api` into a dedicated handler that redirects only `GET, HEAD` and returns JSON `405` for mutation verbs, with regression coverage in `internal/webui/webui_test.go`. Next depth: the remaining bugs now look like tiny protocol or compatibility oddballs, not leftover HTTP-surface clusters.

**Latest (2026-03-22):** **Another deck wrapper inconsistency turned up in the localhost-only gate:** `internal/webui.localhostOnly` already treated `/api/*` and `*.json` as machine-facing JSON surfaces, but it still missed bare `/api`, so remote-denied requests to that scriptable redirect entrypoint fell back to plain-text `403`. Fixed by treating `/api` the same as `/api/*` and adding regression coverage in `internal/webui/webui_test.go`. Next depth: the remaining search space is now mostly tiny wrapper or compatibility oddballs, not another recurring family.

**Latest (2026-03-22):** **A deck browser-surface one-off was still open on the root page and static assets:** `internal/webui.index`, `assetCSS`, and `assetJS` had no method contract, so they would answer mutation verbs like normal reads instead of behaving as browser-only GET/HEAD resources. Fixed by enforcing `GET, HEAD` with plain `405`/`Allow` responses and adding regression coverage in `internal/webui/webui_test.go`. Next depth: keep checking the last standalone wrapper and compatibility edges, because the remaining bugs are no longer surfacing as families inside the main tuner handlers.

**Latest (2026-03-22):** **Another standalone Xtream compatibility edge was still open on `player_api.php`:** `internal/tuner/server_xtream.go` treated the read-only Xtream API like a generic handler and accepted mutation verbs instead of rejecting them, even though the surface is JSON read-only. Fixed by enforcing `GET` with JSON `405` responses and adding regression coverage in `internal/tuner/server_test.go`. Next depth: keep checking the remaining standalone wrappers and compatibility shims that sit outside the main `server.go` sweep, because that is where the leftover one-offs are still hiding.

**Latest (2026-03-22):** **A remaining machine-facing error-contract oddball was in the operator guide preview JSON wrapper:** `internal/tuner/operator_ui.go` still used `http.Error` for `ui/guide-preview.json` method rejection, so the endpoint degraded to plain text on `405` even though it is JSON on success and other failures. Fixed by routing the `405` path through `writeMethodNotAllowedJSON` and adding regression coverage in `internal/tuner/server_test.go`. Next depth: remaining bugs now look like genuinely isolated wrappers or compatibility edges rather than any still-repeating cluster.

**Latest (2026-03-22):** **Another leftover read-only cluster was still open in operator preview/detail/report helpers and `device.xml`:** `internal/tuner/server.go` still let mutation verbs hit `programming/browse`, `programming/harvest/assist`, `programming/channel-detail`, `programming/preview`, `virtual-channels/preview`, `virtual-channels/schedule`, `virtual-channels/channel-detail`, `recordings/recorder-report`, `recordings/rule-preview`, `recordings/history`, and `device.xml` even though they are pure read-only surfaces. Fixed by enforcing `GET` on the operator JSON endpoints and `GET, HEAD` on `device.xml`, with regression coverage in `internal/tuner/server_test.go` and `internal/tuner/ssdp_test.go`. Next depth: the remaining space is now down to isolated oddballs like less-used auth/proxy/device helper edges, not another obvious read-only sweep.

**Latest (2026-03-22):** **Another sparse read-only contract cluster was still open on public status and guide-report surfaces:** `internal/tuner/server.go` and `internal/tuner/guide_health.go` still let mutation verbs hit `healthz`, `readyz`, `guide/epg-store.json`, `guide/health.json`, `guide/doctor.json`, `guide/aliases.json`, `guide/highlights.json`, `guide/capsules.json`, and `guide/policy.json` even though they are pure read-only/public status surfaces. Fixed by enforcing `GET, HEAD` or `GET` with proper `Allow` headers and adding regression coverage in `internal/tuner/server_test.go`. Next depth: keep searching for genuinely isolated one-offs in leftover public/helper endpoints, because the broad repeated classes now look exhausted.

**Latest (2026-03-22):** **One more single-surface drift bug turned up in the channel intelligence plane:** `internal/tuner/serveChannelDNAReport` was the only sibling channel-report endpoint still bypassing operator-access checks, so `/channels/dna.json` remained remotely readable even though `/channels/report.json` and `/channels/leaderboard.json` were operator-only. Fixed by gating it through `operatorUIAllowed` and adding regression coverage in `internal/tuner/server_test.go`. Next depth: remaining issues, if any, are now likely very sparse one-offs without an obvious repeated pattern.

**Latest (2026-03-22):** **One more isolated read-only bug turned up in the virtual-channel export pair:** `internal/tuner/serveVirtualChannelGuide` and `serveVirtualChannelM3U` still accepted mutation verbs even though they are pure export surfaces. Fixed by enforcing `GET, HEAD` with `Allow` and adding regression coverage in `internal/tuner/server_test.go`. Next depth: the remaining search space is now mostly sparse one-offs rather than any still-obvious repeated class.

**Latest (2026-03-22):** **The final broad repeated class in the tuner plane was read-only report endpoints silently accepting mutation verbs:** multiple report/debug/operator surfaces in `internal/tuner/server.go` (`channels/*`, `autopilot/report`, `provider/profile`, `debug/*`, and `guide/lineup-match`) would answer `POST` exactly like `GET` instead of rejecting it. Fixed by enforcing `GET` with `Allow` across that cluster and adding regression coverage in `internal/tuner/server_test.go`. Next depth: broad classes now look mostly exhausted; remaining work is likely isolated one-off bugs rather than another repo-wide pattern.

**Latest (2026-03-22):** **The nil-safety sweep found another zero-value deck panic path in auth state:** `internal/webui.login` could still panic on failed or successful login attempts if `Run()` had not initialized `sessions` and `failedLoginByIP`, because the code wrote directly into nil maps. Fixed by lazily initializing those maps in the shared auth/session helpers and adding regression coverage in `internal/webui/webui_test.go`. Next depth: keep probing optional-subsystem seams, but the remaining nil-safety issues are now likely to be isolated edge helpers rather than broad wrapper patterns.

**Latest (2026-03-22):** **The nil-safety pass found a real deck panic path outside the main server flow:** `internal/webui.index` and `renderLogin` assumed `Run()` had already initialized `s.tmpl` and `s.loginTmpl`, so a zero-value or partially constructed deck server could still panic on page requests. Fixed by lazily initializing the embedded templates on demand and adding regression coverage in `internal/webui/webui_test.go`. Next depth: keep probing optional-subsystem seams for similar fail-open panics, but the remaining issues are now smaller and more isolated than the earlier broad contract-drift classes.

**Latest (2026-03-22):** **The next wrapper-level contract bug matched the earlier deck pattern on the operator side:** `internal/tuner/operator_ui.go` still returned plain-text `403` for remote-denied machine-facing operator endpoints because `operatorUIAllowed` did not distinguish HTML pages from JSON/action surfaces. Fixed by making the gate return real JSON errors for `*.json` and `/ops/` requests while leaving page responses plain text, with regression coverage in `internal/tuner/server_test.go`. Next depth: keep sweeping the remaining lesser-used wrappers and nil-safety seams, but the broad contract-drift families are now mostly exhausted.

**Latest (2026-03-22):** **The last major read-only wrapper bug was in WebDAV `OPTIONS`:** `internal/vodwebdav.NewHandler` delegated `OPTIONS` to `x/net/webdav`, which advertised writable verbs like `PUT`, `DELETE`, `LOCK`, and `MOVE` even though the surrounding surface is explicitly read-only and rejects those methods. Fixed by owning `OPTIONS` in the wrapper and returning the repo’s true read-only `Allow` set, with tighter regression coverage in `internal/vodwebdav/webdav_test.go`. Next depth: keep widening from here into any remaining top-level wrappers or proxy edges, but the broad protocol-contract list is now much smaller than the earlier JSON/read-only drift cluster.

**Latest (2026-03-22):** **The broader Xtream proxy sweep found the same read-only contract bug one layer deeper than the export endpoints:** `internal/tuner/serveXtreamLiveProxy` forwarded arbitrary methods upstream, and the movie/series proxies still hand-rolled the same `GET, HEAD` guard separately. Fixed by enforcing the shared read-only method contract across live, movie, and series proxy surfaces and adding regression coverage in `internal/tuner/server_test.go`. Next depth: keep pushing into the remaining read-only wrapper, especially WebDAV, for adjacent HTTP/WebDAV contract mismatches.

**Latest (2026-03-22):** **The broad protocol/read-only sweep found a real method-contract bug on the Xtream export surfaces:** `internal/tuner/server_xtream.go` accepted non-`GET`/`HEAD` requests for `m3u_plus` and `xmltv.php` as long as auth passed, even though both endpoints are read-only exports. Fixed by enforcing `GET, HEAD` with proper `Allow` headers and adding regression coverage in `internal/tuner/server_test.go`. Next depth: keep sweeping the remaining proxy/read-only wrappers like WebDAV and VOD/live proxy edges for adjacent HTTP contract mismatches.

**Latest (2026-03-22):** **The broad protocol sweep found the same HTTP standards bug on the simulated HDHomeRun TCP/HTTP shim:** `internal/hdhomerun/control.go` returned `405 Method Not Allowed` for known HTTP endpoints without an `Allow` header, even though it was acting as a real HTTP surface over the control socket. Fixed by carrying `Allow: GET, HEAD` through the raw response builder and adding regression coverage in `internal/hdhomerun/control_test.go` for both the helper and an end-to-end socket request. Next depth: keep pushing through the remaining protocol-specific/read-only wrappers like WebDAV and Xtream export/proxy edges for adjacent contract mismatches.

**Latest (2026-03-22):** **The broad sweep found a wrapper-level contract bug in the deck plane, not just another leaf handler:** `internal/webui.localhostOnly` returned plain-text `403` for every blocked request, so machine-facing `/api/*` and `/deck/*.json` surfaces still degraded to text whenever LAN access was disabled. Fixed by making the localhost-only gate return real JSON errors for JSON/API paths while keeping browser-page failures plain text, with regression coverage in `internal/webui/webui_test.go`. Next depth: keep widening into protocol-specific/read-only surfaces and other top-level wrappers where failure semantics are still shared but may drift by endpoint family.

**Latest (2026-03-22):** **The broader machine-facing sweep still had two literal JSON-through-plain-text bugs after the big `server.go` and deck passes:** the deck reverse-proxy entrypoint in `internal/webui/webui.go` returned `{"error":"invalid tuner base"}` through `http.Error`, and the virtual-channel stream surface in `internal/tuner/server.go` did the same for the “slot has no source” failure path. Fixed both to return real JSON with `application/json`, with regression coverage in `internal/webui/webui_test.go` and `internal/tuner/server_test.go`. Next depth: keep widening past JSON-contract cleanup into the remaining protocol/read-only surfaces and lesser-used proxy handlers where failure semantics may still drift by endpoint family.

**Latest (2026-03-22):** **The broader lower-half `server.go` sweep found the same JSON-success/plain-failure drift across programming, virtual-channel, recorder, and recording surfaces:** those handlers already returned structured JSON on success but still used `http.Error` for missing config, validation, not-found, and encode failures. Fixed by routing those lower-surface failures through the shared server JSON error writer and adding representative regression coverage in `internal/tuner/server_test.go`. Next depth: keep going even broader on the remaining structured-success helpers outside this seam, especially operator action/workflow endpoints, mux diagnostics, and proxy/read-only surfaces that still fall back to generic/plain-text failure helpers.

**Latest (2026-03-22):** **The broader `server.go` report/debug audit still had a large sibling cluster of JSON failure drift:** many machine-facing report surfaces (`epg-store`, guide highlights/capsules/policy, ghost/provider/stream/shared-relay reports, runtime/event-hooks/active-streams, and guide-lineup-match) already emitted JSON on success but still used `http.Error` on failure, so unavailable/encode paths degraded to `text/plain`. Fixed by centralizing a shared server-side JSON error writer and routing those report/debug failure paths through it, with regression coverage in `internal/tuner/server_test.go`. Next depth: continue on the remaining JSON-success/plain-failure seams lower in `server.go` for programming/virtual/recording/report helpers and any proxy/read-only surfaces still using generic helpers.

**Latest (2026-03-22):** **The broader JSON/proxy audit found the same machine-contract drift on the Xtream `player_api` surface:** `internal/tuner/serveXtreamPlayerAPI` returned structured JSON on success but still used `http.Error` for auth failure, unsupported action, missing series, and missing stream cases, so those replies degraded to `text/plain` despite looking like JSON. Fixed by centralizing a small Xtream JSON error writer and adding regression coverage in `internal/tuner/server_test.go`. Next depth: keep sweeping the remaining proxy-adjacent surfaces where successful responses are protocol-specific/structured but failure paths still use generic helpers, especially Xtream export/proxy and WebDAV/read-only edges.

**Latest (2026-03-22):** **The broader machine-facing JSON audit still had another sibling cluster after the guide diagnostics fix:** `internal/tuner/serveOperatorGuidePreviewJSON` and multiple `internal/webui` deck/auth helpers were returning plain-text fallback bodies on structured JSON error paths (invalid settings JSON, CSRF/auth rejection, and operator guide-preview unavailable/failure). Fixed by centralizing JSON error writers on those surfaces and adding regression coverage in `internal/tuner/server_test.go` and `internal/webui/webui_test.go`. Next depth: keep sweeping JSON/debug/proxy surfaces that still look structured on success but fall back to plain text on transport/proxy failures or validation errors, especially Xtream and proxy-adjacent handlers.

**Latest (2026-03-22):** **The broader non-normalization audit found another sibling contract bug across machine-facing JSON surfaces:** `internal/tuner/guide_health.go` returned plain-text error responses from `/guide/health.json`, `/guide/doctor.json`, and `/guide/aliases.json` on `503`/`502` failure paths, and `internal/webui.writeMethodNotAllowedJSON` still used `http.Error`, so deck JSON `405` responses advertised the wrong content type. Fixed by centralizing real JSON error writers on those paths and adding regression coverage in `internal/tuner/server_test.go` and `internal/webui/webui_test.go`. Next depth: keep widening through lesser-used JSON/debug/proxy surfaces for the same machine-contract drift, especially endpoints that look JSON on success but still fall back to plain-text or inconsistent status bodies on error.

**Latest (2026-03-22):** **The broader HTTP consistency audit still had a second tuner-only sibling cluster after the first `405` fix batch:** several operator/programming/recording/demo handlers in `internal/tuner/server.go` were still hand-rolling `405 method not allowed` responses without `Allow`, and the JSON-flavored ones were also silently degrading to `text/plain` because they delegated to `http.Error`. Fixed by centralizing plain-text and JSON `405` helpers, wiring the remaining GET-only, POST-only, GET/POST, and GET/HEAD surfaces through them, and adding regression coverage in `internal/tuner/server_test.go`. Next depth: continue the non-normalization audit on lesser-used HTTP/report surfaces for nil-safety, content-type drift, and status-code mismatches that still bypass the shared helpers.

**Latest (2026-03-22):** **The broader audit found a second bug class beyond URL drift: some HTTP surfaces were returning `405` without `Allow`, and the dedicated deck still had a process-panic path on password generation failure:** the shared deck telemetry/activity/settings/login/logout handlers and the tuner ghost-report/operator UI paths were sending method-not-allowed responses without the protocol hint header, and `internal/webui.mustGenerateDeckPassword` would panic the whole process if `crypto/rand` failed. Fixed by centralizing `Allow` headers on the covered `405` paths and degrading deck password generation to a deterministic fallback instead of panicking, with regression coverage in `internal/webui/webui_test.go` and `internal/tuner/server_test.go`. Next depth: keep working the remaining audit list for other status-code / header consistency issues and any latent nil-safety paths that were not part of the URL-normalization burn-down.

**Latest (2026-03-22):** **The Xtream indexer/helper chain still had one deeper normalization bug after the broader audit batches:** `fetchVODStreams`, `fetchSeries`, `fetchXtreamCategoryMap`, and `fetchSeriesInfo` were still trusting raw `apiBase` / `streamBase`, so whitespace- or multi-slash-padded values could internally generate malformed `////player_api.php` calls or broken VOD/series artwork and stream URLs even though the higher-level player API entrypoints were already normalized. In the same sweep, one last catalog helper (`streamURLsFromRankedBases`) was still using the old one-slash trim path. Fixed in `internal/indexer/player_api.go` and `cmd/iptv-tunerr/cmd_catalog.go`, with regression coverage in `internal/indexer/player_api_test.go` and `cmd/iptv-tunerr/cmd_catalog_test.go`. Next depth: continue from the audit list on non-normalization classes too, especially lesser-used HTTP/report surfaces for status-code consistency and latent nil-safety.

**Latest (2026-03-22):** **The catalog/provider-identity and adjacent consumer export/parsing surfaces still had one last normalization drift cluster during the broader audit:** `cmd_catalog` still rebuilt ranked stream URLs and provider identity keys with the old one-slash trim path, `internal/emby.effectiveXMLTVURL` still trusted raw `TunerURL`, `internal/hdhomerun.ParseDiscoverReply` still preserved whitespace from TLV URLs, and `internal/tuner.M3UServe` still only removed a single trailing slash from `BaseURL`. That meant equivalent providers could split into distinct identities during catalog winner ordering and lockout tracking, and whitespace-padded tuner/discovery bases could still leak malformed guide/stream URLs through M3U export, Emby registration fallback, or parsed HDHomeRun replies. Fixed with regression coverage in `cmd/iptv-tunerr/cmd_catalog_test.go`, `internal/emby/register_test.go`, `internal/hdhomerun/client_test.go`, and `internal/tuner/m3u_test.go`. Next depth: summarize the remaining audit list and keep burning down the unfixed collector/report/nil-safety candidates from that list in larger batches instead of one-off hunts.

**Latest (2026-03-22):** **Config, health, and ranked-provider collectors still had the old base-normalization drift after the helper sweep:** `internal/config.M3UURLsOrBuild`, `internal/health.CheckEndpoints`, and `internal/provider.RankedEntries` were still either stripping only one trailing slash or concatenating raw bases directly, so whitespace- or multi-slash-padded provider/tuner bases could still leak malformed `//get.php`, `//discover.json`, `//lineup.json`, `//guide.xml`, or `//player_api.php` paths through configuration- and diagnostics-driven flows even after sibling helpers were hardened. Fixed by reusing full trailing-slash trimming in those collectors, with regression coverage in `internal/config/config_test.go`, `internal/health/health_test.go`, and `internal/provider/probe_test.go`. Next depth: keep widening across remaining report/orchestration collectors that normalize bases locally instead of delegating to the hardened helper builders.

**Latest (2026-03-22):** **Shared guide/lineup helper builders still had base normalization drift after the command and tuner fixes:** `internal/guideinput.ProviderXMLTVURLWithSuffix` and `internal/tuner.providerXMLTVEPGURL` were only stripping a single trailing slash, and `internal/probe.Lineup` was not normalizing its base at all, so whitespace- or multi-slash-padded bases could still leak malformed `//xmltv.php` or `//stream?url=` links through guide intake and generic HDHR lineup helpers even after sibling surfaces were hardened. Fixed by centralizing full trailing-slash trimming in those helpers, with regression coverage in `internal/guideinput/guideinput_test.go`, `internal/tuner/epg_pipeline_test.go`, and `internal/probe/probe_test.go`. Next depth: keep sweeping remaining helper/build-function seams where already-normalized callers can still be undercut by a sibling helper that hand-rolls URL joins differently.

**Latest (2026-03-22):** **The CLI `probe` surface itself still had base URL normalization drift after the deeper provider-probe cleanup:** `handleProbe` was trimming provider bases in the wrong order (`TrimSpace(TrimSuffix(...))`), so a whitespace-padded base could retain a trailing slash and feed malformed `//get.php` paths into the Cloudflare-aware prep step even though the underlying provider probe helpers were already normalized. Fixed by centralizing probe-base normalization in `cmd/iptv-tunerr/cmd_core.go`, with regression coverage in `cmd/iptv-tunerr/main_test.go`. Next depth: keep widening across command/report helpers that collect provider bases locally before handing off to already-normalized lower layers.

**Latest (2026-03-22):** **The tuner-side Xtream export helpers still had whitespace-sensitive base URL drift after the broader provider/runtime normalization sweep:** `xtreamM3U`, `xtreamLiveDirectSource`, movie direct sources, and series episode direct sources were still building URLs from raw `Server.BaseURL` with only trailing-slash trimming, so surrounding whitespace could still leak malformed Xtream `xmltv.php`, `/live/...`, `/movie/...`, and `/series/...` URLs even after sibling Xtream/provider surfaces were normalized. Fixed by centralizing Xtream base normalization in `internal/tuner/server_xtream.go`, with regression coverage in `internal/tuner/server_test.go`. Next depth: keep widening through the remaining Xtream publishing and operator/report helpers for any builders that still bypass normalized base/host joins.

**Latest (2026-03-22):** **The `get.php` catalog fallback path still lagged the rest of the Xtream normalization work:** `catalogFromGetPHP` was trimming only trailing slashes, not surrounding whitespace, before building the fallback M3U URL, so sloppy provider base values could still break the `get.php` catalog path even after the sibling probe/runtime/player_api helpers were normalized. Fixed by tightening base normalization in `cmd_catalog.go`, with regression coverage in `cmd_runtime_test.go`. Next depth: keep widening through the remaining `xmltv.php` and provider-export builders so the rest of the Xtream URL family stops drifting the same way.

**Latest (2026-03-22):** **The provider probe family still had raw base URL drift even after the player_api cleanup in indexer/runtime:** `internal/provider` was still feeding raw base strings into alternate `get.php` probes, POST probes, and `ProbePlayerAPI`, so whitespace or trailing-slash provider URLs could still generate malformed upstream probe URLs even though the sibling indexer/runtime paths were normalized. Fixed by centralizing provider-base normalization inside `internal/provider/probe.go`, with regression coverage in `internal/provider/probe_test.go`. Next depth: keep widening across the remaining `get.php` and `xmltv.php` builders in catalog/guide/tuner paths so the entire Xtream URL family shares the same normalization contract.

**Latest (2026-03-22):** **The Xtream/player_api family still had raw base URL drift after the registration cleanup:** runtime health-check URL synthesis and the indexer’s player_api/live-stream helpers were still only partially trimming bases, so whitespace or trailing-slash provider URLs could produce malformed `player_api.php` requests or `//live/...` stream URLs even after the local registration/export surfaces were normalized. Fixed by centralizing API-base normalization in `internal/indexer/player_api.go` and tightening the runtime health-check builder in `cmd_runtime.go`, with regression coverage in `internal/indexer/player_api_test.go` and `cmd_runtime_test.go`. Next depth: keep widening across remaining provider/get.php/xmltv helpers for the same pattern where one upstream URL family is normalized but sibling endpoints still hand-roll base joins.

**Latest (2026-03-22):** **The Plex runtime registration command still had one trailing-slash stream URL leak after the guide URL cleanup:** lineup rows built for `plex.SyncLineupToPlex` were still using raw `baseURL + "/stream/" + channelID`, so a configured base ending in `/` could still push malformed `//stream/...` URLs into Plex even after discovery, M3U, and guide registration surfaces were normalized. Fixed by centralizing stream URL derivation through a trimmed helper in `cmd_runtime_register.go`, with regression coverage in `cmd_runtime_register_test.go`. Next depth: keep widening across registration/export helpers for the same pattern where guide URLs were normalized but sibling stream/data URLs still use raw concatenation.

**Latest (2026-03-22):** **Emby/Jellyfin server-host joins had the same trailing-slash drift as the tuner-base helpers:** registration and library helpers under `internal/emby` were still building API URLs from raw `cfg.Host + "/..."`, so media-server hosts ending in `/` could produce malformed `//LiveTv/...` and `//Library/...` requests that miss routes. Fixed by centralizing host URL joining and trimming trailing slashes before appending API paths, with regression coverage in `internal/emby/register_test.go` and `internal/emby/library_test.go`. Next depth: keep widening through remaining operator/integration helpers for raw host/base concatenations that bypass the normalized join helpers now present in sibling packages.

**Latest (2026-03-22):** **Discovery producers still had trailing-slash base/lineup drift even after the client-side hardening:** the standalone HDHomeRun simulator, the simulated control server `discover.json`, and the main tuner HDHR discovery surface were still advertising raw trailing-slash `BaseURL` values and building `LineupURL` via direct concatenation, while the main tuner `lineup.json` used that raw base to produce malformed `//stream/...` entries. Fixed by normalizing discovery `BaseURL` and reusing the shared lineup URL helper across those producers, with regression coverage in `internal/hdhomerun/discover_test.go`, `internal/hdhomerun/control_test.go`, and `internal/tuner/hdhr_test.go`. Next depth: keep widening across remaining discovery/registration producers for any direct path concatenations that ignore the shared helper functions already present in sibling layers.

**Latest (2026-03-22):** **The guide-URL trailing-slash bug also existed in the Plex core registration layer:** `internal/plex/dvr.go` built guide URLs from raw `BaseURL + "/guide.xml"` in both DVR API registration and DB registration, so trailing-slash bases could produce malformed `//guide.xml` references inside Plex even after the command-layer registration helper was normalized. Fixed by centralizing guide URL derivation in the Plex package and trimming stored DVR/XMLTV base URLs before writing them, with regression coverage in `internal/plex/dvr_test.go`. Next depth: keep widening across other registration/watchdog helpers for cases where command wrappers are normalized but underlying package helpers still concatenate paths directly.

**Latest (2026-03-22):** **The same trailing-slash guide-URL drift also existed in Plex runtime registration helpers:** `cmd_runtime_register.go` built `baseURL + "/guide.xml"` directly for the setup banner and DVR watchdog, so a base URL ending in `/` produced `//guide.xml` there even after the Emby/Jellyfin registration path was normalized. Fixed by centralizing guide URL derivation through a trimmed helper, with regression coverage in `cmd_runtime_register_test.go`. Next depth: keep widening through registration/watchdog/report helpers for other path concatenations that bypass the normalized base-URL helpers already present elsewhere.

**Latest (2026-03-22):** **Another shim-level URL drift showed up outside HDHomeRun:** `internal/emby.Config.effectiveXMLTVURL()` blindly appended `"/guide.xml"` to `TunerURL`, so configs ending in `/` produced malformed `//guide.xml` listing-provider paths during Emby/Jellyfin registration. Fixed by trimming trailing slashes before appending the guide path, with regression coverage in `internal/emby/register_test.go`. Next depth: keep widening across registration/report helpers for small URL-shape bugs and other places where callers already supply normalized base URLs but sibling helpers append paths naively.

**Latest (2026-03-22):** **The hardened HDHomeRun lineup client surfaced another sibling consumer drift in Plex harvest:** `internal/plexharvest.fetchLineup` was still decoding `/lineup.json` directly as a raw array instead of using the shared HDHomeRun client contract, so lineup-harvest/report paths could still fail on object-shaped payloads with a `Channels` field even after `internal/hdhomerun` was fixed to accept both forms. Fixed by routing Plex harvest through `hdhomerun.FetchLineupJSON` and mapping the resulting channels into `HarvestedChannel` rows, with regression coverage in `internal/plexharvest/plexharvest_test.go`. Next depth: keep widening through other report/import consumers that bypass hardened helper layers and decode device payloads directly.

**Latest (2026-03-22):** **HDHomeRun HTTP client/import code had two sibling compatibility bugs after the simulator fixes:** `internal/hdhomerun.FetchLineupJSON` only decoded `lineup.json` as an object with a `Channels` field even though HDHomeRun-compatible `lineup.json` commonly arrives as a top-level array, and `FetchDiscoverJSON` trusted remote `BaseURL` / `LineupURL` fields verbatim, leaving follow-on scan/import calls with empty endpoints when devices omitted them. That meant `hdhr-scan` and hardware-lineup merge paths could reject valid lineup payloads or lose the usable base URL the caller already had. Fixed by teaching `LineupDoc` to unmarshal both JSON array and object forms, and by falling back to the requested base URL plus `/lineup.json` when discovery JSON omits those fields. Added regression coverage in `internal/hdhomerun/client_test.go`. Next depth: keep widening across other shim/client boundaries where one side already changed shape and sibling consumers may still assume the old encoding or skip obvious fallback data the caller already knows.

**Latest (2026-03-22):** **HDHomeRun simulator TCP control plane had deeper protocol breakage beyond the earlier HTTP payload fixes:** the socket sniffing path dropped the first 4 bytes of every binary HDHomeRun packet after deciding a connection was not HTTP, GET requests serialized without the property-name bytes, decoded GET/SET names still kept their trailing NULs so property lookups missed, and the HTTP sniffer only recognized `GET`/`POST`/`HEAD`, which prevented other verbs from reaching the existing `405` handling. Fixed by preserving the sniffed binary header, serializing GET TLVs correctly, trimming decoded TLV strings, and broadening HTTP-method detection at the socket layer. Added regression coverage in `internal/hdhomerun/control_test.go`. Next depth: keep widening across non-discovery simulator/runtime surfaces for other protocol-shape mismatches, especially places where sniffing/parsing logic diverges from sibling handlers.

**Latest (2026-03-22):** **HDHomeRun simulator HTTP surface had two protocol bugs beyond the earlier identity drift:** `internal/hdhomerun` was serving `/lineup.json` with lineup-status-shaped JSON instead of a lineup array, and its unknown HTTP paths returned a literal `"404 Not Found"` body with an `HTTP/1.1 200 OK` status line. Fixed by splitting `lineup.json` and `lineup_status.json` into distinct payloads and making the HTTP shim return a real `404 Not Found` status for unknown paths. Added regression coverage in `internal/hdhomerun/control_test.go`. Next depth: keep auditing cross-surface identity/status documents for the same “one endpoint honors config, sibling endpoint hardcodes defaults or hand-rolls serialization unsafely” class, while continuing to widen into other public/debug/runtime seams.

**Latest (2026-03-22):** **Guide diagnostics no longer panic when XMLTV is absent:** `/guide/health.json`, `/guide/doctor.json`, and `/guide/aliases.json` were still dereferencing `s.xmltv` unconditionally even though nearby guide surfaces correctly return `503` when guide support is unavailable. Fixed all three handlers to fail closed with `{"error":"xmltv unavailable"}` and added regression coverage in `internal/tuner/server_test.go`. Full `./scripts/verify` is green. Next depth: keep auditing public guide/lineup intelligence endpoints for other nil-safety or silent-wrong-behavior bugs, not just access-control leaks.

**Latest (2026-03-22):** **Broader audit surfaced four more concrete config/debug regressions:** the debug bundle leaked numbered secrets because redaction only matched exact suffixes like `_PASS`; its live fetch path also used unbounded `http.Get` and could hang indefinitely. Separately, guide-input/provider EPG allowlisting built unsuffixed `xmltv.php` URLs even when `IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX` was configured, so repair/guide-health loads could self-block, and the supervisor `envFiles` loader failed to unquote shell-style values while the top-level `.env` loader did. In the same pass, the shared HTTP client pool was found to read `IPTV_TUNERR_HTTP_MAX_IDLE_CONNS*` only during package init, which meant `.env` overrides were silently ignored because `.env` loads later in `main`. All five fixes now have regression coverage across `cmd_debug_bundle`, `internal/guideinput`, `internal/supervisor`, and `internal/httpclient`, and full `./scripts/verify` is green. Next depth: continue the repo-wide audit for more silent wrong-behavior paths, especially operator/debug surfaces and env-driven runtime seams.

**Latest (2026-03-22):** **Operator programming/virtual-channel reads had the same repeat leak pattern as the earlier ruleset endpoints:** several GET handlers under `/programming/*` and `/virtual-channels/*` were still remotely readable even though they exposed recipe paths, writable-state flags, selected include/exclude state, curated-vs-raw decisions, backup topology, or virtual-channel rule file locations. Fixed by requiring `operatorUIAllowed` for the read side of programming categories, browse, channel detail, channels/order/backups, programming preview, and virtual-channel preview/schedule/detail. Added/updated regression coverage in `internal/tuner/server_test.go`, and both focused race tests plus full `./scripts/verify` are green. Next depth: continue the remaining audit on non-programming report endpoints and cross-check whether any public JSON surface still returns local file paths or privileged workflow state without the same operator gate.

**Latest (2026-03-22):** **More public report surfaces were still leaking privileged state:** `/autopilot/report.json` exposed the Autopilot state-file and host-policy-file paths, `/plex/ghost-report.json` exposed PMS session identifiers/player addresses plus recovery guidance, and `/channels/report.json` plus `/channels/leaderboard.json` returned `primary_stream_url` values from live channels, which can carry raw upstream provider URLs/tokens. All four endpoints are now operator-only, with regression coverage in `internal/tuner/server_test.go` and `internal/tuner/ghost_hunter_test.go`. Full `./scripts/verify` is green again. Next depth: only keep auditing public guide/lineup intelligence endpoints if they expose more than ordinary lineup/guide metadata; the obvious local-path and raw-upstream leak class is now largely closed.

**Latest (2026-03-22):** **Operator/config audit found stale preview semantics plus read-side config leaks:** `/entitlements.json` exposed the full Xtream entitlements ruleset, including downstream usernames/passwords and the configured users-file path, to any remote caller even though writes were operator-only. The same read/write mismatch existed on the direct ruleset endpoints for programming recipes, virtual-channel rules, and recording rules, all of which leaked local file paths and config state remotely. In the same pass, `/programming/preview.json` was found to mix two different data sources: after external recipe-file edits it could report stale lineup/count/bucket data from the in-memory curated lineup instead of the just-reloaded recipe, while backup-group analysis intentionally stayed uncollapsed. Fixed by gating those config GETs behind `operatorUIAllowed`, adding regression coverage for the new read-side access control, and making `programming/preview.json` reload the current recipe file for lineup/count/buckets while still using the uncollapsed preview for backup-group analysis. Full `./scripts/verify` is green again. Next depth: keep auditing remaining operator/report endpoints for the same leak/misreport patterns and keep pushing into other env-driven/runtime correctness seams.

**Latest (2026-03-22):** **Broader runtime audit found and fixed post-refresh state drift plus runtime-snapshot mutation:** `run` scheduled refreshes could switch to a new winning provider catalog while leaving the live server’s `Gateway` and `XMLTV` instances on the old provider credentials/base, so fallback auth, provider XMLTV fetches, and `/debug/runtime.json` could drift from the refreshed catalog winner. In the same area, `/debug/runtime.json` mutated the shared `RuntimeSnapshot` maps in-place while serving requests. Added synchronized provider-credential accessors on `Gateway`, synchronized provider-identity accessors on `XMLTV`, `Server.UpdateProviderContext`, cloned runtime snapshots before serving, and regression coverage in `internal/tuner/server_test.go`. Focused package tests, race tests, and full `./scripts/verify` are green. Next depth: continue broad audit only if the user wants more hunting outside provider/runtime state coherence.

**Latest (2026-03-22):** **Repo-wide audit pass found and fixed two concrete regressions:** `go test ./...` was not actually stable because `TestFetchCatalog_FallsBackToGetPHPOnPlayerAPIForbidden` mutated shared hit maps from concurrent handler requests without synchronization, so the suite could fail with `fatal error: concurrent map writes`; the test now locks those counters. Separately, the `probe` CLI path collapsed duplicate provider base URLs through a `map[base]entry`, so multi-account configs that share one host were probed/ranked with the wrong credentials; `handleProbe` now carries per-entry base/user/pass tuples end-to-end and has regression coverage for same-host multi-account probing. Full `./scripts/verify` and focused `go test -race -count=1 ./cmd/iptv-tunerr ./internal/provider ./internal/indexer ./internal/tuner` are green. Next depth: continue broader audit only if the user wants more hunting beyond these fixed findings.

**Latest (2026-03-22):** **Provider lockout visibility shipped:** provider probe now classifies unrecovered `player_api` `401/403` as `access_denied`, `RankedEntries` logs an explicit `provider lockout/bot-filter suspected` warning when every credential is denied/rate-limited, and `fetchCatalog` now returns/logs the same diagnosis when all `player_api` and `get.php` attempts fail in that class. Added regression coverage in `internal/provider/probe_test.go`, `internal/indexer/m3u_test.go`, and `cmd/iptv-tunerr/main_test.go`. Verification is green on targeted packages; next step is full `./scripts/verify` and then continue the provider regression pass itself, not just observability.

**Latest (2026-03-22):** **CF auto-boot no longer opens the desktop browser by default, and the shared-client bootstrap path is now live:** `PrepareCloudflareAwareClient` now upgrades env-configured shared jars to the persistent jar implementation required by the CF bootstrapper, preserving existing host cookies so `IPTV_TUNERR_CF_AUTO_BOOT=true` actually runs on probe/index paths. Real-browser fallback is now opt-in behind `IPTV_TUNERR_CF_REAL_BROWSER_FALLBACK=true`; default behavior stays headless-only and logs that desktop-browser fallback is disabled. Live validation: single-account `player_api` indexing succeeds with a single configured provider (`11657` live channels) when numbered provider vars are exported as empty strings, and multi-account indexing succeeds with the full env (`50173` live channels). Remaining live gap: `get.php` is still provider-blocked (`884`/timeout), so continue reverse-engineering there after verify/commit/build.

**Latest (2026-03-22):** **Catalog ingest is now explicitly `player_api`-first, with `get.php` demoted to true backup-only:** `fetchCatalog` no longer tries per-provider `get.php` fallbacks while untried `player_api` candidates remain. Ranked and direct provider loops now exhaust `player_api` options first; `get.php` is attempted only if no provider indexed successfully via API. Added regressions proving a later successful ranked/direct `player_api` candidate suppresses `get.php` entirely. Live validation after the change: single-account index succeeded on the known-good provider with `25386` live channels after free-source merge, and multi-account index succeeded with `63603` live channels after free-source merge. This is the new steady-state posture while upstream `get.php` remains CF-blocked.

**Latest (2026-03-22):** **Player-api review found and fixed two follow-on bugs in runtime routing:** when a later ranked provider won indexing, `applyStreamVariants` could still put an earlier failed provider first in `StreamURL`/`StreamURLs`; that now prioritizes the winning provider first. Separately, `run` startup health checks could probe the winning base URL with the primary provider credentials instead of the winning provider credentials; `runtimeHealthCheckURL` now uses the effective winning base/user/pass. Full `./scripts/verify` is green, and live single-account plus multi-account index runs still succeed with the current `.env`.

**Latest (2026-03-22):** **Single-credential provider fallback regression hardened:** added `TestFetchCatalog_SingleCredentialDoesNotRetryGetPHPOnPlayerAPIFailure` in `cmd/iptv-tunerr/main_test.go` to pin the 1:1 direct/`get.php` behavior for a single credential. It asserts the single-credential path makes one probe + one direct `player_api` attempt and exactly one fallback `get.php` attempt, then fails cleanly (no hidden retries). Verification is green on `./cmd/iptv-tunerr` and full `./scripts/verify`.

**Latest (2026-03-22):** **Provider fallback churn fix merged:** eliminated duplicate `get.php` retry attempts for direct `player_api` 403 cases by tracking attempted provider credentials in `fetchCatalog` and skipping redundant fallback passes. Added regression test `TestFetchCatalog_DoesNotRetryGetPHPAfterDirectForbiddenFallback` proving each credential gets exactly one `get.php` attempt after direct player/index failures. Verification is green via `go test ./...` and `./scripts/verify`. Next: continue remaining `CMP-*` slices and live provider matrix.

**Latest (2026-03-22):** **CMP-001 streaming-retry slice now active and validated:** `walkStreamUpstreams` now retries bounded upstream concurrency-limit responses (e.g. 423/429/458/509) on the same URL with exponential backoff (`IPTV_TUNERR_UPSTREAM_RETRY_LIMIT` + `IPTV_TUNERR_UPSTREAM_RETRY_BACKOFF_MS`, `Retry-After` aware) before trying backups; non-limit failures still fail over immediately. Added tests for fallback behavior (`TestGateway_stream_concurrencyLimitRetriesThenFallback`) and non-limit immediate fallback (`TestGateway_stream_nonConcurrencyErrorsStillFallbackImmediately`), and normalized existing concurrency/account test expectations by pinning retry limit to 0 in those counter-assertions. Next: verify docs/CLI references are in sync (already noted in `.env.example` + `cli-and-env-reference`) and complete remaining CMP-* slices in `EPIC-operator-completion.md`.

**Latest (2026-03-22):** **Single completion umbrella activated:** aligned planning to your instruction that everything should proceed except public admin-plane + Postgres/shared-writer scope unless explicitly requested. **`docs/epics/EPIC-operator-completion.md`** now acts as the active global umbrella for LP/PM/LH/PAR/ACC/HR/INT/REC/VODX follow-through (`CMP-*`), and both `docs/index.md` and `memory-bank/work_breakdown.md` were updated to wire that lane into the single source of truth. Next depth is reducing active `CMP-*` slices to implementation and proof, not backlog churn.

**Latest (2026-03-21):** **Docs sync after `v0.1.28` is complete:** README plus the docs/reference/how-to indexes now reflect the shipped Programming Manager, diagnostics workflows, Xtream/virtual outputs, release-readiness gate, and macOS/Windows host-validation paths. Immediate next depth returns to the release-readiness weak cells that still depend on external/live environments rather than stale docs.

**Latest (2026-03-21):** **Release-proofing pass is now explicit:** added a real release-readiness matrix and a runnable `scripts/release-readiness.sh` gate so "can we prove it before release?" has a concrete answer per feature family. The gate layers full repo verify, focused parity/programming/provider/WebDAV suites, and optional host proofs (`--include-mac`, `--include-windows-package`) instead of pretending every surface is equally proven. Next depth: keep shrinking the weak cells in that matrix, especially broader live/shared-output proof and real Windows-host validation.

**Latest (2026-03-21):** **Mac host proof now covers more than startup and WebDAV:** the bare-metal macOS lane now exercises Xtream `get.php`, `xmltv.php`, virtual-channel live/short-EPG/schedule/playback, plus the dedicated deck and startup contract on a real host, and `./scripts/release-readiness.sh --include-mac` passed with that expanded scope. The remaining weak cells are increasingly the ones that truly require external provider/client environments, not missing host coverage on the surfaces we control.

**Latest (2026-03-21):** **The dedicated deck is no longer an indirect smoke surface:** `scripts/ci-smoke.sh` now starts a real `run --skip-index --skip-health` instance with the web UI enabled, performs `/login`, reuses the session cookie against `/api/debug/runtime.json`, saves `/deck/settings.json` with the extracted CSRF token, and fetches `/api/ops/workflows/diagnostics.json`. `scripts/release-readiness.sh` now includes the `internal/webui` auth/proxy suite too. Next depth: keep shrinking the remaining non-Windows weak cells, mainly real live provider/client variance rather than unaudited control-plane basics.

**Latest (2026-03-21):** **Dead HLS remux startup now has binary proof too:** `scripts/ci-smoke.sh` now forces a real temp binary through a hung `ffmpeg` startup path for a same-host HLS channel, then asserts the request still delivers bytes and `/debug/stream-attempts.json` records `final_mode: hls_go`. `scripts/release-readiness.sh` now carries the matching focused dead-remux fallback tests so this class is no longer only code-proven. Next depth: keep shrinking the remaining non-Windows weak cells, especially broader live provider/client proof rather than more synthetic-path gaps.

**Latest (2026-03-21):** **Provider-account rollover now has binary proof too:** `scripts/ci-smoke.sh` no longer leaves provider-account pooling at the "indirect" tier. It now drives three overlapping channels that each offer the same three Xtream-path credential variants, runs the temp binary with `IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT=1`, and asserts `/provider/profile.json` exposes three distinct active account leases while the requests overlap. Next depth: keep shrinking the remaining non-Windows weak cells, especially broader live provider/client proof and richer host proof where available.

**Latest (2026-03-21):** **Shared-output proof is stronger now:** `scripts/ci-smoke.sh` no longer treats shared HLS relay reuse as unit-test-only. It now spins up a throttled local HLS upstream, runs two same-channel consumers against a real temp binary with ffmpeg disabled, and asserts both `/debug/shared-relays.json` state and `X-IptvTunerr-Shared-Upstream: hls_go` on the joined client. Next depth: keep shrinking the remaining weak cells, especially real Windows-host proof and broader live provider/client validation.

**Latest (2026-03-21):** **Broader parity pass is still moving, not stopping at one endpoint:** downstream Xtream output now has compact guide exposure too, with `player_api.php` answering `get_short_epg` and `get_simple_data_table` for real live and virtual channels through the existing guide/virtual-schedule pipeline. This closes another real `PAR-004` gap and keeps the broader pass aimed at deeper downstream parity plus richer shared-output/operator flows rather than isolated helpers. Next depth: keep broadening client-facing parity/output behavior and shared multi-consumer reuse, not just sidecar JSON.

**Latest (2026-03-21):** **Broader parity pass pushed the Xtream publishing side further too:** the downstream Xtream surface now exports the entitled lineup as `get.php` (M3U) plus `xmltv.php` (XMLTV), not just `player_api.php` actions and proxy URLs. That means the same server-backed entitlement model now covers M3U, XMLTV, compact EPG, live/VOD/series actions, and virtual-channel publishing instead of fragmenting exports across separate paths. Next depth: keep widening shared-output/multi-consumer behavior and richer operator compare/apply flows rather than stopping at format parity.

**Latest (2026-03-21):** **Programming backup preference is now a real operator/apply flow:** exact-match sibling groups no longer have to live with whichever source happened to ingest first. The saved recipe can now carry `preferred_backup_ids`, `/programming/backups.json` can mutate that preference directly, collapsed preview/output rows honor it, the deck exposes one-click “Prefer <source>” actions, and release smoke now proves the preferred sibling becomes the visible primary. Next depth: keep turning diagnostics/backup intelligence into durable operator decisions and keep widening shared-output behavior.

**Latest (2026-03-21):** **Programming + harvest flows are now much more operator-direct:** `/programming/browse.json` gives one-category batch browse with cached guide-health status, next-hour programme titles/counts, exact-backup counts, recipe inclusion flags, and derived feed descriptors, and the deck can now switch categories into that browse view directly, toggle “Real Guide Only” / “Only Not In Lineup,” launch bounded `stream-compare` / exact-backup `channel-diff` runs from the current Programming selection, and preview/apply ranked harvest assists from `/programming/harvest-assist.json` without dropping to raw payloads. This turns the tester’s curses-side “press `e` and wait on 313 channels” workflow into one server-side cached request plus in-app compare/import/apply hooks, and it makes PPV/event-style quick-add hunts materially faster. Next depth: keep broadening parity/operator productization from shell-only diagnostics toward richer in-app compare/import/apply flows, and keep deepening client-facing parity/output workflows rather than more isolated JSON helpers.

**Latest (2026-03-21):** **`LH-006` and `PAR-006` both moved forward again:** harvested lineups now produce ranked `/programming/harvest-assist.json` recommendations for local-market recipe seeding, and virtual channels now publish focused `/virtual-channels/channel-detail.json` plus a synthetic `/virtual-channels/guide.xml` instead of stopping at preview/schedule/current-slot playback. Release smoke now covers those endpoints, and the deck already consumes the richer harvest/schedule context from the previous pass. Verification is green through targeted tuner tests plus `./scripts/verify`. Next depth: operator-productize the script-only diagnostics (stream compare / channel diff / evidence bundles), and keep deepening parity around virtual-channel publishing and multi-consumer/output workflows instead of more isolated JSON slices.

**Latest (2026-03-21):** **Diagnostics workflow promoted into the operator plane:** `/ops/workflows/diagnostics.json` now turns recent stream attempts into a concrete capture playbook with suggested good/bad channel IDs and latest `.diag/` families, `/ops/actions/evidence-intake-start` scaffolds fresh `.diag/evidence/<case-id>/` bundles directly from the app, the deck surfaces that workflow in Routing/Settings, and `scripts/ci-smoke.sh` now asserts the workflow plus evidence-bundle creation so this operator path is release-gated. Next depth: trigger/summarize `stream-compare` and `channel-diff` from the deck instead of stopping at workflow guidance, while continuing parity work around deeper multi-consumer/output behavior.

**Latest (2026-03-21):** **Diagnostics lane now summarizes the newest harness verdicts:** the diagnostics workflow no longer just lists `.diag/` folders; it now reads the latest `channel-diff`, `stream-compare`, `multi-stream`, and evidence-bundle reports back into the app, extracts verdict/findings, and shows them in the deck so operators get immediate “Tunerr split vs upstream split vs stable parallel reads” context before dropping to shell. Next depth: safe in-app launchers for `stream-compare` / `channel-diff`, then back to the broader parity backlog (shared-output, deeper virtual publishing, richer downstream product surfaces).

**Latest (2026-03-21):** **`PAR-004` and `PAR-006` are now bridged together:** virtual channels no longer stop at `/virtual-channels/live.m3u` and sidecar playback. Enabled virtual channels now flow into the downstream Xtream live surface (`get_live_categories`, `get_live_streams`, and `/live/<user>/<pass>/virtual.<id>.mp4`), so owned-media scheduling is visible to client-facing Xtream consumers too. Next depth: keep broadening client-facing parity around outputs and operator-launched diagnostics, not more isolated side APIs.

**Latest (2026-03-21):** **Plex lineup harvest feature started:** the old wizard-oracle experiment path is now being productized as a real operator feature instead of a buried lab command. `internal/plexharvest` owns reusable target expansion + polling + deduped lineup summaries, `iptv-tunerr plex-lineup-harvest` is the first named CLI surface, and the gateway now has an even tighter 3-account rollover regression so "second device didn't rotate credentials" stays covered. Next depth: persist harvest reports / bridge them into Programming Manager instead of leaving the results as one-shot JSON only.

**Latest (2026-03-21):** **`PAR-005` starter shipped in-tree:** the downstream Xtream surface now has a real file-backed entitlement model via `IPTV_TUNERR_XTREAM_USERS_FILE` and `/entitlements.json`, so different downstream users can see different live/VOD/series slices instead of one global catalog. Release smoke now verifies both the scoped view and denied playback paths. Next depth: virtual channels from owned media (`PAR-006`) or deeper shared-upstream fanout beyond the bounded HLS relay slice (`PAR-002`).

**Latest (2026-03-21):** **`PAR-004` + Programming Manager follow-up shipped in-tree:** the downstream Xtream starter now exposes VOD and series actions/proxies, release smoke now exercises those paths, and `/programming/channel-detail.json` gives category-first tools a focused channel view with 3-hour EPG preview plus exact-match alternative sources. Next depth: real entitlement model (`PAR-005`) or virtual channels from owned media (`PAR-006`).

**Latest (2026-03-21):** **`PAR-004` expanded:** the downstream Xtream starter now exposes VOD and series too, not just live categories/streams. `player_api.php` serves VOD/series category and library actions plus `get_series_info`, and Tunerr-owned `/movie/...` and `/series/...` proxy paths now serve catalog VOD/episode assets through the same output surface. Next depth: entitlement model (`PAR-005`) or virtual channels from owned media (`PAR-006`).

**Latest (2026-03-21):** **`PAR-002` foundation shipped:** same-channel duplicate consumers can now attach to one live HLS Go-relay session instead of always starting another upstream walk. The first cut is intentionally bounded to the native `hls_go` path, exposes `/debug/shared-relays.json`, and gives us a real reusable upstream substrate to deepen later instead of more theory. Next depth: broaden downstream Xtream output or start the server-side entitlement model.

**Latest (2026-03-21):** **`PAR-003` starter shipped:** Tunerr now has a durable server-side recording-rules model behind `IPTV_TUNERR_RECORDING_RULES_FILE`. `/recordings/rules.json` stores and mutates rule definitions, `/recordings/rules/preview.json` evaluates those rules against live catch-up capsules, `/recordings/history.json` classifies recorder state against the active ruleset, and `scripts/ci-smoke.sh` now exercises the rules/history path so it is covered in release gating. Next depth: `PAR-002` shared upstream stream fanout or broader `PAR-004` Xtream output.

**Latest (2026-03-21):** **`PAR-007` intervention follow-up shipped:** `/debug/active-streams.json` now marks cancelable live sessions and includes client UA, and `/ops/actions/stream-stop` can cancel matching active stream contexts by `request_id` or `channel_id` from the localhost operator plane. Next depth: real shared upstream reuse (`PAR-002`) or more downstream publishing depth (`PAR-004`), not more read-only debug surfaces.

**Latest (2026-03-21):** **Tester-driven multi-account rollover fix shipped:** account pooling now falls back to the real Xtream path credentials (`/live/<user>/<pass>/...`, etc.) when `StreamAuths` metadata is missing or incomplete, so distinct provider accounts no longer collapse back to the one global default identity. In the same pass, Tunerr gained `/debug/active-streams.json` as the first `PAR-007` slice so operators can see live in-flight sessions, not just postmortem stream attempts. Next step: keep stacking richer active-stream/control surfaces and the first downstream publishing slice on top of the parity/event foundation.

**Latest (2026-03-21):** **`PAR-004` starter shipped:** Tunerr now has a minimal read-only downstream Xtream-compatible live surface behind `IPTV_TUNERR_XTREAM_USER` / `IPTV_TUNERR_XTREAM_PASS`. `player_api.php` currently serves `get_live_streams` and `get_live_categories`, and `/live/<user>/<pass>/<channel>.ts` proxies through the existing stream gateway. Next depth: VOD/series actions and tighter client-shape validation if we keep pushing the Xtream output lane.

**Latest (2026-03-21):** **Feature-parity epic activated and `PAR-001` shipped:** broad ecosystem gap audit is now a real multi-PR track in `docs/epics/EPIC-feature-parity.md` instead of a vague wishlist. The first foundation slice is in: `IPTV_TUNERR_EVENT_WEBHOOKS_FILE` now enables async JSON webhooks, Tunerr emits `lineup.updated`, `stream.requested`, `stream.rejected`, and `stream.finished`, and `/debug/event-hooks.json` plus `/debug/runtime.json` expose the current webhook state. Next step: stack richer active-stream/control surfaces and the first downstream publishing slice on top of this event model.

**Latest (2026-03-21):** **Windows bare-metal smoke prepared:** added `scripts/windows-baremetal-package.sh` to cross-build a `windows/amd64` package and `scripts/windows-baremetal-smoke.ps1` to run the same local `serve`/web UI/VOD contract on a Windows host without WSL. The package build is verified from Linux, but this path is still **prepared, not host-proven** until it runs on a real Windows VM or box.

**Latest (2026-03-21):** **One-command macOS bare-metal smoke shipped:** added `scripts/mac-baremetal-smoke.sh`, which cross-builds a darwin/arm64 binary, optionally sends Wake-on-LAN magic packets to the MacBook, SSHes in, runs a real `serve`/web UI startup smoke plus the full `vod-webdav` Finder/MiniRedir request matrix, and pulls artifacts back under `.diag/mac-baremetal/<run-id>/`. The first live run passed on `192.168.50.108`, and the Mac already reports `womp=1`, so wake support is enabled on the host side.

**Latest (2026-03-21):** **Real macOS WebDAV validation passed:** passwordless SSH is now configured from this workstation to `keith@192.168.50.108`, a darwin/arm64 `iptv-tunerr` binary was cross-built locally and copied to the Mac, and the Mac-hosted `vod-webdav` instance passed the full Finder/WebDAVFS + Windows MiniRedir request matrix using `scripts/vod-webdav-client-harness.sh` in external mode. The resulting bundle (`mac-selfhost`) diffs cleanly against the local baseline with **no status or header differences**. The cluster node `macair-m4` itself is still `NotReady`, so k8s scheduling is not back yet, but host-level validation is now real instead of purely synthetic.

**Latest (2026-03-21):** **PM-009 regression coverage slice shipped:** Programming Manager now has restart/refresh survival coverage instead of only happy-path mutations. Added a tuner test proving saved category/channel/order/collapse recipe state survives `UpdateChannels` refresh churn and expanded `scripts/ci-smoke.sh` so it restarts `serve` against a reshuffled catalog while reusing the same recipe file, then reasserts curated lineup shape, custom order, and collapse state. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Account-limit persistence + broader WebDAV smoke shipped:** provider-account learned concurrency caps now persist across restarts with a TTL-backed store (`IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE`, `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS`) and are restored into the gateway on startup, surfaced on `/provider/profile.json`, reset with the provider-profile reset flow, and echoed in `/debug/runtime.json`. In parallel, WebDAV validation now exercises real read-path client shapes instead of directory discovery only: unit coverage includes file `HEAD` + byte-range `GET`, and `scripts/ci-smoke.sh` now stands up a local HTTP asset server and proves `iptv-tunerr vod-webdav` can `PROPFIND`, `HEAD`, and range-read through the production cache/materializer path. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Read-only WebDAV contract hardened:** `internal/vodwebdav` now enforces an explicit read-only method surface (`OPTIONS`, `PROPFIND`, `HEAD`, `GET`) and returns clean `405` responses with stable `Allow`/`DAV` headers for mutation methods instead of relying on lower filesystem-layer errors. Validation depth also increased again: file-level `PROPFIND`, episode `HEAD`, episode byte-range reads, and binary smoke assertions for both movie and episode paths now run through the real `vod-webdav` cached-materializer path. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **WebDAV client-matrix harness shipped:** added `scripts/vod-webdav-client-harness.sh` and `scripts/vod-webdav-client-report.py` so macOS Finder/WebDAVFS and Windows MiniRedir request shapes can be replayed against a real `vod-webdav` binary, either in self-contained local mode or against an existing `BASE_URL`. The harness writes `.diag/vod-webdav-client/<run-id>/` bundles with per-step headers/bodies plus `report.json` / `report.txt`, and the new how-to doc wires it into the supported QA toolkit instead of leaving non-Linux VOD validation as ad hoc smoke only. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Baseline-vs-host WebDAV diff workflow shipped:** added `scripts/vod-webdav-client-diff.py` so a known-good self-contained bundle can be compared directly against a macOS or Windows host bundle step-by-step. The VOD WebDAV how-to now documents the baseline -> real-host -> diff loop, and `memory-bank/commands.yml` includes a dedicated `vod_webdav_client_diff` helper entry. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Header-aware WebDAV diffs + Mac-node job template:** `scripts/vod-webdav-client-diff.py` now compares key WebDAV/read-only headers (`Allow`, `DAV`, `MS-Author-Via`, `Accept-Ranges`, `Content-Type`, `Content-Length`, `Content-Range`, `ETag`, `X-Content-Type-Options`) instead of only status codes, so baseline-vs-host drift is much more actionable. Added `k8s/vod-webdav-client-macair-job.yaml` as a starter node-targeted Job for `macair-m4`, and updated the harness how-to with the k8s run path. Live cluster check showed `macair-m4` is reachable and now accepts SSH, but the node still reports `NotReady` with stale heartbeats, so the manifest is ready for when cluster scheduling returns. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **PM-008 deck lane shipped:** the dedicated web UI now has a real Programming lane backed by the existing `/programming/*.json` APIs. Operators can bulk include/exclude categories, pin or block exact channels, nudge manual order from the curated preview, toggle exact-backup collapse, inspect backup groups, and jump straight into raw Programming payloads without hand-posting JSON. Verified with `node --check internal/webui/deck.js`, targeted `go test`, and full `./scripts/verify`.

**Latest (2026-03-21):** **PM-006 / PM-007 backend slice shipped:** Programming Manager now has durable order and backup-grouping primitives. Added `/programming/order.json` for server-side manual order mutations, `/programming/backups.json` for exact-match sibling-group inspection, and `collapse_exact_backups: true` in the saved recipe so strong same-channel siblings (`tvg_id` exact, else `dna_id` exact) can collapse into one visible row with merged backup stream URLs. Binary smoke now exercises the new order/backups flow and full `./scripts/verify` is green.

**Latest (2026-03-21):** **PM-006 / PM-007 backend slice in progress:** next work extends Programming Manager past category/channel toggles into durable manual order semantics and exact-match backup grouping. The goal is to let operators save explicit lineup order changes server-side and collapse strong same-channel siblings (same `tvg_id` / `dna_id`) into one visible row with merged backup streams, then prove it with API tests and binary smoke before touching deck UI (`PM-008`).

**Latest (2026-03-21):** **Adaptive account learning + PM-001/PM-002 foundations shipped:** provider-account pooling now learns tighter per-account concurrency caps from real upstream limit signals and exposes them on `/provider/profile.json` as `account_learned_limits`. In parallel, the first Programming Manager backend slice landed: `internal/programming`, `IPTV_TUNERR_PROGRAMMING_RECIPE_FILE`, and the new `/programming/categories.json`, `/programming/recipe.json`, and `/programming/preview.json` endpoints. Binary smoke now validates those endpoints plus broader WebDAV `OPTIONS` / `PROPFIND` behavior. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **PM-003/PM-004/PM-005 slice shipped:** category-first and channel-level Programming Manager mutations now work over HTTP (`/programming/categories.json` and `/programming/channels.json` POST), and `order_mode: "recommended"` now classifies channels into the requested Local/Entertainment/News/Sports/... taxonomy on the server. `programming/preview.json` now includes bucket counts, and release smoke mutates the recipe over HTTP to prove the preview/output really changes. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Adaptive account learning + Programming Manager foundations:** next slice extends the new provider-account pool so it can learn tighter per-account concurrency limits from real upstream signals instead of relying only on a static env cap, broadens WebDAV validation around macOS/Windows-style client behavior, and starts `PM-001`/`PM-002` with a durable programming recipe + category inventory/preview layer that sits between raw lineup intelligence and final exposed channels.

**Latest (2026-03-21):** **Account-aware provider pooling + VOD mount helpers:** deduplicated multi-account channels now derive a stable provider-account identity per URL, prefer less-loaded accounts during stream ordering, keep active leases for successful live sessions, and can enforce `IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT` as a per-credential cap with local HDHR-style `805` / `503` rejection when every account for a channel is busy. In parallel, the first VOD ergonomics slice landed: `iptv-tunerr vod-webdav-mount-hint` prints platform-specific mount commands, and `vod-webdav` startup logs now include concrete mount commands instead of only generic prose. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Release gating + Programming Manager mapping:** binary smoke now exercises `vod-webdav-mount-hint` and the live `vod-webdav` surface with a real WebDAV `PROPFIND`, while targeted gateway tests now cover provider-account local rejection, lease release after success, and provider-profile account-pool state. In parallel, the tester’s channel-builder request is now formalized as [EPIC-programming-manager](../docs/epics/EPIC-programming-manager.md): category-first selection, per-channel include/exclude, server-saved order, and exact-match backup grouping.

Assumptions:
1. The first safe scheduler model is lease-aware ordering/capping in the existing upstream walk, not a full provider broker rewrite.
2. Some panels allow >1 stream per credential set, so hard per-account caps must stay operator-tunable via env instead of being guessed globally.

**Latest (2026-03-21):** **Cross-platform VOD parity slice 1:** extracted VOD naming/tree logic out of Linux-only files so it can back more than the `go-fuse` path, kept Linux `mount` wired to the same shared tree, and added a new cross-platform `vod-webdav` command backed by `internal/vodwebdav` using read-only WebDAV over the same catalog/materializer model. README/features/platform/CLI docs now describe Linux `mount` vs macOS/Windows `vod-webdav` parity. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Intermittent channel-class diff harness:** tester feedback narrowed the remaining live-stream issue to "some channels work, some don't" on the same build, so the right next tool is a paired good-vs-bad capture instead of more global guesses. Added `scripts/channel-diff-harness.sh` and `scripts/channel-diff-report.py`: seed one good and one bad channel through Tunerr, infer the paired direct upstream URLs from `/debug/stream-attempts.json`, run two `stream-compare` captures, and emit a compact classification report (upstream-only vs Tunerr-only vs channel-class split). **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Cross-platform VOD parity kick-off:** user approved native macOS/Windows parity work for Linux VODFS, but the implementation does **not** need to be FUSE specifically. This track is being formalized as multi-PR work: extract shared VOD tree/naming logic out of Linux-only files, add a cross-platform virtual filesystem surface that macOS/Windows can mount natively, and keep Linux `mount` intact. First slice targets a shared tree model plus a WebDAV-backed VOD surface and mount helpers for non-Linux OSes. 

Assumptions:
1. The fastest supportable parity path is Linux `go-fuse` + cross-platform WebDAV mountability for macOS/Windows rather than immediate macFUSE/WinFsp cgo integration.
2. Product goal is parity of **behavior** (browse `Movies/` + `TV/`, read on demand, Plex-scannable mounted path), not strict parity of kernel/filesystem implementation.

**Latest (2026-03-20):** **Guide startup race fix:** tester logs showed `guide.xml` serving an 82-byte empty `<tv>` for the full 10-minute TTL because XMLTV startup refresh ran before the lineup loaded. The refresh path now skips caching when there are zero lineup channels, and `UpdateChannels` queues a real refresh immediately when channels arrive. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Security/control-plane hardening pass:** fixed the CodeQL **guideinput** remote-fetch sink by resolving remote guide URLs only through a prepared allowlist map before any HTTP request is built, removed the dedicated deck’s `admin/admin` fallback by generating a one-time startup password when `IPTV_TUNERR_WEBUI_PASS` is unset, stopped persisting deck credentials/browser-authored telemetry to `IPTV_TUNERR_WEBUI_STATE_FILE`, and narrowed `/deck/settings.json` to non-secret refresh preferences while `/deck/activity.json` stays server-derived. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Startup registration contract hardening:** `/guide.xml` no longer returns a misleading `200` while the first merged guide is still building. It now serves the visible placeholder body with `503 Service Unavailable`, `Retry-After: 5`, and `X-IptvTunerr-Guide-State: loading` until real guide cache exists; HDHR `discover.json` / `lineup.json` / `lineup_status.json` stay compatible but add `X-IptvTunerr-Startup-State: loading` before lineup load. Also added a named release-smoke regression lane in `memory-bank/commands.yml` for startup/registration coverage. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Binary smoke + CI/release hardening:** added `scripts/ci-smoke.sh`, which builds a temporary binary, runs `serve` against synthetic full/empty catalogs, and asserts the real HTTP startup contract (`readyz`, `guide.xml`, startup headers, lineup behavior). Wired that smoke into `scripts/verify`, CI, and the GitHub release workflow so tags now run real endpoint smoke before packaging. **`./scripts/verify`** OK.

**Latest (2026-03-21):** **Direct-ffplay-vs-Tunerr HLS isolation:** narrowed one plausible playback split to non-transcode ffmpeg remux on cross-host HLS manifests. Added manifest-host regression coverage and changed the remux decision so playlists that reference media/key/map/variant URLs on a different host than the playlist itself prefer the Go relay by default instead of ffmpeg remux; this keeps a single static ffmpeg header context out of the path when direct playback works but Tunerr remux does not.

**Latest (2026-03-21):** **Cross-host HLS segment header fix:** tester logs still showed **Go relay** `.ts` fetches hitting **403** after the remux skip, so the remaining problem was HLS subrequest context, not ffmpeg. Playlist/segment subrequests now inherit fallback **`Referer`** and **`Origin`** from the current playlist URL when the client did not provide them, and the cross-host regression test now models a CDN that rejects segment fetches without that playlist context.

**Latest (2026-03-21):** **Repeated remux-attempt fix:** later tester logs showed the same host still taking the ffmpeg-remux path on later tunes even after prior `ffmpeg_hls_failed` outcomes. Root cause was that generic playlist success cleared host penalty too early. Tunerr now keeps a dedicated HLS remux-failure penalty so later tunes on the same host prefer the Go relay instead of retrying the same dead remux path.

**Latest (2026-03-21):** **Dead remux startup timeout:** another tester trace showed a worse first-tune case: ffmpeg-remux could sit for 15–25 seconds without producing bytes, so Plex gave up before Tunerr ever fell back. Non-transcode HLS ffmpeg-remux now has its own first-byte timeout path, with a fake-ffmpeg regression to prove the fallback happens quickly instead of waiting for client cancellation.

**Latest (2026-03-21):** **Evidence intake path:** added **`scripts/evidence-intake.sh`**, [how-to/evidence-intake](../docs/how-to/evidence-intake.md), and **`planning/README.md`** so working-vs-failing tester cases can be staged consistently under **`.diag/evidence/<case-id>/`** with debug-bundle output, PMS logs, Tunerr logs, pcaps, and notes before running **`scripts/analyze-bundle.py`**.

**Latest (2026-03-21):** **Visible placeholder guide content:** the startup placeholder `/guide.xml` now labels itself as a loading placeholder in XMLTV source metadata and emits programme titles like **`<channel> (guide loading)`** with a short description so Plex users can tell the guide is still building instead of reading the temporary rows as normal data.

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
- Updated `secret/data/iptv` in Bao: replaced the old provider1 host with the current provider1 host.
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
**Session 2026-03-21 (audit follow-through: export identity and browse correctness):**

- Audited the new parity/programming/Xtream surfaces for hidden contract bugs and implemented the concrete fixes instead of leaving them as review notes.
- Fixed Xtream export identity drift:
  - Xtream `xmltv.php` / `get.php` now use a canonical exported channel id based on Tunerr `ChannelID`, not raw provider `TVGID`, so sibling variants stop collapsing when a provider reuses `tvg_id`.
  - Xtream XMLTV programme rows now attach to exported channel ids via actual channel ids from catchup capsules instead of a lossy guide-number overwrite map.
- Fixed catchup/Programming browse channel identity:
  - `BuildCatchupCapsulePreview` now duplicates programme capsules per matching lineup channel sharing a guide number and emits the real `ChannelID` in capsules.
  - `serveProgrammingBrowse` now keys next-hour titles by `ChannelID`, which stops smearing titles across unrelated sibling variants.
  - Added capsule-preview caching inside `XMLTV` so repeated browse/detail calls reuse the same preview snapshot per horizon instead of rebuilding it every request.
- Fixed downstream parity gap in Xtream VOD proxy:
  - `movie/` and `series/` proxies now support `HEAD`, forward `Range`, and preserve `Content-Length`, `Accept-Ranges`, `Content-Range`, `Last-Modified`, and `ETag`.
- Fixed Programming Manager numeric sort correctness:
  - category members and recommended ordering now sort numeric guide numbers numerically instead of lexically.
- Updated release smoke to assert the new canonical Xtream XMLTV ids (`ChannelID`-based) so CI matches real behavior.

**Verification:**
- `go test ./internal/tuner -run 'Test(BuildCatchupCapsulePreview_clampsLargeLimit|BuildCatchupCapsulePreview_duplicatesProgrammePerMatchingChannel|Server_programmingBrowse|Server_XtreamMovieAndSeriesProxy|Server_XtreamXMLTVUsesUniqueChannelIDsWhenTVGIDCollides|Server_Xtream(PlayerAPI_LiveCategories|Exports_M3UAndXMLTV))' -count=1`
- `go test ./internal/programming -run 'Test(CategoryMembers_sortGuideNumbersNumerically|BuildBackupGroupsAndCollapse|BuildBackupGroupsDoesNotCollapseVariantNames|DescribeChannel)' -count=1`
- `./scripts/verify`

**Next focus:** Keep auditing for remaining parity/runtime contract mismatches, but the specific Xtream/Programming identity bugs from the audit are now fixed and release-gated.
