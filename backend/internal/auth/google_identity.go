package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth/google"
	"veylix/backend/internal/store"
)

// IdentityStore is what the Google flow needs from the store layer. Declared
// here (not imported) so the auth package has no dependency on the database and
// the flow is testable with a fake.
type IdentityStore interface {
	CreateOAuthState(ctx context.Context, st store.OAuthState) error
	ConsumeOAuthState(ctx context.Context, state string) (store.OAuthState, error)
	UserIDForIdentity(ctx context.Context, provider, subject string) (int64, error)
	LinkIdentity(ctx context.Context, userID int64, provider, subject, email string) error
	UserByEmail(ctx context.Context, email string) (int64, string, error)
	CreateOAuthUser(ctx context.Context, email, displayName, subject string) (int64, error)
	UserByID(ctx context.Context, id int64) (*store.UserRow, error)
	CreateEmailVerification(ctx context.Context, v store.EmailVerification) error
	ConsumeEmailVerification(ctx context.Context, token string) (store.EmailVerification, error)
	MarkEmailVerified(ctx context.Context, userID int64) error
}

// GoogleAuth ties google.Client to the store and the audit log.
type GoogleAuth struct {
	Client *google.Client
	Store  IdentityStore
	Audit  *audit.Logger
	Now    func() time.Time
}

var (
	// ErrGoogleNotConfigured is returned when no OAuth client exists.
	ErrGoogleNotConfigured = google.ErrNotConfigured
	// ErrGoogleState is a callback this server did not start (CSRF / replay).
	ErrGoogleState = errors.New("auth: google state is invalid or expired")
	// ErrGoogleEmailUnverified means Google itself does not vouch for the address.
	ErrGoogleEmailUnverified = errors.New("auth: google account e-mail is not verified")
	// ErrGoogleIdentityTaken means the Google account is linked to someone else.
	ErrGoogleIdentityTaken = errors.New("auth: this google account is already linked to another user")
	// ErrGoogleDenied is Google's own refusal (user cancelled, bad request).
	ErrGoogleDenied = errors.New("auth: google sign-in was refused")
)

// Start begins a login: it records the handshake and returns the consent URL.
func (g *GoogleAuth) Start(ctx context.Context) (string, error) {
	if g.Client == nil || !g.Client.Configured() {
		return "", ErrGoogleNotConfigured
	}
	hs, challenge, err := g.Client.NewHandshake()
	if err != nil {
		return "", err
	}
	if err := g.Store.CreateOAuthState(ctx, store.OAuthState{
		State:    hs.State,
		Verifier: hs.Verifier,
		Nonce:    hs.Nonce,
		Expires:  g.now().Add(5 * time.Minute),
	}); err != nil {
		return "", err
	}
	return g.Client.AuthCodeURL(hs.State, challenge, hs.Nonce), nil
}

// Finish completes a login and returns the Veylix user plus whether a new account
// was created (so the UI can say "welcome" instead of "welcome back").
//
// The three cases below are the whole security model of the feature:
//  1. the Google account is already linked   → sign in as that user;
//  2. the e-mail exists but is not linked    → LINK it, but only when Google
//     reports the address verified (a Google account holding an unverified
//     address must never be able to claim an existing Veylix account);
//  3. nobody holds the e-mail                → create a client account, e-mail
//     verified by Google, with no password.
func (g *GoogleAuth) Finish(ctx context.Context, state, code string) (*store.UserRow, bool, error) {
	if g.Client == nil || !g.Client.Configured() {
		return nil, false, ErrGoogleNotConfigured
	}
	if code == "" {
		return nil, false, ErrGoogleDenied
	}
	// Consuming the state IS the CSRF check: an unknown state means this callback
	// was not started by this server (or was replayed).
	hs, err := g.Store.ConsumeOAuthState(ctx, state)
	if err != nil {
		return nil, false, ErrGoogleState
	}

	ident, err := g.Client.Exchange(ctx, code, hs.Verifier, hs.Nonce)
	if err != nil {
		slog.Warn("google sign-in failed", "err", err)
		return nil, false, err
	}
	return g.Resolve(ctx, ident)
}

