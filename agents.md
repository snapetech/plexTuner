# agents.md (tool-compat alias)

Some tools look for `agents.md`, others for `AGENTS.md`. **Canonical instructions:** [AGENTS.md](AGENTS.md).

Use the **memory-bank** and workflow described there. Commands (authoritative) are in **`memory-bank/commands.yml`**; CI runs **`scripts/verify`**, which runs **`scripts/verify-steps.sh`** (your format/lint/test/build for any language).

Quick reference: define verification in **`scripts/verify-steps.sh`** and **`memory-bank/commands.yml`**. Do not guess commands — read the repo.

Navigation: **`memory-bank/repo_map.md`** — entrypoints, modules, no-go zones.
