package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// newPayDB opens an isolated database for the order-list tests.
//
// Isolated on purpose: these tests cover a CAP, so they must be able to exceed
// it without touching the development data in ./var/ (change-control §"Data
// safety").
func newPayDB(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "pay-cap.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

// TestPaymentListIsCappedButTheCountIsNot is the D-116 defect, stated as an
// assertion.
//
// The list stopped at 200 rows with nothing to say so, which made "the orders
// screen shows 200 orders" and "the orders screen shows the newest 200 of
// 2,400" produce the identical page. An operator deciding whether they had
// reconciled every payment could not tell them apart.
func TestPaymentListIsCappedButTheCountIsNot(t *testing.T) {
	ctx := context.Background()
	st := newPayDB(t)
	payerID := seedClient(t, st, "bulk@example.com")

	// One more than the cap, so the truncation is real rather than assumed.
	n := PaymentListLimit + 1
	for i := 0; i < n; i++ {
		if _, err := st.DB().ExecContext(ctx,
			`INSERT INTO payment_records
			   (payer_user_id, amount_cents, currency, status, created_at, updated_at)
			 VALUES (?, 1000, 'LKR', 'CONFIRMED', ?, ?)`, payerID, i+1, i+1); err != nil {
			t.Fatalf("seed payment %d: %v", i, err)
		}
	}

	rows, err := st.PaymentsVisible(ctx, 1, "super_admin", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != PaymentListLimit {
		t.Fatalf("list returned %d rows, want the cap of %d", len(rows), PaymentListLimit)
	}

	total, err := st.PaymentsVisibleCount(ctx, 1, "super_admin", false)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != n {
		t.Fatalf("count = %d, want %d — the count must ignore the cap, or the screen cannot say "+
			"that older orders exist", total, n)
	}

	// Newest first is what makes the cap honest: the rows that are missing are
	// the OLD ones, never a gap in the middle.
	if rows[0].ID <= rows[len(rows)-1].ID {
		t.Errorf("list is not newest-first: first id %d, last id %d", rows[0].ID, rows[len(rows)-1].ID)
	}
}

// TestPaymentCountMatchesTheListScope proves the count and the list agree about
// WHICH rows a caller may see.
//
// A count that ignored the ownership filter would tell a reseller "2,400
// orders" while the list showed their own 3 — a number pointing at rows they
// cannot open. Both come from paymentsScope for exactly this reason.
func TestPaymentCountMatchesTheListScope(t *testing.T) {
	ctx := context.Background()
	st := newPayDB(t)
	mineID := seedClient(t, st, "mine@example.com")
	otherID := seedClient(t, st, "other@example.com")

	for _, payer := range []int64{mineID, mineID, otherID} {
		if _, err := st.DB().ExecContext(ctx,
			`INSERT INTO payment_records
			   (payer_user_id, amount_cents, currency, status, created_at, updated_at)
			 VALUES (?, 500, 'LKR', 'CONFIRMED', 1, 1)`, payer); err != nil {
			t.Fatalf("seed payment: %v", err)
		}
	}

	for _, role := range []string{"super_admin", "admin", "client"} {
		t.Run(role, func(t *testing.T) {
			rows, err := st.PaymentsVisible(ctx, mineID, role, false)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			total, err := st.PaymentsVisibleCount(ctx, mineID, role, false)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if total != len(rows) {
				t.Fatalf("%s: count = %d but the list returned %d rows — the two must "+
					"describe the same set", role, total, len(rows))
			}
		})
	}
}

// TestPaymentCountRespectsThePendingFilter guards the other half of the scope:
// a queue badge asks for PENDING only, and must not be told about everything.
func TestPaymentCountRespectsThePendingFilter(t *testing.T) {
	ctx := context.Background()
	st := newPayDB(t)
	payerID := seedClient(t, st, "queue@example.com")

	statuses := []string{"PENDING", "AWAITING_CONFIRMATION", "CONFIRMED", "REJECTED"}
	for i, s := range statuses {
		if _, err := st.DB().ExecContext(ctx,
			`INSERT INTO payment_records
			   (payer_user_id, amount_cents, currency, status, created_at, updated_at)
			 VALUES (?, 100, 'LKR', ?, ?, ?)`, payerID, s, i+1, i+1); err != nil {
			t.Fatalf("seed %s: %v", s, err)
		}
	}

	all, err := st.PaymentsVisibleCount(ctx, 1, "super_admin", false)
	if err != nil {
		t.Fatalf("count all: %v", err)
	}
	pending, err := st.PaymentsVisibleCount(ctx, 1, "super_admin", true)
	if err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if all != len(statuses) {
		t.Errorf("all = %d, want %d", all, len(statuses))
	}
	if pending != 2 {
		t.Errorf("pending = %d, want 2 (PENDING + AWAITING_CONFIRMATION)", pending)
	}
}

// TestPaymentCountOnAnEmptyDatabase returns zero, not an error and not -1.
// "No orders" is a fact; the API reserves -1 for "could not count".
func TestPaymentCountOnAnEmptyDatabase(t *testing.T) {
	st := newPayDB(t)
	got, err := st.PaymentsVisibleCount(context.Background(), 1, "super_admin", false)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 0 {
		t.Fatalf("count = %d, want 0 (%s)", got, fmt.Sprint("an empty table has no rows"))
	}
}
