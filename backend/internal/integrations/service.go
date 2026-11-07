// Package integrations manages panel integrations: add, verify, edit,
// disable, remove — backed by the real 3X-UI adapter.
//
// Invariants:
//   - the API token is sealed with AES-256-GCM before it touches the database
//     and is never returned by any method (D-016);
//   - an integration only becomes ACTIVE after a successful authenticated
//     health check — never on save alone;
//   - a failed check records CONNECTION_UNAVAILABLE/REAUTH_REQUIRED and never
//     claims success (ledger F-002).
package integrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

// ErrNotFound is returned for an unknown integration.
var ErrNotFound = errors.New("integrations: not found")

// ErrInvalidInput is returned for a rejected URL or missing fields.
var ErrInvalidInput = errors.New("integrations: invalid input")

// ErrInUse is returned when an integration cannot be purged because live VPN
// instances (and therefore client history) still point at it. The caller is
// told what to do instead of silently losing data (REQUIREMENTS §3.3).
var ErrInUse = errors.New("integrations: server still has VPN instances")

// panelClient is the adapter surface the service depends on, kept narrow so the
// service stays testable without a network.
type panelClient interface {
	Health(ctx context.Context) panel.HealthResult
	Inbounds(ctx context.Context) ([]panel.InboundOption, error)
	// ListClients backs the read-only panel inspector (D-024 Stage C).
	ListClients(ctx context.Context, page, pageSize int) (*panel.ClientPage, error)
}

// Service owns integration lifecycle.
type Service struct {
	db       *sql.DB
	sealer   *auth.Sealer
	audit    *audit.Logger
	resolve  func(host string) ([]net.IP, error)
	newPanel func(panel.Options) (panelClient, error)
}

// New builds the service.
func New(db *sql.DB, sealer *auth.Sealer, auditLog *audit.Logger) *Service {
	return &Service{
		db:      db,
		sealer:  sealer,
		audit:   auditLog,
		resolve: net.LookupIP,
		newPanel: func(o panel.Options) (panelClient, error) {
			return panel.New(o)
		},
	}
}

// AddInput is the create payload.
type AddInput struct {
	Name                string
	PanelURL            string
	Token               string
	AllowPrivateNetwork bool
	ActorID             int64
}

// UpdateInput is the edit payload. Every field is optional: a nil pointer
// keeps the stored value, which lets the UI change only the token (the one
// thing an operator often must rotate) without re-typing the URL.
//
// The token is write-only here as well — there is no way to read it back.
type UpdateInput struct {
	Name                *string
	PanelURL            *string
	Token               *string
	AllowPrivateNetwork *bool
	ActorID             int64
}

// View is the API projection — intentionally has no token field.
type View struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	PanelURL        string `json:"panel_url"`
	Status          string `json:"status"`
	PanelVersion    string `json:"panel_version"`
	XrayVersion     string `json:"xray_version"`
	LastHealthCheck *int64 `json:"last_health_check"`
	// Detail explains a failed check in the operator's terms (wrong URL, rejected
	// token, unreachable host). Empty when the check passed. It is deliberately
	// free text: the panel's own words are the most useful diagnosis.
	Detail string `json:"detail,omitempty"`
}

// addAAD binds a sealed token to its panel URL so a ciphertext copied onto
// another row cannot be opened.
func addAAD(panelURL string) []byte {
	sum := sha256.Sum256([]byte("integration:" + strings.ToLower(panelURL)))
	return sum[:]
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

// Add validates the URL, performs a real health check, then stores the sealed
// token with the resulting status.
func (s *Service) Add(ctx context.Context, in AddInput) (*View, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Token) == "" {
		return nil, fmt.Errorf("%w: panel API token is required", ErrInvalidInput)
	}

	target, err := panel.ParseTarget(in.PanelURL, in.AllowPrivateNetwork, s.resolve)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	client, err := s.newPanel(panel.Options{Target: target, Token: in.Token})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	health := client.Health(ctx)

	sealed, err := s.sealer.Seal([]byte(in.Token), addAAD(target.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("integrations: seal token: %w", err)
	}

	now := time.Now().Unix()
	var healthAt any
	if health.Status == panel.StatusActive {
		healthAt = now
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO server_integrations
		   (name, panel_url, token_ciphertext, status, panel_version, xray_version,
		    allow_private_network, last_health_check, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, target.BaseURL, sealed, string(health.Status), health.PanelVersion, health.XrayVersion,
		boolToInt(in.AllowPrivateNetwork), healthAt, nullableID(in.ActorID), now, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: this panel URL is already registered", ErrInvalidInput)
		}
		return nil, fmt.Errorf("integrations: insert: %w", err)
	}
	id, _ := res.LastInsertId()

	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: in.ActorID, Action: audit.ActionServerAdd, TargetType: "server",
		TargetID: fmt.Sprint(id),
		Context:  map[string]any{"name": name, "status": string(health.Status), "panel_version": health.PanelVersion},
	})

	view := &View{
		ID: id, Name: name, PanelURL: target.BaseURL, Status: string(health.Status),
		PanelVersion: health.PanelVersion, XrayVersion: health.XrayVersion,
	}
	if health.Status == panel.StatusActive {
		v := now
		view.LastHealthCheck = &v
	}
	return view, nil
}

