package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) payments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPayments(w, r)
	case http.MethodPost:
		s.createManualPayment(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) paymentByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/payments/")
	if !ok || action == "" || r.Method != http.MethodPost {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch action {
	case "approve":
		s.setPaymentStatus(w, r, id, "approved")
	case "reject":
		s.setPaymentStatus(w, r, id, "rejected")
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) walletByUsername(w http.ResponseWriter, r *http.Request) {
	username, action, ok := pathUsername(r.URL.Path, "/api/wallets/")
	if !ok || r.Method != http.MethodPost {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	var in struct {
		Amount      float64 `json:"amount"`
		Balance     float64 `json:"balance"`
		Description string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	switch action {
	case "adjust":
		if in.Amount == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "amount_required"})
			return
		}
		if err := s.applyWalletChange(username, in.Amount, "adjustment", in.Description, "admin"); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	case "set":
		if err := s.setWalletBalance(username, in.Balance, in.Description, "admin"); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "wallet."+action, "wallet", username, nil, map[string]any{"amount": in.Amount, "balance": in.Balance, "description": in.Description}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) listPayments(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`SELECT p.id,p.username,p.amount,p.method,p.status,COALESCE(p.intent_type,'wallet_topup'),p.intent_id,COALESCE(pl.name,''),p.created_at,p.updated_at FROM payments p LEFT JOIN plans pl ON pl.id=p.intent_id AND p.intent_type='plan_renewal' ORDER BY p.id DESC LIMIT 500`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	payments := []Payment{}
	for rows.Next() {
		var p Payment
		var intentID sql.NullInt64
		var created, updated sql.NullTime
		if err := rows.Scan(&p.ID, &p.Username, &p.Amount, &p.Method, &p.Status, &p.IntentType, &intentID, &p.IntentLabel, &created, &updated); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if p.IntentType == "" {
			p.IntentType = "wallet_topup"
		}
		if intentID.Valid {
			p.IntentID = &intentID.Int64
		}
		if created.Valid {
			p.CreatedAt = created.Time.Format(time.RFC3339)
		}
		if updated.Valid {
			p.UpdatedAt = updated.Time.Format(time.RFC3339)
		}
		payments = append(payments, p)
	}
	writeJSON(w, map[string]any{"ok": true, "payments": payments})
}

func (s *Server) createManualPayment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string  `json:"username"`
		Amount      float64 `json:"amount"`
		Method      string  `json:"method"`
		Receipt     string  `json:"receipt"`
		Description string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || in.Amount <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "username_amount_required"})
		return
	}
	if in.Method == "" {
		in.Method = "manual"
	}
	customerID := sql.NullInt64{}
	_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, in.Username).Scan(&customerID.Int64)
	if customerID.Int64 > 0 {
		customerID.Valid = true
	}
	res, err := s.DB.Exec(`INSERT INTO payments(customer_id,username,amount,method,receipt,status,intent_type,admin_note) VALUES($1,$2,$3,$4,$5,'approved','wallet_topup',$6)`, nullableInt(customerID), in.Username, in.Amount, in.Method, in.Receipt, in.Description)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	paymentID, _ := res.LastInsertId()
	if err := s.applyWalletChangeRef(in.Username, in.Amount, "topup", fmt.Sprintf("payment #%d approved", paymentID), "admin", "payment", &paymentID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment.created", "payment", strconv.FormatInt(paymentID, 10), nil, map[string]any{"username": in.Username, "amount": in.Amount}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": paymentID})
}

