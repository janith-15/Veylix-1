package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"veylix/backend/internal/auth"
)

// SQLUsageStore implements UsageStore over the local database.
//
// Ownership rules mirror SQLVPNLister exactly, so the usage endpoint cannot
// become a way to read a VPN the list endpoint would hide (REQUIREMENTS §4).
type SQLUsageStore struct {
	db *sql.DB
}

// NewSQLUsageStore builds the store.
func NewSQLUsageStore(db *sql.DB) *SQLUsageStore { return &SQLUsageStore{db: db} }

// VPNUsageHistory returns one VPN's history when the caller may see it.
func (s *SQLUsageStore) VPNUsageHistory(ctx context.Context, vpnID int64,
	p auth.Principal, since, limit int64) (*VPNUsage, error) {

	if limit <= 0 || limit > 2000 {
		limit = 240
	}

	var (
		status     string
		limitBytes int64
		lifeUp     int64
		lifeDown   int64
		resets     int
		ownerID    int64
		sellerID   sql.NullInt64
		createdAt  int64
		expires    sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx,
		// The panel's own limit is the answer, not `traffic_limit_bytes`.
		// Migration 0006 states it plainly: `panel_*` columns "hold what the PANEL
		// reports ... they are no longer what the UI trusts", and
		// `traffic_limit_bytes` only records what the VPN was created with. Reading
		// the history column here made every VPN whose limit was set on the panel
		// report `limit_bytes: 0` and `remaining_bytes: -1` — "unlimited" — while
		// `/servers/:id` and `/api/super/vpns` reported the real 500 GiB
		// (VPN_FIX_LEDGER #22). Falls back to the create-time value only when the
		// panel mirror is genuinely empty.
		`SELECT i.status,
		        CASE WHEN i.panel_total_gb > 0 THEN i.panel_total_gb
		             ELSE COALESCE(i.traffic_limit_bytes, 0) END,
		        i.lifetime_up_bytes, i.lifetime_down_bytes,
		        i.last_remote_reset_count, i.owner_user_id, a.seller_user_id,
		        i.created_at, i.expires_at
		   FROM vpn_instances i
		   LEFT JOIN vpn_assignments a ON a.vpn_instance_id = i.id
		  WHERE i.id = ?`, vpnID,
	).Scan(&status, &limitBytes, &lifeUp, &lifeDown, &resets, &ownerID, &sellerID,
		&createdAt, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("usage: load vpn: %w", err)
	}

	// The ownership decision uses ONLY the authenticated principal.
	if !p.CanViewResource(auth.Ownership{
		OwnerUserID:  ownerID,
		SellerUserID: sellerID.Int64,
	}) {
		return nil, ErrUsageForbidden
	}

	points, first, err := s.points(ctx, vpnID, since, limit)
	if err != nil {
		return nil, err
	}

	out := &VPNUsage{
		VPNID:         vpnID,
		Status:        status,
		LimitBytes:    limitBytes,
		LifetimeUp:    lifeUp,
		LifetimeDown:  lifeDown,
		LifetimeTotal: lifeUp + lifeDown,
		Resets:        resets,
		CreatedAt:     createdAt,
		FirstSnapshot: first,
		Points:        points,
	}
	if expires.Valid {
		v := expires.Int64
		out.ExpiresAt = &v
	}
	// "How far along is it" — computed once here so every screen agrees.
	if limitBytes > 0 {
		out.UsagePct = int((lifeUp + lifeDown) * 100 / limitBytes)
		if out.UsagePct > 100 {
			// A panel reports usage that can exceed the nominal limit by a
			// little; the bar must not read 104%.
			out.UsagePct = 100
		}
		remaining := limitBytes - (lifeUp + lifeDown)
		if remaining < 0 {
			remaining = 0
		}
		out.RemainingBytes = remaining
	} else {
		out.RemainingBytes = -1 // unlimited, never "0 left"
	}
	// "Never invent history": say plainly what exists (REQUIREMENTS §2.7).
	if len(points) == 0 {
		out.Availability = "NONE"
	} else {
		out.Availability = "AVAILABLE"
	}
	return out, nil
}

