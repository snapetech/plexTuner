**Latest (2026-05-12):** **Plex Live TV playback works again on kspls0; remaining issue is startup latency.** Sports DVR is expanded to 480 channels with `IPTV_TUNERR_LINEUP_RECIPE=sports_now`, `IPTV_TUNERR_LINEUP_MAX_CHANNELS=480`, `IPTV_TUNERR_GUIDE_POLICY=off`, and `IPTV_TUNERR_LINEUP_PROBE_ENABLED=false`; Plex activated all 480. Deployed a proxy fix so JSON `/media/providers` responses rewrite `allowTuners` entitlement hints, matching the existing XML rewrite. Direct Tunerr stream test returned ~24 MB in 20s, and user confirmed playback works but starts slowly.

**Current (2026-05-13):** Build out release scaffolding, asset checks, channel publish rules, and tag-only release gating.

- Goal: make releases produce executable cross-platform assets, package-manager assets, checksums, populated release notes, and channel updates only from release tags on `main`.
- Scope: release workflows/scripts/docs; avoid touching unrelated Plex/proxy runtime changes already in the worktree.
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
- Follow-up: first live package-channel runs may need workflow hardening based on remote service responses.
