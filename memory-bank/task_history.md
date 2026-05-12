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

## 2026-05-12 - Stop kspls0 Plex DVR zombie spam

- Root cause was duplicate registrars. Bare-metal systemd Tunerr services and the removed cluster Tunerr path were registering the same Plex device IDs/friendly names with different guide URLs. Plex accumulated empty `0/0` DVR rows and entered repeated `/livetv/dvrs` timeout / maintenance windows.
- Live fix kept the host/systemd Tunerr services as the single owner, installed the patched binary, and deleted twelve empty IPTV DVR rows.
- Code fix: the watchdog no longer re-registers solely because Plex marks a device `dead` while mappings remain healthy; activation request timeout errors redact token-bearing URLs.
- Verification at that point: `./scripts/verify` passed.
