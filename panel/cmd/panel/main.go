package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"KorisPanel/panel/internal/api"
	"KorisPanel/panel/internal/backup"
	"KorisPanel/panel/internal/certrotation"
	"KorisPanel/panel/internal/cli"
	"KorisPanel/panel/internal/config"
	"KorisPanel/panel/internal/db"
	"KorisPanel/panel/internal/nodeapi"
	"KorisPanel/panel/internal/notify"
	"KorisPanel/panel/internal/protocols"
	"KorisPanel/panel/internal/ratelimit"
	"KorisPanel/panel/internal/sessions"
	"KorisPanel/panel/internal/tui"
	"KorisPanel/panel/internal/worker"
	"KorisPanel/panel/web"

	"github.com/coreos/go-systemd/v22/daemon"
	"golang.org/x/crypto/acme/autocert"
)

// logger is the structured TUI logger used throughout the panel process.
// Initialized in main() before any other component starts.
var logger *tui.Logger

func dbNameFromDSN(dsn string) string {
	parts := strings.Split(dsn, "/")
	if len(parts) >= 2 {
		dbPart := parts[len(parts)-1]
		if i := strings.Index(dbPart, "?"); i != -1 {
			return dbPart[:i]
		}
		return dbPart
	}
	return ""
}

func mysqlCredsFromDSN(dsn string) (user, pass, db string) {
	at := strings.Index(dsn, "@")
	if at == -1 {
		return "", "", ""
	}
	creds := dsn[:at]
	colon := strings.Index(creds, ":")
	if colon != -1 {
		user = creds[:colon]
		pass = creds[colon+1:]
	}
	db = dbNameFromDSN(dsn)
	return
}

func startWorker(db *sql.DB) {
	notifier := notify.New()
	ticker := time.NewTicker(time.Minute)
	var tickCount int
	go func() {
		for range ticker.C {
			tickCount++
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("worker", "recovered from panic", map[string]any{"panic": r})
					}
				}()
				workerTick(db, notifier, tickCount)
			}()
		}
	}()
}

// startWatchdog parses the WATCHDOG_USEC environment variable set by systemd
// and, if present, starts a goroutine that sends WATCHDOG=1 notifications at
// half the configured interval as long as health checks (DB ping) pass.
// On non-Linux systems or when not running under systemd, this is a no-op.
func startWatchdog(database *sql.DB) {
	usecStr := os.Getenv("WATCHDOG_USEC")
	if usecStr == "" {
		return
	}
	usec, err := strconv.ParseInt(usecStr, 10, 64)
	if err != nil || usec <= 0 {
		return
	}

	interval := time.Duration(usec/2) * time.Microsecond
	logger.Info("watchdog", "starting systemd watchdog", map[string]any{"interval": interval.String()})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := database.Ping(); err != nil {
				logger.Warn("watchdog", "health check failed, withholding watchdog", map[string]any{"error": err.Error()})
				continue
			}
			daemon.SdNotify(false, "WATCHDOG=1")
		}
	}()
}

