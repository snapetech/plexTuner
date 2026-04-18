---
id: changelog
type: reference
status: stable
tags: [changelog, reference]
---

# Changelog

All notable changes to IPTV Tunerr are documented here. Repo: [github.com/snapetech/iptvtunerr](https://github.com/snapetech/iptvtunerr).

---

## [Unreleased]

- *(none)*

## [v0.1.45] — 2026-04-18

### Build / release
- **Release verify follow-up:** fixed `gofmt -s` drift in `internal/tuner/gateway_shared_relay.go` so the stability release can pass the GitHub Actions `CI` and `Release` verification jobs. No runtime behavior changed beyond the already-landed `v0.1.44` stream fixes.

## [v0.1.44] — 2026-04-18

### Plex / deployment stability
- **Phantom `plexKube` source root cause fixed:** duplicate blank Live TV sources were traced to an old bare-metal Plex Media Server still publishing the same server identity alongside `plex-standby`. The deployment/runbook now treats disabling and masking that host service as required when `plex-standby` is the active server.
- **Tunerr Plex registration now reconciles stale same-lineage DVRs before creating new ones:** repeated API registrations stop churning duplicate Tunerr-owned DVR/device rows in Plex and instead reuse the canonical current DVR when the device/base-url lineage already matches.

### Streaming / recovery
- **HLS stall recovery is materially stronger:** when a playlist stalls after real progress, Tunerr now retries the same primary upstream in a bounded way before falling through to alternates, which keeps long-running channels alive through transient provider `509` / no-new-segment windows.
- **Post-progress streams no longer downgrade into generic `502` failures:** requests that already delivered bytes are now recorded as `stream_ended_after_progress` instead of being misclassified as `all_upstreams_failed` once all fallback paths are exhausted.
- **Shared relay joins are safer and more observable:** late attaches now skip stale zero-replay shared relays instead of returning hollow `200/0-byte` sessions, and shared-relay logging/state now records replay bytes, idle time, and zero-byte joins explicitly.

### Observability
- **Live logs carry better recovery detail:** stream logs now make same-upstream retries, shared-relay attach accepts/skips, zero-byte shared joins, and post-progress termination more explicit so operators can diagnose real client-facing hiccups without waiting on user reports.

## [v0.1.43] — 2026-04-17

### Plex / multi-DVR onboarding
- **API-first zero-touch Plex registration is now the default documented path:** top-level help, `setup-doctor`, the Plex connection guide, and deployment docs now steer new users toward `PLEX_HOST` + `PLEX_TOKEN` with `run -mode=full -register-plex=api` instead of leading with the older DB-path registration flow.
- **Full-mode setup doctor now checks Plex API readiness:** `setup-doctor` now reports whether `IPTV_TUNERR_PMS_URL` / `PLEX_HOST` and `IPTV_TUNERR_PMS_TOKEN` / `PLEX_TOKEN` are present when users choose `-mode=full`, and it prints the exact zero-touch next step when they are.
- **Official two-DVR pattern is now shipped:** the repo now includes `k8s/iptvtunerr-supervisor-general-sports.example.json`, which documents the validated `general` + `sports_na` supervisor pattern with distinct device IDs, base URLs, and guide-number offset handling.

### Lineup shaping
- **New `sports_na` recipe:** lineup and registration shaping now support a stricter North America sports-first recipe that keeps Canadian and US sports brands/leagues while rejecting obvious international sports-only noise that would otherwise pollute a second sports DVR.
- **Docs/reference updated for `sports_na`:** the README, features reference, CLI/env reference, and Plex setup docs now treat `sports_na` as a first-class built-in recipe rather than leaving `sports_now` as the only documented sports path.

## [v0.1.42] — 2026-04-17

### Plex / cluster playback
- **Full standby lineup restore:** the XMLTV merge path no longer collapses distinct exposed lineup rows that share the same upstream `TVGID`, so Plex standby can import the full curated `479`-channel provider view instead of dropping duplicate-guide rows during XMLTV/channelmap reconciliation.
- **Safer HLS ingest defaults for real ffmpeg builds:** the tuner now treats HLS `http_persistent` and `live_start_index` as opt-in compatibility flags, which avoids cluster ffmpeg startup failures on builds that do not support those options.
- **Localized Plex-first cluster posture:** the cluster Tunerr deployment now resequences guide numbers after lineup shaping so the curated local/Canadian-first order becomes the actual visible `1..N` Plex provider order.
- **PMS internal-fetcher normalization:** the cluster deployment now forces Plex's internal `Lavf` fetcher lane onto the `copyvideomp3` websafe profile, which fixes the standby playback path that previously handed PMS raw AAC with `sampleRate=0` / `channels=0` and caused `sample rate not set` recorder failures.

## [v0.1.41] — 2026-04-17

### CI / security scanning
- **Gitleaks false-positive cleanup:** reworded a token-shaped phrase in `memory-bank/current_task.md` that triggered the `generic-api-key` rule in GitHub Actions after `v0.1.40`. No runtime behavior changed; this release only clears the secret-scan failure and restores a green post-release workflow path.

## [v0.1.40] — 2026-04-17

### Product / onboarding
- **Setup-doctor onboarding path:** added a dedicated `iptv-tunerr setup-doctor` command, shared setup-doctor runtime/report logic, a minimal `.env.minimal.example`, and updated top-level help plus docs so first-run users are steered through `setup-doctor`, `probe`, and `run -mode=easy` instead of the full operator surface.
- **Default deck path is now readiness-first:** the dedicated deck now starts with setup posture, exact tuner/guide/deck URLs, and the shortest path to connecting Plex/Emby/Jellyfin. Advanced workflow and raw-surface lanes remain available, but they are demoted behind explicit advanced-mode preferences instead of defining the first-run experience.

### Runtime / structure
- **Large server/web UI slices were broken up without changing behavior:** setup/auth/migration handlers are now separated out of the old `internal/webui/webui.go` monolith, and major tuner route clusters now live in focused files for status/reporting, operator workflows, programming, virtual channels, virtual playback/recovery, and diagnostics/recordings instead of accumulating in one `server.go` block.
- **Shared setup-doctor contract across CLI and deck:** the deck now exposes `/deck/setup-doctor.json` and reuses the same setup readiness contract as the CLI, so first-run guidance is consistent between command line and web UI.

### Plex / XMLTV
- **Cluster Plex import is fixed for full-size lineups:** Plex DVR/channel repair now sends one full channel-map activation request instead of split mapping batches, and XMLTV channel IDs are shortened aggressively enough for PMS to accept the full request on a real 463-channel lineup.
- **Guide stability improved for real provider flaps:** provider XMLTV disk-cache support prevents intermittent upstream `xmltv.php` failures from collapsing the served guide back to placeholder-only during normal refresh windows.
- **XMLTV channel identity is more Plex-friendly:** guide output now includes numeric guide-number display names alongside channel titles, which makes the imported provider lineup materially cleaner in Plex.

### Fixed

- **Live migration compatibility on real cluster targets**: Jellyfin Live TV rollout audit no longer fails closed on `10.11.x` just because Jellyfin omits read-side `GET /LiveTv/TunerHosts` and `GET /LiveTv/ListingProviders`; Tunerr now reads exact tuner/listing parity from Jellyfin's `GET /System/Configuration/livetv` endpoint instead. The Keycloak OIDC audit/apply lane also no longer has to rely on a short-lived static bearer token when admin username/password credentials are available, because Tunerr can now mint a fresh `admin-cli` token for the run.
- **Catalog identity collapse hardening**: `dedupeByTVGID` now merges duplicate channels only when `tvg_id` and normalized guide-name identity align (and still keeps same-name backups merged), preventing unrelated market or variant feeds (`Plus`, `East`, `West`) from disappearing into one channel when providers over-normalize identifiers.
- **Shared-output reuse expansion**: same-channel viewers now reuse the existing live FFmpeg HLS producer as well as the profile-selected ffmpeg packaged-HLS session when the requested output shape matches, including named-profile `fMP4` output, instead of being rejected as another tuner/account consumer. Shared sessions now keep a bounded replay window for late subscribers, which makes attached `fMP4` consumers materially safer. README/docs and binary smoke coverage now call that out explicitly so “one PPV, one upstream session” is a tested contract rather than an implementation detail.
- **Security and release hardening**: diagnostics run identifiers are now sanitized before filesystem joins, the deck no longer hashes configured credentials when its crypto-random session fallback path is used, `docker/setup-buildx-action` is bumped to `v4`, and the repository licensing language is clarified as AGPL-3.0-or-later or commercial.

### Added

- **Deck OIDC workflow modal history**: the dedicated deck now carries the same recent OIDC apply history controls inside the OIDC workflow modal that it already exposed on the summary card, including `all / success / failed` filtering plus success/failure badges for recent Keycloak/Authentik apply attempts.
- **Deck OIDC modal target outcomes**: OIDC workflow modal history now expands each recent apply into per-target outcome rows so partial Keycloak/Authentik runs show which target was applied and which one was not reached before failure.
- **Station-ops runtime slice (`STN-001` through the first release-grade `STN-005` pass)**: virtual-channel rules can now carry station metadata (`description`, branding/logo fields, bug/banner fields, theme color, per-channel `stream_mode`, and recovery/filler policy metadata), and those fields now surface through `/virtual-channels/rules.json`, `/virtual-channels/channel-detail.json`, `/virtual-channels/report.json`, `/virtual-channels/recovery-report.json`, `/virtual-channels/live.m3u` (`tvg-logo`), and `/virtual-channels/guide.xml` (`<icon>`). Existing channels can now also be updated without replacing the whole rules file: `POST /virtual-channels/channel-detail.json` edits station metadata with merge-safe `branding` / `recovery` updates, and `POST /virtual-channels/schedule.json` supports basic schedule authoring helpers (`append_entry`, `replace_entries`, `append_movies`, `append_episodes`, `remove_entries`) plus daily-slot scheduling (`append_slot`, `replace_slots`, `remove_slots`) and daypart fillers (`fill_daypart`, `fill_movie_category`, `fill_series`). Virtual-channel preview/current-slot/schedule logic now prefers explicit daily `slots[]` when present instead of only replaying the older loop-entry order, branded playback can now render a slate surface (`/virtual-channels/slate/<id>.svg`) and a composited branded stream (`/virtual-channels/branded-stream/<id>.ts`) with bug/banner text and a corner image, and the deck can mutate branding/recovery posture directly from the Programming lane. The recovery lane is no longer startup-only: it now records inspectable recovery events, can persist that history across restarts, can cut over across an ordered fallback chain on startup failures, bad response bodies, repeated live stall/read-error events, and repeated rolling sampled midstream black/silence probes, and reports explicit exhaustion when the fallback chain runs out. Deeper full decode-grade media analytics and richer long-session observability still remain future work.
- **Live TV migration apply path**: `iptv-tunerr live-tv-bundle-apply` can now take a converted Emby/Jellyfin registration plan and register it directly against a live server using the same built-in Live TV APIs as runtime registration. That turns the migration lane into build → convert → apply, and makes “keep Plex running while pre-rolling Emby/Jellyfin” a first-class workflow instead of a lab-only JSON export.
- **Live TV migration diffing**: `live-tv-bundle-diff` and `live-tv-bundle-rollout-diff` can now compare planned tuner-host / XMLTV registrations against one or both live Emby/Jellyfin targets and report reuse/create/conflict results before any apply happens.
- **Live TV multi-target rollout**: `iptv-tunerr live-tv-bundle-rollout` can now build or apply a shared Emby+Jellyfin rollout from one neutral Plex-derived bundle, so overlap migration can be treated as one coordinated step instead of two unrelated manual applies.
- **Library migration foundation**: migration bundles can now optionally include Plex library sections and shared storage paths, and the new `library-migration-convert` / `library-migration-apply` commands can turn those into Emby/Jellyfin library plans and apply them through the built-in media-server APIs. This intentionally migrates library definitions, not vendor metadata databases.
- **Library migration diffing**: `library-migration-diff` can now compare a planned Emby/Jellyfin library migration against the live target server and report which libraries would be reused, created, or blocked by path/type conflicts before apply.
- **Library multi-target rollout**: `library-migration-rollout` can now build or apply the same bundled Plex library definitions across both Emby and Jellyfin in one coordinated step, matching the existing Live TV rollout model.
- **Library multi-target diffing**: `library-migration-rollout-diff` can now compare that same bundled library rollout against one or both live targets at once, so overlap validation does not require running separate single-target diff commands by hand.
- **Combined migration audit**: `migration-rollout-audit` now combines the Live TV and library/catch-up diff lanes into one per-target overlap-readiness report, so operators do not have to manually correlate separate diff outputs before apply.
- **Migration readiness verdicts**: the combined audit now computes `ready_to_apply` plus rolled-up conflict counts per target and overall, turning it from a raw diff aggregator into a real pre-apply gate.
- **Migration convergence signals**: the combined audit now also reports target `status` and current indexed Live TV channel counts, so operators can tell whether a target is merely conflict-free or already visibly converged after registration.
- **Library-aware convergence**: the combined audit now treats missing bundled libraries/catch-up lanes as not yet converged, even when Live TV is already indexed, so `converged` means the whole bundle is materially present on the target rather than only the tuner side.
- **Actionable audit hints**: the combined audit now reports `status_reason` plus explicit present/missing library names so partial migrations point to the exact bundled surfaces that still need to land.
- **Library population hints**: reused bundled libraries in the combined audit now also report whether they are already populated or still empty on the destination server, giving operators a coarse post-cutover signal beyond simple library-definition presence.
- **Library scan visibility**: the combined audit now also includes best-effort library scan task state/progress when Emby/Jellyfin exposes a recognizable refresh-library scheduled task, giving overlap migrations a coarse ingest-progress signal after apply.
- **Library parity hints**: source Plex library counts now flow through the neutral migration bundle, and the combined audit compares them with reused destination-library counts to report which libraries are already synced and which still lag the source.
- **Title-sample parity hints**: the neutral migration bundle now also carries a bounded sample of Plex library item titles, and the combined audit compares those against reused destination-library titles so it can flag reused libraries that still miss specific source sample titles even when the library already exists.
- **Human-readable migration summary**: `migration-rollout-audit` now supports `-summary`, which renders the existing audit as a compact text rollout report with per-target status, reasons, indexed-channel counts, and the main missing/lagging library signals.
- **Sample-title lag detail in summary mode**: the human-readable migration summary now also lists bounded per-library missing title samples for reused libraries that are still behind the Plex source, so operators do not have to open the raw nested JSON to see what is missing.
- **Deck migration workflow**: when `IPTV_TUNERR_MIGRATION_BUNDLE_FILE` is set, the dedicated deck now exposes `/deck/migration-audit.json` and a Migration workflow card that surfaces the same overlap audit from the running process, including ready/converged state and lagging-library hints.
- **Catch-up bundle attachment**: `live-tv-bundle-attach-catchup` can now fold a saved catch-up publish manifest into the same neutral migration bundle, so generated `.strm` / `.nfo` library layouts move through the same convert/apply/rollout lane as Live TV and shared library definitions.
- **Identity migration foundation**: `plex-user-bundle-build`, `identity-migration-convert`, `identity-migration-diff`, `identity-migration-apply`, `identity-migration-rollout`, and `identity-migration-rollout-diff` now give the migration lane a first-class user/account bootstrap path. Tunerr can export Plex users plus visible share/tuner hints, build Emby/Jellyfin local-user plans from them, diff those plans against one or both live targets, and create only the missing users without pretending to copy passwords or solve OIDC yet.
- **Identity audit/readiness reporting**: `identity-migration-audit` now turns that user-migration lane into a real pre-cutover operator surface. It reports per-target status, missing destination users, and managed/share/tuner-entitled Plex users that still need manual post-create follow-up such as permissions, invites, or future SSO alignment, and it also supports a compact `-summary` mode.
- **Deck identity workflow**: when `IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE` is set, the dedicated deck now exposes `/deck/identity-migration-audit.json` and an Identity Migration workflow card so account-cutover readiness is visible from the running process, not only through CLI audit commands.
- **Identity policy parity**: the identity migration lane now also syncs the first safe additive access-policy subset it can infer from Plex share state. Diff/apply/audit now report and push destination Live TV access, sync/download rights, global all-library grants, and remote access for shared users instead of leaving every share/tuner entitlement as manual cleanup.
- **Identity activation readiness**: the same identity diff/apply/audit lane now reports activation-pending destination users separately from missing accounts and policy drift. Existing or newly created local users with no configured password or auto-login path are now called out explicitly so overlap cutovers can distinguish “account exists” from “human can actually sign in.”
- **Provider-agnostic OIDC planning**: `identity-migration-oidc-plan` now derives stable subject hints, usernames, display names, email hints, and Tunerr-owned group claims from the Plex user bundle. This is now the neutral contract for built-in IdP migrations, not a guessed one-provider apply path.
- **Live OIDC backends: Keycloak and Authentik**: `identity-migration-keycloak-diff` / `identity-migration-keycloak-apply` and `identity-migration-authentik-diff` / `identity-migration-authentik-apply` now reconcile that neutral OIDC plan against real IdPs. Current scope is deliberately safe: create missing users, create missing Tunerr-owned migration groups, add missing membership, stamp stable Tunerr migration metadata on newly created users, and optionally bootstrap onboarding mail/passwords (`execute-actions-email` for Keycloak, recovery email for Authentik).
- **OIDC migration audit and deck workflow**: `identity-migration-oidc-audit` now reports missing IdP users, missing migration groups, and missing group membership across Keycloak and/or Authentik from the same neutral OIDC plan, with compact summary output. The dedicated deck now exposes that same state at `/deck/oidc-migration-audit.json` when `IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE` and the relevant IdP envs are set.
- **Existing-user IdP parity plus deck-side OIDC apply**: Keycloak/Authentik OIDC apply no longer treats existing users as “reuse and ignore” when Tunerr-owned migration metadata, display name, or email hint drift. The dedicated deck also now exposes `POST /deck/oidc-migration-apply.json`, so a configured OIDC plan can be applied to the live IdP targets directly from the appliance with the same session/CSRF protections as the rest of the deck control plane.
- **Deck-side OIDC onboarding controls**: the deck OIDC apply path now accepts the same provider knobs as the CLI instead of being a stripped-down wrapper: Keycloak bootstrap password, temporary-password choice, `execute-actions-email` actions plus optional client/redirect/lifespan hints, and Authentik bootstrap password plus recovery-email delivery.
- **OIDC workflow keeps last apply context**: the deck OIDC workflow summary now includes the most recent recorded OIDC apply result from deck activity, including target/provider option hints, so operators can see what was last pushed without relying on the transient modal or manually opening the full activity log.
- **OIDC workflow shows per-target apply deltas**: the deck's persisted last-apply summary now also carries per-target result counts from the last Keycloak/Authentik push, so the workflow shows what actually changed instead of only when it ran and which onboarding knobs were used.
- **OIDC workflow keeps short apply history**: the deck OIDC workflow now also includes a short recent apply history from deck activity instead of only the most recent run, so operators can see a few recent Keycloak/Authentik cutover attempts from the same workflow surface.
- **OIDC workflow now keeps failed apply attempts too**: deck-side OIDC apply failures are now normalized into the same structured `oidc_migration_apply` history instead of only returning transient JSON errors, so the workflow history can show validation/provider failure phase and error context alongside successful runs.
- **OIDC workflow adds success/failure filtering**: the deck's recent OIDC apply history now shows explicit success/failure badges and supports `all`, `success`, and `failed` filtering so operators can isolate bad IdP runs without reading every history line.
- **Keycloak onboarding bootstrap**: the Keycloak apply path can now optionally set a bootstrap password and trigger `execute-actions-email`, which makes the first IdP backend useful for actual staged user onboarding instead of only user/group provisioning.

## [v0.1.29] — 2026-03-21

### Fixed

- **Xtream export identity collisions**: downstream `xmltv.php` / `get.php` now use canonical exported ids based on Tunerr `ChannelID` instead of raw provider `TVGID`, preventing sibling variants from collapsing when an upstream provider reuses `tvg_id` or guide numbers.
- **Catch-up / browse channel mapping drift**: catch-up capsule previews now emit real lineup `ChannelID` values and duplicate programme rows for every matching lineup channel on a shared guide number, which fixes Programming Manager next-hour titles and Xtream XMLTV programme attachment for sibling variants.
- **Xtream VOD proxy parity**: movie/series proxies now support `HEAD`, forward `Range`, and preserve `Content-Length`, `Accept-Ranges`, `Content-Range`, `Last-Modified`, and `ETag` so downstream clients can probe and seek like they already can on the virtual-channel proxy path.
- **Programming numeric ordering**: Programming Manager category members and recommended ordering now sort numeric guide numbers numerically instead of lexically.

### Changed

- **Programming browse guide preview caching**: repeated browse/detail requests now reuse a cached catch-up capsule snapshot per guide cache + horizon instead of rebuilding the same XMLTV preview on every request.
- **Release smoke alignment**: `scripts/ci-smoke.sh` now asserts the canonical Xtream XMLTV ids used after the export-identity fix, keeping CI and the downstream contract in sync.

## [v0.1.28] — 2026-03-21

### Added

- **Feature parity foundation (`PAR-001`)**: file-backed event webhooks via `IPTV_TUNERR_EVENT_WEBHOOKS_FILE`, async JSON delivery, and lifecycle emission for `lineup.updated`, `stream.requested`, `stream.rejected`, and `stream.finished`.
- **Event debug surface**: `/debug/event-hooks.json` reports configured hooks and recent deliveries, and `/debug/runtime.json` now surfaces event-hook runtime state.
- **Active stream debug surface (`PAR-007` slice)**: `/debug/active-streams.json` now reports currently in-flight stream sessions and live tuner occupancy.
- **Active stream stop control (`PAR-007` slice)**: `/ops/actions/stream-stop` can now cancel matching active stream contexts by request ID or channel ID, turning the active-stream surface into a real operator intervention path instead of a read-only report.
- **Xtream-compatible live output starter (`PAR-004` slice)**: optional read-only downstream `player_api.php` (`get_live_streams`, `get_live_categories`) plus `/live/<user>/<pass>/<channel>.ts`, backed by the curated lineup and existing gateway.
- **Xtream VOD/series expansion (`PAR-004` slice)**: the downstream Xtream starter now also serves `get_vod_categories`, `get_vod_streams`, `get_series_categories`, `get_series`, and `get_series_info`, plus Tunerr-owned `/movie/<user>/<pass>/<id>.mp4` and `/series/<user>/<pass>/<episode>.mp4` proxy paths for catalog VOD and series episodes.
- **Xtream short-EPG expansion (`PAR-004` slice)**: the downstream Xtream starter now also answers `get_short_epg` and `get_simple_data_table` for both real live channels and virtual channels using Tunerr's existing guide and synthetic virtual-schedule pipeline.
- **Xtream export expansion (`PAR-004` slice)**: the downstream Xtream starter now also publishes user-scoped `get.php` and `xmltv.php` outputs, so the same entitled live lineup can be exported as M3U + XMLTV without a separate sidecar pipeline.
- **Programming channel detail (`PM-009` follow-up)**: `/programming/channel-detail.json` now gives a focused channel view with category/taxonomy metadata, exact-match backup alternatives, and a 3-hour upcoming-programme preview for curses/CLI-style programming tools.
- **Xtream entitlements starter (`PAR-005` slice)**: `IPTV_TUNERR_XTREAM_USERS_FILE` now enables file-backed downstream users with scoped live/VOD/series access, `/entitlements.json` exposes or updates that file from the operator plane, and both `player_api.php` and `/live|movie|series/...` now filter/deny output based on the authenticated user instead of treating the downstream Xtream surface as one global catalog.
- **Recording rules/history starter (`PAR-003` slice)**: `IPTV_TUNERR_RECORDING_RULES_FILE` now enables durable server-side recording rules, `/recordings/rules.json` CRUD, `/recordings/rules/preview.json` against live catch-up capsules, and `/recordings/history.json` classification of recorder state against the current ruleset.
- **Shared HLS relay reuse foundation (`PAR-002` slice)**: same-channel duplicate consumers can now attach to one live HLS Go-relay session instead of always starting another upstream walk, and `/debug/shared-relays.json` exposes current shared sessions plus subscriber counts.
- **Plex lineup harvest starter (`LH-001` / `LH-002` / `LH-003`)**: extracted the old Plex oracle probe flow into `internal/plexharvest` plus a named `iptv-tunerr plex-lineup-harvest` command. It now expands cap templates, polls Plex channel-map results for each target, captures lineup titles/URLs in structured JSON, and emits a deduped lineup summary so market/provider harvest experiments stop living only in ad hoc lab output.
- **Live TV migration bundle foundation**: added `iptv-tunerr live-tv-bundle-build`, which exports a neutral bundle from Plex DVR/device state, and `iptv-tunerr live-tv-bundle-convert`, which turns that bundle into an Emby/Jellyfin registration plan. This is the first reusable builder/converter slice for “move IPTV users off Plex without retyping tuner/guide identity by hand.”
- **Plex lineup harvest bridge (`LH-004` / `LH-005` starter)**: saved harvest reports can now live in `IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE`, surface through `/programming/harvest.json`, flow into `/programming/preview.json`, and now seed a real recipe-import path via `/programming/harvest-import.json` so Programming Manager can preview/apply harvested lineup candidates instead of treating harvest results as one-shot CLI output only.
- **Deck harvest import controls**: the Programming lane can now preview/apply harvested lineup imports directly from the control deck instead of requiring a manual POST to `/programming/harvest-import.json`.
- **Harvest import heuristics**: Programming harvest imports now report how rows matched back onto the current catalog and can fall back to a local-broadcast callsign stem match when exact market strings differ across otherwise equivalent local channels.
- **Virtual channel schedule surface**: the virtual-channel starter now exposes `/virtual-channels/schedule.json` for a rolling synthetic schedule horizon in addition to preview, M3U export, and current-slot playback.
- **Virtual channels starter (`PAR-006` slice)**: `IPTV_TUNERR_VIRTUAL_CHANNELS_FILE` now enables file-backed virtual-channel rules plus `/virtual-channels/rules.json` and `/virtual-channels/preview.json`, and the starter is now publishable too: `/virtual-channels/live.m3u` exports the enabled synthetic rows while `/virtual-channels/stream/<id>.mp4` proxies the currently scheduled asset.
- **Virtual channels bridged into Xtream live output**: enabled virtual channels now appear in downstream Xtream `get_live_categories` / `get_live_streams` output and play through `/live/<user>/<pass>/virtual.<id>.mp4`, so PAR-006 no longer stops at sidecar-only M3U/XMLTV/pseudo-live surfaces.
- **Diagnostics workflow promoted from scripts**: `/ops/workflows/diagnostics.json` now turns recent stream attempts into a concrete capture playbook with suggested good/bad channel IDs, the latest `.diag/` run families, and summarized verdict/findings from the newest `channel-diff`, `stream-compare`, `multi-stream`, or evidence-bundle artifacts. `/ops/actions/evidence-intake-start` scaffolds `.diag/evidence/<case-id>/` directly from the operator plane, the deck surfaces that workflow in Routing/Settings, and `scripts/ci-smoke.sh` now asserts the workflow plus evidence-bundle creation in the release gate.
- **Bounded diagnostics launchers**: localhost operators can now trigger direct bounded `channel-diff` and `stream-compare` harness runs from `/ops/actions/channel-diff-run` and `/ops/actions/stream-compare-run`, with the deck exposing those actions next to the diagnostics workflow instead of stopping at capture instructions.
- **Programming feed descriptors**: Programming Manager now derives operator-facing feed descriptors from provider-presented metadata (`region | category | feedtype/fps-style tags`) and surfaces them across category members, curated preview rows, channel detail, and exact-backup alternatives.
- **Programming browse lane**: `/programming/browse.json` now returns one category’s channel rows with cached guide-health status, next-hour programme titles/counts, exact-backup counts, recipe inclusion flags, and feed descriptors in one batch, and the deck can switch categories into that browse view directly instead of polling channel detail one row at a time.
- **Programming channel-aware diagnostics**: the deck can now launch bounded `stream-compare` captures for the current Programming selection and exact-backup `channel-diff` captures against alternative sources, instead of relying only on the global diagnostics suggestions.
- **Programming backup-source preference**: exact-backup groups now support durable preferred-primary selection in the saved recipe, so operators can explicitly keep `DirecTV SyFy` ahead of `Sling SyFy` (or vice versa) instead of relying on incidental ingest order. `/programming/backups.json` now mutates that preference, the curated preview applies it to collapsed rows, and binary smoke covers the behavior.
- **Programming quick-add browse filters**: the deck can now toggle “Real Guide Only” and “Only Not In Lineup” against `/programming/browse.json`, making PPV/event-style channel hunts faster without one-off client-side polling or manual JSON filtering.
- **Harvest assist apply UX**: the Programming lane now turns `/programming/harvest-assist.json` into real preview/apply controls, so ranked local-market lineup assists are actionable directly from the deck instead of read-only text.

### Fixed

- **Provider-account rollover robustness**: account pooling now falls back to Xtream path credentials (`/live/<user>/<pass>/...`, `/movie/...`, `/series/...`, `/timeshift/...`) when per-stream auth metadata is missing or incomplete, so concurrent sessions can still spread across distinct provider accounts instead of collapsing back to the global default credentials.
- **Three-account rollover regression coverage**: gateway tests now explicitly pin three simultaneous channel requests to three distinct Xtream-path credential sets so "second device did not roll over" keeps failing in CI if account leasing regresses.
- **Exact-backup over-collapse**: exact backup grouping now requires both the identity key (`tvg_id` or `dna_id`) and normalized guide-name agreement, preventing distinct variants like `AMC` vs `AMC Plus` or East/West feeds from collapsing together when providers over-normalize `tvg_id`.
- **Materializer concurrent same-asset test flake**: `internal/materializer.TestDirectFile_concurrentSameAsset` no longer drives its helper server into a negative `WaitGroup` counter when duplicate GETs race.

## [v0.1.27] — 2026-03-21

### Streaming
- **Provider-account stream pooling:** deduplicated multi-account live channels now derive a stable provider-account identity per URL, prefer less-loaded accounts during upstream ordering, and can enforce **`IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT`** as a per-credential stream cap. When every candidate account for a channel is already at the cap, Tunerr now rejects the tune locally with HDHR-style **805** / HTTP **503** instead of waiting for a later upstream failure.
- **Adaptive provider-account limits:** provider-account pooling is no longer purely static. When a specific credential set starts returning upstream concurrency-limit responses, Tunerr now learns a tighter cap for that account, applies it on later tune attempts, and surfaces the learned state in **`/provider/profile.json`** as **`account_learned_limits`**.
- **Adaptive account-limit persistence:** learned per-account concurrency caps now persist across restarts via **`IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE`** and expire with **`IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS`**. Startup restores the learned state into the gateway, provider-profile reset clears the persisted store, and **`/provider/profile.json`** plus **`/debug/runtime.json`** now expose the persistence file + TTL.

### VOD
- **WebDAV mount helper UX:** added **`iptv-tunerr vod-webdav-mount-hint`** plus concrete platform-specific mount commands so the cross-platform WebDAV VOD surface is easier to mount on macOS, Windows, and Linux without manually translating the server URL each time.
- **Broader WebDAV client validation:** WebDAV coverage now exercises `OPTIONS` + multi-client `PROPFIND` behavior in unit tests and binary smoke, including a live `PROPFIND /Movies` pass against `iptv-tunerr vod-webdav` so macOS/Windows-style clients are less likely to regress silently.
- **Real read-path WebDAV smoke:** WebDAV validation now also covers file `HEAD` and byte-range `GET` through the real cached materializer path. `scripts/ci-smoke.sh` now stands up a local HTTP asset source and proves `iptv-tunerr vod-webdav` can mount, `PROPFIND`, `HEAD`, and range-read real bytes instead of only listing directories.
- **Explicit read-only WebDAV contract:** `iptv-tunerr vod-webdav` now cleanly rejects mutation methods with **`405 Method Not Allowed`** plus stable `Allow` / `DAV` headers, and the validation suite now covers file-level `PROPFIND`, movie/episode reads, and write-attempt rejection instead of only directory traversal.
- **Host validation tooling:** added a WebDAV client harness, bundle diff tool, a real macOS bare-metal smoke path with optional Wake-on-LAN, and a packaged Windows bare-metal smoke path so non-Linux validation no longer depends on ad hoc manual steps.

### Programming Manager
- **PM-001 / PM-002 foundations:** added a durable lineup-recipe layer via **`IPTV_TUNERR_PROGRAMMING_RECIPE_FILE`** and the first Programming Manager endpoints: **`/programming/categories.json`**, **`/programming/recipe.json`**, and **`/programming/preview.json`**. Tunerr now keeps the raw post-intelligence lineup separate from the final exposed lineup so category-first curation and saved custom order can sit between ingest intelligence and Plex-visible output.
- **PM-003 / PM-004 / PM-005 slice:** category-first curation is now mutable over HTTP, not just via recipe-file edits. **`/programming/categories.json`** supports bulk include/exclude/remove actions, **`/programming/channels.json`** supports exact channel include/exclude/remove actions, and `order_mode: "recommended"` now sorts channels into the requested Local/Entertainment/News/Sports/... taxonomy on the server. `programming/preview.json` also reports taxonomy bucket counts.
- **PM-006 / PM-007 slice:** Programming Manager now has durable manual order semantics and exact-match backup grouping. **`/programming/order.json`** supports server-side `prepend` / `append` / `before` / `after` / `remove` order mutations, **`/programming/backups.json`** reports strong same-channel sibling groups, and `collapse_exact_backups: true` can collapse exact `tvg_id` / `dna_id` siblings into one visible lineup row with merged backup stream URLs that survive refreshes.
- **PM-008 slice:** the dedicated control deck now has a real Programming lane. Operators can bulk include/exclude categories, pin or block exact channels, nudge manual order from the curated preview, toggle exact-backup collapse, inspect backup groups, and drill into the raw Programming payloads without hand-posting JSON.
- **PM-009 slice:** Programming Manager now has refresh/restart survival coverage. Tuner tests prove saved recipe mutations survive `UpdateChannels` churn, and `scripts/ci-smoke.sh` now restarts `serve` against a reshuffled catalog while reusing the same recipe file so curated lineup shape, persisted custom order, and exact-backup collapse are asserted across process restarts too.
- **Programming lane live preview:** the deck now exposes an in-place live HLS preview for the currently selected curated channel, backed by the same Tunerr `/stream/<id>?mux=hls` path the lineup will use. The Programming lane also now surfaces focused channel detail, upcoming guide rows, alternative sources, and the virtual-channel schedule horizon so operators can curate from concrete playback evidence instead of list metadata alone.
- **Harvest assist ranking:** added `/programming/harvest-assist.json`, which turns saved harvested lineups into ranked local-market recipe assists instead of forcing operators to mentally compare raw import previews. The report surfaces local-broadcast stem hits, exact guide/tvg matches, recommendation reasons, and ordered matched channel IDs for each harvested lineup title.

### Operator experiments promoted
- **HLS mux demo promoted into the deck:** the old `/debug/hls-mux-demo.html` experiment now supports `base`, `path`, `embed`, and `autoplay` query params so the Programming lane can reuse it as a real embedded preview surface instead of leaving HLS preview as a standalone lab page.
- **Next productization candidates documented:** with diagnostics capture now promoted into the deck/operator plane, the remaining high-signal experiment backlog is the next layer of in-product stream-compare/channel-diff execution and summarized bundle analysis instead of script-only invocation.

### Virtual channels
- **Deeper virtual publishing surfaces:** virtual channels now have `/virtual-channels/channel-detail.json` for focused rule/current-slot/schedule inspection and `/virtual-channels/guide.xml` for a synthetic XMLTV export over the rolling schedule horizon. This pushes the feature beyond “current-slot proxy” toward a real publishable TV-like surface without merging it blindly into the main HDHR lineup yet.

### Testing / CI
- **Release smoke widened again:** `scripts/ci-smoke.sh` now asserts harvest-assist recommendations plus the new virtual-channel detail and guide endpoints, so the richer `LH-006` and `PAR-006` surfaces are covered in the release gate instead of only by targeted tests.

### Testing / CI
- **Provider-pool + WebDAV smoke coverage:** `scripts/ci-smoke.sh` now exercises `vod-webdav-mount-hint` for macOS/Windows output, runs live WebDAV `OPTIONS` / `PROPFIND` smoke against `iptv-tunerr vod-webdav`, and validates the new Programming Manager category/channel mutation flow plus preview endpoints against a real temporary binary. Targeted gateway tests also now cover provider-account local rejection, lease release after successful playback, learned per-account caps, and provider-profile account-pool visibility.
- **Shared-relay binary proof:** `scripts/ci-smoke.sh` now stands up a throttled local HLS upstream, runs two same-channel `/stream/<id>` consumers against a real temp binary with ffmpeg disabled, and asserts `/debug/shared-relays.json` plus `X-IptvTunerr-Shared-Upstream: hls_go` on the joined client. This moves shared HLS relay reuse from unit-only confidence into the release smoke gate.
- **Provider-account rollover binary proof:** `scripts/ci-smoke.sh` now also stands up three synthetic Xtream-path credential variants across three overlapping channels, runs them against a real temp binary with `IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT=1`, and asserts `/provider/profile.json` shows three distinct active account leases. That turns the tester’s "second device did not roll over credentials" class into a release-gated binary proof instead of a unit-only guarantee.
- **Dead-remux fallback binary proof:** `scripts/ci-smoke.sh` now forces a real temp binary through a hung `ffmpeg` startup path for a same-host HLS channel, then asserts the request still completes with bytes and `/debug/stream-attempts.json` records `final_mode: hls_go`. `scripts/release-readiness.sh` now carries the matching focused fallback tests too.
- **Web UI auth/proxy binary proof:** `scripts/ci-smoke.sh` now starts a real `run --skip-index --skip-health` instance with the dedicated deck enabled, logs in through `/login`, reuses the session cookie to hit `/api/debug/runtime.json`, saves `/deck/settings.json` with the deck CSRF token, and fetches `/api/ops/workflows/diagnostics.json`. `scripts/release-readiness.sh` now runs the `internal/webui` auth/proxy suite explicitly too.
- **Expanded macOS host proof:** `scripts/mac-baremetal-smoke.sh` no longer stops at startup + WebDAV. It now also validates Xtream `get.php`, `xmltv.php`, virtual-channel `get_live_streams` / short-EPG output, virtual schedule, and direct virtual playback on a real Mac host, and `scripts/release-readiness.sh --include-mac` carries that full host lane.
- **Bare-metal platform smoke:** Linux can now cross-build and drive a real macOS smoke run end to end, while Windows gets a ready-to-run packaged PowerShell smoke path for the same startup/web UI/VOD contract once a host or VM is available.

### Streaming
- **Provider-account stream pooling:** deduplicated multi-account live channels now derive a stable provider-account identity per URL, prefer less-loaded accounts during upstream ordering, and can enforce **`IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT`** as a per-credential stream cap. When every candidate account for a channel is already at the cap, Tunerr now rejects the tune locally with HDHR-style **805** / HTTP **503** instead of waiting for a later upstream failure.
- **Adaptive provider-account limits:** provider-account pooling is no longer purely static. When a specific credential set starts returning upstream concurrency-limit responses, Tunerr now learns a tighter cap for that account, applies it on later tune attempts, and surfaces the learned state in **`/provider/profile.json`** as **`account_learned_limits`**.
- **Adaptive account-limit persistence:** learned per-account concurrency caps now persist across restarts via **`IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE`** and expire with **`IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS`**. Startup restores the learned state into the gateway, provider-profile reset clears the persisted store, and **`/provider/profile.json`** plus **`/debug/runtime.json`** now expose the persistence file + TTL.

### VOD
- **WebDAV mount helper UX:** added **`iptv-tunerr vod-webdav-mount-hint`** plus concrete platform-specific mount commands so the cross-platform WebDAV VOD surface is easier to mount on macOS, Windows, and Linux without manually translating the server URL each time.
- **Broader WebDAV client validation:** WebDAV coverage now exercises `OPTIONS` + multi-client `PROPFIND` behavior in unit tests and binary smoke, including a live `PROPFIND /Movies` pass against `iptv-tunerr vod-webdav` so macOS/Windows-style clients are less likely to regress silently.
- **Real read-path WebDAV smoke:** WebDAV validation now also covers file `HEAD` and byte-range `GET` through the real cached materializer path. `scripts/ci-smoke.sh` now stands up a local HTTP asset source and proves `iptv-tunerr vod-webdav` can mount, `PROPFIND`, `HEAD`, and range-read real bytes instead of only listing directories.
- **Explicit read-only WebDAV contract:** `iptv-tunerr vod-webdav` now cleanly rejects mutation methods with **`405 Method Not Allowed`** plus stable `Allow` / `DAV` headers, and the validation suite now covers file-level `PROPFIND`, movie/episode reads, and write-attempt rejection instead of only directory traversal.

### Programming Manager
- **PM-001 / PM-002 foundations:** added a durable lineup-recipe layer via **`IPTV_TUNERR_PROGRAMMING_RECIPE_FILE`** and the first Programming Manager endpoints: **`/programming/categories.json`**, **`/programming/recipe.json`**, and **`/programming/preview.json`**. Tunerr now keeps the raw post-intelligence lineup separate from the final exposed lineup so category-first curation and saved custom order can sit between ingest intelligence and Plex-visible output.
- **PM-003 / PM-004 / PM-005 slice:** category-first curation is now mutable over HTTP, not just via recipe-file edits. **`/programming/categories.json`** supports bulk include/exclude/remove actions, **`/programming/channels.json`** supports exact channel include/exclude/remove actions, and `order_mode: "recommended"` now sorts channels into the requested Local/Entertainment/News/Sports/... taxonomy on the server. `programming/preview.json` also reports taxonomy bucket counts.
- **PM-006 / PM-007 slice:** Programming Manager now has durable manual order semantics and exact-match backup grouping. **`/programming/order.json`** supports server-side `prepend` / `append` / `before` / `after` / `remove` order mutations, **`/programming/backups.json`** reports strong same-channel sibling groups, and `collapse_exact_backups: true` can collapse exact `tvg_id` / `dna_id` siblings into one visible lineup row with merged backup stream URLs that survive refreshes.
- **PM-008 slice:** the dedicated control deck now has a real Programming lane. Operators can bulk include/exclude categories, pin or block exact channels, nudge manual order from the curated preview, toggle exact-backup collapse, inspect backup groups, and drill into the raw Programming payloads without hand-posting JSON.
- **PM-009 slice:** Programming Manager now has refresh/restart survival coverage. Tuner tests prove saved recipe mutations survive `UpdateChannels` churn, and `scripts/ci-smoke.sh` now restarts `serve` against a reshuffled catalog while reusing the same recipe file so curated lineup shape, persisted custom order, and exact-backup collapse are asserted across process restarts too.

### Testing / CI
- **Provider-pool + WebDAV smoke coverage:** `scripts/ci-smoke.sh` now exercises `vod-webdav-mount-hint` for macOS/Windows output, runs live WebDAV `OPTIONS` / `PROPFIND` smoke against `iptv-tunerr vod-webdav`, and validates the new Programming Manager category/channel mutation flow plus preview endpoints against a real temporary binary. Targeted gateway tests also now cover provider-account local rejection, lease release after successful playback, learned per-account caps, and provider-profile account-pool visibility.

## [v0.1.26] — 2026-03-21

### Security / Web UI
- **Deck startup auth is usable again:** when `IPTV_TUNERR_WEBUI_PASS` is unset, the generated one-time password is now logged once at startup and shown on the localhost login page instead of silently locking operators out behind an unknown random secret.
- **Deck proxy header hygiene:** the dedicated `/api/*` proxy now strips deck `Authorization`, `Proxy-Authorization`, session `Cookie`, and CSRF headers before forwarding to the tuner, and direct script/API Basic-auth calls no longer mint browser sessions or spam deck activity history.

### Streaming
- **Upstream cookie containment:** stream proxy header copying now strips upstream `Set-Cookie` before responses are relayed back to Plex/clients, so provider session or clearance tokens do not get rebound onto the Tunerr origin.
- **Higher-level HLS dead-remux regression:** added an end-to-end `/stream/<id>` regression proving a dead non-transcode ffmpeg-remux path times out and falls back quickly enough to deliver bytes through the Go relay.

### Startup / HDHR
- **Lineup status loading signal:** during cold start, `/lineup_status.json` now reports `ScanInProgress=1` and `LineupReady=false`, and empty `/lineup.json` responses add `Retry-After: 5` to make the loading state more machine-readable without breaking HDHR-style `200` semantics.

### Licensing
- **Repository license set:** added an explicit `LICENSE` file for **AGPL-3.0-only** and linked it from the README.

## [v0.1.24] — 2026-03-21

### Streaming
- **Remux failure memory now sticks:** ffmpeg-remux failure preference is no longer erased by a later successful playlist fetch on the same host. Tunerr now keeps a dedicated remux-failure penalty for HLS hosts, so later tunes on the same provider/CDN path prefer the Go relay instead of retrying the same dead ffmpeg-remux path.
- **Non-transcode remux first-byte timeout:** ffmpeg-remux on HLS now has a dedicated first-byte deadline, so dead remux attempts fail over quickly instead of sitting for many seconds until Plex gives up first. New env: **`IPTV_TUNERR_FFMPEG_HLS_FIRST_BYTES_TIMEOUT_MS`**.

## [v0.1.23] — 2026-03-21

### Streaming
- **Cross-host HLS remux guardrail:** non-transcode HLS now skips ffmpeg remux and goes straight to the Go relay when a playlist references media/key/map/variant URLs on a different host than the playlist itself, avoiding static ffmpeg header/Host context leaking across host boundaries. Added **`IPTV_TUNERR_HLS_RELAY_ALLOW_FFMPEG_CROSS_HOST`** as an explicit opt-out.
- **Cross-host HLS segment context:** Go-relay HLS playlist/segment subrequests now inherit the current playlist as fallback **`Referer`** and **`Origin`** when the client did not provide them, which helps provider/CDN segment hosts that reject cross-host `.ts` fetches without playlist context.

## [v0.1.22] — 2026-03-21

### Testing / CI
- **Binary startup smoke:** added **`scripts/ci-smoke.sh`**, which builds a temporary binary, runs `serve` against synthetic full/empty catalogs, and asserts the real HTTP startup contract (`/readyz`, `/guide.xml`, `X-IptvTunerr-Guide-State`, `X-IptvTunerr-Startup-State`, lineup/discovery behavior). It now runs inside **`./scripts/verify`**, CI, and the GitHub release workflow before packaging.

## [v0.1.21] — 2026-03-21

### Guide / XMLTV
- **Visible guide-loading placeholders:** when `/guide.xml` is still on the startup placeholder path, programme titles now include **`(guide loading)`**, the XMLTV source metadata is marked as a loading placeholder, and each placeholder row carries a short description explaining that IPTV Tunerr is still building the full guide.
- **Startup guide contract hardening:** while the real merged guide is still building, `/guide.xml` now returns **`503 Service Unavailable`** with **`Retry-After: 5`**, **`X-IptvTunerr-Guide-State: loading`**, and the visible placeholder XMLTV body instead of a misleading **`200`** success response. HDHR **`discover.json`** / **`lineup.json`** / **`lineup_status.json`** stay compatible but add **`X-IptvTunerr-Startup-State: loading`** before the lineup is loaded.

## [v0.1.20] — 2026-03-21

### Guide / XMLTV
- **Startup guide refresh race:** XMLTV startup refresh now skips caching an empty guide when no lineup channels have been loaded yet, and `UpdateChannels` triggers a follow-up refresh as soon as the lineup arrives. This prevents `guide.xml` from getting stuck as an 82-byte empty `<tv>` document for the full cache TTL during startup.

### Operability
- **Evidence intake scaffold:** added **`scripts/evidence-intake.sh`** plus [how-to/evidence-intake](how-to/evidence-intake.md) and **`planning/README.md`** so real tester cases can be staged consistently under **`.diag/evidence/<case-id>/`** with debug-bundle output, PMS logs, Tunerr logs, pcaps, and analyst notes before running **`scripts/analyze-bundle.py`**.

## [v0.1.18] — 2026-03-20

### Guide / XMLTV
- **Guide-versus-lineup match report:** added **`GET /guide/lineup-match.json`** so operators can inspect emitted guide coverage against the active lineup without scraping XML manually. The report includes lineup and guide channel counts, exact guide-name matches, duplicate guide numbers/names, and sampled missing lineup rows.
- **Lineup integrity logging:** channel refreshes now log lineup-integrity counters including linked EPG rows, rows with streams, missing core fields, duplicate guide numbers, and duplicate channel ids so broken generator output is visible immediately in shard logs.
- **First-run mapping regression fix:** runtime EPG repair and guide-health flows now pass their trusted provider/XMLTV refs into the hardened guide-input loader, restoring automatic first-run mapping while keeping the narrowed remote guide allowlist.

### Testing / Performance
- **Faster tuner verification:** the slow HLS relative-URL relay regression test now overrides the long no-progress timeout and refresh sleep in-test only, cutting **`internal/tuner`** package runtime sharply without changing production relay behavior.

## [v0.1.17] — 2026-03-20

### Security
- **Code-scanning hardening sweep:** local guide/alias refs now require a regular file path, while remote guide/alias `http(s)` refs reject private/loopback hosts by default unless **`IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP=1`** is set intentionally for localhost/LAN sources. Stream-attempt and guide preview endpoints now clamp oversized `limit=` requests, Plex adaptation / upstream concurrency logs avoid echoing raw header-derived values, deck login redirects are normalized to path-only targets, logout now mirrors the session cookie security flags, mux decode JSON re-enables default HTML escaping, and debug stream responses set **`X-Content-Type-Options: nosniff`**.
- **Guide-input sandboxing:** local XMLTV / alias file refs now resolve only within the current working directory or explicit **`IPTV_TUNERR_GUIDE_INPUT_ROOTS`** entries, remote XMLTV / alias fetches must match configured provider/XMLTV/HDHR guide URLs (plus optional explicit **`IPTV_TUNERR_GUIDE_INPUT_ALLOWED_URLS`** entries), and guide parsing now reads through a single validated load path instead of reopening separate local-file sinks.

### Guide / XMLTV
- **Force lineup-to-guide matches:** **`IPTV_TUNERR_EPG_FORCE_LINEUP_MATCH=1`** keeps every lineup channel represented in emitted **`guide.xml`** even when **`IPTV_TUNERR_EPG_PRUNE_UNLINKED=1`** is enabled, by allowing unmatched channels to keep placeholder guide rows instead of disappearing from the guide output.

## [v0.1.16] — 2026-03-20

### Reliability
- **Windows release-build portability:** **`internal/hdhomerun/client.go`** no longer hard-casts the UDP socket fd to an **`int`** for **`SO_BROADCAST`**. OS-specific helpers now use the right handle type on Windows, so cross-builds for **`windows-amd64`** and **`windows-arm64`** succeed again in the release workflow.

## [v0.1.15] — 2026-03-20

### Web UI (Control Deck)
- **Host quarantine visibility:** **`internal/webui/deck.js`** **`summarizeProviderProfile`** includes **`quarantined_hosts`**, **`auto_host_quarantine`**, **`upstream_quarantine_skips_total`**; **Watch** / **Routing** lanes surface cumulative skips and current quarantine counts.

### Documentation
- **Project backlog audit:** [explanations/project-backlog.md](explanations/project-backlog.md) — **§1 Shipped** vs **§2 Still open** (avoids treating global hosts, quarantine, harness-index MVP, probe/Plex how-tos, cli ref, **`catchup-daemon`**, Ghost Hunter ops actions, etc. as missing). **opportunities.md:** HLS mux toolkit row marked reference-doc shipped; hidden-grab row marked partially addressed by operator actions.
- **Project backlog index:** [explanations/project-backlog.md](explanations/project-backlog.md) — single entry point for open work (links **[EPIC-live-tv-intelligence](epics/EPIC-live-tv-intelligence.md)**, **[memory-bank/opportunities.md](../memory-bank/opportunities.md)**, **[memory-bank/known_issues.md](../memory-bank/known_issues.md)**, **[docs-gaps.md](docs-gaps.md)**, [features](features.md) § limits). Linked from [docs/index](index.md), [explanations/index](explanations/index.md), **README** documentation map, **repo_map**.
- **Architecture Mermaid diagram:** [explanations/architecture.md](explanations/architecture.md) adds **Visual (Mermaid)** flowchart (provider → catalog → core + intelligence → registration/publishing); [docs-gaps.md](docs-gaps.md) **Medium** section cleared; [explanations/index](explanations/index.md) + [docs/index](index.md) point at diagram.
- **Docs gaps audit (2026-03-19):** [docs-gaps.md](docs-gaps.md) — cleared stale **High**/**Medium**/**Low** rows (canonical env map is [cli-and-env-reference](reference/cli-and-env-reference.md); architecture/VODFS/XMLTV/CF/run-vs-serve/glossary/runbooks/deployment already documented); **Resolved** table expanded; **Medium** keeps optional Mermaid polish for [architecture](explanations/architecture.md). [how-to/index](how-to/index.md) lists [mount-vodfs-and-register-plex-libraries](how-to/mount-vodfs-and-register-plex-libraries.md). [EPIC-live-tv-intelligence](epics/EPIC-live-tv-intelligence.md) / [EPIC-lineup-parity](epics/EPIC-lineup-parity.md) **next** / **PR-6** notes aligned with shipped guide/policy/EPG features. **`memory-bank/opportunities.md`**: superseded narrative on guide-health → policy (partially).
- **Plex onboarding:** new [how-to/connect-plex-to-iptv-tunerr.md](how-to/connect-plex-to-iptv-tunerr.md) (wizard vs **`-register-plex`** vs API; channelmap, **480** limit, empty-guide pitfalls); **README** how-to row; **`docs/how-to/index`**, **`docs/index`**; **`docs/docs-gaps.md`** high gap closed → **Resolved**; **`cli-and-env-reference`** **`IPTV_TUNERR_METRICS_ENABLE`** notes **Autopilot consensus** gauges.
- **`iptv-tunerr probe`:** new [how-to/interpreting-probe-results.md](how-to/interpreting-probe-results.md) (status table, **`get.php`** vs **`player_api`** patterns); **README** **`probe`** row; **runbook §4**; **`docs/docs-gaps.md`** moves probe row to **Resolved**; **`features.md`** row.
- **Harness index helper:** **`scripts/harness-index.py`** lists newest **`.diag/live-race`**, **`.diag/stream-compare`**, **`.diag/multi-stream`** runs (**`--json`**); **`memory-bank/commands.yml`** **`harness_index`**; harness how-tos + **`opportunities.md`** (MVP for unified **`.diag/`** index).
- **Stream-compare harness:** new [how-to/stream-compare-harness.md](how-to/stream-compare-harness.md); **runbook §9** lead-in; **`features.md`** row; cross-links with **live-race** / **multi-stream** how-tos; **`docs/docs-gaps.md`** **Resolved** table; backlog in **`memory-bank/opportunities.md`** (2026-03-22).
- **Live-race harness:** new [how-to/live-race-harness.md](how-to/live-race-harness.md); **runbook §7** lead-in; **`commands.yml`** **`live_race_harness`**; **`features.md`** harness rows link how-tos; cross-links with **multi-stream** how-to (fixed wrong §6 pointer → **§7** for live-race).
- **Multi-stream harness:** new [how-to/multi-stream-harness.md](how-to/multi-stream-harness.md) (quick start + pointers); linked from **`docs/how-to/index`**, **`docs/index`**, **`README`** (Documentation + Recent Changes), **runbook §10** + **runbooks index**.
- **HLS go-relay env:** **`cli-and-env-reference`** + **`plex-livetv-http-tuning`** now describe **`IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE`** as covering **autotune host penalty** (not only learned concurrency), matching **`shouldPreferGoRelayForHLSRemux`** in **`gateway_policy.go`**.
- **Autopilot URL semantics:** **`streamURLsSemanticallyEqual`** godoc in **`gateway_adapt.go`** and **Gateway / Autopilot** row in **`memory-bank/known_issues.md`** spell out what is folded (ports, trailing slash, scheme/host case) vs intentionally not folded (path segment case, exact query).
- **README:** expanded **Documentation** map (CHANGELOG, features, HR/mux references, architecture); **Kubernetes** probe guidance (**`/readyz`** / **`/healthz`** vs **`/discover.json`**); **Recent Changes** bullets for native mux header, profiles file, **HR-010** pool, live-race PMS snapshots; **Repo layout** lists **`internal/probe`**, **`internal/materializer`**; tuner endpoints mention readiness paths.
- **`docs/features.md`:** **`/healthz`** / **`/readyz`**, **`X-IptvTunerr-Native-Mux`**, named profile matrix row, **`provider_profile.json`** mux breadcrumbs, runtime **`URLs.ready`**, materializer scope, live-race harness + Plex sessions; **See also** links **CHANGELOG** / tuning docs.
- **`docs/index.md`:** quick entrypoints (**README**, **CHANGELOG**, **features**, CLI ref); runbooks row points at troubleshooting §8; **See also** → **repo_map**.
- **`docs/reference/index.md`:** **features** + **CHANGELOG** rows for discoverability.
- **LP / LTV epics:** [EPIC-lineup-parity](epics/EPIC-lineup-parity.md) **Implementation status** aligned with shipped **LP-007–LP-009** / **LP-002–LP-003**; [EPIC-live-tv-intelligence](epics/EPIC-live-tv-intelligence.md) **Current status** updated (**INT-003**, Autopilot URL/host, **`intelligence.autopilot`** on **`/provider/profile.json`**). [hybrid-hdhr-iptv](how-to/hybrid-hdhr-iptv.md) §6 LTV endpoint table.

### Live TV intelligence (LTV) / lineup parity (LP)
- **Autopilot global preferred hosts (LTV):** **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`** — provider-level host allowlist for **`reorderStreamURLs`** (after per-DNA memory, before consensus). **`/autopilot/report.json`**, **`intelligence.autopilot`**, **`/debug/runtime.json`**. Tests: **`TestGateway_reorderStreamURLs_autopilotGlobalPreferredHosts`**, **`TestAutopilot_report_includesGlobalPreferredHosts`**.
- **Autopilot host policy file (LTV):** **`IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE`** adds JSON-backed **preferred** and **blocked** upstream host policy on top of **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`**. Runtime/report surfaces now expose **`host_policy_file`** and **`global_blocked_hosts`**, and blocked hosts are skipped only when backup URLs remain.
- **INT-010 / active remediation (host quarantine):** optional **`IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE`** — when autotune is on, **`walkStreamUpstreams`** drops quarantined hosts (failure threshold + cooldown) if backup URLs remain. **`/provider/profile.json`**: **`auto_host_quarantine`**, **`upstream_quarantine_skips_total`** (cumulative), **`quarantined_hosts`**, **`penalized_hosts[].quarantined_until`**, **`remediation_hints`**. **`/debug/runtime.json`** **`tuner.provider_autotune_host_quarantine_*`**. **`IPTV_TUNERR_METRICS_ENABLE`**: **`iptv_tunerr_upstream_quarantine_skips_total`**. Tests: **`TestGateway_stream_skipsQuarantinedPrimaryUsesBackup`**, **`TestGateway_filterQuarantinedUpstreams_*`**, **`TestGateway_ProviderBehaviorProfile_upstreamQuarantineSkipsTotal`**. See **cli-and-env** / **`.env.example`**.
- **Ghost Hunter operator actions:** the localhost/LAN operator surface now exposes **`POST /ops/actions/ghost-visible-stop`** and **`POST /ops/actions/ghost-hidden-recover?mode=dry-run|restart`**. The same guarded helper path is reusable from the CLI and can be overridden with **`IPTV_TUNERR_GHOST_HUNTER_RECOVERY_HELPER`**.
- **LP-010 / LP-011:** named stream profiles can now prefer **`output_mux: "hls"`**, which starts a short-lived **ffmpeg-packaged HLS** session: Tunerr returns the generated playlist, serves follow-up packaged playlist/segment files under internal **`mux=hlspkg`** URLs, and keeps a background tuner hold while the packager is active. Docs/tests updated in **[transcode-profiles](reference/transcode-profiles.md)** and **[EPIC-lineup-parity](epics/EPIC-lineup-parity.md)**.
- **Hot-start by M3U group (`INT-006`):** **`IPTV_TUNERR_HOT_START_GROUP_TITLES`** — comma-separated case-insensitive substrings against **`group_title`**; hot-start **`reason=group_title`** after explicit **`HOT_START_CHANNELS`**, before Autopilot hit threshold. **`/debug/runtime.json`** **`tuner.hot_start_enabled`**, **`tuner.hot_start_min_hits`**, **`tuner.hot_start_group_titles`**. Tests: **`gateway_hotstart_test.go`**.
- **Prometheus — Autopilot consensus (when `IPTV_TUNERR_METRICS_ENABLE`):** **`iptv_tunerr_autopilot_consensus_dna_count`**, **`iptv_tunerr_autopilot_consensus_hit_sum`**, **`iptv_tunerr_autopilot_consensus_runtime_enabled`** (GaugeFuncs; same thresholds as consensus reporting). **`internal/tuner/prometheus_autopilot.go`** + tests **`prometheus_autopilot_test.go`**.
- **Autopilot consensus host (LTV, opt-in):** **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST`** — when enabled, **`reorderStreamURLs`** may prefer a **hostname** that multiple other **`dna_id`** rows already agree on (sum of **`hits`**, **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_DNA`**, **`IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_HIT_SUM`**) when there is no per-channel Autopilot match or remembered URLs no longer appear in the catalog; penalized hosts are skipped. **`/autopilot/report.json`** and **`intelligence.autopilot`** expose **`consensus_host`** / counts / runtime flag. Tests: **`TestAutopilot_consensusPreferredHost`**, **`TestGateway_reorderStreamURLs_autopilotConsensusHost`**.
- **Control Deck (LP-004):** **`internal/webui/deck.js`** derives a compact **provider summary** from flat **`/provider/profile.json`** (the UI previously expected a non-existent **`summary`** field). **Overview / Routing** cards and **Decision Board** show tuner/penalty/mux/autopilot counts (including **Autopilot consensus** from **`intelligence.autopilot`** via **`formatAutopilotConsensusMeta`** on the Operations card, plus **Watch** / **Confirmed wins**); **`remediation_hints`** surface as incidents (warn severity), watch items, a dedicated card, and routing meta.
- **HDHR UDP discovery (LP-001):** **`IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS`** accepts **literal IPv6** targets (unicast, multicast, or link-local with zone) in addition to IPv4; **`DiscoverLAN`** opens a **UDP6** socket and merges replies with the existing IPv4 broadcast path. **`parseLiteralUDPAddr`** / **`parseExtraDiscoverAddrs`**; tests in **`internal/hdhomerun/client_test.go`**.
- **`GET /provider/profile.json`**: **`remediation_hints`** — heuristic operator suggestions (**`code`**, **`severity`**, **`message`**, optional **`env`**) derived from CF blocks, penalized hosts, concurrency signals, and HLS/DASH mux error counters (advisory only; no automatic config changes). Tests: **`gateway_provider_profile_test.go`**.
- **Autopilot URL normalization:** remembered **`preferred_url`** matches catalog **`StreamURLs`** when paths differ only by a trailing slash, scheme/host casing differs, or default **:80** / **:443** is omitted (**`streamURLsSemanticallyEqual`** in **`gateway_adapt.go`**); tests **`TestStreamURLsSemanticallyEqual`**, **`TestGateway_stream_prefersAutopilotRememberedURL_normalizedTrailingSlash`**.
- **LP-012:** new [lineup-parity-lp012-closure](how-to/lineup-parity-lp012-closure.md) checklist; indexed from [docs/index](index.md) and [how-to index](how-to/index.md); **EPIC-lineup-parity** / **hybrid-hdhr** cross-links.
- **`GET /provider/profile.json`**: includes **`intelligence.autopilot`** (`enabled`, `state_file`, `decision_count`, `hot_channels` sample) when Autopilot memory is configured — aggregates provider-runtime + learned playback signals for operator UIs. **`stream-investigate`** workflow actions include **`/autopilot/report.json`** and **`/ops/actions/autopilot-reset`**.

