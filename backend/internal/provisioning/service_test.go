package provisioning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

// fakeClient records panel interactions so tests can assert exactly what was
// written and whether duplicates were attempted.
type fakeClient struct {
	mu          sync.Mutex
	existing    map[string]bool
	addCalls    int32
	delCalls    int32
	enableCalls int32
	addErr      error
	readErr     error
	delErr      error
	// enableErr makes SetClientEnabled fail, which is how a suspend/resume
	// against an unreachable panel is simulated. Without it the fake always
	// succeeded and the suspend error path was never exercised.
	enableErr error
	// links is what Links returns; linkErr simulates an unreachable panel.
	links   []string
	linkErr error
	// specs is every client body the service asked the panel to create, so a
	// test can assert exactly what the customer would receive (D-035.2).
	specs []panel.ClientSpec
	// inboundIDs is the inbound list of every create request, in order. The
	// panel numbers its inbounds itself, so this is how a test proves the
	// service sent the PANEL's number and not Veylix's local row id.
	inboundIDs [][]int
	// records is what GetClient answers, keyed by panel email (P-09).
	records map[string]*panelRecord
	getErr  error
}

// lastSpec returns the most recent create body.
func (f *fakeClient) lastSpec(t *testing.T) panel.ClientSpec {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.specs) == 0 {
		t.Fatal("the panel was never asked to create a client")
	}
	return f.specs[len(f.specs)-1]
}

// Links returns the canned connection URIs (D-026).
func (f *fakeClient) Links(_ context.Context, email string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.linkErr != nil {
		return nil, f.linkErr
	}
	if !f.existing[email] {
		return []string{}, nil
	}
	return f.links, nil
}

func newFake() *fakeClient {
	return &fakeClient{existing: map[string]bool{}, records: map[string]*panelRecord{}}
}

