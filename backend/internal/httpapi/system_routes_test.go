package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"veylix/backend/internal/auth"
)

// stubSystem implements SystemInfoService for the route tests, so the transport
// contract can be proven without a network or a release feed.
type stubSystem struct {
	info     SystemInfo
	infoErr  error
	releases []ReleaseInfo
	relErr   error
	check    UpdateCheck
	checkErr error
}

func (s stubSystem) Info(context.Context) (SystemInfo, error) { return s.info, s.infoErr }
func (s stubSystem) Releases(context.Context) ([]ReleaseInfo, error) {
	return s.releases, s.relErr
}
func (s stubSystem) CheckUpdate(context.Context, string) (UpdateCheck, error) {
	return s.check, s.checkErr
}

// servingMux builds a bare mux with a Super Admin already in the request
// context, so these tests exercise the ROUTE contract (status codes, JSON shape,
// 503-vs-404) without standing up the whole session chain. The auth requirement
// itself is asserted separately by TestSystemRoutesAreBehindAuth.
func servingMux(svc SystemServices, deps Deps) http.Handler {
	mux := http.NewServeMux()
	injectSuper := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithPrincipal(r.Context(), auth.Principal{
				UserID: 1,
				Email:  "super@test",
				Role:   auth.RoleSuperAdmin,
			})
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	SystemRoutes(mux, svc, injectSuper, deps)
	return injectSuper(mux)
}

// ---------------------------------------------------------------------------
// buildVersion
// ---------------------------------------------------------------------------

func TestBuildVersionNeverReturnsEmpty(t *testing.T) {
	if got := buildVersion(""); got != "dev" {
		t.Errorf("buildVersion(\"\") = %q, want \"dev\"", got)
	}
	if got := buildVersion("   "); got != "dev" {
		t.Errorf("buildVersion(whitespace) = %q, want \"dev\"", got)
	}
	if got := buildVersion("v1.4.0"); got != "v1.4.0" {
		t.Errorf("buildVersion(v1.4.0) = %q, want it unchanged", got)
	}
}

// ---------------------------------------------------------------------------
// GET /api/system/version
// ---------------------------------------------------------------------------

func TestSystemVersionReportsTheRunningBuild(t *testing.T) {
	svc := stubSystem{info: SystemInfo{
		Version:       "v1.4.0",
		SchemaVersion: 21,
		UI:            "embedded:85_files",
	}}
	mux := servingMux(svc, Deps{Version: "v1.4.0"})

	rec, body := get(t, mux, "/api/system/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["version"] != "v1.4.0" {
		t.Errorf("version = %v, want v1.4.0", body["version"])
	}
	if body["schema_version"] != float64(21) {
		t.Errorf("schema_version = %v, want 21", body["schema_version"])
	}
}

func TestSystemVersionFallsBackWhenTheServiceFails(t *testing.T) {
	// A database hiccup must not hide the build version: the operator needs to
	// know which binary is running even when the schema cannot be read.
	svc := stubSystem{infoErr: errors.New("db down")}
	mux := servingMux(svc, Deps{Version: "v1.4.0"})

	rec, body := get(t, mux, "/api/system/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["version"] != "v1.4.0" {
		t.Errorf("version = %v, want the build version from Deps", body["version"])
	}
	if body["go_version"] == "" || body["go_version"] == nil {
		t.Error("the fallback must still report go_version")
	}
}

func TestSystemVersionWorksWithNoServiceWired(t *testing.T) {
	mux := servingMux(nil, Deps{Version: "v9.9.9"})

	rec, body := get(t, mux, "/api/system/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["version"] != "v9.9.9" {
		t.Errorf("version = %v, want v9.9.9 from Deps", body["version"])
	}
}

// TestSystemVersionFillsSchemaAndUI is the regression test for the gap that let
// GET /api/system/version report schema 0 and an empty UI: the adapter
// (systemService.Info) cannot know either field, so a stub that mimics it must
// still end up with the real schema from Deps.Health and a non-empty UI status.
func TestSystemVersionFillsSchemaAndUI(t *testing.T) {
	svc := stubSystem{info: SystemInfo{Version: "v1.4.0"}} // schema 0 / ui "" — like the adapter
	mux := servingMux(svc, Deps{Version: "v1.4.0", Health: fakeHealth{schema: 21}})

	rec, body := get(t, mux, "/api/system/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["schema_version"] != float64(21) {
		t.Errorf("schema_version = %v, want 21 (filled from Deps.Health)", body["schema_version"])
	}
	if s, _ := body["ui"].(string); s == "" {
		t.Error("ui is empty; enrichInfo must fill it from spaStatusText")
	}
}

