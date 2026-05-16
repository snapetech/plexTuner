## 2026-05-16 - GitHub PR and security maintenance

- Merged Dependabot PRs `#19` (`github.com/hanwen/go-fuse/v2` to `v2.10.1`) and `#20` (`golang.org/x/net` to `v0.54.0`).
- Applied the stale/conflicting Dependabot PR `#15` directly by upgrading `github.com/andybalholm/brotli` to `v1.2.1` and refreshing `vendor/`.
- Hardened CodeQL findings by removing raw apparent-source and trusted proxy header values from Plex label proxy logs, constraining Plex provider lineup URLs to the configured provider origin, and rejecting invalid HDHomeRun lineup URL schemes/credentials.
- Switched GitHub Actions Linux jobs to the local self-hosted Linux runner labels and Windows package jobs to self-hosted Windows labels.
- Updated the Plex Live TV entitlement proxy runbook for the new redacted audit log fields.
- Verification: focused package tests passed; `./scripts/verify` passed.

## 2026-05-15 - Prepare v0.1.75 release

- Promoted the populated Unreleased changelog notes for Plex Live TV proxy stability and release-process fixes into `v0.1.75`.
- Confirmed the repo was already clean on `main` before release-prep edits; no unrelated dirty files were present to include.
- Verification: `./scripts/verify` passed; `./scripts/release-readiness.sh` passed.

## 2026-05-14 - Fix Winget install validation path

- Inspected `microsoft/winget-pkgs#374269` after the Microsoft package-manager bot reported a general installation failure.
- Root cause: the submitted ZIP portable manifest pointed `NestedInstallerFiles.RelativeFilePath` at root-level `iptv-tunerr-v0.1.68-windows-amd64.exe`, but the release ZIP contains `iptv-tunerr-v0.1.68-windows-amd64/iptv-tunerr.exe`.
- Patched `packaging/scripts/update-winget-manifests.sh` to generate the correct nested executable path and added a Windows ZIP nested-path assertion to `scripts/verify-release-assets.sh`.
- Pushed winget-pkgs PR update commit `740f80f081e` with only the manifest path correction.
- Verification: downloaded the v0.1.68 Windows ZIP, confirmed the corrected manifest path exists in the archive, then ran `./scripts/build-release-assets.sh v0.0.0 dist && ./scripts/verify-release-assets.sh v0.0.0 dist`.

## 2026-05-13 - Pause noisy Windows package publishing

- Closed duplicate automated Winget PRs `microsoft/winget-pkgs#374279`, `#374280`, and `#374285`; kept the original `#374269` open for the first validation gate.
- Confirmed `#374269` has a green `license/cla` check after the agreement comment, but is labeled `Internal-Error`, `Needs-Attention`, and `Validation-Guide` by Microsoft's validation system.
- Confirmed Chocolatey package packing succeeds and the public feed has no existing `iptvtunerr` package entry, but `choco push` returns `403 Forbidden` from Chocolatey's push endpoint.
- Removed Chocolatey and Winget from the automatic post-release channel dispatch list; the workflows remain available for manual dispatch after their external gates are resolved.

## 2026-05-13 - Patch Snap release payload layout

- `v0.1.72` confirmed release assets, Docker, AUR, COPR, PPA, and Winget paths were working, while Snap reached the pack step and failed because the command path referenced a versioned directory that was absent from Snapcraft's dump-plugin prime output.
- Changed the Snap workflow to build a root-level `iptv-tunerr` tarball inside `packaging/snap/` and keep the Snap app command as `iptv-tunerr`.
- Chocolatey remains blocked by a remote `403 Forbidden` during `choco push`, which points at package/account/API-key permission rather than local packaging.

## 2026-05-13 - Add AUR release-channel scaffolding