func (f *fakeClient) AddClient(_ context.Context, req panel.AddClientRequest) error {
	atomic.AddInt32(&f.addCalls, 1)
	if f.addErr != nil {
		return f.addErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs = append(f.specs, req.Client)
	f.inboundIDs = append(f.inboundIDs, req.InboundIDs)
	// Mirrors the panel: a duplicate identity is rejected.
	if f.existing[req.Client.Email] {
		return errors.New("panel: duplicate email")
	}
	f.existing[req.Client.Email] = true
	return nil
}

func (f *fakeClient) ClientExists(_ context.Context, email string) (bool, error) {
	if f.readErr != nil {
		return false, f.readErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.existing[email], nil
}

func (f *fakeClient) DeleteClient(_ context.Context, email string) error {
	atomic.AddInt32(&f.delCalls, 1)
	if f.delErr != nil {
		return f.delErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.existing, email)
	return nil
}

func (f *fakeClient) SetClientEnabled(_ context.Context, _ string, _ bool) error {
	atomic.AddInt32(&f.enableCalls, 1)
	if f.enableErr != nil {
		return f.enableErr
	}
	return nil
}

// GetClient answers the panel's own client record, which the protocol detail
// needs (P-09). A test seeds `records`; `getErr` simulates an unreadable panel.
func (f *fakeClient) GetClient(_ context.Context, email string) (*panel.PanelClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	rec, ok := f.records[email]
	if !ok {
		return nil, nil
	}
	return &panel.PanelClient{
		Email: email, UUID: rec.uuid, Password: rec.password, Flow: rec.flow,
		Security: rec.security, Enable: true,
	}, nil
}

// panelRecord is a scripted panel client record for the fake.
type panelRecord struct {
	uuid, password, flow, security string
}

// errPanicRead stands in for an unreachable panel in the protocol test.
var errPanicRead = errors.New("panel unreachable")

type harness struct {
	svc     *Service
	st      *store.Store
	fake    *fakeClient
	server  int64
	inbound int64
	offer   int64
	admin   int64
	client  int64
}

func newHarness(t *testing.T, quota int, kind string) *harness {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "prov.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db := st.DB()

	var serverID, inboundID, offerID, adminID, clientID int64
	mustExec := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	serverID = mustExec(`INSERT INTO server_integrations
		(name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv','https://panel.example:2053/base', x'00', 'ACTIVE', 1, 1)`)
	inboundID = mustExec(`INSERT INTO inbounds (server_id, remote_id, remark, protocol, port, synced_at)
		VALUES (?, 1, '(IN)-free_2', 'vless', 443, 1)`, serverID)
	adminID = mustExec(`INSERT INTO users (email, role, status, quota_limit, created_at, updated_at)
		VALUES ('admin@example.com','admin','ACTIVE',?,1,1)`, quota)
	clientID = mustExec(`INSERT INTO users (email, role, status, created_at, updated_at)
		VALUES ('client@example.com','client','ACTIVE',1,1)`)

	// D-030.2 + M6-06: a reseller may only release a plan that is his, on a
	// server assigned to him. The harness therefore gives the admin both — the
	// narrow rights are asserted by TestResellerCannotReleaseForeignPlan and
	// TestResellerCannotReleaseOnUnassignedServer below.
	offerID = mustExec(`INSERT INTO vpn_offers
		(kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit, active, owner_user_id, created_at, updated_at)
		VALUES (?, 'Test offer', ?, ?, 30, 30, 2, 1, ?, 1, 1)`, kind, serverID, inboundID, adminID)
	mustExec(`INSERT INTO admin_server_assignments (admin_id, server_id, created_at)
		VALUES (?, ?, 1)`, adminID, serverID)

	fake := newFake()
	svc := New(db, st, audit.New(db), func(context.Context, int64) (PanelClient, error) {
		return fake, nil
	})

	return &harness{svc: svc, st: st, fake: fake, server: serverID, inbound: inboundID, offer: offerID, admin: adminID, client: clientID}
}

func (h *harness) vpnStatus(t *testing.T, id int64) domain.VPNStatus {
	t.Helper()
	var s string
	if err := h.st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatalf("status: %v", err)
	}
	return domain.VPNStatus(s)
}

func adminPrincipal(id int64) auth.Principal {
	return auth.Principal{UserID: id, Role: auth.RoleAdmin}
}

func clientPrincipal(id int64) auth.Principal {
	return auth.Principal{UserID: id, Role: auth.RoleClient}
}

// superAdminPrincipal is the actor for operator-only operations.
//
// `Approved: true` is what a Super-Admin *payment confirmation* sets (D-030.3,
// D-031.3). The HTTP provisioning route never sets it, so an unowned premium
// plan cannot be released from the outside — see
// TestUnownedPremiumPlanNeedsSuperAdminConfirm for the refusal.
func superAdminPrincipal(id int64) auth.Principal {
	return auth.Principal{UserID: id, Role: auth.RoleSuperAdmin}
}

// countVPNs is the number of VPN records currently stored.
func (h *harness) countVPNs(t *testing.T) int {
	t.Helper()
	var n int
	if err := h.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances`).Scan(&n); err != nil {
		t.Fatalf("count vpns: %v", err)
	}
	return n
}

// quotaUsed sums the quota ledger for the harness reseller.
func (h *harness) quotaUsed(t *testing.T) int {
	t.Helper()
	var n int
	if err := h.st.DB().QueryRow(
		`SELECT COALESCE(SUM(delta),0) FROM quota_ledger WHERE admin_id = ?`, h.admin).Scan(&n); err != nil {
		t.Fatalf("quota used: %v", err)
	}
	return n
}

// T-06: a successful provision creates exactly one panel client and confirms it.
func TestCreateProvisionsAndConfirms(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !res.Confirmed || res.Status != domain.VPNActive {
		t.Fatalf("result = %+v, want confirmed ACTIVE", res)
	}
	if got := atomic.LoadInt32(&h.fake.addCalls); got != 1 {
		t.Fatalf("panel AddClient calls = %d, want 1", got)
	}
	if h.vpnStatus(t, res.VPNID) != domain.VPNActive {
		t.Errorf("stored status = %s", h.vpnStatus(t, res.VPNID))
	}
	// D-034: the identity names the customer, and its slot is deterministic per
	// (client, server, kind) — the first paid VPN of this customer is `_paid_1`.
	if res.Email != "client@example.com_paid_1" {
		t.Errorf("panel email = %q, want client@example.com_paid_1", res.Email)
	}
	// D-012 idempotency still holds, because the subId is derived from the row id.
	if res.VPNID == 0 {
		t.Error("a created VPN must have a local id")
	}
}

// T-08: concurrent creates cannot exceed the reseller quota.
func TestQuotaIsEnforcedUnderConcurrency(t *testing.T) {
	h := newHarness(t, 1, "PREMIUM")
	ctx := context.Background()

	var wg sync.WaitGroup
	var ok, quotaErr int32
	var otherErrs sync.Map // index -> error; a third outcome must not vanish
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := h.svc.Create(ctx, CreateInput{
				OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
			})
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, ErrQuotaExceeded):
				atomic.AddInt32(&quotaErr, 1)
			default:
				// A third outcome used to be swallowed by this switch, which is
				// why the CI failure read "ok=1 quotaErr=0" with no cause: the
				// losing request did fail, just not with the error under test.
				// Keep it so the next red build names itself.
				otherErrs.Store(i, err)
			}
		}(i)
	}
	wg.Wait()

	if ok != 1 || quotaErr != 1 {
		var detail []string
		otherErrs.Range(func(k, v any) bool {
			detail = append(detail, fmt.Sprintf("req %v -> %v", k, v))
			return true
		})
		if len(detail) == 0 {
			t.Fatalf("ok=%d quotaErr=%d, want exactly 1 success and 1 refusal",
				ok, quotaErr)
		}
		t.Fatalf("ok=%d quotaErr=%d, want exactly 1 success and 1 refusal; unexpected errors: %s",
			ok, quotaErr, strings.Join(detail, "; "))
	}
	if got := atomic.LoadInt32(&h.fake.addCalls); got != 1 {
		t.Fatalf("panel received %d creates, want 1", got)
	}
}

// T-04: an unreachable panel must never be recorded as REVOKED.
func TestUnreachablePanelIsUnavailableNotRevoked(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()
	// Force the factory to fail (panel disabled / no client buildable).
	svc := New(h.st.DB(), h.st, audit.New(h.st.DB()), func(context.Context, int64) (PanelClient, error) {
		return nil, errors.New("panel disabled")
	})

	res, err := svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Status != domain.VPNServerUnavailable {
		t.Fatalf("status = %s, want SERVER_UNAVAILABLE", res.Status)
	}
	if res.Confirmed {
		t.Fatal("an unreachable panel must not report a confirmed provisioning")
	}
	if got := h.vpnStatus(t, res.VPNID); got == domain.VPNRevoked {
		t.Fatal("an outage must never produce REVOKED")
	}
}

// A write error whose result is unreadable keeps the work queued (no blind claim).
func TestAmbiguousWriteIsQueuedNotAssumed(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()
	h.fake.addErr = errors.New("timeout")
	h.fake.readErr = errors.New("timeout")

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Confirmed {
		t.Fatal("an unreadable outcome must not be reported as confirmed")
	}
	if res.Status != domain.VPNServerUnavailable {
		t.Fatalf("status = %s, want SERVER_UNAVAILABLE", res.Status)
	}

	rows, err := h.st.PendingOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("PendingOutbox: %v", err)
	}
	if len(rows) != 0 {
		t.Fatal("a row scheduled for a future retry must not be returned as pending now")
	}

	// The work must still be recorded (not lost, not silently applied).
	var state string
	var attempts int
	if err := h.st.DB().QueryRow(
		`SELECT state, attempts FROM outbox WHERE op = 'CREATE_CLIENT'`).Scan(&state, &attempts); err != nil {
		t.Fatalf("outbox row missing: %v", err)
	}
	if state != "RETRY" {
		t.Fatalf("outbox state = %s, want RETRY", state)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}

	// After the backoff window it becomes eligible for reconciliation.
	if _, err := h.st.DB().Exec(`UPDATE outbox SET next_attempt_at = 0`); err != nil {
		t.Fatalf("rewind: %v", err)
	}
	rows, err = h.st.PendingOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("PendingOutbox: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("after the backoff the row must be eligible again")
	}
}

// --- verified absence + local removal (D-024.5 / D-027.3) -------------------

// A VPN whose panel client is gone is reconciled to SERVER_UNAVAILABLE by an
// explicit verify, never to REVOKED (we did not delete it) — D-024.5.
func TestVerifyPanelMarksAbsentVPNUnavailable(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if h.vpnStatus(t, res.VPNID) != domain.VPNActive {
		t.Fatalf("setup: VPN not ACTIVE")
	}

	// The operator deleted the client in the 3X-UI panel.
	h.fake.mu.Lock()
	delete(h.fake.existing, res.Email)
	h.fake.mu.Unlock()

	got, err := h.svc.VerifyPanel(ctx, res.VPNID, superAdminPrincipal(1))
	if err != nil {
		t.Fatalf("VerifyPanel: %v", err)
	}
	if got.Exists {
		t.Error("Exists = true, want false after the client was removed")
	}
	if got.Status != domain.VPNServerUnavailable {
		t.Errorf("status = %s, want SERVER_UNAVAILABLE", got.Status)
	}
	if s := h.vpnStatus(t, res.VPNID); s == domain.VPNRevoked {
		t.Error("verify must never produce REVOKED")
	} else if s != domain.VPNServerUnavailable {
		t.Errorf("persisted = %s, want SERVER_UNAVAILABLE", s)
	}
}

// A VPN the panel still holds must not be touched by verify.
func TestVerifyPanelLeavesLiveVPNAlone(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, _ := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})
	got, err := h.svc.VerifyPanel(ctx, res.VPNID, superAdminPrincipal(1))
	if err != nil {
		t.Fatalf("VerifyPanel: %v", err)
	}
	if !got.Exists {
		t.Error("Exists = false for a live client")
	}
	if s := h.vpnStatus(t, res.VPNID); s != domain.VPNActive {
		t.Errorf("status = %s, want ACTIVE unchanged", s)
	}
}

// An unreachable panel is not evidence of absence: verify must refuse rather
// than mark anything (REQUIREMENTS Â§7).
func TestVerifyPanelRefusesWhenPanelUnreadable(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, _ := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})
	h.fake.mu.Lock()
	h.fake.readErr = errors.New("panel: request failed: timeout")
	h.fake.mu.Unlock()

	if _, err := h.svc.VerifyPanel(ctx, res.VPNID, superAdminPrincipal(1)); !errors.Is(err, ErrPanelFailed) {
		t.Fatalf("err = %v, want ErrPanelFailed", err)
	}
	if s := h.vpnStatus(t, res.VPNID); s != domain.VPNActive {
		t.Errorf("status = %s, want ACTIVE — a failed read must change nothing", s)
	}
}

// A suspend the panel refuses is a PANEL failure, never "vpn not found".
//
// The original code wrapped this error in ErrNotFound, so the API answered
// 404 not_found for a VPN that existed and had just been loaded — the operator
// was told their record was gone when the truth was that the panel was
// unreachable. Resume always answered ErrPanelFailed for the same condition;
// this test keeps suspend honest and the two consistent.
func TestSuspendReportsPanelFailureNotNotFound(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, _ := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})

	h.fake.mu.Lock()
	h.fake.enableErr = errors.New("panel: request failed: connection refused")
	h.fake.mu.Unlock()

	err := h.svc.Suspend(ctx, res.VPNID, superAdminPrincipal(1))
	if err == nil {
		t.Fatal("Suspend succeeded against a panel that refused the write")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want a panel failure — ErrNotFound claims the VPN does not exist", err)
	}
	if !errors.Is(err, ErrPanelFailed) {
		t.Fatalf("err = %v, want ErrPanelFailed so the API answers panel_error", err)
	}
	// Nothing was confirmed, so nothing may claim the new state.
	if s := h.vpnStatus(t, res.VPNID); s == domain.VPNSuspended {
		t.Errorf("status = %s, want the previous state — an unconfirmed suspend cannot report SUSPENDED", s)
	}
}

// RemoveAbsent erases only the local record and NEVER the panel client
// (D-027.3), and frees the reseller slot on the way.
func TestRemoveAbsentDeletesLocalRecordOnly(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, _ := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})

	// The panel no longer has this client.
	h.fake.mu.Lock()
	delete(h.fake.existing, res.Email)
	h.fake.mu.Unlock()
	delsBefore := atomic.LoadInt32(&h.fake.delCalls)

	if err := h.svc.RemoveAbsent(ctx, res.VPNID, superAdminPrincipal(1)); err != nil {
		t.Fatalf("RemoveAbsent: %v", err)
	}

	var n int
	if err := h.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances WHERE id = ?`, res.VPNID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Error("the local row must be gone")
	}
	var a int
	if err := h.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_assignments WHERE vpn_instance_id = ?`, res.VPNID).Scan(&a); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if a != 0 {
		t.Error("the assignment must be gone (it RESTRICTs the delete)")
	}
	if got := atomic.LoadInt32(&h.fake.delCalls); got != delsBefore {
		t.Error("RemoveAbsent must never delete the panel client")
	}
}

// --- D-024.1a: a failed create leaves no VPN row ----------------------------

// An offer with no inbound can never produce a working client, so Create must
// refuse it before writing anything (the old behaviour sent inboundIds:[0],
// the panel rejected it, and a phantom VPN row survived).
func TestCreateRefusesOfferWithoutInbound(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	// Replace the offer with one that has no inbound attached.
	if _, err := h.st.DB().Exec(
		`UPDATE vpn_offers SET inbound_id = NULL WHERE id = ?`, h.offer); err != nil {
		t.Fatalf("detach inbound: %v", err)
	}

	before := h.countVPNs(t)

	_, err := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})
	if !errors.Is(err, ErrOfferIncomplete) {
		t.Fatalf("err = %v, want ErrOfferIncomplete", err)
	}
	if after := h.countVPNs(t); after != before {
		t.Errorf("vpn rows = %d, want %d — an incomplete offer must write nothing", after, before)
	}
	if calls := atomic.LoadInt32(&h.fake.addCalls); calls != 0 {
		t.Error("the panel must not be called for an incomplete offer")
	}
}

// The panel is told the PANEL's inbound number, never Veylix's local row id.
//
// This is the defect that made every claim fail. `vpn_offers.inbound_id` is a
// foreign key to `inbounds(id)` — an AUTOINCREMENT row id local to Veylix — while
// the panel numbers its own inbounds. The harness reproduces the real shape: the
// local row is id 1 with remote_id 7, so a create that forwards the local id
// sends `inboundIds:[1]` and the panel answers "invalid inbound". `pushCreate`
// then correctly discarded the provisional row and returned 502 panel_rejected,
// which is why no client could ever obtain a FREE VPN.
func TestCreateSendsThePanelsInboundNumber(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	// Move the panel's number away from the local row id so forwarding the
	// wrong one is visible rather than accidentally correct.
	if _, err := h.st.DB().Exec(
		`UPDATE inbounds SET remote_id = 7 WHERE id = ?`, h.inbound); err != nil {
		t.Fatalf("set remote_id: %v", err)
	}

	if _, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if len(h.fake.inboundIDs) != 1 {
		t.Fatalf("panel creates = %d, want 1", len(h.fake.inboundIDs))
	}
	got := h.fake.inboundIDs[0]
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("inboundIds = %v, want [7] (the panel's number, not local id %d)", got, h.inbound)
	}
}

// mustExec runs one statement and returns LastInsertId, failing the test on
// error. A package-level helper (newHarness has its own closure) so a test can
// seed rows the harness does not build.
func mustExec(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// An offer whose inbound belongs to a DIFFERENT server must refuse.
//
// The FK guarantees the row exists; it cannot guarantee the row belongs to the
// offer's server. Sending that inbound to the panel would attach the client to
// an inbound on the wrong machine — or, once the two numbering schemes differ,
// to nothing at all. This is the reachable form of "unresolvable inbound".
func TestCreateRefusesInboundFromAnotherServer(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	otherServer := mustExec(t, h.st.DB(),
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('other','https://other.example:2053/base', x'00', 'ACTIVE', 1, 1)`)
	if _, err := h.st.DB().Exec(
		`UPDATE inbounds SET server_id = ? WHERE id = ?`, otherServer, h.inbound); err != nil {
		t.Fatalf("move inbound to another server: %v", err)
	}

	before := h.countVPNs(t)
	_, err := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})
	if !errors.Is(err, ErrOfferIncomplete) {
		t.Fatalf("err = %v, want ErrOfferIncomplete", err)
	}
	if after := h.countVPNs(t); after != before {
		t.Errorf("vpn rows = %d, want %d — an unresolvable inbound must write nothing", after, before)
	}
	if calls := atomic.LoadInt32(&h.fake.addCalls); calls != 0 {
		t.Error("the panel must not be called when the inbound cannot be resolved")
	}
}

