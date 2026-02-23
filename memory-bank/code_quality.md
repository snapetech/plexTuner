# Code and doc quality

Short rules so "done" is consistent and docs don't rot.

## Docs as code

- Docs are **PR-reviewed**, **versioned**, and follow the [Diátaxis](../docs/index.md) layout.
- **Docs/examples must be copy/paste safe:** no smart quotes, NBSP, or typographic dashes in command blocks; use ASCII equivalents. See [recurring_loops.md](recurring_loops.md) § Curly quotes / special characters.
- **"Done" includes doc updates** when behavior, interfaces, or config change: update or add one doc (how-to, reference, or explanation); add See also / Related ADRs. See [current_task.md](current_task.md) § Docs.
- Doc gaps out of scope → file in [opportunities.md](opportunities.md) (operability/maintainability).
- Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md) (relative links, frontmatter, See also on every doc).

## Verification

- **`scripts/verify`** runs format, lint, test, build. CI runs only this. Commands: [memory-bank/commands.yml](commands.yml).

## Security

- Before landing: quick pass with [memory-bank/skills/security.md](skills/security.md). Concerns → opportunities.md (security category).
