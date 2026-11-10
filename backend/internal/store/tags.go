package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrTagLocked is returned when re-tagging a VPN that has already been issued.
//
// D-025.2: flipping FREE↔PREMIUM after a sale would retroactively change what
// the customer paid for, so the tag locks the moment the VPN is issued.
var ErrTagLocked = errors.New("store: this VPN is already issued, so its tag is locked")

// Tag is the FREE/PREMIUM classification of a panel client.
type Tag string

const (
	TagFree    Tag = "FREE"
	TagPremium Tag = "PREMIUM"
)

// ClientTag is one panel client's classification.
type ClientTag struct {
	ServerID   int64
	PanelEmail string
	Tag        Tag
	TaggedBy   int64
	TaggedAt   int64
	// LockedAt is non-zero once the VPN has been issued; re-tagging is refused.
	LockedAt int64
}

// SetClientTag tags a panel client FREE or PREMIUM (D-024.3).
//
// Re-tagging is allowed only while the client is unissued; the update is
// conditional on locked_at IS NULL so a concurrent assignment cannot be
// overwritten by a stale tag write.
func (s *Store) SetClientTag(ctx context.Context, serverID int64, panelEmail string, tag Tag, actorID int64) error {
	if tag != TagFree && tag != TagPremium {
		return fmt.Errorf("store: invalid tag %q", tag)
	}
	now := time.Now().Unix()

	// D-025.2 — "the tag locks once issued". A VPN that already has a Veylix
	// row is issued, so the tag written now is locked immediately; re-tagging it
	// would retroactively change what the customer paid for.
	issued := s.issued(ctx, serverID, panelEmail)

	// Upsert, but never overwrite a locked row: the WHERE on DO UPDATE makes the
	// conflict path a no-op for locked rows, and RowsAffected tells us whether
	// the write was refused.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_by, tagged_at, locked_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (server_id, panel_email) DO UPDATE SET
		   tag = excluded.tag, tagged_by = excluded.tagged_by, tagged_at = excluded.tagged_at
		 WHERE panel_client_tags.locked_at IS NULL`,
		serverID, panelEmail, string(tag), nullableID(actorID), now, lockStamp(issued, now))
	if err != nil {
		return fmt.Errorf("store: set client tag: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Either the row exists and is locked — the case we must report.
		var locked sql.NullInt64
		if qerr := s.db.QueryRowContext(ctx,
			`SELECT locked_at FROM panel_client_tags WHERE server_id = ? AND panel_email = ?`,
			serverID, panelEmail).Scan(&locked); qerr == nil && locked.Valid {
			return ErrTagLocked
		}
		return fmt.Errorf("store: set client tag: no row written")
	}
	return nil
}

// issued reports whether Veylix already has a live VPN row for this panel
// client — i.e. it has been given to somebody.
//
// A REVOKED or REMOVED row no longer represents a held VPN, so it does not lock
// the identity: the panel name is free again and may be re-classified.
func (s *Store) issued(ctx context.Context, serverID int64, panelEmail string) bool {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM vpn_instances WHERE server_id = ? AND panel_email = ? ORDER BY id DESC LIMIT 1`,
		serverID, panelEmail).Scan(&status)
	if err != nil {
		return false
	}
	return status != "REVOKED" && status != "REMOVED_FROM_CATALOG"
}

// lockStamp is the locked_at value for a tag write: NOW when the VPN is already
// issued, NULL while it is still unissued (so the Super Admin may keep
// re-classifying the platform's own inventory).
func lockStamp(issued bool, now int64) any {
	if issued {
		return now
	}
	return nil
}

// ClientTag reads one panel client's tag. Returns ErrNoRows when untagged —
// an untagged client is a VPN (D-024.3) but visible only to the Super Admin.
func (s *Store) ClientTag(ctx context.Context, serverID int64, panelEmail string) (*ClientTag, error) {
	var t ClientTag
	var tag string
	var taggedBy, locked sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT server_id, panel_email, tag, tagged_by, tagged_at, locked_at
		   FROM panel_client_tags WHERE server_id = ? AND panel_email = ?`,
		serverID, panelEmail,
	).Scan(&t.ServerID, &t.PanelEmail, &tag, &taggedBy, &t.TaggedAt, &locked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("store: get client tag: %w", err)
	}
	t.Tag = Tag(tag)
	if taggedBy.Valid {
		t.TaggedBy = taggedBy.Int64
	}
	if locked.Valid {
		t.LockedAt = locked.Int64
	}
	return &t, nil
}

// LockClientTag marks a client's tag as final because it has been issued
// (D-025.2). Idempotent: locking an already-locked row keeps the first time.
func (s *Store) LockClientTag(ctx context.Context, serverID int64, panelEmail string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE panel_client_tags SET locked_at = ?
		  WHERE server_id = ? AND panel_email = ? AND locked_at IS NULL`,
		time.Now().Unix(), serverID, panelEmail)
	if err != nil {
		return fmt.Errorf("store: lock client tag: %w", err)
	}
	return nil
}

// UntaggedClients lists panel identities that exist but carry no tag.
//
// Input is the panel's own client list, so Veylix can show the Super Admin what
// is waiting to be classified without querying the panel again.
func (s *Store) UntaggedClients(ctx context.Context, serverID int64, panelEmails []string) ([]string, error) {
	if len(panelEmails) == 0 {
		return nil, nil
	}
	tagged := make(map[string]bool, len(panelEmails))
	rows, err := s.db.QueryContext(ctx,
		`SELECT panel_email FROM panel_client_tags WHERE server_id = ?`, serverID)
	if err != nil {
		return nil, fmt.Errorf("store: list tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, fmt.Errorf("store: scan tag: %w", err)
		}
		tagged[e] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate tags: %w", err)
	}

	out := make([]string, 0, len(panelEmails))
	for _, e := range panelEmails {
		if !tagged[e] {
			out = append(out, e)
		}
	}
	return out, nil
}
