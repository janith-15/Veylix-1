package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// provisionRequest is one "give me an account I can log in with" request.
type provisionRequest struct {
	role     auth.Role
	email    string
	username string
	name     string
	quota    int
	pattern  string
	dryRun   bool
}

// provision creates or resets a staff account and emits the new password.
//
// Behaviour, and why:
//   - an EXISTING account of the SAME role is reset (recovery: a forgotten super
//     admin password must always be recoverable by the operator);
//   - an existing account of a DIFFERENT role is refused — provisioning must never
//     turn a client into a super admin by accident;
//   - a new account is created verified and ACTIVE, because it is handed to a real
//     person in writing.
func provision(ctx context.Context, st *store.Store, req provisionRequest) error {
	pattern := auth.PasswordPattern(req.pattern)
	if !pattern.Valid() {
		pattern = auth.PatternReadable
	}
	password, err := auth.GeneratePassword(pattern)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	email := strings.ToLower(strings.TrimSpace(req.email))
	if email == "" {
		if req.role != auth.RoleSuperAdmin {
			return errors.New("-admin needs -email")
		}
		email = "owner@veylix.test"
	}
	username := normaliseUsername(req.username, email)
	name := strings.TrimSpace(req.name)
	if name == "" {
		name = "Owner"
		if req.role == auth.RoleAdmin {
			name = email
		}
	}

	id, role, lookupErr := findByEmailOrUsername(ctx, st, email, username)
	switch {
	case lookupErr == nil && auth.Role(role) != req.role:
		return fmt.Errorf("%s belongs to a %s account, not %s - provisioning never changes a role",
			email, role, req.role)
	case lookupErr == nil:
		if req.dryRun {
			slog.Info("would reset the password", "email", email, "username", username, "role", role)
			return nil
		}
		if err := resetStaff(ctx, st, id, hash, username, name, req.quota, req.role); err != nil {
			return err
		}
		slog.Info("password reset", "email", email, "username", username, "role", role, "id", id)
		emitCredential(email, username, role, password)
		return nil
	case !errors.Is(lookupErr, sql.ErrNoRows):
		return lookupErr
	}

	if req.dryRun {
		slog.Info("would create", "email", email, "username", username, "role", req.role)
		return nil
	}
	newID, err := insertStaff(ctx, st, email, username, name, hash, req.quota, req.role)
	if err != nil {
		return err
	}
	slog.Info("created", "email", email, "username", username, "role", req.role, "id", newID)
	emitCredential(email, username, string(req.role), password)
	return nil
}

// emitCredential prints the single line dev.ps1 captures. A fixed prefix and a JSON
// body let the script move it into .veylix-secrets and keep the password out of the
// console log and out of shell history.
func emitCredential(email, username, role, password string) {
	payload, _ := json.Marshal(map[string]string{
		"email": email, "username": username, "role": role, "password": password,
	})
	fmt.Printf("CREDENTIAL %s\n", payload)
}

// findByEmailOrUsername resolves either identifier to (id, role).
func findByEmailOrUsername(ctx context.Context, st *store.Store, email, username string) (int64, string, error) {
	var (
		id   int64
		role string
	)
	err := st.DB().QueryRowContext(ctx,
		`SELECT id, role FROM users WHERE email = ?`, email).Scan(&id, &role)
	if err == nil {
		return id, role, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("lookup by e-mail: %w", err)
	}
	// username only exists after migration 0013; a missing column simply means
	// "no match by username", never a crash.
	if hasUsername(ctx, st) {
		if uErr := st.DB().QueryRowContext(ctx,
			`SELECT id, role FROM users WHERE username = ?`, username).Scan(&id, &role); uErr == nil {
			return id, role, nil
		}
	}
	return 0, "", err
}

// insertStaff writes one verified, active staff account.
func insertStaff(ctx context.Context, st *store.Store, email, username, name, hash string, quota int, role auth.Role) (int64, error) {
	now := time.Now().Unix()
	limit := 0
	if role == auth.RoleAdmin {
		limit = quota
		if limit == 0 {
			limit = 25 // the same sensible reseller default as seeddemo
		}
	}

	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (email, username, password_hash, display_name, role, status, email_verified, quota_limit, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'ACTIVE', 1, ?, ?, ?)`,
		email, username, hash, name, string(role), limit, now, now)
	if err != nil {
		return 0, fmt.Errorf("insert %s: %w", email, err)
	}
	id, _ := res.LastInsertId()
	writeAudit(ctx, st, id, "user.provision", map[string]string{"role": string(role), "source": "accounttool"})
	return id, nil
}

// resetStaff rotates a password and re-states the role-safe fields.
func resetStaff(ctx context.Context, st *store.Store, id int64, hash, username, name string, quota int, role auth.Role) error {
	now := time.Now().Unix()
	limit := 0
	if role == auth.RoleAdmin {
		limit = quota
		if limit == 0 {
			limit = 25
		}
	}
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE users SET password_hash = ?,
		        display_name = COALESCE(NULLIF(?, ''), display_name),
		        username = ?, quota_limit = ?, status = 'ACTIVE', email_verified = 1, updated_at = ?
		  WHERE id = ?`, hash, name, username, limit, now, id); err != nil {
		return fmt.Errorf("reset %d: %w", id, err)
	}
	// A rotation must end every live session: a cookie issued under the old
	// password would otherwise outlive it.
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("revoke sessions for %d: %w", id, err)
	}
	writeAudit(ctx, st, id, "user.password_reset_operator", map[string]string{"source": "accounttool"})
	return nil
}

// hasUsername reports whether migration 0013 has run (the column exists).
func hasUsername(ctx context.Context, st *store.Store) bool {
	var n int
	err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'username'`).Scan(&n)
	return err == nil && n > 0
}

// writeAudit records a provisioning write; a failure is logged, never fatal.
func writeAudit(ctx context.Context, st *store.Store, id int64, action string, extra map[string]string) {
	ctxJSON, _ := json.Marshal(extra)
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, context, created_at)
		 VALUES (NULL, ?, 'user', ?, 'OK', ?, ?)`,
		action, fmt.Sprint(id), string(ctxJSON), time.Now().Unix()); err != nil {
		slog.Warn("audit write failed", "err", err, "user_id", id)
	}
}

// normaliseUsername lower-cases a username and falls back to the e-mail prefix.
func normaliseUsername(username, email string) string {
	u := strings.ToLower(strings.TrimSpace(username))
	if u == "" {
		u = strings.SplitN(email, "@", 2)[0]
	}
	var b strings.Builder
	for _, r := range u {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) < 3 {
		out = "user" + out
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
