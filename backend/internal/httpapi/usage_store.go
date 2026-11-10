package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
//
// `bucket` is the plotted point width in seconds, chosen from the window
// (D-096). It is a parameter rather than being derived here because `all` and
// `custom` windows only know their own span at the route layer. When
// `bucketFromSpan` is set the width is instead derived from the first snapshot,
// because those windows arrive with no lower edge at all.
func (s *SQLUsageStore) VPNUsageHistory(ctx context.Context, vpnID int64,
	p auth.Principal, since, until, limit, bucket int64, bucketFromSpan bool) (*VPNUsage, error) {

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

	// The bucket width is chosen by the caller from the window (D-096); a zero
	// falls back to a daily bucket rather than to the raw newest-N read.
	bucketSec := bucket
	if bucketSec <= 0 {
		bucketSec = bucketDay
	}

	points, first, usedBucketSec, usedBucketName, err := s.points(ctx, vpnID, since, until, limit, bucketSec, bucketFromSpan)
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
		BucketSec:     usedBucketSec,
		Bucket:        usedBucketName,
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
//
// The rows are aggregated into fixed-width buckets (D-096). Before this, the
// read was `ORDER BY taken_at DESC LIMIT 240`, which took the NEWEST 240 raw
// rows regardless of the window: every range from 24h to 1y collapsed onto the
// same recent slice — measured, six ranges returned the identical 116 points
// spanning 0.8 days — and the chart then labelled that slice "Last 30 days".
// Bucketing makes a range differ from another range in its actual shape, which
// is the whole promise of a range selector.
//
// `bucketSec` <= 0 falls back to a daily bucket, so a caller that predates this
// change cannot silently get the raw newest-N behaviour back.
func (s *SQLUsageStore) points(ctx context.Context, vpnID, since, until, limit, bucketSec int64, bucketFromSpan bool) ([]UsagePoint, *int64, int64, string, error) {
	// finalBucketName is what the caller should label the axis with. It starts as
	// whatever the route chose and is replaced when the width is derived here.
	finalBucketName := ""
	var first sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MIN(taken_at) FROM usage_snapshots WHERE vpn_instance_id = ?`, vpnID,
	).Scan(&first); err != nil {
		return nil, nil, 0, "", fmt.Errorf("usage: first snapshot: %w", err)
	}

	if bucketSec <= 0 {
		bucketSec = 86400
	}

	// `all` and `custom` arrive without a lower edge (bucketFromSpan), so their
	// width cannot come from the range key. Derive it from the span the data
	// really covers (D-096): an unbounded window must still land in ~30 points
	// rather than one row per snapshot. `first` is already loaded above, so this
	// costs no extra query.
	if bucketFromSpan && first.Valid && until > first.Int64 {
		// The width the store actually used is reported back to the caller, so
		// `?range=all` cannot echo `bucket_sec: 0` after plotting daily buckets
		// (D-096). The route copies it onto the echoed window.
		bucketSec, finalBucketName = bucketFor("custom", first.Int64, until)
	}

	// The upper bound keeps the hard row ceiling: whatever the bucket width, a
	// chart never receives more than `limit` points, so a wide custom window
	// cannot turn one page view into a full table scan on a 1 vCPU host.
	//
	// Each bucket reports the traffic that MOVED inside it, summed from the
	// per-snapshot deltas where a previous snapshot exists, and carries the
	// bucket's start time so the axis can state the period it really covers.
	rows, err := s.db.QueryContext(ctx,
		`WITH ordered AS (
		     SELECT taken_at, up_bytes, down_bytes,
		            lifetime_up_bytes, lifetime_down_bytes,
		            LAG(up_bytes)           OVER (ORDER BY taken_at) AS prev_up,
		            LAG(down_bytes)         OVER (ORDER BY taken_at) AS prev_down
		       FROM usage_snapshots
		      WHERE vpn_instance_id = ? AND taken_at >= ?
		 )
		 SELECT CAST(taken_at / ? AS INTEGER) * ? AS bucket_start,
		        SUM(CASE WHEN prev_up   IS NULL THEN 0 ELSE MAX(0, up_bytes   - prev_up)   END),
		        SUM(CASE WHEN prev_down IS NULL THEN 0 ELSE MAX(0, down_bytes - prev_down) END),
		        MAX(lifetime_up_bytes), MAX(lifetime_down_bytes)
		   FROM ordered
		  GROUP BY bucket_start
		  ORDER BY bucket_start ASC
		  LIMIT ?`, vpnID, since, bucketSec, bucketSec, limit)
	if err != nil {
		return nil, nil, 0, "", fmt.Errorf("usage: list snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]UsagePoint, 0, 32)
	for rows.Next() {
		var pt UsagePoint
		if err := rows.Scan(&pt.TakenAt, &pt.UpBytes, &pt.DownBytes,
			&pt.LifetimeUpBytes, &pt.LifetimeDownBytes); err != nil {
			return nil, nil, 0, "", fmt.Errorf("usage: scan snapshot: %w", err)
		}
		// P-01: upload, download and total are three numbers, not one that a
		// chart has to derive.
		pt.TotalBytes = pt.UpBytes + pt.DownBytes
		pt.LifetimeTotal = pt.LifetimeUpBytes + pt.LifetimeDownBytes
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, "", fmt.Errorf("usage: iterate snapshots: %w", err)
	}

	var firstPtr *int64
	if first.Valid {
		v := first.Int64
		firstPtr = &v
	}
	return out, firstPtr, bucketSec, finalBucketName, nil
}

// requireVPNVisible resolves ownership for a VPN read and is the single place
// the hourly/speed/ping reads decide whether a caller may see a VPN (D-092).
//
// It is deliberately the SAME predicate `VPNUsageHistory` uses
// (`p.CanViewResource` against the row's owner and seller), because two
// different ownership rules for two endpoints on the same row is exactly how a
// later endpoint leaks a VPN the list endpoint hides.
//
// A missing row answers `sql.ErrNoRows` (→ 404) and a row the caller may not see
// answers `ErrUsageForbidden` (→ 403). Those are different answers on purpose:
// collapsing them would tell an unauthorised caller that the VPN exists.
func (s *SQLUsageStore) requireVPNVisible(ctx context.Context, vpnID int64,
	p auth.Principal) error {

	var (
		ownerID  int64
		sellerID sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT i.owner_user_id, a.seller_user_id
		   FROM vpn_instances i
		   LEFT JOIN vpn_assignments a ON a.vpn_instance_id = i.id
		  WHERE i.id = ?`, vpnID,
	).Scan(&ownerID, &sellerID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("usage: vpn lookup: %w", err)
	}
	if !p.CanViewResource(auth.Ownership{
		OwnerUserID:  ownerID,
		SellerUserID: sellerID.Int64,
	}) {
		return ErrUsageForbidden
	}
	return nil
}

