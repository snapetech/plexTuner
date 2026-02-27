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

- **Goal (2–5 sentences):** [Describe the overall objective]
- **Non-goals (scope fence):** [What's explicitly out of scope]

### Milestones (vertical slices)

| Milestone | Done = verifiable outcomes |
|-----------|----------------------------|
| M1: [Name] | [Verifiable outcome] |
| M2: [Name] | [Verifiable outcome] |

### Story list (granular, scoped)

For each story:

| ID | Acceptance criteria | Files/areas (expected) | Verification | Risk flags |
|----|---------------------|------------------------|--------------|------------|
| STORY-001 | [What done looks like] | [Expected files] | [How to verify] | [Risks] |

### PR plan

| PR | Scope |
|----|--------|
| PR-1 | [Stories included] |

### Decision points (needs user input)

- Decision: [Question] → Options: (A) ..., (B) ..., (C) ... → Default if no answer: [A/B/C]

---

## When not active

Leave this file as-is. Use **current_task.md** as the single plan for PR-sized work.
