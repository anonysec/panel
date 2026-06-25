package api

import "database/sql"

// CheckPendingUpdateHealth is a no-op.
// The legacy node_tasks-based update health checking has been removed.
// Agent updates are now dispatched and monitored via gRPC.
func CheckPendingUpdateHealth(_ *sql.DB, _ func(string)) {
	// No-op: node_tasks table is no longer used for agent updates.
}
