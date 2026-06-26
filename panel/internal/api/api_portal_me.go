package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) portalMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var id int64
	var displayName, plan string
	var status string
	var credit float64
	var created sql.NullTime
	var subToken string
	err := s.DB.QueryRow(`SELECT c.id,COALESCE(c.display_name,''),c.status,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at,COALESCE(c.sub_token,'')
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE c.username=$1 AND c.deleted_at IS NULL LIMIT 1`, username).Scan(&id, &displayName, &status, &plan, &credit, &created, &subToken)
	if err == sql.ErrNoRows {
		writeJSON(w, map[string]any{"ok": true, "customer": map[string]any{"username": username, "status": "active"}})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	customer := map[string]any{
		"id":           id,
		"username":     username,
		"display_name": displayName,
		"status":       status,
		"plan":         plan,
		"credit":       credit,
		"sub_token":    subToken,
	}
	if created.Valid {
		customer["created_at"] = created.Time.Format(time.RFC3339)
	}

	var subPlan, subStatus string
	var expires sql.NullTime
	if err := s.DB.QueryRow(`SELECT COALESCE(p.name,''),s.status,s.expires_at
		FROM subscriptions s
		LEFT JOIN plans p ON p.id=s.plan_id
		WHERE s.username=$1 ORDER BY s.id DESC LIMIT 1`, username).Scan(&subPlan, &subStatus, &expires); err == nil {
		sub := map[string]any{"plan": subPlan, "status": subStatus}
		if expires.Valid {
			sub["expires_at"] = expires.Time.Format(time.RFC3339)
		}
		customer["subscription"] = sub
	}

	var maxData string
	if err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=$1 AND attribute='Max-Data' ORDER BY id DESC LIMIT 1`, username).Scan(&maxData); err == nil {
		customer["max_data_bytes"] = maxData
	}

	// Resolve billing mode
	billingEnabled := true // default for admin-created users
	var customerBillingMode sql.NullString
	_ = s.DB.QueryRow(`SELECT billing_mode FROM customers WHERE username=$1`, username).Scan(&customerBillingMode)

	if customerBillingMode.Valid && customerBillingMode.String != "" {
		billingEnabled = customerBillingMode.String == "self_service"
	} else {
		// Check reseller default
		var resellerBillingMode string
		err := s.DB.QueryRow(`SELECT COALESCE(a.billing_mode, 'manual') FROM customers c INNER JOIN admins a ON a.username = c.created_by AND a.role='reseller' WHERE c.username=$1`, username).Scan(&resellerBillingMode)
		if err == nil {
			billingEnabled = resellerBillingMode == "self_service"
		}
		// If no reseller (admin-created), billing is always enabled
	}
	customer["billing_enabled"] = billingEnabled

	writeJSON(w, map[string]any{"ok": true, "customer": customer})
}

func (s *Server) radiusRows(table, username string) ([]RadiusCheck, error) {
	if table != "radcheck" && table != "radreply" {
		return nil, fmt.Errorf("invalid_radius_table")
	}
	rows, err := s.DB.Query(`SELECT id,username,attribute,op,value FROM `+table+` WHERE username=$1 ORDER BY id ASC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RadiusCheck{}
	for rows.Next() {
		var row RadiusCheck
		if err := rows.Scan(&row.ID, &row.Username, &row.Attribute, &row.Op, &row.Value); err != nil {
			return out, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
