package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/integrations"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

// IntegrationService is the surface the handlers need.
type IntegrationService interface {
	Add(ctx context.Context, in integrations.AddInput) (*integrations.View, error)
	Update(ctx context.Context, id int64, in integrations.UpdateInput) (*integrations.View, error)
	Test(ctx context.Context, id, actorID int64) (*integrations.View, error)
	Inbounds(ctx context.Context, id int64) ([]panel.InboundOption, error)
	SetDisabled(ctx context.Context, id int64, disabled bool, actorID int64) error
	Remove(ctx context.Context, id, actorID int64) error
	Purge(ctx context.Context, id, actorID int64) error
}

// ServerStore lists integrations scoped by role.
type ServerStore interface {
	ListServers(ctx context.Context, adminID int64) ([]store.ServerRow, error)
}

// PanelInspector reads a panel's own client list. Read-only by construction:
// the interface exposes no write method, so this route cannot change the panel.
type PanelInspector interface {
	PanelClients(ctx context.Context, id int64) ([]integrations.PanelClientView, error)
}

// TagStore is the tagging surface of the panel_client_tags table (D-024.3).
//
// Kept separate from PanelInspector on purpose: reading the panel and
// classifying a client are different capabilities, and the inspector must stay
// read-only. Only the tagging route gets this interface.
type TagStore interface {
	SetClientTag(ctx context.Context, serverID int64, panelEmail string, tag store.Tag, actorID int64) error
	ClientTag(ctx context.Context, serverID int64, panelEmail string) (*store.ClientTag, error)
}

// AssignmentStore creates the Veylix record for a panel client nobody holds
// (D-037.2). Kept separate from TagStore: assigning a VPN to a customer is a
// bigger act than labelling one.
type AssignmentStore interface {
	AssignPanelClient(ctx context.Context, in store.AssignPanelClientInput) (*store.AssignResult, error)
	// RenameAssignment moves an already-assigned VPN to its D-034 slot name.
	//
	// It is a separate call because a rename is cosmetic and optional (D-063):
	// the assignment is already complete and correct under the panel's own name,
	// so a rename can never be a precondition for the assign to succeed. The
	// store renames the PANEL first (via the callback) and rewrites the row, the
	// tag and the ledger together only after the panel confirmed, and returns the
	// name now in force.
	RenameAssignment(ctx context.Context, serverID, vpnID int64, kind string,
		renamePanel func(ctx context.Context, from, to string) error) (string, error)
}

// PanelRenamer renames a panel client so it joins the D-034 naming scheme.
//
// Separate interface again — the inspector stays read-only, and only the assign
// route can reach a write.
type PanelRenamer interface {
	RenameClient(ctx context.Context, serverID int64, from, to string) error
}

