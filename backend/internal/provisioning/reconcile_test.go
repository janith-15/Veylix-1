package provisioning

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"veylix/backend/internal/domain"
)

// The panel request derived from an offer must carry the device limit in BOTH
// fields the panel enforces. Omitting limitHwid was defect F-024 / D-028.5:
// capacity, duration and IP limit reached the panel, the hardware limit did
// not, so a device cap Veylix believed it had set was never enforced.
func TestBuildClientRequestSendsBothLimits(t *testing.T) {
	offer := &Offer{TrafficGB: 10, DurationDays: 30, DeviceLimit: 2}
	req := buildClientRequest(7, domain.PanelEmail(7), offer, 1)

	// Assert through JSON, because the wire field name is what the panel reads.
	raw, err := json.Marshal(req.Client)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for field, want := range map[string]float64{"limitIp": 2, "limitHwid": 2} {
		if got, ok := body[field]; !ok {
			t.Errorf("%s missing from the client payload", field)
		} else if got != want {
			t.Errorf("%s = %v, want %v", field, got, want)
		}
	}
	if len(req.InboundIDs) != 1 || req.InboundIDs[0] != 1 {
		t.Errorf("inboundIds = %v, want [1]", req.InboundIDs)
	}
}

// The outbox payload must preserve both limits, otherwise a reconciled create
// would silently differ from the original attempt (new_update.md §5).
func TestPayloadRoundTripPreservesLimits(t *testing.T) {
	req := buildClientRequest(3, domain.PanelEmail(3), &Offer{TrafficGB: 20, DeviceLimit: 3}, 2)
	decoded, ok := decodePayload(encodePayload(req))
	if !ok {
		t.Fatal("payload did not decode")
	}
	if decoded.Client.LimitHwid != 3 {
		t.Errorf("LimitHwid = %d, want 3 (lost in the outbox round-trip)", decoded.Client.LimitHwid)
	}
	if decoded.Client.LimitIP != 3 {
		t.Errorf("LimitIP = %d, want 3", decoded.Client.LimitIP)
	}
	if len(decoded.InboundIDs) != 1 || decoded.InboundIDs[0] != 2 {
		t.Errorf("inboundIds = %v", decoded.InboundIDs)
	}
}

