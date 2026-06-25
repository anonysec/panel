// Package sqlite implements the dbstore.Store interface using modernc.org/sqlite (pure Go, no CGO).
// Advisory locks use an internal sync.Mutex since SQLite is inherently single-writer.
// Suitable for development, testing, and lightweight single-user deployments.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"KorisPanel/panel/internal/dbstore"

	_ "modernc.org/sqlite"
)

// Compile-time interface check.
var _ dbstore.Store = (*SQLiteStore)(nil)

// SQLiteStore implements the dbstore.Store interface backed by SQLite.
type SQLiteStore struct {
	db   *sql.DB
	mu   sync.Mutex // advisory lock simulation (single-writer)
	held map[int64]bool
}

// New opens (or creates) a SQLite database at the given DSN path and returns a Store.
// The DSN should be a file path or ":memory:" for in-memory databases.
func New(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma wal: %w", err)
	}
	// Enable foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma fk: %w", err)
	}
	// Busy timeout to reduce SQLITE_BUSY errors under contention.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma busy_timeout: %w", err)
	}

	return &SQLiteStore{
		db:   db,
		held: make(map[int64]bool),
	}, nil
}

// DB returns the underlying *sql.DB.
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection is alive.
func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Migrate reads .sql files from dir in sorted order and executes them.
// Each file is executed within a transaction. A migrations tracking table
// is used to skip already-applied migrations.
func (s *SQLiteStore) Migrate(ctx context.Context, dir string) error {
	// Create migrations tracking table if it doesn't exist.
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("%w: failed to create migrations table: %v", dbstore.ErrMigrationFailed, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("%w: cannot read migrations dir: %v", dbstore.ErrMigrationFailed, err)
	}

	// Filter and sort .sql files.
	var files []fs.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for _, f := range files {
		// Check if already applied.
		var count int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE filename = ?", f.Name()).Scan(&count)
		if err != nil {
			return fmt.Errorf("%w: check migration %s: %v", dbstore.ErrMigrationFailed, f.Name(), err)
		}
		if count > 0 {
			continue
		}

		// Read and execute the migration.
		content, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return fmt.Errorf("%w: read migration %s: %v", dbstore.ErrMigrationFailed, f.Name(), err)
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("%w: begin tx for %s: %v", dbstore.ErrMigrationFailed, f.Name(), err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%w: execute migration %s: %v", dbstore.ErrMigrationFailed, f.Name(), err)
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename) VALUES (?)", f.Name()); err != nil {
			tx.Rollback()
			return fmt.Errorf("%w: record migration %s: %v", dbstore.ErrMigrationFailed, f.Name(), err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("%w: commit migration %s: %v", dbstore.ErrMigrationFailed, f.Name(), err)
		}
	}

	return nil
}

// Begin starts a new database transaction.
func (s *SQLiteStore) Begin(ctx context.Context) (dbstore.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqliteTx{tx: tx}, nil
}

// AcquireLock simulates an advisory lock using an internal mutex.
// Returns true if the lock was acquired, false if already held.
func (s *SQLiteStore) AcquireLock(_ context.Context, lockID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.held[lockID] {
		return false, nil
	}
	s.held[lockID] = true
	return true, nil
}

// ReleaseLock releases a previously acquired advisory lock.
func (s *SQLiteStore) ReleaseLock(_ context.Context, lockID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.held[lockID] {
		return dbstore.ErrLockNotAcquired
	}
	delete(s.held, lockID)
	return nil
}

