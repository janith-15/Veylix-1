package auth

import (
	"strings"
	"testing"
)

// A generated password must ALWAYS satisfy the policy the server enforces —
// otherwise `-SuperAdmin` could hand out a credential the API refuses.
func TestGeneratedPasswordsArePolicyValid(t *testing.T) {
	for _, p := range []PasswordPattern{PatternReadable, PatternPassphrase, PatternBase32} {
		t.Run(string(p), func(t *testing.T) {
			for i := 0; i < 200; i++ {
				pw, err := GeneratePassword(p)
				if err != nil {
					t.Fatalf("generate: %v", err)
				}
				if err := ValidatePasswordPolicy(pw); err != nil {
					t.Fatalf("generated %q is not policy-valid: %v", pw, err)
				}
				if len([]rune(pw)) < MinPasswordLength {
					t.Fatalf("%q is shorter than the minimum (%d)", pw, MinPasswordLength)
				}
			}
		})
	}
}

// Two calls must not return the same password (randomness, not a fixed template).
func TestGeneratedPasswordsDiffer(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		pw, err := GeneratePassword(PatternReadable)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if seen[pw] {
			t.Fatalf("duplicate password generated: %q", pw)
		}
		seen[pw] = true
	}
}

// The readable pattern is readable: words are capitalised, joined by hyphens, and
// exactly two of them end in a digit.
func TestReadablePatternShape(t *testing.T) {
	pw, err := GeneratePassword(PatternReadable)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	parts := strings.Split(pw, "-")
	if len(parts) != 4 {
		t.Fatalf("%q has %d parts, want 4", pw, len(parts))
	}
	digits := 0
	for _, part := range parts {
		if part == "" || part[0] < 'A' || part[0] > 'Z' {
			t.Errorf("part %q does not start with a capital", part)
		}
		if last := part[len(part)-1]; last >= '0' && last <= '9' {
			digits++
		}
	}
	if digits != 2 {
		t.Errorf("%q carries %d digits, want exactly 2", pw, digits)
	}
}

// An unknown pattern is an error, never a default.
func TestUnknownPatternIsRefused(t *testing.T) {
	if _, err := GeneratePassword("somethingelse"); err == nil {
		t.Fatal("an unknown pattern was accepted")
	}
	if PasswordPattern("").Valid() {
		t.Fatal("an empty pattern reports valid")
	}
}