func (s *Server) setPaymentStatus(w http.ResponseWriter, r *http.Request, id int64, status string) {
	if status != "approved" && status != "rejected" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_status"})
		return
	}
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	var username, oldStatus, method, intentType string
	var amount float64
	var intentID sql.NullInt64
	if err := tx.QueryRow(`SELECT username,amount,status,method,COALESCE(intent_type,'wallet_topup'),intent_id FROM payments WHERE id=$1 LIMIT 1 FOR UPDATE`, id).Scan(&username, &amount, &oldStatus, &method, &intentType, &intentID); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if intentType == "" {
		intentType = "wallet_topup"
	}
	if oldStatus != status {
		if _, err := tx.Exec(`UPDATE payments SET status=$1 WHERE id=$2`, status, id); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if method != "wallet" && intentType != "reseller_topup" {
		if err := s.syncPaymentWalletStateTx(tx, id, username, amount, status); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if status == "approved" && intentType == "reseller_topup" {
		_, err = tx.Exec(`UPDATE admins SET credit = credit + $1 WHERE username=$2`, amount, username)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		desc := fmt.Sprintf("Manual Top-up approved (Payment #%d): +%.2f IRT", id, amount)
		_, err = tx.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES($1,$2, 'allocation', $3, $4)`, username, amount, desc, "admin")
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if status == "approved" && intentType == "plan_renewal" && intentID.Valid {
		if err := s.applyPlanIntentTx(tx, username, intentID.Int64, id, "admin"); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Auto-provision WireGuard peer when plan renewal payment is approved
	if status == "approved" && intentType == "plan_renewal" && intentID.Valid {
		var custID int64
		if s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&custID) == nil {
			s.autoProvisionWireGuardPeer(custID)
		}
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment.status_"+status, "payment", strconv.FormatInt(id, 10), map[string]any{"old_status": oldStatus}, map[string]any{"new_status": status}, clientIP(r))
	severity := "info"
	if status == "rejected" {
		severity = "warning"
	}
	s.createEvent("payment", severity, fmt.Sprintf("Payment %s #%d", status, id), fmt.Sprintf("Payment #%d for %s was %s", id, username, status), actor, username)
	writeJSON(w, map[string]any{"ok": true, "intent_type": intentType})
}

func (s *Server) applyPlanIntentTx(tx *sql.Tx, username string, planID, paymentID int64, actor string) error {
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM wallet_transactions WHERE reference_type='payment' AND reference_id=$1 AND type='purchase'`, paymentID).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	var customerID int64
	if err := tx.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err != nil {
		return err
	}
	var plan Plan
	var active bool
	var created sql.NullTime
	if err := tx.QueryRow(`SELECT id,name,data_gb,speed_mbps,duration_days,price,is_active,sort_order,created_at FROM plans WHERE id=$1 LIMIT 1`, planID).Scan(&plan.ID, &plan.Name, &plan.DataGB, &plan.SpeedMbps, &plan.DurationDays, &plan.Price, &active, &plan.SortOrder, &created); err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("plan_inactive")
	}
	var walletCredit float64
	_ = tx.QueryRow(`SELECT COALESCE(credit,0) FROM wallets WHERE username=$1`, username).Scan(&walletCredit)
	if plan.Price > 0 && walletCredit+0.0001 < plan.Price {
		return fmt.Errorf("insufficient_wallet")
	}
	if _, err := tx.Exec(`UPDATE customers SET plan_id=$1,status='active' WHERE id=$2 AND deleted_at IS NULL`, plan.ID, customerID); err != nil {
		return err
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=$1 AND attribute='Max-Data'`, username)
	if plan.DataGB > 0 {
		bytes := int64(math.Round(plan.DataGB * 1024 * 1024 * 1024))
		if _, err := tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,'Max-Data',':=',$2)`, username, bytes); err != nil {
			return err
		}
	}
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=$1 AND attribute='Mikrotik-Rate-Limit'`, username)
	if plan.SpeedMbps > 0 {
		if _, err := tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES($1,'Mikrotik-Rate-Limit',':=',$2)`, username, speedLimitValue(plan.SpeedMbps)); err != nil {
			return err
		}
	}
	var expires any
	if plan.DurationDays > 0 {
		expires = time.Now().AddDate(0, 0, plan.DurationDays)
	}
	if _, err := tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at,paid_amount) VALUES($1,$2,$3,$4,$5)`, customerID, username, plan.ID, expires, plan.Price); err != nil {
		return err
	}
	if plan.Price > 0 {
		desc := "plan activated: " + plan.Name
		if _, err := tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES($1,$2,$3) ON CONFLICT (username) DO UPDATE SET credit=wallets.credit+EXCLUDED.credit, customer_id=COALESCE(EXCLUDED.customer_id,wallets.customer_id)`, customerID, username, -plan.Price); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, customerID, username, -plan.Price, "purchase", desc, actor, "payment", paymentID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) syncPaymentWalletStateTx(tx *sql.Tx, paymentID int64, username string, amount float64, status string) error {
	desired := 0.0
	if status == "approved" {
		desired = amount
	}
	like := fmt.Sprintf("payment #%d %%", paymentID)
	var current float64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM wallet_transactions WHERE username=$1 AND type <> 'purchase' AND (reference_type='payment' AND reference_id=$2 OR description LIKE $3)`, username, paymentID, like).Scan(&current); err != nil {
		return err
	}
	delta := desired - current
	if math.Abs(delta) < 0.0001 {
		return nil
	}
	var customerID sql.NullInt64
	_ = tx.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	if _, err := tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES($1,$2,$3) ON CONFLICT (username) DO UPDATE SET credit=wallets.credit+EXCLUDED.credit, customer_id=COALESCE(EXCLUDED.customer_id,wallets.customer_id)`, nullableInt(customerID), username, delta); err != nil {
		return err
	}
	kind := "adjustment"
	desc := fmt.Sprintf("payment #%d wallet reconciliation: %s", paymentID, status)
	if delta > 0 && status == "approved" {
		kind = "topup"
		desc = fmt.Sprintf("payment #%d approved", paymentID)
	} else if delta < 0 && status != "approved" {
		desc = fmt.Sprintf("payment #%d approval reversed: %s", paymentID, status)
	}
	_, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, nullableInt(customerID), username, delta, kind, desc, "admin", "payment", paymentID)
	return err
}

