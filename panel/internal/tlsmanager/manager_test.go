package tlsmanager

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		input   string
		want    Mode
		wantErr bool
	}{
		{"acme", ModeACME, false},
		{"ACME", ModeACME, false},
		{"letsencrypt", ModeACME, false},
		{"manual", ModeManual, false},
		{"Manual", ModeManual, false},
		{"selfsigned", ModeSelfSigned, false},
		{"self-signed", ModeSelfSigned, false},
		{"disabled", ModePlainHTTP, false},
		{"plainhttp", ModePlainHTTP, false},
		{"plain", ModePlainHTTP, false},
		{"http", ModePlainHTTP, false},
		{"invalid", ModePlainHTTP, true},
		{"", ModePlainHTTP, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseMode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeACME, "acme"},
		{ModeManual, "manual"},
		{ModeSelfSigned, "selfsigned"},
		{ModePlainHTTP, "plainhttp"},
		{Mode(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestNew_ACMERequiresDomain(t *testing.T) {
	_, err := New(Config{Mode: ModeACME, Domain: ""})
	if err == nil {
		t.Fatal("expected error for ACME mode without domain")
	}
}

func TestNew_ACMEValidConfig(t *testing.T) {
	m, err := New(Config{
		Mode:    ModeACME,
		Domain:  "panel.example.com",
		Email:   "admin@example.com",
		CertDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.mode != ModeACME {
		t.Errorf("expected mode ACME, got %v", m.mode)
	}
}

func TestNew_ManualRequiresCertAndKey(t *testing.T) {
	_, err := New(Config{Mode: ModeManual, Cert: "", Key: ""})
	if err == nil {
		t.Fatal("expected error for manual mode without cert/key")
	}
}

func TestNew_ManualCertFileNotFound(t *testing.T) {
	_, err := New(Config{
		Mode: ModeManual,
		Cert: "/nonexistent/cert.pem",
		Key:  "/nonexistent/key.pem",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent cert file")
	}
	if !strings.Contains(err.Error(), "cert file not found") {
		t.Errorf("expected cert file not found error, got: %v", err)
	}
}

func TestNew_ManualKeyFileNotFound(t *testing.T) {
	// Create a valid cert file but no key file
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	if err := os.WriteFile(certPath, []byte("placeholder"), 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	_, err := New(Config{
		Mode: ModeManual,
		Cert: certPath,
		Key:  "/nonexistent/key.pem",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent key file")
	}
	if !strings.Contains(err.Error(), "key file not found") {
		t.Errorf("expected key file not found error, got: %v", err)
	}
}

func TestNew_ManualInvalidX509(t *testing.T) {
	// Create cert and key files with invalid content
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	if err := os.WriteFile(certPath, []byte("not a valid cert"), 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("not a valid key"), 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_, err := New(Config{
		Mode: ModeManual,
		Cert: certPath,
		Key:  keyPath,
	})
	if err == nil {
		t.Fatal("expected error for invalid X.509 cert/key pair")
	}
	if !strings.Contains(err.Error(), "invalid X.509") {
		t.Errorf("expected invalid X.509 error, got: %v", err)
	}
}

func TestNew_ManualValidConfig(t *testing.T) {
	// Create temporary cert and key files
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	// Generate a real self-signed cert to use as the "manual" cert
	cert, err := generateSelfSigned("test.example.com")
	if err != nil {
		t.Fatalf("failed to generate test cert: %v", err)
	}

	// Write cert and key to files in PEM format
	writeCertAndKey(t, certPath, keyPath, cert)

	m, err := New(Config{
		Mode: ModeManual,
		Cert: certPath,
		Key:  keyPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.mode != ModeManual {
		t.Errorf("expected mode Manual, got %v", m.mode)
	}
}

func TestNew_SelfSigned(t *testing.T) {
	m, err := New(Config{Mode: ModeSelfSigned})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.mode != ModeSelfSigned {
		t.Errorf("expected mode SelfSigned, got %v", m.mode)
	}
}

func TestNew_PlainHTTP(t *testing.T) {
	m, err := New(Config{Mode: ModePlainHTTP})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.mode != ModePlainHTTP {
		t.Errorf("expected mode PlainHTTP, got %v", m.mode)
	}
}

func TestTLSConfig_PlainHTTP_ReturnsNil(t *testing.T) {
	m, _ := New(Config{Mode: ModePlainHTTP})
	cfg, err := m.TLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil TLS config for PlainHTTP mode")
	}
}

func TestTLSConfig_SelfSigned(t *testing.T) {
	m, _ := New(Config{Mode: ModeSelfSigned, Domain: "test.local"})
	cfg, err := m.TLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil TLS config for SelfSigned mode")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2, got %d", cfg.MinVersion)
	}
	if len(cfg.Certificates) == 0 {
		t.Error("expected at least one certificate in TLS config")
	}
}

func TestTLSConfig_SelfSigned_CachesCert(t *testing.T) {
	m, _ := New(Config{Mode: ModeSelfSigned, Domain: "test.local"})

	cfg1, err := m.TLSConfig()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	cfg2, err := m.TLSConfig()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	// Both should use the same certificate (pointer equality on the leaf)
	if cfg1.Certificates[0].Leaf != cfg2.Certificates[0].Leaf {
		t.Error("expected cached self-signed certificate to be reused")
	}
}

func TestTLSConfig_ACME(t *testing.T) {
	m, _ := New(Config{
		Mode:    ModeACME,
		Domain:  "panel.example.com",
		CertDir: t.TempDir(),
	})
	cfg, err := m.TLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil TLS config for ACME mode")
	}
	if cfg.GetCertificate == nil {
		t.Error("ACME TLS config should have GetCertificate set")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2, got %d", cfg.MinVersion)
	}
}

func TestTLSConfig_Manual(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	cert, err := generateSelfSigned("manual.example.com")
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	writeCertAndKey(t, certPath, keyPath, cert)

	m, err := New(Config{
		Mode: ModeManual,
		Cert: certPath,
		Key:  keyPath,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cfg, err := m.TLSConfig()
	if err != nil {
		t.Fatalf("TLSConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil TLS config for Manual mode")
	}
	if len(cfg.Certificates) == 0 {
		t.Error("expected at least one certificate")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2, got %d", cfg.MinVersion)
	}
}

func TestHTTPRedirectHandler_Redirects(t *testing.T) {
	m, _ := New(Config{Mode: ModeSelfSigned})
	handler := m.HTTPRedirectHandler()

	req := httptest.NewRequest(http.MethodGet, "http://panel.example.com/dashboard/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://panel.example.com/dashboard/" {
		t.Errorf("expected redirect to HTTPS, got %q", loc)
	}
}

func TestHTTPRedirectHandler_HealthCheck(t *testing.T) {
	m, _ := New(Config{Mode: ModeSelfSigned})
	handler := m.HTTPRedirectHandler()

	req := httptest.NewRequest(http.MethodGet, "http://panel.example.com/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if body != `{"ok":true,"service":"panel","tls":true}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestHTTPRedirectHandler_StripPort(t *testing.T) {
	m, _ := New(Config{Mode: ModeSelfSigned})
	handler := m.HTTPRedirectHandler()

	req := httptest.NewRequest(http.MethodGet, "http://panel.example.com:8080/path", nil)
	req.Host = "panel.example.com:8080"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://panel.example.com/path" {
		t.Errorf("expected redirect without port, got %q", loc)
	}
}

func TestShouldAllowHTTP(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// Loopback addresses — should allow HTTP
		{"127.0.0.1:8080", true},
		{"127.0.0.1:443", true},
		{"[::1]:8080", true},
		{"::1", true},
		{"localhost:8080", true},
		{"localhost", true},

		// Non-loopback — should NOT allow HTTP
		{"0.0.0.0:8080", false},
		{":8080", false},
		{"", false},
		{"192.168.1.1:8080", false},
		{"10.0.0.1:443", false},
		{"panel.example.com:443", false},
		{"::", false},
		{"[::]:8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			got := ShouldAllowHTTP(tt.addr)
			if got != tt.want {
				t.Errorf("ShouldAllowHTTP(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestGenerateSelfSigned(t *testing.T) {
	cert, err := generateSelfSigned("test.example.com")
	if err != nil {
		t.Fatalf("generateSelfSigned: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil certificate")
	}
	if cert.Leaf == nil {
		t.Fatal("expected parsed leaf certificate")
	}

	// Check validity period
	if cert.Leaf.NotBefore.IsZero() {
		t.Error("expected non-zero NotBefore")
	}

	// Check SAN includes our domain and localhost
	foundDomain := false
	foundLocalhost := false
	for _, name := range cert.Leaf.DNSNames {
		if name == "test.example.com" {
			foundDomain = true
		}
		if name == "localhost" {
			foundLocalhost = true
		}
	}
	if !foundDomain {
		t.Error("expected domain in DNS SANs")
	}
	if !foundLocalhost {
		t.Error("expected localhost in DNS SANs")
	}
}

func TestGenerateSelfSigned_EmptyDomain(t *testing.T) {
	cert, err := generateSelfSigned("")
	if err != nil {
		t.Fatalf("generateSelfSigned: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil certificate")
	}
	// Should still have localhost
	foundLocalhost := false
	for _, name := range cert.Leaf.DNSNames {
		if name == "localhost" {
			foundLocalhost = true
		}
	}
	if !foundLocalhost {
		t.Error("expected localhost in DNS SANs even without domain")
	}
}

// writeCertAndKey writes a tls.Certificate to PEM files for testing.
func writeCertAndKey(t *testing.T, certPath, keyPath string, cert *tls.Certificate) {
	t.Helper()

	// Write cert PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate[0],
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	// Write key PEM
	ecKey, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("expected ECDSA private key")
	}
	keyDER, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}