// Resolve applies the linking rules to an already-verified Google identity.
//
// Split out of Finish so the rules can be tested without a live Google exchange:
// this is the part that decides whether a stranger's Google account may become —
// or take over — a Veylix account.
func (g *GoogleAuth) Resolve(ctx context.Context, ident google.Identity) (*store.UserRow, bool, error) {
	// 1) Already linked.
	if id, lookErr := g.Store.UserIDForIdentity(ctx, store.ProviderGoogle, ident.Subject); lookErr == nil {
		usr, err := g.Store.UserByID(ctx, id)
		if err != nil {
			return nil, false, err
		}
		g.record(ctx, id, "auth.google_login", "", map[string]any{"subject": ident.Subject})
		return usr, false, nil
	} else if !errors.Is(lookErr, store.ErrNoSuchIdentity) {
		return nil, false, lookErr
	}

	// 2) Existing e-mail — link only a verified Google address.
	if id, _, lookErr := g.Store.UserByEmail(ctx, ident.Email); lookErr == nil {
		if !ident.EmailVerified {
			g.record(ctx, id, "auth.google_link_denied", "DENIED",
				map[string]any{"reason": "google_email_unverified"})
			return nil, false, ErrGoogleEmailUnverified
		}
		if err := g.Store.LinkIdentity(ctx, id, store.ProviderGoogle, ident.Subject, ident.Email); err != nil {
			if errors.Is(err, store.ErrIdentityTaken) {
				return nil, false, ErrGoogleIdentityTaken
			}
			return nil, false, err
		}
		usr, err := g.Store.UserByID(ctx, id)
		if err != nil {
			return nil, false, err
		}
		g.record(ctx, id, "auth.google_link", "", map[string]any{"subject": ident.Subject})
		return usr, false, nil
	} else if !errors.Is(lookErr, store.ErrNoSuchIdentity) {
		return nil, false, lookErr
	}

	// 3) New account. Google is the authority for the address, so it is verified.
	name := strings.TrimSpace(ident.Name)
	if name == "" {
		name = ident.Email
	}
	id, err := g.Store.CreateOAuthUser(ctx, ident.Email, name, ident.Subject)
	if err != nil {
		return nil, false, err
	}
	if err := g.Store.LinkIdentity(ctx, id, store.ProviderGoogle, ident.Subject, ident.Email); err != nil {
		return nil, false, err
	}
	usr, err := g.Store.UserByID(ctx, id)
	if err != nil {
		return nil, false, err
	}
	g.record(ctx, id, audit.ActionRegister, "", map[string]any{"role": string(RoleClient), "provider": "google"})
	return usr, true, nil
}

func (g *GoogleAuth) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// record writes an audit entry; a nil logger (tests) is tolerated.
func (g *GoogleAuth) record(ctx context.Context, userID int64, action, result string, extra map[string]any) {
	if g.Audit == nil {
		return
	}
	_ = g.Audit.Record(ctx, audit.Entry{
		ActorID:    userID,
		Action:     action,
		TargetType: "user",
		TargetID:   fmt.Sprint(userID),
		Result:     result,
		Context:    extra,
	})
}

// Configured reports whether Google sign-in can be offered.
func (g *GoogleAuth) Configured() bool {
	return g.Client != nil && g.Client.Configured()
}

// randomToken returns n random bytes, base64url encoded (no padding).
func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// --- e-mail verification (SMTP is not wired; see D-039) ----------------------

// ErrVerificationToken means the token is unknown, expired or already used.
var ErrVerificationToken = errors.New("auth: verification link is invalid or expired")

// EmailSender delivers one message. Declared here (not imported from httpapi)
// so the auth package stays free of a dependency on the HTTP layer and the
// verifier is testable with a fake. Nil, or a sender whose Configured reports
// false, means "no relay": callers then report `outbox` instead of a delivery.
type EmailSender interface {
	Configured() bool
	Send(ctx context.Context, to, subject, textBody, htmlBody string) error
}

