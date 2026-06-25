package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(dsn string) (*sql.DB, error) {
	// Detect PostgreSQL/TimescaleDB DSN (starts with postgres:// or postgresql://)
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return openPostgres(dsn)
	}
	return openMySQL(dsn)
}

func openPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return db, nil
}

func openMySQL(dsn string) (*sql.DB, error) {
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

	// Auto-tune pool based on system RAM, then apply env var overrides.
	// Priority: env vars > AutoTunePool > defaults.
	cfg := AutoTunePool(db)
	ApplyEnvOverrides(db, &cfg)

	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// ApplyEnvOverrides checks for environment variable overrides and applies
// them to the database connection pool, taking priority over auto-tuned values.
//
// Supported env vars:
//   - PANEL_DB_MAX_OPEN: integer, overrides MaxOpenConns
//   - PANEL_DB_MAX_IDLE: integer, overrides MaxIdleConns
//   - PANEL_DB_MAX_LIFETIME: duration string (e.g. "5m", "300s"), overrides ConnMaxLifetime
func ApplyEnvOverrides(db *sql.DB, cfg *PoolConfig) {
	if v := os.Getenv("PANEL_DB_MAX_OPEN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			db.SetMaxOpenConns(n)
			cfg.MaxOpen = n
			log.Printf("[db] env override: MaxOpenConns=%d", n)
		} else if err != nil {
			log.Printf("[db] invalid PANEL_DB_MAX_OPEN=%q: %v", v, err)
		}
	}

	if v := os.Getenv("PANEL_DB_MAX_IDLE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			db.SetMaxIdleConns(n)
			cfg.MaxIdle = n
			log.Printf("[db] env override: MaxIdleConns=%d", n)
		} else if err != nil {
			log.Printf("[db] invalid PANEL_DB_MAX_IDLE=%q: %v", v, err)
		}
	}

	if v := os.Getenv("PANEL_DB_MAX_LIFETIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			db.SetConnMaxLifetime(d)
			cfg.MaxLifetime = d
			log.Printf("[db] env override: ConnMaxLifetime=%s", d)
		} else if err != nil {
			log.Printf("[db] invalid PANEL_DB_MAX_LIFETIME=%q: %v", v, err)
		}
	}
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
