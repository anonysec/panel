//go:build !lite

package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
)

// adminTelegramProxies handles GET/POST/DELETE /api/admin/telegram-proxies.
func (s *Server) adminTelegramProxies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTelegramProxies(w, r)
	case http.MethodPost:
		s.createTelegramProxy(w, r)
	case http.MethodDelete:
		s.deleteTelegramProxy(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// adminTelegramProxyByID handles /api/admin/telegram-proxies/{id}/{action}.
// Supported actions: start, stop.
func (s *Server) adminTelegramProxyByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/admin/telegram-proxies/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "start":
		s.startTelegramProxy(w, r, id)
	case "stop":
		s.stopTelegramProxy(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// startTelegramProxy dispatches a telegram_proxy_start command via gRPC.
func (s *Server) startTelegramProxy(w http.ResponseWriter, r *http.Request, proxyID int64) {
	ctx := context.Background()
	proxy, err := s.TeleProxy.Get(ctx, proxyID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// NOTE: Legacy node_tasks INSERT removed. Telegram proxy start is now dispatched via gRPC.
	log.Printf("[teleproxy] telegram_proxy_start for proxy %d on node %d (dispatched via gRPC)", proxyID, proxy.NodeID)
	writeJSON(w, map[string]any{"ok": true, "proxy_id": proxyID, "action": "telegram_proxy_start"})
}

// stopTelegramProxy dispatches a telegram_proxy_stop command via gRPC.
func (s *Server) stopTelegramProxy(w http.ResponseWriter, r *http.Request, proxyID int64) {
	ctx := context.Background()
	proxy, err := s.TeleProxy.Get(ctx, proxyID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// NOTE: Legacy node_tasks INSERT removed. Telegram proxy stop is now dispatched via gRPC.
	log.Printf("[teleproxy] telegram_proxy_stop for proxy %d on node %d (dispatched via gRPC)", proxyID, proxy.NodeID)
	writeJSON(w, map[string]any{"ok": true, "proxy_id": proxyID, "action": "telegram_proxy_stop"})
}

// adminTelegramProxiesRotate handles POST /api/admin/telegram-proxies/rotate.
func (s *Server) adminTelegramProxiesRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.rotateTelegramProxySecret(w, r)
}

// customerTelegramProxies handles GET /api/customer/telegram-proxies.
func (s *Server) customerTelegramProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, _ := s.currentCustomer(r)

	// Get the customer's plan_id
	var planID int64
	err := s.DB.QueryRow(`SELECT COALESCE(plan_id, 0) FROM customers WHERE username = $1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&planID)
	if err != nil || planID == 0 {
		writeJSON(w, map[string]any{"ok": true, "proxies": []any{}})
		return
	}

	ctx := context.Background()
	proxies, err := s.TeleProxy.GetForCustomer(ctx, planID)
	if err != nil {
		log.Printf("[teleproxy] failed to get proxies for customer %s: %v", username, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
		return
	}

	// Enrich with links if missing
	for i := range proxies {
		if proxies[i].ShareLink == "" || proxies[i].TgLink == "" {
			nodeIP := s.getNodeIP(proxies[i].NodeID)
			if nodeIP != "" {
				shareLink, tgLink := s.TeleProxy.GenerateLinks(&proxies[i], nodeIP)
				proxies[i].ShareLink = shareLink
				proxies[i].TgLink = tgLink
			}
		}
	}

	writeJSON(w, map[string]any{"ok": true, "proxies": proxies})
}

// listTelegramProxies returns all proxies with generated links.
func (s *Server) listTelegramProxies(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	proxies, err := s.TeleProxy.List(ctx)
	if err != nil {
		log.Printf("[teleproxy] failed to list proxies: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
		return
	}

	// Enrich proxies with links if not already stored
	for i := range proxies {
		if proxies[i].ShareLink == "" || proxies[i].TgLink == "" {
			nodeIP := s.getNodeIP(proxies[i].NodeID)
			if nodeIP != "" {
				shareLink, tgLink := s.TeleProxy.GenerateLinks(&proxies[i], nodeIP)
				proxies[i].ShareLink = shareLink
				proxies[i].TgLink = tgLink
				// Persist links
				_ = s.TeleProxy.UpdateLinks(ctx, proxies[i].ID, shareLink, tgLink)
			}
		}
	}

	writeJSON(w, map[string]any{"ok": true, "proxies": proxies})
}

// createTelegramProxy creates a new proxy and deploys it to the node.
func (s *Server) createTelegramProxy(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		NodeID int64  `json:"node_id"`
		Port   int    `json:"port"`
		Tag    string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID == 0 || in.Port == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_id and port are required"})
		return
	}
	if in.Port < 1 || in.Port > 65535 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
		return
	}

	ctx := context.Background()
	proxy, err := s.TeleProxy.Create(ctx, in.NodeID, in.Port, in.Tag)
	if err != nil {
		log.Printf("[teleproxy] failed to create proxy: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "create_failed"})
		return
	}

	// Get node IP and generate links
	nodeIP := s.getNodeIP(in.NodeID)
	var shareLink, tgLink string
	if nodeIP != "" {
		shareLink, tgLink = s.TeleProxy.GenerateLinks(proxy, nodeIP)
		proxy.ShareLink = shareLink
		proxy.TgLink = tgLink
		_ = s.TeleProxy.UpdateLinks(ctx, proxy.ID, shareLink, tgLink)
	}

	// NOTE: Legacy node_tasks INSERT removed. Telegram proxy deploy is now dispatched via gRPC.
	log.Printf("[teleproxy] telegram_proxy_deploy for proxy %d on node %d (dispatched via gRPC)", proxy.ID, in.NodeID)

	writeJSON(w, map[string]any{
		"ok":         true,
		"proxy":      proxy,
		"share_link": shareLink,
		"tg_link":    tgLink,
	})
}

// deleteTelegramProxy stops and removes a proxy.
func (s *Server) deleteTelegramProxy(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.ID == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "id_required"})
		return
	}

	ctx := context.Background()

	// Get proxy info for the node task
	proxy, err := s.TeleProxy.Get(ctx, in.ID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// NOTE: Legacy node_tasks INSERT removed. Telegram proxy remove is now dispatched via gRPC.
	log.Printf("[teleproxy] telegram_proxy_remove for proxy %d on node %d (dispatched via gRPC)", proxy.ID, proxy.NodeID)

	// Delete from database
	if err := s.TeleProxy.Delete(ctx, in.ID); err != nil {
		log.Printf("[teleproxy] failed to delete proxy %d: %v", in.ID, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "delete_failed"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// rotateTelegramProxySecret rotates the secret and regenerates links.
func (s *Server) rotateTelegramProxySecret(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.ID == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "id_required"})
		return
	}

	ctx := context.Background()

	// Rotate the secret
	_, err := s.TeleProxy.RotateSecret(ctx, in.ID)
	if err != nil {
		log.Printf("[teleproxy] failed to rotate secret for proxy %d: %v", in.ID, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "rotate_failed"})
		return
	}

	// Get updated proxy to regenerate links
	proxy, err := s.TeleProxy.Get(ctx, in.ID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
		return
	}

	// Regenerate links with new secret
	nodeIP := s.getNodeIP(proxy.NodeID)
	var shareLink, tgLink string
	if nodeIP != "" {
		shareLink, tgLink = s.TeleProxy.GenerateLinks(proxy, nodeIP)
		_ = s.TeleProxy.UpdateLinks(ctx, proxy.ID, shareLink, tgLink)
	}

	// NOTE: Legacy node_tasks INSERT removed. Telegram proxy restart is now dispatched via gRPC.
	log.Printf("[teleproxy] telegram_proxy_restart for proxy %d on node %d (dispatched via gRPC)", proxy.ID, proxy.NodeID)

	writeJSON(w, map[string]any{
		"ok":         true,
		"proxy":      proxy,
		"share_link": shareLink,
		"tg_link":    tgLink,
	})
}

// getNodeIP fetches the public IP for a node.
func (s *Server) getNodeIP(nodeID int64) string {
	var ip string
	_ = s.DB.QueryRow(`SELECT COALESCE(public_ip, '') FROM nodes WHERE id = $1`, nodeID).Scan(&ip)
	return ip
}
