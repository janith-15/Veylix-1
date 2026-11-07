// Diagnostic: what the panel's `totalGB` really holds, and whether the listing
// path treats it as bytes or as gigabytes.
//
// Read-only (mode=ro), safe to run while the server is up (WAL).
//
// The defect it investigates: `internal/integrations/service.go` multiplies
// `c.TotalGB` by 1024^3 when it builds a panel client view, while
// `loadAssignments` documents in SQL that `panel_total_gb` ALREADY holds bytes.
// If both are true, a real 4 PiB sentinel is squared, and a VPN that has used
// 452 MB of its allowance reads as 0% used.
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

const gib = int64(1024 * 1024 * 1024)

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
		SELECT id, panel_email,
		       COALESCE(traffic_limit_bytes,0) AS traffic_limit_bytes,
		       COALESCE(panel_total_gb,0)      AS panel_total_gb,
		       COALESCE(device_limit,0)        AS device_limit,
		       COALESCE(panel_limit_hwid,0)    AS panel_limit_hwid
		  FROM vpn_instances ORDER BY id`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	defer rows.Close()

	fmt.Printf("%-4s %-24s %-18s %-18s %s\n",
		"id", "panel_email", "traffic_limit_bytes", "panel_total_gb", "verdict")
	for rows.Next() {
		var id, traffic, panel, devLimit, panelHwid int64
		var email string
		if err := rows.Scan(&id, &email, &traffic, &panel, &devLimit, &panelHwid); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		verdict := ""
		switch {
		case panel > 0 && traffic == 0:
			verdict = "MISMATCH: panel HAS a limit, traffic_limit_bytes = 0 -> /usage says unlimited"
		case panel != traffic && panel > 0 && traffic > 0:
			verdict = "MISMATCH: the two columns disagree"
		case panel == 0 && traffic == 0:
			verdict = "no limit anywhere (genuinely unlimited)"
		default:
			verdict = "agree"
		}
		fmt.Printf("%-4d %-24s %-18d %-18d %s\n", id, email, traffic, panel, verdict)
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "rows:", err)
		os.Exit(1)
	}
}
