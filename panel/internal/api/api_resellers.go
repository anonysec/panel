package api

import (
	"KorisPanel/panel/internal/auth"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) resellers(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || (role != "owner" && role != "admin") {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	if r.Method == http.MethodGet {
		rows, err := s.DB.Query(`SELECT id, username, role, is_active, credit, COALESCE(avatar,''), created_at FROM admins WHERE role='reseller' ORDER BY id DESC`)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()

		type Reseller struct {
			ID        int64   `json:"id"`
			Username  string  `json:"username"`
			Role      string  `json:"role"`
			IsActive  bool    `json:"is_active"`
			Credit    float64 `json:"credit"`
			Avatar    string  `json:"avatar"`
			CreatedAt string  `json:"created_at"`
		}

		list := []Reseller{}
		for rows.Next() {
			var res Reseller
			var active bool
			var created time.Time
			if err := rows.Scan(&res.ID, &res.Username, &res.Role, &active, &res.Credit, &res.Avatar, &created); err == nil {
				res.IsActive = active
				res.CreatedAt = created.Format(time.RFC3339)
				list = append(list, res)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "resellers": list})
		return
	}

	if r.Method == http.MethodPost {
		var in struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Avatar   string `json:"avatar"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		in.Username = strings.TrimSpace(in.Username)
		if len(in.Username) < 3 || len(in.Password) < 4 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_username_or_password"})
			return
		}

		ph, err := auth.HashPassword(in.Password)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		_, err = s.DB.Exec(`INSERT INTO admins(username, password_hash, role, is_active, avatar) VALUES($1,$2, 'reseller', 1, $3)`, in.Username, ph, nullString(in.Avatar))
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "username_taken"})
			return
		}

		s.logAudit(actor, "reseller.created", "reseller", in.Username, nil, map[string]any{"username": in.Username}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	http.Error(w, "method", http.StatusMethodNotAllowed)
}

func (s *Server) resellerByID(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || (role != "owner" && role != "admin") {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/resellers/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "id_required"})
		return
	}
	id, _ := strconv.ParseInt(parts[0], 10, 64)

	var resellerUsername string
	err := s.DB.QueryRow(`SELECT username FROM admins WHERE id=$1 AND role='reseller' LIMIT 1`, id).Scan(&resellerUsername)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "reseller_not_found"})
		return
	}

	if r.Method == http.MethodDelete {
		_, err = s.DB.Exec(`DELETE FROM admins WHERE id=$1`, id)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		s.logAudit(actor, "reseller.deleted", "reseller", strconv.FormatInt(id, 10), nil, map[string]any{"username": resellerUsername}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	if r.Method == http.MethodPost && len(parts) > 1 && parts[1] == "update" {
		var in struct {
			Password      string `json:"password"`
			DefaultPlanID *int64 `json:"default_plan_id"`
			Avatar        string `json:"avatar"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		if in.Password != "" {
			if len(in.Password) < 4 {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "password_too_short"})
				return
			}
			ph, err := auth.HashPassword(in.Password)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			_, _ = s.DB.Exec(`UPDATE admins SET password_hash=$1 WHERE id=$2`, ph, id)
		}
		// Always update avatar (allow clearing it)
		_, _ = s.DB.Exec(`UPDATE admins SET avatar=$1 WHERE id=$2`, nullString(in.Avatar), id)
		s.logAudit(actor, "reseller.updated", "reseller", strconv.FormatInt(id, 10), nil, map[string]any{"username": resellerUsername}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	if r.Method == http.MethodPost && len(parts) > 1 && parts[1] == "credit" {
		var in struct {
			Amount float64 `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}

		_, err = s.DB.Exec(`UPDATE admins SET credit = credit + $1 WHERE id=$2`, in.Amount, id)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		desc := fmt.Sprintf("Admin %s adjusted reseller credit by %.2f", actor, in.Amount)
		ttype := "allocation"
		if in.Amount < 0 {
			ttype = "deduction"
		}
		_, _ = s.DB.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES($1,$2,$3,$4,$5)`, resellerUsername, in.Amount, ttype, desc, actor)

		s.logAudit(actor, "reseller.credit_adjusted", "reseller", strconv.FormatInt(id, 10), nil, map[string]any{"username": resellerUsername, "amount": in.Amount}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	// GET/POST /api/resellers/:id/plans — manage allowed plans for this reseller
	if len(parts) > 1 && parts[1] == "plans" {
		if r.Method == http.MethodGet {
			rows, err := s.DB.Query(`SELECT plan_id FROM reseller_allowed_plans WHERE reseller_id=$1`, id)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			defer rows.Close()
			planIDs := []int64{}
			for rows.Next() {
				var pid int64
				if err := rows.Scan(&pid); err == nil {
					planIDs = append(planIDs, pid)
				}
			}
			writeJSON(w, map[string]any{"ok": true, "plan_ids": planIDs})
			return
		}
		if r.Method == http.MethodPost {
			limitBody(w, r, maxJSONBody)
			var in struct {
				PlanIDs []int64 `json:"plan_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
				return
			}
			tx, err := s.DB.Begin()
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			defer tx.Rollback()
			_, _ = tx.Exec(`DELETE FROM reseller_allowed_plans WHERE reseller_id=$1`, id)
			for _, pid := range in.PlanIDs {
				_, _ = tx.Exec(`INSERT INTO reseller_allowed_plans(reseller_id, plan_id) VALUES($1,$2)`, id, pid)
			}
			if err := tx.Commit(); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			s.logAudit(actor, "reseller.plans_updated", "reseller", strconv.FormatInt(id, 10), nil, map[string]any{"username": resellerUsername, "plan_ids": in.PlanIDs}, clientIP(r))
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	http.Error(w, "method", http.StatusMethodNotAllowed)
}
