# Opportunities (Continuous Improvement Backlog)

This is a lightweight backlog for improvements discovered during other work.
It exists to encourage quality gains without derailing the current task.

## Rules
- Prefer evidence: link to code, test output, perf numbers, or a specific risk.
- Do NOT expand scope mid-task unless it is small, low-risk, and clearly aligned.
- If an item needs a product/UX decision or significant effort, raise it to the user.

## Entry template
- Date: YYYY-MM-DD
  Category: security | performance | reliability | maintainability | operability | other
  Title: <short>
  Context: <where you noticed it>
  Why it matters: <impact + who it affects>
  Evidence: <link/snippet/metric>
  Suggested fix: <concrete next step>
  Risk/Scope: <low/med/high> | <fits current scope? yes/no>
  User decision needed?: yes/no
  If yes: 2–3 options + recommended default + what you will do if no answer

## Entries

- Date: 2025-02-23
  Category: maintainability
  Title: Add or document internal/indexer dependency
  Context: README/docs pass; build fails without indexer.
  Why it matters: New clones cannot build; unclear whether indexer is external or missing.
  Evidence: `go build ./cmd/plex-tuner` → "no required module provides package .../internal/indexer".
  Suggested fix: Either add the indexer package to the repo (from another branch/repo) or document the dependency and build steps in README/reference.
  Risk/Scope: low | fits current scope: no (documented in docs-gaps).
  User decision needed?: yes (whether indexer lives in-repo or separate).
