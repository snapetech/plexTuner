**Current (2026-05-18):** Cut `v0.1.79` for Plex DVR token-audit hardening.

- Goal: commit current proxy/token-audit hardening, push `main`, tag `v0.1.79`, and monitor the release workflow.
- Scope: include the Plex Live TV classifier/logging hardening, smoke port-collision retry fix, generated council updates, changelog, and memory-bank updates.
- Assumption: next patch tag after `v0.1.78` is `v0.1.79`.
- Done: promoted Unreleased Plex DVR/CI notes into the `v0.1.79` changelog section.
- Done: confirmed `v0.1.78` Release workflow completed successfully, including Discord, Matrix, and package-channel dispatch.
- Next: run release-readiness, commit/push, tag/push `v0.1.79`, then monitor release automation.

**Current (2026-05-18):** Audit Plex Live TV token elevation gaps after `v0.1.78`.

- Goal: find remaining Plex request shapes where shared-user Live TV/DVR flows may need owner-token substitution, clearer denial logging, or explicit non-elevation coverage.
- Scope: `internal/plexlabelproxy` classifier/proxy behavior, recent tests, and live proxy logs where available.
- Assumption: keep relaxation narrow to Live TV/XMLTV/tuner evidence; do not broadly elevate library subscription or non-Live-TV Plex paths.
- Done: found a remaining classifier edge where a DVR save could carry only a double-encoded XMLTV `ratingKey` value.
- Done: patched Live TV text detection to decode repeated URL-encoding layers and still keep matching scoped to Live TV/XMLTV identifiers.
- Done: added access logging for all `/media/subscriptions*` requests, including redacted query-key names and status, so future non-elevated Plex save shapes are visible without raw token/value leakage.
- Done: recent live proxy logs showed no fresh subscription-denial traffic after `v0.1.78`; only zero-count audit summaries appeared in the sampled window.
- Verification: `go test -count=1 ./internal/plexlabelproxy` passed.
- Done: first full `./scripts/verify` run exposed a smoke retry bug where a Web UI sidecar bind failure left the tuner process alive and skipped the port-collision retry branch; patched the smoke script to kill/retry on the log evidence.
- Done: direct `bash scripts/ci-smoke.sh` passed after the smoke retry fix.
- Done: full `./scripts/verify` passed after rerun.
- Done: deployed the patched proxy binary to the internal Live TV proxy service and restarted only that service.
- Done: live no-token validation showed the double-encoded XMLTV `hints[ratingKey]` shape is now `live_tv=true` and denied before elevation due to missing token, while a library subscription shape stays `live_tv=false` and logs redacted query keys/status.
- Next: commit/push if requested, or monitor the next shared-user retry for `/media/subscriptions*` access/audit logs.

**Current (2026-05-18):** Cut `v0.1.78` for Plex DVR save fix.

- Goal: commit any remaining dirty work, push `main`, tag `v0.1.78`, and monitor the release workflow.
- Scope: release prep for the shared-user Plex DVR subscription-save classifier fixes already deployed to the internal proxy.
- Assumption: next patch tag after `v0.1.77` is `v0.1.78`.
- Done: promoted Unreleased Plex DVR notes into the `v0.1.78` changelog section.
- Done: `./scripts/release-readiness.sh` passed; optional macOS and Windows package lanes were skipped by default.
- Next: commit/push release prep, tag and push `v0.1.78`, then monitor GitHub release automation.

**Previous (2026-05-18):** Fix shared-user Plex DVR Record Options failure.

