package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"veylix/backend/internal/domain"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

// Panel is the adapter surface sync needs. Kept minimal so the service is
// testable without a network and so sync can never grow a write path by
// accident — there is deliberately no AddClient/DeleteClient here.
//
// ClientExists is present because absence must be *verified*: a client that a
// page-through did not return is only marked SERVER_UNAVAILABLE once a direct
// read positively reports it gone (D-024.5). Without that second read a
// truncated page would look like mass deletion.
type Panel interface {
	Health(ctx context.Context) panel.HealthResult
	ListClients(ctx context.Context, page, pageSize int) (*panel.ClientPage, error)
	ClientExists(ctx context.Context, email string) (bool, error)
}

// ClientFactory builds a Panel for a server id.
type ClientFactory func(ctx context.Context, serverID int64) (Panel, error)

// ErrServerUnreachable means the panel could not be read this pass.
var ErrServerUnreachable = errors.New("sync: server unreachable")

// Config bounds one sync pass.
type Config struct {
	// PageSize is the panel page size; the verified contract caps it at 200.
	PageSize int
	// MaxPages stops a runaway pagination loop (bounded work per pass).
	MaxPages int
	// Timeout bounds one whole server pass.
	Timeout time.Duration
	// Now overrides the clock. Nil means time.Now; tests use it to freeze a tick.
	Now func() time.Time
}

// DefaultConfig is the production default: 200 clients per page, at most 50
// pages (10 000 clients) and 60 s per server — comfortably inside the 1 vCPU /
// 500 MB target while still finishing before the next 5-minute tick.
func DefaultConfig() Config {
	return Config{PageSize: 200, MaxPages: 50, Timeout: 60 * time.Second}
}

// ServerResult reports one server's pass.
type ServerResult struct {
	ServerID  int64
	Clients   int
	Snapshots int
	Resets    int
	Depleted  int
	Expired   int
	// Absent counts VPNs the panel positively no longer holds (marked
	// SERVER_UNAVAILABLE this pass).
	Absent int
	// Unverified counts VPNs whose absence could not be confirmed — an
	// unreadable panel or an incomplete page-through. Their rows are untouched.
	Unverified int
	// Mirrored counts VPNs whose panel-mirrored limits differed from the panel
	// and were rewritten this pass (D-024.2/.4). A steady panel reports 0.
	Mirrored   int
	Err        error
	DurationMs int64
}

// Result is the outcome of a whole cycle.
type Result struct {
	Servers  []ServerResult
	Duration time.Duration
}

// Service reads panels and records usage history.
type Service struct {
	db      *sql.DB
	st      *store.Store
	factory ClientFactory
	cfg     Config
	log     *slog.Logger
	now     func() time.Time
}

// New builds the service.
func New(db *sql.DB, st *store.Store, factory ClientFactory, cfg Config) *Service {
	if cfg.PageSize <= 0 || cfg.PageSize > 200 {
		cfg.PageSize = DefaultConfig().PageSize
	}
	if cfg.MaxPages <= 0 {
		cfg.MaxPages = DefaultConfig().MaxPages
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultConfig().Timeout
	}
	clock := cfg.Now
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		db: db, st: st, factory: factory, cfg: cfg,
		log: slog.Default(), now: clock,
	}
}

// RunOnce syncs every given server. A failure on one server is recorded on that
// server only and never aborts the cycle (REQUIREMENTS §3: "one slow panel must
// not block the dashboard").
//
// Servers are synced concurrently but with a hard bound, so a 20-panel install
// cannot open 20 sockets at once on a 1 vCPU box.
func (s *Service) RunOnce(ctx context.Context, serverIDs []int64) Result {
	results := make([]ServerResult, len(serverIDs))
	cycleStarted := s.now()

	const maxParallel = 3
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, id := range serverIDs {
		wg.Add(1)
		go func(idx int, serverID int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Each server gets its own deadline, so one hanging panel cannot
			// consume the whole cycle.
			srvCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
			defer cancel()

			started := s.now()
			res := s.syncServer(srvCtx, serverID)
			res.DurationMs = s.now().Sub(started).Milliseconds()
			results[idx] = res
		}(i, id)
	}
	wg.Wait()

	for _, r := range results {
		if r.Err != nil {
			s.log.Warn("sync: server pass failed",
				"server_id", r.ServerID, "err", r.Err, "duration_ms", r.DurationMs)
			continue
		}
		if r.Absent > 0 || r.Unverified > 0 {
			s.log.Info("sync: reconciled VPN presence",
				"server_id", r.ServerID, "absent", r.Absent, "unverified", r.Unverified)
		}
	}
	return Result{Servers: results, Duration: s.now().Sub(cycleStarted)}
}

