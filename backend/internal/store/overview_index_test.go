package store

import "testing"

// TestOverviewQueriesUseIndexes is the 1 vCPU guard (workflow
// performance-verification). A dashboard that scans `payment_records` once per
// page load is fine on a laptop and a problem on the target host the moment the
// table grows.
//
// This asserts the PLAN, not a duration: a timing assertion flakes on CI, while
// "the query planner used an index" is a fact about the schema that stays true.
func TestOverviewQueriesUseIndexes(t *testing.T) {
	st := newOverviewStore(t)

	queries := map[string]string{
		// The month/trend queries filter on `confirmed_at`, which is where an index
		// earns its keep: without one, every dashboard load reads the whole table.
		"monthly trend": `SELECT CAST(strftime('%Y', confirmed_at, 'unixepoch') AS INTEGER),
		        SUM(amount_cents)
		   FROM payment_records
		  WHERE status IN ('CONFIRMED','REFUNDED') AND confirmed_at >= 0
		  GROUP BY 1`,
		"month window": `SELECT SUM(amount_cents) FROM payment_records
		                  WHERE status = 'CONFIRMED' AND confirmed_at >= 0 AND confirmed_at < 9999999999`,
		"quota usage": `SELECT COUNT(*) FROM vpn_assignments
		                 WHERE seller_user_id = 1 AND status = 'ACTIVE'`,
	}

	for name, q := range queries {
		t.Run(name, func(t *testing.T) {
			rows, err := st.DB().Query("EXPLAIN QUERY PLAN " + q)
			if err != nil {
				t.Fatalf("explain: %v", err)
			}
			defer func() { _ = rows.Close() }()
			var detail string
			for rows.Next() {
				var id, parent, notused int
				var d string
				if err := rows.Scan(&id, &parent, &notused, &d); err != nil {
					t.Fatalf("scan plan: %v", err)
				}
				detail += d + " | "
			}
			// A SEARCH means an index was used; a SCAN of the whole table is the
			// case this test exists to catch.
			if !contains(detail, "USING INDEX") && !contains(detail, "USING COVERING INDEX") {
				t.Fatalf("plan does not use an index: %s", detail)
			}
			t.Logf("plan: %s", detail)
		})
	}
}

// contains is a local helper so this file imports nothing beyond testing.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
