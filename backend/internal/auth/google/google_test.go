package google

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient returns a client wired to a local JWKS server with a fresh key.
func newTestClient(t *testing.T) (*Client, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	enc := base64.RawURLEncoding
	jwks := map[string]any{
		"keys": []map[string]any{{
			"kid": "test-kid", "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": enc.EncodeToString(key.PublicKey.N.Bytes()),
			"e": enc.EncodeToString([]byte{1, 0, 1}),
		}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)

	old := JWKSURL
	JWKSURL = srv.URL
	t.Cleanup(func() { JWKSURL = old })

	return New(Config{ClientID: "client-abc", ClientSecret: "secret", RedirectURL: "http://localhost/cb"}), key
}

// sign builds an RS256 ID token with the given claims.
func sign(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","kid":"test-kid","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	payload := enc.EncodeToString(payloadJSON)
	sum := sha256.Sum256([]byte(header + "." + payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return header + "." + payload + "." + enc.EncodeToString(sig)
}

func validClaims() map[string]any {
	now := time.Now().Unix()
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            "client-abc",
		"sub":            "google-sub-123",
		"email":          "Person@Example.com",
		"email_verified": true,
		"name":           "Person Example",
		"nonce":          "nonce-123",
		"exp":            now + 3600,
		"iat":            now,
	}
}

// A correctly signed token for this client signs the person in.
func TestVerifyIDTokenAcceptsAValidToken(t *testing.T) {
	c, key := newTestClient(t)
	ident, err := c.VerifyIDToken(context.Background(), sign(t, key, validClaims()), "nonce-123")
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if ident.Subject != "google-sub-123" || ident.Email != "person@example.com" {
		t.Fatalf("identity = %+v, want the token's subject and a lower-cased e-mail", ident)
	}
	if !ident.EmailVerified {
		t.Fatal("email_verified must be carried through — the linking rule depends on it")
	}
}

// Every way a token can be wrong must be refused. One case per attack.
//
// Each case starts from a VALID token and breaks exactly one thing, so a failure
// names the check that is missing rather than "verification is broken".
func TestVerifyIDTokenRefusesBadTokens(t *testing.T) {
	c, key := newTestClient(t)

	cases := []struct {
		name   string
		mutate func(cl map[string]any)
		nonce  string
	}{
		{"wrong audience (token minted for another app)", func(cl map[string]any) { cl["aud"] = "someone-else" }, "nonce-123"},
		{"wrong issuer", func(cl map[string]any) { cl["iss"] = "https://evil.example.com" }, "nonce-123"},
		{"expired", func(cl map[string]any) { cl["exp"] = time.Now().Add(-time.Minute).Unix() }, "nonce-123"},
		{"issued in the future", func(cl map[string]any) {
			cl["iat"] = time.Now().Add(time.Hour).Unix()
		}, "nonce-123"},
		{"nonce mismatch (replayed token)", func(cl map[string]any) {}, "another-nonce"},
		{"no subject", func(cl map[string]any) { delete(cl, "sub") }, "nonce-123"},
		{"no e-mail", func(cl map[string]any) { delete(cl, "email") }, "nonce-123"},
	}
	for _, tc := range cases {
		cl := validClaims()
		tc.mutate(cl)
		if _, err := c.VerifyIDToken(context.Background(), sign(t, key, cl), tc.nonce); err == nil {
			t.Errorf("%s: token accepted, want a refusal", tc.name)
		}
	}

	// A token signed by a key Google never published.
	rogueKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	if _, err := c.VerifyIDToken(context.Background(), sign(t, rogueKey, validClaims()), "nonce-123"); err == nil {
		t.Error("token signed by an unknown key accepted")
	}
}

// alg=none and HS256 are the classic JWT downgrades.
func TestVerifyIDTokenRefusesAlgorithmDowngrades(t *testing.T) {
	c, _ := newTestClient(t)
	enc := base64.RawURLEncoding
	payloadJSON, _ := json.Marshal(validClaims())
	payload := enc.EncodeToString(payloadJSON)

	for _, header := range []string{`{"alg":"none"}`, `{"alg":"HS256","kid":"test-kid"}`} {
		token := enc.EncodeToString([]byte(header)) + "." + payload + "."
		if _, err := c.VerifyIDToken(context.Background(), token, "nonce-123"); err == nil {
			t.Errorf("header %s accepted, want a refusal", header)
		}
	}
	if _, err := c.VerifyIDToken(context.Background(), "not-a-jwt", ""); err == nil {
		t.Error("non-JWT accepted")
	}
}

// Without credentials nothing pretends to work.
func TestUnconfiguredClientRefusesEverything(t *testing.T) {
	c := New(Config{})
	if c.Configured() {
		t.Fatal("an empty config must not be 'configured'")
	}
	if _, err := c.Exchange(context.Background(), "code", "verifier", "nonce"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Exchange err = %v, want ErrNotConfigured", err)
	}
}

// The handshake is unguessable and the challenge is the S256 of the verifier.
func TestHandshakeIsRandomAndChallengesCorrectly(t *testing.T) {
	c, _ := newTestClient(t)
	a, challenge, err := c.NewHandshake()
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	b, _, err := c.NewHandshake()
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if a.State == b.State || a.Verifier == b.Verifier || a.Nonce == b.Nonce {
		t.Fatal("two handshakes produced the same value — randomness is broken")
	}
	sum := sha256.Sum256([]byte(a.Verifier))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); challenge != want {
		t.Fatalf("challenge = %q, want S256(verifier)", challenge)
	}
	u := c.AuthCodeURL(a.State, challenge, a.Nonce)
	for _, fragment := range []string{"code_challenge_method=S256", "state=" + a.State, "client_id=client-abc", "response_type=code", "nonce=" + a.Nonce} {
		if !strings.Contains(u, fragment) {
			t.Errorf("consent URL is missing %q", fragment)
		}
	}
	if strings.Contains(u, "client_secret") {
		t.Error("the consent URL must never carry the client secret")
	}
}

// The nonce must reach Google, or VerifyIDToken can never match it.
//
// This is the regression test for the `nonce mismatch` failure that made Google
// sign-in impossible to complete: NewHandshake minted a nonce and VerifyIDToken
// compared it, but AuthCodeURL never sent it, so every real login was refused.
func TestAuthCodeURLCarriesTheNonce(t *testing.T) {
	c := New(Config{ClientID: "client-abc", ClientSecret: "shh", RedirectURL: "http://localhost:5173/api/auth/google/callback"})
	hs, challenge, err := c.NewHandshake()
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	u := c.AuthCodeURL(hs.State, challenge, hs.Nonce)
	if !strings.Contains(u, "nonce="+hs.Nonce) {
		t.Fatalf("authorize URL has no nonce; the ID token check can never pass:\n%s", u)
	}
	if hs.Nonce == "" {
		t.Fatal("NewHandshake must mint a non-empty nonce")
	}
}
