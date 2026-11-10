package store

/*
 * Overview aggregates (D-107).
 *
 * Why this file exists. The Super Admin dashboard could show how many panels and
 * clients exist, and nothing about whether the business was alive: no revenue, no
 * pending money, no reseller in trouble. Every number it needed was already in the
 * database â€” `payment_records.amount_cents`, `payment_records.currency`,
 * `users.quota_limit`, `vpn_assignments.seller_user_id` â€” but no query asked for it.
 *
 * Three rules govern everything here:
 *
 *  1. **Money is NET.** A refunded payment is money that came in and went back out.
 *     Counting it as revenue overstates the business, which is the most dangerous
 *     number to overstate on the screen an owner trusts.
 *  2. **Currencies are never summed.** USD + LKR is a number that describes nothing.
 *     When more than one currency is present the totals are zeroed and
 *     `MixedCurrency` is set, so a caller that ignores the flag shows nothing rather
 *     than a plausible lie.
 *  3. **A missing date is not a date.** `confirmed_at` is nullable. A CONFIRMED row
 *     without one counts all-time (it is real money) and is excluded from any month
 *     window (its month is unknown). Inventing a date is not an option.
 */

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// PaymentTotals is the money summary for the overview.
//
// Every amount is integer **cents** and carries the currency it is denominated in
// (`new_update.md` Â§6 â€” money is never a float, never silently rounded).
type PaymentTotals struct {
	// Currency is the single currency the amounts are in. Empty when the
	// installation has no payments yet, or when MixedCurrency is true.
	Currency string `json:"currency"`
	// Currencies lists every currency present, so a caller can name them.
	Currencies []string `json:"currencies"`
	// MixedCurrency means more than one currency exists. The summed amounts below
	// are then ZERO on purpose â€” see rule 2 in the file header.
	MixedCurrency bool `json:"mixed_currency"`

	// MonthCents is NET revenue inside the calendar month: confirmed minus
	// refunded, both dated in that month.
	MonthCents int64 `json:"month_cents"`
	// MonthSales counts CONFIRMED payments dated in that month.
	MonthSales int `json:"month_sales"`
	// AvgTicketCents is MonthCents / MonthSales, or 0 when nothing sold.
	AvgTicketCents int64 `json:"avg_ticket_cents"`
	// MonthDeltaPct is month-over-month growth in whole percent. 0 when the
	// previous month had no revenue: a percentage of zero is not a growth rate.
	MonthDeltaPct int `json:"month_delta_pct"`
	// Months is the last six calendar months, oldest first, for the trend chart.
	Months []MonthTotal `json:"months"`

	// ConfirmedCents is NET revenue all time.
	ConfirmedCents int64 `json:"confirmed_cents"`
	// PendingCents is money the platform is waiting to review or receive.
	PendingCents int64 `json:"pending_cents"`
	// PendingCount is how many payments make up PendingCents â€” the queue depth.
	PendingCount int `json:"pending_count"`
	// RefundCents is money that went back out.
	RefundCents int64 `json:"refund_cents"`
}

// MonthTotal is one calendar month's NET revenue.
type MonthTotal struct {
	// Month is `YYYY-MM`, so a caller can format it and a test can assert it.
	Month string `json:"month"`
	Cents int64  `json:"cents"`
}

// ResellerQuotaRow names one reseller and how much of their ceiling is used.
type ResellerQuotaRow struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	QuotaUsed   int    `json:"quota_used"`
	QuotaLimit  int    `json:"quota_limit"`
}

// ResellerHealth is the reseller book, reduced to the facts an owner acts on.
//
// Each field is a different intervention, not a different statistic: a ceiling to
// raise, an account to chase, an invite that never landed.
type ResellerHealth struct {
	// Total counts every `admin` row, including suspended ones â€” a suspended
	// reseller still exists and still holds a ceiling.
	Total int `json:"total"`
	// Active counts resellers who signed in within the last 30 days.
	Active int `json:"active"`
	// QuotaNear counts resellers at >= 90% of their ceiling (and not full).
	QuotaNear int `json:"quota_near"`
	// QuotaFull counts resellers whose used slots have reached the ceiling.
	QuotaFull int `json:"quota_full"`
	// NoSales counts resellers with no ACTIVE assignment created this month.
	NoSales int `json:"no_sales"`
	// NeverLoggedIn counts resellers who have never signed in.
	NeverLoggedIn int `json:"never_logged_in"`
	// Top names the five resellers closest to their ceiling, fullest first.
	Top []ResellerQuotaRow `json:"top"`
}

