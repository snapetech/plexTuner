# Template onboarding

After **"Use this template"** or cloning this repo as a starting point, do the following so the repo is yours instead of a generic template.

## 1. Run the init script

```bash
./scripts/init-template --repo https://github.com/YOUR_ORG/YOUR_REPO --project "Your Project Name" --owner-email you@example.com
```

Replace placeholders with your repo URL (GitHub, GitLab, or any host), project name, and owner email. The script updates README, CODEOWNERS, and doc placeholders.

## 2. Replace placeholders by hand (if needed)

- **.github/CODEOWNERS:** Set `* your@email.com` or your team.
- **docs/how-to/first-push.md:** Set YOUR_HOST / YOUR_ORG / your-repo for your Git host.
- **memory-bank/repo_map.md:** Update entrypoints and modules to match your app (or leave generic).

## 3. Add your project

- Add your code (any language or stack) in the layout you prefer (e.g. `src/`, `app/`, `cmd/`, package root).
- Add **`scripts/verify-steps.sh`** with your format/lint/test/build commands (see `scripts/verify-steps.sh.example`). CI runs this via `./scripts/verify`.
- Update **`memory-bank/commands.yml`** so it matches your verify steps (for agent reference).
- Optional: **extras/** — remove or keep for optional scripts.

## 4. Verify and push

```bash
./scripts/verify
```

Then add your remote and push (see [docs/how-to/first-push.md](docs/how-to/first-push.md)).
