package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// ProfileStore backs the self-service profile (D-041).
type ProfileStore interface {
	ProfileByID(ctx context.Context, userID int64) (*store.Profile, error)
	UpdateProfile(ctx context.Context, userID int64, u store.ProfileUpdate) (*store.Profile, error)
	UsernameAvailable(ctx context.Context, username string, exceptUserID int64) (bool, error)
	CreateEmailChangeRequest(ctx context.Context, userID int64, newEmail, codeHash string, ttl time.Duration) error
	LiveEmailChangeRequest(ctx context.Context, userID int64) (*store.EmailChangeRequest, error)
	BumpEmailChangeAttempts(ctx context.Context, id int64, max int) error
	ConfirmEmailChange(ctx context.Context, id, userID int64, newEmail string) error
}

// EmailSender delivers one message. Nil (or not configured) means "no relay": the
// routes then say so instead of claiming a mail was sent.
type EmailSender interface {
	Configured() bool
	Send(ctx context.Context, to, subject, textBody, htmlBody string) error
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)

// recordProfile writes an audit entry for a profile change.
func recordProfile(aud *audit.Logger, r *http.Request, p auth.Principal, action string, ctx map[string]any) {
	if aud == nil {
		return
	}
	_ = aud.Record(r.Context(), audit.Entry{
		ActorID: p.UserID, Action: action, TargetType: "user",
		TargetID: itoa(p.UserID), Result: "OK", Context: ctx,
	})
}

// patchedFields lists which profile fields a PATCH actually touched, so the audit
// trail says WHAT changed without storing the values (a phone number is personal).
func patchedFields(in profilePatch) []string {
	out := make([]string, 0, 10)
	for name, set := range map[string]bool{
		"username": in.Username != nil, "display_name": in.DisplayName != nil,
		"avatar_url": in.AvatarURL != nil, "phone": in.Phone != nil,
		"whatsapp": in.WhatsApp != nil, "telegram": in.Telegram != nil,
		"discord": in.Discord != nil, "birthday": in.Birthday != nil, "age": in.Age != nil,
		"bio": in.Bio != nil, "timezone": in.Timezone != nil,
	} {
		if set {
			out = append(out, name)
		}
	}
	return out
}

// validateProfilePatch normalises and checks a patch in place. It returns the
// error CODE (and message) so the form can put the message next to the input that
// caused it, rather than a generic 422.
func validateProfilePatch(r *http.Request, st ProfileStore, userID int64, in *profilePatch) (msg, code string, status int) {
	const unprocessable = http.StatusUnprocessableEntity
	if in.Username != nil {
		u := strings.ToLower(strings.TrimSpace(*in.Username))
		if !usernamePattern.MatchString(u) {
			return "a username is 3-32 characters: letters, digits, dot, hyphen or underscore", "invalid_username", unprocessable
		}
		free, err := st.UsernameAvailable(r.Context(), u, userID)
		if err != nil {
			return "could not check the username", "internal_error", http.StatusInternalServerError
		}
		if !free {
			// 409 is the honest status for "well-formed but taken".
			return "that username is already in use", "username_taken", http.StatusConflict
		}
		in.Username = &u
	}
	if in.DisplayName != nil {
		d := strings.TrimSpace(*in.DisplayName)
		if d == "" || len([]rune(d)) > 80 {
			return "a name is 1-80 characters", "invalid_display_name", unprocessable
		}
		in.DisplayName = &d
	}
	if in.Age != nil && (*in.Age < 1 || *in.Age > 120) {
		return "age must be between 1 and 120", "invalid_age", unprocessable
	}
	if in.Birthday != nil {
		b := strings.TrimSpace(*in.Birthday)
		if b != "" {
			// ISO only: the wire format is the storage format, so nothing has to
			// guess whether 03/04 is March or April.
			d, err := time.Parse("2006-01-02", b)
			if err != nil {
				return "birthday must be a real date in YYYY-MM-DD form", "invalid_birthday", unprocessable
			}
			now := time.Now()
			if d.After(now) {
				return "a birthday cannot be in the future", "invalid_birthday", unprocessable
			}
			if now.Year()-d.Year() < minAgeYears {
				return fmt.Sprintf("you must be at least %d years old", minAgeYears), "invalid_birthday", unprocessable
			}
		}
		in.Birthday = &b
	}
	if in.Bio != nil && len([]rune(*in.Bio)) > 500 {
		return "the bio is limited to 500 characters", "invalid_bio", unprocessable
	}
	if in.AvatarURL != nil {
		a := strings.TrimSpace(*in.AvatarURL)
		// An <img src> may only be an http(s) URL or our own upload path: a
		// `javascript:` or `data:` URL there is a script-execution vector.
		okScheme := a == "" || strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") ||
			strings.HasPrefix(a, "/uploads/")
		if !okScheme || len(a) > 500 {
			return "an avatar must be an http(s) URL or an uploaded image", "invalid_avatar", unprocessable
		}
		in.AvatarURL = &a
	}
	if in.Timezone != nil {
		tz := strings.TrimSpace(*in.Timezone)
		if len(tz) > 64 {
			return "that timezone name is too long", "invalid_timezone", unprocessable
		}
		in.Timezone = &tz
	}
	return "", "", http.StatusOK
}

