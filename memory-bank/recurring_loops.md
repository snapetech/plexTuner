# Recurring loops and hard-to-solve problems

<!-- Document patterns that keep coming back: agentic loops, bugfix loops, and fragile areas. -->

<!-- For each entry include:
  1. What keeps happening (symptom / mistake)
  2. Why it's tricky (root cause / constraint)
  3. What works (concrete fix or rule)
  4. Where it's documented (if applicable)
-->

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
- If the *product requirement* needs typographic quotes, store them in source as Unicode but handle them safely (see below).

**Exit ramps (choose one that fits)**

1) **Normalize to ASCII (fastest fix)**
- Replace smart quotes with ASCII:
  - Curly double `"` `"` → `"`
  - Curly single `'` `'` → `'`
- Also watch for: en dash `–` vs hyphen `-`, ellipsis `…` vs `...`, non-breaking space U+00A0.

2) **Use a file, not inline strings (most robust)**
- Put the exact content into a file, then reference the file.
  - Example: JSON/YAML/template content → `cat file | tool ...`
- This avoids shell quoting entirely.

3) **Use Unicode escapes in JSON and programmatic edits**
- In JSON, use escapes like:
  - `\u201C` (left double quote), `\u201D` (right double quote)
  - `\u2018` (left single quote), `\u2019` (right single quote)
  - `\u00A0` (NBSP), `\u2013` (en dash), `\u2026` (ellipsis)
- If generating JSON from code, prefer a real serializer (no hand-built JSON strings).

4) **Prefer "read from stdin" over shell-quoted arguments**
- Many tools accept stdin; use it:
  - `python - <<'PY' ... PY` (single-quoted heredoc delimiter prevents expansion)
  - `node <<'JS' ... JS`
- For `jq`, prefer: `jq -f script.jq input.json` instead of complex one-liners.

5) **Detect & strip smart punctuation when needed**
- Quick sanity check in a file:
  - Look for common offenders (2018/2019/201C/201D/00A0/2013/2026).
- If found and not desired, normalize the file contents before continuing.

**What to do when you hit this loop**
- STOP trying to "escape harder" in the shell after 2 attempts.
- Switch to: "file-based edit" or "unicode escape + serializer" approach.
- Add a note in `memory-bank/current_task.md` under Evidence showing which character was present and where it came from (doc copy/paste, editor autocorrect, etc.).

**Reference: common code points**
- U+201C left double quotation mark  → `\u201C`
- U+201D right double quotation mark → `\u201D`
- U+2018 left single quotation mark  → `\u2018`
- U+2019 right single quotation mark → `\u2019`
- U+00A0 non-breaking space         → `\u00A0`
- U+2013 en dash                    → `\u2013`
- U+2026 ellipsis                   → `\u2026`

(Add more traps above as they recur.)

---

## Design constraints worth remembering (from spec)

- **VODFS contract:** Only present a file as "ready" once it has a known size (materialized or indexed). Plex is byte-range/seek-heavy; HLS-as-file before size is known causes scan/seek failures. Use `.partial` → final rename when cache is complete.
- **No transcoding:** Materializer uses remux-copy only; keeps CPU low and behavior predictable.
- **Stable paths/inodes:** Files and paths must not rename or change identity on refresh so Plex and continue-watching stay consistent.
