# Work breakdown (multi-PR work only)

**Use this file only when the work is bigger than one PR.** Otherwise use [current_task.md](current_task.md) only.

Create or update this file when **any** of these are true:

- Will take **>1 PR**
- Touches **>~5 modules/areas**
- Multiple stakeholders / UX choices
- Non-trivial migration or rollout
- Meaningful security or perf implications

**Rule:** If the task is multi-PR, you must create/update this (or [docs/epics/EPIC-*.md](../docs/epics/EPIC-template.md)) and **only work on items listed there**. Every task/PR must reference a story ID. Park out-of-scope ideas in [opportunities.md](opportunities.md).

---

## When active: fill the sections below (minimal, high-control)

### North star

- **Goal (2–5 sentences):**
- **Non-goals (scope fence):**

### Milestones (vertical slices)

| Milestone | Done = verifiable outcomes |
|-----------|----------------------------|
| M1: end-to-end thin slice | |
| M2: hardening (tests/edge cases) | |
| M3: perf/security/operability | |

### Story list (granular, scoped)

For each story:

| ID | Acceptance criteria | Files/areas (expected) | Verification | Risk flags |
|----|---------------------|------------------------|--------------|------------|
| STORY-001 | | | `scripts/verify` or … | migration/security/perf |
| STORY-002 | | | | |

### PR plan

| PR | Scope |
|----|--------|
| PR-1 | scaffolding + first slice |
| PR-2 | feature completion |
| PR-3 | hardening / migrations / docs |

### Decision points (needs user input)

- Decision: … → Options: … → Default if no answer: …

---

## When not active

Leave this file as-is. Use **current_task.md** as the single plan for PR-sized work.
