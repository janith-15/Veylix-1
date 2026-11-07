package httpapi

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// listerEnv is one migrated database seeded with the accounts a listing test
// needs.
type listerEnv struct {
	db            *sql.DB
	exec          func(string, ...any) int64
	serverID      int64
	clientID      int64
	otherClientID int64
	adminID       int64
}

func newListerEnv(t *testing.T) *listerEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db := st.DB()
	exec := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	return &listerEnv{
		db:   db,
		exec: exec,
		serverID: exec(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
			VALUES ('s','https://p.example/base', x'00','ACTIVE',1,1)`),
		clientID: exec(`INSERT INTO users (email, password_hash, role, status, created_at, updated_at)
			VALUES ('c@example.com','x','client','ACTIVE',1,1)`),
		otherClientID: exec(`INSERT INTO users (email, password_hash, role, status, created_at, updated_at)
			VALUES ('other@example.com','x','client','ACTIVE',1,1)`),
		adminID: exec(`INSERT INTO users (email, password_hash, role, status, created_at, updated_at)
			VALUES ('a@example.com','x','admin','ACTIVE',1,1)`),
	}
}

// D-037.4 / M7-04 — privacy is an OWNERSHIP rule, not a tag rule.
//
// A client must see every VPN it owns: a paid-for VPN disappearing because the
// operator never labelled it would be a lie. It must never see anybody else's.
func TestSQLVPNListerClientSeesOwnVPNs(t *testing.T) {
	ctx := context.Background()
	env := newListerEnv(t)

	// Two VPNs owned by the client: one tagged, one the operator never labelled.
	taggedID := env.exec(`INSERT INTO vpn_instances
		  (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?,?,'c@example.com_free_1','s1','ACTIVE',1,1)`, env.clientID, env.serverID)
	untaggedID := env.exec(`INSERT INTO vpn_instances
		  (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?,?,'c@example.com_paid_1','s2','ACTIVE',1,1)`, env.clientID, env.serverID)
	env.exec(`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_at)
		VALUES (?, 'c@example.com_free_1', 'FREE', 1)`, env.serverID)

	// Somebody else's VPN, tagged, on the same server — must never appear.
	env.exec(`INSERT INTO vpn_instances
		  (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?,?,'other@example.com_paid_1','s3','ACTIVE',1,1)`, env.otherClientID, env.serverID)
	env.exec(`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_at)
		VALUES (?, 'other@example.com_paid_1', 'PREMIUM', 1)`, env.serverID)

	l := NewSQLVPNLister(env.db)
	p := auth.Principal{UserID: env.clientID, Role: auth.RoleClient}

	rows, err := l.ListVPNs(ctx, p, TagFilterAny)
	if err != nil {
		t.Fatalf("ListVPNs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("client sees %d VPNs, want 2 (its own, tagged or not): %+v", len(rows), rows)
	}
	byID := map[int64]VPNRow{}
	for _, r := range rows {
		byID[r.ID] = r
		if r.OwnerID != env.clientID {
			t.Errorf("a row for another owner leaked: %+v", r)
		}
		if r.OwnerEmail != "c@example.com" {
			t.Errorf("owner_email = %q, want the caller's own address", r.OwnerEmail)
		}
		if !r.Assigned {
			t.Error("an owned row must report assigned=true")
		}
	}
	if _, ok := byID[taggedID]; !ok {
		t.Error("the tagged VPN is missing")
	}
	untagged, ok := byID[untaggedID]
	if !ok {
		t.Fatal("the client's own UNTAGGED VPN disappeared — a paid-for VPN must never vanish (M7-04)")
	}
	if untagged.Tag != nil {
		t.Errorf("tag = %v, want nil for an untagged VPN", *untagged.Tag)
	}

	if rows, err := l.ListVPNs(ctx, p, TagFilterFree); err != nil || len(rows) != 1 || rows[0].ID != taggedID {
		t.Fatalf("?tag=free = %d rows (%v), want only the tagged VPN", len(rows), err)
	}
	if rows, err := l.ListVPNs(ctx, p, TagFilterUntagged); err != nil || len(rows) != 1 || rows[0].ID != untaggedID {
		t.Fatalf("?tag=untagged for a client = %d rows (%v), want only its own untagged VPN", len(rows), err)
	}
	if rows, err := l.ListVPNs(ctx, p, TagFilterPremium); err != nil || len(rows) != 0 {
		t.Fatalf("?tag=premium = %d rows (%v), want 0 (another owner's premium must not match)", len(rows), err)
	}
}

// D-025.1 — a reseller sees what he sold, but never an unclassified VPN.
func TestSQLVPNListerAdminNeverSeesUntagged(t *testing.T) {
	ctx := context.Background()
	env := newListerEnv(t)

	sold := env.exec(`INSERT INTO vpn_instances
		  (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?,?,'c@example.com_paid_1','s1','ACTIVE',1,1)`, env.clientID, env.serverID)
	env.exec(`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, status, started_at, created_at, updated_at)
		VALUES (?,?,?,'ACTIVE',1,1,1)`, sold, env.clientID, env.adminID)
	env.exec(`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_at)
		VALUES (?, 'c@example.com_paid_1', 'PREMIUM', 1)`, env.serverID)

	// Sale number two, from the same reseller, that the operator has not classified.
	unsold := env.exec(`INSERT INTO vpn_instances
		  (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?,?,'c@example.com_paid_2','s2','ACTIVE',1,1)`, env.clientID, env.serverID)
	env.exec(`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, status, started_at, created_at, updated_at)
		VALUES (?,?,?,'ACTIVE',1,1,1)`, unsold, env.clientID, env.adminID)

	l := NewSQLVPNLister(env.db)
	p := auth.Principal{UserID: env.adminID, Role: auth.RoleAdmin}

	rows, err := l.ListVPNs(ctx, p, TagFilterAny)
	if err != nil {
		t.Fatalf("ListVPNs: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != sold {
		t.Fatalf("admin sees %+v, want only the tagged sale", rows)
	}
	if rows[0].SellerID != env.adminID {
		t.Errorf("seller_user_id = %d, want the caller", rows[0].SellerID)
	}
	if rows, err := l.ListVPNs(ctx, p, TagFilterUntagged); err != nil || len(rows) != 0 {
		t.Fatalf("?tag=untagged for an admin = %d rows (%v), want 0", len(rows), err)
	}
}