// A panel rejection must roll the provisional rows back: no VPN, no assignment,
// no quota consumed (D-024.1a). The previous behaviour stored
// REVOCATION_PENDING, leaving a VPN that never existed.
func TestPanelRejectionLeavesNoVPNRow(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	// The panel refuses every write.
	h.fake.mu.Lock()
	h.fake.addErr = errors.New("panel: request rejected: invalid inbound")
	h.fake.mu.Unlock()

	before := h.countVPNs(t)
	quotaBefore := h.quotaUsed(t)

	res, err := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: adminPrincipal(h.admin)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Recorded {
		t.Error("Recorded = true for a create the panel refused")
	}
	if res.Confirmed {
		t.Error("Confirmed = true for a create the panel refused")
	}
	if after := h.countVPNs(t); after != before {
		t.Errorf("vpn rows = %d, want %d — a rejected create must leave no VPN", after, before)
	}
	var assignments int
	if err := h.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_assignments`).Scan(&assignments); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if assignments != 0 {
		t.Errorf("assignments = %d, want 0 (rolled back with the VPN)", assignments)
	}
	if q := h.quotaUsed(t); q != quotaBefore {
		t.Errorf("quota used = %d, want %d — a failed create must not consume capacity", q, quotaBefore)
	}

	// The attempt is still recorded for diagnosis.
	var state string
	if err := h.st.DB().QueryRow(
		`SELECT state FROM outbox WHERE op = 'CREATE_CLIENT' ORDER BY id DESC LIMIT 1`).Scan(&state); err != nil {
		t.Fatalf("outbox row missing: %v", err)
	}
	if state != "FAILED" {
		t.Errorf("outbox state = %s, want FAILED", state)
	}
}

// A rejected create must never leave the VPN reading REVOCATION_PENDING, a state
// that claims a client existed and was deleted.
func TestPanelRejectionDoesNotClaimRevocation(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	h.fake.mu.Lock()
	h.fake.addErr = errors.New("panel: request rejected: invalid inbound")
	h.fake.mu.Unlock()

	res, err := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Status == domain.VPNRevocationPending {
		t.Error("status = REVOCATION_PENDING — no client ever existed to revoke")
	}

	var n int
	_ = h.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances WHERE status = 'REVOCATION_PENDING'`).Scan(&n)
	if n != 0 {
		t.Errorf("%d rows are REVOCATION_PENDING, want 0", n)
	}
}

