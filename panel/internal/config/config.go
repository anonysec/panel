package config

import (
	"log"
	"os"
	"strings"
)

type Config struct {
	Addr           string
	DBDSN          string
	SetupKey       string
	SessionSecret  string
	Version        string
	PublicBase     string
	AdminWebDir    string
	PortalWebDir   string
	TemplateDir    string
	SecureCookies  bool
	TrustedProxies []string
	AllowedOrigins []string
}

func Load() Config {
	devMode := os.Getenv("PANEL_DEV_MODE") == "true"

	setupKey := getenv("PANEL_SETUP_KEY", "")
	sessionSecret := getenv("PANEL_SESSION_SECRET", "")

	if sessionSecret == "" {
		sessionSecret = setupKey
	}
	if sessionSecret == "" {
		if !devMode {
			log.Fatalf("FATAL: PANEL_SESSION_SECRET is required in production. Set PANEL_DEV_MODE=true for development.")
		}
		sessionSecret = "KorisPanel-dev-session-secret"
		log.Println("[SECURITY WARNING] PANEL_SESSION_SECRET is not set. Using insecure default. Set PANEL_SESSION_SECRET in production!")
	}

	if !devMode && len(sessionSecret) < 32 {
		log.Fatalf("FATAL: PANEL_SESSION_SECRET must be at least 32 characters in production (got %d). Set PANEL_DEV_MODE=true for development.", len(sessionSecret))
	}

	dbDSN := getenv("PANEL_DB_DSN", "")
	if dbDSN == "" {
		if !devMode {
			log.Fatalf("FATAL: PANEL_DB_DSN is required in production. Set PANEL_DEV_MODE=true for development.")
		}
		log.Println("[SECURITY WARNING] PANEL_DB_DSN is not set. Using insecure default credentials. Set PANEL_DB_DSN in production!")
		dbDSN = "radius:RadiusDb2026@tcp(127.0.0.1:3306)/radius?parseTime=true&multiStatements=true&charset=utf8mb4,utf8"
	}

	if setupKey == "" {
		log.Println("[SECURITY WARNING] PANEL_SETUP_KEY is not set. The initial owner setup endpoint will not require a key.")
	}

	// Parse trusted proxies
	var trustedProxies []string
	if tp := os.Getenv("PANEL_TRUSTED_PROXIES"); tp != "" {
		for _, p := range strings.Split(tp, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				trustedProxies = append(trustedProxies, p)
			}
		}
	}

	// Parse allowed origins
	var allowedOrigins []string
	if ao := os.Getenv("PANEL_ALLOWED_ORIGINS"); ao != "" {
		for _, o := range strings.Split(ao, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}

	// SecureCookies: configurable via PANEL_SECURE_COOKIES env var.
	// Defaults to false because the panel is designed to run behind a reverse proxy
	// (Nginx handles HTTPS, proxies to Go over HTTP). Setting Secure: true on cookies
	// when Go receives plain HTTP can cause browsers to reject the cookie on refresh.
	// Operators who expose Go directly over HTTPS should set PANEL_SECURE_COOKIES=true.
	secureCookies := false
	if v := os.Getenv("PANEL_SECURE_COOKIES"); v != "" {
		secureCookies = strings.ToLower(v) == "true"
	} else if !devMode {
		// Legacy fallback: if env var is not set and not in dev mode,
		// still default to false for reverse proxy compatibility
		secureCookies = false
	}

	return Config{
		Addr:           getenv("PANEL_ADDR", ":8080"),
		DBDSN:          dbDSN,
		SetupKey:       setupKey,
		SessionSecret:  sessionSecret,
		Version:        getenv("PANEL_VERSION", "next-dev"),
		PublicBase:     getenv("PANEL_PUBLIC_BASE", "/dashboard"),
		AdminWebDir:    getenv("PANEL_ADMIN_WEB_DIR", "/opt/KorisPanel/panel/web/admin/www"),
		PortalWebDir:   getenv("PANEL_PORTAL_WEB_DIR", "/opt/KorisPanel/panel/web/portal/www"),
		TemplateDir:    getenv("PANEL_TEMPLATE_DIR", "/etc/koris/templates/"),
		SecureCookies:  secureCookies,
		TrustedProxies: trustedProxies,
		AllowedOrigins: allowedOrigins,
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