- Goal: stop Plex shared users from seeing "There was a problem saving your changes" when recording Live TV from Plex.
- Scope: Live TV entitlement proxy classification for Plex DVR subscription list/read endpoints and XMLTV-backed rule edit paths; deploy to the live proxy without interrupting tuner/capture services.
- Assumption: read-only `/media/subscriptions` and `/media/subscriptions/scheduled` calls made by Plex's Record Options UI are part of Live TV discovery for this proxy, while create/update paths still need XMLTV scoping.
- Done: PMS logs showed the external tester's Record Options flow was failing on shared-user `403` responses from `GET /media/subscriptions` and `GET /media/subscriptions/scheduled`.
- Done: patched the Live TV proxy classifier so those read-only subscription list endpoints borrow owner tuner entitlement as Live TV discovery.
- Done: added focused proxy coverage for both subscription list paths.
- Done: focused tests passed, deployed the patched proxy binary, restarted `plex-live-tv-proxy.service`, and verified the live service is active.
- Done: live validation showed missing-token probes are still denied, while authorized requests to both subscription list endpoints are elevated and return `200`.
- Done: audited recent proxy/PMS failure evidence after the deploy; no new real shared-user `403` paths appeared, but existing recording-rule edits can use `/media/subscriptions/{id}` with XMLTV body/query evidence.
- Done: hardened the classifier for XMLTV-backed subscription rule edits across PUT/PATCH/DELETE-style paths while leaving id-only deletes and library subscription edits on the user's token.
- Done: redeployed the hardened proxy binary, verified the service is active, and validated that an XMLTV-style no-token rule edit is caught as Live TV while a library-style edit is not classified for elevation.
- Done: committed and pushed the full requested dirty tree as `e9bc7e2`, including package-smoke evidence/scaffolding, council output, workflow changes, and the Plex proxy fix.
- Done: built commit-stamped internal binary `e9bc7e2`, installed it to the primary/sports Tunerr binary and Live TV proxy binary paths, restarted all three services, and verified service health.
- Done: internal validation after deploy returned `200` for primary ready/guide, sports ready/guide, and proxy identity; Plex DVR registration reactivated 426 primary channels and 125 sports channels.
- Done: found the remaining shared-user save failure was `POST /media/subscriptions` carrying XMLTV identity in bracketed query hints such as `hints[guid]`, patched the classifier, deployed the proxy hotfix, and verified the path is now classified as Live TV.
- Next: have the tester retry Plex Record from the same client; if it still fails, capture the next PMS/proxy log window around the retry.

**Previous (2026-05-18):** Fix package-channel publication gaps found by smoke validation.

- Goal: make failed public channels publish the requested release version and install cleanly without needing a new-release-only validation path.
- Scope: Docker Hub image naming/publication, Launchpad Jammy/Noble PPA publishing, COPR chroot/version publication, RPM installer scriptlets, GitLab dispatch/promotion, and package-smoke validation scaffolding.
- Done: changed Docker Hub publishing to the expected `snapetech/iptvtunerr` image when Docker Hub credentials are present, instead of deriving the namespace from a repo variable.
- Done: GitLab release promotion now dispatches AUR, Snap, PPA, and COPR GitHub publisher workflows, and promotes the internal registry image to Docker Hub tags.
- Done: PPA publishing now runs for both `jammy` and `noble` and explicitly includes the staged binary in `debian/source/include-binaries`.
- Done: COPR publishing now modifies existing projects with the required Fedora 43 and Rawhide chroots before building for those chroots.
- Done: direct RPM scriptlets no longer rely on unresolved `%systemd_*` macros when built outside a Fedora macro environment; a local package rebuild showed plain shell scriptlets.
- Validation: YAML parse, shell syntax, `git diff --check`, and targeted RPM package rebuild/scriptlet inspection passed.
- Next: after these changes are pushed, rerun the package publisher workflows for `v0.1.77`, then rerun package-smoke against Docker Hub, PPA Jammy/Noble, and COPR.

**Previous (2026-05-18):** Commit current tree and cut `v0.1.77`.

- Goal: commit and push the entire dirty working tree, configure Discord release announcements, and publish a new release.
- Scope: include dirty and unrelated repo changes as explicitly requested; do not commit secrets. Use the existing release workflow with a repository secret for Discord.
- Assumption: next patch tag after `v0.1.76` is `v0.1.77`.
- Done: confirmed the release workflow already posts Discord announcements when `DISCORD_RELEASE_WEBHOOK` is configured.
- Done: set the repository `DISCORD_RELEASE_WEBHOOK` secret from the operator-provided webhook URL without adding the URL to tracked files.
- Done: promoted current changelog entries into `v0.1.77`.
- Done: `./scripts/release-readiness.sh` passed; optional macOS and Windows package host lanes were skipped by default.
- Done: checked the tracked tree does not contain the Discord webhook literal.
- Done: committed all dirty changes as `8ff0872`, pushed `main`, and pushed tag `v0.1.77`.
- Done: first release workflow run failed at the changelog gate because the real release note used a word reserved by the placeholder detector; reworded that note and moved the tag to the corrected commit.
- Done: second release workflow run reached binary smoke and failed because a smoke server selected a port already in use on the runner; patched the smoke startup path to retry cleanly.
- Done: local `bash ./scripts/ci-smoke.sh` and `./scripts/release-readiness.sh` passed with the smoke retry fix.
- Done: committed and pushed the smoke retry fix as `8e76c52`, then moved tag `v0.1.77` to that commit.
- Done: third release workflow passed verify and smoke, then failed in `Build binaries` because the runner did not have `zip` installed for Windows release archives.
- Done: fourth release workflow published `v0.1.77`; release asset build/verification, Discord announcement, Matrix announcement, and package-channel dispatch all passed.
- Done: committed and pushed Debian package tool installation as `0625c70`, then moved tag `v0.1.77` to include it.
- Done: fifth release workflow hit another smoke port collision in a custom `serve` launch block; extended the retry wrapper to all custom smoke `serve` launch paths and verified `bash ./scripts/ci-smoke.sh` locally.
- In progress: fifth release workflow also exposed council generated-count drift after the smoke hardening; regenerated council state and updated the active backlog count.
- Done: sixth release workflow exposed the same port-collision class on the Web UI sidecar port; added retry handling around the combined tuner/Web UI startup and verified `bash ./scripts/ci-smoke.sh` locally.
- Done: committed and pushed the final smoke fix as `75d800f`, then moved tag `v0.1.77` to include it.
- Done: final Release run `26008882309` passed: verify, release smoke, asset build, package asset build, asset verification, GitHub Release publish, Discord announcement, Matrix announcement, and package-channel dispatch.
- Done: latest `main` checks on `75d800f` passed: CI, CodeQL, Gitleaks, Docker, and Local Identity Leak Check.
- In progress: downstream package channel workflows from the successful release are running/queued.
- Next: monitor package-channel completion if needed; no release blocker remains.

