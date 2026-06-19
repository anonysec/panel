package main

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"KorisPanel/node/internal/config"
	"KorisPanel/node/internal/logger"
	"KorisPanel/node/internal/updater"
)

const agentVersion = "0.36.0"

type DiagnosticsReport struct {
	AgentVersion  string `json:"agent_version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	GoVersion     string `json:"go_version"`
	Goroutines    int    `json:"goroutines"`
	MemAllocBytes int64  `json:"mem_alloc_bytes"`
}

// WireGuardPeerStat holds per-peer statistics from `wg show wg0 dump`.
type WireGuardPeerStat struct {
	PublicKey       string `json:"public_key"`
	Endpoint        string `json:"endpoint"`
	AllowedIPs      string `json:"allowed_ips"`
	LatestHandshake int64  `json:"latest_handshake"`
	RxBytes         int64  `json:"rx_bytes"`
	TxBytes         int64  `json:"tx_bytes"`
	Active          bool   `json:"active"`
}

type Push struct {
	Token                string              `json:"token"`
	Type                 string              `json:"type"`
	Hostname             string              `json:"hostname"`
	PublicIP             string              `json:"public_ip"`
	PublicIPv6           string              `json:"public_ipv6"`
	OS                   string              `json:"os"`
	Timestamp            time.Time           `json:"timestamp"`
	CPUPercent           float64             `json:"cpu_percent"`
	RAMPercent           float64             `json:"ram_percent"`
	DiskPercent          float64             `json:"disk_percent"`
	RxBps                int64               `json:"rx_bps"`
	TxBps                int64               `json:"tx_bps"`
	RxBytes              int64               `json:"rx_bytes"`
	TxBytes              int64               `json:"tx_bytes"`
	OnlineUsers          int                 `json:"online_users"`
	OpenVPNStatus        string              `json:"openvpn_status"`
	L2TPStatus           string              `json:"l2tp_status"`
	IKEv2Status          string              `json:"ikev2_status"`
	WireGuardStatus      string              `json:"wireguard_status"`
	Services             map[string]string   `json:"services"`
	Diagnostics          *DiagnosticsReport  `json:"diagnostics,omitempty"`
	PerUserBandwidth     []UserBandwidth     `json:"per_user_bandwidth,omitempty"`
	WireGuardPeers       []WireGuardPeerStat `json:"wireguard_peers,omitempty"`
	WireGuardActivePeers int                 `json:"wireguard_active_peers"`
}

type Task struct {
	ID      int64           `json:"id"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload_json"`
}

type TaskPollResponse struct {
	OK    bool   `json:"ok"`
	Tasks []Task `json:"tasks"`
}

func main() {
	// Load configuration from env file or environment variables
	envFile := getenv("NODE_ENV_FILE", "/etc/panel-node/node.env")
	cfg, err := config.Load(envFile)
	if err != nil {
		// Fallback: try loading from environment variables directly
		cfg = loadConfigFromEnv()
		if cfg == nil {
			fmt.Fprintf(os.Stderr, `{"timestamp":"%s","level":"error","message":"failed to load configuration","fields":{"error":"%s"}}`+"\n",
				time.Now().UTC().Format(time.RFC3339), err.Error())
			os.Exit(1)
		}
	}

	// Initialize structured logger from config
	logLevel := logger.ParseLevel(cfg.GetLogLevel())
	log := logger.New(logLevel)

	panel := cfg.GetPanelURL()
	token := cfg.GetNodeToken()
	if token == "" {
		log.Error("NODE_TOKEN is required")
		os.Exit(1)
	}
	if panel == "" {
		panel = "http://127.0.0.1:8080"
	}

	interval := cfg.GetInterval()
	if interval < 3 {
		interval = 3
	}
	intervalDuration := time.Duration(interval) * time.Second
	client := &http.Client{Timeout: 20 * time.Second}

	log.Info("node agent starting", map[string]any{
		"panel_url":    panel,
		"interval_sec": interval,
		"log_level":    cfg.GetLogLevel(),
		"version":      agentVersion,
	})

	// Record startup time for diagnostics uptime calculation.
	startTime := time.Now()

	// Graceful shutdown: listen for SIGTERM and SIGINT.
	done := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		log.Info("shutting down gracefully", map[string]any{
			"signal": sig.String(),
		})
		close(done)
	}()

	// SIGHUP signal handler for config hot-reload.
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			log.Info("received SIGHUP, reloading configuration")
			changes, err := cfg.Reload(envFile)
			if err != nil {
				log.Error("config reload failed", map[string]any{
					"error": err.Error(),
				})
				continue
			}
			for key, vals := range changes {
				log.Info("config key changed", map[string]any{
					"key":       key,
					"old_value": vals[0],
					"new_value": vals[1],
				})
			}
			if newLevel, ok := changes["LOG_LEVEL"]; ok {
				log.SetLevel(logger.ParseLevel(newLevel[1]))
				log.Info("log level updated", map[string]any{
					"level": newLevel[1],
				})
			}
			if len(changes) == 0 {
				log.Info("config reload complete, no changes detected")
			}
		}
	}()

	// Auto-update goroutine: checks for new agent version on startup and every 6 hours.
	agentUpdater := updater.New(panel, token, agentVersion, client)
	go func() {
		// Run update check immediately on startup.
		if cfg.GetAutoUpdate() {
			log.Info("running initial auto-update check")
			if err := agentUpdater.CheckAndUpdate(); err != nil {
				log.Error("auto-update check failed", map[string]any{
					"error": err.Error(),
				})
			} else {
				log.Info("auto-update check complete, already up to date")
			}
		} else {
			log.Info("auto-update is disabled, skipping initial check")
		}

		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if !cfg.GetAutoUpdate() {
				log.Debug("auto-update is disabled, skipping scheduled check")
				continue
			}
			log.Info("running scheduled auto-update check")
			if err := agentUpdater.CheckAndUpdate(); err != nil {
				log.Error("auto-update check failed", map[string]any{
					"error": err.Error(),
				})
			} else {
				log.Info("auto-update check complete, already up to date")
			}
		}
	}()

	// Consecutive failure tracking state for push endpoint.
	tracker := NewFailureTracker(3, log)

	collector := NewBandwidthCollector()
	lastRx, lastTx := netBytes()
	lastAt := time.Now()
	for {
		// Check if shutdown was requested
		select {
		case <-done:
			log.Info("main loop exiting")
			return
		default:
		}

		nowRx, nowTx := netBytes()
		now := time.Now()
		dt := now.Sub(lastAt).Seconds()
		if dt <= 0 {
			dt = intervalDuration.Seconds()
		}
		host, _ := os.Hostname()
		services := map[string]string{
			"openvpn":   serviceStatus("openvpn"),
			"l2tp":      serviceStatus("xl2tpd"),
			"ikev2":     serviceStatus("strongswan"),
			"ssh":       serviceStatus("ssh"),
			"wireguard": serviceStatus("wg-quick@wg0"),
		}
		push := Push{
			Token:           token,
			Type:            "status",
			Hostname:        host,
			PublicIP:        firstIP(),
			PublicIPv6:      firstIPv6(),
			OS:              runtime.GOOS,
			Timestamp:       now.UTC(),
			CPUPercent:      cpuPercent(),
			RAMPercent:      memPercent(),
			DiskPercent:     diskPercent("/"),
			RxBytes:         nowRx,
			TxBytes:         nowTx,
			RxBps:           int64(float64(nowRx-lastRx) / dt),
			TxBps:           int64(float64(nowTx-lastTx) / dt),
			OnlineUsers:     onlineUsers(),
			OpenVPNStatus:   services["openvpn"],
			L2TPStatus:      services["l2tp"],
			IKEv2Status:     services["ikev2"],
			WireGuardStatus: services["wireguard"],
			Services:        services,
			Diagnostics:     buildDiagnostics(startTime, agentVersion),
		}

		// Collect per-user bandwidth from tc
		bw := collector.Collect("tun0")
		push.PerUserBandwidth = bw

		// Collect WireGuard peer statistics
		wgPeers, wgActive := collectWireGuardPeers()
		push.WireGuardPeers = wgPeers
		push.WireGuardActivePeers = wgActive

		ok, errMsg := postJSON(client, panel+"/api/node/push", token, push, log)
		if ok {
			tracker.RecordSuccess()
		} else {
			tracker.RecordFailure(errMsg)
		}

		pollTasks(client, panel, token, log, cfg, envFile, agentUpdater)
		lastRx, lastTx, lastAt = nowRx, nowTx, now

		// Sleep with graceful shutdown awareness
		select {
		case <-done:
			log.Info("main loop exiting")
			return
		case <-time.After(intervalDuration):
		}
	}
}

