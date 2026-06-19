package main

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"KorisPanel/panel/internal/api"
	"KorisPanel/panel/internal/backup"
	"KorisPanel/panel/internal/bot"
	"KorisPanel/panel/internal/certrotation"
	"KorisPanel/panel/internal/config"
	"KorisPanel/panel/internal/db"
	"KorisPanel/panel/internal/notify"
	"KorisPanel/panel/internal/ratelimit"
	"KorisPanel/panel/internal/sessions"
)

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
	go func() {
		for range ticker.C {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[worker] recovered from panic: %v", r)
					}
				}()
				workerTick(db, notifier)
			}()
		}
	}()
}

func workerTick(db *sql.DB, notifier *notify.Notifier) {
	// Find customers about to expire (for WireGuard auto-revocation)
	expRows, expErr := db.Query(`SELECT c.id FROM customers c JOIN (SELECT username, MAX(expires_at) as max_expires FROM subscriptions WHERE status='active' GROUP BY username) s ON c.username=s.username WHERE c.status='active' AND s.max_expires <= NOW()`)
	var expiringCustomerIDs []int64
	if expErr == nil {
		for expRows.Next() {
			var cid int64
			if expRows.Scan(&cid) == nil {
				expiringCustomerIDs = append(expiringCustomerIDs, cid)
			}
		}
		expRows.Close()
	}

	if _, err := db.Exec(`UPDATE customers c JOIN (SELECT username, MAX(expires_at) as max_expires FROM subscriptions WHERE status='active' GROUP BY username) s ON c.username=s.username SET c.status='expired' WHERE c.status='active' AND s.max_expires <= NOW()`); err != nil {
		log.Printf("[worker] expire subscriptions: %v", err)
	}

	// Auto-revoke WireGuard peers for expired customers
	for _, cid := range expiringCustomerIDs {
		api.AutoRevokeWireGuardPeersByDB(db, cid)
	}

	if _, err := db.Exec(`UPDATE customers c JOIN radcheck r ON c.username=r.username AND r.attribute='Max-Data' JOIN (SELECT username, COALESCE(SUM(acctinputoctets+acctoutputoctets),0) AS used FROM radacct GROUP BY username) a ON c.username=a.username SET c.status='limited' WHERE c.status='active' AND CAST(r.value AS UNSIGNED) > 0 AND a.used >= CAST(r.value AS UNSIGNED)`); err != nil {
		log.Printf("[worker] data limit enforcement: %v", err)
	}
	_, _ = db.Exec(`UPDATE radacct SET acctstoptime=NOW(), acctterminatecause='Stalled session' WHERE acctstoptime IS NULL AND acctupdatetime < (NOW() - INTERVAL 5 MINUTE)`)

	// Mark nodes offline and notify via Telegram
	rows, err := db.Query(`SELECT name, public_ip FROM nodes WHERE status IN('online','stale') AND last_seen_at < (NOW() - INTERVAL 5 MINUTE)`)
	if err == nil {
		for rows.Next() {
			var name, ip string
			if rows.Scan(&name, &ip) == nil {
				notifier.NotifyNodeOffline(name, ip)
			}
		}
		rows.Close()
	}
	_, _ = db.Exec(`UPDATE nodes SET status='offline' WHERE status IN('online','stale') AND last_seen_at < (NOW() - INTERVAL 5 MINUTE)`)

	// PAYG Billing: deduct from wallet based on usage for pay-as-you-go plans
	processPaygBilling(db)

	// Data retention: prune old snapshots to prevent unbounded growth
	// Keep last 7 days of node_usage_snapshots, last 24h of user_bandwidth_snapshots
	_, _ = db.Exec(`DELETE FROM node_usage_snapshots WHERE created_at < NOW() - INTERVAL 7 DAY`)
	_, _ = db.Exec(`DELETE FROM user_bandwidth_snapshots WHERE created_at < NOW() - INTERVAL 24 HOUR`)
}

