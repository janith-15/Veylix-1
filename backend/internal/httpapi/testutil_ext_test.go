package httpapi_test

import (
	"sync"
	"testing"

	"veylix/backend/internal/auth"
)

// testPasswordHash returns a valid argon2id hash for `password`, computed at
// most once per process per password.
//
// argon2id is deliberately slow (64 MB, memory-hard) — that is the point of it.
// A test that needs *a* usable password must not pay that cost per test, which is
// what made the suite drag as tests were added. The hash is identical to the one
// production would produce, so the sign-in path is still exercised for real.
var testPasswordHash = func() func(*testing.T, string) string {
	var (
		once  sync.Once
		value string
	)
	return func(t *testing.T, password string) string {
		t.Helper()
		once.Do(func() {
			hash, err := auth.HashPassword(password)
			if err != nil {
				t.Fatalf("hash password: %v", err)
			}
			value = hash
		})
		return value
	}
}()
