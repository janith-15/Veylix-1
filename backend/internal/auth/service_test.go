package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"veylix/backend/internal/audit"
)

// clientPortal is the customer door, which is what the pre-D-041 tests exercise.
const clientPortal = PortalClient

// staffPortal is the operator door (admin + super admin).
const staffPortal = PortalStaff

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT,
			google_sub TEXT,
			display_name TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			email_verified INTEGER NOT NULL DEFAULT 0,
			username TEXT,
			avatar_url TEXT NOT NULL DEFAULT '',
			phone TEXT NOT NULL DEFAULT '',
			whatsapp TEXT NOT NULL DEFAULT '',
			telegram TEXT NOT NULL DEFAULT '',
			discord TEXT NOT NULL DEFAULT '',
			age INTEGER NOT NULL DEFAULT 0,
			bio TEXT NOT NULL DEFAULT '',
			timezone TEXT NOT NULL DEFAULT '',
			free_vpn_limit INTEGER NOT NULL DEFAULT 0,
			last_login_at INTEGER NOT NULL DEFAULT 0,
			last_login_ip TEXT NOT NULL DEFAULT '',
			quota_limit INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX idx_users_username ON users (username COLLATE NOCASE) WHERE username IS NOT NULL AND username <> '';
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_id INTEGER,
			action TEXT NOT NULL,
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL DEFAULT 'OK',
			context TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// newTestService returns a service whose self-registered accounts are VERIFIED.
//
// These tests are about authentication and password handling, not about the
// e-mail-confirmation step; the confirmation REFUSAL has its own test below
// (TestUnverifiedAccountCannotSignIn) which builds a service with it left off.
func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db := testDB(t)
	svc := NewService(db, audit.New(db))
	svc.SetAutoVerify(true)
	return svc, db
}

