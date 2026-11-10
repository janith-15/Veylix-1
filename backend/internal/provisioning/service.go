// Package provisioning owns VPN creation and lifecycle across the local
// database and the 3X-UI panel.
//
// Guarantees (new_update.md Â§5, Â§6):
//   - the local row and the quota ledger are written in ONE transaction, so two
//     concurrent requests cannot both take the last reseller slot;
//   - the remote write is recorded in the outbox BEFORE it is attempted, so an
//     ambiguous outcome can always be reconciled;
//   - a transport failure is resolved by a fresh read, never by a blind retry;
//   - a local transaction never claims to cover the panel.
package provisioning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

// Errors surfaced to the transport layer.
var (
	ErrQuotaExceeded = errors.New("provisioning: reseller quota exhausted")
	ErrOfferInactive = errors.New("provisioning: offer is not available")
	ErrOfferNotFound = errors.New("provisioning: offer not found")
	ErrNotPermitted  = errors.New("provisioning: not permitted")
	ErrNotFound      = errors.New("provisioning: vpn not found")
	ErrPanelFailed   = errors.New("provisioning: the panel did not confirm the operation")
	// ErrPlanNotApproved refuses an unowned PREMIUM plan released without a
	// Super-Admin confirmation (D-030.3). It is a hard refusal, never a queue.
	ErrPlanNotApproved = errors.New("provisioning: this premium plan needs a Super Admin confirmation")
	// ErrNotPlanOwner refuses a release of another reseller's plan (D-030.2).
	ErrNotPlanOwner = errors.New("provisioning: this plan belongs to another reseller")
	// ErrServerNotAssigned refuses a release on a server that was not assigned
	// to the caller (M6-06).
	ErrServerNotAssigned = errors.New("provisioning: that server is not assigned to you")
	// ErrPlanExhausted refuses a release when the plan has no slot left
	// (D-030.4).
	ErrPlanExhausted = errors.New("provisioning: this plan has no slots left")
	// ErrPlanArchived refuses a release of a plan that left the catalogue.
	ErrPlanArchived = errors.New("provisioning: this plan is archived")
	// ErrSpecIncomplete refuses a custom release whose spec or target is
	// missing: a VPN cannot be built from a half-stated order.
	ErrSpecIncomplete = errors.New("provisioning: the custom spec is incomplete")
	// ErrVPNStillOnPanel refuses to erase the record of a VPN whose panel client
	// still exists: that VPN works, so it must be revoked (which deletes the
	// client) rather than silently forgotten (D-027.3).
	ErrVPNStillOnPanel = errors.New("provisioning: the panel still holds this client")
	// ErrOfferIncomplete refuses an offer that cannot produce a working client
	// (no inbound attached). Catching it before any write keeps D-024.1a: a
	// failed create leaves no VPN row at all.
	ErrOfferIncomplete = errors.New("provisioning: this offer has no inbound selected")
)

// VerifyResult reports what a panel existence check found.
type VerifyResult struct {
	Exists bool             `json:"exists"`
	Status domain.VPNStatus `json:"status"`
}

// clientFactory builds a panel client for a server id. Injected so tests can
// exercise the whole flow without a network.
type clientFactory func(ctx context.Context, serverID int64) (PanelClient, error)

// PanelClient is the adapter subset provisioning needs.
type PanelClient interface {
	AddClient(ctx context.Context, req panel.AddClientRequest) error
	ClientExists(ctx context.Context, email string) (bool, error)
	DeleteClient(ctx context.Context, email string) error
	SetClientEnabled(ctx context.Context, email string, enable bool) error
	// Links returns the connection URIs (vless://, vmess://, …) for a client.
	// Reading them never changes the panel (D-026): an inactive client simply
	// has no links to hand out.
	Links(ctx context.Context, email string) ([]string, error)
}

// Service creates and manages VPNs.
type Service struct {
	db      *sql.DB
	st      *store.Store
	audit   *audit.Logger
	factory clientFactory
}

// New builds the service.
func New(db *sql.DB, st *store.Store, auditLog *audit.Logger, factory clientFactory) *Service {
	return &Service{db: db, st: st, audit: auditLog, factory: factory}
}

// Spec overrides the offer's limits for one release (D-035.2, D-036.6).
//
// A CUSTOM order carries the customer's own traffic, duration, device and IP
// counts, and the Super Admin picks the panel it lives on. Setting Spec means
// "use these values instead of the package's", which is the only way the
// product can sell a spec the operator never wrote as a package.
type Spec struct {
	TrafficGB    int64
	DurationDays int
	Devices      int
	IPs          int
	ServerID     int64
	InboundID    int64
}

// CreateInput describes one provisioning request.
type CreateInput struct {
	OfferID      int64
	ClientUserID int64 // the account that will own the VPN
	SellerID     int64 // reseller (0 for a direct Super-Admin sale)
	Actor        auth.Principal
	// FreeMonthlyLimit overrides the per-client monthly FREE claim cap; 0 means
	// "use the policy default". Set by the caller from the client's account.
	FreeMonthlyLimit int
	// Approved marks a release authorised by a Super-Admin payment
	// confirmation (D-030.3, D-031.3). Only the payment path may set it; the
	// HTTP provisioning route never does, so an unowned premium plan cannot be
	// reached from the outside.
	Approved bool
	// Spec, when set, replaces the offer's limits and target for this release
	// (a CUSTOM order).
	Spec *Spec
	// ReplacesPanelEmail names the FREE claim this create is REPLACING (D-112).
	//
	// A replace builds the new VPN before releasing the old one, so at the
	// concurrent cap an ordinary claim would always be refused and a customer
	// holding three FREE VPNs could never replace any of them. Naming the old
	// claim lets the booking swap its slot atomically instead of taking a new
	// one. Empty means an ordinary claim.
	ReplacesPanelEmail string
}

// Result reports what actually happened remotely.
type Result struct {
	VPNID     int64            `json:"vpn_id"`
	Email     string           `json:"panel_email"`
	Status    domain.VPNStatus `json:"status"`
	Confirmed bool             `json:"confirmed"`
	// Recorded is false when the panel rejected the create and the provisional
	// local rows were rolled back (D-024.1a): no VPN exists, so VPNID refers to
	// a record that no longer exists and must not be used.
	Recorded bool `json:"recorded"`
	// TagLocked is true when issuing this VPN locked an existing FREE/PREMIUM
	// tag (D-025.2). An untagged client stays untagged: the Super Admin still
	// has to classify it, and locking a row that does not exist would prevent
	// exactly the first classification the panel-as-truth model needs.
	TagLocked bool `json:"tag_locked"`
}

// Offer is the resolved offer used for provisioning.
type Offer struct {
	ID           int64
	ServerID     int64
	InboundID    int64
	TrafficGB    int64
	DurationDays int
	DeviceLimit  int
	// IPLimit overrides the IP cap for one release (a custom spec). 0 means
	// "use DeviceLimit", which is the normal package behaviour.
	IPLimit int
	Active  bool
	Kind    string
	// OwnerUserID is the reseller that owns this plan (D-030). Nil means the
	// plan is the platform's own inventory and needs a Super-Admin
	// confirmation before it can be released.
	OwnerUserID *int64
	SlotsTotal  int
	SlotsUsed   int
	// ArchivedAt is non-NULL once the plan left the catalogue (D-030.6).
	ArchivedAt sql.NullInt64
	PriceCents int64
	Currency   string
}

// NeedsSuperAdminConfirm reports whether releasing this plan requires a
// Super-Admin decision (D-030.3): an unowned PREMIUM plan does.
func (o *Offer) NeedsSuperAdminConfirm() bool {
	return o != nil && o.Kind == "PREMIUM" && o.OwnerUserID == nil
}

