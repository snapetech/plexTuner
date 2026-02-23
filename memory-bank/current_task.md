# Current task

<!-- Update at session start and when focus changes. -->

**Goal:** Stream buffering and transcoding implemented.

**Scope:** In: config (PLEX_TUNER_STREAM_BUFFER_BYTES, PLEX_TUNER_STREAM_TRANSCODE), gateway buffering and remux/transcode choice, Server/main wiring, docs and .env.example. Out: (previous task) README/features/changelog.

**Last updated:** 2025-02-23

---

## Assumptions & questions (only if uncertainty matters)
Assumptions (safe defaults you are proceeding with):
- *(none)*

Questions (ONLY if blocked or high-risk ambiguity):
- Q: *(question)*
  Why it matters: *(risk/rework avoided)*
  Suggested default: *(what you recommend if user doesn't care)*

## Opportunity radar (don't derail)
- If you notice out-of-scope improvements, record them in `memory-bank/opportunities.md` and raise to the user in your summary.

## Self-check (quality bar — fill before claiming done)
- **Correctness:** ✅ / ⚠️ (why)
- **Tests:** ✅ / ⚠️ (what's missing)
- **Risk:** low / med / high (why)
- **Performance impact:** none / likely / unknown
- **Security impact:** none / likely / unknown

## Decisions (single source of truth)
- If you make a **durable** decision, promote it to **ADR** (`docs/adr/`) or **memory-bank**.
- If you're **unsure whether it's durable**, don't promote yet — note it in Assumptions.

## Docs (done = doc update when behavior changes)
- If you changed **behavior, interfaces, or config:** update or create **one** doc in `docs/`; add cross-links. Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md).
- If you **noticed doc gaps** but it's out of scope: file in `memory-bank/opportunities.md`.
