package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"veylix/backend/internal/store"
)

// verifyEmail flips email_verified to 1 and records who did it and why.
//
// This is the manual path for a deployment with no SMTP relay: an operator who
// has confirmed the address by other means marks it. It is audited.
func verifyEmail(ctx context.Context, st *store.Store, email string, dryRun bool) error {
	email = strings.ToLower(strings.TrimSpace(email))
	var (
		id      int64
		role    string
		already int
	)
	err := st.DB().QueryRowContext(ctx,
		`SELECT id, role, email_verified FROM users WHERE email = ?`, email).Scan(&id, &role, &already)
	if err != nil {
		return fmt.Errorf("no account with e-mail %s: %w", email, err)
	}
	if already == 1 {
		slog.Info("already verified", "email", email, "id", id, "role", role)
		return nil
	}
	if dryRun {
		slog.Info("would verify", "email", email, "id", id, "role", role)
		return nil
	}
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE users SET email_verified = 1, status = 'ACTIVE', updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id); err != nil {
		return fmt.Errorf("verify %s: %w", email, err)
	}
	writeAudit(ctx, st, id, "user.email_verified", map[string]string{"source": "accounttool", "method": "manual"})
	slog.Info("verified", "email", email, "id", id, "role", role)
	return nil
}

// purgeChatData deletes every chat conversation and message.
//
// Order matters: child tables are cleared first so the foreign keys never need a
// cascade, and the counts are read BEFORE deleting so the report says what was
// actually removed. The database is copied first — this command sits next to
// production data, so the wrong click must be reversible.
func purgeChatData(ctx context.Context, st *store.Store, dbPath string, dryRun bool) error {
	tables := []string{
		"chat_message_hidden",
		"chat_message_saved",
		"chat_message_important",
		"chat_messages",
		"chat_participants",
		"chat_conversations",
	}

	counts := make(map[string]int64, len(tables))
	for _, t := range tables {
		var n int64
		if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM `+t).Scan(&n); err != nil {
			return fmt.Errorf("count %s: %w", t, err)
		}
		counts[t] = n
		slog.Info("chat rows", "table", t, "count", n)
	}

	backup := ""
	if !dryRun {
		backup = filepath.Join(filepath.Dir(dbPath),
			fmt.Sprintf("veylix.pre-purge-%s.db", time.Now().Format("20060102-150405")))
		if err := copyFile(dbPath, backup); err != nil {
			return fmt.Errorf("backup before purge: %w", err)
		}
		slog.Info("backup written", "path", backup)
	}
	if dryRun {
		slog.Info("dry run: nothing deleted")
		return nil
	}

	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t); err != nil {
			return fmt.Errorf("delete %s: %w", t, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, context, created_at)
		 VALUES (NULL, 'chat.purge', 'chat', '-', 'OK', ?, ?)`,
		fmt.Sprintf(`{"source":"accounttool","messages":%d,"conversations":%d}`,
			counts["chat_messages"], counts["chat_conversations"]),
		time.Now().Unix()); err != nil {
		return fmt.Errorf("audit purge: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Info("chat purged", "messages", counts["chat_messages"],
		"conversations", counts["chat_conversations"], "backup", backup)
	return nil
}

// copyFile copies src to dst, creating dst with owner-only permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// isLoopbackAddr reports whether the configured bind address is loopback-only.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
