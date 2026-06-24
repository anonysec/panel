//go:build lite

package main

import (
	"database/sql"

	"KorisPanel/panel/internal/notify"
)

// workerTickExcluded is a no-op in the lite build.
// Premium-feature worker operations (billing, SLA, teleproxy, load balancing, statistics) are skipped.
func workerTickExcluded(_ *sql.DB, _ *notify.Notifier, _ int) {}
