---
id: story-template
type: reference
status: stable
tags: [stories, template]
---

# STORY-XXX template

Use for **one granular deliverable** inside an epic. Copy into [memory-bank/work_breakdown.md](../../memory-bank/work_breakdown.md) or an epic doc; don’t create a file per story unless you need traceability in issues.

- **ID:** STORY-XXX
- **Acceptance criteria:** (testable)
- **Files/areas (expected):**
- **Verification:** e.g. `scripts/verify`, integration test
- **Risk flags:** migration / security / perf

**When to use which:** PR-sized task → [current_task.md](../../memory-bank/current_task.md). Multi-PR → [work_breakdown.md](../../memory-bank/work_breakdown.md) or [EPIC-*.md](../epics/EPIC-template.md); each PR references a story ID.

See also
--------
- [Epic template](../epics/EPIC-template.md)
- [Work breakdown](../../memory-bank/work_breakdown.md)