- Added AUR package metadata for `iptvtunerr` (source build from tagged GitHub archive) and `iptvtunerr-bin` (Linux amd64 release asset).
- Added systemd, sysusers, tmpfiles, env, and install metadata under `packaging/aur/`.
- Added AUR helper scripts for SSH setup, clone/push, `.SRCINFO` generation, and checksum validation.
- Added `.github/workflows/release-aur.yml` to publish both AUR packages on GitHub release publication or manual tag dispatch when `AUR_SSH_KEY` is configured.
- Added `AUR_SSH_KEY` to `snapetech/iptvtunerr` GitHub secrets from the local AUR SSH key, then created and pushed the initial `iptvtunerr` and `iptvtunerr-bin` AUR repos.
- Unsealed local OpenBao and checked for release-channel credentials; COPR/Snapcraft credentials were not present there. Added available PPA credentials (`GPG_PRIVATE_KEY`, `LAUNCHPAD_SFTP_KEY`, `LAUNCHPAD_SFTP_USER`) to `snapetech/iptvtunerr`.
- Created Launchpad PPA `ppa:keefshape/iptvtunerr`; Launchpad account `keefshape` has display name `slskdn`.
- Added provided COPR credentials as `COPR_LOGIN` and `COPR_TOKEN` GitHub secrets for `snapetech/iptvtunerr`.
- Installed `snapd` from AUR and `snapcraft` via snap locally; attempted constrained Snapcraft credential export, but it requires Ubuntu One interactive email/password/2FA and no local Snapcraft session exists.
- Confirmed user completed Snapcraft credential export and `SNAPCRAFT_STORE_CREDENTIALS` is present in `snapetech/iptvtunerr`.
- Documented the AUR setup and remaining credential sources in `docs/how-to/release-channels.md`.
- Verification: `bash -n packaging/scripts/*.sh`, AUR checksum validation, `.SRCINFO` generation for both packages, and a Go build/version smoke passed.
- Verification: `./scripts/verify` passed after the packaging changes; AUR `list-repos` shows `iptvtunerr` and `iptvtunerr-bin`; fresh HTTPS clones of both AUR repos include `.SRCINFO`, `PKGBUILD`, and support files.
- Opportunity filed: Snap, Launchpad/PPA, and COPR packaging still need adapted package metadata/workflows.

## 2026-05-13 - Add Windows channel scaffolding

- Added Chocolatey package metadata and install/uninstall scripts for the Windows amd64 GitHub Release ZIP.
- Added Winget manifest generator for `snapetech.iptvtunerr`.
- Added manual GitHub Actions workflows for Chocolatey publishing and Winget PR submission from a release tag.
- Added `WINGETCREATE_GITHUB_TOKEN` from the authenticated GitHub CLI token.
- Documented Windows release-channel status and required secrets in `docs/how-to/release-channels.md`.
- Verification: `bash -n packaging/scripts/*.sh`, Winget manifest generation smoke, and `./scripts/windows-baremetal-package.sh` passed.
- Remaining: native Windows host proof is still recommended.

## 2026-05-13 - Add Snap, PPA, COPR, and Docker release-channel wiring

- Added Snap metadata and `release-snap.yml` to build/publish the strict `iptvtunerr` snap from the release binary.
- Added Debian package metadata and `release-ppa.yml` to build/sign/upload a source package to `ppa:keefshape/iptvtunerr`.
- Added RPM spec and `release-copr.yml` to build/upload an SRPM to the `slskdn/iptvtunerr` COPR project.
- Set repo variable `DOCKERHUB_USERNAME=keefshape`; existing Docker workflow now has the Docker Hub username needed to publish alongside GHCR.
- Added `CHOCO_API_KEY` GitHub secret from the `slskdn` Chocolatey account.
- Documented container, Linux package, and Windows package-channel status in `docs/how-to/release-channels.md`.
- Verification: packaging script syntax passed; `./scripts/windows-baremetal-package.sh` passed. Debian/RPM package CLIs were not installed locally, so PPA/COPR workflows still need first-run validation in Actions.

## 2026-05-13 - Harden tag-only release asset pipeline

- Added `scripts/build-release-assets.sh` to produce raw executables, Linux/macOS tarballs, Windows ZIPs, `SHA256SUMS.txt`, and `release-manifest.json` for Linux amd64/arm64/armv7, macOS amd64/arm64, and Windows amd64/arm64.
- Added `scripts/build-linux-package-assets.sh` to add direct `.deb` and `.rpm` assets to GitHub Releases.
- Added `scripts/verify-release-assets.sh` and wired CI to build and verify the full release asset set with a dummy version.
- Added `scripts/ensure-release-tag-on-main.sh` and wired release, Docker, AUR, Snap, PPA, COPR, Chocolatey, and Winget workflows to reject tags that do not point at current `main`.
- Changed Docker publishing to tag-only; `latest` now moves only on release tags.
- Expanded generated release notes with release asset/checksum details and install notes.
- Verification: installed local `dpkg-deb` and `rpmbuild` tooling, then the full release asset path passed locally for raw binaries, archives, `.deb`, `.rpm`, manifest, and checksum verification.

