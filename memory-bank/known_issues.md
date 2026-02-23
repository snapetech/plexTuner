# Known issues

<!-- Add bugs, limitations, and design tradeoffs as they are discovered or fixed. -->

## Security

- **Credentials:** Provider username, password, and URLs must live only in `.env` (or environment). `.env` is in `.gitignore`. Never commit `.env` or log/echo provider credentials. Use `.env.example` as a template (no real values).

## DVR / Plex

- **Plex tuner setup:** Two options. (1) Manual: Settings > Live TV & DVR > Set up, enter Base URL and XMLTV URL (printed at startup). (2) Programmatic: Plex stores DVR/XMLTV in `com.plexapp.plugins.library.db` table `media_provider_resources` (identifiers `tv.plex.grabbers.hdhomerun` and `tv.plex.providers.epg.xmltv`). We can update the `uri` column so Plex uses our tuner without the UI wizard: run with `-register-plex=/path/to/Plex Media Server` (stop Plex first, backup the DB). See `internal/plex/dvr.go`.
- **Gateway errors:** Upstream stream failures are logged to stderr (console/systemd journal).
