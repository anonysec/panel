//go:build !lite

package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxAttachmentSize is the maximum allowed file size for ticket attachments (5MB).
const maxAttachmentSize int64 = 5 << 20

// uploadsBaseDir is the base directory for ticket file uploads.
const uploadsBaseDir = "/opt/KorisPanel/uploads/tickets"

// allowedMimeTypes defines the permitted MIME types for ticket attachments.
var allowedMimeTypes = map[string]bool{
	"image/jpeg":         true,
	"image/png":          true,
	"image/gif":          true,
	"image/webp":         true,
	"application/pdf":    true,
	"text/plain":         true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
}

// sanitizeFilename removes path traversal characters and limits filename length.
func sanitizeFilename(name string) string {
	// Take only the base name (removes directory components)
	name = filepath.Base(name)
	// Remove any remaining path separators or traversal
	name = strings.ReplaceAll(name, "..", "")
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")
	// Trim spaces
	name = strings.TrimSpace(name)
	// Limit length
	if len(name) > 200 {
		name = name[:200]
	}
	// Fallback if empty
	if name == "" || name == "." {
		name = "attachment"
	}
	return name
}

// adminAttachFile handles POST /api/admin/tickets/:id/attach
func (s *Server) adminAttachFile(w http.ResponseWriter, r *http.Request, id int64) {
	// Verify ticket exists
	ticket, err := s.Support.Get(r.Context(), id)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "ticket_not_found"})
		return
	}

	admin, _, _ := s.currentAdmin(r)

	attachment, err := s.handleFileUpload(w, r, ticket.ID, "admin", admin)
	if err != nil {
		// Error already written to response
		return
	}

	s.logAudit(admin, "support_ticket.attachment_uploaded", "ticket", strconv.FormatInt(id, 10), nil,
		map[string]any{"filename": attachment["filename"], "filesize": attachment["file_size"]}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "attachment": attachment})
}

// customerAttachFile handles POST /api/customer/tickets/:id/attach
func (s *Server) customerAttachFile(w http.ResponseWriter, r *http.Request, id int64) {
	username, _ := s.currentCustomer(r)

	// Verify ticket belongs to this customer
	if !s.supportTicketBelongsToCustomer(r, id, username) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	attachment, err := s.handleFileUpload(w, r, id, "customer", username)
	if err != nil {
		// Error already written to response
		return
	}

	writeJSON(w, map[string]any{"ok": true, "attachment": attachment})
}

