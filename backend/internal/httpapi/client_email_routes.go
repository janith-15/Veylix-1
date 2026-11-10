package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// ClientEmailChanger is the Super Admin's path to correcting a customer's
// address (D-077).
//
// It exists because every other recovery in the console ends at a mailbox: the
// password reset e-mails a generated password, verification e-mails a link, and
// the profile flow makes the CUSTOMER confirm a code at their NEW address. When
// the address itself is wrong or dead — exactly the customer an operator is
// trying to rescue — the customer cannot complete any of them, and the console
// could only say "their e-mail is changed by them". That is a permanent dead end.
//
// The change still goes THROUGH the code, not around it: an operator may request
// it, and the confirmation code is mailed to the NEW address. Nothing here writes
// `users.email` directly, so a typo cannot silently move an account's identity
// and its recovery target.
type ClientEmailChanger interface {
	ProfileByID(ctx context.Context, userID int64) (*store.Profile, error)
	// UserByEmail returns (userID, email, err) — the cheap pre-check that stops a
	// code being sent to an address another account already owns. `store.ErrNoSuchIdentity`
	// means the address is free.
	UserByEmail(ctx context.Context, email string) (int64, string, error)
	CreateEmailChangeRequest(ctx context.Context, userID int64, newEmail, codeHash string, ttl time.Duration) error
	LiveEmailChangeRequest(ctx context.Context, userID int64) (*store.EmailChangeRequest, error)
	BumpEmailChangeAttempts(ctx context.Context, id int64, max int) error
	ConfirmEmailChange(ctx context.Context, id, userID int64, newEmail string) error
}

// ClientEmailChangeRoutes mounts the operator's two e-mail-change routes (D-077):
//
//	POST /api/super/clients/{id}/email-change           { new_email }
//	POST /api/super/clients/{id}/email-change/confirm   { code }
//
// Both re-use the store flow the customer's own /api/me/email-change uses — one
// implementation of "confirm at the new address", not a second one that could
// drift. Gated by `PermClientManage`, so a reseller can never reach it.
func ClientEmailChangeRoutes(mux *http.ServeMux, st ClientEmailChanger, mail EmailSender,
	aud *audit.Logger, authed func(http.Handler) http.Handler) {
	manage := func(h http.Handler) http.Handler {
		return authed(RequirePermission(auth.PermClientManage)(h))
	}
	mux.Handle("POST /api/super/clients/{id}/email-change",
		manage(clientRequestEmailChangeHandler(st, mail, aud)))
	mux.Handle("POST /api/super/clients/{id}/email-change/confirm",
		manage(clientConfirmEmailChangeHandler(st, aud)))
}

