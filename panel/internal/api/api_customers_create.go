package api

import (
	"KorisPanel/panel/internal/auth"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) createCustomer(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Username          string   `json:"username"`
		Password          string   `json:"password"`
		DisplayName       string   `json:"display_name"`
		PlanID            *int64   `json:"plan_id"`
		DataGB            *float64 `json:"data_gb"`
		SpeedMbps         *float64 `json:"speed_mbps"`
		Days              *int     `json:"days"`
		IPLimit           *int     `json:"ip_limit"`
		ActivateOnConnect bool     `json:"activate_on_connect"`
		TemplateID        *int64   `json:"template_id"`
		Avatar            *string  `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if !usernamePattern.MatchString(in.Username) || len(in.Password) < 4 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "username_password_required"})
		return
	}

	// Template pre-population: load template and use its values as defaults
	var templateRadiusChecks []radiusAttr
	var templateRadiusReplies []radiusAttr
	if in.TemplateID != nil {
		var tmpl UserTemplate
		row := s.DB.QueryRow(`SELECT id, name, plan_id, status, connection_limit, radius_checks, radius_replies, created_by, deleted_at, created_at, updated_at FROM user_templates WHERE id = $1`, *in.TemplateID)
		var err error
		tmpl, err = scanTemplate(row)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid template_id: template not found")
			return
		}
		if tmpl.DeletedAt != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid template_id: template has been deleted")
			return
		}
		// Pre-populate plan_id from template if not explicitly provided
		if in.PlanID == nil && tmpl.PlanID != nil {
			in.PlanID = tmpl.PlanID
		}
		// Pre-populate connection limit from template if not explicitly provided
		if in.IPLimit == nil && tmpl.ConnectionLimit > 0 {
			in.IPLimit = &tmpl.ConnectionLimit
		}
		// Parse RADIUS check attributes from template
		if len(tmpl.RadiusChecks) > 0 && string(tmpl.RadiusChecks) != "null" {
			if err := json.Unmarshal(tmpl.RadiusChecks, &templateRadiusChecks); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to parse template radius_checks")
				return
			}
		}
		// Parse RADIUS reply attributes from template
		if len(tmpl.RadiusReplies) > 0 && string(tmpl.RadiusReplies) != "null" {
			if err := json.Unmarshal(tmpl.RadiusReplies, &templateRadiusReplies); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to parse template radius_replies")
				return
			}
		}
	}

	if in.PlanID != nil && *in.PlanID == 0 {
		in.PlanID = nil
	}
	dataGB := 0.0
	speedMbps := 0.0
	days := 0
	if in.DataGB != nil {
		dataGB = *in.DataGB
	}
	if in.SpeedMbps != nil {
		speedMbps = *in.SpeedMbps
	}
	if in.Days != nil {
		days = *in.Days
	}
	if dataGB < 0 || speedMbps < 0 || days < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_limits"})
		return
	}
	actor, role, ok := s.currentAdmin(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	if in.PlanID != nil {
		var planDataGB, planSpeedMbps float64
		var planDays int
		var planPrice float64
		var planBillingType string
		if err := s.DB.QueryRow(`SELECT data_gb,speed_mbps,duration_days,price,COALESCE(billing_type,'quota') FROM plans WHERE id=$1 AND is_active=TRUE LIMIT 1`, *in.PlanID).Scan(&planDataGB, &planSpeedMbps, &planDays, &planPrice, &planBillingType); err == nil {
			// Resellers can only assign quota plans, not pay-as-you-go
			if role == "reseller" && planBillingType == "payg" {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "reseller_quota_only"})
				return
			}
			// Resellers can only use plans from their allowed list
			if role == "reseller" {
				var resellerID int64
				_ = s.DB.QueryRow(`SELECT id FROM admins WHERE username=$1`, actor).Scan(&resellerID)
				var allowed int
				_ = s.DB.QueryRow(`SELECT COUNT(*) FROM reseller_allowed_plans WHERE reseller_id=$1 AND plan_id=$2`, resellerID, *in.PlanID).Scan(&allowed)
				if allowed == 0 {
					writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "plan_not_allowed"})
					return
				}
			}
			if in.DataGB == nil {
				dataGB = planDataGB
			}
			if in.SpeedMbps == nil {
				speedMbps = planSpeedMbps
			}
			if in.Days == nil {
				days = planDays
			}
			if role == "reseller" && planPrice > 0 {
				var resellerCredit float64
				_ = s.DB.QueryRow(`SELECT credit FROM admins WHERE username=$1`, actor).Scan(&resellerCredit)
				if resellerCredit < planPrice {
					writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "insufficient_reseller_credit", "credit": resellerCredit, "required": planPrice})
					return
				}
				_, err := s.DB.Exec(`UPDATE admins SET credit = credit - $1 WHERE username=$2`, planPrice, actor)
				if err != nil {
					writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
					return
				}
				_, _ = s.DB.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES($1,$2, 'deduction', $3, $4)`, actor, -planPrice, "Created customer "+in.Username, actor)
			}
		}
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	// Determine avatar: if explicitly provided use it, otherwise admin/owner gets random emoji, reseller leaves NULL (inherits)
	var avatarVal any
	if in.Avatar != nil && *in.Avatar != "" {
		avatarVal = *in.Avatar
	} else if role != "reseller" {
		avatarVal = randomEmoji(s.reservedEmojis())
	}

	res, err := tx.Exec(`INSERT INTO customers(username,display_name,plan_id,sub_token,created_by,avatar) VALUES($1,$2,$3,$4,$5,$6)`, in.Username, in.DisplayName, in.PlanID, auth.RandomToken(24), actor, avatarVal)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	customerID, _ := res.LastInsertId()
	if _, err = tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES($1,$2,0)`, customerID, in.Username); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,'Cleartext-Password',':=',$2)`, in.Username, in.Password); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,'Simultaneous-Use',':=',$2)`, in.Username, strconv.Itoa(func() int {
		if in.IPLimit != nil && *in.IPLimit > 0 {
			return *in.IPLimit
		}
		return 1
	}())); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=$1 AND attribute='Max-Data'`, in.Username)
	if dataGB > 0 {
		bytes := int64(math.Round(dataGB * 1024 * 1024 * 1024))
		if _, err = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,'Max-Data',':=',$2)`, in.Username, bytes); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=$1 AND attribute='Mikrotik-Rate-Limit'`, in.Username)
	if speedMbps > 0 {
		if _, err = tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES($1,'Mikrotik-Rate-Limit',':=',$2)`, in.Username, speedLimitValue(speedMbps)); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	// Insert template RADIUS check attributes (skip attributes already set by explicit fields)
	for _, attr := range templateRadiusChecks {
		if attr.Attribute == "Cleartext-Password" || attr.Attribute == "Simultaneous-Use" || attr.Attribute == "Max-Data" {
			continue // These are managed by explicit fields above
		}
		if _, err = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,$2,$3,$4)`, in.Username, attr.Attribute, attr.Op, attr.Value); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	// Insert template RADIUS reply attributes (skip attributes already set by explicit fields)
	for _, attr := range templateRadiusReplies {
		if attr.Attribute == "Mikrotik-Rate-Limit" {
			continue // Managed by speed_mbps field above
		}
		if _, err = tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES($1,$2,$3,$4)`, in.Username, attr.Attribute, attr.Op, attr.Value); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if days > 0 {
		if in.ActivateOnConnect {
			// First-connection activation: don't set expires_at yet; auth script will set it on first VPN connect
			if _, err = tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,activate_on_connect) VALUES($1,$2,$3,1)`, customerID, in.Username, in.PlanID); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		} else {
			expires := time.Now().AddDate(0, 0, days)
			if _, err = tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at) VALUES($1,$2,$3,$4)`, customerID, in.Username, in.PlanID, expires); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Auto-provision WireGuard peer on WireGuard-enabled nodes
	if in.PlanID != nil && *in.PlanID > 0 {
		s.autoProvisionWireGuardPeer(customerID)
	}
	// Sync user to knode instances via gRPC
	if s.UserSync != nil {
		go func() {
			if err := s.UserSync.SyncUser(context.Background(), in.Username); err != nil {
				log.Printf("[knode] SyncUser failed after customer create for %q: %v", in.Username, err)
			}
		}()
	}
	if s.Cache != nil {
		s.Cache.InvalidatePrefix("stats:")
	}
	actor, _, _ = s.currentAdmin(r)
	s.logAudit(actor, "customer.created", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{"username": in.Username}, clientIP(r))
	s.createEvent("customer", "info", fmt.Sprintf("Customer created: %s", in.Username), fmt.Sprintf("Admin %s created customer %s", actor, in.Username), actor, in.Username)
	writeJSON(w, map[string]any{"ok": true, "id": customerID})
}

func (s *Server) customerByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/customers/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			s.getCustomerDetail(w, r, id)
		case http.MethodPatch:
			s.updateCustomer(w, r, id)
		case http.MethodDelete:
			s.archiveCustomer(w, r, id)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
		return
	}
	if action == "usage" {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.getCustomerUsage(w, id)
		return
	}
	if action == "tags" {
		s.handleCustomerTags(w, r, id)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "enable":
		s.setCustomerStatus(w, id, "active")
	case "disable":
		s.setCustomerStatus(w, id, "disabled")
	case "reset-password":
		s.resetCustomerPassword(w, r, id)
	case "reset-traffic":
		s.resetCustomerTraffic(w, r, id)
	case "renew":
		s.renewCustomer(w, r, id)
	case "restore":
		s.restoreCustomer(w, r, id)
	case "connection-limit":
		s.setConnectionLimit(w, r, id)
	case "switch-plan":
		s.switchCustomerPlan(w, r, id)
	case "impersonate":
		// Only full admins (owner/admin) can impersonate — block resellers
		_, role, _ := s.currentAdmin(r)
		if role == "reseller" {
			writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
			return
		}
		s.adminImpersonateCustomer(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}
