package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"veylix/backend/internal/domain"
)

// Panel assignment (D-037 / G6).
//
// A panel client Veylix has never seen — created directly in 3X-UI, or left
// behind by a cleared-up please — is still a working VPN. The Super Admin can give
// it to a client account: this creates the VPN row, locks the tag and records
// everything in one transaction, so the panel client and the Veylix row can
// never disagree about who holds it.

// Errors the assignment route maps to status codes.
var (
	// ErrPanelClientTaken means a Veylix record already exists for that panel
	// client. Re-assigning would silently move somebody's VPN.
	ErrPanelClientTaken = errors.New("store: this panel client is already assigned")
	// ErrAssignTarget means the chosen account is not a client.
	ErrAssignTarget = errors.New("store: the target account is not a client")
	// ErrAssignOfferKind means the chosen package is not a premium one. An
	// assignment is always a premium issue (D-063), so a FREE package would make
	// the row's package and its locked PREMIUM tag disagree.
	ErrAssignOfferKind = errors.New("store: an assignment needs a premium package")
)

// AssignPanelClientInput is one assign request.
type AssignPanelClientInput struct {
	ServerID     int64
	PanelEmail   string
	ClientUserID int64
	// SellerID credits a reseller's quota (0 = a direct platform issue).
	SellerID int64
	// Kind is free|paid — it becomes part of the panel name (D-034.1).
	Kind string
	// OfferID links the VPN to a package when the Super Admin used one.
	OfferID int64
	// ActorID is recorded so the row is traceable to the operator.
	ActorID int64
}

// AssignResult reports what the assignment produced.
type AssignResult struct {
	VPNID      int64  `json:"vpn_id"`
	PanelEmail string `json:"panel_email"`
	Tag        string `json:"tag"`
	// OfferID is the package the row was recorded against. It is never 0: an
	// assignment without a chosen package gets the platform's own PREMIUM
	// carrier (D-063), because a row with no package has no kind and would be
	// displayed as UNTAGGED.
	OfferID int64 `json:"offer_id"`
}

// tagFor maps the panel-name kind to the FREE/PREMIUM tag.
func tagFor(kind string) Tag {
	if kind == "free" {
		return TagFree
	}
	return TagPremium
}

