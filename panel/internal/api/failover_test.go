package api

import (
	"os"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	os.Setenv("PANEL_SECRET", "test-secret-for-encryption-key-derivation")
	defer os.Unsetenv("PANEL_SECRET")

	tests := []struct {
		name  string
		token string
	}{
		{"simple token", "my-api-token-12345"},
		{"empty string", ""},
		{"long token", "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-_=+"},
		{"special chars", "token!@#$%^&*()_+-=[]{}|;':\",./<>?"},
		{"unicode", "tokenüñîcödé日本語"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := encryptToken(tt.token)
			if err != nil {
				t.Fatalf("encryptToken(%q) error: %v", tt.token, err)
			}

			// Ciphertext must differ from plaintext
			if encrypted == tt.token {
				t.Errorf("encrypted output equals plaintext for %q", tt.token)
			}

			decrypted, err := decryptToken(encrypted)
			if err != nil {
				t.Fatalf("decryptToken() error: %v", err)
			}

			if decrypted != tt.token {
				t.Errorf("round-trip failed: got %q, want %q", decrypted, tt.token)
			}
		})
	}
}

func TestEncryptTokenDifferentCiphertexts(t *testing.T) {
	os.Setenv("PANEL_SECRET", "test-secret-for-encryption")
	defer os.Unsetenv("PANEL_SECRET")

	token := "same-plaintext-token"
	enc1, err := encryptToken(token)
	if err != nil {
		t.Fatalf("first encrypt error: %v", err)
	}
	enc2, err := encryptToken(token)
	if err != nil {
		t.Fatalf("second encrypt error: %v", err)
	}

	// Due to random nonce, same plaintext should produce different ciphertexts
	if enc1 == enc2 {
		t.Error("two encryptions of the same plaintext produced identical ciphertext (nonce reuse)")
	}

	// Both should still decrypt to the same value
	dec1, _ := decryptToken(enc1)
	dec2, _ := decryptToken(enc2)
	if dec1 != token || dec2 != token {
		t.Error("decryption of different ciphertexts should yield same plaintext")
	}
}

func TestEncryptTokenMissingSecret(t *testing.T) {
	os.Unsetenv("PANEL_SECRET")

	_, err := encryptToken("test")
	if err == nil {
		t.Error("expected error when PANEL_SECRET is not set")
	}
}

func TestDecryptTokenMissingSecret(t *testing.T) {
	os.Unsetenv("PANEL_SECRET")

	_, err := decryptToken("dGVzdA==")
	if err == nil {
		t.Error("expected error when PANEL_SECRET is not set")
	}
}

func TestDecryptTokenInvalidBase64(t *testing.T) {
	os.Setenv("PANEL_SECRET", "test-secret")
	defer os.Unsetenv("PANEL_SECRET")

	_, err := decryptToken("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64 input")
	}
}

func TestDecryptTokenTooShort(t *testing.T) {
	os.Setenv("PANEL_SECRET", "test-secret")
	defer os.Unsetenv("PANEL_SECRET")

	// Base64 of a very short byte slice (less than nonce size)
	_, err := decryptToken("AQID")
	if err == nil {
		t.Error("expected error for ciphertext shorter than nonce")
	}
}

func TestDecryptTokenTamperedData(t *testing.T) {
	os.Setenv("PANEL_SECRET", "test-secret")
	defer os.Unsetenv("PANEL_SECRET")

	encrypted, err := encryptToken("my-secret-token")
	if err != nil {
		t.Fatalf("encrypt error: %v", err)
	}

	// Tamper with the ciphertext (flip a character in the middle)
	runes := []rune(encrypted)
	if len(runes) > 10 {
		if runes[10] == 'A' {
			runes[10] = 'B'
		} else {
			runes[10] = 'A'
		}
	}
	tampered := string(runes)

	_, err = decryptToken(tampered)
	if err == nil {
		t.Error("expected error when decrypting tampered ciphertext")
	}
}

func TestIsValidFQDN(t *testing.T) {
	tests := []struct {
		domain string
		valid  bool
	}{
		// Valid FQDNs
		{"example.com", true},
		{"vpn.example.com", true},
		{"sub.domain.example.co.uk", true},
		{"a.b", true},
		{"node-1.vpn.example.com", true},
		{"x.io", true},
		{"example.com.", true}, // trailing dot is valid
		{"a123.example.com", true},

		// Invalid FQDNs
		{"", false},
		{"localhost", false},       // single label
		{".example.com", false},    // leading dot
		{"example..com", false},    // empty label
		{"-example.com", false},    // label starts with hyphen
		{"example-.com", false},    // label ends with hyphen
		{"123.456", false},         // labels start with digit
		{"exam ple.com", false},    // space in label
		{"example.com/path", false},
		{"example_.com", false},    // underscore
		{strings.Repeat("a", 64) + ".com", false}, // label > 63 chars
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := isValidFQDN(tt.domain)
			if got != tt.valid {
				t.Errorf("isValidFQDN(%q) = %v, want %v", tt.domain, got, tt.valid)
			}
		})
	}
}

func TestNormalizeFailoverTTL(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"zero defaults to 60", 0, 60},
		{"negative defaults to 60", -1, 60},
		{"below minimum clamped to 30", 10, 30},
		{"at minimum", 30, 30},
		{"normal value", 60, 60},
		{"mid range", 300, 300},
		{"at maximum", 86400, 86400},
		{"above maximum clamped to 86400", 100000, 86400},
		{"exactly 1 below min", 29, 30},
		{"exactly 1 above max", 86401, 86400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFailoverTTL(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeFailoverTTL(%d) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}
