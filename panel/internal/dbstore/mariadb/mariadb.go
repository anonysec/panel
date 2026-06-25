// Package mariadb implements the dbstore.Store interface using MariaDB/MySQL
// via the go-sql-driver/mysql driver and standard database/sql.
package mariadb

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"KorisPanel/panel/internal/dbstore"
)

// Compile-time check that Store satisfies the dbstore.Store interface.
var _ dbstore.Store = (*Store)(nil)

// Store implements dbstore.Store for MariaDB/MySQL.
type Store struct {
	db *sql.DB
}

// New opens a MariaDB connection using the provided DSN and returns a Store.
// The DSN format follows go-sql-driver/mysql conventions:
// user:password@tcp(host:port)/dbname?parseTime=true
func New(dsn string) (*Store, error) {
	// Ensure parseTime is enabled for proper time.Time scanning.
	if !strings.Contains(dsn, "parseTime=") {
		sep := "&"
		if !strings.Contains(dsn, "?") {
			sep = "?"
		}
		dsn += sep + "parseTime=true"
	}

	// Append connection timeout params if not already specified.
	if !strings.Contains(dsn, "timeout=") {
		dsn += "&timeout=10s&readTimeout=30s&writeTimeout=30s"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mariadb open: %w", err)
	}

	// Apply connection pool defaults (can be tuned externally).
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("mariadb ping: %w", err)
	}

	return &Store{db: db}, nil
}

// NewFromDB wraps an existing *sql.DB connection as a mariadb Store.
// This is useful when the connection was already established elsewhere (e.g., via db.Open).
// The caller retains ownership of the *sql.DB lifecycle (closing it).
func NewFromDB(db *sql.DB) *Store {
	return &Store{db: db}
}

// DB returns the underlying *sql.DB handle.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return dbstore.ErrConnectionLost
	}
	return nil
}

// Migrate runs all .sql migration files from dir in sorted order.
// It tracks applied migrations in a schema_migrations table.
func (s *Store) Migrate(ctx context.Context, dir string) error {
	if dir == "" {
		dir = "panel/migrations"
	}

	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(80) PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("%w: create schema_migrations: %v", dbstore.ErrMigrationFailed, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("%w: read dir %s: %v", dbstore.ErrMigrationFailed, dir, err)
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
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, name).Scan(&exists); err != nil {
			return fmt.Errorf("%w: check migration %s: %v", dbstore.ErrMigrationFailed, name, err)
		}
		if exists > 0 {
			continue
		}

		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("%w: read %s: %v", dbstore.ErrMigrationFailed, name, err)
		}

		if _, err := s.db.ExecContext(ctx, string(b)); err != nil {
			return fmt.Errorf("%w: apply %s: %v", dbstore.ErrMigrationFailed, name, err)
		}

		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES(?)`, name); err != nil {
			return fmt.Errorf("%w: record %s: %v", dbstore.ErrMigrationFailed, name, err)
		}

		log.Printf("[database] applied migration: %s", name)
	}

	return nil
}

// Begin starts a new database transaction.
func (s *Store) Begin(ctx context.Context) (dbstore.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &mariaTx{tx: tx}, nil
}

// AcquireLock is a no-op on MariaDB. It always returns (true, nil) since
// MariaDB doesn't support cooperative advisory locks the same way PostgreSQL does.
// Multi-worker coordination should use alternative mechanisms on MariaDB.
func (s *Store) AcquireLock(ctx context.Context, lockID int64) (bool, error) {
	return true, nil
}

// ReleaseLock is a no-op on MariaDB.
func (s *Store) ReleaseLock(ctx context.Context, lockID int64) error {
	return nil
}

// --- Sessions ---

// GetSession retrieves a session by token. Returns ErrNotFound if the token
// doesn't exist, or ErrSessionExpired if the session has passed its expiry time.
func (s *Store) GetSession(ctx context.Context, token string) (*dbstore.Session, error) {
	sess := &dbstore.Session{}
	err := s.db.QueryRowContext(ctx, `
		SELECT token, admin_id, customer_id, data, ip_address, user_agent,
		       created_at, expires_at, last_seen
		FROM panel_sessions
		WHERE token = ?
	`, token).Scan(
		&sess.Token,
		&sess.AdminID,
		&sess.CustomerID,
		&sess.Data,
		&sess.IPAddress,
		&sess.UserAgent,
		&sess.CreatedAt,
		&sess.ExpiresAt,
		&sess.LastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, dbstore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if time.Now().UTC().After(sess.ExpiresAt) {
		return nil, dbstore.ErrSessionExpired
	}

	// Update last_seen on access.
	_, _ = s.db.ExecContext(ctx, `UPDATE panel_sessions SET last_seen = ? WHERE token = ?`,
		time.Now().UTC(), token)

	return sess, nil
}

// SaveSession creates or updates a session in the database.
func (s *Store) SaveSession(ctx context.Context, sess *dbstore.Session) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO panel_sessions (token, admin_id, customer_id, data, ip_address, user_agent, created_at, expires_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			admin_id = VALUES(admin_id),
			customer_id = VALUES(customer_id),
			data = VALUES(data),
			ip_address = VALUES(ip_address),
			user_agent = VALUES(user_agent),
			expires_at = VALUES(expires_at),
			last_seen = VALUES(last_seen)
	`,
		sess.Token,
		sess.AdminID,
		sess.CustomerID,
		sess.Data,
		sess.IPAddress,
		sess.UserAgent,
		sess.CreatedAt,
		sess.ExpiresAt,
		sess.LastSeen,
	)
	return err
}

