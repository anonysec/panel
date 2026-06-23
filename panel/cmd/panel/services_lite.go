//go:build lite

package main

import (
	"database/sql"

	"KorisPanel/panel/internal/api"
)

// initExcludedServices is a no-op in the lite build.
// Excluded service fields remain nil on the Server struct.
func initExcludedServices(_ *api.Server, _ *sql.DB) {}

// startBot is a no-op in the lite build.
func startBot(_ *sql.DB, _ string, _ []int64) {}
