package store

import (
	"context"
	"testing"
	"time"
)

// newStaffStore seeds two admins, one super admin and one client, plus a server
// assignment, so the staff filters describe something real.
func newStaffStore(t *testing.T) (*Store, map[string]int64) {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir()+"/staff.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ids := map[string]int64{}
	mk := func(key, email, name, role, status string, quota int) {
		res, err := st.DB().Exec(
			`INSERT INTO users (email, username, password_hash, display_name, role, status,
			                    email_verified, quota_limit, created_at, updated_at)
			 VALUES (?, ?, 'x', ?, ?, ?, 1, ?, ?, ?)`,
			email, key, name, role, status, quota, time.Now().Unix(), time.Now().Unix())
		if err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
		id, _ := res.LastInsertId()
		ids[key] = id
	}
	mk("owner", "owner@example.com", "Owner", "super_admin", "ACTIVE", 0)
	mk("reseller", "reseller@example.com", "Reseller", "admin", "ACTIVE", 25)
	mk("reseller2", "reseller2@example.com", "Second Reseller", "admin", "SUSPENDED", 5)
	mk("customer", "customer@example.com", "Customer", "client", "ACTIVE", 0)

	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv', 'https://p.example/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	server, _ := res.LastInsertId()
	ids["server"] = server
	if _, err := st.DB().Exec(
		`INSERT INTO admin_server_assignments (admin_id, server_id, created_at) VALUES (?, ?, 1)`,
		ids["reseller"], server); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	return st, ids
}

// The staff list contains STAFF ONLY — a client must never appear there, because
// it is the console that hands out admin power.
func TestListStaffIsStaffOnly(t *testing.T) {
	ctx := context.Background()
	st, ids := newStaffStore(t)

	rows, total, err := st.ListStaff(ctx, StaffFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("total=%d rows=%d, want the 3 staff accounts", total, len(rows))
	}
	for _, r := range rows {
		if r.Role == "client" {
			t.Fatalf("a client (%s) leaked into the staff list", r.Email)
		}
		if r.ID == ids["customer"] {
			t.Fatal("the customer account leaked into the staff list")
		}
	}
}

// Every filter narrows the set, and they compose.
func TestListStaffFilters(t *testing.T) {
	ctx := context.Background()
	st, ids := newStaffStore(t)

	cases := []struct {
		name string
		f    StaffFilter
		want int
	}{
		{"no filter", StaffFilter{}, 3},
		{"role admin", StaffFilter{Role: "admin"}, 2},
		{"role super_admin", StaffFilter{Role: "super_admin"}, 1},
		{"status ACTIVE", StaffFilter{Status: "ACTIVE"}, 2},
		{"status SUSPENDED", StaffFilter{Status: "SUSPENDED"}, 1},
		{"search by name", StaffFilter{Q: "Second"}, 1},
		{"search by e-mail", StaffFilter{Q: "owner@"}, 1},
		{"quota_min 10", StaffFilter{QuotaMin: 10}, 1},
		{"quota_max 5", StaffFilter{QuotaMax: 5}, 1},
		{"assigned to the server", StaffFilter{ServerID: ids["server"]}, 1},
		{"composed: admin + suspended", StaffFilter{Role: "admin", Status: "SUSPENDED"}, 1},
	}
	for _, c := range cases {
		_, got, err := st.ListStaff(ctx, c.f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: total = %d, want %d", c.name, got, c.want)
		}
	}
}

// The detail names the reseller's own book and their panel scope.
func TestStaffDetailNamesTheBook(t *testing.T) {
	ctx := context.Background()
	st, ids := newStaffStore(t)

	d, err := st.StaffDetailFor(ctx, ids["reseller"])
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.Profile.Role != "admin" {
		t.Errorf("role = %s, want admin", d.Profile.Role)
	}
	if len(d.Servers) != 1 || d.Servers[0].ID != ids["server"] {
		t.Errorf("servers = %+v, want the assigned server", d.Servers)
	}
	if d.Profile.ServerIDs == nil {
		t.Error("server_ids must be an empty slice, never nil, so the UI can rely on it")
	}

	// A client is NOT staff: opening one here must fail.
	if _, err := st.StaffDetailFor(ctx, ids["customer"]); err == nil {
		t.Fatal("the detail returned a record for a client account")
	}
	if _, err := st.StaffDetailFor(ctx, 99999); err == nil {
		t.Fatal("the detail returned a record for an unknown id")
	}
}

// Quota used counts the reseller's sales, which is the number the ceiling is
// measured against.
func TestStaffQuotaUsedCountsSales(t *testing.T) {
	ctx := context.Background()
	st, ids := newStaffStore(t)

	res, err := st.DB().Exec(
		`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		 VALUES (?, ?, 'customer_paid_1', 'sub1', 'ACTIVE', 1, 1)`, ids["customer"], ids["server"])
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	vpn, _ := res.LastInsertId()
	if _, err := st.DB().Exec(
		`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, created_at, updated_at)
		 VALUES (?, ?, ?, 1, 1)`, vpn, ids["customer"], ids["reseller"]); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	rows, _, err := st.ListStaff(ctx, StaffFilter{Role: "admin"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rows {
		if r.ID == ids["reseller"] && r.QuotaUsed != 1 {
			t.Errorf("reseller quota_used = %d, want 1", r.QuotaUsed)
		}
		if r.ID == ids["reseller2"] && r.QuotaUsed != 0 {
			t.Errorf("the second reseller sold nothing, quota_used = %d", r.QuotaUsed)
		}
	}
}
