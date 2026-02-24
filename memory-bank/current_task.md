# Current task

<!-- Update at session start and when focus changes. -->

**Goal:** Verify and harden the pending Plex Tuner changes (atomic catalog save/tests, subscription glob, fetchCatalog refactor) so they are safe to run with Plex. Verification, local tuner smoke, live Plex integration checks, and full 13-category Threadfin DVR insertion/activation checks are complete; main remaining issues are WebSafe metadata slowness with very large lineups and upstream feed/EPG sparsity collapsing the 13-way split.

**Scope:** In: run project verification (`scripts/verify`) on current uncommitted changes, fix any regressions found, and validate Plex integration in the k3s environment (`plex.home`, Plex API tune path, `plextuner-websafe`, Threadfin 13-category split/DVR pipeline). Out: new features, unrelated refactors, permanent infra redesign.

**Last updated:** 2026-02-24

---

## Assumptions & questions (only if uncertainty matters)
Assumptions (safe defaults you are proceeding with):
- Local environment may not have Go installed; OK to use a temporary local Go toolchain (non-system install) only for verification.
- k3s/Plex troubleshooting changes on remote hosts may be temporary runtime fixes unless later codified in infra manifests or host firewall config.

Questions (ONLY if blocked or high-risk ambiguity):
- Q: Do you want WebSafe to keep external XMLTV (slow but richer guide) or prioritize Plex responsiveness by using placeholder guide / cached XMLTV?
  Why it matters: Current live `guide.xml` with external XMLTV was ~45s and stalled Plex DVR metadata flows; disabling it made `guide.xml` ~0.2s and restored fast Plex tuner polling/tune start.
  Suggested default: Prioritize responsiveness for now (fast placeholder guide), then implement XMLTV caching as a follow-up.

## Opportunity radar (don't derail)
- If you notice out-of-scope improvements, record them in `memory-bank/opportunities.md` and raise to the user in your summary.

## Self-check (quality bar — fill before claiming done)
- **Correctness:** ✅ `scripts/verify` passed; local tuner smoke passed; live Plex API `tune` (DVR 138 / channel 11141) returned `200` and triggered `/stream/11141` successfully in `plextuner-websafe`; 13 Threadfin DVRs were created in Plex and the 6 non-empty buckets were activated/mapped successfully (91 channels total).
- **Tests:** ⚠️ Real Plex integration was exercised (WebSafe + Threadfin multi-DVR path), but Plex DVR channel metadata APIs remain slow with ~41k-channel WebSafe lineup; `plextunerTrial` service (`:5004`) is down in the k3s test pod; current Threadfin split output is only 91 channels due source/EPG coverage.
- **Risk:** med (runtime fixes applied on infra hosts/pod; not yet codified/persisted)
- **Performance impact:** observed bottlenecks on external XMLTV remap and oversized lineup metadata (operational, not code changes in this session); Threadfin split volume limited by source/EPG linkability rather than local CPU/network.
- **Security impact:** none (token used in-container only; not printed)

## Decisions (single source of truth)
- If you make a **durable** decision, promote it to **ADR** (`docs/adr/`) or **memory-bank**.
- If you're **unsure whether it's durable**, don't promote yet — note it in Assumptions.

## Docs (done = doc update when behavior changes)
- If you changed **behavior, interfaces, or config:** update or create **one** doc in `docs/`; add cross-links. Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md).
- If you **noticed doc gaps** but it's out of scope: file in `memory-bank/opportunities.md`.
