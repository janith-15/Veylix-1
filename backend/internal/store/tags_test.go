package store

import (
	"context"
	"errors"
	"testing"
)

// D-025.2: the tag locks once the VPN is issued; before that it may change.
func TestTagLocksOnceIssued(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)
	const email = "tag@panel.test"

	if err := st.SetClientTag(ctx, serverID, email, TagFree, 0); err != nil {
		t.Fatalf("initial tag: %v", err)
	}
	// Re-tagging while unissued is allowed.
	if err := st.SetClientTag(ctx, serverID, email, TagPremium, 0); err != nil {
		t.Fatalf("re-tag while unissued must be allowed: %v", err)
	}

	if err := st.LockClientTag(ctx, serverID, email); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := st.SetClientTag(ctx, serverID, email, TagFree, 0); !errors.Is(err, ErrTagLocked) {
		t.Fatalf("re-tag after issue = %v, want ErrTagLocked", err)
	}

	got, err := st.ClientTag(ctx, serverID, email)
	if err != nil {
		t.Fatalf("ClientTag: %v", err)
	}
	if got.Tag != TagPremium {
		t.Errorf("tag = %s, want PREMIUM (the refused write must not have applied)", got.Tag)
	}
	if got.LockedAt == 0 {
		t.Error("LockedAt is zero for an issued VPN")
	}
}

// An untagged client is reported as ErrNoRows — it is a VPN (D-024.3) that has
// simply not been classified yet.
func TestUntaggedClientHasNoTag(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)

	if _, err := st.ClientTag(ctx, serverID, "nobody@panel.test"); !errors.Is(err, ErrNoRows) {
		t.Fatalf("err = %v, want ErrNoRows", err)
	}
	if err := st.SetClientTag(ctx, serverID, "yes@panel.test", TagFree, 0); err != nil {
		t.Fatalf("tag: %v", err)
	}
	untagged, err := st.UntaggedClients(ctx, serverID, []string{"yes@panel.test", "nobody@panel.test"})
	if err != nil {
		t.Fatalf("UntaggedClients: %v", err)
	}
	if len(untagged) != 1 || untagged[0] != "nobody@panel.test" {
		t.Errorf("untagged = %v, want [nobody@panel.test]", untagged)
	}
}

// An invalid tag must be refused before it reaches the CHECK constraint.
func TestSetClientTagRejectsUnknownValue(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)
	if err := st.SetClientTag(ctx, serverID, "x@panel.test", Tag("GOLD"), 0); err == nil {
		t.Fatal("an unknown tag must be refused")
	}
}

// Locking twice keeps the FIRST lock time: the moment of issue is the fact.
func TestLockIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)
	const email = "lock@panel.test"

	if err := st.SetClientTag(ctx, serverID, email, TagFree, 0); err != nil {
		t.Fatalf("tag: %v", err)
	}
	if err := st.LockClientTag(ctx, serverID, email); err != nil {
		t.Fatalf("lock 1: %v", err)
	}
	first, _ := st.ClientTag(ctx, serverID, email)
	if err := st.LockClientTag(ctx, serverID, email); err != nil {
		t.Fatalf("lock 2: %v", err)
	}
	second, _ := st.ClientTag(ctx, serverID, email)
	if first.LockedAt != second.LockedAt {
		t.Errorf("locked_at changed on the second lock: %d -> %d", first.LockedAt, second.LockedAt)
	}
}