// Update edits an integration's name, panel URL, token and private-network
// policy, then re-runs the authenticated health check.
//
// Invariants kept from Add:
//   - a changed URL is re-validated by the SSRF guard (never trusted from the
//     client), and the token is re-sealed with an AAD bound to the NEW URL, so
//     a ciphertext copied between rows still cannot be opened;
//   - the token is never returned, and an EMPTY token means "keep the stored
//     one" — the UI must not be able to blank a credential by accident;
//   - status is always the OBSERVED outcome of the check, never ACTIVE on save.
func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (*View, error) {
	row, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}

	name := row.name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
		}
	}

	rawURL := row.panelURL
	if in.PanelURL != nil && strings.TrimSpace(*in.PanelURL) != "" {
		rawURL = *in.PanelURL
	}
	allowPrivate := row.allowPrivate
	if in.AllowPrivateNetwork != nil {
		allowPrivate = *in.AllowPrivateNetwork
	}
	// Always re-validate: the private-network policy may have been relaxed or
	// tightened in the same request, and a stored URL could predate a change.
	target, err := panel.ParseTarget(rawURL, allowPrivate, s.resolve)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// An absent or blank token keeps the sealed one. Rotating it is the normal
	// case for this endpoint (D-016: the old value is never recoverable).
	token := ""
	if in.Token != nil {
		token = strings.TrimSpace(*in.Token)
	}
	sealed := row.tokenCiphertext
	if token != "" {
		sealed, err = s.sealer.Seal([]byte(token), addAAD(target.BaseURL))
		if err != nil {
			return nil, fmt.Errorf("integrations: seal token: %w", err)
		}
	} else if target.BaseURL != row.panelURL {
		// The AAD is derived from the URL: re-sealing is impossible without the
		// plaintext, so a URL change without a new token must be refused rather
		// than silently storing a ciphertext that can never be opened again.
		return nil, fmt.Errorf("%w: re-enter the panel API token when changing the panel URL", ErrInvalidInput)
	}

	client, err := s.newPanel(panel.Options{Target: target, Token: s.tokenFor(row, token)})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	health := client.Health(ctx)

	now := time.Now().Unix()
	var healthAt any
	if health.Status == panel.StatusActive {
		healthAt = now
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE server_integrations
		    SET name = ?, panel_url = ?, token_ciphertext = ?, status = ?,
		        panel_version = ?, xray_version = ?, allow_private_network = ?,
		        last_health_check = ?, updated_at = ?
		  WHERE id = ? AND status <> 'REMOVED'`,
		name, target.BaseURL, sealed, string(health.Status), health.PanelVersion,
		health.XrayVersion, boolToInt(allowPrivate), healthAt, now, id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: this panel URL is already registered", ErrInvalidInput)
		}
		return nil, fmt.Errorf("integrations: update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}

	// Only non-secret fields are audited; url_changed/token_rotated are booleans.
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: in.ActorID, Action: audit.ActionServerUpdate, TargetType: "server",
		TargetID: fmt.Sprint(id),
		Context: map[string]any{
			"name":          name,
			"status":        string(health.Status),
			"url_changed":   target.BaseURL != row.panelURL,
			"token_rotated": token != "",
		},
	})

	view := &View{
		ID: id, Name: name, PanelURL: target.BaseURL, Status: string(health.Status),
		PanelVersion: health.PanelVersion, XrayVersion: health.XrayVersion,
	}
	if health.Status == panel.StatusActive {
		v := now
		view.LastHealthCheck = &v
	}
	return view, nil
}

// tokenFor returns the plaintext used for this request's health check: the
// newly supplied token when rotating, otherwise the stored one, opened only
// for the duration of the call.
func (s *Service) tokenFor(row *storedRow, provided string) string {
	if provided != "" {
		return provided
	}
	plain, err := s.sealer.Open(row.tokenCiphertext, addAAD(row.panelURL))
	if err != nil {
		// An unopenable ciphertext must not stop an edit that supplies a token;
		// the check will simply report REAUTH_REQUIRED for an empty credential.
		return ""
	}
	return string(plain)
}

// Test re-runs the authenticated health check and persists the outcome.
// This is the endpoint that replaces the frontend's simulated stub (F-002).
func (s *Service) Test(ctx context.Context, id, actorID int64) (*View, error) {
	row, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(row)
	if err != nil {
		return nil, err
	}

	health := client.Health(ctx)
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE server_integrations
		    SET status = ?, panel_version = ?, xray_version = ?, last_health_check = ?, updated_at = ?
		  WHERE id = ?`,
		string(health.Status), health.PanelVersion, health.XrayVersion, now, now, id); err != nil {
		return nil, fmt.Errorf("integrations: update health: %w", err)
	}

	result := "OK"
	if health.Status != panel.StatusActive {
		result = "ERROR"
	}
	// The reason a check failed must reach the operator, not just the status.
	// Without this the UI says "Connection Unavailable" and leaves the operator
	// guessing whether it is the URL, the token, the certificate or the inbound
	// endpoint — the diagnosis lives in `health.Err` and was being discarded.
	failure := ""
	if health.Err != nil {
		failure = health.Err.Error()
	}
	auditCtx := map[string]any{"status": string(health.Status), "latency_ms": health.LatencyMS}
	if failure != "" {
		auditCtx["reason"] = failure
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actorID, Action: audit.ActionServerTest, TargetType: "server",
		TargetID: fmt.Sprint(id), Result: result, Context: auditCtx,
	})

	return &View{
		ID: id, Name: row.name, PanelURL: row.panelURL, Status: string(health.Status),
		PanelVersion: health.PanelVersion, XrayVersion: health.XrayVersion,
		LastHealthCheck: &now,
		// Detail is the human-readable reason, empty when the check passed.
		Detail: failure,
	}, nil
}

