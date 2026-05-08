package store

import (
	"database/sql"
	"fmt"
)

// Each entry advances the schema by one version.
// Append only — never modify an existing migration.
var migrations = []string{
	// v1 — full initial schema
	`
CREATE TABLE IF NOT EXISTS kv_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Channel groups (e.g. "Sports", "News").
CREATE TABLE IF NOT EXISTS channel_groups (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Channel profiles (subsets of the full lineup; each gets its own HDHR/M3U/EPG link).
CREATE TABLE IF NOT EXISTS channel_profiles (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Master channel list.
CREATE TABLE IF NOT EXISTS channels (
    id             INTEGER PRIMARY KEY,
    name           TEXT NOT NULL,
    channel_number TEXT,
    group_id       INTEGER REFERENCES channel_groups(id) ON DELETE SET NULL,
    stream_profile TEXT,
    logo_id        INTEGER,
    tvg_id         TEXT,
    gracenote_id   TEXT,
    epg_id         INTEGER,
    user_level     TEXT NOT NULL DEFAULT 'all',
    mature         INTEGER NOT NULL DEFAULT 0,
    enabled        INTEGER NOT NULL DEFAULT 1,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_channels_group ON channels(group_id);
CREATE INDEX IF NOT EXISTS idx_channels_sort  ON channels(sort_order);

-- Ordered fallback stream URLs for each channel.
CREATE TABLE IF NOT EXISTS channel_streams (
    id          INTEGER PRIMARY KEY,
    channel_id  INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    m3u_account INTEGER,
    url         TEXT NOT NULL,
    name        TEXT,
    position    INTEGER NOT NULL DEFAULT 0,
    stale       INTEGER NOT NULL DEFAULT 0,
    stats_json  TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_channel_streams_channel ON channel_streams(channel_id, position);

-- Channel → profile membership (many-to-many; absence = not in profile).
CREATE TABLE IF NOT EXISTS channel_profile_membership (
    profile_id INTEGER NOT NULL REFERENCES channel_profiles(id) ON DELETE CASCADE,
    channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    enabled    INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (profile_id, channel_id)
);

-- M3U / Xtream provider accounts.
CREATE TABLE IF NOT EXISTS m3u_accounts (
    id                    INTEGER PRIMARY KEY,
    name                  TEXT NOT NULL,
    account_type          TEXT NOT NULL DEFAULT 'standard', -- 'standard' | 'xtream'
    url                   TEXT,
    upload_path           TEXT,
    expiration_date       TEXT,
    max_streams           INTEGER NOT NULL DEFAULT 0,
    user_agent            TEXT,
    refresh_interval_hrs  INTEGER NOT NULL DEFAULT 24,
    refresh_cron          TEXT,
    stale_retention_days  INTEGER NOT NULL DEFAULT 7,
    vod_scanning          INTEGER NOT NULL DEFAULT 0,
    vod_priority          INTEGER NOT NULL DEFAULT 0,
    is_active             INTEGER NOT NULL DEFAULT 1,
    last_refreshed_at     TEXT,
    created_at            TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Alternate credential sets for a single M3U account.
CREATE TABLE IF NOT EXISTS m3u_account_profiles (
    id          INTEGER PRIMARY KEY,
    account_id  INTEGER NOT NULL REFERENCES m3u_accounts(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    username    TEXT,
    password    TEXT,
    search_pat  TEXT,
    replace_pat TEXT,
    max_streams INTEGER NOT NULL DEFAULT 0
);

-- Regex stream filters per M3U account (evaluated in order).
CREATE TABLE IF NOT EXISTS m3u_filters (
    id          INTEGER PRIMARY KEY,
    account_id  INTEGER NOT NULL REFERENCES m3u_accounts(id) ON DELETE CASCADE,
    field       TEXT NOT NULL DEFAULT 'group', -- 'group' | 'name' | 'url'
    pattern     TEXT NOT NULL,
    exclude     INTEGER NOT NULL DEFAULT 0,
    case_sens   INTEGER NOT NULL DEFAULT 0,
    position    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_m3u_filters_account ON m3u_filters(account_id, position);

-- Group-level settings within an M3U account.
CREATE TABLE IF NOT EXISTS m3u_groups (
    id                       INTEGER PRIMARY KEY,
    account_id               INTEGER NOT NULL REFERENCES m3u_accounts(id) ON DELETE CASCADE,
    name                     TEXT NOT NULL,
    enabled                  INTEGER NOT NULL DEFAULT 1,
    auto_channel_sync        INTEGER NOT NULL DEFAULT 0,
    channel_numbering_mode   TEXT NOT NULL DEFAULT 'fixed', -- 'fixed' | 'provider' | 'next'
    start_channel_number     INTEGER,
    force_dummy_epg          INTEGER NOT NULL DEFAULT 0,
    override_group           TEXT,
    name_find_regex          TEXT,
    name_replace             TEXT,
    name_filter_regex        TEXT,
    profile_ids              TEXT, -- JSON array of channel_profile ids
    sort_order_mode          TEXT NOT NULL DEFAULT 'provider',
    stream_profile           TEXT,
    created_at               TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    UNIQUE(account_id, name)
);

-- EPG sources (XMLTV, Schedules Direct, or dummy-pattern).
CREATE TABLE IF NOT EXISTS epg_accounts (
    id                   INTEGER PRIMARY KEY,
    name                 TEXT NOT NULL,
    source_type          TEXT NOT NULL DEFAULT 'xmltv', -- 'xmltv' | 'sd' | 'dummy'
    url                  TEXT,
    api_key              TEXT,
    refresh_interval_hrs INTEGER NOT NULL DEFAULT 12,
    refresh_cron         TEXT,
    priority             INTEGER NOT NULL DEFAULT 0,
    is_active            INTEGER NOT NULL DEFAULT 1,
    -- dummy-pattern fields (JSON-encoded config)
    dummy_config_json    TEXT,
    last_refreshed_at    TEXT,
    created_at           TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Stream profiles (ffmpeg, proxy, redirect, streamlink, vlc, yt-dlp, custom).
CREATE TABLE IF NOT EXISTS stream_profiles (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL DEFAULT 'ffmpeg',
    config_json TEXT, -- type-specific options
    is_default  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Users (admin / standard / streamer).
CREATE TABLE IF NOT EXISTS users (
    id              INTEGER PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'standard', -- 'admin' | 'standard' | 'streamer'
    xc_password     TEXT,
    hide_mature     INTEGER NOT NULL DEFAULT 0,
    stream_limit    INTEGER NOT NULL DEFAULT 0,
    epg_days_back   INTEGER NOT NULL DEFAULT 0,
    epg_days_fwd    INTEGER NOT NULL DEFAULT 7,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Which channel profiles a standard/streamer user can access (empty = all).
CREATE TABLE IF NOT EXISTS user_profile_access (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id INTEGER NOT NULL REFERENCES channel_profiles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, profile_id)
);

-- Scheduled recordings.
CREATE TABLE IF NOT EXISTS recordings (
    id           INTEGER PRIMARY KEY,
    channel_id   INTEGER REFERENCES channels(id) ON DELETE SET NULL,
    title        TEXT NOT NULL,
    start_at     TEXT NOT NULL,
    end_at       TEXT NOT NULL,
    recurring    INTEGER NOT NULL DEFAULT 0,
    rule_id      INTEGER,
    status       TEXT NOT NULL DEFAULT 'scheduled', -- 'scheduled' | 'recording' | 'done' | 'failed'
    file_path    TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Recurring recording rules.
CREATE TABLE IF NOT EXISTS recording_rules (
    id           INTEGER PRIMARY KEY,
    channel_id   INTEGER REFERENCES channels(id) ON DELETE SET NULL,
    title        TEXT NOT NULL,
    days_json    TEXT NOT NULL DEFAULT '[]', -- JSON array of weekday integers (0=Sun)
    start_time   TEXT NOT NULL,             -- "HH:MM"
    end_time     TEXT NOT NULL,
    start_date   TEXT,
    end_date     TEXT,
    is_active    INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Series recording rules (triggered by TV Guide).
CREATE TABLE IF NOT EXISTS series_rules (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    channel_id  INTEGER REFERENCES channels(id) ON DELETE SET NULL,
    mode        TEXT NOT NULL DEFAULT 'all', -- 'all' | 'new'
    is_active   INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Uploaded channel logos.
CREATE TABLE IF NOT EXISTS logos (
    id          INTEGER PRIMARY KEY,
    filename    TEXT NOT NULL UNIQUE,
    content_type TEXT NOT NULL DEFAULT 'image/png',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Installed plugins.
CREATE TABLE IF NOT EXISTS plugins (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    version     TEXT,
    description TEXT,
    enabled     INTEGER NOT NULL DEFAULT 1,
    path        TEXT NOT NULL, -- path to executable / script under state-dir/plugins/
    manifest    TEXT,          -- JSON from plugin manifest
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Event-driven webhooks and script executions (Connections).
CREATE TABLE IF NOT EXISTS event_hooks (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    event_types TEXT NOT NULL DEFAULT '[]', -- JSON array of event type strings
    kind        TEXT NOT NULL DEFAULT 'webhook', -- 'webhook' | 'script'
    target      TEXT NOT NULL, -- URL or script path
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Rolling system event log (capped externally).
CREATE TABLE IF NOT EXISTS system_events (
    id         INTEGER PRIMARY KEY,
    at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    level      TEXT NOT NULL DEFAULT 'info', -- 'info' | 'warn' | 'error'
    source     TEXT,
    message    TEXT NOT NULL,
    detail     TEXT -- JSON
);

CREATE INDEX IF NOT EXISTS idx_system_events_at ON system_events(at DESC);

-- Per-user dismissible notifications.
CREATE TABLE IF NOT EXISTS notifications (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL DEFAULT 'info',
    title       TEXT NOT NULL,
    body        TEXT,
    dismissed   INTEGER NOT NULL DEFAULT 0,
    expires_at  TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
`,
}

func migrate(db *sql.DB) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("store: read user_version: %w", err)
	}
	for i := current; i < len(migrations); i++ {
		if _, err := db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("store: migration v%d: %w", i+1, err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			return fmt.Errorf("store: set user_version v%d: %w", i+1, err)
		}
	}
	return nil
}