func nullable(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func sellerOrDefault(in CreateInput) int64 {
	if in.SellerID > 0 {
		return in.SellerID
	}
	return in.Actor.UserID
}

// panelInboundID translates a LOCAL `inbounds.id` into the 3X-UI panel's own
// inbound id (the `inbounds.remote_id` column) before anything is sent to the
// panel.
//
// Why this is not optional: `vpn_offers.inbound_id` is a foreign key to
// `inbounds(id)`, which is an AUTOINCREMENT row id local to Veylix — on the
// development database the cached inbounds are rows 52/53/54 while the panel
// itself numbers them 1/2/3 (`cacheInbounds` writes `ib.ID` into `remote_id`).
// Passing the local id straight through therefore asked the panel to attach the
// client to a nonexistent inbound, it answered "inbound not found", and
// `pushCreate` correctly discarded the provisional row and answered 502
// `panel_rejected` — a FREE claim could never succeed, on any offer.
//
// The translation lives here, in the layer that talks to the panel, rather than
// in the offer row: the foreign key is the integrity guarantee (an offer cannot
// reference an inbound that is not cached for its server), and only the panel
// call needs the remote number.
//
// A missing row is ErrOfferIncomplete, not a generic failure: the offer points
// at an inbound Veylix has no record of, which is the same condition as "no
// inbound selected" from the caller's point of view — nothing can be built.
func (s *Service) panelInboundID(ctx context.Context, serverID, inboundID int64) (int64, error) {
	var remoteID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT remote_id FROM inbounds WHERE id = ? AND server_id = ?`,
		inboundID, serverID).Scan(&remoteID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrOfferIncomplete
	}
	if err != nil {
		return 0, fmt.Errorf("provisioning: resolve inbound %d: %w", inboundID, err)
	}
	if remoteID <= 0 {
		// remote_id has no CHECK constraint, so a bad sync could store 0. The
		// panel would reject `inboundIds: [0]`, which is the same silent
		// outage this function exists to prevent.
		return 0, ErrOfferIncomplete
	}
	return remoteID, nil
}

// loadOffer reads an offer.
func (s *Service) loadOffer(ctx context.Context, id int64) (*Offer, error) {
	var o Offer
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(server_id,0), COALESCE(inbound_id,0), traffic_gb, duration_days, device_limit, active, kind,
		        owner_user_id, slots_total, slots_used, archived_at, price_cents, currency
		   FROM vpn_offers WHERE id = ?`, id,
	).Scan(&o.ID, &o.ServerID, &o.InboundID, &o.TrafficGB, &o.DurationDays, &o.DeviceLimit, &o.Active, &o.Kind,
		&o.OwnerUserID, &o.SlotsTotal, &o.SlotsUsed, &o.ArchivedAt, &o.PriceCents, &o.Currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOfferNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("provisioning: load offer: %w", err)
	}
	if o.ServerID == 0 {
		return nil, ErrOfferInactive
	}
	// D-030.6: an archived plan has left the catalogue; it can no longer be
	// released, whatever a stale screen still shows.
	if o.ArchivedAt.Valid {
		return nil, ErrPlanArchived
	}
	// An offer without an inbound cannot produce a working client: the panel is
	// asked to attach the client to inbound id 0, which does not exist, and the
	// write is rejected. Catching it here means nothing is recorded at all
	// (D-024.1a) instead of leaving a VPN row that can never work.
	if o.InboundID == 0 {
		return nil, ErrOfferIncomplete
	}
	return &o, nil
}

// authorise applies the role rules (REQUIREMENTS Â§4 and D-030):
//   - a client may self-provision a FREE offer only, and only for themselves;
//   - a reseller may release a PREMIUM plan **he owns** on a server assigned to
//     him — with no approval step (D-030.2) — or assign within quota;
//   - a super admin may do either, including direct sales;
//   - **nobody** may release an unowned PREMIUM plan here: that requires a
//     Super-Admin confirmation, which is the payment path (D-030.3).
//
// A release on behalf of a confirmation sets `in.Approved = true`; only the
// payment confirmation may do that, and the API never exposes it.
func (s *Service) authorise(ctx context.Context, in CreateInput, o *Offer) error {
	if o.NeedsSuperAdminConfirm() && !in.Approved {
		return ErrPlanNotApproved
	}
	switch in.Actor.Role {
	case auth.RoleClient:
		if o.Kind != "FREE" {
			return ErrNotPermitted
		}
		// A client can only provision for themselves.
		if in.ClientUserID != in.Actor.UserID {
			return ErrNotPermitted
		}
	case auth.RoleAdmin:
		if !in.Actor.Can(auth.PermVPNAssignPremium) {
			return ErrNotPermitted
		}
		// D-030.2: a reseller releases only a plan that is his, and only on a
		// server that was assigned to him. Without this, "his own plan" would be
		// any plan on any panel.
		if o.OwnerUserID != nil && *o.OwnerUserID != in.Actor.UserID {
			return ErrNotPlanOwner
		}
		ok, err := s.actorHasServer(ctx, in.Actor.UserID, o.ServerID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrServerNotAssigned
		}
	case auth.RoleSuperAdmin:
		// allowed
	default:
		return ErrNotPermitted
	}
	return nil
}

