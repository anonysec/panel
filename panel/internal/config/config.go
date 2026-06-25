package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr           string
	TLSAddr        string
	TLSCert        string
	TLSKey         string
	TLSEnabled     bool
	TLSCertDir     string
	TLSDomain      string
	TLSEmail       string
	TLSMode        string // acme | manual | selfsigned | disabled
	DBDSN          string
	DBBackend      string // timescaledb | postgres | mariadb | sqlite
	PGDSN          string
	SetupKey       string
	SessionSecret  string
	Version        string
	ReleaseURL     string
	PublicBase     string
	AdminWebDir    string
	PortalWebDir   string
	LandingWebDir  string
	TemplateDir    string
	SecureCookies  bool
	TrustedProxies []string
	AllowedOrigins []string

	// Multi-worker
	Workers  int
	WorkerID string

	// gRPC client settings
	GRPCConnectTimeout    time.Duration
	GRPCKeepaliveInterval time.Duration
	GRPCMetricsInterval   time.Duration

	// Alert thresholds (percent)
	AlertCPUThreshold  int
	AlertRAMThreshold  int
	AlertDiskThreshold int

	// Logging
	LogFormat string // "json" or "text" (default: "text", "json" in Docker)
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
		// Check PANEL_PG_DSN for PostgreSQL/TimescaleDB backend
		dbDSN = getenv("PANEL_PG_DSN", "")
	}
	if dbDSN == "" {
		if !devMode {
			log.Fatalf("FATAL: PANEL_DB_DSN or PANEL_PG_DSN is required in production. Set PANEL_DEV_MODE=true for development.")
		}
		log.Println("[SECURITY WARNING] PANEL_DB_DSN/PANEL_PG_DSN is not set. Using insecure default credentials.")
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

	tlsEnabled := strings.ToLower(os.Getenv("PANEL_TLS_ENABLED")) == "true"

	// Database backend selection
	dbBackend := getenv("PANEL_DB_BACKEND", "timescaledb")
	pgDSN := getenv("PANEL_PG_DSN", "")

	// Multi-worker settings
	workers := getenvInt("PANEL_WORKERS", 1)
	if workers < 1 {
		workers = 1
	}
	workerID := getenv("PANEL_WORKER_ID", "")
	if workerID == "" {
		hostname, _ := os.Hostname()
		workerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

	// gRPC client settings
	grpcConnectTimeout := getenvDuration("PANEL_GRPC_CONNECT_TIMEOUT", 10*time.Second)
	grpcKeepaliveInterval := getenvDuration("PANEL_GRPC_KEEPALIVE_INTERVAL", 30*time.Second)
	grpcMetricsInterval := getenvDuration("PANEL_GRPC_METRICS_INTERVAL", 10*time.Second)

	// Alert thresholds
	alertCPU := getenvInt("PANEL_ALERT_CPU_THRESHOLD", 90)
	alertRAM := getenvInt("PANEL_ALERT_RAM_THRESHOLD", 85)
	alertDisk := getenvInt("PANEL_ALERT_DISK_THRESHOLD", 90)

	// TLS mode
	tlsMode := getenv("PANEL_TLS_MODE", "selfsigned")

	// Log format
	logFormat := getenv("PANEL_LOG_FORMAT", "text")

	return Config{
		Addr:           getenv("PANEL_ADDR", ":8080"),
		TLSAddr:        getenv("PANEL_TLS_ADDR", ":443"),
		TLSCert:        getenv("PANEL_TLS_CERT", "/etc/koris/cert.pem"),
		TLSKey:         getenv("PANEL_TLS_KEY", "/etc/koris/key.pem"),
		TLSEnabled:     tlsEnabled,
		TLSCertDir:     getenv("PANEL_TLS_CERT_DIR", "/etc/koris/certs"),
		TLSDomain:      getenv("PANEL_TLS_DOMAIN", ""),
		TLSEmail:       getenv("PANEL_TLS_EMAIL", ""),
		TLSMode:        tlsMode,
		DBDSN:          dbDSN,
		DBBackend:      dbBackend,
		PGDSN:          pgDSN,
		SetupKey:       setupKey,
		SessionSecret:  sessionSecret,
		Version:        getenv("PANEL_VERSION", readVersionFile()),
		ReleaseURL:     getenv("PANEL_RELEASE_URL", ""),
		PublicBase:     getenv("PANEL_PUBLIC_BASE", "/dashboard"),
		AdminWebDir:    getenv("PANEL_ADMIN_WEB_DIR", "/opt/KorisPanel/panel/web/admin/www"),
		PortalWebDir:   getenv("PANEL_PORTAL_WEB_DIR", "/opt/KorisPanel/panel/web/portal/www"),
		LandingWebDir:  getenv("PANEL_LANDING_WEB_DIR", "/opt/KorisPanel/panel/web/landing/www"),
		TemplateDir:    getenv("PANEL_TEMPLATE_DIR", "/etc/koris/templates/"),
		SecureCookies:  secureCookies,
		TrustedProxies: trustedProxies,
		AllowedOrigins: allowedOrigins,

		Workers:  workers,
		WorkerID: workerID,

		GRPCConnectTimeout:    grpcConnectTimeout,
		GRPCKeepaliveInterval: grpcKeepaliveInterval,
		GRPCMetricsInterval:   grpcMetricsInterval,

		AlertCPUThreshold:  alertCPU,
		AlertRAMThreshold:  alertRAM,
		AlertDiskThreshold: alertDisk,

		LogFormat: logFormat,
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func readVersionFile() string {
	// Try common locations
	for _, path := range []string{"VERSION", "/opt/KorisPanel/VERSION", "/app/VERSION"} {
		if data, err := os.ReadFile(path); err == nil {
			v := strings.TrimSpace(string(data))
			if v != "" {
				return v
			}
		}
	}
	return "0.92.0"
}

func getenvInt(k string, d int) int {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("[config] invalid integer for %s=%q, using default %d", k, v, d)
		return d
	}
	return n
}

func getenvDuration(k string, d time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	dur, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("[config] invalid duration for %s=%q, using default %s", k, v, d)
		return d
	}
	return dur
}