// A VPN the panel still holds cannot be erased — it must be revoked instead.
func TestRemoveAbsentRefusesLiveClient(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, _ := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})

	err := h.svc.RemoveAbsent(ctx, res.VPNID, superAdminPrincipal(1))
	if !errors.Is(err, ErrVPNStillOnPanel) {
		t.Fatalf("err = %v, want ErrVPNStillOnPanel", err)
	}
	if s := h.vpnStatus(t, res.VPNID); s != domain.VPNActive {
		t.Errorf("a refused removal must leave the VPN untouched, got %s", s)
	}
}

// A client (non-administrator) may not erase records.
func TestRemoveAbsentRefusesClientRole(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, _ := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})
	h.fake.mu.Lock()
	delete(h.fake.existing, res.Email)
	h.fake.mu.Unlock()

	err := h.svc.RemoveAbsent(ctx, res.VPNID, clientPrincipal(h.client))
	if !errors.Is(err, ErrNotPermitted) {
		t.Fatalf("err = %v, want ErrNotPermitted", err)
	}
}

// A write that reported an error but actually landed must be adopted, not retried.
func TestWriteThatLandedIsAdopted(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	// Simulate: panel stores the client, but the response was lost.
	h.fake.addErr = errors.New("connection reset")
	h.fake.mu.Lock()
	// Pre-create the identity the service will use: the first paid VPN of this
	// customer on this server (D-034).
	email := "client@example.com_paid_1"
	h.fake.existing[email] = true
	h.fake.mu.Unlock()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !res.Confirmed || res.Status != domain.VPNActive {
		t.Fatalf("result = %+v, want confirmed ACTIVE after adopting the existing client", res)
	}
}

