package integrations

import (
	"context"
	"errors"
	"fmt"

	"veylix/backend/internal/panel"
)

// ErrClientUnavailable means the integration cannot currently serve panel calls
// (not ACTIVE, removed, or the stored token cannot be decrypted).
var ErrClientUnavailable = errors.New("integrations: panel client unavailable")

// PanelClient builds a real 3X-UI client for an ACTIVE integration.
//
// This is the single entry point other packages use to talk to a panel, so the
// "only ACTIVE integrations may be used" rule lives in one place and the sealed
// token is decrypted per call instead of being cached anywhere.
func (s *Service) PanelClient(ctx context.Context, serverID int64) (*panel.Client, error) {
	row, status, err := s.loadWithStatus(ctx, serverID)
	if err != nil {
		return nil, err
	}
	// A disabled or stale integration must never be used for provisioning:
	// that would create clients on a panel the operator already took offline.
	if status != string(panel.StatusActive) {
		return nil, fmt.Errorf("%w: integration status is %s", ErrClientUnavailable, status)
	}

	plain, err := s.sealer.Open(row.tokenCiphertext, addAAD(row.panelURL))
	if err != nil {
		return nil, fmt.Errorf("%w: stored token cannot be decrypted", ErrClientUnavailable)
	}
	target, err := panel.ParseTarget(row.panelURL, row.allowPrivate, s.resolve)
	if err != nil {
		return nil, fmt.Errorf("%w: stored url is no longer acceptable", ErrClientUnavailable)
	}
	return panel.New(panel.Options{Target: target, Token: string(plain)})
}

// loadWithStatus reads an integration row plus its status.
func (s *Service) loadWithStatus(ctx context.Context, id int64) (*storedRow, string, error) {
	var r storedRow
	var allow int
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT name, panel_url, token_ciphertext, allow_private_network, status
		   FROM server_integrations WHERE id = ?`, id,
	).Scan(&r.name, &r.panelURL, &r.tokenCiphertext, &allow, &status)
	if err != nil {
		return nil, "", ErrNotFound
	}
	r.allowPrivate = allow == 1
	return &r, status, nil
}
