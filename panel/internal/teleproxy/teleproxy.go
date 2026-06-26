//go:build !lite

// Package teleproxy provides Telegram MTProto proxy management for KorisPanel.
// It handles proxy lifecycle, link generation, secret rotation, and plan assignment.
package teleproxy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

// Proxy represents a Telegram MTProto proxy instance on a node.
type Proxy struct {
	ID               int64      `json:"id"`
	NodeID           int64      `json:"node_id"`
	Port             int        `json:"port"`
	Secret           string     `json:"secret"`
	Tag              string     `json:"tag,omitempty"`
	Status           string     `json:"status"`
	ShareLink        string     `json:"share_link,omitempty"`
	TgLink           string     `json:"tg_link,omitempty"`
	ConnectionsCount int        `json:"connections_count"`
	LastHealthCheck  *time.Time `json:"last_health_check,omitempty"`
	PlanIDs          []int64    `json:"plan_ids,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ProxyConfig holds deployment configuration for a Telegram proxy on a node.
type ProxyConfig struct {
	ProxyID int64  `json:"proxy_id"`
	NodeID  int64  `json:"node_id"`
	Port    int    `json:"port"`
	Secret  string `json:"secret"`
	Tag     string `json:"tag,omitempty"`
}

// ProxyService manages Telegram proxy operations.
type ProxyService struct {
	db     *sql.DB
	notify func(string)
}

// New creates a new ProxyService with the given database connection.
func New(db *sql.DB) *ProxyService {
	return &ProxyService{
		db:     db,
		notify: func(msg string) { log.Printf("[teleproxy] %s", msg) },
	}
}

// SetNotify sets a custom notification function for proxy events.
func (s *ProxyService) SetNotify(fn func(string)) {
	if fn != nil {
		s.notify = fn
	}
}

// generateSecret creates a random 32-character hex secret for MTProto proxy.
func generateSecret() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Create creates a new Telegram proxy entry and generates a secret.
func (s *ProxyService) Create(ctx context.Context, nodeID int64, port int, tag string) (*Proxy, error) {
	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO telegram_proxies (node_id, port, secret, tag, status)
		VALUES ($1, $2, $3, $4, 'stopped')`,
		nodeID, port, secret, tag,
	)
	if err != nil {
		return nil, fmt.Errorf("insert proxy: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get proxy id: %w", err)
	}

	proxy := &Proxy{
		ID:        id,
		NodeID:    nodeID,
		Port:      port,
		Secret:    secret,
		Tag:       tag,
		Status:    "stopped",
		CreatedAt: time.Now().UTC(),
	}

	log.Printf("[teleproxy] created proxy id=%d on node=%d port=%d", id, nodeID, port)
	return proxy, nil
}

// List returns all Telegram proxies ordered by creation date.
func (s *ProxyService) List(ctx context.Context) ([]Proxy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, node_id, port, secret, tag, status, share_link, tg_link,
		       connections_count, last_health_check, plan_ids, created_at
		FROM telegram_proxies
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list proxies: %w", err)
	}
	defer rows.Close()

	return s.scanProxies(rows)
}

// Get retrieves a single proxy by ID.
func (s *ProxyService) Get(ctx context.Context, proxyID int64) (*Proxy, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, node_id, port, secret, tag, status, share_link, tg_link,
		       connections_count, last_health_check, plan_ids, created_at
		FROM telegram_proxies WHERE id = $1`, proxyID)

	proxy, err := s.scanProxy(row)
	if err != nil {
		return nil, fmt.Errorf("get proxy %d: %w", proxyID, err)
	}
	return proxy, nil
}

// Delete removes a proxy by ID.
func (s *ProxyService) Delete(ctx context.Context, proxyID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM telegram_proxies WHERE id = $1`, proxyID)
	if err != nil {
		return fmt.Errorf("delete proxy %d: %w", proxyID, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("proxy %d not found", proxyID)
	}

	log.Printf("[teleproxy] deleted proxy id=%d", proxyID)
	return nil
}