### Testing
- **CI / verify:** **`scripts/verify-steps.sh`** now runs **`bash -n`** on **`scripts/*.sh`** and **`python3 -m py_compile`** on **`scripts/*.py`** so harness/report syntax errors fail **`./scripts/verify`** before **`go test`**.
- **`internal/tuner`**: **`TestGateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry`** waits on a playlist **509→retry→OK** signal instead of a fixed sleep; **`TestGateway_shouldPreferGoRelayForHLSRemux_hostPenalty`** adds **`autotune_off_no_penalty_signal`** subtest.
- **`internal/probe`**: unit tests for **`Probe`** (path classification, HTTP content-types, body sniff, redirects) plus **`Lineup`**, **`LineupHandler`**, **`DiscoveryHandler`** (`probe_test.go`).
- **`internal/materializer`**: unit tests for **`Stub`**, **`DownloadToFile`** (SSRF guard, full GET, ranged GET, HTTP errors), **`DirectFile`**, and **`Cache`** materialization (`materializer_test.go`). HLS/ffmpeg paths remain integration-only.

### Operability
- **`GET /readyz`**: Kubernetes-oriented readiness JSON — **503** `not_ready` until **`UpdateChannels`** has live channels, then **200** `ready` (same gate as **`/healthz`**, which returns **`loading`** / **`ok`** plus **`source_ready`**). Example **`k8s/`** manifests probe **`/readyz`** for **`readinessProbe`**; **`/discover.json`** remains a better **liveness** target during long first catalog builds. See runbook §8 and **`TestServer_readyz`**.
- **Startup visibility for `run`:** the tuner now binds before long catalog and guide startup work completes, so **`/healthz`** and **`/readyz`** report **`loading`** / **`not_ready`** instead of looking dead during big provider indexes. Catalog startup also logs phase timings (`provider probe + rank`, `index provider ...`, free-source fetch, HDHR merge, EPG repair, smoketest), and **`IndexFromPlayerAPI`** now logs per-step durations for stream-base resolve, live, VOD, and series fetches.

