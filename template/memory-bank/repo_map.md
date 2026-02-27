# Repo map (navigation)

First place to look before editing. Keeps agents from thrashing.

**This is a template repository.** Use it as a starting point for new projects.

## Remotes

| Remote    | Repo           | Host   | Purpose                    |
|-----------|----------------|--------|----------------------------|
| **origin** | your-project   | GitLab | Primary remote             |
| **github** | your-project   | GitHub | Mirror (optional)          |

## Main entrypoints

Update this section with your project's actual entrypoints:

| Path | Purpose |
|------|--------|
| **`AGENTS.md`** | Agent instructions; **`memory-bank/`** = state + process. |
| **`scripts/verify`** | Verification script (format → lint → test → build). |

## Key modules

Add your project's key modules here as you develop them.

## No-go zones

- **`.env`** — Never commit; never log or echo. Credentials live only in env.
- **Generated/vendor** — Don't edit unless the task explicitly requires it.
- **Weakening tests** — Don't "fix" by loosening assertions; fix root cause.

## Verification and QA

- **`scripts/verify`** — Full check: format → lint → test → build. Fail fast, same as CI.
- **`scripts/quick-check.sh`** — Quick tests only; use for short feedback when iterating.
- CI runs only `scripts/verify`.

## Template onboarding

See [TEMPLATE.md](../TEMPLATE.md) for setup instructions after cloning this template.