## 2026-05-13 - Enforce changelog-backed release notes

- Added `scripts/verify-changelog-entry.sh` with staged, range, and release-tag modes.
- Added `.githooks/pre-commit` plus `scripts/install-git-hooks.sh` so local commits reject release-relevant changes that omit `docs/CHANGELOG.md`.
- Wired CI to enforce changelog updates on release-relevant code, workflow, script, packaging, and docs changes.
- Wired the Release workflow to require a populated changelog section for the exact release tag before release notes are generated.
- Documented the hook installation and release changelog rule in `docs/how-to/release-channels.md`.
- Follow-up while testing releases: `v0.1.66` failed direct RPM asset generation on Ubuntu because `rpmbuild` enforced `systemd-rpm-macros`; patched the direct GitHub Release RPM build to use `--nodeps` while leaving COPR metadata intact.
- Follow-up while testing hooks: patched `.githooks/pre-push` to avoid `pipefail` broken-pipe warnings during regex scans.
- Follow-up while testing `v0.1.68`: GitHub Release and Docker succeeded, but release-event package workflows did not fan out from a release created by Actions. Manually dispatched AUR, Snap, PPA, COPR, Chocolatey, and Winget for `v0.1.68`, then patched the Release workflow to explicitly dispatch those channel workflows after asset upload.
- Follow-up while inspecting manual `v0.1.68` channel tests: PPA, Chocolatey, and Winget passed. AUR failed only its tarball layout check, Snap failed because `core24` requires `platforms`, and COPR failed because `slskdn/iptvtunerr` did not exist yet. Patched all three for `v0.1.70`.
- Follow-up while watching `v0.1.69` package-channel auto-dispatch: some queued channel jobs failed the strict tag-equals-current-main guard after a newer release-fix commit advanced `main`. Patched channel workflows to use an ancestor-in-main guard while keeping the primary Release workflow exact.
- Follow-up while watching `v0.1.71` package-channel runs: AUR, COPR, PPA, Winget, Release, Docker, and CI passed. Snap failed because the managed LXD build could not see a source tarball written outside `packaging/snap`; patched the Snap workflow to stage the tarball inside the snap project.

## 2026-05-12 - Expand sports DVR and restore slow-but-working playback

- Expanded `iptvtunerr-sports` from the strict `sports_na` recipe to `sports_now`, set `IPTV_TUNERR_LINEUP_MAX_CHANNELS=480`, disabled runtime lineup probing, and turned sports runtime guide pruning off so non-EPG sports rows remain visible.
- Verified `/lineup.json` and `/guide.xml` expose 480 sports channels and Plex activated all 480 channel mappings.
- Confirmed the guide/EPG path was not the playback blocker: direct Tunerr stream test on `CA| SPORTSNET WORLD FHD` returned about 24 MB over 20 seconds with HTTP 200.
- Patched `plex-label-proxy` to rewrite JSON `allowTuners` hints in addition to XML, because Plex Web was negotiating JSON `/media/providers` and the proxy previously logged that rewrite was skipped.
- Verification: `go test -count=1 ./internal/plexlabelproxy ./cmd/iptv-tunerr` passed; patched proxy binary installed on `kspls0`; `plex-live-tv-proxy.service` restarted; JSON `/media/providers` now reports `allowTuners: true`.
- User confirmed streams do play, but startup remains slow.

## 2026-05-12 - Repair Plex Live TV spinning after Plex Pass switch