### Reliability / Plex ops (work breakdown slices)

- **HLS go-relay vs ffmpeg remux:** when **`IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE`** is enabled (default), Tunerr now also prefers the Go relay for the current stream URL if that upstream host already has a **non-zero penalty** (e.g. prior **`ffmpeg_hls_failed`** on the same host), not only when concurrency caps are learning/hitting. Call site: **`shouldPreferGoRelayForHLSRemux(streamURL)`** in **`gateway_policy.go`** / **`relaySuccessfulHLSUpstream`**. Tests: **`TestGateway_shouldPreferGoRelayForHLSRemux_hostPenalty`**, **`TestGateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry`**.
- **Provider-pressure HLS handling:** upstream playlist refresh now treats **`509`** as a concurrency signal, learns provider limits from playlist failures as well as stream-open failures, retries transient playlist limit hits with bounded backoff, and prefers the Go HLS relay over **ffmpeg remux** for non-transcode HLS once provider pressure has already been observed. This reduces “second stream kills the first” churn on providers that are sensitive to extra remux-side playlist/segment fetches.
- **Multi-stream contention harness:** added **`scripts/multi-stream-harness.sh`** plus **`scripts/multi-stream-harness-report.py`** to reproduce the “first stream dies when the second starts” class against a real tuner with staggered live pulls, periodic **`/provider/profile.json`** / **`/debug/stream-attempts.json`** / **`/debug/runtime.json`** snapshots, optional Plex **`/status/sessions`** capture, and a compact sustained-read vs premature-exit summary.
- **Live-race harness evidence (HR-002 / HR-003):** **`scripts/live-race-harness.sh`** can now poll Plex **`/status/sessions`** during the run window when **`PMS_URL`** + **`PMS_TOKEN`** (or the existing Tunerr/Plex env aliases) are set, storing XML snapshots under **`pms-sessions/`**. **`live-race-harness-report.py`** summarizes observed player titles/products/platforms and live session IDs, so startup runs can correlate real Plex client classes with Tunerr adaptation/log output instead of inferring everything from raw logs.
- **WebSafe startup gate (HR-001):** With **`IPTV_TUNERR_WEBSAFE_REQUIRE_GOOD_START`**, ffmpeg TS prefetch uses a **sliding window** at **`STARTUP_MAX_BYTES`** instead of releasing at the byte cap without a **video keyframe + AAC**; **`startup-gate buffered=`** adds **`release=`** (`min-bytes-idr-aac-ready`, `read-ended-partial-*`, …). **Keyframe scan:** **H.264 IDR** (Annex B) **or** **HEVC IRAP** NAL types **16–21** via **`containsAnnexBVideoKeyframe`**. Opt-in legacy cap: **`IPTV_TUNERR_WEBSAFE_STARTUP_MAX_FALLBACK_WITHOUT_IDR`**. **`trimTSHeadToMaxBytes`** + HEVC/H.264 tests; **`live-race-harness-report.py`** parses optional **`release=`**. **`/debug/runtime.json`** → **`tuner.websafe_*`**. Docs: **[plex-livetv-http-tuning](reference/plex-livetv-http-tuning.md)**, **[cli-and-env-reference](reference/cli-and-env-reference.md)**, runbook §6.
- **Plex Web regression template (HR-002):** **[plex-client-compatibility-matrix](reference/plex-client-compatibility-matrix.md)** adds an **HR-002** section (agreed DVR/channel table, pass criteria vs probe + **`release=`** + PMS logs).
- **Client matrix + QA (HR-003):** **[plex-client-compatibility-matrix](reference/plex-client-compatibility-matrix.md)** defines tier-1 Plex clients (Web Firefox/Chrome, LG webOS, iOS, Shield as Android TV proxy), **`CLIENT_ADAPT`** client classes vs expected paths, repo **`go test`** commands, optional external Plex Web probe notes, and a manual results table; runbook **§10** links it.
- **Client adaptation (HR-004):** after a **non-WebSafe** adaptation choice fails with **`all_upstreams_failed`** or **`upstream_concurrency_limited`**, Tunerr registers a **per-session** WebSafe sticky (channel + Plex session/client id) until **`IPTV_TUNERR_CLIENT_ADAPT_STICKY_TTL_SEC`** (default 4h; clamped). Knobs **`IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK`**, **`IPTV_TUNERR_CLIENT_ADAPT_STICKY_LOG`**; **`/debug/runtime.json`** → **`tuner.client_adapt_sticky_*`**. See **[plex-livetv-http-tuning](reference/plex-livetv-http-tuning.md)**.
- **Lineup / EPG hygiene (HR-005):** Reference **[lineup-epg-hygiene](reference/lineup-epg-hygiene.md)**; **`index`** runs **`dedupeByTVGID`** again after free-source + HDHR hardware merges; **`IPTV_TUNERR_DEDUPE_BY_TVG_ID`** (default on) disables all tvg-id merging when **`false`**; **`/debug/runtime.json`** → **`tuner.dedupe_by_tvg_id`**.
- **Catalog stable live order (HR-006):** **`catalog.ReplaceWithLive`** sorts live channels by **`channel_id`** (tie-break **guide_number**, **guide_name**) before storing so **`catalog.json`** / lineup order do not shuffle when the provider reorders the M3U.
- **Transcode policy (HR-007):** **`IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE`** applies on top of **`IPTV_TUNERR_STREAM_TRANSCODE`** **`off`/`on`/`auto`** (per-channel remux vs transcode), not only **`auto_cached`**; **`gateway: transcode policy`** logs when the file changes the computed base. Runtime snapshot lists **`transcode_overrides_file`** / **`profile_overrides_file`**. Docs: **[plex-livetv-http-tuning](reference/plex-livetv-http-tuning.md)**, **[cli-and-env-reference](reference/cli-and-env-reference.md)**.
- **Named stream profile matrix (LP-010 / LP-011):** **`IPTV_TUNERR_STREAM_PROFILES_FILE`** can define product-facing profile names with **`base_profile`**, **`transcode`**, and preferred **`output_mux`** (`mpegts` / `hls` / `fmp4`). `?profile=<name>`, `IPTV_TUNERR_PROFILE`, and profile overrides can reference those names once loaded, and ffmpeg relay honors the profile’s preferred output mux when no explicit `?mux=` is supplied. Docs: **[transcode-profiles](reference/transcode-profiles.md)**.
- **HTTP client:** **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS`** and **`IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC`** tune the shared **`internal/httpclient`** transport (with **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST`**); **`/debug/runtime.json`** includes **`tuner.http_max_idle_conns`** and **`tuner.http_idle_conn_timeout_sec`**. Reference **[plex-livetv-http-tuning](reference/plex-livetv-http-tuning.md)**; runbook §9 notes **HR-008** / **HR-009** / **HR-010** checklists.

