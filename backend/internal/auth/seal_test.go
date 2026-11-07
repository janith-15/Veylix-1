package auth

import (
	"bytes"
	"errors"
	"testing"
)

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func TestSealerRoundTrip(t *testing.T) {
	s, err := NewSealer(testKey())
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	secret := []byte("panel-api-token-abcdef")
	aad := []byte("integration:1")

	sealed, err := s.Seal(secret, aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, secret) {
		t.Fatal("ciphertext contains the plaintext")
	}

	got, err := s.Open(sealed, aad)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("round trip mismatch: %q", got)
	}
}

func TestSealUsesFreshNonce(t *testing.T) {
	s, _ := NewSealer(testKey())
	a, _ := s.Seal([]byte("same"), []byte("aad"))
	b, _ := s.Seal([]byte("same"), []byte("aad"))
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same value must differ (nonce reuse)")
	}
}

// A ciphertext copied onto another row must not open (defence against a
// database-level swap of credentials between integrations).
func TestSealIsBoundToAAD(t *testing.T) {
	s, _ := NewSealer(testKey())
	sealed, _ := s.Seal([]byte("token-for-integration-1"), []byte("integration:1"))

	if _, err := s.Open(sealed, []byte("integration:2")); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("expected ErrCannotOpen for a different AAD, got %v", err)
	}
}

func TestSealDetectsTampering(t *testing.T) {
	s, _ := NewSealer(testKey())
	sealed, _ := s.Seal([]byte("token"), []byte("aad"))
	sealed[len(sealed)-1] ^= 0xFF

	if _, err := s.Open(sealed, []byte("aad")); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("expected ErrCannotOpen for a tampered ciphertext, got %v", err)
	}
}

func TestSealRejectsWrongKeySize(t *testing.T) {
	for _, size := range []int{0, 16, 31, 33, 64} {
		if _, err := NewSealer(make([]byte, size)); err == nil {
			t.Errorf("key of %d bytes must be rejected", size)
		}
	}
}

func TestOpenRejectsShortCiphertext(t *testing.T) {
	s, _ := NewSealer(testKey())
	if _, err := s.Open([]byte{1, 2, 3}, nil); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("expected ErrCannotOpen, got %v", err)
	}
}

func TestHashTokenIsStableAndIrreversible(t *testing.T) {
	a := HashToken("session-abc")
	b := HashToken("session-abc")
	if !bytes.Equal(a, b) {
		t.Fatal("token fingerprint must be stable")
	}
	if bytes.Contains(a, []byte("session-abc")) {
		t.Fatal("fingerprint contains the raw token")
	}
	if bytes.Equal(a, HashToken("session-abd")) {
		t.Fatal("different tokens must have different fingerprints")
	}
}
