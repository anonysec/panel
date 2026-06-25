package dbstore

import "errors"

// Sentinel errors for database operations.
// These provide typed error codes that callers can check with errors.Is().
var (
	// ErrDuplicateNode is returned when attempting to create a node with a name that already exists.
	ErrDuplicateNode = errors.New("duplicate_node")

	// ErrInvalidReference is returned when a foreign key reference is invalid (e.g., node_id doesn't exist).
	ErrInvalidReference = errors.New("invalid_reference")

	// ErrNotFound is returned when a requested record does not exist.
	ErrNotFound = errors.New("not_found")

	// ErrSessionExpired is returned when a session token exists but has passed its expiry time.
	ErrSessionExpired = errors.New("session_expired")

	// ErrLockNotAcquired is returned when an advisory lock could not be obtained.
	ErrLockNotAcquired = errors.New("lock_not_acquired")

	// ErrMigrationFailed is returned when a database migration fails to apply.
	ErrMigrationFailed = errors.New("migration_failed")

	// ErrConnectionLost is returned when the database connection is unavailable.
	ErrConnectionLost = errors.New("connection_lost")

	// ErrConstraintViolation is returned when a database constraint (unique, check, etc.) is violated.
	ErrConstraintViolation = errors.New("constraint_violation")
)