// actorHasServer reports whether a reseller may operate on a server.
//
// The assignment table is the only source: a reseller must not be able to
// release a plan on a panel nobody gave him (M6-06).
func (s *Service) actorHasServer(ctx context.Context, userID, serverID int64) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admin_server_assignments WHERE admin_id = ? AND server_id = ?`,
		userID, serverID).Scan(&n); err != nil {
		return false, fmt.Errorf("provisioning: check server assignment: %w", err)
	}
	return n > 0, nil
}

// claimSlot consumes one slot of the plan inside the create transaction
// (D-030.4).
//
// Two concurrent releases against the last slot must produce exactly one VPN;
// the claim is therefore in the same transaction as the VPN row, not before it.
// `buyer` is the reseller whose inventory the slot belongs to (0 when the slot
// is the platform's own), so a purchase can be traced back to a person.
func (s *Service) claimSlot(ctx context.Context, tx *sql.Tx, o *Offer, buyer, vpnID int64) (int64, error) {
	purchaseID, err := s.st.ClaimPlanSlot(ctx, tx, o.ID, buyer, vpnID)
	switch {
	case errors.Is(err, store.ErrPlanExhausted):
		return 0, ErrPlanExhausted
	case err != nil:
		return 0, err
	}
	return purchaseID, nil
}

// bytesPerGB converts an offer's traffic quota to bytes.
const bytesPerGB int64 = 1024 * 1024 * 1024

// Create provisions a VPN for an offer.
//
// Order matters and is deliberate:
//  1. validate offer + permissions (fail before touching anything);
//  2. reserve the quota slot and the local row in ONE transaction;
//  3. enqueue the outbox entry in that same transaction;
//  4. write to the panel;
//  5. confirm with a fresh read (else leave it for reconciliation).
func (s *Service) Create(ctx context.Context, in CreateInput) (*Result, error) {
	offer, err := s.loadOffer(ctx, in.OfferID)
	if err != nil {
		return nil, err
	}
	if !offer.Active {
		return nil, ErrOfferInactive
	}
	// D-035.2 / D-036.6: a CUSTOM order overrides the package's limits AND its
	// target panel. Applied BEFORE the permission check, because
	// server-assignment is decided on the server the VPN will actually live on.
	if in.Spec != nil {
		if in.Spec.TrafficGB <= 0 || in.Spec.ServerID <= 0 || in.Spec.InboundID <= 0 {
			return nil, ErrSpecIncomplete
		}
		offer.TrafficGB = in.Spec.TrafficGB
		offer.DurationDays = in.Spec.DurationDays
		// The panel enforces ONE device cap (`limitHwid`) and one IP cap
		// (`limitIp`); a custom spec states both, so the lower one is used for
		// each and the panel is told exactly what the customer ordered.
		offer.DeviceLimit = in.Spec.Devices
		offer.IPLimit = in.Spec.IPs
		offer.ServerID = in.Spec.ServerID
		offer.InboundID = in.Spec.InboundID
	}
	if err := s.authorise(ctx, in, offer); err != nil {
		return nil, err
	}

	// The offer stores Veylix's own inbound row id; the panel knows its own
	// numbering. Everything that reaches the panel — the outbox payload and the
	// live write — must carry the panel's number, or the panel is asked to
	// attach the client to an inbound that does not exist there.
	//
	// Resolved BEFORE the transaction opens, deliberately. It is a read of a
	// cached row that cannot change while this call runs, so it has no business
	// inside the write transaction; putting it there held SQLite's write lock
	// across two statements and made the SECOND of two concurrent creates fail
	// with SQLITE_BUSY instead of the quota refusal it was meant to report
	// (TestQuotaIsEnforcedUnderConcurrency). A resolution failure is still a
	// clean refusal: nothing has been written yet.
	panelInbound, err := s.panelInboundID(ctx, offer.ServerID, offer.InboundID)
	if err != nil {
		return nil, err
	}

	var (
		vpnID        int64
		panelEmail   string
		serverID     int64
		inboundID    int64
		outboxID     int64
		assignmentID int64
	)

	err = s.st.Tx(ctx, func(tx *sql.Tx) error {
		// Quota is enforced INSIDE the transaction by counting ACTIVE
		// assignments, so two concurrent creators cannot both pass (T-08).
		if in.SellerID > 0 {
			var limit int
			if err := tx.QueryRowContext(ctx,
				`SELECT quota_limit FROM users WHERE id = ?`, in.SellerID).Scan(&limit); err != nil {
				return fmt.Errorf("provisioning: read quota: %w", err)
			}
			if limit > 0 {
				var used int
				if err := tx.QueryRowContext(ctx,
					`SELECT COUNT(*) FROM vpn_assignments WHERE seller_user_id = ? AND status = 'ACTIVE'`,
					in.SellerID).Scan(&used); err != nil {
					return fmt.Errorf("provisioning: count quota: %w", err)
				}
				if used >= limit {
					return ErrQuotaExceeded
				}
			}
		}

		now := time.Now().Unix()
		var expiresAt any
		if offer.DurationDays > 0 {
			expiresAt = time.Now().AddDate(0, 0, offer.DurationDays).Unix()
		}

		// createdWith is the record of what the PANEL was told at create time.
		// For a FREE offer that is the fixed 2/2 policy, not the offer's field
		// (D-028.3), so the local row and the panel never disagree about the
		// device cap a VPN was issued with.
		createdWith := specFor(0, "", offer)

		// Insert first to obtain the local id that forms the panel identity.
		res, err := tx.ExecContext(ctx,
			`INSERT INTO vpn_instances
			   (owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id, status,
			    traffic_limit_bytes, expires_at, device_limit,
			    panel_total_gb, panel_expiry_time, panel_limit_ip, panel_limit_hwid, panel_enable,
			    created_at, updated_at)
			 VALUES (?, ?, ?, ?, '', '', 'PENDING', ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			in.ClientUserID, offer.ServerID, nullable(offer.InboundID), offer.ID,
			offer.TrafficGB*bytesPerGB, expiresAt, createdWith.LimitIP,
			createdWith.TotalGB, createdWith.ExpiryTime, createdWith.LimitIP, createdWith.LimitHwid,
			now, now)
		if err != nil {
			return fmt.Errorf("provisioning: insert instance: %w", err)
		}
		vpnID, _ = res.LastInsertId()

		panelEmail, err = s.nextPanelEmail(ctx, tx, in.ClientUserID, offer.ServerID, panelKind(offer))
		if err != nil {
			return err
		}
		serverID = offer.ServerID
		inboundID = offer.InboundID

		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_instances SET panel_email = ?, panel_sub_id = ? WHERE id = ?`,
			panelEmail, domain.PanelSubID(vpnID), vpnID); err != nil {
			return fmt.Errorf("provisioning: set identity: %w", err)
		}

		// D-030.4: this VPN consumes one slot of the plan. Inside the same
		// transaction as the row above, so two concurrent releases against the
		// last slot cannot both succeed. An unlimited plan (slots_total = 0)
		// claims nothing.
		buyer := int64(0)
		if offer.OwnerUserID != nil {
			buyer = *offer.OwnerUserID
		}
		if _, err := s.claimSlot(ctx, tx, offer, buyer, vpnID); err != nil {
			return err
		}

		// D-025.2: issuing the VPN freezes its tag. Done in the SAME transaction
		// as the insert, so there is no window in which an issued VPN still has
		// an unlocked tag that a concurrent re-tag could flip. The UPDATE is
		// conditional on locked_at IS NULL and simply matches zero rows for an
		// untagged client — that is not an error, it means "nothing to lock".
		if _, err := tx.ExecContext(ctx,
			`UPDATE panel_client_tags SET locked_at = ?
			  WHERE server_id = ? AND panel_email = ? AND locked_at IS NULL`,
			now, serverID, panelEmail); err != nil {
			return fmt.Errorf("provisioning: lock tag: %w", err)
		}

		res, err = tx.ExecContext(ctx,
			`INSERT INTO vpn_assignments
			   (vpn_instance_id, client_user_id, seller_user_id, status, started_at, expires_at, created_at, updated_at)
			 VALUES (?, ?, ?, 'ACTIVE', ?, ?, ?, ?)`,
			vpnID, in.ClientUserID, sellerOrDefault(in), now, expiresAt, now, now)
		if err != nil {
			return fmt.Errorf("provisioning: insert assignment: %w", err)
		}
		assignmentID, _ = res.LastInsertId()

		if in.SellerID > 0 {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO quota_ledger (admin_id, delta, reason, assignment_id, created_at)
				 VALUES (?, 1, 'ASSIGN', ?, ?)`, in.SellerID, assignmentID, now); err != nil {
				return fmt.Errorf("provisioning: quota ledger: %w", err)
			}
		}

		outboxID, err = s.st.EnqueueOutbox(ctx, tx, store.OpCreateClient, serverID,
			domain.NewOutboxKey(store.OpCreateClient, serverID, panelEmail),
			encodePayload(buildClientRequest(vpnID, panelEmail, offer, panelInbound)))
		if err != nil {
			return err
		}

		// D-025.4/.5: a FREE claim is booked in the SAME transaction as the VPN
		// row, so the two caps (3 concurrent, 5 per rolling 30 days) are checked
		// and consumed atomically. Checking outside would let two concurrent
		// claims both observe the last slot and both take it.
		//
		// D-112: a replace swaps the claim it is replacing, in the same
		// transaction, because the new VPN is created before the old one is
		// released — at the concurrent cap an ordinary claim would always fail
		// and the customer could never replace. The monthly window is still
		// consumed either way (§17.18: releasing does not refund a claim).
		if offer.Kind == "FREE" {
			var claimID int64
			var cerr error
			if in.ReplacesPanelEmail != "" {
				claimID, cerr = s.st.ReplaceFreeClaim(ctx, tx, in.ClientUserID, serverID,
					in.ReplacesPanelEmail, panelEmail, in.FreeMonthlyLimit)
			} else {
				claimID, cerr = s.st.RecordFreeClaim(ctx, tx, in.ClientUserID, serverID,
					panelEmail, in.FreeMonthlyLimit)
			}
			if cerr != nil {
				return cerr // rolls the whole create back: no VPN without a claim
			}
			if _, uerr := tx.ExecContext(ctx,
				`UPDATE free_claims SET vpn_instance_id = ? WHERE id = ?`, vpnID, claimID); uerr != nil {
				return fmt.Errorf("provisioning: link free claim: %w", uerr)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.pushCreate(ctx, in, offer, vpnID, panelEmail, serverID, inboundID, panelInbound, outboxID), nil
}

// pushCreate performs the remote write and reconciles the outcome.
//
// inboundID is Veylix's own `inbounds.id` (what the VPN row records);
// panelInboundID is the panel's numbering, which is the only one the panel
// accepts. They are passed separately on purpose: recording the local id keeps
// the foreign key meaningful, while sending the remote id is what makes the
// write succeed.
func (s *Service) pushCreate(ctx context.Context, in CreateInput, offer *Offer,
	vpnID int64, panelEmail string, serverID, inboundID, panelInboundID, outboxID int64) *Result {

	result := &Result{VPNID: vpnID, Email: panelEmail, Status: domain.VPNPending, Recorded: true}

	client, err := s.factory(ctx, serverID)
	if err != nil {
		// Cannot even build a client (server disabled/unreachable). The outbox
		// keeps the work and the VPN reads as unavailable — never revoked.
		s.markOutboxRetry(ctx, outboxID, "panel unavailable", err)
		s.setStatus(ctx, vpnID, domain.VPNServerUnavailable)
		result.Status = domain.VPNServerUnavailable
		s.auditProvision(ctx, in, vpnID, result)
		return result
	}

	addErr := client.AddClient(ctx, buildClientRequest(vpnID, panelEmail, offer, panelInboundID))

	// Confirm with a fresh read: success is never assumed (new_update.md Â§5).
	exists, readErr := client.ClientExists(ctx, panelEmail)

	switch {
	case readErr == nil && exists:
		// Covers "write succeeded" AND "write reported an error but landed".
		s.markOutboxApplied(ctx, outboxID)
		s.setStatus(ctx, vpnID, domain.VPNActive)
		result.Status = domain.VPNActive
		result.Confirmed = true
		// The panel confirmed the client, so the tag locked in the create
		// transaction is now permanent (D-025.2). Report whether one existed.
		result.TagLocked = s.tagWasLocked(ctx, serverID, panelEmail)

	case readErr != nil:
		// Outcome unknown → keep it queued; claim neither success nor failure.
		s.markOutboxRetry(ctx, outboxID, "write outcome unknown", readErr)
		s.setStatus(ctx, vpnID, domain.VPNServerUnavailable)
		result.Status = domain.VPNServerUnavailable

	default:
		// A definite rejection: the client is positively absent.
		//
		// D-024.1a: a VPN that the panel refused does not exist, so the local
		// record must not survive. Previously this stored REVOCATION_PENDING —
		// a state that claims a client existed and was deleted — leaving a
		// phantom VPN in every list. The row, its assignment and its quota slot
		// are rolled back here, and the outbox keeps the failed attempt for
		// diagnosis.
		s.markOutboxFailed(ctx, outboxID, "panel rejected the client", addErr)
		s.discardProvisional(ctx, serverID, panelEmail, vpnID)
		result.Recorded = false
		result.Status = domain.VPNPending
		s.auditProvision(ctx, in, vpnID, result)
		return result
	}

	s.auditProvision(ctx, in, vpnID, result)
	return result
}

// discardProvisional rolls back the local rows written for a create the panel
// rejected, so no VPN exists locally that does not exist remotely (D-024.1a).
//
// The assignment RESTRICTs the VPN delete, so it goes first; the quota slot it
// consumed is released in the same transaction so a failed create cannot eat a
// reseller's capacity.
//
// The tag lock is released for the same reason: the create transaction froze
// the tag, but the VPN it was frozen for never existed. Leaving it locked would
// make a refused create permanently unclassifiable — a locked tag for a client
// no one ever received.
func (s *Service) discardProvisional(ctx context.Context, serverID int64, panelEmail string, vpnID int64) {
	_ = s.st.Tx(ctx, func(tx *sql.Tx) error {
		var assignmentID, sellerID int64
		err := tx.QueryRowContext(ctx,
			`SELECT id, seller_user_id FROM vpn_assignments WHERE vpn_instance_id = ?`,
			vpnID).Scan(&assignmentID, &sellerID)
		if err == nil && assignmentID > 0 {
			// Release only a slot that was actually charged: the create path
			// writes a quota_ledger row iff a reseller was involved, so the
			// rollback must use the same condition or it would refund quota the
			// seller never spent.
			var charged int
			if qerr := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM quota_ledger WHERE assignment_id = ? AND reason = 'ASSIGN'`,
				assignmentID).Scan(&charged); qerr == nil && charged > 0 {
				if _, qerr := tx.ExecContext(ctx,
					`INSERT INTO quota_ledger (admin_id, delta, reason, assignment_id, created_at)
					 VALUES (?, -1, 'RELEASE', ?, ?)`,
					sellerID, assignmentID, time.Now().Unix()); qerr != nil {
					return qerr
				}
			}
		}
		if _, derr := tx.ExecContext(ctx,
			`DELETE FROM vpn_assignments WHERE vpn_instance_id = ?`, vpnID); derr != nil {
			return derr
		}
		// A refused create must not consume a FREE claim: the client never got
		// the VPN, so the slot must come back.
		//
		// The row is DELETED, not marked released, and that does not weaken
		// D-025.5 ("releasing does not refund a claim"): that rule protects the
		// monthly cap from being farmed by claiming and releasing a VPN the
		// client actually held. Here the VPN was never issued — the panel
		// refused it — so no claim ever happened. Only the row written by THIS
		// attempt is removed, matched by vpn_instance_id.
		if _, derr := tx.ExecContext(ctx,
			`DELETE FROM free_claims WHERE vpn_instance_id = ? AND released_at IS NULL`,
			vpnID); derr != nil {
			return derr
		}
		// Unlock only the lock this create wrote. A tag that was already locked
		// by an earlier issue keeps its original locked_at, because the create
		// path's conditional UPDATE cannot have touched it.
		if _, derr := tx.ExecContext(ctx,
			`UPDATE panel_client_tags SET locked_at = NULL
			  WHERE server_id = ? AND panel_email = ?`, serverID, panelEmail); derr != nil {
			return derr
		}
		_, derr := tx.ExecContext(ctx, `DELETE FROM vpn_instances WHERE id = ?`, vpnID)
		return derr
	})
}

// tagWasLocked reports whether a tag row exists for the client. It is read after
// a confirmed create, so it answers "was there a FREE/PREMIUM classification to
// freeze?" — a read failure is reported as false rather than failing the create,
// because the VPN itself is already confirmed and correct.
func (s *Service) tagWasLocked(ctx context.Context, serverID int64, panelEmail string) bool {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM panel_client_tags WHERE server_id = ? AND panel_email = ?`,
		serverID, panelEmail).Scan(&n)
	return err == nil && n > 0
}

func (s *Service) auditProvision(ctx context.Context, in CreateInput, vpnID int64, r *Result) {
	res := "ERROR"
	if r.Confirmed {
		res = "OK"
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: in.Actor.UserID, Action: audit.ActionProvision, TargetType: "vpn",
		TargetID: fmt.Sprint(vpnID), Result: res,
		Context: map[string]any{"offer_id": in.OfferID, "status": string(r.Status)},
	})
}

// --- outbox state helpers ------------------------------------------------

func (s *Service) markOutboxApplied(ctx context.Context, id int64) {
	_ = s.st.MarkOutbox(ctx, nil, id, store.OutboxApplied, "", nil)
}

func (s *Service) markOutboxRetry(ctx context.Context, id int64, msg string, cause error) {
	if cause != nil {
		msg = msg + ": " + sanitiseErr(cause)
	}
	next := time.Now().Add(backoff(1)).Unix()
	_ = s.st.MarkOutbox(ctx, nil, id, store.OutboxRetry, msg, &next)
}

func (s *Service) markOutboxFailed(ctx context.Context, id int64, msg string, cause error) {
	if cause != nil {
		msg = msg + ": " + sanitiseErr(cause)
	}
	_ = s.st.MarkOutbox(ctx, nil, id, store.OutboxFailed, msg, nil)
}

// backoff returns an exponential delay with a ceiling, so a broken panel is
// retried politely rather than hammered (new_update.md Â§8).
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<uint(attempt)) * 10 * time.Second
}