// D-030.3 — the owner's rule in one test: "the other premium, the one no admin
// has bought, must be unusable by anyone until the Super Admin confirms it."
//
// A client, and a reseller who does not own it, are both refused with
// ErrPlanNotApproved — a hard refusal, not a pending queue that something else
// could complete.
func TestUnownedPremiumPlanNeedsSuperAdminConfirm(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	// Return the plan to platform inventory, as a plan the SA created.
	if _, err := h.st.DB().Exec(`UPDATE vpn_offers SET owner_user_id = NULL WHERE id = ?`, h.offer); err != nil {
		t.Fatalf("clear owner: %v", err)
	}

	// A reseller who wants to release it to his own client.
	_, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("reseller error = %v, want ErrPlanNotApproved", err)
	}

	// A client who tries to provision it for himself.
	_, err = h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, Actor: clientPrincipal(h.client),
	})
	if !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("client error = %v, want ErrPlanNotApproved", err)
	}

	if n := h.countVPNs(t); n != 0 {
		t.Errorf("VPNs created = %d, want 0 — a refused release must leave no row", n)
	}

	// The Super Admin, on behalf of a confirmation, may release it.
	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true,
	})
	if err != nil {
		t.Fatalf("approved release: %v", err)
	}
	if !res.Recorded || res.Status != domain.VPNActive {
		t.Fatalf("approved release = %+v, want ACTIVE", res)
	}
}

