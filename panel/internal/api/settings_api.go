package api

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ========== Settings API Endpoints ==========

// metricsHistoryPoint is the structured response type for metrics history data points.
type metricsHistoryPoint struct {
	Ts    string  `json:"ts"`
	CPU   float64 `json:"cpu"`
	RAM   float64 `json:"ram"`
	Disk  float64 `json:"disk"`
	RxBps int64   `json:"rx_bps"`
	TxBps int64   `json:"tx_bps"`
}

// handleSettingsOverview returns aggregated panel settings.
// GET /api/admin/settings/overview
func (s *Server) handleSettingsOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// --- Database info ---
	dbBackend := s.Config.DBBackend
	dbConnected := s.DB.Ping() == nil
	var dbVersion string
	_ = s.DB.QueryRow(`SELECT version()`).Scan(&dbVersion)

	database := map[string]any{
		"backend":   dbBackend,
		"connected": dbConnected,
		"version":   dbVersion,
	}

	// --- TLS info ---
	tlsMode := s.Config.TLSMode
	tlsDomain := s.Config.TLSDomain
	var tlsExpiresAt string
	var tlsIssuer string

	// Try to read the certificate to extract expiry and issuer
	certPath := s.Config.TLSCert
	if certData, err := os.ReadFile(certPath); err == nil {
		block, _ := pem.Decode(certData)
		if block != nil {
			if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
				tlsExpiresAt = cert.NotAfter.UTC().Format(time.RFC3339)
				tlsIssuer = cert.Issuer.CommonName
				if tlsIssuer == "" && len(cert.Issuer.Organization) > 0 {
					tlsIssuer = cert.Issuer.Organization[0]
				}
			}
		}
	}

	tlsInfo := map[string]any{
		"mode":       tlsMode,
		"domain":     tlsDomain,
		"expires_at": tlsExpiresAt,
		"issuer":     tlsIssuer,
	}

	// --- Workers info ---
	workers := map[string]any{
		"configured":        s.Config.Workers,
		"active":            1, // Currently single worker
		"leader_id":         s.Config.WorkerID,
		"current_worker_id": s.Config.WorkerID,
	}

	// --- Alert thresholds (from DB, fallback to config defaults) ---
	cpuThreshold := s.Config.AlertCPUThreshold
	ramThreshold := s.Config.AlertRAMThreshold
	diskThreshold := s.Config.AlertDiskThreshold

	// Try to read overrides from panel_settings
	var v string
	if err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, "alert_cpu_threshold").Scan(&v); err == nil {
		if n, err := strconv.Atoi(v); err == nil {
			cpuThreshold = n
		}
	}
	if err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, "alert_ram_threshold").Scan(&v); err == nil {
		if n, err := strconv.Atoi(v); err == nil {
			ramThreshold = n
		}
	}
	if err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, "alert_disk_threshold").Scan(&v); err == nil {
		if n, err := strconv.Atoi(v); err == nil {
			diskThreshold = n
		}
	}

	alerts := map[string]any{
		"cpu_threshold":  cpuThreshold,
		"ram_threshold":  ramThreshold,
		"disk_threshold": diskThreshold,
	}

	// --- gRPC params (from DB, fallback to config defaults) ---
	connectTimeout := int(s.Config.GRPCConnectTimeout.Seconds())
	keepaliveInterval := int(s.Config.GRPCKeepaliveInterval.Seconds())
	metricsInterval := int(s.Config.GRPCMetricsInterval.Seconds())

	if err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, "grpc_connect_timeout").Scan(&v); err == nil {
		if n, err := strconv.Atoi(v); err == nil {
			connectTimeout = n
		}
	}
	if err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, "grpc_keepalive_interval").Scan(&v); err == nil {
		if n, err := strconv.Atoi(v); err == nil {
			keepaliveInterval = n
		}
	}
	if err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, "grpc_metrics_interval").Scan(&v); err == nil {
		if n, err := strconv.Atoi(v); err == nil {
			metricsInterval = n
		}
	}

	grpc := map[string]any{
		"connect_timeout":    connectTimeout,
		"keepalive_interval": keepaliveInterval,
		"metrics_interval":   metricsInterval,
	}

	// --- Panel info ---
	uptimeSeconds := int64(time.Since(s.StartedAt).Seconds())

	// Get migration version from DB
	var migrationVersion int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&migrationVersion)

	panelInfo := map[string]any{
		"version":           s.Config.Version,
		"edition":           PanelEdition,
		"uptime":            uptimeSeconds,
		"go_version":        runtime.Version(),
		"migration_version": migrationVersion,
	}

	writeJSON(w, map[string]any{
		"ok": true,
		"settings": map[string]any{
			"database":   database,
			"tls":        tlsInfo,
			"workers":    workers,
			"alerts":     alerts,
			"grpc":       grpc,
			"panel_info": panelInfo,
		},
	})
}

