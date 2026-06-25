// Package dbstore defines the database abstraction layer interface.
// All implementations (PostgreSQL/TimescaleDB, MariaDB, SQLite) must satisfy the Store contract.
package dbstore

import (
	"context"
	"database/sql"
	"time"
)

// Store is the database abstraction interface.
// All implementations (PostgreSQL, MariaDB, SQLite) must satisfy this contract.
type Store interface {
	// Connection
	DB() *sql.DB
	Close() error
	Ping(ctx context.Context) error
	Migrate(ctx context.Context, dir string) error

	// Transactions
	Begin(ctx context.Context) (Tx, error)

	// Advisory locks (no-op on MariaDB/SQLite)
	AcquireLock(ctx context.Context, lockID int64) (bool, error)
	ReleaseLock(ctx context.Context, lockID int64) error

	// Sessions
	GetSession(ctx context.Context, token string) (*Session, error)
	SaveSession(ctx context.Context, s *Session) error
	DeleteSession(ctx context.Context, token string) error
	CleanExpiredSessions(ctx context.Context) error

	// Time-series (hypertable-optimized on TimescaleDB, regular tables elsewhere)
	InsertMetrics(ctx context.Context, nodeID int64, m *MetricsRow) error
	InsertTrafficLog(ctx context.Context, entry *TrafficLogEntry) error
	QueryMetrics(ctx context.Context, nodeID int64, from, to time.Time) ([]MetricsRow, error)
}

// Tx represents a database transaction.
type Tx interface {
	Commit() error
	Rollback() error
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
}

// Session represents a database-backed HTTP session.
type Session struct {
	Token      string        `json:"token"`
	AdminID    sql.NullInt64 `json:"admin_id,omitempty"`
	CustomerID sql.NullInt64 `json:"customer_id,omitempty"`
	Data       []byte        `json:"data,omitempty"`
	IPAddress  string        `json:"ip_address,omitempty"`
	UserAgent  string        `json:"user_agent,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	ExpiresAt  time.Time     `json:"expires_at"`
	LastSeen   time.Time     `json:"last_seen"`
}

// MetricsRow represents a single time-series metrics entry for a node.
type MetricsRow struct {
	Time           time.Time `json:"time"`
	CPUPercent     float64   `json:"cpu_percent"`
	RAMPercent     float64   `json:"ram_percent"`
	DiskPercent    float64   `json:"disk_percent"`
	RxBPS          int64     `json:"rx_bps"`
	TxBPS          int64     `json:"tx_bps"`
	ActiveSessions int       `json:"active_sessions"`
	UptimeSeconds  int64     `json:"uptime_seconds"`
}

// TrafficLogEntry represents a per-user bandwidth accounting record.
type TrafficLogEntry struct {
	Time    time.Time `json:"time"`
	UserID  int64     `json:"user_id"`
	NodeID  int64     `json:"node_id"`
	RxBytes int64     `json:"rx_bytes"`
	TxBytes int64     `json:"tx_bytes"`
}
