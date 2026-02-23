# Repo map (navigation)

First place to look before editing. Keeps agents from thrashing.

## Main entrypoints

| Path | Purpose |
|------|--------|
| **`AGENTS.md`** | Agent instructions; **`memory-bank/`** = state + process. |
| **`docs/index.md`** | Doc map (Diátaxis). |
| **`scripts/verify-steps.sh`** | Project verification (format/lint/test/build); add your stack here. |

## No-go zones

- **`.env`** — Never commit; never log or echo. Credentials live only in env.
- **Generated/vendor** — Don't edit unless the task explicitly requires it.
- **Weakening tests** — Don't "fix" by loosening assertions; fix root cause.

## Verification

- **`scripts/verify`** — Runs `scripts/verify-steps.sh` if present (any language).
- CI runs only `scripts/verify`.