// AssignPanelClient gives an existing panel client to a client account.
//
// Rules (D-063):
//   - the panel client must not already be held — by a row, by a reservation or
//     by a reservation's target name (ErrPanelClientTaken);
//   - the target must be a real `client` account (ErrAssignTarget);
//   - the tag is written and LOCKED in the same transaction, so an issued VPN
//     can never be re-priced afterwards (D-025.2);
//   - **the VPN keeps the name the panel actually holds.**
//
// The last rule is the fix for "one VPN, two rows". This used to record the row
// under an invented `<client email>_<kind>_<slot>` name and leave the rename for
// the caller to attempt afterwards. When that rename failed, the panel kept the
// old name and the row kept the new one: the console still listed the panel
// client as unassigned (its old name was free) while `panel_assignments` held a
// reservation the console could not click, so a second Assign created a SECOND
// row for ONE physical VPN — two owners, two locked tags, two premium counts.
//
// Now the row, the tag and the reservation all name the panel client exactly as
// the panel does, so the row is always reachable from the name a screen sees. A
// rename into the D-034 scheme stays available, but it is an explicit,
// optional, post-assignment step that never blocks or duplicates the assignment.
func (s *Store) AssignPanelClient(ctx context.Context, in AssignPanelClientInput) (*AssignResult, error) {
	if in.Kind != "free" && in.Kind != "paid" {
		return nil, fmt.Errorf("store: assign: kind must be free or paid")
	}
	name := strings.TrimSpace(in.PanelEmail)
	if name == "" {
		return nil, fmt.Errorf("store: assign: a panel email is required")
	}
	tag := tagFor(in.Kind)
	var (
		vpnID   int64
		offerID int64
	)
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		// The target must be a client: assigning a VPN to a reseller would put a
		// customer's VPN in an operator's own list.
		var role, email string
		err := tx.QueryRowContext(ctx,
			`SELECT role, COALESCE(email,'') FROM users WHERE id = ?`, in.ClientUserID).Scan(&role, &email)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAssignTarget
		}
		if err != nil {
			return fmt.Errorf("store: assign: lookup target: %w", err)
		}
		if role != "client" {
			return ErrAssignTarget
		}

		// Already held by Veylix? Then this is a move, not an assignment — and a
		// move must be a deliberate act, never an accidental click.
		if err := s.panelClientHeld(ctx, tx, in.ServerID, name); err != nil {
			return err
		}

		// D-063: the row MUST carry a package, because `vpn_offers.kind` is what
		// decides Free vs Premium on every client screen. Without one the row had
		// no kind at all and rendered as UNTAGGED — the state reserved for a
		// panel client nobody holds.
		resolvedOferID, err := s.resolveAssignmentOffer(ctx, tx, in.OfferID)
		if err != nil {
			return err
		}
		offerID = resolvedOferID

		now := time.Now().Unix()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO vpn_instances
			   (owner_user_id, server_id, offer_id, panel_email, panel_sub_id, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'pending', 'ACTIVE', ?, ?)`,
			in.ClientUserID, in.ServerID, offerID, name, now, now)
		if err != nil {
			return fmt.Errorf("store: assign: insert vpn: %w", err)
		}
		vpnID, _ = res.LastInsertId()
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_instances SET panel_sub_id = ? WHERE id = ?`, domain.PanelSubID(vpnID), vpnID); err != nil {
			return fmt.Errorf("store: assign: set subId: %w", err)
		}

		// Every assignment is recorded, whether or not a reseller was involved.
		//
		// The count of "premium VPNs this client holds" is derived from the VPN
		// itself now (D-063), but the assignment row is still the history of how
		// the customer got it, so it is always written. `seller_user_id` is NOT
		// NULL in the schema, and for a platform issue the honest value is the
		// operator who issued it — not a made-up reseller id.
		sellerID := in.SellerID
		if sellerID <= 0 {
			sellerID = in.ActorID
		}
		if sellerID <= 0 {
			return fmt.Errorf("store: assign: an issuing actor is required")
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO vpn_assignments
			   (vpn_instance_id, client_user_id, seller_user_id, status, started_at, created_at, updated_at)
			 VALUES (?, ?, ?, 'ACTIVE', ?, ?, ?)`,
			vpnID, in.ClientUserID, sellerID, now, now, now); err != nil {
			return fmt.Errorf("store: assign: insert assignment: %w", err)
		}

		// The tag publishes the VPN, and it locks on issue (D-025.2).
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_by, tagged_at, locked_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (server_id, panel_email) DO UPDATE SET
			   tag = excluded.tag, tagged_by = excluded.tagged_by,
			   tagged_at = excluded.tagged_at, locked_at = excluded.locked_at`,
			in.ServerID, name, string(tag), nullableID(in.ActorID), now, now); err != nil {
			return fmt.Errorf("store: assign: write tag: %w", err)
		}

		// The identity ledger. `source_email` and `panel_email` are the SAME name
		// now — that is the point: one panel client, one name, one row.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO panel_assignments (server_id, source_email, panel_email, vpn_instance_id, actor_id, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			in.ServerID, name, name, vpnID, nullableID(in.ActorID), now); err != nil {
			return fmt.Errorf("store: assign: record source: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &AssignResult{VPNID: vpnID, PanelEmail: name, Tag: string(tag), OfferID: offerID}, nil
}