// setStatus writes a VPN status, refusing illegal transitions so the state
// machine in domain is the only authority.
func (s *Service) setStatus(ctx context.Context, vpnID int64, to domain.VPNStatus) {
	var current string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM vpn_instances WHERE id = ?`, vpnID).
		Scan(&current); err != nil {
		return
	}
	if !domain.CanTransition(domain.VPNStatus(current), to) && domain.VPNStatus(current) != to {
		// An illegal transition is a bug, not a user error: keep the old state
		// rather than writing something inconsistent.
		return
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE vpn_instances SET status = ?, updated_at = ? WHERE id = ?`,
		string(to), time.Now().Unix(), vpnID)
}

// BlockVPN stops a VPN for an order/payment reason (D-036.5).
//
// It is the same panel call as Suspend, but it takes no principal: the caller
// has already decided the operator may act (the order route checks
// `payment:confirm`), and this method exists so the panel write and the local
// state change stay in one place. The PANEL is written first and confirmed with
// a fresh read, so "blocked" is never a local claim the panel disagrees with.
func (s *Service) BlockVPN(ctx context.Context, id int64) error {
	return s.setEnabled(ctx, id, false)
}

// UnblockVPN restarts a VPN whose payment issue was resolved.
func (s *Service) UnblockVPN(ctx context.Context, id int64) error {
	return s.setEnabled(ctx, id, true)
}