// IntegrationRoutes registers /api/servers/*.
//
// Only a Super Admin may manage integrations (REQUIREMENTS §4). Admins may read
// their assigned servers through a separate, read-only projection.
func IntegrationRoutes(mux *http.ServeMux, svc IntegrationService, servers ServerStore,
	inspector PanelInspector, tags TagStore, aud *audit.Logger,
	authed func(http.Handler) http.Handler) {
	manage := func(h http.HandlerFunc) http.Handler {
		return authed(RequirePermission(auth.PermServerManage)(h))
	}
	viewAssigned := func(h http.HandlerFunc) http.Handler {
		return authed(RequirePermission(auth.PermServerViewAssigned)(h))
	}

	// List: super admin sees all; an admin sees only assigned servers.
	mux.Handle("GET /api/servers", viewAssigned(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		scope := int64(0)
		if p.IsAdmin() {
			scope = p.UserID
		}
		rows, err := servers.ListServers(r.Context(), scope)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list servers")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	}))

	mux.Handle("POST /api/servers", manage(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name                string `json:"name"`
			PanelURL            string `json:"panel_url"`
			Token               string `json:"token"`
			AllowPrivateNetwork bool   `json:"allow_private_network"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		p, _ := PrincipalFrom(r.Context())
		view, err := svc.Add(r.Context(), integrations.AddInput{
			Name: in.Name, PanelURL: in.PanelURL, Token: in.Token,
			AllowPrivateNetwork: in.AllowPrivateNetwork, ActorID: p.UserID,
		})
		if err != nil {
			writeIntegrationError(w, err)
			return
		}
		WriteJSON(w, http.StatusCreated, view)
	}))

	// PATCH edits the URL/name/token. The token is write-only: omitting it keeps
	// the stored one, and it is never echoed back.
	mux.Handle("PATCH /api/servers/{id}", manage(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
			return
		}
		var in struct {
			Name                *string `json:"name"`
			PanelURL            *string `json:"panel_url"`
			Token               *string `json:"token"`
			AllowPrivateNetwork *bool   `json:"allow_private_network"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		p, _ := PrincipalFrom(r.Context())
		view, err := svc.Update(r.Context(), id, integrations.UpdateInput{
			Name: in.Name, PanelURL: in.PanelURL, Token: in.Token,
			AllowPrivateNetwork: in.AllowPrivateNetwork, ActorID: p.UserID,
		})
		if err != nil {
			writeIntegrationError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, view)
	}))

	mux.Handle("POST /api/servers/{id}/test", manage(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
			return
		}
		p, _ := PrincipalFrom(r.Context())
		view, err := svc.Test(r.Context(), id, p.UserID)
		if err != nil {
			writeIntegrationError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, view)
	}))

	mux.Handle("GET /api/servers/{id}/inbounds", manage(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
			return
		}
		inbounds, err := svc.Inbounds(r.Context(), id)
		if err != nil {
			writeIntegrationError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, inbounds)
	}))

	mux.Handle("POST /api/servers/{id}/disable", manage(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
			return
		}
		p, _ := PrincipalFrom(r.Context())
		if err := svc.SetDisabled(r.Context(), id, true, p.UserID); err != nil {
			writeIntegrationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// DELETE removes the whole server permanently (hard delete): the panel
	// credentials are destroyed at rest and the cached inbounds are dropped.
	// It is refused with 409 while VPN instances still point at the server, so
	// client history can never be silently destroyed (REQUIREMENTS §3.3).
	mux.Handle("DELETE /api/servers/{id}", manage(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
			return
		}
		p, _ := PrincipalFrom(r.Context())
		if err := svc.Purge(r.Context(), id, p.UserID); err != nil {
			writeIntegrationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// GET /api/servers/{id}/panel-clients — every client the panel holds, joined
	// with Veylix's tag and assignment (D-024 Stage C).
	//
	// READ-ONLY: it exists so an operator can see the panel's truth next to
	// Veylix's records — including clients created directly in 3X-UI, which
	// Veylix has no row for. Super-Admin only: it exposes the whole panel,
	// beyond the role-scoped server list.
	//
	// ?tag=free|premium|untagged narrows the result; anything else is 422 rather
	// than silently ignored, so a typo cannot look like "no results".
	mux.Handle("GET /api/servers/{id}/panel-clients", manage(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
			return
		}
		if inspector == nil {
			WriteError(w, http.StatusServiceUnavailable, "not_wired", "panel inspection is not available")
			return
		}
		rows, err := inspector.PanelClients(r.Context(), id)
		if err != nil {
			writeIntegrationError(w, err)
			return
		}

		// The operator opens a server to answer four questions: how many free
		// and premium VPNs are here, who holds each, how much traffic is left,
		// and what is unowned. The summary is computed on the FULL set before
		// any filter, so a filtered table never makes the counts look wrong.
		summary := summarisePanelClients(rows)

		filtered, ok := filterPanelClients(w, r, rows)
		if !ok {
			return
		}

		// `?summary=1` answers only the counts: the drill-down header needs
		// them without paying for the whole table.
		if r.URL.Query().Get("summary") == "1" {
			WriteJSON(w, http.StatusOK, map[string]any{"summary": summary})
			return
		}
		WriteJSON(w, http.StatusOK, filtered)
	})))
}

