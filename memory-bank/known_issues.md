# Known issues

## Plex / Deployment

- **The old cluster Tunerr/Plex deployment path is intentionally removed (2026-05-12).** Do not recreate manifest trees, cluster deploy workflows, service-DNS DVR URIs, or cluster recovery paths. Active deployment paths are binary, Docker, and systemd/bare-metal only.

- **Plex can report a DVR device as `dead` even when enabled channel mappings are healthy.** The watchdog must not recreate a mapped DVR solely because of that flag; recreate only when mappings are missing or badly under-activated.

## Security

- **Credentials:** Secrets must live only in `.env`, environment variables, or host-local service environment files. `.env` is ignored. Never commit `.env` or log secrets.
