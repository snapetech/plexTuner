# Recurring loops and hard-to-solve problems

<!-- Document patterns that keep coming back: agentic loops, bugfix loops, and fragile areas. -->

<!-- For each entry include:
  1. What keeps happening (symptom / mistake)
  2. Why it's tricky (root cause / constraint)
  3. What works (concrete fix or rule)
  4. Where it's documented (if applicable)
-->

(No entries yet.)

---

## Design constraints worth remembering (from spec)

- **VODFS contract:** Only present a file as "ready" once it has a known size (materialized or indexed). Plex is byte-range/seek-heavy; HLS-as-file before size is known causes scan/seek failures. Use `.partial` → final rename when cache is complete.
- **No transcoding:** Materializer uses remux-copy only; keeps CPU low and behavior predictable.
- **Stable paths/inodes:** Files and paths must not rename or change identity on refresh so Plex and continue-watching stay consistent.
