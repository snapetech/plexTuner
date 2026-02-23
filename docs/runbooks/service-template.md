---
id: service-template
type: reference
status: stable
tags: [runbooks, ops, service, template]
---

# Service runbook template

Use this skeleton when the repo runs as a service. Fill in or mark N/A.

## Start / stop

- **Start:** (e.g. `./scripts/verify` then `go run ./cmd/...` or `systemctl start foo` or `docker compose up -d`)
- **Stop:** (e.g. `systemctl stop foo`, `docker compose down`, or SIGTERM)
- **Restart:** (e.g. `systemctl restart foo`)

## Config knobs

- **Env / config file:** (e.g. `.env`, `config.yaml`; see `.env.example` or README)
- **Key variables:** (list 3–5 that ops care about: listen address, timeouts, feature flags, etc.)

## Logs and metrics

- **Logs:** (where: stdout/stderr, journald, file path; how to tail: `journalctl -u foo -f`, `tail -f /var/log/foo`)
- **Metrics:** (if any: endpoint, scrape config, or N/A)

## Common failures and fixes

| Symptom | Likely cause | Fix / check |
|---------|--------------|-------------|
| (example) Service won't start | Bad config / missing env | Check `.env`, run with `-help` |
| (example) Timeouts | Upstream down / network | Check connectivity, timeouts in config |
| (add rows as you learn) | | |

See also
--------
- [Runbooks index](index.md)
- [memory-bank/repo_map.md](../../memory-bank/repo_map.md) for code layout

Related ADRs
------------
- *(if applicable)*

Related runbooks
----------------
- *(if applicable)*
