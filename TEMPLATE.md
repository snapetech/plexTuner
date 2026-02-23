# Template onboarding

After **"Use this template"** or cloning this repo as a starting point, do the following so the repo is yours instead of a generic template.

## 1. Run the init script

```bash
./scripts/init-template --module github.com/YOUR_ORG/YOUR_REPO --project "Your Project Name" --binary your-binary --owner-email you@example.com
```

Replace placeholders with your Go module path, project name, binary name (e.g. the main under `cmd/`), and owner email. The script updates `go.mod`, README, CODEOWNERS, and doc placeholders.

## 2. Replace placeholders by hand (if needed)

- **.github/CODEOWNERS:** Set `* your@email.com` or your team.
- **docs/how-to/first-push.md:** Set YOUR_GITLAB_HOST / YOUR_GROUP / your-project (or equivalent for GitHub).
- **memory-bank/repo_map.md:** Update entrypoints and modules to match your app (or leave generic).

## 3. Decide what to keep

- **cmd/hello:** Default minimal app. Replace with your own `cmd/your-binary` or keep as a smoke test.
- **extras/:** Optional scripts; remove or keep.

## 4. Verify

```bash
./scripts/verify
```

Then add your remote and push (see [docs/how-to/first-push.md](docs/how-to/first-push.md)).