// means "leave it alone": saving the contact tab must not blank the avatar.
type profilePatch struct {
	Username    *string `json:"username"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Phone       *string `json:"phone"`
	WhatsApp    *string `json:"whatsapp"`
	Telegram    *string `json:"telegram"`
	Discord     *string `json:"discord"`
	// Birthday is ISO YYYY-MM-DD. Preferred over `age`: a stored age is stale the
	// day after it is written, while a birth date stays true and the age is
	// derived on read.
	Birthday *string `json:"birthday"`
	Age      *int    `json:"age"`
	Bio      *string `json:"bio"`
	Timezone *string `json:"timezone"`
}

// minAgeYears is the floor for a self-declared birth date. It is a sanity bound,
// not an age gate: a date under it is refused so a typo (e.g. 2062) cannot be
// saved and show a negative age later.
const minAgeYears = 13

// ProfileRoutes registers the self-service account surface (D-041):
//
//	GET   /api/me/profile              read my profile
//	PATCH /api/me/profile              name, username, contacts, avatar, age, bio…
//	GET   /api/me/username-available   ?username=  (live check for the form)
//	POST  /api/me/email-change/request { new_email, current_password }
//	POST  /api/me/email-change/confirm { code }
//
// Rules that matter:
//   - every handler acts on the SESSION's user id; no id is ever read from the
//     body or query, so an account can only change itself;
//   - `email` is NOT patchable: an address change is a code sent to the new
//     address (it is the login identifier and the recovery target);
//   - `role`, `status`, `quota_limit` and `free_vpn_limit` are not patchable
//     either — those are operator decisions, never self-service.
func ProfileRoutes(mux *http.ServeMux, st ProfileStore, mail EmailSender, aud *audit.Logger,
	sessions SessionManager, verifyPassword func(ctx context.Context, userID int64, password string) bool,
	authed func(http.Handler) http.Handler) {

	mux.Handle("GET /api/me/profile", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		prof, err := st.ProfileByID(r.Context(), p.UserID)
		if err != nil {
			WriteError(w, http.StatusNotFound, "not_found", "account not found")
			return
		}
		WriteJSON(w, http.StatusOK, prof)
	})))

	mux.Handle("PATCH /api/me/profile", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var in profilePatch
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if msg, code, status := validateProfilePatch(r, st, p.UserID, &in); code != "" {
			WriteError(w, status, code, msg)
			return
		}

		prof, err := st.UpdateProfile(r.Context(), p.UserID, store.ProfileUpdate{
			Username: in.Username, DisplayName: in.DisplayName, AvatarURL: in.AvatarURL,
			Phone: in.Phone, WhatsApp: in.WhatsApp, Telegram: in.Telegram, Discord: in.Discord,
			Birthday: in.Birthday, Age: in.Age, Bio: in.Bio, Timezone: in.Timezone,
		})
		if err != nil {
			if errors.Is(err, store.ErrUsernameTaken) {
				WriteError(w, http.StatusConflict, "username_taken", "that username is already in use")
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not save the profile")
			return
		}
		recordProfile(aud, r, p, "profile.update", map[string]any{"fields": patchedFields(in)})
		WriteJSON(w, http.StatusOK, prof)
	})))

	mux.Handle("GET /api/me/username-available", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		u := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("username")))
		if !usernamePattern.MatchString(u) {
			WriteJSON(w, http.StatusOK, map[string]any{"available": false, "reason": "invalid"})
			return
		}
		free, err := st.UsernameAvailable(r.Context(), u, p.UserID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not check the username")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"available": free})
	})))

	// --- e-mail change (the ONE field that must go through e-mail) ------------
	//
	// D-041: "email eka change karanna one hema welawema email hara withayi". The
	// address is the login identifier and the recovery target, so:
	//   1. the account asks for a change and re-states its current password;
	//   2. a 6-digit code goes to the NEW address;
	//   3. confirming swaps the address, marks it verified, and revokes every OTHER
	//      session, because the identity of the account just changed.
	mux.Handle("POST /api/me/email-change/request", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var in struct {
			NewEmail        string `json:"new_email"`
			CurrentPassword string `json:"current_password"`
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
		prof, err := st.ProfileByID(r.Context(), p.UserID)
		if err != nil {
			WriteError(w, http.StatusNotFound, "not_found", "account not found")
			return
		}
		if newEmail == strings.ToLower(prof.Email) {
			WriteError(w, http.StatusUnprocessableEntity, "same_email", "that is already your address")
			return
		}
		// Re-authentication: a stolen session alone must not move the recovery
		// target of the account to an attacker's mailbox.
		if prof.HasPassword {
			if in.CurrentPassword == "" || !verifyAccountPassword(r.Context(), verifyPassword, prof.ID, in.CurrentPassword) {
				WriteError(w, http.StatusUnprocessableEntity, "password_mismatch", "your current password is incorrect")
				return
			}
		}
		code, err := newNumericCode(6)
		if err == nil {
			var hash string
			if hash, err = hashCode(code); err == nil {
				err = st.CreateEmailChangeRequest(r.Context(), p.UserID, newEmail, hash, emailChangeTTL)
			}
		}
		if err != nil {
			if errors.Is(err, store.ErrIdentityTaken) {
				WriteError(w, http.StatusConflict, "email_taken", "another account is already confirming that address")
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not start the change")
			return
		}

		delivery := "outbox"
		if mail != nil && mail.Configured() {
			subject := "Veylix — confirm your new e-mail"
			text := "Your Veylix e-mail change code is " + code + ".\n" +
				"It expires in 15 minutes. If you did not ask for this, ignore this message.\n"
			html := `<p>Your Veylix e-mail change code is <strong style="font-size:20px;letter-spacing:2px">` +
				code + `</strong>.</p><p>It expires in 15 minutes.</p>`
			if err := mail.Send(r.Context(), newEmail, subject, text, html); err == nil {
				delivery = "smtp"
			}
		}
		recordProfile(aud, r, p, "profile.email_change_requested",
			map[string]any{"to": maskEmail(newEmail), "delivery": delivery})
		res := map[string]any{"sent": true, "delivery": delivery, "to": maskEmail(newEmail), "expires_minutes": 15}
		// Without a relay the operator still needs a way in; only a loopback caller
		// (a local operator) sees the code. A deployment never gets it back.
		if delivery == "outbox" && isLoopbackRequest(r) {
			res["code"] = code
		}
		WriteJSON(w, http.StatusAccepted, res)
	})))

	mux.Handle("POST /api/me/email-change/confirm", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var in struct {
			Code string `json:"code"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		code := strings.TrimSpace(in.Code)
		if len(code) != 6 {
			WriteError(w, http.StatusUnprocessableEntity, "invalid_code", "enter the 6-digit code")
			return
		}
		req, err := st.LiveEmailChangeRequest(r.Context(), p.UserID)
		if err != nil {
			WriteError(w, http.StatusUnprocessableEntity, "no_pending_change",
				"there is no pending e-mail change — request a new code")
			return
		}
		if time.Now().After(req.ExpiresAt) {
			WriteError(w, http.StatusUnprocessableEntity, "code_expired", "that code has expired — request a new one")
			return
		}
		if !checkCode(req.CodeHash, code) {
			_ = st.BumpEmailChangeAttempts(r.Context(), req.ID, emailChangeMaxAttempts)
			WriteError(w, http.StatusUnprocessableEntity, "invalid_code", "that code is not correct")
			return
		}
		if err := st.ConfirmEmailChange(r.Context(), req.ID, p.UserID, req.NewEmail); err != nil {
			if errors.Is(err, store.ErrIdentityTaken) {
				WriteError(w, http.StatusConflict, "email_taken", "that address is already registered")
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not change the address")
			return
		}
		// The identity changed: end every other session, then re-issue a cookie for
		// THIS browser so the person is not thrown out of the page they are on.
		if sessions != nil {
			_ = sessions.RevokeAllForUser(r.Context(), p.UserID)
			if token, err := sessions.Create(r.Context(), p.UserID, auth.ClientIP(r), r.UserAgent()); err == nil {
				sessions.SetCookie(w, token)
			}
		}
		recordProfile(aud, r, p, "profile.email_changed", map[string]any{"to": maskEmail(req.NewEmail)})
		prof, _ := st.ProfileByID(r.Context(), p.UserID)
		WriteJSON(w, http.StatusOK, map[string]any{"changed": true, "profile": prof})
	})))
}

