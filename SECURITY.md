# Security

## Template defaults

- **Secrets:** Keep secrets in `.env` (gitignored). Never commit `.env` or log secrets. Use `.env.example` as a template with no real values.
- **Supply chain:** Dependabot and CodeQL workflows are enabled; review and update dependencies.

## For your project

Add your threat model, mitigations, and hardening checklist. Run as unprivileged user where possible; restrict bind addresses and file permissions as needed.

## Reporting issues

Report security bugs privately (e.g. to the maintainer) rather than in a public issue.
