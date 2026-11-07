// Package auth — password generation for operator-provisioned accounts.
//
// Why generated and not typed: an operator-created account (super admin, admin)
// is handed over once, in writing, by `.\dev.ps1 -SuperAdmin` / `-CreateAdmin`.
// A hand-typed password is either weak (fails the policy) or reused, and it ends
// up in shell history. A generated one is strong by construction, shown once, and
// stored only as a hash — the operator is expected to change it at first login.
package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// PasswordPattern selects how a generated password looks.
type PasswordPattern string

const (
	// PatternReadable is "Kavi-Ruhu7-Mango-Tara9": four capitalised words with a
	// digit on two of them. ~20+ chars, easy to read aloud over a phone, and it
	// passes the policy (length, no repeat, no straight sequence).
	PatternReadable PasswordPattern = "readable"
	// PatternPassphrase is "harbor lantern 47 quartz violin": five lowercase words
	// plus a number. Longest and strongest; the easiest to type on a phone.
	PatternPassphrase PasswordPattern = "passphrase"
	// PatternBase32 is "KRTM3XYQ7ZB2NVDP5HFG": no ambiguous separators, for
	// copy-paste or machine use.
	PatternBase32 PasswordPattern = "base32"
)

// ValidPattern reports whether p is a known pattern.
func (p PasswordPattern) Valid() bool {
	switch p {
	case PatternReadable, PatternPassphrase, PatternBase32:
		return true
	}
	return false
}

// wordList is deliberately small and concrete: short, unambiguous, hard to
// confuse when read aloud (no "read/red", no homophones), no offensive words.
var wordList = []string{
	"amber", "anchor", "apple", "arrow", "atlas", "aurora", "bamboo", "basalt",
	"beacon", "birch", "bison", "blaze", "bloom", "bolt", "breeze", "bronze",
	"cactus", "canyon", "cedar", "cinder", "cobalt", "comet", "copper", "coral",
	"cosmos", "crane", "crescent", "crimson", "crystal", "cypress", "dawn", "delta",
	"dune", "echo", "ember", "falcon", "fern", "fjord", "flint", "forest",
	"fossil", "garnet", "glacier", "granite", "harbor", "hazel", "heron", "hollow",
	"indigo", "ivory", "jade", "jasper", "jungle", "kestrel", "lagoon", "lantern",
	"lumen", "maple", "marble", "meadow", "mesa", "mica", "monsoon", "nebula",
	"nickel", "nimbus", "north", "oak", "obsidian", "olive", "onyx", "orbit",
	"osprey", "otter", "pebble", "pine", "plasma", "plume", "prairie", "quartz",
	"quill", "raven", "reef", "rhythm", "ridge", "river", "robin", "saffron",
	"sage", "sapphire", "sierra", "silver", "slate", "solstice", "sparrow", "spruce",
	"summit", "tundra", "turquoise", "umber", "valley", "velvet", "vertex", "violet",
	"willow", "zephyr", "zenith", "zinc",
}

const base32Alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I, O, 0, 1

// GeneratePassword returns a policy-valid password for the given pattern.
//
// Every random choice comes from crypto/rand, and the result is validated against
// ValidatePasswordPolicy before it is returned — so a generator change can never
// hand out a password the server would reject.
func GeneratePassword(p PasswordPattern) (string, error) {
	if !p.Valid() {
		return "", fmt.Errorf("auth: unknown password pattern %q", p)
	}

	var pw string
	switch p {
	case PatternBase32:
		var b strings.Builder
		for i := 0; i < 20; i++ {
			n, err := randInt(len(base32Alphabet))
			if err != nil {
				return "", err
			}
			b.WriteByte(base32Alphabet[n])
		}
		pw = b.String()
	case PatternPassphrase:
		words, err := pickWords(5)
		if err != nil {
			return "", err
		}
		n, err := randInt(90)
		if err != nil {
			return "", err
		}
		pw = strings.Join(words, " ") + fmt.Sprintf(" %02d", n+10)
	default: // PatternReadable
		words, err := pickWords(4)
		if err != nil {
			return "", err
		}
		for i := range words {
			words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
		}
		// Two of the four words carry a digit, so the password has digits without
		// any of them looking like a sequence.
		for _, idx := range []int{1, 3} {
			n, err := randInt(9)
			if err != nil {
				return "", err
			}
			words[idx] += fmt.Sprintf("%d", n+1)
		}
		pw = strings.Join(words, "-")
	}

	if err := ValidatePasswordPolicy(pw); err != nil {
		// Extremely unlikely, but a generated password must never be invalid.
		return "", fmt.Errorf("auth: generated password failed the policy: %w", err)
	}
	return pw, nil
}

// pickWords returns n distinct words from the list.
func pickWords(n int) ([]string, error) {
	used := make(map[int]bool, n)
	out := make([]string, 0, n)
	for len(out) < n {
		idx, err := randInt(len(wordList))
		if err != nil {
			return nil, err
		}
		if used[idx] {
			continue
		}
		used[idx] = true
		out = append(out, wordList[idx])
	}
	return out, nil
}

// randInt returns a uniform random int in [0,n) using crypto/rand.
func randInt(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("auth: randInt needs a positive bound")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, fmt.Errorf("auth: random: %w", err)
	}
	return int(v.Int64()), nil
}
