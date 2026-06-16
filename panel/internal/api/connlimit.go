package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// setConnectionLimit handles POST /api/customers/{id}/connection-limit.
// It sets or removes the Simultaneous-Use RADIUS check attribute for the customer.
func (s *Server) setConnectionLimit(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Limit int `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if in.Limit < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "limit must be >= 0")
		return
	}

	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "customer not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if in.Limit == 0 {
		// Remove the Simultaneous-Use attribute (unlimited sessions)
		_, err = s.DB.Exec(`DELETE FROM radcheck WHERE username=? AND attribute='Simultaneous-Use'`, username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	} else {
		// Delete existing and insert new value
		_, err = s.DB.Exec(`DELETE FROM radcheck WHERE username=? AND attribute='Simultaneous-Use'`, username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		_, err = s.DB.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Simultaneous-Use',':=',?)`, username, strconv.Itoa(in.Limit))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true, "connection_limit": in.Limit})
}