// clientRequestEmailChangeHandler starts an address change on behalf of a client.
//
//	POST /api/super/clients/{id}/email-change   { new_email }
//
// The operator does NOT supply a password (they do not know it, and asking would
// be a worse secret than the one they already hold); the authority is
// `PermClientManage` plus the audit trail. The code still goes to the new
// address, so a mistyped address cannot be completed by the operator alone.
func clientRequestEmailChangeHandler(st ClientEmailChanger, mail EmailSender, aud *audit.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, ok := clientPathID(w, r)
		if !ok {
			return
		}
		var in struct {
			NewEmail string `json:"new_email"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		newEmail := strings.ToLower(strings.TrimSpace(in.NewEmail))
		if !emailShape.MatchString(newEmail) || len(newEmail) > 190 {
			WriteError(w, http.StatusUnprocessableEntity, "invalid_email", "that is not a valid e-mail address")
			return
		}

		prof, err := st.ProfileByID(r.Context(), id)
		if err != nil || prof.Role != "client" {
			WriteError(w, http.StatusNotFound, "not_found", "client not found")
			return
		}
		if newEmail == strings.ToLower(prof.Email) {
			WriteError(w, http.StatusUnprocessableEntity, "same_email", "that is already this client's address")
			return
		}
		// Refuse an address another account already holds BEFORE sending a code.
		//
		// The database would eventually refuse it at confirm time, but by then the
		// operator has read a code out to somebody who does not own that mailbox
		// and waited for them to type it — and a "pending claim" on a taken
		// address would also be created, blocking the real owner from changing to
		// their own address. Failing here is the only moment the answer is cheap.
		if holderID, _, herr := st.UserByEmail(r.Context(), newEmail); herr == nil && holderID != id {
			WriteError(w, http.StatusConflict, "email_taken", "that address already belongs to another account")
			return
		}

		code, err := newNumericCode(6)
		if err == nil {
			var hash string
			if hash, err = hashCode(code); err == nil {
				err = st.CreateEmailChangeRequest(r.Context(), id, newEmail, hash, emailChangeTTL)
			}
		}
		if err != nil {
			if errors.Is(err, store.ErrIdentityTaken) {
				WriteError(w, http.StatusConflict, "email_taken",
					"that address already belongs to an account, or another change to it is in progress")
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not start the change")
			return
		}

		delivery := "outbox"
		if mail != nil && mail.Configured() {
			subject := "Veylix — confirm your new e-mail"
			text := "An operator started an e-mail change on your Veylix account.\n\n" +
				"Your confirmation code is " + code + ".\n" +
				"It expires in 15 minutes. If you did not ask for this, tell support and ignore this message.\n"
			html := `<p>An operator started an e-mail change on your Veylix account.</p>` +
				`<p>Your confirmation code is <strong style="font-size:20px;letter-spacing:2px">` +
				code + `</strong>.</p><p>It expires in 15 minutes.</p>`
			if err := mail.Send(r.Context(), newEmail, subject, text, html); err == nil {
				delivery = "smtp"
			}
		}

		auditClientChange(r.Context(), aud, p, "client.email_change_requested", id,
			map[string]any{"to": newEmail, "delivery": delivery})

		res := map[string]any{
			"sent": true, "to": newEmail, "delivery": delivery,
			"expires_minutes": int(emailChangeTTL.Minutes()),
			"note":            "The code goes to the NEW address. Nothing changes until that code is entered.",
		}
		// No relay configured means the code exists only on this server, and the
		// operator is the one who needs it: without this, "outbox" is a dead end
		// for the very case this route was built for.
		if delivery == "outbox" {
			res["code"] = code
		}
		WriteJSON(w, http.StatusAccepted, res)
	})
}

// clientConfirmEmailChangeHandler finishes it with the code from the new mailbox.
//
//	POST /api/super/clients/{id}/email-change/confirm   { code }
func clientConfirmEmailChangeHandler(st ClientEmailChanger, aud *audit.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, ok := clientPathID(w, r)
		if !ok {
			return
		}
		var in struct {
			Code string `json:"code"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		req, err := st.LiveEmailChangeRequest(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusUnprocessableEntity, "no_pending_change",
				"there is no pending e-mail change for this client — start one first")
			return
		}
		if time.Now().After(req.ExpiresAt) {
			WriteError(w, http.StatusUnprocessableEntity, "expired",
				"that code has expired — start the change again")
			return
		}
		if !checkCode(req.CodeHash, strings.TrimSpace(in.Code)) {
			// Counting the wrong guess is what burns a 6-digit code before it can
			// be found by brute force; the store deletes the request at the limit.
			_ = st.BumpEmailChangeAttempts(r.Context(), req.ID, emailChangeMaxAttempts)
			WriteError(w, http.StatusUnprocessableEntity, "bad_code", "that code is not correct")
			return
		}
		if err := st.ConfirmEmailChange(r.Context(), req.ID, id, req.NewEmail); err != nil {
			if errors.Is(err, store.ErrIdentityTaken) {
				WriteError(w, http.StatusConflict, "email_taken", "that address already belongs to an account")
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not change the address")
			return
		}
		auditClientChange(r.Context(), aud, p, "client.email_changed", id,
			map[string]any{"to": req.NewEmail})
		WriteJSON(w, http.StatusOK, map[string]any{
			"email": req.NewEmail,
			"note":  "The new address is the sign-in identifier now. The client must use it to sign in.",
		})
	})
}
