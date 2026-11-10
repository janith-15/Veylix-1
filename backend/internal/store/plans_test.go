package store

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
)

// seedPlan inserts a PREMIUM plan and returns its id. Each call seeds its own
// panel row, so a test may create several plans.
func seedPlan(t *testing.T, ctx context.Context, st *Store, owner *int64, slots int, price int64) int64 {
	t.Helper()
	var serverID int64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT id FROM server_integrations ORDER BY id LIMIT 1`).Scan(&serverID); err != nil {
		serverID = seedServer(t, st)
	}
	p, err := st.CreatePlan(ctx, CreatePlanInput{
		Kind: "PREMIUM", Name: "Premium 50", ServerID: serverID,
		TrafficGB: 50, DurationDays: 30, DeviceLimit: 2, Active: true,
		OwnerUserID: owner, PriceCents: price, SlotsTotal: slots,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return p.ID
}

func seedUser(t *testing.T, ctx context.Context, st *Store, email, role string) int64 {
	t.Helper()
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES (?, 'x', 'Seed', ?, 'ACTIVE', 0, 1, 1)`, email, role)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// D-030.4: the last slot can be taken exactly once, even when two releases run
// at the same instant. This is the property the slot inventory exists for — a
// check-then-write would let both callers through.
func TestClaimPlanSlotIsAtomic(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	planID := seedPlan(t, ctx, st, nil, 1, 0)
	buyer := seedUser(t, ctx, st, "buyer@example.test", "admin")

	const racers = 2
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
		exhausted int
		other     []error
	)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := st.Tx(ctx, func(tx *sql.Tx) error {
				_, cerr := st.ClaimPlanSlot(ctx, tx, planID, buyer, 0)
				return cerr
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, ErrPlanExhausted):
				exhausted++
			default:
				other = append(other, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	for _, err := range other {
		t.Errorf("unexpected error: %v", err)
	}
	if succeeded != 1 {
		t.Errorf("succeeded = %d, want exactly 1 (the plan had one slot)", succeeded)
	}
	if exhausted != 1 {
		t.Errorf("refused = %d, want exactly 1", exhausted)
	}

	p, err := st.PlanByID(ctx, planID)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if p.SlotsUsed != 1 || p.SlotsRemaining != 0 {
		t.Errorf("slots = used %d / remaining %d, want 1 / 0", p.SlotsUsed, p.SlotsRemaining)
	}
}

// An unlimited plan (slots_total 0) keeps the legacy meaning: it never refuses
// and never writes a slot row.
func TestClaimPlanSlotUnlimited(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	planID := seedPlan(t, ctx, st, nil, 0, 0)

	var purchaseID int64
	if err := st.Tx(ctx, func(tx *sql.Tx) error {
		id, err := st.ClaimPlanSlot(ctx, tx, planID, 0, 0)
		purchaseID = id
		return err
	}); err != nil {
		t.Fatalf("claim on an unlimited plan must succeed: %v", err)
	}
	if purchaseID != 0 {
		t.Errorf("purchase row = %d, want 0 (nothing to reserve)", purchaseID)
	}

	var rows int
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_plan_purchases WHERE offer_id = ?`, planID).Scan(&rows); err != nil {
		t.Fatalf("count purchases: %v", err)
	}
	if rows != 0 {
		t.Errorf("purchase rows = %d, want 0", rows)
	}
}

// D-030.2 and D-030.3: ownership, not the tag, decides who may release a plan.
func TestCheckPlanRelease(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ownerID := seedUser(t, ctx, st, "owner@example.test", "admin")
	otherID := seedUser(t, ctx, st, "other@example.test", "admin")

	unowned, err := st.PlanByID(ctx, seedPlan(t, ctx, st, nil, 5, 1000))
	if err != nil {
		t.Fatalf("read unowned: %v", err)
	}
	owned, err := st.PlanByID(ctx, seedPlan(t, ctx, st, &ownerID, 5, 1000))
	if err != nil {
		t.Fatalf("read owned: %v", err)
	}

	// An unowned premium plan needs a Super-Admin confirmation, whoever asks.
	for _, caller := range []struct {
		id   int64
		role string
	}{{ownerID, "admin"}, {otherID, "admin"}} {
		d, err := st.CheckPlanRelease(unowned, caller.id, caller.role)
		if err != nil {
			t.Fatalf("unowned plan must not error for %s: %v", caller.role, err)
		}
		if !d.NeedsSuperAdminConfirm {
			t.Errorf("unowned plan for %s: NeedsSuperAdminConfirm = false, want true", caller.role)
		}
	}

	// The owner releases his own plan with no approval.
	d, err := st.CheckPlanRelease(owned, ownerID, "admin")
	if err != nil {
		t.Fatalf("owner release: %v", err)
	}
	if d.NeedsSuperAdminConfirm {
		t.Error("an admin's own plan must not need a confirmation (D-030.2)")
	}

	// Another reseller may not release it.
	if _, err := st.CheckPlanRelease(owned, otherID, "admin"); !errors.Is(err, ErrNotPlanOwner) {
		t.Errorf("foreign release error = %v, want ErrNotPlanOwner", err)
	}

	// The Super Admin may release anything.
	if _, err := st.CheckPlanRelease(owned, 99, "super_admin"); err != nil {
		t.Errorf("super admin release: %v", err)
	}
}

// A FREE plan is never gated by ownership — it is public and capped by D-025.
func TestCheckPlanReleaseFreeIsUngated(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)
	p, err := st.CreatePlan(ctx, CreatePlanInput{
		Kind: "FREE", Name: "Free 10", ServerID: serverID, TrafficGB: 10,
		DurationDays: 30, Active: true,
	})
	if err != nil {
		t.Fatalf("create free plan: %v", err)
	}
	d, err := st.CheckPlanRelease(p, 7, "client")
	if err != nil {
		t.Fatalf("free plan must not be gated: %v", err)
	}
	if d.NeedsSuperAdminConfirm {
		t.Error("a FREE plan must never need a Super-Admin confirmation")
	}
}
