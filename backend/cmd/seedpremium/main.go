// Command seedpremium gives the client test account ONE PREMIUM VPN.
//
// Why this exists
// ---------------
// The browser gates for the client charts cannot prove anything against a FREE
// VPN: `VpnUsageSection` renders charts only when `kind !== 'FREE'`, so with three
// FREE VPNs on the test account the range switch and every chart probe is a
// stated SKIP rather than a pass. That gap blocked two QA rows, not because the
// code was wrong but because there was nothing to look at.
//
// This tool closes it honestly: it seeds a REAL row in Veylix's own tables, with
// the same slot naming the provisioning path uses (D-091.4), so the screens are
// exercised against data shaped exactly like production.
//
// What it deliberately does NOT do
// --------------------------------
//   - It does not create a 3X-UI client. Seed data is local only, which is why
//     the VPN it writes is explicitly a DEV FIXTURE and prints that it is.
//   - It does not touch an existing row. It adds one offer assignment and one
//     VPN, and refuses if that VPN already exists (idempotent by panel_email).
//   - It does not delete, truncate or rewrite anything.
//
// Usage:
//
//	go run ./cmd/seedpremium --dry-run   # show the plan, change nothing
//	go run ./cmd/seedpremium             # apply
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"veylix/backend/internal/config"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seedpremium: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	clientEmail := flag.String("email", "client@veylix.test",
		"the client account that receives the PREMIUM VPN")
	offerID := flag.Int64("offer", 20, "the PREMIUM offer to attach (id in vpn_offers)")
	dryRun := flag.Bool("dry-run", false, "print what would be created, change nothing")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", cfg.DBPath, err)
	}
	defer func() { _ = st.Close() }()
	db := st.DB()

	// 1. Resolve the owner. A seed that silently creates an account would be a
	//    bigger change than the one that was asked for.
	var ownerID int64
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = ?`, *clientEmail).Scan(&ownerID); err != nil {
		return fmt.Errorf("no user %q: %w", *clientEmail, err)
	}

	// 2. Resolve the offer and require it to be a usable PREMIUM one. Attaching a
	//    VPN to an inactive or FREE offer would produce a screen that lies about
	//    the package it is showing.
	var (
		kind, name      string
		serverID        int64
		inboundID       int64
		trafficGB, durD int64
		deviceLimit     int64
		active          int
	)
	if err := db.QueryRowContext(ctx,
		`SELECT kind, name, COALESCE(server_id,0), COALESCE(inbound_id,0),
		        traffic_gb, duration_days, device_limit, active
		   FROM vpn_offers WHERE id = ?`, *offerID).
		Scan(&kind, &name, &serverID, &inboundID, &trafficGB, &durD, &deviceLimit, &active); err != nil {
		return fmt.Errorf("no offer %d: %w", *offerID, err)
	}
	if kind != "PREMIUM" {
		return fmt.Errorf("offer %d is %s, not PREMIUM — this tool only seeds PREMIUM", *offerID, kind)
	}
	if active != 1 {
		return fmt.Errorf("offer %d (%s) is inactive, so no client could buy it", *offerID, name)
	}
	if serverID == 0 {
		return fmt.Errorf("offer %d has no server, so it names no panel", *offerID)
	}

	// 3. Name it the way provisioning does: <email>_paid_<n> (D-091.4). The slot
	//    only moves forward, so this never reuses a name a released VPN held.
	base := domain.PanelBase(*clientEmail, "paid")
	panelEmail := ""
	for slot := 1; slot <= 99; slot++ {
		candidate := fmt.Sprintf("%s_%d", base, slot)
		var seen string
		err := db.QueryRowContext(ctx,
			`SELECT panel_email FROM vpn_instances WHERE server_id = ? AND panel_email = ?`,
			serverID, candidate).Scan(&seen)
		if err != nil {
			panelEmail = candidate // no row → slot is free
			break
		}
	}
	if panelEmail == "" {
		return fmt.Errorf("could not find a free slot for %q", base)
	}

	// Idempotent: if this account already holds a PREMIUM VPN on this server, the
	// gate it unblocks is already satisfied and a second row would be noise.
	var held int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances v
		   JOIN vpn_offers o ON o.id = v.offer_id
		  WHERE v.owner_user_id = ? AND o.kind = 'PREMIUM' AND v.server_id = ?`,
		ownerID, serverID).Scan(&held); err != nil {
		return err
	}

	now := time.Now().Unix()
	var expiresAt int64
	if durD > 0 {
		expiresAt = now + durD*86400
	}
	trafficBytes := trafficGB * 1024 * 1024 * 1024

	fmt.Printf("database    : %s\n", cfg.DBPath)
	fmt.Printf("owner       : %s (user %d)\n", *clientEmail, ownerID)
	fmt.Printf("offer       : %d  %q  %d GB / %d days\n", *offerID, name, trafficGB, durD)
	fmt.Printf("panel email : %s\n", panelEmail)
	fmt.Printf("expires_at  : %d\n", expiresAt)

	if held > 0 {
		fmt.Printf("\nAlready holds %d PREMIUM VPN(s) on this server — nothing to do.\n", held)
		return nil
	}
	if *dryRun {
		fmt.Println("\nDRY RUN — would insert 1 vpn_instances row. Nothing was changed.")
		return nil
	}

	res, err := db.ExecContext(ctx,
		`INSERT INTO vpn_instances
		   (owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id,
		    status, traffic_limit_bytes, expires_at, device_limit, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE', ?, ?, ?, ?, ?)`,
		ownerID, serverID, inboundID, *offerID, panelEmail,
		"seed-"+fmt.Sprint(now), trafficBytes, expiresAt, deviceLimit, now, now)
	if err != nil {
		return fmt.Errorf("insert vpn: %w", err)
	}
	id, _ := res.LastInsertId()
	fmt.Printf("\nCREATED vpn_instances id=%d (%s)\n", id, panelEmail)
	fmt.Println("NOTE: local fixture only — no 3X-UI client was created on the panel.")
	return nil
}
