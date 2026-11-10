// Package auth holds credential handling: password hashing, session tokens and
// sealing of panel API tokens at rest (D-016).
//
// Security rules (new_update.md §7, Bible §8):
//   - passwords are never stored or logged in plaintext;
//   - tokens are sealed with AES-256-GCM under a mandatory config key;
//   - comparisons are constant-time where an attacker could time them.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Password hashing parameters — argon2id, per OWASP recommendation.
//
// The algorithm and its parameters are encoded into the stored hash, so the
// cost can be raised later and old hashes still verify (NeedsRehash reports it).
const (
	argonAlgorithm = "argon2id"
	argonMemoryKiB = 64 * 1024 // 64 MB
	argonTime      = 2
	argonThreads   = 2
	argonSaltLen   = 16
	argonKeyLen    = 32
)

// MinPasswordLength is the minimum accepted password length (runes).
const MinPasswordLength = 12

// MaxPasswordLength bounds the input so a huge body cannot be turned into a
// multi-second argon2 job (DoS guard; new_update.md §7).
const MaxPasswordLength = 512

// ErrWeakPassword is returned when a password fails the minimum policy.
var ErrWeakPassword = fmt.Errorf("auth: password must be at least %d characters", MinPasswordLength)

// ErrPasswordTooLong is returned when a password exceeds MaxPasswordLength.
var ErrPasswordTooLong = fmt.Errorf("auth: password must be at most %d characters", MaxPasswordLength)

// blockedPasswords are rejected even when long enough: they are the values a
// spray list tries first, and length alone would let them through.
var blockedPasswords = map[string]bool{
	"password1234":     true,
	"password12345":    true,
	"passwordpassword": true,
	"qwertyuiop123":    true,
	"123456789012":     true,
	"1234567890123":    true,
	"abcdefghijkl":     true,
	"iloveyou1234":     true,
	"administrator":    true,
	"veylixadmin12":    true,
}

// Password-policy sentinels. Every one of them is safe to show to the user:
// it names the rule that failed and never the password itself.
var (
	// ErrPasswordRepeated → the password is one character repeated.
	ErrPasswordRepeated = errors.New("password must not be a single repeated character")
	// ErrPasswordSequence → the password is a straight rune sequence.
	ErrPasswordSequence = errors.New("password must not be a simple sequence")
	// ErrPasswordCommon → the password appears in the built-in blocklist.
	ErrPasswordCommon = errors.New("password is too common")
)

// ValidatePasswordPolicy enforces the full policy: length, a bound on input
// size, and a cheap entropy screen (no single repeated rune, no straight
// sequence, not a well-known credential).
//
// The checks are deliberately understandable to a user: a rejection always
// names the rule that failed, never the password itself.
func ValidatePasswordPolicy(password string) error {
	runes := []rune(password)
	if len(runes) < MinPasswordLength {
		return ErrWeakPassword
	}
	if len(runes) > MaxPasswordLength {
		return ErrPasswordTooLong
	}

	// "aaaaaaaaaaaa" / "111111111111".
	repeated := true
	for i := 1; i < len(runes); i++ {
		if runes[i] != runes[0] {
			repeated = false
			break
		}
	}
	if repeated {
		return ErrPasswordRepeated
	}

	// "abcdefghijkl" (or descending).
	ascending, descending := true, true
	for i := 1; i < len(runes); i++ {
		delta := runes[i] - runes[i-1]
		if delta != 1 {
			ascending = false
		}
		if delta != -1 {
			descending = false
		}
	}
	if ascending || descending {
		return ErrPasswordSequence
	}

	if blockedPasswords[strings.ToLower(password)] {
		return ErrPasswordCommon
	}
	return nil
}

// HashPassword derives an argon2id hash. Format:
//
//	argon2id$<time>$<memoryKiB>$<threads>$<base64 salt>$<base64 key>
func HashPassword(password string) (string, error) {
	if err := ValidatePasswordPolicy(password); err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("%s$%d$%d$%d$%s$%s", argonAlgorithm, argonTime, argonMemoryKiB, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the stored hash.
func VerifyPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != argonAlgorithm {
		return false
	}
	timeCost, err1 := strconv.ParseUint(parts[1], 10, 32)
	memKiB, err2 := strconv.ParseUint(parts[2], 10, 32)
	threads, err3 := strconv.ParseUint(parts[3], 10, 8)
	if err1 != nil || err2 != nil || err3 != nil || timeCost == 0 || memKiB == 0 || threads == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(timeCost), uint32(memKiB), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// NeedsRehash reports whether a stored hash uses weaker parameters than the
// current policy, so it can be upgraded after a successful login.
func NeedsRehash(stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != argonAlgorithm {
		return true
	}
	timeCost, _ := strconv.ParseUint(parts[1], 10, 32)
	memKiB, _ := strconv.ParseUint(parts[2], 10, 32)
	threads, _ := strconv.ParseUint(parts[3], 10, 8)
	return uint32(timeCost) < argonTime || uint32(memKiB) < argonMemoryKiB || uint8(threads) < argonThreads
}

// ErrInvalidCredentials is deliberately identical for "unknown email" and
// "wrong password" so the API cannot be used to enumerate accounts.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")