// overviewQuotaNearPct is the utilisation at which a reseller is called "near".
//
// 90 rather than 100 because the point of the number is to act BEFORE the reseller
// is blocked: at 100% they can no longer sell and the conversation is already late.
const overviewQuotaNearPct = 90

// timeLayoutMonth is the `YYYY-MM` key of a monthly bucket.
const timeLayoutMonth = "2006-01"

// PaymentTotals aggregates the whole payment history into the overview's money row.
//
// `monthStart` is the first second of the calendar month the caller considers
// "now" (see `MonthStartUnix`). It is passed in rather than computed here so a test
// can place rows on either side of the boundary without touching the clock.
//
// ONE query. On a 1 vCPU host, three scans of `payment_records` per dashboard load
// would be three times the work for the same answer, and `idx_pay_status
// (status, created_at)` serves the conditionals below.
func (s *Store) PaymentTotals(ctx context.Context, monthStart int64) (PaymentTotals, error) {
	out := PaymentTotals{Currencies: []string{}, Months: []MonthTotal{}}

	// --- 1. Per-currency aggregates ------------------------------------------
	//
	// Grouped by currency so the totals can refuse to cross currencies. The
	// month window and the trend share this pass.
	monthEnd := MonthEndUnix(monthStart, time.Local)
	prevStart := MonthStartUnix(time.Unix(monthStart, 0).AddDate(0, -1, 0))

	rows, err := s.db.QueryContext(ctx,
		`SELECT currency,
		        SUM(CASE WHEN status = 'CONFIRMED' THEN amount_cents ELSE 0 END)                          AS net_all,
		        SUM(CASE WHEN status = 'CONFIRMED' AND confirmed_at >= ? AND confirmed_at < ?
		                 THEN amount_cents ELSE 0 END)                                                    AS net_month,
		        SUM(CASE WHEN status = 'CONFIRMED' AND confirmed_at >= ? AND confirmed_at < ?
		                 THEN 1 ELSE 0 END)                                                               AS sales_month,
		        SUM(CASE WHEN status = 'CONFIRMED' AND confirmed_at >= ? AND confirmed_at < ?
		                 THEN amount_cents ELSE 0 END)                                                    AS net_prev,
		        SUM(CASE WHEN status = 'REFUNDED' THEN amount_cents ELSE 0 END)                           AS refunded,
		        SUM(CASE WHEN status = 'REFUNDED' AND confirmed_at >= ? AND confirmed_at < ?
		                 THEN amount_cents ELSE 0 END)                                                    AS refunded_month,
		        SUM(CASE WHEN status IN ('PENDING', 'AWAITING_CONFIRMATION')
		                 THEN amount_cents ELSE 0 END)                                                    AS pending_cents,
		        SUM(CASE WHEN status IN ('PENDING', 'AWAITING_CONFIRMATION')
		                 THEN 1 ELSE 0 END)                                                               AS pending_count
		   FROM payment_records
		  GROUP BY currency
		  ORDER BY currency`,
		monthStart, monthEnd,
		monthStart, monthEnd,
		prevStart, monthStart,
		monthStart, monthEnd)
	if err != nil {
		return out, fmt.Errorf("store: payment totals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type currencyAgg struct {
		netAll, netMonth, netPrev int64
		salesMonth                int
		refunded, refundedMonth   int64
		pendingCents              int64
		pendingCount              int
	}
	aggs := map[string]*currencyAgg{}
	order := []string{}
	for rows.Next() {
		var (
			currency string
			a        currencyAgg
		)
		if err := rows.Scan(&currency, &a.netAll, &a.netMonth, &a.salesMonth,
			&a.netPrev, &a.refunded, &a.refundedMonth, &a.pendingCents,
			&a.pendingCount); err != nil {
			return out, fmt.Errorf("store: scan payment totals: %w", err)
		}
		aggs[currency] = &a
		order = append(order, currency)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(order) == 0 {
		// No payments at all: an installation that has sold nothing. Zeroes, no error
		// (a 500 here would make a new platform look broken on its first screen).
		return out, nil
	}

	out.Currencies = order
	out.MixedCurrency = len(order) > 1

	// Counts describe rows, not money, and are true even across currencies.
	for _, c := range order {
		a := aggs[c]
		out.PendingCents += a.pendingCents
		out.PendingCount += a.pendingCount
	}
	if out.MixedCurrency {
		// Rule 2: the summed amounts stay ZERO on purpose. Counts above remain true.
		out.PendingCents = 0
		return out, nil
	}

	a := aggs[order[0]]
	out.Currency = order[0]
	// Rule 1: NET = confirmed - refunded, per window.
	out.MonthCents = a.netMonth - a.refundedMonth
	out.MonthSales = a.salesMonth
	out.ConfirmedCents = a.netAll - a.refunded
	out.RefundCents = a.refunded
	if a.salesMonth > 0 {
		out.AvgTicketCents = a.netMonth / int64(a.salesMonth)
	}
	// Rule about zero: a percentage of zero is not a growth rate.
	if a.netPrev > 0 {
		out.MonthDeltaPct = int((a.netMonth - a.netPrev) * 100 / a.netPrev)
	}

	months, err := s.paymentMonths(ctx, order[0], monthStart)
	if err != nil {
		return out, err
	}
	out.Months = months
	return out, nil
}

// paymentMonths returns the last six calendar months of NET revenue, oldest first.
//
// Six rather than twelve: the trend is there to answer "growing?", and twelve bars
// on a 40%-width card are unreadable slivers. `months` is fixed-length and
// zero-filled IN THE OUTPUT, not in the database â€” a month with no sales really did
// earn zero, which is a fact, unlike a fabricated sample point in a line chart.
func (s *Store) paymentMonths(ctx context.Context, currency string, monthStart int64) ([]MonthTotal, error) {
	const window = 6
	start := MonthStartUnix(time.Unix(monthStart, 0).AddDate(0, -(window - 1), 0))
	end := MonthEndUnix(monthStart, time.Local)

	rows, err := s.db.QueryContext(ctx,
		`SELECT CAST(strftime('%Y', confirmed_at, 'unixepoch', 'localtime') AS INTEGER) * 100
		      + CAST(strftime('%m', confirmed_at, 'unixepoch', 'localtime') AS INTEGER) AS ym,
		        SUM(CASE WHEN status = 'CONFIRMED' THEN amount_cents ELSE 0 END)
		      - SUM(CASE WHEN status = 'REFUNDED'  THEN amount_cents ELSE 0 END) AS net
		   FROM payment_records
		  WHERE currency = ?
		    AND confirmed_at IS NOT NULL
		    AND confirmed_at >= ? AND confirmed_at < ?
		    AND status IN ('CONFIRMED', 'REFUNDED')
		  GROUP BY ym
		  ORDER BY ym`, currency, start, end)
	if err != nil {
		return nil, fmt.Errorf("store: payment months: %w", err)
	}
	defer func() { _ = rows.Close() }()

	got := map[int]int64{}
	for rows.Next() {
		var (
			ym  int
			net int64
		)
		if err := rows.Scan(&ym, &net); err != nil {
			return nil, fmt.Errorf("store: scan payment month: %w", err)
		}
		got[ym] = net
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]MonthTotal, 0, window)
	cursor := time.Unix(start, 0)
	for i := 0; i < window; i++ {
		key := cursor.Year()*100 + int(cursor.Month())
		out = append(out, MonthTotal{Month: cursor.Format(timeLayoutMonth), Cents: got[key]})
		cursor = cursor.AddDate(0, 1, 0)
	}
	return out, nil
}

// ResellerHealth reduces the reseller book to the four facts an owner acts on.
//
// `monthStart` bounds "sold this month"; `nowUnix` bounds "signed in recently".
// Both are parameters so the behaviour is testable without a clock.
//
// ONE query plus one bounded follow-up. The utilisation expression is the same one
// `ListStaff` uses (`vpn_assignments.seller_user_id` with status ACTIVE), so the
// number here and the number on the staff console can never disagree: a reseller who
// reads 19/20 on both screens is the point.
func (s *Store) ResellerHealth(ctx context.Context, monthStart, nowUnix int64) (ResellerHealth, error) {
	out := ResellerHealth{Top: []ResellerQuotaRow{}}
	activeSince := nowUnix - 30*24*60*60

	rows, err := s.db.QueryContext(ctx,
		`WITH book AS (
		   SELECT u.id,
		          COALESCE(u.display_name, '') AS display_name,
		          u.email,
		          COALESCE(u.quota_limit, 0)   AS quota_limit,
		          u.status,
		          u.last_login_at,
		          (SELECT COUNT(*) FROM vpn_assignments va
		            WHERE va.seller_user_id = u.id AND va.status = 'ACTIVE') AS used,
		          (SELECT COUNT(*) FROM vpn_assignments va
		            WHERE va.seller_user_id = u.id AND va.status = 'ACTIVE'
		              AND va.created_at >= ?) AS sold_this_month
		     FROM users u
		    WHERE u.role = 'admin'
		 )
		 SELECT id, display_name, email, quota_limit, used, sold_this_month,
		        last_login_at, status
		   FROM book`, monthStart)
	if err != nil {
		return out, fmt.Errorf("store: reseller health: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			id, limit, used, sold int64
			lastLogin             int64
			name, email, status   string
		)
		if err := rows.Scan(&id, &name, &email, &limit, &used, &sold, &lastLogin, &status); err != nil {
			return out, fmt.Errorf("store: scan reseller health: %w", err)
		}
		out.Total++
		if lastLogin >= activeSince {
			out.Active++
		}
		if lastLogin == 0 {
			out.NeverLoggedIn++
		}
		if sold == 0 {
			out.NoSales++
		}
		// A ceiling of 0 means unlimited (REQUIREMENTS Â§2.2), so it is never "near"
		// and never "full" â€” there is nothing to raise.
		if limit > 0 {
			if used >= limit {
				out.QuotaFull++
			} else if used*100 >= limit*overviewQuotaNearPct {
				out.QuotaNear++
			}
		}
		out.Top = append(out.Top, ResellerQuotaRow{
			ID: id, DisplayName: name, Email: email,
			QuotaUsed: int(used), QuotaLimit: int(limit),
		})
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	// Fullest first, so "who can sell least" is the first row. Ties break by id so
	// the order is stable between two loads of the same data.
	sort.SliceStable(out.Top, func(i, j int) bool {
		a, b := out.Top[i], out.Top[j]
		ar, br := quotaRatio(a), quotaRatio(b)
		if ar != br {
			return ar > br
		}
		return a.ID < b.ID
	})
	if len(out.Top) > 5 {
		out.Top = out.Top[:5]
	}
	return out, nil
}

// quotaRatio is used/limit as a float, with 0 for an unlimited ceiling.
//
// Only used for ORDERING here â€” never printed. An unlimited reseller sorts last
// because they can always sell, which is the correct reading of "who is blocked".
func quotaRatio(r ResellerQuotaRow) float64 {
	if r.QuotaLimit <= 0 {
		return -1
	}
	return float64(r.QuotaUsed) / float64(r.QuotaLimit)
}

// MonthStartUnix is the first second of the calendar month containing `t`.
//
// Calendar month, not a rolling 30 days (D-107): an owner asking "how did this month
// go" means the month on their calendar, and a rolling window silently changes
// meaning from one day to the next.
func MonthStartUnix(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).Unix()
}

// MonthEndUnix is the first second of the NEXT month: the exclusive upper bound of
// the calendar month that starts at `monthStart`.
//
// `loc` is taken because a unix second carries no zone and a month boundary is a
// local-time fact.
func MonthEndUnix(monthStart int64, loc *time.Location) int64 {
	return time.Unix(monthStart, 0).In(loc).AddDate(0, 1, 0).Unix()
}
