package api

import (
	"database/sql"
	"log"
	"sync"
)

// PreparedStmts holds prepared statements for high-frequency queries.
// Statements are prepared lazily on first use via sync.Once to avoid
// startup dependency ordering issues.
type PreparedStmts struct {
	once sync.Once
	err  error

	// Node authentication (used by agent version/download/update endpoints)
	nodeAuth *sql.Stmt
}

// prepareAll prepares all cached statements. Called via sync.Once.
func (p *PreparedStmts) prepareAll(db *sql.DB) {
	p.once.Do(func() {
		var err error

		p.nodeAuth, err = db.Prepare(`SELECT id,status FROM nodes WHERE api_token_hash=$1 LIMIT 1`)
		if err != nil {
			log.Printf("[prepared] failed to prepare nodeAuth: %v", err)
			p.err = err
			return
		}

		log.Printf("[prepared] all statements prepared successfully")
	})
}

// initStmts ensures the prepared statements are initialized and returns
// true if they are ready to use. Falls back to direct queries on failure.
func (s *Server) initStmts() bool {
	s.stmts.prepareAll(s.DB)
	return s.stmts.err == nil
}
