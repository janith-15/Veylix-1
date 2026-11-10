package store

import (
	"context"
	"testing"
	"time"
)

// newOverviewStore opens a migrated database with one panel server, which is the
// minimum a VPN instance needs (server_id is NOT NULL).
func newOverviewStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir()+"/overview.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, created_at, updated_at)
		 VALUES ('s1', 'https://panel.test', x'00', ?, ?)`,
		time.Now().Unix(), time.Now().Unix()); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	return st
}

// overviewUser inserts one account and returns its id.
func overviewUser(t *testing.T, st *Store, email, role string, quota int, lastLogin int64) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := st.DB().Exec(
		`INSERT INTO users (email, password_hash, role, status, display_name, quota_limit,
		                    created_at, updated_at, last_login_at)
		 VALUES (?, 'x', ?, 'ACTIVE', ?, ?, ?, ?, ?)`,
		email, role, email, quota, now, now, lastLogin)
	if err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// overviewPayment inserts one payment record and returns its id.
//
// `confirmedAt` is a pointer because the column is nullable: a CONFIRMED row with
// no confirmation date is exactly the case the month total has to survive.
func overviewPayment(t *testing.T, st *Store, payer int64, cents int64, currency, status string,
	createdAt int64, confirmedAt *int64) int64 {
	t.Helper()
	res, err := st.DB().Exec(
		`INSERT INTO payment_records (payer_user_id, amount_cents, currency, reference, status,
		                              confirmed_at, created_at, updated_at)
		 VALUES (?, ?, ?, '', ?, ?, ?, ?)`,
		payer, cents, currency, status, confirmedAt, createdAt, createdAt)
	if err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// monthStartFor returns the first second of the calendar month `t` falls in, which
// is the boundary PaymentTotals is given by its caller.
func monthStartFor(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).Unix()
}

/*
TestPaymentTotalsNetRevenue is the core money contract (D-107).

Revenue is NET: money actually kept. A refunded payment is money that came in and
went back out, so counting it as revenue overstates the business — the single most
dangerous number to overstate on an owner's screen.

The month window is a CALENDAR month, not a rolling 30 days: an owner asking
"how did this month go" means the month on their calendar.
*/
func TestPaymentTotalsNetRevenue(t *testing.T) {
	st := newOverviewStore(t)
	ctx := context.Background()
	now := time.Now()
	monthStart := monthStartFor(now)
	payer := overviewUser(t, st, "payer@x.com", "client", 0, 0)

	// Inside this month.
	inA := now.Add(-1 * time.Hour).Unix()
	inB := now.Add(-2 * time.Hour).Unix()
	// Before this month started — counted all-time, never monthly.
	beforeMonth := monthStart - 86400

	overviewPayment(t, st, payer, 100000, "USD", PaymentConfirmed, inA, &inA)
	overviewPayment(t, st, payer, 50000, "USD", PaymentConfirmed, inB, &inB)
	overviewPayment(t, st, payer, 30000, "USD", PaymentRefunded, inB, &inB)
	overviewPayment(t, st, payer, 40000, "USD", PaymentRefunded, beforeMonth, &beforeMonth)
	overviewPayment(t, st, payer, 20000, "USD", PaymentPending, inA, nil)
	overviewPayment(t, st, payer, 25000, "USD", PaymentAwaitingConfirmation, inB, nil)
	overviewPayment(t, st, payer, 70000, "USD", PaymentRejected, inA, nil)

	totals, err := st.PaymentTotals(ctx, monthStart)
	if err != nil {
		t.Fatalf("PaymentTotals: %v", err)
	}

	// This month: 100000 + 50000 confirmed = 150000, minus the 30000 refunded here.
	if totals.MonthCents != 120000 {
		t.Fatalf("MonthCents = %d, want 120000 (confirmed 150000 - refunded 30000)", totals.MonthCents)
	}
	// All time: 150000 confirmed minus 30000 + 40000 refunded.
	const wantConfirmed = 150000 - 70000
	if totals.ConfirmedCents != wantConfirmed {
		t.Fatalf("ConfirmedCents = %d, want %d", totals.ConfirmedCents, wantConfirmed)
	}
	// Pending is what is waiting, not what was earned.
	if totals.PendingCents != 45000 || totals.PendingCount != 2 {
		t.Fatalf("pending = %d cents / %d rows, want 45000 / 2", totals.PendingCents, totals.PendingCount)
	}
	// A rejected payment is neither revenue nor a queue item.
	if totals.RefundCents != 70000 {
		t.Fatalf("RefundCents = %d, want 70000", totals.RefundCents)
	}
	if totals.Currency != "USD" || totals.MixedCurrency {
		t.Fatalf("currency = %q mixed=%v, want USD / false", totals.Currency, totals.MixedCurrency)
	}
}

/*
TestPaymentTotalsIgnoresUndatedConfirmations pins a data-quality case the schema
allows: `confirmed_at` is nullable, so a CONFIRMED row can carry no date.

Such a row is real money, so it belongs in the all-time total. It cannot belong in
a calendar month whose date is unknown — attributing it to "now" would invent a
date, and dropping it from all-time would understate revenue.
*/
func TestPaymentTotalsIgnoresUndatedConfirmations(t *testing.T) {
	st := newOverviewStore(t)
	ctx := context.Background()
	now := time.Now()
	monthStart := monthStartFor(now)
	payer := overviewUser(t, st, "payer@x.com", "client", 0, 0)
	created := now.Add(-1 * time.Hour).Unix()

	overviewPayment(t, st, payer, 90000, "USD", PaymentConfirmed, created, nil)
	overviewPayment(t, st, payer, 10000, "USD", PaymentConfirmed, created, &created)

	totals, err := st.PaymentTotals(ctx, monthStart)
	if err != nil {
		t.Fatalf("PaymentTotals: %v", err)
	}
	if totals.ConfirmedCents != 100000 {
		t.Fatalf("ConfirmedCents = %d, want 100000 (both rows are money)", totals.ConfirmedCents)
	}
	if totals.MonthCents != 10000 {
		t.Fatalf("MonthCents = %d, want 10000 (only the dated row can be placed in a month)", totals.MonthCents)
	}
	if totals.MonthSales != 1 {
		t.Fatalf("MonthSales = %d, want 1", totals.MonthSales)
	}
}

/*
TestPaymentTotalsRefusesToSumCurrencies is the honesty guard.

Adding USD to LKR produces a number that describes nothing. The totals then report
`MixedCurrency` and leave the summed amounts at ZERO, so a caller that ignores the
flag shows nothing rather than a plausible lie. The per-currency list is returned
so the UI can name what is actually there.
*/
func TestPaymentTotalsRefusesToSumCurrencies(t *testing.T) {
	st := newOverviewStore(t)
	ctx := context.Background()
	now := time.Now()
	monthStart := monthStartFor(now)
	payer := overviewUser(t, st, "payer@x.com", "client", 0, 0)
	at := now.Add(-1 * time.Hour).Unix()

	overviewPayment(t, st, payer, 100000, "USD", PaymentConfirmed, at, &at)
	overviewPayment(t, st, payer, 5000000, "LKR", PaymentConfirmed, at, &at)
	overviewPayment(t, st, payer, 20000, "USD", PaymentPending, at, nil)

	totals, err := st.PaymentTotals(ctx, monthStart)
	if err != nil {
		t.Fatalf("PaymentTotals: %v", err)
	}
	if !totals.MixedCurrency {
		t.Fatalf("MixedCurrency = false, want true for two currencies")
	}
	if totals.MonthCents != 0 || totals.ConfirmedCents != 0 {
		t.Fatalf("summed across currencies: month=%d confirmed=%d, want 0 / 0",
			totals.MonthCents, totals.ConfirmedCents)
	}
	if len(totals.Currencies) != 2 {
		t.Fatalf("Currencies = %v, want 2 entries", totals.Currencies)
	}
	// Counts stay true: a count of rows does not need a common currency.
	if totals.PendingCount != 1 || totals.PendingCents != 0 {
		t.Fatalf("pending = %d cents / %d rows, want 0 / 1", totals.PendingCents, totals.PendingCount)
	}
}

/*
TestPaymentTotalsOnAnEmptyDatabase returns zeroes and no error.

An installation that has sold nothing is a normal state, not a failure: a 500 here
would make a brand-new platform look broken on its first screen.
*/
func TestPaymentTotalsOnAnEmptyDatabase(t *testing.T) {
	st := newOverviewStore(t)
	totals, err := st.PaymentTotals(context.Background(), monthStartFor(time.Now()))
	if err != nil {
		t.Fatalf("PaymentTotals on empty db: %v", err)
	}
	if totals.MonthCents != 0 || totals.ConfirmedCents != 0 || totals.PendingCount != 0 {
		t.Fatalf("empty db totals = %+v, want zeroes", totals)
	}
	if totals.MixedCurrency {
		t.Fatalf("MixedCurrency = true on an empty db")
	}
	if len(totals.Months) != 0 {
		t.Fatalf("Months = %v, want empty", totals.Months)
	}
}

/*
TestResellerHealthCountsTheThingsOnlyAnOwnerCanFix.

Four numbers, four different interventions:
  - **quota near** — a reseller about to be unable to sell (top up their ceiling)
  - **quota full** — a reseller who already cannot sell
  - **no sales this month** — an account that exists but produces nothing
  - **never signed in** — an account nobody has ever used (a stuck invite)

A platform owner cannot find any of these without opening every reseller in turn,
which is why they belong on the overview instead of a filter on the staff page.
*/
func TestResellerHealthCountsTheThingsOnlyAnOwnerCanFix(t *testing.T) {
	st := newOverviewStore(t)
	ctx := context.Background()
	now := time.Now()
	monthStart := monthStartFor(now)

	// 19/20 used — about to be blocked.
	near := overviewUser(t, st, "near@x.com", "admin", 20, now.Unix())
	// 5/5 used — already blocked.
	full := overviewUser(t, st, "full@x.com", "admin", 5, now.Unix())
	// Healthy: room, has sold, signed in.
	good := overviewUser(t, st, "good@x.com", "admin", 100, now.Unix())
	// Invited and never used.
	ghost := overviewUser(t, st, "ghost@x.com", "admin", 10, 0)
	// A super admin has no ceiling and must never be counted as a reseller.
	overviewUser(t, st, "owner@x.com", "super_admin", 0, now.Unix())
	// A suspended reseller is not an active reseller.
	suspended := overviewUser(t, st, "susp@x.com", "admin", 10, now.Unix())
	if _, err := st.DB().Exec(`UPDATE users SET status = 'SUSPENDED' WHERE id = ?`, suspended); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	// Assignments are what quota is measured against (REQUIREMENTS §2.2).
	seedAssignment := func(n int, sellerID int64, status string) {
		t.Helper()
		for i := 0; i < n; i++ {
			vpnID := overviewVpn(t, st, sellerID, status+"-"+itoa(sellerID)+"-"+itoa(int64(i)))
			if _, err := st.DB().Exec(
				`INSERT INTO vpn_assignments (vpn_instance_id, seller_user_id, client_user_id, status, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				vpnID, sellerID, sellerID, status, now.Unix(), now.Unix()); err != nil {
				t.Fatalf("seed assignment: %v", err)
			}
		}
	}
	seedAssignment(19, near, "ACTIVE")
	seedAssignment(5, full, "ACTIVE")
	seedAssignment(3, good, "ACTIVE")
	// A released assignment frees the slot, so it must not count toward quota.
	seedAssignment(4, ghost, "RELEASED")

	health, err := st.ResellerHealth(ctx, monthStart, now.Unix())
	if err != nil {
		t.Fatalf("ResellerHealth: %v", err)
	}
	if health.Total != 5 {
		t.Fatalf("Total = %d, want 5 (near, full, good, ghost, suspended; never the super admin)", health.Total)
	}
	if health.Active != 4 {
		t.Fatalf("Active = %d, want 4 (near, full, good, suspended; ghost never signed in)", health.Active)
	}
	if health.QuotaNear != 1 {
		t.Fatalf("QuotaNear = %d, want 1 (19/20 only; 5/5 is full, not near)", health.QuotaNear)
	}
	if health.QuotaFull != 1 {
		t.Fatalf("QuotaFull = %d, want 1", health.QuotaFull)
	}
	if health.NeverLoggedIn != 1 {
		t.Fatalf("NeverLoggedIn = %d, want 1 (ghost)", health.NeverLoggedIn)
	}
	// The top list names who can sell least.
	if len(health.Top) == 0 || health.Top[0].ID != full {
		t.Fatalf("Top[0] = %+v, want the 5/5 reseller", health.Top)
	}
	if health.Top[0].QuotaUsed != 5 || health.Top[0].QuotaLimit != 5 {
		t.Fatalf("Top[0] quota = %d/%d, want 5/5", health.Top[0].QuotaUsed, health.Top[0].QuotaLimit)
	}
}

// overviewVpn inserts a VPN instance owned by `owner` and returns its id.
func overviewVpn(t *testing.T, st *Store, owner int64, suffix string) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := st.DB().Exec(
		`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id,
		                            status, created_at, updated_at)
		 VALUES (?, 1, ?, ?, 'ACTIVE', ?, ?)`,
		owner, "u"+suffix+"@panel", "sub-"+suffix, now, now)
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