// D-030.2 — "an admin's own premium needs no approval": the same call that was
// refused above succeeds once the plan belongs to that admin and the server is
// assigned to him.
func TestResellerOwnPlanNeedsNoApproval(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("own-plan release must need no approval: %v", err)
	}
	if !res.Recorded || res.Status != domain.VPNActive {
		t.Fatalf("result = %+v, want ACTIVE", res)
	}
}

// A reseller may not release another reseller's plan (D-030.2), even on a
// server he does have.
func TestResellerCannotReleaseForeignPlan(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	other := mustSeedUser(t, h, "other-admin@example.com", "admin")
	_, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: other, Actor: adminPrincipal(other),
	})
	if !errors.Is(err, ErrNotPlanOwner) {
		t.Fatalf("error = %v, want ErrNotPlanOwner", err)
	}
}

// A reseller may not release on a server that was never assigned to him
// (M6-06).
func TestResellerCannotReleaseOnUnassignedServer(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	if _, err := h.st.DB().Exec(`DELETE FROM admin_server_assignments`); err != nil {
		t.Fatalf("unassign server: %v", err)
	}
	_, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if !errors.Is(err, ErrServerNotAssigned) {
		t.Fatalf("error = %v, want ErrServerNotAssigned", err)
	}
	if n := h.countVPNs(t); n != 0 {
		t.Errorf("VPNs created = %d, want 0", n)
	}
}