// GetSession retrieves a session by token.
func (s *SQLiteStore) GetSession(ctx context.Context, token string) (*dbstore.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT token, admin_id, customer_id, data, ip_address, user_agent, created_at, expires_at, last_seen
		FROM panel_sessions WHERE token = ?
	`, token)

	sess := &dbstore.Session{}
	var createdAt, expiresAt, lastSeen string
	err := row.Scan(
		&sess.Token,
		&sess.AdminID,
		&sess.CustomerID,
		&sess.Data,
		&sess.IPAddress,
		&sess.UserAgent,
		&createdAt,
		&expiresAt,
		&lastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, dbstore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Parse timestamps.
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	sess.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	sess.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)

	// Check expiry.
	if time.Now().After(sess.ExpiresAt) {
		return nil, dbstore.ErrSessionExpired
	}

	return sess, nil
}

// SaveSession inserts or replaces a session in the database.
func (s *SQLiteStore) SaveSession(ctx context.Context, sess *dbstore.Session) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO panel_sessions (token, admin_id, customer_id, data, ip_address, user_agent, created_at, expires_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		sess.Token,
		sess.AdminID,
		sess.CustomerID,
		sess.Data,
		sess.IPAddress,
		sess.UserAgent,
		sess.CreatedAt.Format(time.RFC3339),
		sess.ExpiresAt.Format(time.RFC3339),
		sess.LastSeen.Format(time.RFC3339),
	)
	return err
}

// DeleteSession removes a session by token.
func (s *SQLiteStore) DeleteSession(ctx context.Context, token string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM panel_sessions WHERE token = ?", token)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return dbstore.ErrNotFound
	}
	return nil
}

// CleanExpiredSessions removes all sessions past their expiry time.
func (s *SQLiteStore) CleanExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM panel_sessions WHERE expires_at < ?", time.Now().Format(time.RFC3339))
	return err
}

// InsertMetrics inserts a metrics row for a node.
func (s *SQLiteStore) InsertMetrics(ctx context.Context, nodeID int64, m *dbstore.MetricsRow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO node_metrics_history (time, node_id, cpu_percent, ram_percent, disk_percent, rx_bps, tx_bps, active_sessions, uptime_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		m.Time.Format(time.RFC3339),
		nodeID,
		m.CPUPercent,
		m.RAMPercent,
		m.DiskPercent,
		m.RxBPS,
		m.TxBPS,
		m.ActiveSessions,
		m.UptimeSeconds,
	)
	return err
}

// InsertTrafficLog inserts a traffic accounting entry.
func (s *SQLiteStore) InsertTrafficLog(ctx context.Context, entry *dbstore.TrafficLogEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_traffic_log (time, user_id, node_id, rx_bytes, tx_bytes)
		VALUES (?, ?, ?, ?, ?)
	`,
		entry.Time.Format(time.RFC3339),
		entry.UserID,
		entry.NodeID,
		entry.RxBytes,
		entry.TxBytes,
	)
	return err
}

// QueryMetrics returns metrics rows for a node within a time range.
func (s *SQLiteStore) QueryMetrics(ctx context.Context, nodeID int64, from, to time.Time) ([]dbstore.MetricsRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT time, cpu_percent, ram_percent, disk_percent, rx_bps, tx_bps, active_sessions, uptime_seconds
		FROM node_metrics_history
		WHERE node_id = ? AND time >= ? AND time <= ?
		ORDER BY time ASC
	`, nodeID, from.Format(time.RFC3339), to.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []dbstore.MetricsRow
	for rows.Next() {
		var m dbstore.MetricsRow
		var ts string
		if err := rows.Scan(&ts, &m.CPUPercent, &m.RAMPercent, &m.DiskPercent, &m.RxBPS, &m.TxBPS, &m.ActiveSessions, &m.UptimeSeconds); err != nil {
			return nil, err
		}
		m.Time, _ = time.Parse(time.RFC3339, ts)
		results = append(results, m)
	}
	return results, rows.Err()
}

// sqliteTx wraps *sql.Tx to satisfy the dbstore.Tx interface.
type sqliteTx struct {
	tx *sql.Tx
}

func (t *sqliteTx) Commit() error {
	return t.tx.Commit()
}

func (t *sqliteTx) Rollback() error {
	return t.tx.Rollback()
}

func (t *sqliteTx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

func (t *sqliteTx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}
