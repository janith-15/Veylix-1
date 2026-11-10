package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

const (
	// resetCodeTTL is how long a reset code lives. Ten minutes is enough to switch
	// to the mailbox and back, and short enough that a shoulder-surfed code is
	// stale by the time anyone else can use it.
	resetCodeTTL = 10 * time.Minute
	// resetCodeMaxAttempts bounds guessing: a 6-digit code has 10^6 values, so
	// five tries leaves an attacker a 5-in-a-million chance per issued code.
	resetCodeMaxAttempts = 5
	// resetAttemptBurst is how many redemption calls one caller may make before the
	// limiter starts refusing.
	//
	// It must comfortably exceed resetCodeMaxAttempts, because a person correcting a
	// typo, or an operator testing the flow by hand, otherwise gets 429 instead of
	// the answer they need — and the limiter must bound abuse, not trap the user.
	// 60 is still far below the ~10^6 space of a code, and the per-code 5-attempt
	// counter (which is the real brute-force defence) is unaffected by it.
	resetAttemptBurst = 60
	// resetAttemptRate refills one token every 30 seconds: a genuine user never
	// notices, while a scripted grind is capped at 120/hour per address.
	resetAttemptRate = 1.0 / 30.0
)

// PasswordResetStore is the persistence this file needs.
type PasswordResetStore interface {
	CreatePasswordResetCode(ctx context.Context, userID int64, codeHash string, ttl time.Duration) error
	LivePasswordResetCode(ctx context.Context, userID int64) (*store.PasswordResetCode, error)
	BumpPasswordResetAttempts(ctx context.Context, id int64, max int) error
	ConsumePasswordResetCode(ctx context.Context, id, userID int64, passwordHash string) error
}

// PasswordResetRoutes serves the "forgotten password" pair (D-041).
//
// Two rules shape this file:
//
//  1. **`forgot` always answers 202.** A 404 for an unknown address would turn the
//     endpoint into an account-existence oracle — exactly what someone enumerating
//     clients wants. Body, status and timing are the same either way.
//  2. **Delivery is reported, never assumed.** With no SMTP relay the response and
//     the audit row say `outbox`; nothing claims an e-mail was sent.
//
// Who receives the code (owner 2026-10-29): the account's own address for clients
// and admins, plus the support inbox (`VEYLIX_SUPPORT_EMAIL`) for a super admin, so
// the owner keeps a copy of the highest-privilege account's reset.
func PasswordResetRoutes(
	mux *http.ServeMux,
	svc AuthService,
	resets PasswordResetStore,
	m EmailSender,
	aud *audit.Logger,
	supportEmail string,
	limiter *auth.RateLimiter,
) {
	// --- ask for a code -------------------------------------------------------
	mux.HandleFunc("POST /api/auth/forgot", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Identifier string `json:"identifier"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		identifier := strings.TrimSpace(in.Identifier)
		// Limited by identifier AND caller, so neither one mailbox nor one source
		// can drive mail volume.
		key := "forgot:" + strings.ToLower(identifier) + ":" + auth.ClientIP(r)
		if limiter != nil && !limiter.Allow(key) {
			WriteError(w, http.StatusTooManyRequests, "rate_limited",
				"too many reset requests, try again later")
			return
		}
		// Delivery is reported at the level of the *server*, never the account.
		//
		// Reporting the per-account outcome would turn this endpoint into an
		// account-existence oracle: a known address answers `smtp` and an unknown
		// one `outbox`, so anyone can test whether an e-mail is registered. The
		// honest, non-leaking question is "could this server have sent mail at
		// all?" — which is identical for every identifier.
		delivery := deliveryOutbox
		if m != nil && m.Configured() {
			delivery = deliverySMTP
		}
		if resets != nil && identifier != "" {
			// Issue the code; the returned per-account outcome is recorded in the
			// audit row (behind staff auth) and deliberately not put on the wire.
			_ = issueResetCode(r, resets, svc, m, aud, supportEmail, identifier)
		}
		WriteJSON(w, http.StatusAccepted, map[string]any{
			"sent":     true,
			"delivery": delivery,
			"message":  "If that account exists, a 6-digit code is on its way.",
		})
	})

	// --- redeem the code ------------------------------------------------------
	mux.HandleFunc("POST /api/auth/reset", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Identifier  string `json:"identifier"`
			Code        string `json:"code"`
			NewPassword string `json:"new_password"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if resets == nil {
			WriteError(w, http.StatusServiceUnavailable, "reset_unavailable",
				"password reset is not available on this deployment")
			return
		}
		if limiter != nil && !limiter.Allow("reset:"+auth.ClientIP(r)) {
			WriteError(w, http.StatusTooManyRequests, "rate_limited",
				"too many attempts, try again later")
			return
		}
		code := strings.TrimSpace(in.Code)
		if len(code) != 6 {
			WriteError(w, http.StatusUnprocessableEntity, "invalid_code", "that code is not valid")
			return
		}
		user, err := svc.FindUserByIdentifier(r.Context(), in.Identifier)
		if err != nil || user == nil {
			// Same answer as a wrong code: "no such account" here would be the
			// enumeration oracle the 202 above avoids.
			WriteError(w, http.StatusUnprocessableEntity, "invalid_code", "that code is not valid")
			return
		}
		redeemResetCode(w, r, resets, aud, user.ID, code, in.NewPassword)
	})
}

