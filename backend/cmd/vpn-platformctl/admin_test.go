package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// seedSuperAdmin opens a REAL store in an isolated temp database (never ./var/)
// and inserts one super_admin, so reset-admin is exercised against the actual
// schema rather than a mock.
func seedSuperAdmin(t *testing.T, p paths, email, password string) int64 {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, p.dbFile())
	if err != nil {
		t.Fatalf("open isolated db: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate isolated db: %v", err)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	now := time.Now().Unix()
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES (?, ?, 'Owner', 'super_admin', 'ACTIVE', 1, ?, ?)`,
		email, hash, now, now)
	if err != nil {
		t.Fatalf("insert super admin: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// storedHash reads the password hash back out of the isolated database.
func storedHash(t *testing.T, p paths, email string) string {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, p.dbFile())
	if err != nil {
		t.Fatalf("open isolated db: %v", err)
	}
	defer st.Close()

	var h string
	if err := st.DB().QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE email = ? COLLATE NOCASE`, email).Scan(&h); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	return h
}

// TestResetAdminChangesThePassword is the whole point of the command: the account
// that cannot otherwise be recovered gets a new password, and the new password
// VERIFIES against the stored hash using the same verifier the login route uses.
func TestResetAdminChangesThePassword(t *testing.T) {
	p := newTree(t)
	email := "owner@example.test"
	seedSuperAdmin(t, p, email, "original-password-123")
	write(t, p.envFile(), "VEYLIX_PUBLIC_BASE_URL=https://vpn.example.com")

	const newPassword = "a-much-better-password-456"
	if err := cmdResetAdmin(p, []string{"--password", newPassword}); err != nil {
		t.Fatalf("reset-admin: %v", err)
	}

	got := storedHash(t, p, email)
	if !auth.VerifyPassword(got, newPassword) {
		t.Error("the NEW password does not verify against the stored hash")
	}
	if auth.VerifyPassword(got, "original-password-123") {
		t.Error("the OLD password still verifies - the reset did not take effect")
	}
	if auth.NeedsRehash(got) {
		t.Error("the stored hash needs rehashing straight after a reset")
	}
}

// TestResetAdminDryRunChangesNothing: --dry-run is honoured by every mutation.
func TestResetAdminDryRunChangesNothing(t *testing.T) {
	p := newTree(t)
	email := "owner@example.test"
	seedSuperAdmin(t, p, email, "original-password-123")
	before := storedHash(t, p, email)

	if err := cmdResetAdmin(p, []string{"--dry-run", "--password", "a-much-better-password-456"}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if after := storedHash(t, p, email); after != before {
		t.Fatal("a dry run changed the password hash")
	}
}

// TestResetAdminRejectsAWeakPassword: the policy is the SAME one the UI applies,
// so a recovery must not become a way to install a weak password.
func TestResetAdminRejectsAWeakPassword(t *testing.T) {
	p := newTree(t)
	email := "owner@example.test"
	seedSuperAdmin(t, p, email, "original-password-123")
	before := storedHash(t, p, email)

	err := cmdResetAdmin(p, []string{"--password", "short"})
	if err == nil {
		t.Fatal("a weak password was accepted")
	}
	if !strings.Contains(err.Error(), "not acceptable") {
		t.Errorf("error = %v, want a policy refusal", err)
	}
	if after := storedHash(t, p, email); after != before {
		t.Error("a refused password still changed the hash")
	}
}

// TestResetAdminMatchesEmailCaseInsensitively: emails are stored COLLATE NOCASE,
// so a reset typed in different case must hit the same account rather than
// reporting "no Super Admin" for an account that plainly exists.
func TestResetAdminMatchesEmailCaseInsensitively(t *testing.T) {
	p := newTree(t)
	seedSuperAdmin(t, p, "owner@example.test", "original-password-123")

	if err := cmdResetAdmin(p, []string{"--email", "OWNER@Example.Test",
		"--password", "a-much-better-password-456"}); err != nil {
		t.Fatalf("reset-admin with different case: %v", err)
	}
	if !auth.VerifyPassword(storedHash(t, p, "owner@example.test"), "a-much-better-password-456") {
		t.Error("the account was not changed when the email case differed")
	}
}

// TestResetAdminRefusesAnEmailThatIsNotTheSuperAdmin: a typo must be a clear
// refusal, not a silent no-op and not a write to some other row.
func TestResetAdminRefusesAnEmailThatIsNotTheSuperAdmin(t *testing.T) {
	p := newTree(t)
	seedSuperAdmin(t, p, "owner@example.test", "original-password-123")
	before := storedHash(t, p, "owner@example.test")

	err := cmdResetAdmin(p, []string{"--email", "someone-else@example.test",
		"--password", "a-much-better-password-456"})
	if err == nil {
		t.Fatal("an unknown email was accepted")
	}
	if !strings.Contains(err.Error(), "no Super Admin") {
		t.Errorf("error = %v, want a no-Super-Admin message", err)
	}
	if after := storedHash(t, p, "owner@example.test"); after != before {
		t.Error("a refused reset still changed the real Super Admin")
	}
}

// TestResetAdminRefusesWithoutADatabase: on a host with no install this must say
// so, not create an empty database as a side effect.
func TestResetAdminRefusesWithoutADatabase(t *testing.T) {
	p := newTree(t)
	err := cmdResetAdmin(p, []string{"--password", "a-much-better-password-456"})
	if err == nil {
		t.Fatal("reset-admin on a host with no database must fail")
	}
	if !strings.Contains(err.Error(), "no database") {
		t.Errorf("error = %v, want a no-database message", err)
	}
	if fileExists(p.dbFile()) {
		t.Error("reset-admin CREATED a database while refusing; it must not")
	}
}
