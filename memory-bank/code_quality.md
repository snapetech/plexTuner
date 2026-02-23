# Code and doc quality

Short rules so "done" is consistent and docs don't rot.

## Docs as code

- Docs are **PR-reviewed**, **versioned**, and follow the [Diátaxis](../docs/index.md) layout.
- **Docs/examples must be copy/paste safe:** no smart quotes, NBSP, or typographic dashes in command blocks; use ASCII equivalents. See [recurring_loops.md](recurring_loops.md) § Curly quotes / special characters.
- **"Done" includes doc updates** when behavior, interfaces, or config change: update or add one doc (how-to, reference, or explanation); add See also / Related ADRs. See [current_task.md](current_task.md) § Docs.
- Doc gaps out of scope → file in [opportunities.md](opportunities.md) (operability/maintainability).
- Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md) (relative links, frontmatter, See also on every doc).
- **If docs added:** run link check when available (or N/A). Sets the norm even if not wired in CI yet.

## Verification

- **`scripts/verify`** runs format, lint, test, build. CI runs only this. Commands: [memory-bank/commands.yml](commands.yml).

## Git workflow

- Atomic commits, patch staging (`git add -p`), and history search protocol: [memory-bank/skills/git.md](skills/git.md).

## Performance and resources

- Default posture, performance protocol, resource guardrails, and PR checklist: [memory-bank/skills/performance.md](skills/performance.md).

## Safety and orientation

- **Dangerous ops:** No destructive commands without backup/rollback note in current_task. [memory-bank/skills/dangerous_ops.md](skills/dangerous_ops.md).
- **Repo orientation:** Before editing, run the quick-scan checklist. [memory-bank/skills/repo_orientation.md](skills/repo_orientation.md).

## Security

- Before landing: quick pass with [memory-bank/skills/security.md](skills/security.md). Concerns → opportunities.md (security category).