// VPNUsageHourly returns per-hour traffic DELTAS for the "usage by time" chart
// (D-092).
//
// # WHERE THE NUMBERS COME FROM, AND WHY THEY ARE HONEST
//
// `usage_snapshots` stores CUMULATIVE panel counters every sync tick. A delta is
// therefore `current - previous` across consecutive snapshots, never a stored
// value. Summing those deltas per hour bucket is arithmetic over observations
// that actually happened — there is no interpolation and no synthetic sample.
//
// TWO DEFECTS THIS AVOIDS
//
//  1. A counter that went BACKWARDS is a panel-side reset. `MAX(0, delta)` turns
//     it into 0 for that interval instead of a negative bar, which is the only
//     non-lying answer: we cannot attribute the traffic that happened before the
//     reset to the hour it happened in.
//  2. The first snapshot of the window has no predecessor INSIDE the window, so
//     its "delta" would be the entire lifetime total. It is excluded, which is
//     why an hour with one sample reports `measurements: 1` and a zero delta
//     rather than a fabricated spike.
//
// `measurements` is returned so the caller can say how coarse a bucket is: the
// sync tick is ~15 minutes, so a full hour normally holds four snapshots and one
// holding a single snapshot is a gap, not a quiet hour.
func (s *SQLUsageStore) VPNUsageHourly(ctx context.Context, vpnID int64,
	p auth.Principal, since, until int64) ([]HourlyBucket, error) {

	if err := s.requireVPNVisible(ctx, vpnID, p); err != nil {
		return nil, err
	}
	if until <= 0 {
		until = time.Now().Unix()
	}

	// `LAG(...) OVER (...)` needs SQLite 3.25+, which modernc.org/sqlite provides.
	// The window is bounded on both ends so a custom range is a real range.
	rows, err := s.db.QueryContext(ctx,
		`WITH ordered AS (
		     SELECT taken_at, up_bytes, down_bytes,
		            LAG(up_bytes)   OVER (ORDER BY taken_at) AS prev_up,
		            LAG(down_bytes) OVER (ORDER BY taken_at) AS prev_down
		       FROM usage_snapshots
		      WHERE vpn_instance_id = ? AND taken_at >= ? AND taken_at <= ?
		 )
		 SELECT CAST(taken_at / 3600 AS INTEGER) * 3600 AS hour_start,
		        SUM(CASE WHEN prev_up IS NULL THEN 0
		                 ELSE MAX(0, up_bytes - prev_up) END),
		        SUM(CASE WHEN prev_down IS NULL THEN 0
		                 ELSE MAX(0, down_bytes - prev_down) END),
		        COUNT(*)
		   FROM ordered
		  GROUP BY hour_start
		  ORDER BY hour_start`, vpnID, since, until)
	if err != nil {
		return nil, fmt.Errorf("usage: hourly buckets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]HourlyBucket, 0, 24)
	for rows.Next() {
		var b HourlyBucket
		if err := rows.Scan(&b.HourStart, &b.UpBytes, &b.DownBytes, &b.Measurements); err != nil {
			return nil, fmt.Errorf("usage: scan hourly bucket: %w", err)
		}
		b.TotalBytes = b.UpBytes + b.DownBytes
		// Hour of day in UTC, so the grouping matches the bucket the rows were
		// summed into. The browser localises the LABEL, never the bucket.
		b.HourOfDay = int((b.HourStart % 86400) / 3600)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage: iterate hourly buckets: %w", err)
	}
	return out, nil
}

// VPNSpeedHistory derives a rate between each pair of consecutive stored
// counters (D-092 · Connection Performance).
//
// No new table: `up_bytes` / `down_bytes` and `taken_at` already exist, and a
// rate is their difference over the elapsed time. Doing it in SQL keeps the
// arithmetic on the server so a chart can never disagree with the API
// (D-038 P-01).
//
// An interval is marked `unavailable` — not zero — when:
//   - the elapsed time is not positive (two snapshots in the same second), or
//   - either counter went backwards (a panel-side reset, so the interval says
//     nothing about the rate).
//
// Zero is a real measurement of an idle connection; `unavailable` is the honest
// answer for "this interval cannot tell us" (D-026.3).
func (s *SQLUsageStore) VPNSpeedHistory(ctx context.Context, vpnID int64,
	p auth.Principal, since, until int64) ([]SpeedPoint, error) {

	if err := s.requireVPNVisible(ctx, vpnID, p); err != nil {
		return nil, err
	}
	if until <= 0 {
		until = time.Now().Unix()
	}

	rows, err := s.db.QueryContext(ctx,
		`WITH ordered AS (
		     SELECT taken_at, up_bytes, down_bytes,
		            LAG(taken_at)   OVER (ORDER BY taken_at) AS prev_at,
		            LAG(up_bytes)   OVER (ORDER BY taken_at) AS prev_up,
		            LAG(down_bytes) OVER (ORDER BY taken_at) AS prev_down
		       FROM usage_snapshots
		      WHERE vpn_instance_id = ? AND taken_at >= ? AND taken_at <= ?
		 )
		 SELECT taken_at, prev_at, up_bytes, prev_up, down_bytes, prev_down
		   FROM ordered
		  WHERE prev_at IS NOT NULL
		  ORDER BY taken_at`, vpnID, since, until)
	if err != nil {
		return nil, fmt.Errorf("usage: speed history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SpeedPoint, 0, 64)
	for rows.Next() {
		var (
			at, prevAt     int64
			up, prevUp     int64
			down, prevDown int64
		)
		if err := rows.Scan(&at, &prevAt, &up, &prevUp, &down, &prevDown); err != nil {
			return nil, fmt.Errorf("usage: scan speed point: %w", err)
		}
		elapsed := at - prevAt
		pt := SpeedPoint{TakenAt: at, ElapsedSec: elapsed}
		switch {
		case elapsed <= 0:
			pt.Source = "unavailable"
		case up < prevUp || down < prevDown:
			// A panel-side reset makes this interval's delta meaningless.
			// Reported rather than dropped, so a chart can draw the gap.
			pt.Source = "unavailable"
		default:
			pt.UpBps = (up - prevUp) / elapsed
			pt.DownBps = (down - prevDown) / elapsed
			pt.Source = "measured"
		}
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage: iterate speed history: %w", err)
	}
	return out, nil
}

// RecordPing stores one reachability probe (D-092).
//
// Best-effort by contract: the CALLER decides whether a write failure should
// fail the request, and it must not — the measurement happened, and losing the
// history row is not a reason to hide the number the customer just asked for.
//
// The ownership check is the same predicate the reads use, so a probe cannot be
// recorded against somebody else's VPN.
//
// A failed probe (reachable=false) is STORED, with latency -1. It is a real
// observation, and dropping it would make an outage look like a gap in testing
// rather than a gap in service (D-026.3).
func (s *SQLUsageStore) RecordPing(ctx context.Context, vpnID int64,
	p auth.Principal, reachable bool, latencyMS int64, scope string) error {

	if err := s.requireVPNVisible(ctx, vpnID, p); err != nil {
		return err
	}
	if scope == "" {
		scope = "server_reachability"
	}
	isReachable := 0
	if reachable {
		isReachable = 1
	}
	if latencyMS < 0 {
		latencyMS = -1
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO ping_samples (vpn_instance_id, taken_at, reachable, latency_ms, scope)
		 VALUES (?, ?, ?, ?, ?)`,
		vpnID, time.Now().Unix(), isReachable, latencyMS, scope,
	); err != nil {
		return fmt.Errorf("usage: record ping: %w", err)
	}
	return nil
}

// PingHistory returns stored probes oldest-first so a chart can plot them
// directly (D-092).
//
// A VPN with no probes returns an empty slice, never a synthetic point: "never
// measured" and "measured as unreachable" are different facts and the reader
// distinguishes them by whether the array is empty (D-026.3).
//
// `scope` (D-095) narrows the read to ONE measurement source. It exists because
// `ping_samples.scope` already distinguishes the SERVER's reachability from any
// future client-side probe (D-026.1/.2), and mixing two different measurements
// into one line would draw a chart that silently compares unlike things. An
// empty scope means "every source", which is the behaviour this method had
// before the filter existed.
func (s *SQLUsageStore) PingHistory(ctx context.Context, vpnID int64,
	p auth.Principal, since, until, limit int64, scope string) ([]PingPoint, error) {

	if err := s.requireVPNVisible(ctx, vpnID, p); err != nil {
		return nil, err
	}
	if until <= 0 {
		until = time.Now().Unix()
	}
	if limit <= 0 || limit > 500 {
		limit = 120
	}

	// The scope is a bound parameter, never string-concatenated, so a value from
	// the query string can never alter the statement.
	query := `SELECT taken_at, reachable, latency_ms, scope
	            FROM ping_samples
	           WHERE vpn_instance_id = ? AND taken_at >= ? AND taken_at <= ?`
	args := []any{vpnID, since, until}
	if scope != "" {
		query += ` AND scope = ?`
		args = append(args, scope)
	}
	query += ` ORDER BY taken_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage: ping history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PingPoint, 0, 32)
	for rows.Next() {
		var (
			pt        PingPoint
			reachable int
		)
		if err := rows.Scan(&pt.TakenAt, &reachable, &pt.LatencyMs, &pt.Scope); err != nil {
			return nil, fmt.Errorf("usage: scan ping point: %w", err)
		}
		pt.Reachable = reachable == 1
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage: iterate ping history: %w", err)
	}
	// The LIMIT took the newest; reverse into chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
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
