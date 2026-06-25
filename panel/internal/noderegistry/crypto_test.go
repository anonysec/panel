package noderegistry

import (
	"bytes"
	"testing"
)

func TestEncryptorRoundTrip(t *testing.T) {
	enc := NewEncryptor("test-session-secret-at-least-32chars")

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"api key", []byte("sk-abcdef1234567890")},
		{"private key pem", []byte("-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----")},
		{"empty", []byte("")},
		{"single byte", []byte("x")},
		{"binary data", []byte{0x00, 0x01, 0xFF, 0xFE, 0x80}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := enc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Ciphertext must differ from plaintext (unless empty)
			if len(tt.plaintext) > 0 && bytes.Equal(ciphertext, tt.plaintext) {
				t.Fatal("ciphertext equals plaintext")
			}

			decrypted, err := enc.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Fatalf("round-trip mismatch: got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptorDifferentCiphertexts(t *testing.T) {
	enc := NewEncryptor("another-secret-string-32chars-long")
	plaintext := []byte("same-input-produces-different-output")

	ct1, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	ct2, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Each encryption should produce different ciphertext due to random nonce
	if bytes.Equal(ct1, ct2) {
		t.Fatal("two encryptions of the same plaintext produced identical ciphertext")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	enc1 := NewEncryptor("secret-one-at-least-32-characters")
	enc2 := NewEncryptor("secret-two-at-least-32-characters")

	ciphertext, err := enc1.Encrypt([]byte("sensitive-data"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = enc2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
}

func TestDecryptTooShort(t *testing.T) {
	enc := NewEncryptor("short-test-secret-32-chars-needed")

	_, err := enc.Decrypt([]byte{0x01, 0x02, 0x03})
	if err == nil {
		t.Fatal("expected error for ciphertext shorter than nonce")
	}
}

func TestDecryptCorrupted(t *testing.T) {
	enc := NewEncryptor("corruption-test-secret-32chars-x")
	plaintext := []byte("hello world")

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Flip a byte in the ciphertext portion (after nonce)
	if len(ciphertext) > 13 {
		ciphertext[13] ^= 0xFF
	}

	_, err = enc.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected error for corrupted ciphertext")
	}
}
