package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedClient(t *testing.T, st *Store, email string) int64 {
	t.Helper()
	res, err := st.DB().Exec(
		`INSERT INTO users (email, role, status, created_at, updated_at)
		 VALUES (?, 'client', 'ACTIVE', 1, 1)`, email)
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func emailN(i int) string {
	return "free" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + "@panel.test"
}

// D-025.4: a client may hold at most 3 FREE VPNs per server at once.
func TestFreeConcurrentCapIsThree(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)
	clientID := seedClient(t, st, "cap@example.com")

	for i := 0; i < FreeConcurrentCap; i++ {
		if _, err := st.RecordFreeClaim(ctx, nil, clientID, serverID, emailN(i), FreeMonthlyDefault); err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
	}
	if _, err := st.RecordFreeClaim(ctx, nil, clientID, serverID, emailN(99), FreeMonthlyDefault); !errors.Is(err, ErrFreeConcurrentLimit) {
		t.Fatalf("4th claim = %v, want ErrFreeConcurrentLimit", err)
	}

	usage, err := st.FreeUsage(ctx, clientID, serverID, FreeMonthlyDefault)
	if err != nil {
		t.Fatalf("FreeUsage: %v", err)
	}
	if usage.Concurrent != 3 || usage.ConcurrentLimit != 3 {
		t.Errorf("usage = %+v, want 3/3", usage)
	}
}

// D-025.5: a released claim does NOT refund a monthly slot.
func TestReleaseDoesNotRefundMonthlySlot(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)
	clientID := seedClient(t, st, "monthly@example.com")

	for i := 0; i < 3; i++ {
		e := emailN(i)
		if _, err := st.RecordFreeClaim(ctx, nil, clientID, serverID, e, 3); err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if err := st.ReleaseFreeClaim(ctx, clientID, serverID, e); err != nil {
			t.Fatalf("release %d: %v", i+1, err)
		}
	}

	usage, err := st.FreeUsage(ctx, clientID, serverID, 3)
	if err != nil {
		t.Fatalf("FreeUsage: %v", err)
	}
	if usage.Concurrent != 0 {
		t.Errorf("concurrent = %d, want 0 after releasing", usage.Concurrent)
	}
	if usage.UsedThisWindow != 3 {
		t.Errorf("used this window = %d, want 3 — releasing must not refund", usage.UsedThisWindow)
	}
	if _, err := st.RecordFreeClaim(ctx, nil, clientID, serverID, emailN(50), 3); !errors.Is(err, ErrFreeMonthlyLimit) {
		t.Fatalf("claim after 3 released = %v, want ErrFreeMonthlyLimit", err)
	}
}

// A claim older than the rolling window no longer counts.
func TestMonthlyWindowRollsOver(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)
	clientID := seedClient(t, st, "window@example.com")

	old := time.Now().Add(-31 * 24 * time.Hour).Unix()
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO free_claims (client_user_id, server_id, panel_email, claimed_at, released_at)
		 VALUES (?, ?, 'old@panel.test', ?, ?)`, clientID, serverID, old, old); err != nil {
		t.Fatalf("seed old claim: %v", err)
	}

	usage, err := st.FreeUsage(ctx, clientID, serverID, 1)
	if err != nil {
		t.Fatalf("FreeUsage: %v", err)
	}
	if usage.UsedThisWindow != 0 {
		t.Errorf("used this window = %d, want 0 — a 31-day-old claim is outside the window", usage.UsedThisWindow)
	}
	if _, err := st.RecordFreeClaim(ctx, nil, clientID, serverID, emailN(1), 1); err != nil {
		t.Fatalf("a new claim must be allowed once the window rolled: %v", err)
	}
}

// The caps are per (client, server): another server is a separate allowance.
func TestFreeCapsArePerServer(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverA := seedServer(t, st)
	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv2', 'https://panel2.example:2053/b', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed server 2: %v", err)
	}
	serverB, _ := res.LastInsertId()
	clientID := seedClient(t, st, "per@example.com")

	for i := 0; i < FreeConcurrentCap; i++ {
		if _, err := st.RecordFreeClaim(ctx, nil, clientID, serverA, emailN(i), FreeMonthlyDefault); err != nil {
			t.Fatalf("server A claim %d: %v", i+1, err)
		}
	}
	if _, err := st.RecordFreeClaim(ctx, nil, clientID, serverB, emailN(77), FreeMonthlyDefault); err != nil {
		t.Fatalf("server B must have its own allowance: %v", err)
	}
}