// PanelClientSummary is the header of the server drill-down (D-037).
// The owner asked for exactly these numbers: how many free VPNs exist, how many
// premium, who holds them, and how much traffic is left — so the operator can
// see the state of a panel without reading a table.
type PanelClientSummary struct {
	Total      int `json:"total"`
	Free       int `json:"free"`
	Premium    int `json:"premium"`
	Untagged   int `json:"untagged"`
	Assigned   int `json:"assigned"`
	Unassigned int `json:"unassigned"`
	// Exhausted counts VPNs whose traffic is used up (a 100% usage bar).
	Exhausted int `json:"exhausted"`
	// ExpiringSoon counts VPNs with 7 days or fewer left.
	ExpiringSoon int `json:"expiring_soon"`
	// Expired counts VPNs whose expiry has passed.
	Expired int `json:"expired"`
	// Disabled counts VPNs the panel itself reports as disabled.
	Disabled int `json:"disabled"`
}

// summarisePanelClients counts a full panel listing.
func summarisePanelClients(rows []integrations.PanelClientView) PanelClientSummary {
	var s PanelClientSummary
	for _, v := range rows {
		s.Total++
		switch {
		case v.Tag == nil:
			s.Untagged++
		case *v.Tag == "FREE":
			s.Free++
		case *v.Tag == "PREMIUM":
			s.Premium++
		}
		if v.Assigned {
			s.Assigned++
		} else {
			s.Unassigned++
		}
		if v.UsagePct >= 100 {
			s.Exhausted++
		}
		if v.DaysLeft == 0 && v.ExpiryTime > 0 {
			s.Expired++
		} else if v.DaysLeft > 0 && v.DaysLeft <= 7 {
			s.ExpiringSoon++
		}
		if !v.Enable {
			s.Disabled++
		}
	}
	return s
}

// filterPanelClients applies the drill-down query string.
//
// Every unknown value answers 422 instead of being silently ignored: a typo that
// returns "everything" looks like a filter that found nothing wrong.
func filterPanelClients(w http.ResponseWriter, r *http.Request, rows []integrations.PanelClientView) ([]integrations.PanelClientView, bool) {
	q := r.URL.Query()
	tag := strings.ToLower(strings.TrimSpace(q.Get("tag")))
	if tag != "" {
		switch tag {
		case "free", "premium", "untagged":
		default:
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"tag must be free, premium or untagged")
			return nil, false
		}
	}
	assigned := strings.ToLower(strings.TrimSpace(q.Get("assigned")))
	if assigned != "" && assigned != "1" && assigned != "0" && assigned != "true" && assigned != "false" {
		WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
			"assigned must be 1 or 0")
		return nil, false
	}
	usage := strings.ToLower(strings.TrimSpace(q.Get("usage")))
	if usage != "" {
		switch usage {
		case "exhausted", "expiring", "expired", "unused", "healthy":
		default:
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"usage must be exhausted, expiring, expired, unused or healthy")
			return nil, false
		}
	}
	search := strings.ToLower(strings.TrimSpace(q.Get("q")))

	kept := make([]integrations.PanelClientView, 0, len(rows))
	for _, v := range rows {
		if tag != "" {
			switch tag {
			case "untagged":
				if v.Tag != nil {
					continue
				}
			case "free":
				if v.Tag == nil || *v.Tag != "FREE" {
					continue
				}
			case "premium":
				if v.Tag == nil || *v.Tag != "PREMIUM" {
					continue
				}
			}
		}
		if assigned == "1" || assigned == "true" {
			if !v.Assigned {
				continue
			}
		}
		if assigned == "0" || assigned == "false" {
			if v.Assigned {
				continue
			}
		}
		switch usage {
		case "exhausted":
			if v.UsagePct < 100 {
				continue
			}
		case "expired":
			if !(v.DaysLeft == 0 && v.ExpiryTime > 0) {
				continue
			}
		case "expiring":
			if !(v.DaysLeft > 0 && v.DaysLeft <= 7) {
				continue
			}
		case "unused":
			if v.UsedBytes != 0 {
				continue
			}
		case "healthy":
			if v.UsagePct >= 100 || (v.DaysLeft == 0 && v.ExpiryTime > 0) || !v.Enable {
				continue
			}
		}
		if search != "" {
			hay := strings.ToLower(v.PanelEmail + " " + v.OwnerEmail + " " + v.OwnerName + " " + v.OfferName)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		kept = append(kept, v)
	}
	return kept, true
}

