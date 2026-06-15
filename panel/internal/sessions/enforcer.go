package sessions

import (
	"database/sql"
	"log"
	"time"
)

type Enforcer struct {
	db *sql.DB
}

func NewEnforcer(db *sql.DB) *Enforcer {
	return &Enforcer{db: db}
}

// EnforceConnLimit kills excess active sessions for users who exceed their connection limit.
// Called periodically by the worker goroutine.
func (e *Enforcer) EnforceConnLimit() {
	// Get users with conn_limit > 0 who have more active sessions than allowed
	rows, err := e.db.Query(`
		SELECT c.username, COALESCE(JSON_EXTRACT(c.extra_json, '$.conn_limit'), 0) AS conn_limit,
			(SELECT COUNT(*) FROM radacct WHERE username=c.username AND acctstoptime IS NULL) AS active
		FROM customers c
		WHERE c.status = 'active'
		HAVING conn_limit > 0 AND active > conn_limit
	`)
	if err != nil {
		log.Printf("[enforcer] query: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var username string
		var limit, active int
		if err := rows.Scan(&username, &limit, &active); err != nil {
			continue
		}
		excess := active - limit
		if excess <= 0 {
			continue
		}
		// Kill oldest excess sessions
		_, err := e.db.Exec(`
			UPDATE radacct SET acctstoptime=NOW(), acctterminatecause='Connection-Limit'
			WHERE username=? AND acctstoptime IS NULL
			ORDER BY acctstarttime ASC LIMIT ?
		`, username, excess)
		if err != nil {
			log.Printf("[enforcer] kill sessions for %s: %v", username, err)
		} else {
			log.Printf("[enforcer] killed %d excess sessions for %s (limit=%d)", excess, username, limit)
		}
	}
}

// Start runs the enforcer every 30 seconds
func (e *Enforcer) Start() {
	go func() {
		for {
			e.EnforceConnLimit()
			time.Sleep(30 * time.Second)
		}
	}()
}
