package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// AuthService is the auth surface the handlers need.
type AuthService interface {
	Register(ctx context.Context, in auth.RegisterInput) (*auth.User, error)
	Authenticate(ctx context.Context, identifier, password, ip string, portal auth.Portal) (*auth.User, error)
	BootstrapSuperAdmin(ctx context.Context, email, password, displayName string) (*auth.User, error)
	ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error
	// VerifyPassword is the re-authentication check for a sensitive self-service
	// change (an e-mail move), so a stolen session alone is not enough.
	VerifyPassword(ctx context.Context, userID int64, password string) bool
	// FindUserByIdentifier resolves an e-mail or username for password reset. It
	// returns (nil, nil) when nothing matches, so the reset route can answer the
	// same way either way (D-041).
	FindUserByIdentifier(ctx context.Context, identifier string) (*auth.User, error)
	// ResetStaffPassword sets a new password for another account and ends its
	// sessions. Used by the staff console AND by the Super Admin's client console:
	// a customer whose mailbox is wrong or gone has no other way back in.
	ResetStaffPassword(ctx context.Context, actor auth.Principal, userID int64, newPassword string) error
}

// SessionManager issues and revokes sessions.
type SessionManager interface {
	Create(ctx context.Context, userID int64, ip, userAgent string) (string, error)
	Revoke(ctx context.Context, token string) error
	RevokeAllForUser(ctx context.Context, userID int64) error
	// RevokeOthersForUser ends every session except keepToken and returns how
	// many it ended (D3). Nil-free: an empty keepToken ends nothing.
	RevokeOthersForUser(ctx context.Context, userID int64, keepToken string) (int64, error)
	SetCookie(w http.ResponseWriter, token string)
	ClearCookie(w http.ResponseWriter)
}

// SetupStateReader reports whether the installation still needs its first
// super admin. It is a separate, dependency-free interface so the router can
// expose GET /api/setup/status without mounting the whole auth stack.
type SetupStateReader interface {
	BootstrapRequired(ctx context.Context) (bool, error)
}

// AccountReader reads the safe projection of one account, for GET /api/me (D1).
//
// It is deliberately narrow — one method, no password hash, no session — so the
// self-description route cannot be widened into an account dump by accident.
// A nil reader falls back to the principal carried by the session, which is what
// this route answered before D1: the screen then shows the e-mail prefix instead
// of the display name, never a wrong account.
type AccountReader interface {
	UserByID(ctx context.Context, id int64) (*store.UserRow, error)
}

// SetupStatusRoutes registers GET /api/setup/status.
//
// Public on purpose: an unauthenticated visitor must be told whether to see the
// setup wizard or the login form. It leaks exactly one bit ("this install has
// zero accounts") and nothing about existing accounts.
func SetupStatusRoutes(mux *http.ServeMux, state SetupStateReader) {
	mux.HandleFunc("GET /api/setup/status", func(w http.ResponseWriter, r *http.Request) {
		required, err := state.BootstrapRequired(r.Context())
		if err != nil {
			WriteError(w, http.StatusServiceUnavailable, "internal_error", "could not read setup state")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"bootstrap_required": required})
	})
}

// notifyPasswordChanged e-mails the account holder that the password changed.
//
// It returns the delivery outcome (`smtp` or `outbox`) so the response states the
// truth instead of implying a message went out (D-041). The message names the
// time and the address that made the change — never the password itself, and
// never a token: this is a warning, not an action.
//
// With no relay configured nothing is sent, and the UI reports `outbox` so the
// person knows to watch their account by other means.
func notifyPasswordChanged(r *http.Request, mail EmailSender, p auth.Principal) string {
	if mail == nil || !mail.Configured() || p.Email == "" {
		return "outbox"
	}
	when := time.Now().UTC().Format("2006-01-02 15:04 UTC")
	ip := auth.ClientIP(r)
	subject := "Veylix — your password was changed"
	text := "Your Veylix password was changed.\n\n" +
		"When: " + when + "\n" +
		"From: " + ip + "\n\n" +
		"If you did not do this, someone may have access to your account.\n" +
		"Reset your password immediately at /forgot and contact support.\n"
	html := `<p>Your Veylix password was changed.</p>` +
		`<p><strong>When:</strong> ` + when + `<br><strong>From:</strong> ` + ip + `</p>` +
		`<p>If you did not do this, someone may have access to your account. ` +
		`Reset your password immediately at <code>/forgot</code> and contact support.</p>`
	if err := mail.Send(r.Context(), p.Email, subject, text, html); err != nil {
		// A failed send must not fail the request: the password is already
		// changed and telling the user otherwise would be a lie.
		slog.Warn("password change notification failed", "err", err)
		return "outbox"
	}
	return "smtp"
}

