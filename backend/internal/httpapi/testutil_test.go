package httpapi

import (
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"veylix/backend/internal/auth"
)

// testTransport is shared by every test client that needs one.
//
// Why it matters: each `&http.Transport{}` opens its own connection pool, so
// every test paid a fresh TCP connect before its first assertion. Across a
// hundred tests that is seconds of pure setup — which is what made the suite
// feel like it was timing out. One shared, keep-alive transport removes that
// cost without changing a single assertion.
var testTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   100,
	IdleConnTimeout:       30 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// auth.HashPassword uses argon2id (64 MB, memory-hard) by design — hundreds of
// milliseconds per call. A test that only needs *a* usable password must not pay
// that per test, so the same password is hashed once per process and reused.
var (
	testHashOnce = sync.OnceValue(func() string {
		hash, err := auth.HashPassword("a very long password")
		if err != nil {
			panic("hash test password: " + err.Error())
		}
		return hash
	})
	// testHashes caches one hash per distinct password, so a test that needs a
	// second account does not pay for another argon2 run.
	testHashes   = map[string]string{}
	testHashesMu sync.Mutex
)

// testPasswordHash returns a valid argon2id hash for `password`, computed at
// most once per process per password.
func testPasswordHash(t *testing.T, password string) string {
	t.Helper()
	if password == "a very long password" {
		return testHashOnce()
	}
	testHashesMu.Lock()
	defer testHashesMu.Unlock()
	if h, ok := testHashes[password]; ok {
		return h
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	testHashes[password] = hash
	return hash
}
