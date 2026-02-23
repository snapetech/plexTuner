# Agentic Go Service Template

A **reusable template** for Go services with an agent-friendly workflow: single source of truth for commands, memory-bank for state, Diátaxis docs, and one verify loop for CI.

- **Agents:** Read [AGENTS.md](AGENTS.md) and the [memory-bank/](memory-bank/) (including `recurring_loops.md`) before making changes.
- **After "Use this template":** Run [scripts/init-template](scripts/init-template) and follow [TEMPLATE.md](TEMPLATE.md).

## What this template includes

| Keep | Purpose |
|------|--------|
| **AGENTS.md** + **memory-bank/** | Agent workflow, commands, loops, opportunities, repo_map. |
| **scripts/verify** | Single CI entrypoint: format → lint → test → build. |
| **docs/** | Diátaxis layout (how-to, reference, explanations, adr, runbooks), frontmatter, glossary. |
| **.github/** | CI workflow, CodeQL, Gitleaks, Dependabot, CODEOWNERS (replace placeholder). |
| **cmd/hello** | Minimal default app so the template builds and tests out of the box. |
| **.env.example** | Generic app config pattern; copy to `.env`, never commit. |

| Optional / remove | Purpose |
|-------------------|--------|
| **extras/** | Optional scripts; not part of the template core. |

## Commands (generic)

Verification (run by CI and locally):

- **Format:** `go fmt ./...`
- **Lint:** `go vet ./...`
- **Test:** `go test ./...`
- **Build:** `go build -o /dev/null ./cmd/...`

Default run: `go run ./cmd/hello`. Authoritative list: [memory-bank/commands.yml](memory-bank/commands.yml).

## Setup

1. **Clone or "Use this template"** and run **`./scripts/init-template`** with your module name, project name, and owner email.
2. Replace placeholders in `.github/CODEOWNERS`, `docs/how-to/first-push.md`, and any runbook/service templates.
3. Copy `.env.example` to `.env` and add your app-specific variables (never commit `.env`).
4. Run **`./scripts/verify`** to confirm the repo is green.

## Docs

- **Doc map:** [docs/index.md](docs/index.md)
- **First push:** [docs/how-to/first-push.md](docs/how-to/first-push.md)
