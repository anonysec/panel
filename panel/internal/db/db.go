package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func Open(dsn string) (*sql.DB, error) {
	// Append connection timeout params if not already specified in the DSN.
	// This prevents "Aborted connection ... Got an error reading communication packets"
	// by ensuring client-side timeouts are shorter than MariaDB's wait_timeout.
	if !strings.Contains(dsn, "timeout=") {
		sep := "&"
		if !strings.Contains(dsn, "?") {
			sep = "?"
		}
		dsn += sep + "timeout=10s&readTimeout=30s&writeTimeout=30s"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// Connection pool tuned for 1-core/1GB RAM servers.
	// ConnMaxLifetime must be less than MariaDB's wait_timeout (default: 28800s)
	// to prevent "Aborted connection ... Got an error reading communication packets".
	// Using conservative values ensures connections are recycled before server-side timeout.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetConnMaxIdleTime(90 * time.Second)

	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func Migrate(db *sql.DB, dir string) error {
	if dir == "" {
		dir = "panel/migrations"
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(80) PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=?`, name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(b)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, name); err != nil {
			return err
		}
	}
	return nil
}
