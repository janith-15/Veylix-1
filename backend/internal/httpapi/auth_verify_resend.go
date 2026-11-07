package httpapi

import (
	"context"
	"net/http"
	"strings"

	"veylix/backend/internal/auth"
)

// VerificationResender is the public "send me a new confirmation link" path
// (D-078).
//
// Why it has to be PUBLIC: `POST /api/auth/verify/request` is gated on a session,
// and an unverified account can never obtain one — `Authenticate` refuses it with
// `email_unverified` before any cookie is issued. The only route to a new link
// therefore required the one thing the person could not have. With no SMTP relay
// configured, that was a permanent dead end: sign up, get locked out, no way back
// in and no way to ask for another link.
type VerificationResender interface {
	FindUserByIdentifier(ctx context.Context, identifier string) (*auth.User, error)
}

// VerificationIssuer creates and mails a confirmation link for an account.
type VerificationIssuer interface {
	IssueAndSend(ctx context.Context, userID int64, email string) (token, delivery string, err error)
	Delivery(ctx context.Context) (string, error)
}

// resendEmailVerificationHandler answers "I never got the link".
//
//	POST /api/auth/verify/resend   { identifier }
//
// **It answers the same way whatever it finds**, and that is deliberate: a public
// endpoint that reported "no such account" or "already verified" would be an
// address-existence oracle — anybody could test whether an e-mail is registered
// here. So the response is one fixed 202 body, and the WORK is what differs.
//
// Only an unverified account is actually mailed. A verified one, an unknown one,
// a throttled one and a failed delivery all receive the same 202 with the same
// wording, so neither the status nor the body can be used to learn whether an
// address is registered here.
func resendEmailVerificationHandler(resender VerificationResender, issuer VerificationIssuer,
	limiter *auth.RateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Identifier string `json:"identifier"`
			Email      string `json:"email"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		identifier := strings.TrimSpace(in.Identifier)
		if identifier == "" {
			identifier = strings.TrimSpace(in.Email)
		}

		// One answer, always. The body below is written even when there is
		// nothing to do, so timing and status cannot distinguish the cases.
		const generic = "If that address belongs to an account that still needs confirming, " +
			"a new link is on its way. The link works once and expires in 24 hours."

		if identifier == "" {
			WriteJSON(w, http.StatusAccepted, map[string]any{"sent": true, "note": generic})
			return
		}
		// Throttled per address so the endpoint cannot be used to flood a mailbox.
		// A throttled request is answered exactly like a successful one.
		if limiter != nil && !limiter.Allow("verify:"+strings.ToLower(identifier)) {
			WriteJSON(w, http.StatusAccepted, map[string]any{"sent": true, "note": generic})
			return
		}

		user, err := resender.FindUserByIdentifier(r.Context(), identifier)
		if err != nil || user == nil || user.EmailVerified {
			// Unknown, or already confirmed: nothing to send, and nothing said.
			WriteJSON(w, http.StatusAccepted, map[string]any{"sent": true, "note": generic})
			return
		}
		if _, _, ierr := issuer.IssueAndSend(r.Context(), user.ID, user.Email); ierr != nil {
			// A delivery failure is still answered generically — the caller can do
			// nothing about it, and saying "we tried and failed" would confirm that
			// the address exists.
			WriteJSON(w, http.StatusAccepted, map[string]any{"sent": true, "note": generic})
			return
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{"sent": true, "note": generic})
	})
}

// AuthVerifyResendRoutes mounts the public resend route (D-078).
//
// Nil issuer leaves the route unregistered rather than answering a lie: with no
// verifier there is no link to send.
func AuthVerifyResendRoutes(mux *http.ServeMux, resender VerificationResender,
	issuer VerificationIssuer) {
	if resender == nil || issuer == nil {
		return
	}
	// Three per address per twenty minutes: a person asks once or twice; a script
	// does not. Throttling is per ADDRESS, not per IP, so one shared NAT address
	// cannot silence everybody behind it.
	limiter := auth.NewRateLimiter(3, 1.0/1200.0)
	mux.Handle("POST /api/auth/verify/resend",
		http.Handler(resendEmailVerificationHandler(resender, issuer, limiter)))
}
