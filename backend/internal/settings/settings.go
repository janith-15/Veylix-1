// Package settings stores operator-editable values that used to be environment
// only, and applies them live.
//
// The first such value is the Google OAuth client (D-080). It answers the
// operator's question "where do I change the Google login now?" from inside the
// product instead of via SSH + an env edit + a restart. The rules are the panel
// token's rules: a secret is sealed at rest (AES-256-GCM, D-016) and is never
// handed back by a read.
package settings

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/config"
	"veylix/backend/internal/store"
)

// Storage keys. Constants so a typo cannot write a value no reader ever looks at.
const (
	keyGoogleClientID     = "google.client_id"
	keyGoogleClientSecret = "google.client_secret"
	keyGoogleRedirectURL  = "google.redirect_url"
)

// GoogleCallbackPath is the single path the Google sign-in callback is served at
// (`internal/httpapi/google_routes.go`). A redirect URL that does not end here
// answers nowhere, so Google returns `redirect_uri_mismatch` and the whole
// feature is dead. It is refused at save time rather than after a round trip to
// Google. Exported so the HTTP layer can build the exact URL the operator must
// register instead of keeping a second copy of the string.
const GoogleCallbackPath = "/api/auth/google/callback"

// Store is the persistence the service needs, kept narrow so a test can supply a
// fake without a database.
type Store interface {
	GetAppSetting(ctx context.Context, key string) (store.AppSetting, bool, error)
	SetAppSetting(ctx context.Context, key string, value []byte, isSecret bool, updatedBy int64) error
}

// ResolvedGoogle is the effective Google client after the database/environment
// merge. The secret is plaintext IN MEMORY only; it is never returned by HTTP.
type ResolvedGoogle struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Configured reports whether enough is set to run the Google flow at all.
func (r ResolvedGoogle) Configured() bool {
	return looksLikeClientID(r.ClientID) && looksLikeClientSecret(r.ClientSecret) && r.RedirectURL != ""
}

// GoogleView is the safe, masked form returned to the UI: never the secret.
type GoogleView struct {
	ClientID    string `json:"client_id"`
	HasSecret   bool   `json:"has_secret"`
	RedirectURL string `json:"redirect_url"`
	Configured  bool   `json:"configured"`
	Source      string `json:"source"` // "database" | "environment" | "none"
}

// GoogleInput is what the UI submits. An empty ClientSecret means "keep the
// stored one", so an operator can change the client id without re-typing a
// secret they can no longer read.
type GoogleInput struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// ValidationError is a 400-class error the route maps to a clean message.
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

// NewValidationError builds a ValidationError. Exported so a route test (a
// sibling package) can produce one without reaching into the field.
func NewValidationError(msg string) *ValidationError { return &ValidationError{msg: msg} }

// Service reads and writes settings, sealing secrets and auditing every change.
type Service struct {
	store  Store
	sealer *auth.Sealer
	audit  *audit.Logger
	env    config.GoogleOAuth

	// OnGoogleChange, when set, runs after a successful save so the running
	// process can rebuild the Google client without a restart.
	OnGoogleChange func(ResolvedGoogle)
}

// New builds the service. env is the environment fallback, so an installation
// that only ever set VEYLIX_GOOGLE_* keeps working unchanged.
func New(st Store, sealer *auth.Sealer, aud *audit.Logger, env config.GoogleOAuth) *Service {
	return &Service{store: st, sealer: sealer, audit: aud, env: env}
}

// aad binds a sealed value to its key, so a ciphertext copied onto another row
// fails to open.
func aad(key string) []byte { return []byte("app_settings:" + key) }

// get reads one key, decrypting when the row is a secret. (value, found, err).
func (s *Service) get(ctx context.Context, key string) (string, bool, error) {
	row, ok, err := s.store.GetAppSetting(ctx, key)
	if err != nil || !ok {
		return "", false, err
	}
	if !row.IsSecret {
		return string(row.ValueCiphertext), true, nil
	}
	if s.sealer == nil {
		return "", true, errors.New("settings: this installation has no encryption key; cannot read a sealed setting")
	}
	plain, err := s.sealer.Open(row.ValueCiphertext, aad(key))
	if err != nil {
		return "", true, fmt.Errorf("settings: cannot decrypt %s with the current key", key)
	}
	return string(plain), true, nil
}

// Resolve merges the database over the environment. A value present in the
// database wins even when the environment also sets it, because the operator's
// in-app edit is the more recent intent.
func (s *Service) Resolve(ctx context.Context) (ResolvedGoogle, error) {
	out := ResolvedGoogle{
		ClientID:     strings.TrimSpace(s.env.ClientID),
		ClientSecret: strings.TrimSpace(s.env.ClientSecret),
		RedirectURL:  strings.TrimSpace(s.env.RedirectURL),
	}
	if v, ok, err := s.get(ctx, keyGoogleClientID); err != nil {
		return out, err
	} else if ok {
		out.ClientID = strings.TrimSpace(v)
	}
	if v, ok, err := s.get(ctx, keyGoogleClientSecret); err != nil {
		return out, err
	} else if ok {
		out.ClientSecret = strings.TrimSpace(v)
	}
	if v, ok, err := s.get(ctx, keyGoogleRedirectURL); err != nil {
		return out, err
	} else if ok {
		out.RedirectURL = strings.TrimSpace(v)
	}
	return out, nil
}