**Previous (2026-05-17):** Improve Plex DVR event-only sports recording windows.

- Goal: make Plex's own Record button work for event-only sports rows without requiring shared users to visit the Tunerr operator UI.
- Scope: XMLTV fallback programme durations for parseable event rows; deploy to the live tuner host after verification.
- Assumptions: Plex cannot present Tunerr-owned prompts, so duration handling must be encoded in guide metadata.
- Done: local standby Live TV proxy saw no traffic during the reported attempt; sports tuner lineup and guide endpoints were healthy, and the suspected NBA Pass stream delivered MPEG-TS bytes.
- Done: root cause was event-only sports rows falling back to a week-long XMLTV placeholder, which gave Plex a bad DVR scheduling window and produced a vague client-side recording error.
- Done: patched event fallback programmes to use bounded 3-hour windows from explicit `(YYYY-MM-DD HH:MM:SS)` or named `Sun 17 May 19:00 EDT` channel times, including fixed North American timezone offsets.
- Done: installed the patched binary on the live tuner host, restarted `iptvtunerr-sports.service` and `iptvtunerr-primary.service`, and verified Plex activated all 160 sports channel mappings.
- Done: validated the DET/CLE event row now publishes `20260517230000 +0000` to `20260518020000 +0000`; direct stream pull returned MPEG-TS data.
- Done: `./scripts/verify` passed.
- Done: added sport-aware duration defaults for Plex users who only see the Plex guide: basketball/hockey 3.5h, soccer/rugby 2.5h, baseball 4.5h, plus extra padding for Game 7/finals/playoff text.
- Done: deployed the refined duration build and verified the DET/CLE NBA row now publishes `20260517230000 +0000` to `20260518023000 +0000`; Plex reactivated 158 sports mappings.
- Done: `./scripts/verify` passed after rerun; first full run hit an existing guide-policy timing flake, while focused tests and rerun passed.
- Done: checked recent live Tunerr logs after the user retried; no new Plex DVR tune request reached Tunerr, but the matching guide row is stream `1634335`, guide channel `10129`, titled `NEXT | DET - PISTONS VS CLE - CAVALIERS | Sun 17 May 19:00 EDT (US) | 8K EXCLUSIVE | US: NBA PASS PPV 1`.
- Done: started an emergency manual capture for stream `1634335` on the live tuner host at `/var/lib/iptvtunerr/emergency-recordings/det-cle-game7-20260518T001150Z.ts`; verified the ffmpeg process is running and the file is growing.
- Done: added a Plex-library visible capture under `/mnt/datapool_lvm_media/plex/movies/NBA Game 7 Pistons vs Cavaliers (2026)/`, adjusted per operator request from 6h to 3.5h. Active visible file: `NBA Game 7 Pistons vs Cavaliers (2026) - 3h30-20260518T001519Z.ts`.
- Done: found the shared user's Record button failure in PMS logs: `GET /media/subscriptions/template?guid=tv.plex.xmltv...` returned `403` as the shared user because the Live TV proxy did not classify Plex XMLTV recording subscription template/create paths as Live TV entitlement requests.
- Done: patched the Live TV proxy classifier so XMLTV-backed `/media/subscriptions/template` and `POST /media/subscriptions` requests borrow owner tuner entitlement while ordinary library subscriptions remain on the user token.
- Done: focused proxy tests passed; deployed the patched proxy binary and restarted only `plex-live-tv-proxy.service`. Active captures were not interrupted. Validation: no-token request to the formerly failing template path is denied by the proxy as Live TV, and an authorized-token request is elevated and returns `200`.
- Next: wait for user retry; if the Record button still fails, capture the next PMS/proxy log window.

