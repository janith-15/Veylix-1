// Package google implements "Continue with Google" for Veylix (D-039).
//
// Design rules, in the order they matter:
//
//   - **Server-side only.** The browser is sent to Google, and Google sends it
//     back with a `code`. The code is exchanged for tokens by THIS server, so the
//     client secret never leaves the backend and no access token is ever exposed
//     to JavaScript.
//   - **No trusting the front end.** The identity comes from the ID token, which
//     is verified here: RS256 signature against Google's published keys, `iss`,
//     `aud`, `exp`/`iat` and the `nonce` this server issued.
//   - **No silent account takeover.** Linking to an existing account only happens
//     when Google reports the address as verified.
//   - **Fail closed.** Without configured credentials every entry point answers
//     ErrNotConfigured; nothing pretends to work.
package google

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Endpoints are variables so tests can point them at a local server.
var (
	AuthEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenEndpoint = "https://oauth2.googleapis.com/token"
	JWKSURL       = "https://www.googleapis.com/oauth2/v3/certs"
	// allowedIssuers are the `iss` values Google may use.
	allowedIssuers = []string{"https://accounts.google.com", "accounts.google.com"}
)

// ErrNotConfigured means no Google client is set up (see config.GoogleOAuth).
var ErrNotConfigured = errors.New("google: oauth client not configured")

// ErrInvalidToken is any failure to prove the ID token's authenticity.
var ErrInvalidToken = errors.New("google: invalid id token")

// Config is the OAuth client configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Client talks to Google. It is safe for concurrent use.
type Client struct {
	cfg  Config
	http *http.Client

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	keysAt  time.Time
	keyLife time.Duration
	nowFn   func() time.Time
}

// New builds a client. The HTTP client is bounded: an outbound call to Google
// must never hang a request handler.
func New(cfg Config) *Client {
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 10 * time.Second},
		keyLife: time.Hour,
		nowFn:   time.Now,
	}
}

// Configured reports whether a real client is available.
func (c *Client) Configured() bool {
	return c.cfg.ClientID != "" && c.cfg.ClientSecret != "" && c.cfg.RedirectURL != ""
}

// RedirectURL returns the callback URL to register in Google Cloud Console.
func (c *Client) RedirectURL() string { return c.cfg.RedirectURL }

// ClientID is public and may be shown to the UI (it is not a secret).
func (c *Client) ClientID() string { return c.cfg.ClientID }

// Handshake is the server-side state of one login attempt.
type Handshake struct {
	State    string
	Verifier string
	Nonce    string
}