// setEnabled performs one reversible enable/disable against the panel, then
// records it locally.
func (s *Service) setEnabled(ctx context.Context, id int64, enable bool) error {
	var (
		serverID   int64
		panelEmail string
		status     string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT server_id, panel_email, status FROM vpn_instances WHERE id = ?`, id).
		Scan(&serverID, &panelEmail, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("provisioning: load vpn: %w", err)
	}
	if panelEmail == "" {
		return ErrPanelFailed
	}

	client, err := s.factory(ctx, serverID)
	if err != nil {
		return ErrPanelFailed
	}
	if err := client.SetClientEnabled(ctx, panelEmail, enable); err != nil {
		return ErrPanelFailed
	}
	// Confirm the panel actually changed, instead of trusting the response.
	exists, readErr := client.ClientExists(ctx, panelEmail)
	if readErr != nil || !exists {
		return ErrPanelFailed
	}

	target := domain.VPNActive
	if !enable {
		target = domain.VPNSuspended
	}
	s.setStatus(ctx, id, target)
	return nil
}

// sanitiseErr keeps error text short and free of client identifiers.
func sanitiseErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

// vpnRow is the ownership/identity data needed for lifecycle operations.
type vpnRow struct {
	ID         int64
	OwnerID    int64
	SellerID   int64
	ServerID   int64
	PanelEmail string
	Status     domain.VPNStatus
}

// loadVPN returns a VPN if the principal may act on it.
//
// A resource the caller cannot see returns ErrNotFound — not 403 — so object
// ids cannot be probed (auth.ErrNotFound rationale).
func (s *Service) loadVPN(ctx context.Context, id int64, p auth.Principal, mutate bool) (*vpnRow, error) {
	var v vpnRow
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT i.id, i.owner_user_id, COALESCE(a.seller_user_id, 0), i.server_id, i.panel_email, i.status
		   FROM vpn_instances i
		   LEFT JOIN vpn_assignments a ON a.vpn_instance_id = i.id
		  WHERE i.id = ?`, id,
	).Scan(&v.ID, &v.OwnerID, &v.SellerID, &v.ServerID, &v.PanelEmail, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("provisioning: load vpn: %w", err)
	}
	v.Status = domain.VPNStatus(status)

	owner := auth.Ownership{OwnerUserID: v.OwnerID, SellerUserID: v.SellerID, ServerID: v.ServerID}
	allowed := p.CanViewResource(owner)
	if mutate {
		allowed = p.CanMutateResource(owner)
	}
	if !allowed {
		return nil, ErrNotFound
	}
	return &v, nil
}

// Suspend disables the panel client and marks the VPN SUSPENDED.
// Suspension is reversible by design (never a delete).
func (s *Service) Suspend(ctx context.Context, id int64, p auth.Principal) error {
	v, err := s.loadVPN(ctx, id, p, true)
	if err != nil {
		return err
	}
	client, err := s.factory(ctx, v.ServerID)
	if err != nil {
		return ErrNotPermitted
	}
	if err := client.SetClientEnabled(ctx, v.PanelEmail, false); err != nil {
		// Not confirmed → do not claim the new state.
		// The failure is the PANEL's, not the caller's: reporting ErrNotFound here
		// answered 404 "vpn not found" for a VPN that plainly exists (the row was
		// loaded a few lines up), which sent the operator looking for a deleted
		// record instead of an unreachable panel. Resume already answered
		// ErrPanelFailed for the same condition, so suspend must agree.
		return fmt.Errorf("%w: %v", ErrPanelFailed, sanitiseErr(err))
	}
	s.setStatus(ctx, id, domain.VPNSuspended)
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: p.UserID, Action: audit.ActionSuspend, TargetType: "vpn", TargetID: fmt.Sprint(id),
	})
	return nil
}

// Resume re-enables a suspended panel client.
func (s *Service) Resume(ctx context.Context, id int64, p auth.Principal) error {
	v, err := s.loadVPN(ctx, id, p, true)
	if err != nil {
		return err
	}
	client, err := s.factory(ctx, v.ServerID)
	if err != nil {
		return ErrNotPermitted
	}
	if err := client.SetClientEnabled(ctx, v.PanelEmail, true); err != nil {
		return fmt.Errorf("%w: %v", ErrPanelFailed, sanitiseErr(err))
	}
	s.setStatus(ctx, id, domain.VPNActive)
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: p.UserID, Action: audit.ActionResume, TargetType: "vpn", TargetID: fmt.Sprint(id),
	})
	return nil
}

// Dormant puts a VPN to sleep from the customer's point of view (D-094).
//
// WHAT IT DOES NOT DO — and this is the whole design
//
//   - no panel call. The 3X-UI client is not disabled, not removed, not touched.
//   - no status change. `status` keeps reporting what the panel says, so a
//     dormant VPN cannot claim to be ACTIVE when it is EXHAUSTED, or vice versa.
//   - no deletion and no release window. Unlike Release (D-027) there is nothing
//     to undo, because nothing was removed.
//
// What it changes is entirely local and entirely about the customer's attention:
// the VPN sorts last, stops counting toward "Needs attention", and its Usage
// block stops polling.
//
// Ownership is enforced by loadVPN: a VPN the caller cannot see answers
// ErrNotFound, so ids cannot be probed. `mutate=true` is honest even though no
// remote state moves — this is still a state-changing request worth the same
// permission checks as any other.
//
// Idempotent: putting an already-dormant VPN to sleep does not move its original
// timestamp. "Asleep since" is a fact about the first time, and overwriting it on
// a double-click would quietly reset it.
func (s *Service) Dormant(ctx context.Context, id int64, p auth.Principal) error {
	v, err := s.loadVPN(ctx, id, p, true)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE vpn_instances SET dormant_at = ?, updated_at = ?
		  WHERE id = ? AND dormant_at IS NULL`, now, now, v.ID); err != nil {
		return fmt.Errorf("provisioning: put vpn to sleep: %w", err)
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: p.UserID, Action: audit.ActionVPNDormant, TargetType: "vpn", TargetID: fmt.Sprint(id),
		Context: map[string]any{
			// Stated explicitly so a later reader cannot mistake this for a panel
			// action or a deletion.
			"panel_client": "untouched",
			"deleted":      false,
		},
	})
	return nil
}

// Wake brings a dormant VPN back into view. It clears the flag and nothing else:
// there is no panel call to make, because Dormant never made one.
//
// It does NOT force the VPN's status back to ACTIVE. If the panel still cannot
// see the client, the honest state is SERVER_UNAVAILABLE and the customer is told
// so — waking a VPN does not make it work, it only makes it visible again.
func (s *Service) Wake(ctx context.Context, id int64, p auth.Principal) error {
	v, err := s.loadVPN(ctx, id, p, true)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE vpn_instances SET dormant_at = NULL, updated_at = ?
		  WHERE id = ? AND dormant_at IS NOT NULL`, now, v.ID); err != nil {
		return fmt.Errorf("provisioning: wake vpn: %w", err)
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: p.UserID, Action: audit.ActionVPNWake, TargetType: "vpn", TargetID: fmt.Sprint(id),
	})
	return nil
}

