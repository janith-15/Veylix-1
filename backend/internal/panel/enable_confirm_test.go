package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestSetClientEnabledConfirmsThePanelActuallyChanged is the D-114 defect.
//
// Found live: `POST /api/vpns/55/suspend` answered **204** while the panel still
// reported the client `enable: true`. `SetClientEnabled` trusted HTTP 200 and
// the envelope's `success` flag, and never read the client back — so a panel
// that accepts the request and does not apply it produced a *false success*, and
// `Suspend`/`Resume` marked the local row with a state the panel never entered.
//
// A 3X-UI bulk call can legitimately answer 200 {"success":true} and still leave
// the client alone (observed against the live panel: an intermittent no-op). The
// only honest confirmation is a fresh read of the client, which this adapter
// already knows how to do (`GetClient`).
func TestSetClientEnabledConfirmsThePanelActuallyChanged(t *testing.T) {
	// The panel accepts both calls and reports success, but the client stays
	// enabled. This is the state that produced the false 204.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/panel/api/clients/bulkDisable":
			_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":null}`))
		case r.URL.Path == "/panel/api/clients/bulkEnable":
			_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":null}`))
		case r.URL.Path == "/panel/api/clients/get/probe@example.com":
			// Still enabled — the write did not take effect.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "msg": "",
				"obj": map[string]any{"client": map[string]any{"email": "probe@example.com", "enable": true}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	c, _ := newTestClient(t, h)

	err := c.SetClientEnabled(context.Background(), "probe@example.com", false)
	if err == nil {
		t.Fatal("SetClientEnabled reported success although the panel still holds the client enabled — " +
			"a suspend that does nothing must never answer 204 (D-114)")
	}
}

// TestSetClientEnabledSucceedsWhenThePanelReallyDisabledIt is the other half:
// the confirmation must not make a genuine change fail.
func TestSetClientEnabledSucceedsWhenThePanelReallyDisabledIt(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/panel/api/clients/bulkDisable":
			_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":null}`))
		case r.URL.Path == "/panel/api/clients/get/probe@example.com":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "msg": "",
				"obj": map[string]any{"client": map[string]any{"email": "probe@example.com", "enable": false}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	c, _ := newTestClient(t, h)

	if err := c.SetClientEnabled(context.Background(), "probe@example.com", false); err != nil {
		t.Fatalf("SetClientEnabled on a panel that really disabled the client = %v, want nil", err)
	}
}
