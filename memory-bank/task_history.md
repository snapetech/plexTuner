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

- Date: 2026-03-22
  Title: Correct Jellyfin migration convergence semantics
  Summary:
    - Tightened `internal/livetvbundle` audit status logic so bundled library parity warnings are no longer treated as converged state.
    - `AuditBundleTargets(...)` now keeps a target at `ready_to_apply` when bundled libraries are still missing, lagging Plex source counts, missing sampled titles, or completely empty, even if Live TV is indexed and the definitions are already present.
    - Updated the dependent deck/web UI migration audit expectation so the operator surface matches the corrected backend semantics.
    - Re-ran the live Jellyfin rollout audit from the real `.env`; the target now truthfully reports `ready_to_apply` with explicit lagging/empty library reasons instead of the earlier false `converged`.
  Verification:
    - `go test ./internal/livetvbundle ./internal/emby -run 'Test(AuditBundleTargets.*|DiffEmbyPlanJellyfinConfigurationFallback|GetLiveTVConfiguration|ListTunerHostsMethodNotAllowed|ListListingProvidersMethodNotAllowed)$'`
    - `go test ./cmd/iptv-tunerr ./internal/webui ./internal/migrationident ./internal/keycloak`
    - `set -a; source ./.env; set +a; go run ./cmd/iptv-tunerr migration-rollout-audit -in "$IPTV_TUNERR_MIGRATION_BUNDLE_FILE" -targets jellyfin -summary`
    - `./scripts/verify`
  Notes:
    - The remaining live Jellyfin gap is now content parity, not definition parity: libraries are mounted and created, but the test target still shows lagging counts/titles and empty mounted roots.
    - A future audit refinement may want a stronger status than `ready_to_apply` for “definitions present but content not synced.”
  Opportunities filed:
    - `memory-bank/opportunities.md` existing migration-reporting refinement backlog is still the right home; no new backlog file created in this pass.
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Fix live Jellyfin migration audit and Keycloak credential auth
  Summary:
    - Added an explicit Jellyfin Live TV exact-read fallback so `migration-rollout-audit` no longer fails closed when Jellyfin `10.11.x` returns `405` on `GET /LiveTv/TunerHosts` and `GET /LiveTv/ListingProviders`.
    - Tunerr now switches to Jellyfin's `GET /System/Configuration/livetv` endpoint for exact tuner/listing parity when those read-side list endpoints are unavailable, instead of silently assuming empty state or downgrading to a vague best-effort read.
    - Added Keycloak admin username/password support across the core IdP migration lane, CLI, and deck env handling so Tunerr can mint a fresh `admin-cli` token per diff/audit/apply run instead of depending on a fragile static bearer token.
    - Revalidated both fixes live from the restored cluster-backed `.env`: Jellyfin migration audit now returns `ready_to_apply` with normal exact diff semantics, and Keycloak OIDC audit now succeeds without an inline hand-minted token.
  Verification:
    - `go test ./internal/keycloak ./internal/emby ./internal/livetvbundle ./internal/migrationident ./cmd/iptv-tunerr ./internal/webui`
    - `go run ./cmd/iptv-tunerr migration-rollout-audit -in .diag/live-migration/migration-bundle.json -targets jellyfin -summary`
    - `go run ./cmd/iptv-tunerr identity-migration-oidc-audit -in .diag/live-migration/identity-oidc-plan.json -targets keycloak -summary`
    - `./scripts/verify`
  Notes:
    - Jellyfin still does not expose exact read-side tuner/listing object lists under `/LiveTv/*`, but `System/Configuration/livetv` contains the same state on the tested `10.11.6` server and is now the exact diff source.
    - `.env` now prefers `IPTV_TUNERR_KEYCLOAK_USER` / `IPTV_TUNERR_KEYCLOAK_PASSWORD` for the disposable test realm, but the old token env remains supported as a fallback.
  Opportunities filed:
    - none
  Links:
    - `internal/emby/register.go`
    - `internal/livetvbundle/bundle.go`
    - `internal/keycloak/keycloak.go`
    - `internal/migrationident/bundle.go`
    - `cmd/iptv-tunerr/cmd_identity_migration.go`
    - `internal/webui/webui.go`

- Date: 2026-03-22
  Title: Fill live migration env from k3s and validate migration/IdP workflows
  Summary:
    - Restored and expanded the repo `.env` with live cluster-backed Plex, Emby, Jellyfin, Authentik, and disposable Keycloak test values without printing secrets.
    - Built live migration artifacts under `.diag/live-migration/` (`migration-bundle.json`, `identity-bundle.json`, and `identity-oidc-plan.json`) from the real Plex DVR and real Plex user/share state.
    - Fixed the `identity-migration-oidc-audit` CLI target-filter bug so `keycloak,authentik` targets are accepted instead of being incorrectly validated as Emby/Jelly-only targets.
    - Verified live identity migration against Emby and Jellyfin, live OIDC migration against Authentik, and live OIDC migration against Keycloak when supplied a freshly minted admin token.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(FilterRequestedOIDCTargets|FilterRequestedOIDCTargetsRejectsUnknownTarget)$'`
    - `go run ./cmd/iptv-tunerr migration-rollout-audit -in .diag/live-migration/migration-bundle.json -targets emby -summary`
    - `go run ./cmd/iptv-tunerr identity-migration-audit -in .diag/live-migration/identity-bundle.json -summary`
    - `go run ./cmd/iptv-tunerr identity-migration-oidc-audit -in .diag/live-migration/identity-oidc-plan.json -targets authentik -summary`
    - `go run ./cmd/iptv-tunerr identity-migration-oidc-audit -in .diag/live-migration/identity-oidc-plan.json -targets keycloak -keycloak-token <fresh token> -summary`
  Notes:
    - Live provider/tuner smoke and probe were healthy enough to support real migration testing, and the live multi-stream harness showed one concrete upstream-path failure rather than a tuner-limit failure.
    - Jellyfin `10.11.6` currently rejects `GET /LiveTv/TunerHosts` and `GET /LiveTv/ListingProviders` with `405`, so live-TV rollout audit currently works on Emby but fails closed on Jellyfin.
    - The current Keycloak env-token model is fragile in live use because short-lived admin access tokens expire quickly; a fresh token works immediately, but a static env token does not stay valid.
  Opportunities filed:
    - none
  Links:
    - `.diag/live-migration/migration-bundle.json`
    - `.diag/live-migration/identity-bundle.json`
    - `.diag/live-migration/identity-oidc-plan.json`
    - `cmd/iptv-tunerr/cmd_identity_migration.go`
    - `cmd/iptv-tunerr/cmd_identity_migration_test.go`

- Date: 2026-03-22
  Title: Raise `cmd/iptv-tunerr` coverage on runtime and free-source helpers
  Summary:
    - Added direct command-layer tests for `loadRuntimeLiveChannels`, `loadRuntimeCatalog`, runtime snapshot exposure, and runtime server propagation of virtual recovery state.
    - Added helper tests for `parseCSV` and `hostPortFromBaseURL`.
    - Added free-source tests for cache-dir/key helpers plus supplement, merge, and full application behavior so those paths are no longer sitting at zero coverage.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(ParseCSV_TrimAndDropEmpty|ParseCSV_BlankReturnsNil|HostPortFromBaseURL_ReturnsHostPort|HostPortFromBaseURL_RejectsMissingHost|LoadRuntimeLiveChannels_LoadsCatalogAndAssignsDNA|LoadRuntimeCatalog_LoadsMoviesSeriesAndLive|FreeSourceCacheDir_PrefersExplicitDir|FreeSourceCacheDir_FallsBackToCacheDirChild|URLCacheKey_UsesHashPrefixAndLastSegment|MaxPaidGuideNumber_UsesLeadingIntegerOnly|AssignFreeGuideNumbers_StartsAfterBase|ApplyFreeSourcesSupplement_AddsOnlyNewTVGIDsAndRenumbers|ApplyFreeSourcesMerge_EnrichesPaidAndAddsNewChannels|ApplyFreeSourcesFull_RenumbersOnlyAppendedChannels|BuildRuntimeSnapshot_ExposesVirtualRecoveryRuntimeFields|NewRuntimeServer_PropagatesVirtualRecoveryStateFile)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `cmd/iptv-tunerr` package coverage moved to `34.3%`.
    - The biggest remaining low-cost command-layer targets are still `fetchRawCached`, `loadIptvOrgFilter`, and `applyIptvOrgFilter`.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime_test.go`
    - `cmd/iptv-tunerr/cmd_util_test.go`
    - `cmd/iptv-tunerr/free_sources_test.go`

- Date: 2026-03-22
  Title: Cover free-source cache and iptv-org filter loading paths
  Summary:
    - Added direct tests for `fetchRawCached` using a real cache-hit flow, so the command-layer free-source cache path is exercised instead of assumed.
    - Added a seeded-cache test for `loadIptvOrgFilter`, covering real parsing of cached iptv-org blocklist/channels metadata without relying on the network.
    - Added `applyIptvOrgFilter` tests for both tag-only and drop behavior across blocked, NSFW, closed, and safe channels.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(FetchRawCached_UsesCacheAfterFirstFetch|LoadIptvOrgFilter_LoadsFromSeededCache|ApplyIptvOrgFilter_TagsOrDropsChannels|FreeSourceCacheDir_PrefersExplicitDir|FreeSourceCacheDir_FallsBackToCacheDirChild|URLCacheKey_UsesHashPrefixAndLastSegment|MaxPaidGuideNumber_UsesLeadingIntegerOnly|AssignFreeGuideNumbers_StartsAfterBase|ApplyFreeSourcesSupplement_AddsOnlyNewTVGIDsAndRenumbers|ApplyFreeSourcesMerge_EnrichesPaidAndAddsNewChannels|ApplyFreeSourcesFull_RenumbersOnlyAppendedChannels)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `cmd/iptv-tunerr` package coverage moved to `36.3%`.
    - The largest remaining low-cost command-layer targets are `fetchFreeSources` and the `applyFreeSources` dispatcher, followed by command handlers in `cmd_runtime_register.go`.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/free_sources_test.go`

- Date: 2026-03-22
  Title: Cover free-source fetch/dispatch and runtime-register helpers
  Summary:
    - Added `fetchFreeSources` tests for both the no-source path and a real cached-filter M3U ingest path.
    - Added direct coverage for the `applyFreeSources` dispatcher so supplement/merge/full mode selection is exercised through the public helper too.
    - Added cheap helper coverage in `cmd_runtime_register_test.go` for `guideURLForBase`, `streamURLForBase`, `minInt`, and `maxInt`.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(FetchFreeSources_NoURLsReturnsNil|FetchFreeSources_UsesCachedFilterAndDropsBlockedChannels|ApplyFreeSources_DispatchesByMode|GuideURLForBase_TrimsTrailingSlash|StreamURLForBase_TrimsTrailingSlash|MinInt|MaxInt)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `cmd/iptv-tunerr` package coverage moved to `36.5%`.
    - The remaining low-coverage areas are now mainly heavier integration-style command handlers such as `registerRunPlex`, `registerRunMediaServers`, and `handleRun`.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/free_sources_test.go`
    - `cmd/iptv-tunerr/cmd_runtime_register_test.go`

- Date: 2026-03-22
  Title: Cover runtime guard branches and registration policy paths
  Summary:
    - Added registration-path tests for `applyRegistrationRecipe` off/healthy behavior, easy-mode `registerRunPlex`, and missing-credentials `registerRunMediaServers`.
    - Added runtime guard tests for disabled `maybeOpenEpgStore` and disabled/nil `startDedicatedWebUI`.
    - Pushed the command package coverage further without introducing brittle network-heavy integration fixtures.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(MaybeOpenEpgStore_DisabledReturnsNil|StartDedicatedWebUI_DisabledIsNoOp|ApplyRegistrationRecipe_OffReturnsInput|ApplyRegistrationRecipe_HealthyDropsWeakGuide|RegisterRunPlex_EasyModeReturnsFalseWithoutRegistration|RegisterRunMediaServers_MissingCredentialsDoesNothing)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `cmd/iptv-tunerr` package coverage moved to `37.7%`.
    - The remaining low-coverage command paths are now mostly heavier integration-style handlers such as `handleServe`, `handleRun`, and deeper media-server registration flows.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime_test.go`
    - `cmd/iptv-tunerr/cmd_runtime_register_test.go`

- Date: 2026-03-22
  Title: Add file-backed EPG open coverage and another safe Plex registration branch
  Summary:
    - Added a real temp-SQLite integration test for `maybeOpenEpgStore`, verifying the command layer can open and close an on-disk EPG store successfully.
    - Added a `registerRunPlex` test for the register-only/no-live branch so that path is exercised without needing live Plex credentials or a PMS instance.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(MaybeOpenEpgStore_OpensSQLiteFile|RegisterRunPlex_RegisterOnlyWithoutLiveReturnsTrue|MaybeOpenEpgStore_DisabledReturnsNil|ApplyRegistrationRecipe_HealthyDropsWeakGuide)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `cmd/iptv-tunerr` package coverage moved to `38.2%`.
    - The remaining command-layer gaps are now mostly in the blocking/`os.Exit`-driven handlers such as `handleServe`, `handleRun`, and the VOD mount/WebDAV flows.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime_test.go`
    - `cmd/iptv-tunerr/cmd_runtime_register_test.go`

- Date: 2026-03-22
  Title: Add subprocess integration coverage for blocking VOD command handlers
  Summary:
    - Added a subprocess-based helper harness in `cmd_vod_integration_test.go` so blocking `os.Exit`/`ListenAndServe` VOD handlers can be exercised without refactoring production code.
    - Covered `handleVODWebDAV` success with a real served catalog and read-only DAV response validation.
    - Covered `handleVODWebDAV` and `handleMount` failure exits on missing catalog inputs.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(HandleVODWebDAV_ServesReadOnlyDAV|HandleVODWebDAV_MissingCatalogExits|HandleMount_MissingCatalogExits)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `cmd/iptv-tunerr` package coverage moved to `38.5%`.
    - `cmd_vod.go` is no longer a total blind spot; the remaining big runtime gaps are now mostly `handleServe` and `handleRun`.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_vod_integration_test.go`

- Date: 2026-03-22
  Title: Add subprocess integration coverage for runtime entrypoints
  Summary:
    - Added a subprocess helper harness in `cmd_runtime_integration_test.go` to exercise `handleRun` register-only success/failure paths and `handleServe` startup against a real temp catalog.
    - Changed the `handleServe` integration test to stop the helper with `SIGTERM` instead of killing it so Go coverage data flushes from the child process.
    - Raised command-layer confidence on the last major runtime blind spot without refactoring the blocking entrypoints themselves.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(HandleServe_ServesDiscoverJSON|HandleRun_RegisterOnlySuccess|HandleRun_MissingCatalogExits)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `cmd/iptv-tunerr` package coverage moved to `40.0%`.
    - `handleServe` now reports `52.4%` coverage and `handleRun` `37.3%`.
    - The remaining command-layer cliff is now mainly `main()` dispatch and deeper long-lived `handleRun` branches, which would need a larger CLI subprocess harness to improve further.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime_integration_test.go`

- Date: 2026-03-22
  Title: Add CLI subprocess coverage for `main()` and deeper `handleRun` refresh
  Summary:
    - Added `main_integration_test.go` so the real `main()` command dispatch is exercised through subprocess-driven `run` invocations instead of only helper functions.
    - Added a deeper `handleRun` subprocess path that refreshes a catalog from a direct M3U source before exiting in register-only mode.
    - Raised the last major command-layer success-path blind spots instead of only covering helper logic.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(Main_RunCommandDispatchesSuccessfully|Main_RunCommandRefreshesFromM3UEnv|HandleRun_RefreshesCatalogFromDirectM3U)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `cmd/iptv-tunerr` package coverage moved to `41.1%`.
    - `handleRun` is now `56.4%` and `main()` `60.0%`.
    - What remains is mostly lower-value or more brittle coverage: long-lived loops, more registration combinations, and CLI error-exit branches.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/main_integration_test.go`
    - `cmd/iptv-tunerr/cmd_runtime_integration_test.go`

- Date: 2026-03-22
  Title: Finish top-level CLI dispatch coverage
  Summary:
    - Extended the `main()` subprocess harness to cover `index`, `version`, `--help`, no-args usage, and unknown-command exits in addition to the existing `run` coverage.
    - Finished covering the meaningful top-level CLI success/error dispatch behavior without refactoring production command wiring.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(Main_IndexCommandDispatchesSuccessfully|Main_VersionCommand|Main_HelpExitsZero|Main_NoArgsExitsNonZero|Main_UnknownCommandExitsNonZero)$' -v`
    - `go test -coverprofile=/tmp/cmd-cover.out ./cmd/iptv-tunerr`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `main()` is now `100.0%` covered.
    - `cmd/iptv-tunerr` package coverage moved to `41.6%`.
    - Remaining gaps are mostly lower-yield or more brittle: long-lived runtime loops, deeper registration permutations, and OS-specific successful VOD mount behavior.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/main_integration_test.go`

- Date: 2026-03-22
  Title: Audit README and station-ops docs for final runtime behavior
  Summary:
    - Updated `README.md` so the virtual-channel/station-ops section now reflects branded stream publishing, report surfaces, deck-side branding/recovery controls, fallback-chain recovery, and persisted recovery history.
    - Updated `docs/reference/virtual-channel-stations.md` so branding fields are described as active branded-stream/slate behavior instead of future-only metadata.
    - Updated `docs/index.md` so the station reference is framed as runtime station-operations behavior, not only schema/reference metadata.
  Verification:
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This was a docs/readme alignment pass only; no production code changed.
  Opportunities filed:
    - none
  Links:
    - `README.md`
    - `docs/reference/virtual-channel-stations.md`
    - `docs/index.md`

- Date: 2026-03-22
  Title: Audit deck and docs for release-surface coverage
  Summary:
    - Added a shared-relay Routing card to the deck so active shared live sessions and subscriber counts are visible from the operator plane.
    - Tightened the deck asset test to pin the shared-relay endpoint.
    - Updated `docs/features.md` and `README.md` so the release docs explicitly mention migration, identity, OIDC/IdP, and shared-relay operator surfaces.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The deck already had the migration/identity/OIDC workflow cards; the main operator-plane gap was shared-relay visibility and the main doc gap was `docs/features.md`.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`
    - `docs/features.md`
    - `README.md`

- Date: 2026-03-22
  Title: Persist explicit OIDC target failure status in deck activity
  Summary:
    - Added backend `target_statuses` recording for deck-side OIDC applies so each requested IdP target now has an explicit `applied`, `failed`, `validation_failed`, or `not_reached` state in persisted activity.
    - Switched the OIDC workflow modal to use that explicit target-status map instead of inferring failures from missing result rows.
    - Added regression coverage for target-status maps and provider-failure target state.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The target-status map still carries summary-level failure context, not provider-native per-user failure details.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Add per-target OIDC apply outcome rows to the workflow modal
  Summary:
    - Extended the OIDC workflow modal so each recent apply entry now renders structured per-target outcome rows instead of only one compact summary line.
    - Failed partial runs now show which target was not reached before the apply stopped, which makes Keycloak vs Authentik partial failures much easier to triage during release work.
    - Updated release-facing docs and the deck asset guardrail to pin the modal target-detail UI.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Target-specific failure reasons are still inferred from missing result rows; the backend does not yet persist per-target failure payloads.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/webui/deck.css`
    - `internal/webui/webui_test.go`
    - `docs/emby-jellyfin-support.md`

- Date: 2026-03-22
  Title: Extend OIDC apply history controls into the workflow modal
  Summary:
    - Reused the deck's OIDC apply history filter controls inside the OIDC workflow modal so operators can inspect recent IdP runs without leaving the modal.
    - Added a dedicated modal history wrapper and tightened the deck asset test so the modal-specific history strings/classes stay pinned.
    - Updated README, migration docs, changelog, and current-task notes to reflect that the OIDC history filters now exist in both the card and the modal.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The modal still uses the compact per-run summary string; richer per-target failure drill-down is the next likely refinement.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/webui/deck.css`
    - `internal/webui/webui_test.go`
    - `docs/emby-jellyfin-support.md`

- Date: 2026-03-22
  Title: Add OIDC success/failure filters to deck history
  Summary:
    - Added `all / success / failed` filtering to the deck's `OIDC recent applies` card.
    - Added simple success/failure badge styling so recent OIDC history is scannable without parsing every line.
    - Tightened the deck asset test so these OIDC history controls do not quietly disappear in later frontend edits.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Filtering is currently card-level only; the workflow modal still shows the raw payload.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/webui/deck.css`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Record failed OIDC apply attempts in deck workflow history
  Summary:
    - Normalized deck-side OIDC apply failures into the same persisted `oidc_migration_apply` activity history used by successful runs.
    - Added validation/provider failure phase and error context so recent OIDC history can show what failed, not just what succeeded.
    - Added regression coverage for validation failure and provider failure activity entries in the OIDC apply handler.
  Verification:
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The workflow history is still rendered as flat text summaries; success/failure badges remain follow-on work.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Extend virtual live recovery across the fallback chain
  Summary:
    - Changed the recoverable virtual-channel relay so it can switch again after the first rescue source if that fallback later stalls or hard-errors too.
    - Recovery events now record the actual hop that happened at each cutover, instead of only the original source-to-first-fallback picture.
    - Recovery now also records explicit exhausted events when the fallback chain runs out, the station report/deck summarize that posture with `recovery_events`, `recovery_exhausted`, and `last_recovery_reason`, an optional recovery state file preserves those events across restarts, and the relay now performs repeated rolling in-session media-content probes after startup.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_virtualChannelStream(FallsBackDuringLiveStall|LiveStallSkipsBrokenFirstFallback|FallsBackAgainAfterFallbackStalls|ReportsRecoveryExhaustion)$' -v`
    - `node -c internal/webui/deck.js`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This still reacts to byte-level stalls/errors, not decoded black-frame or silence analysis over the full live session.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `docs/reference/virtual-channel-stations.md`

- Date: 2026-03-22
  Title: Add recent OIDC apply history to the deck workflow
  Summary:
    - Extended `/deck/oidc-migration-audit.json` with a short recent `oidc_migration_apply` history derived from persisted deck activity.
    - Added an `OIDC recent applies` card so the deck workflow now shows a few recent Keycloak/Authentik pushes instead of only the latest run.
    - Added regression coverage for the recent-history helper ordering and the richer workflow summary payload.
  Verification:
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is still success-history only; failed apply attempts are not yet normalized into the same structured summary.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Add per-target OIDC apply deltas to the deck workflow
  Summary:
    - Extended the persisted `oidc_migration_apply` activity detail with compact per-target apply deltas for Keycloak/Authentik results.
    - Updated the deck OIDC workflow summary so `OIDC last apply` now shows those per-target created-user/group, membership, metadata-update, and activation-pending counts.
    - Added regression coverage for the compact result-summary helper and the richer workflow summary payload.
  Verification:
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The workflow still only keeps the latest recorded apply in its summary; multi-run history remains in the general activity log.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Surface last OIDC apply result in the deck workflow
  Summary:
    - Extended `/deck/oidc-migration-audit.json` so it includes the latest recorded `oidc_migration_apply` activity entry as `summary.last_apply`.
    - Updated the deck workflow cards to show an `OIDC last apply` summary, including target/provider onboarding hints from the most recent push.
    - Added regression coverage for the workflow summary and the helper that selects the most recent matching activity entry.
  Verification:
    - `go test ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This reuses persisted deck activity instead of introducing a second OIDC-apply state store.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Add deck-side OIDC onboarding apply controls
  Summary:
    - Extended `/deck/oidc-migration-apply.json` so the deck can pass the same practical Keycloak/Authentik onboarding options as the CLI: bootstrap passwords, Keycloak temporary-password choice, Keycloak execute-actions-email settings, and Authentik recovery-email delivery.
    - Updated the deck UI to prompt for those provider-specific knobs before running OIDC apply, so the workflow card is no longer a stripped-down wrapper over the backend migration path.
    - Added validation and regression coverage for the new request shape, including negative Keycloak email-lifespan rejection and activity logging of the apply options.
  Verification:
    - `go test ./internal/webui ./internal/migrationident ./internal/keycloak ./internal/authentik ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The deck still does not persist reusable provider-onboarding presets; it prompts interactively each time.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Add Keycloak onboarding bootstrap options
  Summary:
    - Extended the Keycloak apply path with optional bootstrap-password and execute-actions-email support so IdP migration can now do basic onboarding, not just user/group provisioning.
    - Added the corresponding CLI flags to `identity-migration-keycloak-apply` and kept the provider-agnostic OIDC plan unchanged.
    - Updated docs and current-task state to reflect that the first live IdP backend can now help with staged onboarding too.
  Verification:
    - `go test ./internal/keycloak ./internal/migrationident ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This still does not attempt full Keycloak attribute sync or arbitrary required-action policy mapping.
  Opportunities filed:
    - none
  Links:
    - `internal/keycloak/keycloak.go`
    - `cmd/iptv-tunerr/cmd_identity_migration.go`

- Date: 2026-03-22
  Title: Surface station-ops recovery in the deck
  Summary:
    - Added the virtual recovery endpoint to the deck endpoint catalog and raw inspector.
    - The Programming lane now shows a Virtual recovery history card built from `/api/virtual-channels/recovery-report.json?limit=8`.
    - Programming detail panels now include recent virtual recovery context alongside schedule context.
  Verification:
    - `go test ./internal/webui ./internal/tuner ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This is UI wiring only; it reuses the existing tuner-side recovery report instead of inventing a separate deck backend surface.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`
    - `docs/features.md`

- Date: 2026-03-22
  Title: Add branded-default virtual channel publishing
  Summary:
    - Added `published_stream_url` to virtual channel detail responses so operators can see which stream surface is actually being exported.
    - Added `IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT` so branded virtual channels can publish the branded stream path directly in `/virtual-channels/live.m3u`.
    - Surfaced the same env in the runtime snapshot so the deck/settings lane can inspect the publish posture.
  Verification:
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - The branded-default mode is opt-in and only affects channels that actually carry branding metadata.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `cmd/iptv-tunerr/cmd_runtime_server.go`
    - `docs/reference/virtual-channel-stations.md`

- Date: 2026-03-22
  Title: Probe actual virtual response bytes before filler cutover
  Summary:
    - Extended virtual recovery evaluation so it buffers a bounded startup sample from the real upstream response body and reconstructs the response afterward.
    - Added ffmpeg byte-sampled `blackdetect` / `silencedetect` probing on those buffered bytes, producing reasons like `content-blackdetect-bytes`.
    - Updated recovery docs/tests to distinguish URL-preflight recovery from sampled-response-body recovery.
  Verification:
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This is still startup-sampled detection, not continuous in-stream monitoring across long-running sessions.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `docs/reference/virtual-channel-stations.md`

- Date: 2026-03-22
  Title: Add per-channel virtual publish mode overrides
  Summary:
    - Added `branding.stream_mode` (`plain`, `branded`, or auto/empty) to the virtual-channel rules schema.
    - Published virtual stream URLs now respect the per-channel stream mode before falling back to the process-wide branding default env.
    - Updated station docs/tests so the override is treated as part of the authoring contract, not a hidden implementation detail.
  Verification:
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This is a server/schema capability now; the deck still needs direct mutation UX for the new field.
  Opportunities filed:
    - none
  Links:
    - `internal/virtualchannels/virtualchannels.go`
    - `internal/tuner/server.go`
    - `docs/reference/virtual-channel-stations.md`

- Date: 2026-03-22
  Title: Add virtual station report surface
  Summary:
    - Added `GET /virtual-channels/report.json` with per-station publish mode, published stream URL, resolved-now slot, branded/slate URLs, and recent recovery history.
    - Wired the deck Programming lane to consume that report for virtual-station posture cards/context instead of only stitching schedule and recovery payloads together.
    - Added tuner/web UI coverage so the endpoint and deck wiring are now part of the tested surface.
  Verification:
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This is still read-only operator visibility; station mutation UX remains follow-on work.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/webui/deck.js`
    - `docs/reference/virtual-channel-stations.md`

- Date: 2026-03-22
  Title: Add deck controls for per-station publish mode
  Summary:
    - Added Programming-lane controls to force `plain`, force `branded`, or reset `auto` for virtual station publish mode.
    - Changed virtual channel branding mutation handling so partial branding updates merge into the existing branding object instead of replacing it.
    - Added regression coverage so a stream-mode-only update does not drop existing logo metadata.
  Verification:
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This only covers `stream_mode` today; broader deck-side branding editors remain follow-on work.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Persist runtime tuning knobs through the deck state file
  Summary:
    - Extended the existing `IPTV_TUNERR_WEBUI_STATE_FILE` settings payload to persist `shared_relay_replay_bytes` and `virtual_channel_recovery_live_stall_sec`.
    - Replayed those persisted values back into the tuner on web UI startup through the existing localhost operator actions.
    - Kept the deck Settings lane aligned so it saves both the persistent setting values and the live runtime action updates together.
  Verification:
    - `go test ./internal/webui -run 'Test(PersistStateExcludesTelemetryAndAuthSecret|LoadStateRestoresRuntimeSettings|ApplyPersistedRuntimeAction)$' -v`
    - `node -c internal/webui/deck.js`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - Persistence only happens when `IPTV_TUNERR_WEBUI_STATE_FILE` is configured; otherwise the knobs remain process-local.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`
    - `internal/webui/deck.js`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Add live operator control for virtual live-stall recovery
  Summary:
    - Added localhost-only `POST /ops/actions/virtual-channel-live-stall` so the live-stall watchdog can be updated at runtime for new virtual-channel sessions.
    - Exposed that action and current seconds in `/ops/actions/status.json`.
    - Wired the deck Settings lane to edit and save `virtual_channel_recovery_live_stall_sec` alongside shared replay bytes.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - Like shared replay bytes, this is a live process knob that applies to new sessions, not persisted config.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `internal/webui/deck.js`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Add bounded live-session fallback recovery for virtual channels
  Summary:
    - Wrapped filler-enabled virtual stream bodies in a recoverable relay so the plain/branded virtual playback paths can perform one midstream cutover to filler when the active upstream stalls or hard-errors after startup.
    - Added `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC` and surfaced it in `/debug/runtime.json` plus the deck Settings lane.
    - Added a regression proving a live-stalling upstream can switch to filler in the same session and record `live-stall-timeout` in the recovery report.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - The runtime is still bounded to one live-session cutover, not repeated decoded-media health analysis.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `cmd/iptv-tunerr/cmd_runtime_server.go`
    - `internal/webui/deck.js`

- Date: 2026-03-22
  Title: Add merge-safe virtual recovery controls to the deck
  Summary:
    - Added merge-safe `recovery` mutation handling plus `recovery_clear` so partial station recovery edits no longer wipe existing filler entries.
    - Extended `/virtual-channels/report.json` with `recovery_mode`, `black_screen_seconds`, and fallback-entry counts for station cards.
    - Added deck controls for `Disable Recovery`, `Enable Filler`, and `Black Sec` editing on virtual-station cards.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This is still startup-window recovery control, not true continuous midstream cutover.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `internal/webui/deck.js`
    - `docs/reference/virtual-channel-stations.md`

- Date: 2026-03-22
  Title: Extend deck branding edits and add recovery warmup control
  Summary:
    - Extended the deck’s virtual-station cards to edit `logo_url`, `bug_text`, and `banner_text` in addition to `stream_mode`.
    - Added `branding_clear` handling so empty deck submissions can clear specific branding fields without wiping the rest of the branding object.
    - Added `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_WARMUP_SEC` so startup response-byte monitoring can run longer than the per-channel black-screen timeout when desired.
  Verification:
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - Recovery is still startup/warmup scoped, not a continuous midstream cutover engine.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/tuner/server.go`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Finish first-pass deck branding controls
  Summary:
    - Extended the deck’s virtual-station cards to edit `bug_image_url`, `bug_position`, and `theme_color`, completing the first-pass branding field set.
    - Extended the virtual station report rows so the deck can populate those editors from current server-side values.
    - Surfaced `virtual_channel_branding_default` and `virtual_channel_recovery_warmup_sec` in the Settings lane via the existing runtime snapshot.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/tuner ./internal/webui ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This closes the obvious first-step station-branding UI gap; the next meaningful work is runtime behavior, not more controls.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/tuner/server.go`
    - `docs/reference/virtual-channel-stations.md`

- Date: 2026-03-22
  Title: Deepen station-ops recovery and branded playback
  Summary:
    - Added in-memory virtual recovery event tracking plus `GET /virtual-channels/recovery-report.json` so filler cutovers and content-probe recoveries are inspectable.
    - Extended per-channel virtual detail reports to include recent recovery history rather than only static metadata and schedule views.
    - Upgraded `/virtual-channels/branded-stream/<id>.ts` to share the same fallback/content-probe logic as the plain stream path and to support a first corner-image overlay lane via `branding.bug_image_url` or `branding.logo_url`.
  Verification:
    - `go test ./internal/tuner ./internal/virtualchannels ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - Recovery detection is still preflight/startup-oriented, not continuous in-stream decoded-media analysis.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `docs/reference/virtual-channel-stations.md`
    - `docs/epics/EPIC-station-ops.md`

- Date: 2026-03-22
  Title: Add Keycloak identity migration diff/apply
  Summary:
    - Added `internal/keycloak` with the first live IdP admin integration for users, groups, and membership.
    - Extended the identity migration lane so the provider-agnostic OIDC plan can now be diffed against and applied to a real Keycloak realm.
    - Added CLI commands `identity-migration-keycloak-diff` and `identity-migration-keycloak-apply`, and updated docs/current-task state to reflect that Keycloak is now the first supported IdP backend.
  Verification:
    - `go test ./internal/keycloak ./internal/migrationident ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Current Keycloak scope is intentionally limited to user/group creation and membership. Credentials and required actions remain future work.
  Opportunities filed:
    - none
  Links:
    - `internal/keycloak/keycloak.go`
    - `cmd/iptv-tunerr/cmd_identity_migration.go`

- Date: 2026-03-22
  Title: Add provider-agnostic OIDC migration planning
  Summary:
    - Added `BuildOIDCPlan` to `internal/migrationident` so the same Plex user bundle can now emit stable OIDC subject hints, usernames, display names, email hints, and Tunerr-owned migration group claims.
    - Added the CLI command `identity-migration-oidc-plan` so that OIDC planning is a first-class built-in feature, not an external script or ad hoc export.
    - Updated docs and current-task state to position this as the neutral foundation for future Authentik/Keycloak/Caddy-backed integration rather than a provider-specific apply path.
  Verification:
    - `go test ./internal/migrationident ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This slice is intentionally export-only. It does not talk to a live IdP yet.
  Opportunities filed:
    - none
  Links:
    - `internal/migrationident/bundle.go`
    - `cmd/iptv-tunerr/cmd_identity_migration.go`

- Date: 2026-03-22
  Title: Add identity activation-readiness reporting
  Summary:
    - Extended the Emby/Jellyfin helper layer to classify destination users as activation-pending when they still have no configured password or auto-login path.
    - Extended identity diff/apply/audit so `activation_pending_users` is reported separately from `missing_users` and `policy_update_users`, which makes overlap cutover reports distinguish “account exists” from “user can actually sign in.”
    - Updated docs and current-task state so the identity migration lane now explicitly covers additive policy parity plus activation-readiness reporting, while still leaving real password/invite/OIDC provisioning as future work.
  Verification:
    - `go test ./internal/emby ./internal/migrationident ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Activation readiness is intentionally heuristic and local-user oriented: no configured password or auto-login path means “pending” unless the account is disabled.
  Opportunities filed:
    - none
  Links:
    - `internal/emby/users.go`
    - `internal/migrationident/bundle.go`

- Date: 2026-03-22
  Title: Add additive identity policy parity for Emby/Jellyfin cutover
  Summary:
    - Extended `internal/emby` with full user fetch plus `/Users/{id}/Policy` update support so Tunerr can preserve and update destination user policy safely instead of hand-building partial replacement payloads.
    - Extended `internal/migrationident` plans/diffs/applies/audits so Plex share signals now drive additive destination policy sync for Live TV access, sync/download access, all-library access, and remote access for shared users.
    - Updated the identity audit/reporting contract so operators can distinguish `missing_users`, `policy_update_users`, and truly manual follow-up cases instead of treating every share/tuner entitlement as manual cleanup.
    - Updated migration docs and current-task state to reflect that identity migration is no longer local-user bootstrap only.
  Verification:
    - `go test ./internal/emby ./internal/migrationident ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is intentionally additive-only parity. It still does not infer folder-specific grants, invite activation state, or OIDC/Caddy identity lifecycle.
  Opportunities filed:
    - none
  Links:
    - `internal/emby/users.go`
    - `internal/migrationident/bundle.go`

- Date: 2026-03-22
  Title: Add first-class Plex-user identity migration commands
  Summary:
    - Added `internal/plex` user export plus `internal/emby` destination user list/create helpers.
    - Added `internal/migrationident` and new CLI commands (`plex-user-bundle-build`, `identity-migration-convert`, `identity-migration-diff`, `identity-migration-apply`, `identity-migration-rollout`, `identity-migration-rollout-diff`) so Tunerr can build, diff, and apply overlap-friendly Emby/Jellyfin account bootstrap plans from Plex users.
    - Added `identity-migration-audit` plus compact summary output so the same identity bundle can now answer readiness/follow-up questions per target instead of only raw create/reuse counts.
    - Exposed that identity audit in the dedicated deck at `/deck/identity-migration-audit.json` so account-cutover readiness is visible from the running process too.
    - Updated migration docs and backlogged the broader “general-purpose library janitor” direction in `memory-bank/opportunities.md` and `docs/explanations/project-backlog.md`.
  Verification:
    - `go test ./internal/plex ./internal/emby ./internal/migrationident ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This slice intentionally does not clone passwords, invites, OIDC identities, or library-permission parity; it is local-user bootstrap only.
  Opportunities filed:
    - `memory-bank/opportunities.md` — `Grow Tunerr into a full general-purpose library janitor`
  Links:
    - `internal/migrationident/bundle.go`
    - `cmd/iptv-tunerr/cmd_identity_migration.go`
    - `docs/emby-jellyfin-support.md`

- Date: 2026-03-22
  Title: Add experimental AAC variant of the TV-safe transcode profile
  Summary:
    - Added a new built-in profile `plexsafeaac` in `internal/tuner/gateway_profiles.go` so the high-quality TV-safe lane can be A/B tested with AAC audio instead of MP3.
    - Added focused regression coverage for profile normalization and ffmpeg args in `internal/tuner/gateway_profiles_test.go`.
    - Updated profile/env reference docs while intentionally leaving the live helper on the known-good `plexsafemax` runtime.
  Verification:
    - `go test ./internal/tuner -run 'Test(BuildFFmpegStreamCodecArgs_(plexsafeHQ|plexsafeMax|plexsafeAAC)|NormalizeProfileName_HDHRStyleAliases)$'`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - `plexsafeaac` is not live yet; it exists only as an explicit next-step experiment because AAC may not satisfy the same PMS/LG compatibility path that MP3 fixed.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_profiles.go`
    - `internal/tuner/gateway_profiles_test.go`
    - `docs/reference/transcode-profiles.md`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Add higher-quality TV-safe Plex transcode profile
  Summary:
    - Added a new built-in profile `plexsafemax` in `internal/tuner/gateway_profiles.go` so the TV-safe lane keeps the same compatibility shape as `plexsafehq` while using a slower preset, lower CRF, and higher bitrate ceilings.
    - Updated the host-local `:5005` runtime so browser playback stays on `copyvideomp3` and ambiguous/internal PMS fetchers use `plexsafemax`.
    - Fixed an unrelated compile blocker in `internal/emby/register.go` where `TriggerGuideRefresh` had lost local `client/status` declarations.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `go test ./internal/tuner -run 'TestBuildFFmpegStreamCodecArgs_(plexsafeHQ|plexsafeMax)$'`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
    - `curl -s http://127.0.0.1:5005/debug/runtime.json | jq '.tuner | {force_websafe_profile,plex_web_client_profile,plex_internal_fetcher_profile,stream_transcode,count}'`
  Notes:
    - The new profile still uses MP3 audio and preserves source resolution; this pass intentionally avoided AAC or forced upscaling.
    - The live `:5005` helper is currently running in exec session `26611`.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_profiles.go`
    - `internal/tuner/gateway_profiles_test.go`
    - `internal/emby/register.go`
    - `docs/reference/transcode-profiles.md`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Add source-vs-destination library parity hints to the migration audit
  Summary:
    - Added Plex library section item-count reads and carried those counts through the neutral migration bundle and library migration plans.
    - Extended library diffs and the combined migration audit with `source_item_count`, `existing_item_count`, `parity_status`, and rolled-up `synced_libraries` / `lagging_libraries`.
    - Updated README and migration docs so the audit is now described as comparing destination library presence against both destination counts and source Plex counts.
  Verification:
    - `go test ./internal/plex ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Parity is still aggregate-count based; it does not yet verify title-level or metadata-level equivalence.
  Opportunities filed:
    - none
  Links:
    - `internal/plex/library.go`
    - `internal/plex/library_test.go`
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Add best-effort library scan status to the migration audit
  Summary:
    - Added `GetLibraryScanStatus(...)` in `internal/emby`, backed by scheduled-task inspection for recognizable library refresh/scan tasks.
    - Extended the combined migration audit with `library_scan` per target so overlap migrations can see coarse scan running/state/progress hints after library apply.
    - Updated the migration docs to frame the new fields as visibility only, not a readiness or convergence gate.
  Verification:
    - `go test ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Library scan status is best-effort and depends on the destination server exposing recognizable scheduled-task metadata.
  Opportunities filed:
    - none
  Links:
    - `internal/emby/library.go`
    - `internal/emby/register.go`
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Add coarse library population hints to the migration audit
  Summary:
    - Added `GetLibraryItemCount(...)` in `internal/emby` and enriched reused library diff rows with `existing_item_count`.
    - Extended the combined migration audit with `populated_libraries` and `empty_libraries` so reused library definitions can be distinguished from still-empty destination libraries.
    - Updated README and migration docs to describe the new audit hints as visibility only, not a readiness/convergence gate change.
  Verification:
    - `go test ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Population is currently a coarse item-count signal only; it does not yet model deeper scan/index/metadata ingest progress.
  Opportunities filed:
    - none
  Links:
    - `internal/emby/library.go`
    - `internal/emby/library_test.go`
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Split Plex browser and TV-safe fallback profiles on the host-local helper
  Summary:
    - Added client-class-specific WebSafe profile selection in `internal/tuner/gateway_adapt.go` so the adaptation `websafe` branch no longer has to use one shared profile for browser and TV/internal PMS fetchers.
    - Added new env overrides `IPTV_TUNERR_PLEX_WEB_CLIENT_PROFILE`, `IPTV_TUNERR_PLEX_NATIVE_CLIENT_PROFILE`, and `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE`, exposed them in `/debug/runtime.json`, and documented them in the CLI/env reference.
    - Restarted the live `:5005` helper with `copyvideomp3` for resolved web clients and `plexsafehq` for internal/no-hints PMS fetchers so browser playback can stay lighter while TV playback keeps the stricter compatibility transcode.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `go test ./internal/tuner -run 'TestGateway_requestAdaptation_(resolvedWebUsesWebProfileOverride|internalFetcherUsesInternalProfileOverride|unknownInternalFetcherAmbiguousFallsBackToInternalPolicy)$'`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
    - `curl -s http://127.0.0.1:5005/debug/runtime.json | jq '.tuner | {stream_transcode,client_adapt,force_websafe_profile,plex_web_client_profile,plex_internal_fetcher_profile,plex_unknown_client_policy,plex_internal_fetcher_policy,plex_resolve_error_policy,count}'`
  Notes:
    - Global runtime still reports `stream_transcode="off"`; the new behavior only changes which profile the adaptation layer chooses when it decides a request must go to the `websafe` branch.
    - The live `:5005` helper is currently running in exec session `40620`.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_adapt.go`
    - `internal/tuner/gateway_test.go`
    - `cmd/iptv-tunerr/cmd_runtime_server.go`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Restore remux-first host-local Plex adaptation policy
  Summary:
    - Added explicit Plex adaptation policy envs in `internal/tuner/gateway_adapt.go`: `IPTV_TUNERR_PLEX_UNKNOWN_CLIENT_POLICY`, `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY`, and `IPTV_TUNERR_PLEX_RESOLVE_ERROR_POLICY`.
    - Added request-adaptation tests proving `direct` policy keeps remux/transcode-off for unknown and internal-fetcher cases, and surfaced the new policy keys in `/debug/runtime.json`.
    - Updated docs and restarted the host-local `:5005` helper in remux-first mode with the new policies set to `direct`, keeping the music-drop, lineup-shaping, and FFmpeg DNS-rewrite fixes in place.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
    - `curl -fsS http://127.0.0.1:5005/debug/runtime.json | jq '.tuner | {stream_transcode, plex_unknown_client_policy, plex_internal_fetcher_policy, plex_resolve_error_policy}'`
    - Fresh live stream probe on `15578` logged `plex-hints none`, `hls-mode transcode=false mode="off"`, and `ffmpeg-remux profile=default`
  Notes:
    - This restores remux as the first attempt, but it does not fully automate fallback for the PMS-only failure class where `universal/decision` fails after Tunerr already served a healthy stream.
    - The live `:5005` helper is currently running in exec session `17057`.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_adapt.go`
    - `internal/tuner/gateway_test.go`
    - `cmd/iptv-tunerr/cmd_runtime_server.go`
    - `docs/reference/cli-and-env-reference.md`
    - `docs/reference/transcode-profiles.md`

- Date: 2026-03-22
  Title: Add multi-target Live TV rollout planning for Emby and Jellyfin
  Summary:
    - Added `RolloutPlan` / `RolloutApplyResult` helpers in `internal/livetvbundle` so one neutral Plex-derived bundle can build or apply coordinated Emby+Jellyfin Live TV targets.
    - Added `iptv-tunerr live-tv-bundle-rollout`, which can either emit a multi-target rollout artifact or apply it directly while intentionally leaving Plex untouched.
    - Updated the Emby/Jellyfin support docs, CLI reference, and changelog to describe coordinated overlap migration instead of only single-target apply.
  Verification:
    - `gofmt -w internal/livetvbundle/bundle.go internal/livetvbundle/bundle_test.go cmd/iptv-tunerr/cmd_live_tv_bundle.go cmd/iptv-tunerr/cmd_live_tv_bundle_test.go`
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This rollout lane is still intentionally scoped to Live TV registration state, not catch-up/library sync.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `docs/emby-jellyfin-support.md`

- Date: 2026-03-22
  Title: Extend migration bundle flow into library definitions
  Summary:
    - Expanded `internal/livetvbundle` so migration bundles can optionally include Plex library sections and shared storage paths alongside Live TV data.
    - Added `library-migration-convert` and `library-migration-apply` so bundled Plex movie/show libraries can be turned into Emby/Jellyfin library plans and applied through built-in server APIs.
    - Updated README plus Emby/Jellyfin and CLI docs to make the migration boundary explicit: library definitions and paths migrate, but vendor metadata databases do not.
  Verification:
    - `gofmt -w internal/livetvbundle/bundle.go internal/livetvbundle/bundle_test.go cmd/iptv-tunerr/cmd_live_tv_bundle.go cmd/iptv-tunerr/cmd_live_tv_bundle_test.go`
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This intentionally targets server-facing library config, not Plex watched-state/metadata-row translation.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `docs/emby-jellyfin-support.md`

- Date: 2026-03-22
  Title: Add multi-target library rollout planning for Emby and Jellyfin
  Summary:
    - Added `LibraryRolloutPlan` / `LibraryRolloutApplyResult` helpers so one neutral bundle can drive coordinated Emby+Jellyfin library rollout from the same bundled Plex library definitions.
    - Added `library-migration-rollout`, which can emit or apply a multi-target library rollout in one step while intentionally leaving Plex untouched.
    - Updated the CLI reference, Emby/Jellyfin support docs, and changelog so coordinated library overlap migration is documented alongside the Live TV rollout flow.
  Verification:
    - `gofmt -w internal/livetvbundle/bundle.go internal/livetvbundle/bundle_test.go cmd/iptv-tunerr/cmd_live_tv_bundle.go cmd/iptv-tunerr/cmd_live_tv_bundle_test.go`
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This still migrates server-facing library definitions only; vendor-specific metadata/state translation remains explicitly out of scope.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `docs/emby-jellyfin-support.md`

- Date: 2026-03-22
  Title: Add built-in Live TV migration apply flow for Emby and Jellyfin
  Summary:
    - Added reusable `internal/livetvbundle` plan-apply helpers so a converted Live TV bundle can be turned into a real Emby/Jellyfin registration using the existing built-in registration APIs instead of external scripts.
    - Added `iptv-tunerr live-tv-bundle-apply` with target-aware host/token env fallback and optional state-file persistence so migration now has a built-in build → convert → apply path.
    - Updated README and Emby/Jellyfin/CLI/changelog docs to describe gradual Plex + Emby/Jelly overlap as a supported migration shape rather than a forced one-shot cutover.
    - Repaired unrelated compile drift in `internal/plex/dvr.go` (`ChannelMappings` field usage) so the full command package builds cleanly again.
  Verification:
    - `gofmt -w internal/livetvbundle/bundle.go internal/livetvbundle/bundle_test.go cmd/iptv-tunerr/cmd_live_tv_bundle.go cmd/iptv-tunerr/cmd_live_tv_bundle_test.go`
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `go test ./internal/plex -run 'Test(ActivateChannelsAPI_keepsFullEnabledSetAcrossBatches|RepairDVRChannelActivation.*)$'`
    - `./scripts/verify`
  Notes:
    - The migration lane still targets supported registration APIs, not raw media-server DB writes.
    - The next useful slice is a dual-register/sync workflow that makes simultaneous Plex + Emby/Jelly migration explicit.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `docs/emby-jellyfin-support.md`

- Date: 2026-03-22
  Title: Repair blank Plex XMLTV provider tabs caused by stale DVR activation
  Summary:
    - Traced the fully blank `plexkube` TV source to PMS provider state rather than raw Tunerr guide emptiness: `GET /tv.plex.providers.epg.xmltv:723/lineups/dvr/channels` was returning `size="0"` while Tunerr still exposed 479 lineup rows and a non-empty `guide.xml`.
    - Compared the active DVR row against the current channelmap and found the real break: DVR `723` still had 475 enabled IDs, but only 3 overlapped the current valid channelmap; the current map contained 380 complete mappings and 99 incomplete rows.
    - Reapplied activation against the exact current valid map for device `722`, then reloaded the guide. PMS immediately repopulated the provider surface: `/tv.plex.providers.epg.xmltv:723/lineups/dvr/channels` now returns `size="80"` and `/tv.plex.providers.epg.xmltv:723/hubs/discover` returns real items again.
    - Added `plex-dvr-repair` to the CLI so this repair can be replayed without hand-crafted PUT requests; the command permits localhost use without an explicit token in this environment.
  Verification:
    - `curl -s http://127.0.0.1:32400/tv.plex.providers.epg.xmltv:723/lineups/dvr/channels`
    - `curl -s 'http://127.0.0.1:32400/tv.plex.providers.epg.xmltv:723/hubs/discover?includeCollections=1&includeExternalMedia=1&includePreferences=1&includeActions=1&stationKey=1'`
    - `curl -s http://127.0.0.1:32400/livetv/dvrs/723`
    - `go test ./internal/plex ./cmd/iptv-tunerr`
  Notes:
    - This fixes the blank provider tab, but PMS is still surfacing only 80 channels from the repaired current-valid subset. The next investigation is why 300+ otherwise-valid current channelmap rows are still not appearing in the provider layer.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_plex_ops.go`
    - `internal/plex/dvr.go`

- Date: 2026-03-22
  Title: Raise forced Plex-safe quality for the host-local DVR path
  Summary:
    - Added built-in profile `plexsafehq` in `internal/tuner/gateway_profiles.go` so the forced compatibility path can keep MP3 audio for Plex/client tolerance while using `setsar=1`, `crf=18`, and much higher bitrate and mux ceilings than the old `plexsafe` workaround.
    - Added `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE` in `internal/tuner/gateway_adapt.go`, which lets forced-websafe use a higher-quality built-in or named profile instead of hardcoding `plexsafe`.
    - Updated the live host-local tuner on `http://127.0.0.1:5005` to run with `IPTV_TUNERR_FORCE_WEBSAFE=true`, `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE=plexsafehq`, and `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE=true`; direct probe logs now show `profile=\"plexsafehq\"`.
    - Updated the operator docs in `docs/reference/transcode-profiles.md` and `docs/reference/cli-and-env-reference.md` to record the new profile and env knob.
  Verification:
    - `gofmt -w internal/tuner/gateway_profiles.go internal/tuner/gateway_adapt.go internal/tuner/gateway_profiles_test.go internal/tuner/gateway_test.go`
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
    - `curl -fsS http://127.0.0.1:5005/discover.json`
    - `curl -fsS http://127.0.0.1:5005/lineup_status.json`
    - `curl --max-time 8 -o /tmp/plexsafehq-probe.ts -sS http://127.0.0.1:5005/stream/1019880`
    - `ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,width,height,display_aspect_ratio,sample_aspect_ratio,avg_frame_rate -of json /tmp/plexsafehq-probe.ts`
    - `ffprobe -v error -select_streams a:0 -show_entries stream=codec_name,channels,sample_rate,bit_rate -of json /tmp/plexsafehq-probe.ts`
  Notes:
    - The probe output now reports sane geometry/audio (`1280x720`, `SAR 1:1`, `DAR 16:9`, `MP3 stereo 48 kHz 192k`), which directly targets the earlier TV complaints about bad aspect and low quality. TV-side validation is still required because longer sessions can still hit upstream `509` limits later.
  Opportunities filed:
    - none
  Links:
    - `docs/reference/transcode-profiles.md`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Surface and smoke-proof shared relay replay controls
  Summary:
    - Documented `IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES` in the CLI/env reference and clarified that `/debug/shared-relays.json` now reports generic shared-output sessions, not only `hls_go`.
    - Added `shared_relay_replay_bytes` to the runtime snapshot so the operator plane exposes the configured replay window alongside the other stream-shaping knobs.
    - Tightened `scripts/ci-smoke.sh` so the real temp-binary smoke asserts replay-prefixed startup bytes on the deterministic FFmpeg shared TS and fMP4 lanes instead of only checking that duplicate viewers received non-empty output.
  Verification:
    - `bash -n scripts/ci-smoke.sh`
    - `go test ./internal/tuner -run 'TestGateway_(sharedRelaySessionLateSubscriberGetsReplay|stream_sameChannelFFmpegRelayReusesExistingSession|stream_sameChannelFFmpegFMP4ReusesExistingSession)$|TestGateway_ffmpegPackagedHLS_(sameProfileReusesExistingSession|lookupReusableHLSPackagerSessionDropsExitedSession)$|TestServer_SharedRelayReport$'`
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - The stricter replay-prefix assertion is intentionally limited to the deterministic FFmpeg smoke lanes; `hls_go` still relays valid MPEG-TS packet bytes, but that packetization makes a literal ASCII prefix check the wrong contract there.
  Opportunities filed:
    - none
  Links:
    - `docs/reference/cli-and-env-reference.md`
    - `cmd/iptv-tunerr/cmd_runtime_server.go`
    - `scripts/ci-smoke.sh`

- Date: 2026-03-22
  Title: Surface shared replay sizing in the deck runtime view
  Summary:
    - Updated the dedicated deck runtime/settings card to show `Shared replay bytes` alongside the other tuner and transport posture fields.
    - Added a regression test that asserts the served `deck.js` asset still contains both the replay label and the `shared_relay_replay_bytes` runtime field.
  Verification:
    - `go test ./internal/webui -run 'Test(IndexAndAssetsRequireGetOrHead|DeckJSIncludesSharedReplaySetting)$'`
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - This is intentionally a visibility-only pass; replay sizing is still configured by env/runtime, not mutated live from the deck.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Add live operator control for shared replay bytes
  Summary:
    - Added localhost-only tuner action `/ops/actions/shared-relay-replay` so operators can update `shared_relay_replay_bytes` for new shared sessions without restarting the process.
    - Wired the action into `/ops/actions/status.json`, the deck Settings save flow, and the runtime snapshot so the live control is both discoverable and immediately visible after mutation.
    - Updated feature/reference/changelog docs so this ships as a documented operator feature rather than a hidden endpoint.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(operatorActionStatus|sharedRelayReplayUpdateAction|sharedRelayReplayUpdateActionRejectsNegative)$'`
    - `go test ./internal/webui -run 'Test(DeckJSIncludesSharedReplaySetting|DeckSettingsGETAndPOST|IndexAndAssetsRequireGetOrHead)$'`
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - The updated replay size applies to new shared live sessions only; it does not resize already-running shared sessions in place.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `internal/webui/deck.js`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Separate smart-TV Live TV failure from backend recorder failure
  Summary:
    - Correlated the smart-TV browse attempt on `45955` with PMS logs showing `GET /video/:/transcode/universal/decision` failing immediately with `Invalid argument`, which is a different failure mode from the later backend recorder errors on long-running manual tunes.
    - Confirmed the local tuner steady-state stream for `43784` is valid enough for `ffprobe` to see `aac stereo 48000`, while Plex live-session XML for both `45955` and `43784` still records `audioChannelLayout="0 channels"` and `samplingRate="0"`, narrowing the defect to early startup/probe behavior.
    - Restarted the host-local `:5005` tuner with `IPTV_TUNERR_FORCE_WEBSAFE=true` so Plex requests are forced onto the `plexsafe` transcode/bootstrap path; under that runtime, PMS again starts manual rolling subscriptions on `45955` instead of failing immediately.
    - Fixed a masking bug in `internal/tuner/gateway_stream_response.go` so FFmpeg HLS failures log the real error instead of the stale outer `err`, then proved the real root cause: FFmpeg input-host IP rewriting was breaking Cloudflare-backed HLS startup. Running the local tuner with `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE=true` let FFmpeg keep the hostname, pass the startup gate, emit bootstrap TS, and stream real transcoded bytes for `1019880`.
  Verification:
    - `ffprobe -v error -show_entries stream=index,codec_type,codec_name,profile,width,height,r_frame_rate,sample_rate,channels,channel_layout -of json -i 'http://127.0.0.1:5005/stream/1002837'`
    - `curl -fsS -X POST 'http://127.0.0.1:32400/livetv/dvrs/723/channels/45955/tune?X-Plex-Token=…' -H 'X-Plex-Session-Identifier: fw45955…'`
    - inspected `/var/lib/plex-standby-config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log`
    - inspected live tuner session `30511`
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
    - `curl -sS --max-time 18 -o /tmp/tunerr-probe2.bin 'http://127.0.0.1:5005/stream/1019880'`
  Notes:
    - The forced-websafe plus no-DNS-rewrite runtime is still an operator workaround, not a committed default.
    - Provider `509` concurrency limits can still end long-running sessions even when startup is fixed.
  Opportunities filed:
    - none
  Links:
    - `memory-bank/current_task.md`
    - `memory-bank/known_issues.md`
    - `internal/tuner/gateway_stream_response.go`

- Date: 2026-03-22
  Title: Expand and document shared-output reuse across FFmpeg paths
  Summary:
    - Extended the gateway so same-channel viewers requesting the same live FFmpeg HLS output shape can attach to the existing producer, and kept the earlier packaged-HLS reuse path keyed by resolved profile/output contract.
    - Added bounded replay buffering to shared sessions so late subscribers receive recent startup bytes, which makes attached `fMP4` consumers materially safer than a pure live-only attach.
    - Added focused gateway regressions for same-profile packaged-HLS reuse, stale exited-session cleanup, generic shared-relay reporting, replay delivery, and live FFmpeg HLS TS/fMP4 producer sharing.
    - Updated `README.md`, `docs/features.md`, `docs/reference/transcode-profiles.md`, `docs/explanations/release-readiness-matrix.md`, `docs/CHANGELOG.md`, and `scripts/ci-smoke.sh` so the broader shared-output feature is documented and binary-smoke proven.
    - Repaired unrelated compile drift in `internal/tuner/epg_pipeline.go` so the repo would build cleanly again while widening verification.
  Verification:
    - `go test ./internal/tuner -run 'TestGateway_(sharedRelaySessionFanout|tryServeSharedRelay|stream_sameChannelFFmpegRelayReusesExistingSession)|TestServer_SharedRelayReport|TestGateway_ffmpegPackagedHLS_(namedProfileServesPlaylistAndSegment|targetRequiresGetOrHead|sameProfileReusesExistingSession|lookupReusableHLSPackagerSessionDropsExitedSession)$'`
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
    - full format/vet/test/build/smoke included in `./scripts/verify`
  Notes:
    - Shared-output reuse is intentionally keyed to the resolved output contract, not to arbitrary mixed mux/profile requests.
    - The native `hls_go` relay, live FFmpeg HLS output, and packaged-HLS reuse paths are now all binary-smoke covered; the new replay buffer is still a bounded startup replay, not full DVR-style rewind.
  Opportunities filed:
    - none
  Links:
    - `README.md`
    - `docs/reference/transcode-profiles.md`
    - `scripts/ci-smoke.sh`

- Date: 2026-03-22
  Title: Add provider short-EPG guide fallback
  Summary:
    - Added an opt-in fallback in `internal/tuner/epg_pipeline.go` that queries upstream `player_api.php?action=get_short_epg` when provider `xmltv.php` is unavailable.
    - Verified the local `:5005` tuner can now build real programme blocks for a subset of channels instead of serving only one-week placeholders.
    - Confirmed PMS playback still works on a guide-backed video channel after enabling the fallback.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
    - `curl -sS http://127.0.0.1:5005/guide/health.json | jq '.summary'`
    - `curl -sS 'http://127.0.0.1:32400/livetv/dvrs/723/channels/45955/tune' -X POST -H 'X-Plex-Session-Identifier: iptvtunerr-manual-tune-723g' -H 'X-Plex-Client-Identifier: iptvtunerr-manual-client-723g'`
  Notes:
    - Short-EPG coverage is partial: 31 of 479 lineup channels gained real schedule rows on this provider.
    - The fallback is opt-in via `IPTV_TUNERR_PROVIDER_SHORT_EPG_FALLBACK=true`; it is not a complete replacement for a working `xmltv.php`.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/epg_pipeline.go`
    - `internal/tuner/epg_pipeline_test.go`
    - `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-22
  Title: Restore Plex local DVR tune compatibility via `/auto/`
  Summary:
    - Mounted `/auto/` on the tuner server so PMS can resolve HDHomeRun-style `/auto/v<guide-number>` tune paths against the rebuilt local `:5005` helper.
    - Rebuilt and restarted the local tuner, then replayed manual PMS tune requests against DVR `723`.
    - Verified that enabled video channel `43784` now tunes end-to-end through the local DVR and PMS serves a real Live TV HLS session plus TS segments.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
    - `curl -sS -X POST 'http://127.0.0.1:32400/livetv/dvrs/723/channels/43784/tune' -H 'X-Plex-Session-Identifier: iptvtunerr-manual-tune-723e' -H 'X-Plex-Client-Identifier: iptvtunerr-manual-client-723e'`
    - `curl -sS 'http://127.0.0.1:32400/livetv/sessions/3c7dd407-8251-443a-b6a8-5b771116941c/iptvtunerr-manual-tune-723e/index.m3u8?offset=-1.000000'`
  Notes:
    - Radio-only channel `43401` still fails later in PMS recorder startup even though PMS now reaches the tuner.
    - DVR `723` currently exposes only 75 enabled mappings despite the earlier 475-row activation report.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `memory-bank/current_task.md`
    - `memory-bank/known_issues.md`

- Date: 2026-03-22
  Title: Reject empty HDHomeRun guide bases early
  Summary:
    - Fixed `internal/hdhomerun/guide.go` so empty base inputs no longer collapse into a relative `/guide.xml` URL.
    - `FetchGuideXML` now fails locally with a clear base-url-required error instead of surfacing a later transport-layer failure.
    - Added regression coverage in `internal/hdhomerun/guide_test.go`.
  Verification:
    - `go test ./internal/hdhomerun -run 'Test(AnalyzeGuideXMLStats_sample|GuideURLFromBase|FetchGuideXMLRejectsEmptyBaseURL)$'`
    - `./scripts/verify`
  Notes:
    - This matched the earlier HDHomeRun discover/lineup helper bug class: empty bases should fail fast as local validation errors.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/guide.go`
    - `internal/hdhomerun/guide_test.go`

- Date: 2026-03-22
  Title: Preserve existing SSDP device.xml URLs
  Summary:
    - Fixed `internal/tuner/ssdp.go` so `joinDeviceXMLURL` no longer appends a second `/device.xml` when the base is already a full device XML URL.
    - Added regression coverage in `internal/tuner/ssdp_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'Test(JoinDeviceXMLURL|SSDP_searchResponse|Server_deviceXML.*)$'`
    - `./scripts/verify`
  Notes:
    - This was another helper-level normalization bug: already-complete discovery URLs should be preserved, not extended again.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/ssdp.go`
    - `internal/tuner/ssdp_test.go`

- Date: 2026-03-22
  Title: Reject empty HDHomeRun client bases early
  Summary:
    - Fixed `internal/hdhomerun/client.go` so empty base inputs no longer produce path-only `discover.json` or `lineup.json` URLs.
    - `FetchDiscoverJSON` and `FetchLineupJSON` now fail locally with clear base-url-required errors instead of surfacing lower-level transport errors.
    - Added regression coverage in `internal/hdhomerun/client_test.go`.
  Verification:
    - `go test ./internal/hdhomerun -run 'Test(ParseDiscoverReply_roundTrip|FetchLineupJSONAcceptsJSONArray|FetchDiscoverJSONFallsBackToRequestedBaseURL|DiscoverAndLineupURLFromBase_empty|FetchDiscoverJSONRejectsEmptyBaseURL|FetchLineupJSONRejectsEmptyBaseURL)$'`
    - `./scripts/verify`
  Notes:
    - This was a helper-level validation bug: empty HDHomeRun bases should fail fast as local configuration errors, not collapse into malformed relative URLs.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/client.go`
    - `internal/hdhomerun/client_test.go`

- Date: 2026-03-22
  Title: Preserve hostname-only deck proxy targets
  Summary:
    - Fixed `internal/webui/webui.go` so `proxyBase` no longer rewrites hostname-only or bare IPv6 `TunerAddr` values to `127.0.0.1`.
    - Hostname-only targets now keep their host while still defaulting the port to `5004`.
    - Added regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'TestProxy(Base|ForwardsAPIPath|InvalidBaseStaysJSON|EmptyBaseStaysJSON)$'`
    - `./scripts/verify`
  Notes:
    - This was a helper-level target-normalization bug: hostname-only deck targets were silently repointed at localhost instead of the configured host.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Fail closed on empty deck proxy targets
  Summary:
    - Fixed `internal/webui/webui.go` so the deck reverse proxy now treats empty or hostless `tunerBase` values as invalid local configuration instead of falling through to a generic `502 tuner unreachable`.
    - Kept the valid proxy path unchanged and extended regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'TestProxy(ForwardsAPIPath|InvalidBaseStaysJSON|EmptyBaseStaysJSON)$'`
    - `./scripts/verify`
  Notes:
    - This was a zero-value/partial-init edge: syntactically valid-but-empty proxy bases should fail locally as configuration errors, not as downstream reachability errors.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Accept hostname localhost in local-only gates
  Summary:
    - Fixed `internal/webui/webui.go` so the deck localhost-only wrapper accepts `localhost:port` in addition to numeric loopback IPs.
    - Fixed `internal/tuner/operator_ui.go` so operator-only localhost gating also treats hostname `localhost` as local.
    - Added regression coverage in `internal/webui/webui_test.go` and `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/webui ./internal/tuner -run 'Test(LocalhostOnlyAllowsHostnameLocalhost|Server_operatorGuidePreviewJSONAllowsHostnameLocalhost)$'`
    - `./scripts/verify`
  Notes:
    - The bug was a false-negative local access check: legitimate localhost-origin requests could be denied simply because the remote host string was a hostname instead of a parsed loopback IP.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/tuner/operator_ui.go`
    - `internal/webui/webui_test.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Keep deck rate-limit responses aligned with browser vs API flows
  Summary:
    - Fixed `internal/webui/webui.go` so blocked-login handling no longer returns JSON `429` for ordinary browser page requests.
    - Scriptable `/api` and `/deck/*` paths still return JSON `429`, while browser requests now redirect back to `/login` with `Retry-After`.
    - Added regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'TestSessionAuthOnly(BlockedBrowserRequestsStillRedirectToLogin|BlockedAPIRequestsStayJSON|RedirectsBrowserRequests|RejectsAPIsWithoutSession)$'`
    - `./scripts/verify`
  Notes:
    - The bug was a mixed-surface contract leak in the shared auth wrapper: rate-limited API behavior had accidentally become the browser behavior too.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Harden core gateway stream method contract
  Summary:
    - Fixed `internal/tuner/gateway_servehttp.go` so the core `/stream/*` gateway surface rejects mutation verbs instead of serving them like normal reads.
    - Preserved the existing HLS mux `OPTIONS` preflight behavior by advertising `Allow: GET, HEAD, OPTIONS` when `mux=hls` CORS support is enabled.
    - Added regression coverage in `internal/tuner/gateway_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestGateway_(stream_requiresGetOrHead|stream_hlsMuxMethodAllowIncludesOptionsWhenCORSEnabled|ffmpegPackagedHLS_targetRequiresGetOrHead|MaybeServeHLSMuxOPTIONS)$'`
    - `./scripts/verify`
  Notes:
    - This was deeper than the wrapper cleanup: the main stream plane itself was still method-loose after the standalone handlers had been hardened.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_servehttp.go`
    - `internal/tuner/gateway_test.go`

- Date: 2026-03-22
  Title: Harden FFmpeg-packaged HLS target method contract
  Summary:
    - Fixed `internal/tuner/gateway_hls_packager.go` so packaged HLS playlist and segment fetches under `mux=hls_ffmpeg_packager` reject mutation verbs instead of serving them like normal reads.
    - Added direct regression coverage in `internal/tuner/gateway_hls_packager_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestGateway_ffmpegPackagedHLS_(namedProfileServesPlaylistAndSegment|targetRequiresGetOrHead)$'`
    - `./scripts/verify`
  Notes:
    - This was another read-only wrapper gap outside the main `server.go` sweep, this time in the gateway’s packaged-HLS compatibility shim.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_hls_packager.go`
    - `internal/tuner/gateway_hls_packager_test.go`

- Date: 2026-03-22
  Title: Harden standalone M3U/XMLTV exports and repair Plex log-inspect drift
  Summary:
    - Fixed `internal/tuner/m3u.go` and `internal/tuner/xmltv.go` so the standalone `live.m3u` and `guide.xml` exports reject mutation verbs instead of serving them like normal reads.
    - Added direct regression coverage in `internal/tuner/m3u_test.go` and `internal/tuner/xmltv_test.go`.
    - Repaired unrelated compile drift in `internal/plex/logs.go` by moving the log aggregation helper type to package scope so the scanner helper could build again.
  Verification:
    - `go test ./internal/tuner ./internal/plex`
    - `./scripts/verify`
  Notes:
    - The real bugfix was another standalone read-only wrapper gap outside `server.go`; the `internal/plex` change was unrelated repo drift required for green verification.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/m3u.go`
    - `internal/tuner/xmltv.go`
    - `internal/tuner/m3u_test.go`
    - `internal/tuner/xmltv_test.go`
    - `internal/plex/logs.go`

- Date: 2026-03-22
  Title: Ship Plex Live TV reverse-engineering commands and clamp evidence docs
  Summary:
    - Added new Plex reverse-engineering commands for SQLite inspection, PMS API snapshots, arbitrary PMS or plex.tv request replay, PMS log mining, and share recreation testing.
    - Added operator docs for proving where IPTV Tunerr inserts Live TV state and for reproducing the non-Home `allowTuners` clamp with evidence instead of guesswork.
    - Recorded the current constraint in the memory bank: tuner and DVR objects are local PMS state, but non-Home Live TV entitlement is presently enforced by plex.tv shared-server metadata.
  Verification:
    - `gofmt -w internal/plex/inspect.go internal/plex/inspect_test.go internal/plex/home.go internal/plex/home_test.go internal/plex/logs.go internal/plex/logs_test.go cmd/iptv-tunerr/cmd_plex_ops.go`
    - N/A
    - `go test ./internal/plex ./cmd/iptv-tunerr`
    - `go test ./internal/plex ./cmd/iptv-tunerr`
  Notes:
    - The decisive live result was a silent policy clamp: recreating a non-Home share with requested `allowTuners=1` succeeded, but the resulting share still came back with `allowTuners=0`.
    - Follow-on probing also established that `api/v2/shared_servers/<share-id>` is a writable row-level mutator, but it mutates library membership/share shape rather than bypassing the non-Home tuner clamp.
    - Added a passive follow-up harness, `scripts/plex-client-browse-capture.sh`, so the next step can use real smart-TV browse traffic instead of more speculative control-plane writes.
  Opportunities filed:
    - none
  Links:
    - `internal/plex/inspect.go`
    - `internal/plex/home.go`
    - `internal/plex/logs.go`
    - `cmd/iptv-tunerr/cmd_plex_ops.go`
    - `docs/how-to/reverse-engineer-plex-livetv-access.md`

- Date: 2026-03-22
  Title: Harden standalone HDHR and probe read-only handlers
  Summary:
    - Fixed `internal/tuner/hdhr.go` so the standalone HDHomeRun compatibility endpoints reject mutation verbs instead of treating them like normal reads.
    - Fixed `internal/probe/probe.go` so `lineup.json` and `device.xml` also enforce `GET, HEAD` with a proper `Allow` contract.
    - Added direct regression coverage in `internal/tuner/hdhr_test.go` and `internal/probe/probe_test.go`.
  Verification:
    - `go test ./internal/tuner ./internal/probe`
    - `./scripts/verify`
  Notes:
    - This was a leftover compatibility-wrapper gap outside the main `server.go` sweep: the handlers were read-only but had no method contract at all.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/hdhr.go`
    - `internal/probe/probe.go`
    - `internal/tuner/hdhr_test.go`
    - `internal/probe/probe_test.go`

- Date: 2026-03-22
  Title: Finish remaining tuner JSON 405 cleanup and repair Plex test drift
  Summary:
    - Fixed the remaining JSON report/history endpoints in `internal/tuner/server.go` so runtime snapshot, event hooks, active streams, guide lineup match, and recording history no longer fall back to plain-text `405` responses.
    - Extended `internal/tuner/server_test.go` so representative operator/report/guide JSON surfaces assert JSON `405` behavior with the correct `Allow` headers.
    - Repaired unrelated repo-drift in `internal/plex` needed for green verification: restored `shareServerWithHomeUserBase`, made `plexHTTPClient` swappable in tests again, and added the missing `encoding/xml` import in `inspect_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(operatorJSONMethodRejectionsStayJSON|guideDiagnosticsFailuresStayJSON|publicReadOnlyEndpointsRequireGet)'`
    - `go test ./internal/plex ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - The primary bugfix was the last visible JSON `405` leak in tuner report/history endpoints. Full verify also exposed unrelated Plex test-harness drift that had to be repaired before the repo could go green again.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `internal/plex/home.go`
    - `internal/plex/library.go`
    - `internal/plex/home_test.go`
    - `internal/plex/inspect_test.go`

- Date: 2026-03-22
  Title: Keep guide diagnostics 405 responses JSON
  Summary:
    - Fixed `internal/tuner/guide_health.go` so the dedicated guide diagnostics endpoints no longer fall back to plain-text `405` responses on unsupported methods.
    - Covered `guide/health.json`, `guide/doctor.json`, and `guide/aliases.json`, which now return JSON `405` with `Allow: GET`.
    - Extended regression coverage in `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(guideDiagnosticsRequireXMLTV|guideDiagnosticsFailuresStayJSON|operatorJSONMethodRejectionsStayJSON|publicReadOnlyEndpointsRequireGet)'`
    - `./scripts/verify`
  Notes:
    - This was a small leftover after the wider JSON 405 cleanup: the guide diagnostics already stayed JSON on ordinary errors, but method rejection still degraded to plain text.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/guide_health.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Keep remaining operator JSON 405 paths machine-readable
  Summary:
    - Fixed `internal/tuner/server.go` so the remaining operator-facing JSON endpoints no longer fall back to plain-text `405` responses.
    - Covered representative programming, virtual-channel, recording, report/debug, ghost-hunter, and operator-action JSON surfaces by routing unsupported-method handling through the JSON `405` helper.
    - Added regression coverage in `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(operatorJSONMethodRejectionsStayJSON|operatorReadOnlyEndpointsRequireGet|publicReadOnlyEndpointsRequireGet|operatorGuidePreviewJSON(MethodRejectionStaysJSON|ErrorsStayJSON|$))'`
    - `./scripts/verify`
  Notes:
    - This was a leftover from the broader JSON-contract cleanup: the endpoints already enforced methods correctly, but their `405` path still silently degraded to `text/plain`.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Preserve HEAD on deck auth redirects
  Summary:
    - Fixed `internal/webui/webui.go` so unauthenticated browser redirects to `/login` no longer always use `303 See Other`.
    - `GET` and `HEAD` requests now redirect with `307 Temporary Redirect`, preserving the method, while non-safe methods still use `303`.
    - Added regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(SessionAuthOnly(RedirectsBrowserRequests|RejectsAPIsWithoutSession|Allows(BasicAuthFallback|ScriptableBasicAuthWithoutSession))|APIRootRedirectRequiresGetOrHead|LoginAllowsHeadAndRejectsOtherMethods)'`
    - `./scripts/verify`
  Notes:
    - This was the auth-side version of the earlier slash-canonical redirect bug: once `HEAD` was supported on the target login page, the redirect also needed to preserve that method.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Preserve HEAD across slash-canonical redirects
  Summary:
    - Fixed `internal/webui/webui.go` and `internal/tuner/server.go` so bare `/api`, `/ui`, and `/ui/guide` no longer use `303 See Other` for read-side slash canonicalization.
    - Those redirect entrypoints now use `307 Temporary Redirect`, which preserves `HEAD` instead of rewriting it into `GET`.
    - Added regression coverage in `internal/webui/webui_test.go` and `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(APIRootRedirectRequiresGetOrHead|LoginAllowsHeadAndRejectsOtherMethods|IndexAndAssetsRequireGetOrHead)'`
    - `go test ./internal/tuner -run 'TestServer_(operatorRedirectsPreserveReadMethods|operatorHTMLPagesAllowHead|operatorGuidePreviewJSON(MethodRejectionStaysJSON|ErrorsStayJSON|$))'`
    - `./scripts/verify`
  Notes:
    - This was a subtle protocol bug rather than another missing method gate: once `HEAD` was allowed on the target surfaces, the redirect wrappers also had to preserve that method.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/tuner/server.go`
    - `internal/webui/webui_test.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Allow HEAD on the deck login page
  Summary:
    - Fixed `internal/webui/webui.go` so the HTML login page now accepts `HEAD` in addition to `GET` and `POST`.
    - Updated the method-rejection contract to advertise `Allow: GET, HEAD, POST`.
    - Added regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(IndexAndAssetsRequireGetOrHead|LoginAllowsHeadAndRejectsOtherMethods|IndexAndLoginLazilyInitializeTemplates|APIRootRedirectRequiresGetOrHead)'`
    - `./scripts/verify`
  Notes:
    - This was a small browser-surface protocol mismatch: the login page is read-only on GET, but unlike the deck root and operator HTML pages it still rejected `HEAD`.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Allow HEAD on operator HTML pages
  Summary:
    - Fixed `internal/tuner/operator_ui.go` so the read-only operator HTML pages `/ui/` and `/ui/guide/` now accept `HEAD` as well as `GET`.
    - Updated the method-rejection contract for those pages to advertise `Allow: GET, HEAD`.
    - Added regression coverage in `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(operatorHTMLPagesAllowHead|operatorGuidePreviewJSON(MethodRejectionStaysJSON|ErrorsStayJSON|$))'`
    - `./scripts/verify`
  Notes:
    - This was a small protocol-semantics mismatch rather than a data/serialization bug: adjacent read-only browser/export surfaces already handled `HEAD`, but the operator HTML pages still rejected it.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/operator_ui.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Stop rewriting mutation verbs through the deck API root redirect
  Summary:
    - Fixed `internal/webui/webui.go` so bare `/api` no longer redirects every method with `303 See Other`.
    - The API root now redirects only `GET, HEAD` to `/api/` and rejects mutation verbs with JSON `405` plus `Allow: GET, HEAD`.
    - Added regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(APIRootRedirectRequiresGetOrHead|LocalhostOnlyJSONEndpointsStayJSON|IndexAndAssetsRequireGetOrHead|Proxy(ForwardsAPIPath|InvalidBaseStaysJSON))'`
    - `./scripts/verify`
  Notes:
    - This was a real semantics bug in the deck API wrapper, not just a content-type mismatch: `POST /api` could be rewritten into a `GET` through the redirector.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Keep localhost-only deck API redirect machine-readable
  Summary:
    - Fixed `internal/webui/webui.go` so the localhost-only gate now treats bare `/api` as a machine-facing path just like `/api/*`.
    - Remote-denied requests to `/api` now return JSON `403` instead of plain-text `403`.
    - Extended regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(LocalhostOnlyJSONEndpointsStayJSON|IndexAndAssetsRequireGetOrHead|Proxy(ForwardsAPIPath|InvalidBaseStaysJSON))'`
    - `./scripts/verify`
  Notes:
    - This was a wrapper-level mismatch, not a leaf-handler bug: `/api` is the scriptable deck redirect entrypoint, but the localhost-only gate had drifted from the `/api/*` behavior.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Enforce GET and HEAD on deck browser assets
  Summary:
    - Fixed `internal/webui/webui.go` so the deck root page and static asset handlers no longer answer mutation verbs like ordinary reads.
    - Covered `index`, `assetCSS`, and `assetJS`, which now enforce `GET, HEAD` with `Allow: GET, HEAD`.
    - Added regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(IndexAnd(LoginLazilyInitializeTemplates|AssetsRequireGetOrHead)|TelemetryGETAndDeleteOnly|ActivityGETAndDeleteOnly|Proxy(ForwardsAPIPath|InvalidBaseStaysJSON))'`
    - `./scripts/verify`
  Notes:
    - This was a browser-surface analogue of the earlier read-only API/export bugs: the page and asset handlers had simply never declared a method contract.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Enforce GET-only behavior on Xtream player_api
  Summary:
    - Fixed `internal/tuner/server_xtream.go` so the read-only Xtream `player_api.php` surface no longer accepts mutation verbs.
    - The endpoint now rejects non-`GET` requests with a JSON `405` and `Allow: GET`, preserving the machine-facing contract.
    - Added regression coverage in `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_XtreamPlayerAPI_(LiveCategories|VODAndSeries|ShortEPG|FailuresStayJSON|MethodRejectionStaysJSON)'`
    - `./scripts/verify`
  Notes:
    - This was another standalone wrapper leftover after the broader Xtream export/proxy cleanup: `player_api.php` was still behaving like an unrestricted generic handler instead of a read-only API surface.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server_xtream.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Keep operator guide-preview 405 responses JSON
  Summary:
    - Fixed `internal/tuner/operator_ui.go` so `ui/guide-preview.json` no longer falls back to plain-text `405` responses on method rejection.
    - Routed that path through the shared JSON method-rejection helper to preserve `application/json` and `Allow: GET`.
    - Added regression coverage in `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_operatorGuidePreviewJSON(MethodRejectionStaysJSON|ErrorsStayJSON|$)'`
    - `./scripts/verify`
  Notes:
    - This was a genuine one-off wrapper bug after the broader JSON-contract sweep: the endpoint’s success and ordinary failure paths were JSON, but its `405` path still used `http.Error`.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/operator_ui.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Enforce read-only methods on operator preview/detail helpers and device XML
  Summary:
    - Fixed `internal/tuner/server.go` so several operator-facing read-only helpers no longer serve `POST` like `GET`: programming browse/harvest-assist/channel-detail/preview, virtual preview/schedule/detail, recorder report, recording rule preview, and recording history.
    - Fixed `internal/tuner/server.go` so `/device.xml` now enforces `GET, HEAD` instead of accepting arbitrary methods.
    - Added regression coverage in `internal/tuner/server_test.go` and `internal/tuner/ssdp_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(operatorReadOnlyEndpointsRequireGet|programmingBrowse|programmingChannelDetail|virtualChannelRulesAndPreview|RecordingRulesRequireOperatorAccess|publicReadOnlyEndpointsRequireGet)|TestServer_deviceXML(RequiresGetOrHead|$)'`
    - `./scripts/verify`
  Notes:
    - This was another leftover from the read-only sweep: these handlers were structurally pure GET surfaces but had never been given an explicit method contract.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `internal/tuner/ssdp_test.go`

- Date: 2026-03-22
  Title: Enforce read-only methods on public status and guide reports
  Summary:
    - Fixed `internal/tuner/server.go` and `internal/tuner/guide_health.go` so public status and guide-report surfaces no longer serve mutation verbs like ordinary reads.
    - Covered `healthz`, `readyz`, `guide/epg-store.json`, `guide/health.json`, `guide/doctor.json`, `guide/aliases.json`, `guide/highlights.json`, `guide/capsules.json`, and `guide/policy.json`.
    - Added regression coverage in `internal/tuner/server_test.go` to ensure these surfaces emit `405` with the correct `Allow` headers.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(healthz|readyz|publicReadOnlyEndpointsRequireGet|guideDiagnosticsRequireXMLTV|guideDiagnosticsFailuresStayJSON)'`
    - `./scripts/verify`
  Notes:
    - This was a sparse leftover after the broader read-only cleanup: public status and guide-report handlers were still missing the same method gating already applied to sibling report and export surfaces.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/guide_health.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Align channel DNA access with sibling report surfaces
  Summary:
    - Fixed `internal/tuner/server.go` so `/channels/dna.json` now requires operator access like the sibling channel report endpoints instead of remaining remotely readable.
    - Updated the existing happy-path test to use localhost explicitly and added a remote-denied regression in `internal/tuner/server_test.go`.
    - This closes a single-surface access-control drift rather than another broad class.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(channelDNAReport(RequiresOperatorAccess)?|channelIntelligenceRequiresOperatorAccess|methodNotAllowedSetsAllowHeadersAcrossTunerSurfaces)'`
    - `./scripts/verify`
  Notes:
    - `/channels/dna.json` had drifted away from its sibling intelligence endpoints and was the only one still skipping the shared operator-access gate.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Enforce read-only methods on virtual export surfaces
  Summary:
    - Fixed `internal/tuner/server.go` so `virtual-channels/guide.xml` and `virtual-channels/live.m3u` no longer accept mutation verbs.
    - Both export surfaces now enforce `GET, HEAD` with `Allow: GET, HEAD`, matching the rest of the read-only export plane.
    - Added regression coverage in `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(virtualChannelRulesAndPreview|virtualChannelExportsRequireGetOrHead|methodNotAllowedSetsAllowHeadersAcrossTunerSurfaces)'`
    - `./scripts/verify`
  Notes:
    - This was an isolated leftover after the broader read-only cleanup: adjacent stream and JSON surfaces were already gated, but the XML/M3U virtual exports were still wide open on method handling.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Enforce GET-only behavior on remaining report surfaces
  Summary:
    - Fixed a remaining cluster of read-only tuner report/debug endpoints in `internal/tuner/server.go` so they no longer serve `POST` like `GET`.
    - Covered `channels/report.json`, `channels/leaderboard.json`, `channels/dna.json`, `autopilot/report.json`, `provider/profile.json`, `debug/stream-attempts.json`, `debug/shared-relays.json`, `debug/runtime.json`, `debug/event-hooks.json`, `debug/active-streams.json`, and `guide/lineup-match.json`.
    - Added regression coverage in `internal/tuner/server_test.go` to ensure those surfaces emit `405` with `Allow: GET`.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(methodNotAllowedSetsAllowHeadersAcrossTunerSurfaces|reportJSONFailuresStayJSON|operatorJSONEndpointsStayJSONWhenOperatorAccessDenied|SharedRelayReport)'`
    - `./scripts/verify`
  Notes:
    - This was the last broad repeated HTTP-contract bug from the sweep: read-only report endpoints without any method gate.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Remove deck auth nil-map panic paths
  Summary:
    - Fixed `internal/webui/webui.go` so login/session helpers lazily initialize `sessions` and `failedLoginByIP` instead of assuming `Run()` has already created those maps.
    - This removes panic paths on both failed and successful login attempts for zero-value or partially constructed `webui.Server` instances.
    - Added regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(Login(LazilyInitializesStateMaps|AndLogoutFlow|IgnoresRedirectTargets)|IndexAndLoginLazilyInitializeTemplates)'`
    - `./scripts/verify`
  Notes:
    - This was another optional-subsystem initialization bug in the deck plane: auth state, not templates, still depended on the normal `Run()` path to avoid a panic.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Remove deck template nil-panic paths
  Summary:
    - Fixed `internal/webui/webui.go` so the main deck page and login renderer lazily initialize embedded templates instead of assuming `Run()` already populated `s.tmpl` and `s.loginTmpl`.
    - This removes a panic path for zero-value or partially constructed `webui.Server` instances while preserving the existing runtime behavior.
    - Added regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(IndexAndLoginLazilyInitializeTemplates|Proxy(ForwardsAPIPath|InvalidBaseStaysJSON)|LoginAndLogoutFlow)'`
    - `./scripts/verify`
  Notes:
    - This was a nil-safety bug, not an HTTP contract bug: the handlers crashed before they could even form a response if template initialization had not happened through the normal `Run()` path.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Keep remote-denied operator JSON surfaces machine-readable
  Summary:
    - Fixed `internal/tuner/operator_ui.go` so the shared operator-access gate now returns JSON `403` errors for machine-facing `*.json` and `/ops/` requests instead of flattening them to plain text.
    - Left HTML/page rejections unchanged, so only structured operator surfaces changed behavior.
    - Added representative regression coverage in `internal/tuner/server_test.go` across programming preview, guide-preview JSON, action-status, and action POST endpoints.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(Programming(ReadEndpointsRequireOperatorAccess|RecipeRequiresOperatorAccess)|operatorJSONEndpointsStayJSONWhenOperatorAccessDenied|operatorGuidePreviewJSON|operatorActionStatus|guideRefreshAction)'`
    - `./scripts/verify`
  Notes:
    - This was the operator-plane version of the earlier deck localhost-only bug: leaf handlers had been fixed, but the top-level access-control gate could still silently degrade JSON/action clients to `text/plain`.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/operator_ui.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Keep WebDAV OPTIONS honest on the read-only surface
  Summary:
    - Fixed `internal/vodwebdav/webdav.go` so the read-only WebDAV wrapper now handles `OPTIONS` itself instead of delegating to `x/net/webdav`, which was advertising writable methods that the wrapper immediately rejects.
    - The WebDAV surface now consistently advertises only `OPTIONS, PROPFIND, HEAD, GET` plus `DAV: 1, 2`, matching the actual read-only contract.
    - Tightened regression coverage in `internal/vodwebdav/webdav_test.go` so `OPTIONS` fails if writable verbs reappear in `Allow`.
  Verification:
    - `go test ./internal/vodwebdav -run TestHandler_OPTIONSAndPROPFIND_ClientShapes -v`
    - `./scripts/verify`
  Notes:
    - This was a concrete protocol contradiction, not a cosmetic header tweak: compliant WebDAV clients were being told the server supported writes that the outer wrapper never allows.
  Opportunities filed:
    - none
  Links:
    - `internal/vodwebdav/webdav.go`
    - `internal/vodwebdav/webdav_test.go`

- Date: 2026-03-22
  Title: Enforce read-only methods on Xtream proxy surfaces
  Summary:
    - Fixed `internal/tuner/server_xtream.go` so the Xtream live proxy no longer forwards arbitrary methods upstream and now enforces the same `GET, HEAD` contract as the movie/series proxies.
    - Switched the movie/series proxy `405` path to the shared helper so all Xtream proxy surfaces emit the same `Allow: GET, HEAD` behavior.
    - Added regression coverage in `internal/tuner/server_test.go` for live, movie, and series proxy method rejection.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_Xtream(MovieAndSeriesProxy|ProxySurfacesRequireGetOrHead|LiveProxyAndVirtualChannels|Exports_(M3UAndXMLTV|RequireGetOrHead))'`
    - `./scripts/verify`
  Notes:
    - This was the next layer of the same read-only contract bug after the Xtream export fix: proxies were still behaving like generic upstream tunnels instead of read-only playback endpoints.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server_xtream.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Enforce read-only methods on Xtream exports
  Summary:
    - Fixed `internal/tuner/server_xtream.go` so the Xtream `m3u_plus` and `xmltv.php` export handlers now reject non-`GET`/`HEAD` requests instead of serving read-only exports to arbitrary methods.
    - Routed those rejections through the shared `405` helper so they emit `Allow: GET, HEAD` consistently.
    - Added regression coverage in `internal/tuner/server_test.go` for both export surfaces.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_XtreamExports_(M3UAndXMLTV|RequireGetOrHead|NormalizeBaseURLWhitespace)|TestServer_XtreamXMLTVUsesUniqueChannelIDsWhenTVGIDCollides'`
    - `./scripts/verify`
  Notes:
    - This was a read-only surface contract bug, not a JSON-contract issue: authenticated `POST` requests were incorrectly treated like ordinary export reads.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server_xtream.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Fix HDHomeRun control HTTP 405 protocol hints
  Summary:
    - Fixed `internal/hdhomerun/control.go` so the simulated HDHomeRun HTTP shim now includes `Allow: GET, HEAD` on `405 Method Not Allowed` responses for known control endpoints.
    - Updated the raw control-socket response builder instead of only patching one helper, so direct TCP/HTTP clients and tests now see the correct protocol hint.
    - Added regression coverage in `internal/hdhomerun/control_test.go` for both helper-level method handling and an end-to-end socket `PUT /discover.json` request.
  Verification:
    - `go test ./internal/hdhomerun -run 'TestControlServer_(httpResponseFor(Path|RequestMethodHandling)|handleConnectionRecognizesPutAsHTTP)'`
    - `./scripts/verify`
  Notes:
    - This was the same `405`/`Allow` standards bug as earlier HTTP-surface fixes, just on the lower-level HDHomeRun control shim rather than the main tuner handlers.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/control.go`
    - `internal/hdhomerun/control_test.go`

- Date: 2026-03-22
  Title: Keep localhost-only deck JSON endpoints machine-readable
  Summary:
    - Fixed `internal/webui/webui.go` so the top-level localhost-only wrapper returns real JSON `403` errors for blocked `/api/*` and `*.json` deck paths instead of degrading those machine-facing surfaces to plain text.
    - Left browser-page failures as plain text, so the change only affects the structured/API side of the deck plane.
    - Added focused regression coverage in `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui -run 'Test(LocalhostOnlyJSONEndpointsStayJSON|Proxy(ForwardsAPIPath|InvalidBaseStaysJSON)|SessionAuthOnlyAllowsScriptableBasicAuthWithoutSession)'`
    - `./scripts/verify`
  Notes:
    - This was a wrapper-level version of the same contract drift bug: even after leaf handlers were fixed, the top-level access-control gate could still flatten JSON endpoints back to `text/plain`.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/webui_test.go`

- Date: 2026-03-22
  Title: Finish the last literal JSON-through-plain-text failures
  Summary:
    - Fixed `internal/webui/webui.go` so the deck reverse-proxy surface returns a real JSON error with `application/json` when `tunerBase` is invalid instead of sending a JSON-looking string through `http.Error`.
    - Fixed `internal/tuner/server.go` so the virtual-channel stream “slot has no source” failure path now returns a real JSON error instead of the same JSON-through-plain-text bug.
    - Added focused regression coverage in `internal/webui/webui_test.go` and `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/webui -run 'TestProxy(ForwardsAPIPath|InvalidBaseStaysJSON)'`
    - `go test ./internal/tuner -run 'TestServer_(virtualChannelRulesAndPreview|virtualChannelStreamMissingSourceStaysJSON|lowerJSONFailuresStayJSON)'`
    - `./scripts/verify`
  Notes:
    - This closes the remaining repo-wide instances where a handler was literally writing a JSON body through `http.Error`, which silently mislabeled the response as plain text.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/tuner/server.go`
    - `internal/webui/webui_test.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Keep programming, virtual, and recording failures JSON
  Summary:
    - Fixed lower-half `internal/tuner/server.go` JSON handlers so programming, virtual-channel, recorder, recording, and mux-decode validation/error paths no longer degrade to plain text on failure.
    - Routed missing-config, invalid-json, not-found, bad-gateway, and encode-failure cases through the shared server JSON error writer.
    - Added representative regression coverage in `internal/tuner/server_test.go` for programming, virtual-channel, recorder, recording-preview/history, and mux-decode failure cases.
  Verification:
    - `go test ./internal/tuner -run 'Test(Server_(reportJSONFailuresStayJSON|lowerJSONFailuresStayJSON|programmingEndpoints|programmingHarvestEndpoint|virtualChannelRulesAndPreview|RecordingRulesRequireOperatorAccess|methodNotAllowedSetsAllowHeadersAcrossTunerSurfaces))'`
    - `./scripts/verify`
  Notes:
    - This extends the same machine-facing contract fix from report/debug and guide/deck/Xtream surfaces into the lower `server.go` JSON lanes.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Keep server report/debug failure paths JSON
  Summary:
    - Fixed `internal/tuner/server.go` so multiple report/debug endpoints that already return JSON on success now also return real JSON errors on unavailable, bad-gateway, and encode-failure paths.
    - Covered `epg-store`, guide highlights/capsules/policy, ghost/provider/stream/shared-relay reports, runtime/event-hooks/active-streams, and guide-lineup-match.
    - Added regression coverage in `internal/tuner/server_test.go` for representative unavailable/error cases and JSON content-type checks.
  Verification:
    - `go test ./internal/tuner -run 'Test(Server_(epgStoreReport_disabled|reportJSONFailuresStayJSON|runtimeSnapshot|guideDiagnosticsRequireXMLTV|guideDiagnosticsFailuresStayJSON|ghostReportStopRequiresOperatorPost))'`
    - `./scripts/verify`
  Notes:
    - This was the same machine-facing contract bug class as the earlier guide/deck/operator/Xtream passes, just across a bigger `server.go` report/debug batch.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Keep Xtream player_api failures machine-readable
  Summary:
    - Fixed `internal/tuner/server_xtream.go` so Xtream `player_api.php` auth failures, unsupported actions, and missing series/stream replies now return real JSON bodies with `application/json`.
    - Added regression coverage in `internal/tuner/server_test.go` for unauthorized, unsupported-action, missing-series, and missing-stream player_api paths.
    - Re-ran full verification after one transient smoke startup failure; the rerun passed cleanly, so the batch is green.
  Verification:
    - `go test ./internal/tuner -run 'Test(Server_XtreamPlayerAPI_(VODAndSeries|ShortEPG|FailuresStayJSON)|Server_XtreamLiveCategoriesAndStreams)'`
    - `./scripts/verify`
  Notes:
    - The first `./scripts/verify` attempt hit a transient `scripts/ci-smoke.sh` Xtream startup failure (`discover.json not ready`); a second full run passed unchanged, which points to smoke flake rather than a persistent code regression.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server_xtream.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Keep operator and deck JSON error paths structured
  Summary:
    - Fixed `internal/tuner/operator_ui.go` so the operator guide preview JSON endpoint now returns real JSON error bodies on unavailable and bad-gateway paths.
    - Fixed `internal/webui/webui.go` so deck settings, auth, CSRF, and report helper errors stay `application/json` instead of degrading to plain text through `http.Error`.
    - Added regression coverage in `internal/tuner/server_test.go` and `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/webui ./internal/tuner -run 'Test(DeckSettings(GETAndPOST|InvalidJSONStaysJSON)|SessionAuthOnlyRejectsMutationsWithoutCSRF|Server_operatorGuidePreviewJSON(ErrorsStayJSON)?)'`
    - `./scripts/verify`
  Notes:
    - This was the same machine-facing contract bug class as the previous guide diagnostics pass, just on adjacent operator/deck surfaces.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/operator_ui.go`
    - `internal/webui/webui.go`

- Date: 2026-03-22
  Title: Keep guide and deck error responses machine-readable
  Summary:
    - Fixed `internal/tuner/guide_health.go` so `/guide/health.json`, `/guide/doctor.json`, and `/guide/aliases.json` now return real JSON error bodies with `application/json` on unavailable and bad-gateway paths.
    - Fixed `internal/webui/webui.go` so JSON `405` responses from deck endpoints no longer degrade to `text/plain` through `http.Error`.
    - Added regression coverage in `internal/tuner/server_test.go` and `internal/webui/webui_test.go`.
  Verification:
    - `go test ./internal/tuner ./internal/webui -run 'Test(Server_guideDiagnosticsRequireXMLTV|Server_guideDiagnosticsFailuresStayJSON|TelemetryGETAndDeleteOnly|ActivityGETAndDeleteOnly|DeckSettingsGETAndPOST)'`
    - `./scripts/verify`
  Notes:
    - This came directly from the broader audit for machine-facing contract drift: endpoints that looked JSON on success but stopped being JSON on failure.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/guide_health.go`
    - `internal/webui/webui.go`

- Date: 2026-03-22
  Title: Finish tuner HTTP 405 consistency sweep
  Summary:
    - Fixed the remaining `internal/tuner/server.go` operator/programming/recording/demo handlers that still hand-rolled `405 method not allowed` responses without `Allow`.
    - Centralized plain-text and JSON `405` helpers so JSON surfaces keep `application/json` instead of silently degrading to `text/plain` through `http.Error`.
    - Added regression coverage in `internal/tuner/server_test.go` for representative GET-only, POST-only, GET/POST, and GET/HEAD tuner surfaces.
  Verification:
    - `go test ./internal/tuner -run 'Test(Server_methodNotAllowedSetsAllowHeadersAcrossTunerSurfaces|Server_programmingEndpoints|Server_ghostReportStopRequiresOperatorPost|Server_RecordingRulesRequireOperatorAccess)'`
    - `./scripts/verify`
  Notes:
    - Focused tests exposed a second bug inside the fix itself: JSON `405` responses were still advertising the wrong content type until the helper stopped using `http.Error`.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Fix 405 protocol hints and remove deck password panic path
  Summary:
    - Fixed `internal/webui/webui.go` so deck telemetry/activity/settings/login/logout method rejections now include `Allow`, and deck password generation no longer panics the process if `crypto/rand` fails.
    - Fixed covered tuner/operator UI 405 paths in `internal/tuner/operator_ui.go` and `internal/tuner/server.go` to emit `Allow` headers consistently.
    - Added regression coverage in `internal/webui/webui_test.go` and `internal/tuner/server_test.go`.
  Verification:
    - `go test ./internal/webui ./internal/tuner -run 'Test(TelemetryGETAndDeleteOnly|ActivityGETAndDeleteOnly|DeckSettingsGETAndPOST|Server_ghostReportStopRequiresOperatorPost)'`
  Notes:
    - This was the first non-normalization bug batch from the broader audit: protocol correctness and fail-closed behavior instead of URL joining.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/tuner/operator_ui.go`
    - `internal/tuner/server.go`

- Date: 2026-03-22
  Title: Normalize deeper Xtream indexer helper bases
  Summary:
    - Fixed `internal/indexer/player_api.go` so the internal VOD/series/category/info helpers normalize `apiBase` and `streamBase` before building `player_api.php`, artwork, and VOD/series stream URLs.
    - Fixed the remaining catalog helper `streamURLsFromRankedBases` in `cmd/iptv-tunerr/cmd_catalog.go` to reuse the same normalized provider-base join as the rest of the catalog path.
    - Added focused regression coverage in `internal/indexer/player_api_test.go` and `cmd/iptv-tunerr/cmd_catalog_test.go`.
  Verification:
    - `go test ./internal/indexer ./cmd/iptv-tunerr ./internal/emby ./internal/hdhomerun ./internal/tuner -run 'Test(Fetch(VODStreams|Series)NormalizesRelativeArtworkBase|FetchLiveStreamsNormalizesStreamBase|NormalizeAPIBase|StreamURLsFromRankedBases_NormalizesBase|NormalizeCatalogProviderBase|CatalogProviderIdentityKey_NormalizesBase|PrioritizeWinningProvider_NormalizesEquivalentBases|StreamVariantsFromRankedEntries_NormalizesBase|EffectiveXMLTVURL(_TrimsWhitespaceAndTrailingSlash)?|ParseDiscoverReply_(roundTrip|TrimsWhitespaceAndTrailingSlashes)|M3UServe_(urlTvg|NormalizesWhitespaceAndTrailingSlashes|epgPruneUnlinked|epgPruneUnlinked_false|404))'`
  Notes:
    - This batch came from the audit list after focused tests exposed that normalization still broke inside lower-level Xtream helper calls, not just at public entrypoints.
  Opportunities filed:
    - none
  Links:
    - `internal/indexer/player_api.go`
    - `cmd/iptv-tunerr/cmd_catalog.go`

- Date: 2026-03-22
  Title: Normalize catalog identity and adjacent export/parser bases
  Summary:
    - Fixed `cmd/iptv-tunerr/cmd_catalog.go` so ranked stream URL rebuilding and provider identity keying trim whitespace and all trailing slashes, preventing equivalent providers from splitting across winner ordering and lockout tracking.
    - Fixed `internal/emby/register.go`, `internal/hdhomerun/client.go`, and `internal/tuner/m3u.go` so Emby XMLTV fallback URLs, parsed HDHomeRun discover replies, and live M3U export URLs normalize whitespace-padded or multi-slash bases.
    - Added focused regression coverage in `cmd/iptv-tunerr/cmd_catalog_test.go`, `internal/emby/register_test.go`, `internal/hdhomerun/client_test.go`, and `internal/tuner/m3u_test.go`.
  Verification:
    - `go test ./cmd/iptv-tunerr ./internal/emby ./internal/hdhomerun ./internal/tuner -run 'Test(StreamURLsFromRankedBases_NormalizesBase|NormalizeCatalogProviderBase|CatalogProviderIdentityKey_NormalizesBase|PrioritizeWinningProvider_NormalizesEquivalentBases|StreamVariantsFromRankedEntries_NormalizesBase|EffectiveXMLTVURL(_TrimsWhitespaceAndTrailingSlash)?|ParseDiscoverReply_(roundTrip|TrimsWhitespaceAndTrailingSlashes)|M3UServe_(urlTvg|NormalizesWhitespaceAndTrailingSlashes|epgPruneUnlinked|epgPruneUnlinked_false|404))'`
  Notes:
    - This batch came directly out of the broader audit list and closes a mixed cluster of provider identity drift plus adjacent consumer-facing URL rebuilders/parsers.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_catalog.go`
    - `internal/emby/register.go`
    - `internal/hdhomerun/client.go`
    - `internal/tuner/m3u.go`

- Date: 2026-03-22
  Title: Normalize config, health, and provider collector base URLs
  Summary:
    - Fixed `internal/config/config.go` so `M3UURLsOrBuild` trims whitespace and all trailing slashes before building provider `get.php` URLs.
    - Fixed `internal/health/health.go` so endpoint health checks normalize the base before probing `/discover.json`, `/lineup.json`, and `/guide.xml`.
    - Fixed `internal/provider/probe.go` so `RankedEntries` reuses the hardened provider-base normalizer before probing `player_api.php`.
    - Added focused regression coverage in `internal/config/config_test.go`, `internal/health/health_test.go`, and `internal/provider/probe_test.go`.
  Verification:
    - `go test ./internal/config ./internal/health ./internal/provider -run 'Test(M3UURLsOrBuild_(single|multiple|UsesPerProviderCredentials|NormalizesWhitespaceAndTrailingSlashes)|CheckEndpoints_(ok|missing|normalizesWhitespaceAndTrailingSlashes)|RankedEntries_(multipleProvidersRankedBestFirst|NormalizesWhitespaceAndTrailingSlashes|blockCF_separateCreds|allCF_blockReturnsEmpty)|ProbePlayerAPI_TrimsWhitespaceAndTrailingSlash|NormalizeProviderBaseURL)'`
  Notes:
    - This closes another collector-layer sibling where already-normalized helpers were still being bypassed by raw base joins.
  Opportunities filed:
    - none
  Links:
    - `internal/config/config.go`
    - `internal/health/health.go`
    - `internal/provider/probe.go`

- Date: 2026-03-22
  Title: Normalize shared guide and lineup helper base URLs
  Summary:
    - Fixed `internal/guideinput/guideinput.go` and `internal/tuner/epg_pipeline.go` so provider `xmltv.php` URL builders trim whitespace and all trailing slashes instead of only removing a single slash.
    - Fixed `internal/probe/probe.go` so the generic HDHomeRun lineup helper normalizes its base before building `/stream?url=` links.
    - Added focused regression coverage in `internal/guideinput/guideinput_test.go`, `internal/tuner/epg_pipeline_test.go`, and `internal/probe/probe_test.go`.
  Verification:
    - `go test ./internal/guideinput ./internal/tuner ./internal/probe -run 'Test(ProviderXMLTVURL(WithSuffix(_NormalizesWhitespaceAndTrailingSlashes)?)?|ProviderXMLTVEPGURL_(suffix|normalizesWhitespaceAndTrailingSlashes)|Lineup_(withBaseURL(_NormalizesWhitespaceAndTrailingSlashes)?|noBaseURL)|LineupHandler|DiscoveryHandler)'`
  Notes:
    - This closes helper-level siblings that could still emit malformed guide or lineup URLs even after higher-level callers were normalized.
  Opportunities filed:
    - none
  Links:
    - `internal/guideinput/guideinput.go`
    - `internal/tuner/epg_pipeline.go`
    - `internal/probe/probe.go`

- Date: 2026-03-22
  Title: Normalize CLI probe command base URLs
  Summary:
    - Fixed `cmd/iptv-tunerr/cmd_core.go` so `handleProbe` trims whitespace before trimming trailing slashes when collecting provider base URLs from config or `-urls`.
    - Prevented the CLI probe path from feeding malformed `//get.php` URLs into the Cloudflare-aware prep step while the deeper provider probe helpers were already normalized.
    - Added focused regression coverage in `cmd/iptv-tunerr/main_test.go` to prove whitespace-padded bases still hit both `player_api.php` and `get.php`.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'TestHandleProbe_(TrimsWhitespaceAndTrailingSlashBaseURL|UsesMultipleCredentialSetsPerHost)|TestNormalizeTopLevelCommand'`
  Notes:
    - This was a command-layer sibling of the provider probe normalization work, not a lower-level transport bug.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_core.go`
    - `cmd/iptv-tunerr/main_test.go`

- Date: 2026-03-22
  Title: Normalize tuner-side Xtream export base URLs
  Summary:
    - Centralized Xtream base URL normalization in `internal/tuner/server_xtream.go` so whitespace and trailing slashes no longer leak into `get.php`/`xmltv.php` output or live/movie/series direct-source URLs.
    - Updated Xtream M3U, live direct source, movie streams, and series episode exports to reuse the normalized base helper instead of hand-rolling from raw `Server.BaseURL`.
    - Added focused regression coverage in `internal/tuner/server_test.go` for guide, live, movie, and series URLs built from a whitespace-padded base.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_XtreamExports_(M3UAndXMLTV|NormalizeBaseURLWhitespace)|TestServer_XtreamMovieAndSeriesProxy'`
  Notes:
    - This closes another sibling Xtream publishing surface after the wider provider/runtime normalization sweep.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server_xtream.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Normalize get.php catalog fallback base URLs
  Summary:
    - Fixed `cmd/iptv-tunerr/cmd_catalog.go` so the `get.php` catalog fallback trims whitespace as well as trailing slashes before building the fallback M3U URL.
    - Prevented malformed `get.php` fallback URLs when provider base values carry surrounding whitespace.
    - Added focused regression coverage in `cmd/iptv-tunerr/cmd_runtime_test.go`.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(RuntimeHealthCheckURL_(PrefersWinningProviderCredentials|FallsBackToRunProviderBaseWhenAPIBaseEmpty|TrimsWhitespaceAndTrailingSlash)|CatalogFromGetPHP_TrimsWhitespaceAndTrailingSlash|ApplyRegistrationRecipe_HealthyFiltersWeakChannels|ApplyRegistrationRecipe_ResilientSortsBackupFirst|ApplyRegistrationRecipe_AppliesDNAPolicy|ApplyRegistrationRecipe_SportsNowUsesIntentRecipe|GuideURLForBaseTrimsTrailingSlash|StreamURLForBaseTrimsTrailingSlash)'`
  Notes:
    - This was the `get.php` sibling of the earlier runtime/provider/player_api base normalization fixes.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_catalog.go`
    - `cmd/iptv-tunerr/cmd_runtime_test.go`

- Date: 2026-03-22
  Title: Normalize provider probe base URLs
  Summary:
    - Fixed `internal/provider/probe.go` to normalize provider bases before building alternate `get.php` probes, POST probes, and `player_api.php` probes.
    - Prevented malformed upstream probe URLs when provider base values carry whitespace or trailing slashes.
    - Added focused regression coverage in `internal/provider/probe_test.go`.
  Verification:
    - `go test ./internal/provider ./internal/indexer ./cmd/iptv-tunerr -run 'Test(ProbePlayerAPI_(ok|serverInfoOnly|cloudflareServer200JSONStillOK|forbiddenThenAcceptedWithFallbackUA|badStatus|forbiddenClassifiedAsAccessDenied|TrimsWhitespaceAndTrailingSlash)|NormalizeProviderBaseURL|RuntimeHealthCheckURL_(PrefersWinningProviderCredentials|FallsBackToRunProviderBaseWhenAPIBaseEmpty|TrimsWhitespaceAndTrailingSlash)|NormalizeAPIBase|FetchLiveStreamsNormalizesStreamBase)'`
  Notes:
    - This was the provider-probe sibling of the earlier indexer/runtime `player_api` normalization fix.
  Opportunities filed:
    - none
  Links:
    - `internal/provider/probe.go`
    - `internal/provider/probe_test.go`

- Date: 2026-03-22
  Title: Normalize Xtream player_api and live stream base URLs
  Summary:
    - Fixed `cmd/iptv-tunerr/cmd_runtime.go` so runtime health-check URLs trim whitespace and trailing slashes from provider/API bases before appending `player_api.php`.
    - Fixed `internal/indexer/player_api.go` so player_api requests and generated `/live/...` stream URLs normalize their API and stream bases consistently.
    - Added focused regression coverage in `cmd/iptv-tunerr/cmd_runtime_test.go` and `internal/indexer/player_api_test.go`.
  Verification:
    - `go test ./internal/indexer ./cmd/iptv-tunerr -run 'Test(RuntimeHealthCheckURL_(PrefersWinningProviderCredentials|FallsBackToRunProviderBaseWhenAPIBaseEmpty|TrimsWhitespaceAndTrailingSlash)|ParseSeriesEpisodesSupportsSeasonKeyedArrays|ParseSeriesEpisodesSupportsFlatArray|IsPlayerAPIErrorStatus|NormalizeAPIBase|FetchLiveStreamsNormalizesStreamBase)'`
  Notes:
    - This was the upstream-provider sibling of the earlier trailing-slash fixes on local discovery, registration, and export surfaces.
  Opportunities filed:
    - none
  Links:
    - `internal/indexer/player_api.go`
    - `internal/indexer/player_api_test.go`
    - `cmd/iptv-tunerr/cmd_runtime.go`
    - `cmd/iptv-tunerr/cmd_runtime_test.go`

- Date: 2026-03-22
  Title: Normalize Plex runtime-registration stream URLs
  Summary:
    - Fixed `cmd/iptv-tunerr/cmd_runtime_register.go` so lineup rows sent to Plex derive stream URLs from a trimmed helper instead of raw `baseURL + "/stream/" + channelID` concatenation.
    - Prevented malformed `//stream/...` URLs from being pushed into Plex when the configured tuner base ends with `/`.
    - Added focused regression coverage in `cmd/iptv-tunerr/cmd_runtime_register_test.go`.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(ApplyRegistrationRecipe_HealthyFiltersWeakChannels|ApplyRegistrationRecipe_ResilientSortsBackupFirst|ApplyRegistrationRecipe_AppliesDNAPolicy|ApplyRegistrationRecipe_SportsNowUsesIntentRecipe|GuideURLForBaseTrimsTrailingSlash|StreamURLForBaseTrimsTrailingSlash)'`
  Notes:
    - This was the stream-URL sibling of the earlier guide-URL normalization work in the same runtime registration path.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime_register.go`
    - `cmd/iptv-tunerr/cmd_runtime_register_test.go`

- Date: 2026-03-22
  Title: Normalize Emby/Jellyfin server host URL joins
  Summary:
    - Fixed registration and library helpers under `internal/emby` to join API paths against a trimmed host instead of raw `cfg.Host + "/..."` concatenation.
    - Prevented malformed `//LiveTv/...` and `//Library/...` requests when the configured media-server host ends with `/`.
    - Added focused regression coverage in `internal/emby/register_test.go` and `internal/emby/library_test.go`.
  Verification:
    - `go test ./internal/emby -run 'Test(EffectiveXMLTVURL|RegisterTunerHost_success|RegisterTunerHost_trimsTrailingSlashHost|RegisterTunerHost_serverError|RegisterListingProvider_success|DeleteTunerHost_tolerates404|DeleteTunerHost_emptyID|EnsureLibraryReusesExisting|EnsureLibraryCreatesMissing|CreateLibraryJellyfinUsesQueryParams|EnsureLibraryFallsBackToLegacyVirtualFoldersList|RefreshLibraryScan|ListLibraries_trimsTrailingSlashHost)'`
  Notes:
    - This was the media-server-host sibling of the earlier guide/base URL normalization fixes.
  Opportunities filed:
    - none
  Links:
    - `internal/emby/register.go`
    - `internal/emby/library.go`
    - `internal/emby/register_test.go`
    - `internal/emby/library_test.go`

- Date: 2026-03-22
  Title: Normalize base and lineup URLs across HDHomeRun discovery producers
  Summary:
    - Fixed the standalone HDHomeRun simulator, the simulated control server `discover.json`, and the main tuner HDHR discovery surface to normalize `BaseURL` and derive `LineupURL` from the shared helper instead of raw concatenation.
    - Fixed the main tuner `lineup.json` surface to normalize its base before building `/stream/...` URLs.
    - Prevented discovery payloads and lineup entries from advertising malformed `//lineup.json` and `//stream/...` URLs when configured bases end with `/`.
    - Added focused regression coverage in `internal/hdhomerun/discover_test.go`, `internal/hdhomerun/control_test.go`, and `internal/tuner/hdhr_test.go`.
  Verification:
    - `go test ./internal/hdhomerun ./internal/tuner -run 'Test(CreateDefaultDevice(DefaultFriendlyName|PrefersHDHRFriendlyNameEnv|NormalizesLineupURL)|ControlServer_(getDiscoverJSONEscapesFriendlyName|getDiscoverJSONNormalizesLineupURL|httpResponseForPath|httpResponseForRequestMethodHandling|handleConnectionBinaryRequestUsesSniffedHeader|handleConnectionRecognizesPutAsHTTP)|HDHR_(discover|discover_defaults|discover_normalizesLineupURL|lineup|lineup_status|lineup_status_scan_possible_false|lineup_explicit_channel_id|lineup_multiple_channels|lineup_empty|not_found)|Server_deviceXML(DefaultFriendlyName|UsesEnvFallbacks|EscapesConfiguredIdentity)? )'`
  Notes:
    - This was the producer-side sibling of the earlier lineup client fallback/shape hardening.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/discover.go`
    - `internal/hdhomerun/control.go`
    - `internal/tuner/hdhr.go`

- Date: 2026-03-22
  Title: Normalize Plex core guide URLs
  Summary:
    - Fixed `internal/plex/dvr.go` to derive guide URLs from a trimmed helper instead of raw `BaseURL + "/guide.xml"` concatenation in both DVR API registration and DB registration.
    - Prevented Plex registrations from persisting malformed `//guide.xml` XMLTV references when the configured base ends with `/`.
    - Added focused regression coverage in `internal/plex/dvr_test.go`.
  Verification:
    - `go test ./internal/plex -run 'Test(RegisterTuner_noDB|RegisterTuner_withDB|RegisterTuner_withTrailingSlashBaseURL)'`
  Notes:
    - This was the package-level sibling of the command-layer runtime registration normalization fix.
  Opportunities filed:
    - none
  Links:
    - `internal/plex/dvr.go`
    - `internal/plex/dvr_test.go`

- Date: 2026-03-22
  Title: Normalize Plex runtime-registration guide URLs
  Summary:
    - Fixed `cmd/iptv-tunerr/cmd_runtime_register.go` to derive guide URLs from a trimmed helper instead of raw `baseURL + "/guide.xml"` concatenation.
    - Prevented the Plex setup banner and DVR watchdog from emitting malformed `//guide.xml` URLs when the configured base ends with `/`.
    - Added focused regression coverage in `cmd/iptv-tunerr/cmd_runtime_register_test.go`.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(ApplyRegistrationRecipe_HealthyFiltersWeakChannels|ApplyRegistrationRecipe_ResilientSortsBackupFirst|ApplyRegistrationRecipe_AppliesDNAPolicy|ApplyRegistrationRecipe_SportsNowUsesIntentRecipe|GuideURLForBaseTrimsTrailingSlash)'`
  Notes:
    - This was the Plex-side sibling of the Emby/Jellyfin guide-URL normalization fix.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime_register.go`
    - `cmd/iptv-tunerr/cmd_runtime_register_test.go`

- Date: 2026-03-22
  Title: Normalize Emby/Jellyfin guide URL fallback
  Summary:
    - Fixed `internal/emby.Config.effectiveXMLTVURL()` to trim trailing slashes from `TunerURL` before appending `/guide.xml`.
    - Prevented registration payloads from emitting malformed `//guide.xml` listing-provider URLs when the configured tuner base already ends with `/`.
    - Added focused regression coverage in `internal/emby/register_test.go`.
  Verification:
    - `go test ./internal/emby -run 'Test(EffectiveXMLTVURL|AuthHeader|RegisterTunerHost_success|RegisterTunerHost_serverError|RegisterListingProvider_success|DeleteTunerHost_tolerates404|DeleteTunerHost_emptyID|GetChannelCount_success|GetChannelCount_serverDown|Trunc|FullRegister_roundtrip)'`
  Notes:
    - This was found while widening the shim/client audit beyond HDHomeRun-specific surfaces.
  Opportunities filed:
    - none
  Links:
    - `internal/emby/register.go`
    - `internal/emby/register_test.go`

- Date: 2026-03-22
  Title: Align Plex lineup harvest with hardened HDHomeRun lineup decoding
  Summary:
    - Fixed `internal/plexharvest.fetchLineup` to stop decoding `/lineup.json` as a raw array and instead reuse the shared HDHomeRun client helper that accepts both array-shaped and object-shaped lineup payloads.
    - Preserved Plex harvest reporting by mapping the shared lineup document back into `HarvestedChannel` rows after decode.
    - Added focused regression coverage in `internal/plexharvest/plexharvest_test.go`.
  Verification:
    - `go test ./internal/plexharvest ./internal/hdhomerun -run 'Test(FetchLineupAcceptsObjectShapedPayload|ExpandTargets_templateAndFriendlyNames|BuildSummary_groupsSuccessfulLineups|Probe_pollsAndCapturesLineupTitle|Probe_recordsErrorsPerTarget|SaveLoadReportFile_roundTrip|ParseDiscoverReply_roundTrip|LineupDocUnmarshalJSONArray|FetchLineupJSONAcceptsJSONArray|FetchDiscoverJSONFallsBackToRequestedBaseURL)'`
  Notes:
    - This was the next sibling consumer drift after hardening the shared HDHomeRun client layer.
  Opportunities filed:
    - none
  Links:
    - `internal/plexharvest/plexharvest.go`
    - `internal/plexharvest/plexharvest_test.go`

- Date: 2026-03-22
  Title: Harden HDHomeRun HTTP client fallbacks and lineup decoding
  Summary:
    - Fixed `internal/hdhomerun.FetchLineupJSON` to accept both top-level lineup arrays and object-shaped payloads with a `Channels` field.
    - Fixed `internal/hdhomerun.FetchDiscoverJSON` to fall back to the requested base URL and derived `/lineup.json` URL when devices omit `BaseURL` or `LineupURL` from `discover.json`.
    - Restored compatibility between the HDHomeRun client/import code and both real HDHomeRun-style payloads and the now-corrected local simulator surface.
    - Added focused regression coverage in `internal/hdhomerun/client_test.go`.
  Verification:
    - `go test ./internal/hdhomerun -run 'Test(ParseDiscoverReply_roundTrip|ParseDiscoverReply_wrongType|ParseExtraDiscoverAddrs_env|ParseLiteralUDPAddr_ipv6Zone|ParseLiteralUDPAddr_bracketIPv6|LineupDocUnmarshalJSONArray|FetchLineupJSONAcceptsJSONArray|FetchDiscoverJSONFallsBackToRequestedBaseURL|ControlServer_(getDiscoverJSONEscapesFriendlyName|httpResponseForPath|httpResponseForRequestMethodHandling|handleConnectionBinaryRequestUsesSniffedHeader|handleConnectionRecognizesPutAsHTTP)|CreateDefaultDevice(DefaultFriendlyName|PrefersHDHRFriendlyNameEnv)|NewGetReqIncludesPropertyName)'`
  Notes:
    - This came directly from widening the HDHomeRun audit from simulator protocol fixes into the sibling client/import layer.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/client.go`
    - `internal/hdhomerun/client_test.go`

- Date: 2026-03-22
  Title: Fix HDHomeRun simulator control-plane packet parsing and method sniffing
  Summary:
    - Fixed the simulated HDHomeRun TCP control server so binary control packets no longer lose their first 4 header bytes after HTTP sniffing.
    - Fixed `NewGetReq` serialization to include the requested property name, and trimmed decoded GET/SET TLV strings so control-property dispatch no longer misses handlers because of trailing NUL bytes.
    - Broadened the HTTP socket sniffer beyond `GET`/`POST`/`HEAD`, so unsupported verbs like `PUT` now reach the existing `405 Method Not Allowed` path instead of being misclassified as binary traffic.
    - Added focused regression coverage in `internal/hdhomerun/control_test.go`.
  Verification:
    - `go test ./internal/hdhomerun -run 'Test(ControlServer_(getDiscoverJSONEscapesFriendlyName|httpResponseForPath|httpResponseForRequestMethodHandling|handleConnectionBinaryRequestUsesSniffedHeader|handleConnectionRecognizesPutAsHTTP)|CreateDefaultDevice(DefaultFriendlyName|PrefersHDHRFriendlyNameEnv)|NewGetReqIncludesPropertyName)'`
  Notes:
    - This came from widening the HDHomeRun audit from discovery/status HTTP surfaces into the sibling TCP control protocol.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/control.go`
    - `internal/hdhomerun/packet.go`
    - `internal/hdhomerun/control_test.go`

- Date: 2026-03-22
  Title: Lock down programming and virtual-channel read surfaces
  Summary:
    - Gated additional programming GET endpoints behind the existing operator UI policy: categories, browse, channel detail, channels, order, backups, and preview.
    - Gated additional virtual-channel GET endpoints behind the same policy: preview, schedule, and channel detail.
    - Updated and expanded `internal/tuner/server_test.go` so the localhost/operator contract is explicit and the audit fails closed if those reads become remotely accessible again.
  Verification:
    - `go test -count=1 ./internal/tuner`
    - `go test -race -count=1 ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This was the same read/write policy drift pattern that had already shown up on entitlements, rulesets, and recorder/debug surfaces; the programming and virtual-channel lanes still had several read-only leaks left.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Lock down operator-only report surfaces that leaked raw upstream and state paths
  Summary:
    - Gated `/autopilot/report.json` because it exposed the Autopilot state-file and host-policy-file paths.
    - Gated `/plex/ghost-report.json` because it exposed PMS session/player metadata and recovery guidance.
    - Gated `/channels/report.json` and `/channels/leaderboard.json` because their `ChannelHealth` rows included `primary_stream_url`, which can reveal raw upstream provider URLs or tokens.
    - Added regression coverage for the new operator-only contract in `internal/tuner/server_test.go` and `internal/tuner/ghost_hunter_test.go`.
  Verification:
    - `go test -count=1 ./internal/tuner ./internal/webui`
    - `go test -race -count=1 ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This was the same underlying leak class as the earlier debug/programming fixes, but on “reporting” endpoints rather than config-edit endpoints.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `internal/tuner/ghost_hunter_test.go`

- Date: 2026-03-22
  Title: Lock down operator config reads and fix programming preview stale-file drift
  Summary:
    - Fixed `/entitlements.json` so read access is operator-only; it previously exposed the full Xtream entitlements ruleset, including downstream usernames/passwords and the configured users-file path, to any remote caller.
    - Extended the same operator-only read gate to the direct ruleset endpoints for programming recipes, virtual-channel rules, and recording rules, which had protected writes but still leaked config state and local file paths on GET.
    - Fixed `/programming/preview.json` so lineup/count/bucket data are recomputed from the just-reloaded recipe file instead of the stale in-memory curated lineup after out-of-band recipe edits.
    - Preserved the preview endpoint's intended split contract by continuing to build backup-group analysis from the uncollapsed preview view even when the visible lineup/count follows the fully applied recipe.
  Verification:
    - `go test -count=1 ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - The preview regression surfaced only after the stale-file fix because binary smoke relies on the endpoint continuing to show collapsed lineup counts while backup-group analysis stays uncollapsed.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-22
  Title: Fix debug bundle leaks, provider XMLTV suffix drift, supervisor env quoting, and HTTP pool init-order
  Summary:
    - Fixed `debug-bundle` env redaction so numbered secrets like `IPTV_TUNERR_PROVIDER_PASS_2` / `_USER_3` are actually redacted, and added regression coverage proving numbered credentials no longer leak into `env.json`.
    - Fixed `debug-bundle` live fetches to use a timeout-capped shared HTTP client instead of bare `http.Get`, preventing the command from hanging indefinitely when the local tuner is stalled or unreachable.
    - Fixed guide-input/provider-EPG URL synthesis to carry `IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX` consistently, so allowlisting, EPG repair, and guide-health match the actual provider `xmltv.php` fetch URL instead of self-blocking suffixed feeds.
    - Fixed supervisor `envFiles` parsing to unquote shell-style values, keeping it aligned with the top-level `.env` loader for Bao/Vault-style `export KEY="value"` files.
    - Fixed `internal/httpclient` shared-client initialization to read HTTP pool env knobs lazily at first use rather than during package init, so `.env`-loaded `IPTV_TUNERR_HTTP_MAX_IDLE_CONNS*` settings now actually take effect.
  Verification:
    - `go test ./internal/httpclient ./internal/supervisor ./internal/guideinput ./cmd/iptv-tunerr`
    - `go test -race -count=1 ./...`
    - `./scripts/verify`
  Notes:
    - This batch was a pure audit/fix pass; no runtime behavior changed outside the affected debug/config paths.
    - The existing live conclusion still holds: `player_api` is healthy on the current providers while `get.php` remains upstream-blocked from this machine.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_debug_bundle.go`
    - `internal/guideinput/guideinput.go`
    - `internal/supervisor/supervisor.go`
    - `internal/httpclient/httpclient.go`

- Date: 2026-03-22
  Title: Fix winning-provider stream ordering and runtime health-check credentials
  Summary:
    - Fixed `fetchCatalog` so when a later ranked provider wins `player_api` indexing, that winning provider stays first in `StreamURL` / `StreamURLs` instead of reintroducing an earlier failed provider as the primary playback target.
    - Added a regression proving later ranked `player_api` success keeps the winning provider first in channel stream ordering.
    - Fixed `run` startup health checks to use the effective winning provider base URL and credentials, rather than always combining the winning base with the primary env credentials.
    - Added focused runtime helper tests and revalidated live single-account and multi-account indexing with the current local `.env`.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'Test(RuntimeHealthCheckURL_|FetchCatalog_)'`
    - `set -a && source ./.env && set +a && IPTV_TUNERR_PROVIDER_URL=\"$IPTV_TUNERR_PROVIDER_URL_3\" IPTV_TUNERR_PROVIDER_USER=\"$IPTV_TUNERR_PROVIDER_USER_3\" IPTV_TUNERR_PROVIDER_PASS=\"$IPTV_TUNERR_PROVIDER_PASS_3\" IPTV_TUNERR_PROVIDER_URLS=\"\" IPTV_TUNERR_PROVIDER_URL_2=\"\" IPTV_TUNERR_PROVIDER_USER_2=\"\" IPTV_TUNERR_PROVIDER_PASS_2=\"\" IPTV_TUNERR_PROVIDER_URL_3=\"\" IPTV_TUNERR_PROVIDER_USER_3=\"\" IPTV_TUNERR_PROVIDER_PASS_3=\"\" IPTV_TUNERR_LIVE_ONLY=true IPTV_TUNERR_CF_AUTO_BOOT=false go run ./cmd/iptv-tunerr index -catalog /tmp/review-single.json`
    - `set -a && source ./.env && set +a && IPTV_TUNERR_LIVE_ONLY=true IPTV_TUNERR_CF_AUTO_BOOT=false go run ./cmd/iptv-tunerr index -catalog /tmp/review-multi.json`
    - `./scripts/verify`
  Notes:
    - This review did not change the upstream `get.php` conclusion: on this machine it remains CF/WAF-blocked while `player_api` works.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_catalog.go`
    - `cmd/iptv-tunerr/cmd_runtime.go`
    - `cmd/iptv-tunerr/cmd_runtime_test.go`

- Date: 2026-03-22
  Title: Make catalog ingest player_api-first and reserve get.php for final backup
  Summary:
    - Changed `fetchCatalog` so ranked and direct provider attempts exhaust `player_api` candidates before trying any `get.php` fallback.
    - Removed the eager per-provider `get.php` fallback during direct/ranked API loops, making `get.php` a true last-ditch backup only when no provider indexed successfully through `player_api`.
    - Added regressions proving `get.php` is not touched when a later ranked or direct `player_api` candidate succeeds.
    - Revalidated live single-account and multi-account indexing after the change.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'TestFetchCatalog_(DoesNotUseGetPHPWhenLaterRankedPlayerAPISucceeds|DoesNotUseGetPHPWhenDirectPlayerAPISucceedsLater|FallsBackToPlayerAPIWhenBuiltGetPHPFails|DirectPlayerAPIFallbackWhenProbeFindsNoOKHost|DoesNotRetryGetPHPAfterDirectForbiddenFallback|SingleCredentialDoesNotRetryGetPHPOnPlayerAPIFailure)'`
    - `set -a && source ./.env && set +a && IPTV_TUNERR_PROVIDER_URL=\"$IPTV_TUNERR_PROVIDER_URL_3\" IPTV_TUNERR_PROVIDER_USER=\"$IPTV_TUNERR_PROVIDER_USER_3\" IPTV_TUNERR_PROVIDER_PASS=\"$IPTV_TUNERR_PROVIDER_PASS_3\" IPTV_TUNERR_PROVIDER_URLS=\"\" IPTV_TUNERR_PROVIDER_URL_2=\"\" IPTV_TUNERR_PROVIDER_USER_2=\"\" IPTV_TUNERR_PROVIDER_PASS_2=\"\" IPTV_TUNERR_PROVIDER_URL_3=\"\" IPTV_TUNERR_PROVIDER_USER_3=\"\" IPTV_TUNERR_PROVIDER_PASS_3=\"\" IPTV_TUNERR_LIVE_ONLY=true IPTV_TUNERR_CF_AUTO_BOOT=false go run ./cmd/iptv-tunerr index -catalog /tmp/playerapi-single-provider3-hardening.json`
    - `set -a && source ./.env && set +a && IPTV_TUNERR_LIVE_ONLY=true IPTV_TUNERR_CF_AUTO_BOOT=false go run ./cmd/iptv-tunerr index -catalog /tmp/playerapi-multi-hardening.json`
  Notes:
    - Reverse-engineering on this machine still points to a hard upstream/Cloudflare deny on `get.php` (`884` / timeout) even with browser-TLS impersonation.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_catalog.go`
    - `cmd/iptv-tunerr/main_test.go`

- Date: 2026-03-22
  Title: Restore CF auto-boot on shared clients and disable desktop-browser fallback by default
  Summary:
    - Fixed `PrepareCloudflareAwareClient` so env-configured shared HTTP clients are upgraded to the persistent cookie-jar type required by the CF bootstrapper, while preserving existing host cookies for the target provider.
    - Added regression coverage for the shared-client jar upgrade path and for the new real-browser fallback gate.
    - Changed CF auto-boot so desktop-browser launch is opt-in via `IPTV_TUNERR_CF_REAL_BROWSER_FALLBACK=true`; default behavior remains headless-only and now logs that real-browser fallback is disabled.
    - Documented the `.env` contamination trap for single-provider diagnostics and reconfirmed live `player_api` success in both single-account and multi-account modes.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `set -a && source ./.env && set +a && IPTV_TUNERR_PROVIDER_URL=\"$IPTV_TUNERR_PROVIDER_URL_3\" IPTV_TUNERR_PROVIDER_USER=\"$IPTV_TUNERR_PROVIDER_USER_3\" IPTV_TUNERR_PROVIDER_PASS=\"$IPTV_TUNERR_PROVIDER_PASS_3\" IPTV_TUNERR_PROVIDER_URLS=\"\" IPTV_TUNERR_PROVIDER_URL_2=\"\" IPTV_TUNERR_PROVIDER_USER_2=\"\" IPTV_TUNERR_PROVIDER_PASS_2=\"\" IPTV_TUNERR_PROVIDER_URL_3=\"\" IPTV_TUNERR_PROVIDER_USER_3=\"\" IPTV_TUNERR_PROVIDER_PASS_3=\"\" IPTV_TUNERR_LIVE_ONLY=true IPTV_TUNERR_CF_AUTO_BOOT=false go run ./cmd/iptv-tunerr index -catalog /tmp/playerapi-single-provider3.json`
    - `set -a && source ./.env && set +a && IPTV_TUNERR_LIVE_ONLY=true IPTV_TUNERR_CF_AUTO_BOOT=false go run ./cmd/iptv-tunerr index -catalog /tmp/playerapi-multi.json`
  Notes:
    - `get.php` remains provider-blocked in the live environment (`884` / timeout) even though `player_api` remains healthy.
    - The newly added local free-source URL currently parses as `0` channels from the fetched response.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/cf_client.go`
    - `internal/tuner/cf_bootstrap.go`
    - `docs/how-to/cloudflare-bypass.md`

- Date: 2026-03-22
  Title: Add single-credential provider fallback regression test
  Summary:
    - Added `TestFetchCatalog_SingleCredentialDoesNotRetryGetPHPOnPlayerAPIFailure` to lock the single-credential failure/retry shape when `player_api` returns 403 and `get.php` fallback also fails.
    - Cleared numbered provider env overrides in that test (`_2.._4`) to guarantee a true single-credential path independent of local developer env.
    - Kept the prior duplicate-`get.php` prevention fix unchanged and preserved existing multi-provider and mixed fallback coverage in `main_test.go`.
  Verification:
    - `go test -count=1 ./cmd/iptv-tunerr -run '^TestFetchCatalog_SingleCredentialDoesNotRetryGetPHPOnPlayerAPIFailure$' -v`
    - `go test -count=1 ./cmd/iptv-tunerr -run 'TestFetchCatalog_FallsBackToGetPHPOnPlayerAPIForbidden|TestFetchCatalog_SingleCredentialDoesNotRetryGetPHPOnPlayerAPIFailure|TestFetchCatalog_DoesNotRetryGetPHPAfterDirectForbiddenFallback'`
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The new test confirms both probe-stage and direct `player_api` calls remain bounded while `get.php` is not retried per-credential.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/main_test.go`

- Date: 2026-03-22
  Title: Fix duplicated get.php fallback attempts after player_api 403 failures
  Summary:
    - Added tracking of attempted provider credentials in `fetchCatalog` to avoid calling `get.php` twice for the same `(base,user,pass)` when direct `player_api` probing returns 403 and later fallback loops run.
    - Kept the existing merge behavior for successful `get.php` feeds while ensuring each credential is attempted at most once in the direct/rollback/fallback passes.
    - Added regression coverage in `TestFetchCatalog_DoesNotRetryGetPHPAfterDirectForbiddenFallback` and exercised mixed candidate counts in `cmd/iptv-tunerr/main_test.go`.
  Verification:
    - `go test -count=1 ./cmd/iptv-tunerr -run 'TestFetchCatalog_DoesNotRetryGetPHPAfterDirectForbiddenFallback|TestFetchCatalog_FallsBackToGetPHPOnPlayerAPIForbidden|TestFetchCatalog_FallsBackToPlayerAPIWhenBuiltGetPHPFails'`
    - `go test ./...`
    - `./scripts/verify`
  Notes:
    - Addresses the tester-reported 403 churn with 3–4 configured credentials and makes provider index fallback behavior deterministic again.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_catalog.go`
    - `cmd/iptv-tunerr/main_test.go`

- Date: 2026-03-21
  Title: Expand macOS host proof beyond WebDAV
  Summary:
    - Extended `scripts/mac-baremetal-smoke.sh` so the real Mac host lane now covers the startup contract, dedicated web UI, Xtream `get.php` / `xmltv.php`, virtual-channel live/short-EPG/schedule/playback, and the existing WebDAV checks.
    - Re-ran `./scripts/release-readiness.sh --include-mac` successfully with that expanded scope.
    - Tightened the release-readiness matrix to mark startup, Xtream export, virtual-channel, and deck rows as macOS host-proven where that evidence now exists.
  Verification:
    - `./scripts/mac-baremetal-smoke.sh`
    - `./scripts/release-readiness.sh --include-mac`
  Notes:
    - This still does not replace real Windows host proof or live provider/client matrix proof, but it removes several "partial" host-proof gaps for surfaces we directly control.
  Opportunities filed:
    - none
  Links:
    - `scripts/mac-baremetal-smoke.sh`
    - `docs/explanations/release-readiness-matrix.md`

- Date: 2026-03-21
  Title: Add dedicated web UI binary smoke proof
  Summary:
    - Extended `scripts/ci-smoke.sh` to start a real `run --skip-index --skip-health` instance with the dedicated web UI enabled.
    - The smoke now logs in through `/login`, reuses the session cookie to hit `/api/debug/runtime.json`, saves `/deck/settings.json` with the extracted CSRF token, and fetches `/api/ops/workflows/diagnostics.json`.
    - Updated `scripts/release-readiness.sh` and the release-readiness matrix so the deck auth/proxy/operator plane is no longer treated as indirect-only proof.
  Verification:
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/release-readiness.sh`
  Notes:
    - This materially strengthens the operator plane contract, but it still is not the same as exhaustive browser automation across every deck lane.
  Opportunities filed:
    - none
  Links:
    - `scripts/ci-smoke.sh`
    - `scripts/release-readiness.sh`
    - `docs/explanations/release-readiness-matrix.md`

- Date: 2026-03-21
  Title: Add dead-remux fallback binary smoke proof
  Summary:
    - Extended `scripts/ci-smoke.sh` with a fake-ffmpeg same-host HLS fallback run against a real temp binary.
    - The smoke now asserts the request still returns bytes and `/debug/stream-attempts.json` records `final_mode: hls_go`.
    - Updated `scripts/release-readiness.sh` and the release-readiness matrix so the HLS fallback path is no longer treated as binary-smoke partial.
  Verification:
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - This strengthens the HLS fallback proof but does not eliminate real live-provider/client variance on all channels.
  Opportunities filed:
    - none
  Links:
    - `scripts/ci-smoke.sh`
    - `scripts/release-readiness.sh`
    - `docs/explanations/release-readiness-matrix.md`

- Date: 2026-03-21
  Title: Add provider-account rollover binary smoke proof
  Summary:
    - Extended `scripts/ci-smoke.sh` with a three-channel synthetic Xtream-path account-pool run against a real temp binary.
    - The smoke now asserts `/provider/profile.json` exposes three distinct active account leases while the requests overlap, turning provider-account rollover into release-gated binary proof.
    - Updated the release-readiness matrix to move provider-account pooling out of the "indirect" smoke tier.
  Verification:
    - `bash -n ./scripts/ci-smoke.sh`
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - This still does not replace real live/provider proof, but it materially reduces the chance of regressing the "second device did not roll over credentials" class.
  Opportunities filed:
    - none
  Links:
    - `scripts/ci-smoke.sh`
    - `docs/explanations/release-readiness-matrix.md`

- Date: 2026-03-21
  Title: Add shared-relay binary smoke proof
  Summary:
    - Extended `scripts/ci-smoke.sh` with a throttled local HLS upstream and a two-consumer same-channel run against a real temp binary.
    - The smoke now asserts both `/debug/shared-relays.json` state and `X-IptvTunerr-Shared-Upstream: hls_go` on the joined client, moving shared HLS relay reuse beyond unit-only confidence.
    - Updated the release-readiness matrix to mark shared relay reuse as binary-smoke proven.
  Verification:
    - `bash -n ./scripts/ci-smoke.sh`
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - This is still not the same as broad host/live proof across Plex/client/provider combinations, but it materially improves release confidence for PAR-002.
  Opportunities filed:
    - none
  Links:
    - `scripts/ci-smoke.sh`
    - `docs/explanations/release-readiness-matrix.md`

- Date: 2026-03-21
  Title: Add explicit release-readiness matrix and gate
  Summary:
    - Added `scripts/release-readiness.sh` so pre-release validation is a concrete repeatable gate instead of an ad hoc mix of `verify` plus memory.
    - Added `docs/explanations/release-readiness-matrix.md`, which maps major feature families to unit/focused, binary-smoke, and host-proof coverage tiers and states what is still not host-proven.
    - Added the new gate to `memory-bank/commands.yml` so future release prep can use one authoritative command.
  Verification:
    - `bash -n ./scripts/release-readiness.sh`
    - `./scripts/release-readiness.sh`
  Notes:
    - Optional host checks remain opt-in because macOS and Windows hosts are not always available from every environment.
  Opportunities filed:
    - none
  Links:
    - `scripts/release-readiness.sh`
    - `docs/explanations/release-readiness-matrix.md`

- Date: 2026-03-21
  Title: Expand Xtream output with short EPG actions
  Summary:
    - Extended the downstream Xtream `player_api.php` surface so it now answers `get_short_epg` and `get_simple_data_table` in addition to the existing live/VOD/series actions.
    - Backed those compact EPG responses with Tunerr's existing guide and virtual-channel schedule pipeline, so both real live channels and virtual channels can return now/next style listings without a separate guide export path.
    - Added focused Xtream regressions covering both a real live channel and a virtual channel short-EPG response.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_Xtream(PlayerAPI_LiveCategories|PlayerAPI_VODAndSeries|PlayerAPI_ShortEPG|LiveProxy|LiveProxy_VirtualChannel|MovieAndSeriesProxy|XtreamEntitlementsLimitOutput)$' -count=1`
    - `./scripts/verify`
  Notes:
    - This is another `PAR-004` expansion inside the broader parity pass, not the end of downstream output work.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server_xtream.go`
    - `internal/tuner/server_test.go`

- Date: 2026-03-21
  Title: Add Xtream M3U and XMLTV export parity
  Summary:
    - Added downstream `get.php` and `xmltv.php` handlers so the same entitled Xtream live lineup can now be exported as user-scoped M3U and XMLTV, not only through `player_api.php` actions and `/live/...` proxies.
    - Backed `xmltv.php` with Tunerr's existing guide and virtual-channel schedule pipeline, so real live channels and virtual channels share one scoped export path.
    - Extended binary smoke and focused Xtream tests to cover the new export surfaces plus entitlement filtering.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_Xtream(PlayerAPI_LiveCategories|PlayerAPI_VODAndSeries|PlayerAPI_ShortEPG|Exports_M3UAndXMLTV|LiveProxy|LiveProxy_VirtualChannel|MovieAndSeriesProxy|XtreamEntitlementsLimitOutput)$' -count=1`
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - This keeps `PAR-004` moving toward a real publishing surface instead of a narrow action proxy.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server_xtream.go`
    - `scripts/ci-smoke.sh`

- Date: 2026-03-21
  Title: Add durable Programming backup-source preference
  Summary:
    - Extended the saved Programming recipe with explicit preferred backup-source IDs so exact-match sibling groups stop depending on incidental ingest order.
    - `/programming/backups.json` now supports preference mutation, collapsed preview/output rows honor the preferred sibling as the visible primary, and the deck exposes one-click source preference in the Programming lane.
    - Added targeted tests plus binary smoke coverage so the preferred sibling survives recipe application and stays release-gated.
  Verification:
    - `go test ./internal/programming ./internal/tuner -run 'Test(UpdateRecipeMutations|BuildBackupGroupsAndCollapse|BuildBackupGroupsAndCollapse_WithPreferences|Server_programmingEndpoints)$' -count=1`
    - `node --check internal/webui/deck.js`
    - `./scripts/verify`
  Notes:
    - This turns the tester's "use DirecTV SyFy when Sling SyFy is flaky" idea into durable server behavior, not just a diagnostics observation.
  Opportunities filed:
    - none
  Links:
    - `internal/programming/programming.go`
    - `internal/tuner/server.go`
    - `internal/webui/deck.js`

- Date: 2026-03-21
  Title: Add batch Programming browse with cached next-hour EPG summaries
  Summary:
    - Added `/programming/browse.json`, which returns one category’s channel rows with derived feed descriptors, recipe inclusion flags, exact-backup counts, cached guide-health status, and next-hour programme titles/counts in one request.
    - Updated the dedicated deck so category cards can switch into a browse view instead of forcing one-channel-at-a-time detail polling, so the selected Programming channel can launch bounded `stream-compare` or exact-backup `channel-diff` captures directly from the same lane, and so operators can toggle “Real Guide Only” / “Only Not In Lineup” while hunting quick-add PPV/event channels.
    - This directly productizes the tester’s “flip through channels and see which ones really have guide data” workflow using cached server state instead of repeated client-side probes.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(programmingEndpoints|programmingBrowse|programmingChannelDetail|diagnosticsHarnessActions)' -count=1`
    - `node --check internal/webui/deck.js`
    - `./scripts/verify`
  Notes:
    - Browse status is intentionally derived from cached guide-health and next-hour capsule preview, so the UI stays interactive even for large categories.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/webui/deck.js`

- Date: 2026-03-21
  Title: Productize diagnostics launchers and Programming feed descriptors
  Summary:
    - Added localhost operator actions for bounded `channel-diff` and `stream-compare` runs so the deck can launch evidence capture directly instead of only listing workflow guidance.
    - Hardened exact-backup grouping to require `tvg_id`/`dna_id` plus normalized guide-name agreement, which stops over-normalized variants like `AMC` vs `AMC Plus` and East/West feeds from collapsing into one visible row.
    - Added derived Programming feed descriptors (`region | category | feedtype/fps-style tags`) across category members, curated preview cards, channel detail, and backup alternatives so the deck can show tester-style feed context without mutating canonical channel names.
    - Fixed a real flaky test in `internal/materializer`: concurrent same-asset GETs could drive the test server `WaitGroup` negative; guarded with `sync.Once` so full verify is stable again.
  Verification:
    - `go test ./internal/programming -run 'Test(BuildBackupGroupsAndCollapse|BuildBackupGroupsDoesNotCollapseVariantNames|DescribeChannel)' -count=1`
    - `go test ./internal/tuner -run 'TestServer_(programmingEndpoints|programmingChannelDetail|diagnosticsHarnessActions|diagnosticsWorkflowAndEvidenceAction|XtreamPlayerAPI_LiveCategories|XtreamLiveProxy|XtreamLiveProxy_VirtualChannel)' -count=1`
    - `node --check internal/webui/deck.js`
    - `./scripts/verify`
  Notes:
    - Feed descriptors are intentionally derived from provider-presented naming/group metadata, not claimed as probed media truth.
  Opportunities filed:
    - none
  Links:
    - `internal/programming/programming.go`
    - `internal/tuner/server.go`
    - `internal/webui/deck.js`

- Date: 2026-03-21
  Title: Promote diagnostics capture into the operator plane
  Summary:
    - Added `/ops/workflows/diagnostics.json`, which turns recent stream attempts into a concrete good-vs-bad capture playbook with suggested channel IDs and latest `.diag/` run families.
    - Added `/ops/actions/evidence-intake-start`, which scaffolds `.diag/evidence/<case-id>/` directly from the app with notes and bundle layout instead of leaving evidence intake purely script-driven.
    - Wired the diagnostics workflow into the dedicated deck and expanded `scripts/ci-smoke.sh` so both the workflow and evidence-bundle creation are release-gated.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(diagnosticsWorkflowAndEvidenceAction|programmingHarvestImport|virtualChannelRulesAndPreview)' -count=1`
    - `node --check internal/webui/deck.js`
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - This is the first productized diagnostics layer; stream-compare/channel-diff execution and in-card report summaries are still the next depth, not already shipped.
  Opportunities filed:
    - `memory-bank/opportunities.md` — remaining step is launching harnesses and summarizing their reports from the deck.
  Links:
    - diagnostics workflow / evidence-intake action

- Date: 2026-03-21
  Title: Surface latest diagnostics verdicts in the deck
  Summary:
    - Extended the diagnostics workflow so it now reads the latest `.diag` report artifacts for `channel-diff`, `stream-compare`, `multi-stream`, and evidence bundles instead of only listing run folders.
    - Added verdict/findings extraction so the deck can show “Tunerr split”, “upstream split”, “stable parallel reads”, or evidence-bundle guidance directly in the Routing lane.
    - Added tuner coverage for the new summary extraction and kept the full release gate green.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_diagnosticsWorkflowAndEvidenceAction' -count=1`
    - `node --check internal/webui/deck.js`
    - `./scripts/verify`
  Notes:
    - This still stops short of launching the harnesses from the deck; it productizes their latest conclusions first.
  Opportunities filed:
    - `memory-bank/opportunities.md` — remaining step is bounded in-app harness execution plus richer report summaries.
  Links:
    - diagnostics verdict summaries

- Date: 2026-03-21
  Title: Bridge virtual channels into Xtream live output
  Summary:
    - Extended the downstream Xtream live surface so enabled virtual channels now appear in `get_live_categories` and `get_live_streams` alongside curated real channels.
    - Added virtual-channel playback through `/live/<user>/<pass>/virtual.<id>.mp4`, reusing the existing Xtream output flow instead of forcing virtual channels to stay in sidecar-only APIs.
    - Added focused Xtream regressions covering virtual-channel category/list output and live proxy playback.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_Xtream(PlayerAPI_LiveCategories|LiveProxy|LiveProxy_VirtualChannel|EntitlementsLimitOutput)' -count=1`
    - `./scripts/verify`
  Notes:
    - This keeps the virtual-channel parity work moving into real client-facing outputs without pretending the full virtual-channel product is finished.
  Opportunities filed:
    - none
  Links:
    - virtual channels in Xtream live output
- Date: 2026-03-21
  Title: Add parity recording-rules and recorder-history starter
  Summary:
    - Added a durable server-side recording-rules model behind `IPTV_TUNERR_RECORDING_RULES_FILE` with `/recordings/rules.json` CRUD.
    - Added `/recordings/rules/preview.json` to evaluate the current ruleset against live catch-up capsules and `/recordings/history.json` to classify recorder state against the active rules.
    - Extended `scripts/ci-smoke.sh` so the release gate now mutates recorder rules over HTTP and checks recorder-history output instead of leaving the new surface as unit-only coverage.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr -run 'Test(RecordingRulesFileRoundTrip|Server_recording(RulesEndpoint|RulePreview|History))' -count=1`
    - `./scripts/verify`
  Notes:
    - This is the first `PAR-003` slice, not the final DVR product; it establishes durable rule state and operator-visible history without pretending full series-rule scheduling is already finished.
  Opportunities filed:
    - none
  Links:
    - recording rules / recorder history starter

- Date: 2026-03-21
  Title: Add active-stream stop control
  Summary:
    - Extended the active-stream registry so sessions now carry client UA, cancelability, and cancellation-request state.
    - Added `/ops/actions/stream-stop`, which cancels matching active stream contexts by request ID or channel ID from the localhost operator plane.
    - Added targeted server tests so active-stream cancellation is covered alongside the existing report surface.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr -run 'Test(Server_(ActiveStreamsReport|StreamStopAction)|RecordingRulesFileRoundTrip|Server_recording(RulesEndpoint|RulePreview|History))' -count=1`
    - `./scripts/verify`
  Notes:
    - This is the next `PAR-007` control slice, not full fanout/reuse yet.
  Opportunities filed:
    - none
  Links:
    - active stream stop control

- Date: 2026-03-21
  Title: Add shared HLS relay reuse foundation
  Summary:
    - Added a bounded `PAR-002` foundation where duplicate same-channel consumers can attach to one live `hls_go` relay instead of always starting another upstream walk.
    - Added `/debug/shared-relays.json` so operators can see current shared sessions and subscriber counts instead of treating the reuse layer as invisible behavior.
    - Added targeted relay/session tests covering fanout delivery, shared-session attachment, and the existing playlist-concurrency retry path.
  Verification:
    - `go test ./internal/tuner -run 'Test(Server_SharedRelayReport|Gateway_(sharedRelaySessionFanout|tryServeSharedRelay|relayHLSAsTS_survivesPlaylistConcurrencyRetry))' -count=1`
    - `./scripts/verify`
  Notes:
    - This first cut is intentionally limited to the native HLS Go-relay path; it is the substrate for deeper fanout/reuse later, not a claim that every stream mode is now shared.
  Opportunities filed:
    - none
  Links:
    - shared HLS relay reuse foundation

- Date: 2026-03-21
  Title: Prepare Windows bare-metal smoke package
  Summary:
    - Added `scripts/windows-baremetal-package.sh` to cross-build a `windows/amd64` `iptv-tunerr.exe` and bundle it with a Windows-local smoke runner.
    - Added `scripts/windows-baremetal-smoke.ps1`, a PowerShell smoke that starts local asset/serve/vod-webdav processes and checks the same startup, web UI, and WebDAV contract we validate elsewhere.
    - Added `docs/how-to/windows-baremetal-smoke.md` and a commands entry so the Windows path is ready the moment a real VM or host is available.
  Verification:
    - `bash -n scripts/windows-baremetal-package.sh`
    - `./scripts/windows-baremetal-package.sh`
  Notes:
    - This is intentionally marked as prepared but not host-validated; there is still no real Windows execution result yet.
  Opportunities filed:
    - none
  Links:
    - windows bare-metal smoke package

- Date: 2026-03-21
  Title: Automate bare-metal macOS smoke with Wake-on-LAN
  Summary:
    - Added `scripts/mac-baremetal-smoke.sh`, a one-command Linux-side runner that cross-builds a darwin/arm64 `iptv-tunerr`, optionally sends Wake-on-LAN magic packets, stages the binary and helper scripts over SSH, runs a real macOS `serve`/web UI smoke plus the `vod-webdav` client-matrix harness, and pulls artifacts back under `.diag/mac-baremetal/`.
    - Added `docs/how-to/mac-baremetal-smoke.md` and a `mac_baremetal_smoke` command entry so the workflow is documented and discoverable.
    - Verified the end-to-end path on `192.168.50.108`; the first run passed and the Mac reported `womp=1`, so host-side wake-for-network access is already enabled.
  Verification:
    - `bash -n scripts/mac-baremetal-smoke.sh`
    - `./scripts/mac-baremetal-smoke.sh`
    - `./scripts/verify`
  Notes:
    - The Wake-on-LAN path is wired and host-side power settings are correct, but this specific run did not require waking the Mac from sleep because it was already online.
  Opportunities filed:
    - none
  Links:
    - bare-metal macOS smoke / Wake-on-LAN

- Date: 2026-03-21
  Title: Add WebDAV header-diff tooling and validate on a real macOS host
  Summary:
    - Extended `scripts/vod-webdav-client-diff.py` so baseline-vs-host comparisons include key read-only/WebDAV response headers, not just HTTP status codes.
    - Added `k8s/vod-webdav-client-macair-job.yaml` and updated the harness how-to with the Mac-node execution path for when `macair-m4` returns to `Ready`.
    - Verified a real macOS host run by cross-building a darwin/arm64 `iptv-tunerr`, copying it to the Mac, running `vod-webdav` locally there, and replaying the full Finder/WebDAVFS + Windows MiniRedir matrix; the resulting bundle matched the local baseline with no status or header differences.
  Verification:
    - `python3 -m py_compile scripts/vod-webdav-client-diff.py`
    - `python3 scripts/vod-webdav-client-diff.py --left <bundle> --right <same-bundle> --print`
    - macOS host run via `scripts/vod-webdav-client-harness.sh` in external mode against a darwin/arm64 binary on `192.168.50.108`
    - `python3 scripts/vod-webdav-client-diff.py --left .diag/vod-webdav-client/<baseline> --right /tmp/iptvtunerr-mac-bundles/mac-selfhost --left-label baseline --right-label macos --print`
    - `./scripts/verify`
  Notes:
    - The Mac itself is usable over SSH now, but the `macair-m4` Kubernetes node still has stale heartbeats and remains `NotReady`, so host-level validation is ahead of cluster scheduling.
  Opportunities filed:
    - none
  Links:
    - WebDAV client diff + macOS host validation

- Date: 2026-03-21
  Title: Start cross-platform VOD parity with shared tree and WebDAV backend
  Summary:
    - Extracted VOD naming/tree generation out of Linux-only files so it can back more than the existing `go-fuse` mount path.
    - Kept Linux `mount` behavior on the shared tree and added `internal/vodwebdav` plus a new `iptv-tunerr vod-webdav` command to expose the same synthetic `Movies/` / `TV/` tree over read-only WebDAV.
    - Updated README/features/platform/CLI docs so Linux `mount` and macOS/Windows `vod-webdav` parity are documented explicitly.
  Verification:
    - `go test ./internal/vodfs ./internal/vodwebdav ./cmd/iptv-tunerr -count=1`
    - `./scripts/verify`
  Notes:
    - This is the first non-Linux parity slice, not the final platform story; mount-helper ergonomics and real macOS/Windows validation still need follow-through.
  Opportunities filed:
    - none
  Links:
    - cross-platform VOD parity / WebDAV backend

- Date: 2026-03-21
  Title: Add channel-class diff harness for intermittent live failures
  Summary:
    - Added `scripts/channel-diff-harness.sh` to capture one known-good and one known-bad channel with matched `stream-compare` runs, inferring paired direct upstream URLs from Tunerr's own `/debug/stream-attempts.json`.
    - Added `scripts/channel-diff-report.py` to classify the pair as upstream-only, Tunerr-only, or a deeper channel-class split using final status/mode, startup duration, upstream status codes, and manifest host spread.
    - Updated troubleshooting docs and `memory-bank/commands.yml` so intermittent reports follow a repeatable good-vs-bad workflow instead of treating all failing channels as one bug.
  Verification:
    - `bash -n scripts/channel-diff-harness.sh`
    - `python3 -m py_compile scripts/channel-diff-report.py`
    - synthetic `python3 scripts/channel-diff-report.py --good <tmp>/good --bad <tmp>/bad --print`
    - `./scripts/verify`
  Notes:
    - This does not prove the tester's exact panel yet; it gives us a reproducible way to classify "some channels work, some don't" without waiting for ad hoc log snippets.
  Opportunities filed:
    - none
  Links:
    - intermittent live-stream diagnostics / good-vs-bad channel diff

- Date: 2026-03-21
  Title: Harden deck proxy/auth flow and add end-to-end dead-remux fallback proof
  Summary:
    - Fixed the dedicated web UI’s generated-password path so a random startup password is actually usable: it is logged once at startup and shown on the localhost login page until a real password is pinned.
    - Stopped `/api/*` proxy forwarding of deck `Authorization`, `Proxy-Authorization`, `Cookie`, and CSRF headers to the tuner, and prevented script/API Basic-auth calls from minting browser sessions or polluting shared deck activity.
    - Stripped upstream `Set-Cookie` from relayed stream responses and added a higher-level `/stream/<id>` regression proving a dead non-transcode ffmpeg-remux path falls back quickly enough to deliver bytes through the Go relay; also made HDHR startup status more explicit with `ScanInProgress=1`, `LineupReady=false`, and `Retry-After` on empty lineup responses.
    - Added an explicit `LICENSE` file and switched the repo to AGPL-3.0-only in README/docs metadata.
  Verification:
    - `go test ./internal/webui ./internal/tuner -run 'Test(NewGeneratesPasswordWhenUnset|SessionAuthOnlyAllowsScriptableBasicAuthWithoutSession|ProxyStripsDeckAuthHeaders|HDHR_lineup_status|HDHR_lineup_empty|CopyStreamResponseHeaders_StripsSetCookie|Gateway_stream_hlsDeadRemuxFallsBackQuickly|Gateway_relayHLSWithFFmpeg_nonTranscodeFirstBytesTimeout|Gateway_relaySuccessfulHLSUpstream_crossHostPlaylistPrefersGoBeforeFFmpegFailure)'`
    - `node --check internal/webui/deck.js`
    - `./scripts/verify`
  Notes:
    - HDHR lineup still stays `200 OK` during startup for compatibility; the fix here is stronger machine-readable loading state, not a protocol break.
  Opportunities filed:
    - none
  Links:
    - deck hardening / HLS fallback proof / AGPL-3.0-only

- Date: 2026-03-21
  Title: Add binary startup smoke to verify, CI, and release
  Summary:
    - Added **`scripts/ci-smoke.sh`** to build a temporary binary, start `serve` against synthetic full and empty catalogs, and assert the real endpoint contract for `readyz`, `guide.xml`, `discover.json`, and `lineup.json`.
    - Wired that smoke into **`scripts/verify-steps.sh`**, **`memory-bank/commands.yml`**, **`.github/workflows/ci.yml`**, and **`.github/workflows/release.yml`** so endpoint startup regressions are checked before tags are packaged.
    - Revalidated the tightened startup guide contract under both focused tuner tests and the new binary smoke lane.
  Verification:
    - `bash ./scripts/ci-smoke.sh`
    - `./scripts/verify`
  Notes:
    - The smoke script intentionally allows a brief cold-start connection race and then asserts stable behavior once the listener is up, which matches real process startup better than assuming instant bind.
  Opportunities filed:
    - none
  Links:
    - release hardening / binary smoke

- Date: 2026-03-21
  Title: Harden startup guide contract and release smoke coverage
  Summary:
    - Changed **`/guide.xml`** startup behavior so provisional placeholder XMLTV now returns `503 Service Unavailable` with `Retry-After: 5` and `X-IptvTunerr-Guide-State: loading` until a real merged guide cache exists.
    - Added `X-IptvTunerr-Startup-State: loading` on HDHR discovery/lineup surfaces before the lineup is loaded so operators and probes can distinguish early startup from steady state.
    - Added a named startup/registration **release_smoke** lane in **`memory-bank/commands.yml`** and corresponding regression tests in **`internal/tuner`**.
  Verification:
    - `go test -count=1 ./internal/tuner -run 'Test(XMLTV_serve|XMLTV_serveCachedGuideReady|HDHR_discover|HDHR_lineup|HDHR_lineup_status|Server_UpdateChannelsTriggersXMLTVRefresh)'`
    - `./scripts/verify`
  Notes:
    - This keeps the placeholder visible for humans while making it non-successful for clients that would otherwise cache startup junk as a real guide.
  Opportunities filed:
    - none
  Links:
    - startup/registration contract

- Date: 2026-03-21
  Title: Harden guide-input fetches and deck auth/state handling
  Summary:
    - Reworked **`internal/guideinput`** so remote guide fetches resolve only through a prepared exact allowlist map before any outbound request is built, addressing the CodeQL uncontrolled-network-request path cleanly.
    - Removed the dedicated deck’s `admin/admin` fallback: unset **`IPTV_TUNERR_WEBUI_PASS`** now generates a one-time startup password, and persisted web UI state no longer stores deck credentials or browser-authored telemetry.
    - Narrowed **`/deck/settings.json`** to non-secret refresh preferences and kept **`/deck/activity.json`** read-only/server-derived; updated runtime/docs/tests to match.
  Verification:
    - `go test ./internal/guideinput ./internal/webui ./internal/config ./cmd/iptv-tunerr`
    - `node --check internal/webui/deck.js`
    - `./scripts/verify`
  Notes:
    - Browser-local trend cards still exist, but shared deck telemetry is no longer client-authored server state.
  Opportunities filed:
    - `memory-bank/opportunities.md` startup/registration readiness contract follow-up
  Links:
    - CodeQL issue #34 + control-deck hardening

- Date: 2026-03-20
  Title: Project-backlog shipped vs open audit + opportunities status
  Summary:
    - **`docs/explanations/project-backlog.md`**: **§1 Shipped** table vs **§2** strategic gaps; **§3** engineering themes; aligns with **[EPIC-live-tv-intelligence](../../docs/epics/EPIC-live-tv-intelligence.md)** (host policy file shipped).
    - **`memory-bank/opportunities.md`**: HLS mux toolkit **Status**; hidden-grab **Status**; Autopilot policy opportunity updated for **`IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE`**.
    - **`docs/CHANGELOG.md`** **[Unreleased]** Documentation.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - backlog accuracy

- Date: 2026-03-20
  Title: Autopilot host policy file for preferred and blocked hosts
  Summary:
    - Added **`IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE`** with JSON **preferred** and **blocked** host lists.
    - Preferred hosts merge with **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`**; blocked hosts are skipped during **`reorderStreamURLs`** when backups remain.
    - Surfaced policy metadata on **`/autopilot/report.json`** and **`/debug/runtime.json`**; added tests and doc updates.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - INT-005 policy memory depth

- Date: 2026-03-20
  Title: Ghost Hunter operator actions and guarded recovery loop
  Summary:
    - Added **`POST /ops/actions/ghost-visible-stop`** and **`POST /ops/actions/ghost-hidden-recover?mode=dry-run|restart`** to the tuner operator surface.
    - Added reusable **`RunGhostHunterRecoveryHelper`** with **`IPTV_TUNERR_GHOST_HUNTER_RECOVERY_HELPER`** override, and reused it from the CLI recovery hook.
    - Updated ops workflow and deck action dock so Ghost Hunter is actionable instead of report-only.
  Verification:
    - `go test ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - INT-008 product loop

- Date: 2026-03-20
  Title: Document project-backlog.md (open work index)
  Summary:
    - **`docs/explanations/project-backlog.md`**: single entry point linking **EPIC LTV/LP**, **`memory-bank/opportunities.md`**, **`known_issues`**, **`docs-gaps`**, **features** § limits; maintenance rules.
    - **Cross-links:** **`docs/index.md`**, **`docs/explanations/index.md`**, **`README`**, **`AGENTS.md`** memory-bank table, **`repo_map.md`**, **`opportunities.md`** intro, **EPIC** See also, **`CHANGELOG`** **[Unreleased]**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - answers “is that all documented?” for consolidated backlog orientation

- Date: 2026-03-19
  Title: Control Deck host quarantine + EPIC LTV observability note
  Summary:
    - **`internal/webui/deck.js`**: **`summarizeProviderProfile`** adds **`quarantined_hosts`**, **`auto_host_quarantine`**, **`upstream_quarantine_skips_total`**; Watch / Routing / wins copy.
    - **`docs/epics/EPIC-live-tv-intelligence.md`**: quarantine observability shipped; further work scoped.
    - **`docs/CHANGELOG.md`**, **`docs/features.md`** (Deck row).
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - INT-010 operator UX

- Date: 2026-03-19
  Title: Provider profile upstream_quarantine_skips_total (INT-010)
  Summary:
    - **`Gateway.upstreamQuarantineSkips`** atomic + **`noteUpstreamQuarantineFilterSkipped`** (Prometheus + profile).
    - **`ProviderBehaviorProfile.UpstreamQuarantineSkipsTotal`** on **`GET /provider/profile.json`**.
    - **`TestGateway_ProviderBehaviorProfile_upstreamQuarantineSkipsTotal`**; **cli-and-env** / **CHANGELOG** / **features**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - complements **`iptv_tunerr_upstream_quarantine_skips_total`**

- Date: 2026-03-19
  Title: Host quarantine tests + Prometheus upstream_quarantine_skips_total
  Summary:
    - **`internal/tuner/prometheus_upstream.go`**: **`iptv_tunerr_upstream_quarantine_skips_total`**; **`promNoteUpstreamQuarantineSkips`** from **`filterQuarantinedUpstreams`** when backups remain.
    - **`internal/tuner/server.go`**: register upstream metrics when **`IPTV_TUNERR_METRICS_ENABLE`**.
    - **`internal/tuner/gateway_test.go`**: **`TestGateway_stream_skipsQuarantinedPrimaryUsesBackup`**, autotune-off + multi-quarantine filter tests.
    - **`internal/tuner/prometheus_upstream_test.go`**: smoke + counter test.
    - **Docs:** **CHANGELOG**, **cli-and-env**, **features**, **observability-prometheus-and-otel**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - INT-010 observability hardening

- Date: 2026-03-19
  Title: Autopilot global preferred hosts (IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS)
  Summary:
    - **`internal/tuner/gateway_adapt.go`**: **`parseAutopilotGlobalPreferredHosts`**, **`pickFirstURLMatchingGlobalPreferredHosts`**; **`autopilotPreferredStreamURL`** order: per-DNA memory → global hosts → consensus.
    - **`internal/tuner/autopilot.go`**: **`AutopilotReport.GlobalPreferredHosts`**; **`gateway_provider_profile.go`**: **`intelligence.autopilot.global_preferred_hosts`** (including env-only when no state file).
    - **`internal/tuner/gateway_stream_upstream.go`** + **`gateway_provider_profile.go`**: optional **host quarantine** (**`IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE`**), **`filterQuarantinedUpstreams`**, profile **`quarantined_hosts`** / **`remediation_hints`**.
    - **`cmd/iptv-tunerr/cmd_runtime_server.go`**: **`tuner.autopilot_global_preferred_hosts`**, **`provider_autotune_host_quarantine_*`** on **`/debug/runtime.json`**.
    - **Tests:** **`gateway_test.go`**, **`autopilot_test.go`**.
    - **Docs:** **CHANGELOG**, **cli-and-env**, **.env.example**, **features**, **README**, **EPIC-live-tv-intelligence**, **opportunities**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - partial entry for provider-level policy (remaining: quarantine / file policy)

- Date: 2026-03-19
  Title: Architecture Mermaid diagram + docs-gaps Medium cleared
  Summary:
    - **`docs/explanations/architecture.md`**: **Visual (Mermaid)** `flowchart` mirroring ASCII overview.
    - **`docs/docs-gaps.md`**: **Medium** empty; **Resolved** row for Mermaid; **`docs/explanations/index`**, **`docs/index`** links.
    - **`docs/CHANGELOG.md`** **[Unreleased]** Documentation.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-19
  Title: Doc/epic backlog validation (docs-gaps, epics, opportunities)
  Summary:
    - **`docs/docs-gaps.md`**: removed stale gap rows; **Resolved** expanded; optional Mermaid for **architecture** remains one medium row.
    - **`docs/epics/EPIC-live-tv-intelligence.md`**, **`EPIC-lineup-parity.md`**, **`memory-bank/work_breakdown.md`**: aligned “next” / EPG notes with shipped policy + **`IPTV_TUNERR_PROVIDER_EPG_*`**.
    - **`memory-bank/opportunities.md`**: guide-health→publishing entry partially superseded.
    - **`docs/how-to/index.md`**: VODFS how-to row; **`docs/CHANGELOG.md`** **[Unreleased]**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - [docs-gaps.md](../../docs/docs-gaps.md)

- Date: 2026-03-20
  Title: LP-010 / LP-011 packaged HLS profile mode
  Summary:
    - **`internal/tuner/gateway_hls_packager.go`** adds a real ffmpeg-packaged HLS session path for named profiles with **`output_mux: "hls"`**, including background tuner holds, session janitor cleanup, packaged playlist rewrite, and packaged segment serving under internal **`mux=hlspkg`** URLs.
    - **`internal/tuner/gateway_stream_response.go`**, **`gateway_servehttp.go`**, **`gateway.go`**, **`gateway_profiles.go`** wire profile-selected packaged HLS without changing explicit **`?mux=hls`** native rewrite/proxy semantics.
    - **`internal/tuner/gateway_hls_packager_test.go`** proves packaged playlist + segment serving and that the packager continues holding a tuner slot after the first response; **`gateway_profiles_test.go`** covers named-profile **`output_mux: "hls"`**.
    - **Docs:** **`docs/reference/transcode-profiles.md`**, **`docs/reference/cli-and-env-reference.md`**, **`docs/features.md`**, **`docs/epics/EPIC-lineup-parity.md`**, **`docs/CHANGELOG.md`**, **`.env.example`**, **`memory-bank/work_breakdown.md`**.
  Verification:
    - `go test ./internal/tuner -run 'Test(BuildFFmpegStreamOutputArgs_fmp4|LoadNamedProfilesFile|PreferredOutputMuxForProfile_namedProfile|Gateway_ffmpegPackagedHLS_namedProfileServesPlaylistAndSegment)'`
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - LP-010 / LP-011

- Date: 2026-03-19
  Title: Hot-start by M3U group_title (IPTV_TUNERR_HOT_START_GROUP_TITLES) + gateway build fix
  Summary:
    - **`internal/tuner/gateway_hotstart.go`**: **`matchesHotStartGroupTitle`**, **`hotStartReason`** **`group_title`**; **`cmd_runtime_server.go`** runtime snapshot **`hot_start_*`** keys.
    - **`internal/tuner/gateway_hotstart_test.go`**: table tests + precedence vs **`HOT_START_CHANNELS`**.
    - **`internal/tuner/gateway.go`**: drop unused **`hlsPackager*`** fields referencing undefined **`ffmpegHLSPackagerSession`**.
    - **Docs:** **CHANGELOG**, **cli-and-env**, **.env.example**, **README** Recent Changes, **features**, **EPIC-live-tv-intelligence**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - LTV **INT-006**

- Date: 2026-03-19
  Title: Prometheus Autopilot consensus gauges + Plex connect how-to (doc closure)
  Summary:
    - **`internal/tuner/prometheus_autopilot.go`**: **`iptv_tunerr_autopilot_consensus_*`** GaugeFuncs when **`IPTV_TUNERR_METRICS_ENABLE`**; wired from **`server.go`**; tests **`prometheus_autopilot_test.go`**.
    - **Docs:** [connect-plex-to-iptv-tunerr.md](../docs/how-to/connect-plex-to-iptv-tunerr.md) indexed; **README** Recent Changes + How-To; **`docs/index`**, **`reference/index`**; **`docs-gaps`** Resolved; **`cli-and-env-reference`**; **`features.md`**; **`opportunities.md`** Plex connect shipped.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - [CHANGELOG](../docs/CHANGELOG.md) [Unreleased]

- Date: 2026-03-19
  Title: Control Deck surfaces Autopilot consensus (intelligence.autopilot)
  Summary:
    - **`internal/webui/deck.js`**: **`summarizeProviderProfile`** adds consensus fields; **`formatAutopilotConsensusMeta`**; **Watch** / **Confirmed wins**; **Operations** Autopilot card meta; endpoint catalog summary.
    - **`docs/CHANGELOG.md`**, **`docs/features.md`**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-19
  Title: Autopilot consensus host (LTV, opt-in runtime + report)
  Summary:
    - **`internal/tuner/autopilot.go`**: **`consensusPreferredHost`**, **`AutopilotReport`** consensus fields; **`report()`** fills thresholds from env.
    - **`internal/tuner/gateway_adapt.go`**: **`autopilotConsensusPreferredURL`**, **`autopilotConsensusHostEnabled`**, **`autopilotPreferredStreamURL`** tries consensus after per-DNA memory misses.
    - **`internal/tuner/gateway_provider_profile.go`**: **`AutopilotIntelSnapshot`** consensus fields.
    - **Docs:** **CHANGELOG**, **cli-and-env-reference**, **.env.example**, **features**, **EPIC-live-tv-intelligence**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - LTV epic Autopilot next slice

- Date: 2026-03-19
  Title: Control Deck provider profile + remediation_hints (LP-004)
  Summary:
    - **`internal/webui/deck.js`**: **`summarizeProviderProfile`**, **`remediationHintsFromProfile`**; fix cards that referenced missing **`provider.summary`**; incidents/watch/wins + decision-board + routing integration for **`remediation_hints`**.
    - **`docs/CHANGELOG.md`** (LP-004 note).
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - **`/provider/profile.json`** **`remediation_hints`** (LTV)

- Date: 2026-03-19
  Title: HDHR DiscoverLAN IPv6 + IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS literals (LP-001)
  Summary:
    - **`internal/hdhomerun/client.go`**: **`parseExtraDiscoverAddrs`** (IPv4 + IPv6), **`parseLiteralUDPAddr`** / **`udpAddrFromLiteralHost`** (zone, bracket IPv6, safe **`::1:port`** split); **`discoverReadLoop`**; **`DiscoverLAN`** parallel UDP6 when IPv6 targets present.
    - **`internal/hdhomerun/client_test.go`**: env split tests, **`[::1]:65001`**, **`fe80::1%eth0:65001`**.
    - **Docs:** **CHANGELOG**, **cli-and-env-reference**, **.env.example**, **features**, **hybrid-hdhr-iptv**, **EPIC-lineup-parity**, **work_breakdown**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - LP-001 epic remaining polish = optional default multicast, not env literals

- Date: 2026-03-19
  Title: Provider profile remediation_hints (LTV advisory layer)
  Summary:
    - **`internal/tuner/gateway_provider_profile.go`**: **`ProviderRemediationHint`** + **`remediationHintsForProfile`**; **`ProviderBehaviorProfile.RemediationHints`** populated on **`GET /provider/profile.json`** from CF blocks, host penalties, concurrency signals, HLS/DASH mux counters.
    - **`internal/tuner/gateway_provider_profile_test.go`**: table-style tests for empty profile, CF hint, penalized hosts, sort order.
    - **`docs/CHANGELOG.md`**, **`docs/reference/cli-and-env-reference.md`**: document **`remediation_hints`**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - LTV epic operator surfacing / “active remediation” precursor

- Date: 2026-03-22
  Title: interpreting-probe-results.md + harness-index.py (backlog closure)
  Summary:
    - **`docs/how-to/interpreting-probe-results.md`**: **`get.php`** vs **`player_api`**, status table, common patterns; **README** **`probe`**, **runbook §4**, **features**, **docs-gaps** (probe row → **Resolved**).
    - **`scripts/harness-index.py`**: newest runs per **`.diag/`** family; **`commands.yml`** **`harness_index`**; harness how-tos; **`opportunities.md`** status lines.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - **`connect-plex`** how-to still open in **`opportunities.md`**

- Date: 2026-03-22
  Title: How-to — stream-compare-harness.md + opportunities backlog (harness index, probe, Plex connect)
  Summary:
    - **`docs/how-to/stream-compare-harness.md`**: **§9** quick start + report; indexes, README, **runbook §9** lead-in, **live-race**/**multi-stream** related sections, **`features`**, **`repo_map`**.
    - **`docs/docs-gaps.md`**: **Resolved** harness how-tos table.
    - **`memory-bank/opportunities.md`**: **`2026-03-22`** entries (unified **`.diag/`** index script, **probe** interpretation how-to, **Plex connect** how-to).
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - see **`memory-bank/opportunities.md`** **2026-03-22** rows (this commit adds them)

- Date: 2026-03-21
  Title: How-to — live-race-harness.md + §7 wiring + multi-stream §7 fix
  Summary:
    - **`docs/how-to/live-race-harness.md`**: quick start, report, **runbook §7**, related harnesses, verify note.
    - **Cross-links:** **`docs/how-to/index`**, **`docs/index`**, **`README`**, **runbooks index**, **troubleshooting §7** lead-in, **`repo_map`**, **`commands.yml`** **`live_race_harness`**.
    - **`multi-stream-harness.md`**: related harness now points to **§7** (was **§6**).
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-21
  Title: How-to — multi-stream-harness.md + doc cross-links
  Summary:
    - **`docs/how-to/multi-stream-harness.md`**: quick start, report command, pointers to **runbook §10** / related harnesses / verify.
    - Linked from **`docs/how-to/index`**, **`docs/index`**, **`README`** (Documentation + Recent Changes), **`docs/runbooks/index`**, **troubleshooting §10** lead-in, **`repo_map`**, **CHANGELOG**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-21
  Title: Verify — bash -n + py_compile for scripts/*.sh and scripts/*.py
  Summary:
    - **`scripts/verify-steps.sh`**: new step after **vet**, before **go test**.
    - **`memory-bank/commands.yml`** **`verify_steps`**, **`repo_map`** QA line, **CHANGELOG** Testing.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-21
  Title: Tests — deterministic HLS playlist retry wait + autotune-off host penalty subtest
  Summary:
    - **`TestGateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry`**: **`sync.Once`** + channel when third playlist response succeeds (after **509** retry); **30s** timeout; atomic playlist hit count.
    - **`TestGateway_shouldPreferGoRelayForHLSRemux_hostPenalty`**: subtests **`penalized_host`** / **`autotune_off_no_penalty_signal`** (`IPTV_TUNERR_PROVIDER_AUTOTUNE=false`).
    - **CHANGELOG** Testing note.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-21
  Title: HLS — prefer Go relay when upstream host penalized + playlist 509 retry integration test
  Summary:
    - **`shouldPreferGoRelayForHLSRemux(streamURL)`**: after provider-pressure checks, return true when **`hostPenalty`** for the stream authority is **> 0** (e.g. ffmpeg remux already failed for that host).
    - **`relaySuccessfulHLSUpstream`**: passes **`streamURL`** into the helper.
    - **Tests:** **`TestGateway_shouldPreferGoRelayForHLSRemux_hostPenalty`**, **`TestGateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry`** (restored after an earlier mistaken **`git restore`**).
    - **Docs:** **CHANGELOG**; **`recurring_loops`** guardrail on discarding dirty WIP.
  Verification:
    - `./scripts/verify`
  Notes:
    - Work was briefly dropped from the working tree while landing a doc-only commit; reconstructed from session diff.
  Opportunities filed:
    - none

- Date: 2026-03-21
  Title: Autopilot URL match — godoc + known_issues (integration test baseline on main)
  Summary:
    - **`gateway_adapt.go`**: **`streamURLsSemanticallyEqual`** godoc lists equivalences, explicit non-goals, and test names.
    - **`known_issues.md`**: **Gateway / Autopilot** explains narrow semantic matching (path segment case not folded).
    - **`current_task.md`**: notes **`TestGateway_stream_..._normalizedTrailingSlash`** already on **`origin/main`**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-21
  Title: LP/LTV close-by-design — Autopilot URL norm + HDHR directed broadcast + LP-012 checklist
  Summary:
    - **`gateway_adapt`**: **`streamURLsSemanticallyEqual`** + tests; **HDHR**: **`IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS`**.
    - **Docs:** [lineup-parity-lp012-closure](../docs/how-to/lineup-parity-lp012-closure.md), indexes, **cli-and-env**, **CHANGELOG**, epics; **`hdhr-scan`** summary.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-20
  Title: Harden HLS provider-pressure handling for multi-stream playback
  Summary:
    - **`509`** now counts as an upstream concurrency signal, and HLS playlist refreshes learn provider limits just like stream-open failures do.
    - HLS playlist refreshes now use bounded retry/backoff on concurrency-style failures, which helps short-lived provider contention recover without immediately killing the stream.
    - Non-transcode HLS now prefers the Go relay over **ffmpeg remux** once provider pressure is already known, reducing extra upstream request churn on fragile providers.
    - Added an integration-style relay test proving **`relayHLSAsTS`** survives a transient **`509`** playlist refresh and keeps writing bytes instead of aborting the stream immediately.
  Verification:
    - `go test ./internal/tuner -run 'Test(IsUpstreamConcurrencyLimit_509|Gateway_shouldPreferGoRelayForHLSRemux|Gateway_fetchAndRewritePlaylist_retriesConcurrencyLimit|Gateway_learnsUpstreamConcurrencyLimitAndRejectsLocally|ParseUpstreamConcurrencyLimit)'`
    - `go test ./internal/tuner -run 'TestGateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry'`
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-20
  Title: Add native mux manifest Prometheus metrics
  Summary:
    - [prometheus_mux.go](../internal/tuner/prometheus_mux.go) now exposes **`iptv_tunerr_mux_manifest_outcomes_total`** and **`iptv_tunerr_mux_manifest_request_duration_seconds`** alongside the existing **`seg`** metrics.
    - [gateway_hls.go](../internal/tuner/gateway_hls.go) and [gateway_stream_response.go](../internal/tuner/gateway_stream_response.go) now record manifest-level outcomes for playlist/MPD rewrites, manifest upstream HTTP errors, **304** responses, and binary relays.
    - [hls-mux-toolkit](../docs/reference/hls-mux-toolkit.md) and [CHANGELOG](../docs/CHANGELOG.md) document the new metrics so operators can distinguish manifest-stage failures from **`seg=`** failures.
  Verification:
    - `go test ./internal/tuner`
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-20
  Title: README + features + doc indexes — cross-reference sync
  Summary:
    - **README**: documentation map (**CHANGELOG**, **features**, HR/mux refs, **architecture**); Kubernetes **`/readyz`** / **`/healthz`** / **`/discover.json`**; **Recent Changes** (native mux header, **`STREAM_PROFILES_FILE`**, **HR-010**, live-race PMS); tuner + **repo layout** (**probe**, **materializer**).
    - **`docs/features.md`**, **`docs/index.md`**, **`reference/index.md`**, **`runbooks/index.md`**, **`how-to/index.md`**.
    - **`docs/CHANGELOG.md`** **[Unreleased · Documentation]**.
  Verification:
    - N/A (markdown)
  Opportunities filed:
    - none

- Date: 2026-03-20
  Title: LP + LTV — **`intelligence.autopilot`** on **`/provider/profile.json`** + epic doc sync
  Summary:
    - **`internal/tuner/gateway_provider_profile.go`**: **`intelligence.autopilot`** snapshot from Autopilot **`report(5)`**.
    - **`serveStreamInvestigateWorkflow`**: **`/autopilot/report.json`**, **`autopilot-reset`** in actions.
    - **`TestGateway_ProviderBehaviorProfile_includesIntelligenceAutopilot`**.
    - **Epics / hybrid-hdhr / features / CHANGELOG / work_breakdown** LP–LTV alignment.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-20
  Title: Capture Plex session snapshots in the live-race harness (**HR-002** / **HR-003**)
  Summary:
    - [live-race-harness.sh](../scripts/live-race-harness.sh) now polls Plex **`/status/sessions`** during the run window when **`PMS_URL`** + **`PMS_TOKEN`** (or Tunerr/Plex env aliases) are configured, storing XML snapshots under **`pms-sessions/`**.
    - [live-race-harness-report.py](../scripts/live-race-harness-report.py) now summarizes observed player titles/products/platforms and live session IDs from those snapshots, not just PMS log patterns.
    - [iptvtunerr-troubleshooting](../docs/runbooks/iptvtunerr-troubleshooting.md), [plex-client-compatibility-matrix](../docs/reference/plex-client-compatibility-matrix.md), and [CHANGELOG](../docs/CHANGELOG.md) document the new evidence bundle.
  Verification:
    - `bash -n scripts/live-race-harness.sh`
    - `python3 -m py_compile scripts/live-race-harness-report.py`
    - synthetic `scripts/live-race-harness-report.py --dir <tmpdir>`
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-20
  Title: Unit tests for **`internal/probe`** and **`internal/materializer`**
  Summary:
    - **`probe_test.go`**: path-only **`Probe`**, HTTP content-type + body sniff, redirect final URL, unknown type; **`Lineup`** / **`LineupHandler`** / **`DiscoveryHandler`**.
    - **`materializer_test.go`**: **`ErrNotReady`**, **`Stub`**, **`DownloadToFile`** (scheme guard, full + ranged download, HTTP error), **`DirectFile`** + concurrent dedup, **`Cache`** hit + direct MP4 + probe failure.
    - **`docs/CHANGELOG.md`**, **`memory-bank/commands.yml`** (**integration_test** clarification).
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none

- Date: 2026-03-20
  Title: **`/readyz`** operability docs + **k8s** readiness alignment
  Summary:
    - Code already exposed **`GET /readyz`** + richer **`/healthz`** (`internal/tuner/server.go`, **`TestServer_readyz`**). This slice documents both and points example **`readinessProbe`** paths at **`/readyz`** (`k8s/iptvtunerr-hdhr-test.yaml`, `k8s/iptvtunerr-supervisor-singlepod.example.yaml`).
    - Runbook §8 **`curl`** recipes; **`k8s/README.md`** verify; **`architecture.md`**; tuner static **`ui/index.html`**; **[CHANGELOG](../docs/CHANGELOG.md)** **[Unreleased]**; **LP-012**; **`opportunities.md`** superseded **`/healthz`** row; **`work_breakdown`** readiness note.
  Verification:
    - `./scripts/verify`
  Links:
    - `internal/tuner/server.go`, `docs/runbooks/iptvtunerr-troubleshooting.md`, `k8s/*`

- Date: 2026-03-20
  Title: Supersede smoketest-cache opportunity + doc cross-links (**LP-012** hygiene)
  Summary:
    - [opportunities.md](opportunities.md): **2026-02-25** smoketest “no disk cache” → **superseded** (**`IPTV_TUNERR_SMOKETEST_CACHE_*`**, **`internal/indexer/smoketest_cache.go`**).
    - [plex-livetv-http-tuning](../docs/reference/plex-livetv-http-tuning.md): **`X-IptvTunerr-Native-Mux`** pointer to [hls-mux-toolkit](../docs/reference/hls-mux-toolkit.md).
    - [hybrid-hdhr-iptv](../docs/how-to/hybrid-hdhr-iptv.md) See also: mux toolkit + troubleshooting; [k8s/README.md](../k8s/README.md) verify **`/healthz`**; [repo_map](repo_map.md) indexer row mentions smoketest cache.
    - [CHANGELOG](../docs/CHANGELOG.md) **[Unreleased]** backlog + docs bullets.
  Verification:
    - `./scripts/verify`
  Links:
    - `memory-bank/opportunities.md`, `memory-bank/repo_map.md`, `docs/reference/plex-livetv-http-tuning.md`, `docs/how-to/hybrid-hdhr-iptv.md`, `k8s/README.md`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: Native mux **`X-IptvTunerr-Native-Mux`** + HR-002 runbook + k8s **`/healthz`** readiness + LP-010 epic sync
  Summary:
    - **`internal/tuner`**: **`X-IptvTunerr-Native-Mux`** (**`hls`** / **`dash`**) on native mux success paths (**`gateway_stream_response`**, **`serveNativeMuxTarget`**); CORS exposes header; tests **`TestGateway_stream_dashMux_returns`**, extended HLS/DASH **`seg=`** assertions.
    - [hls-mux-toolkit](../docs/reference/hls-mux-toolkit.md) + [troubleshooting runbook](../docs/runbooks/iptvtunerr-troubleshooting.md): header doc + **HR-002** closure checklist + **`/healthz`** sanity **`curl`**.
    - [EPIC-lineup-parity](../docs/epics/EPIC-lineup-parity.md): **LP-010** status reflects **`STREAM_PROFILES_FILE`**.
    - **`k8s/`** `iptvtunerr-hdhr-test.yaml` + supervisor example: **readinessProbe** → **`/healthz`** (commented rationale; liveness unchanged on **`/discover.json`** for long startups).
    - [opportunities.md](opportunities.md): superseded **Save/UpdateChannels**, **SIGHUP**, **`/healthz`** rows (all shipped).
  Verification:
    - `go test ./internal/tuner -count=1`
    - `./scripts/verify`
  Links:
    - `internal/tuner/gateway_hls.go`, `internal/tuner/gateway_stream_response.go`, `internal/tuner/gateway_test.go`, `docs/reference/hls-mux-toolkit.md`, `docs/runbooks/iptvtunerr-troubleshooting.md`, `docs/epics/EPIC-lineup-parity.md`, `k8s/iptvtunerr-hdhr-test.yaml`, `k8s/iptvtunerr-supervisor-singlepod.example.yaml`, `memory-bank/opportunities.md`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: Supersede stale XMLTV **`/guide.xml`** cache opportunities
  Summary:
    - [opportunities.md](opportunities.md): **2026-02-24** and **2026-02-25** performance rows marked **superseded** — merged guide caching + **`IPTV_TUNERR_XMLTV_CACHE_TTL`** + **`TestXMLTV_cacheHit`** already live in **`internal/tuner/xmltv.go`**.
    - [CHANGELOG](../docs/CHANGELOG.md) **[Unreleased]** maintainability note; [repo_map](repo_map.md) **`xmltv.go`** row calls out merged-guide cache + test pointer.
  Verification:
    - `rg -n 'cachedXML|CacheTTL|TestXMLTV_cacheHit' internal/tuner/xmltv.go internal/tuner/xmltv_test.go`
    - `./scripts/verify`
  Links:
    - `memory-bank/opportunities.md`, `memory-bank/repo_map.md`, `internal/tuner/xmltv.go`, `internal/tuner/xmltv_test.go`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: LP-010 / LP-011 named profile matrix + mux-aware ffmpeg defaults
  Summary:
    - **`IPTV_TUNERR_STREAM_PROFILES_FILE`** now loads named stream profiles with **`base_profile`**, **`transcode`**, and preferred **`output_mux`** so operators can define product-facing profile names without editing code.
    - Query-profile selection, default profile env, and profile override maps can reference those names; ffmpeg HLS relay now honors a named profile’s preferred output mux when no explicit **`?mux=`** is supplied.
    - Runtime snapshot echoes **`tuner.stream_profiles_file`**; docs updated in [transcode-profiles](../docs/reference/transcode-profiles.md) and [CHANGELOG](../docs/CHANGELOG.md).
  Verification:
    - `go test ./internal/tuner -run 'Test(NormalizeProfileName_HDHRStyleAliases|BuildFFmpegStreamOutputArgs_fmp4|LoadNamedProfilesFile|PreferredOutputMuxForProfile_namedProfile|Gateway_requestAdaptation_queryProfile(NamedProfile|HDHRAlias|PMSXcode))'`
    - `./scripts/verify`
  Links:
    - `internal/tuner/gateway_profiles.go`, `internal/tuner/gateway_relay.go`, `internal/tuner/server.go`, `cmd/iptv-tunerr/cmd_runtime_server.go`, `docs/reference/transcode-profiles.md`

- Date: 2026-03-19
  Title: **`gateway_profiles`** tests + supersede wget runbook opportunity
  Summary:
    - **`internal/tuner/gateway_profiles_test.go`**: **`loadNamedProfilesFile`** (empty path, bad JSON, default **`base_profile`**) + **`resolveProfileSelection`** (named **`transcode`** default **`true`**, explicit remux, HDHR alias **`internet360`**, unknown label) — locks **`IPTV_TUNERR_STREAM_PROFILES_FILE`** behavior.
    - [opportunities.md](opportunities.md): **`plex-reload-guides-batched.py`** **`wget`** item marked **superseded** ( **`curl`** is the tracked fix).
    - [CHANGELOG](../docs/CHANGELOG.md) **[Unreleased]** testing note.
  Verification:
    - `go test ./internal/tuner -run 'Test(LoadNamedProfilesFile|ResolveProfileSelection|PreferredOutputMuxForProfile)'`
    - `./scripts/verify`
  Links:
    - `internal/tuner/gateway_profiles_test.go`, `memory-bank/opportunities.md`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: Named stream profiles env + doc closure (LP-010) + **`potential_fixes`** link
  Summary:
    - **`IPTV_TUNERR_STREAM_PROFILES_FILE`**: optional JSON named profile matrix (**`base_profile`**, **`transcode`**, **`output_mux`**, **`description`**) documented in [cli-and-env-reference](../docs/reference/cli-and-env-reference.md), [transcode-profiles](../docs/reference/transcode-profiles.md), [CHANGELOG](../docs/CHANGELOG.md) (transcode section), **`.env.example`**; [repo_map](repo_map.md) points at **`gateway_profiles.go`**.
    - [potential_fixes](../docs/potential_fixes.md): fixed relative link to **`gateway_adapt.go`**; [docs index](../docs/index.md) lists the doc.
    - [CHANGELOG](../docs/CHANGELOG.md) **Maintainability**: restored **`HTTP_*`** idle-pool scope bullet alongside architecture note; **`potential_fixes`** refresh called out.
  Verification:
    - `./scripts/verify`
  Links:
    - `internal/tuner/gateway_profiles.go`, `internal/tuner/server.go`, `cmd/iptv-tunerr/cmd_runtime_server.go`, `docs/reference/cli-and-env-reference.md`, `docs/reference/transcode-profiles.md`, `docs/potential_fixes.md`, `docs/index.md`, `memory-bank/repo_map.md`

- Date: 2026-03-19
  Title: Architecture + reference index + opportunities hygiene
  Summary:
    - [architecture](../docs/explanations/architecture.md): ingest/lineup/catch-up primary-code links → **`cmd_*`** / **`cmd/iptv-tunerr/`**; runtime learning → **`gateway_servehttp`**, **`gateway_provider_profile`**; “design tension” reflects split CLI layout.
    - [reference index](../docs/reference/index.md): **`cli-and-env-reference`** described as canonical.
    - [opportunities.md](opportunities.md): superseded “missing official CLI reference” and 2025 **`internal/indexer`** dependency notes.
  Verification:
    - `./scripts/verify`
  Links:
    - `docs/explanations/architecture.md`, `docs/reference/index.md`, `memory-bank/opportunities.md`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: Docs + backlog — **`HTTP_*`** scope in cli-and-env; supersede stale **`main.go`** split opportunity
  Summary:
    - [cli-and-env-reference](../docs/reference/cli-and-env-reference.md): paragraph under **`IPTV_TUNERR_HTTP_MAX_IDLE_*`** / idle timeout listing **`httpclient`** consumers and mux **`seg=`** exception.
    - [opportunities.md](opportunities.md): replaced duplicate “split **`main.go`**” entry with **INT-005** superseded note.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - Superseded row for monolithic **`main.go`**.
  Links:
    - `docs/reference/cli-and-env-reference.md`, `memory-bank/opportunities.md`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: Split stream relay branches again and surface recent mux outcomes
  Summary:
    - **`internal/tuner/gateway_stream_response.go`** now owns non-OK upstream handling plus DASH/HLS/raw success relay branches, leaving **`walkStreamUpstreams`** in **`gateway_stream_upstream.go`** as the top-level URL loop.
    - **`/provider_profile.json`** now includes **`last_hls_mux_outcome`** / **`last_dash_mux_outcome`** with matching redacted target URLs + timestamps so operators can see the most recent native-mux success/failure mode without scraping logs.
    - Docs updated in [hls-mux-toolkit](../docs/reference/hls-mux-toolkit.md) and [CHANGELOG](../docs/CHANGELOG.md); regression tests extended in **`gateway_test.go`**.
  Verification:
    - `go test ./internal/tuner -run 'TestGateway_(hlsMuxSeg_upstreamHTTP_passedThrough|dashMuxSeg_successUpdatesRecentOutcome|ProviderBehaviorProfile_hlsMuxSegLimit|hlsMuxSeg_successIncrementsProfileCounter)'`
    - `./scripts/verify`
  Opportunities filed:
    - Marked the old **`gateway_stream_upstream.go`** split opportunity as superseded in **`memory-bank/opportunities.md`**.
  Links:
    - `internal/tuner/gateway_stream_response.go`, `internal/tuner/gateway_stream_upstream.go`, `internal/tuner/gateway_mux_target.go`, `internal/tuner/gateway_provider_profile.go`

- Date: 2026-03-19
  Title: **HR-010** docs + test — **`plex-livetv-http-tuning`**, mux negative test client
  Summary:
    - [plex-livetv-http-tuning](../docs/reference/plex-livetv-http-tuning.md): expanded **`httpclient`** consumer list + mux **`seg=`** exception; HR-007 link → **`gateway_adapt.go`** / **`gateway_policy.go`**.
    - **`gateway_test.go`**: **`serveHLSMuxTarget`** scheme test uses **`httpclient.Default()`** instead of **`http.DefaultClient`**.
  Verification:
    - `./scripts/verify`
  Opportunities filed:
    - none
  Links:
    - `docs/reference/plex-livetv-http-tuning.md`, `internal/tuner/gateway_test.go`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: **HR-010** — Plex, Emby, provider probe on **`httpclient.WithTimeout`**
  Summary:
    - **`internal/plex/dvr.go`**: all former **`&http.Client{Timeout:…}`** → **`httpclient.WithTimeout`** (15s / 30s / 60s preserved).
    - **`internal/plex/library.go`**: **`plexHTTPClient`** → **`httpclient.WithTimeout(60s)`**.
    - **`internal/provider/probe.go`**: default client → **`httpclient.WithTimeout(15s)`**.
    - **`internal/emby/register.go`**: **`newHTTPClient`** → **`httpclient.WithTimeout(30s)`**.
  Verification:
    - `./scripts/verify`
  Notes:
    - Mux **`seg=`** client stays custom (**`mux_http_client.go`**) for **`CheckRedirect`**.
  Opportunities filed:
    - none
  Links:
    - `internal/plex/dvr.go`, `internal/plex/library.go`, `internal/provider/probe.go`, `internal/emby/register.go`

- Date: 2026-03-19
  Title: **HR-010** alignment — EPG pipeline, health, probe on **`httpclient.WithTimeout`**
  Summary:
    - **`internal/tuner/epg_pipeline.go`**: **`httpClientOrDefault`** uses **`httpclient.WithTimeout`** (shared transport + idle pool).
    - **`internal/health/health.go`**: provider + endpoint checks use **`httpclient.WithTimeout`** (15s / 5s).
    - **`internal/probe/probe.go`**: default **`Probe`** client uses **`httpclient.WithTimeout(8s)`**.
    - **`docs/explanations/architecture.md`**: tuner primary code points at **`gateway_servehttp.go`** + **`gateway_*.go`**.
  Verification:
    - `./scripts/verify`
  Notes:
    - Plex / provider / Emby migration completed in a follow-up task same day.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/epg_pipeline.go`, `internal/health/health.go`, `internal/probe/probe.go`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: Lineup-parity slice — CF upstream helper, HDHR **`httpclient`, epic status, backlog cleanup
  Summary:
    - **`internal/tuner/gateway_upstream_cf.go`**: **`tryRecoverCFUpstream`**; **`walkStreamUpstreams`** delegates CF UA cycle + bootstrap.
    - **`internal/hdhomerun`**: nil-client **`FetchDiscoverJSON`** / **`FetchLineupJSON`** / **`FetchGuideXML`** use **`httpclient`**; **`cmd_hdhr_scan`** uses **`httpclient.Default`** and **`WithTimeout(90s)`** for guide.
    - Docs: [EPIC-lineup-parity](docs/epics/EPIC-lineup-parity.md) implementation status; [hls-mux-toolkit](docs/reference/hls-mux-toolkit.md) related-code list; [CHANGELOG](docs/CHANGELOG.md) [Unreleased].
    - Memory bank: **`work_breakdown`** LP progress note; **`opportunities.md`** replace three stale audit entries with one superseded row; **`repo_map`** gateway + HDHR pointers.
  Verification:
    - `./scripts/verify`
  Notes:
    - SQLite / incremental EPG / Postgres / continuous recorder remain explicitly out of this slice (multi-PR).
  Opportunities filed:
    - Superseded entry consolidates old **`main.go`** / monolithic **`gateway.go`** / **`DefaultClient`** audit bullets.
  Links:
    - `internal/tuner/gateway_upstream_cf.go`, `internal/hdhomerun/client.go`, `internal/hdhomerun/guide.go`, `cmd/iptv-tunerr/cmd_hdhr_scan.go`

- Date: 2026-03-19
  Title: INT-006 — extract **`walkStreamUpstreams`** to **`gateway_stream_upstream.go`**
  Summary:
    - New **`internal/tuner/gateway_stream_upstream.go`**: **`walkStreamUpstreams`** contains the historical upstream **`for`** loop (SSRF guard, CF cycling, DASH/HLS/raw paths, autopilot hooks).
    - **`gateway_servehttp.go`**: **`ServeHTTP`** delegates after tuner acquire; unchanged failure surfacing (**503** vs **502**).
  Verification:
    - `./scripts/verify`
  Notes:
    - Raw TS via **`relayRawTSWithFFmpeg`** still returns **`streamHandled`** with empty **`finalStatus`** (unchanged defer semantics).
  Opportunities filed:
    - **`memory-bank/opportunities.md`** optional split note retargeted at **`gateway_stream_upstream.go`** size.
  Links:
    - `internal/tuner/gateway_stream_upstream.go`, `internal/tuner/gateway_servehttp.go`

- Date: 2026-03-19
  Title: INT-006 gateway split + INT-001 httpclient (materializer + loopback)
  Summary:
    - **`internal/tuner/gateway_servehttp.go`**: **`ServeHTTP`** and main stream orchestration moved out of **`gateway.go`**.
    - **`internal/tuner/gateway_mux_ratelimit.go`**: **`allowMuxSegRate`**, **`noteHLSMuxSegOutcome`**, **`noteMuxSegOutcome`**.
    - **`internal/tuner/gateway.go`**: struct + **`errCFBlock`** + context keys only.
    - Materializer / server loopback: nil or default HTTP paths use **`internal/httpclient`** (streaming / default) instead of **`http.DefaultClient`** where applicable.
  Verification:
    - `./scripts/verify`
  Notes:
    - Optional follow-up: split **`gateway_servehttp.go`** further if merge conflicts concentrate there.
  Opportunities filed:
    - Updated **`memory-bank/opportunities.md`** (replaced completed backlog rows with optional **`gateway_servehttp`** split note).
  Links:
    - `internal/tuner/gateway_servehttp.go`, `internal/tuner/gateway_mux_ratelimit.go`, `internal/materializer`, `internal/tuner/server.go`

- Date: 2026-03-19
  Title: Work breakdown HR-006 — deterministic live channel order in catalog
  Summary:
    - **`catalog.ReplaceWithLive`** sorts **`live_channels`** in place by **`channel_id`** / **guide_number** / **guide_name**; test **`TestReplaceWithLive_stableChannelOrder`**.
  Verification:
    - `./scripts/verify`
  Notes:
    - Mutates the caller’s live slice; documented on **`ReplaceWithLive`**.
  Opportunities filed:
    - none
  Links:
    - `internal/catalog/catalog.go`, `docs/reference/plex-livetv-http-tuning.md`

- Date: 2026-03-19
  Title: Work breakdown HR-007 — transcode override file merges with off/on/auto
  Summary:
    - **`effectiveTranscodeForChannelMeta`**: **`IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`** overrides global **`STREAM_TRANSCODE`** for **`off`/`on`/`auto`**; **`auto_cached`** unchanged (file-only + remux default).
    - Logs **`gateway: transcode policy ...`**; tests **`internal/tuner/gateway_policy_test.go`**; runtime **`transcode_overrides_file`** / **`profile_overrides_file`**; docs + **README** + **`.env.example`** + **CHANGELOG**.
  Verification:
    - `./scripts/verify`
  Notes:
    - Client adaptation still applied after base transcode bool in **`gateway.go`**.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_policy.go`, `docs/reference/plex-livetv-http-tuning.md`

- Date: 2026-03-19
  Title: Work breakdown HR-010 / HR-009 / HR-008 — HTTP pool env + Plex ops docs
  Summary:
    - **`internal/httpclient`**: **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS`**, **`IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC`**, **`parseSharedTransportEnv`** + tests; **`/debug/runtime.json`** tuner keys **`http_max_idle_conns`**, **`http_idle_conn_timeout_sec`**.
    - **`docs/reference/plex-livetv-http-tuning.md`**; runbook §9 (parallel HTTP, live failover, DVR soak); **CHANGELOG**, **cli-and-env**, **hls-mux-toolkit**, **`.env.example`**, **work_breakdown** progress notes.
  Verification:
    - `./scripts/verify`
  Notes:
    - Granular stories from [work_breakdown.md](work_breakdown.md) worked **end toward beginning** (**HR-010** → **HR-009** → **HR-008**).
  Opportunities filed:
    - none
  Links:
    - `internal/httpclient/httpclient.go`, `docs/runbooks/iptvtunerr-troubleshooting.md`

- Date: 2026-03-19
  Title: Close mux regression-fixture backlog with committed HLS and DASH captures
  Summary:
    - Added committed stream-compare fixture docs in **`internal/tuner/testdata/README.md`**, tracked DASH upstream/expected MPD goldens, and gitignored **`.diag/`** so local harness captures stay disposable until promoted.
    - Finished the native mux regression slice around those fixtures: HLS BOM stripping and **`URI='...'`** rewrite, DASH quote-aware **`SegmentTimeline`** parsing, paired **`SegmentTemplate`**, **`$Time$`** / padded **`$Number%0Nd$`**, fuzz seeds, and full-body HLS/DASH golden tests.
    - Aligned operator/release docs and cleaned the active handoff summary so the repo reflects one coherent “capture -> fixture -> regression test” workflow instead of scattered notes.
  Verification:
    - `./scripts/verify`
  Notes:
    - The DASH stream-compare golden intentionally enables **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE=1`**, so the expected MPD is post-expand **`SegmentList`** output rather than raw **`SegmentTemplate`**.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/testdata/README.md`, `internal/tuner/gateway_test.go`, `docs/runbooks/iptvtunerr-troubleshooting.md`

- Date: 2026-03-19
  Title: DASH stream-compare golden — SegmentTemplate expansion enabled
  Summary:
    - **`TestRewriteDASHManifestToGatewayProxy_streamCompareCaptureGolden`** sets **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE=1`**; expected MPD is expanded **SegmentList** + proxy URLs (**3** segments for **`PT6S`** / 2s @ timescale **600**).
  Verification:
    - `./scripts/verify`
  Notes:
    - Upstream fixture unchanged (**SegmentTemplate** with **`$Number$`**); golden encodes post-expand Tunerr output.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/testdata/stream_compare_dash_mux_capture_tunerr_expected.mpd`, `docs/CHANGELOG.md`, runbook

- Date: 2026-03-19
  Title: DASH stream-compare golden + strict HLS stream-compare golden
  Summary:
    - **`testdata/stream_compare_dash_mux_capture_{upstream,tunerr_expected}.mpd`** + **`TestRewriteDASHManifestToGatewayProxy_streamCompareCaptureGolden`** with **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`** (DASH expansion policy: see the newer entry above).
    - **`TestRewriteHLSPlaylistToGatewayProxy_streamCompareCaptureGolden`** now **`bytes.Equal`** full bodies ( **`splitHLSLines`** trailing-empty drop + upstream newline shape align with expected).
  Verification:
    - `./scripts/verify`
  Notes:
    - Regenerate DASH expected if **`gatewayDashMuxProxyURL`** / **`dashSegQueryEscape`** or **SegmentList** expansion output changes.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_test.go`, `docs/CHANGELOG.md`, `docs/runbooks/iptvtunerr-troubleshooting.md`

- Date: 2026-03-19
  Title: Stream-compare HLS capture → testdata golden + .diag gitignore
  Summary:
    - **`testdata/stream_compare_hls_mux_capture_upstream.m3u8`** / **`_tunerr_expected.m3u8`** from harness synthetic run; **`TestRewriteHLSPlaylistToGatewayProxy_streamCompareCaptureGolden`** with **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`**.
    - **`.diag/`** in **`.gitignore`**; runbook “Turning a failing provider stream…” + **CHANGELOG** [Unreleased].
  Verification:
    - `./scripts/verify`
  Notes:
    - Compare **`bytes.TrimRight(..., \"\\n\")`** so playlist trailing-newline quirks don’t flake.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_test.go`, `docs/runbooks/iptvtunerr-troubleshooting.md`

- Date: 2026-03-19
  Title: SegmentTimeline <S> — quote-aware scanner, nested balance, no regex
  Summary:
    - Replaced **`reSegmentTimelineS`** with **`dashConsumeSTag`**, **`dashFindMatchingCloseS`**, **`dashParseSegmentTimeline`** ( **`dashIsTimelineOpenSTag`** avoids **`<SegmentTimeline>`** false positives).
    - Tests: nested **`<S>`**, quoted **`>`** in attrs, **`hls-mux-proxy`** note on **testdata** fixtures for harness captures.
  Verification:
    - `./scripts/verify`
  Notes:
    - Nested **`<S>`** still yields one segment row from outer **`<S>`** only (invalid MPD).
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_dash_expand.go`

- Date: 2026-03-19
  Title: Mux gateway — SegmentTimeline <S>inner</S> (attrs-only), harness coordination note
  Summary:
    - **`reSegmentTimelineS`:** second branch **`>[\s\S]*?</S\s*>`** so comments/text inside **`S`** do not block expansion.
    - **`current_task.md`**: handoff line—harness captures fixtures; gateway owns rewrite/expand in **`gateway_dash_expand.go`**.
    - Docs: toolkit table row, backlog (nested **`S`**, **`>`** in quoted attrs); CHANGELOG folded into mux polish bullet; fuzz seed.
  Verification:
    - `./scripts/verify`
  Notes:
    - Nested **`<S>`** inside **`<S>`** still unsupported (invalid MPD; document only).
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_dash_expand.go`, `memory-bank/current_task.md`

- Date: 2026-03-19
  Title: Mux toolkit — SegmentTimeline <S></S>, UTF-8 BOM strip on mux rewrite
  Summary:
    - **`reSegmentTimelineS`:** alternation for **`<S …/>`** and empty **`<S …></S>`**.
    - **`stripLeadingUTF8BOM`** (**`gateway_support.go`**) at start of **`rewriteHLSPlaylistToGatewayProxy`** and **`rewriteDASHManifestToGatewayProxy`**.
    - Tests: paired-**S** expand, BOM HLS/DASH rewrite.
  Verification:
    - `./scripts/verify`
  Notes:
    - **`<S>`** with non-whitespace child content still unsupported (backlog).
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_dash_expand.go`, `gateway_support.go`, `gateway_hls.go`, `gateway_dash.go`

- Date: 2026-03-19
  Title: Mux toolkit — DASH quotes, template % width, SegmentTimeline expand, HLS URI='
  Summary:
    - **DASH rewrite:** **`dashAttrURL`** matches **single- or double-quoted** **`media=`** / **`initialization=`** / **`sourceURL`** / **`url`** / **`segmentURL`**; output normalizes to double quotes.
    - **`dashSegQueryEscape`:** restores **`$Number%2505d$` → `$Number%05d$`** (and **`$Time…`**) after **`url.QueryEscape`**.
    - **Expand:** **`reSegmentTemplatePaired`**; **`SegmentTimeline`** + **`$Time$`** / **`$Number$`**; **`$Number%0Nd$`**; skip self-closing templates nested inside a paired template (**`dashSpanInsideAny`**).
    - **HLS:** **`hlsQuotedURIAttrSingle`** + **`rewriteHLSQuotedURIAttrs`**; **`URI='`** gate on **`#`** lines.
    - **Tests/docs:** **`gateway_test`**, **`gateway_dash_expand_test`**, fuzz seed; toolkit / LL-HLS / how-to / CLI env / CHANGELOG.
  Verification:
    - `./scripts/verify`
  Notes:
    - **`<S …></S>`** (non–self-closing) timeline elements not parsed yet — listed in toolkit backlog.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_dash.go`, `gateway_dash_expand.go`, `gateway_hls.go`

- Date: 2026-03-19
  Title: Mux toolkit follow-up — runtime.json echo, fuzz seeds, how-to + repo_map
  Summary:
    - **`buildRuntimeSnapshot`**: **`tuner.hls_mux_dash_expand_segment_template`**, **`tuner.hls_mux_dash_expand_max_segments`** (raw env strings).
    - **Fuzz:** HLS merged **EXTINF/BYTERANGE** seed; DASH **SegmentTemplate** MPD seed (expand still default off during fuzz).
    - **Docs:** **`docs/how-to/hls-mux-proxy.md`**, **`docs/CHANGELOG.md`**; **`memory-bank/repo_map.md`** links **`gateway_dash_expand.go`**.
  Verification:
    - `./scripts/verify`
  Notes:
    - none
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime_server.go`, `internal/tuner/gateway_fuzz_test.go`

- Date: 2026-03-19
  Title: Mux toolkit — DASH SegmentTemplate→SegmentList (opt-in), HLS merged EXTINF+BYTERANGE
  Summary:
    - **DASH:** **`expandDASHSegmentTemplatesToSegmentList`** (**`internal/tuner/gateway_dash_expand.go`**) behind **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE`** + **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_MAX_SEGMENTS`**; **`rewriteDASHManifestToGatewayProxy`** calls it first when enabled.
    - **HLS:** **`parseExtInfMergedByteRange`** normalizes same-line **`BYTERANGE=`** to **`#EXT-X-BYTERANGE`**.
    - **Tests:** **`gateway_dash_expand_test.go`**, **`gateway_test.go`** cases for expand + merged EXTINF + env-gated rewrite.
    - **Docs:** **`docs/reference/hls-mux-toolkit.md`**, **`hls-mux-ll-hls-tags.md`**, **`cli-and-env-reference.md`**, **`.env.example`**, **`docs/CHANGELOG.md`** [Unreleased].
  Verification:
    - `./scripts/verify`
    - `go test ./internal/tuner/ -count=1`
  Notes:
    - Expansion skips **`$Time$`**, missing template duration, **`SegmentTimeline`** in attr scan only (non–self-closing templates not matched by expand regex).
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_dash_expand.go`, `internal/tuner/gateway_hls.go`

- Date: 2026-03-19
  Title: Native mux nice-to-haves — DASH $ in seg=, LL-HLS tags, Brotli, Prometheus histogram/channel labels, Autopilot seg bonus
  Summary:
    - **DASH:** **`dashSegQueryEscape`** preserves **`$`** in **`seg=`** for **SegmentTemplate**; **`gatewayDashMuxProxyURL`**; **`<BaseURL>`** chain includes **`$`** paths.
    - **HLS:** **`extInfSameLineMedia`** rewrite; tests for **PRELOAD-HINT**, **RENDITION-REPORT**; **`docs/reference/hls-mux-ll-hls-tags.md`**.
    - **HTTP:** **`IPTV_TUNERR_HTTP_ACCEPT_BROTLI`**, **`brotliRoundTripper`**, lazy env read per request; **`CloneDefaultTransport`**; vendored **andybalholm/brotli**.
    - **Metrics:** **`iptv_tunerr_mux_seg_request_duration_seconds`**; optional **`IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS`**; **`noteMuxSegOutcome`** passes **`channelID`** + duration.
    - **Autopilot:** **`muxAutopilotMaxHits`**, **`IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_*`**, **`effectiveHLSMuxSegLimitLocked(channel)`**.
    - **Runtime snapshot** keys for new envs; docs/CHANGELOG/.env/cli/toolkit/index updates.
  Verification:
    - `./scripts/verify`
  Notes:
    - Histogram/counter label set chosen at first metrics registration (process lifetime).
  Opportunities filed:
    - none
  Links:
    - `internal/httpclient/brotli.go`, `internal/tuner/gateway_dash.go`, `internal/tuner/gateway_hls.go`, `internal/tuner/prometheus_mux.go`, `internal/tuner/gateway_policy.go`, `internal/tuner/autopilot.go`

- Date: 2026-03-19
  Title: Native mux — redirect SSRF hardening, DASH relative rewrite, adaptive seg slots, access log, golden/tests, ADR/OTEL docs
  Summary:
    - **`muxSegHTTPClient`**: **`CheckRedirect`** validates each hop (scheme + literal/resolved private, max **10**); **`errMuxRedirectPolicy`** → **403** / **502** + **`redirect_rejected`** / **`blocked_private_upstream`**.
    - **`safeurl.ValidateMuxSegTarget`** shared with gateway **`seg=`** checks; **`internal/safeurl/mux_target_test.go`**.
    - **`rewriteDASHManifestToGatewayProxy`**: **`<BaseURL>`** chain + relative attribute resolution; **`$`** in template values skipped.
    - **`IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO`** (+ window / per-hit / cap); **`noteMuxSegConcurrencyReject`** on **503** limit; **`effectiveHLSMuxSegLimitLocked`** adds bonus when **`MAX_CONCURRENT`** unset.
    - **`IPTV_TUNERR_HLS_MUX_ACCESS_LOG`** JSONL on successful **`seg=`**; Prometheus outcome **`err_redirect`**.
    - Golden **`internal/tuner/testdata/hls_mux_small_playlist.golden`**; tests: adaptive limit, chunked upstream, redirect block, DASH relative, **`TestRewriteHLSPlaylistToGatewayProxy_matchesGolden`**.
    - **ADR 0005** (no in-process disk packager); **`docs/explanations/observability-prometheus-and-otel.md`**; toolkit/how-to/CHANGELOG/.env.example/index updates.
  Verification:
    - `./scripts/verify`
  Notes:
    - OpenTelemetry in-process OTLP not added; collector scrape of **`/metrics`** documented.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/mux_http_client.go`, `internal/tuner/gateway_dash.go`, `internal/tuner/gateway_policy.go`, `internal/tuner/gateway_hls.go`, `internal/tuner/gateway.go`, `docs/adr/0005-hls-mux-no-disk-packager.md`, `docs/explanations/observability-prometheus-and-otel.md`

- Date: 2026-03-19
  Title: Native mux epic — DASH MPD proxy, DNS SSRF option, Prometheus, rate limit, demo, fuzz, soak, ops decode
  Summary:
    - **`?mux=dash`** (**experimental**): **`rewriteDASHManifestToGatewayProxy`**, **`serveNativeMuxTarget`** / shared **`seg=`** pool with HLS; main loop **`dash_native_mux`** / passthrough; profile **`dash_mux_seg_*`** atomics.
    - **`IPTV_TUNERR_HLS_MUX_DENY_RESOLVED_PRIVATE_UPSTREAM`** + **`safeurl.HTTPURLHostResolvesToBlockedPrivate`**; per-IP **`IPTV_TUNERR_HLS_MUX_SEG_RPS_PER_IP`** (**429** **`seg_rate_limited`**); stream-attempt prefixes **`hls_`** / **`dash_`**.
    - **Prometheus** **`iptv_tunerr_mux_seg_outcomes_total{mux,outcome}`** + **`GET /metrics`** when **`IPTV_TUNERR_METRICS_ENABLE`**; **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST`** in **`httpclient`**; logs **`hls_mux_diag`** on client/upstream mux errors.
    - **`POST /ops/actions/mux-seg-decode`**, **`/debug/hls-mux-demo.html`** (**`IPTV_TUNERR_HLS_MUX_WEB_DEMO`**), **`scripts/hls-mux-soak.sh`**, fuzz tests, **EXT-X-PART** regression, **`go mod vendor`** (+ **client_golang**, **x/time/rate**).
    - Docs/toolkit/how-to/index/features/CHANGELOG/cli/.env.example/runtime URLs; **`operator_ui` embed** includes demo HTML.
  Verification:
    - `./scripts/verify`
  Notes:
    - Follow-up: redirect-hop validation + DASH relative **`<BaseURL>`** chain shipped in a later entry; DNS deny fails open on lookup errors.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_dash.go`, `internal/tuner/gateway_hls.go`, `internal/tuner/gateway.go`, `internal/tuner/prometheus_mux.go`, `internal/safeurl/privateresolve.go`, `docs/reference/hls-mux-toolkit.md`

- Date: 2026-03-19
  Title: HLS mux backlog slice — limits, SSRF-ish literal-private block, JSON errors, HEAD, counters, docs
  Summary:
    - **`safeurl.HTTPURLHostIsLiteralBlockedPrivate`**; gateway validates **`seg=`** order: length → scheme → optional literal-private deny → concurrency → proxy.
    - **`newUpstreamRequestMethod`**: **`HEAD`** preserved only for **`serveHLSMuxTarget`**; main **`/stream`** stays **GET**. Forward **`X-Request-Id`**, **`X-Correlation-Id`**, **`X-Trace-Id`** on upstream requests.
    - Atomic **`hls_mux_seg_*`** counters on **`Gateway`** + **`provider_profile.json`** fields; **`/debug/runtime.json`** **`tuner.hls_mux_*`** env echo for new knobs.
    - Tests: safeurl private IP helper; tuner tests for JSON **400**, param max, **403** private, **HEAD** upstream, success counter, correlation headers.
    - Docs: CHANGELOG, CLI/env, hls-mux-toolkit (status table + backlog ticks), how-to, **`.env.example`**.
    - **`internal/webui`**: fix **`logout`** / **`strconv`** / test **`Server`** literals so **`go vet`** + tests pass.
  Verification:
    - `./scripts/verify`
  Notes:
    - Hostname SSRF (DNS → internal IP) not addressed; counters are in-process only (no Prometheus).
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway.go`, `internal/tuner/gateway_hls.go`, `internal/tuner/gateway_upstream.go`, `internal/tuner/gateway_provider_profile.go`, `internal/safeurl/safeurl.go`, `cmd/iptv-tunerr/cmd_runtime_server.go`

- Date: 2026-03-19
  Title: HLS mux — operator toolkit reference + large enhancement backlog list
  Summary:
    - New **`docs/reference/hls-mux-toolkit.md`**: shipped-behavior table, **`X-IptvTunerr-Hls-Mux-Error`** values, stream-attempt statuses, env quick list, **`curl`**/jq snippets, categorized backlog (protocol, security, perf, DRM, observability, testing, ecosystem).
    - Linked from **`docs/index.md`**, **`docs/reference/index.md`**, **`docs/how-to/hls-mux-proxy.md`**, **`docs/reference/transcode-profiles.md`**, **`memory-bank/repo_map.md`**; **`[Unreleased]`** CHANGELOG bullet; **`memory-bank/opportunities.md`** entry (docs-only).
  Verification:
    - `./scripts/verify`
  Notes:
    - Backlog rows are planning prompts; promote to **`opportunities.md`** with evidence when starting a slice.
  Opportunities filed:
    - none (toolkit is the umbrella; see opportunities entry “consolidated operator toolkit”)
  Links:
    - `docs/reference/hls-mux-toolkit.md`

- Date: 2026-03-19
  Title: HLS mux — pass through upstream 4xx/5xx for seg= (vs always 502)
  Summary:
    - `hlsMuxUpstreamHTTPError` + `respondHLSMuxUpstreamHTTP`; preview body up to **8 KiB**; diagnostic **`X-IptvTunerr-Hls-Mux-Error: upstream_http_<status>`**; **`finalStatus`** **`hls_mux_upstream_http_<status>`**; playlist branch uses same type for unexpected non-200 HLS responses.
    - Tests `TestServeHLSMuxTarget_returnsUpstreamHTTPError`, `TestGateway_hlsMuxSeg_upstreamHTTP_passedThrough`; docs CHANGELOG/cli/how-to; removed unused **`fmt`** import from **`gateway_hls.go`**.
  Verification:
    - `./scripts/verify`
  Notes:
    - Transport failures and bad `newUpstreamRequest` still surface as gateway **502**.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_hls.go`, `internal/tuner/gateway.go`, `internal/tuner/gateway_test.go`

- Date: 2026-03-19
  Title: HLS mux — diagnostic header on unsupported seg= (X-IptvTunerr-Hls-Mux-Error)
  Summary:
    - `respondHLSMuxUnsupportedTargetScheme`: **`applyHLSMuxCORS`** + **`X-IptvTunerr-Hls-Mux-Error: unsupported_target_scheme`** + **400**; **`Access-Control-Expose-Headers`** includes that header when CORS is on.
    - Tests for header and CORS expose; docs CHANGELOG/cli/how-to.
  Verification:
    - `./scripts/verify`
  Notes:
    - Distinct from **`X-HDHomeRun-Error` / `805`** (tuner-in-use) signal.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_hls.go`, `internal/tuner/gateway.go`, `internal/tuner/gateway_test.go`

- Date: 2026-03-19
  Title: HLS mux — 400 for unsupported seg= schemes + stream-attempt status
  Summary:
    - Reject non-http(s) **`?mux=hls&seg=`** before segment concurrency acquire; **`400`** + body **`unsupported hls mux target URL scheme`**; log line with **`safeurl.RedactURL(target)`**.
    - Stream attempt **`finalStatus`** **`hls_mux_unsupported_target_scheme`** when **`errors.Is(..., errHLSMuxUnsupportedTargetScheme)`**; `serveHLSMuxTarget` still returns sentinel for direct callers/tests.
    - Docs: CLI/env reference; CHANGELOG + how-to already mention behavior; test `TestGateway_hlsMuxSeg_unsupportedScheme_returnsBadRequest`.
  Verification:
    - `./scripts/verify`
  Notes:
    - Avoids burning an **`hlsMuxSegInUse`** slot on **`skd://`** and similar.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway.go`, `internal/tuner/gateway_hls.go`, `internal/tuner/gateway_test.go`

- Date: 2026-03-19
  Title: HLS mux — SAMPLE-AES / SESSION-KEY rewrite hardening + tests
  Summary:
    - `rewriteHLSQuotedURIAttrs`: skip empty inner URI; tag-line gate uses case-insensitive `URI="` so `uri="` rewrites.
    - Tests: SAMPLE-AES + EXT-X-SESSION-KEY + EXT-X-MEDIA; empty `URI=""`; lowercase `uri=`; docs/CHANGELOG caveats (DRM limits unchanged).
  Verification:
    - `./scripts/verify`
  Notes:
    - HTTP(S) key URLs still proxy; `skd://` / non-HTTP schemes remain unsupported at `seg=` (existing `safeurl` check).
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_hls.go`, `internal/tuner/gateway_test.go`, `docs/how-to/hls-mux-proxy.md`

- Date: 2026-03-19
  Title: HLS mux — optional CORS for ?mux=hls (playlist + seg + OPTIONS)
  Summary:
    - `IPTV_TUNERR_HLS_MUX_CORS` + `config.HlsMuxCORS`; `applyHLSMuxCORS` / `maybeServeHLSMuxOPTIONS` in `gateway_hls.go`; gateway calls OPTIONS handler after channel resolve.
    - `/debug/runtime.json` `tuner.hls_mux_cors`; docs (CHANGELOG, how-to, cli ref, README, `.env.example`); tests for OPTIONS, playlist, and seg responses.
  Verification:
    - `./scripts/verify`
  Notes:
    - Off by default; permissive `Access-Control-Allow-Origin: *` when enabled.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_hls.go`, `internal/tuner/gateway.go`, `internal/config/config.go`, `cmd/iptv-tunerr/cmd_runtime_server.go`

- Date: 2026-03-19
  Title: HLS mux — seg concurrency cap + 304 passthrough + conditional upstream headers
  Summary:
    - `hlsMuxSegInUse` + `effectiveHLSMuxSegLimitLocked` (`IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT`, `IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER`); 503/805 when over cap; acquire/release around `serveHLSMuxTarget` in `gateway.go`.
    - `serveHLSMuxTarget`: HTTP **304** with ETag/Cache-Control forward; `If-None-Match` / `If-Modified-Since` on `forwardedUpstreamHeaderNames`; provider profile `hls_mux_seg_in_use` / `hls_mux_seg_limit`.
    - Tests `TestGateway_serveHLSMuxTarget_forwardsNotModified`, `TestGateway_hlsMuxSeg_rejectsWhenAtConcurrencyLimit`, `TestGateway_ProviderBehaviorProfile_hlsMuxSegLimit`; docs/CHANGELOG/cli/how-to/README/.env.example.
  Verification:
    - `./scripts/verify`
  Notes:
    - Default cap = effective tuner limit × 8 so parallel HLS segment fetches do not starve a typical player.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway.go`, `internal/tuner/gateway_hls.go`, `internal/tuner/gateway_policy.go`, `internal/tuner/gateway_upstream.go`, `internal/tuner/gateway_provider_profile.go`

- Date: 2026-03-20
  Title: HLS mux — forward Range/If-Range; preserve 206 + Content-Range on seg=
  Summary:
    - `forwardedUpstreamHeaderNames`: `Range`, `If-Range`; `serveHLSMuxTarget` accepts 200/206, copies Content-Range/Accept-Ranges/etc. for binary responses; playlists still require 200.
    - Test `TestGateway_serveHLSMuxTarget_forwardsRangeAndPartialContent`; docs/cli CHANGELOG.
  Verification:
    - `./scripts/verify`
  Notes:
    - Applies to all gateway upstream requests, not only mux (helps any ranged upstream fetch).
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_upstream.go`, `internal/tuner/gateway_hls.go`

- Date: 2026-03-20
  Title: HLS mux proxy — rewrite URI= on #EXT-X-KEY / MAP / STREAM-INF
  Summary:
    - `rewriteHLSQuotedURIAttrs` + `resolveHLSMediaRef` in `internal/tuner/gateway_hls.go`; tag lines with `URI="..."` now proxy like segment lines.
    - Test `TestRewriteHLSPlaylistToGatewayProxy_rewritesExtXKeyAndStreamInfURI`; CHANGELOG + how-to caveat update.
  Verification:
    - `./scripts/verify`
  Notes:
    - Widevine/FairPlay DRM not in scope.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_hls.go`

- Date: 2026-03-19
  Title: HLS mux proxy how-to + doc/index sweep
  Summary:
    - New `docs/how-to/hls-mux-proxy.md`; `docs/reference/transcode-profiles.md` and `docs/epics/EPIC-lineup-parity.md` updated (proxy vs ffmpeg packaging).
    - Linked from docs index, how-to index, reference index; CHANGELOG [Unreleased]; `memory-bank/repo_map.md` gateway_hls note; opportunities entry refreshed.
  Verification:
    - `./scripts/verify`
  Notes:
    - Docs-only; behavior unchanged.
  Opportunities filed:
    - Updated incremental XMLTV entry
  Links:
    - `docs/how-to/hls-mux-proxy.md`

- Date: 2026-03-19
  Title: HLS mux + incremental EPG — docs, runtime field, epg-store flags, tests
  Summary:
    - `/debug/runtime.json` tuner: `stream_public_base_url`; `/guide/epg-store.json`: `incremental_upsert`, `provider_epg_incremental`.
    - Docs: CHANGELOG, features, README, cli-and-env-reference, `.env.example` for `?mux=hls` and `IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`.
    - Tests: `TestGateway_stream_hlsMux_returnsRewrittenPlaylist`, `TestServer_epgStoreReport_incrementalFlags`.
  Verification:
    - `./scripts/verify`
  Notes:
    - Completes documentation/tests pass started in the same session.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime_server.go`, `internal/tuner/server.go`, `internal/tuner/gateway_test.go`

- Date: 2026-03-19
  Title: Provider incremental suffix tokens, SQLite upsert mode, native HLS mux proxy
  Summary:
    - Provider EPG: `IPTV_TUNERR_PROVIDER_EPG_INCREMENTAL` + suffix tokens `{from_unix}`/`{to_unix}`/`{from_ymd}`/`{to_ymd}` using SQLite horizon.
    - SQLite: `SyncMergedGuideXMLUpsert` and env `IPTV_TUNERR_EPG_SQLITE_INCREMENTAL_UPSERT` for overlap-window upsert sync.
    - Gateway: `?mux=hls` serves rewritten HLS playlists and proxied segment/variant targets from Tunerr (`/stream/<id>?mux=hls&seg=...`).
  Verification:
    - `./scripts/verify`
  Notes:
    - Provider window params remain panel-specific; token rendering only shapes suffix strings.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/epg_pipeline.go`, `internal/epgstore/sync.go`, `internal/tuner/gateway_hls.go`

- Date: 2026-03-19
  Title: Provider EPG conditional HTTP + disk cache (`IPTV_TUNERR_PROVIDER_EPG_DISK_CACHE`)
  Summary:
    - `parseXMLTVProgrammes`, `fetchProviderXMLTVConditional`: optional file path + `*.meta.json` for ETag/Last-Modified; HTTP 304 parses cached body.
    - Config / `Server` / `XMLTV` wiring; test `TestFetchProviderXMLTV_conditionalDiskCache`.
    - Docs: CHANGELOG, cli-and-env-reference, features, README, `.env.example`; opportunities updated.
  Verification:
    - `./scripts/verify`
  Notes:
    - Many Xtream panels omit validators — full download each refresh unless upstream supports 304.
  Opportunities filed:
    - Updated existing “incremental XMLTV” entry (partial mitigation)
  Links:
    - `internal/tuner/epg_pipeline.go`, `internal/config/config.go`

- Date: 2026-03-19
  Title: Lineup parity — LP-002 lineup merge, LP-009 max bytes, fMP4 mux, LP-012 how-to
  Summary:
    - Index: `IPTV_TUNERR_HDHR_LINEUP_URL` / `HDHR_LINEUP_ID_PREFIX`, `hdhomerun.LiveChannelsFromLineupDoc`, `mergeHDHRCatalogChannels`.
    - EPG SQLite: `EnforceMaxDBBytes`, `IPTV_TUNERR_EPG_SQLITE_MAX_BYTES` / `_MAX_MB`, `max_bytes` on epg-store report.
    - Gateway: `buildFFmpegStreamOutputArgs`, `?mux=fmp4` (transcode), Content-Type `video/mp4`.
    - Docs: `docs/how-to/hybrid-hdhr-iptv.md`, epic/changelog/features/cli updates.
  Verification:
    - `./scripts/verify`
  Notes:
    - Multi-file HLS packaging from Tunerr not implemented; provider incremental xmltv fetch still future optimization.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_profiles.go`, `internal/epgstore/quota.go`, `cmd/iptv-tunerr/cmd_catalog.go`

- Date: 2026-03-19
  Title: LP-009 partial — EPG SQLite VACUUM opt-in + epg-store file stats
  Summary:
    - `Store` retains DB path; `DBFilePath`, `DBFileStat`, `Vacuum()`; `IPTV_TUNERR_EPG_SQLITE_VACUUM` runs after retain-past prune when rows removed.
    - `/guide/epg-store.json`: `db_file_bytes`, `db_file_modified_utc`, `vacuum_after_prune`.
    - Config + `XMLTV`/`Server` wiring; tests in `epgstore` and `server_test`.
  Verification:
    - `./scripts/verify`
  Notes:
    - No hard disk quota; VACUUM can be slow on very large files — opt-in by design.
  Opportunities filed:
    - none
  Links:
    - `internal/epgstore/store.go`, `internal/tuner/epg_pipeline.go`

- Date: 2026-03-19
  Title: LP-003 partial — HDHR guide.xml merge into /guide.xml
  Summary:
    - Config: `IPTV_TUNERR_HDHR_GUIDE_URL`, `IPTV_TUNERR_HDHR_GUIDE_TIMEOUT`; `Server` / `XMLTV` / `newRuntimeServer` wiring.
    - `buildMergedEPG` fetches HDHR XMLTV; `mergeChannelProgrammes` adds non-overlapping hardware programmes after provider+external; HDHR-only path when no provider/external for a `tvg-id`.
    - ADR [docs/adr/0004-hdhr-guide-epg-merge.md](../docs/adr/0004-hdhr-guide-epg-merge.md); cli/features/changelog/epic updates.
  Verification:
    - `./scripts/verify`
  Notes:
    - Matching is strict `tvg-id` equality with device `guide.xml` programme `channel` attrs; catalog alignment is operator responsibility.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/epg_pipeline.go`, `internal/config/config.go`

- Date: 2026-03-19
  Title: LP-010/LP-011 partial — transcode profile aliases + pmsxcode query
  Summary:
    - `normalizeProfileName`: HDHR/SiliconDust-style labels (`native`, `internet360`, `mobile`, …) map to existing TS profiles.
    - `requestAdaptation`: explicit `?profile=pmsxcode` uses transcode=true with other named profiles.
    - Tests: `gateway_profiles_test.go`, `gateway_test.go` query cases; doc `docs/reference/transcode-profiles.md`.
  Verification:
    - `./scripts/verify`
  Notes:
    - HLS/fMP4 container outputs still out of scope; epic documents future work.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/gateway_profiles.go`, `internal/tuner/gateway_adapt.go`

- Date: 2026-03-19
  Title: LP-009 partial — SQLite retain prune + provider EPG URL suffix
  Summary:
    - `SyncMergedGuideXML(data, retainPastHours)` prunes programmes with `stop_unix` before cutoff; orphan `epg_channel` rows; returns pruned row count.
    - Config: `IPTV_TUNERR_EPG_SQLITE_RETAIN_PAST_HOURS`, `IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX`; `providerXMLTVEPGURL` helper + test; `Server` / `XMLTV` wiring.
    - `/guide/epg-store.json` includes `retain_past_hours`; docs/README/.env.example.
  Verification:
    - `./scripts/verify`
  Notes:
    - Standard Xtream `xmltv.php` has no documented date-range params; suffix is for panels that support extra query params.
  Opportunities filed:
    - none
  Links:
    - `internal/epgstore/sync.go`, `internal/tuner/epg_pipeline.go`

- Date: 2026-03-19
  Title: LP-008 partial — merged guide sync to SQLite + /guide/epg-store.json
  Summary:
    - `epgstore.SyncMergedGuideXML`, migration v2 `epg_meta`, `MaxStopUnixPerChannel` / `GlobalMaxStopUnix` / `RowCounts` / `MetaLastSyncUTC`.
    - `XMLTV.EpgStore` + `Server.EpgStore`; `maybeOpenEpgStore` returns `*Store`; sync after each successful `refresh()` in `epg_pipeline.go`.
    - `GET /guide/epg-store.json` (`?detail=1`); operator `/ui/` link; tests in `epgstore` + `server_test`.
  Verification:
    - `./scripts/verify`
  Notes:
    - Full-table replace each refresh — optimize later if needed; provider-side incremental fetch using max-stop not wired yet.
  Opportunities filed:
    - none (perf note can go to opportunities if we measure huge guides)
  Links:
    - `internal/epgstore/sync.go`, `internal/tuner/epg_pipeline.go`, `internal/tuner/server.go`

- Date: 2026-03-19
  Title: LP-007 partial — epgstore schema + ADR 0003 (SQLite vs Postgres)
  Summary:
    - `internal/epgstore`: open SQLite with WAL, `PRAGMA user_version` migrations, tables `epg_channel` / `epg_programme`.
    - `IPTV_TUNERR_EPG_SQLITE_PATH` + `config.EpgSQLitePath`; `maybeOpenEpgStore` in `serve`/`run`.
    - ADR [docs/adr/0003-epg-sqlite-vs-postgres.md](../docs/adr/0003-epg-sqlite-vs-postgres.md); docs: CHANGELOG, features, CLI ref, epic, README, `.env.example`, `docs/adr/index.md`.
    - No write path from XMLTV yet — **LP-008**.
  Verification:
    - `./scripts/verify`
  Notes:
    - Postgres intentionally out of scope until multi-writer/shared EPG is a product requirement; see `memory-bank/opportunities.md`.
  Opportunities filed:
    - `memory-bank/opportunities.md` (Postgres optional backend)
  Links:
    - `internal/epgstore/`, `docs/adr/0003-epg-sqlite-vs-postgres.md`

- Date: 2026-03-19
  Title: LP-006 operator guide preview (/ui/guide/)
  Summary:
    - `XMLTV.GuidePreview(limit)` + `GuidePreview` / `GuidePreviewRow` types: sorted programmes from merged cached XMLTV, cache expiry metadata.
    - `internal/tuner/operator_ui.go`: `operatorUIAllowed`, `/ui/guide/` HTML (`static/ui/guide.html`), `/ui/guide-preview.json`; `serveOperatorUI` refactored to shared gate.
    - `internal/tuner/server.go`: register routes before `/ui/` prefix; tests in `xmltv_test.go` / `server_test.go`.
    - Docs: CHANGELOG, features, CLI reference, epic status; `gofmt -s` on `internal/config/config.go` (verify drift).
  Verification:
    - `./scripts/verify`
  Notes:
    - Distinct from `/guide/highlights.json` (time-windowed “now/soon”); preview is linear cache inspection for operators.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/xmltv.go`, `internal/tuner/operator_ui.go`

- Date: 2026-03-19
  Title: PR-2/3 partial: HDHR guide.xml probe + operator /ui
  Summary:
    - `internal/hdhomerun/guide.go`: `FetchGuideXML`, `AnalyzeGuideXMLStats`, `GuideURLFromBase`; `hdhr-scan -guide-xml`.
    - `internal/tuner/operator_ui.go` + `static/ui/index.html`: embedded `/ui/` dashboard; `Server.AppVersion`; `IPTV_TUNERR_UI_DISABLED` / `IPTV_TUNERR_UI_ALLOW_LAN`; `Version` wired in `newRuntimeServer`.
    - Docs: CLI reference, CHANGELOG, features, epic status, README.
  Verification:
    - `./scripts/verify`
  Notes:
    - No Tunerr EPG pipeline merge yet (LP-003 follow-on).
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/guide.go`, `internal/tuner/operator_ui.go`, `docs/reference/cli-and-env-reference.md`

- Date: 2026-03-19
  Title: PR-1 LP-001/LP-002: hdhr-scan + HDHR client + ADR 0002 merge semantics
  Summary:
    - Added `internal/hdhomerun/client.go`: `DiscoverLAN`, `ParseDiscoverReply`, `FetchDiscoverJSON`, `FetchLineupJSON`, URL helpers; `client_test.go` for TLV round-trip.
    - Added `iptv-tunerr hdhr-scan` (`cmd_hdhr_scan.go`) with `-timeout`, `-addr`, `-lineup`, `-json`; wired in `main.go`.
    - ADR [docs/adr/0002-hdhr-hardware-iptv-merge.md](../docs/adr/0002-hdhr-hardware-iptv-merge.md); docs: CLI reference, features, CHANGELOG [Unreleased], README table, `docs/adr/index.md`, EPIC-lineup-parity status.
  Verification:
    - `./scripts/verify`
  Notes:
    - No catalog import yet; separate instances per ADR until explicit merge.
  Opportunities filed:
    - none
  Links:
    - `docs/epics/EPIC-lineup-parity.md`

- Date: 2026-03-19
  Title: Epic: Lineup-app parity (HDHR client, dashboard, SQLite EPG, transcode profiles)
  Summary:
    - User approved all four product tracks vs [am385/lineup](https://github.com/am385/lineup); added [docs/epics/EPIC-lineup-parity.md](../docs/epics/EPIC-lineup-parity.md) with stories `LP-001`–`LP-012`, milestones, PR plan, decision defaults, coordination with `INT-*` epic.
    - Linked epic from [docs/index.md](../docs/index.md); [memory-bank/work_breakdown.md](../memory-bank/work_breakdown.md) overlay; [memory-bank/current_task.md](../memory-bank/current_task.md) approval note.
  Verification:
    - N/A (docs/memory-bank only)
  Notes:
    - Existing `internal/hdhomerun/` is virtual-server discovery; epic distinguishes client-side hardware integration.
  Opportunities filed:
    - none
  Links:
    - `docs/epics/EPIC-lineup-parity.md` · `memory-bank/work_breakdown.md` · `memory-bank/current_task.md` · `docs/index.md`

- Date: 2026-03-19
  Title: README refresh for catch-up recording (daemon, env, CLI)
  Summary:
    - README: What's New for `catchup-daemon`; After Catch-Up Publishing subsection (commands, fallbacks, deprioritize hosts, retention, observability, doc links); CLI table row for `catchup-record`; Key Environment Variables for catch-up guide/replay plus subsection **Catch-up recording (daemon / CLI)** with `IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE` and `IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS`.
  Verification:
    - `./scripts/verify`
  Notes:
    - `IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE` lives under the catch-up recording subsection (not the Guide/XMLTV table).
  Opportunities filed:
    - none
  Links:
    - `README.md`

- Date: 2026-03-19
  Title: Release v0.1.14 (CF docs, debug-bundle, recorder deprioritize hosts)
  Summary:
    - Shipped `iptv-tunerr debug-bundle`, `scripts/analyze-bundle.py`, `docs/how-to/cloudflare-bypass.md`, `docs/how-to/debug-bundle.md`, README Cloudflare section; wired `debugBundleCommands()` in main.
    - Recorder: `IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS` + `ApplyRecordURLDeprioritizeHosts` / `DeprioritizeRecordSourceURLs`; docs and features tables updated; CHANGELOG [v0.1.14]. (Remote already had `v0.1.13` at another commit; this release is tagged `v0.1.14`.)
  Verification:
    - `./scripts/verify`
  Notes:
    - Git tag `v0.1.14` triggers GitHub release workflow (binaries).
  Opportunities filed:
    - none
  Links:
    - `docs/CHANGELOG.md`, `.github/workflows/release.yml`

- Date: 2026-03-19
  Title: Recorder multi-upstream failover, catalog UA on capture, time-based retention, soak script
  Summary:
    - `CatchupCapsule` gains `record_source_urls` / `preferred_stream_ua`; `EnrichCatchupCapsulesRecordURLs` merges Tunerr `/stream/<id>` with catalog stream URLs when `-record-upstream-fallback` is on (default for daemon/catchup-record).
    - `RecordCatchupCapsuleResilient` loops URLs: resets spool on upstream switch; `spoolCopyFromHTTP` sends optional UA; metrics `upstream_switches` + state `sum_capture_upstream_switches`.
    - Daemon: `-retain-completed-max-age`, `-retain-completed-max-age-per-lane` (`72h`, `7d`, …); `pruneCatchupRecorderCompletedMaxAge`.
    - `scripts/recorder-daemon-soak.sh`; tests for URL build, failover 404→200, max-age prune, duration parsing.
  Verification:
    - `./scripts/verify`
  Notes:
    - Fallback order follows catalog stream URL list, not live host-penalty autotune.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/catchup_record_urls.go`, `internal/tuner/catchup_record_resilient.go`, `cmd/iptv-tunerr/cmd_catalog.go`, `cmd/iptv-tunerr/cmd_reports.go`

- Date: 2026-03-19
  Title: Recorder resilient HTTP (Range resume, Retry-After backoff, metrics) + CF ops
  Summary:
    - Recorder: `RecordCatchupCapsuleResilient` / `spoolCopyFromHTTP` with 200 vs 206 append (seek-to-EOF before copy on 206), `recordHTTPStatusError`, `parseRetryAfterHeader`, status backoff multipliers; `catchup-record` stays one-shot via thin wrapper; daemon uses `-record-resume-partial` (default true).
    - Recorder observability: per-item `capture_http_attempts`, `capture_transient_retries`, `capture_bytes_resumed` and statistics `sum_*` fields on `CatchupRecorderItem` / `CatchupRecorderStatistics`.
    - Upstream/CF: persisted `cf-learned.json` + startup restore, `IPTV_TUNERR_HOST_UA`, `cf-status` CLI, CF bootstrap browser headers + optional clearance freshness monitor; gateway/learned UA wiring.
  Verification:
    - `./scripts/verify`
  Notes:
    - Resume requires upstream support for Range/206; otherwise behavior falls back to full re-fetch semantics.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/catchup_record_resilient.go`, `internal/tuner/catchup_record_retry.go`, `internal/tuner/catchup_daemon.go`, `internal/tuner/cf_learned_store.go`, `cmd/iptv-tunerr/cmd_cf_status.go`, `docs/CHANGELOG.md`

- Date: 2026-03-19
  Title: Recorder retries, lane budget stats, deferred library refresh
  Summary:
    - Added transient error classification and exponential-backoff retries for `catchup-daemon` captures (`RecordMaxAttempts`, `RecordRetryInitial`, `RecordRetryMax`).
    - Extended `recorder-state.json` statistics with `lane_storage` (used vs budget headroom) when per-lane byte budgets apply.
    - Added optional `-defer-library-refresh` with `OnManifestSaved` to refresh Plex/Emby/Jellyfin once after `recorded-publish-manifest.json` updates; added `LoadRecordedCatchupPublishManifest`.
    - Tests: retry helpers, daemon 503→200 integration, lane stats, manifest round-trip, CLI hook wiring.
  Verification:
    - `./scripts/verify`
  Notes:
    - Non-transient failures (e.g. HTTP 404) do not loop; context cancel/deadline still fail fast without “retry success” semantics.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/catchup_daemon.go`, `internal/tuner/catchup_record_retry.go`, `internal/tuner/catchup_record_publish.go`, `cmd/iptv-tunerr/cmd_reports.go`

- Date: 2026-03-19
  Title: Catch-up recorder spool/finalize (.partial.ts → .ts)
  Summary:
    - `RecordCatchupCapsule` writes to a spool file and renames to the final `.ts` only after a complete, successful transfer; added `CatchupRecordArtifactPaths`.
    - `catchup-daemon` active items now record `output_path` as the spool path while a capture is in flight.
    - Added unit tests for paths, successful finalize, and deadline leaving a spool without a final file; adjusted interrupted-recording daemon test for `*.partial.ts` naming.
    - Updated changelog, features blurb, and CLI reference for the spool/finalize behavior.
  Verification:
    - `gofmt` on touched Go files
    - `go test -count=1 ./...`
    - `./scripts/verify`
  Notes:
    - Remaining pragmatic recorder slices (see prior roadmap): richer mid-record retry/backoff policy, tighter publish/budget/ops polish.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/catchup_record.go`, `internal/tuner/catchup_daemon.go`, `internal/tuner/catchup_record_test.go`, `internal/tuner/catchup_daemon_test.go`

- Date: 2026-03-19
  Title: Harden local smoke harness and normalize epg-link-report output
  Summary:
    - Updated `scripts/iptvtunerr-local-test.sh` so its default smoke path no longer depends on remote provider/XMLTV fetches from `.env`, making local loopback readiness deterministic.
    - Validated `guide-health`, `epg-doctor`, `catchup-capsules`, and `epg-link-report` against a real locally served `guide.xml`.
    - Changed `epg-link-report` to emit full JSON to stdout when `-out` is not provided, matching the rest of the report commands.
  Verification:
    - `IPTV_TUNERR_BASE_URL=http://127.0.0.1:5019 IPTV_TUNERR_ADDR=:5019 ./scripts/iptvtunerr-local-test.sh all`
    - `go run ./cmd/iptv-tunerr guide-health -catalog ./catalog.json -guide http://127.0.0.1:5019/guide.xml`
    - `go run ./cmd/iptv-tunerr epg-doctor -catalog ./catalog.json -guide http://127.0.0.1:5019/guide.xml`
    - `go run ./cmd/iptv-tunerr catchup-capsules -catalog ./catalog.json -xmltv http://127.0.0.1:5019/guide.xml`
    - `go run ./cmd/iptv-tunerr epg-link-report -catalog ./catalog.json -xmltv http://127.0.0.1:5019/guide.xml`
  Notes:
    - `epg-link-report` still logs its summary and unmatched rows to stderr; this pass only made the full report available on stdout by default.
  Opportunities filed:
    - none
  Links:
    - scripts/iptvtunerr-local-test.sh, cmd/iptv-tunerr/cmd_guide_reports.go, docs/reference/cli-and-env-reference.md

- Date: 2026-03-19
  Title: Fix second-pass audit CLI and local-test harness issues
  Summary:
    - Adjusted the top-level usage path so `iptv-tunerr help` behaves like a normal help request and exits successfully.
    - Fixed `scripts/iptvtunerr-local-test.sh` so explicit runtime override env vars win over `.env`, which restores the loopback/alternate-port smoke path.
    - Revalidated the exact repros with `help`, explicit loopback smoke, and full repo verification.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `go run ./cmd/iptv-tunerr help`
    - `IPTV_TUNERR_BASE_URL=http://127.0.0.1:5015 IPTV_TUNERR_ADDR=:5015 ./scripts/iptvtunerr-local-test.sh all`
    - `./scripts/verify`
  Notes:
    - This closes the remaining concrete defects found during the local audit that were fixable without external provider/media-server credentials.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/main_test.go, scripts/iptvtunerr-local-test.sh

- Date: 2026-03-19
  Title: Fix audit follow-up CLI and script defects
  Summary:
    - Added a top-level `help` / `-h` / `--help` alias path in the CLI entrypoint so usage is reachable through the expected command form.
    - Added entrypoint tests for command normalization and rendered usage text.
    - Restored the executable bit on `scripts/quick-check.sh` and reran the second-pass audit checks.
  Verification:
    - `go test ./cmd/iptv-tunerr`
    - `./scripts/quick-check.sh`
    - `go run ./cmd/iptv-tunerr help`
    - `./scripts/verify`
  Notes:
    - This was the direct follow-up to the broad repo audit; it closes both concrete defects found in the first pass.
  Opportunities filed:
    - none
  Links:
    - cmd/iptv-tunerr/main.go, cmd/iptv-tunerr/main_test.go, scripts/quick-check.sh

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

- Date: 2026-03-21
  Title: Keep HLS remux failure memory across later playlist success
  Summary:
    - Tester logs showed the same playlist host retrying ffmpeg remux on later tunes even after prior `ffmpeg_hls_failed` outcomes, which meant the original Go-relay preference memory was not surviving long enough to help.
    - Fixed that by separating HLS remux-failure memory from generic upstream-success clearing, so a later successful playlist fetch no longer erases the “prefer Go relay for this host” signal before the next tune.
    - Extended HLS regressions to prove remux penalties survive generic playlist success and only clear on actual remux success.
    - Added a separate non-transcode ffmpeg-remux first-byte timeout so dead remux attempts fail fast before Plex gives up; covered with a fake-ffmpeg integration test that sleeps without writing bytes.
  Verification:
    - `go test -count=1 ./internal/tuner -run 'Test(Gateway_shouldPreferGoRelayForHLSRemux|Gateway_relaySuccessfulHLSUpstream_crossHostPlaylistPrefersGoBeforeFFmpegFailure|Gateway_relayHLSWithFFmpeg_nonTranscodeFirstBytesTimeout|Gateway_fetchAndRewritePlaylist_retriesConcurrencyLimit|Gateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry)'`
    - `./scripts/verify`

- Date: 2026-03-21
  Title: Isolate and harden the cross-host HLS remux path
  Summary:
    - Isolated one plausible `ffplay direct works, Tunerr remux fails` path to non-transcode ffmpeg remux on HLS manifests whose media/key/map/variant URLs live on a different host than the playlist itself.
    - Added manifest-shape regression coverage plus an end-to-end localhost-vs-127.0.0.1 integration test that proves Tunerr now prefers the Go relay before any bad ffmpeg subrequest can be made against the wrong host context.
    - Changed the non-transcode HLS decision path so cross-host manifests skip ffmpeg remux by default and go straight to the Go relay, with `IPTV_TUNERR_HLS_RELAY_ALLOW_FFMPEG_CROSS_HOST` as an explicit opt-out.
    - Followed up on tester `403` segment logs by making HLS playlist/segment subrequests inherit fallback `Referer` and `Origin` from the current playlist URL when the client did not provide them; the same end-to-end test now models a CDN that rejects segment fetches without playlist context.
  Verification:
    - `go test -count=1 ./internal/tuner -run 'Test(HLSPlaylistCrossHostRefs|Gateway_relaySuccessfulHLSUpstream_crossHostPlaylistPrefersGoBeforeFFmpegFailure|Gateway_shouldPreferGoRelayForHLSRemux|Gateway_fetchAndRewritePlaylist_retriesConcurrencyLimit|Gateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry)'`
    - `./scripts/verify`

- Date: 2026-03-22
  Title: Make run startup visible and diagnose slow catalog phases
  Summary:
    - Changed `run` startup so the tuner listener comes up before long catalog and guide warm-up work, allowing `/healthz` and `/readyz` to report `loading` / `not_ready` instead of the process appearing dead while indexing.
    - Moved XMLTV startup refresh off the listener critical path; `/guide.xml` already serves placeholder content until the merged guide cache is ready.
    - Added timing logs for catalog phases and for `IndexFromPlayerAPI` substeps (`resolveStreamBaseURL`, `fetchLiveStreams`, `fetchVODStreams`, `fetchSeries`) so provider-specific startup stalls identify the exact slow phase.
  Verification:
    - `go test ./cmd/iptv-tunerr ./internal/indexer ./internal/tuner`
    - `go build ./cmd/iptv-tunerr`
    - manual `go run ./cmd/iptv-tunerr run` repro confirming `:5004` binds and `/readyz` returns `503 not_ready` during startup

- Date: 2026-03-21
  Title: Add a dedicated multi-stream contention harness
  Summary:
    - Added `scripts/multi-stream-harness.sh` to run 2+ staggered live pulls against a real Tunerr instance and capture per-stream curl/body artifacts plus periodic `/provider/profile.json`, `/debug/stream-attempts.json`, and `/debug/runtime.json` snapshots.
    - Added `scripts/multi-stream-harness-report.py` to summarize sustained reads, premature exits, zero-byte opens, recent final statuses, and provider concurrency signals into `report.txt` / `report.json`.
    - Updated the troubleshooting runbook, features matrix, changelog, and `memory-bank/commands.yml` so the new harness is discoverable for “second stream starts, first dies” investigations.
  Verification:
    - `bash -n scripts/multi-stream-harness.sh`
    - `python3 -m py_compile scripts/multi-stream-harness-report.py`
    - `python3 scripts/multi-stream-harness-report.py --dir <synthetic-run-dir> --print`

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

---

- Date: 2026-03-19
  Title: Finish Cloudflare handoff and multi-provider credential rolling
  Summary:
    - Evaluated the public `rkdavies/iptvtunerr` fork state and confirmed the remaining Cloudflare/credential work was not fully represented there yet.
    - Added per-stream auth metadata to live channels so fallback URLs can keep the correct provider credentials after M3U enrichment, duplicate-channel merging, and host filtering.
    - Updated the gateway and ffmpeg HLS relay input-header generation to select auth by stream URL and forward shared cookie-jar cookies for the effective playlist URL.
    - Added regression tests covering auth-preserving dedupe/strip, multi-provider auth assignment, gateway per-stream auth usage, and ffmpeg cookie forwarding.
    - Updated the changelog and recurring-loops notes so future Cloudflare/fallback work does not regress back to global-credential assumptions.
  Verification:
    - `go test ./internal/tuner`
    - `go test ./cmd/iptv-tunerr`
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-19
  Title: Validate real providers and fix direct-fallback failover gaps
  Summary:
    - Tested against the real two-provider `.env` setup without exposing credentials and proved both provider accounts answer direct `player_api` auth plus `get_live_streams` successfully.
    - Fixed `iptv-tunerr probe` so it now includes numbered provider entries (`_2`, `_3`, …) instead of silently inspecting only the primary provider URL.
    - Fixed the no-ranked direct `player_api` fallback so successful direct indexing still attaches multi-provider backup URLs plus per-stream auth rules in the real provider path.
    - Fixed HLS gateway failover so `.m3u8` responses that are HTML/empty count as `invalid-hls-playlist` and fall through to the next backup URL instead of stalling on a bogus `200`.
    - Fixed `safeurl.RedactURL` to redact Xtream path-embedded credentials in logged URLs.
    - Revalidated the real provider flow: the app now tries backup URL 2 after rejecting provider-2 HTML and returns a clean `502` when provider-1 answers `513`, which is an honest upstream failure instead of an app-side stall.
  Verification:
    - `go test ./internal/safeurl ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
    - Real-provider `go run ./cmd/iptv-tunerr probe` with local `.env`
    - Real-provider `go run ./cmd/iptv-tunerr run -skip-health` with `IPTV_TUNERR_BLOCK_CF_PROVIDERS=false IPTV_TUNERR_LIVE_ONLY=true`

---

- Date: 2026-03-19
  Title: Fix Cloudflare-200 JSON probe false negatives
  Summary:
    - Root-caused the remaining provider-probe mismatch to `ProbePlayerAPI` consuming the first chunk of `Server: cloudflare` `200 application/json` responses before attempting JSON decode.
    - Reworked `ProbePlayerAPI` to inspect a preview from the full buffered body, then unmarshal the same body for Xtream auth-shape detection.
    - Added regression coverage for a Cloudflare-served `200 application/json` Xtream auth response.
    - Revalidated against the real local providers: `iptv-tunerr probe` now reports both providers as `player_api ok HTTP 200`.
  Verification:
    - `go test ./internal/provider ./cmd/iptv-tunerr`
    - `./scripts/verify`
    - Real-provider `go run ./cmd/iptv-tunerr probe` with local `.env`

---

- Date: 2026-03-19
  Title: Retry lower-ranked providers and finish release-confidence smoke
  Summary:
    - Release-confidence smoke exposed one more ranked-path bug: once `probe` correctly marked both providers `OK`, `fetchCatalog` could still fail if the top-ranked provider authenticated successfully but could not complete live indexing.
    - Fixed the ranked index path to try the next ranked provider before giving up, while still preserving the full ranked backup/auth set on the resulting live channels.
    - Revalidated with a fresh real-provider `run -skip-health`: the server now boots cleanly on the ranked path, keeps `51641` channels with backups, and sampled channel failures are now clearly upstream (`invalid-hls-playlist`, `513`, timeout) rather than app-side routing failures.
  Verification:
    - `go test ./cmd/iptv-tunerr ./internal/tuner`
    - Real-provider `go run ./cmd/iptv-tunerr run -skip-health` on loopback with local `.env`

---

- Date: 2026-03-19
  Title: Fix post-release audit gaps in probe and get.php fallback
  Summary:
    - Fixed the remaining multi-provider gap in the `get.php` fallback path so successful fallback feeds are merged and deduped instead of collapsing to the first provider only.
    - Fixed `probe` logging to redact numbered-provider credentials via `safeurl.RedactURL` instead of only masking the primary provider password.
    - Fixed `probe` ranking output to respect `IPTV_TUNERR_BLOCK_CF_PROVIDERS`, aligning operator-facing ranking with runtime ingest policy.
    - Added regression coverage for merged multi-provider `get.php` fallback behavior.
  Verification:
    - `go test ./cmd/iptv-tunerr ./internal/provider ./internal/safeurl ./internal/tuner`
    - `./scripts/verify`

---

- Date: 2026-03-19
  Title: Add direct-vs-Tunerr stream comparison harness
  Summary:
    - Added `scripts/stream-compare-harness.sh` to run direct upstream and Tunerr stream URLs side by side with `curl`, `ffprobe`, `ffplay`, and optional `tcpdump` capture in one output bundle.
    - Added `scripts/stream-compare-report.py` to summarize the resulting artifacts into a quick text/JSON diff so operators can see status, stream-shape, and playback mismatches without manually opening every file first.
    - Added `/debug/stream-attempts.json` so Tunerr now exports recent structured gateway decisions, including per-upstream outcomes, effective URLs, and redacted request/ffmpeg header summaries.
    - Wired the harness to fetch that debug export automatically when the Tunerr target has a resolvable base URL.
    - Documented the new workflow in the troubleshooting runbook and added the helper command to `memory-bank/commands.yml`.
    - Documented the recurring local-test pitfall where repo-root `.env` auto-loading contaminates synthetic harness runs unless the process is launched from a clean working directory.
  Verification:
    - `bash -n scripts/stream-compare-harness.sh`
    - `python3 -m py_compile scripts/stream-compare-report.py`
    - `go test -count=1 ./cmd/iptv-tunerr ./internal/tuner`
    - Clean-cwd local smoke:
      `DIRECT_URL=http://127.0.0.1:18086/playlist.m3u8 TUNERR_BASE_URL=http://127.0.0.1:5522 CHANNEL_ID=diag RUN_SECONDS=3 USE_TCPDUMP=false ./scripts/stream-compare-harness.sh`
    - `./scripts/verify`

---

- Date: 2026-03-19
  Title: Add catch-up recorder daemon MVP
  Summary:
    - Added a new recorder workstream to `memory-bank/work_breakdown.md` (`REC-001`..`REC-003`) and implemented `REC-001` as the first real vertical slice.
    - Added `iptv-tunerr catchup-daemon`, which continuously scans guide-derived capsules, schedules eligible `in_progress` / `starting_soon` items, enforces max-concurrency, and persists `active` / `completed` / `failed` state to JSON.
    - Added reusable recording helpers so both one-shot `catchup-record` and the new daemon share the same capsule-to-TS fetch/write path.
    - Extended the daemon with optional publish layout generation for completed recordings (`.ts` + `.nfo` plus `recorded-publish-manifest.json`) and automatic expiry/retention pruning.
    - Tightened legitimate ffmpeg/HLS parity for tricky CDN cases by propagating effective UA/referer/cookies more faithfully and enabling persistent/multi-request HTTP input by default.
    - Updated features/reference/changelog/docs and command references to describe the new recorder daemon MVP honestly.
  Verification:
    - `go test -run 'TestRunCatchupRecorderDaemon' -v ./internal/tuner`
    - `go test -count=1 ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`

---

- Date: 2026-03-19
  Title: Wire catch-up daemon publishing into media-server registration
  Summary:
    - Extended `catchup-daemon` so publish-time completion events can reuse the existing `catchup-publish` library-registration workflow instead of duplicating Plex/Emby/Jellyfin automation.
    - Added `catchup-daemon` flags for `-library-prefix`, `-register-plex`, `-register-emby`, `-register-jellyfin`, and `-refresh`, with the same access/env fallbacks as the one-shot publisher.
    - Added a recorded-item manifest bridge so each completed recording can trigger targeted lane-library create/reuse and refresh behavior as it lands under `-publish-dir`.
    - Added regression coverage for the recorded-item manifest bridge and for daemon publish hooks that fail after publication.
    - Updated CLI/docs/changelog entries to reflect publish-time library automation and the legitimate ffmpeg HLS parity improvements.
  Verification:
    - `go test -count=1 ./internal/tuner ./cmd/iptv-tunerr`

---

- Date: 2026-03-19
  Title: Refine catch-up daemon policy filters and duplicate suppression
  Summary:
    - Extended `catchup-daemon` with `-channels` and `-exclude-channels`, matching exact `channel_id`, `guide_number`, `dna_id`, or `channel_name` so recorder policy can target specific services instead of only lane-level buckets.
    - Added persistent programme-level duplicate suppression using the same curated key shape as capsule dedupe (`dna_id` or channel fallback + start + normalized title), so duplicate provider variants do not both record if they appear as separate capsules.
    - Persisted the new record key in recorder state for debuggability and future policy work.
    - Updated docs/changelog/features to describe the broader recorder-policy surface honestly.
  Verification:
    - `go test -count=1 ./internal/tuner ./cmd/iptv-tunerr`

---

- Date: 2026-03-19
  Title: Add recorder status reporting surfaces
  Summary:
    - Added a shared recorder-report loader over the persistent daemon state file so recorder status can be summarized consistently without embedding daemon logic into the server or CLI layers.
    - Added `iptv-tunerr catchup-recorder-report` to inspect recorder state from disk, including aggregate stats, per-lane counts, published totals, and recent active/completed/failed items.
    - Added `/recordings/recorder.json` so a running tuner can expose the same recorder summary when `IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE` is configured.
    - Added tests for the shared report loader and the new server endpoint, and updated docs/changelog/features accordingly.
  Verification:
    - `go test -count=1 ./internal/tuner ./cmd/iptv-tunerr`

---

- Date: 2026-03-19
  Title: Add lane-specific recorder retention and storage budgets
  Summary:
    - Extended `catchup-daemon` with global retention flags (`-retain-completed`, `-retain-failed`) plus per-lane retention counts (`-retain-completed-per-lane`, `-retain-failed-per-lane`) and per-lane completed-item storage budgets (`-budget-bytes-per-lane`).
    - Implemented newer-first per-lane pruning for completed and failed items before the global retention caps are applied, using `BytesRecorded` or on-disk file sizes for completed-item byte budgeting.
    - Fixed a subtle state bug where items removed by expiry or retention pruning could leave stale duplicate-tracking keys behind and block future rerecords of the same programme identity.
    - Added parser tests for the new CLI limit formats and recorder-state tests covering per-lane retention, per-lane byte budgets, and rerecord-after-prune behavior.
  Verification:
    - `go test -count=1 ./internal/tuner ./cmd/iptv-tunerr`

---

- Date: 2026-03-19
  Title: Improve recorder restart recovery semantics
  Summary:
    - Extended daemon restart handling so unfinished active items are preserved as explicit failed `status=interrupted` records with `recovery_reason=daemon_restart`, `recovered_at`, and partial byte counts when output data already exists.
    - Added automatic retry of interrupted recordings when the same programme window is still eligible after restart, carrying the attempt counter forward instead of silently restarting from attempt `1`.
    - Extended the recorder report surface with `interrupted_count` so operators can see restart-damaged recordings without grepping raw state JSON.
    - Added regression tests for interrupted partial-recording annotation and retry-within-window behavior, then updated docs/changelog/features to describe the restart semantics honestly.
  Verification:
    - `go test -count=1 ./internal/tuner ./cmd/iptv-tunerr`

---

- Date: 2026-03-19
  Title: Ship dedicated integrated web UI on port 48879
  Summary:
    - Replaced the unfinished `internal/webui` placeholder with a real operator dashboard served on port `48879` (`0xBEEF`) by default, using a single-origin `/api/*` reverse proxy over the main tuner server.
    - Pushed the dashboard beyond a flat dev panel into a structured control-plane UI with explicit mode navigation (overview / guide / routing / ops / settings), clearer hierarchy, richer cards, quick-route affordances, endpoint indexing, and modal raw-payload drill-down.
    - Added a new read-only runtime/settings surface at `/debug/runtime.json` so the dashboard can show effective tuner, guide, provider, recorder, HDHR, media-server, and web UI configuration without exposing secrets.
    - Wired the dedicated dashboard into `serve` and `run`, added `IPTV_TUNERR_WEBUI_DISABLED`, `IPTV_TUNERR_WEBUI_PORT`, and `IPTV_TUNERR_WEBUI_ALLOW_LAN`, and kept the older `/ui/` pages on the tuner port for backward compatibility.
    - Updated README, feature/changelog/env/CLI reference docs, plus repo navigation entries for the new operator surface.
  Verification:
    - `go test ./internal/webui ./internal/config ./cmd/iptv-tunerr ./internal/tuner -run 'TestProxyBase|TestProxyForwardsAPIPath|TestServer_runtimeSnapshot|TestServer_operatorGuidePreviewJSON|TestServer_epgStoreReport_disabled|TestServer_epgStoreReport_fileStatsAndVacuumFlag|TestWebUIConfig'`
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Turn the web UI into an actionable operator control surface
  Summary:
    - Added safe operator actions on the tuner side for manual guide refresh, recent stream-attempt buffer clearing, provider-profile penalty reset, and Autopilot memory reset, plus status/workflow JSON surfaces for the deck.
    - Extended XMLTV refresh tracking so the UI can show real guide-refresh state instead of blind buttons, and kept the older synchronous `refresh()` helper as a compatibility wrapper for existing tests.
    - Reworked the integrated dashboard with an action dock, playbook/workflow modals, inline action feedback, and embedded action buttons inside guide/routing/ops cards so the UX reads like a control plane rather than a passive report wall.
    - Added regression coverage for the new operator endpoints and the HLS playlist public-base rewrite helper.
  Verification:
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Add ops-recovery workflow and close out HLS mux docs/runtime surfacing
  Summary:
    - Added a third operator workflow (`ops-recovery`) that summarizes recorder, ghost-hunter, and Autopilot state into a guided recovery playbook instead of leaving operations as disconnected cards.
    - Reworked the deck with a visual signal board and stronger ops affordances so the surface leads with operator judgment and lane health, not only text summaries.
    - Folded in the previously dirty HLS mux follow-up work: `?mux=hls` docs/how-to, `IPTV_TUNERR_STREAM_PUBLIC_BASE_URL` runtime snapshot exposure, README/reference/features/index updates, and regression coverage for the HLS playlist path.
  Verification:
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Add session-local telemetry trends to the control deck
  Summary:
    - Added browser-session telemetry sampling in the integrated web UI so the deck keeps a rolling memory of key signals instead of re-rendering each fetch as an isolated snapshot.
    - Introduced trend cards and lightweight sparkline visuals for guide confidence, stream stability, recorder yield, and ops cleanliness, making the page read more like an active control room than a static report.
    - Kept telemetry capture tied to the fetch/reload path rather than the render path so searches, mode changes, and modal interactions do not distort the sampled history.
  Verification:
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Persist deck memory and add optional HLS mux browser CORS
  Summary:
    - Added browser-local persistence for deck mode, refresh cadence, selected raw endpoint, and recent telemetry samples so the dashboard behaves like a sticky operator cockpit instead of a disposable page.
    - Added an explicit “Clear Deck Memory” control and adjusted the trend/history copy to reflect persisted browser-local history rather than per-tab ephemeral state.
    - Folded in the pending HLS mux browser support slice: `IPTV_TUNERR_HLS_MUX_CORS`, `OPTIONS` preflight handling, CORS headers on `?mux=hls` playlist/segment responses, runtime snapshot exposure, and regression tests.
  Verification:
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Add shared deck telemetry memory and tighten HLS mux controls
  Summary:
    - Added a server-backed `/deck/telemetry.json` endpoint in the dedicated web UI so trend cards can use shared in-process operator memory instead of only per-browser local storage.
    - Switched the deck trend/history surfaces to prefer shared web UI memory while keeping personal UI preferences local to the browser, making the page behave more like a shared cockpit.
    - Tightened the HLS mux path with explicit segment-proxy concurrency limits and 304/conditional-fetch handling, while keeping the browser-facing CORS/preflight support for `?mux=hls`.
  Verification:
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Add deck auth defaults and persisted operator memory
  Summary:
    - Added HTTP Basic auth on the dedicated `internal/webui` origin, defaulting to `admin` / `admin` unless `IPTV_TUNERR_WEBUI_USER` / `IPTV_TUNERR_WEBUI_PASS` override it.
    - Added optional `IPTV_TUNERR_WEBUI_STATE_FILE` persistence so shared deck telemetry/history survives dedicated web UI restarts instead of disappearing with process memory.
    - Surfaced deck auth and memory posture in `/debug/runtime.json` and in the deck UI itself, while also closing the leftover HLS mux regression coverage for conditional `304` forwarding and segment concurrency caps.
  Verification:
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Replace browser auth prompt with a real deck session flow
  Summary:
    - Added a dedicated `internal/webui/login.html` entry page with operator-facing login UX instead of relying on the browser’s raw Basic-auth prompt as the deck front door.
    - Switched the dedicated deck origin to cookie-backed sessions with explicit logout and session-expiry redirects, while keeping HTTP Basic auth as a compatibility fallback for scriptable clients.
    - Added visible sign-out affordances in the deck and kept the auth story consistent with the persisted/shared deck memory work.
  Verification:
    - `go test ./internal/webui`
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Add safe-root policy for local guide inputs
  Summary:
    - Local XMLTV and alias refs now resolve only within the working directory or explicit `IPTV_TUNERR_GUIDE_INPUT_ROOTS` entries instead of allowing arbitrary file paths.
    - `internal/guideinput` now parses alias/XMLTV content from a single validated load path rather than reopening extra local-file sinks after validation.
    - Docs and env examples were updated so the new guide-input sandbox is discoverable before operators hit it.
  Verification:
    - `go test ./internal/refio ./internal/guideinput ./cmd/iptv-tunerr`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Remove more CodeQL dataflow sinks from refio and deck auth
  Summary:
    - Removed free-form deck post-login redirects and now always land successful deck sessions on `/`, which drops the remaining `next=` redirect surface from the login path.
    - Reduced `internal/refio` to validation-only helpers and moved guide-input file/network reads into `internal/guideinput`, so the generic ref helper no longer performs filesystem or HTTP I/O itself.
    - Replaced the last request-header debug sinks with name-only logging and removed more request-sized preallocation in guide/attempt report helpers.
  Verification:
    - `go test ./internal/refio ./internal/guideinput ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Burn down high-risk CodeQL findings on ref loading, limits, logs, and deck redirects
  Summary:
    - Hardened `internal/refio` so local refs must be regular files and remote guide/alias `http(s)` refs reject private or loopback hosts by default unless `IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP=1` is set intentionally for localhost/LAN sources.
    - Clamped oversized `limit=` handling in guide preview/capsule and stream-attempt report paths, reducing uncontrolled preallocation risk on operator/debug surfaces.
    - Reduced raw request/header-derived logging in Plex adaptation and upstream concurrency paths, normalized deck login redirects to path-only targets, aligned logout cookie security flags, and added `X-Content-Type-Options: nosniff` plus default JSON HTML escaping on debug/operator surfaces.
  Verification:
    - `go test ./internal/refio ./internal/guideinput ./internal/webui ./internal/tuner`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Add guide versus lineup match report endpoint
  Summary:
    - Added `/guide/lineup-match.json` to report current lineup count, guide channel count, exact-name coverage, duplicate guide names/numbers, and a sample of lineup rows missing from emitted `guide.xml`.
    - Wired the endpoint into runtime links and tests so operators can validate Plex matchability directly from the tuner instead of manually diffing `lineup.json` and `guide.xml`.
    - This complements `IPTV_TUNERR_EPG_FORCE_LINEUP_MATCH` by making the remaining mismatch surface observable.
  Verification:
    - `go test ./internal/tuner -run 'Test(XMLTV_GuideLineupMatchReport|Server_guideLineupMatch|XMLTV_forceLineupMatchOverridesPrune)'`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Add guide mode that always represents the lineup
  Summary:
    - Added `IPTV_TUNERR_EPG_FORCE_LINEUP_MATCH` so `guide.xml` can keep every lineup channel represented even when `IPTV_TUNERR_EPG_PRUNE_UNLINKED=1` is enabled.
    - Wired the new mode through config, runtime snapshot, XMLTV filtering, docs, and tests so it is an explicit operator choice rather than an implicit side effect.
    - This preserves Plex matchability by letting unmatched channels keep placeholder guide rows instead of disappearing from the guide output.
  Verification:
    - `go test ./internal/config ./internal/tuner -run 'TestEpgPruneUnlinked|TestXMLTV_(epgPruneUnlinked|forceLineupMatchOverridesPrune)'`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Validate live shards and add lineup integrity logging
  Summary:
    - Swept 18 live Tunerr ports and confirmed that the sampled `lineup.json` and `guide.xml` pairs were structurally healthy: zero malformed rows, zero duplicate guide numbers, and exact guide-name matches for every lineup row checked.
    - Added a concise `UpdateChannels` lineup-integrity summary log so future reports show channel count, linked count, stream coverage, missing core fields, and duplicate guide numbers/channel ids at refresh time.
    - Added a unit test for the integrity summarizer and kept the post-regression faster `internal/tuner` suite intact.
  Verification:
    - live validation sweep across ports `5004`, `5006-5013`, `5101-5103`, `5201-5206`
    - `go test ./internal/tuner -run 'TestSummarizeLineupIntegrity|TestServer_(healthz|readyz)'`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Restore first-run channel mapping after guide-input hardening
  Summary:
    - Fixed the post-hardening regression where runtime EPG repair and guide-health flows could no longer fetch provider-derived XMLTV on first run unless matching env vars were already populated.
    - Added explicit trusted-ref plumbing so internal callers can allow their exact runtime provider/XMLTV/alias URLs without reopening generic remote guide fetches.
    - Reduced the heaviest tuner test by overriding HLS relay timeout/sleep hooks inside the test, cutting the `internal/tuner` package from roughly 13.6s to 1.6s in local verification.
  Verification:
    - `go test ./cmd/iptv-tunerr -run 'TestApplyRuntimeEPGRepairs_(ExternalRepairsIncorrectTVGID|PrefersProviderBeforeExternal)|TestChannelDNAStableAfterRuntimeEPGRepair'`
    - `time go test ./internal/tuner -run '^TestGateway_stream_rewritesHLSRelativeURLs$' -count=1`
    - `go test ./internal/tuner`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Constrain remote guide inputs to configured hosts
  Summary:
    - Tightened `internal/guideinput` so remote XMLTV and alias fetches only target exact URLs already declared in provider, XMLTV, or HDHomeRun guide config, with optional explicit additions via `IPTV_TUNERR_GUIDE_INPUT_ALLOWED_URLS`.
    - Updated guide-input tests and runtime EPG-repair tests so the hardened host-allowlist path is exercised intentionally instead of depending on ambient test URLs.
    - Documented the new env in the README, `.env.example`, cli/env reference, and changelog as part of the ongoing CodeQL burn-down.
  Verification:
    - `./scripts/verify`

---

- Date: 2026-03-21
  Title: Make startup guide placeholders visibly provisional
  Summary:
    - Updated the placeholder `/guide.xml` path so XMLTV source metadata explicitly says it is a guide-loading placeholder instead of looking like a normal guide feed.
    - Placeholder programme titles now include `(guide loading)` and carry a short description explaining that IPTV Tunerr is still building the full guide.
    - Added XMLTV tests so the placeholder labeling remains visible on future startup-guide changes.
  Verification:
    - `go test ./internal/tuner -run 'TestXMLTV_serve'`
    - `./scripts/verify`

---

- Date: 2026-03-21
  Title: Add standardized evidence intake path for tester cases
  Summary:
    - Added `scripts/evidence-intake.sh` to scaffold or populate `.diag/evidence/<case-id>/` with a consistent layout for debug-bundle output, PMS logs, Tunerr logs, pcap captures, and analyst notes.
    - Documented the workflow in `docs/how-to/evidence-intake.md`, indexed it from the docs entrypoints, and added `planning/README.md` so future plans point at the evidence bundle instead of mixing raw captures into planning files.
    - Added an `evidence_intake` helper to `memory-bank/commands.yml` so the intake path is discoverable alongside the existing harness/debug commands.
  Verification:
    - `bash -n scripts/evidence-intake.sh`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Fix guide.xml startup race before lineup load
  Summary:
    - Stopped XMLTV startup refresh from caching an empty guide when zero lineup channels are loaded, preserving placeholder behavior instead of serving an 82-byte empty `<tv>` document for the full cache TTL.
    - Taught `UpdateChannels` to trigger a follow-up XMLTV refresh when the lineup arrives so `guide.xml` populates immediately after the catalog load rather than waiting for the next ticker.
    - Added tuner tests that prove the no-lineup refresh preserves an empty cache and that `UpdateChannels` refreshes XMLTV once guideable channels exist.
  Verification:
    - `go test ./internal/tuner -run 'Test(XMLTV_runRefresh_noChannelsPreservesEmptyCache|Server_UpdateChannelsTriggersXMLTVRefresh|XMLTV_GuideLineupMatchReport|Server_guideLineupMatch)'`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Enrich guide-lineup mismatch samples with TVG identifiers
  Summary:
    - Added `channel_id` and observed `tvg_id` to `/guide/lineup-match.json` sample rows so mismatch reports show the lineup record and upstream guide-link state together.
    - Kept the endpoint diagnostic-only: it exposes real observed linkage metadata instead of synthesizing `tvg_id` from `guide_number`.
    - Updated the CLI/env reference and tuner tests to lock the richer payload shape in place.
  Verification:
    - `go test ./internal/tuner -run 'Test(XMLTV_GuideLineupMatchReport|Server_guideLineupMatch)'`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Add shared operator activity memory to the deck
  Summary:
    - Added `/deck/activity.json` on the dedicated web UI server and persisted it alongside deck telemetry so operator activity survives reloads and optional deck restarts.
    - Recorded login/logout, memory clears, and deck-triggered action outcomes as shared operator activity instead of leaving those events trapped in browser state.
    - Surfaced recent activity directly in the overview and operations lanes so the deck now shows operator behavior and not only backend condition snapshots.
  Verification:
    - `go test ./internal/webui`
    - `go test ./...`
    - `./scripts/verify`

---

- Date: 2026-03-20
  Title: Harden deck mutations and expand the settings control surface
  Summary:
    - Added session-bound CSRF protection for state-changing requests on the dedicated deck origin, including deck memory/activity/settings writes and proxied operator actions, while keeping HTTP Basic auth usable for script clients.
    - Switched sign-out to a deliberate POST flow and exposed the CSRF header/runtime posture in the deck snapshot so auth/session behavior is operator-visible instead of implicit.
    - Expanded the Settings lane into a fuller control surface with grouped endpoint inventory, richer runtime/config posture cards, and a clearer atlas of actions, workflows, persistence, and security state.
  Verification:
    - `node --check internal/webui/deck.js`
    - `GOFLAGS=-mod=mod go test ./internal/webui`
    - `GOFLAGS=-mod=mod go test ./cmd/iptv-tunerr ./internal/tuner`

---

- Date: 2026-03-19
  Title: Teach the stream compare harness to capture reusable HLS and DASH samples
  Summary:
    - Extended `scripts/stream-compare-harness.sh` so each curl artifact can also emit `manifest.json` when the body looks like HLS or DASH, instead of leaving operators with only raw body previews and packet captures.
    - Added `scripts/stream-compare-manifest.py` to inventory URI-bearing references, decode Tunerr `?mux=...&seg=` targets into redacted upstream URLs, and turn failing provider manifests into reusable analysis artifacts.
    - Updated `scripts/stream-compare-report.py`, the troubleshooting runbook, and `memory-bank/commands.yml` so the new manifest capture path is visible in summaries and operator docs.
  Verification:
    - `bash -n scripts/stream-compare-harness.sh`
    - `python3 -m py_compile scripts/stream-compare-report.py scripts/stream-compare-manifest.py`
    - `python3 scripts/stream-compare-manifest.py --body <synthetic.m3u8> --meta <meta.json> --curl-meta <curl.meta.json> --out <manifest.json>`
    - `python3 scripts/stream-compare-manifest.py --body <synthetic.mpd> --meta <meta.json> --curl-meta <curl.meta.json> --out <manifest.json>`
    - `python3 scripts/stream-compare-report.py --dir <synthetic-run-dir>`
- 2026-03-21: Added account-aware provider pooling and VOD WebDAV mount helpers. Gateway now derives provider-account identities from per-stream auth rules, prefers less-loaded accounts during stream ordering, keeps active leases for successful live sessions, and can enforce `IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT` with local `805` / `503` rejection when every account for a deduplicated channel is busy. Also added `iptv-tunerr vod-webdav-mount-hint` plus concrete startup mount commands for `vod-webdav`. Verification: `go test ./internal/tuner ./internal/vodwebdav ./cmd/iptv-tunerr -count=1`, `./scripts/verify`. Opportunity filed: adaptive per-account contract learning.
- 2026-03-21: Added stronger release gating for new provider-pool and VOD parity features. `scripts/ci-smoke.sh` now asserts `vod-webdav-mount-hint` output for macOS/Windows and runs a real WebDAV `PROPFIND` smoke against `iptv-tunerr vod-webdav`; gateway/provider-profile tests now cover provider-account lease release and account-pool state reporting. Also mapped the tester’s channel-builder request into `docs/epics/EPIC-programming-manager.md`. Verification: `go test ./internal/tuner ./internal/vodwebdav ./cmd/iptv-tunerr -count=1`, `./scripts/verify`.
- 2026-03-21: Landed adaptive provider-account learning and the first Programming Manager backend slice. Gateway now learns tighter per-account concurrency caps from upstream `423`/`458`/`509`-style limit responses, applies them on later tunes, and exposes them as `account_learned_limits` on `/provider/profile.json`. Added `internal/programming`, `IPTV_TUNERR_PROGRAMMING_RECIPE_FILE`, and the first curation endpoints (`/programming/categories.json`, `/programming/recipe.json`, `/programming/preview.json`) so category inventory and a durable saved recipe now sit between raw lineup intelligence and final exposed channels. Expanded WebDAV/release smoke to cover `OPTIONS`, root `PROPFIND`, `PROPFIND /Movies`, and the new programming endpoints. Verification: `go test ./internal/programming ./internal/tuner ./internal/vodwebdav ./cmd/iptv-tunerr -count=1`, `bash ./scripts/ci-smoke.sh`, `./scripts/verify`. Opportunity updated: learned account-limit persistence/decay still open.
- 2026-03-21: Extended Programming Manager into real mutation APIs and recommended ordering. `/programming/categories.json` now supports bulk include/exclude/remove for category-first curation, `/programming/channels.json` supports exact channel overrides, and `order_mode: "recommended"` now sorts channels into the requested Local/Entertainment/News/Sports/... taxonomy on the server. `programming/preview.json` now includes bucket counts, and binary smoke mutates the recipe over HTTP to prove the exposed lineup changes. Verification: `go test ./internal/programming ./internal/tuner ./cmd/iptv-tunerr -count=1`, `bash ./scripts/ci-smoke.sh`, `./scripts/verify`.
- 2026-03-21: Added durable manual order and exact-match backup grouping to Programming Manager. `/programming/order.json` now supports `prepend` / `append` / `before` / `after` / `remove` order mutations that persist in `IPTV_TUNERR_PROGRAMMING_RECIPE_FILE`, `/programming/backups.json` reports strong same-channel sibling groups (`tvg_id` exact, else `dna_id` exact), and `collapse_exact_backups: true` can collapse those groups into one visible lineup row with merged `stream_urls` that survive refreshes. `scripts/ci-smoke.sh` now exercises the new order/backups flow against a real temporary binary. Verification: `go test ./internal/programming ./internal/tuner -count=1`, `bash ./scripts/ci-smoke.sh`, `./scripts/verify`.
- 2026-03-21: Added the Programming Manager lane to the dedicated control deck (`PM-008`). The web UI now exposes category inventory controls, exact channel include/exclude mutations, manual order nudges from the curated lineup preview, backup-collapse toggles, and exact backup-group inspection without requiring operators to hand-post JSON. Verification: `node --check internal/webui/deck.js`, `go test ./internal/webui ./internal/tuner ./cmd/iptv-tunerr -count=1`, `./scripts/verify`.
- 2026-03-21: Expanded Programming Manager regression coverage (`PM-009`) around refresh and restart survival. Added a tuner test proving saved recipe mutations survive `UpdateChannels` refresh churn and expanded `scripts/ci-smoke.sh` so it restarts `serve` against a reshuffled catalog while reusing the same recipe file, then reasserts curated lineup shape, custom order, and exact-backup collapse state. Verification: `go test ./internal/tuner -run 'TestServer_(ProgrammingMutationsSurviveRefresh|UpdateChannelsPreservesProgrammingCustomOrderAndCollapse|programmingEndpoints)' -count=1`, `./scripts/verify`.
- 2026-03-21: Persisted adaptive provider-account limits and broadened live WebDAV validation. Added a TTL-backed provider-account learned-limit store so `account_learned_limits` survive restarts via `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE` / `IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS`, restored that state into the gateway at startup, exposed the persistence knobs on `/provider/profile.json` and `/debug/runtime.json`, and made provider-profile reset clear the persisted state too. Expanded WebDAV coverage from directory discovery into real file-read client shapes: unit tests now cover file `HEAD` and byte-range `GET`, and `scripts/ci-smoke.sh` now stands up a local HTTP asset server so `iptv-tunerr vod-webdav` is exercised through the real cache/materializer path. Verification: `go test ./internal/tuner ./internal/vodwebdav ./cmd/iptv-tunerr -run 'Test(AccountLimitStorePersistsAndPrunesExpired|ProviderBehaviorProfile_includesLearnedAccountLimits|Handler_OPTIONSAndPROPFIND_ClientShapes)' -count=1`, `bash ./scripts/ci-smoke.sh`, `./scripts/verify`.
- 2026-03-21: Hardened the read-only WebDAV contract for non-Linux VOD parity. `internal/vodwebdav` now explicitly allows only `OPTIONS`, `PROPFIND`, `HEAD`, and `GET`, returning stable `405` responses with `Allow`/`DAV` headers for mutation methods instead of leaking lower-layer write errors. Validation depth increased again: unit tests now cover file-level `PROPFIND`, episode `HEAD`, episode range reads, and clean `PUT` rejection, while `scripts/ci-smoke.sh` now asserts both movie and episode file reads plus read-only `PUT` rejection against a real temp `vod-webdav` binary. Verification: `go test ./internal/vodwebdav -count=1`, `./scripts/verify`.
- 2026-03-21: Added a dedicated non-Linux VOD client-matrix harness. `scripts/vod-webdav-client-harness.sh` now replays a Finder/WebDAVFS + Windows MiniRedir style request matrix against either a self-contained temp `vod-webdav` binary or an operator-supplied `BASE_URL`, and `scripts/vod-webdav-client-report.py` summarizes the bundle. Added `docs/how-to/vod-webdav-client-harness.md`, wired it into the docs index and `memory-bank/commands.yml`, and verified the harness locally with a passing bundle under `.diag/vod-webdav-client/`. Verification: `bash -n scripts/vod-webdav-client-harness.sh`, `python3 -m py_compile scripts/vod-webdav-client-report.py`, `./scripts/vod-webdav-client-harness.sh`, `./scripts/verify`.
- 2026-03-21: Added a baseline-vs-host diff tool for the WebDAV client harness. `scripts/vod-webdav-client-diff.py` now compares two harness bundles step-by-step so a local known-good run can be diffed directly against a real macOS or Windows host run. Updated `docs/how-to/vod-webdav-client-harness.md` with the baseline -> real-host -> diff workflow and added `vod_webdav_client_diff` to `memory-bank/commands.yml`. Verification: `python3 -m py_compile scripts/vod-webdav-client-diff.py`, `python3 scripts/vod-webdav-client-diff.py --left <bundle> --right <same-bundle> --print`, `./scripts/verify`.
- 2026-03-21: Shipped feature-parity foundation (`PAR-001`) with event webhooks.
  - Added `docs/epics/EPIC-feature-parity.md` to turn the broad ecosystem gap audit into a real multi-PR parity track.
  - Added `internal/eventhooks` with file-backed webhook config, async JSON delivery, wildcard/event filtering, and recent-delivery reporting.
  - Wired `IPTV_TUNERR_EVENT_WEBHOOKS_FILE` through runtime config, `tuner.Server`, and `Gateway`.
  - Emitted `lineup.updated`, `stream.requested`, `stream.rejected`, and `stream.finished` lifecycle events.
  - Added `/debug/event-hooks.json` and runtime snapshot exposure for current webhook state.
  - Updated changelog/features/CLI reference plus memory-bank planning files.
  - Verification: `go test ./internal/eventhooks ./internal/tuner ./cmd/iptv-tunerr -run 'Test(LoadAndDispatch|HookMatchesWildcard|Server_UpdateChannelsEmitsLineupEvent|Server_EventHooksReport|Gateway_streamRejectEmitsWebhookEvent)' -count=1`; `./scripts/verify`.
  - Opportunities filed: follow-on parity slices for stream fanout, DVR rules/history, Xtream-compatible downstream output, multi-user entitlements, virtual channels, and richer active-stream analytics/control.
- 2026-03-21: Fixed tester-reported multi-account rollover failure and added the first active-stream parity surface.
  - Account identity and auth fallback now derive credentials directly from Xtream-style stream paths when `StreamAuths` are missing or incomplete, so provider-account pooling can still spread concurrent sessions across distinct accounts.
  - Added `/debug/active-streams.json` as the first `PAR-007` operator surface for in-flight stream visibility.
  - Verification: `go test ./internal/tuner -run 'Test(Gateway_reorderStreamURLsByAccountLoad_prefersFreeAccount|Gateway_reorderStreamURLsByAccountLoad_prefersFreeXtreamPathAccountWithoutStreamAuths|Gateway_authForURL_fallsBackToXtreamPathCredentials|Gateway_stream_providerAccountLimitRejectsLocally|Gateway_stream_successReleasesProviderAccountLease)' -count=1`; `./scripts/verify`.
- 2026-03-21: Added a read-only downstream Xtream-compatible live output starter (`PAR-004`).
  - Added optional `IPTV_TUNERR_XTREAM_USER` / `IPTV_TUNERR_XTREAM_PASS` gating for a read-only downstream Xtream surface.
  - Added `/player_api.php` support for `get_live_streams` and `get_live_categories`.
  - Added `/live/<user>/<pass>/<channel>.ts` proxying into the existing gateway.
  - Verification: `go test ./internal/tuner ./cmd/iptv-tunerr -run 'Test(Server_(XtreamPlayerAPI_LiveCategories|XtreamLiveProxy|ActiveStreamsReport)|Gateway_stream_rollsAcrossThreeXtreamPathAccounts|Gateway_authForURL_fallsBackToXtreamPathCredentials)' -count=1`.
- 2026-03-21: Expanded downstream Xtream output to VOD and series, and added a focused Programming Manager detail view.
  - `player_api.php` now serves `get_vod_categories`, `get_vod_streams`, `get_series_categories`, `get_series`, and `get_series_info`, and Tunerr-owned `/movie/<user>/<pass>/<id>.mp4` / `/series/<user>/<pass>/<episode>.mp4` paths proxy catalog VOD assets without exposing raw upstream URLs directly.
  - Added `/programming/channel-detail.json` so category-first or curses-style tools can fetch one channel’s category/taxonomy metadata, exact-match backup alternatives, and a 3-hour upcoming-programme preview from the merged guide.
  - Strengthened release gating: `scripts/ci-smoke.sh` now exercises the expanded Xtream VOD/series surface and the new programming channel-detail endpoint against a real temp binary.
  - Added a multi-channel 3-account rollover regression (`TestGateway_stream_twoChannelsPreferDifferentXtreamPathAccounts`) so “second device didn’t rotate credentials” becomes a deterministic test failure.
  - Verification: `go test ./internal/tuner ./cmd/iptv-tunerr -run 'Test(Server_(XtreamPlayerAPI_(LiveCategories|VODAndSeries)|XtreamLiveProxy|XtreamMovieAndSeriesProxy|programmingChannelDetail|programmingEndpoints)|Gateway_(stream_rollsAcrossThreeXtreamPathAccounts|stream_twoChannelsPreferDifferentXtreamPathAccounts))' -count=1`; `./scripts/verify`.
- 2026-03-21: Added the first real multi-user / entitlement slice for downstream Xtream output (`PAR-005`).
  - Added `internal/entitlements` plus `IPTV_TUNERR_XTREAM_USERS_FILE` so Tunerr can load file-backed downstream users with scoped live/VOD/series access.
  - Added `/entitlements.json` for operator visibility and file-backed updates to the current ruleset.
  - `player_api.php` and `/live|movie|series/...` now authenticate either the legacy admin user or a file-backed downstream user, and filter or deny output per user instead of treating the Xtream surface as one global catalog.
  - Strengthened release gating again: `scripts/ci-smoke.sh` now verifies a limited user sees only its allowed live/movie rows and gets `404` on denied live/series playback.
  - Verification: `go test ./internal/entitlements ./internal/tuner ./cmd/iptv-tunerr -run 'Test(LoadSaveAndAuthenticate|Server_(XtreamEntitlementsLimitOutput|XtreamPlayerAPI_(LiveCategories|VODAndSeries)|XtreamMovieAndSeriesProxy|XtreamLiveProxy))' -count=1`; `./scripts/verify`.
- 2026-03-21: Productized the old Plex wizard-oracle flow into a real lineup-harvest feature.
  - Added `internal/plexharvest` with reusable target expansion, bounded channelmap polling, per-target result capture, and deduped lineup summaries.
  - Added `iptv-tunerr plex-lineup-harvest` as the named CLI surface for cap/template sweeps against Plex lineup matching.
  - Added stronger 3-account rollover regression coverage so three simultaneous channels must lease three distinct Xtream-path credential sets.
  - Docs: `docs/epics/EPIC-lineup-harvest.md`, `docs/how-to/plex-lineup-harvest.md`, `docs/reference/cli-and-env-reference.md`, `docs/features.md`, `docs/CHANGELOG.md`, `README.md`.
  - Verification: `go test ./internal/plexharvest ./internal/tuner ./cmd/iptv-tunerr -run 'Test(ExpandTargets_templateAndFriendlyNames|BuildSummary_groupsSuccessfulLineups|Probe_(pollsAndCapturesLineupTitle|recordsErrorsPerTarget)|Gateway_stream_threeChannelsUseThreeXtreamPathAccounts)' -count=1`; `./scripts/verify`.
- 2026-03-21: Bridged saved lineup harvest reports into Programming Manager and restarted virtual channels as a durable server feature.
  - `internal/plexharvest` now supports atomic save/load of persisted report files, and Tunerr can reload those reports from `IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE`.
  - Added `/programming/harvest.json`, embedded harvest state into `/programming/preview.json`, and surfaced harvested lineup candidates in the dedicated deck’s Programming lane so saved harvest runs become usable operator input instead of dead JSON.
  - Added `internal/virtualchannels` with file-backed virtual-channel rules plus preview scheduling over catalog movies and episodes, and exposed `/virtual-channels/rules.json` and `/virtual-channels/preview.json`.
  - Expanded `scripts/ci-smoke.sh` so release gating now verifies both the harvest bridge and virtual-channel preview paths against a real temp binary.
  - Verification: `go test ./internal/plexharvest ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr -count=1`; `bash ./scripts/ci-smoke.sh`; `./scripts/verify`.
- 2026-03-21: Pushed virtual channels past preview-only into a publishable/playable starter.
  - Added current-slot resolution in `internal/virtualchannels`, including resolved source URLs for catalog movies and episodes.
  - Added `/virtual-channels/live.m3u` to export enabled synthetic channel rows and `/virtual-channels/stream/<id>.mp4` to proxy the currently scheduled asset.
  - Expanded tuner tests and `scripts/ci-smoke.sh` so the virtual-channel M3U and stream path are exercised against real asset bytes, not just JSON previews.
  - Verification: `go test ./internal/virtualchannels ./internal/tuner -run 'Test(ResolveCurrentSlot_resolvesCurrentEntryAndSource|Server_virtualChannelRulesAndPreview)' -count=1`; `bash ./scripts/ci-smoke.sh`; `./scripts/verify`.
- 2026-03-21: Made saved lineup harvest results actionable for Programming Manager.
  - Harvest probe results now capture harvested `lineup.json` rows alongside lineup titles/URLs, not just high-level summary counts.
  - Added `/programming/harvest-import.json` so operators can preview or apply a chosen harvested lineup as a real saved recipe.
  - Matching uses strong signals first (`tvg_id`, then normalized guide name) and preserves harvested order as a custom Programming Manager order.
  - Added server tests plus smoke coverage for the harvest-import preview contract.
  - Verification: `go test ./internal/plexharvest ./internal/tuner -run 'Test(Probe_pollsAndCapturesLineupTitle|SaveLoadReportFile_roundTrip|Server_(programmingHarvestEndpoint|programmingPreviewIncludesHarvestSummary|programmingHarvestImport))' -count=1`; `bash ./scripts/ci-smoke.sh`; `./scripts/verify`.
- 2026-03-21: Wired harvest import actions into the dedicated control deck.
  - The Programming lane now offers direct Preview Import and Apply actions for harvested lineup candidates instead of only an inspect button.
  - The deck now calls `/programming/harvest-import.json` directly, so the new backend import flow is operable without hand-posting JSON.
  - Verification: `node --check internal/webui/deck.js`; `./scripts/verify`.
- 2026-03-21: Added smarter harvest matching diagnostics and a real virtual-channel schedule surface.
  - Programming harvest imports now report match-strategy counts and can fall back to a local-broadcast stem heuristic when exact market strings differ across equivalent locals.
  - Added `/virtual-channels/schedule.json` so the synthetic-channel starter now exposes a rolling schedule horizon instead of only static preview JSON plus current-slot playback.
  - Expanded tuner tests and smoke coverage for the new strategy summary and schedule endpoint.
  - Verification: `go test ./internal/virtualchannels ./internal/tuner -run 'Test(BuildSchedule_coversHorizonAcrossLoop|Server_(virtualChannelRulesAndPreview|programmingHarvestImport))' -count=1`; `bash ./scripts/ci-smoke.sh`; `./scripts/verify`.
- 2026-03-21: Productized live Programming Manager preview in the deck.
  - Added a real in-place HLS preview for the selected curated channel using the same Tunerr `/stream/<id>?mux=hls` path that lineup consumers will hit.
  - Surfaced focused channel detail, 3-hour upcoming guide rows, exact alternative sources, and virtual-channel schedule context in the Programming lane instead of forcing raw JSON inspection.
  - Upgraded the old HLS mux demo page with embeddable query params (`base`, `path`, `embed`, `autoplay`) so it can serve as a reusable product surface instead of a standalone experiment.
  - Filed the next productization opportunity for stream-compare / channel-diff / evidence-intake workflows.
  - Verification: `node --check internal/webui/deck.js`; `go test ./internal/webui ./internal/tuner ./cmd/iptv-tunerr -count=1`; `./scripts/verify`.
- 2026-03-21: Turned harvest diagnostics into a local-market assist and deepened virtual-channel publishing.
  - Added `/programming/harvest-assist.json`, which ranks saved harvested lineups as recipe assists using exact `tvg_id`, exact guide name, guide number, and local-broadcast stem hits instead of leaving operators with raw import previews only.
  - Added `/virtual-channels/channel-detail.json` and `/virtual-channels/guide.xml`, pushing virtual channels toward a real publishable TV-like surface rather than only preview/schedule JSON plus current-slot playback.
  - Expanded `scripts/ci-smoke.sh` and tuner tests so the new harvest-assist and virtual detail/guide endpoints are release-gated.
  - Verification: `go test ./internal/tuner -run 'TestServer_(programmingHarvestImport|virtualChannelRulesAndPreview)' -count=1`; `./scripts/verify`.
- Date: 2026-03-21
  Title: Make lineup harvest assists actionable in the Programming lane
  Summary:
    - Turned ranked `/programming/harvest-assist.json` rows into real deck controls instead of text-only summaries.
    - Operators can now preview or apply the top recommended local-market lineup assists directly from the Programming lane using the same harvest-import path as raw saved lineup reports.
  Verification:
    - `node --check internal/webui/deck.js`
    - `go test ./internal/webui ./internal/tuner -run 'TestServer_(programmingEndpoints|programmingBrowse|programmingChannelDetail|diagnosticsHarnessActions)' -count=1`
  Notes:
    - This closes the most obvious deck UX gap for `LH-005`/`LH-006`; harvest results are now visible and operable from the same lane.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/deck.js`
    - `docs/epics/EPIC-lineup-harvest.md`
- Date: 2026-03-21
  Title: Refresh top-level docs for the shipped `v0.1.28` surface
  Summary:
    - Synced `README.md` with the current product story: release-readiness gate, stronger recent-changes list, and explicit macOS/Windows WebDAV host-validation paths.
    - Updated `docs/features.md` to call out the release-readiness gate, macOS bare-metal smoke, Windows smoke package prep, and the WebDAV client-matrix tooling as first-class shipped features.
    - Expanded `docs/index.md`, `docs/how-to/index.md`, and `docs/reference/index.md` so the release-readiness matrix, lineup-harvest guide, and host-smoke docs are discoverable from the top-level doc map instead of buried in changelog/task history.
  Verification:
    - `./scripts/verify`
  Notes:
    - `v0.1.28` was already pushed before this pass; this task was documentation catch-up, not another code/release cut.
  Opportunities filed:
    - none
  Links:
    - `README.md`
    - `docs/features.md`
    - `docs/index.md`
    - `docs/how-to/index.md`
    - `docs/reference/index.md`
- Date: 2026-03-21
  Title: Expand README to reflect the recent parity and programming work
  Summary:
    - Added substantial README sections for Programming Manager, Plex lineup harvest import/assist flows, downstream Xtream publishing, virtual channels, and the current release-proof/testing story.
    - Updated the README contents map so those newer product surfaces are visible from the top instead of buried under older tuner/EPG framing.
    - Kept the existing docs-index sync from the previous pass, but made the README itself carry the product narrative instead of only acting as a link hub.
  Verification:
    - `./scripts/verify`
  Notes:
    - This was the missing “real README update” after the earlier lighter docs-sync pass.
  Opportunities filed:
    - none
  Links:
    - `README.md`
- Date: 2026-03-22
  Title: Fix runtime provider-context drift after scheduled refresh
  Summary:
    - Fixed `run` scheduled refresh so a newly winning provider/account now updates the live server’s provider context instead of leaving `Gateway` and `XMLTV` on stale credentials/base from startup.
    - Added synchronized provider-credential access for `Gateway`, synchronized provider identity access for `XMLTV`, and a `Server.UpdateProviderContext` path used during runtime refresh.
    - Fixed `/debug/runtime.json` to clone the stored runtime snapshot before enriching/serializing it, so requests no longer mutate shared snapshot maps in place.
    - Added tuner regressions proving runtime snapshot serving is non-mutating and provider-context updates propagate into runtime children.
  Verification:
    - `go test -count=1 ./internal/tuner ./cmd/iptv-tunerr`
    - `go test -race -count=1 ./internal/tuner ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This came from the broader post-provider audit, not a user-visible feature request.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_runtime.go`
    - `internal/tuner/server.go`
    - `internal/tuner/gateway_upstream.go`
    - `internal/tuner/xmltv.go`
    - `internal/tuner/server_test.go`
- Date: 2026-03-22
  Title: Audit fix for unstable provider tests and duplicate-base probe credentials
  Summary:
    - Fixed an unstable `cmd/iptv-tunerr` regression test that could crash with `fatal error: concurrent map writes` because the test server updated shared hit counters without synchronization while `fetchCatalog` exercised concurrent provider requests.
    - Fixed `handleProbe` so duplicate provider base URLs no longer overwrite each other via `map[base]entry`; probe now preserves each configured credential set independently, which restores correct probing/ranking for same-host multi-account configs.
    - Added regression coverage proving `probe` uses both credential sets when two providers share one base URL.
  Verification:
    - `go test -count=1 ./cmd/iptv-tunerr`
    - `go test -race -count=1 ./cmd/iptv-tunerr ./internal/provider ./internal/indexer ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This came from a repo-wide audit pass, not a provider feature request.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_core.go`
    - `cmd/iptv-tunerr/main_test.go`
- Date: 2026-03-22
  Title: Fix Docker workflow toolchain drift after Go 1.25 bump
  Summary:
    - Fixed the Docker workflow failure caused by `Dockerfile` still using `golang:1.24-alpine` after `go.mod` moved to `go 1.25.0`.
    - Updated the container build stage to `golang:1.25-alpine`, and aligned the macOS WebDAV harness k8s job image to `golang:1.25-bookworm` so helper jobs do not carry the same stale toolchain assumption.
  Verification:
    - `./scripts/verify`
    - `docker build -t iptvtunerr:ci-fix-test .`
  Notes:
    - The failing GitHub runs were Docker-only; `CI` itself was green. Root error was `go.mod requires go >= 1.25.0 (running go 1.24.13; GOTOOLCHAIN=local)`.
  Opportunities filed:
    - none
  Links:
    - `Dockerfile`
    - `k8s/vod-webdav-client-macair-job.yaml`
- Date: 2026-03-22
  Title: Fix HDHomeRun simulator HTTP status and lineup payload shape
  Summary:
    - Fixed the simulated HDHomeRun TCP control server so `/lineup.json` returns a lineup array and `/lineup_status.json` returns lineup-status metadata instead of both endpoints sharing the same status-shaped payload.
    - Fixed unknown HTTP paths on that surface to return `HTTP/1.1 404 Not Found` instead of a `200 OK` status with a `"404 Not Found"` body.
    - Added focused regression coverage in `internal/hdhomerun/control_test.go`.
  Verification:
    - `go test ./internal/hdhomerun -run 'Test(ControlServer_(getDiscoverJSONEscapesFriendlyName|httpResponseForPath)|CreateDefaultDevice(DefaultFriendlyName|PrefersHDHRFriendlyNameEnv)|ParseDiscoverReply_roundTrip)'`
  Notes:
    - This came from the same ongoing audit of discovery/status siblings, but it is a real protocol-behavior fix rather than a pure serialization cleanup.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/control.go`
    - `internal/hdhomerun/control_test.go`
- Date: 2026-03-22
  Title: Align standalone HDHomeRun default identity
  Summary:
    - Fixed `internal/hdhomerun.CreateDefaultDevice` to default its friendly name to `IPTV Tunerr` instead of `IptvTunerr-HDHR`, bringing the standalone HDHomeRun simulator back in line with the tuner-side discovery surfaces.
    - Added focused regression coverage proving the default and HDHR-specific env precedence in `internal/hdhomerun/discover_test.go`.
  Verification:
    - `go test ./internal/hdhomerun -run 'Test(CreateDefaultDevice(DefaultFriendlyName|PrefersHDHRFriendlyNameEnv)|ControlServer_getDiscoverJSONEscapesFriendlyName|ParseDiscoverReply_roundTrip)'`
  Notes:
    - This was another cross-surface identity-drift fix from the ongoing discovery/status audit.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/discover.go`
    - `internal/hdhomerun/discover_test.go`
- Date: 2026-03-22
  Title: Harden HDHomeRun discovery serialization
  Summary:
    - Fixed the simulated HDHomeRun control server to build `discover.json` with `json.Marshal` instead of `fmt.Sprintf`, so configured friendly names with quotes/backslashes no longer generate invalid JSON.
    - Added focused regression coverage in `internal/hdhomerun/control_test.go`.
  Verification:
    - `go test ./internal/hdhomerun -run 'Test(ControlServer_getDiscoverJSONEscapesFriendlyName|ParseDiscoverReply_roundTrip|FetchDiscoverJSON)'`
  Notes:
    - This was found while continuing the cross-surface discovery identity/serialization audit after the `/device.xml` fixes.
  Opportunities filed:
    - none
  Links:
    - `internal/hdhomerun/control.go`
    - `internal/hdhomerun/control_test.go`
- Date: 2026-03-22
  Title: Align device.xml identity with discover.json
  Summary:
    - Fixed `/device.xml` to honor the configured friendly name and the same env fallback chain as `/discover.json` instead of always advertising `IPTV Tunerr`.
    - Fixed `/device.xml` to honor `IPTV_TUNERR_DEVICE_ID` / `HOSTNAME` fallbacks so its `UDN` stays aligned with the discovery JSON document.
    - Fixed both the main tuner `/device.xml` handler and the generic `internal/probe` discovery helper to XML-escape friendly names and device IDs so special characters no longer generate invalid XML.
    - Added focused regression coverage in `internal/tuner/ssdp_test.go` and `internal/probe/probe_test.go`.
  Verification:
    - `go test ./internal/tuner ./internal/probe -run 'Test(Server_deviceXML(DefaultFriendlyName|UsesEnvFallbacks|EscapesConfiguredIdentity)?|SSDP_searchResponse|JoinDeviceXMLURL|DiscoveryHandler(EscapesXML)?)'`
  Notes:
    - This came from the same broader bug-hunt pass looking for cross-surface identity drift and silent wrong behavior.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/ssdp_test.go`
- Date: 2026-03-22
  Title: Harden guide diagnostics when XMLTV is unavailable
  Summary:
    - Fixed `/guide/health.json`, `/guide/doctor.json`, and `/guide/aliases.json` to return `503` instead of dereferencing a nil `s.xmltv`.
    - Added `TestServer_guideDiagnosticsRequireXMLTV` so the whole guide-diagnostics family stays covered together.
  Verification:
    - `go test ./internal/tuner -run 'TestServer_(guideDiagnosticsRequireXMLTV|epgStoreReport_incrementalFlags|runtimeSnapshot)'`
    - `./scripts/verify`
  Notes:
    - This was found during the broader repo-wide bug hunt on public guide/debug surfaces.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/guide_health.go`
    - `internal/tuner/server_test.go`
- Date: 2026-03-22
  Title: Move the public-feed sample URL out of README/reference literals
  Summary:
    - Removed the hardcoded public-feed sample URL from README and reference examples.
    - Added a commented `.env.example` convenience preset for a custom free-source URL and updated examples to feed `IPTV_TUNERR_FREE_SOURCES` from that shell/env variable instead.
  Verification:
    - `./scripts/verify`
  Notes:
    - This is a docs/env hygiene change only; Tunerr does not read the convenience alias directly.
  Opportunities filed:
    - none
  Links:
    - `.env.example`
    - `README.md`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Fix Xtream export identity collisions and Programming browse smearing
  Summary:
    - Reworked Xtream `xmltv.php` / `get.php` exports to use canonical exported ids based on Tunerr `ChannelID` so sibling variants stop collapsing when a provider reuses `TVGID` or guide numbers.
    - Updated catchup capsule preview generation to emit real lineup `ChannelID` values and duplicate programme capsules per matching lineup row, which fixes Programming browse next-hour titles and Xtream short-EPG/export mapping.
    - Added cached catchup capsule previews per guide snapshot/horizon to avoid redoing the same expensive browse-time XMLTV work on every request.
    - Hardened Xtream VOD proxy parity by adding `HEAD`, `Range`, and downstream range/cache header propagation.
    - Fixed Programming guide-number ordering to sort numerically instead of lexically.
    - Updated binary smoke and focused tests to lock all of the above down.
  Verification:
    - `go test ./internal/tuner -run 'Test(BuildCatchupCapsulePreview_clampsLargeLimit|BuildCatchupCapsulePreview_duplicatesProgrammePerMatchingChannel|Server_programmingBrowse|Server_XtreamMovieAndSeriesProxy|Server_XtreamXMLTVUsesUniqueChannelIDsWhenTVGIDCollides|Server_Xtream(PlayerAPI_LiveCategories|Exports_M3UAndXMLTV))' -count=1`
    - `go test ./internal/programming -run 'Test(CategoryMembers_sortGuideNumbersNumerically|BuildBackupGroupsAndCollapse|BuildBackupGroupsDoesNotCollapseVariantNames|DescribeChannel)' -count=1`
    - `./scripts/verify`
  Notes:
    - This was the direct implementation pass after the audit, not a speculative refactor.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/xmltv.go`
    - `internal/tuner/server_xtream.go`
    - `internal/tuner/server.go`
    - `internal/programming/programming.go`
    - `scripts/ci-smoke.sh`
- Date: 2026-03-22
  Title: Add Plex Live TV reverse-engineering capture, device audit, and DVR cutover tooling
  Summary:
    - Added PMS/plex.tv reverse-engineering commands for DB snapshots, API replay, PMS log mining, per-device reachability auditing, and a real-client browse capture harness.
    - Confirmed from live PMS logs that the current smart-TV `Unavailable` state is caused by dead injected HDHR device URIs, not just Plex Home gating: PMS cannot refresh many `http://plextuner-*.plex.svc:5004` devices and marks them dead.
    - Added `plex-dvr-cutover`, which reads the existing TSV cutover-map shape and dry-runs or applies unsupported PMS-side DVR/device deletion plus re-registration against new URIs.
    - Dry-run validation matched 21 current registered devices/DVRs from a generated cutover TSV, proving the command can target the existing injected fleet without manual SQLite editing.
  Verification:
    - `go test ./internal/plex ./cmd/iptv-tunerr`
    - `go run ./cmd/iptv-tunerr plex-device-audit -plex-url http://127.0.0.1:32400 -token <owner-token>`
    - `go run ./cmd/iptv-tunerr plex-dvr-cutover -plex-url http://127.0.0.1:32400 -token <owner-token> -map /tmp/iptvtunerr-cutover-dryrun.tsv`
  Notes:
    - Candidate `.plex.home` replacement hostnames in this environment still return `404` on `/discover.json`, so the new cutover command is ready but should not be applied until a reachable HDHR surface exists.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_plex_ops.go`
    - `internal/plex/inspect.go`
    - `internal/plex/cutover.go`
    - `internal/plex/logs.go`
    - `scripts/plex-client-browse-capture.sh`
    - `docs/how-to/reverse-engineer-plex-livetv-access.md`
- Date: 2026-03-22
  Title: Stand up a host-local Plex tuner target and harden channel activation against malformed Plex mappings
  Summary:
    - Brought up a self-consistent local `iptv-tunerr run` target on `http://127.0.0.1:5005` using the repo catalog and provider credentials, so PMS can reach a real HDHR surface from the host without relying on broken `.plex.svc` DNS.
    - Used `plex-dvr-cutover` to replace the main broken DVR/device with a new local DVR (`723`) and device (`722`) pointing at `http://127.0.0.1:5005`.
    - Found that Plex's `GET /livetv/epg/channelmap` can emit malformed rows with missing `channelKey`/`lineupIdentifier`, which was causing activation to fail with duplicate-mapping errors; fixed `GetChannelMap` to drop incomplete rows and added a regression test.
    - After the fix, Plex successfully activated 475 channels on the new local DVR.
    - Manual tune replay now reaches Plex's rolling-subscription scheduler when a real `X-Plex-Session-Identifier` is supplied, but still fails before any `/stream/...` request with `The device does not tune the required channel`, so the remaining issue is deeper than URI reachability.
  Verification:
    - `go test ./internal/plex ./cmd/iptv-tunerr`
    - `curl http://127.0.0.1:5005/discover.json`
    - `curl http://127.0.0.1:5005/lineup.json`
    - `go run ./cmd/iptv-tunerr plex-dvr-cutover -plex-url http://127.0.0.1:32400 -token <owner-token> -map /tmp/iptvtunerr-hdhrlocal-cutover.tsv -reload-guide -activate -do`
  Notes:
    - PMS tune replay requires `X-Plex-Session-Identifier`; without it, Plex returns `400` before scheduling.
    - With the session id supplied, PMS logs show `Grabber: Operation ... completed with status error (The device does not tune the required channel)` and never requests `/stream/...` from Tunerr.
  Opportunities filed:
    - none
  Links:
    - `internal/plex/dvr.go`
    - `internal/plex/dvr_test.go`
    - `internal/plex/cutover.go`
    - `cmd/iptv-tunerr/cmd_plex_ops.go`
- Date: 2026-03-22
  Title: Fold generated catch-up libraries into the migration bundle lane
  Summary:
    - Added `Bundle.Catchup` plus `AttachCatchupManifest(...)` in `internal/livetvbundle` so a saved `catchup-publish` manifest can be merged into the same neutral migration artifact as Live TV and bundled Plex libraries.
    - Added `iptv-tunerr live-tv-bundle-attach-catchup` to import `publish-manifest.json` into an existing bundle.
    - Updated library planning so attached catch-up lanes flow through the existing `library-migration-convert`, `library-migration-apply`, and `library-migration-rollout` commands without needing a separate script path.
    - Updated migration docs/changelog to make the new artifact shape explicit.
  Verification:
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This still migrates generated library layouts and shared paths only; it does not attempt Plex metadata DB conversion.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `docs/emby-jellyfin-support.md`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Add live target diffing for library migration plans
  Summary:
    - Added `DiffLibraryPlan(...)` in `internal/livetvbundle` so a planned Emby/Jellyfin library migration can be compared against the live target server before apply.
    - Added `iptv-tunerr library-migration-diff`, which reports per-library `reuse`, `create`, `conflict_type`, and `conflict_path` outcomes using the same host/token resolution rules as the apply path.
    - Updated README and migration/reference docs so the overlap workflow explicitly includes a dry-run validation stage, not just convert and apply.
  Verification:
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is a server-state diff for library definitions and paths only; it is not a metadata DB reconciliation engine.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Add multi-target diffing for library rollout plans
  Summary:
    - Added `DiffLibraryRolloutPlan(...)` so one bundled library rollout can be compared against multiple live non-Plex targets in one pass.
    - Added `iptv-tunerr library-migration-rollout-diff`, which uses the same target selection and host/token env fallback as the rollout/apply path but returns per-target diff results instead of mutating anything.
    - Updated README and migration docs so the overlap workflow now explicitly supports one-shot multi-target validation as well as one-shot multi-target apply.
  Verification:
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This still validates library definitions/paths only; it does not attempt metadata synchronization between Plex and the destination servers.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `docs/emby-jellyfin-support.md`
- Date: 2026-03-22
  Title: Add Live TV diffing for single-target and rollout migration plans
  Summary:
    - Added Emby/Jellyfin read helpers for existing tuner hosts and listing providers so Live TV migration plans can be compared against live target state.
    - Added `DiffEmbyPlan(...)` plus `iptv-tunerr live-tv-bundle-diff` for single-target tuner-host/XMLTV diffing.
    - Added `DiffRolloutPlan(...)` plus `iptv-tunerr live-tv-bundle-rollout-diff` so the same neutral bundle can validate both Emby and Jellyfin targets in one pass before any registration apply.
    - Updated README and migration/reference docs so the Live TV overlap workflow now explicitly includes dry-run validation, not just convert and apply.
  Verification:
    - `go test ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This compares tuner-host and listing-provider definition/state only; it is not channel-level guide or metadata parity analysis.
  Opportunities filed:
    - none
  Links:
    - `internal/emby/register.go`
    - `internal/emby/register_test.go`
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
- Date: 2026-03-22
  Title: Add a combined overlap audit for migration bundles
  Summary:
    - Added `AuditBundleTargets(...)` in `internal/livetvbundle` to combine Live TV diffing and optional library/catch-up diffing into one per-target migration audit result.
    - Added `iptv-tunerr migration-rollout-audit`, which uses the same target/env selection rules as the rollout commands but returns one combined readiness report instead of separate diff artifacts.
    - Updated README and migration/reference docs so the binary now has a top-level answer for "is this whole bundle ready for Emby/Jellyfin?".
  Verification:
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Library audit is optional inside the combined report and is explicitly marked skipped when the bundle carries no shared libraries or catch-up lanes.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Expose migration audit workflow in the dedicated deck
  Summary:
    - Added `/deck/migration-audit.json` in `internal/webui`, backed by `IPTV_TUNERR_MIGRATION_BUNDLE_FILE` plus the configured Emby/Jellyfin targets.
    - The deck Operations lane now includes a Migration workflow card and endpoint wiring so overlap-readiness and lagging-library signals are available from the running appliance, not only the CLI.
    - Reused the same audit formatter from `internal/livetvbundle` so the CLI summary and deck workflow stay aligned.
  Verification:
    - `go test ./internal/webui ./cmd/iptv-tunerr ./internal/livetvbundle ./internal/tuner`
    - `./scripts/verify`
  Notes:
    - This is intentionally read-only; migration apply still lives in the CLI lane.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/webui.go`
    - `internal/webui/deck.js`
    - `internal/livetvbundle/report.go`
    - `docs/emby-jellyfin-support.md`
- Date: 2026-03-22
  Title: Add missing-title detail to the human-readable migration summary
  Summary:
    - Extended `migration-rollout-audit -summary` so title-lagging reused libraries now print bounded `title_missing[...]` lines with missing source sample titles.
    - This keeps the summary compact but makes it materially more actionable for library-sync triage.
    - The change is presentation-only and reuses the existing title-sample parity logic from the audit JSON.
  Verification:
    - `go test ./cmd/iptv-tunerr ./internal/livetvbundle`
    - `./scripts/verify`
  Notes:
    - The summary intentionally caps both the number of libraries and titles shown to stay operator-readable.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle_test.go`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Add a human-readable migration rollout summary
  Summary:
    - Extended `iptv-tunerr migration-rollout-audit` with a `-summary` mode that renders the existing audit as a compact text report instead of raw JSON.
    - The summary includes overall verdict plus per-target status, reason, indexed Live TV channel count, and the main missing/lagging library signals.
    - This keeps the machine-readable audit intact while giving operators a first-class sync/readiness report without external shell or `jq` shaping.
  Verification:
    - `go test ./cmd/iptv-tunerr ./internal/livetvbundle`
    - `./scripts/verify`
  Notes:
    - `-summary` reuses the exact same audit logic; it is only an alternate presentation layer.
  Opportunities filed:
    - none
  Links:
    - `cmd/iptv-tunerr/cmd_live_tv_bundle.go`
    - `cmd/iptv-tunerr/cmd_live_tv_bundle_test.go`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Add title-sample parity hints to the migration audit
  Summary:
    - Extended the neutral migration bundle to carry a bounded sample of Plex library item titles alongside source item counts.
    - Added destination-side title sampling for reused Emby/Jellyfin libraries and surfaced per-library `source_titles`, `existing_titles`, `missing_titles`, and `title_parity_status` in library diffs.
    - Rolled that up into the combined migration audit as `title_synced_libraries` / `title_lagging_libraries` so reused libraries can be flagged as still missing specific source sample titles even when they already exist on the target.
  Verification:
    - `go test ./internal/plex ./internal/emby ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - This is intentionally bounded sample parity, not full metadata-equivalence or watched-state comparison.
  Opportunities filed:
    - none
  Links:
    - `internal/plex/library.go`
    - `internal/emby/library.go`
    - `internal/livetvbundle/bundle.go`
    - `docs/emby-jellyfin-support.md`
- Date: 2026-03-22
  Title: Add readiness verdicts to the combined migration audit
  Summary:
    - Extended the combined migration audit to compute `ready_to_apply` at both the overall and per-target level.
    - Added rolled-up conflict counts plus surface-specific readiness so operators do not have to manually interpret nested Live TV and library diff counts before deciding whether to apply.
    - Updated README and migration docs so the audit is now described as a real pre-apply gate, not just a raw diff aggregator.
  Verification:
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Readiness currently means "no definition conflicts detected"; it does not yet include post-cutover metadata/state progress.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Add convergence status and indexed-channel visibility to the migration audit
  Summary:
    - Extended the combined migration audit with per-target and overall `status` values plus rolled-up indexed Live TV channel counts from the target server.
    - The audit now distinguishes `blocked_conflicts`, `ready_to_apply`, and `converged`, so conflict-free targets that have not indexed channels yet are no longer conflated with already-visible cutovers.
    - Updated README and migration docs so the audit is framed as both a pre-apply gate and a lightweight post-registration visibility signal.
  Verification:
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Convergence currently uses Live TV channel visibility as the post-cutover signal; library/catalog metadata convergence is not yet modeled.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Make migration convergence library-aware
  Summary:
    - Extended library diffs with desired/present counts so the audit can tell whether bundled libraries are already present on the target, not just whether conflicts exist.
    - Updated target/overall `status` so `converged` now requires both indexed Live TV and already-present bundled libraries/catch-up lanes when applicable.
    - Added regression coverage for the partial-migration case where Live TV is indexed but bundled libraries are still missing; that now stays `ready_to_apply` instead of being overstated as converged.
  Verification:
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Convergence still does not model library metadata scans or watch-state parity; it currently means “definitions are present and Live TV is indexed.”
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `docs/emby-jellyfin-support.md`
- Date: 2026-03-22
  Title: Add actionable status reasons and missing-library hints to the migration audit
  Summary:
    - Extended each target audit with `status_reason` plus explicit present/missing bundled library names.
    - This turns partial-migration output into an actionable checklist instead of only a readiness/status label.
    - Updated README and migration docs so the combined audit is framed as an operator workflow tool, not just a machine-readable gate.
  Verification:
    - `go test ./internal/livetvbundle ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - The library hints still reflect definition presence only; they do not yet expose deeper media-library scan or metadata ingestion progress.
  Opportunities filed:
    - none
  Links:
    - `internal/livetvbundle/bundle.go`
    - `internal/livetvbundle/bundle_test.go`
    - `docs/reference/cli-and-env-reference.md`
- Date: 2026-03-22
  Title: Start the free Station Ops lane with virtual-channel station metadata
  Summary:
    - Added a new `STN-*` epic for the fully free station-operations lane, covering branded synthetic stations, filler recovery, richer scheduling, and multi-backend rollout.
    - Extended `internal/virtualchannels` so channels now support station metadata (`description`, branding fields, and recovery/fallback policy) instead of only entry loops.
    - Wired the first visible publish surface changes now: virtual M3U exports `tvg-logo`, synthetic virtual XMLTV exports `<icon>`, and detail/rules responses round-trip the new metadata.
  Verification:
    - `go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This is foundation only. Runtime black-screen detection, filler insertion, and overlay rendering are still future `STN-*` stories.
  Opportunities filed:
    - none
  Links:
    - `docs/epics/EPIC-station-ops.md`
    - `docs/reference/virtual-channel-stations.md`
    - `internal/virtualchannels/virtualchannels.go`
    - `internal/tuner/server.go`
- Date: 2026-03-22
  Title: Add first authoring APIs for station metadata and virtual schedules
  Summary:
    - Extended the existing virtual-channel APIs so operators can update station metadata and schedule entries without replacing the whole rules file.
    - `POST /virtual-channels/channel-detail.json` now updates station metadata for an existing channel, and `POST /virtual-channels/schedule.json` now supports basic authoring helpers such as `append_movies`, `append_episodes`, and `remove_entries`.
    - Updated the station-ops epic/reference docs so this lane is documented as a real API foundation rather than metadata-only scaffolding.
  Verification:
    - `go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - These helpers still operate on entry lists, not true daypart/time-slot templates yet.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `docs/reference/virtual-channel-stations.md`
    - `docs/epics/EPIC-station-ops.md`
- Date: 2026-03-22
  Title: Add daily-slot scheduling to Station Ops virtual channels
  Summary:
    - Extended virtual-channel rules with daily `slots[]` so channels can define UTC `HH:MM` slot starts, durations, labels, and scheduled entries.
    - Updated preview/current-slot/schedule logic to prefer those explicit slots when present instead of only replaying the older looping entry order.
    - Expanded the schedule mutation API with `append_slot`, `replace_slots`, and `remove_slots`, giving the station lane its first real daypart/time-placement substrate.
  Verification:
    - `go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - Slot scheduling is daily UTC only for now; richer template-based authoring and timezone/day-of-week semantics are still future work.
  Opportunities filed:
    - none
  Links:
    - `internal/virtualchannels/virtualchannels.go`
    - `internal/virtualchannels/virtualchannels_test.go`
    - `internal/tuner/server.go`
    - `docs/reference/virtual-channel-stations.md`
- Date: 2026-03-22
  Title: Add a daypart filler helper to Station Ops scheduling
  Summary:
    - Extended `POST /virtual-channels/schedule.json` with `fill_daypart`, which expands a start/end window plus movies/episodes/entries into explicit daily slots.
    - That gives the station lane its first real “fill mornings / fill prime time” helper instead of only manual slot edits or raw loop-entry mutations.
    - Updated the station reference and epic docs so the API surface and roadmap reflect the new daypart builder.
  Verification:
    - `go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - The daypart helper is still content-list driven; it does not yet auto-pull from saved collections/seasons/categories.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `docs/reference/virtual-channel-stations.md`
- Date: 2026-03-22
  Title: Add collection-aware daypart fillers and first runtime filler fallback
  Summary:
    - Extended `POST /virtual-channels/schedule.json` with `fill_movie_category` and `fill_series`, so dayparts can now be auto-built from indexed movie categories or series episode pools instead of only manual lists.
    - Upgraded virtual-channel playback so `recovery.mode=filler` is now executed in one real runtime path: missing-source or failed upstream requests can fall back to configured filler entries.
    - Updated the station reference and epic/task docs so the repo reflects that recovery is no longer metadata-only for virtual channels.
  Verification:
    - `go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - Runtime recovery currently triggers on missing/failed upstreams, not actual black-frame/media-content analysis yet.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `docs/reference/virtual-channel-stations.md`
- Date: 2026-03-22
  Title: Harden virtual-channel filler fallback against obviously bad upstream payloads
  Summary:
    - Added a first dead-air/bad-response guard to the virtual-channel playback path: HTML/text/JSON responses and empty first-read payloads now trigger filler fallback instead of being proxied as fake media.
    - Kept the existing filler execution path for missing-source and failed-upstream cases, so virtual-channel recovery now covers a broader class of obvious playback failures.
    - Updated the station reference/epic/current-task notes to reflect that the runtime recovery path is no longer limited to missing URLs or transport errors.
  Verification:
    - `go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This is still heuristic response validation, not true black-frame media-content analysis.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `docs/reference/virtual-channel-stations.md`
- Date: 2026-03-22
  Title: Make black_screen_seconds a real startup dead-air guard for virtual playback
  Summary:
    - Extended the virtual-channel recovery guard so `recovery.black_screen_seconds` now acts as a startup-byte timeout when headers arrive but no usable media bytes show up.
    - That means virtual playback can now switch to configured filler not just on missing URLs, request failures, or obviously bad non-media bodies, but also on stalled startup responses.
    - Updated the station reference/epic/current-task notes so the repo documents the first real runtime meaning of `black_screen_seconds`.
  Verification:
    - `go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This is still a startup dead-air timeout, not true decoded-video black-frame analysis.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `docs/reference/virtual-channel-stations.md`
- Date: 2026-03-22
  Title: Add OIDC IdP audit reporting and deck workflow
  Summary:
    - Added `AuditOIDCPlanTargets` plus `FormatOIDCAuditSummary` so the neutral OIDC plan can now be audited across Keycloak and/or Authentik with operator-readable status instead of only raw diff/apply output.
    - Added the CLI command `identity-migration-oidc-audit` with JSON and `-summary` output, mirroring the existing media-server migration audit shape.
    - Exposed the same IdP readiness report in the deck at `/deck/oidc-migration-audit.json` and wired the workflow catalog to show it next to the existing migration and identity-cutover lanes.
    - Tightened Keycloak activation-pending detection so existing enabled users are not falsely reported as onboarding-pending just because they already exist.
  Verification:
    - `go test ./internal/migrationident ./internal/webui ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Current OIDC audit is still provisioning-focused; it does not claim full downstream SSO-policy parity.
  Opportunities filed:
    - none
  Links:
    - `internal/migrationident/bundle.go`
    - `cmd/iptv-tunerr/cmd_identity_migration.go`
    - `internal/webui/webui.go`
    - `internal/webui/deck.js`
- Date: 2026-03-22
  Title: Add Authentik OIDC migration backend and IdP-side bootstrap metadata
  Summary:
    - Added `internal/authentik` as the second built-in IdP backend on top of the neutral OIDC identity plan, covering user listing/creation, group listing/creation, membership add, password bootstrap, and recovery-email onboarding.
    - Extended `internal/migrationident` plus `cmd_identity_migration.go` with `identity-migration-authentik-diff` and `identity-migration-authentik-apply`, and kept the same provider-agnostic OIDC plan contract used by Keycloak.
    - Stamped stable Tunerr migration metadata onto newly created Keycloak and Authentik users so later cutover/audit work can trace subject hints, Plex ids/uuid, and group hints from the IdP side too.
  Verification:
    - `go test ./internal/authentik ./internal/keycloak ./internal/migrationident ./cmd/iptv-tunerr`
    - `./scripts/verify`
  Notes:
    - Current IdP scope is still intentionally migration-safe rather than full provider-policy sync: create missing users/groups, add membership, and optional bootstrap/onboarding mail.
  Opportunities filed:
    - none
  Links:
    - `internal/authentik/authentik.go`
    - `internal/authentik/authentik_test.go`
    - `internal/migrationident/bundle.go`
    - `cmd/iptv-tunerr/cmd_identity_migration.go`
- Date: 2026-03-22
  Title: Add ffmpeg-backed content probes and rendered station slates to Station Ops
  Summary:
    - Added an ffmpeg-backed content probe on the virtual playback path that can trigger filler fallback when sampled content looks black (`blackdetect`) or silent (`silencedetect`) from the start.
    - Wired that probe into the existing virtual recovery lane, so filler cutover now has a first true content-aware path in addition to transport/startup heuristics.
    - Added `/virtual-channels/slate/<id>.svg`, a rendered station-slate surface driven by branding metadata (`logo_url`, bug text/position, banner text, theme color), so the branding lane now has a first real rendered output.
  Verification:
    - `go test ./internal/virtualchannels ./internal/tuner ./cmd/iptv-tunerr`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - The content-aware path is still preflight/probe based, not decoded in-stream frame/audio analysis.
    - Slate rendering is real output, but not yet live-video compositing over the stream itself.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `docs/reference/virtual-channel-stations.md`
    - `docs/epics/EPIC-station-ops.md`
- Date: 2026-03-22
  Title: Audit and improve operator deck affordances and accessibility
  Summary:
    - Added skip-to-content, labeled search/raw-endpoint controls, stronger focus-visible states, `aria-live` status/feedback, and explicit pressed-state / section-visibility semantics across the dedicated deck.
    - Reworked the shared deck modal to use real dialog semantics with focus restoration/trapping, then replaced virtual-station branding/recovery `window.prompt` edits with an in-deck modal editor.
    - Updated `README.md` and `docs/features.md` so the operator-plane docs now describe the deck accessibility/modal behavior instead of leaving it implicit in code.
  Verification:
    - `node -c internal/webui/deck.js`
    - `go test ./internal/webui ./cmd/iptv-tunerr ./internal/tuner ./internal/virtualchannels`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This pass focused on the dedicated deck/operator surface, not the older `/ui/` shell.
  Opportunities filed:
    - none
  Links:
    - `internal/webui/index.html`
    - `internal/webui/deck.css`
    - `internal/webui/deck.js`
    - `internal/webui/webui_test.go`
    - `README.md`
    - `docs/features.md`
- Date: 2026-03-22
  Title: Reframe legacy `/ui/` as a compatibility shell
  Summary:
    - Audited the old tuner-port UI and confirmed it is limited to `/ui/`, `/ui/guide/`, and `/ui/guide-preview.json` rather than a second full operator plane.
    - Updated the served legacy HTML so it explicitly tells operators the dedicated Control Deck is primary and links back to it using the configured web UI port.
    - Updated docs so `/ui/` is now described as compatibility/read-only surface instead of a first-class operator UI.
  Verification:
    - `go test ./internal/webui ./cmd/iptv-tunerr ./internal/tuner ./internal/virtualchannels`
    - `go build -o ./iptv-tunerr ./cmd/iptv-tunerr`
  Notes:
    - This keeps `/ui/` available for lightweight/read-only use while making the product posture explicit.
  Opportunities filed:
    - none
  Links:
    - `internal/tuner/operator_ui.go`
    - `internal/tuner/static/ui/index.html`
    - `internal/tuner/static/ui/guide.html`
    - `internal/tuner/server_test.go`
    - `README.md`
    - `docs/features.md`
- Date: 2026-03-22
  Title: Rewrite the README opening into a real product intro
  Summary:
    - Rewrote the README opening again so the top of the repo reads like a product summary instead of a migration/identity wall.
    - Pulled custom EPG generation/repair, staged migration, and the owned-media / indie-broadcaster story higher into the visible intro instead of leaving them implied or buried later.
    - Replaced the earlier dense list-heavy migration block with shorter paragraphs that match the rest of the README better.
  Verification:
    - manual README review
  Notes:
    - This was a docs-only pass aimed at discoverability and readability, not a runtime behavior change.
  Opportunities filed:
    - none
  Links:
    - `README.md`
- Date: 2026-03-23
  Title: Close release hardening items for CodeQL, Dependabot, and licensing
  Summary:
    - Bumped `.github/workflows/docker.yml` from `docker/setup-buildx-action@v3` to `@v4`.
    - Fixed the open diagnostics path-traversal class by sanitizing operator/env run identifiers before `filepath.Join(...)` in `internal/tuner/server.go`, with a regression that exercises a malicious `case_id`.
    - Removed credential-derived fallback session token generation from `internal/webui/webui.go` so the deck no longer hashes configured auth credentials when `crypto/rand` fails.
    - Clarified repository licensing as AGPL-3.0-or-later or commercial via `LICENSE`, `LICENSE-COMMERCIAL.md`, `README.md`, and `docs/CHANGELOG.md`.
  Verification:
    - `go test ./internal/tuner ./internal/webui`
    - `./scripts/verify`
  Notes:
    - This pass was driven by live release prep: one Dependabot PR, five CodeQL path-expression alerts in `internal/tuner/server.go`, one CodeQL crypto alert in `internal/webui/webui.go`, and the requested dual-license wording update.
  Opportunities filed:
    - none
  Links:
    - `.github/workflows/docker.yml`
    - `internal/tuner/server.go`
    - `internal/tuner/server_test.go`
    - `internal/webui/webui.go`
    - `LICENSE`
    - `LICENSE-COMMERCIAL.md`
    - `README.md`
    - `docs/CHANGELOG.md`
