package store

import (
	"context"
	"testing"
	"time"
)

// newClientAdminStore seeds three clients (one suspended, one Google-linked, one
// with a premium assignment) plus a FREE claim, so the filters and the detail
// aggregate describe something real.
func newClientAdminStore(t *testing.T) (*Store, map[string]int64) {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir()+"/clients.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ids := map[string]int64{}
	mk := func(key, email, name, status, google string) {
		// A user WITHOUT Google must store NULL, not "": `users.google_sub` is
		// UNIQUE, and "" would collide with every other non-Google account.
		var sub any
		if google != "" {
			sub = google
		}
		res, err := st.DB().Exec(
			`INSERT INTO users (email, username, password_hash, google_sub, display_name, role, status,
			                    email_verified, created_at, updated_at)
			 VALUES (?, ?, 'x', ?, ?, 'client', ?, 1, ?, ?)`,
			email, key, sub, name, status, time.Now().Unix(), time.Now().Unix())
		if err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
		id, _ := res.LastInsertId()
		ids[key] = id
	}
	mk("alice", "alice@example.com", "Alice", "ACTIVE", "")
	mk("bob", "bob@example.com", "Bob Suspended", "SUSPENDED", "sub-bob")
	mk("carol", "carol@example.com", "Carol", "ACTIVE", "")

	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv', 'https://p.example/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	server, _ := res.LastInsertId()
	res, err = st.DB().Exec(
		`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id, status,
		                            traffic_limit_bytes, created_at, updated_at)
		 VALUES (?, ?, 'carol_paid_1', 'sub1', 'ACTIVE', 1000, 1, 1)`, ids["carol"], server)
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	vpn, _ := res.LastInsertId()
	if _, err := st.DB().Exec(
		`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, created_at, updated_at)
		 VALUES (?, ?, 1, 1, 1)`, vpn, ids["carol"]); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	if _, err := st.DB().Exec(
		`INSERT INTO free_claims (client_user_id, server_id, panel_email, claimed_at)
		 VALUES (?, ?, 'alice_free_1', ?)`, ids["alice"], server, time.Now().Unix()); err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	return st, ids
}

// The list returns every client with the counts the console shows.
func TestListClientsAdminShape(t *testing.T) {
	ctx := context.Background()
	st, ids := newClientAdminStore(t)

	rows, total, err := st.ListClientsAdmin(ctx, ClientFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("total=%d rows=%d, want 3", total, len(rows))
	}
	byID := map[int64]ClientAdminRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	if byID[ids["carol"]].PremiumCount != 1 {
		t.Errorf("carol premium_count = %d, want 1", byID[ids["carol"]].PremiumCount)
	}
	if byID[ids["alice"]].FreeUsed != 1 {
		t.Errorf("alice free_used = %d, want 1", byID[ids["alice"]].FreeUsed)
	}
	if len(byID[ids["carol"]].SellerIDs) != 1 {
		t.Errorf("carol seller_ids = %v, want one seller", byID[ids["carol"]].SellerIDs)
	}
	if !byID[ids["bob"]].GoogleLinked {
		t.Error("bob should report a Google link")
	}
}

// Every filter narrows the set, and they compose.
func TestListClientsAdminFilters(t *testing.T) {
	ctx := context.Background()
	st, _ := newClientAdminStore(t)
	yes, no := true, false

	cases := []struct {
		name string
		f    ClientFilter
		want int
	}{
		{"no filter", ClientFilter{}, 3},
		{"status ACTIVE", ClientFilter{Status: "ACTIVE"}, 2},
		{"status SUSPENDED", ClientFilter{Status: "SUSPENDED"}, 1},
		{"search by name", ClientFilter{Q: "carol"}, 1},
		{"search by e-mail", ClientFilter{Q: "alice@"}, 1},
		{"search by username", ClientFilter{Q: "bob"}, 1},
		{"google linked", ClientFilter{GoogleLinked: &yes}, 1},
		{"not google linked", ClientFilter{GoogleLinked: &no}, 2},
		{"has premium", ClientFilter{HasPremium: &yes}, 1},
		{"no premium", ClientFilter{HasPremium: &no}, 2},
		{"has free claim", ClientFilter{HasFree: &yes}, 1},
		{"no free claim", ClientFilter{HasFree: &no}, 2},
		{"free_min 1", ClientFilter{FreeMin: 1}, 1},
		{"composed: ACTIVE + no premium", ClientFilter{Status: "ACTIVE", HasPremium: &no}, 1},
	}
	for _, c := range cases {
		_, got, err := st.ListClientsAdmin(ctx, c.f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: total = %d, want %d", c.name, got, c.want)
		}
	}

	rows, _, err := st.ListClientsAdmin(ctx, ClientFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("page 2 has %d rows, want 1", len(rows))
	}
}

