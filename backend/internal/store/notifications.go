package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Notification kinds the UI knows how to render. The table stores the string,
// so an unknown value from a future producer still renders as `info` rather
// than breaking the feed.
const (
	NotifInfo    = "info"
	NotifSuccess = "success"
	NotifWarning = "warning"
	NotifError   = "error"
)

// ErrNotificationNotFound is returned when an id does not belong to the caller.
var ErrNotificationNotFound = errors.New("store: notification not found")

// NotificationRow is one inbox entry as returned to the API.
//
// `ReadAt` is a pointer because "never read" and "read at epoch 0" must not
// collapse into the same value.
type NotificationRow struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	ReadAt    *int64 `json:"read_at"`
	CreatedAt int64  `json:"created_at"`
}

// NotificationsFor returns a user's newest notifications, newest first.
//
// The scope is always the caller's own `user_id`: a notification is private by
// construction, so there is no role that may read someone else's inbox
// (REQUIREMENTS §2.8).
func (s *Store) NotificationsFor(ctx context.Context, userID int64, limit int) ([]NotificationRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, title, body, read_at, created_at
		   FROM notifications
		  WHERE user_id = ?
		  ORDER BY id DESC
		  LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]NotificationRow, 0, limit)
	for rows.Next() {
		var r NotificationRow
		var readAt sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Kind, &r.Title, &r.Body, &readAt, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan notification: %w", err)
		}
		if readAt.Valid {
			v := readAt.Int64
			r.ReadAt = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UnreadNotificationCount reports the badge number for one user.
func (s *Store) UnreadNotificationCount(ctx context.Context, userID int64) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`,
		userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count unread notifications: %w", err)
	}
	return n, nil
}

// MarkNotificationRead stamps one notification as read.
//
// The UPDATE is scoped by `user_id` **and** `read_at IS NULL`, so:
//   - marking somebody else's notification is impossible (it affects 0 rows and
//     the caller learns nothing about whether the id exists);
//   - re-marking an already read entry is idempotent and keeps the original
//     timestamp instead of moving it forward.
func (s *Store) MarkNotificationRead(ctx context.Context, userID, id int64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var owner int64
		err := tx.QueryRowContext(ctx, `SELECT user_id FROM notifications WHERE id = ?`, id).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotificationNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup notification: %w", err)
		}
		if owner != userID {
			// Same error as "missing": a caller must not be able to probe for
			// other users' notification ids.
			return ErrNotificationNotFound
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE notifications SET read_at = ? WHERE id = ? AND read_at IS NULL`,
			nowUnix(), id); err != nil {
			return fmt.Errorf("store: mark notification read: %w", err)
		}
		return nil
	})
}

// MarkAllNotificationsRead clears the whole badge for one user.
func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`,
		nowUnix(), userID)
	if err != nil {
		return 0, fmt.Errorf("store: mark all notifications read: %w", err)
	}
	return res.RowsAffected()
}

// CreateNotificationInput is a producer's payload.
type CreateNotificationInput struct {
	UserID int64
	Kind   string
	Title  string
	Body   string
}

// CreateNotification appends one inbox entry.
//
// Producers inside services call this **after** the operation it describes has
// been committed, and a failure is logged rather than rolled back: an inbox
// entry must never undo a VPN revocation (REQUIREMENTS §2.8).
func (s *Store) CreateNotification(ctx context.Context, in CreateNotificationInput) (*NotificationRow, error) {
	kind := in.Kind
	switch kind {
	case NotifInfo, NotifSuccess, NotifWarning, NotifError:
	default:
		kind = NotifInfo
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, kind, title, body, read_at, created_at)
		 VALUES (?, ?, ?, ?, NULL, ?)`,
		in.UserID, kind, in.Title, in.Body, nowUnix())
	if err != nil {
		return nil, fmt.Errorf("store: create notification: %w", err)
	}
	id, _ := res.LastInsertId()
	var r NotificationRow
	if err := s.db.QueryRowContext(ctx,
		`SELECT id, kind, title, body, read_at, created_at FROM notifications WHERE id = ?`,
		id).Scan(&r.ID, &r.Kind, &r.Title, &r.Body, new(sql.NullInt64), &r.CreatedAt); err != nil {
		return nil, fmt.Errorf("store: read back notification: %w", err)
	}
	return &r, nil
}

// Notify appends one inbox entry for a user.
//
// It is the audit logger's delivery seam, so it normalises the kind rather than
// trusting the caller: an unknown kind must never break the feed.
func (s *Store) Notify(ctx context.Context, userID int64, kind, title, body string) error {
	if userID <= 0 || title == "" {
		return nil
	}
	_, err := s.CreateNotification(ctx, CreateNotificationInput{
		UserID: userID, Kind: kind, Title: title, Body: body,
	})
	return err
}

// Admins returns the ids of active reseller accounts.
//
// Used to fan an operator-level event (a server removed, credentials rotated)
// out to the people whose clients will notice first. Suspended accounts are
// excluded: they cannot sign in, so an entry would just pile up unseen.
func (s *Store) Admins(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM users WHERE role = 'admin' AND status = 'ACTIVE' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list admin ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan admin id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
