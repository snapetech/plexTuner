# Agent instructions — Plex Tuner

Instructions for AI agents and maintainers. **This repo is Plex Tuner** (IPTV→Plex bridge); it also uses the agentic template workflow (memory-bank, verify, docs). Main app: [cmd/plex-tuner](cmd/plex-tuner/main.go). Navigation: [memory-bank/repo_map.md](memory-bank/repo_map.md).

**Tool-compat:** Tools that look for `agents.md` can use [agents.md](agents.md), which points here. Commands are authoritative in **`memory-bank/commands.yml`**; CI runs **`scripts/verify`** (no drift).

---

## Commands (authoritative)

Do not guess commands. Use **`memory-bank/commands.yml`** (machine-readable). CI runs `scripts/verify`, which runs **`scripts/verify-steps.sh`** if present (format → lint → test → build or whatever the project defines).

If `scripts/verify-steps.sh` is missing, verify exits success so the template stays green until you add your own checks. Update **`memory-bank/commands.yml`** when you add or change verification steps.

---

## Memory bank (required)

**Maintain a `/memory-bank/` directory** and use it. It is the single source of truth for project state that should persist across sessions and handoffs.

### Files to keep updated

| File | Purpose |
|------|--------|
| **`memory-bank/current_task.md`** | What is being worked on *right now*: goal, scope, and next steps. Update at session start and when focus changes so the next agent (or you later) knows where things left off. |
| **`memory-bank/known_issues.md`** | Known bugs, limitations, and design tradeoffs. Add when you discover or fix something non-obvious so others don't re-fight the same battles. |
| **`memory-bank/recurring_loops.md`** | **Critical:** Recurring agentic loops, bugfix loops, and hard-to-solve problems that keep coming back. Document them here with: what keeps happening, why it's tricky, and what actually works. Future agents should read this first to avoid repeating the same mistakes and to get early warnings about fragile areas. |
| **`memory-bank/opportunities.md`** | Lightweight backlog for security/perf/reliability/maintainability/operability discoveries. File out-of-scope improvements here; raise to user in summary. |
| **`memory-bank/task_history.md`** | Append-only log of completed tasks (summary, verification, opportunities filed). |
| **`memory-bank/repo_map.md`** | Navigation: main entrypoints, key modules, hot paths, no-go zones. Check before wandering. |
| **`memory-bank/commands.yml`** | Machine-readable verification commands; `scripts/verify` runs `scripts/verify-steps.sh` (which should align with this file). |
| **`memory-bank/code_quality.md`** | Code and doc quality: "done" includes doc updates when behavior changes; docs as code. |
| **`memory-bank/work_breakdown.md`** | Multi-PR epics only: WBS + story list + PR plan. Use when >1 PR or scope warrants it; see guardrail below. |

Doc layout (Diátaxis): **`docs/index.md`** (tutorials, how-to, reference, explanations, adr, runbooks). New docs: frontmatter + See also; see `docs/_meta/linking.md`.

### How to use the memory bank

- **At session start:** Read `current_task.md`, `known_issues.md`, `recurring_loops.md`, and `repo_map.md` (navigation) before making changes.
- **When taking on work:** Set or update `current_task.md` (goal, scope, next step).
- **When hitting a recurring or painful issue:** Add or update `recurring_loops.md` with the pattern and the resolution (or "still open").
- **When discovering a limitation or bug:** Add to `known_issues.md` with enough context to reproduce or work around.

Phrase entries so that **another agent** can act on them: concise, actionable, and with "why" where it helps.

### Work breakdown (multi-PR only) — guardrail

- **One PR:** Use **only** `current_task.md` (objective, success criteria, In/Out, plan, verification). No work breakdown.
- **More than one PR:** You **must** create or update the work breakdown ([memory-bank/work_breakdown.md](memory-bank/work_breakdown.md) or [docs/epics/EPIC-*.md](docs/epics/EPIC-template.md)) and **only work on items listed there**. Every task/PR must reference a story ID. Park improvements in `opportunities.md`; don't hijack the epic.

**When to create a work breakdown:** Any of: will take >1 PR, touches >~5 modules/areas, multiple stakeholders/UX choices, non-trivial migration/rollout, meaningful security/perf implications. **When to use which:** PR-sized → `current_task.md` only; product scope → `docs/product/PRD-template.md`; multi-PR epic → `work_breakdown.md` or `docs/epics/EPIC-*.md`; story = row in that breakdown (or `docs/stories/STORY-template.md` for format).

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

For how to ask well: `memory-bank/skills/asking.md`. Security checklist before landing: `memory-bank/skills/security.md`. Dangerous ops (backup/rollback note required): `memory-bank/skills/dangerous_ops.md`. Quick repo orientation: `memory-bank/skills/repo_orientation.md`.

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

**Scope guard (no drive-by refactors):** If it isn't in In/Out, don't touch it — file it as an opportunity. Keeps improvement-hunting from becoming a refactor spree.

**Definition of done:** Done = green verification + task_history entry + docs updated if behavior changed + opportunities filed if discovered.

---

## Recurring loops and hard problems (emphasis)

Some issues show up again and again: agents or humans fix something, then the same class of problem reappears elsewhere or after a refactor. **Those belong in `memory-bank/recurring_loops.md`.**

Examples of what to record there:

- **Agentic loops:** e.g. "Agent repeatedly changes X to fix Y, which breaks Z; the real fix is W."
- **Bugfix loops:** e.g. "Bug in component A keeps reappearing because B assumes C; document that B must never assume C."
- **Hard-to-solve / easy-to-regress:** e.g. "Component A assumes B is immutable; changing B at runtime causes subtle bugs—document the invariant and add tests."

When you add an entry, include:

1. **What keeps happening** (the recurring symptom or mistake).
2. **Why it's tricky** (root cause or constraint).
3. **What works** (concrete fix or rule to follow).
4. **Where it's documented** (doc link or code path) if applicable.

The goal is to give future agents **early warnings** and **concrete guidance** so they don't re-enter the same loops.

---

## Project overview (for context)

This repo is an **agentic repo template** for **any language or stack**: memory-bank workflow, single verify script, Diátaxis docs. Add your own code and define verification in `scripts/verify-steps.sh` and `memory-bank/commands.yml`.

**After cloning from template:** Run `scripts/init-template`; replace placeholders in README, CODEOWNERS, and docs. See [TEMPLATE.md](TEMPLATE.md). The memory bank holds **current state**, **known issues**, and **recurring loops**. When behavior/config changes, update or add one doc and file gaps in `opportunities.md`; see [memory-bank/code_quality.md](memory-bank/code_quality.md).
