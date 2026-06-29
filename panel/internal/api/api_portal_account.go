package api

import (
	"KorisPanel/panel/internal/auth"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) ikev2MobileConfig(username string, r *http.Request, nodeID int64) string {
	host, _, _, _ := s.openVPNEndpointNode(r, nodeID)
	if host == "" {
		host = r.Host
	}
	uuidPayload := strings.ToLower(auth.RandomToken(8) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(12))
	uuidProfile := strings.ToLower(auth.RandomToken(8) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(12))
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadDescription</key>
			<string>Configures IKEv2 VPN</string>
			<key>PayloadDisplayName</key>
			<string>Koris IKEv2</string>
			<key>PayloadIdentifier</key>
			<string>koris.vpn.ikev2.%s</string>
			<key>PayloadType</key>
			<string>com.apple.vpn.managed</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>UserDefinedName</key>
			<string>Koris IKEv2</string>
			<key>VPNType</key>
			<string>IKEv2</string>
			<key>IPv4</key>
			<dict>
				<key>OverridePrimary</key>
				<integer>1</integer>
			</dict>
			<key>AuthenticationMethod</key>
			<string>UserName</string>
			<key>AuthName</key>
			<string>%s</string>
			<key>ExtendedAuthEnabled</key>
			<true/>
			<key>ServerAddress</key>
			<string>%s</string>
			<key>RemoteAddress</key>
			<string>%s</string>
			<key>IKEv2</key>
			<dict>
				<key>AuthenticationMethod</key>
				<string>UserName</string>
				<key>AuthName</key>
				<string>%s</string>
				<key>ExtendedAuthEnabled</key>
				<true/>
				<key>RemoteAddress</key>
				<string>%s</string>
				<key>ServerAddress</key>
				<string>%s</string>
				<key>DeadPeerDetectionRate</key>
				<string>Medium</string>
				<key>DisableMOBIKE</key>
				<integer>0</integer>
				<key>DisableRedirect</key>
				<integer>0</integer>
				<key>EnableCertificateRevocationCheck</key>
				<integer>0</integer>
				<key>EnablePFS</key>
				<integer>0</integer>
				<key>ChildSecurityAssociationParameters</key>
				<dict>
					<key>EncryptionAlgorithm</key>
					<string>AES-256-GCM</string>
					<key>IntegrityAlgorithm</key>
					<string>SHA2-384</string>
					<key>DiffieHellmanGroup</key>
					<integer>20</integer>
					<key>LifeTimeInMinutes</key>
					<integer>1440</integer>
				</dict>
				<key>IKESecurityAssociationParameters</key>
				<dict>
					<key>EncryptionAlgorithm</key>
					<string>AES-256-GCM</string>
					<key>IntegrityAlgorithm</key>
					<string>SHA2-384</string>
					<key>DiffieHellmanGroup</key>
					<integer>20</integer>
					<key>LifeTimeInMinutes</key>
					<integer>1440</integer>
				</dict>
			</dict>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>Koris IKEv2</string>
	<key>PayloadIdentifier</key>
	<string>koris.vpn.ikev2.profile.%s</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`, username, uuidPayload, username, host, host, username, host, host, username, uuidProfile)
}

func (s *Server) portalPlans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT id,name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order,created_at FROM plans WHERE is_active=TRUE ORDER BY sort_order ASC, id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	plans := []Plan{}
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err == nil {
			plans = append(plans, plan)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "plans": plans})
}

func (s *Server) portalRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
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

	var customerID int64
	if err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	plan, err := scanPlan(s.DB.QueryRow(`SELECT id,name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order,created_at FROM plans WHERE id=$1 LIMIT 1`, in.PlanID))
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "plan_not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !plan.IsActive {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_inactive"})
		return
	}

	var walletCredit float64
	_ = s.DB.QueryRow(`SELECT COALESCE(credit,0) FROM wallets WHERE username=$1`, username).Scan(&walletCredit)
	if plan.Price > 0 && walletCredit+0.0001 < plan.Price {
		required := plan.Price - walletCredit
		if required < plan.Price && required < 1 {
			required = plan.Price
		}
		res, err := s.DB.Exec(`INSERT INTO payments(customer_id,username,amount,method,receipt,status,intent_type,intent_id,metadata_json,admin_note) VALUES($1,$2,$3,'portal_topup','','pending','plan_renewal',$4,JSON_OBJECT('plan_name',$5,'plan_price',$6,'wallet_at_request',$7),$8)`, customerID, username, required, plan.ID, plan.Name, plan.Price, walletCredit, "portal renewal request: "+plan.Name)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		paymentID, _ := res.LastInsertId()
		writeJSON(w, map[string]any{"ok": true, "renewed": false, "payment_required": true, "payment_id": paymentID, "required_amount": required, "wallet": walletCredit, "price": plan.Price})
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE customers SET plan_id=$1,status='active' WHERE id=$2 AND deleted_at IS NULL`, plan.ID, customerID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=$1 AND attribute='Max-Data'`, username)
	if plan.DataGB > 0 {
		bytes := int64(math.Round(plan.DataGB * 1024 * 1024 * 1024))
		if _, err := tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,'Max-Data',':=',$2)`, username, strconv.FormatInt(bytes, 10)); err != nil {
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
	if _, err := tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at,paid_amount) VALUES($1,$2,$3,$4,$5)`, customerID, username, plan.ID, expires, plan.Price); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if plan.Price > 0 {
		desc := "portal plan activated: " + plan.Name
		paymentRes, err := tx.Exec(`INSERT INTO payments(customer_id,username,amount,method,status,admin_note) VALUES($1,$2,$3,'wallet','approved',$4)`, customerID, username, plan.Price, desc)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		paymentID, _ := paymentRes.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES($1,$2,$3) ON CONFLICT (username) DO UPDATE SET credit=wallets.credit+EXCLUDED.credit, customer_id=COALESCE(EXCLUDED.customer_id,wallets.customer_id)`, customerID, username, -plan.Price); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, customerID, username, -plan.Price, "purchase", desc, "customer", "payment", paymentID); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Auto-provision WireGuard peer on portal subscription renewal
	s.autoProvisionWireGuardPeer(customerID)
	writeJSON(w, map[string]any{"ok": true, "renewed": true, "payment_required": false, "wallet_deducted": plan.Price, "plan": plan})
}

