# Security

## Overview

plex-tuner is a local TV tuner and VOD catalog for Plex. It talks to your IPTV provider (player_api / get.php), builds a catalog, and serves an HDHomeRun-compatible API and stream gateway. This document describes threat model, mitigations, and hardening.

## Threat model

- **Misuse (accidental)**: Passwords with `&`, `=`, or `#` breaking URLs; paths with `..`; invalid base URL written to Plex DB.
- **Misuse (purposeful)**: Attacker controls provider response (malicious catalog with `file://` or internal URLs); attacker has shell/env access and sets malicious `.env` or flags; network access to tuner HTTP server.
- **Supply chain**: Default provider host list sends credentials to third-party domains when only user/pass are set in env.

## Mitigations in code

- **URL query/path injection**: All provider URLs that include username/password use `url.QueryEscape` / `url.PathEscape`. Prevents parameter injection and broken auth when credentials contain special characters.
- **SSRF**: Stream URLs (from catalog or provider API) are only fetched if the scheme is `http` or `https`. `file://`, `ftp://`, and other schemes are rejected in the gateway and in the materializer (download/probe). Reduces risk of hitting internal or local resources if the provider returns malicious URLs.
- **Plex base URL**: `-register-plex` requires a valid `http` or `https` base URL before writing to the Plex DB.
- **Env file path**: `LoadEnvFile` uses `filepath.Clean` on the path to reduce traversal risk if the path is ever user-influenced.
- **Cache paths**: Asset IDs used for cache file paths are sanitized (no `/`, `\`, NUL) to avoid path traversal under the cache dir.
- **Logging**: Gateway logs no longer include full upstream URLs (which may contain tokens); they log channel name and status only.

## What the app does *not* do

- **No authentication** on the tuner HTTP server. The HDHomeRun/XMLTV endpoints and `/stream/<n>` are unauthenticated. Anyone who can reach the listen address can list channels and stream. **You should bind to a private address and/or use a firewall** so only Plex and trusted clients can access the tuner.
- **No encryption** of catalog or credentials at rest. Keep `.env` out of git and restrict file permissions (e.g. `chmod 600 .env`).
- **No check** that provider URLs or default host list are “trusted”. If you set only `PLEX_TUNER_PROVIDER_USER` and `PLEX_TUNER_PROVIDER_PASS`, the app will try the hardcoded default provider hosts. Use `PLEX_TUNER_PROVIDER_URL` or `PLEX_TUNER_PROVIDER_URLS` to point only at providers you trust.

## Hardening checklist

- Run as an unprivileged user (no root).
- Bind the tuner to a private IP or `127.0.0.1` if only local Plex needs it (e.g. `-addr=127.0.0.1:5004`).
- Restrict filesystem permissions: `chmod 600 .env`, restrict catalog and cache dir to the process user.
- Keep `.env` (and any env files) out of version control and backups that are shared.
- Prefer explicit `PLEX_TUNER_PROVIDER_URL(S)` over relying on the default provider host list.
- Stop Plex and back up its database before using `-register-plex`.

## Reporting issues

If you find a security bug, please report it privately (e.g. to the maintainer or via a private channel) rather than in a public issue.