// points loads the history oldest-first so a chart can plot it directly.
func (s *SQLUsageStore) points(ctx context.Context, vpnID, since, limit int64) ([]UsagePoint, *int64, error) {
	var first sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MIN(taken_at) FROM usage_snapshots WHERE vpn_instance_id = ?`, vpnID,
	).Scan(&first); err != nil {
		return nil, nil, fmt.Errorf("usage: first snapshot: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT taken_at, up_bytes, down_bytes, lifetime_up_bytes, lifetime_down_bytes
		   FROM usage_snapshots
		  WHERE vpn_instance_id = ? AND taken_at >= ?
		  ORDER BY taken_at DESC
		  LIMIT ?`, vpnID, since, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("usage: list snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]UsagePoint, 0, 32)
	for rows.Next() {
		var pt UsagePoint
		if err := rows.Scan(&pt.TakenAt, &pt.UpBytes, &pt.DownBytes,
			&pt.LifetimeUpBytes, &pt.LifetimeDownBytes); err != nil {
			return nil, nil, fmt.Errorf("usage: scan snapshot: %w", err)
		}
		// P-01: upload, download and total are three numbers, not one that a
		// chart has to derive.
		pt.TotalBytes = pt.UpBytes + pt.DownBytes
		pt.LifetimeTotal = pt.LifetimeUpBytes + pt.LifetimeDownBytes
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("usage: iterate snapshots: %w", err)
	}

	// Reverse into chronological order (the LIMIT took the newest).
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	var firstPtr *int64
	if first.Valid {
		v := first.Int64
		firstPtr = &v
	}
	return out, firstPtr, nil
}

// ServerMetrics returns server-wide samples for a server the caller may see.
func (s *SQLUsageStore) ServerMetrics(ctx context.Context, serverID int64,
	p auth.Principal, since, limit int64) ([]MetricPoint, error) {

	if limit <= 0 || limit > 2000 {
		limit = 240
	}

	// Clients never see server-wide metrics (REQUIREMENTS §4).
	if p.IsClient() {
		return nil, ErrUsageForbidden
	}

	var exists int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_integrations WHERE id = ? AND status <> 'REMOVED'`, serverID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("usage: server lookup: %w", err)
	}
	if exists == 0 {
		return nil, sql.ErrNoRows
	}

	if p.IsAdmin() {
		// An admin only sees servers explicitly assigned to them.
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM admin_server_assignments WHERE admin_user_id = ? AND server_id = ?`,
			p.UserID, serverID).Scan(&n); err != nil {
			return nil, fmt.Errorf("usage: assignment lookup: %w", err)
		}
		if n == 0 {
			return nil, ErrUsageForbidden
		}
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT taken_at, cpu_pct, mem_used, mem_total
		   FROM server_metrics
		  WHERE server_id = ? AND taken_at >= ?
		  ORDER BY taken_at DESC
		  LIMIT ?`, serverID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("usage: list metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]MetricPoint, 0, 32)
	for rows.Next() {
		var pt MetricPoint
		if err := rows.Scan(&pt.TakenAt, &pt.CPUPct, &pt.MemUsed, &pt.MemTotal); err != nil {
			return nil, fmt.Errorf("usage: scan metric: %w", err)
		}
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage: iterate metrics: %w", err)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// SyncStatus lists every live integration with its last sync timestamp.
func (s *SQLUsageStore) SyncStatus(ctx context.Context) ([]ServerSyncState, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, status, last_sync
		   FROM server_integrations
		  WHERE status <> 'REMOVED'
		  ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("usage: list sync state: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ServerSyncState, 0, 8)
	for rows.Next() {
		var (
			st   ServerSyncState
			last sql.NullInt64
		)
		if err := rows.Scan(&st.ServerID, &st.Name, &st.Status, &last); err != nil {
			return nil, fmt.Errorf("usage: scan sync state: %w", err)
		}
		if last.Valid {
			v := last.Int64
			st.LastSync = &v
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
