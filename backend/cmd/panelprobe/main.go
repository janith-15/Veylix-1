// Command panelprobe is a READ-ONLY diagnostic: for every locally recorded VPN
// instance it reads the owning integration's panel URL and asks that panel
// whether it is reachable and whether the instance's client actually exists.
//
// Why it exists: the owner-reported symptom "VPNs are listed in Veylix but are
// not visible as clients in 3X-UI" (ledger F-028) cannot be answered from the
// database alone — it needs the panel's own answer. This binary prints that
// answer per row instead of guessing.
//
// Design rules (mirrors cmd/seeddemo):
//   - it is a SEPARATE binary; the server never runs probes itself;
//   - it never writes: no UPDATE/INSERT/DELETE and no panel write endpoint.
//     Only GET /panel/api/server/status and GET /panel/api/clients/get/{email};
//   - the sealed panel token is opened per server and never printed or logged;
//   - a probe failure is reported as UNKNOWN, never as "the client is missing":
//     an unreachable panel proves nothing (REQUIREMENTS §3).
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/config"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

// probeOutcome is what the panel said about one instance.
type probeOutcome string

const (
	outcomeFound    probeOutcome = "FOUND"    // the panel positively reports the client
	outcomeMissing  probeOutcome = "MISSING"  // the panel positively says it does not exist
	outcomeUnknown  probeOutcome = "UNKNOWN"  // the probe could not decide — not evidence
	outcomeDisabled probeOutcome = "DISABLED" // integration is not ACTIVE, so it was not polled
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("panelprobe failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		serverFlag  = flag.Int64("server", 0, "only probe instances on this integration id (0 = all)")
		vpnFlag     = flag.Int64("vpn", 0, "only probe this VPN instance id (0 = all)")
		timeoutFlag = flag.Duration("timeout", 60*time.Second, "overall deadline for the whole probe run")
		clientFlag  = flag.Duration("client-timeout", 10*time.Second, "deadline for a single panel request")
	)
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	// Read-only: the database is opened but NOT migrated. A diagnostic must not
	// change the schema of the database it is inspecting.
	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("database %s is not readable: %w (is VEYLIX_DATA_DIR correct?)", cfg.DBPath, err)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	slog.Info("probing", "db", st.Path(), "server", *serverFlag, "vpn", *vpnFlag)

	instances, err := loadInstances(ctx, st.DB(), *serverFlag, *vpnFlag)
	if err != nil {
		// A stream error is reported before anything is probed: a truncated
		// instance list must never be presented as a complete answer.
		return err
	}
	if len(instances) == 0 {
		slog.Info("no VPN instances matched the filters — nothing to probe")
		return nil
	}

	// One health read per integration, never per instance: the panel caches the
	// status for ~2 s, and polling it per row would be a request storm.
	health := make(map[int64]panel.HealthResult, 4)
	suspects := 0

	for _, inst := range instances {
		status, ok := health[inst.serverID]
		if !ok {
			status = probeIntegration(ctx, st.DB(), cfg.EncryptionKey, inst, *clientFlag)
			health[inst.serverID] = status
			logHealth(inst, status)
		}

		if status.Status != panel.StatusActive {
			q := qualified{inst: inst, result: outcomeDisabled, detail: "integration status " + string(status.Status)}
			if status.Err != nil {
				q.detail += ": " + status.Err.Error()
			}
			logQualified(q)
			continue
		}

		q := probeClient(ctx, st.DB(), cfg.EncryptionKey, inst, *clientFlag)
		logQualified(q)
		if q.result != outcomeFound {
			suspects++
		}
	}

	slog.Info("done", "instances", len(instances), "integrations", len(health), "suspects", suspects)
	if suspects > 0 {
		return fmt.Errorf("%d of %d instances are not confirmed on their panel (see MISSING/UNKNOWN/DISABLED rows)", suspects, len(instances))
	}
	return nil
}

// instance is one vpn_instances row joined with its integration.
type instance struct {
	vpnID      int64
	serverID   int64
	panelURL   string
	panelEmail string
	status     string
}

// qualified is one instance's probe result.
type qualified struct {
	inst   instance
	result probeOutcome
	detail string
}