// TagRoutes registers the panel-client classification and assignment writes.
//
// Split out of IntegrationRoutes so the read-only inspector stays visibly
// read-only: everything in this function changes Veylix's own rows, never the
// panel's state.
func TagRoutes(mux *http.ServeMux, tags TagStore, aud *audit.Logger, authed func(http.Handler) http.Handler) {
	// PUT /api/panel-clients/{server_id}/{email}/tag — classify a panel client as
	// FREE or PREMIUM (D-024.3, D-025.1/.2, step 4).
	//
	// This is a LOCAL write: it records the operator's decision and never talks
	// to the panel, so a panel outage cannot block classification (the whole
	// point of D-024 is that the panel owns the VPN, not the label on it).
	//
	// Refused with 409 once the VPN has been issued: flipping FREE↔PREMIUM after
	// a sale would retroactively change what the customer paid for, so the store
	// enforces the lock in SQL (conditional upsert) and this handler only maps
	// the refusal. The error comes from the database, not from a pre-read, so a
	// concurrent issuance cannot slip through between a check and a write.
	//
	// A refused attempt is audited as DENIED: an attempt to re-price an issued
	// VPN is precisely what an audit trail exists to show.
	mux.Handle("PUT /api/panel-clients/{server_id}/{email}/tag", authed(RequirePermission(auth.PermOfferManage)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverID, err := strconv.ParseInt(r.PathValue("server_id"), 10, 64)
		if err != nil || serverID <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
			return
		}
		// The email is a path segment; an empty one cannot identify a client.
		panelEmail := strings.TrimSpace(r.PathValue("email"))
		if panelEmail == "" {
			WriteError(w, http.StatusBadRequest, "invalid_email", "panel email is required")
			return
		}
		p, _ := PrincipalFrom(r.Context())

		var in struct {
			Tag string `json:"tag"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "body must be {\"tag\":\"FREE\"|\"PREMIUM\"}")
			return
		}
		// Accept lowercase from a UI while storing the canonical uppercase value;
		// anything else is 422 rather than a silent no-op, so a typo cannot look
		// like a successful classification.
		tag := store.Tag(strings.ToUpper(strings.TrimSpace(in.Tag)))
		if tag != store.TagFree && tag != store.TagPremium {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"tag must be FREE or PREMIUM")
			return
		}
		if tags == nil {
			WriteError(w, http.StatusServiceUnavailable, "not_wired", "tagging is not available")
			return
		}

		// Read the previous tag first so the audit entry records the transition
		// (untagged → FREE) rather than only the end state.
		previous := "untagged"
		if cur, cerr := tags.ClientTag(r.Context(), serverID, panelEmail); cerr == nil && cur != nil {
			previous = string(cur.Tag)
		}

		record := func(e audit.Entry) {
			if aud == nil {
				return
			}
			_ = aud.Record(r.Context(), e)
		}

		if err := tags.SetClientTag(r.Context(), serverID, panelEmail, tag, p.UserID); err != nil {
			switch {
			case errors.Is(err, store.ErrTagLocked):
				record(audit.Entry{
					ActorID: p.UserID, Action: audit.ActionPanelClientTagDenied,
					TargetType: "panel_client", TargetID: panelEmail, Result: "DENIED",
					Context: map[string]any{"server_id": serverID, "panel_email": panelEmail,
						"requested": string(tag), "previous": previous},
				})
				WriteError(w, http.StatusConflict, "tag_locked",
					"this VPN is already issued, so its tag is locked")
			default:
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not store the tag")
			}
			return
		}

		record(audit.Entry{
			ActorID: p.UserID, Action: audit.ActionPanelClientTag,
			TargetType: "panel_client", TargetID: panelEmail, Result: "OK",
			Context: map[string]any{"server_id": serverID, "panel_email": panelEmail,
				"previous": previous, "tag": string(tag)},
		})
		WriteJSON(w, http.StatusOK, map[string]any{"tagged": true, "tag": string(tag)})
	}))))
}

// AssignRoutes registers the panel-client assignment (D-037.2, D-063).
//
// A panel client Veylix has never seen — created directly in 3X-UI — is a real,
// working VPN that belongs to nobody. The Super Admin may give it to a client
// account; from that moment it behaves as a normal PREMIUM VPN, with every
// feature of D-038.
//
// Order of operations, and why (D-063):
//  1. the LOCAL rows are created in one transaction — the VPN, its assignment
//     record and the locked tag — **under the name the panel actually holds**;
//  2. a rename into the D-034 scheme happens only when the operator asks for
//     it, and only AFTER the assignment is safely recorded;
//  3. a failed rename changes NOTHING: the VPN keeps its existing name, which is
//     a correct final state, and the answer says so.
//
// Step 1 used to record the row under an invented name and leave the rename to
// step 2 unconditionally. When the rename failed the panel kept the old name and
// the row kept the new one, so the console still offered "Assign" for a client
// somebody already held — and a second click produced a SECOND row for the SAME
// physical VPN. Keeping the panel's name makes the row reachable from the name a
// screen sees, and a rename can no longer break that.
//
// Super-Admin only. Assigning a VPN is an ownership change, which is the single
// most dangerous local write in the system.
func AssignRoutes(mux *http.ServeMux, st AssignmentStore, renamer PanelRenamer,
	aud *audit.Logger, authed func(http.Handler) http.Handler) {

	mux.Handle("POST /api/panel-clients/{server_id}/{email}/assign",
		authed(RequirePermission(auth.PermOfferManage)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serverID, err := strconv.ParseInt(r.PathValue("server_id"), 10, 64)
			if err != nil || serverID <= 0 {
				WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
				return
			}
			source := strings.TrimSpace(r.PathValue("email"))
			if source == "" {
				WriteError(w, http.StatusBadRequest, "validation_failed", "a panel email is required")
				return
			}
			var in struct {
				ClientUserID int64  `json:"client_user_id"`
				Kind         string `json:"kind"`
				OfferID      int64  `json:"offer_id"`
				SellerID     int64  `json:"seller_user_id"`
				// Rename lets the operator opt out of the panel rename; the row is
				// then recorded under the panel's own name, which is legal but
				// breaks the D-034 naming scheme for that one VPN.
				Rename *bool `json:"rename"`
			}
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
				return
			}
			if in.ClientUserID <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "client_user_id is required")
				return
			}
			// D-063: an assignment is always a premium issue. There is no FREE
			// assignment in the product, so accepting the value would only let a
			// caller create a row whose tag and package contradict each other.
			kind := strings.ToLower(strings.TrimSpace(in.Kind))
			switch kind {
			case "", "paid", "premium":
				kind = "paid"
			case "free":
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
					"an assignment is always a premium VPN; a FREE VPN is claimed from an offer, not assigned")
				return
			default:
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "kind must be paid")
				return
			}
			p, _ := PrincipalFrom(r.Context())

			res, err := st.AssignPanelClient(r.Context(), store.AssignPanelClientInput{
				ServerID: serverID, PanelEmail: source, ClientUserID: in.ClientUserID,
				SellerID: in.SellerID, Kind: kind, OfferID: in.OfferID, ActorID: p.UserID,
			})
			if err != nil {
				writeAssignError(w, err)
				return
			}

			record := func(e audit.Entry) {
				if aud == nil {
					return
				}
				_ = aud.Record(r.Context(), e)
			}

			// The assignment is committed. A rename is a separate, optional act
			// that can only improve the name — it can never invalidate the result.
			renamed := false
			renameWarning := ""
			if in.Rename != nil && *in.Rename {
				renamePanel := func(ctx context.Context, from, to string) error {
					if renamer == nil {
						return errors.New("no panel adapter is wired for renames")
					}
					return renamer.RenameClient(ctx, serverID, from, to)
				}
				target, terr := st.RenameAssignment(r.Context(), serverID, res.VPNID, kind, renamePanel)
				if terr != nil {
					// Keep the existing name. This is a finished state, not a
					// half-applied one: the VPN works, is premium, and is
					// reachable by the name its screen shows.
					renameWarning = "the panel did not confirm the rename, so the VPN keeps its existing name"
					record(audit.Entry{
						ActorID: p.UserID, Action: audit.ActionPanelClientTag,
						TargetType: "panel_client", TargetID: res.PanelEmail, Result: "FAILED",
						Context: map[string]any{"server_id": serverID, "vpn_id": res.VPNID,
							"kept_name": res.PanelEmail, "error": renameWarning},
					})
				} else {
					renamed = true
					res.PanelEmail = target
				}
			}

			record(audit.Entry{
				ActorID: p.UserID, Action: audit.ActionPanelClientTag,
				TargetType: "panel_client", TargetID: res.PanelEmail, Result: "OK",
				Context: map[string]any{"server_id": serverID, "panel_email": source,
					"assigned_to": in.ClientUserID, "kind": kind, "vpn_id": res.VPNID,
					"offer_id": res.OfferID, "renamed": renamed},
			})
			WriteJSON(w, http.StatusCreated, map[string]any{
				"vpn_id":      res.VPNID,
				"panel_email": res.PanelEmail,
				"tag":         res.Tag,
				"offer_id":    res.OfferID,
				"renamed":     renamed,
				// `assigned` is unambiguous and true whenever this response is
				// 201. `warning` appears only for a requested rename that did not
				// land; the VPN keeps its existing name and the assignment stands.
				"assigned": true,
				"warning":  renameWarning,
			})
		}))))
}

// writeAssignError maps assignment errors to safe responses.
func writeAssignError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrPanelClientTaken):
		WriteError(w, http.StatusConflict, "panel_client_taken",
			"this panel client is already assigned to an account")
	case errors.Is(err, store.ErrAssignTarget):
		WriteError(w, http.StatusUnprocessableEntity, "not_a_client",
			"a VPN can only be assigned to a client account")
	case errors.Is(err, store.ErrAssignOfferKind):
		WriteError(w, http.StatusUnprocessableEntity, "not_a_premium_offer",
			"an assignment needs a premium package")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", "could not assign this panel client")
	}
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// writeIntegrationError maps service errors to safe HTTP responses.
//
// The messages are operator-facing: an SSRF/validation rejection carries the
// actual reason (e.g. "private address…", "this panel URL is already
// registered") so a failed add/edit is diagnosable. They never contain the
// token, the stored ciphertext or a raw panel response.
func writeIntegrationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, integrations.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "server integration not found")
	case errors.Is(err, integrations.ErrInUse):
		WriteError(w, http.StatusConflict, "server_in_use",
			"this server still has VPN instances; revoke or move them before deleting it")
	case errors.Is(err, integrations.ErrInvalidInput):
		WriteError(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	default:
		// Say what actually failed. The wrapped error never carries the token or
		// the ciphertext (the panel client builds it from the endpoint and HTTP
		// status), and "the panel could not be reached" alone leaves an operator
		// with no way to tell a wrong URL from a rejected key from a 404 endpoint.
		WriteError(w, http.StatusBadGateway, "panel_error", panelFailureMessage(err))
	}
}

// panelFailureMessage turns a wrapped panel error into one diagnosable line.
//
// It is bounded and single-line: enough to act on ("HTTP 404 from
// /panel/api/clients/list/paged"), never enough to leak a secret.
func panelFailureMessage(err error) string {
	msg := strings.TrimSpace(err.Error())
	// Collapse newlines: a multi-line message would break the JSON-one-line style
	// every other error uses.
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	const max = 220
	if len(msg) > max {
		msg = msg[:max] + "…"
	}
	if msg == "" {
		return "the panel could not be reached or rejected the request"
	}
	return msg
}