- Found the immediate external failure was the Live TV entitlement proxy's persisted abuse block: `/media/providers` from the user's apparent external source was returning `blocked_bad_actor` even when later requests carried a valid Plex token.
- Backed up and removed `/var/lib/iptvtunerr/plex-live-tv-proxy-blocks.json`, restarted `plex-live-tv-proxy.service`, and verified the same source now gets `outcome=elevated_live_tv` with HTTP 200.
- Found Plex was still advertising a dead automatic remote route (`plex.direct:55556`) after NAT-PMP. Cleared `LastAutomaticMappedPort`, enforced `ManualPortMappingMode=1`, `ManualPortMappingPort=443`, `RelayEnabled=0`, and `customConnections=https://media.snape.tech:443`, then restarted Plex.
- Verified PMS stayed on the Plex Pass build `1.43.2.10687-563d026ea`; plex.tv resources now advertise `https://media.snape.tech:443` plus the static `plex.direct:443`, with the stale `:55556` route gone.
- Found the sports tuner lineup had dropped to 0 channels after runtime probing; disabled `IPTV_TUNERR_LINEUP_PROBE_ENABLED` in `/etc/iptvtunerr/sports.env`, restarted `iptvtunerr-sports.service`, and verified `lineup.json` recovered to 100 channels.
- Plex sports DVR activation completed successfully for all 100 channels.

## 2026-05-12 - Restore external Live TV after Plex Pass switch

- External users reported Live TV disappeared after the Plex container was recreated for the Plex Pass update channel.
- Confirmed Plex, Tunerr, and proxy services were active, but PMS returned `503 Maintenance` for several minutes while database migrations completed.
- Verified the public Cloudflare path `https://media.snape.tech` returns `200` for owner-token `/media/providers` and `/livetv/dvrs`; no-token Live TV remains denied.
- Found Plex was advertising both the intended custom remote URL and a direct `plex.direct` public-port URL. Later repair showed the working production state is static manual mapping (`ManualPortMappingMode=1`) with port 443, `RelayEnabled=0`, and `customConnections=https://media.snape.tech:443`.
- Proxy audit showed an external `/media/providers` request from `204.83.235.92` was elevated after PMS recovered.
- Updated the Live TV entitlement proxy runbook so future Plex updates keep static port mode and do not reintroduce automatic NAT-PMP routes.

## 2026-05-12 - Enable Plex Pass PMS update channel on kspls0

- Found `plex-host` was running `linuxserver/plex:latest` without `VERSION`; LSIO logs said the update routine was skipped because `VERSION` was unset.
- Recreated `plex-host` with the same host network, mounts, `/dev/dri`, and group settings plus `VERSION=latest`, which lets the signed-in Plex Pass account receive the newest entitled PMS build.
- Preserved rollback container as `plex-host.pre-version-latest-20260512-165623` and left it stopped.
- PMS upgraded from `1.43.1.10611-1e34174b1` to `1.43.2.10687-563d026ea`; `/identity` returned 200 after startup migrations.
- Validation: `plex-host`, `iptvtunerr-primary`, `iptvtunerr-sports`, and `plex-live-tv-proxy` are active; Live TV channel mappings remain enabled (`385/385` primary, `77/77` sports after guide-policy filtering).
- Updated deployment docs to require/check `VERSION=latest` for the live Plex host container.

## 2026-05-12 - Point local Tunerr env/docs at kspls0

- Updated local `.env` `IPTV_TUNERR_BASE_URL` from `http://kspld0:5004` to `http://kspls0:5004`.
- Updated the stale shared-lease deployment note in `docs/reference/cli-and-env-reference.md` to name `kspls0`.
- Verified `http://kspls0:5004/discover.json` returns 200 and advertises `http://192.168.50.84:5004`.
- Verified `http://kspls0:5004/guide.xml` returns 200 with `X-Iptvtunerr-Guide-State: ready`; response size was 6,937,320 bytes.
- Found Plex channel activation, not Tunerr guide serving, was the slow step: primary `/guide.xml` served in milliseconds, but Plex's full 385-channel activation PUT exceeded the old 60s client timeout.
- Live repair: sent full primary and sports channelmaps with a longer timeout; Plex now reports enabled `ChannelMapping` rows: primary `385/385`, sports `79/79`.
- Code fix: raised `ActivateChannelsAPI` timeout to 5 minutes and removed token-bearing activation URLs from timeout errors.
- Verification: `go test -count=1 ./internal/plex ./cmd/iptv-tunerr` passed; patched binary installed on `kspls0`; both Tunerr services restarted and `/discover.json` checks passed.

## 2026-05-12 - Fix external Plex Live TV abuse-block false positive

