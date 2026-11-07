package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRejectsShortPassword(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected a weak-password error")
	}
}

func TestHashPasswordProducesArgon2id(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(h, "argon2id$") {
		t.Fatalf("hash does not use argon2id: %q", h)
	}
	if strings.Contains(h, "correct horse") {
		t.Fatal("hash contains the plaintext password")
	}
}

func TestVerifyPassword(t *testing.T) {
	const pw = "correct horse battery staple"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword(h, pw) {
		t.Error("the correct password must verify")
	}
	if VerifyPassword(h, pw+"x") {
		t.Error("a wrong password must not verify")
	}
	if VerifyPassword(h, "") {
		t.Error("an empty password must not verify")
	}
}

// Two hashes of the same password must differ (unique salt).
func TestHashPasswordIsSalted(t *testing.T) {
	a, _ := HashPassword("a sufficiently long password")
	b, _ := HashPassword("a sufficiently long password")
	if a == b {
		t.Fatal("hashes must be salted: identical output for the same input")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	bad := []string{"", "plain", "argon2id$", "argon2id$1$1$1", "bcrypt$x$y$z$w$v", "argon2id$0$0$0$aa$bb"}
	for _, h := range bad {
		if VerifyPassword(h, "anything at all here") {
			t.Errorf("malformed hash %q must not verify", h)
		}
	}
}

func TestNeedsRehashDetectsWeakerParameters(t *testing.T) {
	strong, _ := HashPassword("a sufficiently long password")
	if NeedsRehash(strong) {
		t.Error("a current hash must not need rehashing")
	}
	if !NeedsRehash("argon2id$1$1024$1$c2FsdA$a2V5") {
		t.Error("a weak-parameter hash must be flagged for rehash")
	}
	if !NeedsRehash("pbkdf2-sha256$600000$c2FsdA$a2V5") {
		t.Error("an unknown algorithm must be flagged for rehash")
	}
}

// F-017: length alone is not enough — trivially guessable values must be
// rejected even though they satisfy the 12-character minimum.
func TestValidatePasswordPolicyRejectsGuessableValues(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		want error
	}{
		{"too short", "short", ErrWeakPassword},
		{"eleven runes", "abcdefghijk", ErrWeakPassword},
		{"one rune repeated", "aaaaaaaaaaaa", ErrPasswordRepeated},
		{"digit repeated", "111111111111", ErrPasswordRepeated},
		{"ascending sequence", "abcdefghijkl", ErrPasswordSequence},
		{"descending sequence", "lkjihgfedcba", ErrPasswordSequence},
		{"blocklisted", "password1234", ErrPasswordCommon},
		{"blocklisted uppercase", "PASSWORD1234", ErrPasswordCommon},
		{"too long", strings.Repeat("Zx9!", MaxPasswordLength), ErrPasswordTooLong},
	}
	for _, tc := range cases {
		if err := ValidatePasswordPolicy(tc.pw); err != tc.want {
			t.Errorf("%s (%q) = %v, want %v", tc.name, tc.pw, err, tc.want)
		}
	}
}

// A strong password must pass, and the message for a rejection must never
// contain the password itself (it would then be written to the client).
func TestValidatePasswordPolicyAcceptsStrongPassword(t *testing.T) {
	for _, pw := range []string{
		"correct horse battery staple",
		"මගේ රහස් උද්‍යානයේ කුරුල්ලෝ", // Sinhala, > 12 runes
		"Tr0ub4dor&3xK9!wq",
	} {
		if err := ValidatePasswordPolicy(pw); err != nil {
			t.Errorf("%q must pass the policy, got %v", pw, err)
		}
	}

	rejected := "aaaaaaaaaaaa"
	if err := ValidatePasswordPolicy(rejected); err != nil && strings.Contains(err.Error(), rejected) {
		t.Fatal("a rejection message must never echo the password")
	}
}

// HashPassword must refuse guessable values too, so no call site can bypass
// the policy by hashing directly.
func TestHashPasswordAppliesFullPolicy(t *testing.T) {
	if _, err := HashPassword("aaaaaaaaaaaa"); err != ErrPasswordRepeated {
		t.Errorf("repeated rune hash = %v, want ErrPasswordRepeated", err)
	}
	if _, err := HashPassword("password1234"); err != ErrPasswordCommon {
		t.Errorf("blocklisted hash = %v, want ErrPasswordCommon", err)
	}
}
