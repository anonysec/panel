package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func (s *Server) liveSessionsPayload() []map[string]any {
	rows, err := s.DB.Query(`SELECT radacctid, username, COALESCE(framedipaddress,''), acctinputoctets, acctoutputoctets, acctsessiontime FROM radacct WHERE acctstoptime IS NULL`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()

	now := time.Now()
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	activeIDs := make(map[int64]bool)
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var username, ip string
		var rx, tx, duration int64
		if err := rows.Scan(&id, &username, &ip, &rx, &tx, &duration); err == nil {
			activeIDs[id] = true
			prev, exists := s.prevSessionBytes[id]
			rxSpeed := 0.0
			txSpeed := 0.0
			if exists {
				dt := now.Sub(prev.Timestamp).Seconds()
				if dt > 0.1 {
					rxSpeed = float64(rx-prev.InputBytes) / 1024.0 / dt
					txSpeed = float64(tx-prev.OutputBytes) / 1024.0 / dt
					if rxSpeed < 0 {
						rxSpeed = 0
					}
					if txSpeed < 0 {
						txSpeed = 0
					}
				}
			}
			s.prevSessionBytes[id] = SessionBytes{
				InputBytes:  rx,
				OutputBytes: tx,
				Timestamp:   now,
			}

			out = append(out, map[string]any{
				"id":            id,
				"username":      username,
				"ip":            ip,
				"duration":      duration,
				"rx_bytes":      rx,
				"tx_bytes":      tx,
				"rx_speed_kbps": rxSpeed,
				"tx_speed_kbps": txSpeed,
			})
		}
	}

	// Cleanup stale entries: remove sessions no longer active to prevent memory leak
	for id := range s.prevSessionBytes {
		if !activeIDs[id] {
			delete(s.prevSessionBytes, id)
		}
	}

	return out
}

