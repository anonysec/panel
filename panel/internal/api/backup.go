package api

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"KorisPanel/panel/internal/backup"
)

// backupList handles GET /api/admin/backups — lists all backup records.
func (s *Server) backupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	records, err := s.BackupService.ListBackups(r.Context())
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	type backupJSON struct {
		ID            int64   `json:"id"`
		Filename      string  `json:"filename"`
		Status        string  `json:"status"`
		Type          string  `json:"type"`
		SizeBytes     *int64  `json:"size_bytes"`
		Checksum      *string `json:"checksum"`
		NodesIncluded any     `json:"nodes_included"`
		NodesSkipped  any     `json:"nodes_skipped"`
		ErrorMessage  *string `json:"error_message"`
		StartedAt     string  `json:"started_at"`
		CompletedAt   *string `json:"completed_at"`
	}

	out := make([]backupJSON, 0, len(records))
	for _, rec := range records {
		b := backupJSON{
			ID:        rec.ID,
			Filename:  rec.Filename,
			Status:    rec.Status,
			Type:      rec.Type,
			StartedAt: rec.StartedAt.Format(time.RFC3339),
		}
		if rec.SizeBytes.Valid {
			b.SizeBytes = &rec.SizeBytes.Int64
		}
		if rec.Checksum.Valid {
			b.Checksum = &rec.Checksum.String
		}
		if rec.ErrorMessage.Valid {
			b.ErrorMessage = &rec.ErrorMessage.String
		}
		if rec.CompletedAt.Valid {
			t := rec.CompletedAt.Time.Format(time.RFC3339)
			b.CompletedAt = &t
		}
		if rec.NodesIncluded.Valid {
			var v any
			if json.Unmarshal([]byte(rec.NodesIncluded.String), &v) == nil {
				b.NodesIncluded = v
			}
		}
		if rec.NodesSkipped.Valid {
			var v any
			if json.Unmarshal([]byte(rec.NodesSkipped.String), &v) == nil {
				b.NodesSkipped = v
			}
		}
		out = append(out, b)
	}

	writeJSON(w, map[string]any{"ok": true, "backups": out})
}

// backupCreate handles POST /api/admin/backups — triggers a manual backup.
func (s *Server) backupCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	if s.BackupService == nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "backup service not initialized"})
		return
	}

	// Start backup asynchronously
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	var backupID int64
	var startErr error

	// Try to acquire the mutex immediately to get the backup ID
	// We insert the record synchronously to return the ID, then do the rest async
	backupID, startErr = s.BackupService.CreateBackupAsync(ctx)
	if startErr != nil {
		cancel()
		if strings.Contains(startErr.Error(), "already in progress") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "backup_in_progress"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": startErr.Error()})
		return
	}

	// Run the backup in background
	go func() {
		defer cancel()
		s.BackupService.RunBackup(ctx, backupID, "manual")
	}()

	writeJSON(w, map[string]any{"ok": true, "backup_id": backupID})
}

// backupDownload handles GET /api/admin/backups/{id}/download — streams the backup file.
func (s *Server) backupDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse backup ID from URL path: /api/admin/backups/{id}/download
	id := extractBackupID(r.URL.Path, "/api/admin/backups/", "/download")
	if id <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid backup id"})
		return
	}

	var filename string
	err := s.DB.QueryRowContext(r.Context(), `SELECT filename FROM backups WHERE id=$1`, id).Scan(&filename)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "backup not found"})
		return
	}

	archivePath := filepath.Join(s.BackupService.StorageDir(), filename)
	f, err := os.Open(archivePath)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "backup_file_not_found"})
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "stat failed"})
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	http.ServeContent(w, r, filename, fi.ModTime(), f)
}

// backupVerify handles POST /api/admin/backups/{id}/verify — verifies backup integrity.
func (s *Server) backupVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	id := extractBackupID(r.URL.Path, "/api/admin/backups/", "/verify")
	if id <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid backup id"})
		return
	}

	valid, err := s.BackupService.VerifyIntegrity(r.Context(), id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "valid": valid})
}

