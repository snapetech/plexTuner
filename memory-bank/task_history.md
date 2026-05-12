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