// RenameAssignmentName moves an assigned VPN onto its D-034 slot name.
//
// The order is deliberate and is the whole point of D-063: the PANEL is renamed
// first, and the local rows are only rewritten once the panel has confirmed.
// The panel is remote, so it cannot join a local transaction — but ordering it
// this way means a failure leaves the VPN exactly as it was, still reachable by
// the name the panel holds. The previous order (write the row under the new
// name, then try the rename) is what produced one VPN under two names.
//
// `renamePanel` is the remote step, injected so this package keeps no dependency
// on the panel adapter. It receives the current name and the wanted one.
func (s *Store) RenameAssignmentName(ctx context.Context, serverID, vpnID int64, kind string,
	renamePanel func(ctx context.Context, from, to string) error) (string, error) {
	if kind != "free" && kind != "paid" {
		return "", fmt.Errorf("store: rename: kind must be free or paid")
	}
	var (
		current string
		owner   int64
	)
	if err := s.db.QueryRowContext(ctx,
		`SELECT panel_email, owner_user_id FROM vpn_instances WHERE id = ? AND server_id = ?`,
		vpnID, serverID).Scan(&current, &owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("store: rename: vpn %d not found on server %d", vpnID, serverID)
		}
		return "", fmt.Errorf("store: rename: read vpn: %w", err)
	}
	var email string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(email,'') FROM users WHERE id = ?`, owner).Scan(&email); err != nil {
		return "", fmt.Errorf("store: rename: read owner: %w", err)
	}

	base := domain.PanelBase(email, kind)
	from := 1
	if domain.HasSlotPrefix(current, base) {
		// Already one of this customer's slots: keep the number so the audit
		// trail stays continuous.
		from = domain.PanelSlotNext(current) - 1
	}
	taken := s.takenAssignmentNames(ctx, serverID)
	target, err := domain.PanelFindNextFreeSlot(base, from, taken)
	if err != nil {
		return "", err
	}
	if target == current {
		return current, nil
	}

	// Step 1 — the panel. A failure here returns the error and touches nothing,
	// so the VPN simply keeps the name it has.
	if renamePanel != nil {
		if err := renamePanel(ctx, current, target); err != nil {
			return "", fmt.Errorf("store: rename: panel: %w", err)
		}
	}

	// Step 2 — the local rows, together.
	err = s.Tx(ctx, func(tx *sql.Tx) error {
		now := time.Now().Unix()
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_instances SET panel_email = ?, updated_at = ? WHERE id = ?`,
			target, now, vpnID); err != nil {
			return fmt.Errorf("store: rename: update vpn: %w", err)
		}
		// The tag follows the name it labels; leaving it behind would orphan the
		// locked classification on a name no row uses any more.
		if _, err := tx.ExecContext(ctx,
			`UPDATE panel_client_tags SET panel_email = ? WHERE server_id = ? AND panel_email = ?`,
			target, serverID, current); err != nil {
			return fmt.Errorf("store: rename: update tag: %w", err)
		}
		// `source_email` stays the name the panel client had when it was
		// assigned — that is history. `panel_email` tracks the name in use, so
		// the "already held" check answers for both.
		if _, err := tx.ExecContext(ctx,
			`UPDATE panel_assignments SET panel_email = ? WHERE server_id = ? AND vpn_instance_id = ?`,
			target, serverID, vpnID); err != nil {
			return fmt.Errorf("store: rename: update ledger: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return target, nil
}

// RenameAssignment is the httpapi-facing name for RenameAssignmentName.
//
// It exists so the transport layer names the operation the way the route and the
// interface do, while the store keeps the longer name that says exactly what it
// writes (a name, not an assignment).
func (s *Store) RenameAssignment(ctx context.Context, serverID, vpnID int64, kind string,
	renamePanel func(ctx context.Context, from, to string) error) (string, error) {
	return s.RenameAssignmentName(ctx, serverID, vpnID, kind, renamePanel)
}

// takenAssignmentNames reports whether a candidate name is already in use on
// this server by a row, a tag or an assignment ledger entry.
//
// All three are consulted: a reservation can exist before its row, and a tag can
// exist for a panel client Veylix has no row for yet.
func (s *Store) takenAssignmentNames(ctx context.Context, serverID int64) func(string) bool {
	used := make(map[string]bool, 8)
	rows, err := s.db.QueryContext(ctx,
		`SELECT panel_email FROM vpn_instances WHERE server_id = ?
		 UNION SELECT panel_email FROM panel_client_tags WHERE server_id = ?
		 UNION SELECT source_email FROM panel_assignments WHERE server_id = ?
		 UNION SELECT panel_email FROM panel_assignments WHERE server_id = ?`,
		serverID, serverID, serverID, serverID)
	if err == nil {
		for rows.Next() {
			var name string
			if scanErr := rows.Scan(&name); scanErr == nil {
				used[name] = true
			}
		}
		_ = rows.Close()
	}
	return func(name string) bool { return used[name] }
}

// platformAssignmentOffer is the internal PREMIUM package an assignment falls
// back to when the operator does not pick one (D-063).
//
// It exists because the client projection reads `vpn_offers.kind` to decide Free
// vs Premium, so a NULL `offer_id` produced a VPN with no kind at all — shown as
// UNTAGGED, the state reserved for a panel client nobody holds. The carrier is
// `active = 0` on purpose: it is not a product, it is the row that says "this
// issue is premium".
const platformAssignmentOffer = "Platform assignment"

// panelClientHeld reports an error when this panel client is already spoken for.
//
// Three facts make a client "held", and all three are checked because the split
// this prevents came from checking only one of them (D-063):
//
//  1. a `vpn_instances` row already carries this name;
//  2. a reservation exists whose `source_email` is this name (the identity the
//     panel client had when it was assigned);
//  3. a reservation exists whose `panel_email` is this name — a rename moved
//     the row to a different name, and assigning the old one again would create
//     a second row for the same physical VPN.
func (s *Store) panelClientHeld(ctx context.Context, tx *sql.Tx, serverID int64, name string) error {
	var held int
	if err := tx.QueryRowContext(ctx,
		`SELECT
		   (SELECT COUNT(*) FROM vpn_instances
		     WHERE server_id = ? AND panel_email = ? AND status <> 'REVOKED')
		 + (SELECT COUNT(*) FROM panel_assignments
		     WHERE server_id = ? AND (source_email = ? OR panel_email = ?))`,
		serverID, name, serverID, name, name).Scan(&held); err != nil {
		return fmt.Errorf("store: assign: check existing: %w", err)
	}
	if held > 0 {
		return ErrPanelClientTaken
	}
	return nil
}

// resolveAssignmentOffer decides which package an assigned row points at.
//
// Order: the operator's choice, then any active PREMIUM catalogue row, then the
// internal platform carrier. It never returns 0 — see platformAssignmentOffer
// for why a NULL package is not an option.
//
// A chosen package is validated rather than trusted: an unknown id would fail
// the foreign key with an opaque error, and a FREE package would contradict the
// PREMIUM tag written beside it.
func (s *Store) resolveAssignmentOffer(ctx context.Context, tx *sql.Tx, chosen int64) (int64, error) {
	if chosen > 0 {
		var kind string
		err := tx.QueryRowContext(ctx, `SELECT kind FROM vpn_offers WHERE id = ?`, chosen).Scan(&kind)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("store: assign: offer %d does not exist", chosen)
		}
		if err != nil {
			return 0, fmt.Errorf("store: assign: read offer: %w", err)
		}
		if kind != "PREMIUM" {
			// An assignment is a premium issue (D-063); a FREE package would make
			// the row's package and its locked tag disagree.
			return 0, ErrAssignOfferKind
		}
		return chosen, nil
	}

	// Step 1 — the operator's own catalogue, when one exists.
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM vpn_offers
		  WHERE kind = 'PREMIUM' AND active = 1 AND archived_at IS NULL
		  ORDER BY price_cents DESC, id LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: assign: find premium offer: %w", err)
	}

	// Step 2 — the platform CARRIER, by NAME and not by `active`.
	//
	// This is the defect that produced four identical "Platform assignment" rows.
	// The carrier is created with `active = 0` on purpose (it is not a product),
	// but the lookup above — and this one, before the fix — required `active = 1`
	// for catalogue rows only, while the carrier branch itself had no lookup at
	// all: it went straight to INSERT on every assignment. Reusing the row by name
	// makes "one carrier per installation" a fact instead of a hope, and it repairs
	// installations that already hold duplicates by picking the OLDEST (which is the
	// one existing VPN rows already point at).
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM vpn_offers
		  WHERE kind = 'PREMIUM' AND name = ? AND archived_at IS NULL
		  ORDER BY id LIMIT 1`, platformAssignmentOffer).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: assign: find platform offer: %w", err)
	}

	// Step 3 — create the carrier once, on a fresh installation.
	//
	// The currency is READ from the catalogue rather than hard-coded: the first
	// version wrote 'LKR' regardless of what the platform actually bills in, so a
	// USD installation ended up with an LKR carrier — visible in the offer list and
	// wrong in every report that groups by currency.
	now := time.Now().Unix()
	var currency string
	if cerr := tx.QueryRowContext(ctx,
		`SELECT currency FROM vpn_offers
		  WHERE currency <> '' ORDER BY id LIMIT 1`).Scan(&currency); cerr != nil || currency == "" {
		currency = "LKR"
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO vpn_offers (kind, name, price_cents, currency, active, created_at, updated_at)
		 VALUES ('PREMIUM', ?, 0, ?, 0, ?, ?)`,
		platformAssignmentOffer, currency, now, now)
	if err != nil {
		return 0, fmt.Errorf("store: assign: create platform offer: %w", err)
	}
	return res.LastInsertId()
}
