package api

import (
	"KorisPanel/panel/internal/grpcclient"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) resetCustomerPassword(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if len(in.Password) < 4 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "password_too_short"})
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
	res, err := s.DB.Exec(`UPDATE radcheck SET value=$1 WHERE username=$2 AND attribute IN('Cleartext-Password','User-Password')`, in.Password, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_, err = s.DB.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,'Cleartext-Password',':=',$2)`, username, in.Password)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	// Sync updated password to knode instances via gRPC
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
				log.Printf("[knode] SyncUser failed after password reset for %q: %v", username, err)
			}
		}()
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) resetCustomerTraffic(w http.ResponseWriter, r *http.Request, id int64) {
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Reset traffic by archiving old radacct records (set stop time) so usage counters restart
	result, err := s.DB.Exec(`UPDATE radacct SET acctstoptime=COALESCE(acctstoptime, NOW()), acctterminatecause=COALESCE(acctterminatecause, 'Admin-Reset') WHERE username=$1`, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	affected, _ := result.RowsAffected()

	// If customer was in 'limited' status, re-enable them
	_, _ = s.DB.Exec(`UPDATE customers SET status='active' WHERE username=$1 AND status='limited' AND deleted_at IS NULL`, username)

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "customer.traffic_reset", "customer", strconv.FormatInt(id, 10), nil, map[string]any{"username": username, "sessions_reset": affected}, clientIP(r))
	s.createEvent("customer", "info", fmt.Sprintf("Traffic reset: %s", username), fmt.Sprintf("Admin %s reset traffic counters for %s (%d sessions archived)", actor, username, affected), actor, username)

	// Also reset traffic on knode instances via gRPC
	if s.GRPCPool != nil && s.GRPCStore != nil {
		go func() {
			if err := grpcclient.ResetUserTraffic(context.Background(), username, s.GRPCPool, s.GRPCStore, s.TrafficCollector); err != nil {
				log.Printf("[knode] ResetUserTraffic failed for %q: %v", username, err)
			}
		}()
	}
	// Re-sync user state (re-enabled after traffic reset)
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
				log.Printf("[knode] SyncUser failed after traffic reset for %q: %v", username, err)
			}
		}()
	}

	writeJSON(w, map[string]any{"ok": true, "sessions_reset": affected})
}

func (s *Server) renewCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.PlanID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_required"})
		return
	}

	var username, createdBy string
	if err := s.DB.QueryRow(`SELECT username, COALESCE(created_by,'') FROM customers WHERE id=$1 AND deleted_at IS NULL LIMIT 1`, id).Scan(&username, &createdBy); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Resellers can only renew their own customers
	actor2, role2, _ := s.currentAdmin(r)
	if role2 == "reseller" && createdBy != actor2 {
		writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "not_your_customer"})
		return
	}

	var plan Plan
	var active bool
	if err := s.DB.QueryRow(`SELECT id,name,data_gb,speed_mbps,duration_days,price,is_active,sort_order,created_at FROM plans WHERE id=$1 LIMIT 1`, in.PlanID).Scan(&plan.ID, &plan.Name, &plan.DataGB, &plan.SpeedMbps, &plan.DurationDays, &plan.Price, &active, &plan.SortOrder, new(sql.NullTime)); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "plan_not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	plan.IsActive = active
	if !active {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_inactive"})
		return
	}

	actor, role, ok := s.currentAdmin(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	if role == "reseller" {
		var resellerCredit float64
		if err := tx.QueryRow(`SELECT COALESCE(credit,0) FROM admins WHERE username=$1 FOR UPDATE`, actor).Scan(&resellerCredit); err != nil && err != sql.ErrNoRows {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if plan.Price > 0 && resellerCredit < plan.Price {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "insufficient_reseller_credit", "credit": resellerCredit, "required": plan.Price})
			return
		}
	} else {
		var walletCredit float64
		if err := tx.QueryRow(`SELECT COALESCE(credit,0) FROM wallets WHERE username=$1 FOR UPDATE`, username).Scan(&walletCredit); err != nil && err != sql.ErrNoRows {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if plan.Price > 0 && walletCredit+0.0001 < plan.Price {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "insufficient_wallet", "wallet": walletCredit, "required": plan.Price})
			return
		}
	}

	if _, err := tx.Exec(`UPDATE customers SET plan_id=$1,status='active' WHERE id=$2 AND deleted_at IS NULL`, plan.ID, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=$1 AND attribute='Max-Data'`, username)
	if plan.DataGB > 0 {
		bytes := int64(math.Round(plan.DataGB * 1024 * 1024 * 1024))
		if _, err := tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,'Max-Data',':=',$2)`, username, bytes); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=$1 AND attribute='Mikrotik-Rate-Limit'`, username)
	if plan.SpeedMbps > 0 {
		if _, err := tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES($1,'Mikrotik-Rate-Limit',':=',$2)`, username, speedLimitValue(plan.SpeedMbps)); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}

	var expires any
	if plan.DurationDays > 0 {
		expires = time.Now().AddDate(0, 0, plan.DurationDays)
	}
	if _, err := tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at,paid_amount) VALUES($1,$2,$3,$4,$5)`, id, username, plan.ID, expires, plan.Price); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if plan.Price > 0 {
		desc := "plan activated: " + plan.Name
		if role == "reseller" {
			_, err = tx.Exec(`UPDATE admins SET credit = credit - $1 WHERE username=$2`, plan.Price, actor)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			_, _ = tx.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES($1,$2, 'deduction', $3, $4)`, actor, -plan.Price, "Renewed plan for "+username, actor)
			paymentRes, err := tx.Exec(`INSERT INTO payments(customer_id,username,amount,method,status,admin_note) VALUES($1,$2,$3,'reseller','approved',$4)`, id, username, plan.Price, desc+" (reseller: "+actor+")")
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			paymentID, _ := paymentRes.LastInsertId()
			if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, username, 0.0, "purchase", desc+" (reseller paid)", actor, "payment", paymentID); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		} else {
			paymentRes, err := tx.Exec(`INSERT INTO payments(customer_id,username,amount,method,status,admin_note) VALUES($1,$2,$3,'wallet','approved',$4)`, id, username, plan.Price, desc)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			paymentID, _ := paymentRes.LastInsertId()
			_, err = tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES($1,$2,$3) ON CONFLICT (username) DO UPDATE SET credit=wallets.credit+EXCLUDED.credit, customer_id=COALESCE(EXCLUDED.customer_id,wallets.customer_id)`, id, username, -plan.Price)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, username, -plan.Price, "purchase", desc, "admin", "payment", paymentID); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Auto-provision WireGuard peer on subscription renewal/activation
	s.autoProvisionWireGuardPeer(id)
	// Sync updated user limits to knode instances via gRPC
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
				log.Printf("[knode] SyncUser failed after customer renew for %q: %v", username, err)
			}
		}()
	}
	actor, _, _ = s.currentAdmin(r)
	s.createEvent("plan", "info", fmt.Sprintf("Plan applied: %s", plan.Name), fmt.Sprintf("Admin %s applied plan %s to %s", actor, plan.Name, username), actor, username)
	writeJSON(w, map[string]any{"ok": true, "plan": plan, "wallet_deducted": plan.Price})
}