func workerTick(db *sql.DB, notifier *notify.Notifier, tickCount int) {
	// Find customers whose subscriptions have expired
	// First attempt auto-renewal for eligible customers
	autoRenewRows, _ := db.Query(`
		SELECT c.id, c.username, c.plan_id, p.price, p.duration_days, COALESCE(w.credit, 0) as credit
		FROM customers c
		JOIN (SELECT username, MAX(expires_at) as max_expires FROM subscriptions WHERE status='active' GROUP BY username) s ON c.username=s.username
		JOIN plans p ON p.id = c.plan_id
		LEFT JOIN wallets w ON w.username = c.username
		WHERE c.status = 'active' AND c.auto_renew = 1 AND s.max_expires <= NOW()
		AND COALESCE(w.credit, 0) >= p.price`)
	if autoRenewRows != nil {
		for autoRenewRows.Next() {
			var cid, planID int64
			var username string
			var price, credit float64
			var durationDays int
			if autoRenewRows.Scan(&cid, &username, &planID, &price, &durationDays, &credit) == nil {
				// Deduct from wallet and create new subscription
				db.Exec(`UPDATE wallets SET credit = credit - ? WHERE username = ?`, price, username)
				expires := time.Now().AddDate(0, 0, durationDays)
				db.Exec(`INSERT INTO subscriptions(customer_id, username, plan_id, expires_at, status) VALUES(?,?,?,?,'active')`, cid, username, planID, expires)
				db.Exec(`INSERT INTO wallet_transactions(customer_id, username, amount, type, description, actor) VALUES(?,?,?,?,?,?)`,
					cid, username, -price, "purchase", "Auto-renewal", "system")
				logger.Info("worker", "auto-renewed customer", map[string]any{"username": username, "plan": planID, "charged": price})
				notifier.SendEvent("renewal", fmt.Sprintf("🔄 Auto-renewed: %s", username), fmt.Sprintf("Plan renewed for %d days, charged $%.2f from wallet", durationDays, price))
			}
		}
		autoRenewRows.Close()
	}

	// Find remaining expired customers (not auto-renewed)
	expRows, expErr := db.Query(`SELECT c.id, c.username, COALESCE(p.grace_days, 0) as grace_days, s.max_expires
		FROM customers c
		JOIN (SELECT username, MAX(expires_at) as max_expires FROM subscriptions WHERE status='active' GROUP BY username) s ON c.username=s.username
		LEFT JOIN plans p ON p.id = c.plan_id
		WHERE c.status IN ('active', 'limited') AND s.max_expires <= NOW()`)
	var expiringCustomerIDs []int64
	if expErr == nil {
		for expRows.Next() {
			var cid int64
			var username string
			var graceDays int
			var maxExpires time.Time
			if expRows.Scan(&cid, &username, &graceDays, &maxExpires) == nil {
				graceEnd := maxExpires.AddDate(0, 0, graceDays)
				now := time.Now()

				if graceDays > 0 && now.Before(graceEnd) {
					// Within grace period → set to 'limited' (not expired yet)
					db.Exec(`UPDATE customers SET status='limited' WHERE id=? AND status='active'`, cid)
				} else {
					// Grace period over (or no grace days) → expire
					db.Exec(`UPDATE customers SET status='expired' WHERE id=? AND status IN ('active','limited')`, cid)
					expiringCustomerIDs = append(expiringCustomerIDs, cid)
				}
			}
		}
		expRows.Close()
	}

	// Auto-revoke WireGuard peers for fully expired customers
	for _, cid := range expiringCustomerIDs {
		api.AutoRevokeWireGuardPeersByDB(db, cid)
	}

	// Usage warnings: notify admin via Telegram when users hit thresholds (80%, 95%)
	warnRows, warnErr := db.Query(`
		SELECT c.username, CAST(r.value AS UNSIGNED) as max_bytes, a.used
		FROM customers c
		JOIN radcheck r ON c.username=r.username AND r.attribute='Max-Data'
		JOIN (SELECT username, COALESCE(SUM(acctinputoctets+acctoutputoctets),0) AS used FROM radacct GROUP BY username) a ON c.username=a.username
		WHERE c.status='active' AND CAST(r.value AS UNSIGNED) > 0`)
	if warnErr == nil {
		for warnRows.Next() {
			var username string
			var maxBytes, used int64
			if warnRows.Scan(&username, &maxBytes, &used) == nil && maxBytes > 0 {
				percent := int(float64(used) / float64(maxBytes) * 100)
				// Notify at 80% and 95% (check if not already notified via events)
				if percent >= 95 {
					var already int
					db.QueryRow(`SELECT COUNT(*) FROM events WHERE related=? AND type='data_warning' AND title LIKE '%95%' AND created_at > NOW() - INTERVAL 1 DAY`, username).Scan(&already)
					if already == 0 {
						notifier.SendEvent("data_warning", fmt.Sprintf("⚠️ %s at 95%% data", username), fmt.Sprintf("User %s has used 95%% of their data limit", username))
						db.Exec(`INSERT INTO events(type,severity,title,message,actor,related) VALUES('data_warning','warning',?,?,?,?)`, fmt.Sprintf("%s at 95%% data", username), fmt.Sprintf("Used %d%% of data limit", percent), "system", username)
					}
				} else if percent >= 80 {
					var already int
					db.QueryRow(`SELECT COUNT(*) FROM events WHERE related=? AND type='data_warning' AND title LIKE '%80%' AND created_at > NOW() - INTERVAL 1 DAY`, username).Scan(&already)
					if already == 0 {
						notifier.SendEvent("data_warning", fmt.Sprintf("📊 %s at 80%% data", username), fmt.Sprintf("User %s has used 80%% of their data limit", username))
						db.Exec(`INSERT INTO events(type,severity,title,message,actor,related) VALUES('data_warning','info',?,?,?,?)`, fmt.Sprintf("%s at 80%% data", username), fmt.Sprintf("Used %d%% of data limit", percent), "system", username)
					}
				}
			}
		}
		warnRows.Close()
	}

	if _, err := db.Exec(`UPDATE customers c JOIN radcheck r ON c.username=r.username AND r.attribute='Max-Data' JOIN (SELECT username, COALESCE(SUM(acctinputoctets+acctoutputoctets),0) AS used FROM radacct GROUP BY username) a ON c.username=a.username SET c.status='limited' WHERE c.status='active' AND CAST(r.value AS UNSIGNED) > 0 AND a.used >= CAST(r.value AS UNSIGNED)`); err != nil {
		logger.Error("worker", "data limit enforcement failed", map[string]any{"error": err.Error()})
	}
	_, _ = db.Exec(`UPDATE radacct SET acctstoptime=NOW(), acctterminatecause='Stalled session' WHERE acctstoptime IS NULL AND acctupdatetime < (NOW() - INTERVAL 5 MINUTE)`)

	// Mark nodes offline, record downtime for SLA tracking, and notify via Telegram
	rows, err := db.Query(`SELECT id, name, public_ip FROM nodes WHERE status IN('online','stale') AND last_seen_at < (NOW() - INTERVAL 5 MINUTE)`)
	if err == nil {
		for rows.Next() {
			var nodeID int64
			var name, ip string
			if rows.Scan(&nodeID, &name, &ip) == nil {
				api.RecordNodeDowntime(db, nodeID, "Node went offline (no push for 5+ minutes)")
				notifier.NotifyNodeOffline(name, ip)
			}
		}
		rows.Close()
	}
	_, _ = db.Exec(`UPDATE nodes SET status='offline' WHERE status IN('online','stale') AND last_seen_at < (NOW() - INTERVAL 5 MINUTE)`)

	// Data retention: prune old snapshots to prevent unbounded growth
	// Keep last 7 days of node_usage_snapshots, last 24h of user_bandwidth_snapshots
	_, _ = db.Exec(`DELETE FROM node_usage_snapshots WHERE created_at < NOW() - INTERVAL 7 DAY`)
	_, _ = db.Exec(`DELETE FROM user_bandwidth_snapshots WHERE created_at < NOW() - INTERVAL 24 HOUR`)

	// History retention: prune old radacct and wallet_transactions
	// Runs only at midnight (00:00) to avoid unnecessary load
	now := time.Now()
	if now.Hour() == 0 && now.Minute() == 0 {
		retentionDays := 45
		var retVal string
		if db.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key='history_retention_days'`).Scan(&retVal) == nil {
			if d, err := strconv.Atoi(retVal); err == nil && d > 0 {
				retentionDays = d
			}
		}
		_, _ = db.Exec(`DELETE FROM radacct WHERE acctstoptime IS NOT NULL AND acctstoptime < NOW() - INTERVAL ? DAY`, retentionDays)
		_, _ = db.Exec(`DELETE FROM wallet_transactions WHERE created_at < NOW() - INTERVAL ? DAY`, retentionDays)
		logger.Info("worker", "history retention: purged old records", map[string]any{"retention_days": retentionDays})
	}

	// Node resource alerts: check CPU/RAM/disk against per-node thresholds
	api.CheckNodeAlerts(db, notifier.Send)

	// Bandwidth quota alerts: check usage against configured thresholds
	api.CheckBandwidthQuotas(db, notifier.Send)

	// Bandwidth quota reset: reset current_usage_gb on the configured reset_day
	api.ResetBandwidthQuotas(db)

	// Protocol health checks: TCP connect test for each enabled protocol per node
	protocols.CheckProtocolHealth(db)

	// Node bandwidth quotas (Server-level): check nodes table bandwidth columns
	api.CheckNodeBandwidthQuotas(db, notifier.Send)

	// Reset monthly bandwidth counters on 1st of month
	api.ResetMonthlyNodeBandwidth(db)

	// Pending update health: fail stale update_agent tasks
	api.CheckPendingUpdateHealth(db, notifier.Send)

	// Excluded-feature worker operations (billing, SLA, teleproxy, load balancing)
	// No-op in lite build.
	workerTickExcluded(db, notifier, tickCount)
}

// loadBotConfigFromDB reads telegram_token and telegram_chat_id from the panel_settings table.
// Returns empty values if the table doesn't exist or the keys are not set.
func loadBotConfigFromDB(database *sql.DB) (token string, chatIDs []int64) {
	rows, err := database.Query(`SELECT setting_key, setting_value FROM panel_settings WHERE setting_key IN ('telegram_token', 'telegram_chat_id')`)
	if err != nil {
		// Table might not exist yet on first run
		return "", nil
	}
	defer rows.Close()
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		switch key {
		case "telegram_token":
			token = strings.TrimSpace(val)
		case "telegram_chat_id":
			for _, s := range strings.Split(val, ",") {
				s = strings.TrimSpace(s)
				if id, err := strconv.ParseInt(s, 10, 64); err == nil && id != 0 {
					chatIDs = append(chatIDs, id)
				}
			}
		}
	}
	return
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// startSocketListener starts an HTTP server on a Unix domain socket for local
// CLI communication. On Windows this is a no-op. Returns the listener (for
// shutdown) and any error. The caller should serve in a goroutine.
func startSocketListener(handler http.Handler, socketPath string) (net.Listener, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}

	// Remove stale socket file from a previous run.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove old socket: %w", err)
	}

	// Ensure the parent directory exists.
	if dir := filepath.Dir(socketPath); dir != "" {
		os.MkdirAll(dir, 0755)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", socketPath, err)
	}

	// Set socket file permissions to 0660 (owner + group read/write).
	if err := os.Chmod(socketPath, 0660); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	return ln, nil
}

func parseCertInfo(certPath string) (expiry string, issuer string) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return "", ""
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", ""
	}
	expiry = cert.NotAfter.Format(time.RFC3339)
	issuer = cert.Issuer.CommonName
	if issuer == "" && len(cert.Issuer.Organization) > 0 {
		issuer = cert.Issuer.Organization[0]
	}
	return
}

func generateNginxConfig(domain, panelAddr string, withSSL bool) string {
	if withSSL {
		return fmt.Sprintf(`# KorisPanel nginx config (auto-generated)
# Domain: %s | SSL: Let's Encrypt (managed by certbot)

server {
    listen 80;
    server_name %s;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name %s;
    client_max_body_size 20m;

    ssl_certificate /etc/letsencrypt/live/%s/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/%s/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    location = / { return 302 /dashboard/; }
    location = /dashboard { return 302 /dashboard/; }
    location /dashboard/ {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    location /api/ {
        proxy_pass http://%s;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    location = /portal { return 302 /portal/; }
    location /portal/ {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    location /portal/sub {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`, domain, domain, domain, domain, domain, panelAddr, panelAddr, panelAddr, panelAddr)
	}

	// HTTP only (no SSL)
	return fmt.Sprintf(`# KorisPanel nginx config (auto-generated)
# Domain: %s | SSL: disabled

server {
    listen 80 default_server;
    server_name %s;
    client_max_body_size 20m;

    location = / { return 302 /dashboard/; }
    location = /dashboard { return 302 /dashboard/; }
    location /dashboard/ {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    location /api/ {
        proxy_pass http://%s;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    location = /portal { return 302 /portal/; }
    location /portal/ {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    location /portal/sub {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`, domain, domain, panelAddr, panelAddr, panelAddr, panelAddr)
}

func isCLICommand(arg string) bool {
	commands := map[string]bool{
		"status":  true,
		"nodes":   true,
		"users":   true,
		"cleanup": true,
		"workers": true,
		"logs":    true,
		"update":  true,
		"help":    true,
		"--help":  true,
		"--json":  true,
	}
	return commands[arg]
}

// startTLSListener starts HTTPS (and HTTP redirect) listeners based on config.
// It supports two modes:
//   - Autocert (Let's Encrypt): when PANEL_DOMAIN is set and no custom cert/key files exist.
//     Creates an autocert.Manager, serves HTTPS on TLSAddr, and starts an HTTP->HTTPS
//     redirect on :80.
//   - Custom cert/key: when TLSCert and TLSKey files exist. Uses tls.LoadX509KeyPair
//     and serves HTTPS on TLSAddr.
//
// This function blocks (runs the HTTPS server). It should be called from a goroutine
// or as the final blocking call in main.
func startTLSListener(handler http.Handler, cfg config.Config) {
	domain := os.Getenv("PANEL_DOMAIN")

	// Mode 1: Custom cert/key files provided
	if fileExists(cfg.TLSCert) && fileExists(cfg.TLSKey) {
		logger.Info("tls", "starting HTTPS with custom cert/key", map[string]any{
			"cert": cfg.TLSCert,
			"key":  cfg.TLSKey,
			"addr": cfg.TLSAddr,
		})

		// Start HTTP->HTTPS redirect on :80
		go startHTTPRedirect(cfg.TLSAddr)

		if err := http.ListenAndServeTLS(cfg.TLSAddr, cfg.TLSCert, cfg.TLSKey, handler); err != nil {
			logger.Error("tls", "HTTPS server failed (custom cert)", map[string]any{"error": err.Error()})
		}
		return
	}

	// Mode 2: Autocert (Let's Encrypt) — requires PANEL_DOMAIN
	if domain == "" {
		logger.Error("tls", "TLS enabled but no PANEL_DOMAIN set and no custom cert/key found — cannot start HTTPS")
		logger.Error("tls", "set PANEL_DOMAIN for autocert or provide PANEL_TLS_CERT/PANEL_TLS_KEY files")
		return
	}

	// Ensure cert cache directory exists
	certDir := cfg.TLSCertDir
	if err := os.MkdirAll(certDir, 0700); err != nil {
		logger.Error("tls", "failed to create cert cache dir", map[string]any{"dir": certDir, "error": err.Error()})
		return
	}

	m := &autocert.Manager{
		Cache:      autocert.DirCache(certDir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domain),
	}

	tlsSrv := &http.Server{
		Addr:      cfg.TLSAddr,
		Handler:   handler,
		TLSConfig: m.TLSConfig(),
	}

	logger.Info("tls", "starting HTTPS with Let's Encrypt autocert", map[string]any{
		"domain":   domain,
		"addr":     cfg.TLSAddr,
		"cert_dir": certDir,
	})

	// Start HTTP challenge handler + redirect on :80
	go func() {
		// autocert.Manager.HTTPHandler handles ACME HTTP-01 challenges and
		// redirects all other traffic to HTTPS.
		httpSrv := &http.Server{
			Addr:    ":80",
			Handler: m.HTTPHandler(nil),
		}
		if err := httpSrv.ListenAndServe(); err != nil {
			logger.Error("tls", "HTTP challenge/redirect server error", map[string]any{"error": err.Error()})
		}
	}()

	if err := tlsSrv.ListenAndServeTLS("", ""); err != nil {
		logger.Error("tls", "HTTPS server failed (autocert)", map[string]any{"error": err.Error()})
	}
}

// startHTTPRedirect starts an HTTP server on :80 that redirects all traffic to HTTPS.
func startHTTPRedirect(tlsAddr string) {
	redirectMux := http.NewServeMux()
	redirectMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.RequestURI()
		if tlsAddr != ":443" {
			host := r.Host
			if idx := strings.Index(host, ":"); idx != -1 {
				host = host[:idx]
			}
			target = "https://" + host + tlsAddr + r.URL.RequestURI()
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
	redirectMux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"service":"panel","tls":true}`))
	})
	if err := http.ListenAndServe(":80", redirectMux); err != nil {
		logger.Error("tls", "HTTP redirect server error", map[string]any{"error": err.Error()})
	}
}

func main() {
	// Check for CLI mode first — before any heavy initialization.
	if len(os.Args) > 1 && isCLICommand(os.Args[1]) {
		c := cli.New(cli.WithOutput(os.Stdout))
		cli.RegisterDefaultCommands(c)
		if err := c.Execute(os.Args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Multi-worker manager mode detection.
	// If PANEL_WORKERS is set to a value > 1 (or "auto") and we are NOT a
	// child worker process, this process becomes the manager that forks and
	// monitors worker children. Worker children fall through to normal startup.
	if panelWorkers := os.Getenv("PANEL_WORKERS"); panelWorkers != "" && panelWorkers != "1" {
		isChild, _ := worker.IsWorkerProcess()
		if !isChild {
			// We are the master/manager process — fork workers and monitor.
			numWorkers := 0 // 0 = auto (resolved by Config.ResolvedWorkers)
			if n, err := strconv.Atoi(panelWorkers); err == nil && n > 1 {
				numWorkers = n
			}
			port := os.Getenv("PANEL_PORT")
			if port == "" {
				port = "8088"
			}
			graceSec := 30
			if gs := os.Getenv("PANEL_GRACEFUL_WAIT"); gs != "" {
				if v, err := strconv.Atoi(gs); err == nil && v > 0 {
					graceSec = v
				}
			}
			cfg := worker.Config{
				NumWorkers:   numWorkers,
				Addr:         ":" + port,
				GracefulWait: time.Duration(graceSec) * time.Second,
				MaxRestarts:  5,
			}
			mgr := worker.NewManager(cfg)
			ctx, cancel := context.WithCancel(context.Background())

			// Handle SIGINT/SIGTERM for the manager process.
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				cancel()
				mgr.Stop()
			}()

			if err := mgr.Start(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "manager error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		// If we ARE a worker child, fall through to normal server startup.
	}

	// Auto-tune for available CPU cores (respect env override)
	if os.Getenv("GOMAXPROCS") == "" {
		cores := runtime.NumCPU()
		if cores > 4 {
			cores = 4 // cap at 4 for a panel process
		}
		runtime.GOMAXPROCS(cores)
	}

	// GC tuning: balance throughput vs memory
	// GOGC=100 (default) is fine for 4GB — more throughput, less GC overhead
	// Only reduce for <2GB RAM
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(100)
	}
	// Memory limit: use 512MB on 4GB+ servers, 100MB on 1GB
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(512 * 1024 * 1024) // 512MB
	}

	cfg := config.Load()

	// Initialize structured TUI logger early — all subsequent logging uses this.
	logger = tui.New(tui.WithLevel(tui.LevelInfo))

	database, err := db.Open(cfg.DBDSN)
	if err != nil {
		logger.Error("main", "failed to open database", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	migDir := os.Getenv("PANEL_MIGRATIONS")
	if err := db.Migrate(database, migDir); err != nil {
		logger.Error("main", "database migration failed", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	// Start background ticker. In multi-worker mode only the leader worker
	// runs the ticker to avoid duplicate billing/cleanup. Leader election is
	// via an exclusive file lock — only one worker process can acquire it.
	if isChild, _ := worker.IsWorkerProcess(); isChild {
		ll := worker.NewLeaderLock("")
		if ll.TryAcquire() {
			logger.Info("worker", "acquired leader lock — running background ticker", map[string]any{"pid": os.Getpid()})
			startWorker(database)
		} else {
			logger.Info("worker", "not leader — skipping background ticker", map[string]any{"pid": os.Getpid()})
		}
	} else {
		// Single-process mode: always run the ticker.
		startWorker(database)
	}

	// Initialize backup service
	backupCfg := backup.LoadConfigFromDB(database)
	backupService := backup.New(database, backupCfg)
	backupService.StartScheduler()

	// Start certificate rotation worker
	certEventFn := func(eventType, severity, title, message string) {
		_, _ = database.Exec(`INSERT INTO events(type,severity,title,message,actor,related) VALUES(?,?,?,?,?,?)`,
			eventType, severity, title, message, "system", "")
	}
	certWorker := certrotation.New(database, certEventFn)
	certWorker.Start()

	// Start session enforcer (kills excess connections every 30s)
	enforcer := sessions.NewEnforcer(database)
	enforcer.Start()
	logger.Info("main", "session enforcer started")

	srv := api.New(database, cfg)
	// Embed pre-built frontend assets into the binary — no external www/ needed
	adminFS, _ := fs.Sub(web.AdminFS, "admin/www")
	portalFS, _ := fs.Sub(web.PortalFS, "portal/www")
	landingFS, _ := fs.Sub(web.LandingFS, "landing/www")
	srv.AdminEmbedFS = adminFS
	srv.PortalEmbedFS = portalFS
	srv.LandingEmbedFS = landingFS
	srv.BackupService = backupService
	srv.NodeMgr = nodeapi.NewNodeConnectionManager(database)
	srv.NodeMgr.NotifyFn = func(msg string) {
		srv.Notify.Send(msg)
	}

	// Initialize excluded services (billing, support, teleproxy, antidpi, payment)
	// No-op in lite build.
	initExcludedServices(srv, database)
	srv.LogEntries = func(n int) []tui.LogEntry {
		return logger.LastEntries(n)
	}

	mux := srv.Routes()

	// Start Telegram bot (no-op in lite build)
	startBot(database, srv, mux)

	// Certificate status endpoint
	mux.HandleFunc("/api/admin/cert-status", srv.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		certPath := cfg.TLSCert
		keyPath := cfg.TLSKey
		certExists := fileExists(certPath)
		keyExists := fileExists(keyPath)
		tlsActive := certExists && keyExists && r.TLS != nil
		result := map[string]any{
			"ok":          true,
			"cert_exists": certExists,
			"key_exists":  keyExists,
			"tls_active":  tlsActive,
			"tls_addr":    cfg.TLSAddr,
			"cert_path":   certPath,
			"key_path":    keyPath,
			"expiry":      "",
			"issuer":      "",
		}
		if certExists {
			expiry, issuer := parseCertInfo(certPath)
			result["expiry"] = expiry
			result["issuer"] = issuer
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))

	// Certificate upload endpoint
	mux.HandleFunc("/api/admin/cert-upload", srv.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"ok":false,"error":"invalid multipart form"}`))
			return
		}
		certFile, _, err := r.FormFile("cert")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"ok":false,"error":"cert file required"}`))
			return
		}
		defer certFile.Close()
		keyFile, _, err := r.FormFile("key")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"ok":false,"error":"key file required"}`))
			return
		}
		defer keyFile.Close()

		certData, _ := io.ReadAll(certFile)
		keyData, _ := io.ReadAll(keyFile)

		// Validate that cert and key form a valid TLS pair
		if _, err := tls.X509KeyPair(certData, keyData); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid certificate/key pair: " + err.Error()})
			return
		}

		// Save to the configured paths
		certPath := cfg.TLSCert
		keyPath := cfg.TLSKey
		os.MkdirAll(filepath.Dir(certPath), 0755)
		if err := os.WriteFile(certPath, certData, 0600); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"ok":false,"error":"failed to save cert"}`))
			return
		}
		if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"ok":false,"error":"failed to save key"}`))
			return
		}

		// Parse cert info for response
		expiry, issuer := parseCertInfo(certPath)

		logger.Info("tls", "new certificate uploaded — restart required for HTTPS", map[string]any{"expiry": expiry, "issuer": issuer})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":               true,
			"message":          "Certificate saved. Restart the panel service to enable HTTPS.",
			"restart_required": true,
			"expiry":           expiry,
			"issuer":           issuer,
			"tls_addr":         cfg.TLSAddr,
		})
	}))

	// ─── Nginx & Domain Management ─────────────────────────────────────────
	mux.HandleFunc("/api/admin/domain", srv.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			domain := ""
			_ = database.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key='panel_domain'`).Scan(&domain)
			sslActive := fileExists("/etc/letsencrypt/live/" + domain + "/fullchain.pem")
			var expiry, issuer string
			if sslActive {
				expiry, issuer = parseCertInfo("/etc/letsencrypt/live/" + domain + "/fullchain.pem")
			} else if fileExists(cfg.TLSCert) {
				expiry, issuer = parseCertInfo(cfg.TLSCert)
				sslActive = true
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":         true,
				"domain":     domain,
				"ssl_active": sslActive,
				"expiry":     expiry,
				"issuer":     issuer,
			})

		case http.MethodPost:
			var in struct {
				Domain string `json:"domain"`
				SSL    bool   `json:"ssl"`
				Email  string `json:"email"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "bad_json"})
				return
			}
			in.Domain = strings.TrimSpace(in.Domain)
			if in.Domain == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "domain_required"})
				return
			}
			_, _ = database.Exec(`INSERT INTO panel_settings(setting_key,setting_value) VALUES('panel_domain',?) ON DUPLICATE KEY UPDATE setting_value=?`, in.Domain, in.Domain)

			panelAddr := cfg.Addr
			if panelAddr == "" {
				panelAddr = "127.0.0.1:8088"
			}
			nginxConf := generateNginxConfig(in.Domain, panelAddr, in.SSL)
			nginxPath := "/etc/nginx/sites-available/koris-panel.conf"
			enabledPath := "/etc/nginx/sites-enabled/panel-next.conf"
			if err := os.WriteFile(nginxPath, []byte(nginxConf), 0644); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "write_nginx: " + err.Error()})
				return
			}
			os.Remove(enabledPath)
			os.Symlink(nginxPath, enabledPath)

			testCmd := exec.Command("nginx", "-t")
			if out, err := testCmd.CombinedOutput(); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "nginx_test_failed: " + string(out)})
				return
			}
			_ = exec.Command("systemctl", "reload", "nginx").Run()

			sslResult := ""
			if in.SSL {
				args := []string{"--nginx", "-d", in.Domain, "--non-interactive", "--agree-tos", "--redirect"}
				if in.Email != "" {
					args = append(args, "--email", in.Email)
				} else {
					args = append(args, "--register-unsafely-without-email")
				}
				out, err := exec.Command("certbot", args...).CombinedOutput()
				if err != nil {
					sslResult = "certbot_failed: " + string(out)
					logger.Error("domain", "certbot failed", map[string]any{"domain": in.Domain, "output": string(out)})
				} else {
					sslResult = "ssl_installed"
					logger.Info("domain", "SSL installed", map[string]any{"domain": in.Domain})
					certSrc := "/etc/letsencrypt/live/" + in.Domain + "/fullchain.pem"
					keySrc := "/etc/letsencrypt/live/" + in.Domain + "/privkey.pem"
					if fileExists(certSrc) && fileExists(keySrc) {
						os.MkdirAll("/etc/panel", 0755)
						exec.Command("cp", certSrc, cfg.TLSCert).Run()
						exec.Command("cp", keySrc, cfg.TLSKey).Run()
					}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":         true,
				"domain":     in.Domain,
				"ssl_result": sslResult,
				"nginx":      "configured",
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	logger.Info("main", "panel listening", map[string]any{"addr": cfg.Addr})

	// Notify systemd that the service is ready (no-op on non-Linux or without systemd).
	daemon.SdNotify(false, daemon.SdNotifyReady)
	logger.Info("main", "sent sd_notify ready")

	// Start systemd watchdog heartbeat if WATCHDOG_USEC is configured.
	startWatchdog(database)

	// Rate limiter: 30 requests/sec per IP, burst 60
	limiter := ratelimit.New(30, 60, cfg.TrustedProxies)

	// Apply no-cache middleware on API responses
	handler := api.NoCacheMiddleware(mux)

	// ─── Unix Socket Listener (Linux only, for local CLI) ──────────────────
	socketPath := os.Getenv("PANEL_SOCKET_PATH")
	if socketPath == "" {
		socketPath = "/var/run/panel.sock"
	}

	socketLn, sockErr := startSocketListener(handler, socketPath)
	if sockErr != nil {
		logger.Warn("main", "unix socket listener failed (CLI will use HTTP fallback)", map[string]any{"error": sockErr.Error(), "path": socketPath})
	} else if socketLn != nil {
		logger.Info("main", "unix socket listener started", map[string]any{"path": socketPath})
		go func() {
			if err := http.Serve(socketLn, handler); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
				logger.Error("main", "unix socket serve error", map[string]any{"error": err.Error()})
			}
		}()
	}

	// Graceful shutdown: clean up socket file on SIGINT/SIGTERM.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("main", "shutting down...")
		if socketLn != nil {
			socketLn.Close()
			os.Remove(socketPath)
		}
		os.Exit(0)
	}()

	// Start server: use TLS if explicitly enabled via PANEL_TLS_ENABLED=true,
	// OR if cert and key files exist AND the panel is NOT behind a reverse proxy.
	// Detection: if PANEL_ADDR is bound to loopback (127.0.0.1), assume nginx handles TLS.
	// To force direct TLS even on loopback, set PANEL_TLS_DIRECT=true.
	if cfg.TLSEnabled {
		// New built-in TLS mode: autocert or custom cert/key
		logger.Info("tls", "PANEL_TLS_ENABLED=true — starting built-in TLS", map[string]any{"addr": cfg.TLSAddr})

		// Start the plain HTTP server in the background (for any non-TLS traffic or fallback)
		go func() {
			logger.Info("main", "HTTP listener (fallback/internal)", map[string]any{"addr": cfg.Addr})
			if err := http.ListenAndServe(cfg.Addr, limiter.Middleware(handler)); err != nil {
				logger.Error("main", "HTTP server failed", map[string]any{"error": err.Error()})
			}
		}()

		// startTLSListener blocks — it handles autocert or custom cert mode
		startTLSListener(limiter.Middleware(handler), cfg)
	} else {
		tlsCert := cfg.TLSCert
		tlsKey := cfg.TLSKey
		behindProxy := strings.HasPrefix(cfg.Addr, "127.") || strings.HasPrefix(cfg.Addr, "localhost")
		forceTLS := strings.ToLower(os.Getenv("PANEL_TLS_DIRECT")) == "true"

		if fileExists(tlsCert) && fileExists(tlsKey) && (!behindProxy || forceTLS) {
			logger.Info("tls", "TLS enabled (legacy mode)", map[string]any{"cert": tlsCert, "key": tlsKey, "addr": cfg.TLSAddr})
			logger.Info("tls", "HTTP redirect configured", map[string]any{"from": cfg.Addr, "to": cfg.TLSAddr})

			// Start HTTP server that redirects to HTTPS
			go func() {
				redirectMux := http.NewServeMux()
				redirectMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					target := "https://" + r.Host + r.URL.RequestURI()
					if cfg.TLSAddr != ":443" {
						host := r.Host
						if idx := strings.Index(host, ":"); idx != -1 {
							host = host[:idx]
						}
						target = "https://" + host + cfg.TLSAddr + r.URL.RequestURI()
					}
					http.Redirect(w, r, target, http.StatusMovedPermanently)
				})
				redirectMux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"ok":true,"service":"panel","tls":true}`))
				})
				if err := http.ListenAndServe(cfg.Addr, redirectMux); err != nil {
					logger.Error("tls", "HTTP redirect server error", map[string]any{"error": err.Error()})
				}
			}()

			// Start HTTPS server — with fallback to HTTP if TLS fails
			tlsErr := make(chan error, 1)
			go func() {
				err := http.ListenAndServeTLS(cfg.TLSAddr, tlsCert, tlsKey, limiter.Middleware(handler))
				tlsErr <- err
			}()

			// Give TLS server a moment to start or fail
			select {
			case err := <-tlsErr:
				// TLS failed to start — fall back to plain HTTP
				logger.Error("tls", "TLS server failed", map[string]any{"error": err.Error()})
				logger.Warn("tls", "falling back to plain HTTP — fix your certificate and restart", map[string]any{"addr": cfg.Addr})
				if httpErr := http.ListenAndServe(cfg.Addr, limiter.Middleware(handler)); httpErr != nil {
					logger.Error("main", "HTTP server failed", map[string]any{"error": httpErr.Error()})
					os.Exit(1)
				}
			case <-time.After(2 * time.Second):
				// TLS started OK, block on redirect server (already running in goroutine)
				logger.Info("tls", "HTTPS server running", map[string]any{"addr": cfg.TLSAddr})
				select {} // block forever
			}
		} else {
			if fileExists(tlsCert) && fileExists(tlsKey) && behindProxy {
				logger.Info("tls", "TLS available but behind reverse proxy — nginx handles TLS", map[string]any{"addr": cfg.Addr})
				logger.Info("tls", "set PANEL_TLS_DIRECT=true to serve TLS directly from Go")
			} else if !fileExists(tlsCert) || !fileExists(tlsKey) {
				logger.Info("tls", "TLS disabled: cert/key not found", map[string]any{"cert": tlsCert, "key": tlsKey})
			}
			if httpErr := http.ListenAndServe(cfg.Addr, limiter.Middleware(handler)); httpErr != nil {
				logger.Error("main", "HTTP server failed", map[string]any{"error": httpErr.Error()})
				os.Exit(1)
			}
		}
	}
}