// D-030.4 — a plan with one slot produces exactly one VPN; the second release
// is refused and leaves no row behind.
func TestPlanSlotsLimitReleases(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	if _, err := h.st.DB().Exec(
		`UPDATE vpn_offers SET slots_total = 1, slots_used = 0 WHERE id = ?`, h.offer); err != nil {
		t.Fatalf("set slots: %v", err)
	}

	if _, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	}); err != nil {
		t.Fatalf("first release: %v", err)
	}

	second := mustSeedUser(t, h, "second-client@example.com", "client")
	_, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: second, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if !errors.Is(err, ErrPlanExhausted) {
		t.Fatalf("second release error = %v, want ErrPlanExhausted", err)
	}
	if n := h.countVPNs(t); n != 1 {
		t.Errorf("VPNs = %d, want exactly 1 (the plan had one slot)", n)
	}

	var used, total int
	if err := h.st.DB().QueryRow(
		`SELECT slots_used, slots_total FROM vpn_offers WHERE id = ?`, h.offer).Scan(&used, &total); err != nil {
		t.Fatalf("read slots: %v", err)
	}
	if used != 1 || total != 1 {
		t.Errorf("slots = %d/%d, want 1/1", used, total)
	}
}

// mustSeedUser inserts an account and returns its id.
func mustSeedUser(t *testing.T, h *harness, email, role string) int64 {
	t.Helper()
	res, err := h.st.DB().Exec(
		`INSERT INTO users (email, role, status, created_at, updated_at) VALUES (?, ?, 'ACTIVE', 1, 1)`,
		email, role)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestForcePurgeKeepsThePanelClient is the P-3 regression test.
//
// The defect: a VPN record whose panel client still existed could not be removed
// at all. `RemoveAbsent` refuses with `ErrVPNStillOnPanel`, and that refusal was
// the ONLY path — so an operator had no way to retire a record while deliberately
// KEEPING its panel client, which is exactly what D-027.3 requires Veylix to do
// (never delete a panel client).
//
// What is asserted:
//   - without the panel email repeated back, the purge is refused and NOTHING is
//     deleted (a mis-clicked row cannot destroy the wrong record);
//   - with it, the LOCAL record goes;
//   - the PANEL CLIENT SURVIVES — the fake records every DeleteClient call and the
//     count must not move;
//   - a non-Super-Admin cannot force a purge at all;
//   - the audit entry is `vpn.force_purge`, not `vpn.remove_absent`.
func TestForcePurgeKeepsThePanelClient(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, _ := h.svc.Create(ctx, CreateInput{OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true})

	delsBefore := atomic.LoadInt32(&h.fake.delCalls)

	// 1. No confirmation → refused, nothing changed.
	err := h.svc.ForcePurgeRecord(ctx, res.VPNID, superAdminPrincipal(1), "")
	var pc *PurgeConfirmError
	if !errors.As(err, &pc) {
		t.Fatalf("purge without confirmation = %v, want PurgeConfirmError", err)
	}
	var n int
	if err := h.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances WHERE id = ?`, res.VPNID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatal("a refused purge deleted the record")
	}

	// 2. A WRONG email is refused too — this is the guard against a mis-clicked row.
	err = h.svc.ForcePurgeRecord(ctx, res.VPNID, superAdminPrincipal(1), "somebody-else@example.com")
	if !errors.As(err, &pc) {
		t.Fatalf("purge with the wrong email = %v, want PurgeConfirmError", err)
	}

	// 3. A reseller cannot force a purge, even with the right email.
	reseller := auth.Principal{UserID: 2, Role: auth.RoleAdmin}
	if err := h.svc.ForcePurgeRecord(ctx, res.VPNID, reseller, res.Email); !errors.Is(err, ErrNotPermitted) {
		t.Fatalf("a reseller's purge = %v, want ErrNotPermitted", err)
	}

	// 4. The real thing.
	if err := h.svc.ForcePurgeRecord(ctx, res.VPNID, superAdminPrincipal(1), res.Email); err != nil {
		t.Fatalf("ForcePurgeRecord: %v", err)
	}
	if err := h.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances WHERE id = ?`, res.VPNID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatal("the local row must be gone after a confirmed purge")
	}

	// 5. The panel client must be UNTOUCHED — the whole point of P-3 is that a
	//    purge is bookkeeping, not revocation (D-027.3).
	if got := atomic.LoadInt32(&h.fake.delCalls); got != delsBefore {
		t.Fatalf("DeleteClient was called %d time(s); a purge must NEVER delete the panel client", got-delsBefore)
	}
	h.fake.mu.Lock()
	_, stillOnPanel := h.fake.existing[res.Email]
	h.fake.mu.Unlock()
	if !stillOnPanel {
		t.Fatal("the panel client disappeared; a purge must leave it in place")
	}

	// 6. The trail says it was forced.
	var action string
	if err := h.st.DB().QueryRow(
		`SELECT action FROM audit_logs WHERE target_id = ? ORDER BY id DESC LIMIT 1`, fmt.Sprint(res.VPNID),
	).Scan(&action); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if action != audit.ActionVPNForcePurge {
		t.Fatalf("audited as %q, want %q", action, audit.ActionVPNForcePurge)
	}
}
