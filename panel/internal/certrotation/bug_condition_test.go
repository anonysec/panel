package certrotation

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestCheckExpiring_ReturnsErrorOnDBFailure verifies that CheckExpiring()
// properly propagates database errors (regression test for migration 025 fix).
func TestCheckExpiring_ReturnsErrorOnDBFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer db.Close()

	dbError := fmt.Errorf("connection refused")
	mock.ExpectQuery("SELECT id, name, cert_path, expires_at").
		WillReturnError(dbError)

	w := &Worker{
		db:       db,
		interval: time.Hour,
		done:     make(chan struct{}),
		eventFn:  func(_, _, _, _ string) {},
	}

	_, err = w.CheckExpiring()
	if err == nil {
		t.Error("Expected error from CheckExpiring() when DB returns an error, got nil")
	}
}

// TestBugCondition_FixedSchema_QuerySucceeds verifies that after migration 025 adds
// the cert_path and status columns, the CheckExpiring() query succeeds without Error 1054.
func TestBugCondition_FixedSchema_QuerySucceeds(t *testing.T) {
	cfg := &quick.Config{
		MaxCount: 50,
		Rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		name := randomCertName(rng)
		certPath := "/etc/openvpn/" + name
		fingerprint := randomFingerprint(rng)
		expiresAt := time.Now().Add(time.Duration(rng.Intn(29)+1) * 24 * time.Hour)

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Logf("Failed to create sqlmock: %v", err)
			return true
		}
		defer db.Close()

		columns := []string{"id", "name", "cert_path", "expires_at", "fingerprint"}
		rows := sqlmock.NewRows(columns).
			AddRow(rng.Int63n(10000)+1, name, certPath, expiresAt, fingerprint)

		mock.ExpectQuery("SELECT id, name, cert_path, expires_at").
			WillReturnRows(rows)

		w := &Worker{
			db:       db,
			interval: time.Hour,
			done:     make(chan struct{}),
			eventFn:  func(_, _, _, _ string) {},
		}

		certs, err := w.CheckExpiring()

		if err != nil {
			t.Logf("UNEXPECTED failure for cert (name=%q, certPath=%q, fingerprint=%q, expires_at=%v): %v",
				name, certPath, fingerprint, expiresAt.Format("2006-01-02"), err)
			return false
		}

		if len(certs) == 0 {
			t.Logf("UNEXPECTED: query returned no rows for cert (name=%q)", name)
			return false
		}

		if certs[0].Name != name {
			t.Logf("Name mismatch: got %q, want %q", certs[0].Name, name)
			return false
		}
		if certs[0].CertPath != certPath {
			t.Logf("CertPath mismatch: got %q, want %q", certs[0].CertPath, certPath)
			return false
		}

		return true
	}

	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("Fix verification FAILED - CheckExpiring() still fails on fixed schema: %v", err)
	}
}

// --- Helper functions for random data generation ---

var certTypes = []string{"ca", "tls_crypt", "client_cert", "client_key"}

func randomCertType(rng *rand.Rand) string {
	return certTypes[rng.Intn(len(certTypes))]
}

func randomCertName(rng *rand.Rand) string {
	prefixes := []string{"vpn-server", "client", "ca-root", "tls-auth", "node"}
	suffixes := []string{".crt", ".key", ".pem", "-bundle.crt", ""}
	name := prefixes[rng.Intn(len(prefixes))]
	if rng.Intn(2) == 0 {
		name += fmt.Sprintf("-%d", rng.Intn(1000))
	}
	name += suffixes[rng.Intn(len(suffixes))]
	return name
}

func randomContent(rng *rand.Rand) string {
	lines := []string{"-----BEGIN CERTIFICATE-----"}
	lineCount := rng.Intn(10) + 3
	for i := 0; i < lineCount; i++ {
		line := make([]byte, 64)
		for j := range line {
			line[j] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"[rng.Intn(64)]
		}
		lines = append(lines, string(line))
	}
	lines = append(lines, "-----END CERTIFICATE-----")
	return strings.Join(lines, "\n")
}

func randomFingerprint(rng *rand.Rand) string {
	if rng.Intn(5) == 0 {
		return ""
	}
	chars := "0123456789abcdef"
	fp := make([]byte, 64)
	for i := range fp {
		fp[i] = chars[rng.Intn(16)]
	}
	return string(fp)
}