// DeleteSession removes a session by token.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM panel_sessions WHERE token = ?`, token)
	return err
}

// CleanExpiredSessions removes all sessions past their expiry time.
func (s *Store) CleanExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM panel_sessions WHERE expires_at < ?`, time.Now().UTC())
	return err
}

// --- Time-series data ---

// InsertMetrics writes a metrics row to the node_metrics_history table.
func (s *Store) InsertMetrics(ctx context.Context, nodeID int64, m *dbstore.MetricsRow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO node_metrics_history (time, node_id, cpu_percent, ram_percent, disk_percent, rx_bps, tx_bps, active_sessions, uptime_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		m.Time.UTC(),
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

// InsertTrafficLog writes a per-user traffic accounting entry.
func (s *Store) InsertTrafficLog(ctx context.Context, entry *dbstore.TrafficLogEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_traffic_log (time, user_id, node_id, rx_bytes, tx_bytes)
		VALUES (?, ?, ?, ?, ?)
	`,
		entry.Time.UTC(),
		entry.UserID,
		entry.NodeID,
		entry.RxBytes,
		entry.TxBytes,
	)
	return err
}

// QueryMetrics retrieves metrics rows for a node within the given time range.
func (s *Store) QueryMetrics(ctx context.Context, nodeID int64, from, to time.Time) ([]dbstore.MetricsRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT time, cpu_percent, ram_percent, disk_percent, rx_bps, tx_bps, active_sessions, uptime_seconds
		FROM node_metrics_history
		WHERE node_id = ? AND time >= ? AND time <= ?
		ORDER BY time ASC
	`, nodeID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []dbstore.MetricsRow
	for rows.Next() {
		var m dbstore.MetricsRow
		if err := rows.Scan(
			&m.Time,
			&m.CPUPercent,
			&m.RAMPercent,
			&m.DiskPercent,
			&m.RxBPS,
			&m.TxBPS,
			&m.ActiveSessions,
			&m.UptimeSeconds,
		); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// --- Transaction wrapper ---

// mariaTx wraps a *sql.Tx to satisfy the dbstore.Tx interface.
type mariaTx struct {
	tx *sql.Tx
}

func (t *mariaTx) Commit() error {
	return t.tx.Commit()
}

func (t *mariaTx) Rollback() error {
	return t.tx.Rollback()
}

func (t *mariaTx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

func (t *mariaTx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}
