package provisioning

import (
	"context"
	"time"

	"veylix/backend/internal/domain"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

// MaxOutboxAttempts bounds how many times one queued remote operation may be
// retried before it is declared FAILED.
//
// Without a cap the outbox is an infinite loop: backoff() saturates at 640s, so
// a permanently broken operation (a deleted inbound, a token the panel has
// revoked, a client the panel refuses by validation) would be retried roughly
// 135 times a day forever. That burns the 1 vCPU target and fills the log with
// a failure nobody can act on. At 8 attempts with exponential backoff the last
// retry lands about an hour after the first, which is long enough to ride out a
// panel restart and short enough to surface a real problem.
const MaxOutboxAttempts = 8

// ErrOutboxExhausted is recorded as the terminal reason when an operation hits
// MaxOutboxAttempts.
const ErrOutboxExhausted = "retry limit reached — operation abandoned after max attempts"

// Reconciliation outcomes for one outbox row.
type ReconcileResult string

const (
	// ReconcileApplied means the remote state matches the intent.
	ReconcileApplied ReconcileResult = "APPLIED"
	// ReconcileRetry means the panel could not be reached — try again later.
	ReconcileRetry ReconcileResult = "RETRY"
	// ReconcileResolved means the operation was abandoned as impossible.
	ReconcileResolved ReconcileResult = "RESOLVED"
	// ReconcileExhausted means the operation ran out of retry attempts.
	//
	// It is distinct from RESOLVED on purpose: RESOLVED means "we proved this can
	// never work" (malformed key, unsupported op), while EXHAUSTED means "we gave
	// up" — the panel might come back. An operator reading the trail needs to
	// tell those apart, because only one of them is worth re-queueing by hand.
	ReconcileExhausted ReconcileResult = "EXHAUSTED"
)

// Reconcile processes pending/retry outbox rows.
//
// The rule from new_update.md §5 — "when a request times out, determine whether
// the remote operation was applied before retrying" — is implemented here by
// ALWAYS reading the panel state before acting:
//
//	CREATE_CLIENT → if the client exists, mark APPLIED; only create when absent;
//	DELETE_CLIENT → if the client is gone, mark APPLIED; only delete when present;
//	DISABLE/ENABLE → read, then converge.
//
// Because the local identity is deterministic (PanelEmail), a duplicate is
// impossible even if two passes race: the panel rejects a second client with
// the same email/subId and our next read still reports "exists".
func (s *Service) Reconcile(ctx context.Context, limit int) (map[ReconcileResult]int, error) {
	rows, err := s.st.PendingOutbox(ctx, limit)
	if err != nil {
		return nil, err
	}

	out := map[ReconcileResult]int{}
	for _, row := range rows {
		res := s.reconcileOne(ctx, row)
		out[res]++
	}
	return out, nil
}

func (s *Service) reconcileOne(ctx context.Context, row store.OutboxRow) ReconcileResult {
	// The idempotency key carries the panel email: "<op>:<server>:<email>".
	email := panelEmailFromKey(row.IdempotencyKey)
	if email == "" {
		s.markOutboxFailed(ctx, row.ID, "malformed idempotency key", nil)
		return ReconcileResolved
	}

	client, err := s.factory(ctx, row.ServerID)
	if err != nil {
		// Panel unreachable: keep the row, back off, try later. This also marks
		// the affected VPN unavailable rather than revoked.
		res := s.retryWithBackoff(ctx, row, "panel unavailable", err)
		s.markVPNByEmail(ctx, row.ServerID, email, domain.VPNServerUnavailable)
		return res
	}

	exists, readErr := client.ClientExists(ctx, email)
	if readErr != nil {
		return s.retryWithBackoff(ctx, row, "cannot read panel state", readErr)
	}

	switch row.Op {
	case store.OpCreateClient:
		return s.reconcileCreate(ctx, row, client, email, exists)
	case store.OpDeleteClient:
		return s.reconcileDelete(ctx, row, client, email, exists)
	default:
		s.markOutboxFailed(ctx, row.ID, "unsupported op", nil)
		return ReconcileResolved
	}
}

func (s *Service) reconcileCreate(ctx context.Context, row store.OutboxRow,
	client PanelClient, email string, exists bool) ReconcileResult {

	if exists {
		// The earlier write landed — no duplicate is created.
		s.markOutboxApplied(ctx, row.ID)
		s.markVPNByEmail(ctx, row.ServerID, email, domain.VPNActive)
		return ReconcileApplied
	}

	// Absent: the write genuinely never landed, so creating now is safe.
	// The exact original intent is replayed from the outbox payload.
	payload, ok := decodePayload(row.PayloadJSON)
	if !ok {
		s.markOutboxFailed(ctx, row.ID, "missing payload for create", nil)
		return ReconcileResolved
	}
	if err := client.AddClient(ctx, panel.AddClientRequest{
		Client:     payload.Client,
		InboundIDs: payload.InboundIDs,
	}); err != nil {
		return s.retryWithBackoff(ctx, row, "retry create failed", err)
	}
	confirmed, err := client.ClientExists(ctx, email)
	if err != nil || !confirmed {
		return s.retryWithBackoff(ctx, row, "create not confirmed", err)
	}
	s.markOutboxApplied(ctx, row.ID)
	s.markVPNByEmail(ctx, row.ServerID, email, domain.VPNActive)
	return ReconcileApplied
}

func (s *Service) reconcileDelete(ctx context.Context, row store.OutboxRow,
	client PanelClient, email string, exists bool) ReconcileResult {

	if !exists {
		s.markOutboxApplied(ctx, row.ID)
		s.markVPNByEmail(ctx, row.ServerID, email, domain.VPNRevoked)
		return ReconcileApplied
	}

	if err := client.DeleteClient(ctx, email); err != nil {
		return s.retryWithBackoff(ctx, row, "retry delete failed", err)
	}
	gone, err := client.ClientExists(ctx, email)
	if err != nil || gone {
		return s.retryWithBackoff(ctx, row, "delete not confirmed", err)
	}
	s.markOutboxApplied(ctx, row.ID)
	s.markVPNByEmail(ctx, row.ServerID, email, domain.VPNRevoked)
	return ReconcileApplied
}

// retryWithBackoff schedules the next attempt, or gives up once the operation
// has burned through MaxOutboxAttempts.
//
// It returns the outcome so the caller reports EXHAUSTED rather than RETRY on
// the final attempt — the reconciler's per-result tallies are what an operator
// reads to see whether a pass made progress.
func (s *Service) retryWithBackoff(ctx context.Context, row store.OutboxRow, msg string, cause error) ReconcileResult {
	if cause != nil {
		msg = msg + ": " + sanitiseErr(cause)
	}
	// row.Attempts is the count BEFORE this attempt. MarkOutbox increments it,
	// so the attempt just made is row.Attempts+1.
	if row.Attempts+1 >= MaxOutboxAttempts {
		s.markOutboxFailed(ctx, row.ID, ErrOutboxExhausted+": "+msg, nil)
		// The VPN stays in whatever honest state the outage left it in. We do
		// NOT write REVOKED: we still cannot prove the panel has no client, and
		// a fabricated revocation is the one thing D-024 forbids outright.
		return ReconcileExhausted
	}
	next := time.Now().Add(backoff(row.Attempts + 1)).Unix()
	_ = s.st.MarkOutbox(ctx, nil, row.ID, store.OutboxRetry, msg, &next)
	return ReconcileRetry
}

// markVPNByEmail updates the VPN row matching a panel email, applying the state
// machine so an outage can never be recorded as REVOKED.
func (s *Service) markVPNByEmail(ctx context.Context, serverID int64, email string, to domain.VPNStatus) {
	var id int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM vpn_instances WHERE server_id = ? AND panel_email = ?`, serverID, email,
	).Scan(&id); err != nil {
		return
	}
	s.setStatus(ctx, id, to)
	if to == domain.VPNRevoked {
		s.closeAssignment(ctx, id)
	}
}

// panelEmailFromKey extracts the panel email from "<op>:<server>:<email>".
func panelEmailFromKey(key string) string {
	// op may not contain ':' and server ids are numeric, so split from the left.
	idx := 0
	parts := 0
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			parts++
			if parts == 2 {
				idx = i + 1
				break
			}
		}
	}
	if parts < 2 || idx == 0 || idx >= len(key) {
		return ""
	}
	return key[idx:]
}
