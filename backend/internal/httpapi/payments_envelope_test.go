package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"veylix/backend/internal/store"
)

// TestPaymentsEnvelopeStatesTruncation proves the ROUTE, not just the store.
//
// The store could already cap, and a count now exists, but the defect was that
// nothing joined them: `GET /api/payments` answered with a bare array, so a
// screen had no way to know it was looking at a window. This asserts the
// envelope says so — and, equally important, that the plain response is still
// the bare array every existing caller depends on (D-116).
func TestPaymentsEnvelopeStatesTruncation(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t)

	// One more than the cap, so truncation is real rather than assumed.
	n := store.PaymentListLimit + 5
	for i := 0; i < n; i++ {
		if _, err := env.st.DB().ExecContext(context.Background(),
			`INSERT INTO payment_records
			   (payer_user_id, amount_cents, currency, status, created_at, updated_at)
			 VALUES (1, 100, 'LKR', 'CONFIRMED', ?, ?)`, i+1, i+1); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	// ---- the envelope ------------------------------------------------------
	_, page := getJSON(t, root, env.srv.URL+"/api/payments?envelope=1")

	shown := int(page["shown"].(float64))
	total := int(page["total"].(float64))
	truncated, _ := page["truncated"].(bool)
	rows, _ := page["payments"].([]any)

	if shown != store.PaymentListLimit {
		t.Errorf("shown = %d, want the cap of %d", shown, store.PaymentListLimit)
	}
	if total != n {
		t.Errorf("total = %d, want %d — the wire must carry the UNCAPPED count", total, n)
	}
	if !truncated {
		t.Error("truncated = false although the table holds more rows than the cap — " +
			"this is the exact silent-cap defect D-116")
	}
	if len(rows) != shown {
		t.Errorf("payments len = %d but shown says %d", len(rows), shown)
	}

	// ---- the shape existing callers rely on --------------------------------
	rows2 := getJSONArray(t, root, env.srv.URL+"/api/payments")
	if len(rows2) != store.PaymentListLimit {
		t.Errorf("plain array len = %d, want %d — existing callers must keep working",
			len(rows2), store.PaymentListLimit)
	}

	// ---- an anonymous caller learns nothing ---------------------------------
	resp, err := http.Get(env.srv.URL + "/api/payments?envelope=1")
	if err != nil {
		t.Fatalf("anonymous get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous envelope = %d, want 401", resp.StatusCode)
	}
}