func (s *Service) Revoke(ctx context.Context, id int64, p auth.Principal) (domain.VPNStatus, error) {
	v, err := s.loadVPN(ctx, id, p, true)
	if err != nil {
		return "", err
	}

	key := domain.NewOutboxKey(store.OpDeleteClient, v.ServerID, v.PanelEmail)
	outboxID, err := s.st.EnqueueOutbox(ctx, nil, store.OpDeleteClient, v.ServerID, key, "{}")
	if err != nil {
		return "", err
	}
	s.setStatus(ctx, id, domain.VPNRevocationPending)

	client, cerr := s.factory(ctx, v.ServerID)
	if cerr != nil {
		s.markOutboxRetry(ctx, outboxID, "panel unavailable", cerr)
		return domain.VPNRevocationPending, nil
	}

	delErr := client.DeleteClient(ctx, v.PanelEmail)
	exists, readErr := client.ClientExists(ctx, v.PanelEmail)

	switch {
	case readErr == nil && !exists:
		// Verified gone — the only path to REVOKED.
		s.markOutboxApplied(ctx, outboxID)
		s.setStatus(ctx, id, domain.VPNRevoked)
		s.closeAssignment(ctx, id)
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID: p.UserID, Action: audit.ActionRevoke, TargetType: "vpn", TargetID: fmt.Sprint(id),
		})
		return domain.VPNRevoked, nil

	case readErr != nil:
		s.markOutboxRetry(ctx, outboxID, "revocation outcome unknown", readErr)
		return domain.VPNRevocationPending, nil

	default:
		// The client still exists: report pending, queue a retry.
		s.markOutboxRetry(ctx, outboxID, "client still present", delErr)
		return domain.VPNRevocationPending, nil
	}
}

// VerifyPanel asks the panel whether this VPN's client still exists and
// reconciles the local status with the answer.
//
// This is the on-demand counterpart to sync's periodic check (D-024.5): an
// operator can ask "is this VPN real?" without waiting for the next tick.
// Nothing is written to the panel.
func (s *Service) VerifyPanel(ctx context.Context, id int64, p auth.Principal) (*VerifyResult, error) {
	v, err := s.loadVPN(ctx, id, p, true)
	if err != nil {
		return nil, err
	}
	client, err := s.factory(ctx, v.ServerID)
	if err != nil {
		// Cannot reach the panel: that is not evidence of anything.
		return nil, ErrPanelFailed
	}
	exists, err := client.ClientExists(ctx, v.PanelEmail)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPanelFailed, sanitiseErr(err))
	}

	out := &VerifyResult{Exists: exists, Status: domain.VPNStatus(v.Status)}
	if exists {
		out.Status = domain.VPNStatus(v.Status)
		return out, nil
	}
	// Absent: SERVER_UNAVAILABLE, never REVOKED (we did not delete it).
	if domain.CanTransition(domain.VPNStatus(v.Status), domain.VPNServerUnavailable) {
		s.setStatus(ctx, id, domain.VPNServerUnavailable)
		out.Status = domain.VPNServerUnavailable
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID: p.UserID, Action: audit.ActionVPNAbsent, TargetType: "vpn",
			TargetID: fmt.Sprint(id),
			Context:  map[string]any{"panel_email": v.PanelEmail, "verdict": "absent"},
		})
	}
	return out, nil
}

// RemoveAbsent deletes Veylix's record of a VPN whose panel client is
// positively gone. The panel is NOT touched (D-027.3).
//
// It refuses unless the panel confirms the client is absent, so a working VPN
// can never be dropped by accident; a supported VPN should be revoked instead.
func (s *Service) RemoveAbsent(ctx context.Context, id int64, p auth.Principal) error {
	return s.removeRecordAs(ctx, id, p, "", false)
}

// ErrPurgeNeedsConfirmation refuses a destructive purge that did not repeat the
// VPN's panel identity back.
//
// P-3: the Super Admin could not remove a VPN record at all while the panel still
// held the client — `ErrVPNStillOnPanel`. That is the right default, because
// deleting the record orphans a working client and hides it from its owner. But it
// left no path for the case that actually happens: a record whose panel client must
// be KEPT (D-027.3: Veylix never deletes a panel client) while the local row is
// retired, e.g. after the client was re-created under a new slot, or after an
// operator deliberately handed the panel client to somebody else.
//
// A force purge therefore exists, and it is gated on the operator typing the panel
// email — the VPN's identity — so it cannot be triggered by a mis-clicked row.
type PurgeConfirmError struct {
	Want string
}

func (e *PurgeConfirmError) Error() string {
	return "provisioning: confirm the panel identity to purge this record"
}

// RemoveRecord is RemoveAbsent with an optional operator override.
//
// `confirmPanelEmail` empty (or the non-force path) keeps the original rule
// exactly: the panel must positively report the client absent.
//
// With `force` set, the panel client is NOT deleted — D-027.3 is unconditional —
// and the local record may go even while the client lives. What changes is who is
// warned: the operator must repeat `confirmPanelEmail`, the record is audited as
// `vpn.force_purge`, and the panel client is left in place so the customer's
// connection keeps working until something else removes it.
func (s *Service) RemoveRecord(ctx context.Context, id int64, p auth.Principal, confirmPanelEmail string) error {
	return s.removeRecordAs(ctx, id, p, confirmPanelEmail, false)
}

// ForcePurgeRecord is the explicit Super Admin path (P-3).
func (s *Service) ForcePurgeRecord(ctx context.Context, id int64, p auth.Principal, confirmPanelEmail string) error {
	if !p.IsSuperAdmin() {
		return ErrNotPermitted
	}
	return s.removeRecordAs(ctx, id, p, confirmPanelEmail, true)
}

