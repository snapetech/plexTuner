# Current task

<!-- Update at session start and when focus changes. -->

**Goal:** Land and verify the direct PlexTuner playback hardening work for Plex clients: (1) `/auto/v<guide-number>` fallback when `channel_id` is non-numeric, and (2) capability-based Plex client adaptation with a safe default (`unknown => websafe`, resolved non-web => full). Preserve the no-placeholder direct WebSafe path (real XMLTV + EPG-linked + deduped lineup/guide) from the live k3s testing.

**Scope:** In: commit the pending `internal/tuner/gateway*` changes and update memory-bank documentation with the latest direct PlexTuner findings (lineup/guide mismatch root cause, WebSafe fix, remaining Plex Web `start.mpd` failure). Out: Threadfin pipeline changes, infra/firewall persistence, large refactors, and Plex Web DASH deep debugging beyond documenting the current failure point.

**Last updated:** 2026-02-24

---

## Assumptions & questions (only if uncertainty matters)
Assumptions (safe defaults you are proceeding with):
- Local environment may not have Go installed; OK to use a temporary local Go toolchain (non-system install) only for verification.
- k3s/Plex troubleshooting changes on remote hosts may be temporary runtime fixes unless later codified in infra manifests or host firewall config.

Questions (ONLY if blocked or high-risk ambiguity):
- Q: None currently blocking for this patch-sized change.

## Opportunity radar (don't derail)
- If you notice out-of-scope improvements, record them in `memory-bank/opportunities.md` and raise to the user in your summary.

## Self-check (quality bar — fill before claiming done)
- **Correctness:** ✅ Targeted gateway tests pass and `go build ./cmd/plex-tuner` succeeds with the temporary Go toolchain (`/tmp/go`). Live k3s testing already proved the direct PlexTuner WebSafe path can `tune` and relay bytes after real XMLTV + lineup dedupe fixes.
- **Tests:** ⚠️ Full `./scripts/verify` is currently blocked by unrelated repo-wide `gofmt -s` drift in tracked files (`internal/config/config.go`, `internal/hdhomerun/*.go`) not touched in this task. Remaining runtime failure is Plex Web browser playback timing out on DASH `start.mpd` after a successful tune/stream start.
- **Risk:** med (behavior change in client adaptation defaults for unknown Plex clients; guarded by `PLEX_TUNER_CLIENT_ADAPT`)
- **Performance impact:** no material regression expected from this patch; direct WebSafe performance still depends on lineup size and XMLTV remap cost (documented in known issues/opportunities).
- **Security impact:** none (token used in-container only; not printed)

## Decisions (single source of truth)
- If you make a **durable** decision, promote it to **ADR** (`docs/adr/`) or **memory-bank**.
- If you're **unsure whether it's durable**, don't promote yet — note it in Assumptions.

## Docs (done = doc update when behavior changes)
- If you changed **behavior, interfaces, or config:** update or create **one** doc in `docs/`; add cross-links. Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md). (Memory-bank updates are in scope for this patch; broader docs can follow if this behavior is promoted.)
- If you **noticed doc gaps** but it's out of scope: file in `memory-bank/opportunities.md`.
