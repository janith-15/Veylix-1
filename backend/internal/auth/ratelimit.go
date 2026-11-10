package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter is a small in-process token bucket keyed by an arbitrary string
// (login key = client IP, provisioning key = user id).
//
// It is deliberately simple: no Redis, no goroutines per key, and a periodic
// sweep keeps memory bounded on a 500 MB server (new_update.md §8).
type RateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	capacity  float64
	perSecond float64
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter allows burst events sustained at ratePerSecond.
func NewRateLimiter(burst int, ratePerSecond float64) *RateLimiter {
	return &RateLimiter{
		buckets:   make(map[string]*bucket),
		capacity:  float64(burst),
		perSecond: ratePerSecond,
		lastSweep: time.Now(),
	}
}

// Allow consumes one token for key.
func (l *RateLimiter) Allow(key string) bool {
	return l.AllowN(key, 1)
}

// AllowN consumes n tokens.
func (l *RateLimiter) AllowN(key string, n float64) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Sweep idle buckets at most once a minute so the map cannot grow forever.
	if now.Sub(l.lastSweep) > time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	}
	// Refill using nanosecond precision: a millisecond-truncated delta would
	// round to zero for fast rates and never refill.
	if elapsed := now.Sub(b.last); elapsed > 0 {
		b.tokens += elapsed.Seconds() * l.perSecond
		if b.tokens > l.capacity {
			b.tokens = l.capacity
		}
		b.last = now
	}

	if b.tokens < n {
		return false
	}
	b.tokens -= n
	return true
}

// RetryAfter reports how long the caller must wait before one more token is
// available for key. Zero means a token is available right now.
//
// It exists so a 429 can carry a Retry-After header: "too many attempts" with no
// duration leaves a locked-out person refreshing blindly, which is exactly the
// trap the login limiter is meant to avoid.
func (l *RateLimiter) RetryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		return 0
	}
	now := time.Now()
	tokens := b.tokens
	if elapsed := now.Sub(b.last); elapsed > 0 {
		tokens += elapsed.Seconds() * l.perSecond
	}
	if tokens >= 1 {
		return 0
	}
	if l.perSecond <= 0 {
		return time.Hour // no refill configured: effectively permanent
	}
	missing := 1 - tokens
	wait := time.Duration(missing / l.perSecond * float64(time.Second))
	if wait < time.Second {
		wait = time.Second
	}
	return wait
}

// Reset forgets a key (e.g. after a successful login so a legitimate user is
// not punished for earlier typos).
func (l *RateLimiter) Reset(key string) {
	l.mu.Lock()
	delete(l.buckets, key)
	l.mu.Unlock()
}

// ClientIP derives the caller address, for rate limiting and audit only.
//
// The server normally sits behind Caddy on loopback, so a forwarded header is
// trusted ONLY when the TCP peer really is loopback. That is the strongest
// condition available without configuration, and it is why a direct connection
// from the LAN cannot spoof its own address.
//
// When the peer is NOT loopback (a direct connection, a test, or a misconfigured
// proxy) the peer address is used and the header is ignored entirely — a caller
// must never be able to choose the key its rate limit is counted against.
func ClientIP(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	ip := net.ParseIP(peer)
	if ip == nil || !ip.IsLoopback() {
		return peer
	}
	fwd := r.Header.Get("X-Forwarded-For")
	if fwd == "" {
		return peer
	}
	// The FIRST entry is the original client: each proxy appends, so the list is
	// "client, proxy1, proxy2". A malformed value falls back to the peer rather
	// than being echoed into a log or a limiter key.
	first := strings.TrimSpace(fwd)
	if i := strings.IndexByte(first, ','); i >= 0 {
		first = strings.TrimSpace(first[:i])
	}
	if net.ParseIP(first) == nil {
		return peer
	}
	return first
}

// TrustForwardedProto reports whether the X-Forwarded-Proto header may be believed.
//
// It follows the same rule as ClientIP: only a loopback peer (our own Caddy) may
// change how the request is interpreted. Otherwise a client could claim `https`
// and steer the CSRF origin check.
func TrustForwardedProto(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
