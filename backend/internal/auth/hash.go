package auth

import "crypto/sha256"

// sha256Sum is a tiny helper kept separate so seal.go stays focused on AEAD.
func sha256Sum(b []byte) [32]byte {
	return sha256.Sum256(b)
}