// UpdateStatus updates the status of a proxy (active, stopped, error).
func (s *ProxyService) UpdateStatus(ctx context.Context, proxyID int64, status string) error {
	if status != "active" && status != "stopped" && status != "error" {
		return fmt.Errorf("invalid status: %s", status)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE telegram_proxies SET status = $1 WHERE id = $2`, status, proxyID)
	if err != nil {
		return fmt.Errorf("update proxy status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("proxy %d not found", proxyID)
	}

	log.Printf("[teleproxy] updated proxy id=%d status=%s", proxyID, status)
	return nil
}

// GenerateLinks creates the tg:// and https://t.me/ share links for a proxy.
func (s *ProxyService) GenerateLinks(proxy *Proxy, nodeIP string) (shareLink, tgLink string) {
	tgLink = fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s",
		nodeIP, proxy.Port, proxy.Secret)
	shareLink = fmt.Sprintf("https://t.me/proxy?server=%s&port=%d&secret=%s",
		nodeIP, proxy.Port, proxy.Secret)
	return shareLink, tgLink
}

// RotateSecret generates a new secret for a proxy and updates the database.
func (s *ProxyService) RotateSecret(ctx context.Context, proxyID int64) (string, error) {
	newSecret, err := generateSecret()
	if err != nil {
		return "", fmt.Errorf("generate new secret: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE telegram_proxies SET secret = $1, share_link = NULL, tg_link = NULL WHERE id = $2`,
		newSecret, proxyID)
	if err != nil {
		return "", fmt.Errorf("rotate secret: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return "", fmt.Errorf("proxy %d not found", proxyID)
	}

	log.Printf("[teleproxy] rotated secret for proxy id=%d", proxyID)
	s.notify(fmt.Sprintf("secret rotated for proxy %d", proxyID))
	return newSecret, nil
}

// AssignToPlans associates a proxy with a set of plan IDs.
func (s *ProxyService) AssignToPlans(ctx context.Context, proxyID int64, planIDs []int64) error {
	planJSON, err := json.Marshal(planIDs)
	if err != nil {
		return fmt.Errorf("marshal plan IDs: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE telegram_proxies SET plan_ids = $1 WHERE id = $2`, string(planJSON), proxyID)
	if err != nil {
		return fmt.Errorf("assign plans: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("proxy %d not found", proxyID)
	}

	log.Printf("[teleproxy] assigned proxy id=%d to plans %v", proxyID, planIDs)
	return nil
}

// GetForCustomer returns all active proxies accessible by a customer's plan.
func (s *ProxyService) GetForCustomer(ctx context.Context, customerPlanID int64) ([]Proxy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, node_id, port, secret, tag, status, share_link, tg_link,
		       connections_count, last_health_check, plan_ids, created_at
		FROM telegram_proxies
		WHERE status = 'active'
		  AND (plan_ids IS NULL OR JSON_CONTAINS(plan_ids, CAST($1 AS JSON)))
		ORDER BY created_at DESC`, customerPlanID)
	if err != nil {
		return nil, fmt.Errorf("get proxies for plan %d: %w", customerPlanID, err)
	}
	defer rows.Close()

	return s.scanProxies(rows)
}

// UpdateLinks persists generated share links for a proxy.
func (s *ProxyService) UpdateLinks(ctx context.Context, proxyID int64, shareLink, tgLink string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE telegram_proxies SET share_link = $1, tg_link = $2 WHERE id = $3`,
		shareLink, tgLink, proxyID)
	if err != nil {
		return fmt.Errorf("update links: %w", err)
	}
	return nil
}

// UpdateHealthCheck records a health check timestamp and connection count.
func (s *ProxyService) UpdateHealthCheck(ctx context.Context, proxyID int64, connections int) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE telegram_proxies
		SET last_health_check = $1, connections_count = $2
		WHERE id = $1`, now, connections, proxyID)
	if err != nil {
		return fmt.Errorf("update health check: %w", err)
	}
	return nil
}

// scanProxy scans a single proxy row.
func (s *ProxyService) scanProxy(row *sql.Row) (*Proxy, error) {
	var p Proxy
	var tag, shareLink, tgLink sql.NullString
	var lastHealth sql.NullTime
	var planJSON sql.NullString

	err := row.Scan(
		&p.ID, &p.NodeID, &p.Port, &p.Secret, &tag, &p.Status,
		&shareLink, &tgLink, &p.ConnectionsCount, &lastHealth,
		&planJSON, &p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if tag.Valid {
		p.Tag = tag.String
	}
	if shareLink.Valid {
		p.ShareLink = shareLink.String
	}
	if tgLink.Valid {
		p.TgLink = tgLink.String
	}
	if lastHealth.Valid {
		p.LastHealthCheck = &lastHealth.Time
	}
	if planJSON.Valid && planJSON.String != "" {
		if err := json.Unmarshal([]byte(planJSON.String), &p.PlanIDs); err != nil {
			log.Printf("[teleproxy] warning: failed to parse plan_ids for proxy %d: %v", p.ID, err)
		}
	}

	return &p, nil
}

// scanProxies scans multiple proxy rows.
func (s *ProxyService) scanProxies(rows *sql.Rows) ([]Proxy, error) {
	var proxies []Proxy
	for rows.Next() {
		var p Proxy
		var tag, shareLink, tgLink sql.NullString
		var lastHealth sql.NullTime
		var planJSON sql.NullString

		if err := rows.Scan(
			&p.ID, &p.NodeID, &p.Port, &p.Secret, &tag, &p.Status,
			&shareLink, &tgLink, &p.ConnectionsCount, &lastHealth,
			&planJSON, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan proxy row: %w", err)
		}

		if tag.Valid {
			p.Tag = tag.String
		}
		if shareLink.Valid {
			p.ShareLink = shareLink.String
		}
		if tgLink.Valid {
			p.TgLink = tgLink.String
		}
		if lastHealth.Valid {
			p.LastHealthCheck = &lastHealth.Time
		}
		if planJSON.Valid && planJSON.String != "" {
			if err := json.Unmarshal([]byte(planJSON.String), &p.PlanIDs); err != nil {
				log.Printf("[teleproxy] warning: failed to parse plan_ids for proxy %d: %v", p.ID, err)
			}
		}

		proxies = append(proxies, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate proxy rows: %w", err)
	}

	return proxies, nil
}

// CheckHealth queries all telegram proxies with their node's public IP and
// performs a TCP connect health check on each. If the connection succeeds
// within 5 seconds, the proxy status is set to 'active' and the health check
// timestamp is recorded. If it fails, the status is set to 'error'.
// This function is intended to be called from the background worker tick.
func CheckHealth(db *sql.DB) {
	type proxyNode struct {
		ProxyID int64
		NodeIP  string
		Port    int
	}

	rows, err := db.Query(`
		SELECT tp.id, n.public_ip, tp.port
		FROM telegram_proxies tp
		JOIN nodes n ON n.id = tp.node_id
		WHERE tp.status IN ('active', 'stopped', 'error')`)
	if err != nil {
		log.Printf("[teleproxy] health check query failed: %v", err)
		return
	}
	defer rows.Close()

	var proxies []proxyNode
	for rows.Next() {
		var p proxyNode
		if err := rows.Scan(&p.ProxyID, &p.NodeIP, &p.Port); err != nil {
			log.Printf("[teleproxy] health check scan error: %v", err)
			continue
		}
		proxies = append(proxies, p)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[teleproxy] health check rows error: %v", err)
		return
	}

	for _, p := range proxies {
		addr := net.JoinHostPort(p.NodeIP, fmt.Sprintf("%d", p.Port))
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		now := time.Now().UTC()

		if err != nil {
			// Connection failed — mark as error
			db.Exec(`UPDATE telegram_proxies SET status = 'error', last_health_check = $1 WHERE id = $2`, now, p.ProxyID)
			log.Printf("[teleproxy] health check failed for proxy %d (%s): %v", p.ProxyID, addr, err)
		} else {
			conn.Close()
			// Connection succeeded — mark as active and record health check
			db.Exec(`UPDATE telegram_proxies SET status = 'active', last_health_check = $1 WHERE id = $2`, now, p.ProxyID)
		}
	}
}
