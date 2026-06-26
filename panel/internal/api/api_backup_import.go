package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) backupImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_form"})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file_required"})
		return
	}
	defer file.Close()

	var backup struct {
		Version    int                         `json:"version"`
		ExportedAt string                      `json:"exported_at"`
		Tables     map[string][]map[string]any `json:"tables"`
	}
	if err := json.NewDecoder(file).Decode(&backup); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_json"})
		return
	}
	if backup.Version == 0 || backup.Tables == nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_backup_format"})
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	// Track import statistics per table
	imported := map[string]int{}
	failed := map[string]int{}

	// Import nodes first (referenced by vpn_configs)
	if nodes, ok := backup.Tables["nodes"]; ok {
		for _, n := range nodes {
			_, err := tx.Exec(`INSERT INTO nodes(id, name, public_ip, domain, status) VALUES($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`,
				toInt64(n["id"]), toString(n["name"]), toString(n["public_ip"]),
				toString(n["domain"]), toString(n["status"]))
			if err != nil {
				failed["nodes"]++
			} else {
				imported["nodes"]++
			}
		}
	}

	// Import plans (referenced by customers)
	if plans, ok := backup.Tables["plans"]; ok {
		for _, p := range plans {
			_, err := tx.Exec(`INSERT INTO plans(id, name, data_gb, speed_mbps, duration_days, price, billing_type, price_per_gb, price_per_day, disconnect_on_zero, is_active, sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (id) DO NOTHING`,
				toInt64(p["id"]), toString(p["name"]), toFloat64(p["data_gb"]), toFloat64(p["speed_mbps"]),
				toInt(p["duration_days"]), toFloat64(p["price"]), toString(p["billing_type"]),
				toFloat64(p["price_per_gb"]), toFloat64(p["price_per_day"]), toBool(p["disconnect_on_zero"]),
				toBool(p["is_active"]), toInt(p["sort_order"]))
			if err != nil {
				failed["plans"]++
			} else {
				imported["plans"]++
			}
		}
	}

	// Import customers
	if customers, ok := backup.Tables["customers"]; ok {
		for _, c := range customers {
			planID := sql.NullInt64{}
			if v, exists := c["plan_id"]; exists && v != nil {
				planID = sql.NullInt64{Int64: toInt64(v), Valid: true}
			}
			_, err := tx.Exec(`INSERT INTO customers(id, username, display_name, status, plan_id, notes, sub_token) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
				toInt64(c["id"]), toString(c["username"]), toString(c["display_name"]),
				toString(c["status"]), planID, toString(c["notes"]), toString(c["sub_token"]))
			if err != nil {
				failed["customers"]++
			} else {
				imported["customers"]++
			}
		}
	}

	// Import wallets
	if wallets, ok := backup.Tables["wallets"]; ok {
		for _, wal := range wallets {
			_, err := tx.Exec(`INSERT INTO wallets(username, credit) VALUES($1,$2) ON CONFLICT (username) DO UPDATE SET credit=EXCLUDED.credit`,
				toString(wal["username"]), toFloat64(wal["credit"]))
			if err != nil {
				failed["wallets"]++
			} else {
				imported["wallets"]++
			}
		}
	}

	// Import radcheck
	if radcheck, ok := backup.Tables["radcheck"]; ok {
		for _, rc := range radcheck {
			_, err := tx.Exec(`INSERT INTO radcheck(id, username, attribute, op, value) VALUES($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`,
				toInt64(rc["id"]), toString(rc["username"]), toString(rc["attribute"]), toString(rc["op"]), toString(rc["value"]))
			if err != nil {
				failed["radcheck"]++
			} else {
				imported["radcheck"]++
			}
		}
	}

	// Import payments
	if payments, ok := backup.Tables["payments"]; ok {
		for _, p := range payments {
			intentID := sql.NullInt64{}
			if v, exists := p["intent_id"]; exists && v != nil {
				intentID = sql.NullInt64{Int64: toInt64(v), Valid: true}
			}
			_, err := tx.Exec(`INSERT INTO payments(id, username, amount, method, status, intent_type, intent_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
				toInt64(p["id"]), toString(p["username"]), toFloat64(p["amount"]),
				toString(p["method"]), toString(p["status"]), toString(p["intent_type"]), intentID)
			if err != nil {
				failed["payments"]++
			} else {
				imported["payments"]++
			}
		}
	}

	// Import subscriptions
	if subs, ok := backup.Tables["subscriptions"]; ok {
		for _, sub := range subs {
			_, err := tx.Exec(`INSERT INTO subscriptions(id, username, plan, status, paid_amount, discount_code) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`,
				toInt64(sub["id"]), toString(sub["username"]), toString(sub["plan"]),
				toString(sub["status"]), toFloat64(sub["paid_amount"]), toString(sub["discount_code"]))
			if err != nil {
				failed["subscriptions"]++
			} else {
				imported["subscriptions"]++
			}
		}
	}

	// Import tickets
	if tickets, ok := backup.Tables["tickets"]; ok {
		for _, tk := range tickets {
			customerID := sql.NullInt64{}
			if v, exists := tk["customer_id"]; exists && v != nil {
				customerID = sql.NullInt64{Int64: toInt64(v), Valid: true}
			}
			_, err := tx.Exec(`INSERT INTO tickets(id, customer_id, username, subject, status, priority) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`,
				toInt64(tk["id"]), customerID, toString(tk["username"]),
				toString(tk["subject"]), toString(tk["status"]), toString(tk["priority"]))
			if err != nil {
				failed["tickets"]++
			} else {
				imported["tickets"]++
			}
		}
	}

	// Import wallet transactions
	if wtxs, ok := backup.Tables["wallet_transactions"]; ok {
		for _, wt := range wtxs {
			refID := sql.NullInt64{}
			if v, exists := wt["reference_id"]; exists && v != nil {
				refID = sql.NullInt64{Int64: toInt64(v), Valid: true}
			}
			_, err := tx.Exec(`INSERT INTO wallet_transactions(id, username, amount, type, description, actor, reference_type, reference_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`,
				toInt64(wt["id"]), toString(wt["username"]), toFloat64(wt["amount"]),
				toString(wt["type"]), toString(wt["description"]), toString(wt["actor"]),
				toString(wt["reference_type"]), refID)
			if err != nil {
				failed["wallet_transactions"]++
			} else {
				imported["wallet_transactions"]++
			}
		}
	}

	// Import vpn_configs (depends on nodes)
	if vpnConfigs, ok := backup.Tables["vpn_configs"]; ok {
		for _, vc := range vpnConfigs {
			var extraJSONStr sql.NullString
			if extra, exists := vc["extra_json"]; exists && extra != nil {
				if extraBytes, err := json.Marshal(extra); err == nil {
					extraJSONStr = sql.NullString{String: string(extraBytes), Valid: true}
				}
			}
			_, err := tx.Exec(`INSERT INTO vpn_configs(id, node_id, protocol, port, network, enabled, mtu, max_clients, enable_logs, conn_limit, extra_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (id) DO NOTHING`,
				toInt64(vc["id"]), toInt64(vc["node_id"]), toString(vc["protocol"]),
				toInt(vc["port"]), toString(vc["network"]), toBool(vc["enabled"]),
				toInt(vc["mtu"]), toInt(vc["max_clients"]), toBool(vc["enable_logs"]),
				toInt(vc["conn_limit"]), extraJSONStr)
			if err != nil {
				failed["vpn_configs"]++
			} else {
				imported["vpn_configs"]++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "imported": imported, "failed": failed})
}

// Backup helper functions for type conversion
func toInt64(v any) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	case json.Number:
		n, _ := val.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	}
	return 0
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	case json.Number:
		n, _ := val.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(val, 64)
		return n
	}
	return 0
}

func toInt(v any) int {
	return int(toInt64(v))
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int:
		return val != 0
	case string:
		return val == "true" || val == "1"
	}
	return false
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		if val {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%v", v)
}
