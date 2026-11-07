// Diagnostic: list the accounts in the dev database and whether each has a
// usable password hash. Read-only; safe to run while the server is up (WAL).
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	dir := os.Getenv("VEYLIX_DATA_DIR")
	if dir == "" {
		fmt.Fprintln(os.Stderr, "VEYLIX_DATA_DIR is not set")
		os.Exit(1)
	}
	path := filepath.Join(dir, "veylix.db")
	dsn := "file:" + url.PathEscape(filepath.ToSlash(path)) + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT id, email, COALESCE(username,''), role, COALESCE(status,''),
		       CASE WHEN COALESCE(password_hash,'') = '' THEN 'EMPTY' ELSE 'SET' END,
		       COALESCE(length(password_hash),0), email_verified
		  FROM users ORDER BY id`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	defer rows.Close()

	// verified matters because the login door refuses unverified accounts.
	fmt.Printf("%-4s %-24s %-12s %-13s %-10s %-6s %-5s %s\n", "id", "email", "username", "role", "status", "hash", "len", "verified")
	for rows.Next() {
		var id, hlen int64
		var email, username, role, status, hasHash string
		var verified int64
		if err := rows.Scan(&id, &email, &username, &role, &status, &hasHash, &hlen, &verified); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		fmt.Printf("%-4d %-24s %-12s %-13s %-10s %-6s %-5d %d\n", id, email, username, role, status, hasHash, hlen, verified)
	}

	// Which login identities exist (password vs google) — D-039 made these explicit.
	fmt.Println()
	fmt.Println("auth_identities:")
	irows, err := db.QueryContext(ctx, `SELECT user_id, provider, COALESCE(subject,'') FROM auth_identities ORDER BY user_id`)
	if err != nil {
		fmt.Println("  (no auth_identities table or query failed:", err, ")")
	} else {
		defer irows.Close()
		for irows.Next() {
			var uid int64
			var provider, subject string
			_ = irows.Scan(&uid, &provider, &subject)
			fmt.Printf("  user %-3d provider=%-10s subject=%s\n", uid, provider, subject)
		}
	}

	// Row counts for the tables the Phase-L screens read. An empty screen is
	// either "no data" or "broken", and these counts separate the two.
	fmt.Println()
	fmt.Println("row counts (Phase L reads these):")
	for _, t := range []string{
		"server_integrations", "vpn_offers", "vpn_instances", "vpn_assignments",
		"payment_records", "payment_methods", "pricing_rates",
	} {
		var n int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); err != nil {
			fmt.Printf("  %-22s (no table)\n", t)
			continue
		}
		fmt.Printf("  %-22s %d\n", t, n)
	}
}