// AuthRoutes registers the /api/auth/* endpoints.
//
// The rate limiter is required, not optional: registration sends a verification
// e-mail, so an unthrottled endpoint lets one host create accounts in a loop and
// burn the relay's sending reputation (which then also blocks password resets).
//
// mail notifies the account after a password change. It is nil-safe: with no
// relay the change still succeeds and the response says `outbox`, because a
// missing notification must never block a security action (D-041).
// AuthRoutes mounts the auth surface. `displayTZ` is the zone every screen must
// render absolute instants in (D-099). It rides on /api/me because that is the
// one authenticated call every signed-in page already makes, so publishing it
// costs no extra request and cannot answer before the caller is known.
//
// It is deliberately NOT a per-user setting: two people looking at the same VPN
// must read the same clock, or a support call becomes an argument about whose
// screen is wrong.
func AuthRoutes(mux *http.ServeMux, svc AuthService, sessions SessionManager, authed func(http.Handler) http.Handler, limiter *auth.RateLimiter, mail EmailSender, accounts AccountReader, displayTZ string) {
	// One sign-up per address per minute, bursts of three. A person does not
	// register four times in a row; a script does.
	registerLimiter := limiter
	if registerLimiter == nil {
		registerLimiter = auth.NewRateLimiter(3, 1.0/60.0)
	}

	mux.HandleFunc("POST /api/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if !registerLimiter.Allow("register:" + auth.ClientIP(r)) {
			if wait := registerLimiter.RetryAfter("register:" + auth.ClientIP(r)); wait > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds()+0.999)))
			}
			WriteError(w, http.StatusTooManyRequests, "rate_limited",
				"too many sign-up attempts, try again later")
			return
		}
		var in struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		user, err := svc.Register(r.Context(), auth.RegisterInput{
			Email: in.Email, Password: in.Password, DisplayName: in.DisplayName,
		})
		if err != nil {
			WriteAuthError(w, err)
			return
		}

		// THE DOOR IS THE SAME FOR EVERYONE.
		//
		// This route used to create a session and answer 201 as if the person were
		// signed in. They were not: `Authenticate` refuses an unverified address
		// (403 email_unverified), so the cookie was the ONE thing that worked —
		// until it expired, at which point the same account could never sign in
		// again. A customer walked away believing the account was fine and was
		// locked out minutes later. That is the "email and password do not work"
		// report, and the fix is to stop pretending: no session, no cookie, and a
		// body that states exactly what happened.
		//
		// An auto-verified account (operator-created, or a test harness) is a real
		// sign-in, so it keeps the old contract.
		if !user.EmailVerified {
			WriteJSON(w, http.StatusAccepted, map[string]any{
				"pending_verification": true,
				"email":                user.Email,
				"email_verified":       false,
				"delivery":             user.VerificationDelivery,
				"note": "Confirm your e-mail address before signing in. " +
					"If the link does not arrive, request a new one from /login.",
			})
			return
		}

		token, err := sessions.Create(r.Context(), user.ID, auth.ClientIP(r), r.UserAgent())
		if err != nil {
			WriteAuthError(w, err)
			return
		}
		sessions.SetCookie(w, token)
		WriteJSON(w, http.StatusCreated, user)
	})

	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			// identifier is an e-mail address OR a username (D-041). `email` is still
			// accepted so an older client/browser keeps working.
			Identifier string `json:"identifier"`
			Email      string `json:"email"`
			Password   string `json:"password"`
			// portal defaults to "client": a missing field must never unlock staff.
			Portal string `json:"portal"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		identifier := in.Identifier
		if identifier == "" {
			identifier = in.Email
		}
		portal := auth.Portal(strings.ToLower(strings.TrimSpace(in.Portal)))
		if portal == "" {
			portal = auth.PortalClient
		}
		user, err := svc.Authenticate(r.Context(), identifier, in.Password, auth.ClientIP(r), portal)
		if err != nil {
			WriteAuthError(w, err)
			return
		}
		token, err := sessions.Create(r.Context(), user.ID, auth.ClientIP(r), r.UserAgent())
		if err != nil {
			WriteAuthError(w, err)
			return
		}
		sessions.SetCookie(w, token)
		WriteJSON(w, http.StatusOK, user)
	})

	mux.Handle("POST /api/auth/logout", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = sessions.Revoke(r.Context(), auth.TokenFromRequest(r))
		sessions.ClearCookie(w)
		w.WriteHeader(http.StatusNoContent)
	})))

	// D3: "sign out other devices". The caller's own session survives — the point
	// is to evict a device you no longer hold, not to sign yourself out. The
	// session id comes from the cookie, never from the body: a body-supplied
	// token would let one account keep another account's session alive.
	mux.Handle("POST /api/auth/logout-others", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		ended, err := sessions.RevokeOthersForUser(r.Context(), p.UserID, auth.TokenFromRequest(r))
		if err != nil {
			slog.Error("httpapi: revoke other sessions", "user_id", p.UserID, "err", err)
			WriteError(w, http.StatusInternalServerError, "sessions_not_revoked",
				"the other sessions could not be ended; nothing was changed")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"ended": ended})
	})))

	mux.Handle("GET /api/me", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		// The identity always comes from the session; the body only adds what the
		// session does not carry.
		body := map[string]any{
			"id": p.UserID, "email": p.Email, "role": string(p.Role),
		}
		// D-099: the zone every screen must render absolute instants in.
		//
		// Sent here rather than from a new endpoint because every signed-in page
		// already calls /api/me, so the UI gets the fact with no extra request
		// and no chance of rendering a timestamp before the zone arrives.
		//
		// It is the zone NAME ("Asia/Colombo"), not an offset: a browser given
		// "+05:30" would keep showing the wrong hour if the zone's rules ever
		// changed, and an offset is exactly the thing that drifts.
		if displayTZ != "" {
			body["display_timezone"] = displayTZ
		}
		// D1: without this the screen shows the e-mail prefix as the user's name
		// after every reload. `display_name` is read from the account row, never
		// from the request. A failed read is NOT an auth failure — the caller is
		// demonstrably signed in — so the route still answers 200 with the
		// session's own facts and says the read failed.
		if accounts != nil {
			row, err := accounts.UserByID(r.Context(), p.UserID)
			switch {
			case err != nil:
				slog.Warn("httpapi: /api/me could not read the account row",
					"user_id", p.UserID, "err", err)
				body["profile_read_error"] = true
			case row != nil:
				body["display_name"] = row.DisplayName
				body["email_verified"] = row.EmailVerified
				body["created_at"] = row.CreatedAt
			}
		}
		WriteJSON(w, http.StatusOK, body)
	})))

	// First-run setup. Refused once any user exists (no default password).
	//
	// On success the new super admin is signed in immediately (same session
	// contract as register/login): leaving them logged out would send the
	// operator to a login form with a credential they just chose, and the setup
	// wizard would appear to have failed.
	mux.HandleFunc("POST /api/setup/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		user, err := svc.BootstrapSuperAdmin(r.Context(), in.Email, in.Password, in.DisplayName)
		if err != nil {
			WriteAuthError(w, err)
			return
		}
		token, err := sessions.Create(r.Context(), user.ID, auth.ClientIP(r), r.UserAgent())
		if err != nil {
			WriteAuthError(w, err)
			return
		}
		sessions.SetCookie(w, token)
		WriteJSON(w, http.StatusCreated, user)
	})

	// Change password. Authenticated; the caller may only change its own
	// password (the user id comes from the session, never from the body).
	//
	// Every session is revoked after a successful change, so a stolen cookie
	// cannot outlive the rotation; the caller's own cookie is re-issued so the
	// current browser stays signed in.
	mux.Handle("POST /api/auth/change-password", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var in struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if err := svc.ChangePassword(r.Context(), p.UserID, in.CurrentPassword, in.NewPassword); err != nil {
			WriteAuthError(w, err)
			return
		}

		// Rotate: drop every session, then issue a fresh one for this browser.
		if err := sessions.RevokeAllForUser(r.Context(), p.UserID); err != nil {
			WriteAuthError(w, err)
			return
		}
		token, err := sessions.Create(r.Context(), p.UserID, auth.ClientIP(r), r.UserAgent())
		if err != nil {
			WriteAuthError(w, err)
			return
		}
		sessions.SetCookie(w, token)

		// Tell the account holder. A stolen session can change the password, and
		// without this mail the real owner would never learn it happened — the
		// one signal that turns a silent takeover into a recoverable event.
		// Delivery never blocks the change: the password is already different.
		delivery := notifyPasswordChanged(r, mail, p)

		WriteJSON(w, http.StatusOK, map[string]any{
			"changed":  true,
			"delivery": delivery,
			"note":     "every other session was signed out",
		})
	})))
}