func (s *Server) applyWalletChange(username string, amount float64, kind, description, actor string) error {
	return s.applyWalletChangeRef(username, amount, kind, description, actor, "", nil)
}

func (s *Server) applyWalletChangeRef(username string, amount float64, kind, description, actor, referenceType string, referenceID *int64) error {
	var customerID sql.NullInt64
	_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	_, err := s.DB.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES($1,$2,$3) ON CONFLICT (username) DO UPDATE SET credit=wallets.credit+EXCLUDED.credit, customer_id=COALESCE(EXCLUDED.customer_id,wallets.customer_id)`, nullableInt(customerID), username, amount)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, nullableInt(customerID), username, amount, kind, description, actor, referenceType, nullableInt64Ptr(referenceID))
	return err
}

func (s *Server) setWalletBalance(username string, balance float64, description, actor string) error {
	var customerID sql.NullInt64
	_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	var current float64
	_ = s.DB.QueryRow(`SELECT COALESCE(credit,0) FROM wallets WHERE username=$1`, username).Scan(&current)
	delta := balance - current
	_, err := s.DB.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES($1,$2,$3) ON CONFLICT (username) DO UPDATE SET credit=EXCLUDED.credit, customer_id=COALESCE(EXCLUDED.customer_id,wallets.customer_id)`, nullableInt(customerID), username, balance)
	if err != nil {
		return err
	}
	if math.Abs(delta) < 0.0001 {
		return nil
	}
	if description == "" {
		description = fmt.Sprintf("set wallet balance to %.2f", balance)
	}
	_, err = s.DB.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES($1,$2,$3,$4,$5,$6,$7,NULL)`, nullableInt(customerID), username, delta, "adjustment", description, actor, "manual")
	return err
}

func nullableInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func nullableInt64Ptr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func pathUsername(urlPath, prefix string) (string, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(urlPath, prefix), "/")
	if rest == "" || strings.Contains(rest, "..") {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	username := strings.TrimSpace(parts[0])
	if username == "" {
		return "", "", false
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	return username, action, true
}