// A reconciliation pass must not create a duplicate when the write landed.
func TestReconcileSkipsDuplicateCreate(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	email := domain.PanelEmail(1)
	key := domain.NewOutboxKey("CREATE_CLIENT", h.server, email)
	req := buildClientRequest(1, email, &Offer{TrafficGB: 30, DurationDays: 30, DeviceLimit: 2}, 1)
	if _, err := h.st.EnqueueOutbox(ctx, nil, "CREATE_CLIENT", h.server, key, encodePayload(req)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// The panel already has it (write landed, response lost).
	h.fake.mu.Lock()
	h.fake.existing[email] = true
	h.fake.mu.Unlock()

	res, err := h.svc.Reconcile(ctx, 10)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res[ReconcileApplied] != 1 {
		t.Fatalf("outcomes = %v, want 1 APPLIED", res)
	}
	if got := atomic.LoadInt32(&h.fake.addCalls); got != 0 {
		t.Fatalf("AddClient calls = %d, want 0 (must not duplicate)", got)
	}
}

// When the create genuinely never landed, reconciliation creates it once.
func TestReconcileCreatesWhenAbsent(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	email := domain.PanelEmail(2)
	key := domain.NewOutboxKey("CREATE_CLIENT", h.server, email)
	req := buildClientRequest(2, email, &Offer{TrafficGB: 10, DeviceLimit: 1}, 1)
	if _, err := h.st.EnqueueOutbox(ctx, nil, "CREATE_CLIENT", h.server, key, encodePayload(req)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	res, err := h.svc.Reconcile(ctx, 10)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res[ReconcileApplied] != 1 {
		t.Fatalf("outcomes = %v, want 1 APPLIED", res)
	}
	if got := atomic.LoadInt32(&h.fake.addCalls); got != 1 {
		t.Fatalf("AddClient calls = %d, want exactly 1", got)
	}
}

// If the panel is unreachable, reconciliation backs off and changes nothing.
func TestReconcileBacksOffWhenPanelDown(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	email := domain.PanelEmail(3)
	key := domain.NewOutboxKey("DELETE_CLIENT", h.server, email)
	if _, err := h.st.EnqueueOutbox(ctx, nil, "DELETE_CLIENT", h.server, key, "{}"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	h.fake.readErr = errors.New("dial tcp: timeout")

	res, err := h.svc.Reconcile(ctx, 10)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res[ReconcileRetry] != 1 {
		t.Fatalf("outcomes = %v, want 1 RETRY", res)
	}
	if got := atomic.LoadInt32(&h.fake.delCalls); got != 0 {
		t.Fatalf("DeleteClient calls = %d, want 0 while the panel is unreadable", got)
	}
}

// A permanently failing operation must eventually be abandoned.
//
// Regression test for the uncapped retry loop: backoff() saturates at 640s, so
// without MaxOutboxAttempts a broken panel operation would be retried ~135
// times a day forever, burning the 1 vCPU budget and hiding the failure in an
// endless RETRY state instead of surfacing it as FAILED.
func TestReconcileStopsAfterMaxAttempts(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	email := domain.PanelEmail(4)
	key := domain.NewOutboxKey("CREATE_CLIENT", h.server, email)
	req := buildClientRequest(4, email, &Offer{TrafficGB: 10, DeviceLimit: 1}, 1)
	id, err := h.st.EnqueueOutbox(ctx, nil, "CREATE_CLIENT", h.server, key, encodePayload(req))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Drive the row to one attempt short of the cap, then let the pass that
	// performs attempt number MaxOutboxAttempts run.
	for i := 0; i < MaxOutboxAttempts-1; i++ {
		if err := h.st.MarkOutbox(ctx, nil, id, "RETRY", "panel down", nil); err != nil {
			t.Fatalf("mark retry: %v", err)
		}
	}
	h.fake.readErr = errors.New("dial tcp: timeout")

	res, err := h.svc.Reconcile(ctx, 10)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res[ReconcileExhausted] != 1 {
		t.Fatalf("outcomes = %v, want 1 EXHAUSTED", res)
	}
	if res[ReconcileRetry] != 0 {
		t.Fatalf("outcomes = %v, want 0 RETRY (the cap must stop the loop)", res)
	}

	row, err := h.st.GetOutbox(ctx, id)
	if err != nil {
		t.Fatalf("GetOutbox: %v", err)
	}
	if row.State != "FAILED" {
		t.Fatalf("state = %q, want FAILED", row.State)
	}
	if row.NextAttemptAt != nil {
		t.Fatalf("next_attempt_at = %v, want nil (a FAILED row is never re-scheduled)", *row.NextAttemptAt)
	}

	// A FAILED row must not be picked up by a later pass.
	if res2, err := h.svc.Reconcile(ctx, 10); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	} else if len(res2) != 0 {
		t.Fatalf("second pass processed %v, want nothing (FAILED is terminal)", res2)
	}
}

// The cap must not fire early: an operation one attempt BELOW the limit still
// retries. Without this, a cap set too aggressively would abandon operations
// that a single panel restart would have healed.
func TestReconcileRetriesBelowMaxAttempts(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	email := domain.PanelEmail(5)
	key := domain.NewOutboxKey("CREATE_CLIENT", h.server, email)
	req := buildClientRequest(5, email, &Offer{TrafficGB: 10, DeviceLimit: 1}, 1)
	id, err := h.st.EnqueueOutbox(ctx, nil, "CREATE_CLIENT", h.server, key, encodePayload(req))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// MaxOutboxAttempts-2 prior attempts: the next pass is attempt N-1.
	for i := 0; i < MaxOutboxAttempts-2; i++ {
		if err := h.st.MarkOutbox(ctx, nil, id, "RETRY", "panel down", nil); err != nil {
			t.Fatalf("mark retry: %v", err)
		}
	}
	h.fake.readErr = errors.New("dial tcp: timeout")

	res, err := h.svc.Reconcile(ctx, 10)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res[ReconcileRetry] != 1 {
		t.Fatalf("outcomes = %v, want 1 RETRY (under the cap)", res)
	}
	row, err := h.st.GetOutbox(ctx, id)
	if err != nil {
		t.Fatalf("GetOutbox: %v", err)
	}
	if row.NextAttemptAt == nil {
		t.Fatal("next_attempt_at = nil, want a scheduled retry")
	}
}

func TestPanelEmailFromKey(t *testing.T) {
	cases := []struct{ key, want string }{
		{"CREATE_CLIENT:1:vyx-7", "vyx-7"},
		{"DELETE_CLIENT:42:vyx-130", "vyx-130"},
		{"CREATE_CLIENT:1", ""},
		{"garbage", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := panelEmailFromKey(c.key); got != c.want {
			t.Errorf("panelEmailFromKey(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

// Idempotency: enqueueing the same logical op twice reuses the row.
func TestOutboxIsIdempotent(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()
	key := domain.NewOutboxKey("CREATE_CLIENT", h.server, "vyx-9")

	a, err := h.st.EnqueueOutbox(ctx, nil, "CREATE_CLIENT", h.server, key, "{}")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := h.st.EnqueueOutbox(ctx, nil, "CREATE_CLIENT", h.server, key, "{}")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a != b {
		t.Fatalf("duplicate rows created: %d vs %d", a, b)
	}
	var n int
	if err := h.st.DB().QueryRow(`SELECT COUNT(*) FROM outbox`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
}

// Revocation only reaches REVOKED after a confirmed absence.
func TestRevokeRequiresVerification(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	created, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Panel unreadable → cannot confirm, so the state must stay pending.
	h.fake.readErr = errors.New("timeout")
	status, err := h.svc.Revoke(ctx, created.VPNID, adminPrincipal(h.admin))
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if status != domain.VPNRevocationPending {
		t.Fatalf("status = %s, want REVOCATION_PENDING while unconfirmed", status)
	}

	// Panel readable and the client is gone → REVOKED is now legitimate.
	h.fake.readErr = nil
	status, err = h.svc.Revoke(ctx, created.VPNID, adminPrincipal(h.admin))
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if status != domain.VPNRevoked {
		t.Fatalf("status = %s, want REVOKED after verification", status)
	}
}

// Ownership: another client must not even see the VPN (404, not 403).
func TestOwnershipIsEnforced(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	created, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	other := clientPrincipal(h.client + 999)
	if err := h.svc.Suspend(ctx, created.VPNID, other); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign client suspend = %v, want ErrNotFound", err)
	}
	// The seller may act on their own sale.
	if err := h.svc.Suspend(ctx, created.VPNID, adminPrincipal(h.admin)); err != nil {
		t.Fatalf("seller suspend: %v", err)
	}
	if got := h.vpnStatus(t, created.VPNID); got != domain.VPNSuspended {
		t.Fatalf("status = %s, want SUSPENDED", got)
	}
}

// A client may only self-provision a FREE offer.
func TestFreeOfferRules(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	// Premium via a client principal is refused.
	if _, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, Actor: clientPrincipal(h.client),
	}); !errors.Is(err, ErrNotPermitted) {
		t.Fatalf("client premium = %v, want ErrNotPermitted", err)
	}

	// A client cannot provision on behalf of someone else.
	freeH := newHarness(t, 0, "FREE")
	if _, err := freeH.svc.Create(ctx, CreateInput{
		OfferID: freeH.offer, ClientUserID: freeH.client + 1, Actor: clientPrincipal(freeH.client),
	}); !errors.Is(err, ErrNotPermitted) {
		t.Fatalf("cross-client free = %v, want ErrNotPermitted", err)
	}

	// Their own FREE offer succeeds.
	res, err := freeH.svc.Create(ctx, CreateInput{
		OfferID: freeH.offer, ClientUserID: freeH.client, Actor: clientPrincipal(freeH.client),
	})
	if err != nil {
		t.Fatalf("free self-provision: %v", err)
	}
	if res.Status != domain.VPNActive {
		t.Fatalf("status = %s, want ACTIVE", res.Status)
	}
}