// loadConfigFromEnv creates a Config from environment variables directly
// when no env file is available.
func loadConfigFromEnv() *config.Config {
	token := os.Getenv("NODE_TOKEN")
	if token == "" {
		return nil
	}

	panelURL := strings.TrimRight(getenv("PANEL_URL", "http://127.0.0.1:8080"), "/")
	intervalSeconds, _ := strconv.Atoi(getenv("NODE_INTERVAL", "10"))
	if intervalSeconds <= 0 {
		intervalSeconds = 10
	}
	logLevel := getenv("LOG_LEVEL", "info")
	autoUpdate := getenv("NODE_AUTO_UPDATE", "true")

	// Write a temporary env file for the config package to load.
	// Note: we remove the file AFTER config.Load returns to avoid a race
	// condition where defer would delete the file before Load finishes reading it.
	tmpFile, err := os.CreateTemp("", "node-env-*.env")
	if err != nil {
		return nil
	}
	tmpPath := tmpFile.Name()

	content := fmt.Sprintf("NODE_TOKEN=%s\nPANEL_URL=%s\nNODE_INTERVAL=%d\nLOG_LEVEL=%s\nNODE_AUTO_UPDATE=%s\n",
		token, panelURL, intervalSeconds, logLevel, autoUpdate)
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil
	}
	tmpFile.Close()

	cfg, err := config.Load(tmpPath)
	os.Remove(tmpPath) // Clean up after config.Load has finished reading
	if err != nil {
		return nil
	}
	return cfg
}

// buildDiagnostics collects runtime diagnostics for inclusion in the status push.
func buildDiagnostics(startTime time.Time, version string) *DiagnosticsReport {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return &DiagnosticsReport{
		AgentVersion:  version,
		UptimeSeconds: int64(time.Since(startTime).Seconds()),
		GoVersion:     runtime.Version(),
		Goroutines:    runtime.NumGoroutine(),
		MemAllocBytes: int64(m.Alloc),
	}
}