func (s *Service) removeRecordAs(ctx context.Context, id int64, p auth.Principal, confirmPanelEmail string, force bool) error {
	v, err := s.loadVPN(ctx, id, p, true)
	if err != nil {
		return err
	}
	// Only an administrator may erase a record.
	if p.IsClient() {
		return ErrNotPermitted
	}

	// A forced purge must prove the operator is looking at THIS VPN. The panel
	// email is its identity (D-091.4), so it is the natural thing to repeat back.
	if force {
		if strings.TrimSpace(confirmPanelEmail) == "" {
			return &PurgeConfirmError{Want: v.PanelEmail}
		}
		if !strings.EqualFold(strings.TrimSpace(confirmPanelEmail), v.PanelEmail) {
			return &PurgeConfirmError{Want: v.PanelEmail}
		}
	}

	client, err := s.factory(ctx, v.ServerID)
	if err != nil {
		if !force {
			return ErrPanelFailed
		}
		// A forced purge may proceed when the panel is unreachable: the record is
		// local, the panel client is left alone either way, and refusing would make
		// a dead integration block the owner from cleaning up.
		client = nil
	}

	if client != nil {
		exists, existErr := client.ClientExists(ctx, v.PanelEmail)
		if existErr != nil && !force {
			return fmt.Errorf("%w: %v", ErrPanelFailed, sanitiseErr(existErr))
		}
		if existErr == nil && exists && !force {
			// The VPN is alive on the panel: removing the record would orphan a
			// working client and hide it from its owner. Refuse (P-3 gives the
			// Super Admin ForcePurgeRecord instead).
			return ErrVPNStillOnPanel
		}
	}

	if err := s.st.Tx(ctx, func(tx *sql.Tx) error {
		// Assignments RESTRICT the delete, so they go first. The VPN row itself
		// carries no panel state that matters now — the panel never had it.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM quota_ledger WHERE assignment_id IN
			   (SELECT id FROM vpn_assignments WHERE vpn_instance_id = ?)`, id); err != nil {
			return fmt.Errorf("provisioning: clear quota ledger: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM vpn_assignments WHERE vpn_instance_id = ?`, id); err != nil {
			return fmt.Errorf("provisioning: clear assignments: %w", err)
		}
		// usage_snapshots cascades on vpn_instances.
		if _, err := tx.ExecContext(ctx, `DELETE FROM vpn_instances WHERE id = ?`, id); err != nil {
			return fmt.Errorf("provisioning: delete vpn: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	// A forced purge is a different event and says so: the panel client is
	// deliberately ALIVE (P-3, D-027.3), and the reason is operator-supplied.
	action := audit.ActionVPNRemoveAbsent
	ctxMap := map[string]any{
		"panel_email":       v.PanelEmail,
		"panel_client_kept": true,
		"reason":            "panel client verified absent",
	}
	if force {
		action = audit.ActionVPNForcePurge
		ctxMap = map[string]any{
			"panel_email":       v.PanelEmail,
			"panel_client_kept": true,
			"forced":            true,
			"reason":            "operator purged the record; the panel client was left in place",
		}
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: p.UserID, Action: action, TargetType: "vpn",
		TargetID: fmt.Sprint(id),
		Context:  ctxMap,
	})
	return nil
}

// closeAssignment releases the reseller slot once the client is verified gone.
func (s *Service) closeAssignment(ctx context.Context, vpnID int64) {
	_ = s.st.Tx(ctx, func(tx *sql.Tx) error {
		var assignmentID, sellerID int64
		err := tx.QueryRowContext(ctx,
			`SELECT id, seller_user_id FROM vpn_assignments WHERE vpn_instance_id = ?`, vpnID,
		).Scan(&assignmentID, &sellerID)
		if err != nil {
			return nil // nothing to close
		}
		now := time.Now().Unix()
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_assignments SET status = 'REVOKED', updated_at = ? WHERE id = ?`, now, assignmentID); err != nil {
			return err
		}
		if sellerID > 0 {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO quota_ledger (admin_id, delta, reason, assignment_id, created_at)
				 VALUES (?, -1, 'REVOKE', ?, ?)`, sellerID, assignmentID, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// ConnectionInfo is what a client needs to actually USE their VPN (D-026).
//
// `Links` are the panel's own URIs, passed through untouched: rewriting a
// vless:// string would produce something that looks right and cannot connect.
// An empty list is meaningful — it means the panel has no link to give (the
// client is disabled, or the inbound is not link-capable), and the UI must say
// so rather than inventing one.
type ConnectionInfo struct {
	VPNID      int64    `json:"vpn_id"`
	PanelEmail string   `json:"panel_email"`
	Status     string   `json:"status"`
	Links      []string `json:"links"`
	// Source is "panel" when the panel answered, "unavailable" when it could
	// not be reached. A UI must not present an outage as "no links exist".
	Source string `json:"source"`
}

// LinkSource values.
const (
	LinkSourcePanel       = "panel"
	LinkSourceUnavailable = "unavailable"
)

// Links returns the connection URIs of a VPN the caller may see.
//
// Ownership is checked by loadVPN (a resource you cannot see is ErrNotFound, so
// ids cannot be probed). The links come from the panel every time: caching a
// credential would mean serving a stale URI after the panel rotates it.
func (s *Service) Links(ctx context.Context, id int64, p auth.Principal) (*ConnectionInfo, error) {
	// mutate=false: reading a link is not a lifecycle change.
	v, err := s.loadVPN(ctx, id, p, false)
	if err != nil {
		return nil, err
	}
	info := &ConnectionInfo{
		VPNID: v.ID, PanelEmail: v.PanelEmail, Status: string(v.Status),
		Links: []string{}, Source: LinkSourcePanel,
	}
	client, err := s.factory(ctx, v.ServerID)
	if err != nil {
		info.Source = LinkSourceUnavailable
		return info, nil
	}
	links, err := client.Links(ctx, v.PanelEmail)
	if err != nil {
		// Same rule as a failed health check: an unreachable panel is not
		// evidence that the VPN has no links.
		info.Source = LinkSourceUnavailable
		return info, nil
	}
	if links != nil {
		info.Links = links
	}
	return info, nil
}

// ProtocolDetail is P-09: what the customer is actually connecting with.
//
// The owner asked for "vless vmess, protocol eka mokadda tcp ws, user name
// password" — the shape of the credential, not just a URL. Every field comes
// from the panel or from Veylix's record of the inbound it attached to; nothing
// here is derived or guessed (D-038).
//
// Security: this is credential material. It is returned only to a caller that
// may already read the VPN (the same ownership check as the detail page), and it
// is never logged.
type ProtocolDetail struct {
	VPNID      int64  `json:"vpn_id"`
	PanelEmail string `json:"panel_email"`
	// Protocol / Transport / Security come from the INBOUND (vless, vmess, …),
	// because the protocol is a property of the inbound, not of the client.
	Protocol  string `json:"protocol"`
	Transport string `json:"transport"`
	Security  string `json:"security"`
	// Server is host:port, taken from the panel URL and the inbound port.
	Server string `json:"server"`
	// Credential material. Which field is populated depends on the protocol:
	// vless/vmess use UUID, trojan/shadowsocks use a password.
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	Flow     string `json:"flow,omitempty"`
	SubID    string `json:"sub_id,omitempty"`
	// Remark is the inbound's own label, so the customer recognises it.
	Remark string `json:"remark,omitempty"`
	// Source is "panel" when the panel answered, "unavailable" when it could not
	// be read. A screen must not present an outage as "no credential exists".
	Source string `json:"source"`
}

// Protocol returns the credential shape of one VPN (D-038 P-09).
//
// Sources, in order of authority:
//   - the CLIENT record from the panel gives uuid / password / flow — the
//     credential itself;
//   - Veylix's own `inbounds` row gives protocol / port / remark — the protocol
//     is a property of the inbound, and Veylix mirrors it during sync;
//   - the panel URL gives the host.
//
// An unreadable panel is reported as `source: "unavailable"`, never as "this VPN
// has no credential": the customer's VPN is fine, our read failed.
func (s *Service) Protocol(ctx context.Context, id int64, p auth.Principal) (*ProtocolDetail, error) {
	v, err := s.loadVPN(ctx, id, p, false)
	if err != nil {
		return nil, err
	}
	out := &ProtocolDetail{
		VPNID:      v.ID,
		PanelEmail: v.PanelEmail,
		SubID:      domain.PanelSubID(v.ID),
		Source:     LinkSourceUnavailable,
	}

	// The inbound Veylix attached the client to, and the panel host.
	//
	// Every joined column is COALESCEd, and `port` is read through sql.NullInt64.
	// A LEFT JOIN produces NULLs for an instance with no inbound (an ASSIGNED panel
	// client is recorded with `inbound_id = 0`, D-063), and scanning a NULL into a
	// plain `int` fails the whole row — which surfaced as `GET /api/vpns/{id}/protocol
	// → 500` on the very detail page a customer opens. A missing inbound is a fact
	// to report, not an error to raise: the protocol section simply has less to say.
	var (
		protocol, remark, panelURL string
		port                       sql.NullInt64
	)
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(ib.protocol,''), ib.port, COALESCE(ib.remark,''),
		        COALESCE(si.panel_url,'')
		   FROM vpn_instances i
		   LEFT JOIN inbounds ib          ON ib.id = i.inbound_id
		   LEFT JOIN server_integrations si ON si.id = i.server_id
		  WHERE i.id = ?`, id).Scan(&protocol, &port, &remark, &panelURL)
	if err != nil {
		return nil, fmt.Errorf("provisioning: read protocol facts: %w", err)
	}
	out.Protocol = protocol
	out.Remark = remark
	out.Server = hostPort(panelURL, int(port.Int64))

	// Panel transport/security live in the inbound's streamSettings, which this
	// adapter does not decode (D-024: whole-config reads are deliberately out of
	// scope). Leave them empty rather than inventing "tcp".
	client, ferr := s.factory(ctx, v.ServerID)
	if ferr != nil {
		return out, nil
	}
	full, ok := client.(interface {
		GetClient(ctx context.Context, email string) (*panel.PanelClient, error)
	})
	if !ok {
		return out, nil
	}
	rec, gerr := full.GetClient(ctx, v.PanelEmail)
	if gerr != nil || rec == nil {
		return out, nil
	}
	out.UUID = rec.UUID
	out.Password = rec.Password
	out.Flow = rec.Flow
	if out.Security == "" {
		out.Security = rec.Security
	}
	out.Source = LinkSourcePanel
	return out, nil
}

// hostPort joins a panel URL's host with an inbound's port.
//
// The panel URL may carry a dev-only port (2053) that is not the inbound's; the
// inbound port is what a client connects to, so it wins. A URL that will not
// parse yields an empty string rather than a misleading "https://".
func hostPort(panelURL string, port int) string {
	if panelURL == "" {
		return ""
	}
	u, err := url.Parse(panelURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	if port <= 0 {
		return u.Hostname()
	}
	return net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
}

// LiveTraffic is P-04: how fast this VPN is moving right now.
//
// A rate can only come from two observations, so it is computed from the last
// two usage snapshots the sync wrote. It is therefore "as of the last sync",
// which the response states — the panel has no per-client live-rate endpoint, and
// inventing one would be a lie (D-038, D-026.2).
type LiveTraffic struct {
	VPNID int64 `json:"vpn_id"`
	// UpBps / DownBps are BYTES PER SECOND between the last two snapshots.
	UpBps   int64 `json:"up_bps"`
	DownBps int64 `json:"down_bps"`
	// MeasuredFrom / MeasuredTo bracket the two readings.
	MeasuredFrom int64 `json:"measured_from"`
	MeasuredTo   int64 `json:"measured_to"`
	// ConnectedNow is the panel's online answer when it supports one; nil when
	// the panel cannot say (never guessed).
	ConnectedNow *bool `json:"connected_now"`
	// Source is "sync" (computed from snapshots) or "unavailable" (too few
	// readings). A screen must not draw 0 B/s as "idle" when it simply does not
	// know (D-026.3).
	Source string `json:"source"`
}

// TrafficSource values.
const (
	TrafficSourceSync        = "sync"
	TrafficSourceUnavailable = "unavailable"
)

// LiveTraffic returns the per-second rate of one VPN (D-038 P-04).
//
// One bounded query for the last two snapshots: the delta divided by the elapsed
// seconds. A counter reset (the panel reset the traffic) is reported as
// unavailable rather than as a negative or a huge spike.
func (s *Service) LiveTraffic(ctx context.Context, id int64, p auth.Principal) (*LiveTraffic, error) {
	v, err := s.loadVPN(ctx, id, p, false)
	if err != nil {
		return nil, err
	}
	out := &LiveTraffic{VPNID: v.ID, Source: TrafficSourceUnavailable}

	rows, err := s.db.QueryContext(ctx,
		`SELECT taken_at, up_bytes, down_bytes
		   FROM usage_snapshots WHERE vpn_instance_id = ?
		  ORDER BY taken_at DESC LIMIT 2`, v.ID)
	if err != nil {
		return nil, fmt.Errorf("provisioning: read live traffic: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type sample struct {
		at       int64
		up, down int64
	}
	var samples []sample
	for rows.Next() {
		var sp sample
		if err := rows.Scan(&sp.at, &sp.up, &sp.down); err != nil {
			return nil, fmt.Errorf("provisioning: scan live traffic: %w", err)
		}
		samples = append(samples, sp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("provisioning: iterate live traffic: %w", err)
	}
	if len(samples) < 2 {
		// One reading is a baseline, not a rate. Saying "unavailable" is the
		// honest answer; drawing 0 would claim the VPN is idle.
		return out, nil
	}
	latest, prev := samples[0], samples[1]
	elapsed := latest.at - prev.at
	if elapsed <= 0 {
		return out, nil
	}
	upDelta, downDelta := latest.up-prev.up, latest.down-prev.down
	if upDelta < 0 || downDelta < 0 {
		// The counters were reset between the readings; a rate cannot be
		// computed across a reset.
		return out, nil
	}
	out.UpBps = upDelta / elapsed
	out.DownBps = downDelta / elapsed
	out.MeasuredFrom = prev.at
	out.MeasuredTo = latest.at
	out.Source = TrafficSourceSync

	// `connected_now` is only answered when the adapter can ask; a nil means
	// "the panel cannot say", never "not connected".
	if client, ferr := s.factory(ctx, v.ServerID); ferr == nil {
		if online, ok := client.(interface {
			Online(ctx context.Context) ([]string, bool)
		}); ok {
			if list, supported := online.Online(ctx); supported {
				found := false
				for _, e := range list {
					if e == v.PanelEmail {
						found = true
						break
					}
				}
				out.ConnectedNow = &found
			}
		}
	}
	return out, nil
}

// PingResult is the server-reachability measurement behind the detail page's
// "Ping" button (D-026). It is NOT the client's own latency.
type PingResult struct {
	VPNID    int64 `json:"vpn_id"`
	ServerID int64 `json:"server_id"`
	// Reachable is whether the panel answered a TCP dial at all.
	Reachable bool `json:"reachable"`
	// LatencyMS is the dial time in milliseconds (-1 when unreachable).
	LatencyMS int64 `json:"latency_ms"`
	// Scope is the constant label the UI must show, so nobody reads this as
	// their own connection quality.
	Scope string `json:"scope"`
}

// PanelPinger measures reachability of the panel host. Injected so tests do not
// open sockets.
type PanelPinger func(ctx context.Context, serverID int64) (reachable bool, latencyMS int64)

// Ping measures the panel host's reachability for one VPN's server.
//
// The label is part of the contract, not the UI: `ScopeServerReachability` is
// returned on the wire so a screen physically cannot claim this is the user's
// own ping time.
func (s *Service) Ping(ctx context.Context, id int64, p auth.Principal, ping PanelPinger) (*PingResult, error) {
	v, err := s.loadVPN(ctx, id, p, false)
	if err != nil {
		return nil, err
	}
	out := &PingResult{VPNID: v.ID, ServerID: v.ServerID, LatencyMS: -1, Scope: ScopeServerReachability}
	if ping == nil {
		return out, nil
	}
	reachable, ms := ping(ctx, v.ServerID)
	out.Reachable = reachable
	if reachable {
		out.LatencyMS = ms
	}
	return out, nil
}

// ScopeServerReachability is the only honest label for PingResult.LatencyMS.
const ScopeServerReachability = "server_reachability"

// panelKind is the middle segment of a panel name (D-034.1).
//
// `free` for a FREE plan, `paid` for everything else — a PREMIUM package and a
// CUSTOM order share the `paid` counter, because both are something the
// customer paid for and an operator reading the panel does not need them
// separated.
func panelKind(o *Offer) string {
	if o != nil && o.Kind == KindFree {
		return "free"
	}
	return "paid"
}

// nextPanelEmail reserves the next free `<client email>_<kind>_<n>` (D-034).
//
// It runs INSIDE the caller's transaction, so two concurrent creates for the
// same customer cannot take the same slot: SQLite holds the write lock from the
// first INSERT, and the lookup below sees the row the other create just wrote.
//
// The counter never goes backwards (D-034.2): a revoked `_paid_2` is not
// re-issued, so an operator reading the panel sees a continuous history rather
// than recycled labels.
func (s *Service) nextPanelEmail(ctx context.Context, tx *sql.Tx, clientUserID, serverID int64, kind string) (string, error) {
	var email string
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(email, '') FROM users WHERE id = ?`, clientUserID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotPermitted
	}
	if err != nil {
		return "", fmt.Errorf("provisioning: read client email: %w", err)
	}
	base := domain.PanelBase(email, kind)

	// Every name this server already uses, from Veylix's rows and from the tag
	// table (a tag can exist for a panel client Veylix has no row for yet).
	used := make(map[string]bool, 16)
	rows, err := tx.QueryContext(ctx,
		`SELECT panel_email FROM vpn_instances WHERE server_id = ?
		 UNION
		 SELECT panel_email FROM panel_client_tags WHERE server_id = ?`, serverID, serverID)
	if err != nil {
		return "", fmt.Errorf("provisioning: list panel names: %w", err)
	}
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr == nil {
			used[name] = true
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("provisioning: iterate panel names: %w", err)
	}

	return domain.PanelFindNextFreeSlot(base, 1, func(n string) bool { return used[n] })
}