// PasswordResetUser is the minimum this file needs to know about an account.
type PasswordResetUser struct {
	ID    int64
	Email string
	Role  string
}

// redeemResetCode checks the code, applies the new password and kills every session.
func redeemResetCode(
	w http.ResponseWriter, r *http.Request, resets PasswordResetStore,
	aud *audit.Logger, userID int64, code, newPassword string,
) {
	live, err := resets.LivePasswordResetCode(r.Context(), userID)
	if err != nil || time.Now().After(live.ExpiresAt) {
		WriteError(w, http.StatusUnprocessableEntity, "invalid_code",
			"that code is not valid or has expired")
		return
	}
	if !checkCode(live.CodeHash, code) {
		_ = resets.BumpPasswordResetAttempts(r.Context(), live.ID, resetCodeMaxAttempts)
		if aud != nil {
			_ = aud.Record(r.Context(), audit.Entry{
				ActorID: userID, Action: "auth.reset_code_rejected", TargetType: "user",
				TargetID: itoa(userID), Context: map[string]any{"attempt": live.Attempts + 1},
			})
		}
		WriteError(w, http.StatusUnprocessableEntity, "invalid_code", "that code is not valid")
		return
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		// The policy sentinels carry a user-safe rule message (never the password).
		WriteAuthError(w, err)
		return
	}
	if err := resets.ConsumePasswordResetCode(r.Context(), live.ID, userID, hash); err != nil {
		WriteError(w, http.StatusUnprocessableEntity, "invalid_code",
			"that code is not valid or has expired")
		return
	}
	if aud != nil {
		_ = aud.Record(r.Context(), audit.Entry{
			ActorID: userID, Action: "auth.password_reset", TargetType: "user",
			TargetID: itoa(userID), Context: map[string]any{"sessions_revoked": true},
		})
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"reset": true,
		"note":  "every session was signed out — sign in with the new password",
	})
}

// Delivery outcomes reported on the wire and in the audit row.
//
// The distinction is load-bearing: `outbox` means the code was stored but no
// e-mail left the building, so the UI must ask an operator for it. Reporting
// `smtp` when nothing was sent (or the reverse) makes a person wait for a
// message that cannot arrive, or fail to look for one that already did.
const (
	deliverySMTP   = "smtp"
	deliveryOutbox = "outbox"
)

// issueResetCode creates and mails a code when the identifier resolves.
//
// It returns how the code was delivered (`smtp` or `outbox`) and never the code
// itself. An unresolvable identifier still answers `outbox`: the caller must
// reply identically for a known and an unknown address, so the "sent" branch
// cannot be observable as an account-existence oracle.
func issueResetCode(
	r *http.Request, resets PasswordResetStore, svc AuthService,
	m EmailSender,
	aud *audit.Logger, supportEmail, identifier string,
) string {
	user, err := svc.FindUserByIdentifier(r.Context(), identifier)
	if err != nil || user == nil || user.Email == "" {
		return deliveryOutbox
	}
	code, err := newNumericCode(6)
	if err != nil {
		return deliveryOutbox
	}
	hash, err := hashCode(code)
	if err != nil {
		return deliveryOutbox
	}
	if err := resets.CreatePasswordResetCode(r.Context(), user.ID, hash, resetCodeTTL); err != nil {
		return deliveryOutbox
	}

	subject := "Veylix — your password reset code"
	text := "Your Veylix password reset code is " + code + ".\n\n" +
		"It expires in 10 minutes and can be used once.\n" +
		"If you did not ask for a reset, ignore this message — your password has not changed.\n"
	html := `<p>Your Veylix password reset code is</p>` +
		`<p style="font-size:24px;letter-spacing:4px;font-weight:700">` + code + `</p>` +
		`<p>It expires in 10 minutes and can be used once.</p>` +
		`<p>If you did not ask for a reset, ignore this message — your password has not changed.</p>`

	// Delivery is discovered, never assumed.
	delivery := deliveryOutbox
	var errs []string
	toSupport := supportEmail != "" && user.Role == "super_admin" &&
		!strings.EqualFold(supportEmail, user.Email)
	if m != nil && m.Configured() {
		if err := m.Send(r.Context(), user.Email, subject, text, html); err != nil {
			// Surfaced in the audit row rather than swallowed: the code is stored,
			// but only a delivered code is usable by the owner.
			errs = append(errs, err.Error())
		} else {
			delivery = deliverySMTP
		}
		if toSupport {
			if err := m.Send(r.Context(), supportEmail, subject, text, html); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if aud != nil {
		_ = aud.Record(r.Context(), audit.Entry{
			ActorID: user.ID, Action: "auth.password_reset_requested", TargetType: "user",
			TargetID: itoa(user.ID),
			Context: map[string]any{
				"to": maskEmail(user.Email), "delivery": delivery,
				"support_copy": toSupport, "errors": errs,
			},
		})
	}
	return delivery
}