func pollTasks(client *http.Client, panel, token string, log *logger.Logger, cfg *config.Config, envFile string, agentUpdater *updater.Updater) {
	url := panel + "/api/node/tasks/poll"
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Token", token)

	log.Debug("polling tasks", map[string]any{
		"url": url,
	})

	resp, err := client.Do(req)
	if err != nil {
		log.Error("task poll failed", map[string]any{
			"error": err.Error(),
			"url":   url,
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Warn("task poll non-2xx response", map[string]any{
			"url":    url,
			"status": resp.Status,
			"body":   string(body),
		})
		return
	}

	log.Debug("task poll response received", map[string]any{
		"url":    url,
		"status": resp.Status,
	})

	var out TaskPollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		log.Error("task poll decode failed", map[string]any{
			"error": err.Error(),
			"url":   url,
		})
		return
	}

	for _, task := range out.Tasks {
		log.Info("executing task", map[string]any{
			"task_id": task.ID,
			"action":  task.Action,
		})
		status, result, errText := executeTask(task, cfg, envFile, log, agentUpdater)
		log.Info("task completed", map[string]any{
			"task_id": task.ID,
			"action":  task.Action,
			"status":  status,
		})
		complete := map[string]any{"status": status, "result_json": result, "error": errText}
		_, _ = postJSON(client, fmt.Sprintf("%s/api/node/tasks/%d/complete", panel, task.ID), token, complete, log)
	}
}

func executeTask(task Task, cfg *config.Config, envFile string, log *logger.Logger, agentUpdater *updater.Updater) (string, map[string]any, string) {
	var payload map[string]any
	_ = json.Unmarshal(task.Payload, &payload)
	switch task.Action {
	case "agent.status":
		return "succeeded", map[string]any{"message": "agent alive", "time": time.Now().UTC()}, ""
	case "service.status":
		service := normalizeService(fmt.Sprint(payload["service"]))
		if service == "" {
			return "failed", map[string]any{}, "invalid service"
		}
		return "succeeded", map[string]any{"service": service, "status": serviceStatus(service)}, ""
	case "service.restart", "service.reload":
		service := normalizeService(fmt.Sprint(payload["service"]))
		if service == "" {
			return "failed", map[string]any{}, "invalid service"
		}
		verb := "restart"
		if task.Action == "service.reload" {
			verb = "reload"
		}
		unitName := serviceToUnit(service)
		cmd := exec.Command("systemctl", verb, unitName)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "failed", map[string]any{"service": service, "unit": unitName, "output": string(out)}, err.Error()
		}
		return "succeeded", map[string]any{"service": service, "output": string(out), "status": serviceStatus(service)}, ""
	case "service.stop":
		service := normalizeService(fmt.Sprint(payload["service"]))
		if service == "" {
			return "failed", map[string]any{}, "invalid service"
		}
		unitName := serviceToUnit(service)
		cmd := exec.Command("systemctl", "stop", unitName)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "failed", map[string]any{"service": service, "unit": unitName, "output": string(out)}, err.Error()
		}
		return "succeeded", map[string]any{"service": service, "output": string(out), "status": serviceStatus(service)}, ""
	case "agent.update":
		if agentUpdater == nil {
			return "failed", map[string]any{}, "updater not configured"
		}
		log.Info("manual update triggered via task")
		if err := agentUpdater.CheckAndUpdate(); err != nil {
			return "failed", map[string]any{"error": err.Error()}, err.Error()
		}
		return "succeeded", map[string]any{"message": "update check completed"}, ""
	case "vpn.disconnect-user":
		username := fmt.Sprint(payload["username"])
		if username == "" || username == "<nil>" {
			return "failed", map[string]any{}, "username is required"
		}
		result, err := disconnectVPNUser(username, log)
		if err != nil {
			return "failed", map[string]any{"username": username}, err.Error()
		}
		return "succeeded", map[string]any{"username": username, "result": result}, ""
	case "vpn.apply_outbound":
		extraJSONRaw, _ := json.Marshal(payload["extra_json"])
		protocol := fmt.Sprint(payload["protocol"])
		configDir := "/etc/openvpn/server"
		if protocol == "ikev2" || protocol == "strongswan" {
			configDir = "/etc/strongswan"
		} else if protocol == "ssh" {
			configDir = "/etc/ssh"
		}
		outCfg := parseOutboundConfig(json.RawMessage(extraJSONRaw))
		if outCfg == nil {
			return "succeeded", map[string]any{"message": "outbound not configured or disabled"}, ""
		}
		switch protocol {
		case "openvpn":
			directives, err := applyOutboundOpenVPN(outCfg, configDir)
			if err != nil {
				return "failed", map[string]any{}, err.Error()
			}
			// Write directives to include file
			includeFile := filepath.Join(configDir, "outbound.conf")
			if err := os.WriteFile(includeFile, []byte(strings.Join(directives, "\n")+"\n"), 0644); err != nil {
				return "failed", map[string]any{}, err.Error()
			}
			return "succeeded", map[string]any{"protocol": protocol, "directives": directives}, ""
		case "ikev2", "strongswan":
			if err := applyOutboundIKEv2(outCfg, configDir); err != nil {
				return "failed", map[string]any{}, err.Error()
			}
			return "succeeded", map[string]any{"protocol": protocol, "message": "outbound routing script installed"}, ""
		case "ssh":
			proxyCmd, err := applyOutboundSSH(outCfg, configDir)
			if err != nil {
				return "failed", map[string]any{}, err.Error()
			}
			return "succeeded", map[string]any{"protocol": protocol, "proxy_command": proxyCmd}, ""
		default:
			return "failed", map[string]any{}, "unsupported protocol for outbound: " + protocol
		}
	case "agent.reload_config":
		changes, err := cfg.Reload(envFile)
		if err != nil {
			log.Error("config reload failed via task", map[string]any{
				"error": err.Error(),
			})
			return "failed", map[string]any{}, err.Error()
		}
		for key, vals := range changes {
			log.Info("config key changed", map[string]any{
				"key":       key,
				"old_value": vals[0],
				"new_value": vals[1],
			})
		}
		if newLevel, ok := changes["LOG_LEVEL"]; ok {
			log.SetLevel(logger.ParseLevel(newLevel[1]))
			log.Info("log level updated", map[string]any{
				"level": newLevel[1],
			})
		}
		if len(changes) == 0 {
			log.Info("config reload complete via task, no changes detected")
		}
		changesList := make(map[string]any)
		for k, v := range changes {
			changesList[k] = map[string]string{"old": v[0], "new": v[1]}
		}
		return "succeeded", map[string]any{"changes": changesList}, ""

	case "agent.update_config":
		// Panel pushes config key-value pairs to the agent env file.
		// Accepts: { "config": { "KEY": "VALUE", ... } }
		// This allows the panel to manage NODE_NAME, PANEL_URL, etc.
		configMap, ok := payload["config"].(map[string]any)
		if !ok || len(configMap) == 0 {
			return "failed", map[string]any{}, "config map required"
		}
		// Read current env file
		envData, err := os.ReadFile(envFile)
		if err != nil {
			return "failed", map[string]any{}, "read env file: " + err.Error()
		}
		lines := strings.Split(string(envData), "\n")
		updated := map[string]bool{}
		// Update existing keys
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			eqIdx := strings.Index(trimmed, "=")
			if eqIdx == -1 {
				continue
			}
			key := trimmed[:eqIdx]
			if newVal, exists := configMap[key]; exists {
				lines[i] = fmt.Sprintf("%s=%s", key, fmt.Sprint(newVal))
				updated[key] = true
			}
		}
		// Append new keys that weren't in the file
		for key, val := range configMap {
			if !updated[key] {
				lines = append(lines, fmt.Sprintf("%s=%s", key, fmt.Sprint(val)))
			}
		}
		// Write back
		if err := os.WriteFile(envFile, []byte(strings.Join(lines, "\n")), 0600); err != nil {
			return "failed", map[string]any{}, "write env file: " + err.Error()
		}
		// Reload config
		cfg.Reload(envFile)
		log.Info("config updated via panel task", map[string]any{"keys": configMap})
		return "succeeded", map[string]any{"updated_keys": configMap}, ""
	case "wireguard.setup":
		return executeWireGuardSetup(payload)
	case "wireguard.add_peer":
		pubKey := fmt.Sprint(payload["public_key"])
		psk := fmt.Sprint(payload["preshared_key"])
		allowedIPs := fmt.Sprint(payload["allowed_ips"])
		if pubKey == "" || pubKey == "<nil>" {
			return "failed", map[string]any{}, "public_key is required"
		}
		if !isValidWireGuardKey(pubKey) {
			return "failed", map[string]any{}, "public_key must be a valid 44-character base64 WireGuard key"
		}
		if allowedIPs == "" || allowedIPs == "<nil>" {
			return "failed", map[string]any{}, "allowed_ips is required"
		}
		if !isValidAllowedIPs(allowedIPs) {
			return "failed", map[string]any{}, "allowed_ips must contain valid CIDR notation"
		}
		if psk != "" && psk != "<nil>" && !isValidWireGuardKey(psk) {
			return "failed", map[string]any{}, "preshared_key must be a valid 44-character base64 WireGuard key"
		}
		// Add peer to live interface
		args := []string{"set", "wg0", "peer", pubKey, "allowed-ips", allowedIPs}
		if psk != "" && psk != "<nil>" {
			// Use preshared-key via stdin
			cmd := exec.Command("wg", "set", "wg0", "peer", pubKey, "preshared-key", "/dev/stdin", "allowed-ips", allowedIPs)
			cmd.Stdin = strings.NewReader(psk)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return "failed", map[string]any{"output": string(out)}, err.Error()
			}
		} else {
			cmd := exec.Command("wg", args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return "failed", map[string]any{"output": string(out)}, err.Error()
			}
		}
		// Append peer to config file
		confFile := "/etc/wireguard/wg0.conf"
		// Read existing content to check if it already ends with a newline
		existingData, _ := os.ReadFile(confFile)
		prefix := "\n"
		if len(existingData) > 0 && existingData[len(existingData)-1] == '\n' {
			prefix = ""
		}
		peerBlock := fmt.Sprintf("%s[Peer]\nPublicKey = %s\nAllowedIPs = %s\n", prefix, pubKey, allowedIPs)
		if psk != "" && psk != "<nil>" {
			peerBlock = fmt.Sprintf("%s[Peer]\nPublicKey = %s\nPresharedKey = %s\nAllowedIPs = %s\n", prefix, pubKey, psk, allowedIPs)
		}
		f, err := os.OpenFile(confFile, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("open config: %s", err.Error())
		}
		_, err = f.WriteString(peerBlock)
		f.Close()
		if err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("write config: %s", err.Error())
		}
		return "succeeded", map[string]any{"public_key": pubKey, "allowed_ips": allowedIPs}, ""
	case "wireguard.remove_peer":
		pubKey := fmt.Sprint(payload["public_key"])
		if pubKey == "" || pubKey == "<nil>" {
			return "failed", map[string]any{}, "public_key is required"
		}
		if !isValidWireGuardKey(pubKey) {
			return "failed", map[string]any{}, "public_key must be a valid 44-character base64 WireGuard key"
		}
		// Remove peer from live interface
		cmd := exec.Command("wg", "set", "wg0", "peer", pubKey, "remove")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "failed", map[string]any{"output": string(out)}, err.Error()
		}
		// Remove peer from config file
		confFile := "/etc/wireguard/wg0.conf"
		confData, err := os.ReadFile(confFile)
		if err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("read config: %s", err.Error())
		}
		newConf := removePeerFromConfig(string(confData), pubKey)
		if err := os.WriteFile(confFile, []byte(newConf), 0600); err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("write config: %s", err.Error())
		}
		return "succeeded", map[string]any{"public_key": pubKey, "removed": true}, ""
	case "cert.distribute":
		certPath := fmt.Sprint(payload["cert_path"])
		certContent := fmt.Sprint(payload["cert_content"])
		if certPath == "" || certPath == "<nil>" {
			return "failed", map[string]any{}, "cert_path is required"
		}
		if certContent == "" || certContent == "<nil>" {
			return "failed", map[string]any{}, "cert_content is required"
		}
		// Path traversal protection: only allow known VPN config directories
		cleanPath := filepath.Clean(certPath)
		allowedPrefixes := []string{"/etc/openvpn/", "/etc/wireguard/", "/etc/strongswan/"}
		pathAllowed := false
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(cleanPath, prefix) {
				pathAllowed = true
				break
			}
		}
		if !pathAllowed {
			return "failed", map[string]any{}, "cert_path must be under /etc/openvpn/, /etc/wireguard/, or /etc/strongswan/"
		}
		// Decode base64 content
		decoded, err := base64.StdEncoding.DecodeString(certContent)
		if err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("base64 decode: %s", err.Error())
		}
		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("create dir: %s", err.Error())
		}
		// Write cert file with restrictive permissions
		if err := os.WriteFile(cleanPath, decoded, 0600); err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("write cert: %s", err.Error())
		}
		// Validate with openssl verify if ca.crt exists
		validationResult := "skipped"
		caPath := filepath.Join(filepath.Dir(cleanPath), "ca.crt")
		if _, err := os.Stat(caPath); err == nil {
			cmd := exec.Command("openssl", "verify", "-CAfile", caPath, cleanPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				validationResult = fmt.Sprintf("failed: %s", strings.TrimSpace(string(out)))
			} else {
				validationResult = "ok"
			}
		}
		return "succeeded", map[string]any{"cert_path": cleanPath, "validation": validationResult}, ""
	case "wireguard.update_config":
		return executeWireGuardUpdateConfig(payload)
	case "wireguard.sync_config":
		// Use wg-quick strip to produce a config without wg-quick directives (Address, DNS, etc.)
		stripCmd := exec.Command("wg-quick", "strip", "wg0")
		strippedConf, err := stripCmd.Output()
		if err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("wg-quick strip: %s", err.Error())
		}
		// Write stripped config to a temp file for wg syncconf
		tmpFile, err := os.CreateTemp("", "wg-syncconf-*.conf")
		if err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("create temp file: %s", err.Error())
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)
		if _, err := tmpFile.Write(strippedConf); err != nil {
			tmpFile.Close()
			return "failed", map[string]any{}, fmt.Sprintf("write temp file: %s", err.Error())
		}
		tmpFile.Close()
		// Sync runtime config with stripped file
		cmd := exec.Command("wg", "syncconf", "wg0", tmpPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "failed", map[string]any{"output": string(out)}, err.Error()
		}
		return "succeeded", map[string]any{"message": "config synced"}, ""
	case "backup.collect_configs":
		return executeBackupCollectConfigs(log)
	case "backup.restore_configs":
		return executeBackupRestoreConfigs(payload, log)
	default:
		return "failed", map[string]any{}, "unsupported action"
	}
}

