// Package postgres implements the dbstore.Store interface using PostgreSQL/TimescaleDB
// via the pgx/v5 driver. It provides advisory lock support, hypertable-optimized
// time-series inserts, and database-backed session management.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"KorisPanel/panel/internal/dbstore"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Compile-time assertion that Store implements dbstore.Store.
var _ dbstore.Store = (*Store)(nil)

// Store implements dbstore.Store for PostgreSQL/TimescaleDB.
type Store struct {
	pool   *pgxpool.Pool
	sqlDB  *sql.DB
	config *pgxpool.Config
}

// New creates a new PostgreSQL store from a DSN string.
// DSN format: postgres://user:password@host:port/dbname?sslmode=disable
func New(ctx context.Context, dsn string) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}

	// Connection pool settings aligned with project conventions
	config.MaxConns = 25
	config.MinConns = 2
	config.MaxConnLifetime = 5 * time.Minute
	config.MaxConnIdleTime = 2 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	// Create a *sql.DB via pgx stdlib for compatibility with code expecting database/sql
	sqlDB := stdlib.OpenDBFromPool(pool)

	return &Store{
		pool:   pool,
		sqlDB:  sqlDB,
		config: config,
	}, nil
}

// DB returns the underlying *sql.DB for compatibility with existing code.
func (s *Store) DB() *sql.DB {
	return s.sqlDB
}

// Close shuts down the connection pool and closes the sql.DB.
func (s *Store) Close() error {
	s.sqlDB.Close()
	s.pool.Close()
	return nil
}

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return dbstore.ErrConnectionLost
	}
	return nil
}

// Migrate reads .sql files from the given directory and executes them in order.
// Files are sorted lexicographically (e.g., 064_xxx.sql, 065_yyy.sql).
// It tracks applied migrations in a `schema_migrations` table.
func (s *Store) Migrate(ctx context.Context, dir string) error {
	// Ensure the migrations tracking table exists
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("%w: create schema_migrations: %v", dbstore.ErrMigrationFailed, err)
	}

	// Read migration files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("%w: read migrations dir: %v", dbstore.ErrMigrationFailed, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, filename := range files {
		// Check if already applied
		var exists bool
		err := s.pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)",
			filename,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("%w: check migration %s: %v", dbstore.ErrMigrationFailed, filename, err)
		}
		if exists {
			continue
		}

		// Read and execute the migration
		content, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			return fmt.Errorf("%w: read %s: %v", dbstore.ErrMigrationFailed, filename, err)
		}

		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("%w: begin tx for %s: %v", dbstore.ErrMigrationFailed, filename, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("%w: execute %s: %v", dbstore.ErrMigrationFailed, filename, err)
		}

		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (filename) VALUES ($1)", filename,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("%w: record %s: %v", dbstore.ErrMigrationFailed, filename, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("%w: commit %s: %v", dbstore.ErrMigrationFailed, filename, err)
		}
	}

	return nil
}

// Begin starts a new database transaction.
func (s *Store) Begin(ctx context.Context) (dbstore.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin tx: %w", err)
	}
	return &pgTx{tx: tx}, nil
}

// AcquireLock attempts to acquire a PostgreSQL advisory lock.
// Returns true if the lock was acquired, false if another session holds it.
// Uses pg_try_advisory_lock which is session-level and non-blocking.
func (s *Store) AcquireLock(ctx context.Context, lockID int64) (bool, error) {
	var acquired bool
	err := s.pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired)
	if err != nil {
		return false, fmt.Errorf("postgres: acquire lock %d: %w", lockID, err)
	}
	return acquired, nil
}

// ReleaseLock releases a previously acquired advisory lock.
func (s *Store) ReleaseLock(ctx context.Context, lockID int64) error {
	var released bool
	err := s.pool.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", lockID).Scan(&released)
	if err != nil {
		return fmt.Errorf("postgres: release lock %d: %w", lockID, err)
	}
	if !released {
		return dbstore.ErrLockNotAcquired
	}
	return nil
}

// InsertMetrics inserts a metrics row into the node_metrics_history hypertable.
func (s *Store) InsertMetrics(ctx context.Context, nodeID int64, m *dbstore.MetricsRow) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO node_metrics_history (time, node_id, cpu_percent, ram_percent, disk_percent, rx_bps, tx_bps, active_sessions, uptime_seconds)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, m.Time, nodeID, m.CPUPercent, m.RAMPercent, m.DiskPercent, m.RxBPS, m.TxBPS, m.ActiveSessions, m.UptimeSeconds)
	if err != nil {
		return wrapPgError(err)
	}
	return nil
}