// EmailVerifier issues and redeems verification tokens.
//
// Why it exists: `Register` writes email_verified = 0, but nothing could ever set
// it, so "verify your e-mail" was a promise with no implementation. The token is
// also what an operator hands over after a manual check while no SMTP server is
// configured (`cmd/accounttool -verify` does the same thing without a token).
type EmailVerifier struct {
	Store IdentityStore
	Now   func() time.Time
	// TTL bounds how long a link works; 24h is enough for a human to click it.
	TTL time.Duration
	// Mail delivers the link. Nil (or not configured) means "no relay": the token
	// is still created and the caller reports `outbox` rather than claiming a send.
	Mail EmailSender
	// BaseURL is the public origin the link points at (e.g. https://vpn.example).
	// Empty falls back to a relative path, which a loopback operator can follow.
	BaseURL string
}

// Delivery reports how a verification link can leave the server.
//
// It is a property of the server, not of one account: answering per-address
// would let the endpoint be used to test whether an e-mail is registered.
func (v *EmailVerifier) Delivery(ctx context.Context) (string, error) {
	if v != nil && v.Mail != nil && v.Mail.Configured() {
		return "smtp", nil
	}
	return "outbox", nil
}

// Issue creates a token for an account, e-mails the link, and returns the token.
//
// A send failure does NOT fail the call: the token is valid and an operator can
// still hand it over, and the caller reports the delivery honestly. Swallowing
// the error would be the bug the package comment warns about, so it is returned
// as a second signal the caller may log.
func (v *EmailVerifier) Issue(ctx context.Context, userID int64, email string) (string, error) {
	token, _, err := v.IssueAndSend(ctx, userID, email)
	return token, err
}

// IssueAndSend is Issue plus the delivery outcome, for callers that report it.
func (v *EmailVerifier) IssueAndSend(ctx context.Context, userID int64, email string) (string, string, error) {
	ttl := v.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	now := time.Now()
	if v.Now != nil {
		now = v.Now()
	}
	token, err := randomToken(32)
	if err != nil {
		return "", "outbox", err
	}
	if err := v.Store.CreateEmailVerification(ctx, store.EmailVerification{
		Token:     token,
		UserID:    userID,
		Email:     email,
		ExpiresAt: now.Add(ttl),
	}); err != nil {
		return "", "outbox", err
	}

	link := strings.TrimRight(v.BaseURL, "/") + "/api/auth/verify/" + token
	if v.Mail == nil || !v.Mail.Configured() {
		return token, "outbox", nil
	}
	subject := "Veylix — confirm your e-mail address"
	text := "Confirm your Veylix e-mail address by opening this link:\n\n" + link + "\n\n" +
		"The link works once and expires in 24 hours.\n" +
		"If you did not create a Veylix account, ignore this message.\n"
	html := `<p>Confirm your Veylix e-mail address:</p>` +
		`<p><a href="` + link + `">Confirm my e-mail</a></p>` +
		`<p>The link works once and expires in 24 hours.</p>` +
		`<p>If you did not create a Veylix account, ignore this message.</p>`
	if err := v.Mail.Send(ctx, email, subject, text, html); err != nil {
		// The token exists and is usable; only the delivery failed. Reported, not
		// hidden — a lost link is a person who cannot sign in.
		return token, "outbox", err
	}
	return token, "smtp", nil
}

// Redeem marks the address verified. A replayed token is refused.
func (v *EmailVerifier) Redeem(ctx context.Context, token string) (int64, error) {
	rec, err := v.Store.ConsumeEmailVerification(ctx, token)
	if err != nil {
		return 0, ErrVerificationToken
	}
	if err := v.Store.MarkEmailVerified(ctx, rec.UserID); err != nil {
		return 0, err
	}
	return rec.UserID, nil
}