// executeBackupCollectConfigs collects VPN config files from well-known directories,
// creates a tar archive, base64-encodes it, and returns the result.
func executeBackupCollectConfigs(log *logger.Logger) (string, map[string]any, string) {
	configDirs := []string{
		"/etc/openvpn/",
		"/etc/wireguard/",
		"/etc/ipsec.d/",
		"/etc/xl2tpd/",
	}

	const maxTotalSize int64 = 10 * 1024 * 1024 // 10MB limit

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	var filesCount int
	var totalSize int64

	for _, dir := range configDirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			log.Debug("skipping non-existent config dir", map[string]any{"dir": dir})
			continue
		}

		err = filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil // skip files we can't access
			}
			if fi.IsDir() {
				return nil
			}
			if totalSize+fi.Size() > maxTotalSize {
				log.Warn("backup config collection size limit reached", map[string]any{
					"limit_mb": 10,
					"current":  totalSize,
					"skipped":  path,
				})
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil // skip unreadable files
			}

			// Use path relative to / so it preserves the full /etc/... structure
			hdr := &tar.Header{
				Name:    strings.TrimPrefix(path, "/"),
				Size:    int64(len(content)),
				Mode:    int64(fi.Mode().Perm()),
				ModTime: fi.ModTime(),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if _, err := tw.Write(content); err != nil {
				return err
			}

			filesCount++
			totalSize += int64(len(content))
			return nil
		})
		if err != nil {
			log.Error("error walking config dir", map[string]any{"dir": dir, "error": err.Error()})
		}
	}

	if err := tw.Close(); err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("close tar writer: %s", err.Error())
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	return "succeeded", map[string]any{
		"configs_tar_base64": encoded,
		"files_count":        filesCount,
		"total_size":         totalSize,
	}, ""
}