// A nil Health reader must not panic and must still report the build version.
func TestSystemVersionWithoutHealthReader(t *testing.T) {
	mux := servingMux(nil, Deps{Version: "v9.9.9"})
	rec, body := get(t, mux, "/api/system/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["version"] != "v9.9.9" {
		t.Errorf("version = %v, want v9.9.9", body["version"])
	}
}

// ---------------------------------------------------------------------------
// GET /api/system/releases
// ---------------------------------------------------------------------------

func TestSystemReleasesListsVersions(t *testing.T) {
	svc := stubSystem{releases: []ReleaseInfo{
		{Tag: "v1.5.0", IsLatest: true},
		{Tag: "v1.4.0", IsCurrent: true},
	}}
	mux := servingMux(svc, Deps{Version: "v1.4.0"})

	rec, body := get(t, mux, "/api/system/releases")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	rows, ok := body["releases"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("releases = %v, want a 2-element array", body["releases"])
	}
}

func TestSystemReleasesWithNoServiceIs503(t *testing.T) {
	mux := servingMux(nil, Deps{Version: "v1.4.0"})

	rec, _ := get(t, mux, "/api/system/releases")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (not 404: 'not wired' must be distinguishable)", rec.Code)
	}
}

func TestSystemReleasesSurfacesAFeedFailure(t *testing.T) {
	svc := stubSystem{relErr: errors.New("rate limit")}
	mux := servingMux(svc, Deps{Version: "v1.4.0"})

	rec, _ := get(t, mux, "/api/system/releases")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/system/update-check
// ---------------------------------------------------------------------------

func TestUpdateCheckReportsAnAvailableUpdate(t *testing.T) {
	svc := stubSystem{check: UpdateCheck{
		Current:         "v1.4.0",
		Latest:          "v1.5.0",
		UpdateAvailable: true,
	}}
	mux := servingMux(svc, Deps{Version: "v1.4.0"})

	rec, body := get(t, mux, "/api/system/update-check")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["update_available"] != true {
		t.Errorf("update_available = %v, want true", body["update_available"])
	}
	if body["latest"] != "v1.5.0" {
		t.Errorf("latest = %v, want v1.5.0", body["latest"])
	}
}

func TestUpdateCheckFailureIs200WithAnExplicitError(t *testing.T) {
	// The CHECK succeeded; the REMOTE failed. A 5xx would tell the UI the route
	// is broken, and an empty success would read as "up to date" — both wrong.
	svc := stubSystem{
		checkErr: errors.New("network down"),
		check:    UpdateCheck{Current: "v1.4.0", Error: "network down"},
	}
	mux := servingMux(svc, Deps{Version: "v1.4.0"})

	rec, body := get(t, mux, "/api/system/update-check")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with an error field", rec.Code)
	}
	if body["error"] == "" || body["error"] == nil {
		t.Error("error must be present so the UI can say why the check failed")
	}
	if body["update_available"] == true {
		t.Error("a failed check must never report an available update")
	}
}

func TestUpdateCheckWithNoServiceIs503(t *testing.T) {
	mux := servingMux(nil, Deps{Version: "v1.4.0"})

	rec, _ := get(t, mux, "/api/system/update-check")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// /api/ready must expose the version too
// ---------------------------------------------------------------------------

func TestReadyReportsTheVersion(t *testing.T) {
	h := Router(Deps{Health: fakeHealth{schema: 21}, Version: "v1.4.0"})

	rec, body := get(t, h, "/api/ready")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["version"] != "v1.4.0" {
		t.Fatalf("version = %v, want v1.4.0 (an operator must read it from the running process)",
			body["version"])
	}
	if body["schema_version"] != float64(21) {
		t.Errorf("schema_version = %v, want 21", body["schema_version"])
	}
}

func TestReadyReportsDevForAnUnstampedBuild(t *testing.T) {
	h := Router(Deps{Health: fakeHealth{schema: 21}})

	rec, body := get(t, h, "/api/ready")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["version"] != "dev" {
		t.Fatalf("version = %v, want \"dev\" (never an empty string)", body["version"])
	}
}

// ---------------------------------------------------------------------------
// The system routes must not be reachable without a session
// ---------------------------------------------------------------------------

func TestSystemRoutesAreBehindAuth(t *testing.T) {
	// Router mounts SystemRoutes only inside the auth block, so with no auth
	// services wired the routes must not exist at all (never a 200).
	h := Router(Deps{Health: fakeHealth{schema: 21}, Version: "v1.4.0"})

	for _, path := range []string{
		"/api/system/version",
		"/api/system/releases",
		"/api/system/update-check",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("%s answered 200 with no auth stack wired; it must not be public", path)
		}
	}
}