func (s *Server) portalPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var in struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if len(in.NewPassword) < 4 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "password_too_short"})
		return
	}
	var currentPw string
	err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=$1 AND attribute IN('Cleartext-Password','User-Password') ORDER BY id DESC LIMIT 1`, username).Scan(&currentPw)
	if err != nil {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid_old_password"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(currentPw), []byte(in.OldPassword)) != 1 {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid_old_password"})
		return
	}
	res, err := s.DB.Exec(`UPDATE radcheck SET value=$1 WHERE username=$2 AND attribute IN('Cleartext-Password','User-Password')`, in.NewPassword, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_, err = s.DB.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,'Cleartext-Password',':=',$2)`, username, in.NewPassword)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	s.createEvent("account", "info", "Password changed", "Customer changed their VPN password", username, username)
	writeJSON(w, map[string]any{"ok": true})
}

// portalPreferredNode allows customers to get/set their preferred VPN node.
// GET: returns current preferred node ID
// POST: sets preferred node (0 = random/auto)
func (s *Server) portalPreferredNode(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		var preferredNodeID int64
		_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=$1 AND deleted_at IS NULL`, username).Scan(&preferredNodeID)
		writeJSON(w, map[string]any{"ok": true, "preferred_node_id": preferredNodeID})
	case http.MethodPost:
		var in struct {
			NodeID int64 `json:"node_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		// Validate node exists and is active (0 = auto/random)
		if in.NodeID > 0 {
			var exists int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM knode_connections WHERE id=$1 AND enabled=TRUE`, in.NodeID).Scan(&exists); err != nil || exists == 0 {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node"})
				return
			}
		}
		if in.NodeID == 0 {
			_, _ = s.DB.Exec(`UPDATE customers SET preferred_node_id=NULL WHERE username=$1 AND deleted_at IS NULL`, username)
		} else {
			_, _ = s.DB.Exec(`UPDATE customers SET preferred_node_id=$1 WHERE username=$2 AND deleted_at IS NULL`, in.NodeID, username)
		}
		writeJSON(w, map[string]any{"ok": true, "preferred_node_id": in.NodeID})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) portalPayments(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.DB.Query(`SELECT p.id,p.username,p.amount,p.method,p.status,COALESCE(p.intent_type,'wallet_topup'),p.intent_id,COALESCE(pl.name,''),p.created_at,p.updated_at FROM payments p LEFT JOIN plans pl ON pl.id=p.intent_id AND p.intent_type='plan_renewal' WHERE p.username=$1 ORDER BY p.id DESC LIMIT 100`, username)
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
			if err := rows.Scan(&p.ID, &p.Username, &p.Amount, &p.Method, &p.Status, &p.IntentType, &intentID, &p.IntentLabel, &created, &updated); err == nil {
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
		}
		writeJSON(w, map[string]any{"ok": true, "payments": payments})
	case http.MethodPost:
		var in struct {
			Amount  float64 `json:"amount"`
			Method  string  `json:"method"`
			Receipt string  `json:"receipt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		if in.Amount <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "amount_required"})
			return
		}
		if strings.TrimSpace(in.Method) == "" {
			in.Method = "manual"
		}
		var customerID sql.NullInt64
		_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
		res, err := s.DB.Exec(`INSERT INTO payments(customer_id,username,amount,method,receipt,status,intent_type,admin_note) VALUES($1,$2,$3,$4,$5,'pending','wallet_topup','portal request')`, nullableInt(customerID), username, in.Amount, strings.TrimSpace(in.Method), strings.TrimSpace(in.Receipt))
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, map[string]any{"ok": true, "id": id})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}
