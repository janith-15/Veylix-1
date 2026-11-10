package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// FREE claim caps (D-025.4, D-025.5).
const (
	// FreeConcurrentCap is the number of FREE VPNs one client may hold per
	// server at a time. Fixed by policy, not configurable.
	FreeConcurrentCap = 3
	// FreeMonthlyDefault is the default number of FREE claims per rolling 30
	// days per client per server. The Super Admin may raise or lower it.
	FreeMonthlyDefault = 5
	// FreeClaimWindow is the rolling window the monthly cap counts over.
	FreeClaimWindow = 30 * 24 * time.Hour
)

// Errors for the FREE claim path.
var (
	// ErrFreeConcurrentLimit means the client already holds the maximum number
	// of simultaneous FREE VPNs on this server.
	ErrFreeConcurrentLimit = errors.New("store: free concurrent limit reached")
	// ErrFreeMonthlyLimit means the client has used their claims for this window.
	ErrFreeMonthlyLimit = errors.New("store: free monthly limit reached")
)

// FreeClaimUsage reports what a client has consumed.
type FreeClaimUsage struct {
	// Concurrent is the number of FREE claims not yet released.
	Concurrent int `json:"concurrent"`
	// UsedThisWindow is the number of claims inside the rolling window,
	// including released ones: releasing does not refund a slot (D-025.5).
	UsedThisWindow int `json:"used_this_window"`
	// MonthlyLimit is the effective ceiling for this client.
	MonthlyLimit int `json:"monthly_limit"`
	// ConcurrentLimit is the fixed simultaneous ceiling.
	ConcurrentLimit int `json:"concurrent_limit"`
}

