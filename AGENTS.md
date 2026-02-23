# Agent instructions — Plex IPTV Tuner

Instructions for AI agents and maintainers working on this repo.

---

## Memory bank (required)

**Maintain a `/memory-bank/` directory** and use it. It is the single source of truth for project state that should persist across sessions and handoffs.

### Files to keep updated

| File | Purpose |
|------|--------|
| **`memory-bank/current_task.md`** | What is being worked on *right now*: goal, scope, and next steps. Update at session start and when focus changes so the next agent (or you later) knows where things left off. |
| **`memory-bank/known_issues.md`** | Known bugs, limitations, and design tradeoffs. Add when you discover or fix something non-obvious so others don’t re-fight the same battles. |
| **`memory-bank/recurring_loops.md`** | **Critical:** Recurring agentic loops, bugfix loops, and hard-to-solve problems that keep coming back. Document them here with: what keeps happening, why it's tricky, and what actually works. Future agents should read this first to avoid repeating the same mistakes and to get early warnings about fragile areas. |
| **`memory-bank/opportunities.md`** | Lightweight backlog for security/perf/reliability/maintainability/operability discoveries. File out-of-scope improvements here; raise to user in summary. |
| **`memory-bank/task_history.md`** | Append-only log of completed tasks (summary, verification, opportunities filed). |

### How to use the memory bank

- **At session start:** Read `current_task.md`, `known_issues.md`, and `recurring_loops.md` before making changes.
- **When taking on work:** Set or update `current_task.md` (goal, scope, next step).
- **When hitting a recurring or painful issue:** Add or update `recurring_loops.md` with the pattern and the resolution (or “still open”).
- **When discovering a limitation or bug:** Add to `known_issues.md` with enough context to reproduce or work around.

Phrase entries so that **another agent** can act on them: concise, actionable, and with “why” where it helps.

---

## Uncertainty policy (ask only when it matters)

Agents should not get bogged down asking permission. Ask questions only when:

- requirements are ambiguous in a way that risks rework or wrong behavior
- a change could break compatibility, security posture, or data integrity
- the task would expand scope materially (new features, new deps, major refactor)
- there are multiple plausible interpretations with different outcomes

If uncertain but not blocked:

- make the safest reasonable assumption
- document it in `memory-bank/current_task.md` under "Assumptions"
- proceed and keep the diff small

For how to ask well: `memory-bank/skills/asking.md`

---

## Continuous improvement (without scope creep)

While working, keep an eye out for:

- security hazards (input validation, auth, secrets handling, injection paths)
- performance wins (allocations, IO, hot paths, caching)
- maintainability (clarity, duplication, brittle coupling)
- operability (logs, metrics, error messages, debuggability)

Policy:

- If it is a small, low-risk improvement that is clearly within scope, you may do it.
- If it expands scope or carries meaningful risk, log it to `memory-bank/opportunities.md` and raise it to the user in your summary.

---

## Recurring loops and hard problems (emphasis)

Some issues show up again and again: agents or humans fix something, then the same class of problem reappears elsewhere or after a refactor. **Those belong in `memory-bank/recurring_loops.md`.**

Examples of what to record there:

- **Agentic loops:** e.g. “Agent repeatedly changes X to fix Y, which breaks Z; the real fix is W.”
- **Bugfix loops:** e.g. “Bug in component A keeps reappearing because B assumes C; document that B must never assume C.”
- **Hard-to-solve / easy-to-regress:** e.g. “Plex expects stable file size and mtime; presenting HLS as a file before materialization causes scan/seek failures—only expose files after size is known (see VODFS contract).”

When you add an entry, include:

1. **What keeps happening** (the recurring symptom or mistake).
2. **Why it’s tricky** (root cause or constraint).
3. **What works** (concrete fix or rule to follow).
4. **Where it’s documented** (doc link or code path) if applicable.

The goal is to give future agents **early warnings** and **concrete guidance** so they don’t re-enter the same loops.

---

## Project overview (for context)

This repo is an **IPTV Tuner for Plex**:

- **Live TV:** HDHomeRun-style emulator (discover/lineup/lineup_status) + XMLTV guide + stream gateway.
- **VOD:** Virtual filesystem (VODFS, FUSE) so Plex sees real-looking Movies/TV paths; a **materializer** turns provider streams (including HLS) into local cached files (remux-only, no transcode) so Plex gets stable size/mtime and byte-range seeks.

**Full design and phased plan:** see **`docs/DESIGN.md`** (Strong8k assumptions, all components A–F, Plex naming rules, VODFS contract, stream auto-detect, deployment, Phases 1–5). The memory bank holds **current state**, **known issues**, and **recurring loops** so work stays consistent across sessions and agents.
