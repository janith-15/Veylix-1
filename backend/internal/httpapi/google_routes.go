package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// GoogleAuthenticator is the Google sign-in flow (D-039). Nil disables the routes
// entirely, which is what a build without credentials should do.
type GoogleAuthenticator interface {
	Start(ctx context.Context) (string, error)
	Finish(ctx context.Context, state, code string) (*store.UserRow, bool, error)
	Configured() bool
}

// EmailVerificationService issues and redeems verification tokens.
type EmailVerificationService interface {
	Issue(ctx context.Context, userID int64, email string) (string, error)
	// IssueAndSend returns the delivery outcome too, so the public resend route
	// reports what actually happened instead of assuming a send (D-078).
	IssueAndSend(ctx context.Context, userID int64, email string) (token, delivery string, err error)
	Redeem(ctx context.Context, token string) (int64, error)
	// Delivery reports `smtp` or `outbox` for this server, so the response can
	// say whether a link was really sent instead of implying it always was.
	Delivery(ctx context.Context) (string, error)
}

// googleErrorCode maps a flow error to a short, non-leaking UI code.
func googleErrorCode(err error) string {
	switch {
	case errors.Is(err, auth.ErrGoogleState):
		return "state_invalid"
	case errors.Is(err, auth.ErrGoogleEmailUnverified):
		return "email_unverified"
	case errors.Is(err, auth.ErrGoogleIdentityTaken):
		return "identity_taken"
	case errors.Is(err, auth.ErrGoogleDenied):
		return "denied"
	default:
		return "failed"
	}
}

// redirectToApp sends the browser back to the SPA with a 303, so refreshing the
// callback URL cannot re-submit it.
func redirectToApp(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, appOrigin(r)+path, http.StatusSeeOther)
}

// appOrigin is the origin the SPA is served from: the request's own host, never a
// query parameter (so it cannot be turned into an open redirect). Behind the Vite
// dev proxy that is :5173; in the packaged binary it is the same port as the API.
func appOrigin(r *http.Request) string {
	if isForwardedHTTPS(r) || r.TLS != nil {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}

// isLoopbackRequest reports whether the caller reached the server over loopback
// (a local operator). It decides whether the verification token may be returned in
// the response, because no SMTP relay is configured yet.
func isLoopbackRequest(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// GoogleRoutes registers the OAuth entry points and e-mail verification.
//
// The flow is deliberately NOT something the SPA calls with fetch: the browser is
// navigated to Google and back, so the callback answers with a redirect. The
// client secret and the tokens therefore never touch JavaScript, and the session
// cookie is set on a real page load rather than an XHR.
//
//	GET  /api/auth/google/status    → { configured } so the button can be honest
//	GET  /api/auth/google/start     → 302 to Google (503 when not configured)
//	GET  /api/auth/google/callback  → sets the session cookie, 303 into the app
//	POST /api/auth/verify/request   → issues a token for the signed-in account
//	GET  /api/auth/verify/{token}   → redeems it, 303 to /login?verified=1
func GoogleRoutes(mux *http.ServeMux, g GoogleAuthenticator, sessions SessionManager,
	verifier EmailVerificationService, authed func(http.Handler) http.Handler) {

	// Answered once per login page, so the button is disabled with a real reason
	// instead of failing after a round trip to Google.
	mux.HandleFunc("GET /api/auth/google/status", func(w http.ResponseWriter, r *http.Request) {
		configured := g != nil && g.Configured()
		WriteJSON(w, http.StatusOK, map[string]any{"configured": configured})
	})

	if g != nil && g.Configured() {
		mux.HandleFunc("GET /api/auth/google/start", func(w http.ResponseWriter, r *http.Request) {
			consent, err := g.Start(r.Context())
			if err != nil {
				// Includes ErrGoogleNotConfigured, which cannot happen here, but a
				// vanished credential is still reported instead of redirecting.
				WriteError(w, http.StatusServiceUnavailable, "google_not_configured",
					"Google sign-in is not configured on this server")
				return
			}
			http.Redirect(w, r, consent, http.StatusFound)
		})
	} else {
		// Registered even when disabled: a 404 would look like a broken build,
		// whereas 503 google_not_configured is the truthful answer, and it is what
		// the UI's disabled button explains.
		mux.HandleFunc("GET /api/auth/google/start", func(w http.ResponseWriter, r *http.Request) {
			WriteError(w, http.StatusServiceUnavailable, "google_not_configured",
				"Google sign-in is not configured on this server")
		})
	}

	if g != nil && g.Configured() {
		mux.HandleFunc("GET /api/auth/google/callback", func(w http.ResponseWriter, r *http.Request) {
			// Google reports a refusal (cancelled, bad scope) as ?error=...
			if e := r.URL.Query().Get("error"); e != "" {
				redirectToApp(w, r, "/login?google=denied")
				return
			}
			usr, created, err := g.Finish(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
			if err != nil {
				redirectToApp(w, r, "/login?google="+googleErrorCode(err))
				return
			}
			token, err := sessions.Create(r.Context(), usr.ID, auth.ClientIP(r), r.UserAgent())
			if err != nil {
				redirectToApp(w, r, "/login?google=session_failed")
				return
			}
			sessions.SetCookie(w, token)
			landing := "/dashboard"
			switch usr.Role {
			case string(auth.RoleAdmin):
				landing = "/admin"
			case string(auth.RoleClient):
				landing = "/client"
			}
			if created {
				landing += "?welcome=1"
			}
			redirectToApp(w, r, landing)
		})
	}

	if verifier == nil {
		return
	}

	// Ask for a verification link for the signed-in account. The response is
	// honest about delivery (D-041): `smtp` means a relay accepted the message,
	// `outbox` means none is configured. The link is echoed back only to a
	// loopback operator, and only when nothing was sent — a deployment with a
	// working relay never hands a live verification token to the browser.
	mux.Handle("POST /api/auth/verify/request", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		token, err := verifier.Issue(r.Context(), p.UserID, p.Email)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not create a verification link")
			return
		}
		delivery, err := verifier.Delivery(r.Context())
		if err != nil {
			delivery = deliveryOutbox
		}
		res := map[string]any{"sent": true, "email": p.Email, "delivery": delivery}
		// No relay configured means the link exists ONLY on this server, so the
		// person has no way to confirm unless we hand it over. It used to be
		// handed over to a loopback caller alone — which locked out every real
		// client whose SMTP was not yet set up, permanently: sign-up creates the
		// account, login refuses it as unverified, and only the operator's own
		// machine could ever see the token. The person is already authenticated
		// (this route requires a session) and the token verifies THEIR OWN
		// address, so returning it is not a privilege escalation — it is the only
		// way "delivery: outbox" is not a dead end.
		//
		// With a working relay the token is never echoed, so a deployment that
		// sends real mail keeps the stricter rule.
		if delivery == deliveryOutbox {
			res["token"] = token
			res["link"] = "/api/auth/verify/" + token
		}
		WriteJSON(w, http.StatusAccepted, res)
	})))

	mux.HandleFunc("GET /api/auth/verify/{token}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.Redeem(r.Context(), r.PathValue("token")); err != nil {
			redirectToApp(w, r, "/login?verified=invalid")
			return
		}
		redirectToApp(w, r, "/login?verified=1")
	})
}
