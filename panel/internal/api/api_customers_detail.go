package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) archiveCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	actor, _, _ := s.currentAdmin(r)
	var username string
	var deletedAt sql.NullTime
	if err := s.DB.QueryRow(`SELECT username,deleted_at FROM customers WHERE id=$1 LIMIT 1`, id).Scan(&username, &deletedAt); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if deletedAt.Valid {
		writeJSON(w, map[string]any{"ok": true, "already_deleted": true})
		return
	}
	radChecks, _ := s.radiusRows("radcheck", username)
	radReplies, _ := s.radiusRows("radreply", username)
	payloadBytes, _ := json.Marshal(map[string]any{
		"customer_id": id,
		"username":    username,
		"radcheck":    radChecks,
		"radreply":    radReplies,
		"archived_at": time.Now().UTC(),
	})
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO deleted_archive(type,name,archive_key,payload,created_by) VALUES('customer',$1,$2,$3,$4)`, username, strconv.FormatInt(id, 10), string(payloadBytes), actor); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err := tx.Exec(`UPDATE customers SET status='deleted',deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=$1`, username)
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=$1`, username)
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.Cache != nil {
		s.Cache.InvalidatePrefix("stats:")
	}
	// Sync user deletion to knode instances via gRPC (will send enabled=false)
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
				log.Printf("[knode] SyncUser failed after customer archive for %q: %v", username, err)
			}
		}()
	}
	s.logAudit(actor, "customer.archived", "customer", strconv.FormatInt(id, 10), nil, map[string]any{"username": username}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) restoreCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	var username string
	if err := s.DB.QueryRow(`SELECT username FROM customers WHERE id=$1 LIMIT 1`, id).Scan(&username); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var archiveID int64
	var payload string
	_ = s.DB.QueryRow(`SELECT id,COALESCE(payload,'') FROM deleted_archive WHERE type='customer' AND archive_key=$1 ORDER BY id DESC LIMIT 1`, strconv.FormatInt(id, 10)).Scan(&archiveID, &payload)
	var archived struct {
		RadCheck []RadiusCheck `json:"radcheck"`
		RadReply []RadiusCheck `json:"radreply"`
	}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &archived)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE customers SET status='active',deleted_at=NULL WHERE id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=$1`, username)
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=$1`, username)
	for _, row := range archived.RadCheck {
		_, _ = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,$2,$3,$4)`, username, row.Attribute, row.Op, row.Value)
	}
	for _, row := range archived.RadReply {
		_, _ = tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES($1,$2,$3,$4)`, username, row.Attribute, row.Op, row.Value)
	}
	if archiveID > 0 {
		_, _ = tx.Exec(`UPDATE deleted_archive SET restored_at=NOW() WHERE id=$1`, archiveID)
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.Cache != nil {
		s.Cache.InvalidatePrefix("stats:")
	}
	// Sync restored user to knode instances via gRPC (re-enable access)
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
				log.Printf("[knode] SyncUser failed after customer restore for %q: %v", username, err)
			}
		}()
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "customer.restored", "customer", strconv.FormatInt(id, 10), nil, map[string]any{"username": username}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) getCustomerDetail(w http.ResponseWriter, r *http.Request, id int64) {
	var c CustomerDetail
	var planID sql.NullInt64
	var created sql.NullTime
	err := s.DB.QueryRow(`SELECT c.id,c.username,COALESCE(c.display_name,''),c.status,c.plan_id,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at,COALESCE(c.notes,''),COALESCE(c.sub_token,''),COALESCE(c.avatar, (SELECT a.avatar FROM admins a WHERE a.username=c.created_by AND a.role='reseller' LIMIT 1), ''),COALESCE(c.created_by,'')
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE c.id=$1 AND c.deleted_at IS NULL LIMIT 1`, id).Scan(&c.ID, &c.Username, &c.DisplayName, &c.Status, &planID, &c.Plan, &c.Credit, &created, &c.Notes, &c.SubToken, &c.Avatar, &c.CreatedBy)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if planID.Valid {
		c.PlanID = &planID.Int64
	}
	if created.Valid {
		c.CreatedAt = created.Time.Format(time.RFC3339)
	}

	rows, err := s.DB.Query(`SELECT id,username,attribute,op,value FROM radcheck WHERE username=$1 ORDER BY id ASC`, c.Username)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var rc RadiusCheck
			if err := rows.Scan(&rc.ID, &rc.Username, &rc.Attribute, &rc.Op, &rc.Value); err == nil {
				if strings.Contains(strings.ToLower(rc.Attribute), "password") {
					rc.Value = "••••••••"
				}
				c.RadiusChecks = append(c.RadiusChecks, rc)
			}
		}
	}
	replyRows, err := s.DB.Query(`SELECT id,username,attribute,op,value FROM radreply WHERE username=$1 ORDER BY id ASC`, c.Username)
	if err == nil {
		defer replyRows.Close()
		for replyRows.Next() {
			var rr RadiusCheck
			if err := replyRows.Scan(&rr.ID, &rr.Username, &rr.Attribute, &rr.Op, &rr.Value); err == nil {
				c.RadiusReplies = append(c.RadiusReplies, rr)
			}
		}
	}

	var subID int64
	var subPlan, subStatus string
	var started, expires sql.NullTime
	if err := s.DB.QueryRow(`SELECT s.id,COALESCE(p.name,''),s.status,s.started_at,s.expires_at
		FROM subscriptions s
		LEFT JOIN plans p ON p.id=s.plan_id
		WHERE s.username=$1 ORDER BY s.id DESC LIMIT 1`, c.Username).Scan(&subID, &subPlan, &subStatus, &started, &expires); err == nil {
		sub := map[string]any{"id": subID, "plan": subPlan, "status": subStatus}
		if started.Valid {
			sub["started_at"] = started.Time.Format(time.RFC3339)
		}
		if expires.Valid {
			sub["expires_at"] = expires.Time.Format(time.RFC3339)
		}
		c.Subscription = sub
	}

	subRows, err := s.DB.Query(`SELECT s.id,s.username,COALESCE(p.name,''),s.status,s.started_at,s.expires_at,s.paid_amount,COALESCE(s.discount_code,'')
		FROM subscriptions s
		LEFT JOIN plans p ON p.id=s.plan_id
		WHERE s.username=$1 ORDER BY s.id DESC LIMIT 100`, c.Username)
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var item SubscriptionHistory
			var startedAt, expiresAt sql.NullTime
			if err := subRows.Scan(&item.ID, &item.Username, &item.Plan, &item.Status, &startedAt, &expiresAt, &item.PaidAmount, &item.DiscountCode); err == nil {
				if startedAt.Valid {
					item.StartedAt = startedAt.Time.Format(time.RFC3339)
				}
				if expiresAt.Valid {
					item.ExpiresAt = expiresAt.Time.Format(time.RFC3339)
				}
				c.Subscriptions = append(c.Subscriptions, item)
			}
		}
	}

	txRows, err := s.DB.Query(`SELECT id,username,amount,type,description,actor,COALESCE(reference_type,''),reference_id,created_at FROM wallet_transactions WHERE username=$1 ORDER BY id DESC LIMIT 100`, c.Username)
	if err == nil {
		defer txRows.Close()
		for txRows.Next() {
			var item WalletTransaction
			var refID sql.NullInt64
			var createdAt sql.NullTime
			if err := txRows.Scan(&item.ID, &item.Username, &item.Amount, &item.Type, &item.Description, &item.Actor, &item.ReferenceType, &refID, &createdAt); err == nil {
				if refID.Valid {
					item.ReferenceID = &refID.Int64
				}
				if createdAt.Valid {
					item.CreatedAt = createdAt.Time.Format(time.RFC3339)
				}
				c.WalletTransactions = append(c.WalletTransactions, item)
			}
		}
	}

	// Include billing_mode for reseller-created users
	var billingMode sql.NullString
	_ = s.DB.QueryRow(`SELECT billing_mode FROM customers WHERE id=$1`, id).Scan(&billingMode)

	resp := map[string]any{"ok": true, "customer": c}
	if billingMode.Valid {
		resp["billing_mode"] = billingMode.String
	} else {
		resp["billing_mode"] = ""
	}
	writeJSON(w, resp)
}