// executeBackupRestoreConfigs accepts a base64-encoded tar from the task payload,
// extracts files to their original absolute paths, and restarts affected services.
func executeBackupRestoreConfigs(payload map[string]any, log *logger.Logger) (string, map[string]any, string) {
	configsTarBase64, _ := payload["configs_tar_base64"].(string)
	if configsTarBase64 == "" {
		return "failed", map[string]any{}, "configs_tar_base64 is required"
	}

	data, err := base64.StdEncoding.DecodeString(configsTarBase64)
	if err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("base64 decode: %s", err.Error())
	}

	tr := tar.NewReader(bytes.NewReader(data))

	// Track which service directories were modified
	servicesAffected := map[string]bool{}
	var filesRestored int

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("read tar entry: %s", err.Error())
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}

		// Restore to absolute path (tar entries are stored without leading /)
		absPath := "/" + strings.TrimPrefix(hdr.Name, "/")

		// Path traversal protection: only allow known VPN config directories
		allowed := false
		if strings.HasPrefix(absPath, "/etc/openvpn/") {
			allowed = true
			servicesAffected["openvpn"] = true
		} else if strings.HasPrefix(absPath, "/etc/wireguard/") {
			allowed = true
			servicesAffected["wireguard"] = true
		} else if strings.HasPrefix(absPath, "/etc/ipsec.d/") {
			allowed = true
			servicesAffected["ipsec"] = true
		} else if strings.HasPrefix(absPath, "/etc/xl2tpd/") {
			allowed = true
			servicesAffected["xl2tpd"] = true
		}

		if !allowed {
			log.Warn("skipping restore of file outside allowed paths", map[string]any{"path": absPath})
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			log.Error("failed to create dir for restore", map[string]any{"path": absPath, "error": err.Error()})
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			log.Error("failed to read tar content", map[string]any{"path": absPath, "error": err.Error()})
			continue
		}

		mode := os.FileMode(hdr.Mode)
		if mode == 0 {
			mode = 0640
		}
		if err := os.WriteFile(absPath, content, mode); err != nil {
			log.Error("failed to write restored file", map[string]any{"path": absPath, "error": err.Error()})
			continue
		}

		filesRestored++
	}

	// Restart affected services
	serviceMap := map[string]string{
		"openvpn":   "openvpn",
		"wireguard": "wg-quick@wg0",
		"ipsec":     "strongswan",
		"xl2tpd":    "xl2tpd",
	}

	var restartedServices []string
	var restartErrors []string

	for svc := range servicesAffected {
		unitName, ok := serviceMap[svc]
		if !ok {
			continue
		}
		cmd := exec.Command("systemctl", "restart", unitName)
		out, err := cmd.CombinedOutput()
		if err != nil {
			restartErrors = append(restartErrors, fmt.Sprintf("%s: %s", unitName, strings.TrimSpace(string(out))))
			log.Error("failed to restart service after config restore", map[string]any{
				"service": unitName,
				"error":   err.Error(),
				"output":  string(out),
			})
		} else {
			restartedServices = append(restartedServices, unitName)
			log.Info("restarted service after config restore", map[string]any{"service": unitName})
		}
	}

	result := map[string]any{
		"files_restored":     filesRestored,
		"services_restarted": restartedServices,
	}
	if len(restartErrors) > 0 {
		result["restart_errors"] = restartErrors
	}

	return "succeeded", result, ""
}

// executeWireGuardSetup handles the "wireguard.setup" task: generates a server key pair,
// writes /etc/wireguard/wg0.conf, and starts the WireGuard service.
func executeWireGuardSetup(payload map[string]any) (string, map[string]any, string) {
	// Extract payload fields with defaults
	port := 51820
	if p, ok := payload["port"].(float64); ok && p > 0 {
		port = int(p)
	}
	network := "10.66.66.0/24"
	if n, ok := payload["network"].(string); ok && n != "" {
		network = n
	}
	dns1 := "1.1.1.1"
	if d, ok := payload["dns_1"].(string); ok && d != "" {
		dns1 = d
	}
	dns2 := "8.8.8.8"
	if d, ok := payload["dns_2"].(string); ok && d != "" {
		dns2 = d
	}
	mtu := 1420
	if m, ok := payload["mtu"].(float64); ok && m > 0 {
		mtu = int(m)
	}

	// Parse network CIDR to derive server address (first host = .1)
	ip, ipNet, err := net.ParseCIDR(network)
	if err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("invalid network CIDR: %s", err.Error())
	}
	_ = ip
	// Compute gateway address: network base + 1
	serverIP := make(net.IP, len(ipNet.IP))
	copy(serverIP, ipNet.IP)
	// Increment last byte for gateway
	if len(serverIP) == 4 {
		serverIP[3] = serverIP[3] + 1
	} else if len(serverIP) == 16 {
		serverIP[15] = serverIP[15] + 1
	}
	// Get prefix length
	ones, _ := ipNet.Mask.Size()
	serverAddress := fmt.Sprintf("%s/%d", serverIP.String(), ones)

	// Generate server private key
	genKeyCmd := exec.Command("wg", "genkey")
	privateKeyBytes, err := genKeyCmd.Output()
	if err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("wg genkey failed: %s", err.Error())
	}
	privateKey := strings.TrimSpace(string(privateKeyBytes))

	// Derive public key from private key
	pubKeyCmd := exec.Command("wg", "pubkey")
	pubKeyCmd.Stdin = strings.NewReader(privateKey)
	publicKeyBytes, err := pubKeyCmd.Output()
	if err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("wg pubkey failed: %s", err.Error())
	}
	publicKey := strings.TrimSpace(string(publicKeyBytes))

	// Build wg0.conf content
	dnsLine := dns1
	if dns2 != "" {
		dnsLine = dns1 + ", " + dns2
	}
	confContent := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = %s\nListenPort = %d\nDNS = %s\nMTU = %d\n",
		privateKey, serverAddress, port, dnsLine, mtu)

	// Ensure /etc/wireguard directory exists
	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("create /etc/wireguard: %s", err.Error())
	}

	// Write wg0.conf
	if err := os.WriteFile("/etc/wireguard/wg0.conf", []byte(confContent), 0600); err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("write wg0.conf: %s", err.Error())
	}

	// Enable wg-quick@wg0 service
	enableCmd := exec.Command("systemctl", "enable", "wg-quick@wg0")
	if out, err := enableCmd.CombinedOutput(); err != nil {
		return "failed", map[string]any{"output": string(out)}, fmt.Sprintf("systemctl enable wg-quick@wg0: %s", err.Error())
	}

	// Start WireGuard interface
	upCmd := exec.Command("wg-quick", "up", "wg0")
	if out, err := upCmd.CombinedOutput(); err != nil {
		return "failed", map[string]any{"output": string(out)}, fmt.Sprintf("wg-quick up wg0: %s", err.Error())
	}

	return "succeeded", map[string]any{"server_public_key": publicKey}, ""
}