// handleFileUpload processes the multipart file upload for ticket attachments.
// Returns attachment metadata map on success, or an error (with response already written).
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request, ticketID int64, senderType, senderName string) (map[string]any, error) {
	// Limit total request body to maxAttachmentSize + some overhead for multipart headers
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentSize+1024)

	// Parse multipart form
	if err := r.ParseMultipartForm(maxAttachmentSize); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file_too_large", "max_size_mb": 5})
		return nil, fmt.Errorf("parse multipart: %w", err)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file_required"})
		return nil, fmt.Errorf("form file: %w", err)
	}
	defer file.Close()

	// Check file size
	if header.Size > maxAttachmentSize {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file_too_large", "max_size_mb": 5})
		return nil, fmt.Errorf("file too large: %d bytes", header.Size)
	}

	// Sanitize filename
	filename := sanitizeFilename(header.Filename)

	// Detect MIME type from content
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	detectedMime := http.DetectContentType(buf[:n])

	// Also check the extension-based MIME type
	ext := filepath.Ext(filename)
	extMime := mime.TypeByExtension(ext)

	// Use detected MIME, but prefer extension for known types
	mimeType := detectedMime
	if extMime != "" && allowedMimeTypes[extMime] {
		mimeType = extMime
	}

	// Validate MIME type
	if !allowedMimeTypes[mimeType] {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file_type_not_allowed", "mime_type": mimeType})
		return nil, fmt.Errorf("disallowed mime type: %s", mimeType)
	}

	// Seek back to beginning after reading for MIME detection
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "upload_failed"})
		return nil, fmt.Errorf("seek: %w", err)
	}

	// Create upload directory
	uploadDir := filepath.Join(uploadsBaseDir, strconv.FormatInt(ticketID, 10))
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("[support] failed to create upload dir %s: %v", uploadDir, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "upload_failed"})
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	// Generate unique filename
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		log.Printf("[support] failed to generate random ID: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "upload_failed"})
		return nil, fmt.Errorf("rand: %w", err)
	}
	fileUUID := hex.EncodeToString(randBytes)
	storedName := fileUUID + "_" + filename
	filePath := filepath.Join(uploadDir, storedName)

	// Write file to disk
	dst, err := os.Create(filePath)
	if err != nil {
		log.Printf("[support] failed to create file %s: %v", filePath, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "upload_failed"})
		return nil, fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		os.Remove(filePath)
		log.Printf("[support] failed to write file %s: %v", filePath, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "upload_failed"})
		return nil, fmt.Errorf("copy file: %w", err)
	}

	// Get the latest message for this ticket, or create one
	var messageID int64
	err = s.DB.QueryRowContext(r.Context(),
		`SELECT id FROM ticket_messages WHERE ticket_id = $1 ORDER BY created_at DESC LIMIT 1`, ticketID,
	).Scan(&messageID)
	if err != nil {
		// No messages exist — create an attachment message
		msg, err := s.Support.Reply(r.Context(), ticketID, senderType, senderName, "[File attachment]", false)
		if err != nil {
			os.Remove(filePath)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "message_create_failed"})
			return nil, fmt.Errorf("create message: %w", err)
		}
		messageID = msg.ID
	}

	// Insert attachment record into database
	result, err := s.DB.ExecContext(r.Context(), `
		INSERT INTO ticket_attachments (message_id, filename, filepath, filesize, mime_type)
		VALUES ($1, $2, $3, $4, $5)`,
		messageID, filename, filePath, written, mimeType,
	)
	if err != nil {
		os.Remove(filePath)
		log.Printf("[support] failed to insert attachment record: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "upload_failed"})
		return nil, fmt.Errorf("insert attachment: %w", err)
	}

	attachmentID, _ := result.LastInsertId()

	log.Printf("[support] attachment uploaded: ticket #%d, file=%s, size=%d, mime=%s",
		ticketID, filename, written, mimeType)

	return map[string]any{
		"id":         attachmentID,
		"message_id": messageID,
		"filename":   filename,
		"file_size":  written,
		"mime_type":  mimeType,
		"created_at": "now",
	}, nil
}

// serveAttachment handles GET /api/tickets/attachments/:id — serves a file with access control.
func (s *Server) serveAttachment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse attachment ID from URL
	rest := strings.TrimPrefix(r.URL.Path, "/api/tickets/attachments/")
	rest = strings.Trim(rest, "/")
	attachmentID, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || attachmentID <= 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Fetch attachment record
	var filename, filePath, mimeType string
	var messageID int64
	err = s.DB.QueryRowContext(r.Context(), `
		SELECT id, message_id, filename, filepath, mime_type
		FROM ticket_attachments WHERE id = $1`, attachmentID,
	).Scan(&attachmentID, &messageID, &filename, &filePath, &mimeType)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Get the ticket ID from the message
	var ticketID int64
	err = s.DB.QueryRowContext(r.Context(),
		`SELECT ticket_id FROM ticket_messages WHERE id = $1`, messageID,
	).Scan(&ticketID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Access control: admin always has access; customer only their own tickets
	_, _, isAdmin := s.currentAdmin(r)
	if !isAdmin {
		// Check if it's a customer
		username, ok := s.currentCustomer(r)
		if !ok {
			writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		// Verify ticket belongs to this customer
		if !s.supportTicketBelongsToCustomer(r, ticketID, username) {
			writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
			return
		}
	}

	// Verify file exists on disk
	if _, err := os.Stat(filePath); err != nil {
		log.Printf("[support] attachment file missing from disk: %s", filePath)
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "file_not_found"})
		return
	}

	// Serve the file
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, filePath)
}