func (s *Server) updateCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		DisplayName *string  `json:"display_name"`
		Status      *string  `json:"status"`
		PlanID      *int64   `json:"plan_id"`
		Notes       *string  `json:"notes"`
		DataGB      *float64 `json:"data_gb"`
		SpeedMbps   *float64 `json:"speed_mbps"`
		Days        *int     `json:"days"`
		IPLimit     *int     `json:"ip_limit"`
		Avatar      *string  `json:"avatar"`
		BillingMode *string  `json:"billing_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Reseller restrictions: only allow status, display_name, notes, billing_mode edits
	_, editRole, _ := s.currentAdmin(r)
	if editRole == "reseller" {
		if in.DataGB != nil || in.SpeedMbps != nil || in.Days != nil || in.PlanID != nil || in.IPLimit != nil || in.Avatar != nil {
			writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "reseller_edit_restricted"})
			return
		}
	}

	sets := []string{}
	args := []any{}
	if in.DisplayName != nil {
		displayName := strings.TrimSpace(*in.DisplayName)
		sets = append(sets, "display_name=$1")
		args = append(args, displayName)
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if !validCustomerStatus(status) {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_status"})
			return
		}
		sets = append(sets, "status=$1")
		args = append(args, status)
	}
	if in.PlanID != nil {
		sets = append(sets, "plan_id=$1")
		if *in.PlanID == 0 {
			args = append(args, nil)
		} else {
			args = append(args, *in.PlanID)
		}
	}
	if in.Notes != nil {
		sets = append(sets, "notes=$1")
		args = append(args, strings.TrimSpace(*in.Notes))
	}
	if in.Avatar != nil {
		sets = append(sets, "avatar=$1")
		if *in.Avatar == "" {
			args = append(args, nil)
		} else {
			args = append(args, *in.Avatar)
		}
	}
	if in.BillingMode != nil {
		bm := strings.TrimSpace(*in.BillingMode)
		if bm != "" && bm != "manual" && bm != "self_service" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_billing_mode"})
			return
		}
		sets = append(sets, "billing_mode=$1")
		if bm == "" {
			args = append(args, nil) // inherit from reseller
		} else {
			args = append(args, bm)
		}
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := s.DB.Exec(`UPDATE customers SET `+strings.Join(sets, ",")+` WHERE id=$1 AND deleted_at IS NULL`, args...); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if in.DataGB != nil {
		if *in.DataGB < 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_limits"})
			return
		}
		if *in.DataGB == 0 {
			_, _ = s.DB.Exec(`DELETE FROM radcheck WHERE username=$1 AND attribute='Max-Data'`, username)
		} else {
			bytes := int64(math.Round(*in.DataGB * 1024 * 1024 * 1024))
			if err := s.upsertRadCheck(username, "Max-Data", strconv.FormatInt(bytes, 10)); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
	}
	if in.SpeedMbps != nil {
		if *in.SpeedMbps < 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_limits"})
			return
		}
		if *in.SpeedMbps == 0 {
			_, _ = s.DB.Exec(`DELETE FROM radreply WHERE username=$1 AND attribute='Mikrotik-Rate-Limit'`, username)
		} else if err := s.upsertRadReply(username, "Mikrotik-Rate-Limit", speedLimitValue(*in.SpeedMbps)); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if in.Days != nil && *in.Days > 0 {
		var planID any
		if in.PlanID != nil && *in.PlanID > 0 {
			planID = *in.PlanID
		}
		expires := time.Now().AddDate(0, 0, *in.Days)
		_, _ = s.DB.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at) VALUES($1,$2,$3,$4)`, id, username, planID, expires)
	}
	if in.IPLimit != nil {
		if *in.IPLimit <= 0 {
			_, _ = s.DB.Exec(`DELETE FROM radcheck WHERE username=$1 AND attribute='Simultaneous-Use'`, username)
		} else {
			_ = s.upsertRadCheck(username, "Simultaneous-Use", strconv.Itoa(*in.IPLimit))
		}
	}

	if in.Status != nil && *in.Status != "active" {
		// Only disconnect if status is actually changing (avoid unnecessary disconnection
		// when updating other fields on an already-disabled/limited customer)
		var currentStatus string
		_ = s.DB.QueryRow(`SELECT status FROM customers WHERE id=$1 LIMIT 1`, id).Scan(&currentStatus)
		if currentStatus != *in.Status {
			s.disconnectCustomerSessions(username)
		}
	}

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("stats:")
	}
	// Sync user to knode instances via gRPC after update
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
				log.Printf("[knode] SyncUser failed after customer update for %q: %v", username, err)
			}
		}()
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "customer.updated", "customer", strconv.FormatInt(id, 10), nil, map[string]any{"username": username}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) setCustomerStatus(w http.ResponseWriter, id int64, status string) {
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if _, err := s.DB.Exec(`UPDATE customers SET status=$1 WHERE id=$2 AND deleted_at IS NULL`, status, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if status != "active" {
		s.disconnectCustomerSessions(username)
	}

	// Sync user status to knode instances via gRPC (enabled=false for non-active)
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
				log.Printf("[knode] SyncUser failed after status change for %q: %v", username, err)
			}
		}()
	}

	writeJSON(w, map[string]any{"ok": true})
}
