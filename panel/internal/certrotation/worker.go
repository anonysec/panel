package certrotation

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
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

// Worker periodically checks for expiring certificates and handles rotation.
type Worker struct {
	db       *sql.DB
	interval time.Duration
	done     chan struct{}
	eventFn  func(eventType, severity, title, message string)
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
				fmt.Sprintf("Certificate at %s expires on %s. Automatic regeneration initiated.", cert.CertPath, cert.ExpiresAt.Format("2006-01-02")))

			// Attempt regeneration
			newFingerprint, err := w.Regenerate(cert)
			if err != nil {
				log.Printf("[certrotation] regenerate %s: %v", cert.Name, err)
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

// Regenerate regenerates a certificate using openssl based on its type.
// It updates the database with the new expiry and fingerprint.
func (w *Worker) Regenerate(cert ExpiringCert) (string, error) {
	cType := certType(cert.CertPath)

	var cmd *exec.Cmd
	var newDays int

	switch cType {
	case "ca":
		// Regenerate CA certificate
		keyPath := strings.TrimSuffix(cert.CertPath, ".crt") + ".key"
		cmd = exec.Command("openssl", "req", "-x509", "-nodes",
			"-days", "3650",
			"-newkey", "ec",
			"-pkeyopt", "ec_paramgen_curve:prime256v1",
			"-keyout", keyPath,
			"-out", cert.CertPath,
			"-subj", "/CN=VPN-CA")
		newDays = 3650
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

// DistributeToNodes creates cert.distribute tasks for all online/stale nodes.
func (w *Worker) DistributeToNodes(cert ExpiringCert) error {
	// Read cert content for distribution
	certData, err := os.ReadFile(cert.CertPath)
	if err != nil {
		return fmt.Errorf("read cert for distribution: %v", err)
	}
	certContent := base64.StdEncoding.EncodeToString(certData)

	// Query online and stale nodes
	rows, err := w.db.Query(`SELECT id FROM nodes WHERE status IN ('online', 'stale')`)
	if err != nil {
		return fmt.Errorf("query nodes: %v", err)
	}
	defer rows.Close()

	payload := map[string]string{
		"cert_path":    cert.CertPath,
		"cert_content": certContent,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %v", err)
	}

	for rows.Next() {
		var nodeID int64
		if err := rows.Scan(&nodeID); err != nil {
			continue
		}
		_, err := w.db.Exec(
			`INSERT INTO node_tasks (node_id, action, payload_json, status) VALUES (?, 'cert.distribute', ?, 'pending')`,
			nodeID, string(payloadJSON),
		)
		if err != nil {
			log.Printf("[certrotation] create task for node %d: %v", nodeID, err)
		}
	}
	return rows.Err()
}

// certType determines the certificate type from its file path.
func certType(path string) string {
	base := strings.ToLower(filepath.Base(path))

	if strings.Contains(base, "ca") {
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
