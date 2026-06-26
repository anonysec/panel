// Package tlsmanager provides unified TLS configuration for the panel,
// supporting four modes: ACME (Let's Encrypt), Manual (user-provided certs),
// SelfSigned (auto-generated at runtime), and PlainHTTP (loopback-only).
package tlsmanager

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// Mode represents the TLS operation mode.
type Mode int

const (
	// ModeACME uses Let's Encrypt automatic certificate management.
	ModeACME Mode = iota
	// ModeManual uses user-provided certificate and key files.
	ModeManual
	// ModeSelfSigned generates a self-signed certificate at runtime.
	ModeSelfSigned
	// ModePlainHTTP serves without TLS (only allowed on loopback addresses).
	ModePlainHTTP
)

// String returns the human-readable name of the mode.
func (m Mode) String() string {
	switch m {
	case ModeACME:
		return "acme"
	case ModeManual:
		return "manual"
	case ModeSelfSigned:
		return "selfsigned"
	case ModePlainHTTP:
		return "plainhttp"
	default:
		return "unknown"
	}
}

// ParseMode converts a string to a Mode. Accepts: "acme", "manual",
// "selfsigned", "disabled", "plainhttp". Returns ModePlainHTTP for "disabled".
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "acme", "letsencrypt":
		return ModeACME, nil
	case "manual":
		return ModeManual, nil
	case "selfsigned", "self-signed":
		return ModeSelfSigned, nil
	case "disabled", "plainhttp", "plain", "http":
		return ModePlainHTTP, nil
	default:
		return ModePlainHTTP, fmt.Errorf("tlsmanager: unknown mode %q", s)
	}
}

// Manager handles TLS configuration for the panel HTTP server.
type Manager struct {
	mode    Mode
	domain  string
	email   string
	cert    string // path to cert file (ModeManual)
	key     string // path to key file (ModeManual)
	certDir string // directory for ACME cert caching

	// cached self-signed certificate (generated once)
	selfSignedCert *tls.Certificate
}

// Config holds the parameters for creating a new Manager.
type Config struct {
	Mode    Mode
	Domain  string
	Email   string
	Cert    string // path to cert file
	Key     string // path to key file
	CertDir string
}

// New creates a new TLS Manager with the given configuration.
func New(cfg Config) (*Manager, error) {
	m := &Manager{
		mode:    cfg.Mode,
		domain:  cfg.Domain,
		email:   cfg.Email,
		cert:    cfg.Cert,
		key:     cfg.Key,
		certDir: cfg.CertDir,
	}

	// Validate based on mode
	switch cfg.Mode {
	case ModeACME:
		if cfg.Domain == "" {
			return nil, errors.New("tlsmanager: a domain is required for ACME mode; set PANEL_TLS_DOMAIN to use Let's Encrypt")
		}
		if cfg.CertDir == "" {
			m.certDir = "/etc/koris/certs"
		}
	case ModeManual:
		if cfg.Cert == "" || cfg.Key == "" {
			return nil, errors.New("tlsmanager: cert and key paths are required for manual mode")
		}
		// Verify files exist
		if _, err := os.Stat(cfg.Cert); err != nil {
			return nil, fmt.Errorf("tlsmanager: cert file not found: %s: %w", cfg.Cert, err)
		}
		if _, err := os.Stat(cfg.Key); err != nil {
			return nil, fmt.Errorf("tlsmanager: key file not found: %s: %w", cfg.Key, err)
		}
		// Verify cert and key are valid X.509 at startup
		if _, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key); err != nil {
			return nil, fmt.Errorf("tlsmanager: invalid X.509 certificate/key pair at %s / %s: %w", cfg.Cert, cfg.Key, err)
		}
	case ModeSelfSigned:
		// No external config needed; cert generated on first TLSConfig() call
		log.Printf("[tlsmanager] WARNING: using self-signed certificate mode. This is not recommended for production use.")
	case ModePlainHTTP:
		// Nothing to validate
	}

	log.Printf("[tlsmanager] initialized: mode=%s, domain=%s", m.mode, m.domain)
	return m, nil
}

