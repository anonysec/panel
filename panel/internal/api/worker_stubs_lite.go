//go:build lite

package api

import "database/sql"

// No-op stubs for lite build — these functions are defined in !lite files
// but called from shared code paths (main.go worker tick, api.go handlers).

// CheckSLABreachesStandalone is a no-op in the lite build (defined in worker_sla.go).
func CheckSLABreachesStandalone(_ *sql.DB, _ func(string)) {}

// AutoCloseStaleTicketsStandalone is a no-op in the lite build (defined in worker_sla.go).
func AutoCloseStaleTicketsStandalone(_ *sql.DB, _ func(string)) {}

// CheckOverdueTickets is a no-op in the lite build (defined in sla_timers.go).
func CheckOverdueTickets(_ *sql.DB, _ func(string)) {}

// ReEvaluateLoadBalancing is a no-op in the lite build (defined in worker_loadbalancing.go).
func ReEvaluateLoadBalancing(_ *sql.DB, _ func(string)) {}

// RecordNodeDowntime is a no-op in the lite build (defined in node_sla.go).
func RecordNodeDowntime(_ *sql.DB, _ int64, _ string) {}

// CloseNodeDowntime is a no-op in the lite build (defined in node_sla.go).
func CloseNodeDowntime(_ *sql.DB, _ int64) {}
