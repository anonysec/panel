package certrotation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ExpiringCert represents a certificate that is approaching its expiration date.
type ExpiringCert struct {
	ID              int64
	Name            string
	CertPath        string
	ExpiresAt       time.Time
	Fingerprint     string
	DaysUntilExpiry int
}

// CertPusher is the interface for pushing certificates to nodes via gRPC.
// Implemented by grpcclient.CertManager.
type CertPusher interface {
	SetCertificates(ctx context.Context, nodeID int64, coreType string, caCert, cert, key []byte) error
}

// Worker periodically checks for expiring certificates and handles rotation.
type Worker struct {
	db       *sql.DB
	interval time.Duration
	done     chan struct{}
	eventFn  func(eventType, severity, title, message string)
	pusher   CertPusher // gRPC cert pusher for distributing certs to nodes
}

// New creates a new certificate rotation Worker with a 1-hour check interval.
func New(db *sql.DB, eventFn func(string, string, string, string)) *Worker {
	return &Worker{
		db:       db,
		interval: 1 * time.Hour,
		done:     make(chan struct{}),
		eventFn:  eventFn,
	}
}

// SetPusher sets the gRPC certificate pusher. When set, DistributeToNodes will use
// gRPC SetCertificates calls to push certificates to nodes.
// This should be called during startup after the gRPC pool is initialized.
func (w *Worker) SetPusher(pusher CertPusher) {
	w.pusher = pusher
}

// Start launches the background goroutine that periodically checks for expiring certs.
func (w *Worker) Start() {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-w.done:
				return
			case <-ticker.C:
				w.run()
			}
		}
	}()
	log.Println("[certrotation] worker started")
}

// Stop signals the worker to shut down.
func (w *Worker) Stop() {
	close(w.done)
}

// run performs a single check cycle: finds expiring certs, emits events, and handles rotation.
func (w *Worker) run() {
	certs, err := w.CheckExpiring()
	if err != nil {
		log.Printf("[certrotation] check expiring: %v", err)
		return
	}

	for _, cert := range certs {
		if cert.DaysUntilExpiry <= 7 {
			// Critical: cert expires within 7 days
			w.eventFn("cert.expiring", "error",
				fmt.Sprintf("Certificate %q expires in %d days", cert.Name, cert.DaysUntilExpiry),
				fmt.Sprintf("Certificate at %s expires on %s.", cert.CertPath, cert.ExpiresAt.Format("2006-01-02")))

			// Attempt regeneration
			newFingerprint, err := w.Regenerate(cert)
			if err != nil {
				if err == ErrCARequiresManualRotation {
					log.Printf("[certrotation] %s: %v", cert.Name, err)
				} else {
					log.Printf("[certrotation] regenerate %s: %v", cert.Name, err)
				}
				continue
			}
			log.Printf("[certrotation] regenerated %s, new fingerprint: %s", cert.Name, newFingerprint)

			// Distribute to nodes
			if err := w.DistributeToNodes(cert); err != nil {
				log.Printf("[certrotation] distribute %s: %v", cert.Name, err)
			}
		} else {
			// Warning: cert expires within 30 days
			w.eventFn("cert.expiring", "warning",
				fmt.Sprintf("Certificate %q expires in %d days", cert.Name, cert.DaysUntilExpiry),
				fmt.Sprintf("Certificate at %s expires on %s. Will be auto-renewed when within 7 days of expiry.", cert.CertPath, cert.ExpiresAt.Format("2006-01-02")))
		}
	}
}