// switchCustomerPlan cancels the current subscription with a pro-rated refund
// and applies a new plan. POST /api/customers/{id}/switch-plan
// Body: { "plan_id": N }
// Response: { "ok": true, "refund_amount": X.XX, "new_plan": "..." }
func (s *Server) switchCustomerPlan(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.PlanID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_required"})
		return
	}

	// Get customer info
	var username string
	if err := s.DB.QueryRow(`SELECT username FROM customers WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&username); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Get current active subscription
	var subID int64
	var currentPlanID int64
	var paidAmount float64
	var startDate time.Time
	var expiresAt sql.NullTime
	err := s.DB.QueryRow(`
		SELECT s.id, s.plan_id, COALESCE(s.paid_amount, 0), s.started_at, s.expires_at
		FROM subscriptions s
		WHERE s.customer_id = $1 AND s.status = 'active'
		ORDER BY s.id DESC LIMIT 1`, id).Scan(&subID, &currentPlanID, &paidAmount, &startDate, &expiresAt)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no_active_subscription"})
		return
	}

	// Get current plan details for refund calculation
	var currentDataGB float64
	var currentDurationDays int
	s.DB.QueryRow(`SELECT COALESCE(data_gb, 0), COALESCE(duration_days, 0) FROM plans WHERE id=$1`, currentPlanID).Scan(&currentDataGB, &currentDurationDays)

	// Get customer's current data usage since subscription started
	var totalUsageBytes int64
	s.DB.QueryRow(`SELECT COALESCE(SUM(acctinputoctets + acctoutputoctets), 0) FROM radacct WHERE username=$1 AND acctstarttime >= $2`, username, startDate).Scan(&totalUsageBytes)
	usedGB := float64(totalUsageBytes) / (1024 * 1024 * 1024)

	// Calculate pro-rated refund
	refundAmount := calculateProRatedRefund(paidAmount, currentDataGB, currentDurationDays, usedGB, startDate, expiresAt)

	// Get the new plan
	var newPlan Plan
	var active bool
	if err := s.DB.QueryRow(`SELECT id, name, data_gb, speed_mbps, duration_days, price, is_active FROM plans WHERE id=$1`, in.PlanID).Scan(
		&newPlan.ID, &newPlan.Name, &newPlan.DataGB, &newPlan.SpeedMbps, &newPlan.DurationDays, &newPlan.Price, &active); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "plan_not_found"})
		return
	}
	if !active {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_inactive"})
		return
	}

	// Start transaction
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	// 1. Cancel current subscription
	if _, err := tx.Exec(`UPDATE subscriptions SET status='cancelled' WHERE id=$1`, subID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// 2. Credit refund to wallet
	if refundAmount > 0 {
		if _, err := tx.Exec(`INSERT INTO wallets(customer_id, username, credit) VALUES($1,$2,$3) ON CONFLICT (username) DO UPDATE SET credit = wallets.credit + EXCLUDED.credit`, id, username, refundAmount); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		actor, _, _ := s.currentAdmin(r)
		refundPct := 0.0
		if paidAmount > 0 {
			refundPct = (refundAmount / paidAmount) * 100
		}
		if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id, username, amount, type, description, actor) VALUES($1,$2,$3,$4,$5,$6)`,
			id, username, refundAmount, "refund", fmt.Sprintf("Pro-rated refund for plan switch (%.1f%% unused)", refundPct), actor); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// 3. Apply the new plan (outside tx since applyPlanDirectly has its own operations)
	s.applyPlanDirectly(id, username, in.PlanID, r)

	writeJSON(w, map[string]any{
		"ok":            true,
		"refund_amount": math.Round(refundAmount*100) / 100,
		"new_plan":      newPlan.Name,
	})
}