// syncServer performs one bounded, paginated read of a single panel and records
// a snapshot for every VPN instance that exists locally.
func (s *Service) syncServer(ctx context.Context, serverID int64) (res ServerResult) {
	res.ServerID = serverID

	defer func() {
		if p := recover(); p != nil {
			res.Err = fmt.Errorf("sync: panic on server %d: %v", serverID, p)
		}
	}()

	client, err := s.factory(ctx, serverID)
	if err != nil {
		res.Err = fmt.Errorf("%w: %v", ErrServerUnreachable, err)
		s.recordSyncFailure(ctx, serverID)
		return res
	}

	// Server-wide metrics (CPU/RAM). Recorded separately from client usage so the
	// UI can always label "server-wide" vs "this VPN" (REQUIREMENTS §2.7).
	health := client.Health(ctx)
	if health.Err == nil {
		s.recordMetrics(ctx, serverID, health)
	}

	byEmail, total, err := s.readAllClients(ctx, client)
	if err != nil {
		res.Err = err
		s.recordSyncFailure(ctx, serverID)
		return res
	}
	res.Clients = total

	takenAt := s.now().Unix()
	if err := s.applyReadings(ctx, serverID, client, byEmail, takenAt, &res); err != nil {
		res.Err = err
		return res
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE server_integrations SET last_sync = ? WHERE id = ?`, takenAt, serverID); err != nil {
		res.Err = fmt.Errorf("sync: stamp last_sync: %w", err)
	}
	return res
}

// readAllClients pages through GET /clients/list/paged until the panel's own
// total is satisfied or MaxPages is reached.
func (s *Service) readAllClients(ctx context.Context, client Panel) (map[string]panel.ClientRow, int, error) {
	out := make(map[string]panel.ClientRow, s.cfg.PageSize)

	for page := 1; page <= s.cfg.MaxPages; page++ {
		pageData, err := client.ListClients(ctx, page, s.cfg.PageSize)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: read page %d: %v", ErrServerUnreachable, page, err)
		}
		for _, row := range pageData.Items {
			if row.Email == "" {
				continue
			}
			out[row.Email] = row
		}
		// Stop when the page is short (last page) or the reported total is met.
		if len(pageData.Items) < s.cfg.PageSize {
			break
		}
		if pageData.Total > 0 && len(out) >= pageData.Total {
			break
		}
	}
	return out, len(out), nil
}

// localVPN is the subset of a vpn_instances row sync needs.
//
// The panel_* columns are the MIRROR of what the panel reports (D-024.2): sync
// compares them with the current reading so it only writes when the panel's
// answer actually changed, and so a divergence caused by a direct 3X-UI edit is
// repaired on the next tick (D-024.4 — the panel wins every conflict).
type localVPN struct {
	ID                 int64
	PanelEmail         string
	Status             string
	TrafficLimitBytes  int64
	ExpiresAt          sql.NullInt64
	LifetimeUp         int64
	LifetimeDown       int64
	LastRemoteResetCnt int

	PanelTotalGB    int64
	PanelExpiryTime int64
	PanelLimitIP    int64
	PanelLimitHwid  int64
	PanelEnable     int64
	PanelSyncedAt   sql.NullInt64
}

// limitsFrom extracts the limits a panel row reports.
//
// `enable` is part of the mirror because a client disabled inside 3X-UI is
// real state: the UI must not claim a VPN is working when the panel says it is
// not (D-024.4). The comparison struct is deliberately flat so a difference in
// one field is visible in a test.
type panelLimits struct {
	TotalGB    int64
	ExpiryTime int64
	LimitIP    int64
	LimitHwid  int64
	Enable     int64
}

func limitsFrom(row panel.ClientRow) panelLimits {
	return panelLimits{
		TotalGB:    row.TotalGB,
		ExpiryTime: row.ExpiryTime,
		LimitIP:    int64(row.LimitIP),
		LimitHwid:  int64(row.LimitHwid),
		Enable:     boolToInt64(row.Enable),
	}
}

func (v localVPN) mirrored() panelLimits {
	return panelLimits{
		TotalGB:    v.PanelTotalGB,
		ExpiryTime: v.PanelExpiryTime,
		LimitIP:    v.PanelLimitIP,
		LimitHwid:  v.PanelLimitHwid,
		Enable:     v.PanelEnable,
	}
}

// mirrorLimits copies the panel's own limits onto the local row (D-024.2/.4).
//
// Nothing is derived here and nothing is decided: sync never writes to the
// panel, it only records what the panel said. When the values already match it
// returns false without touching the row, so a steady panel costs no writes and
// `panel_synced_at` keeps meaning "when the mirror last changed", not "when sync
// last ran" (that is `server_integrations.last_sync`).
//
// A row with a NULL `panel_synced_at` was written before the mirror existed: its
// values came from the create-time offer, not from the panel, so it is rewritten
// once even when the numbers happen to match. `forget` does the same for a
// client that is currently impossible to verify — the mirror is cleared so the
// next successful tick rewrites it, because a stale mirror is a claim about a
// panel we could not read.
func (s *Service) mirrorLimits(ctx context.Context, v localVPN, row panel.ClientRow, takenAt int64, forget bool) (bool, error) {
	if !forget && v.PanelSyncedAt.Valid {
		current := limitsFrom(row)
		if current == v.mirrored() {
			return false, nil
		}
	}
	if forget {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE vpn_instances
			    SET panel_total_gb = 0, panel_expiry_time = 0, panel_limit_ip = 0,
			        panel_limit_hwid = 0, panel_enable = 0, panel_synced_at = NULL,
			        updated_at = ?
			  WHERE id = ?`, takenAt, v.ID); err != nil {
			return false, fmt.Errorf("sync: forget panel limits: %w", err)
		}
		return true, nil
	}
	cur := limitsFrom(row)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE vpn_instances
		    SET panel_total_gb = ?, panel_expiry_time = ?, panel_limit_ip = ?,
		        panel_limit_hwid = ?, panel_enable = ?, panel_synced_at = ?,
		        updated_at = ?
		  WHERE id = ?`,
		cur.TotalGB, cur.ExpiryTime, cur.LimitIP, cur.LimitHwid, cur.Enable, takenAt, takenAt, v.ID); err != nil {
		return false, fmt.Errorf("sync: mirror panel limits: %w", err)
	}
	return true, nil
}

// applyReadings folds every panel reading into local history.
func (s *Service) applyReadings(ctx context.Context, serverID int64, client Panel,
	byEmail map[string]panel.ClientRow, takenAt int64, res *ServerResult) error {

	vpns, err := s.localVPNs(ctx, serverID)
	if err != nil {
		return err
	}

	for _, v := range vpns {
		row, seen := byEmail[v.PanelEmail]
		if !seen {
			// The VPN was not in this page-through. That alone is NOT proof it
			// is gone — pagination can truncate, and a panel cannot be trusted
			// to have shown everything. Absence is therefore *verified* with a
			// direct read before any state changes (D-024.5).
			//
			// Outcomes:
			//   gone  + 404            -> SERVER_UNAVAILABLE (never REVOKED)
			//   exists                -> re-read below, normal path
			//   read failed / timeout -> change NOTHING (an outage is invisible)
			exists, verifyErr := client.ClientExists(ctx, v.PanelEmail)
			switch {
			case verifyErr != nil:
				// Unknown outcome: leave the row exactly as it was. An
				// unreadable panel must never look like a deleted client.
				//
				// The mirrored limits are the one exception: they are a claim
				// about a panel we just failed to read, so they are cleared and
				// rewritten by the next tick that succeeds. The VPN's own state
				// is untouched.
				if _, err := s.mirrorLimits(ctx, v, panel.ClientRow{}, takenAt, true); err != nil {
					return err
				}
				res.Unverified++
				continue
			case exists:
				// The page-through was incomplete, not the panel. The usage for
				// this client is unknown this tick, so skip the snapshot without
				// touching its state or its limits.
				res.Unverified++
				continue
			default:
				// Positively absent. A VPN with no panel client is not a
				// working VPN: mark it, and say so (the client can then remove
				// it — D-027). REVOKED is deliberately not used here: only a
				// verified *delete we performed* may produce REVOKED.
				if err := s.markAbsent(ctx, v, takenAt); err != nil {
					return err
				}
				res.Absent++
				continue
			}
		}
		usage := readUsage(row)

		// D-024.2/.4: the panel's own limits become the local mirror. This runs
		// before nextStatus so a capacity/expiry the operator changed directly
		// in 3X-UI is reflected in the same tick that reads the usage.
		mirrored, err := s.mirrorLimits(ctx, v, row, takenAt, false)
		if err != nil {
			return err
		}
		if mirrored {
			res.Mirrored++
		}

		prevUsage := s.previousUsage(ctx, v.ID)
		folded := Fold(
			Lifetime{UpBytes: v.LifetimeUp, DownBytes: v.LifetimeDown, ResetCount: v.LastRemoteResetCnt},
			prevUsage, usage,
		)

		if err := s.writeSnapshot(ctx, v.ID, takenAt, usage, folded); err != nil {
			return err
		}
		res.Snapshots++
		if folded.ResetDetected {
			res.Resets++
		}

		next, changed := nextStatus(v, row, folded, s.now())
		if changed {
			if _, err := s.db.ExecContext(ctx,
				`UPDATE vpn_instances SET status = ?, updated_at = ? WHERE id = ?`,
				string(next), takenAt, v.ID); err != nil {
				return fmt.Errorf("sync: update status: %w", err)
			}
			switch next {
			case domain.VPNTrafficDepleted:
				res.Depleted++
			case domain.VPNExpired:
				res.Expired++
			}
		}
	}
	return nil
}

// markAbsent records that the panel positively no longer holds a VPN's client.
//
// The state is SERVER_UNAVAILABLE, never REVOKED (D-024.5 / REQUIREMENTS §7): we
// did not delete the client and cannot know why it went away. The row is kept
// so the client's own record of the purchase survives and so they can release
// it (D-027); it is excluded from every "working VPN" projection.
func (s *Service) markAbsent(ctx context.Context, v localVPN, takenAt int64) error {
	current := domain.VPNStatus(v.Status)
	if current == domain.VPNRevoked || current == domain.VPNRemovedFromCatalog {
		return nil
	}
	// A released VPN is absent from the panel BY DESIGN: releasing disabled its
	// client, so the sync pass is describing our own action, not a fault. Calling
	// it SERVER_UNAVAILABLE told the customer "the server broke" during an undo
	// window they had opened themselves (D-027).
	//
	// The state machine also refuses RELEASED → SERVER_UNAVAILABLE, so this is a
	// second line of defence rather than the only one.
	if current == domain.VPNReleased {
		return nil
	}
	if !domain.CanTransition(current, domain.VPNServerUnavailable) {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE vpn_instances SET status = ?, updated_at = ? WHERE id = ?`,
		string(domain.VPNServerUnavailable), takenAt, v.ID); err != nil {
		return fmt.Errorf("sync: mark absent: %w", err)
	}
	s.log.Info("sync: panel no longer holds this VPN; marked unavailable",
		"vpn_id", v.ID, "panel_email", v.PanelEmail, "from", v.Status)
	return nil
}

