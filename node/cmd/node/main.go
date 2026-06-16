package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"koris-next/node/internal/config"
	"koris-next/node/internal/logger"
	"koris-next/node/internal/updater"
)

const agentVersion = "0.36.0"

type DiagnosticsReport struct {
	AgentVersion  string `json:"agent_version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	GoVersion     string `json:"go_version"`
	Goroutines    int    `json:"goroutines"`
	MemAllocBytes int64  `json:"mem_alloc_bytes"`
}

type Push struct {
	Token         string             `json:"token"`
	Type          string             `json:"type"`
	Hostname      string             `json:"hostname"`
	PublicIP      string             `json:"public_ip"`
	OS            string             `json:"os"`
	Timestamp     time.Time          `json:"timestamp"`
	CPUPercent    float64            `json:"cpu_percent"`
	RAMPercent    float64            `json:"ram_percent"`
	DiskPercent   float64            `json:"disk_percent"`
	RxBps         int64              `json:"rx_bps"`
	TxBps         int64              `json:"tx_bps"`
	RxBytes       int64              `json:"rx_bytes"`
	TxBytes       int64              `json:"tx_bytes"`
	OnlineUsers   int                `json:"online_users"`
	OpenVPNStatus string             `json:"openvpn_status"`
	L2TPStatus    string             `json:"l2tp_status"`
	IKEv2Status   string             `json:"ikev2_status"`
	Services      map[string]string  `json:"services"`
	Diagnostics   *DiagnosticsReport `json:"diagnostics,omitempty"`
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

	lastRx, lastTx := netBytes()
	lastAt := time.Now()
	for {
		nowRx, nowTx := netBytes()
		now := time.Now()
		dt := now.Sub(lastAt).Seconds()
		if dt <= 0 {
			dt = intervalDuration.Seconds()
		}
		host, _ := os.Hostname()
		services := map[string]string{
			"openvpn": serviceStatus("openvpn"),
			"l2tp":    serviceStatus("xl2tpd"),
			"ikev2":   serviceStatus("strongswan"),
			"ssh":     serviceStatus("ssh"),
		}
		push := Push{
			Token:         token,
			Type:          "status",
			Hostname:      host,
			PublicIP:      firstIP(),
			OS:            runtime.GOOS,
			Timestamp:     now.UTC(),
			CPUPercent:    cpuPercent(),
			RAMPercent:    memPercent(),
			DiskPercent:   diskPercent("/"),
			RxBytes:       nowRx,
			TxBytes:       nowTx,
			RxBps:         int64(float64(nowRx-lastRx) / dt),
			TxBps:         int64(float64(nowTx-lastTx) / dt),
			OnlineUsers:   0,
			OpenVPNStatus: services["openvpn"],
			L2TPStatus:    services["l2tp"],
			IKEv2Status:   services["ikev2"],
			Services:      services,
			Diagnostics:   buildDiagnostics(startTime, agentVersion),
		}

		ok, errMsg := postJSON(client, panel+"/api/node/push", token, push, log)
		if ok {
			tracker.RecordSuccess()
		} else {
			tracker.RecordFailure(errMsg)
		}

		pollTasks(client, panel, token, log, cfg, envFile)
		lastRx, lastTx, lastAt = nowRx, nowTx, now
		time.Sleep(intervalDuration)
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

	// Write a temporary env file for the config package to load
	tmpFile, err := os.CreateTemp("", "node-env-*.env")
	if err != nil {
		return nil
	}
	defer os.Remove(tmpFile.Name())

	content := fmt.Sprintf("NODE_TOKEN=%s\nPANEL_URL=%s\nNODE_INTERVAL=%d\nLOG_LEVEL=%s\nNODE_AUTO_UPDATE=%s\n",
		token, panelURL, intervalSeconds, logLevel, autoUpdate)
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return nil
	}
	tmpFile.Close()

	cfg, err := config.Load(tmpFile.Name())
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

func pollTasks(client *http.Client, panel, token string, log *logger.Logger, cfg *config.Config, envFile string) {
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
		status, result, errText := executeTask(task, cfg, envFile, log)
		log.Info("task completed", map[string]any{
			"task_id": task.ID,
			"action":  task.Action,
			"status":  status,
		})
		complete := map[string]any{"status": status, "result_json": result, "error": errText}
		_, _ = postJSON(client, fmt.Sprintf("%s/api/node/tasks/%d/complete", panel, task.ID), token, complete, log)
	}
}

func executeTask(task Task, cfg *config.Config, envFile string, log *logger.Logger) (string, map[string]any, string) {
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
		cmd := exec.Command("systemctl", verb, service)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "failed", map[string]any{"service": service, "output": string(out)}, err.Error()
		}
		return "succeeded", map[string]any{"service": service, "output": string(out), "status": serviceStatus(service)}, ""
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
	default:
		return "failed", map[string]any{}, "unsupported action"
	}
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
	default:
		return ""
	}
}

func postJSON(client *http.Client, url, token string, v any, log *logger.Logger) (bool, string) {
	b, _ := json.Marshal(v)

	log.Debug("sending request to panel", map[string]any{
		"url":          url,
		"method":       "POST",
		"body_size":    len(b),
		"request_body": string(b),
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

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func firstIP() string {
	ifaces, _ := net.Interfaces()
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
	return ""
}

func serviceStatus(service string) string {
	// Map logical names to systemd unit names
	unitName := service
	switch service {
	case "ssh":
		// Try sshd first (most distros), fallback to ssh (Debian/Ubuntu)
		out, err := exec.Command("systemctl", "is-active", "sshd").Output()
		if err == nil {
			status := strings.TrimSpace(string(out))
			if status == "active" {
				return "running"
			}
		}
		unitName = "ssh"
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

func cpuPercent() float64 {
	idle1, total1 := readCPU()
	time.Sleep(180 * time.Millisecond)
	idle2, total2 := readCPU()
	idle := float64(idle2 - idle1)
	total := float64(total2 - total1)
	if total <= 0 {
		return 0
	}
	return round2((1 - idle/total) * 100)
}

func readCPU() (idle, total uint64) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	for i, field := range fields[1:] {
		v, _ := strconv.ParseUint(field, 10, 64)
		total += v
		if i == 3 || i == 4 {
			idle += v
		}
	}
	return idle, total
}

func memPercent() float64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	vals := map[string]float64{}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			key := strings.TrimSuffix(fields[0], ":")
			vals[key], _ = strconv.ParseFloat(fields[1], 64)
		}
	}
	total := vals["MemTotal"]
	available := vals["MemAvailable"]
	if total <= 0 {
		return 0
	}
	return round2((total - available) / total * 100)
}

func diskPercent(mount string) float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mount, &stat); err != nil {
		return 0
	}
	total := float64(stat.Blocks)
	free := float64(stat.Bavail)
	if total <= 0 {
		return 0
	}
	return round2((total - free) / total * 100)
}

func netBytes() (rx, tx int64) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.Split(line, ":")
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		r, _ := strconv.ParseInt(fields[0], 10, 64)
		t, _ := strconv.ParseInt(fields[8], 10, 64)
		rx += r
		tx += t
	}
	return rx, tx
}

func round2(v float64) float64 {
	n, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", v), 64)
	return n
}