// NewHandshake creates a state, a PKCE verifier and a nonce, and returns the
// PKCE challenge to send to Google.
//
// The state is what a callback must echo: it is stored server-side and deleted on
// use, so a callback the server never initiated (CSRF, or a code injected by a
// third party) cannot complete a login. PKCE additionally binds the code to this
// server even if the redirect URI leaks.
func (c *Client) NewHandshake() (Handshake, string, error) {
	state, err := randomToken(32)
	if err != nil {
		return Handshake{}, "", err
	}
	verifier, err := randomToken(32)
	if err != nil {
		return Handshake{}, "", err
	}
	nonce, err := randomToken(16)
	if err != nil {
		return Handshake{}, "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return Handshake{State: state, Verifier: verifier, Nonce: nonce},
		base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// AuthCodeURL builds the consent URL the browser is sent to.
//
// The nonce is NOT optional. Google echoes it inside the ID token, and
// VerifyIDToken compares it against the value this server stored for the
// handshake; omitting it here made that comparison fail every single time
// (`nonce mismatch`), so Google sign-in could never complete.
func (c *Client) AuthCodeURL(state, challenge, nonce string) string {
	q := url.Values{
		"client_id":             {c.cfg.ClientID},
		"redirect_uri":          {c.cfg.RedirectURL},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"access_type":           {"online"},
		"prompt":                {"select_account"},
	}
	return AuthEndpoint + "?" + q.Encode()
}

// randomToken returns n random bytes, base64url encoded (no padding).
func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("google: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Identity is the verified result of a Google login.
type Identity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// tokenResponse is the token endpoint's answer.
type tokenResponse struct {
	IDToken string `json:"id_token"`
	Error   string `json:"error"`
	Desc    string `json:"error_description"`
}

// Exchange swaps an authorization code for a verified identity.
//
// The exchange happens server-side with the client secret and the PKCE verifier;
// the ID token it returns is then verified before anything is trusted.
func (c *Client) Exchange(ctx context.Context, code, verifier, nonce string) (Identity, error) {
	if !c.Configured() {
		return Identity{}, ErrNotConfigured
	}
	form := url.Values{
		"code":          {code},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"redirect_uri":  {c.cfg.RedirectURL},
		"grant_type":    {"authorization_code"},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("google: token request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Identity{}, fmt.Errorf("google: read token response: %w", err)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Identity{}, fmt.Errorf("%w: token response is not JSON", ErrInvalidToken)
	}
	if res.StatusCode != http.StatusOK || tr.Error != "" || tr.IDToken == "" {
		// The description is for the server log only; it never reaches a user.
		return Identity{}, fmt.Errorf("%w: exchange refused (%s)", ErrInvalidToken, firstNonEmpty(tr.Error, res.Status))
	}
	return c.VerifyIDToken(ctx, tr.IDToken, nonce)
}

// claims is the subset of the ID token this product uses.
type claims struct {
	Issuer        string `json:"iss"`
	Audience      string `json:"aud"`
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Nonce         string `json:"nonce"`
	Expires       int64  `json:"exp"`
	IssuedAt      int64  `json:"iat"`
}

// VerifyIDToken proves the token was issued by Google for THIS client.
func (c *Client) VerifyIDToken(ctx context.Context, token, nonce string) (Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, fmt.Errorf("%w: not a JWT", ErrInvalidToken)
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Identity{}, fmt.Errorf("%w: bad header encoding", ErrInvalidToken)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return Identity{}, fmt.Errorf("%w: bad header", ErrInvalidToken)
	}
	// `none` and HS* are the classic JWT downgrades; Google only signs RS256.
	if header.Alg != "RS256" {
		return Identity{}, fmt.Errorf("%w: unexpected alg %q", ErrInvalidToken, header.Alg)
	}

	key, err := c.key(ctx, header.Kid)
	if err != nil {
		return Identity{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Identity{}, fmt.Errorf("%w: bad signature encoding", ErrInvalidToken)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	// The digest info is REQUIRED: PKCS#1 v1.5 signs a DigestInfo structure, and
	// passing 0 here would build a bare digest that no real Google token matches.
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return Identity{}, fmt.Errorf("%w: signature mismatch", ErrInvalidToken)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, fmt.Errorf("%w: bad payload encoding", ErrInvalidToken)
	}
	var cl claims
	if err := json.Unmarshal(payload, &cl); err != nil {
		return Identity{}, fmt.Errorf("%w: bad payload", ErrInvalidToken)
	}

	if !contains(allowedIssuers, cl.Issuer) {
		return Identity{}, fmt.Errorf("%w: issuer %q", ErrInvalidToken, cl.Issuer)
	}
	if cl.Audience != c.cfg.ClientID {
		// A token minted for another application must never sign anyone in here.
		return Identity{}, fmt.Errorf("%w: audience mismatch", ErrInvalidToken)
	}
	now := c.nowFn()
	if cl.Expires == 0 || now.Unix() >= cl.Expires {
		return Identity{}, fmt.Errorf("%w: expired", ErrInvalidToken)
	}
	if cl.IssuedAt != 0 && cl.IssuedAt > now.Add(5*time.Minute).Unix() {
		return Identity{}, fmt.Errorf("%w: issued in the future", ErrInvalidToken)
	}
	if nonce != "" && subtle.ConstantTimeCompare([]byte(cl.Nonce), []byte(nonce)) != 1 {
		return Identity{}, fmt.Errorf("%w: nonce mismatch", ErrInvalidToken)
	}
	if cl.Subject == "" {
		return Identity{}, fmt.Errorf("%w: no subject", ErrInvalidToken)
	}
	if cl.Email == "" {
		return Identity{}, fmt.Errorf("%w: no e-mail", ErrInvalidToken)
	}
	return Identity{
		Subject:       cl.Subject,
		Email:         strings.ToLower(strings.TrimSpace(cl.Email)),
		EmailVerified: cl.EmailVerified,
		Name:          cl.Name,
	}, nil
}

// key returns Google's signing key for kid, caching the key set for an hour.
func (c *Client) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	cached := c.keys[kid]
	fresh := c.nowFn().Sub(c.keysAt) < c.keyLife
	c.mu.Unlock()
	if cached != nil && fresh {
		return cached, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, JWKSURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google: fetch keys: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google: keys answered %s", res.Status)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&set); err != nil {
		return nil, fmt.Errorf("google: decode keys: %w", err)
	}

	parsed := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 {
			continue
		}
		parsed[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	c.mu.Lock()
	c.keys, c.keysAt = parsed, c.nowFn()
	c.mu.Unlock()

	if got := parsed[kid]; got != nil {
		return got, nil
	}
	return nil, fmt.Errorf("%w: unknown signing key %q", ErrInvalidToken, kid)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return "unknown"
}
