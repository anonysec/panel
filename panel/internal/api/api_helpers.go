package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func NoCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	writeJSONCode(w, http.StatusOK, v)
}

func writeJSONCode(w http.ResponseWriter, code int, v any) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		buf.Reset()
		bufPool.Put(buf)
	}()

	enc := json.NewEncoder(buf)
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"ok":false,"error":"encoding_error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = buf.WriteTo(w)
}

// limitBody wraps the request body with a max size reader (1MB default for JSON endpoints).
// Returns false and writes a 413 error if the limit would be exceeded.
func limitBody(w http.ResponseWriter, r *http.Request, maxBytes int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
}

// maxJSONBody is the default max size for JSON request bodies (1MB).
const maxJSONBody int64 = 1 << 20

// ErrorResponse represents a structured error returned by the API.
type ErrorResponse struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	Status int    `json:"status"`
}

// writeError writes a standardized JSON error response with proper headers.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:  message,
		Code:   code,
		Status: status,
	})
}

// ========== Null-Safety Scanning Helpers ==========

// nullStringPtr converts a sql.NullString to a *string.
// Returns nil if the value is not valid, preserving JSON null serialization.
func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

// nullInt64Ptr converts a sql.NullInt64 to a *int64.
// Returns nil if the value is not valid, preserving JSON null serialization.
func nullInt64Ptr(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	return &ni.Int64
}

// nullTimePtr converts a sql.NullTime to a *string formatted as RFC3339.
// Returns nil if the value is not valid, preserving JSON null serialization.
func nullTimePtr(nt sql.NullTime) *string {
	if !nt.Valid {
		return nil
	}
	s := nt.Time.Format(time.RFC3339)
	return &s
}

// ========== Per-Node VPN Config ==========

type NodeVPNConfig struct {
	ID       int64           `json:"id"`
	NodeID   int64           `json:"node_id"`
	Protocol string          `json:"protocol"`
	Enabled  bool            `json:"enabled"`
	Port     int             `json:"port"`
	Network  string          `json:"network"`
	Extra    json.RawMessage `json:"extra_json,omitempty"`
}
