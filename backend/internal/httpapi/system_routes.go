package httpapi

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"time"

	"veylix/backend/internal/auth"
)

// buildVersion normalises the injected build version. `go build` without
// -ldflags leaves "", and an empty string on the wire reads as "unknown" to a
// script. "dev" is honest: this binary was not built from a tagged release.
func buildVersion(v string) string {
	if s := strings.TrimSpace(v); s != "" {
		return s
	}
	return "dev"
}

// SystemInfo is what GET /api/system/version reports.
//
// It answers "what is actually running here?" from the RUNNING PROCESS, which is
// the only authority. A file on disk can be replaced without the process
// restarting, so the filename must never be used as evidence.
type SystemInfo struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	UI            string `json:"ui"`
	GoVersion     string `json:"go_version"`
	Platform      string `json:"platform"`
}

// ReleaseInfo is one row of GET /api/system/releases.
type ReleaseInfo struct {
	Tag         string `json:"tag"`
	PublishedAt string `json:"published_at,omitempty"`
	IsCurrent   bool   `json:"is_current"`
	IsLatest    bool   `json:"is_latest"`
	NotesURL    string `json:"notes_url,omitempty"`
}

// UpdateCheck is what GET /api/system/update-check reports.
//
// `Error` is set instead of fabricated values when the network or the release
// feed cannot be read: an update check that invents a version is worse than one
// that admits it failed.
type UpdateCheck struct {
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	CheckedAt       string `json:"checked_at,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	Error           string `json:"error,omitempty"`
}

// SystemInfoService is what Deps.System holds: the page needs Info plus the two
// feed-backed answers, and a test can supply a stub for either.
type SystemInfoService interface {
	SystemServices
}

// SystemServices is the collaborator set SystemRoutes needs. Every entry is an
// interface so this file imports nothing from internal/versioning or store.
//
// Access: the running version is safe for any authenticated caller (it is on the
// login page's mind anyway) but the release list and update check are restricted
// to the platform owner, because they describe the operator's deployment.
type SystemServices interface {
	// Info reports the running build.
	Info(ctx context.Context) (SystemInfo, error)
	// Releases lists published versions, newest first.
	Releases(ctx context.Context) ([]ReleaseInfo, error)
	// CheckUpdate compares the running version against the newest release.
	CheckUpdate(ctx context.Context, current string) (UpdateCheck, error)
}

// SystemRoutes registers the version, release-list and update-check surface.
//
// Access: the running version is safe for any authenticated caller (it is on the
// login page's mind anyway) but the release list and update check are restricted
// to the platform owner, because they describe the operator's deployment.
func SystemRoutes(mux *http.ServeMux, svc SystemServices, authed func(http.Handler) http.Handler, deps Deps) {
	// Super-Admin only: naming the release feed tells an attacker which version
	// to target, and the feed is about the platform, not about a customer.
	manage := func(h http.HandlerFunc) http.Handler {
		return authed(RequirePermission(auth.PermServerViewAll)(h))
	}

	if svc == nil {
		// Still expose the version: it comes from Deps and needs no service.
		// Anything that needs the release feed answers 503 rather than 404, so a
		// caller can tell "not wired" from "wrong path".
		mux.Handle("GET /api/system/version", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			WriteJSON(w, http.StatusOK, enrichInfo(r.Context(), deps, fallbackInfo(deps)))
		})))
		notWired := func(w http.ResponseWriter, r *http.Request) {
			WriteError(w, http.StatusServiceUnavailable, "releases_unavailable",
				"the release feed is not configured on this installation")
		}
		mux.Handle("GET /api/system/releases", manage(notWired))
		mux.Handle("GET /api/system/update-check", manage(notWired))
		return
	}

	mux.Handle("GET /api/system/version", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, err := svc.Info(r.Context())
		if err != nil {
			// A schema read failure must not hide the build version.
			info = fallbackInfo(deps)
		}
		// The adapter cannot know the schema version or the embedded-UI status;
		// fill both here, or the endpoint would report schema 0 and an empty UI.
		WriteJSON(w, http.StatusOK, enrichInfo(r.Context(), deps, info))
	})))

	mux.Handle("GET /api/system/releases", manage(func(w http.ResponseWriter, r *http.Request) {
		rows, err := svc.Releases(r.Context())
		if err != nil {
			WriteError(w, http.StatusBadGateway, "releases_unavailable", err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"releases": rows})
	}))

	mux.Handle("GET /api/system/update-check", manage(func(w http.ResponseWriter, r *http.Request) {
		current := buildVersion(deps.Version)
		res, err := svc.CheckUpdate(r.Context(), current)
		if err != nil {
			// 200 with an explicit error field: the check itself succeeded, the
			// remote did not. Callers must see "unknown", never "up to date".
			WriteJSON(w, http.StatusOK, UpdateCheck{Current: current, Error: err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, res)
	}))
}

// enrichInfo fills the two fields the SystemServices adapter cannot know: the
// applied schema version (only the Health reader has it) and whether this binary
// carries the embedded UI (only this package owns spaStatusText). Both paths —
// wired and fallback — go through it, so GET /api/system/version can never
// silently report schema 0 and an empty UI.
func enrichInfo(ctx context.Context, deps Deps, info SystemInfo) SystemInfo {
	info.UI = spaStatusText()
	if deps.Health != nil {
		// A bounded read: a hung database must not stall the version endpoint.
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if schema, err := deps.Health.SchemaVersion(cctx); err == nil {
			info.SchemaVersion = schema
		}
	}
	return info
}

// fallbackInfo builds the version document without touching the database.
func fallbackInfo(deps Deps) SystemInfo {
	return SystemInfo{
		Version:   buildVersion(deps.Version),
		UI:        spaStatusText(),
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
}