// executeWireGuardUpdateConfig handles the "wireguard.update_config" task: rewrites the
// [Interface] section of /etc/wireguard/wg0.conf preserving [Peer] blocks, applies the
// new config via wg syncconf, and handles gaming optimize routing rules.
func executeWireGuardUpdateConfig(payload map[string]any) (string, map[string]any, string) {
	// Extract payload fields with defaults
	port := 51820
	if p, ok := payload["port"].(float64); ok && p > 0 {
		port = int(p)
	}
	network := "10.66.66.0/24"
	if n, ok := payload["network"].(string); ok && n != "" {
		network = n
	}
	dns1 := "1.1.1.1"
	if d, ok := payload["dns_1"].(string); ok && d != "" {
		dns1 = d
	}
	dns2 := "8.8.8.8"
	if d, ok := payload["dns_2"].(string); ok && d != "" {
		dns2 = d
	}
	mtu := 1420
	if m, ok := payload["mtu"].(float64); ok && m > 0 {
		mtu = int(m)
	}
	gamingOptimize := false
	if g, ok := payload["gaming_optimize"].(bool); ok {
		gamingOptimize = g
	}

	// Parse network CIDR to derive server address (first host = .1)
	_, ipNet, err := net.ParseCIDR(network)
	if err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("invalid network CIDR: %s", err.Error())
	}
	serverIP := make(net.IP, len(ipNet.IP))
	copy(serverIP, ipNet.IP)
	if len(serverIP) == 4 {
		serverIP[3] = serverIP[3] + 1
	} else if len(serverIP) == 16 {
		serverIP[15] = serverIP[15] + 1
	}
	ones, _ := ipNet.Mask.Size()
	serverAddress := fmt.Sprintf("%s/%d", serverIP.String(), ones)

	// Read existing config to extract [Peer] blocks
	confFile := "/etc/wireguard/wg0.conf"
	existingData, err := os.ReadFile(confFile)
	if err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("read wg0.conf: %s", err.Error())
	}

	// Extract the private key from existing config
	privateKey := extractPrivateKey(string(existingData))
	if privateKey == "" {
		return "failed", map[string]any{}, "could not find PrivateKey in existing wg0.conf"
	}

	// Extract all [Peer] blocks from existing config
	peerBlocks := extractPeerBlocks(string(existingData))

	// Override MTU for gaming optimize
	if gamingOptimize {
		mtu = 1280
	}

	// Build new [Interface] section
	dnsLine := dns1
	if dns2 != "" {
		dnsLine = dns1 + ", " + dns2
	}
	var confBuilder strings.Builder
	confBuilder.WriteString("[Interface]\n")
	confBuilder.WriteString(fmt.Sprintf("PrivateKey = %s\n", privateKey))
	confBuilder.WriteString(fmt.Sprintf("Address = %s\n", serverAddress))
	confBuilder.WriteString(fmt.Sprintf("ListenPort = %d\n", port))
	confBuilder.WriteString(fmt.Sprintf("DNS = %s\n", dnsLine))
	confBuilder.WriteString(fmt.Sprintf("MTU = %d\n", mtu))

	// Append [Peer] blocks
	for _, peer := range peerBlocks {
		confBuilder.WriteString("\n")
		confBuilder.WriteString(peer)
	}

	// Write the updated config
	if err := os.WriteFile(confFile, []byte(confBuilder.String()), 0600); err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("write wg0.conf: %s", err.Error())
	}

	// Apply config using wg syncconf (strip wg-quick directives first)
	stripCmd := exec.Command("wg-quick", "strip", "wg0")
	strippedConf, err := stripCmd.Output()
	if err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("wg-quick strip: %s", err.Error())
	}
	tmpFile, err := os.CreateTemp("", "wg-update-*.conf")
	if err != nil {
		return "failed", map[string]any{}, fmt.Sprintf("create temp file: %s", err.Error())
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := tmpFile.Write(strippedConf); err != nil {
		tmpFile.Close()
		return "failed", map[string]any{}, fmt.Sprintf("write temp file: %s", err.Error())
	}
	tmpFile.Close()
	syncCmd := exec.Command("wg", "syncconf", "wg0", tmpPath)
	if out, err := syncCmd.CombinedOutput(); err != nil {
		return "failed", map[string]any{"output": string(out)}, fmt.Sprintf("wg syncconf: %s", err.Error())
	}

	// Apply gaming optimize rules
	if gamingOptimize {
		if err := applyGamingOptimize(); err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("apply gaming optimize: %s", err.Error())
		}
	} else {
		if err := removeGamingOptimize(mtu); err != nil {
			return "failed", map[string]any{}, fmt.Sprintf("remove gaming optimize: %s", err.Error())
		}
	}

	return "succeeded", map[string]any{"message": "config updated", "gaming_optimize": gamingOptimize}, ""
}

// extractPrivateKey reads the PrivateKey value from a WireGuard config string.
func extractPrivateKey(config string) string {
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "PrivateKey") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// extractPeerBlocks extracts all [Peer] sections from a WireGuard config string.
// Each returned block includes the [Peer] header and all its key-value lines, ending with a newline.
func extractPeerBlocks(config string) []string {
	lines := strings.Split(config, "\n")
	var peers []string
	var current []string
	inPeer := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "[Peer]") {
			if inPeer && len(current) > 0 {
				peers = append(peers, strings.Join(current, "\n")+"\n")
			}
			inPeer = true
			current = []string{line}
			continue
		}
		if strings.HasPrefix(trimmed, "[") && inPeer {
			// New section that's not [Peer] — flush current peer
			if len(current) > 0 {
				peers = append(peers, strings.Join(current, "\n")+"\n")
			}
			inPeer = false
			current = nil
			continue
		}
		if inPeer {
			current = append(current, line)
		}
	}
	// Flush last peer block
	if inPeer && len(current) > 0 {
		// Trim trailing empty lines from the last peer block
		for len(current) > 0 && strings.TrimSpace(current[len(current)-1]) == "" {
			current = current[:len(current)-1]
		}
		if len(current) > 0 {
			peers = append(peers, strings.Join(current, "\n")+"\n")
		}
	}
	return peers
}

// applyGamingOptimize applies fwmark-based priority routing and reduced MTU for gaming.
func applyGamingOptimize() error {
	// Set fwmark on wg0
	cmd := exec.Command("wg", "set", "wg0", "fwmark", "51820")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg set fwmark: %s (%s)", err.Error(), string(out))
	}

	// Add ip rule for priority routing (ignore error if already exists)
	cmd = exec.Command("ip", "rule", "add", "not", "fwmark", "51820", "table", "51820", "priority", "100")
	cmd.CombinedOutput() // Ignore error if rule already exists

	// Add default route in table 51820 (ignore error if already exists)
	cmd = exec.Command("ip", "route", "add", "default", "dev", "wg0", "table", "51820")
	cmd.CombinedOutput() // Ignore error if route already exists

	// Set interface MTU to 1280
	cmd = exec.Command("ip", "link", "set", "wg0", "mtu", "1280")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set mtu 1280: %s (%s)", err.Error(), string(out))
	}

	return nil
}

// removeGamingOptimize removes fwmark routing rules and reverts MTU.
func removeGamingOptimize(mtu int) error {
	// Remove ip rule (ignore error if not present)
	cmd := exec.Command("ip", "rule", "del", "not", "fwmark", "51820", "table", "51820")
	cmd.CombinedOutput()

	// Remove route (ignore error if not present)
	cmd = exec.Command("ip", "route", "del", "default", "dev", "wg0", "table", "51820")
	cmd.CombinedOutput()

	// Revert MTU to specified value (or default 1420)
	if mtu <= 0 {
		mtu = 1420
	}
	cmd = exec.Command("ip", "link", "set", "wg0", "mtu", strconv.Itoa(mtu))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set mtu %d: %s (%s)", mtu, err.Error(), string(out))
	}

	return nil
}

