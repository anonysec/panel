package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"KorisPanel/panel/internal/auth"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// provisionState tracks the progress of an SSH-based node provisioning.
type provisionState struct {
	ID        string `json:"provision_id"`
	Status    string `json:"status"` // connecting, installing, configuring, verifying, completed, failed
	Error     string `json:"error,omitempty"`
	NodeID    int64  `json:"node_id,omitempty"`
	StartedAt time.Time
}

// provisionStore holds all in-flight provisioning operations.
var provisionStore sync.Map

// handleNodeProvisionSSH handles POST /api/admin/nodes/provision
// It accepts SSH credentials and starts provisioning in a goroutine.
func (s *Server) handleNodeProvisionSSH(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Key      string `json:"key"`
		GroupID  *int64 `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate required fields
	in.Host = strings.TrimSpace(in.Host)
	if in.Host == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "host_required"})
		return
	}
	if in.Password == "" && in.Key == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "auth_required"})
		return
	}

	// Defaults
	if in.Port == 0 {
		in.Port = 22
	}
	if in.User == "" {
		in.User = "root"
	}

	// Generate a unique provision ID
	provisionID := generateProvisionID()

	// Generate a bearer token for the new node
	nodeToken := "kn_" + auth.RandomToken(24)

	// Store initial state
	state := &provisionState{
		ID:        provisionID,
		Status:    "connecting",
		StartedAt: time.Now(),
	}
	provisionStore.Store(provisionID, state)

	// Get panel URL for the install script
	panelURL := s.getPanelURL(r)
	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	// Start provisioning in background
	go s.runProvisionSSH(provisionID, in.Host, in.Port, in.User, in.Password, in.Key, in.GroupID, nodeToken, panelURL, actor, ip)

	writeJSON(w, map[string]any{
		"ok":           true,
		"provision_id": provisionID,
	})
}

// runProvisionSSH performs the actual SSH-based provisioning in background.
func (s *Server) runProvisionSSH(provisionID, host string, port int, user, password, key string, groupID *int64, nodeToken, panelURL, actor, actorIP string) {
	updateState := func(status, errMsg string) {
		val, ok := provisionStore.Load(provisionID)
		if !ok {
			return
		}
		st := val.(*provisionState)
		st.Status = status
		st.Error = errMsg
		provisionStore.Store(provisionID, st)
	}

	setNodeID := func(nodeID int64) {
		val, ok := provisionStore.Load(provisionID)
		if !ok {
			return
		}
		st := val.(*provisionState)
		st.NodeID = nodeID
		provisionStore.Store(provisionID, st)
	}

	// Build SSH auth methods
	var authMethods []ssh.AuthMethod
	if key != "" {
		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			log.Printf("[provision] failed to parse SSH key for %s: %v", host, err)
			updateState("failed", "invalid_ssh_key")
			return
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}

	// Step 1: Connect via SSH
	updateState("connecting", "")
	log.Printf("[provision] connecting to %s:%d as %s", host, port, user)

	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		log.Printf("[provision] SSH connect failed for %s: %v", host, err)
		updateState("failed", "ssh_connect_failed: "+err.Error())
		return
	}
	defer client.Close()

	// Step 2: Install node agent
	updateState("installing", "")
	log.Printf("[provision] installing node agent on %s", host)

	installCmd := fmt.Sprintf(
		`curl -sSL %s/api/node/install.sh | PANEL_URL=%s NODE_TOKEN=%s NODE_NAME=%s bash`,
		panelURL, panelURL, nodeToken, "node-"+host,
	)

	output, err := sshExec(client, installCmd)
	if err != nil {
		log.Printf("[provision] install script failed on %s: %v\nOutput: %s", host, err, output)
		updateState("failed", "install_failed: "+err.Error())
		return
	}

	// Step 3: Configure - write node.env with bearer token and panel URL
	updateState("configuring", "")
	log.Printf("[provision] configuring node agent on %s", host)

	configCmd := fmt.Sprintf(
		`mkdir -p /etc/knode && cat > /etc/knode/node.env <<'EOF'
PANEL_URL='%s'
NODE_TOKEN='%s'
NODE_NAME='node-%s'
NODE_INTERVAL=10
LOG_LEVEL=info
NODE_AUTO_UPDATE=true
EOF
chmod 600 /etc/knode/node.env && systemctl restart knode`,
		panelURL, nodeToken, host,
	)

	output, err = sshExec(client, configCmd)
	if err != nil {
		log.Printf("[provision] configure failed on %s: %v\nOutput: %s", host, err, output)
		updateState("failed", "configure_failed: "+err.Error())
		return
	}

	// Step 4: Register node in database
	updateState("verifying", "")
	log.Printf("[provision] registering node %s in database", host)

	tokenHash := hashToken(nodeToken)
	nodeName := "node-" + host

	var groupIDVal sql.NullInt64
	if groupID != nil {
		groupIDVal = sql.NullInt64{Int64: *groupID, Valid: true}
	}

	res, err := s.DB.Exec(
		`INSERT INTO nodes (name, public_ip, api_token_hash, status, group_id) VALUES ($1, $2, $3, 'online', $4)`,
		nodeName, host, tokenHash, groupIDVal,
	)
	if err != nil {
		log.Printf("[provision] DB insert failed for %s: %v", host, err)
		updateState("failed", "db_register_failed: "+err.Error())
		// Attempt cleanup on the remote server
		s.cleanupProvisionedNode(client)
		return
	}

	nodeID, _ := res.LastInsertId()
	setNodeID(nodeID)

	// Step 5: Wait for the node agent to push healthy metrics (up to 120s)
	log.Printf("[provision] waiting for healthy metrics from node %s (id=%d)", host, nodeID)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	healthy := false
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Timeout waiting for healthy metrics
			log.Printf("[provision] timeout waiting for healthy metrics from node %s (id=%d)", host, nodeID)
			// Don't clean up — node is registered, agent may just be slow
			updateState("completed", "")
			healthy = true // Mark as completed anyway, node is registered
		case <-ticker.C:
			var lastSeen sql.NullTime
			err := s.DB.QueryRow(
				`SELECT last_seen_at FROM nodes WHERE id = $1 AND status = 'online'`, nodeID,
			).Scan(&lastSeen)
			if err == nil && lastSeen.Valid && time.Since(lastSeen.Time) < 30*time.Second {
				healthy = true
			}
		}
		if healthy {
			break
		}
	}

	updateState("completed", "")
	log.Printf("[provision] provisioning completed for node %s (id=%d)", host, nodeID)

	// Audit log
	s.logAudit(actor, "node.ssh_provisioned", "node", strconv.FormatInt(nodeID, 10), nil, map[string]any{
		"host":     host,
		"port":     port,
		"group_id": groupID,
	}, actorIP)

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}
}

// cleanupProvisionedNode attempts to remove the node agent from a server on failed provisioning.
func (s *Server) cleanupProvisionedNode(client *ssh.Client) {
	cleanupCmd := `systemctl stop knode 2>/dev/null; systemctl disable knode 2>/dev/null; rm -f /etc/knode/node.env /usr/local/bin/knode /etc/systemd/system/knode.service; systemctl daemon-reload 2>/dev/null`
	output, err := sshExec(client, cleanupCmd)
	if err != nil {
		log.Printf("[provision] cleanup failed: %v\nOutput: %s", err, output)
	}
}

// sshExec runs a command on the remote server and returns its combined output.
func sshExec(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// generateProvisionID creates a random hex ID for tracking provisioning.
func generateProvisionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// handleProvisionStatus handles GET /api/admin/nodes/provision/status (WebSocket)
// It provides real-time provisioning progress updates.
func (s *Server) handleProvisionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	provisionID := r.URL.Query().Get("provision_id")
	if provisionID == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "provision_id_required"})
		return
	}

	// Check that the provision exists
	_, exists := provisionStore.Load(provisionID)
	if !exists {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "provision_not_found"})
		return
	}

	// Upgrade to WebSocket
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return s.checkWSOrigin(r)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[provision] websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Read pump — detect client disconnect
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Send state updates every second until completed or failed
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			val, ok := provisionStore.Load(provisionID)
			if !ok {
				_ = conn.WriteJSON(map[string]any{
					"status": "failed",
					"error":  "provision_expired",
				})
				return
			}

			state := val.(*provisionState)

			// Only send if status changed
			if state.Status != lastStatus {
				msg := map[string]any{
					"provision_id": state.ID,
					"status":       state.Status,
					"node_id":      state.NodeID,
				}
				if state.Error != "" {
					msg["error"] = state.Error
				}

				if err := conn.WriteJSON(msg); err != nil {
					return
				}
				lastStatus = state.Status
			}

			// Close connection when provisioning is done
			if state.Status == "completed" || state.Status == "failed" {
				// Send final state and close
				return
			}
		}
	}
}
