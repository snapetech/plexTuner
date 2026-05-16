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
- Next: wait for the queued remote replacement Gitleaks scan to get a local runner and complete.

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