// CheckExpiring queries the database for certificates expiring within 30 days.
func (w *Worker) CheckExpiring() ([]ExpiringCert, error) {
	rows, err := w.db.Query(`
		SELECT id, name, cert_path, expires_at, COALESCE(fingerprint, '')
		FROM vpn_certificates
		WHERE expires_at IS NOT NULL
		  AND expires_at < NOW() + INTERVAL 30 DAY
		  AND (status IS NULL OR status != 'revoked')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []ExpiringCert
	now := time.Now()
	for rows.Next() {
		var c ExpiringCert
		if err := rows.Scan(&c.ID, &c.Name, &c.CertPath, &c.ExpiresAt, &c.Fingerprint); err != nil {
			return nil, err
		}
		c.DaysUntilExpiry = int(c.ExpiresAt.Sub(now).Hours() / 24)
		if c.DaysUntilExpiry < 0 {
			c.DaysUntilExpiry = 0
		}
		certs = append(certs, c)
	}
	return certs, rows.Err()
}

// ErrCARequiresManualRotation is returned when Regenerate is called for a CA certificate.
// CA certificates cannot be auto-regenerated because doing so would invalidate all
// client certificates signed by the old CA key.
var ErrCARequiresManualRotation = fmt.Errorf("CA certificates require manual rotation; auto-regeneration is not supported")

// Regenerate regenerates a certificate using openssl based on its type.
// It updates the database with the new expiry and fingerprint.
// For CA certificates, it returns ErrCARequiresManualRotation instead of regenerating,
// because a new CA key would invalidate all existing client certificates.
func (w *Worker) Regenerate(cert ExpiringCert) (string, error) {
	cType := certType(cert.CertPath)

	var cmd *exec.Cmd
	var newDays int

	switch cType {
	case "ca":
		// CA certificates must not be auto-regenerated. A new CA key pair would
		// invalidate all client certs signed by the old CA. Require manual rotation.
		return "", ErrCARequiresManualRotation
	case "server":
		// Regenerate server certificate (self-signed for simplicity)
		keyPath := strings.TrimSuffix(cert.CertPath, ".crt") + ".key"
		cmd = exec.Command("openssl", "req", "-x509", "-nodes",
			"-days", "825",
			"-newkey", "ec",
			"-pkeyopt", "ec_paramgen_curve:prime256v1",
			"-keyout", keyPath,
			"-out", cert.CertPath,
			"-subj", "/CN=VPN-Server")
		newDays = 825
	case "tls-crypt":
		// Regenerate tls-crypt key (openvpn --genkey)
		cmd = exec.Command("openvpn", "--genkey", "tls-crypt-v2-server", cert.CertPath)
		newDays = 3650
	default:
		return "", fmt.Errorf("unknown cert type for path: %s", cert.CertPath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("openssl/openvpn command failed: %v, output: %s", err, string(output))
	}

	// Read new cert data and calculate fingerprint
	certData, err := os.ReadFile(cert.CertPath)
	if err != nil {
		return "", fmt.Errorf("read regenerated cert: %v", err)
	}

	newFingerprint := calcFingerprint(certData)
	newExpiry := time.Now().Add(time.Duration(newDays) * 24 * time.Hour)

	// Update database
	_, err = w.db.Exec(
		`UPDATE vpn_certificates SET expires_at = ?, fingerprint = ? WHERE id = ?`,
		newExpiry, newFingerprint, cert.ID,
	)
	if err != nil {
		return "", fmt.Errorf("update db: %v", err)
	}

	return newFingerprint, nil
}

// DistributeToNodes pushes regenerated certificates to all online/stale nodes.
// Uses gRPC SetCertificates calls via the configured CertPusher.
func (w *Worker) DistributeToNodes(cert ExpiringCert) error {
	// Read cert content for distribution
	certData, err := os.ReadFile(cert.CertPath)
	if err != nil {
		return fmt.Errorf("read cert for distribution: %v", err)
	}

	// Use gRPC SetCertificates for distribution
	if w.pusher != nil {
		return w.distributeViaGRPC(cert, certData)
	}

	// No pusher configured — log warning
	log.Printf("[certrotation] no gRPC pusher configured, cannot distribute cert %q to nodes", cert.Name)
	return nil
}

// distributeViaGRPC pushes certificates to nodes using the gRPC SetCertificates RPC.
// Satisfies Requirement 12.4: When the cert rotation worker detects an expiring
// certificate, the panel SHALL call SetCertificates to push the renewed cert to the node.
func (w *Worker) distributeViaGRPC(cert ExpiringCert, certData []byte) error {
	coreType := certType(cert.CertPath)
	ctx := context.Background()

	// Also read key file if it exists alongside the cert
	keyPath := strings.TrimSuffix(cert.CertPath, filepath.Ext(cert.CertPath)) + ".key"
	keyData, _ := os.ReadFile(keyPath)

	// Read CA cert if available (look in same directory)
	caPath := filepath.Join(filepath.Dir(cert.CertPath), "ca.crt")
	caData, _ := os.ReadFile(caPath)

	// Query online and stale nodes
	rows, err := w.db.Query(`SELECT id FROM nodes WHERE status IN ('online', 'stale')`)
	if err != nil {
		return fmt.Errorf("query nodes: %v", err)
	}
	defer rows.Close()

	var lastErr error
	for rows.Next() {
		var nodeID int64
		if err := rows.Scan(&nodeID); err != nil {
			continue
		}

		if err := w.pusher.SetCertificates(ctx, nodeID, coreType, caData, certData, keyData); err != nil {
			log.Printf("[certrotation] SetCertificates via gRPC failed for node %d, cert %q: %v", nodeID, cert.Name, err)
			lastErr = err
			continue
		}
		log.Printf("[certrotation] pushed cert %q to node %d via gRPC", cert.Name, nodeID)
	}

	if err := rows.Err(); err != nil {
		return err
	}
	return lastErr
}

// certType determines the certificate type from its file path.
// For "ca" classification, the base name must equal "ca.crt", "ca.key", or start with "ca." or "ca-".
// This avoids false positives like "cascade" which merely contains "ca" as a substring.
func certType(path string) string {
	base := strings.ToLower(filepath.Base(path))

	// Strict "ca" matching: base must be exactly "ca.*" or start with "ca." / "ca-"
	if base == "ca.crt" || base == "ca.key" || strings.HasPrefix(base, "ca.") || strings.HasPrefix(base, "ca-") {
		return "ca"
	}
	if strings.Contains(base, "tls") || base == "ta.key" {
		return "tls-crypt"
	}
	if strings.Contains(base, "server") {
		return "server"
	}
	// Check for known server cert extensions in server directories
	dir := strings.ToLower(filepath.Dir(path))
	ext := strings.ToLower(filepath.Ext(path))
	if (ext == ".crt" || ext == ".key") && strings.Contains(dir, "server") {
		return "server"
	}
	return "unknown"
}

// calcFingerprint computes a SHA256 fingerprint of the given certificate data.
func calcFingerprint(certData []byte) string {
	hash := sha256.Sum256(certData)
	return fmt.Sprintf("%x", hash[:])
}