### Maintainability
- **Gateway struct / packager path:** packaged-HLS session state now lives in a real in-process manager (`ffmpegHLSPackagerSession`) for named-profile **`output_mux: "hls"`** playback, replacing the earlier placeholder note about undefined packager fields.
- **Gateway upstream walk:** **`internal/tuner/gateway_stream_response.go`** now holds non-OK upstream handling and success relay branches, leaving **`walkStreamUpstreams`** in **`gateway_stream_upstream.go`** as the top-level orchestration loop.
- **Gateway:** Cloudflare recovery on the live upstream walk lives in **`internal/tuner/gateway_upstream_cf.go`** (**`tryRecoverCFUpstream`**) and is called from **`walkStreamUpstreams`**.
- **Docs:** [architecture](explanations/architecture.md) links updated for **`cmd_*`** layout, **`gateway_*`** modules, and softer CLI “tension” note; [reference index](reference/index.md) calls [cli-and-env-reference](reference/cli-and-env-reference.md) canonical. **`memory-bank/opportunities.md`**: superseded stale items (“missing” CLI reference page; pre-repo **`internal/indexer`**).
- **Docs:** [cli-and-env-reference](reference/cli-and-env-reference.md) clarifies scope of **`IPTV_TUNERR_HTTP_*`** idle-pool env vars (which subsystems use **`internal/httpclient`** vs mux **`seg=`** exception).
- **Docs:** [potential_fixes](potential_fixes.md) “current implementation” aligns with post-**`gateway_*`** split (symbol links, not stale **`gateway.go`** line anchors); references [plex-livetv-http-tuning](reference/plex-livetv-http-tuning.md) / troubleshooting runbook.
- **Backlog:** **`memory-bank/opportunities.md`** superseded duplicate **XMLTV `/guide.xml` cache** rows (**2026-02-24** / **2026-02-25**) now that **`internal/tuner/xmltv.go`** ships merged-guide TTL cache + tests (**`TestXMLTV_cacheHit`**).
- **Backlog:** **`memory-bank/opportunities.md`** superseded **2026-02-25** smoketest “no disk cache” row — **`IPTV_TUNERR_SMOKETEST_CACHE_FILE`** / **`_TTL`** + **`internal/indexer/smoketest_cache.go`** already shipped.
- **Docs:** [plex-livetv-http-tuning](reference/plex-livetv-http-tuning.md) links **`X-IptvTunerr-Native-Mux`** to **[hls-mux-toolkit](reference/hls-mux-toolkit.md)**; [hybrid-hdhr-iptv](how-to/hybrid-hdhr-iptv.md) See also → mux toolkit + troubleshooting. **`k8s/README.md`** verify snippet includes **`/readyz`** / **`/healthz`**.
- **Docs:** [plex-livetv-http-tuning](reference/plex-livetv-http-tuning.md) lists all major **`httpclient`** consumers (Plex, Emby, provider, HDHR, EPG pipeline, health, probe) and notes mux **`seg=`** exception; HR-007 precedence pointer updated to **`gateway_adapt.go`** / **`gateway_policy.go`**.
- **Shared HTTP client:** **`httpclient.WithTimeout`** replaces raw timeout-only clients in **`internal/tuner/epg_pipeline.go`** (**`httpClientOrDefault`**), **`internal/health`**, **`internal/probe`**, **`internal/plex`** (**`dvr.go`**, **`library.go`** **`plexHTTPClient`**), **`internal/provider/probe.go`**, and **`internal/emby/register.go`** (**`newHTTPClient`**) so media-server registration, provider ranking, and guide fetches share tuned idle pools (**HR-010**). **`internal/tuner/mux_http_client.go`** still builds a custom **`&http.Client`** for redirect policy.
- **HDHR client HTTP:** **`internal/hdhomerun`** (**`FetchDiscoverJSON`**, **`FetchLineupJSON`**, **`FetchGuideXML`**) and **`iptv-tunerr hdhr-scan`** use **`internal/httpclient`** (shared transport / idle pool) instead of ad hoc **`http.Client`** timeouts.
- **Lineup parity doc:** [EPIC-lineup-parity](epics/EPIC-lineup-parity.md) adds an **implementation status** section (**LP-001** / **LP-010** / dashboard / remaining multi-PR items).
- **Gateway layout (INT-006):** **`internal/tuner/gateway.go`** holds the **`Gateway`** struct and context keys; **`gateway_servehttp.go`** owns **`ServeHTTP`** (tuner slot + orchestration); **`gateway_stream_upstream.go`** owns **`walkStreamUpstreams`** (upstream URL loop and DASH/HLS/raw dispatch); **`gateway_mux_ratelimit.go`** owns mux-segment rate limiting and outcome counters.
- **Shared HTTP client (INT-001 tail):** **`internal/materializer`** default/nil client paths and tuner **loopback** stream self-fetch use **`internal/httpclient`** instead of **`http.DefaultClient`** so timeouts and idle pooling match the rest of the binary.
- **CLI helpers (INT-005 slice):** moved **`parseCSV`**, **`firstNonEmpty`**, **`hostPortFromBaseURL`** from **`main.go`** to **`cmd_util.go`** so **`main`** stays a thin dispatcher.