**Latest (2026-05-12):** **Plex Live TV playback works again on kspls0; remaining issue is startup latency.** Sports DVR is expanded to 480 channels with `IPTV_TUNERR_LINEUP_RECIPE=sports_now`, `IPTV_TUNERR_LINEUP_MAX_CHANNELS=480`, `IPTV_TUNERR_GUIDE_POLICY=off`, and `IPTV_TUNERR_LINEUP_PROBE_ENABLED=false`; Plex activated all 480. Deployed a proxy fix so JSON `/media/providers` responses rewrite `allowTuners` entitlement hints, matching the existing XML rewrite. Direct Tunerr stream test returned ~24 MB in 20s, and user confirmed playback works but starts slowly.

**Current (2026-05-16):** Cut `v0.1.76` from current `main`.

- Goal: publish a new GitHub release for the GitHub PR/security maintenance batch now on `main`.
- Scope: release prep only: changelog promotion, release-readiness verification, commit/push, tag/push, and workflow monitoring.
- Assumption: next semver patch tag after `v0.1.75` is `v0.1.76`.
- Done: promoted `docs/CHANGELOG.md` Unreleased notes into `v0.1.76`.
- Done: patched local-runner workflow failures by replacing Debian-only installs with `scripts/install-ci-tools.sh` and making CodeQL use an explicit Go build.
- Done: `./scripts/release-readiness.sh` passed locally.
- Done: committed and pushed release prep as `80004d4`, pushed annotated tag `v0.1.76`, and the GitHub release job uploaded release assets.
- Done: found a local-runner Gitleaks action cache extraction failure after the tag release; patched and pushed the workflow fix as `c8bcd40`.
- Done: remote replacement Gitleaks scan completed successfully; release Discord announcement completed successfully.
- Done: patched release-channel local-runner follow-up failures through `d502b64`: Gitleaks direct CLI scan, PPA direct `dpkg`/FTP upload, COPR isolated CLI venv, and Snap direct `snap pack`.
- Done: PPA rerun completed successfully with the inline installer and Launchpad FTP upload.
- Done: COPR and Snap reruns completed successfully.
- Done: latest `main` checks on `d502b64` completed successfully: CI, CodeQL, Gitleaks, and Local Identity Leak Check.
- Next: no release-monitoring action pending.

**Current (2026-05-18):** Add reusable post-release package-channel validation.

- Goal: validate public install channels after release publication from internal GitLab, with equivalent GitHub scaffolding committed but disabled.
- Scope: package-smoke harness, project channel manifest, GitLab tag-only post-release validation jobs, and disabled GitHub workflow entrypoint.
- Assumption: Windows package channels remain manually gated; Linux public channels and container channels can run from GitLab runners as they become available.
- Done: added `packaging/smoke/package-smoke` with evidence JSON, JUnit, logs, public-channel install adapters, version checks, container multi-arch smoke, and uninstall cleanup hooks.
- Done: added GitLab `post_release_validate` stage with tag-only matrix jobs for containers, Ubuntu channels, Fedora channels, AUR, and Snap.
- Done: added disabled `workflow_dispatch` GitHub package-smoke workflow for future activation.
- Next: run the new GitLab validation stage against the next release tag and tighten `allow_failure` once runner/channel delays are characterized.

**Previous (2026-05-16):** Triage and action all open GitHub PRs and security issues for `snapetech/iptvtunerr`.

