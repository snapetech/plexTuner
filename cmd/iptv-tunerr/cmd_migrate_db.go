package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/store"
)

func migrateDBCommands() []commandSpec {
	fs := flag.NewFlagSet("migrate-to-db", flag.ExitOnError)
	dbPath := fs.String("db", "", "path to tunerr.db (default: $IPTV_TUNERR_DB_PATH or ./tunerr.db)")
	dryRun := fs.Bool("dry-run", false, "open and migrate the DB but import no data")
	return []commandSpec{{
		Name:    "migrate-to-db",
		Section: "Lab/ops",
		Summary: "Initialise the application SQLite store and import existing config/state",
		FlagSet: fs,
		Run: func(cfg *config.Config, args []string) {
			runMigrateDB(cfg, *dbPath, *dryRun)
		},
	}}
}

func runMigrateDB(cfg *config.Config, dbPath string, dryRun bool) {
	if dbPath == "" {
		dbPath = strings.TrimSpace(os.Getenv("IPTV_TUNERR_DB_PATH"))
	}
	if dbPath == "" {
		dbPath = "tunerr.db"
	}

	log.Printf("migrate-to-db: opening %s", dbPath)
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("migrate-to-db: %v", err)
	}
	defer st.Close()

	log.Printf("migrate-to-db: schema_version=%d", st.SchemaVersion())

	if dryRun {
		log.Printf("migrate-to-db: dry-run — DB initialised, no data imported")
		return
	}

	imported := 0

	// Seed a default stream profile from env-var presets when none exist yet.
	var profileCount int
	if err := st.DB.QueryRow("SELECT COUNT(*) FROM stream_profiles").Scan(&profileCount); err != nil {
		log.Printf("migrate-to-db: check stream_profiles: %v", err)
	}
	if profileCount == 0 {
		ua := strings.TrimSpace(os.Getenv("IPTV_TUNERR_UPSTREAM_USER_AGENT"))
		if ua == "" {
			ua = "lavf"
		}
		_, err := st.DB.Exec(
			`INSERT INTO stream_profiles (name, type, config_json, is_default) VALUES (?, 'ffmpeg', ?, 1)`,
			"Default",
			fmt.Sprintf(`{"user_agent":"%s"}`, ua),
		)
		if err != nil {
			log.Printf("migrate-to-db: insert default stream profile: %v", err)
		} else {
			imported++
			log.Printf("migrate-to-db: created default stream profile (ua=%s)", ua)
		}
	}

	// Seed default admin user from webui credentials when no users exist.
	var userCount int
	if err := st.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		log.Printf("migrate-to-db: check users: %v", err)
	}
	if userCount == 0 && cfg != nil {
		user := strings.TrimSpace(cfg.WebUIUser)
		if user == "" {
			user = "admin"
		}
		_, err := st.DB.Exec(
			`INSERT INTO users (username, password_hash, role) VALUES (?, ?, 'admin')`,
			user, "changeme",
		)
		if err != nil {
			log.Printf("migrate-to-db: insert admin user: %v", err)
		} else {
			imported++
			log.Printf("migrate-to-db: created admin user %q (password_hash=changeme — set a real hash)", user)
		}
	}

	log.Printf("migrate-to-db: done — %d records imported", imported)
}