### Testing / operator tooling
- **Gateway profiles:** **`gateway_profiles_test.go`** covers **`loadNamedProfilesFile`** (empty path, invalid JSON, omitted **`base_profile`**) and **`resolveProfileSelection`** / **`preferredOutputMuxForProfile`** for named profiles vs built-ins vs unknown labels (**`IPTV_TUNERR_STREAM_PROFILES_FILE`**).
- **Mux regression (HLS + DASH):** **`internal/tuner/testdata/stream_compare_{hls,dash}_mux_capture_*`** (harness-style captures) with **`TestRewriteHLSPlaylistToGatewayProxy_streamCompareCaptureGolden`** and **`TestRewriteDASHManifestToGatewayProxy_streamCompareCaptureGolden`**. DASH golden asserts **SegmentTemplate → SegmentList** expansion (**`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE=1`**) plus proxy rewrite. **`.diag/`** gitignored. Runbook notes how to promote captures to **testdata**.

### Web UI
- **Deck build fixes:** `logout` activity logging no longer referenced a non-existent **`Server.User`** field; rate-limit responses use **`strconv`** for **`Retry-After`**. Tests construct **`DeckSettings`** explicitly and expect **401** on failed login (matches **`renderLogin`** status).
- **Dedicated control deck (`internal/webui`)**: `serve` / `run` now start a separate operator dashboard on **`48879`** by default (`0xBEEF`), with a single-origin `/api/*` proxy over the tuner server.
- **Runtime settings snapshot**: added **`/debug/runtime.json`** so operators can inspect effective tuner/guide/provider/HDHR/web UI settings without spelunking env files or logs.
- **Web UI envs**: added **`IPTV_TUNERR_WEBUI_DISABLED`**, **`IPTV_TUNERR_WEBUI_PORT`**, and **`IPTV_TUNERR_WEBUI_ALLOW_LAN`**. The older `/ui/` pages on the tuner port remain available.
- **Deck auth + persisted memory**: the dedicated deck now opens on its own login page with cookie-backed sessions, while still accepting direct HTTP Basic auth for scripts. If **`IPTV_TUNERR_WEBUI_PASS`** is unset, Tunerr generates a one-time startup password instead of falling back to `admin/admin`; **`IPTV_TUNERR_WEBUI_STATE_FILE`** now persists only non-secret deck preferences plus server-derived operator activity, not deck credentials or browser-authored telemetry.
- **Operator activity memory**: the dedicated deck now keeps a shared activity log (`/deck/activity.json`) for deck logins, logouts, memory clears, and deck-triggered actions, and surfaces that timeline inside overview/ops so the control plane shows what operators actually did, not just what the backend reported.
- **Operator actions + workflows**: the deck now drives safe control endpoints under **`/ops/actions/*`** plus workflow/playbook endpoints under **`/ops/workflows/*`** (`guide-repair`, `stream-investigate`, `ops-recovery`), and the UI exposes them with action docks, workflow modals, and signal boards instead of treating operations as raw payloads only.
- **Session telemetry**: the deck now keeps a browser-local rolling history of key signals (guide, stream, recorder, ops) and renders trend cards/sparklines so operators can see direction of travel instead of only the latest snapshot.
- **Sticky deck prefs**: the integrated dashboard now persists mode, refresh cadence, selected raw endpoint, and recent telemetry samples in browser-local storage, with an explicit “Clear Deck Memory” control.
- **Shared deck memory**: the dedicated web UI now exposes a small in-process telemetry endpoint (`/deck/telemetry.json`) so trend cards can use shared operator history across reloads/browser sessions hitting the same deck, instead of only per-browser state.
- **Editable deck controls**: the dedicated web UI now exposes **`/deck/settings.json`** only for non-secret deck preferences such as default refresh cadence, while auth remains env/startup-controlled and the Settings lane surfaces the live deck-security posture directly.
- **Deck mutation hardening**: state-changing deck requests now require a session-bound **`X-IPTVTunerr-Deck-CSRF`** token for cookie-backed sessions, sign-out is a deliberate **POST** flow instead of a GET, and the runtime snapshot now exposes the deck CSRF header alongside login-failure limits so auth/session behavior is operator-visible.
- **Expanded control surface**: the Settings lane now inventories grouped runtime/config posture (deck security, tuner/mux, guide pipeline, provider ingress, HDHR/media hooks, action/workflow atlas) instead of acting like a thin summary list, and the raw endpoint index is grouped by subsystem for faster drill-down.
- **Live shared-replay operator control**: the dedicated deck now shows the active shared replay window in its `Tuner + transport` posture card and can apply a new `shared_relay_replay_bytes` value for future shared sessions through the new localhost-only `/ops/actions/shared-relay-replay` tuner action. `/ops/actions/status.json` advertises that action and `/debug/runtime.json` updates in place so operators can see the change immediately.

