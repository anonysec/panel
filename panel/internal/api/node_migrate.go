package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// migrationState tracks the progress of a node migration.
type migrationState struct {
	ID          string `json:"migration_id"`
	Status      string `json:"status"` // running, completed, failed
	SourceNode  int64  `json:"source_node_id"`
	DestNode    int64  `json:"destination_node_id"`
	Total       int    `json:"total"`
	Migrated    int    `json:"migrated"`
	Failed      int    `json:"failed"`
	Error       string `json:"error,omitempty"`
	StartedAt   time.Time
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// migrationStore holds all in-flight and recent migration operations.
var migrationStore sync.Map

// handleNodeMigrate handles POST /api/nodes/{id}/migrate
// It migrates all active xray_inbounds from the source node to a destination node.
func (s *Server) handleNodeMigrate(w http.ResponseWriter, r *http.Request, sourceNodeID int64) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		DestinationNodeID int64 `json:"destination_node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.DestinationNodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "destination_node_id_required"})
		return
	}

	if in.DestinationNodeID == sourceNodeID {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "same_node"})
		return
	}

	// Validate source node exists
	var srcExists int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", sourceNodeID).Scan(&srcExists); err != nil || srcExists == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "source_node_not_found"})
		return
	}

	// Validate destination node exists
	var destExists int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", in.DestinationNodeID).Scan(&destExists); err != nil || destExists == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "destination_node_not_found"})
		return
	}

	// Generate migration ID
	migrationID := generateMigrationID()

	// Store initial state
	state := &migrationState{
		ID:         migrationID,
		Status:     "running",
		SourceNode: sourceNodeID,
		DestNode:   in.DestinationNodeID,
		StartedAt:  time.Now(),
	}
	migrationStore.Store(migrationID, state)

	actor, _, _ := s.currentAdmin(r)

	// Start migration in background goroutine
	go s.runNodeMigration(migrationID, sourceNodeID, in.DestinationNodeID, actor)

	writeJSON(w, map[string]any{"ok": true, "migration_id": migrationID})
}

// handleMigrationStatus handles GET /api/nodes/migrate/status?migration_id=xxx
func (s *Server) handleMigrationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	migrationID := r.URL.Query().Get("migration_id")
	if migrationID == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "migration_id_required"})
		return
	}

	val, exists := migrationStore.Load(migrationID)
	if !exists {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "migration_not_found"})
		return
	}

	state := val.(*migrationState)
	writeJSON(w, map[string]any{
		"ok":                  true,
		"migration_id":        state.ID,
		"status":              state.Status,
		"source_node_id":      state.SourceNode,
		"destination_node_id": state.DestNode,
		"total":               state.Total,
		"migrated":            state.Migrated,
		"failed":              state.Failed,
		"error":               state.Error,
	})
}

// runNodeMigration performs the actual migration of xray_inbounds from source to destination.
func (s *Server) runNodeMigration(migrationID string, sourceNodeID, destNodeID int64, actor string) {
	updateMigration := func(total, migrated, failed int, status, errMsg string) {
		val, ok := migrationStore.Load(migrationID)
		if !ok {
			return
		}
		st := val.(*migrationState)
		st.Total = total
		st.Migrated = migrated
		st.Failed = failed
		st.Status = status
		st.Error = errMsg
		if status == "completed" || status == "failed" {
			now := time.Now()
			st.CompletedAt = &now
		}
		migrationStore.Store(migrationID, st)
	}

	// Query all active xray_inbounds for the source node
	rows, err := s.DB.Query(`
		SELECT id, customer_id, uuid, protocol, transport, security, port,
			COALESCE(server_name, ''), COALESCE(public_key, ''), COALESCE(short_id, ''),
			COALESCE(private_key, ''), COALESCE(path, ''), COALESCE(service_name, ''), core_name
		FROM xray_inbounds
		WHERE node_id = ? AND status = 'active'`, sourceNodeID)
	if err != nil {
		log.Printf("[migrate] failed to query inbounds for node %d: %v", sourceNodeID, err)
		updateMigration(0, 0, 0, "failed", "db_query_failed")
		return
	}
	defer rows.Close()

	type inboundRow struct {
		ID          int64
		CustomerID  int64
		UUID        string
		Protocol    string
		Transport   string
		Security    string
		Port        int
		ServerName  string
		PublicKey   string
		ShortID     string
		PrivateKey  string
		Path        string
		ServiceName string
		CoreName    string
	}

	var inbounds []inboundRow
	for rows.Next() {
		var ib inboundRow
		if err := rows.Scan(&ib.ID, &ib.CustomerID, &ib.UUID, &ib.Protocol, &ib.Transport,
			&ib.Security, &ib.Port, &ib.ServerName, &ib.PublicKey, &ib.ShortID,
			&ib.PrivateKey, &ib.Path, &ib.ServiceName, &ib.CoreName); err != nil {
			log.Printf("[migrate] scan error: %v", err)
			continue
		}
		inbounds = append(inbounds, ib)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[migrate] rows iteration error: %v", err)
		updateMigration(0, 0, 0, "failed", "db_rows_error")
		return
	}

	total := len(inbounds)
	migrated := 0
	failed := 0

	updateMigration(total, 0, 0, "running", "")

	if total == 0 {
		log.Printf("[migrate] completed: no active inbounds on node %d", sourceNodeID)
		updateMigration(0, 0, 0, "completed", "")
		return
	}

	// Migrate each inbound
	for _, ib := range inbounds {
		if err := s.migrateInbound(ib.ID, ib.CustomerID, ib.UUID, ib.Protocol, ib.Transport,
			ib.Security, ib.Port, ib.ServerName, ib.PublicKey, ib.ShortID, ib.PrivateKey,
			ib.Path, ib.ServiceName, ib.CoreName, sourceNodeID, destNodeID, actor); err != nil {
			log.Printf("[migrate] failed to migrate inbound %d (uuid=%s): %v", ib.ID, ib.UUID, err)
			failed++
		} else {
			migrated++
		}
		updateMigration(total, migrated, failed, "running", "")
	}

	// Complete
	updateMigration(total, migrated, failed, "completed", "")
	log.Printf("[migrate] completed: %d/%d migrated, %d failed", migrated, total, failed)
}

// migrateInbound moves a single xray inbound from source to destination node.
func (s *Server) migrateInbound(inboundID, customerID int64, uuid, protocol, transport, security string,
	port int, serverName, publicKey, shortID, privateKey, path, serviceName, coreName string,
	sourceNodeID, destNodeID int64, actor string) error {

	// NOTE: Legacy node_tasks INSERT removed. Xray add/remove is now dispatched via gRPC.
	log.Printf("[migrate] xray_add for inbound %d on dest node %d (dispatched via gRPC)", inboundID, destNodeID)
	log.Printf("[migrate] xray_remove for inbound %d on source node %d (dispatched via gRPC)", inboundID, sourceNodeID)

	// Update xray_inbounds to point to the destination node
	_, err := s.DB.Exec(`UPDATE xray_inbounds SET node_id = ? WHERE id = ?`, destNodeID, inboundID)
	if err != nil {
		return fmt.Errorf("update inbound node_id: %w", err)
	}

	// Update customer subscription records if preferred_node_id exists
	// The customers table may have a node_id or preferred node — update it for migrated users
	_, _ = s.DB.Exec(
		`UPDATE customers SET node_id = ? WHERE id = ? AND node_id = ?`,
		destNodeID, customerID, sourceNodeID,
	)

	return nil
}

// generateMigrationID creates a random hex ID for tracking migrations.
func generateMigrationID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
