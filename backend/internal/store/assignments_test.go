package store

import (
	"context"
	"testing"
)

// M6-07: a reseller's client list must count only the VPNs HE sold. Before
// 2026-10-25 the count was unscoped, so one sale by another reseller inflated
// the number this one saw for the same customer.
func TestClientVPNCountIsSellerScoped(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)

	sellerA := seedUser(t, ctx, st, "seller-a@example.test", "admin")
	sellerB := seedUser(t, ctx, st, "seller-b@example.test", "admin")
	customer := seedUser(t, ctx, st, "customer@example.test", "client")

	// Seller A sold the customer one VPN; seller B sold two.
	soldBy := map[int64]int{sellerA: 1, sellerB: 2}
	for seller, n := range soldBy {
		for i := 0; i < n; i++ {
			email := "panel-" + itoa(seller) + "-" + itoa(int64(i)) + "@panel.test"
			res, err := st.DB().ExecContext(ctx,
				`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
				 VALUES (?, ?, ?, ?, 'ACTIVE', 1, 1)`, customer, serverID, email, "sub-"+email)
			if err != nil {
				t.Fatalf("insert vpn: %v", err)
			}
			vpnID, _ := res.LastInsertId()
			if _, err := st.DB().ExecContext(ctx,
				`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, status, started_at, created_at, updated_at)
				 VALUES (?, ?, ?, 'ACTIVE', 1, 1, 1)`, vpnID, customer, seller); err != nil {
				t.Fatalf("insert assignment: %v", err)
			}
		}
	}

	for seller, want := range soldBy {
		rows, err := st.ClientsVisible(ctx, seller, "admin")
		if err != nil {
			t.Fatalf("ClientsVisible: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("seller %d sees %d clients, want 1", seller, len(rows))
		}
		if rows[0].VPNCount != want {
			t.Errorf("seller %d sees vpn_count %d, want %d (only his own sales)", seller, rows[0].VPNCount, want)
		}
	}

	// The Super Admin still sees every VPN the customer owns.
	all, err := st.ClientsVisible(ctx, 0, "super_admin")
	if err != nil {
		t.Fatalf("super admin ClientsVisible: %v", err)
	}
	if len(all) != 1 || all[0].VPNCount != 3 {
		t.Errorf("super admin sees %+v, want one client with 3 VPNs", all)
	}
}

// itoa avoids importing strconv into a short test helper.
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := make([]byte, 0, 8)
	for v > 0 {
		buf = append([]byte{byte('0' + v%10)}, buf...)
		v /= 10
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
}