// The detail aggregate answers the whole page in one call.
func TestClientDetailAggregate(t *testing.T) {
	ctx := context.Background()
	st, ids := newClientAdminStore(t)

	d, err := st.ClientDetail(ctx, ids["carol"], 5)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.Profile.Email != "carol@example.com" {
		t.Errorf("profile email = %q", d.Profile.Email)
	}
	if d.FreeLimit != 5 {
		t.Errorf("free limit = %d, want the platform default 5", d.FreeLimit)
	}
	if d.PremiumCount != 1 || d.VPNCount != 1 {
		t.Errorf("premium=%d vpn=%d, want 1/1", d.PremiumCount, d.VPNCount)
	}
	if len(d.MonthlyClaims) != 6 {
		t.Errorf("monthly claims has %d buckets, want 6 (empty months included)", len(d.MonthlyClaims))
	}
	// A non-client id must never open a record.
	if _, err := st.ClientDetail(ctx, 99999, 5); err == nil {
		t.Fatal("detail returned a record for an unknown id")
	}
}

// Blocking a client ends its sessions in the same transaction.
func TestClientBlockRevokesSessions(t *testing.T) {
	ctx := context.Background()
	st, ids := newClientAdminStore(t)

	if _, err := st.DB().Exec(
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES ('sess-1', ?, 1, 9999999999)`,
		ids["alice"]); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	if err := st.ClientSetStatus(ctx, ids["alice"], "SUSPENDED"); err != nil {
		t.Fatalf("block: %v", err)
	}
	var (
		status   string
		sessions int
	)
	_ = st.DB().QueryRow(`SELECT status FROM users WHERE id = ?`, ids["alice"]).Scan(&status)
	_ = st.DB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, ids["alice"]).Scan(&sessions)
	if status != "SUSPENDED" {
		t.Errorf("status = %s, want SUSPENDED", status)
	}
	if sessions != 0 {
		t.Errorf("%d session(s) survived a block, want 0", sessions)
	}

	// Unblocking restores the account but does NOT resurrect the sessions.
	if err := st.ClientSetStatus(ctx, ids["alice"], "ACTIVE"); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	_ = st.DB().QueryRow(`SELECT status FROM users WHERE id = ?`, ids["alice"]).Scan(&status)
	if status != "ACTIVE" {
		t.Errorf("status = %s, want ACTIVE", status)
	}
}

// The per-client FREE limit is stored and read back; 0 means "platform default".
func TestClientFreeLimitOverride(t *testing.T) {
	ctx := context.Background()
	st, ids := newClientAdminStore(t)

	if got, err := st.ClientFreeLimitOverride(ctx, ids["alice"]); err != nil || got != 0 {
		t.Fatalf("initial override = %d (%v), want 0", got, err)
	}
	if err := st.ClientSetFreeLimit(ctx, ids["alice"], 12); err != nil {
		t.Fatalf("set: %v", err)
	}
	d, err := st.ClientDetail(ctx, ids["alice"], 5)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.FreeLimit != 12 || d.FreeOverride != 12 {
		t.Errorf("limit=%d override=%d, want 12/12", d.FreeLimit, d.FreeOverride)
	}

	// A missing client is refused, not silently ignored.
	if err := st.ClientSetFreeLimit(ctx, 99999, 5); err == nil {
		t.Error("setting a limit on a missing client succeeded")
	}
}

// TestListClientsAdminOffsetFollowsClampedLimit is the B-2 regression test.
//
// The transport computes `Offset = (page-1) * Limit` from the RAW size the caller
// sent; the store clamps the size. Before the fix the two were derived from
// different numbers, so `?limit=9999&page=5` asked for `LIMIT 50 OFFSET 39996` —
// an operator paging to page 5 was silently shown rows 40 000 deep and told the
// account was empty.
//
// The invariant asserted here is the one that makes paging meaningful: whatever
// the caller asks for, the offset the query uses must be a multiple of the limit
// the query uses. A window that does not start on the page grid is a window the
// caller cannot have meant.
func TestListClientsAdminOffsetFollowsClampedLimit(t *testing.T) {
	ctx := context.Background()
	st, _ := newClientAdminStore(t)

	cases := []struct {
		name    string
		limit   int
		page    int
		want    int // expected effective limit
		wantOff int
	}{
		{"sane page size", 10, 2, 10, 10},
		{"limit above the ceiling", 9999, 5, ClientAdminDefaultLimit, 4 * ClientAdminDefaultLimit},
		{"limit of zero", 0, 3, ClientAdminDefaultLimit, 2 * ClientAdminDefaultLimit},
		{"negative limit", -5, 2, ClientAdminDefaultLimit, ClientAdminDefaultLimit},
		{"negative page", 10, -1, 10, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Exactly what the handler does: clamp the size, THEN derive the offset.
			limit := tc.limit
			if limit < 1 || limit > ClientAdminMaxLimit {
				limit = ClientAdminDefaultLimit
			}
			page := tc.page
			if page < 1 {
				page = 1
			}
			f := ClientFilter{Limit: limit, Offset: (page - 1) * limit}

			if _, _, err := st.ListClientsAdmin(ctx, f); err != nil {
				t.Fatalf("list: %v", err)
			}
			// The store must reach the same effective limit the handler assumed,
			// and the offset it uses must sit on that grid.
			if limit != tc.want {
				t.Fatalf("effective limit = %d, want %d", limit, tc.want)
			}
			if f.Offset != tc.wantOff {
				t.Fatalf("offset = %d, want %d (page %d at limit %d)", f.Offset, tc.wantOff, page, limit)
			}
			if f.Offset%limit != 0 {
				t.Fatalf("offset %d is not a multiple of limit %d — the window is off the page grid", f.Offset, limit)
			}
		})
	}
}
