// Package config loads and validates the Veylix server configuration.
//
// Design rules (new_update.md §4, §7):
//   - configuration never comes from source code;
//   - a missing or weak secret is a hard start-up failure (fail closed);
//   - secrets are never logged.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the resolved runtime configuration.
type Config struct {
	// HTTPAddr is the listen address. Default binds to loopback only: the
	// public listener is Caddy in front of the binary (Bible §13.2).
	HTTPAddr string

	// DataDir holds the SQLite database and is created if missing.
	DataDir string

	// DBPath is the resolved SQLite file path.
	DBPath string

	// EncryptionKey seals panel credentials at rest (AES-256-GCM, D-011).
	// It is mandatory: without it the server refuses to start.
	EncryptionKey []byte

	// SessionKey signs session cookies.
	SessionKey []byte

	// SecureCookies marks the session cookie Secure. It defaults to true and
	// must only be disabled for plain-HTTP local development: sending a session
	// cookie over cleartext would expose it on the wire (Bible §8).
	//
	// Rules:
	//   - Secure by default, always;
	//   - a non-loopback bind is ALWAYS Secure — a cleartext-exposed cookie is
	//     never acceptable on a network-reachable listener, whatever the env says;
	//   - VEYLIX_INSECURE_COOKIES=1 disables it only for a loopback bind, which
	//     is the documented local-development override.
	SecureCookies bool

	// ReadTimeout / WriteTimeout / IdleTimeout bound every HTTP exchange.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// SyncInterval is how often the read-only panel sync runs (Stage E5).
	// Zero or negative disables the scheduler, which is what tests and a
	// read-only deployment want.
	SyncInterval time.Duration
	// SyncTimeout bounds ONE server's pass, so a hanging panel cannot consume
	// the whole cycle (REQUIREMENTS §3).
	SyncTimeout time.Duration
	// SyncPageSize / SyncMaxPages bound the paginated client read (D-012).
	SyncPageSize int
	SyncMaxPages int

	// GoogleOAuth is the optional "Continue with Google" client (D-039).
	// Empty ClientID/Secret means the feature is OFF and /api/auth/google/start
	// answers 503 google_not_configured — never a fake success.
	GoogleOAuth GoogleOAuth

	// SMTP is the transactional mail relay (D-041): verification links, e-mail
	// change codes and password resets. Empty Host means "no relay", and every
	// route that would send mail then reports so instead of claiming a delivery.
	SMTP SMTPConfig

	// PublicBaseURL is the absolute origin the browser uses (e.g.
	// https://vpn.example.com). It exists so e-mail can carry a clickable link:
	// a relative "/api/auth/verify/..." path is meaningless in a mailbox.
	PublicBaseURL string
}

// SMTPConfig is the mail relay. The password is never logged.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// Support is the owner's inbox. A super admin's password reset is copied here
	// (D-041) so the highest-privilege account stays recoverable even if its own
	// mailbox is lost.
	Support string
}

// Configured reports whether mail can be sent at all.
func (c SMTPConfig) Configured() bool { return c.Host != "" && c.From != "" }

// GoogleOAuth holds the OAuth 2.0 web client used for sign-in/sign-up.
//
// The secret is never logged (Config.String reports no secret values) and never
// sent to the browser: the authorization-code exchange happens server-side.
type GoogleOAuth struct {
	ClientID     string
	ClientSecret string
	// RedirectURL must match the URI registered in Google Cloud Console exactly.
	RedirectURL string
}

// Configured reports whether Google sign-in can be offered at all.
//
// A non-empty value is NOT enough: env templates ship placeholders such as
// <id>.apps.googleusercontent.com and <secret>. A placeholder that passes
// this check is worse than no client at all: /status answers configured=true,
// the login page enables the button, and the user is sent to Google only to
// return with invalid_client. The SHAPE is verified, not merely presence.
func (g GoogleOAuth) Configured() bool {
	return looksLikeClientID(g.ClientID) &&
		looksLikeClientSecret(g.ClientSecret) &&
		g.RedirectURL != ""
}

// looksLikeClientID matches every Google web client id: they always end in
// .apps.googleusercontent.com and the numeric project prefix makes them longer
// than 30 characters.
func looksLikeClientID(id string) bool {
	return len(id) > 30 && strings.HasSuffix(id, ".apps.googleusercontent.com")
}

// looksLikeClientSecret matches the secret format Google issues (GOCSPX-).
// A secret without that prefix cannot be exchanged for a token.
func looksLikeClientSecret(secret string) bool {
	return strings.HasPrefix(secret, "GOCSPX-") && len(secret) > 20
}

