package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"veylix/backend/internal/auth"
)

// principalKey is the context key for the authenticated actor.
type principalKey struct{}

// WithPrincipal stores the actor on the request context.
func WithPrincipal(ctx context.Context, p auth.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the actor, or false when the request is anonymous.
func PrincipalFrom(ctx context.Context) (auth.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(auth.Principal)
	return p, ok
}

// Authenticator resolves a raw session token to a principal.
type Authenticator interface {
	Lookup(ctx context.Context, token string) (auth.Principal, error)
}

// RequireAuth rejects anonymous requests and injects the principal.
//
// Authorization is decided here and in the service layer — never by the client
// (REQUIREMENTS §4).
func RequireAuth(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := auth.TokenFromRequest(r)
			p, err := a.Lookup(r.Context(), token)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// RequirePermission enforces one capability from the role matrix.
func RequirePermission(perm auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			if !p.Can(perm) {
				WriteError(w, http.StatusForbidden, "forbidden", "you do not have permission for this action")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SameOrigin rejects state-changing requests from another origin (CSRF).
//
// Three cases are distinguished, because "no Origin header" is not one thing:
//
//  1. `Origin` present      → must match an allowed origin, else 403.
//  2. No `Origin`, but the request is shaped like a browser fetch
//     (`Sec-Fetch-Site`, `Sec-Fetch-Mode`, or a `Referer`) → 403. Every browser
//     sends `Sec-Fetch-*` on subresource/mutation requests, so a browser POST
//     cannot slip through by omitting `Origin`. This closes "form CSRF" against
//     routes that a cross-site form can reach.
//  3. Neither → a non-browser API client (curl, the test harnesses, the CLI
//     tools). Allowed: it carries no ambient cookie session by default, and the
//     session cookie is `HttpOnly` + host-scoped, so it cannot be borrowed.
//
// Allowed origins come from the request itself (`scheme://Host`) plus any
// explicitly configured public origin, so a reverse proxy does not have to
// rewrite headers for the app to work.
func SameOrigin(extraOrigins ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(extraOrigins)+1)
	for _, o := range extraOrigins {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			allowed[strings.ToLower(o)] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				if looksLikeBrowser(r) {
					WriteError(w, http.StatusForbidden, "csrf_blocked",
						"cross-site request blocked")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if !originAllowed(strings.ToLower(strings.TrimRight(origin, "/")), schemeHost(r), allowed) {
				WriteError(w, http.StatusForbidden, "csrf_blocked", "cross-origin request blocked")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// originAllowed accepts the request's own origin or any configured public one.
func originAllowed(origin, self string, allowed map[string]struct{}) bool {
	origin = strings.ToLower(strings.TrimRight(origin, "/"))
	if _, ok := allowed[origin]; ok {
		return true
	}
	return origin == strings.ToLower(strings.TrimRight(self, "/"))
}

// looksLikeBrowser reports whether the request carries browser fetch metadata.
//
// `Sec-Fetch-Site` and `Sec-Fetch-Mode` are set by browsers on navigations and
// fetches and cannot be suppressed by page script, which makes them a reliable
// "this came from a browser" signal — and therefore a reliable reason to demand
// an `Origin` header.
func looksLikeBrowser(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Sec-Fetch-Mode") != "" {
		return true
	}
	return r.Header.Get("Referer") != ""
}

func schemeHost(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && !isForwardedHTTPS(r) {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// isForwardedHTTPS honours X-Forwarded-Proto only from a trusted (loopback) peer.
func isForwardedHTTPS(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-Proto") != "https" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// LogRequests writes one line per request to the standard logger.
//
// Why the backend needed this: apart from "server listening" there was **no**
// access log, so a production question like "who called this route at 14:02 and
// what did it answer?" had no answer in the logs at all. The audit trail records
// BUSINESS writes; it does not record reads, refusals, or 500s.
//
// What it deliberately does NOT log:
//   - the query string, because password-reset and e-mail-verification TOKENS
//     travel there (`/api/auth/verify/<token>`) and a log is a file that gets
//     copied, tailed and shipped;
//   - the request or response body, which may carry credentials;
//   - the caller's identity, which the audit trail already records for writes and
//     which would otherwise duplicate a personal identifier into a second place.
//
// A 5xx is logged at ERROR, a 4xx at WARN and everything else at INFO, so a
// healthy log stays quiet and a failure is not buried.
func LogRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets are noise: one page load is a dozen files.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		level := slog.LevelInfo
		switch {
		case sw.status >= 500:
			level = slog.LevelError
		case sw.status >= 400:
			level = slog.LevelWarn
		}
		slog.Log(r.Context(), level, "http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

// statusWriter records the status and size a handler produced.
//
// It defaults to 200 because a handler that never calls WriteHeader implies 200,
// and reporting 0 for those would make the commonest response look like a failure.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer, which is what
// streaming (SSE, file copy) needs. Without it, wrapping silently breaks Flush.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteAuthError maps service errors to HTTP codes without leaking internals.
func WriteAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, auth.ErrForbidden):
		WriteError(w, http.StatusForbidden, "forbidden", "action not permitted")
	case errors.Is(err, auth.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, auth.ErrInvalidCredentials):
		WriteError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
	case errors.Is(err, auth.ErrPasswordNotSet):
		// The account exists but was created through Google, so there is no
		// password to compare. Saying "wrong password" here sends the customer to a
		// reset flow for a credential that was never set — a permanent dead end.
		WriteError(w, http.StatusForbidden, "password_not_set",
			"this account was created with Google — use \"Continue with Google\" to sign in, or ask the seller to issue you a password from the console")
	case errors.Is(err, auth.ErrEmailNotVerified):
		// It is a LINK, not a code: the verifier e-mails
		// /api/auth/verify/<token>. Claiming otherwise sends the user hunting
		// for six digits that do not exist.
		WriteError(w, http.StatusForbidden, "email_unverified",
			"confirm your e-mail address first — open the link we e-mailed you")
	case errors.Is(err, auth.ErrWrongPortal):
		WriteError(w, http.StatusForbidden, "wrong_portal",
			"this account signs in through the other portal")
	case errors.Is(err, auth.ErrRateLimited):
		// Tell the caller how long to wait instead of leaving them refreshing
		// blindly. The seconds travel on the error so concurrent requests do not
		// overwrite each other's value.
		var rl *auth.RateLimitedError
		if errors.As(err, &rl) && rl.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.RetryAfter.Seconds()+0.999)))
		}
		WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
	case errors.Is(err, auth.ErrEmailTaken):
		WriteError(w, http.StatusConflict, "email_taken", "that email is already registered")
	case errors.Is(err, auth.ErrWeakPassword):
		WriteError(w, http.StatusUnprocessableEntity, "weak_password", err.Error())
	case errors.Is(err, auth.ErrPasswordRepeated),
		errors.Is(err, auth.ErrPasswordSequence),
		errors.Is(err, auth.ErrPasswordCommon):
		// The policy sentinels carry a user-safe rule message and never the
		// password itself.
		WriteError(w, http.StatusUnprocessableEntity, "weak_password", err.Error())
	case errors.Is(err, auth.ErrPasswordMismatch):
		WriteError(w, http.StatusUnprocessableEntity, "password_mismatch", "current password is incorrect")
	case errors.Is(err, auth.ErrPasswordTooLong):
		WriteError(w, http.StatusUnprocessableEntity, "password_too_long", err.Error())
	case errors.Is(err, auth.ErrBootstrapNotAllowed):
		WriteError(w, http.StatusConflict, "bootstrap_complete", "setup has already been completed")
	case errors.Is(err, auth.ErrUserNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "account not found")
	case errors.Is(err, auth.ErrCannotModifySelf):
		WriteError(w, http.StatusUnprocessableEntity, "cannot_modify_self", err.Error())
	case errors.Is(err, auth.ErrSuperAdminExists):
		// P-1: the seat is taken. 409 rather than 422 because the request itself
		// is well-formed — the platform's state is what refuses it, and the
		// operator must change that state (demote the current owner) first.
		WriteError(w, http.StatusConflict, "super_admin_exists", err.Error())
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", "unexpected server error")
	}
}
