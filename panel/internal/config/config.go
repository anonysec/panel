package config

import (
	"log"
	"os"
)

type Config struct {
	Addr          string
	DBDSN         string
	SetupKey      string
	SessionSecret string
	Version       string
	PublicBase    string
	AdminWebDir   string
	PortalWebDir  string
}

func Load() Config {
	setupKey := getenv("PANEL_SETUP_KEY", "")
	sessionSecret := getenv("PANEL_SESSION_SECRET", "")

	if sessionSecret == "" {
		sessionSecret = setupKey
	}
	if sessionSecret == "" {
		sessionSecret = "koris-next-dev-session-secret"
		log.Println("[SECURITY WARNING] PANEL_SESSION_SECRET is not set. Using insecure default. Set PANEL_SESSION_SECRET in production!")
	}

	dbDSN := getenv("PANEL_DB_DSN", "")
	if dbDSN == "" {
		log.Println("[SECURITY WARNING] PANEL_DB_DSN is not set. Using insecure default credentials. Set PANEL_DB_DSN in production!")
		dbDSN = "radius:RadiusDb2026@tcp(127.0.0.1:3306)/radius?parseTime=true&multiStatements=true&charset=utf8mb4,utf8"
	}

	if setupKey == "" {
		log.Println("[SECURITY WARNING] PANEL_SETUP_KEY is not set. The initial owner setup endpoint will not require a key.")
	}

	return Config{
		Addr:          getenv("PANEL_ADDR", ":8080"),
		DBDSN:         dbDSN,
		SetupKey:      setupKey,
		SessionSecret: sessionSecret,
		Version:       getenv("PANEL_VERSION", "next-dev"),
		PublicBase:    getenv("PANEL_PUBLIC_BASE", "/dashboard"),
		AdminWebDir:   getenv("PANEL_ADMIN_WEB_DIR", "/opt/koris-next/panel/web/admin/www"),
		PortalWebDir:  getenv("PANEL_PORTAL_WEB_DIR", "/opt/koris-next/panel/web/portal/www"),
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
