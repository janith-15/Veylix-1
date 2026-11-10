package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/settings"
)

// SettingsService is the operator-editable settings surface (D-080). Nil keeps
// the routes absent, which is honest: a deployment with no settings service
// cannot pretend the settings page works.
type SettingsService interface {
	View(ctx context.Context) (settings.GoogleView, error)
	Save(ctx context.Context, in settings.GoogleInput, actorID int64) (settings.GoogleView, error)
}

// googleSettingsBody is the stored view plus the redirect URL DERIVED from the
// address the operator is on right now.
//
// Why derive it: the stored value can be mistyped, and a wrong path is a
// guaranteed `redirect_uri_mismatch` at sign-in — an error that reads as "Google
// is broken" rather than "the path is wrong". The derived string comes from the
// request's own host (`schemeHost`, the same origin the CSRF guard trusts), so it
// is the URL the running server actually answers at. The page shows it verbatim
// for the operator to register, and can fill the field from it, so a typo is
// impossible. The embedded view keeps the wire shape flat and unchanged.
type googleSettingsBody struct {
	settings.GoogleView
	ExpectedRedirectURL string `json:"expected_redirect_url"`
}

func googleSettingsBodyFor(r *http.Request, v settings.GoogleView) googleSettingsBody {
	return googleSettingsBody{
		GoogleView:          v,
		ExpectedRedirectURL: schemeHost(r) + settings.GoogleCallbackPath,
	}
}

// SettingsRoutes registers the Super-Admin settings surface.
//
// Access: Super Admin only (PermServerViewAll). The Google client describes the
// operator's own deployment, and the secret it stores is a full OAuth
// credential — a reseller must never read it, and the read is masked regardless.
//
// A save is applied live (settings.Service.OnGoogleChange rebuilds the client in
// the running process), so the response reports what the process now holds, not
// merely what was written.
func SettingsRoutes(mux *http.ServeMux, svc SettingsService, authed func(http.Handler) http.Handler) {
	if svc == nil {
		return
	}
	manage := func(h http.HandlerFunc) http.Handler {
		return authed(RequirePermission(auth.PermServerViewAll)(h))
	}

	mux.Handle("GET /api/super/settings/google", manage(func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.View(r.Context())
		if err != nil {
			// A decryption failure is reported, never masked with an empty value
			// that would read as "not configured".
			WriteError(w, http.StatusInternalServerError, "settings_read_failed",
				"could not read the Google settings")
			return
		}
		WriteJSON(w, http.StatusOK, googleSettingsBodyFor(r, view))
	}))

	mux.Handle("PUT /api/super/settings/google", manage(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())

		var in struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RedirectURL  string `json:"redirect_url"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "the request body is not valid JSON")
			return
		}

		view, err := svc.Save(r.Context(), settings.GoogleInput{
			ClientID:     in.ClientID,
			ClientSecret: in.ClientSecret,
			RedirectURL:  in.RedirectURL,
		}, p.UserID)
		if err != nil {
			var ve *settings.ValidationError
			if errors.As(err, &ve) {
				WriteError(w, http.StatusBadRequest, "invalid_setting", ve.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, "settings_write_failed",
				"could not save the Google settings")
			return
		}
		WriteJSON(w, http.StatusOK, googleSettingsBodyFor(r, view))
	}))

	// GET /api/super/settings/public-url — the address this deployment is
	// actually served at (D-125).
	//
	// Derived from the request, not from a stored value: schemeHost is the same
	// origin the CSRF guard trusts, so the page shows the URL the running server
	// answers at. A stored copy could drift from the Caddyfile after a
	// `domain-set`, and a URL that disagrees with reality is worse than none.
	mux.Handle("GET /api/super/settings/public-url", manage(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, publicURLBody{
			URL:  schemeHost(r),
			Host: r.Host,
			// The login page is what an operator hands to their users, so it is
			// given assembled rather than left to string concatenation in the UI.
			LoginURL: schemeHost(r) + "/login",
		})
	}))
}

// publicURLBody is what the Super Admin settings page shows as "this deployment".
type publicURLBody struct {
	URL      string `json:"url"`
	Host     string `json:"host"`
	LoginURL string `json:"login_url"`
}