// readUsage extracts the counters from a panel row, tolerating a missing
// traffic block (a client can exist before any traffic is recorded).
func readUsage(row panel.ClientRow) Usage {
	if row.Traffic == nil {
		return Usage{}
	}
	return Usage{
		UpBytes:    row.Traffic.Up,
		DownBytes:  row.Traffic.Down,
		ResetCount: row.Traffic.ResetCount,
	}
}

// nextStatus decides whether the observed usage/expiry changes the VPN state.
//
// Every candidate is validated against the domain state machine, so sync can
// never write a state the lifecycle rules forbid, and it can never produce
// REVOKED (an unverified absence is not a revocation — REQUIREMENTS §7).
func nextStatus(v localVPN, row panel.ClientRow, folded DeltaResult, now time.Time) (domain.VPNStatus, bool) {
	current := domain.VPNStatus(v.Status)

	candidates := make([]domain.VPNStatus, 0, 3)

	// Expiry wins: an expired VPN must not also read as traffic-depleted.
	if v.ExpiresAt.Valid && v.ExpiresAt.Int64 > 0 && now.Unix() >= v.ExpiresAt.Int64 {
		candidates = append(candidates, domain.VPNExpired)
	}
	if folded.Lifetime.Exceeded(v.TrafficLimitBytes) {
		candidates = append(candidates, domain.VPNTrafficDepleted)
	}
	// The panel disabled the client while we still consider it ACTIVE: reflect it
	// so the UI never shows a working VPN that cannot connect.
	if !row.Enable {
		candidates = append(candidates, domain.VPNSuspended)
	}

	for _, to := range candidates {
		if domain.CanTransition(current, to) {
			return to, true
		}
	}
	return current, false
}