// backupDelete handles DELETE /api/admin/backups/{id} — deletes a backup.
func (s *Server) backupDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Path: /api/admin/backups/{id}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/backups/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid backup id"})
		return
	}

	if err := s.BackupService.DeleteBackup(r.Context(), id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// backupRestore handles POST /api/admin/backups/restore — restores from uploaded backup.
func (s *Server) backupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (limit to 500MB)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid multipart form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file required"})
		return
	}
	defer file.Close()

	if err := s.BackupService.RestoreFromUpload(r.Context(), file, header.Filename); err != nil {
		if strings.Contains(err.Error(), "already in progress") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "backup_in_progress"})
			return
		}
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "missing") || strings.Contains(err.Error(), "checksum") {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "message": "restore completed"})
}

// backupSettings handles GET/PUT /api/admin/backups/settings.
func (s *Server) backupSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.backupSettingsGet(w, r)
	case http.MethodPut:
		s.backupSettingsPut(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) backupSettingsGet(w http.ResponseWriter, r *http.Request) {
	var schedule, retentionStr string
	_ = s.DB.QueryRowContext(r.Context(), `SELECT setting_value FROM panel_settings WHERE setting_key='backup_schedule'`).Scan(&schedule)
	_ = s.DB.QueryRowContext(r.Context(), `SELECT setting_value FROM panel_settings WHERE setting_key='backup_retention_count'`).Scan(&retentionStr)

	if schedule == "" {
		schedule = "daily:02"
	}
	retention := 7
	if n, err := strconv.Atoi(retentionStr); err == nil && n > 0 {
		retention = n
	}

	writeJSON(w, map[string]any{
		"ok":              true,
		"schedule":        schedule,
		"retention_count": retention,
	})
}

func (s *Server) backupSettingsPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Schedule       string `json:"schedule"`
		RetentionCount int    `json:"retention_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	in.Schedule = strings.TrimSpace(in.Schedule)
	if in.Schedule == "" {
		in.Schedule = "disabled"
	}
	if in.RetentionCount < 1 {
		in.RetentionCount = 1
	}
	if in.RetentionCount > 30 {
		in.RetentionCount = 30
	}

	// Validate schedule format
	sched := backup.ParseSchedule(in.Schedule)
	if sched.Type != "daily" && sched.Type != "weekly" && sched.Type != "disabled" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid schedule format"})
		return
	}

	if err := s.BackupService.UpdateConfig(in.Schedule, in.RetentionCount); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// backupRoot dispatches /api/admin/backups (no trailing slash) by method.
func (s *Server) backupRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.backupList(w, r)
	case http.MethodPost:
		s.backupCreate(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// backupByID dispatches /api/admin/backups/{id}... sub-routes.
func (s *Server) backupByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/backups/")

	// /api/admin/backups/{id}/preview
	if strings.HasSuffix(path, "/preview") {
		s.backupPreview(w, r)
		return
	}

	// /api/admin/backups/{id}/download
	if strings.HasSuffix(path, "/download") {
		s.backupDownload(w, r)
		return
	}

	// /api/admin/backups/{id}/verify
	if strings.HasSuffix(path, "/verify") {
		s.backupVerify(w, r)
		return
	}

	// /api/admin/backups/{id} — DELETE
	if r.Method == http.MethodDelete {
		s.backupDelete(w, r)
		return
	}

	writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
}

// backupPreview handles GET /api/admin/backups/{id}/preview — returns the manifest from the archive.
func (s *Server) backupPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	id := extractBackupID(r.URL.Path, "/api/admin/backups/", "/preview")
	if id <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid backup id"})
		return
	}

	var filename string
	err := s.DB.QueryRowContext(r.Context(), `SELECT filename FROM backups WHERE id=$1`, id).Scan(&filename)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "backup not found"})
		return
	}

	archivePath := filepath.Join(s.BackupService.StorageDir(), filename)
	f, err := os.Open(archivePath)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "backup_file_not_found"})
		return
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "invalid archive"})
		return
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var manifestData []byte

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "corrupt archive"})
			return
		}
		if hdr.Name == "manifest.json" {
			manifestData, err = io.ReadAll(tr)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "read manifest failed"})
				return
			}
			break
		}
	}

	if manifestData == nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "manifest_not_found"})
		return
	}

	var manifest any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "invalid manifest json"})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "manifest": manifest})
}

// extractBackupID extracts an integer ID from a URL path between prefix and suffix.
// Example: extractBackupID("/api/admin/backups/42/download", "/api/admin/backups/", "/download") => 42
func extractBackupID(urlPath, prefix, suffix string) int64 {
	s := strings.TrimPrefix(urlPath, prefix)
	s = strings.TrimSuffix(s, suffix)
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
