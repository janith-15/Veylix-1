package integrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"veylix/backend/internal/audit"
)

// Purge permanently deletes an integration and everything that only exists
// because of it (cached inbounds, metrics, pending outbox work).
//
// This is the operator's "remove the whole server" action. It is deliberately
// NOT the same as Remove, which only flips the row to REMOVED so billing and
// assignment history survive (REQUIREMENTS §3.3):
//
//   - Purge refuses with ErrInUse while any VPN instance still points at the
//     server. `vpn_instances.server_id` is `ON DELETE RESTRICT`, so deleting the
//     row first would either fail half-way or — worse — destroy client history.
//     The operator must migrate or revoke those instances first.
//   - The delete is one transaction: a partial delete would leave inbounds
//     orphaned or metrics pointing at a server that no longer exists.
//   - The audit entry is written BEFORE the delete, because the target id is
//     about to disappear.
func (s *Service) Purge(ctx context.Context, id, actorID int64) error {
	var (
		name     string
		status   string
		vpnCount int
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT name, status,
		        (SELECT COUNT(*) FROM vpn_instances v WHERE v.server_id = s.id)
		   FROM server_integrations s WHERE s.id = ?`, id,
	).Scan(&name, &status, &vpnCount)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("integrations: purge lookup: %w", err)
	}
	if vpnCount > 0 {
		return fmt.Errorf("%w: %d VPN instance(s) still reference %q", ErrInUse, vpnCount, name)
	}

	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actorID, Action: audit.ActionServerRemove, TargetType: "server",
		TargetID: fmt.Sprint(id),
		Context:  map[string]any{"name": name, "previous_status": status, "hard_delete": true},
	})

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("integrations: purge begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Child rows that are not covered by ON DELETE CASCADE in every schema
	// version are cleared explicitly so the order is explicit and auditable.
	for _, stmt := range []string{
		`DELETE FROM inbounds WHERE server_id = ?`,
		`DELETE FROM admin_server_assignments WHERE server_id = ?`,
		`DELETE FROM server_metrics WHERE server_id = ?`,
		`DELETE FROM outbox WHERE server_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, id); err != nil {
			return fmt.Errorf("integrations: purge children: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM server_integrations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("integrations: purge server: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("integrations: purge commit: %w", err)
	}
	return nil
}
