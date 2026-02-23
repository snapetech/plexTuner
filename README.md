# Agentic Repo Template

A **reusable template** for any language or stack: agent-friendly workflow, single source of truth for commands, memory-bank for state, and Diátaxis docs.

- **Agents:** Read [AGENTS.md](AGENTS.md) and the [memory-bank/](memory-bank/) (including `recurring_loops.md`) before making changes.
- **After "Use this template":** Run [scripts/init-template](scripts/init-template) and follow [TEMPLATE.md](TEMPLATE.md).

## What this template includes

| Keep | Purpose |
|------|--------|
| **AGENTS.md** + **memory-bank/** | Agent workflow, commands, loops, opportunities, repo_map. |
| **scripts/verify** | Single CI entrypoint; runs your checks (see below). |
| **docs/** | Diátaxis layout (how-to, reference, explanations, adr, runbooks), frontmatter, glossary. |
| **.github/** | CI workflow, CodeQL, Gitleaks, Dependabot, CODEOWNERS (replace placeholder). |
| **.env.example** | Generic app config pattern; copy to `.env`, never commit. |

| Optional / remove | Purpose |
|-------------------|--------|
| **extras/** | Optional scripts; not part of the template core. |

## Verification (any language)

Define your own format/lint/test/build in **`scripts/verify-steps.sh`** (see `scripts/verify-steps.sh.example`). CI and `./scripts/verify` run that script. Authoritative command list: [memory-bank/commands.yml](memory-bank/commands.yml).

## Setup

1. **Clone or "Use this template"** and run **`./scripts/init-template`** with your repo URL, project name, and owner email.
2. Replace placeholders in `.github/CODEOWNERS`, `docs/how-to/first-push.md`, and any runbook templates.
3. Copy `.env.example` to `.env` and add your app-specific variables (never commit `.env`).
4. Add your code and **`scripts/verify-steps.sh`** (format/lint/test/build for your stack). Then run **`./scripts/verify`** to confirm the repo is green.

## Docs

- **Doc map:** [docs/index.md](docs/index.md)
- **First push:** [docs/how-to/first-push.md](docs/how-to/first-push.md)