// processPaygBilling deducts wallet credit for customers on pay-as-you-go plans
// based on data usage (per GB) and time (per day).
func processPaygBilling(db *sql.DB) {
	type paygCustomer struct {
		ID               int64
		Username         string
		PlanID           int64
		PricePerGB       float64
		PricePerDay      float64
		DisconnectOnZero bool
		Credit           float64
	}

	rows, err := db.Query(`
		SELECT c.id, c.username, p.id, p.price_per_gb, p.price_per_day, p.disconnect_on_zero, w.credit
		FROM customers c
		JOIN plans p ON p.id = c.plan_id AND p.billing_type = 'payg'
		JOIN wallets w ON w.username = c.username
		WHERE c.status = 'active' AND c.deleted_at IS NULL
	`)
	if err != nil {
		log.Printf("[worker] payg billing query: %v", err)
		return
	}
	defer rows.Close()

	var customers []paygCustomer
	for rows.Next() {
		var c paygCustomer
		var disconn int
		if err := rows.Scan(&c.ID, &c.Username, &c.PlanID, &c.PricePerGB, &c.PricePerDay, &disconn, &c.Credit); err != nil {
			log.Printf("[worker] payg scan: %v", err)
			continue
		}
		c.DisconnectOnZero = disconn == 1
		customers = append(customers, c)
	}

	for _, c := range customers {
		// Get last deduction time for this user
		var lastDeduction time.Time
		err := db.QueryRow(`SELECT COALESCE(MAX(created_at), '2000-01-01') FROM payg_deductions WHERE username = ?`, c.Username).Scan(&lastDeduction)
		if err != nil {
			log.Printf("[worker] payg last deduction for %s: %v", c.Username, err)
			continue
		}

		// Calculate data used since last deduction (in bytes from radacct)
		var dataUsedBytes int64
		err = db.QueryRow(`
			SELECT COALESCE(SUM(acctinputoctets + acctoutputoctets), 0)
			FROM radacct
			WHERE username = ? AND (acctstarttime >= ? OR (acctstoptime IS NULL AND acctupdatetime >= ?))
		`, c.Username, lastDeduction, lastDeduction).Scan(&dataUsedBytes)
		if err != nil {
			log.Printf("[worker] payg data usage for %s: %v", c.Username, err)
			continue
		}

		// Calculate days since last deduction
		daysSinceLastDeduction := time.Since(lastDeduction).Hours() / 24.0

		// Calculate charges
		dataGB := float64(dataUsedBytes) / (1024 * 1024 * 1024)
		dataCharge := dataGB * c.PricePerGB
		timeCharge := daysSinceLastDeduction * c.PricePerDay
		totalCharge := dataCharge + timeCharge

		// Only deduct if charge is meaningful (> $0.001)
		if totalCharge < 0.001 {
			continue
		}

		balanceBefore := c.Credit
		balanceAfter := balanceBefore - totalCharge

		// Deduct from wallet
		_, err = db.Exec(`UPDATE wallets SET credit = credit - ? WHERE username = ?`, totalCharge, c.Username)
		if err != nil {
			log.Printf("[worker] payg wallet deduct for %s: %v", c.Username, err)
			continue
		}

		// Record data deduction if applicable
		if dataCharge > 0.001 {
			_, _ = db.Exec(`INSERT INTO payg_deductions(customer_id, username, plan_id, deduction_type, amount, usage_value, balance_before, balance_after) VALUES(?,?,?,?,?,?,?,?)`,
				c.ID, c.Username, c.PlanID, "data", dataCharge, dataGB, balanceBefore, balanceAfter)
		}

		// Record time deduction if applicable
		if timeCharge > 0.001 {
			_, _ = db.Exec(`INSERT INTO payg_deductions(customer_id, username, plan_id, deduction_type, amount, usage_value, balance_before, balance_after) VALUES(?,?,?,?,?,?,?,?)`,
				c.ID, c.Username, c.PlanID, "time", timeCharge, daysSinceLastDeduction, balanceBefore, balanceAfter)
		}

		// If wallet credit <= 0 and disconnect_on_zero, limit the customer
		if balanceAfter <= 0 && c.DisconnectOnZero {
			_, _ = db.Exec(`UPDATE customers SET status = 'limited' WHERE id = ? AND status = 'active'`, c.ID)
			// Disconnect active sessions
			_, _ = db.Exec(`UPDATE radacct SET acctstoptime = NOW(), acctterminatecause = 'PAYG-Zero-Balance' WHERE username = ? AND acctstoptime IS NULL`, c.Username)
			log.Printf("[worker] payg: disconnected %s (zero balance)", c.Username)
		}
	}
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

func main() {
	// Optimize for single-core servers
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(1)
	}

	// Optimize GC for low-memory environments (1GB RAM)
	// GOGC=50 means GC triggers at 50% heap growth (more frequent but lower peak memory)
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(50)
	}
	// Set soft memory limit to 100MB for the Go process
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(100 * 1024 * 1024) // 100MB
	}

	cfg := config.Load()
	database, err := db.Open(cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	migDir := os.Getenv("PANEL_MIGRATIONS")
	if err := db.Migrate(database, migDir); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	startWorker(database)

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
	log.Println("[main] session enforcer started")

	srv := api.New(database, cfg)
	srv.BackupService = backupService

	// Start Telegram bot
	// Load bot config from DB first, env vars override
	dbToken, dbChatIDs := loadBotConfigFromDB(database)
	botToken := os.Getenv("PANEL_TG_BOT_TOKEN")
	if botToken == "" {
		botToken = dbToken
	}
	botEnabled := strings.ToLower(os.Getenv("PANEL_TG_ENABLED")) == "true"
	if !botEnabled && botToken != "" {
		// If token exists (from DB or env) but PANEL_TG_ENABLED is not explicitly set,
		// auto-enable if token is present
		if os.Getenv("PANEL_TG_ENABLED") == "" && botToken != "" {
			botEnabled = true
		}
	}
	var adminChats []int64
	envChatID := os.Getenv("PANEL_TG_CHAT_ID")
	if envChatID != "" {
		for _, s := range strings.Split(envChatID, ",") {
			s = strings.TrimSpace(s)
			if id, err := strconv.ParseInt(s, 10, 64); err == nil && id != 0 {
				adminChats = append(adminChats, id)
			}
		}
	} else {
		adminChats = dbChatIDs
	}
	telegramBot := bot.New(bot.Config{
		Token:      botToken,
		AdminChats: adminChats,
		Enabled:    botEnabled,
	}, database)
	telegramBot.Start()

	mux := srv.Routes()

	// Bot restart endpoint (hot-reload)
	mux.HandleFunc("/api/admin/bot/restart", srv.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Re-read config from DB
		newToken, newChatIDs := loadBotConfigFromDB(database)
		envToken := os.Getenv("PANEL_TG_BOT_TOKEN")
		if envToken != "" {
			newToken = envToken
		}
		envChat := os.Getenv("PANEL_TG_CHAT_ID")
		var chats []int64
		if envChat != "" {
			for _, s := range strings.Split(envChat, ",") {
				s = strings.TrimSpace(s)
				if id, err := strconv.ParseInt(s, 10, 64); err == nil && id != 0 {
					chats = append(chats, id)
				}
			}
		} else {
			chats = newChatIDs
		}
		enabled := newToken != ""
		telegramBot.Restart(bot.Config{
			Token:      newToken,
			AdminChats: chats,
			Enabled:    enabled,
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"message":"bot restarted"}`))
	}))

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

		log.Printf("[tls] new certificate uploaded, expires=%s issuer=%s — restart required for HTTPS", expiry, issuer)
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
					log.Printf("[domain] certbot failed for %s: %s", in.Domain, string(out))
				} else {
					sslResult = "ssl_installed"
					log.Printf("[domain] SSL installed for %s", in.Domain)
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

	log.Printf("panel listening on %s", cfg.Addr)

	// Rate limiter: 10 requests/sec per IP, burst 30
	limiter := ratelimit.New(10, 30, cfg.TrustedProxies)

	// Apply no-cache middleware on API responses
	handler := api.NoCacheMiddleware(mux)

	// Start server: use TLS if cert and key files exist AND the panel is NOT behind a reverse proxy.
	// Detection: if PANEL_ADDR is bound to loopback (127.0.0.1), assume nginx handles TLS.
	// To force direct TLS even on loopback, set PANEL_TLS_DIRECT=true.
	tlsCert := cfg.TLSCert
	tlsKey := cfg.TLSKey
	behindProxy := strings.HasPrefix(cfg.Addr, "127.") || strings.HasPrefix(cfg.Addr, "localhost")
	forceTLS := strings.ToLower(os.Getenv("PANEL_TLS_DIRECT")) == "true"

	if fileExists(tlsCert) && fileExists(tlsKey) && (!behindProxy || forceTLS) {
		log.Printf("TLS enabled: cert=%s key=%s addr=%s", tlsCert, tlsKey, cfg.TLSAddr)
		log.Printf("HTTP redirect: %s -> %s", cfg.Addr, cfg.TLSAddr)

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
				log.Printf("HTTP redirect server error: %v", err)
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
			log.Printf("[CRITICAL] TLS server failed: %v", err)
			log.Printf("[FALLBACK] Starting plain HTTP on %s — fix your certificate and restart", cfg.Addr)
			log.Fatal(http.ListenAndServe(cfg.Addr, limiter.Middleware(handler)))
		case <-time.After(2 * time.Second):
			// TLS started OK, block on redirect server (already running in goroutine)
			log.Printf("HTTPS server running on %s", cfg.TLSAddr)
			select {} // block forever
		}
	} else {
		if fileExists(tlsCert) && fileExists(tlsKey) && behindProxy {
			log.Printf("TLS available but panel is behind reverse proxy (%s) — nginx handles TLS", cfg.Addr)
			log.Printf("Set PANEL_TLS_DIRECT=true to serve TLS directly from Go")
		} else if !fileExists(tlsCert) || !fileExists(tlsKey) {
			log.Printf("TLS disabled: cert/key not found at %s / %s", tlsCert, tlsKey)
		}
		log.Fatal(http.ListenAndServe(cfg.Addr, limiter.Middleware(handler)))
	}
}
