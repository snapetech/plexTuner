# Skill: Git-first workflow (staging, commit/push cadence, and history search)

## Git is a first thought (always)
- Before doing anything: `git status`
- While editing: check yourself constantly:
  - `git diff` (what changed?)
  - `git diff --staged` (what will be committed?)
- Never claim "done" with a dirty tree unless the task explicitly allows it.

## Staging discipline (craft commits, don't dump)
Goal: **atomic commits** (one logical change per commit).

Preferred workflow:
1) Review changes: `git diff`
2) Stage intentionally:
   - Use patch staging: `git add -p`
   - Stage file-by-file only when it's already cleanly separated
3) Re-check staged diff: `git diff --staged`
4) Commit with a message that explains intent (not mechanics)

Rules:
- Do NOT mix unrelated changes in one commit.
- If you touched unrelated files while exploring, split them out or revert them.
- If you need to reorder/split, use patch staging instead of "I'll fix it later".

## Commit frequency (cheap locally, meaningful logically)
Commit when you have **one coherent unit**:
- a bugfix + its regression test
- a refactor that preserves behavior (with tests still passing)
- a small feature slice (vertical slice preferred)

Avoid:
- "every 5 minutes" noise commits
- "one giant commit at the end" (hard to review/revert)

## Push frequency (useful, not spammy)
Push when:
- You hit a coherent checkpoint **and** it passes the repo's verification commands (format/lint/tests/build as applicable)
- You're switching tasks / contexts
- You're about to do risky git operations (rebase, large conflict resolution)
- You need feedback / CI signal

Avoid:
- pushing half-broken checkpoints unless explicitly requested
- pushing micro-commits that you intend to immediately rewrite (prefer local amend/fixup first)

## Commit message rules (readable + machine-usable)
- Use a clear subject line (imperative voice): "Fix…", "Add…", "Refactor…"
- Prefer Conventional Commits:
  - `feat(scope): ...`
  - `fix(scope): ...`
  - `refactor(scope): ...`
  - `test(scope): ...`
  - `docs(scope): ...`
  - `chore(scope): ...`
- If it's breaking, say so (footer / BREAKING CHANGE).

## History/search protocol (don't thrash, don't cherry-pick randomly)
Before searching, write down:
- What are you trying to learn? (who/what/when/why)
- Do you need **latest** behavior or **origin** of a change?
- What scope? (file/path, module, branch, time window)

### 1) Start narrow, then widen (default order)
A) Working tree / HEAD content:
- `git grep -n "needle" -- path/if/known`
- (If you have ripgrep available, use it too; but git grep respects repo context.)

B) Recent commits (don't list all time if you only need recent):
- `git log -n 20 --oneline --decorate`
- Add constraints early:
  - time: `git log --since="YYYY-MM-DD" -n 50`
  - author: `git log --author="name" -n 50`
  - message: `git log --grep="needle" -n 50`

C) Find when something was introduced/changed (diff-aware search):
- Exact string occurrence count changes ("pickaxe"):
  - `git log -S "needle" -n 50 --oneline`
- Regex in diffs:
  - `git log -G "regex" -n 50 --oneline`
- Show the patches:
  - add `-p` to any of the above when you need evidence

D) File evolution:
- `git log --follow -n 50 -- path/to/file`

E) Function/line history (surgical, stops wandering):
- `git log -L :function_name:path/to/file`
- Or line range: `git log -L start,end:path/to/file`

F) Regression hunting (when you know "good" vs "bad"):
- `git bisect start`
- mark endpoints:
  - `git bisect bad`
  - `git bisect good <rev>`
- test each step, then `git bisect reset`

### 2) Result selection rules (this prevents "random commit syndrome")
- If you limit output (e.g., `-n 5`), SAY you did, and how to expand (`-n 50`, add/remove `--since`, etc.).
- If you need "newest", default to newest. Do not cite older commits unless asked or unless the older commit is the origin you're tracing.
- If the query can match many things, start with last 20–50, then refine by path/time/message. Don't blindly surface the first 5 hits.
- Always attach evidence when making claims: show the `git log ... -p` diff or the exact commit hash + file path.

## When to ask the user (don't ask for permission constantly)
Ask only if:
- choosing between multiple plausible histories/interpretations changes behavior materially
- you're about to rewrite history on a shared branch
- the "best" fix implies scope expansion (new deps, major refactor, migrations)

Otherwise:
- proceed with safe defaults
- document assumptions in `memory-bank/current_task.md`

---

Sources (for reference):
- Interactive staging + atomic commits: [Atlassian – Saving changes](https://www.atlassian.com/git/tutorials/saving-changes)
- Commit message structure: [Atlassian – Git commit](https://www.atlassian.com/git/tutorials/saving-changes/git-commit)
- Conventional Commits: [conventionalcommits.org](https://www.conventionalcommits.org/en/v1.0.0/)
- History search: [Git – Searching](https://git-scm.com/book/en/v2/Git-Tools-Searching)
- Regression pinpointing: [git bisect](https://git-scm.com/docs/git-bisect)
