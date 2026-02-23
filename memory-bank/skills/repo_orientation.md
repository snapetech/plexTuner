# Skill: Quick repo scan (fast orientation, low context)

Use this checklist before editing. Stops random exploration and thrash.

1. **Tree:** `ls` (or `ls -la`) at repo root — see top-level layout.
2. **State:** `git status` — clean or what changed?
3. **Recent history:** `git log -n 20 --oneline --decorate` — what landed lately?
4. **Build/entrypoints:** Find build files (e.g. `Makefile`, `go.mod`, `package.json`, `Cargo.toml`, `docker-compose.yml`) — `find . -maxdepth 3 -type f \( -name Makefile -o -name go.mod -o -name package.json -o -name Cargo.toml -o -name docker-compose.yml \) 2>/dev/null`.
5. **Tests:** Where are tests? (e.g. `*_test.go`, `tests/`, `spec/`, `__tests__/`) — `find . -type d -name test* -o -name spec -o -name __tests__ 2>/dev/null | head -20`.
6. **CI:** What runs in CI? — `.github/workflows/*.yml` or `.gitlab-ci.yml` or similar; read the main workflow.
7. **Commands:** Authoritative commands — `memory-bank/commands.yml` or README/AGENTS.md.
8. **Entrypoint doc:** `AGENTS.md` or `agents.md` — read it; then `memory-bank/current_task.md`, `memory-bank/repo_map.md`.
9. **Config/secrets:** Where is config? (`.env.example`, `config/`, env vars in README) — never commit secrets.
10. **Docs map:** `docs/index.md` (if present) — where do explanations vs how-tos live?

Then start work. Don't edit widely until you've done at least 1–4 and 8.