func TestRegisterCreatesClientOnly(t *testing.T) {
	svc, _ := newTestService(t)
	u, err := svc.Register(context.Background(), RegisterInput{
		Email: "User@Example.COM", Password: "a very long password", DisplayName: "User",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if u.Role != string(RoleClient) {
		t.Fatalf("self-registration role = %q, want client", u.Role)
	}
	if u.Email != "user@example.com" {
		t.Errorf("email not normalised: %q", u.Email)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	svc, _ := newTestService(t)
	in := RegisterInput{Email: "dup@example.com", Password: "a very long password"}
	if _, err := svc.Register(context.Background(), in); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.Register(context.Background(), in); err != ErrEmailTaken {
		t.Fatalf("second = %v, want ErrEmailTaken", err)
	}
}

func TestRegisterRejectsBadEmailAndWeakPassword(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.Register(context.Background(), RegisterInput{Email: "nope", Password: "a very long password"}); err == nil {
		t.Error("invalid email must be rejected")
	}
	if _, err := svc.Register(context.Background(), RegisterInput{Email: "ok@example.com", Password: "short"}); err != ErrWeakPassword {
		t.Errorf("weak password = %v, want ErrWeakPassword", err)
	}
}

func TestAuthenticateSucceedsAndFails(t *testing.T) {
	svc, _ := newTestService(t)
	const pw = "a very long password"
	if _, err := svc.Register(context.Background(), RegisterInput{Email: "l@example.com", Password: pw}); err != nil {
		t.Fatalf("register: %v", err)
	}

	u, err := svc.Authenticate(context.Background(), "l@example.com", pw, "1.2.3.4", clientPortal)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u.ID == 0 || u.Role != string(RoleClient) {
		t.Errorf("unexpected user: %+v", u)
	}

	if _, err := svc.Authenticate(context.Background(), "l@example.com", "wrong password", "1.2.3.4", clientPortal); err != ErrInvalidCredentials {
		t.Errorf("wrong password = %v, want ErrInvalidCredentials", err)
	}
	// An unknown email must produce the SAME error (no account enumeration).
	if _, err := svc.Authenticate(context.Background(), "nobody@example.com", pw, "1.2.3.5", clientPortal); err != ErrInvalidCredentials {
		t.Errorf("unknown email = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthenticateSuspendedUserIsForbidden(t *testing.T) {
	svc, db := newTestService(t)
	const pw = "a very long password"
	u, _ := svc.Register(context.Background(), RegisterInput{Email: "s@example.com", Password: pw})
	if _, err := db.Exec(`UPDATE users SET status = 'SUSPENDED' WHERE id = ?`, u.ID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), "s@example.com", pw, "9.9.9.9", clientPortal); err != ErrForbidden {
		t.Errorf("suspended login = %v, want ErrForbidden", err)
	}
}

func TestLoginRateLimitBlocksAfterBurst(t *testing.T) {
	svc, _ := newTestService(t)
	const pw = "a very long password"
	if _, err := svc.Register(context.Background(), RegisterInput{Email: "r@example.com", Password: pw}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// The limiter allows 10 attempts (raised from 5 so a person who mistypes a
	// few times is not locked out); the 11th must be throttled.
	var lastErr error
	for i := 0; i < 11; i++ {
		_, lastErr = svc.Authenticate(context.Background(), "r@example.com", "wrong", "5.5.5.5", clientPortal)
	}
	if !errors.Is(lastErr, ErrRateLimited) {
		t.Fatalf("after the burst the error = %v, want ErrRateLimited", lastErr)
	}

	// The lockout must say how long it lasts, or the client cannot tell the user
	// anything actionable.
	var rl *RateLimitedError
	if !errors.As(lastErr, &rl) {
		t.Fatalf("error %T must carry the retry duration", lastErr)
	}
	if rl.RetryAfter <= 0 {
		t.Fatalf("RetryAfter = %v, want a positive wait", rl.RetryAfter)
	}
}

func TestBootstrapOnlyRunsOnce(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	const pw = "a very long password"

	u, err := svc.BootstrapSuperAdmin(ctx, "owner@example.com", pw, "Owner")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if u.Role != string(RoleSuperAdmin) {
		t.Fatalf("bootstrap role = %q", u.Role)
	}
	if _, err := svc.BootstrapSuperAdmin(ctx, "second@example.com", pw, "Second"); err != ErrBootstrapNotAllowed {
		t.Fatalf("second bootstrap = %v, want ErrBootstrapNotAllowed", err)
	}
}

func TestBootstrapForbiddenAfterAnyUserExists(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, RegisterInput{Email: "c@example.com", Password: "a very long password"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.BootstrapSuperAdmin(ctx, "owner@example.com", "a very long password", "X"); err != ErrBootstrapNotAllowed {
		t.Fatal("bootstrap must be refused once a user exists (no privilege escalation)")
	}
}

// F-016: the credential can be rotated, and the old one stops working.
func TestChangePasswordRotatesCredential(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	const oldPW = "a very long password"
	const newPW = "an entirely different passphrase"

	u, err := svc.Register(ctx, RegisterInput{Email: "cc@example.com", Password: oldPW})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := svc.ChangePassword(ctx, u.ID, oldPW, newPW); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "cc@example.com", oldPW, "7.7.7.1", clientPortal); err != ErrInvalidCredentials {
		t.Errorf("the old password must stop working, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, "cc@example.com", newPW, "7.7.7.2", clientPortal); err != nil {
		t.Errorf("the new password must work, got %v", err)
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	const pw = "a very long password"
	u, _ := svc.Register(ctx, RegisterInput{Email: "wc@example.com", Password: pw})

	if err := svc.ChangePassword(ctx, u.ID, "not the right one", "another long passphrase"); err != ErrPasswordMismatch {
		t.Fatalf("wrong current = %v, want ErrPasswordMismatch", err)
	}
	// The original credential must still work: a failed change changes nothing.
	if _, err := svc.Authenticate(ctx, "wc@example.com", pw, "8.8.8.1", clientPortal); err != nil {
		t.Errorf("a failed change must not alter the password, got %v", err)
	}
}

func TestChangePasswordEnforcesPolicyOnTheNewValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	const pw = "a very long password"
	u, _ := svc.Register(ctx, RegisterInput{Email: "pc@example.com", Password: pw})

	if err := svc.ChangePassword(ctx, u.ID, pw, "aaaaaaaaaaaa"); err != ErrPasswordRepeated {
		t.Errorf("repeated rune = %v, want ErrPasswordRepeated", err)
	}
	if err := svc.ChangePassword(ctx, u.ID, pw, "short"); err != ErrWeakPassword {
		t.Errorf("short = %v, want ErrWeakPassword", err)
	}
	// Unchanged after both rejections.
	if _, err := svc.Authenticate(ctx, "pc@example.com", pw, "8.8.8.2", clientPortal); err != nil {
		t.Errorf("rejected changes must not alter the password, got %v", err)
	}
}

func TestChangePasswordForUnknownUserIsUnauthenticated(t *testing.T) {
	svc, _ := newTestService(t)
	if err := svc.ChangePassword(context.Background(), 4242, "x", "a very long password"); err != ErrUnauthenticated {
		t.Fatalf("unknown user = %v, want ErrUnauthenticated", err)
	}
}

// D-080: a Google-only account has NO password to confirm with, so the FIRST
// password must be settable without a current one — that is the self-service
// way out of the "cannot log in with e-mail and password" loop. The mirror
// case matters just as much: once a password EXISTS, an empty current value
// must be refused, or a stolen session could silently replace the credential.
func TestChangePasswordFirstSetNeedsNoCurrent(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()
	const pw = "a very long password"

	// A Google-created client: CreateOAuthUser stores password_hash = NULL.
	res, err := db.Exec(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('goog@example.com', NULL, 'Goog', 'client', 'ACTIVE', 1, 100, 100)`)
	if err != nil {
		t.Fatalf("seed google-only user: %v", err)
	}
	id, _ := res.LastInsertId()

	if err := svc.ChangePassword(ctx, id, "", pw); err != nil {
		t.Fatalf("first set with no current = %v, want success", err)
	}
	if _, err := svc.Authenticate(ctx, "goog@example.com", pw, "9.9.9.1", clientPortal); err != nil {
		t.Errorf("after the first set, e-mail+password must work, got %v", err)
	}

	// The mirror case: the credential now exists, so an empty current value is
	// just a missing confirmation and must not rotate it.
	if err := svc.ChangePassword(ctx, id, "", "an entirely different passphrase"); err != ErrPasswordMismatch {
		t.Errorf("empty current with a password set = %v, want ErrPasswordMismatch", err)
	}
	if _, err := svc.Authenticate(ctx, "goog@example.com", pw, "9.9.9.2", clientPortal); err != nil {
		t.Errorf("a refused change must not alter the password, got %v", err)
	}
}