// calculateProRatedRefund computes the refund based on minimum of time% and data% remaining.
// - Unlimited data (dataGB=0): use time only
// - Unlimited time (durationDays=0): use data only
// - Both unlimited: no refund
// - Free plan (paidAmount=0): no refund
func calculateProRatedRefund(paidAmount, dataGB float64, durationDays int, usedGB float64, startDate time.Time, expiresAt sql.NullTime) float64 {
	if paidAmount <= 0 {
		return 0
	}

	timeRemainingPct := -1.0 // -1 means "not applicable" (unlimited)
	dataRemainingPct := -1.0

	// Calculate time remaining %
	if durationDays > 0 && expiresAt.Valid {
		totalDuration := expiresAt.Time.Sub(startDate).Hours() / 24
		elapsed := time.Since(startDate).Hours() / 24
		if totalDuration > 0 {
			timeRemainingPct = math.Max(0, (totalDuration-elapsed)/totalDuration)
		}
	}

	// Calculate data remaining %
	if dataGB > 0 {
		dataRemainingPct = math.Max(0, (dataGB-usedGB)/dataGB)
	}

	// Determine which percentage to use
	var refundPct float64
	if timeRemainingPct < 0 && dataRemainingPct < 0 {
		// Both unlimited — no refund
		return 0
	} else if timeRemainingPct < 0 {
		// Unlimited time — use data only
		refundPct = dataRemainingPct
	} else if dataRemainingPct < 0 {
		// Unlimited data — use time only
		refundPct = timeRemainingPct
	} else {
		// Both limited — use minimum (protects against abuse)
		refundPct = math.Min(timeRemainingPct, dataRemainingPct)
	}

	return paidAmount * refundPct
}

