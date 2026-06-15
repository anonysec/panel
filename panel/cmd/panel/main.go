package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"koris-next/panel/internal/api"
	"koris-next/panel/internal/config"
	"koris-next/panel/internal/db"
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

func startWorker(database *sql.DB) {
	ticker := time.NewTicker(time.Minute)
	go func() {
		for t := range ticker.C {
			if _, err := database.Exec(`UPDATE customers c JOIN (SELECT username, MAX(expires_at) as max_expires FROM subscriptions WHERE status='active' GROUP BY username) s ON c.username=s.username SET c.status='expired' WHERE c.status='active' AND s.max_expires <= NOW()`); err != nil {
				log.Printf("[worker] expire subscriptions: %v", err)
			}
			if _, err := database.Exec(`UPDATE customers c JOIN radcheck r ON c.username=r.username AND r.attribute='Max-Data' JOIN (SELECT username, COALESCE(SUM(acctinputoctets+acctoutputoctets),0) AS used FROM radacct GROUP BY username) a ON c.username=a.username SET c.status='limited' WHERE c.status='active' AND CAST(r.value AS UNSIGNED) > 0 AND a.used >= CAST(r.value AS UNSIGNED)`); err != nil {
				log.Printf("[worker] data limit enforcement: %v", err)
			}
			if _, err := database.Exec(`UPDATE radacct SET acctstoptime=NOW(), acctterminatecause='Stalled session' WHERE acctstoptime IS NULL AND acctupdatetime < (NOW() - INTERVAL 5 MINUTE)`); err != nil {
				log.Printf("[worker] stale session cleanup: %v", err)
			}
			if _, err := database.Exec(`UPDATE nodes SET status='offline' WHERE status IN('online','stale') AND last_seen_at < (NOW() - INTERVAL 5 MINUTE)`); err != nil {
				log.Printf("[worker] node offline mark: %v", err)
			}
			if t.Hour() == 2 && t.Minute() == 0 {
				if err := runBackup(); err != nil {
					log.Printf("[worker] backup failed: %v", err)
				}
			}
		}
	}()
}

func runBackup() error {
	dir := "/var/backups/koris-next"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	file := filepath.Join(dir, fmt.Sprintf("db-%s.sql.gz", time.Now().Format("2006-01-02")))
	user, pass, dbname := mysqlCredsFromDSN(os.Getenv("PANEL_DB_DSN"))
	if dbname == "" {
		dbname = "radius_next"
	}

	out, err := os.Create(file)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer out.Close()

	// Pipe mysqldump through gzip for proper compression
	dump := exec.Command("mysqldump", "-u", user, "-p"+pass, "--single-transaction", dbname)
	gzip := exec.Command("gzip", "-9")

	gzip.Stdout = out

	pipe, err := dump.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	gzip.Stdin = pipe

	if err := dump.Start(); err != nil {
		return fmt.Errorf("start mysqldump: %w", err)
	}
	if err := gzip.Start(); err != nil {
		_ = dump.Wait()
		return fmt.Errorf("start gzip: %w", err)
	}

	dumpErr := dump.Wait()
	gzipErr := gzip.Wait()

	if dumpErr != nil {
		return fmt.Errorf("mysqldump: %w", dumpErr)
	}
	if gzipErr != nil {
		return fmt.Errorf("gzip: %w", gzipErr)
	}

	log.Printf("[worker] backup completed: %s", file)
	return nil
}

func main() {
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
	srv := api.New(database, cfg)

	httpServer := &http.Server{
		Addr:         cfg.Addr,
		Handler:      srv.Routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("panel listening on %s", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}

	_ = database.Close()
	log.Println("panel stopped")
}