// Inbounds lists selectable inbounds and caches their metadata.
func (s *Service) Inbounds(ctx context.Context, id int64) ([]panel.InboundOption, error) {
	row, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(row)
	if err != nil {
		return nil, err
	}
	inbounds, err := client.Inbounds(ctx)
	if err != nil {
		return nil, fmt.Errorf("integrations: list inbounds: %w", err)
	}
	if err := s.cacheInbounds(ctx, id, inbounds); err != nil {
		return nil, err
	}
	return inbounds, nil
}

// PanelClientView is one client as the PANEL reports it, joined with Veylix's
// own classification.
//
// Every field except Tag/Assigned comes from the panel (D-024.2): the panel is
// the only authority on a VPN's limits. Tag is the operator's FREE/PREMIUM
// classification (D-024.3) and Assigned says whether a Veylix client holds it.
type PanelClientView struct {
	PanelEmail string `json:"panel_email"`
	SubID      string `json:"sub_id"`
	InboundIDs []int  `json:"inbound_ids"`
	TotalGB    int64  `json:"total_gb"`
	ExpiryTime int64  `json:"expiry_time"`
	LimitIP    int    `json:"limit_ip"`
	LimitHwid  int    `json:"limit_hwid"`
	Enable     bool   `json:"enable"`
	UpBytes    int64  `json:"up_bytes"`
	DownBytes  int64  `json:"down_bytes"`
	// Tag is nil for an unclassified VPN — still a VPN, visible only to the
	// Super Admin (D-024.3).
	Tag *string `json:"tag"`
	// Assigned is true when a Veylix client holds this VPN.
	Assigned bool `json:"assigned"`
	// VeylixStatus is the local lifecycle state, empty when Veylix has no row
	// (a client created directly in 3X-UI).
	VeylixStatus string `json:"veylix_status,omitempty"`

	// ---- who owns it (D-037) -------------------------------------------
	//
	// "This is a VPN — whose?" is the question the operator actually asks when
	// a server is opened, and before 2026-10-25 the wire could not answer it:
	// `Assigned` said *a* client holds it, not which one. These fields make
	// every row attributable, which is what the client-side privacy rule
	// (D-037.4) is enforced from.
	VPNInstanceID int64  `json:"vpn_instance_id,omitempty"`
	OwnerUserID   int64  `json:"owner_user_id,omitempty"`
	OwnerEmail    string `json:"owner_email,omitempty"`
	OwnerName     string `json:"owner_name,omitempty"`
	SellerUserID  int64  `json:"seller_user_id,omitempty"`
	SellerEmail   string `json:"seller_email,omitempty"`
	OfferID       int64  `json:"offer_id,omitempty"`
	OfferName     string `json:"offer_name,omitempty"`
	OfferKind     string `json:"offer_kind,omitempty"`

	// ---- how far along it is (owner: "usage kohomda iwara wela thiyenne")
	LimitBytes int64 `json:"limit_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
	UsagePct   int   `json:"usage_pct"`
	DaysLeft   int   `json:"days_left"`
	CreatedAt  int64 `json:"created_at,omitempty"`

	// Protocol facts that come from the panel, never invented (D-038 P-09).
	Protocol  string `json:"protocol,omitempty"`
	Transport string `json:"transport,omitempty"`
}

// PanelClients reads every client on the panel and joins each with its tag and
// assignment. READ-ONLY against the panel: no write path exists here, so this
// can never change the panel's state (D-024 Stage C).
//
// Pagination is bounded by the same limits the sync layer uses, so a large
// panel cannot make this endpoint unbounded work on a 1 vCPU box.
func (s *Service) PanelClients(ctx context.Context, id int64) ([]PanelClientView, error) {
	row, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(row)
	if err != nil {
		return nil, err
	}

	const pageSize, maxPages = 200, 50
	seen := make([]panel.ClientRow, 0, pageSize)
	byEmail := make(map[string]bool, pageSize)
	for page := 1; page <= maxPages; page++ {
		res, err := client.ListClients(ctx, page, pageSize)
		if err != nil {
			return nil, fmt.Errorf("integrations: list panel clients: %w", err)
		}
		for _, c := range res.Items {
			if c.Email == "" || byEmail[c.Email] {
				continue
			}
			byEmail[c.Email] = true
			seen = append(seen, c)
		}
		if len(res.Items) < pageSize {
			break
		}
		if res.Total > 0 && len(seen) >= res.Total {
			break
		}
	}

	// Local classification and assignment, read in two bounded queries rather
	// than one per client.
	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return nil, err
	}
	owners, err := s.loadAssignments(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	out := make([]PanelClientView, 0, len(seen))
	for _, c := range seen {
		v := PanelClientView{
			PanelEmail:   c.Email,
			SubID:        c.SubID,
			InboundIDs:   c.InboundIDs,
			TotalGB:      c.TotalGB,
			ExpiryTime:   c.ExpiryTime,
			LimitIP:      c.LimitIP,
			LimitHwid:    c.LimitHwid,
			Enable:       c.Enable,
			VeylixStatus: owners[c.Email].Status,
		}
		if c.Traffic != nil {
			v.UpBytes = c.Traffic.Up
			v.DownBytes = c.Traffic.Down
		}
		// Whether it is held, and — the question the wire could not answer
		// before D-037 — by whom.
		if o, ok := owners[c.Email]; ok {
			// A REVOKED or REMOVED row no longer represents a held VPN.
			v.Assigned = o.Status != "REVOKED" && o.Status != "REMOVED_FROM_CATALOG"
			v.VPNInstanceID = o.VPNInstanceID
			v.OwnerUserID = o.OwnerUserID
			v.OwnerEmail = o.OwnerEmail
			v.OwnerName = o.OwnerName
			v.SellerUserID = o.SellerUserID
			v.SellerEmail = o.SellerEmail
			v.OfferID = o.OfferID
			v.OfferName = o.OfferName
			v.OfferKind = o.OfferKind
			v.CreatedAt = o.CreatedAt
			// The panel's own limit wins when it reports one (D-024.2); the local
			// mirror is only a fallback for a panel that omitted it.
			//
			// `c.TotalGB` is BYTES despite its name: 3X-UI sends the field in
			// bytes (10 GiB arrives as 10737418240). Scaling it by 1024^3 again
			// was the same defect `loadAssignments` already had to drop from its
			// SQL (see the note there) — a 500 GiB plan overflowed int64 and
			// wrapped to exactly 2^62, so a VPN that had used 452 MB of its
			// allowance reported `usage_pct: 0` on every screen. Copy it as-is.
			v.LimitBytes = o.LimitBytes
			if c.TotalGB > 0 {
				v.LimitBytes = c.TotalGB
			}
			v.UsedBytes = v.UpBytes + v.DownBytes
			if v.LimitBytes > 0 {
				v.UsagePct = int(v.UsedBytes * 100 / v.LimitBytes)
			}
			// The panel reports expiry in milliseconds; a value in the past is
			// 0 days left, never a negative number a UI would have to defend.
			if c.ExpiryTime > 0 {
				days := int((c.ExpiryTime/1000 - now) / 86400)
				if days < 0 {
					days = 0
				}
				v.DaysLeft = days
			}
		}
		if t, ok := tags[c.Email]; ok {
			tag := string(t)
			v.Tag = &tag
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) loadTags(ctx context.Context, serverID int64) (map[string]store.Tag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT panel_email, tag FROM panel_client_tags WHERE server_id = ?`, serverID)
	if err != nil {
		return nil, fmt.Errorf("integrations: read tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]store.Tag, 16)
	for rows.Next() {
		var email, tag string
		if err := rows.Scan(&email, &tag); err != nil {
			return nil, fmt.Errorf("integrations: scan tag: %w", err)
		}
		out[email] = store.Tag(tag)
	}
	return out, rows.Err()
}

// panelOwner is the Veylix-side identity of one panel client.
//
// It replaces the old `map[string]bool` "assigned" answer: the operator's first
// question is "whose is this", and a boolean cannot answer it (D-037).
type panelOwner struct {
	PanelEmail    string
	VPNInstanceID int64
	OwnerUserID   int64
	OwnerEmail    string
	OwnerName     string
	SellerUserID  int64
	SellerEmail   string
	OfferID       int64
	OfferName     string
	OfferKind     string
	Status        string
	CreatedAt     int64
	ExpiryAt      int64
	LimitBytes    int64
}

// loadAssignments reads the local VPN rows for one server, keyed by panel email.
//
// One bounded query, never one per client: a server may hold thousands of
// clients and this runs on a 1 vCPU host.
func (s *Service) loadAssignments(ctx context.Context, serverID int64) (map[string]panelOwner, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT i.panel_email, i.id, COALESCE(i.owner_user_id,0), COALESCE(u.email,''), COALESCE(u.display_name,''),
		        COALESCE(a.seller_user_id,0), COALESCE(st.email,''),
		        COALESCE(i.offer_id,0), COALESCE(o.name,''), COALESCE(o.kind,''),
		        i.status, i.created_at, COALESCE(i.expires_at,0),
		        -- panel_total_gb already holds BYTES, not gigabytes: sync mirrors the
		        -- panel's totalGB field verbatim, and 3X-UI sends that field in
		        -- bytes (10 GiB arrives as 10737418240). Multiplying by 1073741824
		        -- here overflowed int64 for any real limit — 2^63 exactly — and
		        -- broke this whole listing with a scan error.
		        COALESCE(i.panel_total_gb,0)
		   FROM vpn_instances i
		   LEFT JOIN users u           ON u.id = i.owner_user_id
		   LEFT JOIN vpn_assignments a ON a.vpn_instance_id = i.id
		   LEFT JOIN users st          ON st.id = a.seller_user_id
		   LEFT JOIN vpn_offers o      ON o.id = i.offer_id
		  WHERE i.server_id = ?`, serverID)
	if err != nil {
		return nil, fmt.Errorf("integrations: read assignments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]panelOwner, 16)
	for rows.Next() {
		var o panelOwner
		if err := rows.Scan(&o.PanelEmail, &o.VPNInstanceID, &o.OwnerUserID, &o.OwnerEmail, &o.OwnerName,
			&o.SellerUserID, &o.SellerEmail, &o.OfferID, &o.OfferName, &o.OfferKind,
			&o.Status, &o.CreatedAt, &o.ExpiryAt, &o.LimitBytes); err != nil {
			return nil, fmt.Errorf("integrations: scan assignment: %w", err)
		}
		out[o.PanelEmail] = o
	}
	return out, rows.Err()
}

// RenameClient renames a panel client (D-034.5, D-037.2).
//
// The panel has no rename operation, and `POST /clients/update/{email}` is a
// REPLACE: it overwrites every editable field. So the current record is read and
// re-written with only the name changed, then confirmed with a fresh read —
// success is never assumed (`new_update.md` §5).
//
// The credential material (uuid / password / flow / limits / expiry) is carried
// across untouched, because losing it would break a working VPN.
func (s *Service) RenameClient(ctx context.Context, serverID int64, from, to string) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("integrations: rename: the new name is empty")
	}
	if from == to {
		return nil
	}
	row, err := s.load(ctx, serverID)
	if err != nil {
		return err
	}
	client, err := s.clientFor(row)
	if err != nil {
		return err
	}
	renamer, ok := client.(panelRenamer)
	if !ok {
		return fmt.Errorf("integrations: this panel adapter cannot rename clients")
	}

	current, err := renamer.GetClient(ctx, from)
	if err != nil {
		return fmt.Errorf("integrations: rename: read %s: %w", from, err)
	}
	if current == nil {
		return fmt.Errorf("integrations: rename: the panel does not hold %s", from)
	}

	spec := panel.SpecFromPanelClient(current)
	spec.Email = to
	if err := renamer.UpdateClient(ctx, from, spec); err != nil {
		return fmt.Errorf("integrations: rename: write: %w", err)
	}

	// Confirm with a fresh read: the panel applies an update to each attached
	// inbound independently, so a 200 does not prove the rename landed.
	after, err := renamer.GetClient(ctx, to)
	if err != nil {
		return fmt.Errorf("integrations: rename: confirm: %w", err)
	}
	if after == nil {
		return fmt.Errorf("integrations: rename: the panel did not confirm the new name")
	}
	return nil
}

// Devices lists the hardware identifiers a VPN's panel client has registered
// against its device cap (D8). Read-only: it never changes the panel.
//
// The panel exposes a short fingerprint only, never the full hash, so this list
// is safe to show a Super Admin. An unknown client answers an empty list, not an
// error — "no devices" and "no such client" look the same to the caller, and
// both mean "nothing is holding a slot".
func (s *Service) Devices(ctx context.Context, serverID int64, panelEmail string) ([]panel.Hwid, error) {
	if strings.TrimSpace(panelEmail) == "" {
		return nil, fmt.Errorf("integrations: devices: panel email is required")
	}
	row, err := s.load(ctx, serverID)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(row)
	if err != nil {
		return nil, err
	}
	hw, ok := client.(panelHwidInspector)
	if !ok {
		return nil, fmt.Errorf("integrations: this panel adapter cannot list devices")
	}
	devices, err := hw.Hwids(ctx, panelEmail)
	if err != nil {
		return nil, fmt.Errorf("integrations: devices: %w", err)
	}
	return devices, nil
}

// RevokeDevice frees one device slot on a VPN (D8), then CONFIRMS with a fresh
// read: the delete answers 200 even when the panel has already dropped the row,
// so an unconfirmed success would report a freed slot that still holds one.
//
// A device that is already gone is a success — the desired end state is "this
// fingerprint no longer holds a slot", and reaching it twice is not a failure.
func (s *Service) RevokeDevice(ctx context.Context, serverID int64, panelEmail string, hwidID int64) error {
	if strings.TrimSpace(panelEmail) == "" {
		return fmt.Errorf("integrations: revoke device: panel email is required")
	}
	if hwidID <= 0 {
		return fmt.Errorf("integrations: revoke device: invalid device id")
	}
	row, err := s.load(ctx, serverID)
	if err != nil {
		return err
	}
	client, err := s.clientFor(row)
	if err != nil {
		return err
	}
	hw, ok := client.(panelHwidInspector)
	if !ok {
		return fmt.Errorf("integrations: this panel adapter cannot revoke devices")
	}

	before, err := hw.Hwids(ctx, panelEmail)
	if err != nil {
		return fmt.Errorf("integrations: revoke device: read: %w", err)
	}
	if !containsHwid(before, hwidID) {
		// Already absent: the requested end state already holds.
		return nil
	}

	if err := hw.DeleteHwid(ctx, panelEmail, hwidID); err != nil {
		return fmt.Errorf("integrations: revoke device: write: %w", err)
	}

	after, err := hw.Hwids(ctx, panelEmail)
	if err != nil {
		return fmt.Errorf("integrations: revoke device: confirm: %w", err)
	}
	if containsHwid(after, hwidID) {
		return fmt.Errorf("integrations: revoke device: the panel did not confirm the removal")
	}
	return nil
}

// containsHwid reports whether id appears in devices.
func containsHwid(devices []panel.Hwid, id int64) bool {
	for _, d := range devices {
		if d.ID == id {
			return true
		}
	}
	return false
}

// panelRenamer is the write surface a rename needs. Kept separate from
// panelClient, which is deliberately read-only: the inspector must not be able
// to write, and a wide interface would let it.
type panelRenamer interface {
	GetClient(ctx context.Context, email string) (*panel.PanelClient, error)
	UpdateClient(ctx context.Context, email string, spec panel.ClientSpec) error
}

// panelHwidInspector is the device-limit surface (D8). Separate from panelClient
// for the same reason as panelRenamer: listing a VPN's devices and freeing a
// slot are capabilities the read-only inspector must not hold.
//
// The panel names these "hwids" — the hardware identifiers a client has
// registered against its device cap. Veylix never stores them; it reads the
// panel's own list, so this adapter is the only source of truth.
type panelHwidInspector interface {
	Hwids(ctx context.Context, email string) ([]panel.Hwid, error)
	DeleteHwid(ctx context.Context, email string, id int64) error
}

// clientFor decrypts the stored token and builds an adapter. The plaintext
// token never leaves this function's scope.
func (s *Service) clientFor(row *storedRow) (panelClient, error) {
	plain, err := s.sealer.Open(row.tokenCiphertext, addAAD(row.panelURL))
	if err != nil {
		return nil, fmt.Errorf("integrations: cannot open stored token (was the encryption key rotated?)")
	}
	target, err := panel.ParseTarget(row.panelURL, row.allowPrivate, s.resolve)
	if err != nil {
		return nil, fmt.Errorf("integrations: stored url is no longer acceptable: %w", err)
	}
	return s.newPanel(panel.Options{Target: target, Token: string(plain)})
}

// SetDisabled flips the operator-controlled DISABLED state.
func (s *Service) SetDisabled(ctx context.Context, id int64, disabled bool, actorID int64) error {
	status := "PENDING_VERIFICATION"
	if disabled {
		status = "DISABLED"
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE server_integrations SET status = ?, updated_at = ? WHERE id = ? AND status <> 'REMOVED'`,
		status, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("integrations: set status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actorID, Action: audit.ActionServerDisable, TargetType: "server",
		TargetID: fmt.Sprint(id), Context: map[string]any{"status": status},
	})
	return nil
}

