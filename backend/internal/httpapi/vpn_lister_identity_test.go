package httpapi

import (
	"context"
	"testing"

	"veylix/backend/internal/auth"
)

// D-083 — a VPN row must NAME its server and its seller, not just number them.
//
// `server_id` and `seller_user_id` are integers a customer cannot recognise, so
// the row carries the operator's own server label (`server_integrations.name`)
// and the seller's configured business contacts (Bible §2.3). Before this the
// client screen could do no better than "server 3" and "Sold by user #3".
//
// This is the SQL path, so it also proves the two LEFT JOINs are correct: a
// wrong join would silently null the name rather than fail.
func TestSQLVPNListerNamesServerAndSeller(t *testing.T) {
	ctx := context.Background()
	env := newListerEnv(t)

	// The operator's label for the server, and the seller's own profile — the
	// same values the payment destination already publishes (D-036.2).
	env.exec(`UPDATE server_integrations SET name = 'indian-1' WHERE id = ?`, env.serverID)
	env.exec(`UPDATE users SET display_name = 'Nimal Reseller', whatsapp = '+94771234567',
		telegram = '@nimalvpn' WHERE id = ?`, env.adminID)

	vpnID := env.exec(`INSERT INTO vpn_instances
		  (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?,?,'c@example.com_paid_1','s1','ACTIVE',1,1)`, env.clientID, env.serverID)
	env.exec(`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, status, started_at, created_at, updated_at)
		VALUES (?,?,?,'ACTIVE',1,1,1)`, vpnID, env.clientID, env.adminID)

	l := NewSQLVPNLister(env.db)
	rows, err := l.ListVPNs(ctx, auth.Principal{UserID: env.clientID, Role: auth.RoleClient}, TagFilterAny)
	if err != nil {
		t.Fatalf("ListVPNs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("client sees %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.ServerName != "indian-1" {
		t.Errorf("server_name = %q, want the operator's label - a bare server id is not a name", r.ServerName)
	}
	if r.SellerID != env.adminID {
		t.Fatalf("seller_user_id = %d, want %d", r.SellerID, env.adminID)
	}
	if r.SellerName != "Nimal Reseller" {
		t.Errorf("seller_name = %q, want the seller's display name", r.SellerName)
	}
	if r.SellerEmail != "a@example.com" {
		t.Errorf("seller_email = %q, want the seller's address", r.SellerEmail)
	}
	if r.SellerWhatsApp != "+94771234567" {
		t.Errorf("seller_whatsapp = %q", r.SellerWhatsApp)
	}
	if r.SellerTelegram != "@nimalvpn" {
		t.Errorf("seller_telegram = %q", r.SellerTelegram)
	}
}

// A VPN nobody sold (the platform issued it directly) must not invent a seller:
// empty strings, not the Super Admin's own profile. "No seller" is a fact the
// screen states, and a fabricated one would send a client to the wrong person.
func TestSQLVPNListerHasNoSellerWhenNoneAssigned(t *testing.T) {
	ctx := context.Background()
	env := newListerEnv(t)

	env.exec(`INSERT INTO vpn_instances
		  (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?,?,'c@example.com_free_1','s1','ACTIVE',1,1)`, env.clientID, env.serverID)

	l := NewSQLVPNLister(env.db)
	rows, err := l.ListVPNs(ctx, auth.Principal{UserID: env.clientID, Role: auth.RoleClient}, TagFilterAny)
	if err != nil {
		t.Fatalf("ListVPNs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("client sees %d rows, want 1", len(rows))
	}
	if rows[0].SellerID != 0 || rows[0].SellerName != "" || rows[0].SellerWhatsApp != "" {
		t.Errorf("an unassigned VPN invented a seller: %+v", rows[0])
	}
}