// --- helpers shared by the profile routes ------------------------------------

const (
	// emailChangeTTL is how long a change code lives: long enough to open a mail
	// on a phone, short enough that a leaked code goes stale.
	emailChangeTTL = 15 * time.Minute
	// emailChangeMaxAttempts bounds guessing (a 6-digit code has 10^6 values).
	emailChangeMaxAttempts = 5
)

// emailShape is the same rule the auth service applies at registration.
var emailShape = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// itoa is strconv.FormatInt without the import noise at every call site.
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// maskEmail shows enough of an address to recognise it, not enough to harvest it:
// "nimal@example.com" → "n****@example.com".
func maskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 2 {
		return string(local[0]) + "***" + domain
	}
	return string(local[0]) + strings.Repeat("*", min(len(local)-1, 5)) + domain
}

// --- code + password helpers -------------------------------------------------

// newNumericCode returns a uniform random decimal code of `digits` digits.
//
// crypto/rand, not math/rand: a predictable reset code is an account takeover.
// `rand.Int` avoids the modulo bias of `% 1000000`.
func newNumericCode(digits int) (string, error) {
	if digits < 4 || digits > 10 {
		return "", errors.New("httpapi: code length out of range")
	}
	limit := 1
	for i := 0; i < digits; i++ {
		limit *= 10
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", digits, n.Int64()), nil
}

// hashCode hashes a short code for storage. bcrypt is deliberately slow, so the
// stored value is useless to anyone who reads the table.
func hashCode(code string) (string, error) {
	hash, err := auth.HashPassword(code + codeSaltSuffix)
	if err != nil {
		return "", err
	}
	return hash, nil
}

// checkCode compares a submitted code against a stored hash.
func checkCode(hash, code string) bool {
	return auth.VerifyPassword(hash, code+codeSaltSuffix)
}

// codeSaltSuffix lengthens a 6-digit code before bcrypt.
//
// A plain 6-digit value is inside bcrypt's short-input path and, more importantly,
// would let a precomputed table of the 10^6 codes work across every account. The
// suffix costs nothing and removes that.
const codeSaltSuffix = "|veylix-verification-code|"

// verifyAccountPassword checks the caller's own password, for
// re-authentication before a sensitive change (e-mail move). `verify` is injected
// so this package never imports the auth service just to compare a hash.
func verifyAccountPassword(ctx context.Context, verify func(ctx context.Context, userID int64, password string) bool, userID int64, password string) bool {
	if password == "" || verify == nil {
		return false
	}
	return verify(ctx, userID, password)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