// Mode returns the configured TLS mode.
func (m *Manager) Mode() Mode {
	return m.mode
}

// TLSConfig returns a *tls.Config appropriate for the configured mode.
// Returns nil for ModePlainHTTP (no TLS needed).
func (m *Manager) TLSConfig() (*tls.Config, error) {
	switch m.mode {
	case ModeACME:
		return m.acmeTLSConfig()
	case ModeManual:
		return m.manualTLSConfig()
	case ModeSelfSigned:
		return m.selfSignedTLSConfig()
	case ModePlainHTTP:
		return nil, nil
	default:
		return nil, fmt.Errorf("tlsmanager: unsupported mode %d", m.mode)
	}
}

// HTTPRedirectHandler returns an http.Handler that redirects all HTTP requests
// to HTTPS with a 301 Moved Permanently status. Health check requests at
// /api/health are served directly without redirect.
func (m *Manager) HTTPRedirectHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow health checks over plain HTTP
		if r.URL.Path == "/api/health" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"service":"panel","tls":true}`))
			return
		}

		// Build HTTPS target URL
		host := r.Host
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			host = host[:idx]
		}
		target := "https://" + host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

// ShouldAllowHTTP reports whether plain HTTP (no TLS) is permitted for the
// given listen address. Plain HTTP is allowed only when the address resolves
// to a loopback interface (127.0.0.1, ::1, localhost).
func ShouldAllowHTTP(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// addr might not have a port; treat the whole string as host
		host = addr
	}

	// Empty host means "all interfaces" (0.0.0.0) — not loopback
	if host == "" || host == "0.0.0.0" || host == "::" {
		return false
	}

	// Check well-known loopback names
	if host == "localhost" {
		return true
	}

	// Parse as IP and check loopback
	ip := net.ParseIP(host)
	if ip == nil {
		// Not a valid IP; try resolving the hostname
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			return false
		}
		// All resolved addresses must be loopback
		for _, a := range addrs {
			resolved := net.ParseIP(a)
			if resolved == nil || !resolved.IsLoopback() {
				return false
			}
		}
		return true
	}

	return ip.IsLoopback()
}

// --- Private mode implementations ---

func (m *Manager) acmeTLSConfig() (*tls.Config, error) {
	if err := os.MkdirAll(m.certDir, 0700); err != nil {
		return nil, fmt.Errorf("tlsmanager: create cert dir: %w", err)
	}

	mgr := &autocert.Manager{
		Cache:      autocert.DirCache(m.certDir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(m.domain),
	}
	if m.email != "" {
		mgr.Email = m.email
	}

	tlsCfg := mgr.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS12
	return tlsCfg, nil
}

func (m *Manager) manualTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(m.cert, m.key)
	if err != nil {
		return nil, fmt.Errorf("tlsmanager: load certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func (m *Manager) selfSignedTLSConfig() (*tls.Config, error) {
	if m.selfSignedCert != nil {
		return &tls.Config{
			Certificates: []tls.Certificate{*m.selfSignedCert},
			MinVersion:   tls.VersionTLS12,
		}, nil
	}

	cert, err := generateSelfSigned(m.domain)
	if err != nil {
		return nil, fmt.Errorf("tlsmanager: generate self-signed: %w", err)
	}
	m.selfSignedCert = cert

	log.Printf("[tlsmanager] generated self-signed certificate (valid 1 year)")
	return &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// generateSelfSigned creates a self-signed TLS certificate using ECDSA P-256.
// The certificate is valid for 1 year and covers the given domain (if non-empty),
// plus localhost and 127.0.0.1 for local access.
func generateSelfSigned(domain string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"KorisPanel"},
			CommonName:   "KorisPanel Self-Signed",
		},
		NotBefore:             now,
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	if domain != "" {
		template.DNSNames = append(template.DNSNames, domain)
		// If domain looks like an IP, add it to IPAddresses too
		if ip := net.ParseIP(domain); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
	// Parse the leaf for convenience
	tlsCert.Leaf, _ = x509.ParseCertificate(certDER)

	return tlsCert, nil
}
