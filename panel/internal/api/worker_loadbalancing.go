//go:build !lite

package api

import (
	"database/sql"
	"fmt"
	"log"

	"KorisPanel/panel/internal/loadbalance"
)

// ReEvaluateLoadBalancing queries all node groups with load_balancing_enabled=1,
// checks capacity for each, and notifies the admin if any group is overloaded.
// Designed to be called from the background worker every 5 minutes.
func ReEvaluateLoadBalancing(db *sql.DB, notify func(string)) {
	rows, err := db.Query(`SELECT id, name, max_load_percent FROM node_groups WHERE load_balancing_enabled = 1`)
	if err != nil {
		log.Printf("[loadbalance] re-evaluate query error: %v", err)
		return
	}
	defer rows.Close()

	type group struct {
		ID             int64
		Name           string
		MaxLoadPercent int
	}

	var groups []group
	for rows.Next() {
		var g group
		if err := rows.Scan(&g.ID, &g.Name, &g.MaxLoadPercent); err != nil {
			log.Printf("[loadbalance] scan error: %v", err)
			continue
		}
		groups = append(groups, g)
	}

	for _, g := range groups {
		overloaded, notifications := checkGroupCapacityStandalone(db, g.ID, g.Name, g.MaxLoadPercent)
		if overloaded {
			log.Printf("[loadbalance] group %q (ID=%d) is overloaded", g.Name, g.ID)
		}
		for _, msg := range notifications {
			if notify != nil {
				notify(msg)
			}
		}
	}
}

// checkGroupCapacityStandalone is a standalone version of checkGroupCapacity that
// works with raw *sql.DB instead of requiring a *Server. Checks if any nodes in
// a group are over the 85% threshold and if all nodes exceed 90%, sends notification.
func checkGroupCapacityStandalone(db *sql.DB, groupID int64, groupName string, maxLoadPercent int) (overloaded bool, notifications []string) {
	rows, err := db.Query(
		`SELECT id, name, public_ip, max_capacity FROM nodes WHERE group_id = ? AND status <> 'disabled'`,
		groupID,
	)
	if err != nil {
		log.Printf("[loadbalance] failed to query nodes for group %d: %v", groupID, err)
		return false, nil
	}
	defer rows.Close()

	type nodeState struct {
		id          int64
		name        string
		loadPercent float64
	}

	var nodeStates []nodeState
	for rows.Next() {
		var id int64
		var name, publicIP string
		var maxCapacity int
		if err := rows.Scan(&id, &name, &publicIP, &maxCapacity); err != nil {
			continue
		}

		var activeSessions int
		_ = db.QueryRow(
			`SELECT COUNT(*) FROM radacct WHERE acctstoptime IS NULL AND nasipaddress = ?`,
			publicIP,
		).Scan(&activeSessions)

		load := loadbalance.CalculateLoad(activeSessions, maxCapacity)
		nodeStates = append(nodeStates, nodeState{id: id, name: name, loadPercent: load})
	}

	if len(nodeStates) == 0 {
		return false, nil
	}

	// Check 85% threshold — flag for re-evaluation
	anyOver85 := false
	allOver90 := true

	for _, ns := range nodeStates {
		if ns.loadPercent >= 85.0 {
			anyOver85 = true
		}
		if ns.loadPercent < 90.0 {
			allOver90 = false
		}
	}

	overloaded = anyOver85

	// If all nodes >= 90%, send overload notification
	if allOver90 {
		msg := fmt.Sprintf("🚨 *Node Group Overloaded*\nGroup: `%s`\nAll nodes exceed 90%% load. No new users can be assigned until capacity is freed.", groupName)
		notifications = append(notifications, msg)
		log.Printf("[loadbalance] all nodes in group %q (ID=%d) exceed 90%% load", groupName, groupID)
	}

	return overloaded, notifications
}
