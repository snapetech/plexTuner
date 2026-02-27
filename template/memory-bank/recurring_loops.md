# Recurring loops and hard-to-solve problems

Document patterns that keep coming back: agentic loops, bugfix loops, and fragile areas.

For each entry include:
  1. What keeps happening (symptom / mistake)
  2. Why it's tricky (root cause / constraint)
  3. What works (concrete fix or rule)
  4. Where it's documented (if applicable)

## Loop protocol
- If you attempt the same approach twice and it still fails, STOP.
- Collect evidence (errors, logs, repro steps).
- Pick an alternate strategy (exit ramp) before trying again.

## Common traps + exit ramps

### Permission paralysis
- **Symptom:** Agent asks for approval on every micro-step; progress stalls.
- **Exit ramp:** Follow AGENTS.md uncertainty policy: proceed with safe assumptions, document them in `memory-bank/current_task.md`, ask only when ambiguity is blocking or high-risk. See `memory-bank/skills/asking.md`.

### Tooling mismatch / guessed commands
- **Symptom:** Invented flags/commands/config keys.
- **Exit ramp:** Use repo README and existing scripts; run real commands; update docs if you add new ones.

### "Fixing" by silencing failures
- **Symptom:** Weakening assertions, skipping tests, broad try/catch.
- **Exit ramp:** Revert; add minimal repro; fix root cause; keep tests strict.

### Context thrash
- **Symptom:** Touching unrelated files; refactor drift.
- **Exit ramp:** Tighten scope in `memory-bank/current_task.md`; stop editing unrelated areas.

### Repro-first for bugs
- **Symptom:** Hours of blind edits; "fix" doesn't stick or breaks something else.
- **Rule:** If it's a bug: add **repro steps or a failing test before** attempting fixes. Prevents random poking.

### Loop: Curly quotes / special characters break piping, sed, JSON, shells

**Symptom**
- Agent keeps failing at `sed`, `jq`, `python -c`, `node -e`, bash heredocs, etc.
- Errors like: `unexpected EOF`, `invalid character`, `unterminated string`, `invalid JSON`, `bad substitution`
- Root cause: "smart quotes" (curly quotes) or other Unicode punctuation got introduced (often from docs/ChatGPT), and the shell/toolchain treats them differently than ASCII quotes.

**Policy (preferred)**
- In repo docs and code examples, use **ASCII quotes only**:
  - Use `"` and `'` not `"` or `'` (curly).
- Avoid requiring fancy punctuation in commands/config where copy/paste matters.

**Exit ramps (choose one that fits)**

1) **Normalize to ASCII (fastest fix)**
- Replace smart quotes with ASCII:
  - Curly double `"` `"` → `"`
  - Curly single `'` `'` → `'`

2) **Use a file, not inline strings (most robust)**
- Put the exact content into a file, then reference the file.

3) **Use Unicode escapes in JSON and programmatic edits**
- In JSON, use escapes like `\u201C` (left double quote), `\u201D` (right double quote)

4) **Prefer "read from stdin" over shell-quoted arguments**
- Many tools accept stdin; use it:
  - `python - <<'PY' ... PY` (single-quoted heredoc delimiter prevents expansion)

**Reference: common code points**
- U+201C left double quotation mark  → `\u201C`
- U+201D right double quotation mark → `\u201D`
- U+2018 left single quotation mark  → `\u2018`
- U+2019 right single quotation mark → `\u2019`
- U+00A0 non-breaking space         → `\u00A0`

(Add more traps above as they recur.)
