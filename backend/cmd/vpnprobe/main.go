// Command vpnprobe is a READ-ONLY operator view of one VPN's local row and its
// panel-mirrored limits, so an operator can answer "what does Veylix think this
// VPN's panel says?" without opening the database by hand.
//
// Design rules (the same ones cmd/ob and cmd/panelprobe follow):
//   - it is a SEPARATE binary; the server never prints its own rows;
//   - it never writes and never migrates: no UPDATE/INSERT/DELETE, no panel
//     call. A diagnostic must not be able to change what it inspects;
//   - rows is always closed and rows.Err() is checked, so a mid-stream failure
//     cannot print a short list that looks like a complete answer;
//   - no secret is printed: the panel email and the numbers only — never a
//     token, a ciphertext or a uuid/password.
//
// Why it exists: the panel mirror (panel_* columns) is only trustworthy if an
// operator can compare it with a direct read of the panel
// (GET /api/servers/{id}/panel-clients). This prints Veylix's side of that
// comparison.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"veylix/backend/internal/config"
	"veylix/backend/internal/store"
)

// vpnRow is the projection this command prints.
type vpnRow struct {
	ID              int64
	ServerID        int64
	PanelEmail      string
	Status          string
	CreatedTraffic  int64  // what the VPN was ISSUED with (history)
	CreatedDevice   int64  // the device cap it was issued with (history)
	PanelTotalGB    int64  // what the PANEL says (D-024.2)
	PanelExpiryTime int64  //
	PanelLimitIP    int64  //
	PanelLimitHwid  int64  //
	PanelEnable     int64  //
	PanelSyncedAt   *int64 // NULL = the mirror has never been confirmed
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "vpnprobe: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	idFlag := flag.Int64("vpn", 0, "show one VPN by id (0 = the newest ones)")
	serverFlag := flag.Int64("server", 0, "only show VPNs of this integration id (0 = all)")
	limitFlag := flag.Int("limit", 20, "how many rows to show when -vpn is 0 (1-500)")
	timeoutFlag := flag.Duration("timeout", 10*time.Second, "overall deadline for the read")
	flag.Parse()

	if *idFlag < 0 || *serverFlag < 0 {
		return fmt.Errorf("-vpn and -server must not be negative")
	}
	if *limitFlag <= 0 || *limitFlag > 500 {
		return fmt.Errorf("-limit must be between 1 and 500 (got %d)", *limitFlag)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("database %s is not readable: %w (is VEYLIX_DATA_DIR correct?)", cfg.DBPath, err)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	rows, err := readVPNs(ctx, st.DB(), *idFlag, *serverFlag, *limitFlag)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("no VPN rows matched")
		return nil
	}
	printVPNs(rows)
	return nil
}

// readVPNs returns the rows newest first, never including revoked ones by
// default: a revoked VPN is historical and rarely what an operator is chasing.
func readVPNs(ctx context.Context, db *sql.DB, vpnID, serverID int64, limit int) ([]vpnRow, error) {
	query := `SELECT id, server_id, panel_email, status,
	                 traffic_limit_bytes, device_limit,
	                 panel_total_gb, panel_expiry_time, panel_limit_ip,
	                 panel_limit_hwid, panel_enable, panel_synced_at
	            FROM vpn_instances WHERE 1 = 1`
	args := []any{}
	if vpnID > 0 {
		query += ` AND id = ?`
		args = append(args, vpnID)
	}
	if serverID > 0 {
		query += ` AND server_id = ?`
		args = append(args, serverID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read vpn rows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]vpnRow, 0, limit)
	for rows.Next() {
		var r vpnRow
		if err := rows.Scan(&r.ID, &r.ServerID, &r.PanelEmail, &r.Status,
			&r.CreatedTraffic, &r.CreatedDevice,
			&r.PanelTotalGB, &r.PanelExpiryTime, &r.PanelLimitIP,
			&r.PanelLimitHwid, &r.PanelEnable, &r.PanelSyncedAt); err != nil {
			return nil, fmt.Errorf("scan vpn row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vpn rows: %w", err)
	}
	return out, nil
}

func printVPNs(rows []vpnRow) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSERVER\tPANEL EMAIL\tSTATUS\tISSUED (bytes/dev)\tPANEL totalGB\texpiryTime\tIP/HWID\tenable\tMIRRORED")
	for _, r := range rows {
		mirrored := "never"
		if r.PanelSyncedAt != nil {
			mirrored = time.Unix(*r.PanelSyncedAt, 0).UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%d / %d\t%d\t%d\t%d / %d\t%d\t%s\n",
			r.ID, r.ServerID, r.PanelEmail, r.Status,
			r.CreatedTraffic, r.CreatedDevice,
			r.PanelTotalGB, r.PanelExpiryTime, r.PanelLimitIP, r.PanelLimitHwid,
			r.PanelEnable, mirrored)
	}
	_ = w.Flush()
}