// applyPlanDirectly applies a plan to a customer without wallet deduction (used after switch-plan refund).
func (s *Server) applyPlanDirectly(customerID int64, username string, planID int64, r *http.Request) {
	var plan Plan
	var active bool
	if err := s.DB.QueryRow(`SELECT id, name, data_gb, speed_mbps, duration_days, price, is_active FROM plans WHERE id=$1`, planID).Scan(
		&plan.ID, &plan.Name, &plan.DataGB, &plan.SpeedMbps, &plan.DurationDays, &plan.Price, &active); err != nil {
		return
	}

	// Update customer's plan
	_, _ = s.DB.Exec(`UPDATE customers SET plan_id=$1, status='active' WHERE id=$2 AND deleted_at IS NULL`, planID, customerID)

	// Update RADIUS attributes
	_, _ = s.DB.Exec(`DELETE FROM radcheck WHERE username=$1 AND attribute='Max-Data'`, username)
	if plan.DataGB > 0 {
		bytes := int64(math.Round(plan.DataGB * 1024 * 1024 * 1024))
		_, _ = s.DB.Exec(`INSERT INTO radcheck(username, attribute, op, value) VALUES($1,'Max-Data',':=',$2)`, username, bytes)
	}
	_, _ = s.DB.Exec(`DELETE FROM radreply WHERE username=$1 AND attribute='Mikrotik-Rate-Limit'`, username)
	if plan.SpeedMbps > 0 {
		_, _ = s.DB.Exec(`INSERT INTO radreply(username, attribute, op, value) VALUES($1,'Mikrotik-Rate-Limit',':=',$2)`, username, speedLimitValue(plan.SpeedMbps))
	}

	// Create new subscription (paid_amount=0 since refund was already credited)
	var expires any
	if plan.DurationDays > 0 {
		expires = time.Now().AddDate(0, 0, plan.DurationDays)
	}
	_, _ = s.DB.Exec(`INSERT INTO subscriptions(customer_id, username, plan_id, expires_at, paid_amount) VALUES($1,$2,$3,$4,0)`, customerID, username, planID, expires)

	// Reset traffic counters for new plan
	_, _ = s.DB.Exec(`UPDATE radacct SET acctinputoctets=0, acctoutputoctets=0 WHERE username=$1 AND acctstoptime IS NULL`, username)

	// Auto-provision WireGuard
	s.autoProvisionWireGuardPeer(customerID)

	// Sync updated user limits to knode instances via gRPC
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
				log.Printf("[knode] SyncUser failed after plan switch for %q: %v", username, err)
			}
		}()
	}

	actor, _, _ := s.currentAdmin(r)
	s.createEvent("plan", "info", fmt.Sprintf("Plan switched to: %s", plan.Name), fmt.Sprintf("Admin %s switched plan for %s (with refund)", actor, username), actor, username)
}
