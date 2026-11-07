package store

import (
	"context"
	"database/sql"
	"testing"
)

// The 0008 schema is the foundation of D-030 … D-033: a plan has an owner, a
// price and a slot inventory; an order records what it bought and what it
// produced. If any of these are missing, the money path cannot work.
func TestPlanMoneySchemaIsPresent(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	offers, err := tableColumns(ctx, t, st, "vpn_offers")
	if err != nil {
		t.Fatalf("table_info(vpn_offers): %v", err)
	}
	for _, col := range []string{
		"owner_user_id", "price_cents", "currency",
		"slots_total", "slots_used", "archived_at",
	} {
		if !offers[col] {
			t.Errorf("vpn_offers.%s is missing", col)
		}
	}

	pays, err := tableColumns(ctx, t, st, "payment_records")
	if err != nil {
		t.Fatalf("table_info(payment_records): %v", err)
	}
	for _, col := range []string{"offer_id", "target_user_id", "vpn_instance_id", "quantity"} {
		if !pays[col] {
			t.Errorf("payment_records.%s is missing", col)
		}
	}

	var n int
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='vpn_plan_purchases'`).Scan(&n); err != nil {
		t.Fatalf("look up vpn_plan_purchases: %v", err)
	}
	if n != 1 {
		t.Error("table vpn_plan_purchases is missing")
	}
}

// A pre-0008 row must stay readable and keep its documented meaning: no price
// (0), no owner (NULL = unowned inventory), unlimited slots (0) and not
// archived. A migration that defaulted these to anything else would silently
// re-price or hide offers that customers had already seen.
func TestPlanDefaultsKeepLegacyRowsMeaningful(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert exactly as the pre-0008 code did: no new column named at all.
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_offers (kind, name, traffic_gb, duration_days, device_limit, active, created_at, updated_at)
		 VALUES ('PREMIUM', 'Legacy plan', 50, 30, 2, 1, 1, 1)`)
	if err != nil {
		t.Fatalf("insert legacy offer: %v", err)
	}
	id, _ := res.LastInsertId()

	var (
		owner    sql.NullInt64
		price    int64
		currency string
		total    int
		used     int
		archived sql.NullInt64
	)
	if err := st.DB().QueryRowContext(ctx,
		`SELECT owner_user_id, price_cents, currency, slots_total, slots_used, archived_at
		   FROM vpn_offers WHERE id = ?`, id,
	).Scan(&owner, &price, &currency, &total, &used, &archived); err != nil {
		t.Fatalf("read legacy offer: %v", err)
	}
	if owner.Valid {
		t.Error("a legacy offer must be unowned (owner_user_id NULL), so only the Super Admin may release it")
	}
	if price != 0 {
		t.Errorf("price_cents = %d, want 0 (no price recorded)", price)
	}
	if currency != "LKR" {
		t.Errorf("currency = %q, want LKR", currency)
	}
	if total != 0 || used != 0 {
		t.Errorf("slots = %d/%d, want 0/0 (unlimited)", used, total)
	}
	if archived.Valid {
		t.Error("a legacy offer must not be archived")
	}
}

// An order that pre-dates D-031 still names who it was for, so the confirmation
// centre can list old and new orders the same way. The migration backfills
// `target_user_id = payer_user_id` for those rows, and a new insert that omits
// it keeps NULL — which the API layer fills in explicitly.
func TestPaymentTargetDefaultsToNullAndIsReadable(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('payer@example.test', 'x', 'Payer', 'client', 'ACTIVE', 0, 1, 1)`)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	userID, _ := res.LastInsertId()

	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO payment_records (payer_user_id, amount_cents, currency, reference, status, created_at, updated_at)
		 VALUES (?, 1500, 'LKR', 'slip-1', 'PENDING', 1, 1)`, userID); err != nil {
		t.Fatalf("insert payment: %v", err)
	}
	var target sql.NullInt64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT target_user_id FROM payment_records WHERE payer_user_id = ?`, userID).Scan(&target); err != nil {
		t.Fatalf("read payment: %v", err)
	}
	if target.Valid {
		t.Errorf("target_user_id is optional and must stay NULL when omitted, got %d", target.Int64)
	}
}

// A slot row must be attachable to a plan, a buyer and (once used) a VPN, and
// must refuse a row that names a plan which does not exist.
func TestPlanPurchaseRequiresRealPlan(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('seller@example.test', 'x', 'Seller', 'admin', 'ACTIVE', 0, 1, 1)`)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	buyerID, _ := res.LastInsertId()

	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_plan_purchases (offer_id, buyer_user_id, quantity, created_at)
		 VALUES (9999, ?, 1, 1)`, buyerID); err == nil {
		t.Error("a purchase pointing at a missing plan must be rejected by the FK")
	}
}