### Streaming / HLS (Tunerr-native mux)
- **Mux manifest metrics:** Prometheus now includes **`iptv_tunerr_mux_manifest_outcomes_total`** and **`iptv_tunerr_mux_manifest_request_duration_seconds`** for native mux playlist/MPD handling (direct **`/stream/<id>?mux=hls|dash`** entry rewrites plus nested manifest targets served through **`seg=`**). This separates manifest rewrite failures / upstream manifest HTTP errors from segment-relay outcomes in **`iptv_tunerr_mux_seg_*`**.
- **DASH / HLS mux polish:** optional **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE`** (+ **`_MAX_SEGMENTS`**) expands **`SegmentTemplate`** to **`SegmentList`** (uniform duration, paired closing tags, **`SegmentTimeline`** via quote-aware **`<S>`** scanner—nested **`<S>…</S>`** balanced, **`>`** in quoted attrs OK, outer **`<S>`** row only when nested); plus **`$Time$`** / **`$Number$`**, **`$Number%0Nd$`**; leading **UTF-8 BOM** strip on rewrite; HLS **`URI='...'`** rewrite; DASH URL attributes **single- or double-quoted**; **`dashSegQueryEscape`** keeps **`%`** in **`$Number%05d$`**-style templates; HLS playlists normalize non-standard **`#EXTINF:...,BYTERANGE=...`** into separate **`#EXTINF`** + **`#EXT-X-BYTERANGE`** lines (bis-style tags); **`/debug/runtime.json`** includes **`tuner.hls_mux_dash_expand_*`** echo.
- **Nice-to-have mux pack:** DASH **`SegmentTemplate`** URLs keep **`$Number$` / `$RepresentationID$`** unescaped in **`seg=`**; LL-HLS-style **`URI=`** tags (**`PRELOAD-HINT`**, **`RENDITION-REPORT`**, **`PART`**) + conservative same-line **`#EXTINF`** media; optional **`IPTV_TUNERR_HTTP_ACCEPT_BROTLI`**; Prometheus **`iptv_tunerr_mux_seg_request_duration_seconds`** + optional **`IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS`**; Autopilot-driven **`IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS`**; docs **[hls-mux-ll-hls-tags](reference/hls-mux-ll-hls-tags.md)**; dependency **`github.com/andybalholm/brotli`** (vendored).
- **Redirect SSRF hardening:** **`seg=`** fetches use a dedicated HTTP client that validates **every redirect hop** (scheme + same literal/resolved private rules as the initial URL, max **10** hops). Blocked private hops → **403**; other policy failures → **502** + **`X-IptvTunerr-Hls-Mux-Error: redirect_rejected`**.
- **DASH rewrite:** relative **`media=`** / **`initialization=`** and **`<BaseURL>`** text resolve against the manifest URL and an ordered **`<BaseURL>`** chain; **`$`** template placeholders are not rewritten.
- **Adaptive seg cap:** **`IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO`** (+ window / per-hit / cap envs) adds temporary bonus concurrent **`seg=`** slots after **503** limit rejections when **`IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT`** is not set.
- **Access log:** **`IPTV_TUNERR_HLS_MUX_ACCESS_LOG`** appends one JSON line per successful **`seg=`** (redacted target, duration). **ADR** [0005](adr/0005-hls-mux-no-disk-packager.md) documents **no in-process disk packager**; use external packagers if needed.
- **Docs:** [Observability: Prometheus + OTEL bridge](explanations/observability-prometheus-and-otel.md) (scrape **`/metrics`** with a collector). Golden fixture **`internal/tuner/testdata/hls_mux_small_playlist.golden`** + integration tests for **302→private** and **chunked** upstream.
- **Operator reference:** [docs/reference/hls-mux-toolkit.md](reference/hls-mux-toolkit.md) — diagnostic headers, stream-attempt statuses, **`curl`** snippets, and a categorized **enhancement backlog** for future mux work.
- **Native mux visibility:** responses set **`X-IptvTunerr-Native-Mux: hls`** or **`dash`** on successful **entry** playlist/MPD rewrites, **`seg=`** relays (**200**/**206**/**304**), and internal **`serveNativeMuxTarget`** paths; included in **`Access-Control-Expose-Headers`** when **`IPTV_TUNERR_HLS_MUX_CORS`** is enabled.
- **Provider-profile mux breadcrumbs:** **`/provider_profile.json`** now exposes **`last_hls_mux_outcome`** / **`last_dash_mux_outcome`** with matching redacted target URLs + timestamps so operators can see the most recent native-mux success/failure mode without log scraping.
- **`?mux=hls`** on **`/stream/<channel>`**: returns a rewritten **HLS playlist** whose media lines point back through Tunerr (`/stream/<id>?mux=hls&seg=<encoded-upstream-url>`), and fetches segments/variants through the same proxy. **MPEG-TS relay** remains the default when `mux` is omitted or set to `mpegts`.
- **`IPTV_TUNERR_STREAM_PUBLIC_BASE_URL`**: optional prefix (e.g. `http://192.168.1.10:5004`) so playlist lines use **absolute** Tunerr URLs; exposed in **`/debug/runtime.json`** (`tuner.stream_public_base_url`).
- **How-to:** [docs/how-to/hls-mux-proxy.md](how-to/hls-mux-proxy.md); transcode reference updated in [transcode-profiles.md](reference/transcode-profiles.md).
- **`?mux=hls` playlist rewrite:** tag lines with **`URI="..."`** (e.g. **`#EXT-X-KEY`**, **`#EXT-X-SESSION-KEY`**, **`METHOD=SAMPLE-AES`**, **`#EXT-X-MAP`**, **`#EXT-X-STREAM-INF`**) are rewritten through the same Tunerr proxy as media lines so keys/init/variant playlists can use upstream auth and cookies; **`uri="`** (lowercase) is recognized; empty **`URI=""`** is not rewritten to a bogus proxy URL.
- **Non-HTTP HLS mux targets:** direct **`?mux=hls&seg=`** requests with unsupported target schemes (for example **`skd://...`**) now return **`400 Bad Request`** with a clear error string instead of a generic `502`, and header **`X-IptvTunerr-Hls-Mux-Error: unsupported_target_scheme`**. When **`IPTV_TUNERR_HLS_MUX_CORS`** is enabled, that header is listed in **`Access-Control-Expose-Headers`** so browser devtools can read it on failed fetches.
- **Upstream HTTP errors on HLS mux segments:** when the CDN returns **4xx/5xx** (or other non-success statuses) for a **`?mux=hls&seg=`** fetch, Tunerr **passes through that status code** (and up to **8 KiB** of the upstream body) instead of always **`502`**. Response includes **`X-IptvTunerr-Hls-Mux-Error: upstream_http_<status>`**. Stream-attempt **`finalStatus`** uses **`hls_mux_upstream_http_<status>`**.
- **Upstream forwarding:** **`Range`** / **`If-Range`** / **`If-None-Match`** / **`If-Modified-Since`** are forwarded on gateway upstream requests (with **`Cookie`**, **`Referer`**, **`Origin`**). **`?mux=hls&seg=`** responses pass through **`206 Partial Content`** + **`Content-Range`**, and **`304 Not Modified`** for conditional segment fetches.
- **`IPTV_TUNERR_HLS_MUX_CORS`**: optional permissive CORS + **`OPTIONS`** preflight for **`?mux=hls`** (playlist + **`seg=`**); exposed in **`/debug/runtime.json`** as **`tuner.hls_mux_cors`**.
- **HLS mux segment concurrency:** concurrent **`?mux=hls&seg=`** proxies are capped (default **effective tuner limit × 8** via **`IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER`**). Override with **`IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT`**. **`provider_profile.json`** (and provider profile detail) includes **`hls_mux_seg_in_use`** / **`hls_mux_seg_limit`**. Over-cap requests return **`503`** with **`X-HDHomeRun-Error: 805`** (same signal as main tuner exhaustion).
- **HLS mux hardening + ops:** decoded **`seg=`** length cap (**`IPTV_TUNERR_HLS_MUX_MAX_SEG_PARAM_BYTES`**, default 256 KiB); optional block for **literal** private/loopback/link-local IPs (**`IPTV_TUNERR_HLS_MUX_DENY_LITERAL_PRIVATE_UPSTREAM`**, hostnames not resolved); tunable upstream error preview (**`IPTV_TUNERR_HLS_MUX_UPSTREAM_ERR_BODY_MAX`**). Playlist parsing avoids **`bufio.Scanner`** token limits on long lines. **`Accept: application/json`** returns **`{"error","message"}`** on mux client errors. **`X-Request-Id` / `X-Correlation-Id` / `X-Trace-Id`** forward to upstream with other mux headers. **`HEAD`** on **`seg=`** uses upstream **HEAD** (playlist rewrite skipped when there is no body). **`provider_profile.json`** adds **`hls_mux_seg_*`** outcome counters; **`/debug/runtime.json`** includes the new mux env keys (raw env strings).
- **Native DASH (experimental):** **`?mux=dash`** on **`/stream/<channel>`** when the upstream is an **MPD** rewrites absolute **`media=`** / **`initialization=`** / **`BaseURL`** URLs through **`?mux=dash&seg=`** (`internal/tuner/gateway_dash.go`). Shares the same **`seg=`** concurrency pool and SSRF knobs as HLS. Profile JSON includes **`dash_mux_seg_*`** counters.
- **DNS-assisted SSRF guard:** **`IPTV_TUNERR_HLS_MUX_DENY_RESOLVED_PRIVATE_UPSTREAM`** resolves **`seg=`** hostnames and blocks if any address is private/link-local/loopback (**DNS errors are logged and treated as pass-through**).
- **Per-IP seg rate limit:** **`IPTV_TUNERR_HLS_MUX_SEG_RPS_PER_IP`** (token bucket; **429** + **`seg_rate_limited`**).
- **Observability:** **`hls_mux_diag=...`** in gateway logs on mux client errors and upstream pass-through responses; Prometheus **`iptv_tunerr_mux_seg_outcomes_total{mux,outcome}`** when **`IPTV_TUNERR_METRICS_ENABLE`** — **`GET /metrics`**.
- **HTTP client tuning:** **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST`** (shared default transport **`MaxIdleConnsPerHost`**).
- **Operator tooling:** **`POST /ops/actions/mux-seg-decode`** (**`{"seg_b64":"..."}`** → redacted URL, localhost/LAN UI policy); **`IPTV_TUNERR_HLS_MUX_WEB_DEMO`** serves **`/debug/hls-mux-demo.html`** (hls.js); **`scripts/hls-mux-soak.sh`** helper; **fuzz** tests on playlist + MPD rewriters.
- **Prometheus + docs sweep:** added Prometheus wiring and mux outcome counters across the gateway/toolkit path, plus observability docs in [explanations/observability-prometheus-and-otel.md](explanations/observability-prometheus-and-otel.md) and ADR [0005](adr/0005-hls-mux-no-disk-packager.md) clarifying the no-in-process-packager stance.
- **Dependencies / vendor:** **`github.com/prometheus/client_golang`**, **`golang.org/x/time/rate`**; **`go mod vendor`** updated.

### Provider EPG + SQLite (incremental follow-ups)
- **`IPTV_TUNERR_PROVIDER_EPG_INCREMENTAL`** + suffix tokens `{from_unix}` / `{to_unix}` / `{from_ymd}` / `{to_ymd}` on **`IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX`** (horizon from SQLite when available).
- **`IPTV_TUNERR_EPG_SQLITE_INCREMENTAL_UPSERT`**: overlap-window upsert sync instead of full table replace.
- **`/guide/epg-store.json`**: includes **`incremental_upsert`** and **`provider_epg_incremental`** when SQLite is enabled.

### Lineup parity — remaining LP stories (LP-002 / LP-009 / LP-010 / LP-011 / LP-012)

- **LP-002**: **`IPTV_TUNERR_HDHR_LINEUP_URL`** (+ optional **`IPTV_TUNERR_HDHR_LINEUP_ID_PREFIX`**) merges physical **`lineup.json`** into the catalog during **`iptv-tunerr index`** (`internal/hdhomerun/LiveChannelsFromLineupDoc`).
- **LP-009**: **`IPTV_TUNERR_EPG_SQLITE_MAX_BYTES`** / **`IPTV_TUNERR_EPG_SQLITE_MAX_MB`** post-sync SQLite size enforcement (`Store.EnforceMaxDBBytes`); **`max_bytes`** on `/guide/epg-store.json`.
- **LP-010 / LP-011**: **`buildFFmpegStreamOutputArgs`**, optional **`?mux=fmp4`** on HLS ffmpeg relay (fragmented MP4 when transcoding); default remains MPEG-TS.
- **LP-012**: [how-to/hybrid-hdhr-iptv.md](how-to/hybrid-hdhr-iptv.md).

### HDHR EPG merge (LP-003 partial)
- **`IPTV_TUNERR_HDHR_GUIDE_URL`** / **`IPTV_TUNERR_HDHR_GUIDE_TIMEOUT`**: optional fetch of a physical HDHomeRun-style `guide.xml`, merged after provider + external EPG; non-overlapping programmes per `tvg-id`. See [ADR 0004](adr/0004-hdhr-guide-epg-merge.md).

### Transcode profiles (LP-010 / LP-011 partial)
- **HDHR-style profile aliases** in `normalizeProfileName` (`native`, `heavy`, `internet360`, `mobile`, … → existing ffmpeg TS presets); **hyphen/punctuation variants** match the same presets (e.g. `Internet-1080` → `dashfast`).
- **Explicit `?profile=pmsxcode`** now forces transcode (same as other named profiles).
- **Named stream profiles file:** optional **`IPTV_TUNERR_STREAM_PROFILES_FILE`** JSON maps operator-defined profile names → **`base_profile`**, optional **`transcode`**, **`output_mux`** (`mpegts` / `fmp4`), and **`description`**; resolves **`?profile=`** and pairs with per-channel **`IPTV_TUNERR_PROFILE_OVERRIDES_FILE`**. **`/debug/runtime.json`** → **`stream_profiles_file`**.
- **Reference:** [docs/reference/transcode-profiles.md](reference/transcode-profiles.md). Separate HLS/fMP4 packaging remains future epic work.

### EPG SQLite foundation (LP-007 partial)
- **`internal/epgstore`**: optional SQLite file (`IPTV_TUNERR_EPG_SQLITE_PATH`), WAL + migrations (`epg_channel`, `epg_programme`); opened during `serve` / `run` when set.
- **ADR 0003** ([docs/adr/0003-epg-sqlite-vs-postgres.md](adr/0003-epg-sqlite-vs-postgres.md)): SQLite default for Tunerr-local EPG; Postgres only for explicit multi-writer/shared-state requirements.

### EPG SQLite cleanup (LP-009 partial)
- **`IPTV_TUNERR_EPG_SQLITE_VACUUM`**: when `true`/`1`, run **`VACUUM`** on the EPG SQLite file after retain-past pruning removes one or more programme rows (reclaim space; can add latency on large files).
- **`/guide/epg-store.json`**: includes `db_file_bytes`, `db_file_modified_utc`, and `vacuum_after_prune` for operator visibility.

### EPG SQLite retention + provider URL hook (LP-009 partial + LP-008 follow-on)
- **`IPTV_TUNERR_EPG_SQLITE_RETAIN_PAST_HOURS`**: after merged-guide sync, delete SQLite programmes whose **end** is before `now - N hours`, then drop orphan `epg_channel` rows; `SyncMergedGuideXML` returns prune count; `/guide/epg-store.json` includes `retain_past_hours`.
- **`IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX`**: optional query string appended to provider `xmltv.php` (for panels that support extra parameters — **not** standard Xtream; verify with your provider).
- **`IPTV_TUNERR_PROVIDER_EPG_DISK_CACHE`**: optional path to a file storing the last provider `xmltv.php` body; sends **`If-None-Match`** / **`If-Modified-Since`** and reuses the file on **HTTP 304** (reduces bandwidth when the upstream supports conditional GET). Sidecar: `*.meta.json`.

### EPG SQLite sync + incremental window (LP-008 partial)
- **Merged guide → SQLite**: after each successful XMLTV refresh, `SyncMergedGuideXML` replaces `epg_channel` / `epg_programme` + `last_sync_utc` metadata (schema v2: `epg_meta`).
- **`/guide/epg-store.json`**: programme/channel counts, `global_max_stop_unix`, optional `?detail=1` for per-channel max stop (incremental fetch input).
- **Operator `/ui/`** links to `/guide/epg-store.json` when exploring the store.

### Hardware HDHomeRun (client spike)
- **`hdhr-scan`**: UDP discovery for physical SiliconDust tuners (or `-addr` for HTTP-only `discover.json` / optional `lineup.json`). Implemented in `internal/hdhomerun` (`DiscoverLAN`, `FetchDiscoverJSON`, `FetchLineupJSON`).
- **`IPTV_TUNERR_HDHR_LINEUP_URL`** merge semantics: imported hardware rows now keep a live-channel **`source_tag`** and are not dropped only because an IPTV row already uses the same **`tvg_id`**; exact **`channel_id`** duplicates are still skipped so hybrid catalogs stay source-tagged instead of collapsing blindly.
- **`hdhr-scan -guide-xml`**: fetch device `guide.xml`, count XMLTV `channel` / `programme` elements (`internal/hdhomerun/guide.go`). Runtime merge: **`IPTV_TUNERR_HDHR_GUIDE_URL`** ([ADR 0004](adr/0004-hdhr-guide-epg-merge.md)); catalog merge semantics remain [ADR 0002](adr/0002-hdhr-hardware-iptv-merge.md).
- **Operator `/ui/`**: minimal embedded HTML dashboard (`internal/tuner/static/ui/`, `IPTV_TUNERR_UI_*`); localhost-only by default.
- **Operator guide preview (`LP-006`)**: `/ui/guide/` shows a read-only table from the **merged cached** XMLTV (`XMLTV.GuidePreview`); `/ui/guide-preview.json` returns the same data (optional `?limit=` up to 500).
- **ADR 0002** ([docs/adr/0002-hdhr-hardware-iptv-merge.md](adr/0002-hdhr-hardware-iptv-merge.md)): how HDHR hardware lineups relate to IPTV catalogs (tag sources; separate instances until explicit merge).

---

## [v0.1.14] — 2026-03-19

### Documentation & diagnostics
- **Cloudflare operator guide**: added [how-to/cloudflare-bypass.md](how-to/cloudflare-bypass.md) (automatic UA cycling, header profiles, cookies, `cf-status`, env knobs).
- **Debug bundle workflow**: added `iptv-tunerr debug-bundle` plus [how-to/debug-bundle.md](how-to/debug-bundle.md) and `scripts/analyze-bundle.py` for correlating stream attempts, logs, and pcaps.
- **README**: expanded Cloudflare troubleshooting section and cross-links to the new how-to guides.

### QA / diagnostics
- **Direct-vs-Tunerr comparison harness**: added `scripts/stream-compare-harness.sh` and `scripts/stream-compare-report.py` to capture `ffprobe`, `ffplay`, `curl`, and optional `tcpdump` evidence for a direct upstream URL versus the equivalent Tunerr stream URL in one reproducible bundle.
- **Structured stream-attempt export**: added `/debug/stream-attempts.json`, which exposes recent gateway decisions, per-upstream outcomes, effective URLs, and redacted request/ffmpeg header summaries for debugging direct-vs-Tunerr mismatches.
- **Troubleshooting workflow update**: the runbook now documents the new comparison harness, including header-file inputs, pcap generation, and how to inspect the resulting artifacts in Wireshark or `tshark`.

### Catch-up recording
- **Recorder daemon MVP**: added `iptv-tunerr catchup-daemon`, which continuously scans guide-derived capsules, records eligible `in_progress` / `starting_soon` items, dedupes by capsule identity, enforces a max-concurrency budget, and persists `active` / `completed` / `failed` state to JSON.
- **Recorder publish/retention hooks**: completed daemon recordings can now be published into a media-server-friendly directory layout with `.nfo` sidecars, and expired or over-retained recordings are pruned automatically.
- **Recorder publish-time library registration**: `catchup-daemon` can now reuse the same lane library workflow as `catchup-publish`, creating/reusing Plex, Emby, and Jellyfin libraries and triggering targeted refreshes as completed recordings land under `-publish-dir`.
- **Recorder policy filters and duplicate suppression**: `catchup-daemon` now supports channel-level allow/deny filters (`-channels`, `-exclude-channels`) and suppresses duplicate recordings by programme identity (`dna_id`/channel + start + title), not only by exact `capsule_id`.
- **Recorder status/reporting surface**: added `catchup-recorder-report` plus `/recordings/recorder.json`, which summarize recorder state, per-lane counts, published item totals, and recent active/completed/failed items from the persistent daemon state file.
- **Lane-specific retention and storage budgets**: `catchup-daemon` now supports per-lane completed/failed retention counts and per-lane completed-item storage budgets, pruning older items first within each lane before global retention limits are applied.
- **Interrupted-recording recovery semantics**: daemon restarts now preserve unfinished recordings as explicit failed `status=interrupted` items with recovery metadata and partial byte counts when available, and the scheduler can automatically retry the same programme window if it is still eligible after restart.
- **Recorder spool/finalize**: `catchup-record` / `catchup-daemon` capture streams to `<lane>/<sanitized-capsule-id>.partial.ts` first and rename to the final `.ts` only after a clean HTTP 200 + body copy; interrupted or failed runs no longer leave a finished-looking `.ts` on disk.
- **Recorder transient retry/backoff**: `catchup-daemon` can retry a capture when errors look transient (typical 5xx/429/408-style HTTP failures, timeouts, connection resets) with exponential backoff capped by `-record-retry-backoff-max`, up to `-record-max-attempts`.
- **Recorder same-spool HTTP Range resume**: after transient mid-stream failures, `catchup-daemon` can retry with `Range` against the existing `.partial.ts` spool when the upstream supports partial content (206), avoiding a full re-download when possible (`-record-resume-partial`, default on).
- **Recorder smarter backoff**: transient retries honor `Retry-After` when present and apply HTTP-status-aware backoff multipliers (e.g. 429/502/503/504) on top of exponential backoff.
- **Recorder capture observability**: per-item fields `capture_http_attempts`, `capture_transient_retries`, and `capture_bytes_resumed`, plus aggregate `sum_*` fields in `recorder-state.json` statistics, summarize HTTP attempts, retry churn, and bytes appended via resume.
- **Recorder multi-upstream failover**: catalog `stream_url` / `stream_urls` are merged (after the Tunerr `/stream/<id>` URL) into `record_source_urls` when `-record-upstream-fallback` is enabled (default on for `catchup-daemon` / `catchup-record`); capture advances to the next URL after non-transient failures or exhausted transient retries, with `capture_upstream_switches` / `sum_capture_upstream_switches` metrics.
- **Recorder catalog UA on capture**: `preferred_ua` from the live channel is sent as `User-Agent` on capture HTTP requests when present.
- **Recorder time-based completed retention**: `-retain-completed-max-age` and `-retain-completed-max-age-per-lane` (e.g. `sports=72h`, `7d`) prune old completed items from state and delete associated files.
- **Recorder soak helper**: `scripts/recorder-daemon-soak.sh` wraps `catchup-daemon -run-for` for bounded soak runs.
- **Recorder fallback URL ordering**: `IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS` (comma-separated hosts) moves matching catalog fallbacks after healthier URLs; the Tunerr `/stream/<id>` URL stays first.

### Upstream / Cloudflare hardening
- **`cf-status` CLI**: inspect per-host Cloudflare state from the cookie jar and persisted learned file (`cf_clearance` freshness, working UA, CF-tagged flag); JSON output supported.
- **CF learned persistence**: Tunerr persists per-host working UA and CF-tagged flags to `cf-learned.json` (path via `IPTV_TUNERR_CF_LEARNED_FILE` or auto-derived next to the cookie jar), and restores them on startup.
- **Per-host UA override**: `IPTV_TUNERR_HOST_UA=host:preset,...` pins a resolved User-Agent preset per upstream host without waiting for cycling.
- **CF bootstrap**: browser-style header profiles accompany browser UAs during probe cycling; optional background freshness refresh reduces mid-session expiry surprises for `cf_clearance`.
- **Recorder lane budget visibility**: `recorder-state.json` statistics now include `lane_storage` with per-lane `used_bytes` and optional `budget_bytes` / `headroom_bytes` when `-budget-bytes-per-lane` is set.
- **Deferred library refresh**: with `-register-*` and `-refresh`, `-defer-library-refresh` registers/reuses libraries per recording but runs the media-server library scan once after `recorded-publish-manifest.json` is updated for that completion.
- **Better ffmpeg HLS request parity**: ffmpeg relay inputs now inherit the effective upstream `User-Agent`, `Referer`, and cookie-jar cookies more faithfully, and enable persistent/multi-request HLS HTTP input by default to better match successful direct `ffplay` behavior on legitimate CDN/HLS paths.

---

## [v0.1.12] — 2026-03-19

### Streaming
- **Provider/CDN compatibility controls**: added `IPTV_TUNERR_UPSTREAM_HEADERS`, `IPTV_TUNERR_UPSTREAM_ADD_SEC_FETCH`, `IPTV_TUNERR_UPSTREAM_USER_AGENT`, `IPTV_TUNERR_COOKIE_JAR_FILE`, `IPTV_TUNERR_FFMPEG_DISABLED`, and `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE` so operators can match stricter upstream header/cookie expectations and disable ffmpeg-side host rewriting when necessary.
- **Redirect-safe HLS relay**: HLS playlist rewriting and refresh now track the effective post-redirect playlist URL, so relative segment or nested playlist paths keep resolving correctly after CDN redirects.
- **Credential-aware fallback stream routing**: multi-provider fallback URLs now keep per-stream auth metadata through catalog dedupe and host filtering, so channel changes and second-session failover do not silently reuse provider-1 credentials against provider-2 URLs.
- **FFmpeg Cloudflare cookie forwarding**: ffmpeg HLS relay inputs now inherit the same per-stream credentials and learned upstream cookies as the Go gateway client, which closes the remaining gap where Cloudflare-cleared playlists still failed once ffmpeg took over segment fetches.
- **Direct player_api fallback now preserves multi-provider backups**: when probe ranking returns no provider as `OK` but direct `player_api` indexing still works, the catalog now keeps multi-provider fallback URLs and per-stream auth rules instead of collapsing back to a single provider path.
- **Invalid HLS playlists now fail over**: `.m3u8` responses that come back as empty or HTML are now treated as upstream failures and the gateway advances to the next fallback URL instead of stalling on a useless `200`.

### Guide / intelligence
- **Guide health report**: added `guide-health` plus `/guide/health.json` to combine XMLTV match status with actual merged-guide coverage, including detection of placeholder-only channel rows versus real programme blocks.
- **EPG doctor workflow**: added `epg-doctor` plus `/guide/doctor.json` as the combined top-level diagnosis path, and cached live guide match provenance so repeated guide diagnostics do not rebuild the same match analysis on every request.
- **EPG auto-fixer**: `epg-doctor -write-aliases` and `/guide/aliases.json` can now export reviewable `name_to_xmltv_id` suggestions from healthy normalized-name matches so repaired guide links can be persisted.
- **Channel leaderboard**: added `channel-leaderboard` plus `/channels/leaderboard.json` for hall-of-fame, hall-of-shame, guide-risk, and stream-risk snapshots of the lineup.
- **Guide-quality policy hooks**: added shared guide-health caching plus `IPTV_TUNERR_GUIDE_POLICY` / `IPTV_TUNERR_CATCHUP_GUIDE_POLICY` so runtime lineup shaping and catch-up capsule output can optionally suppress placeholder-only or no-programme channels.
- **Intent lineup recipes**: `IPTV_TUNERR_LINEUP_RECIPE` now includes `sports_now`, `kids_safe`, and `locals_first` in addition to the earlier score-based recipes.
- **Registration recipes**: added `IPTV_TUNERR_REGISTER_RECIPE` / `run -register-recipe` so Plex, Emby, and Jellyfin registration can now reuse channel-intelligence scoring instead of blindly syncing catalog order.
- **Registration intent presets**: media-server registration now also accepts `sports_now`, `kids_safe`, and `locals_first`, matching the runtime lineup recipe presets.
- **Source-backed catch-up replay mode**: `catchup-capsules`, `/guide/capsules.json`, and `catchup-publish` now support `IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE`, which renders programme-window replay URLs when a real replay-capable source exists instead of pretending the live launcher is a recording.
- **Autopilot hot-start**: added `autopilot-report` plus `/autopilot/report.json`, and hot-start tuning now lets favorite or high-hit channels use more aggressive ffmpeg startup thresholds/keepalive on the HLS path.
- **Autopilot upstream memory**: remembered playback decisions now also keep the last known-good upstream URL/host, so repeat requests can prefer the working stream path first on duplicate or multi-CDN channels.
- **Provider host penalties**: provider autotune now tracks repeated host-level upstream failures and automatically prefers healthier stream hosts/CDNs before retrying known-bad ones.
- **Channel DNA preference policy**: added `IPTV_TUNERR_DNA_POLICY=prefer_best|prefer_resilient` so lineup and registration flows can now collapse duplicate `dna_id` variants to a preferred winner instead of only reporting the group.
- **Channel DNA preferred hosts**: added `IPTV_TUNERR_DNA_PREFERRED_HOSTS` so duplicate-variant selection can bias trusted provider/CDN authorities before falling back to score-based tie-breaking.
- **Ghost Hunter action recommendations**: visible stale sessions and hidden-grab suspicion now produce different recommended next actions and recovery commands, and the live endpoint supports `?stop=true`.
- **Catch-up capsule curation**: duplicate programme rows that share the same `dna_id + start + title` are now curated down to the richer capsule candidate before export/publish.
- **Autopilot failure memory**: remembered Autopilot decisions now track failure counts/streaks too, so stale remembered paths stop being reused automatically after repeated misses.
- **Ghost Hunter recovery hook**: the CLI can now run the guarded hidden-grab helper directly with `-recover-hidden dry-run|restart`.
- **Catch-up recorder**: added `catchup-record`, which records current in-progress capsules to local TS files plus `record-manifest.json` for non-replay sources.
- **Shared ref loader**: report and guide tooling now use one shared local-file/URL loader with the repo HTTP client defaults instead of duplicated `http.DefaultClient` code paths.

### Ingest / probe
- **Server-info Xtream auth probes**: `player_api.php` probes now treat `server_info`-only JSON responses as valid Xtream-style auth success, matching panels that index correctly even when they do not return `user_info`.
- **Direct player_api fallback restored**: when no provider host ranks as probe-OK, catalog refresh now retries direct `IndexFromPlayerAPI` before falling through to `get.php`, restoring the older behavior that kept indexing alive on panels with probe-only response-shape quirks.
- **Multi-entry probe coverage**: `iptv-tunerr probe` now inspects numbered provider entries (`IPTV_TUNERR_PROVIDER_URL_2`, `_3`, etc.) instead of only the primary provider URL.

### Security
- **Xtream path credential redaction**: URL logging now redacts provider credentials embedded in Xtream-style stream paths (`/live/<user>/<pass>/...`, `/movie/...`, `/series/...`, `/timeshift/...`) instead of only stripping query parameters.

---

## [v0.1.10] — 2026-03-18

### Live TV intelligence
- **Channel intelligence foundation**: added `channel-report` plus `/channels/report.json` to score channels by guide confidence, stream resilience, and next-step fixes.
- **EPG match provenance in reports**: when XMLTV is supplied, channel reports now show whether a channel matched by exact `tvg-id`, alias override, normalized-name repair, or not at all.
- **Intelligence-driven lineup recipes**: added `IPTV_TUNERR_LINEUP_RECIPE` with `high_confidence`, `balanced`, `guide_first`, and `resilient` lineup shaping modes.
- **Channel DNA foundation**: live channels now persist a `dna_id` derived from repaired `TVGID` or normalized channel identity inputs, creating a stable identity substrate for future cross-provider intelligence.
- **Channel DNA grouping surface**: added `/channels/dna.json` and `iptv-tunerr channel-dna-report` to group live channels by shared stable identity instead of exposing `dna_id` only as a per-row field.
- **Autopilot memory foundation**: added optional JSON-backed remembered playback decisions keyed by `dna_id + client_class`, allowing successful stream transcode/profile choices to be reused on later requests.
- **Ghost Hunter foundation**: added `ghost-hunter` plus `/plex/ghost-report.json` to observe visible Plex Live TV sessions, classify stalls with reaper heuristics, and optionally stop stale visible transcode sessions.
- **Ghost Hunter escalation**: when Plex exposes zero visible live sessions, Ghost Hunter now flags the hidden-grab pattern explicitly and returns the guarded recovery helper command and runbook path.
- **Provider behavior profile foundation**: added `/provider/profile.json` to expose learned effective tuner cap, recent upstream concurrency-limit signals, Cloudflare-abuse hits, and current auth-context forwarding posture.
- **Provider autotune foundation**: when `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` is not explicitly set, Tunerr can now auto-arm ffmpeg HLS reconnect after it has actually observed HLS playlist/segment instability at runtime.
- **Guide highlights foundation**: added `/guide/highlights.json`, which repackages the cached merged guide into `current`, `starting_soon`, `sports_now`, and `movies_starting_soon` lanes.

### Catch-up publishing
- **Catch-up capsule preview foundation**: added `/guide/capsules.json`, which turns real guide rows into near-live capsule candidates with lane, publish, and expiry metadata for future library publishing.
- **Catch-up capsule export**: added `iptv-tunerr catchup-capsules` to export the capsule preview model to JSON from a catalog plus guide/XMLTV input.
- **Catch-up capsule layout export**: `catchup-capsules -layout-dir` now writes deterministic lane-split JSON files plus `manifest.json` for downstream publisher automation.
- **Catch-up capsule publishing**: added `iptv-tunerr catchup-publish`, which turns capsule rows into `.strm + .nfo` lane libraries plus `publish-manifest.json`, and can now create/reuse matching Plex, Emby, and Jellyfin libraries in one pass.
- **Jellyfin catch-up library compatibility**: catch-up publishing now uses Jellyfin's current `/Library/VirtualFolders` API shape (list via `GET /Library/VirtualFolders`, create with query params) instead of assuming Emby's older `/Query` behavior.
- **Live server validation**: Emby and Jellyfin catch-up publishing were proven live in-cluster against real server PVC paths and created lane libraries plus `.strm + .nfo` output successfully.

### Docs
- **Product roadmap**: documented the Live TV Intelligence epic (Channel DNA, Autopilot, lineup recipes, Ghost Hunter, catch-up capsules).

---

## [v0.1.9] — 2026-03-18

### Build / release
- **Expanded Docker image matrix**: registry publishes now target `linux/amd64`, `linux/arm64`, and `linux/arm/v7`.
- **Correct armv7 Docker cross-builds**: the Docker build path now translates BuildKit's `TARGETVARIANT` into `GOARM`, which is required for correct Go builds on `linux/arm/v7`.

### Docs
- **Container platform alignment**: Docker and packaging docs now match the actual Linux image platforms shipped by the workflow.

---

## [v0.1.8] — 2026-03-18

### Build / release
- **Expanded tagged release binaries**: GitHub Releases now publish `linux/arm/v7` and `windows/arm64` artifacts in addition to the existing `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64` builds.
- **Cleaner release pages**: release notes are generated from repo data instead of generic GitHub auto-notes. When a changelog section exists for the tag, it is used directly on the release page.

### Docs
- **Platform support alignment**: packaging and platform docs now match the actual published binary matrix so operators can see which targets are shipped on tagged releases.

---

## [v0.1.2] — 2026-03-18

### Features
- **Layered EPG pipeline**: guide data now comes from three sources merged by priority — provider XMLTV (`xmltv.php`) > external XMLTV (`IPTV_TUNERR_XMLTV_URL`) > placeholder. External gap-fills provider for any time windows the provider EPG doesn't cover. Placeholder is always the final fallback per channel.
- **Provider EPG via `xmltv.php`**: fetches the Xtream-standard EPG endpoint using existing provider credentials. No additional configuration required for Xtream providers. Produces real programme schedule data without any third-party EPG source.
- **Background refresh**: guide cache is pre-warmed synchronously at startup (first `/guide.xml` request is never cold), then refreshed by a background goroutine on the TTL tick. Stale cache is preserved on fetch error — no guide outage on transient provider failures.

### New env vars
- `IPTV_TUNERR_PROVIDER_EPG_ENABLED` (default `true`) — disable provider `xmltv.php` fetch if not needed
- `IPTV_TUNERR_PROVIDER_EPG_TIMEOUT` (default `90s`) — per-fetch timeout (provider XMLTV can be large)
- `IPTV_TUNERR_PROVIDER_EPG_CACHE_TTL` (default `10m`) — refresh interval; overrides `XMLTV_CACHE_TTL` when set

### Fixes
- **HDHR tuner count integer overflow**: `uint8(tunerCount)` with no bounds check would silently truncate values > 255 in the HDHR discovery packet. Now clamped to [0, 255]. (CodeQL alert #5)

---

## [v0.1.1] — 2026-03-18

- CI: use `GHCR_TOKEN` secret for GHCR registry login; `GITHUB_TOKEN` cannot create new container packages.
- CI: add `release.yml` workflow — creates a GitHub Release with auto-generated notes on every `v*` tag push. `tester-bundles.yml` is now manual-only (`workflow_dispatch`).

---

## [v0.1.0] — 2026-03-17

First tagged release. Covers all features developed through the pre-release testing cycle.

### Features
- **IPTV indexing**: M3U and Xtream `player_api` (live channels, VOD movies, series) with multi-host failover and Cloudflare detection
- **HDHomeRun emulation**: `/discover.json`, `/lineup.json`, `/lineup_status.json`, `/guide.xml`, `/stream/{id}`, `/live.m3u`, `/healthz`
- **Optional native HDHR network mode**: UDP/TCP 65001 for LAN broadcast discovery
- **Stream gateway**: direct MPEG-TS proxy, HLS-to-TS relay, optional ffmpeg transcode (`off`/`on`/`auto`); adaptive buffer; client detection for browser-compatible codec
- **Live TV startup race hardening**: bootstrap TS burst, startup gate, null-TS and PAT+PMT keepalive to prevent Plex `dash_init_404`
- **XMLTV guide**: placeholder or external XMLTV fetch/filter/remap; language/script normalization; TTL cache
- **Supervisor mode**: `iptv-tunerr supervise` runs many child tuner instances from one JSON config for multi-DVR category deployments
- **Plex DVR injection**: programmatic DVR/guide registration via Plex internal API and SQLite (`-register-plex`), bypassing 480-channel wizard limit
- **Emby and Jellyfin support**: tuner registration, idempotent state file, watchdog auto-recovery on server restart
- **VOD filesystem (Linux)**: FUSE mount exposing VOD catalog as directories for Plex library scanning (`iptv-tunerr mount` / `plex-vod-register`)
- **EPG link report**: deterministic coverage report (tvg-id / alias / name match tiers) for improving unlinked channel tail
- **Plex stale-session reaper**: built-in background worker with poll + SSE, configurable idle/lease timeouts
- **Smoketest**: optional per-channel stream probe at index time with persistent cache
- **Lineup shaping**: wizard-safe cap (479), drop-music, region profile, overflow shards (`LINEUP_SKIP`/`LINEUP_TAKE`) for category DVR buckets

### Security
- SSRF prevention: stream gateway validates URLs as HTTP/HTTPS before any fetch
- Credentials redacted from all logs via `safeurl.RedactURL()`
- No TLS verification bypass

### Build / ops
- Single static binary (CGO disabled), Alpine Docker image with ffmpeg
- CI: `go test ./...`, `go vet`, `gofmt` on every push/PR
- Docker: multi-arch (`linux/amd64`, `linux/arm64`), GHCR image on tag push
- Tester bundle workflow: per-platform ZIPs + SHA256SUMS attached to GitHub Release on tag push
- Version embedded at build time via `-ldflags "-X main.Version=..."`; `iptv-tunerr --version` prints it

---

## History (from git)

### Merge and integration (current main)

- **Merge remote-tracking branch origin/main** — Integrate GitHub template updates and restore Plex tuner runtime. Single codebase with agentic template (memory-bank, verify, Diátaxis docs).
- **repo_map:** Document remotes so iptvTunerr only pushes to `origin` and `plex`; do not push from this folder to `github` or `template`.
- **README:** Fix mirror link to iptvTunerr GitHub (not repoTemplate).

### IPTV Tunerr content and docs

- **Fix README and repo docs for IPTV Tunerr** — Align README and docs with actual IPTV Tunerr behavior (IPTV bridge, catalog, tuner, VODFS).
- **Strip all iptvTunerr content from template** — Template repo stripped to generic agentic template; IPTV Tunerr lives in this repo only.
- **Add IPTV Tunerr: IPTV indexer, catalog, VODFS, gateway, and tests** — Initial IPTV Tunerr implementation: index from M3U or player_api, catalog (movies/series/live), HDHomeRun emulator, XMLTV, stream gateway, optional VODFS mount, materializer (cache, direct file, HLS), config from env, health check, Plex DB registration, provider probe. Subcommands: run, index, serve, mount, probe.
- **Learnings from k3s IPTV, HLS smoketest, config/gateway/VODFS and scripts** — Document k3s IPTV stack (Threadfin, M3U server, Plex EPG), what we reuse (player_api first, multi-host, EPG-linked, smoketest), and optional future work (Plex API DVR, 480-channel split, EPG prune). Add systemd example and LEARNINGS-FROM-K3S-IPTV.md.

### Template and agentic workflow

- **Language-agnostic template** — Any language, not just Go.
- **Harden .gitignore for reusable Go template.**
- **Strip to generic agentic Go template** — Remove iptv-tunerr, k3s, all project examples.
- **Template: decision log, definition of done, dangerous ops, repro-first, runbook, scope guard, repo orientation, link check.**
- **Add performance & resource-respect skill, Git-first workflow skill.**
- **Add curly-quotes/special-chars loop + copy/paste-safe doc policy.**
- **Template: agentic repo v4** — Memory bank, Diátaxis docs, CI, work breakdown.

### Initial commits

- **Merge GitLab initial repo with iptv-tunerr.**
- **Initial commit: iptv-tunerr Live TV/VOD catalog and HDHomeRun tuner for Plex.**

---

## Versioning

Currently no semantic version tags; releases are identified by commit. When tagging releases, use [Semantic Versioning](https://semver.org/) (e.g. `v0.1.0`).
