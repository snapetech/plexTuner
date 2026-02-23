---
id: linking-rules
type: reference
status: stable
tags: [docs, linking, conventions]
---

# Linking and doc conventions

Rules that keep docs cross-reference friendly and portable.

## Linking rules

- **Use relative links** within the repo (portable across GitHub, GitLab, local).
- **Stable IDs:** Every doc has an `id` in frontmatter; use for future doc-site URLs/sidebars.
- **Every doc ends with:**
  - **See also** — other docs (required; can be empty list).
  - **Related ADRs** — when the doc reflects or depends on a decision (optional).
  - **Related runbooks** — when the doc ties to an ops procedure (optional).

## Naming and structure

- **File names:** `kebab-case.md`.
- **First H1** in the doc = document title (matches purpose).
- **Sections:** One idea per section; headings as noun phrases (e.g. "Token rotation", "Failure modes").

## How-to pattern

1. Goal  
2. Preconditions  
3. Steps  
4. Verify  
5. Rollback  
6. Troubleshooting  

## Reference docs

Keep reference pages **dense and factual** (no narratives). Commands in fenced blocks; show expected output only for key lines.

## Glossary

Maintain [../glossary.md](../glossary.md) and link key terms from other docs so vocabulary stays consistent.

See also
--------
- [Docs index](../index.md)
- [Frontmatter template](frontmatter-template.md)