// localVPNs reads the VPN instances of one server.
func (s *Service) localVPNs(ctx context.Context, serverID int64) ([]localVPN, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, panel_email, status, traffic_limit_bytes, expires_at,
		        lifetime_up_bytes, lifetime_down_bytes, last_remote_reset_count,
		        panel_total_gb, panel_expiry_time, panel_limit_ip, panel_limit_hwid,
		        panel_enable, panel_synced_at
		   FROM vpn_instances
		  WHERE server_id = ? AND status NOT IN ('REVOKED','REMOVED_FROM_CATALOG')`, serverID)
	if err != nil {
		return nil, fmt.Errorf("sync: list local vpns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]localVPN, 0, 32)
	for rows.Next() {
		var v localVPN
		if err := rows.Scan(&v.ID, &v.PanelEmail, &v.Status, &v.TrafficLimitBytes,
			&v.ExpiresAt, &v.LifetimeUp, &v.LifetimeDown, &v.LastRemoteResetCnt,
			&v.PanelTotalGB, &v.PanelExpiryTime, &v.PanelLimitIP, &v.PanelLimitHwid,
			&v.PanelEnable, &v.PanelSyncedAt); err != nil {
			return nil, fmt.Errorf("sync: scan vpn: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// previousUsage returns the counters of the most recent snapshot for a VPN, so
// Fold can compute a delta. A VPN with no history yields zero, which makes the
// first snapshot a baseline rather than a giant jump.
func (s *Service) previousUsage(ctx context.Context, vpnID int64) Usage {
	var u Usage
	err := s.db.QueryRowContext(ctx,
		`SELECT up_bytes, down_bytes, remote_reset_count
		   FROM usage_snapshots WHERE vpn_instance_id = ?
		  ORDER BY taken_at DESC, id DESC LIMIT 1`, vpnID,
	).Scan(&u.UpBytes, &u.DownBytes, &u.ResetCount)
	if err != nil {
		return Usage{}
	}
	return u
}

// writeSnapshot stores one observation and moves the lifetime total forward in
// the SAME transaction, so a crash between the two can never leave them
// disagreeing.
//
// The ON CONFLICT keeps a re-run of the same second idempotent instead of
// producing duplicate rows for one tick.
func (s *Service) writeSnapshot(ctx context.Context, vpnID, takenAt int64, usage Usage, folded DeltaResult) error {
	return s.st.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO usage_snapshots
			   (vpn_instance_id, taken_at, up_bytes, down_bytes,
			    lifetime_up_bytes, lifetime_down_bytes, remote_reset_count)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (vpn_instance_id, taken_at) DO UPDATE SET
			   up_bytes = excluded.up_bytes,
			   down_bytes = excluded.down_bytes,
			   lifetime_up_bytes = excluded.lifetime_up_bytes,
			   lifetime_down_bytes = excluded.lifetime_down_bytes,
			   remote_reset_count = excluded.remote_reset_count`,
			vpnID, takenAt, usage.UpBytes, usage.DownBytes,
			folded.Lifetime.UpBytes, folded.Lifetime.DownBytes, folded.Lifetime.ResetCount,
		); err != nil {
			return fmt.Errorf("sync: insert snapshot: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_instances
			    SET lifetime_up_bytes = ?, lifetime_down_bytes = ?,
			        last_remote_reset_count = ?, updated_at = ?
			  WHERE id = ?`,
			folded.Lifetime.UpBytes, folded.Lifetime.DownBytes,
			folded.Lifetime.ResetCount, takenAt, vpnID,
		); err != nil {
			return fmt.Errorf("sync: update lifetime: %w", err)
		}
		return nil
	})
}

// recordMetrics stores one server-wide sample.
//
// `net_up` / `net_down` are BYTES PER SECOND, computed as the difference between
// this reading and the previous one divided by the elapsed seconds. The panel
// reports cumulative byte counters, so storing those directly would make the
// chart climb forever; storing a constant 0 (the previous behaviour, M6-12) drew
// a flat zero line that claimed "no traffic". A negative difference means the
// interface counter reset, which is recorded as 0 rather than as a huge spike.
func (s *Service) recordMetrics(ctx context.Context, serverID int64, h panel.HealthResult) {
	now := s.now().Unix()

	var prevAt int64 = 0
	var prevUp, prevDown int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT taken_at, net_up, net_down FROM server_metrics
		  WHERE server_id = ? ORDER BY taken_at DESC LIMIT 1`, serverID,
	).Scan(&prevAt, &prevUp, &prevDown)

	upRate, downRate := int64(0), int64(0)
	if prevAt > 0 && now > prevAt {
		elapsed := now - prevAt
		if upRate = rate(prevUp, h.NetUpBytes, elapsed); upRate < 0 {
			upRate = 0
		}
		if downRate = rate(prevDown, h.NetDownBytes, elapsed); downRate < 0 {
			downRate = 0
		}
	}

	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO server_metrics (server_id, taken_at, cpu_pct, mem_used, mem_total, net_up, net_down)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (server_id, taken_at) DO UPDATE SET
		   cpu_pct = excluded.cpu_pct, mem_used = excluded.mem_used, mem_total = excluded.mem_total,
		   net_up = excluded.net_up, net_down = excluded.net_down`,
		serverID, now, h.CPUPct, h.MemCurrent, h.MemTotal, upRate, downRate)
}

// rate converts two cumulative readings into a per-second rate.
func rate(previous, current, elapsedSeconds int64) int64 {
	if elapsedSeconds <= 0 {
		return 0
	}
	return (current - previous) / elapsedSeconds
}

// recordSyncFailure marks an integration unreachable so the operator sees it.
//
// Only ACTIVE integrations are touched (a DISABLED one was taken offline on
// purpose), and no VPN row is modified: an unreadable panel must never be
// interpreted as "the clients are gone" (REQUIREMENTS §7).
func (s *Service) recordSyncFailure(ctx context.Context, serverID int64) {
	_, _ = s.db.ExecContext(ctx,
		`UPDATE server_integrations
		    SET status = 'CONNECTION_UNAVAILABLE', updated_at = ?
		  WHERE id = ? AND status = 'ACTIVE'`,
		s.now().Unix(), serverID)
}