// FreeUsage computes a client's claim usage on one server.
//
// `monthlyLimit` <= 0 falls back to FreeMonthlyDefault; the Super Admin's
// per-client and global settings both arrive through that parameter so the cap
// has exactly one implementation.
func (s *Store) FreeUsage(ctx context.Context, clientUserID, serverID int64, monthlyLimit int) (*FreeClaimUsage, error) {
	if monthlyLimit <= 0 {
		monthlyLimit = FreeMonthlyDefault
	}
	since := time.Now().Add(-FreeClaimWindow).Unix()

	var live, window int
	if err := s.db.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(CASE WHEN released_at IS NULL THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN claimed_at > ? THEN 1 ELSE 0 END), 0)
		 FROM free_claims WHERE client_user_id = ? AND server_id = ?`,
		since, clientUserID, serverID).Scan(&live, &window); err != nil {
		return nil, fmt.Errorf("store: free usage: %w", err)
	}
	return &FreeClaimUsage{
		Concurrent:      live,
		UsedThisWindow:  window,
		MonthlyLimit:    monthlyLimit,
		ConcurrentLimit: FreeConcurrentCap,
	}, nil
}

// RecordFreeClaim books a FREE claim, enforcing both caps inside one
// transaction so two concurrent claims cannot both take the last slot (the same
// concern the reseller quota path already solves this way).
func (s *Store) RecordFreeClaim(ctx context.Context, tx *sql.Tx, clientUserID, serverID int64, panelEmail string, monthlyLimit int) (int64, error) {
	if monthlyLimit <= 0 {
		monthlyLimit = FreeMonthlyDefault
	}
	exec := s.execer(tx)
	now := time.Now().Unix()
	since := now - int64(FreeClaimWindow.Seconds())

	var live, window int
	if err := exec.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(CASE WHEN released_at IS NULL THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN claimed_at > ? THEN 1 ELSE 0 END), 0)
		 FROM free_claims WHERE client_user_id = ? AND server_id = ?`,
		since, clientUserID, serverID).Scan(&live, &window); err != nil {
		return 0, fmt.Errorf("store: free claim usage: %w", err)
	}
	if live >= FreeConcurrentCap {
		return 0, ErrFreeConcurrentLimit
	}
	if window >= monthlyLimit {
		return 0, ErrFreeMonthlyLimit
	}

	res, err := exec.ExecContext(ctx,
		`INSERT INTO free_claims (client_user_id, server_id, panel_email, claimed_at)
		 VALUES (?, ?, ?, ?)`, clientUserID, serverID, panelEmail, now)
	if err != nil {
		return 0, fmt.Errorf("store: record free claim: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// MonthlyLimit reports the effective monthly FREE claim cap for a client.
//
// The Super Admin can raise or lower one customer from the client console
// (D-025.5), and that override is stored on `users.free_vpn_limit`. This method
// used to ignore its own argument and return FreeMonthlyDefault for everybody,
// which meant the number saved on /clients/{id} was written correctly and then
// NEVER READ: the console showed "5 / 10 (own limit)" while the sixth claim was
// refused with free_monthly_limit. The cap has exactly one implementation — this
// one — so every caller (POST /api/vpns and GET /api/vpns/claims/me) inherits
// the fix.
//
// 0 (or an account that cannot be found) means "platform default", which is what
// the console documents its own zero as.
func (s *Store) MonthlyLimit(ctx context.Context, clientUserID int64) (int, error) {
	if clientUserID <= 0 {
		return FreeMonthlyDefault, nil
	}
	limit, err := s.ClientFreeLimitOverride(ctx, clientUserID)
	if err != nil {
		// A lookup that fails must not block a claim that would otherwise be
		// allowed: the default is the safe, pre-existing answer.
		return FreeMonthlyDefault, nil
	}
	if limit <= 0 {
		return FreeMonthlyDefault, nil
	}
	return limit, nil
}

// ReleaseFreeClaim marks a claim released (the VPN was given up).
//
// The row is kept: the monthly cap counts claims, not live VPNs, so releasing
// must not hand the client a fresh slot (D-025.5 / D-027.4).
func (s *Store) ReleaseFreeClaim(ctx context.Context, clientUserID, serverID int64, panelEmail string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE free_claims SET released_at = ?
		  WHERE client_user_id = ? AND server_id = ? AND panel_email = ? AND released_at IS NULL`,
		time.Now().Unix(), clientUserID, serverID, panelEmail)
	if err != nil {
		return fmt.Errorf("store: release free claim: %w", err)
	}
	return nil
}

// ReplaceFreeClaim books a FREE claim that REPLACES an existing one (D-112).
//
// Why this cannot simply be `RecordFreeClaim`: that method refuses once the
// client holds `FreeConcurrentCap` live VPNs, and a replace builds the new VPN
// *before* releasing the old one (D-112's ordering — never leave a customer with
// zero VPNs). At the cap, the create would therefore always fail, and a customer
// holding three FREE VPNs could never replace any of them — the commonest case
// of the feature.
//
// A replace does not increase the number of live VPNs: one is released and one
// is created, so the CONCURRENT cap is not the right measure of it. What must
// still hold is its PURPOSE — a client may not use replacement to run more than
// `FreeConcurrentCap` live VPNs at once — so this takes the old claim's slot
// rather than a new one:
//
//  1. The MONTHLY window is still enforced, because Bible §17.18 says releasing
//     does not refund a claim, so a replace spends one. That is checked in
//     `provisioning.Replace` before the panel is touched, and again here.
//  2. The replaced claim's row is closed in the SAME transaction that opens the
//     new one, so the concurrent count never dips below, or rises above, what it
//     was. Two replaces racing each other cannot both take the last slot.
//
// `replacedEmail` names the claim being given up. If it holds no live claim the
// call behaves like an ordinary `RecordFreeClaim`, so a replace of a VPN that
// was never booked (an operator-created one) cannot mint a free slot.
func (s *Store) ReplaceFreeClaim(ctx context.Context, tx *sql.Tx, clientUserID, serverID int64, replacedEmail, newEmail string, monthlyLimit int) (int64, error) {
	if monthlyLimit <= 0 {
		monthlyLimit = FreeMonthlyDefault
	}

	// The caller's transaction is used, not a new one. `Create` already holds a
	// transaction when it books the claim, and opening a second one on the same
	// database deadlocks SQLite (SQLITE_BUSY) — this is the same reason
	// `RecordFreeClaim` takes a `tx` rather than opening its own.
	exec := s.execer(tx)
	now := time.Now().Unix()
	since := now - int64(FreeClaimWindow.Seconds())

	var window int
	if err := exec.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM free_claims
		  WHERE client_user_id = ? AND server_id = ? AND claimed_at > ?`,
		clientUserID, serverID, since).Scan(&window); err != nil {
		return 0, fmt.Errorf("store: replace claim: window: %w", err)
	}
	// The monthly cap counts claims, so a replace spends one even though it
	// gives one back — otherwise replacing would be a free way to reset a
	// spent allowance (Bible §17.18).
	if window >= monthlyLimit {
		return 0, ErrFreeMonthlyLimit
	}

	// Close the replaced claim FIRST, in the caller's transaction. Doing it here
	// rather than earlier is what keeps the concurrent count honest: the swap is
	// atomic, so another request cannot observe a freed slot and take it.
	res, err := exec.ExecContext(ctx,
		`UPDATE free_claims SET released_at = ?
		  WHERE client_user_id = ? AND server_id = ? AND panel_email = ? AND released_at IS NULL`,
		now, clientUserID, serverID, replacedEmail)
	if err != nil {
		return 0, fmt.Errorf("store: replace claim: close old: %w", err)
	}
	closed, _ := res.RowsAffected()

	// If the old claim could not be closed, this is not a swap — it is a new
	// claim, and it must obey the concurrent cap like any other.
	if closed == 0 {
		var live int
		if err := exec.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM free_claims
			  WHERE client_user_id = ? AND server_id = ? AND released_at IS NULL`,
			clientUserID, serverID).Scan(&live); err != nil {
			return 0, fmt.Errorf("store: replace claim: concurrent: %w", err)
		}
		if live >= FreeConcurrentCap {
			return 0, ErrFreeConcurrentLimit
		}
	}

	ins, err := exec.ExecContext(ctx,
		`INSERT INTO free_claims (client_user_id, server_id, panel_email, claimed_at)
		 VALUES (?, ?, ?, ?)`, clientUserID, serverID, newEmail, now)
	if err != nil {
		return 0, fmt.Errorf("store: replace claim: open new: %w", err)
	}
	id, _ := ins.LastInsertId()
	return id, nil
}
