package noderegistry

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

// Encryptor handles AES-256-GCM encryption for sensitive node credentials
// (API keys and client private keys). The 32-byte encryption key is derived
// from PANEL_SESSION_SECRET via HKDF with SHA-256.
type Encryptor struct {
	key [32]byte
}

// NewEncryptor derives a 32-byte AES key from the given secret using HKDF (SHA-256).
// The info parameter "noderegistry-encryption" scopes this key derivation to this use case.
func NewEncryptor(secret string) *Encryptor {
	e := &Encryptor{}
	reader := hkdf.New(sha256.New, []byte(secret), nil, []byte("noderegistry-encryption"))
	if _, err := io.ReadFull(reader, e.key[:]); err != nil {
		// HKDF with SHA-256 should never fail to produce 32 bytes
		panic("noderegistry: failed to derive encryption key: " + err.Error())
	}
	return e
}

// Encrypt encrypts plaintext using AES-256-GCM. The 12-byte nonce is prepended
// to the ciphertext for storage. Returns nonce || ciphertext || tag.
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Seal appends ciphertext+tag to nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext produced by Encrypt. It expects the format:
// nonce (12 bytes) || ciphertext || tag (16 bytes).
func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("noderegistry: ciphertext too short")
	}

	nonce, sealed := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, errors.New("noderegistry: decryption failed (invalid key or corrupted data)")
	}

	return plaintext, nil
}