// source reports where the values came from, for the settings page to show an
// honest "edited here" vs "from the environment" label.
func (s *Service) source(ctx context.Context) string {
	if _, ok, _ := s.get(ctx, keyGoogleRedirectURL); ok {
		return "database"
	}
	if s.env.RedirectURL != "" || s.env.ClientID != "" {
		return "environment"
	}
	return "none"
}

// View returns the masked settings for the UI.
func (s *Service) View(ctx context.Context) (GoogleView, error) {
	r, err := s.Resolve(ctx)
	if err != nil {
		return GoogleView{}, err
	}
	return GoogleView{
		ClientID:    r.ClientID,
		HasSecret:   r.ClientSecret != "",
		RedirectURL: r.RedirectURL,
		Configured:  r.Configured(),
		Source:      s.source(ctx),
	}, nil
}

// Save validates and persists the Google client. An empty ClientSecret keeps the
// existing one. Every change is audited; the secret never enters the audit.
func (s *Service) Save(ctx context.Context, in GoogleInput, actorID int64) (GoogleView, error) {
	clientID := strings.TrimSpace(in.ClientID)
	redirect := strings.TrimSpace(in.RedirectURL)
	secret := strings.TrimSpace(in.ClientSecret)

	if clientID != "" && !looksLikeClientID(clientID) {
		return GoogleView{}, &ValidationError{"client_id must end in .apps.googleusercontent.com"}
	}
	if secret != "" && !looksLikeClientSecret(secret) {
		return GoogleView{}, &ValidationError{"client_secret must start with GOCSPX-"}
	}
	if redirect != "" {
		if err := validateRedirect(redirect); err != nil {
			return GoogleView{}, err
		}
	}
	// Enabling the feature needs all three. Allowing a partial save would leave
	// the page reporting "configured" for a set that cannot sign anyone in.
	if clientID != "" && redirect != "" && secret == "" {
		if _, ok, _ := s.get(ctx, keyGoogleClientSecret); !ok && strings.TrimSpace(s.env.ClientSecret) == "" {
			return GoogleView{}, &ValidationError{"a client_secret is required the first time you configure Google sign-in"}
		}
	}

	if err := s.store.SetAppSetting(ctx, keyGoogleClientID, []byte(clientID), false, actorID); err != nil {
		return GoogleView{}, err
	}
	if err := s.store.SetAppSetting(ctx, keyGoogleRedirectURL, []byte(redirect), false, actorID); err != nil {
		return GoogleView{}, err
	}
	if secret != "" {
		if s.sealer == nil {
			return GoogleView{}, errors.New("settings: no encryption key; cannot store a secret")
		}
		sealed, err := s.sealer.Seal([]byte(secret), aad(keyGoogleClientSecret))
		if err != nil {
			return GoogleView{}, err
		}
		if err := s.store.SetAppSetting(ctx, keyGoogleClientSecret, sealed, true, actorID); err != nil {
			return GoogleView{}, err
		}
	}

	if s.audit != nil {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     audit.ActionSettingsUpdate,
			TargetType: "setting",
			TargetID:   "google",
			Result:     "OK",
			Context: map[string]any{
				"client_id":  clientID,
				"redirect":   redirect,
				"secret_set": secret != "",
			},
		})
	}

	resolved, err := s.Resolve(ctx)
	if err != nil {
		return GoogleView{}, err
	}
	if s.OnGoogleChange != nil {
		s.OnGoogleChange(resolved)
	}
	return s.View(ctx)
}

func looksLikeClientID(id string) bool {
	return strings.HasSuffix(strings.ToLower(id), ".apps.googleusercontent.com")
}

func looksLikeClientSecret(s string) bool { return strings.HasPrefix(s, "GOCSPX-") }

func validateRedirect(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return &ValidationError{"redirect_url must be an absolute URL registered in Google Cloud Console"}
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return &ValidationError{"redirect_url must use https (or http for local development)"}
	}
	// The callback lives at exactly one path. A URL that ends anywhere else is a
	// guaranteed `redirect_uri_mismatch` — and that error reads as "Google is
	// broken", not "the path is wrong". A leading sub-path is allowed (a reverse
	// proxy that rewrites `/veylix/...` still ends at the callback), but the tail
	// must be the one route Veylix answers.
	if !strings.HasSuffix(u.Path, GoogleCallbackPath) {
		return &ValidationError{"redirect_url must end with " + GoogleCallbackPath}
	}
	return nil
}
