package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// Sealer encrypts panel API tokens at rest with AES-256-GCM (D-016).
//
// A random nonce is generated per seal and prepended to the ciphertext, so the
// same token never produces the same bytes twice. The additional-data argument
// binds a ciphertext to its owner (e.g. the integration id), which means a
// ciphertext copied onto another row fails to open.
type Sealer struct {
	aead cipher.AEAD
}

// ErrCannotOpen is returned when a ciphertext is corrupt, tampered with, or
// bound to a different row.
var ErrCannotOpen = errors.New("auth: cannot open sealed value")

// NewSealer builds a Sealer from a 32-byte key.
func NewSealer(key []byte) (*Sealer, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("auth: encryption key must be 32 bytes (got %d)", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("auth: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("auth: gcm: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts plaintext and binds it to aad.
func (s *Sealer) Seal(plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("auth: nonce: %w", err)
	}
	// nonce || ciphertext (ciphertext already includes the GCM tag)
	return s.aead.Seal(nonce, nonce, plaintext, aad), nil
}

// Open decrypts a value produced by Seal with the same aad.
func (s *Sealer) Open(sealed, aad []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n+s.aead.Overhead() {
		return nil, ErrCannotOpen
	}
	plaintext, err := s.aead.Open(nil, sealed[:n], sealed[n:], aad)
	if err != nil {
		return nil, ErrCannotOpen
	}
	return plaintext, nil
}

// HashToken returns a stable, non-reversible fingerprint of a secret, used to
// store session identifiers without keeping the raw cookie value.
func HashToken(token string) []byte {
	sum := sha256Sum([]byte(token))
	return sum[:]
}