- External user reported the Plex Live TV provider unavailable again.
- Found the live proxy had temporarily blocked the user's apparent source after repeated missing-token Live TV probes, and the source-level block was also rejecting later `/media/providers` requests that carried valid Plex tokens.
- Immediate live fix: cleared the persisted abuse-block state and restarted `plex-live-tv-proxy.service`; the affected source resumed `elevated_live_tv` requests immediately.
- Code fix: source blocks now apply only after checking for an authorized inbound Plex token. Owner tokens and tokens already authorized for the Plex server bypass the source block; missing/unauthorized tokens remain blocked or cooled down.
- Verification: public `/identity` returned `200`, public no-token `/livetv/dvrs` returned `403`, all media services remained active, and `go test -count=1 ./cmd/iptv-tunerr ./internal/plexlabelproxy` passed.

## 2026-05-12 - Harden Plex Live TV proxy abuse controls

- Added env/CLI knobs for Live TV bad-source blocking threshold/window/duration, denied source+token authorization cooldown, optional persisted block state, and audit summary interval.
- Changed source identity handling to prefer trusted `CF-Connecting-IP`, then trusted `X-Forwarded-For`, then socket remote address; forwarded source headers are trusted only from loopback/private frontend peers.
- Added optional persisted JSON block/cooldown state so bad actors remain blocked across proxy restarts without storing Plex tokens.
- Added aggregate `plexlabelproxy_audit_summary` counters for elevated, denied, cooldown, cache-hit/cache-miss, and blocked requests.
- Updated `docs/scripts/validate-plex-live-tv-proxy.sh` to verify owner success, random/no-token denial, optional shared-user success, and repeated bad-attempt blocking.
- Verification: `bash -n docs/scripts/validate-plex-live-tv-proxy.sh`, `go test -count=1 ./internal/plexlabelproxy ./cmd/iptv-tunerr`, and `./scripts/verify` passed.

## 2026-05-12 - Validate v0.1.63 live Plex/Tunerr/proxy stack

- Confirmed GitHub `v0.1.63` release, CI, Docker, CodeQL, and Gitleaks completed successfully.
- Reinstalled the live proxy binary with `-X main.Version=v0.1.63`; `/opt/iptvtunerr/iptv-tunerr-proxy --version` now reports `v0.1.63`.
- Live services checked: Plex host, proxy, primary Tunerr bridge, sports Tunerr bridge, frontend tunnel, and VODFS services are running.
- Tunerr endpoints checked: primary and sports `/discover.json`, `/lineup.json`, `/lineup_status.json`, and `/guide.xml` all return `200`.
- Plex DVR state checked: two intended Tunerr DVRs, both enabled, with 479 and 92 enabled channel mappings.
- Entitlement checks: local and public Live TV owner-token paths return `200`; public no-token and fake-token Live TV paths return `403`; local non-Live-TV no-token path returns Plex `401` without entitlement audit noise.
- Added automated proxy coverage for an external shared user coming through forwarded frontend headers: shared token is authorized, Live TV request is elevated to owner token, source headers are retained for audit context, and raw tokens are not logged.
- Verification: `go test -count=1 ./internal/plexlabelproxy` passed; `./scripts/verify` passed.

## 2026-05-12 - Block repeated bad Plex Live TV elevation attempts

- Added an in-process temporary block for repeated missing-token or unauthorized-token Live TV elevation attempts from the same apparent source.
- Default policy: 5 failed Live TV elevation attempts within 5 minutes blocks that source from Live TV entitlement paths for 30 minutes; ordinary non-Live-TV paths are not blocked by this guard.
- Added redacted audit outcomes for block creation (`bad_actor_blocked`) and blocked requests (`blocked_bad_actor`).
- Verification: `go test -count=1 ./internal/plexlabelproxy` passed; `./scripts/verify` passed.
- Live deploy: installed the patched proxy binary and restarted `plex-live-tv-proxy.service`.
- Live validation: `/library/sections` without a token returned `401` without an audit denial; six bad Live TV requests from a synthetic source returned `403` for the first five and `429` for the sixth, with redacted block audit logs.
- Monitoring: watched the live audit journal for 3 minutes after deploy; observed authorized Live TV elevation only, plus the synthetic block validation.

## 2026-05-12 - Add Plex Live TV proxy security audit logging

