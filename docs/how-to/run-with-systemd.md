---
id: run-with-systemd
type: how-to
status: stable
tags: [how-to, systemd, live-tv, dvr, ops]
---

# Run plex-tuner with systemd (one-run Live TV/DVR)

Goal: run plex-tuner as a systemd service so Live TV/DVR works with zero interaction after credentials.

Preconditions
-------------
- Binary built and installed (e.g. to `/opt/plextuner/plex-tuner`).
- `.env` with provider credentials and **`PLEX_TUNER_BASE_URL`** set to the URL Plex will use (e.g. `http://YOUR_SERVER_IP:5004`).

Steps
-----

1. Copy the example unit file:
   ```bash
   sudo cp docs/systemd/plextuner.service.example /etc/systemd/system/plextuner.service
   ```
2. Edit if needed: `WorkingDirectory`, `EnvironmentFile`, `ExecStart` path or flags (e.g. `-addr :5004`, `-refresh 6h`).
3. Reload and enable:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now plextuner
   ```

Verify
------
- `systemctl status plextuner` shows active (running).
- Journal: `journalctl -u plextuner -f` shows startup and printed Base URL + XMLTV URL.
- In Plex: Settings → Live TV & DVR → Set up; enter Base URL and XMLTV URL once.

Rollback
--------
- `sudo systemctl stop plextuner && sudo systemctl disable plextuner`

Troubleshooting
---------------
- **Fail at startup:** Check `.env` and `PLEX_TUNER_BASE_URL`; errors are logged with `[ERROR]`.
- **Plex can't reach tuner:** Ensure `PLEX_TUNER_BASE_URL` matches the address Plex uses (server IP or hostname).

See also
--------
- [systemd example unit](../systemd/plextuner.service.example)
- [Explanations: architecture](../explanations/architecture.md)
- [Reference: implementation stories](../reference/implementation-stories.md)

Related ADRs
------------
- *(none)*

Related runbooks
----------------
- *(none)*