// Load resolves configuration from the environment.
//
// Recognised variables:
//
//	VEYLIX_ADDR              listen address               (default 127.0.0.1:8080)
//	VEYLIX_DATA_DIR          data directory               (default ./var/data)
//	VEYLIX_ENCRYPTION_KEY    base64, 32 bytes             (required)
//	VEYLIX_SESSION_KEY       base64, >= 32 bytes          (required)
func Load(getenv func(string) string) (*Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	addr := getenv("VEYLIX_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	dataDir := getenv("VEYLIX_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join("var", "data")
	}
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("config: resolve data dir: %w", err)
	}

	encKey, err := decodeKey("VEYLIX_ENCRYPTION_KEY", getenv("VEYLIX_ENCRYPTION_KEY"), 32)
	if err != nil {
		return nil, err
	}
	sessKey, err := decodeKey("VEYLIX_SESSION_KEY", getenv("VEYLIX_SESSION_KEY"), 32)
	if err != nil {
		return nil, err
	}
	secureCookies := resolveSecureCookies(addr, getenv("VEYLIX_INSECURE_COOKIES"))

	return &Config{
		HTTPAddr:      addr,
		DataDir:       absDataDir,
		DBPath:        filepath.Join(absDataDir, "veylix.db"),
		EncryptionKey: encKey,
		SessionKey:    sessKey,
		SecureCookies: secureCookies,
		ReadTimeout:   15 * time.Second,
		WriteTimeout:  30 * time.Second,
		IdleTimeout:   60 * time.Second,

		SyncInterval: resolveDuration(getenv("VEYLIX_SYNC_INTERVAL"), 5*time.Minute),
		SyncTimeout:  resolveDuration(getenv("VEYLIX_SYNC_TIMEOUT"), 60*time.Second),
		SyncPageSize: resolveInt(getenv("VEYLIX_SYNC_PAGE_SIZE"), 200, 1, 200),
		SyncMaxPages: resolveInt(getenv("VEYLIX_SYNC_MAX_PAGES"), 50, 1, 1000),

		GoogleOAuth: GoogleOAuth{
			ClientID:     strings.TrimSpace(getenv("VEYLIX_GOOGLE_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(getenv("VEYLIX_GOOGLE_CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(getenv("VEYLIX_GOOGLE_REDIRECT_URL")),
		},

		// Only used to build absolute links in e-mail (verification, reset). A
		// relative link in an e-mail is useless, so this must be the public origin.
		// Left empty in dev: the operator reads the token from the API instead.
		PublicBaseURL: strings.TrimRight(strings.TrimSpace(getenv("VEYLIX_PUBLIC_URL")), "/"),

		SMTP: SMTPConfig{
			Host:     strings.TrimSpace(getenv("VEYLIX_SMTP_HOST")),
			Port:     resolveInt(getenv("VEYLIX_SMTP_PORT"), 587, 1, 65535),
			Username: strings.TrimSpace(getenv("VEYLIX_SMTP_USER")),
			// NOT trimmed beyond spaces: a Gmail app password is often pasted with
			// a trailing newline, and trimming only that is safe; internal spacing
			// is never part of a password.
			Password: strings.TrimSpace(getenv("VEYLIX_SMTP_PASS")),
			From:     strings.TrimSpace(getenv("VEYLIX_MAIL_FROM")),
			// Defaults to the sender, because one address is the common case; an
			// explicit VEYLIX_SUPPORT_EMAIL separates them when they differ.
			Support: resolveSupport(strings.TrimSpace(getenv("VEYLIX_SUPPORT_EMAIL")),
				strings.TrimSpace(getenv("VEYLIX_MAIL_FROM"))),
		},
	}, nil
}

// resolveDuration parses a Go duration string, falling back to def on any
// problem. "0" or a negative value deliberately disables the feature.
func resolveDuration(raw string, def time.Duration) time.Duration {
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def
	}
	if d <= 0 {
		return 0
	}
	return d
}

// resolveInt parses an int within [lo, hi], falling back to def.
func resolveInt(raw string, def, lo, hi int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return def
	}
	return n
}

// resolveSecureCookies decides the cookie Secure flag.
//
// A network-reachable bind is always Secure. Only a loopback bind may opt out
// (plain-HTTP local development), and only when the operator asks explicitly.
// resolveSupport picks the inbox that receives super admin reset copies.
//
// An explicit VEYLIX_SUPPORT_EMAIL wins; otherwise the mail sender doubles as the
// support address, because a single-address deployment is the common case and a
// blank support inbox would silently drop the owner's copy.
func resolveSupport(support, from string) string {
	if support != "" {
		return support
	}
	// A From header may carry a display name ("Veylix <a@b>"); keep only the address.
	if i := strings.LastIndexByte(from, '<'); i >= 0 {
		if j := strings.IndexByte(from[i:], '>'); j > 0 {
			return strings.TrimSpace(from[i+1 : i+j])
		}
	}
	return from
}

func resolveSecureCookies(addr, insecureFlag string) bool {
	if !isLoopback(addr) {
		return true
	}
	return insecureFlag != "1"
}

// isLoopback reports whether addr binds only to the loopback interface, which
// is the development/default posture (Caddy terminates TLS in production).
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// decodeKey decodes a base64 key and enforces a minimum length.
func decodeKey(name, raw string, minLen int) ([]byte, error) {
	if raw == "" {
		return nil, fmt.Errorf("config: %s is required (generate one with: openssl rand -base64 32)", name)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("config: %s must be base64: %w", name, err)
	}
	if len(key) < minLen {
		return nil, fmt.Errorf("config: %s must decode to at least %d bytes (got %d)", name, minLen, len(key))
	}
	return key, nil
}

// EnsureDataDir creates the data directory with owner-only permissions.
func (c *Config) EnsureDataDir() error {
	if c.DataDir == "" {
		return errors.New("config: empty data dir")
	}
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("config: create data dir: %w", err)
	}
	return nil
}

// String summarises the configuration without exposing secrets.
func (c *Config) String() string {
	return fmt.Sprintf("addr=%s data=%s db=%s encKey=%dB sessKey=%dB google=%v sync=%s",
		c.HTTPAddr, c.DataDir, c.DBPath, len(c.EncryptionKey), len(c.SessionKey),
		c.GoogleOAuth.Configured(), c.SyncInterval)
}

// Port returns the numeric port of the listen address (used by readiness logs).
func (c *Config) Port() int {
	_, portStr, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return p
}
