package api

import (
	"database/sql"
	"net/http"
	"os"
)

// AgentVersionResponse is the response payload for GET /api/node/agent/version.
type AgentVersionResponse struct {
	OK       bool   `json:"ok"`
	Version  string `json:"version"`
	URL      string `json:"url"`
	Checksum string `json:"checksum_sha256"`
}

// agentVersion handles GET /api/node/agent/version.
// It authenticates the calling node via X-Node-Token header, then returns
// the latest agent release info (version, download URL, checksum).
func (s *Server) agentVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate node via X-Node-Token header
	_, ok := s.authNode(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "bad_token"})
		return
	}

	// Query latest agent release
	var version, binaryPath, checksum string
	err := s.DB.QueryRow(`SELECT version, binary_path, checksum_sha256 FROM agent_releases ORDER BY released_at DESC LIMIT 1`).Scan(&version, &binaryPath, &checksum)
	if err == sql.ErrNoRows {
		// No releases exist yet — return empty response per spec
		writeJSON(w, AgentVersionResponse{
			OK:       true,
			Version:  "",
			URL:      "",
			Checksum: "",
		})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
		return
	}

	writeJSON(w, AgentVersionResponse{
		OK:       true,
		Version:  version,
		URL:      "/api/node/agent/download",
		Checksum: checksum,
	})
}

// agentDownload handles GET /api/node/agent/download.
// It authenticates the calling node via X-Node-Token header, then serves
// the binary file referenced in the latest agent_releases record.
func (s *Server) agentDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate node via X-Node-Token header
	_, ok := s.authNode(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "bad_token"})
		return
	}

	// Query latest agent release binary path
	var binaryPath string
	err := s.DB.QueryRow(`SELECT binary_path FROM agent_releases ORDER BY released_at DESC LIMIT 1`).Scan(&binaryPath)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "no_release"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
		return
	}

	// Verify the file exists before serving
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "binary_not_found"})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="node-agent"`)
	http.ServeFile(w, r, binaryPath)
}