// Remove marks an integration REMOVED without deleting history
// (REQUIREMENTS §3.3 — Premium assignments are never hard-deleted).
func (s *Service) Remove(ctx context.Context, id, actorID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE server_integrations SET status = 'REMOVED', updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("integrations: remove: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actorID, Action: audit.ActionServerRemove, TargetType: "server", TargetID: fmt.Sprint(id),
	})
	return nil
}

type storedRow struct {
	name            string
	panelURL        string
	tokenCiphertext []byte
	allowPrivate    bool
}

func (s *Service) load(ctx context.Context, id int64) (*storedRow, error) {
	var r storedRow
	var allow int
	err := s.db.QueryRowContext(ctx,
		`SELECT name, panel_url, token_ciphertext, allow_private_network
		   FROM server_integrations WHERE id = ? AND status <> 'REMOVED'`, id,
	).Scan(&r.name, &r.panelURL, &r.tokenCiphertext, &allow)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("integrations: load: %w", err)
	}
	r.allowPrivate = allow == 1
	return &r, nil
}

func (s *Service) cacheInbounds(ctx context.Context, serverID int64, inbounds []panel.InboundOption) error {
	now := time.Now().Unix()
	for _, ib := range inbounds {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO inbounds (server_id, remote_id, remark, tag, protocol, port, enabled, synced_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (server_id, remote_id) DO UPDATE SET
			   remark = excluded.remark, tag = excluded.tag, protocol = excluded.protocol,
			   port = excluded.port, enabled = excluded.enabled, synced_at = excluded.synced_at`,
			serverID, ib.ID, ib.Remark, ib.Tag, ib.Protocol, ib.Port, boolToInt(ib.Enable), now,
		); err != nil {
			return fmt.Errorf("integrations: cache inbound %d: %w", ib.ID, err)
		}
	}
	return nil
}
