package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"veylix/backend/internal/store"
)

// resetExceptSuperAdmin wipes every row except the super_admin accounts.
//
// Why it exists: the demo seed and manual testing leave VPNs, orders, payments,
// chat and usage history behind, and a fresh start is otherwise a hand-written
// DELETE list that nobody can review. The list below is explicit and ordered so
// a foreign key is never relied on for a cascade.
//
// Safety, in order:
//  1. every table's row count is read and logged BEFORE anything is deleted;
//  2. the database file is copied to veylix.pre-reset-<stamp>.db;
//  3. all deletes run in ONE transaction, so a failure leaves no half-state;
//  4. the deletion itself is written to the audit log.
//
// What is deliberately KEPT:
//   - users with role = super_admin (the owner's own login);
//   - payment_methods and pricing_rates — these are configuration the operator
//     entered by hand, not customer data, and re-typing them is the kind of
//     chore a reset should not create;
//   - schema_migrations (the schema version).
func resetExceptSuperAdmin(ctx context.Context, st *store.Store, dbPath string, dryRun bool) error {
	// Child tables first. Order is explicit rather than trusting ON DELETE.
	tables := []string{
		"chat_message_hidden",
		"chat_message_saved",
		"chat_message_important",
		"chat_messages",
		"chat_participants",
		"chat_conversations",
		"panel_assignments",
		"panel_client_tags",
		"vpn_assignments",
		"vpn_instances",
		"vpn_releases",
		"vpn_plan_purchases",
		"free_claims",
		"payment_record_events",
		"payment_records",
		"quota_ledger",
		"usage_snapshots",
		"server_metrics",
		"outbox",
		"notifications",
		"email_verifications",
		"email_change_requests",
		"password_reset_codes",
		"oauth_states",
		"auth_identities",
		"client_storage_limits",
		"sessions",
		"admin_server_assignments",
		"inbounds",
		"server_integrations",
		"vpn_offers",
		"audit_logs",
	}

	counts := make(map[string]int64, len(tables)+1)
	var usersBefore, superAdmins int64

	for _, t := range tables {
		var n int64
		if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM `+t).Scan(&n); err != nil {
			return fmt.Errorf("count %s: %w", t, err)
		}
		counts[t] = n
		if n > 0 {
			slog.Info("will delete", "table", t, "rows", n)
		}
	}
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&usersBefore); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE role = 'super_admin'`).Scan(&superAdmins); err != nil {
		return fmt.Errorf("count super admins: %w", err)
	}
	if superAdmins == 0 {
		return fmt.Errorf("refusing to reset: there is no super_admin to keep")
	}
	slog.Info("users", "total", usersBefore, "keep(super_admin)", superAdmins,
		"delete", usersBefore-superAdmins)

	if dryRun {
		slog.Info("dry run: nothing deleted")
		return nil
	}

	// Back up first: a reset next to real data must be reversible.
	backup := filepath.Join(filepath.Dir(dbPath),
		fmt.Sprintf("veylix.pre-reset-%s.db", time.Now().Format("20060102-150405")))
	if err := copyFile(dbPath, backup); err != nil {
		return fmt.Errorf("backup before reset: %w", err)
	}
	slog.Info("backup written", "path", backup)

	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	deleted := int64(0)
	for _, t := range tables {
		res, err := tx.ExecContext(ctx, `DELETE FROM `+t)
		if err != nil {
			return fmt.Errorf("delete %s: %w", t, err)
		}
		n, _ := res.RowsAffected()
		deleted += n
	}
	// Users last, and only the non-owners.
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE role <> 'super_admin'`)
	if err != nil {
		return fmt.Errorf("delete users: %w", err)
	}
	usersDeleted, _ := res.RowsAffected()
	deleted += usersDeleted

	// An audit row survives the purge it describes: it is written inside the same
	// transaction, after the clear, so the log starts with why it is empty.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, context, created_at)
		 VALUES (NULL, 'data.reset', 'database', '-', 'OK', ?, ?)`,
		fmt.Sprintf(`{"source":"accounttool","rows_deleted":%d,"users_deleted":%d,"kept":"super_admin","backup":%q}`,
			deleted, usersDeleted, filepath.Base(backup)),
		time.Now().Unix()); err != nil {
		return fmt.Errorf("audit reset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	slog.Info("reset complete", "rows_deleted", deleted, "users_deleted", usersDeleted,
		"super_admins_kept", superAdmins, "backup", backup)
	return nil
}