// loadInstances reads every VPN instance and the panel URL of the integration
// that hosts it.
//
// This is the query panelprobe exists for. Two rules apply to it:
//
//  1. rows is always closed (defer) — a leaked *sql.Rows pins a connection out
//     of the four-connection pool, and with WAL enabled the next write waits
//     on it;
//  2. rows.Err() is checked after the loop. Next() returning false means "no
//     more rows" OR "the iteration stopped on an error", so without the check a
//     mid-stream failure (busy timeout, context cancel, I/O error) returns a
//     shorter list that looks like a complete answer. For a diagnostic that
//     reads as "these VPNs do not exist", a silently truncated list would be a
//     wrong conclusion, not just an incomplete one.
func loadInstances(ctx context.Context, db *sql.DB, serverID, vpnID int64) ([]instance, error) {
	const base = `SELECT i.id, i.server_id, s.panel_url, i.panel_email, i.status
		   FROM vpn_instances i JOIN server_integrations s ON s.id = i.server_id`

	query := base
	var args []any
	switch {
	case serverID > 0 && vpnID > 0:
		query += ` WHERE i.server_id = ? AND i.id = ? ORDER BY i.id`
		args = []any{serverID, vpnID}
	case serverID > 0:
		query += ` WHERE i.server_id = ? ORDER BY i.id`
		args = []any{serverID}
	case vpnID > 0:
		query += ` WHERE i.id = ? ORDER BY i.id`
		args = []any{vpnID}
	default:
		query += ` ORDER BY i.id`
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("panelprobe: list instances: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]instance, 0, 64)
	for rows.Next() {
		var in instance
		if err := rows.Scan(&in.vpnID, &in.serverID, &in.panelURL, &in.panelEmail, &in.status); err != nil {
			return nil, fmt.Errorf("panelprobe: scan instance: %w", err)
		}
		out = append(out, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("panelprobe: iterate instances: %w", err)
	}
	return out, nil
}

// integrationRow is the credential-bearing part of a server_integrations row.
type integrationRow struct {
	status       string
	panelURL     string
	tokenSealed  []byte
	allowPrivate bool
}

// loadIntegration reads the integration behind an instance.
func loadIntegration(ctx context.Context, db *sql.DB, serverID int64) (*integrationRow, error) {
	var r integrationRow
	var allow int
	err := db.QueryRowContext(ctx,
		`SELECT status, panel_url, token_ciphertext, allow_private_network
		   FROM server_integrations WHERE id = ?`, serverID,
	).Scan(&r.status, &r.panelURL, &r.tokenSealed, &allow)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("integration %d does not exist", serverID)
	}
	if err != nil {
		return nil, fmt.Errorf("panelprobe: load integration %d: %w", serverID, err)
	}
	r.allowPrivate = allow == 1
	return &r, nil
}

// panelClient builds a read-only panel client for an integration.
//
// The token is sealed with AES-256-GCM bound to the panel URL (integrations
// addAAD), so it can only be opened with the same URL it was stored with. The
// AAD here must match that construction exactly or every probe would report an
// unopenable credential.
func panelClient(key []byte, row *integrationRow, timeout time.Duration) (*panel.Client, error) {
	sealer, err := auth.NewSealer(key)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte("integration:" + strings.ToLower(row.panelURL)))
	plain, err := sealer.Open(row.tokenSealed, sum[:])
	if err != nil {
		return nil, errors.New("stored panel token cannot be decrypted with VEYLIX_ENCRYPTION_KEY")
	}

	target, err := panel.ParseTarget(row.panelURL, row.allowPrivate, nil)
	if err != nil {
		// An IP literal needs no resolver; a hostname does.
		target, err = panel.ParseTarget(row.panelURL, row.allowPrivate, net.LookupIP)
		if err != nil {
			return nil, err
		}
	}
	return panel.New(panel.Options{Target: target, Token: string(plain), Timeout: timeout})
}

// probeIntegration performs the authenticated health check once for a server.
func probeIntegration(ctx context.Context, db *sql.DB, key []byte, inst instance, timeout time.Duration) panel.HealthResult {
	row, err := loadIntegration(ctx, db, inst.serverID)
	if err != nil {
		// Report per server instead of aborting the whole run: one missing
		// integration row must not hide the verdict for every other instance.
		return panel.HealthResult{Status: panel.StatusConnectionUnavailable, Err: err}
	}
	if row.status != string(panel.StatusActive) {
		// Never poll a DISABLED/REAUTH_REQUIRED/REMOVED integration: a failed
		// credential must not generate traffic (REQUIREMENTS §3).
		return panel.HealthResult{Status: panel.Status(row.status)}
	}
	client, err := panelClient(key, row, timeout)
	if err != nil {
		return panel.HealthResult{Status: panel.StatusReauthRequired, Err: err}
	}
	return client.Health(ctx)
}

// probeClient asks the panel whether one instance's client exists.
//
// The three outcomes are kept distinct on purpose: only a positive "not found"
// from the panel may be reported as MISSING, because reporting an outage as a
// missing VPN is exactly the defect F-028 describes.
func probeClient(ctx context.Context, db *sql.DB, key []byte, inst instance, timeout time.Duration) qualified {
	row, err := loadIntegration(ctx, db, inst.serverID)
	if err != nil {
		return qualified{inst: inst, result: outcomeUnknown, detail: err.Error()}
	}
	client, err := panelClient(key, row, timeout)
	if err != nil {
		return qualified{inst: inst, result: outcomeUnknown, detail: err.Error()}
	}
	if err := panel.ValidateEmail(inst.panelEmail); err != nil {
		return qualified{inst: inst, result: outcomeUnknown, detail: err.Error()}
	}

	exists, err := client.ClientExists(ctx, inst.panelEmail)
	switch {
	case err != nil:
		return qualified{inst: inst, result: outcomeUnknown, detail: err.Error()}
	case exists:
		return qualified{inst: inst, result: outcomeFound}
	default:
		return qualified{inst: inst, result: outcomeMissing,
			detail: "panel positively reports no such client"}
	}
}

// logHealth reports one integration's health. The token is never logged.
func logHealth(inst instance, h panel.HealthResult) {
	slog.Info("integration",
		"server_id", inst.serverID,
		"panel_url", inst.panelURL,
		"status", string(h.Status),
		"panel_version", h.PanelVersion,
		"xray_version", h.XrayVersion,
		"xray_state", h.XrayState,
		"latency_ms", h.LatencyMS,
		"err", errText(h.Err),
	)
}

// logQualified reports one instance's result.
func logQualified(q qualified) {
	slog.Info("instance",
		"vpn_id", q.inst.vpnID,
		"server_id", q.inst.serverID,
		"panel_email", q.inst.panelEmail,
		"local_status", q.inst.status,
		"probe", string(q.result),
		"detail", q.detail,
	)
}

// errText renders an error for logging without ever emitting a nil pointer.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
