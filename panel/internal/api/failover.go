package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
)

// DNSProvider stores credentials for DNS API integrations (Cloudflare or manual).
type DNSProvider struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Type              string `json:"type"`               // "cloudflare" | "manual"
	APITokenEncrypted string `json:"-"`                  // never exposed via API
	ZoneID            string `json:"zone_id,omitempty"`  // Cloudflare zone ID
	AccountID         string `json:"account_id,omitempty"`
	IsActive          bool   `json:"is_active"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// FailoverDomain maps a domain name to its current target node.
type FailoverDomain struct {
	ID             int64   `json:"id"`
	Domain         string  `json:"domain"`
	CurrentNodeID  int64   `json:"current_node_id"`
	DNSProviderID  *int64  `json:"dns_provider_id"`
	DNSRecordID    string  `json:"dns_record_id"`
	TTL            int     `json:"ttl"`
	IsActive       bool    `json:"is_active"`
	LastFailoverAt *string `json:"last_failover_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	// Joined fields
	CurrentNodeName string `json:"current_node_name,omitempty"`
	CurrentNodeIP   string `json:"current_node_ip,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
}

// FailoverEvent is an audit record tracking a single failover action.
type FailoverEvent struct {
	ID                            int64   `json:"id"`
	DomainID                      int64   `json:"domain_id"`
	FromNodeID                    int64   `json:"from_node_id"`
	ToNodeID                      int64   `json:"to_node_id"`
	Reason                        string  `json:"reason"`
	Status                        string  `json:"status"`
	DNSPropagationStartedAt       *string `json:"dns_propagation_started_at"`
	DNSPropagationCompletedAt     *string `json:"dns_propagation_completed_at"`
	TriggeredBy                   string  `json:"triggered_by"`
	ErrorMessage                  *string `json:"error_message"`
	CreatedAt                     string  `json:"created_at"`
	// Joined fields
	DomainName   string `json:"domain_name,omitempty"`
	FromNodeName string `json:"from_node_name,omitempty"`
	ToNodeName   string `json:"to_node_name,omitempty"`
}

// encryptionKey derives a 32-byte AES-256 key from the PANEL_SECRET env var.
func encryptionKey() ([]byte, error) {
	secret := os.Getenv("PANEL_SECRET")
	if secret == "" {
		return nil, errors.New("PANEL_SECRET environment variable is not set")
	}
	hash := sha256.Sum256([]byte(secret))
	return hash[:], nil
}

// encryptToken encrypts plaintext using AES-256-GCM and returns base64-encoded ciphertext.
func encryptToken(plaintext string) (string, error) {
	key, err := encryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptToken decrypts base64-encoded AES-256-GCM ciphertext and returns the plaintext.
func decryptToken(ciphertext string) (string, error) {
	key, err := encryptionKey()
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, encrypted := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// fqdnPattern validates domain labels per RFC 1035:
// - Each label: 1-63 chars, starts with letter, ends with letter/digit, hyphens allowed in middle
// - Total length: 1-253 characters
// - At least two labels (e.g., "example.com")
var fqdnPattern = regexp.MustCompile(`^(?i)([a-z]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

// isValidFQDN validates that a domain string is a valid FQDN per RFC 1035.
func isValidFQDN(domain string) bool {
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" || len(domain) > 253 {
		return false
	}
	return fqdnPattern.MatchString(domain)
}

// normalizeFailoverTTL clamps a TTL value to the valid range [30, 86400].
// A zero or negative value defaults to 60.
func normalizeFailoverTTL(ttl int) int {
	if ttl <= 0 {
		return 60
	}
	if ttl < 30 {
		return 30
	}
	if ttl > 86400 {
		return 86400
	}
	return ttl
}
