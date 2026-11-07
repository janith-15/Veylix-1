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
