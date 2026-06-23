//go:build !lite

package api

import (
	"fmt"
	"log"
	"net/http"

	"KorisPanel/panel/internal/loadbalance"
)

// handleNodeGroupsLoad serves GET /api/node-groups/load — admin load overview dashboard.
// Returns per-group and per-node utilization percentages for groups with load balancing enabled.
func (s *Server) handleNodeGroupsLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONCode(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
		return
	}

	// Get all node groups with load balancing enabled
	rows, err := s.DB.Query(`SELECT id, name, max_load_percent FROM node_groups WHERE load_balancing_enabled = 1 ORDER BY name ASC`)
	if err != nil {
		log.Printf("[loadbalance] failed to query node groups: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type nodeInfo struct {
		ID             int64   `json:"id"`
		Name           string  `json:"name"`
		PublicIP       string  `json:"public_ip"`
		ActiveSessions int     `json:"active_sessions"`
		MaxCapacity    int     `json:"max_capacity"`
		LoadPercent    float64 `json:"load_percent"`
	}

	type groupInfo struct {
		ID            int64      `json:"id"`
		Name          string     `json:"name"`
		Nodes         []nodeInfo `json:"nodes"`
		TotalCapacity int        `json:"total_capacity"`
		TotalSessions int        `json:"total_sessions"`
		TotalLoad     float64    `json:"total_load"`
	}

	var groups []groupInfo
	for rows.Next() {
		var g groupInfo
		var maxLoadPercent int
		if err := rows.Scan(&g.ID, &g.Name, &maxLoadPercent); err != nil {
			continue
		}

		// Get all nodes in this group with their capacities
		nodeRows, err := s.DB.Query(
			`SELECT id, name, public_ip, max_capacity FROM nodes WHERE group_id = ? AND status <> 'disabled' ORDER BY name ASC`,
			g.ID,
		)
		if err != nil {
			log.Printf("[loadbalance] failed to query nodes for group %d: %v", g.ID, err)
			continue
		}

		for nodeRows.Next() {
			var n nodeInfo
			if err := nodeRows.Scan(&n.ID, &n.Name, &n.PublicIP, &n.MaxCapacity); err != nil {
				continue
			}

			// Count active sessions for this node via radacct
			_ = s.DB.QueryRow(
				`SELECT COUNT(*) FROM radacct WHERE acctstoptime IS NULL AND nasipaddress = ?`,
				n.PublicIP,
			).Scan(&n.ActiveSessions)

			n.LoadPercent = loadbalance.CalculateLoad(n.ActiveSessions, n.MaxCapacity)
			g.Nodes = append(g.Nodes, n)
			g.TotalCapacity += n.MaxCapacity
			g.TotalSessions += n.ActiveSessions
		}
		nodeRows.Close()

		// Calculate group-level aggregate load
		if g.TotalCapacity > 0 {
			g.TotalLoad = (float64(g.TotalSessions) / float64(g.TotalCapacity)) * 100.0
		} else {
			g.TotalLoad = 100.0
		}

		if g.Nodes == nil {
			g.Nodes = []nodeInfo{}
		}

		groups = append(groups, g)
	}

	if groups == nil {
		groups = []groupInfo{}
	}

	writeJSON(w, map[string]any{"ok": true, "groups": groups})
}

// selectNodeForGroup selects the least-loaded node in a group using the loadbalance package.
// Returns the selected node ID, or an error if the group is overloaded or has no nodes.
func (s *Server) selectNodeForGroup(groupID int64) (int64, error) {
	// Get the group's overload threshold
	var maxLoadPercent int
	err := s.DB.QueryRow(`SELECT max_load_percent FROM node_groups WHERE id = ?`, groupID).Scan(&maxLoadPercent)
	if err != nil {
		return 0, fmt.Errorf("group not found: %v", err)
	}

	// Get all active nodes in the group
	rows, err := s.DB.Query(
		`SELECT id, public_ip, max_capacity FROM nodes WHERE group_id = ? AND status <> 'disabled'`,
		groupID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to query nodes: %v", err)
	}
	defer rows.Close()

	var nodes []loadbalance.NodeLoad
	for rows.Next() {
		var nodeID int64
		var publicIP string
		var maxCapacity int
		if err := rows.Scan(&nodeID, &publicIP, &maxCapacity); err != nil {
			continue
		}

		var activeSessions int
		_ = s.DB.QueryRow(
			`SELECT COUNT(*) FROM radacct WHERE acctstoptime IS NULL AND nasipaddress = ?`,
			publicIP,
		).Scan(&activeSessions)

		nodes = append(nodes, loadbalance.NodeLoad{
			NodeID:         nodeID,
			ActiveSessions: activeSessions,
			MaxCapacity:    maxCapacity,
		})
	}

	// Use the loadbalance package to select the best node
	return loadbalance.SelectNode(nodes, float64(maxLoadPercent))
}

// checkGroupCapacity checks if any nodes in a group are over the 85% threshold,
// and if all nodes exceed 90%, flags the group as overloaded with a notification.
func (s *Server) checkGroupCapacity(groupID int64) (overloaded bool, notifications []string) {
	var groupName string
	err := s.DB.QueryRow(`SELECT name FROM node_groups WHERE id = ?`, groupID).Scan(&groupName)
	if err != nil {
		return false, nil
	}

	rows, err := s.DB.Query(
		`SELECT id, name, public_ip, max_capacity FROM nodes WHERE group_id = ? AND status <> 'disabled'`,
		groupID,
	)
	if err != nil {
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
		_ = s.DB.QueryRow(
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
		msg := fmt.Sprintf("group %s is overloaded", groupName)
		notifications = append(notifications, msg)

		// Notify admin via Telegram
		if s.Notify != nil {
			s.Notify.SendEvent("warning", "Node Group Overloaded",
				fmt.Sprintf("All nodes in group \"%s\" exceed 90%% load. No new users can be assigned until capacity is freed.", groupName))
		}
		log.Printf("[loadbalance] all nodes in group %q (ID=%d) exceed 90%% load", groupName, groupID)
	}

	return overloaded, notifications
}
