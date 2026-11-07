package httpapi

import (
	"math/big"
	"testing"

	"veylix/backend/internal/auth"
)

// The operator tool and the API must agree on how a code is hashed, or a code
// issued by ".\dev.ps1 -ResetLink" would never redeem. This reproduces the tool's
// exact steps against the API's verifier.
func TestOperatorIssuedCodeRedeems(t *testing.T) {
	// Mirrors cmd/accounttool/resetcode.go: same salt, same TTL path.
	const toolSalt = "|veylix-verification-code|"
	suffix := codeSaltSuffix
	if toolSalt != suffix {
		t.Fatalf("salt drift: tool=%q api=%q", toolSalt, suffix)
	}

	for i := 0; i < 20; i++ {
		max := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		code, err := newNumericCode(6)
		if err != nil {
			t.Fatalf("newNumericCode: %v", err)
		}
		_ = max
		if len(code) != 6 {
			t.Fatalf("code %q must be 6 digits", code)
		}
		hash, err := auth.HashPassword(code + toolSalt)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		if !checkCode(hash, code) {
			t.Fatalf("code %q hashed by the tool must verify through the API", code)
		}
		if checkCode(hash, "000000") && code != "000000" {
			t.Fatalf("a different code must not verify")
		}
	}
}