func (s *Server) bandwidthPayload() []map[string]any {
	rows, err := s.DB.Query(`SELECT username, ip, rx_bps, tx_bps FROM user_bandwidth_snapshots WHERE created_at >= NOW() - INTERVAL '30 seconds' ORDER BY created_at DESC`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()

	// Group by username, take the most recent entry per user
	seen := make(map[string]bool)
	out := []map[string]any{}
	for rows.Next() {
		var username, ip string
		var rxBps, txBps int64
		if err := rows.Scan(&username, &ip, &rxBps, &txBps); err == nil {
			if !seen[username] {
				seen[username] = true
				out = append(out, map[string]any{
					"username": username,
					"ip":       ip,
					"rx_bps":   rxBps,
					"tx_bps":   txBps,
				})
			}
		}
	}
	return out
}

func (s *Server) killSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	var in struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	var sessionID, username, nasIP string
	err := s.DB.QueryRow(`SELECT acctsessionid, username, COALESCE(nasipaddress,'127.0.0.1') FROM radacct WHERE radacctid=$1 LIMIT 1`, in.ID).Scan(&sessionID, &username, &nasIP)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "session_not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Validate nasIP to prevent command injection (must be a valid IP address)
	if net.ParseIP(nasIP) == nil {
		nasIP = "127.0.0.1"
	}

	go func() {
		attrs := fmt.Sprintf("User-Name=%s,Acct-Session-Id=%s", username, sessionID)
		cmd := exec.Command("radclient", "-x", nasIP+":3799", "disconnect", "testing123")
		cmd.Stdin = strings.NewReader(attrs)
		_ = cmd.Run()
	}()

	_, err = s.DB.Exec(`UPDATE radacct SET acctstoptime=NOW() WHERE radacctid=$1`, in.ID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "session.killed", "session", strconv.FormatInt(in.ID, 10), nil, map[string]any{"username": username, "session_id": sessionID}, clientIP(r))
	s.createEvent("session", "warning", fmt.Sprintf("Session terminated: %s", username), fmt.Sprintf("Admin %s terminated VPN session #%d for %s", actor, in.ID, username), actor, username)

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) resellerCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	actor, role, ok := s.currentAdmin(r)
	if !ok || role != "reseller" {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "reseller_only"})
		return
	}

	var in struct {
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Amount <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_amount"})
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE admins SET credit = credit + $1 WHERE username=$2`, in.Amount, actor)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	desc := fmt.Sprintf("Automatic Gateway Top-up (Cryptomus/Zarinpal): +%.2f IRT", in.Amount)
	_, err = tx.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES($1,$2, 'allocation', $3, $4)`, actor, in.Amount, desc, actor)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	s.logAudit(actor, "reseller.checkout_completed", "reseller", actor, nil, map[string]any{"username": actor, "amount": in.Amount}, clientIP(r))
	s.createEvent("reseller", "success", fmt.Sprintf("Reseller top-up: %s", actor), fmt.Sprintf("Reseller %s automatically topped up +%.2f IRT via gateway", actor, in.Amount), actor, actor)

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) resellerTransactions(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	where := "1=1"
	args := []any{}
	if role == "reseller" {
		where = "reseller_username = $1"
		args = append(args, actor)
	} else {
		reseller := r.URL.Query().Get("reseller")
		if reseller != "" {
			where = "reseller_username = $1"
			args = append(args, reseller)
		}
	}

	rows, err := s.DB.Query(`SELECT id, reseller_username, amount, type, description, actor, created_at FROM reseller_transactions WHERE `+where+` ORDER BY id DESC LIMIT 500`, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type Tx struct {
		ID          int64   `json:"id"`
		Reseller    string  `json:"reseller_username"`
		Amount      float64 `json:"amount"`
		Type        string  `json:"type"`
		Description string  `json:"description"`
		Actor       string  `json:"actor"`
		CreatedAt   string  `json:"created_at"`
	}

	list := []Tx{}
	for rows.Next() {
		var t Tx
		var created time.Time
		if err := rows.Scan(&t.ID, &t.Reseller, &t.Amount, &t.Type, &t.Description, &t.Actor, &created); err == nil {
			t.CreatedAt = created.Format(time.RFC3339)
			list = append(list, t)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "transactions": list})
}

func (s *Server) disconnectCustomerSessions(username string) {
	rows, err := s.DB.Query(`SELECT radacctid, acctsessionid, COALESCE(nasipaddress,'127.0.0.1') FROM radacct WHERE username=$1 AND acctstoptime IS NULL`, username)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var sessionID, nasIP string
		if err := rows.Scan(&id, &sessionID, &nasIP); err == nil {
			go func(u, sID, ip string) {
				// Validate nasIP to prevent command injection
				if net.ParseIP(ip) == nil {
					ip = "127.0.0.1"
				}
				attrs := fmt.Sprintf("User-Name=%s,Acct-Session-Id=%s", u, sID)
				cmd := exec.Command("radclient", "-x", ip+":3799", "disconnect", "testing123")
				cmd.Stdin = strings.NewReader(attrs)
				_ = cmd.Run()
			}(username, sessionID, nasIP)

			_, _ = s.DB.Exec(`UPDATE radacct SET acctstoptime=NOW() WHERE radacctid=$1`, id)
		}
	}
}

func (s *Server) resellerPayments(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || role != "reseller" {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "reseller_only"})
		return
	}

	if r.Method == http.MethodPost {
		var in struct {
			Amount      float64 `json:"amount"`
			Description string  `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Amount <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_amount"})
			return
		}

		_, err := s.DB.Exec(`INSERT INTO payments(username, amount, method, status, intent_type, admin_note) VALUES($1, $2, 'manual', 'pending', 'reseller_topup', $3)`, actor, in.Amount, in.Description)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		s.createEvent("reseller", "info", fmt.Sprintf("Reseller top-up request: %s", actor), fmt.Sprintf("Reseller %s requested +%.2f IRT credit top-up", actor, in.Amount), actor, actor)
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	if r.Method == http.MethodGet {
		rows, err := s.DB.Query(`SELECT id, amount, method, status, COALESCE(admin_note,''), created_at FROM payments WHERE username=$1 AND intent_type='reseller_topup' ORDER BY id DESC LIMIT 100`, actor)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()

		type Pay struct {
			ID        int64   `json:"id"`
			Amount    float64 `json:"amount"`
			Method    string  `json:"method"`
			Status    string  `json:"status"`
			Note      string  `json:"note"`
			CreatedAt string  `json:"created_at"`
		}
		list := []Pay{}
		for rows.Next() {
			var p Pay
			var created time.Time
			if err := rows.Scan(&p.ID, &p.Amount, &p.Method, &p.Status, &p.Note, &created); err == nil {
				p.CreatedAt = created.Format(time.RFC3339)
				list = append(list, p)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "payments": list})
		return
	}

	http.Error(w, "method", http.StatusMethodNotAllowed)
}

// NoCacheMiddleware sets Cache-Control: no-store on all /api/ responses to prevent
// browser-level caching of dynamic data.