// removePeerFromConfig removes a [Peer] block with the given public key from a WireGuard config.
func removePeerFromConfig(config, pubKey string) string {
	lines := strings.Split(config, "\n")
	var result []string
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Start of a new section
		if strings.HasPrefix(trimmed, "[") {
			if skip {
				skip = false
			}
			// Check if this is the peer we want to remove
			if strings.EqualFold(trimmed, "[Peer]") {
				// Look ahead - we need to mark for potential skipping
				skip = false // Will be determined by PublicKey line
				result = append(result, line)
				continue
			}
		}
		if !skip && strings.HasPrefix(strings.TrimSpace(line), "PublicKey") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[1]) == pubKey {
				// Remove this peer block: remove the [Peer] line we just added
				// and skip until next section
				if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "[Peer]" {
					result = result[:len(result)-1]
					// Also remove trailing empty line before [Peer] if present
					for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
						result = result[:len(result)-1]
					}
				}
				skip = true
				continue
			}
		}
		if skip {
			// Skip lines until next section header
			if strings.HasPrefix(trimmed, "[") {
				skip = false
				result = append(result, line)
			}
			continue
		}
		result = append(result, line)
	}
	// Trim trailing empty lines to prevent accumulation over repeated add/remove cycles
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	// Ensure file ends with a single newline
	return strings.Join(result, "\n") + "\n"
}

// isValidWireGuardKey checks that a WireGuard public/preshared key is a valid
// 44-character base64 string that decodes to exactly 32 bytes.
func isValidWireGuardKey(key string) bool {
	if len(key) != 44 {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return false
	}
	return len(decoded) == 32
}

// isValidAllowedIPs validates that the allowed_ips value contains only valid CIDR
// notations separated by commas, with no newlines or other dangerous characters.
func isValidAllowedIPs(allowedIPs string) bool {
	if strings.ContainsAny(allowedIPs, "\n\r") {
		return false
	}
	parts := strings.Split(allowedIPs, ",")
	for _, part := range parts {
		cidr := strings.TrimSpace(part)
		if cidr == "" {
			return false
		}
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return false
		}
	}
	return true
}

func normalizeService(input string) string {
	s := strings.ToLower(strings.TrimSpace(input))
	switch s {
	case "openvpn", "openvpn@server", "openvpn-server@server":
		return "openvpn"
	case "l2tp", "xl2tpd":
		return "xl2tpd"
	case "ikev2", "ipsec", "strongswan", "strongswan-starter":
		return "strongswan"
	case "ssh", "sshd", "ssh-tunnel", "dropbear":
		return "ssh"
	case "wireguard", "wg", "wg-quick@wg0":
		return "wg-quick@wg0"
	default:
		return ""
	}
}

// serviceToUnit maps a normalized logical service name to the actual systemd unit name.
// It tries to detect which unit is available on the system.
func serviceToUnit(service string) string {
	switch service {
	case "openvpn":
		// Prefer openvpn-server@server (modern), fallback to openvpn@server
		if checkUnit("openvpn-server@server") != "stopped" {
			return "openvpn-server@server"
		}
		return "openvpn@server"
	case "strongswan":
		// Ubuntu uses strongswan-starter, others use ipsec or strongswan
		if checkUnit("strongswan-starter") != "stopped" || unitExists("strongswan-starter") {
			return "strongswan-starter"
		}
		if unitExists("ipsec") {
			return "ipsec"
		}
		return "strongswan"
	case "ssh":
		if checkUnit("sshd") == "running" {
			return "sshd"
		}
		return "ssh"
	default:
		return service
	}
}

// unitExists checks if a systemd unit is loaded (exists in systemd).
func unitExists(unit string) bool {
	out, err := exec.Command("systemctl", "list-unit-files", unit+".service", "--no-legend").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// isValidVPNUsername checks that a username contains only safe characters.
// Allowed: alphanumeric, dots, hyphens, underscores, and @ signs.
// This prevents injection of management protocol commands via newlines or control characters.
func isValidVPNUsername(username string) bool {
	for _, c := range username {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		if c == '.' || c == '-' || c == '_' || c == '@' {
			continue
		}
		return false
	}
	return true
}

// disconnectVPNUserSocket attempts to disconnect a user via a single Unix management socket.
// Returns the response string on success, or an error. The caller does NOT need to close the connection.
func disconnectVPNUserSocket(sockPath, username string, log *logger.Logger) (string, error) {
	conn, err := net.DialTimeout("unix", sockPath, 3*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	killCmd := fmt.Sprintf("kill %s\n", username)
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte(killCmd)); err != nil {
		log.Warn("failed to write kill command to mgmt socket", map[string]any{
			"socket": sockPath,
			"error":  err.Error(),
		})
		return "", err
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 512)
	n, _ := conn.Read(buf)
	response := strings.TrimSpace(string(buf[:n]))
	log.Info("vpn user disconnect via management socket", map[string]any{
		"username": username,
		"socket":   sockPath,
		"response": response,
	})
	return response, nil
}

// disconnectVPNUser disconnects a specific user from the VPN by writing a kill
// command to the OpenVPN management interface, or by removing their session file.
func disconnectVPNUser(username string, log *logger.Logger) (string, error) {
	// Validate username to prevent command injection via the management protocol.
	if !isValidVPNUsername(username) {
		return "", fmt.Errorf("invalid username: contains disallowed characters")
	}

	// Try OpenVPN management socket first (default: /run/openvpn/management.sock or TCP 127.0.0.1:7505)
	mgmtPaths := []string{
		"/run/openvpn/management.sock",
		"/var/run/openvpn/management.sock",
	}

	for _, sockPath := range mgmtPaths {
		response, err := disconnectVPNUserSocket(sockPath, username, log)
		if err != nil {
			continue
		}
		return response, nil
	}

	// Fallback: try TCP management interface (common config: 127.0.0.1:7505)
	conn, err := net.DialTimeout("tcp", "127.0.0.1:7505", 3*time.Second)
	if err == nil {
		defer conn.Close()
		killCmd := fmt.Sprintf("kill %s\n", username)
		conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write([]byte(killCmd)); err == nil {
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			buf := make([]byte, 512)
			n, _ := conn.Read(buf)
			response := strings.TrimSpace(string(buf[:n]))
			log.Info("vpn user disconnect via TCP management", map[string]any{
				"username": username,
				"response": response,
			})
			return response, nil
		}
	}

	// Last resort: try to remove the client session from CCD or ipp file
	ccdPath := fmt.Sprintf("/etc/openvpn/server/ccd/%s", username)
	if _, err := os.Stat(ccdPath); err == nil {
		// Temporary disable: rename to .disabled (non-destructive)
		log.Info("disabling CCD for user to force disconnect", map[string]any{
			"username": username,
			"ccd_path": ccdPath,
		})
	}

	return "", fmt.Errorf("could not connect to OpenVPN management interface to disconnect user %s", username)
}

func postJSON(client *http.Client, url, token string, v any, log *logger.Logger) (bool, string) {
	b, _ := json.Marshal(v)

	log.Debug("sending request to panel", map[string]any{
		"url":       url,
		"method":    "POST",
		"body_size": len(b),
	})

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		log.Error("POST request failed", map[string]any{
			"url":   url,
			"error": err.Error(),
		})
		return false, err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		errMsg := fmt.Sprintf("non-2xx response: %d %s", resp.StatusCode, string(body))
		log.Warn("non-2xx response from panel", map[string]any{
			"url":    url,
			"status": resp.StatusCode,
			"body":   string(body),
		})
		return false, errMsg
	}

	log.Debug("panel response received", map[string]any{
		"url":           url,
		"status":        resp.Status,
		"response_code": resp.StatusCode,
	})

	return true, ""
}

