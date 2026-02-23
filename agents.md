# agents.md (tool-compat alias)

Some tools look for `agents.md`, others for `AGENTS.md`. **Canonical instructions:** [AGENTS.md](AGENTS.md).

Use the **memory-bank** and workflow described there. Commands (authoritative) are in **`memory-bank/commands.yml`**; CI runs **`scripts/verify`**, which runs format → lint → test → build from that file.

Quick reference — verification commands (do not guess; see `memory-bank/commands.yml`):

| Step   | Command |
|--------|---------|
| Format | `go fmt ./...` |
| Lint   | `go vet ./...` |
| Test   | `go test ./...` |
| Build  | `go build -o /dev/null ./cmd/plex-tuner` |

Navigation: **`memory-bank/repo_map.md`** — entrypoints, modules, no-go zones.
