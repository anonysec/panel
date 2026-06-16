package main

import (
	"database/sql"
	"fmt"
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

	"koris-next/panel/internal/api"
	"koris-next/panel/internal/bot"
	"koris-next/panel/internal/config"
	"koris-next/panel/internal/db"
	"koris-next/panel/internal/notify"
	"koris-next/panel/internal/ratelimit"
	"koris-next/panel/internal/sessions"
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
		for t := range ticker.C {
			if _, err := db.Exec(`UPDATE customers c JOIN (SELECT username, MAX(expires_at) as max_expires FROM subscriptions WHERE status='active' GROUP BY username) s ON c.username=s.username SET c.status='expired' WHERE c.status='active' AND s.max_expires <= NOW()`); err != nil {
				log.Printf("[worker] expire subscriptions: %v", err)
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

			if t.Hour() == 2 && t.Minute() == 0 {
				dir := "/var/backups/koris-next"
				_ = os.MkdirAll(dir, 0755)
				file := filepath.Join(dir, fmt.Sprintf("db-%s.sql.gz", t.Format("2006-01-02")))
				user, pass, dbname := mysqlCredsFromDSN(os.Getenv("PANEL_DB_DSN"))
				if dbname == "" {
					dbname = "radius_next"
				}
				// Use MYSQL_PWD environment variable instead of -p flag to prevent
				// password exposure in process list (visible via ps aux or /proc/*/cmdline).
				cmd := exec.Command("mysqldump", "-u", user, dbname)
				cmd.Env = append(os.Environ(), "MYSQL_PWD="+pass)
				out, err := os.Create(file)
				if err == nil {
					cmd.Stdout = out
					_ = cmd.Run()
					_ = out.Close()
				}
			}
		}
	}()
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

	// Start session enforcer (kills excess connections every 30s)
	enforcer := sessions.NewEnforcer(database)
	enforcer.Start()
	log.Println("[main] session enforcer started")

	srv := api.New(database, cfg)

	// Start Telegram bot
	botToken := os.Getenv("PANEL_TG_BOT_TOKEN")
	botEnabled := strings.ToLower(os.Getenv("PANEL_TG_ENABLED")) == "true"
	botWebhook := os.Getenv("PANEL_TG_WEBHOOK_URL")
	var adminChats []int64
	for _, s := range strings.Split(os.Getenv("PANEL_TG_CHAT_ID"), ",") {
		s = strings.TrimSpace(s)
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id != 0 {
			adminChats = append(adminChats, id)
		}
	}
	telegramBot := bot.New(bot.Config{
		Token:      botToken,
		AdminChats: adminChats,
		WebhookURL: botWebhook,
		Enabled:    botEnabled,
	}, database)
	telegramBot.Start()

	mux := srv.Routes()
	// Register webhook handler if in webhook mode
	if botWebhook != "" && botEnabled {
		mux.HandleFunc("/api/bot/webhook", telegramBot.WebhookHandler())
		log.Printf("[bot] webhook endpoint registered at /api/bot/webhook")
	}

	log.Printf("panel listening on %s", cfg.Addr)

	// Rate limiter: 10 requests/sec per IP, burst 30
	limiter := ratelimit.New(10, 30, cfg.TrustedProxies)

	// Apply no-cache middleware on API responses
	handler := api.NoCacheMiddleware(mux)

	log.Fatal(http.ListenAndServe(cfg.Addr, limiter.Middleware(handler)))
}