// handleSettingsAlerts handles POST /api/admin/settings/alerts.
// Accepts: {"cpu": 90, "ram": 85, "disk": 90} — validates each value 1-100.
func (s *Server) handleSettingsAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		CPU  *int `json:"cpu"`
		RAM  *int `json:"ram"`
		Disk *int `json:"disk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate
	if in.CPU == nil || in.RAM == nil || in.Disk == nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}
	if *in.CPU < 1 || *in.CPU > 100 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_cpu_threshold"})
		return
	}
	if *in.RAM < 1 || *in.RAM > 100 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_ram_threshold"})
		return
	}
	if *in.Disk < 1 || *in.Disk > 100 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_disk_threshold"})
		return
	}

	// Store in panel_settings
	upsertQuery := `INSERT INTO panel_settings(setting_key, setting_value, updated_at) VALUES($1, $2, NOW()) ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value, updated_at = NOW()`

	if _, err := s.DB.Exec(upsertQuery, "alert_cpu_threshold", strconv.Itoa(*in.CPU)); err != nil {
		log.Printf("[settings] failed to save alert_cpu_threshold: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if _, err := s.DB.Exec(upsertQuery, "alert_ram_threshold", strconv.Itoa(*in.RAM)); err != nil {
		log.Printf("[settings] failed to save alert_ram_threshold: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if _, err := s.DB.Exec(upsertQuery, "alert_disk_threshold", strconv.Itoa(*in.Disk)); err != nil {
		log.Printf("[settings] failed to save alert_disk_threshold: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	log.Printf("[settings] alert thresholds updated: cpu=%d, ram=%d, disk=%d", *in.CPU, *in.RAM, *in.Disk)
	writeJSON(w, map[string]any{"ok": true})
}

// handleSettingsGrpc handles POST /api/admin/settings/grpc.
// Accepts: {"connect_timeout": 10, "keepalive_interval": 30, "metrics_interval": 60}
// All values are positive integers (seconds).
func (s *Server) handleSettingsGrpc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		ConnectTimeout    *int `json:"connect_timeout"`
		KeepaliveInterval *int `json:"keepalive_interval"`
		MetricsInterval   *int `json:"metrics_interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate — all fields required, positive integers
	if in.ConnectTimeout == nil || in.KeepaliveInterval == nil || in.MetricsInterval == nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}
	if *in.ConnectTimeout <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_connect_timeout"})
		return
	}
	if *in.KeepaliveInterval <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_keepalive_interval"})
		return
	}
	if *in.MetricsInterval <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_metrics_interval"})
		return
	}

	// Store in panel_settings
	upsertQuery := `INSERT INTO panel_settings(setting_key, setting_value, updated_at) VALUES($1, $2, NOW()) ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value, updated_at = NOW()`

	if _, err := s.DB.Exec(upsertQuery, "grpc_connect_timeout", strconv.Itoa(*in.ConnectTimeout)); err != nil {
		log.Printf("[settings] failed to save grpc_connect_timeout: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if _, err := s.DB.Exec(upsertQuery, "grpc_keepalive_interval", strconv.Itoa(*in.KeepaliveInterval)); err != nil {
		log.Printf("[settings] failed to save grpc_keepalive_interval: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if _, err := s.DB.Exec(upsertQuery, "grpc_metrics_interval", strconv.Itoa(*in.MetricsInterval)); err != nil {
		log.Printf("[settings] failed to save grpc_metrics_interval: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	log.Printf("[settings] gRPC params updated: connect_timeout=%d, keepalive_interval=%d, metrics_interval=%d",
		*in.ConnectTimeout, *in.KeepaliveInterval, *in.MetricsInterval)

	writeJSON(w, map[string]any{"ok": true, "restart_required": true})
}

// handleSettingsTLSUpload handles POST /api/admin/settings/tls/upload.
// Accepts: {"cert_pem": "...", "key_pem": "..."} — validates PEM format
// and writes to the configured TLS cert/key paths.
func (s *Server) handleSettingsTLSUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		CertPEM string `json:"cert_pem"`
		KeyPEM  string `json:"key_pem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if strings.TrimSpace(in.CertPEM) == "" || strings.TrimSpace(in.KeyPEM) == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	// Validate PEM format — cert
	certBlock, _ := pem.Decode([]byte(in.CertPEM))
	if certBlock == nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_cert_pem"})
		return
	}

	// Validate PEM format — key
	keyBlock, _ := pem.Decode([]byte(in.KeyPEM))
	if keyBlock == nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_key_pem"})
		return
	}

	// Validate that the cert and key form a valid pair
	if _, err := tls.X509KeyPair([]byte(in.CertPEM), []byte(in.KeyPEM)); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": fmt.Sprintf("cert_key_mismatch: %v", err)})
		return
	}

	// Write to configured paths
	certPath := s.Config.TLSCert
	keyPath := s.Config.TLSKey

	if err := os.WriteFile(certPath, []byte(in.CertPEM), 0644); err != nil {
		log.Printf("[settings] failed to write TLS cert to %s: %v", certPath, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "write_cert_failed"})
		return
	}

	if err := os.WriteFile(keyPath, []byte(in.KeyPEM), 0600); err != nil {
		log.Printf("[settings] failed to write TLS key to %s: %v", keyPath, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "write_key_failed"})
		return
	}

	log.Printf("[settings] TLS certificate uploaded and written to %s, %s", certPath, keyPath)
	writeJSON(w, map[string]any{"ok": true})
}

// handleNodeMetricsHistory handles GET /api/admin/nodes/{id}/metrics/history?range=24h.
// Queries node_metrics_history for the given node and time range, downsamples to ~60 points.
func (s *Server) handleNodeMetricsHistory(w http.ResponseWriter, r *http.Request, nodeID int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse range parameter
	rangeStr := r.URL.Query().Get("range")
	var duration time.Duration
	switch rangeStr {
	case "1h":
		duration = 1 * time.Hour
	case "6h":
		duration = 6 * time.Hour
	case "24h", "":
		duration = 24 * time.Hour
	default:
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_range"})
		return
	}

	// Calculate time boundary
	since := time.Now().UTC().Add(-duration)

	// Query all metrics in the time range for this node
	rows, err := s.DB.Query(`
		SELECT time, cpu_percent, ram_percent, disk_percent, rx_bps, tx_bps
		FROM node_metrics_history
		WHERE node_id = $1 AND time >= $2
		ORDER BY time ASC
	`, nodeID, since)
	if err != nil {
		log.Printf("[settings] metrics history query failed for node %d: %v", nodeID, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	var allPoints []metricsHistoryPoint
	for rows.Next() {
		var t time.Time
		var cpu, ram, disk float64
		var rxBps, txBps int64
		if err := rows.Scan(&t, &cpu, &ram, &disk, &rxBps, &txBps); err != nil {
			log.Printf("[settings] metrics history scan error: %v", err)
			continue
		}
		allPoints = append(allPoints, metricsHistoryPoint{
			Ts:    t.UTC().Format(time.RFC3339),
			CPU:   cpu,
			RAM:   ram,
			Disk:  disk,
			RxBps: rxBps,
			TxBps: txBps,
		})
	}

	// Downsample to ~60 points
	const targetPoints = 60
	data := downsampleMetrics(allPoints, targetPoints)

	writeJSON(w, map[string]any{
		"ok":   true,
		"data": data,
	})
}

// downsampleMetrics reduces a slice of metric points to approximately targetN points
// by averaging values within each bucket.
func downsampleMetrics(points []metricsHistoryPoint, targetN int) []metricsHistoryPoint {
	if len(points) <= targetN {
		return points
	}

	bucketSize := len(points) / targetN
	if bucketSize < 1 {
		bucketSize = 1
	}

	var result []metricsHistoryPoint
	for i := 0; i < len(points); i += bucketSize {
		end := i + bucketSize
		if end > len(points) {
			end = len(points)
		}
		bucket := points[i:end]

		var sumCPU, sumRAM, sumDisk float64
		var sumRx, sumTx int64
		for _, p := range bucket {
			sumCPU += p.CPU
			sumRAM += p.RAM
			sumDisk += p.Disk
			sumRx += p.RxBps
			sumTx += p.TxBps
		}

		n := float64(len(bucket))
		// Use the timestamp of the middle point in the bucket
		midIdx := len(bucket) / 2
		result = append(result, metricsHistoryPoint{
			Ts:    bucket[midIdx].Ts,
			CPU:   sumCPU / n,
			RAM:   sumRAM / n,
			Disk:  sumDisk / n,
			RxBps: int64(float64(sumRx) / n),
			TxBps: int64(float64(sumTx) / n),
		})
	}

	return result
}
