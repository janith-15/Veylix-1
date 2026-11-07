package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func seedUser(t *testing.T, db *sql.DB, id int64, email string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO users (id, email, role, status, created_at, updated_at)
		 VALUES (?, ?, 'admin', 'ACTIVE', 1, 1)`, id, email); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	db := testDB(t)
	seedUser(t, db, 7, "sess@example.com")
	store := NewSessionStore(db, false)
	ctx := context.Background()

	token, err := store.Create(ctx, 7, "1.1.1.1", "test-agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	p, err := store.Lookup(ctx, token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if p.UserID != 7 || p.Role != RoleAdmin || p.Email != "sess@example.com" {
		t.Fatalf("unexpected principal: %+v", p)
	}

	if err := store.Revoke(ctx, token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := store.Lookup(ctx, token); err != ErrUnauthenticated {
		t.Fatalf("after revoke = %v, want ErrUnauthenticated", err)
	}
}

// The raw token must never be stored: a DB dump must not yield usable cookies.
func TestSessionStoresOnlyFingerprint(t *testing.T) {
	db := testDB(t)
	seedUser(t, db, 8, "fp@example.com")
	store := NewSessionStore(db, false)

	token, err := store.Create(context.Background(), 8, "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var stored string
	if err := db.QueryRow(`SELECT id FROM sessions`).Scan(&stored); err != nil {
		t.Fatalf("select: %v", err)
	}
	if stored == token {
		t.Fatal("raw session token stored in the database")
	}
	if stored != string(HashToken(token)) {
		t.Fatal("stored value is not the token fingerprint")
	}
}

func TestExpiredSessionIsRejectedAndPurged(t *testing.T) {
	db := testDB(t)
	seedUser(t, db, 9, "exp@example.com")
	store := NewSessionStore(db, false)
	ctx := context.Background()

	token, _ := store.Create(ctx, 9, "", "")
	if _, err := db.Exec(`UPDATE sessions SET expires_at = ?`, time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatalf("expire: %v", err)
	}

	if _, err := store.Lookup(ctx, token); err != ErrUnauthenticated {
		t.Fatalf("expired lookup = %v, want ErrUnauthenticated", err)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&count)
	if count != 0 {
		t.Error("an expired session should be purged on lookup")
	}
}

func TestSuspendedUserSessionIsRejected(t *testing.T) {
	db := testDB(t)
	seedUser(t, db, 10, "sus@example.com")
	store := NewSessionStore(db, false)
	ctx := context.Background()

	token, _ := store.Create(ctx, 10, "", "")
	if _, err := db.Exec(`UPDATE users SET status = 'SUSPENDED' WHERE id = 10`); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if _, err := store.Lookup(ctx, token); err != ErrUnauthenticated {
		t.Fatalf("suspended user session = %v, want ErrUnauthenticated", err)
	}
}

func TestEmptyTokenIsUnauthenticated(t *testing.T) {
	store := NewSessionStore(testDB(t), false)
	if _, err := store.Lookup(context.Background(), ""); err != ErrUnauthenticated {
		t.Fatalf("empty token = %v, want ErrUnauthenticated", err)
	}
}

func TestRevokeAllForUser(t *testing.T) {
	db := testDB(t)
	seedUser(t, db, 11, "multi@example.com")
	store := NewSessionStore(db, false)
	ctx := context.Background()

	a, _ := store.Create(ctx, 11, "", "")
	b, _ := store.Create(ctx, 11, "", "")
	if err := store.RevokeAllForUser(ctx, 11); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	for _, tok := range []string{a, b} {
		if _, err := store.Lookup(ctx, tok); err != ErrUnauthenticated {
			t.Errorf("session should be gone, got %v", err)
		}
	}
}
