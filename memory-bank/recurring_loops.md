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

(Add more traps above as they recur.)

---

## Design constraints worth remembering (from spec)

- **VODFS contract:** Only present a file as "ready" once it has a known size (materialized or indexed). Plex is byte-range/seek-heavy; HLS-as-file before size is known causes scan/seek failures. Use `.partial` → final rename when cache is complete.
- **No transcoding:** Materializer uses remux-copy only; keeps CPU low and behavior predictable.
- **Stable paths/inodes:** Files and paths must not rename or change identity on refresh so Plex and continue-watching stay consistent.
