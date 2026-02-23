# Current task

<!-- Update at session start and when focus changes. -->

**Goal:** Phases 1–4 done. Next: Phase 5 (artwork, collections, health) or polish (Docker Compose, docs).

**Scope:** Phase 1–4 implemented: catalog (movies + series + live), indexer M3U, VODFS + Cache materializer (direct + HLS via ffmpeg), tuner serve (HDHR + XMLTV + gateway). Commands: `index`, `mount` (optional `-cache`), `serve` (tuner).

**Next steps:** Phase 5 (artwork, collections, health) or SSDP so Plex can auto-discover tuner. See docs/STORIES.md.

**One-run DVR:** `plex-tuner run` does index + health check + serve; errors surface to console with `[ERROR]`; Plex one-time setup URLs printed at startup. systemd example: `docs/systemd/plextuner.service.example`.

**Last updated:** 2025-02-22

---

## Assumptions & questions (only if uncertainty matters)
Assumptions (safe defaults you are proceeding with):
- <assumption + why it's safe>

Questions (ONLY if blocked or high-risk ambiguity):
- Q: <question>
  Why it matters: <risk/rework avoided>
  Suggested default: <what you recommend if user doesn't care>

## Opportunity radar (don't derail)
- If you notice out-of-scope improvements, record them in `memory-bank/opportunities.md` and raise to the user in your summary.

## Self-check (quality bar — fill before claiming done)
- **Correctness:** ✅ / ⚠️ (why)
- **Tests:** ✅ / ⚠️ (what's missing)
- **Risk:** low / med / high (why)
- **Performance impact:** none / likely / unknown
- **Security impact:** none / likely / unknown
(Feeds opportunities if any ⚠️ or impact; no need to ask permission for every item.)

## Decisions (single source of truth)
- If you make a **durable** decision (design, tech choice, contract), promote it to **ADR** (`docs/adr/`) or **memory-bank** (e.g. known_issues / recurring_loops with "why").
- If you're **unsure whether it's durable**, don't promote yet — note it in Assumptions and revisit when stable.

## Docs (done = doc update when behavior changes)
- If you changed **behavior, interfaces, or config:** update or create **one** doc in `docs/` (how-to, reference, or explanation); add cross-links (See also, Related ADRs). Use [Diátaxis](../docs/index.md): `tutorials/`, `how-to/`, `reference/`, `explanations/`, `adr/`, `runbooks/`. Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md).
- If you **noticed doc gaps** but it's out of scope: file in `memory-bank/opportunities.md` (category: operability or maintainability).
