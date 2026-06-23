//go:build !lite

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// ─── Admin handlers ───────────────────────────────────────────────────────────

func (s *Server) handleKBArticles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listKBArticles(w, r)
	case http.MethodPost:
		s.createKBArticle(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleKBArticleByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/kb/articles/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		s.updateKBArticle(w, r, id)
	case http.MethodDelete:
		s.deleteKBArticle(w, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listKBArticles(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	query := "SELECT id, title, category, status, locale, COALESCE(parent_id, 0), view_count, created_at, updated_at FROM kb_articles"
	var args []any
	if category != "" {
		query += " WHERE category = ?"
		args = append(args, category)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		log.Printf("[kb] list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type article struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Category  string `json:"category"`
		Status    string `json:"status"`
		Locale    string `json:"locale"`
		ParentID  *int64 `json:"parent_id,omitempty"`
		ViewCount int    `json:"view_count"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	var articles []article
	for rows.Next() {
		var a article
		var parentID int64
		if err := rows.Scan(&a.ID, &a.Title, &a.Category, &a.Status, &a.Locale, &parentID, &a.ViewCount, &a.CreatedAt, &a.UpdatedAt); err != nil {
			log.Printf("[kb] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		if parentID > 0 {
			a.ParentID = &parentID
		}
		articles = append(articles, a)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[kb] rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if articles == nil {
		articles = []article{}
	}
	writeJSON(w, map[string]any{"ok": true, "articles": articles})
}

func (s *Server) createKBArticle(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Category string `json:"category"`
		Status   string `json:"status"`
		Locale   string `json:"locale"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.Title == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "title_required"})
		return
	}
	if in.Body == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "body_required"})
		return
	}
	if in.Category == "" {
		in.Category = "general"
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if in.Status != "draft" && in.Status != "published" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_status"})
		return
	}
	if in.Locale == "" {
		in.Locale = "en"
	}

	var parentID any
	if in.ParentID != nil && *in.ParentID > 0 {
		// Verify parent exists
		var exists int
		if err := s.DB.QueryRow("SELECT COUNT(*) FROM kb_articles WHERE id = ?", *in.ParentID).Scan(&exists); err != nil || exists == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "parent_not_found"})
			return
		}
		parentID = *in.ParentID
	}

	result, err := s.DB.Exec(
		"INSERT INTO kb_articles (title, body, category, status, locale, parent_id) VALUES (?, ?, ?, ?, ?, ?)",
		in.Title, in.Body, in.Category, in.Status, in.Locale, parentID,
	)
	if err != nil {
		log.Printf("[kb] insert failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) updateKBArticle(w http.ResponseWriter, r *http.Request, id int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Title    *string `json:"title"`
		Body     *string `json:"body"`
		Category *string `json:"category"`
		Status   *string `json:"status"`
		Locale   *string `json:"locale"`
		ParentID *int64  `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	var setClauses []string
	var args []any

	if in.Title != nil {
		if *in.Title == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "title_required"})
			return
		}
		setClauses = append(setClauses, "title = ?")
		args = append(args, *in.Title)
	}
	if in.Body != nil {
		setClauses = append(setClauses, "body = ?")
		args = append(args, *in.Body)
	}
	if in.Category != nil {
		setClauses = append(setClauses, "category = ?")
		args = append(args, *in.Category)
	}
	if in.Status != nil {
		if *in.Status != "draft" && *in.Status != "published" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_status"})
			return
		}
		setClauses = append(setClauses, "status = ?")
		args = append(args, *in.Status)
	}
	if in.Locale != nil {
		setClauses = append(setClauses, "locale = ?")
		args = append(args, *in.Locale)
	}
	if in.ParentID != nil {
		if *in.ParentID > 0 {
			var exists int
			if err := s.DB.QueryRow("SELECT COUNT(*) FROM kb_articles WHERE id = ?", *in.ParentID).Scan(&exists); err != nil || exists == 0 {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "parent_not_found"})
				return
			}
			setClauses = append(setClauses, "parent_id = ?")
			args = append(args, *in.ParentID)
		} else {
			setClauses = append(setClauses, "parent_id = NULL")
		}
	}

	if len(setClauses) == 0 {
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	args = append(args, id)
	query := "UPDATE kb_articles SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	result, err := s.DB.Exec(query, args...)
	if err != nil {
		log.Printf("[kb] update failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deleteKBArticle(w http.ResponseWriter, id int64) {
	result, err := s.DB.Exec("DELETE FROM kb_articles WHERE id = ?", id)
	if err != nil {
		log.Printf("[kb] delete failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ─── Customer (portal) handlers ──────────────────────────────────────────────

func (s *Server) handlePortalKB(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.portalKBList(w, r)
}

func (s *Server) handlePortalKBSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.portalKBSearch(w, r)
}

func (s *Server) handlePortalKBByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, _, ok := pathID(r.URL.Path, "/api/portal/kb/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	s.portalKBArticle(w, r, id)
}

func (s *Server) portalKBList(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	locale := r.URL.Query().Get("locale")

	query := "SELECT id, title, category, locale, view_count, created_at FROM kb_articles WHERE status = 'published'"
	var args []any

	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if locale != "" {
		query += " AND locale = ?"
		args = append(args, locale)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		log.Printf("[kb] portal list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type article struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Category  string `json:"category"`
		Locale    string `json:"locale"`
		ViewCount int    `json:"view_count"`
		CreatedAt string `json:"created_at"`
	}

	var articles []article
	for rows.Next() {
		var a article
		if err := rows.Scan(&a.ID, &a.Title, &a.Category, &a.Locale, &a.ViewCount, &a.CreatedAt); err != nil {
			log.Printf("[kb] portal scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		articles = append(articles, a)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[kb] portal rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if articles == nil {
		articles = []article{}
	}
	writeJSON(w, map[string]any{"ok": true, "articles": articles})
}

func (s *Server) portalKBSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "query_required"})
		return
	}

	locale := r.URL.Query().Get("locale")

	query := "SELECT id, title, category, locale, view_count, created_at FROM kb_articles WHERE MATCH(title, body) AGAINST(? IN BOOLEAN MODE) AND status = 'published'"
	args := []any{q}

	if locale != "" {
		query += " AND locale = ?"
		args = append(args, locale)
	}
	query += " ORDER BY created_at DESC LIMIT 50"

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		log.Printf("[kb] search query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type article struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Category  string `json:"category"`
		Locale    string `json:"locale"`
		ViewCount int    `json:"view_count"`
		CreatedAt string `json:"created_at"`
	}

	var articles []article
	for rows.Next() {
		var a article
		if err := rows.Scan(&a.ID, &a.Title, &a.Category, &a.Locale, &a.ViewCount, &a.CreatedAt); err != nil {
			log.Printf("[kb] search scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		articles = append(articles, a)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[kb] search rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if articles == nil {
		articles = []article{}
	}
	writeJSON(w, map[string]any{"ok": true, "articles": articles})
}

func (s *Server) portalKBArticle(w http.ResponseWriter, r *http.Request, id int64) {
	locale := r.URL.Query().Get("locale")

	// If locale is specified and differs from the article's locale, try to find a translation
	if locale != "" {
		var translationID int64
		err := s.DB.QueryRow(
			"SELECT id FROM kb_articles WHERE parent_id = ? AND locale = ? AND status = 'published' LIMIT 1",
			id, locale,
		).Scan(&translationID)
		if err == nil {
			id = translationID
		}
		// If no translation found, fall through to original article
	}

	type article struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		Category  string `json:"category"`
		Locale    string `json:"locale"`
		ParentID  *int64 `json:"parent_id,omitempty"`
		ViewCount int    `json:"view_count"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	var a article
	var parentID int64
	err := s.DB.QueryRow(
		"SELECT id, title, body, category, locale, COALESCE(parent_id, 0), view_count, created_at, updated_at FROM kb_articles WHERE id = ? AND status = 'published'",
		id,
	).Scan(&a.ID, &a.Title, &a.Body, &a.Category, &a.Locale, &parentID, &a.ViewCount, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if parentID > 0 {
		a.ParentID = &parentID
	}

	// Increment view count
	_, _ = s.DB.Exec("UPDATE kb_articles SET view_count = view_count + 1 WHERE id = ?", id)

	writeJSON(w, map[string]any{"ok": true, "article": a})
}
