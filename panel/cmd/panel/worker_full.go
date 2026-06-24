//go:build !lite

package main

import (
	"database/sql"

	"KorisPanel/panel/internal/api"
	"KorisPanel/panel/internal/notify"
	"KorisPanel/panel/internal/teleproxy"
)

// workerTickExcluded runs premium-feature worker operations in the full build.
// Called from workerTick on every tick cycle.
func workerTickExcluded(db *sql.DB, notifier *notify.Notifier, tickCount int) {
	// PAYG Billing: deduct from wallet based on usage for pay-as-you-go plans
	processPaygBilling(db)

	// SLA breach detection: mark overdue tickets and notify admin
	api.CheckSLABreachesStandalone(db, notifier.Send)

	// Auto-close stale tickets: close tickets with no customer reply after configured days
	api.AutoCloseStaleTicketsStandalone(db, notifier.Send)

	// SLA timers: check for overdue support tickets and send alerts
	api.CheckOverdueTickets(db, notifier.Send)

	// Telegram proxy health checks: TCP ping each proxy, update status
	teleproxy.CheckHealth(db)

	// Load balancing re-evaluation: every 5 ticks (5 minutes)
	if tickCount%5 == 0 {
		api.ReEvaluateLoadBalancing(db, notifier.Send)
	}
}
