package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) backupExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	backup := map[string]any{
		"version":     1,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"tables":      map[string]any{},
	}
	tables := backup["tables"].(map[string]any)

	// Customers
	customers := []map[string]any{}
	rows, err := s.DB.Query(`SELECT id, username, COALESCE(display_name,''), status, plan_id, COALESCE(notes,''), COALESCE(sub_token,''), created_at FROM customers WHERE deleted_at IS NULL ORDER BY id`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var username, displayName, status, notes, subToken string
			var planID sql.NullInt64
			var created sql.NullTime
			if err := rows.Scan(&id, &username, &displayName, &status, &planID, &notes, &subToken, &created); err != nil {
				continue
			}
			c := map[string]any{"id": id, "username": username, "display_name": displayName, "status": status, "notes": notes, "sub_token": subToken}
			if planID.Valid {
				c["plan_id"] = planID.Int64
			}
			if created.Valid {
				c["created_at"] = created.Time.Format(time.RFC3339)
			}
			customers = append(customers, c)
		}
	}
	tables["customers"] = customers

	// Payments
	payments := []map[string]any{}
	pRows, err := s.DB.Query(`SELECT id, username, amount, method, status, COALESCE(intent_type,''), intent_id, created_at FROM payments ORDER BY id`)
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var id int64
			var username, method, status, intentType string
			var amount float64
			var intentID sql.NullInt64
			var created sql.NullTime
			if err := pRows.Scan(&id, &username, &amount, &method, &status, &intentType, &intentID, &created); err != nil {
				continue
			}
			p := map[string]any{"id": id, "username": username, "amount": amount, "method": method, "status": status, "intent_type": intentType}
			if intentID.Valid {
				p["intent_id"] = intentID.Int64
			}
			if created.Valid {
				p["created_at"] = created.Time.Format(time.RFC3339)
			}
			payments = append(payments, p)
		}
	}
	tables["payments"] = payments

	// Plans
	plans := []map[string]any{}
	plRows, err := s.DB.Query(`SELECT id, name, COALESCE(data_gb,0), COALESCE(speed_mbps,0), COALESCE(duration_days,0), COALESCE(price,0), COALESCE(billing_type,'fixed'), COALESCE(price_per_gb,0), COALESCE(price_per_day,0), disconnect_on_zero, is_active, COALESCE(sort_order,0), created_at FROM plans ORDER BY id`)
	if err == nil {
		defer plRows.Close()
		for plRows.Next() {
			var id int64
			var name, billingType string
			var dataGB, speedMbps, price, pricePerGB, pricePerDay float64
			var durationDays, sortOrder int
			var disconnectOnZero, isActive bool
			var created sql.NullTime
			if err := plRows.Scan(&id, &name, &dataGB, &speedMbps, &durationDays, &price, &billingType, &pricePerGB, &pricePerDay, &disconnectOnZero, &isActive, &sortOrder, &created); err != nil {
				continue
			}
			pl := map[string]any{"id": id, "name": name, "data_gb": dataGB, "speed_mbps": speedMbps, "duration_days": durationDays, "price": price, "billing_type": billingType, "price_per_gb": pricePerGB, "price_per_day": pricePerDay, "disconnect_on_zero": disconnectOnZero, "is_active": isActive, "sort_order": sortOrder}
			if created.Valid {
				pl["created_at"] = created.Time.Format(time.RFC3339)
			}
			plans = append(plans, pl)
		}
	}
	tables["plans"] = plans

	// Wallets
	wallets := []map[string]any{}
	wRows, err := s.DB.Query(`SELECT username, credit FROM wallets ORDER BY username`)
	if err == nil {
		defer wRows.Close()
		for wRows.Next() {
			var username string
			var credit float64
			if err := wRows.Scan(&username, &credit); err != nil {
				continue
			}
			wallets = append(wallets, map[string]any{"username": username, "credit": credit})
		}
	}
	tables["wallets"] = wallets

	// Nodes
	nodes := []map[string]any{}
	nRows, err := s.DB.Query(`SELECT id, name, public_ip, COALESCE(domain,''), status, created_at FROM nodes ORDER BY id`)
	if err == nil {
		defer nRows.Close()
		for nRows.Next() {
			var id int64
			var name, publicIP, domain, status string
			var created sql.NullTime
			if err := nRows.Scan(&id, &name, &publicIP, &domain, &status, &created); err != nil {
				continue
			}
			n := map[string]any{"id": id, "name": name, "public_ip": publicIP, "domain": domain, "status": status}
			if created.Valid {
				n["created_at"] = created.Time.Format(time.RFC3339)
			}
			nodes = append(nodes, n)
		}
	}
	tables["nodes"] = nodes

	// VPN Configs
	vpnConfigs := []map[string]any{}
	vcRows, err := s.DB.Query(`SELECT id, node_id, protocol, port, COALESCE(network,''), enabled, COALESCE(mtu,1500), COALESCE(max_clients,0), COALESCE(enable_logs,1), COALESCE(conn_limit,0), extra_json FROM vpn_configs ORDER BY id`)
	if err == nil {
		defer vcRows.Close()
		for vcRows.Next() {
			var id, nodeID int64
			var protocol, network string
			var port, mtu, maxClients, connLimit int
			var enabled, enableLogs bool
			var extraJSON sql.NullString
			if err := vcRows.Scan(&id, &nodeID, &protocol, &port, &network, &enabled, &mtu, &maxClients, &enableLogs, &connLimit, &extraJSON); err != nil {
				continue
			}
			vc := map[string]any{"id": id, "node_id": nodeID, "protocol": protocol, "port": port, "network": network, "enabled": enabled, "mtu": mtu, "max_clients": maxClients, "enable_logs": enableLogs, "conn_limit": connLimit}
			if extraJSON.Valid && extraJSON.String != "" {
				var extra map[string]any
				if json.Unmarshal([]byte(extraJSON.String), &extra) == nil {
					vc["extra_json"] = extra
				}
			}
			vpnConfigs = append(vpnConfigs, vc)
		}
	}
	tables["vpn_configs"] = vpnConfigs

	// Radcheck
	radcheck := []map[string]any{}
	rcRows, err := s.DB.Query(`SELECT id, username, attribute, op, value FROM radcheck ORDER BY id`)
	if err == nil {
		defer rcRows.Close()
		for rcRows.Next() {
			var id int64
			var username, attribute, op, value string
			if err := rcRows.Scan(&id, &username, &attribute, &op, &value); err != nil {
				continue
			}
			radcheck = append(radcheck, map[string]any{"id": id, "username": username, "attribute": attribute, "op": op, "value": value})
		}
	}
	tables["radcheck"] = radcheck

	// Subscriptions
	subscriptions := []map[string]any{}
	subRows, err := s.DB.Query(`SELECT id, username, COALESCE(plan,''), status, started_at, expires_at, COALESCE(paid_amount,0), COALESCE(discount_code,'') FROM subscriptions ORDER BY id`)
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var id int64
			var username, plan, status, discountCode string
			var paidAmount float64
			var startedAt, expiresAt sql.NullTime
			if err := subRows.Scan(&id, &username, &plan, &status, &startedAt, &expiresAt, &paidAmount, &discountCode); err != nil {
				continue
			}
			sub := map[string]any{"id": id, "username": username, "plan": plan, "status": status, "paid_amount": paidAmount, "discount_code": discountCode}
			if startedAt.Valid {
				sub["started_at"] = startedAt.Time.Format(time.RFC3339)
			}
			if expiresAt.Valid {
				sub["expires_at"] = expiresAt.Time.Format(time.RFC3339)
			}
			subscriptions = append(subscriptions, sub)
		}
	}
	tables["subscriptions"] = subscriptions

	// Tickets
	tickets := []map[string]any{}
	tRows, err := s.DB.Query(`SELECT id, customer_id, username, subject, status, priority, created_at FROM tickets WHERE deleted_at IS NULL ORDER BY id`)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var id int64
			var customerID sql.NullInt64
			var username, subject, status, priority string
			var created sql.NullTime
			if err := tRows.Scan(&id, &customerID, &username, &subject, &status, &priority, &created); err != nil {
				continue
			}
			tk := map[string]any{"id": id, "username": username, "subject": subject, "status": status, "priority": priority}
			if customerID.Valid {
				tk["customer_id"] = customerID.Int64
			}
			if created.Valid {
				tk["created_at"] = created.Time.Format(time.RFC3339)
			}
			tickets = append(tickets, tk)
		}
	}
	tables["tickets"] = tickets

	// Wallet Transactions
	walletTx := []map[string]any{}
	wtRows, err := s.DB.Query(`SELECT id, username, amount, type, description, actor, COALESCE(reference_type,''), reference_id, created_at FROM wallet_transactions ORDER BY id`)
	if err == nil {
		defer wtRows.Close()
		for wtRows.Next() {
			var id int64
			var amount float64
			var username, ttype, description, actor, refType string
			var refID sql.NullInt64
			var created sql.NullTime
			if err := wtRows.Scan(&id, &username, &amount, &ttype, &description, &actor, &refType, &refID, &created); err != nil {
				continue
			}
			wt := map[string]any{"id": id, "username": username, "amount": amount, "type": ttype, "description": description, "actor": actor, "reference_type": refType}
			if refID.Valid {
				wt["reference_id"] = refID.Int64
			}
			if created.Valid {
				wt["created_at"] = created.Time.Format(time.RFC3339)
			}
			walletTx = append(walletTx, wt)
		}
	}
	tables["wallet_transactions"] = walletTx

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="panel-backup.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(backup)
}