- Added redacted `plexlabelproxy_audit` log lines for Live TV owner-token elevation, missing-token denial, and unauthorized-token denial decisions.
- Audit fields include method, path, Live TV classifier state, remote address, proxy source headers, and a SHA-256 token fingerprint; raw Plex tokens are not logged.
- Updated the Live TV entitlement proxy runbook with audit queries for operator checks.
- Verification: `go test -count=1 ./internal/plexlabelproxy` passed; `./scripts/verify` passed.
- Live deploy: rebuilt and installed the proxy binary, restarted `plex-live-tv-proxy.service`, and confirmed live missing-token/fake-token probes return `403` while emitting audit lines.

## 2026-05-12 - Harden Plex Live TV entitlement proxy

- Found the live `plex-live-tv-proxy.service` was running `-elevate-all`; unauthenticated public requests to the media frontend returned `200`, so the proxy was effectively acting as an owner-token deputy for anyone who could reach it.
- Added proxy-side elevation gating: missing inbound Plex tokens are never elevated, and production CLI wiring validates inbound tokens against PMS `/library/sections` before borrowing the owner token.
- Narrowed Live TV elevation classification to known Live TV paths/helpers and safe methods, with `POST /playQueues` allowed only for Live TV stream starts.
- Updated runbook/reference/systemd docs to describe the no-friction model: Plex clients keep sending their own Plex tokens; only already-shared users can borrow owner tuner entitlement for Live TV.
- Verification: `./scripts/verify` passed; `./scripts/release-readiness.sh` passed.
- Live deploy: installed patched `/opt/iptvtunerr/iptv-tunerr-proxy`, backed up the previous binary/service under `/opt/iptvtunerr/backups/`, switched `plex-live-tv-proxy.service` from `-elevate-all` to `-elevate-live-tv -neutralize-owner-history`, and restarted it.
- Live validation: owner token returns `200`; fake token returns `401` for `/library/sections` and `403` for `/livetv/dvrs`; missing token returns `401` for libraries and `403` for Live TV; public direct `media.example.com:32400` timed out.

## 2026-05-12 - Clarify k3s remains supported

- User clarified the deployment boundary after `v0.1.59`: k3s is still a supported user/lab deployment mode; the removed fallback was only the local production split-brain path.
- Added `docs/how-to/k3s-deployment.md` with Secret, Deployment, Service, probe, Plex reachability, HDHomeRun discovery, and multi-DVR ownership guidance.
- Updated deployment docs, README, docs index, and changelog wording to include k3s while preserving the rule that the local production host is systemd-owned.
- Verification: `./scripts/verify` passed; `./scripts/release-readiness.sh` passed.

## 2026-05-12 - Prepare v0.1.59 deployment-contract release

- Added release notes for DVR safety, token redaction, retired orchestration cleanup, and supported deployment contract.
- Documented supported deployment as binary, systemd, Docker/container-on-host, or k3s with one active Tunerr owner per Plex DVR identity.
- Documented duplicate/empty Plex DVR recovery order in Plex ops patterns.
- Verification: targeted retired-path searches were clean; `./scripts/verify` passed; `./scripts/release-readiness.sh` passed.

## 2026-05-12 - Remove old cluster Tunerr/Plex fallback

- Deleted live Tunerr/Plex remnants from the orchestration namespace, including stale Tunerr deployments and matching proxy/config/secret leftovers.
- Removed repo deployment artifacts for that path: manifest tree, cluster deploy workflows, deploy scripts, Plex runbooks, and cluster-specific helper scripts.
- Updated docs/scripts/code fixtures to stop pointing operators or agents at service-DNS DVR URLs, cluster commands, or cluster recovery paths.
- Active local production direction is systemd/bare-metal ownership; k3s remains supported for users/labs when it is the single Plex DVR owner for its identity.

## 2026-05-12 - Stop the Plex host Plex DVR zombie spam

- Root cause was duplicate registrars. Bare-metal systemd Tunerr services and the removed cluster Tunerr path were registering the same Plex device IDs/friendly names with different guide URLs. Plex accumulated empty `0/0` DVR rows and entered repeated `/livetv/dvrs` timeout / maintenance windows.
- Live fix kept the host/systemd Tunerr services as the single owner, installed the patched binary, and deleted twelve empty IPTV DVR rows.
- Code fix: the watchdog no longer re-registers solely because Plex marks a device `dead` while mappings remain healthy; activation request timeout errors redact token-bearing URLs.
- Verification at that point: `./scripts/verify` passed.
