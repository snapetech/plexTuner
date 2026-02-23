# Repo map (navigation)

First place to look before editing. Keeps agents from thrashing.

## Main entrypoints

| Path | Purpose |
|------|--------|
| **`cmd/hello/`** | Default app. Replace with your own `cmd/` entrypoint. |
| **`AGENTS.md`** | Agent instructions; **`memory-bank/`** = state + process. |
| **`docs/index.md`** | Doc map (Diátaxis). |

## No-go zones

- **`.env`** — Never commit; never log or echo. Credentials live only in env.
- **Generated/vendor** — Don't edit unless the task explicitly requires it.
- **Weakening tests** — Don't "fix" by loosening assertions; fix root cause.

## Verification

- **`scripts/verify`** — Runs format, lint, test, build (see `memory-bank/commands.yml`).
- CI runs only `scripts/verify`.
