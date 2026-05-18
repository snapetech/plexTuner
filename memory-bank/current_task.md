**Current (2026-05-18):** Commit current tree and cut `v0.1.77`.

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
- In progress: committing and pushing the smoke retry fix, then moving tag `v0.1.77` to the corrected release commit.
- Next: monitor release/announcement workflows.

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
