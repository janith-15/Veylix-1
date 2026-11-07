package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeHealth struct {
	pingErr   error
	schema    int
	schemaErr error
}

func (f fakeHealth) Ping(context.Context) error { return f.pingErr }
func (f fakeHealth) SchemaVersion(context.Context) (int, error) {
	return f.schema, f.schemaErr
}

func get(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: response is not JSON: %v", path, err)
		}
	}
	return rec, body
}

func TestHealthIsIndependentOfDatabase(t *testing.T) {
	// Liveness must answer even when the database is down, otherwise an
	// orchestrator would restart a healthy process.
	h := Router(Deps{Health: fakeHealth{pingErr: errors.New("db down")}, Version: "test"})
	rec, body := get(t, h, "/api/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %v, want ok", body["status"])
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff header")
	}
}

func TestReadyReportsSchemaVersion(t *testing.T) {
	h := Router(Deps{Health: fakeHealth{schema: 3}, Version: "test"})
	rec, body := get(t, h, "/api/ready")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["status"] != "ready" {
		t.Errorf("status field = %v, want ready", body["status"])
	}
	if body["schema_version"] != float64(3) {
		t.Errorf("schema_version = %v, want 3", body["schema_version"])
	}
}

func TestReadyFailsClosedWhenDatabaseIsDown(t *testing.T) {
	h := Router(Deps{Health: fakeHealth{pingErr: errors.New("boom")}, Version: "test"})
	rec, body := get(t, h, "/api/ready")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error envelope: %v", body)
	}
	if errObj["code"] != "db_unavailable" {
		t.Errorf("code = %v, want db_unavailable", errObj["code"])
	}
	// The internal error text must not reach the client.
	if msg, _ := errObj["message"].(string); msg == "boom" {
		t.Error("internal error text leaked to the client")
	}
}

func TestUnknownAPIRouteReturnsEnvelope(t *testing.T) {
	h := Router(Deps{Health: fakeHealth{}, Version: "test"})
	rec, body := get(t, h, "/api/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok || errObj["code"] != "not_found" {
		t.Fatalf("want not_found envelope, got %v", body)
	}
}
