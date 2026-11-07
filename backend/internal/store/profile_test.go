package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// newProfileStore returns a migrated store with one client account.
func newProfileStore(t *testing.T) (*Store, int64) {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir()+"/profile.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	res, err := st.DB().Exec(
		`INSERT INTO users (email, username, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('a@example.com', 'alice', 'x', 'Alice', 'client', 'ACTIVE', 1, 1, 1)`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id, _ := res.LastInsertId()
	return st, id
}

// A patch must only touch the fields it names: saving the contact tab must not
// blank the avatar or the username.
func TestProfilePatchOnlyTouchesNamedFields(t *testing.T) {
	ctx := context.Background()
	st, id := newProfileStore(t)

	avatar := "/uploads/alice.png"
	name := "Alice Perera"
	if _, err := st.UpdateProfile(ctx, id, ProfileUpdate{AvatarURL: &avatar, DisplayName: &name}); err != nil {
		t.Fatalf("first patch: %v", err)
	}
	wa := "+94770000000"
	prof, err := st.UpdateProfile(ctx, id, ProfileUpdate{WhatsApp: &wa})
	if err != nil {
		t.Fatalf("second patch: %v", err)
	}
	if prof.AvatarURL != avatar || prof.DisplayName != name {
		t.Fatalf("a patch of one field cleared another: %+v", prof)
	}
	if prof.WhatsApp != wa {
		t.Fatalf("whatsapp = %q, want %q", prof.WhatsApp, wa)
	}
	if prof.Username != "alice" {
		t.Fatalf("username changed unexpectedly: %q", prof.Username)
	}
}

// A username is a login identifier: it must be unique, and case-insensitive.
func TestUsernameIsUnique(t *testing.T) {
	ctx := context.Background()
	st, id := newProfileStore(t)

	if _, err := st.DB().Exec(
		`INSERT INTO users (email, username, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('b@example.com', 'bob', 'x', 'Bob', 'client', 'ACTIVE', 1, 1, 1)`); err != nil {
		t.Fatalf("seed bob: %v", err)
	}

	taken := "BOB"
	if _, err := st.UpdateProfile(ctx, id, ProfileUpdate{Username: &taken}); err == nil {
		t.Fatal("a taken username was accepted")
	} else if !strings.Contains(err.Error(), "username already taken") {
		t.Fatalf("unexpected error: %v", err)
	}

	free, err := st.UsernameAvailable(ctx, "alice", id)
	if err != nil || !free {
		t.Fatalf("keeping your own username must be available: free=%v err=%v", free, err)
	}
}

// The e-mail change is single-use, expires, and burns after too many attempts.
func TestEmailChangeLifecycle(t *testing.T) {
	ctx := context.Background()
	st, id := newProfileStore(t)

	if err := st.CreateEmailChangeRequest(ctx, id, "new@example.com", "hash", 15*time.Minute); err != nil {
		t.Fatalf("create: %v", err)
	}
	req, err := st.LiveEmailChangeRequest(ctx, id)
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	if req.NewEmail != "new@example.com" {
		t.Fatalf("new email = %q", req.NewEmail)
	}

	if err := st.ConfirmEmailChange(ctx, req.ID, id, req.NewEmail); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	prof, err := st.ProfileByID(ctx, id)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if prof.Email != "new@example.com" || !prof.EmailVerified {
		t.Fatalf("email = %q verified=%v, want new@example.com verified", prof.Email, prof.EmailVerified)
	}

	// The request is used: a second confirm finds nothing to do.
	if _, err := st.LiveEmailChangeRequest(ctx, id); err == nil {
		t.Fatal("a used request is still live")
	}
}

// Asking again replaces the code, and too many wrong attempts burn the request.
func TestEmailChangeSupersedeAndBurn(t *testing.T) {
	ctx := context.Background()
	st, id := newProfileStore(t)

	if err := st.CreateEmailChangeRequest(ctx, id, "one@example.com", "h1", time.Minute); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := st.CreateEmailChangeRequest(ctx, id, "two@example.com", "h2", time.Minute); err != nil {
		t.Fatalf("second: %v", err)
	}
	req, err := st.LiveEmailChangeRequest(ctx, id)
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	if req.NewEmail != "two@example.com" {
		t.Fatalf("newest request must win, got %q", req.NewEmail)
	}

	for i := 0; i < 5; i++ {
		if err := st.BumpEmailChangeAttempts(ctx, req.ID, 5); err != nil {
			t.Fatalf("bump: %v", err)
		}
	}
	if _, err := st.LiveEmailChangeRequest(ctx, id); err == nil {
		t.Fatal("the request survived the attempt limit")
	}
}
