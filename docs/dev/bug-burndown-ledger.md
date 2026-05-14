# Bug Burndown Ledger — TEMPLATE

Copy into `docs/dev/bug-burndown-ledger.md`. The ledger is the canonical list of accepted bugs. Sweep registers reference it; CI gates check that ledger row IDs are present.

Use any prefix that is meaningful for your project (e.g. `RT-`, `BUG-`, `COUNCIL-`). Keep the prefix monotonic so a row ID never collides.

## Format

```
| ID | Title | Severity | Status | Sweep | Notes |
| --- | --- | --- | --- | --- | --- |
| RT-001 | Untrusted count flows into byte allocation | High | Verified | bug-council-sweep-protocol-length-2026-05-05.md | ProtocolCountReader.ReadValidatedCount enforced; behavior pinned. |
```

Status values:

- `New` — candidate filed, not yet triaged.
- `Accepted` — council confirmed it is a real bug; fix in flight.
- `Fixed` — fix landed; not yet pinned.
- `Verified` — fix landed AND behavior-pinned by a test (per `bug-council-behavior-pinning.md`).
- `Reverted` — fix had to be backed out; reason in Notes.
- `WontFix` — closed without action; reason in Notes.

Severity values come from `bug-council-severity-schema.md`.

## Conventions

- Add new rows at the bottom; do not renumber.
- One row per finding, not per fix. A single underlying bug fixed in three places is still one row, with three fix references in Notes.
- Cross-link from sweep registers using the row ID (e.g. `RT-101`). Cross-link from baseline `require_pattern` lines so a removed pattern is the same signal as a removed row.
