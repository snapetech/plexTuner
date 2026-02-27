# Project Name

Brief description of your project.

## Quick start

1. Copy `.env.example` to `.env` and configure
2. Run `./scripts/verify` to check everything is set up
3. Start developing!

## Development

This project uses the agentic repo template with memory-bank workflow.

### Commands

See `memory-bank/commands.yml` for available commands.

Key commands:
- `./scripts/verify` - Run format, lint, test, build
- `./scripts/quick-check.sh` - Quick tests only

### Documentation

- `AGENTS.md` - Agent instructions
- `memory-bank/` - Project state and task tracking
- `docs/` - Project documentation (Diátaxis format)

## Project structure

```
.
├── AGENTS.md              # Agent instructions
├── memory-bank/           # Project state and knowledge
│   ├── repo_map.md        # Navigation guide
│   ├── current_task.md    # What we're working on now
│   ├── known_issues.md    # Bugs and limitations
│   └── ...
├── scripts/               # Utility scripts
│   ├── verify             # CI verification
│   └── verify-steps.sh    # Your project verification (customize me!)
├── docs/                  # Documentation
├── .github/               # GitHub Actions, CODEOWNERS
└── ...                    # Your project code
```

## Template

This repo was created from the agentic repo template.
See `TEMPLATE.md` for template-specific onboarding instructions.

## Security

See `SECURITY.md` for security policies and reporting procedures.

## License

[Your License Here]
