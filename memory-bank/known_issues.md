# Known issues

## Plex / Deployment

- **The old local split-brain Tunerr/Plex fallback is intentionally removed (2026-05-12).** Do not recreate local production jobs that register the same Plex DVR identity as the systemd-owned host. Active supported deployment paths are binary, Docker, systemd/bare-metal, and k3s when k3s is the single owner for its Plex DVR identity.

- **Plex can report a DVR device as `dead` even when enabled channel mappings are healthy.** The watchdog must not recreate a mapped DVR solely because of that flag; recreate only when mappings are missing or badly under-activated.

## Security

- **Credentials:** Secrets must live only in `.env`, environment variables, or host-local service environment files. `.env` is ignored. Never commit `.env` or log secrets.