- Goal: inspect open PRs, failing checks, review comments, and GitHub security/dependency/code-scanning alerts; fix what is actionable; merge PRs that are safe; document anything blocked.
- Scope: GitHub PR and security maintenance only. Avoid unrelated refactors and do not recreate removed split-brain deployment paths.
- Assumptions: user explicitly asked to action/upgrade/fix/merge/resolve all items, so safe merges and alert resolutions are in scope after verification; anything requiring unavailable external privileges or unsafe compatibility choices should be reported rather than guessed.
- Done: merged Dependabot PRs `#19` and `#20`; applied the stale/conflicting `#15` brotli upgrade directly on current `main`.
- Done: hardened CodeQL security findings by redacting Plex proxy source/header logs, constraining provider lineup URLs, and validating HDHomeRun lineup URLs.
- Done: switched GitHub Actions Linux jobs to the local `self-hosted, Linux, X64, iptvtunerr-deploy` runner labels and Windows package jobs to `self-hosted, Windows, X64`.
- Done: focused package tests and `./scripts/verify` passed locally.
- Next: commit and push `main`; close obsolete PR `#15` after the direct upgrade is on `main`.

**Previous (2026-05-15):** Cut `v0.1.75` from current `main`.

- Goal: commit/push the current repo state and publish a new GitHub release tag.
- Scope: whole worktree as explicitly requested by the user; working tree was already clean before release prep, so only release-prep memory/changelog edits are expected locally.
- Assumption: next semver patch tag after `v0.1.74` is `v0.1.75`; use the existing populated changelog entries as the release notes.
- Done: confirmed `main` is at `origin/main` with commits after `v0.1.74`.
- Done: promoted `docs/CHANGELOG.md` Unreleased notes into `v0.1.75`.
- Done: `./scripts/verify` and `./scripts/release-readiness.sh` passed locally.
- Next: commit, push `main`, tag `v0.1.75`, push the tag, and monitor the release workflow.

**Previous (2026-05-14):** Repair Winget PR installation validation for `microsoft/winget-pkgs#374269`.

- Done: patched the Winget manifest generator and release-asset verification so generated ZIP portable manifests use `iptv-tunerr-vX.Y.Z-windows-amd64/iptv-tunerr.exe`.
- Done: pushed `microsoft/winget-pkgs#374269` update commit `740f80f081e` with only the corrected nested installer path.
- Follow-up: wait for Microsoft validation to rerun; do not post another `@wingetbot run` unless the pushed manifest update does not trigger validation.
- Done: added `packaging/aur` metadata for `iptvtunerr` and `iptvtunerr-bin`, AUR helper scripts, and `.github/workflows/release-aur.yml`.
- Done: unsealed local OpenBao, checked for release-channel credentials, added `AUR_SSH_KEY`, `GPG_PRIVATE_KEY`, `LAUNCHPAD_SFTP_KEY`, and `LAUNCHPAD_SFTP_USER` GitHub secrets for `snapetech/iptvtunerr`.
- Done: created and pushed the initial AUR repos `iptvtunerr` and `iptvtunerr-bin`.
- Done: created Launchpad PPA `ppa:keefshape/iptvtunerr` (Launchpad account name `keefshape`, display name `slskdn`).
- Done: added `COPR_LOGIN` and `COPR_TOKEN` GitHub secrets for `snapetech/iptvtunerr`; installed `snapd`/`snapcraft` locally.
- Done: `SNAPCRAFT_STORE_CREDENTIALS` is now configured for `snapetech/iptvtunerr`.
- Done: added Chocolatey and Winget packaging/workflow scaffolding for Windows release ZIP assets.
- Done: added Snap, Launchpad/PPA, and COPR package metadata/workflows for this Go binary.
- Done: set `DOCKERHUB_USERNAME=keefshape` repo variable and `WINGETCREATE_GITHUB_TOKEN` secret.
- Done: added `CHOCO_API_KEY` GitHub secret for the `slskdn` Chocolatey account.
- Done: added release asset build/verification scripts for raw binaries, archives, checksums, and a manifest.
- Done: added direct `.deb` and `.rpm` GitHub Release package assets.
- Done: added tag-on-current-main guards to release, Docker, AUR, Snap, PPA, COPR, Chocolatey, and Winget workflows.
- Done: changed Docker publishing to tag-only and added CI release-asset verification.
- Done: added local git hook, installer, CI gate, and release gate requiring changelog updates and populated release tag sections.
- Done: signed the Microsoft CLA for the Winget PR by posting the GitHub bot agreement comment; the CLA check cleared.
- Done: patched the Snap source archive layout; `v0.1.73` Snap, AUR, COPR, PPA, Winget workflow, Docker, and GitHub Release runs completed successfully.
- In progress: assessing Windows package gates and pausing automatic Chocolatey/Winget dispatch until Chocolatey push permissions and Winget validation state are resolved.
- Follow-up: Chocolatey still needs portal/account/API-key remediation for `403 Forbidden`; Winget PR `microsoft/winget-pkgs#374269` needs validation follow-up before submitting more versions.
