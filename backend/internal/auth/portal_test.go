package auth

import (
	"context"
	"errors"
	"testing"

	"veylix/backend/internal/audit"
)

// D-041: an unverified self-registered client cannot sign in, and the refusal is
// distinct from "wrong password" so the UI can offer a resend instead of blaming
// the user's password.
func TestUnverifiedAccountCannotSignIn(t *testing.T) {
	db := testDB(t)
	svc := NewService(db, audit.New(db)) // auto-verify OFF: production behaviour

	user, err := svc.Register(context.Background(), RegisterInput{
		Email: "pending@example.com", Password: "a very long password", DisplayName: "Pending",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.EmailVerified {
		t.Fatal("a self-registered account must start unverified")
	}
	if _, err := svc.Authenticate(context.Background(), "pending@example.com",
		"a very long password", "1.2.3.4", PortalClient); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("unverified login = %v, want ErrEmailNotVerified", err)
	}

	// After verification the same credentials work.
	if err := svc.SetEmailVerifiedForTest(context.Background(), user.ID); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), "pending@example.com",
		"a very long password", "1.2.3.4", PortalClient); err != nil {
		t.Fatalf("verified login = %v, want success", err)
	}
}

// D-041: the two doors. A client is refused at the staff door and staff at the
// client door, and the error is `ErrWrongPortal` (not "bad credentials"), so the
// UI can point the person at the right login page.
func TestPortalsAreSeparate(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	client, err := svc.Register(ctx, RegisterInput{
		Email: "door-client@example.com", Password: "a very long password", DisplayName: "Client",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "door-client@example.com",
		"a very long password", "1.1.1.1", PortalStaff); !errors.Is(err, ErrWrongPortal) {
		t.Fatalf("client at the staff door = %v, want ErrWrongPortal", err)
	}

	hash, err := HashPassword("a very long password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('door-admin@example.com', ?, 'Admin', 'admin', 'ACTIVE', 1, 1, 1)`, hash); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "door-admin@example.com",
		"a very long password", "1.1.1.2", PortalClient); !errors.Is(err, ErrWrongPortal) {
		t.Fatalf("admin at the client door = %v, want ErrWrongPortal", err)
	}
	if _, err := svc.Authenticate(ctx, "door-admin@example.com",
		"a very long password", "1.1.1.3", PortalStaff); err != nil {
		t.Fatalf("admin at the staff door = %v, want success", err)
	}

	_ = client
}

// D-041: signing in by USERNAME works, and last_login_at is recorded, so the
// management pages can answer "when did this person last sign in".
func TestUsernameLoginAndLastSeen(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	if _, err := svc.Register(ctx, RegisterInput{
		Email: "Nimal.Perera@example.com", Password: "a very long password", DisplayName: "Nimal",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	var username string
	if err := db.QueryRow(`SELECT username FROM users WHERE email = 'nimal.perera@example.com'`).
		Scan(&username); err != nil {
		t.Fatalf("username: %v", err)
	}
	if username != "nimal.perera" {
		t.Fatalf("derived username = %q, want nimal.perera", username)
	}

	if _, err := svc.Authenticate(ctx, username, "a very long password", "2.2.2.2", PortalClient); err != nil {
		t.Fatalf("login by username = %v, want success", err)
	}
	var (
		lastAt int64
		lastIP string
	)
	if err := db.QueryRow(`SELECT last_login_at, last_login_ip FROM users WHERE email = 'nimal.perera@example.com'`).
		Scan(&lastAt, &lastIP); err != nil {
		t.Fatalf("last seen: %v", err)
	}
	if lastAt == 0 || lastIP != "2.2.2.2" {
		t.Fatalf("last_seen = (%d, %q), want a timestamp and the caller IP", lastAt, lastIP)
	}
}