// onlineUsers counts the number of connected OpenVPN clients by parsing the status file.
// Returns 0 if the file does not exist or cannot be read.
func onlineUsers() int {
	b, err := os.ReadFile("/run/openvpn-status.log")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(b), "\n")
	count := 0
	inClientList := false

	for _, line := range lines {
		if strings.HasPrefix(line, "Common Name,") {
			inClientList = true
			continue
		}
		if strings.HasPrefix(line, "ROUTING TABLE") {
			break
		}
		if inClientList && strings.TrimSpace(line) != "" {
			count++
		}
	}

	return count
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func firstIP() string {
	ifaces, _ := net.Interfaces()
	// First pass: try to find an IPv4 address
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil || ipNet.IP.To4() == nil {
				continue
			}
			return ipNet.IP.String()
		}
	}
	// Second pass: fallback to IPv6 (skip link-local fe80:: addresses)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			// Skip IPv4 (already tried) and link-local IPv6
			if ipNet.IP.To4() != nil {
				continue
			}
			if ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			return ipNet.IP.String()
		}
	}
	return ""
}

func firstIPv6() string {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			// Skip IPv4 and link-local IPv6
			if ipNet.IP.To4() != nil {
				continue
			}
			if ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			return ipNet.IP.String()
		}
	}
	return ""
}

func serviceStatus(service string) string {
	// Map logical names to systemd unit names
	unitName := service
	switch service {
	case "ssh":
		// Try sshd first (most distros), fallback to ssh (Debian/Ubuntu), then dropbear
		out, err := exec.Command("systemctl", "is-active", "sshd").Output()
		if err == nil {
			status := strings.TrimSpace(string(out))
			if status == "active" {
				return "running"
			}
		}
		out, err = exec.Command("systemctl", "is-active", "ssh").Output()
		if err == nil {
			status := strings.TrimSpace(string(out))
			if status == "active" {
				return "running"
			}
		}
		// Try dropbear as alternative SSH daemon
		unitName = "dropbear"
	case "openvpn":
		// Try openvpn@server first, fallback to openvpn
		out, err := exec.Command("systemctl", "is-active", "openvpn@server").Output()
		if err == nil {
			status := strings.TrimSpace(string(out))
			if status == "active" {
				return "running"
			}
		}
		unitName = "openvpn"
	case "wg-quick@wg0":
		// Check wg-quick@wg0 service
		out, err := exec.Command("systemctl", "is-active", "wg-quick@wg0").Output()
		if err == nil {
			status := strings.TrimSpace(string(out))
			if status == "active" {
				return "running"
			}
		}
		return "stopped"
	case "strongswan", "ikev2", "ipsec":
		// Try strongswan-starter (Ubuntu), then ipsec, then strongswan
		for _, unit := range []string{"strongswan-starter", "ipsec", "strongswan"} {
			status := checkUnit(unit)
			if status == "running" {
				return "running"
			}
		}
		return "stopped"
	case "xl2tpd":
		// L2TP/IPSec needs both xl2tpd and an IPSec daemon.
		// Check xl2tpd first.
		l2tpStatus := checkUnit("xl2tpd")
		// Also check ipsec/strongswan-starter for the IPSec component.
		ipsecStatus := checkUnit("ipsec")
		if ipsecStatus != "running" {
			ipsecStatus = checkUnit("strongswan-starter")
		}
		// Report combined status: both must be running for full L2TP/IPSec.
		if l2tpStatus == "running" && ipsecStatus == "running" {
			return "running"
		}
		if l2tpStatus == "running" || ipsecStatus == "running" {
			return "degraded"
		}
		return "stopped"
	}
	out, err := exec.Command("systemctl", "is-active", unitName).Output()
	if err != nil {
		return "stopped"
	}
	status := strings.TrimSpace(string(out))
	switch status {
	case "active":
		return "running"
	case "inactive", "dead":
		return "stopped"
	case "failed":
		return "failed"
	default:
		return status
	}
}

// checkUnit queries systemctl is-active for a single unit and returns a normalized status.
func checkUnit(unit string) string {
	out, err := exec.Command("systemctl", "is-active", unit).Output()
	if err != nil {
		return "stopped"
	}
	status := strings.TrimSpace(string(out))
	switch status {
	case "active":
		return "running"
	case "inactive", "dead":
		return "stopped"
	case "failed":
		return "failed"
	default:
		return status
	}
}

// collectWireGuardPeers runs `wg show wg0 dump` and parses peer statistics.
// Returns the peer list and a count of active peers (handshake within 3 minutes).
// If wg0 doesn't exist or the command fails, returns nil and 0.
func collectWireGuardPeers() ([]WireGuardPeerStat, int) {
	cmd := exec.Command("wg", "show", "wg0", "dump")
	out, err := cmd.Output()
	if err != nil {
		return nil, 0
	}
	return parseWgDump(string(out), time.Now().Unix())
}

// parseWgDump parses the tab-separated output of `wg show wg0 dump`.
// The first line is the interface line (private_key, listen_port, fwmark); subsequent lines are peers.
// Peer line format: public_key \t preshared_key \t endpoint \t allowed_ips \t latest_handshake \t transfer_rx \t transfer_tx \t persistent_keepalive
func parseWgDump(output string, nowUnix int64) ([]WireGuardPeerStat, int) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil, 0
	}

	var peers []WireGuardPeerStat
	activePeers := 0

	// Skip first line (interface info), parse peer lines
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		pubKey := fields[0]
		endpoint := fields[2]
		allowedIPs := fields[3]
		handshake, _ := strconv.ParseInt(fields[4], 10, 64)
		rxBytes, _ := strconv.ParseInt(fields[5], 10, 64)
		txBytes, _ := strconv.ParseInt(fields[6], 10, 64)

		// Active = handshake within last 3 minutes (180 seconds)
		active := handshake > 0 && (nowUnix-handshake) < 180

		peers = append(peers, WireGuardPeerStat{
			PublicKey:       pubKey,
			Endpoint:        endpoint,
			AllowedIPs:      allowedIPs,
			LatestHandshake: handshake,
			RxBytes:         rxBytes,
			TxBytes:         txBytes,
			Active:          active,
		})

		if active {
			activePeers++
		}
	}

	return peers, activePeers
}