// InsertTrafficLog inserts a traffic log entry into the user_traffic_log hypertable.
func (s *Store) InsertTrafficLog(ctx context.Context, entry *dbstore.TrafficLogEntry) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_traffic_log (time, user_id, node_id, rx_bytes, tx_bytes)
		VALUES ($1, $2, $3, $4, $5)
	`, entry.Time, entry.UserID, entry.NodeID, entry.RxBytes, entry.TxBytes)
	if err != nil {
		return wrapPgError(err)
	}
	return nil
}

// QueryMetrics retrieves metrics for a node within a time range, ordered by time ascending.
func (s *Store) QueryMetrics(ctx context.Context, nodeID int64, from, to time.Time) ([]dbstore.MetricsRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT time, cpu_percent, ram_percent, disk_percent, rx_bps, tx_bps, active_sessions, uptime_seconds
		FROM node_metrics_history
		WHERE node_id = $1 AND time >= $2 AND time <= $3
		ORDER BY time ASC
	`, nodeID, from, to)
	if err != nil {
		return nil, wrapPgError(err)
	}
	defer rows.Close()

	var results []dbstore.MetricsRow
	for rows.Next() {
		var m dbstore.MetricsRow
		if err := rows.Scan(&m.Time, &m.CPUPercent, &m.RAMPercent, &m.DiskPercent, &m.RxBPS, &m.TxBPS, &m.ActiveSessions, &m.UptimeSeconds); err != nil {
			return nil, fmt.Errorf("postgres: scan metrics row: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate metrics rows: %w", err)
	}
	return results, nil
}

// GetSession retrieves a session by token.
// Returns ErrNotFound if the session does not exist, ErrSessionExpired if it has expired.
func (s *Store) GetSession(ctx context.Context, token string) (*dbstore.Session, error) {
	var sess dbstore.Session
	err := s.pool.QueryRow(ctx, `
		SELECT token, admin_id, customer_id, data, ip_address, user_agent, created_at, expires_at, last_seen
		FROM panel_sessions
		WHERE token = $1
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
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, dbstore.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get session: %w", err)
	}

	if time.Now().After(sess.ExpiresAt) {
		return nil, dbstore.ErrSessionExpired
	}

	// Update last_seen on access
	_, _ = s.pool.Exec(ctx, `
		UPDATE panel_sessions SET last_seen = $1 WHERE token = $2
	`, time.Now().UTC(), token)

	return &sess, nil
}

// SaveSession inserts or updates a session (upsert on token).
func (s *Store) SaveSession(ctx context.Context, sess *dbstore.Session) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO panel_sessions (token, admin_id, customer_id, data, ip_address, user_agent, created_at, expires_at, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (token) DO UPDATE SET
			admin_id = EXCLUDED.admin_id,
			customer_id = EXCLUDED.customer_id,
			data = EXCLUDED.data,
			ip_address = EXCLUDED.ip_address,
			user_agent = EXCLUDED.user_agent,
			expires_at = EXCLUDED.expires_at,
			last_seen = EXCLUDED.last_seen
	`, sess.Token, sess.AdminID, sess.CustomerID, sess.Data, sess.IPAddress, sess.UserAgent, sess.CreatedAt, sess.ExpiresAt, sess.LastSeen)
	if err != nil {
		return wrapPgError(err)
	}
	return nil
}

// DeleteSession removes a session by token.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM panel_sessions WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("postgres: delete session: %w", err)
	}
	if result.RowsAffected() == 0 {
		return dbstore.ErrNotFound
	}
	return nil
}

// CleanExpiredSessions removes all sessions past their expiry time.
func (s *Store) CleanExpiredSessions(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM panel_sessions WHERE expires_at < $1`, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("postgres: clean expired sessions: %w", err)
	}
	return nil
}

// pgTx wraps a pgx transaction to satisfy the dbstore.Tx interface.
type pgTx struct {
	tx pgx.Tx
}

func (t *pgTx) Commit() error {
	return t.tx.Commit(context.Background())
}

func (t *pgTx) Rollback() error {
	return t.tx.Rollback(context.Background())
}

func (t *pgTx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	tag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, wrapPgError(err)
	}
	return pgResult{tag: tag}, nil
}

func (t *pgTx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	// For QueryRow, we use the underlying *sql.DB approach since pgx.Tx.QueryRow
	// returns pgx.Row not *sql.Row. We work around this by using the stdlib wrapper.
	// This is a limitation; callers needing QueryRow in a tx should use the pgx row directly.
	// For now, return nil — actual usage should prefer Exec or direct pgx operations.
	return nil
}

// pgResult wraps pgconn.CommandTag to satisfy sql.Result.
type pgResult struct {
	tag pgconn.CommandTag
}

func (r pgResult) LastInsertId() (int64, error) {
	return 0, errors.New("postgres: LastInsertId not supported, use RETURNING")
}

func (r pgResult) RowsAffected() (int64, error) {
	return r.tag.RowsAffected(), nil
}

// wrapPgError translates PostgreSQL-specific errors into dbstore sentinel errors.
func wrapPgError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", dbstore.ErrDuplicateNode, pgErr.Message)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: %s", dbstore.ErrInvalidReference, pgErr.Message)
		case "23514": // check_violation
			return fmt.Errorf("%w: %s", dbstore.ErrConstraintViolation, pgErr.Message)
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return dbstore.ErrNotFound
	}

	return err
}
