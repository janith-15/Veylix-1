package auth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRateLimiterAllowsBurstThenBlocks(t *testing.T) {
	l := NewRateLimiter(3, 0.001) // 3 burst, effectively no refill
	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if l.Allow("k") {
		t.Fatal("the 4th attempt must be blocked")
	}
}

func TestRateLimiterIsPerKey(t *testing.T) {
	l := NewRateLimiter(1, 0.001)
	if !l.Allow("a") {
		t.Fatal("key a should be allowed")
	}
	if !l.Allow("b") {
		t.Fatal("key b must not be affected by key a")
	}
}

func TestRateLimiterRefills(t *testing.T) {
	// A slow, deterministic refill: 2 tokens/second means a blocked caller
	// recovers after ~500 ms.
	l := NewRateLimiter(1, 2)
	if !l.Allow("k") {
		t.Fatal("first should pass")
	}
	if l.Allow("k") {
		t.Fatal("second should be blocked (bucket empty)")
	}
	time.Sleep(600 * time.Millisecond)
	if !l.Allow("k") {
		t.Fatal("the bucket should have refilled after the wait")
	}
}

func TestRateLimiterReset(t *testing.T) {
	l := NewRateLimiter(1, 0.0001)
	_ = l.Allow("k")
	if l.Allow("k") {
		t.Fatal("expected to be blocked")
	}
	l.Reset("k")
	if !l.Allow("k") {
		t.Fatal("reset should restore the budget")
	}
}

func TestRateLimiterIsSafeForConcurrentUse(t *testing.T) {
	l := NewRateLimiter(1000, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("shared") {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed == 0 || allowed > 1000 {
		t.Fatalf("unexpected allowed count: %d", allowed)
	}
}

func TestClientIPIgnoresForwardedHeaderFromNonLoopback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.9:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	if got := ClientIP(r); got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want the direct peer (headers from strangers are not trusted)", got)
	}
}

func TestClientIPTrustsForwardedFromLoopbackProxy(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 127.0.0.1")
	if got := ClientIP(r); got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q, want the first forwarded hop", got)
	}
}
