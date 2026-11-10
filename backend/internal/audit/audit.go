// Package audit records privileged actions (REQUIREMENTS §8, Bible §8).
//
// Rules:
//   - the log is append-only: no update or delete API is exposed;
//   - only non-secret context is stored (never tokens, passwords or payloads);
//   - writing an entry must never break the business operation, but a failure
//     is reported to the caller so it can be surfaced rather than swallowed.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Action names used across the platform. Keeping them in one place stops log
// drift between handlers.
const (
	ActionLogin                = "auth.login"
	ActionLogout               = "auth.logout"
	ActionRegister             = "auth.register"
	ActionLoginFailed          = "auth.login_failed"
	ActionPasswordChange       = "auth.password_change"
	ActionPasswordChangeFailed = "auth.password_change_failed"
	ActionServerAdd            = "server.add"
	ActionServerUpdate         = "server.update"
	ActionServerTest           = "server.test"
	ActionServerDisable        = "server.disable"
	ActionServerRemove         = "server.remove"
	ActionCredentialChange     = "server.credential_change"
	ActionProvision            = "vpn.provision"
	ActionRevoke               = "vpn.revoke"
	ActionSuspend              = "vpn.suspend"
	ActionResume               = "vpn.resume"
	// ActionVPNDormant records that a customer put a VPN to sleep (D-094). It is
	// a LOCAL preference: no panel client is touched, nothing is deleted, and the
	// VPN keeps reporting the panel's own status. It is audited because it
	// changes what the customer is shown and what gets polled, but it is
	// deliberately NOT notified — putting a VPN to sleep must not generate the
	// very inbox entry the customer was trying to escape.
	ActionVPNDormant = "vpn.dormant"
	// ActionVPNWake records the customer taking a VPN out of dormancy.
	ActionVPNWake = "vpn.wake"
	// ActionVPNStaleReleased records the lifecycle sweep setting a long-inactive
	// VPN aside (D-098). It is NOT a deletion: the row moves to RELEASED, the
	// 3-day undo window opens, and the 3X-UI client is untouched. The context
	// carries the status it held, how long it held it, and the window that was
	// crossed, so a later reader can tell a policy sweep apart from a customer
	// releasing their own VPN.
	ActionVPNStaleReleased = "vpn.stale_released"
	// ActionVPNStaleFailed records a row the sweep could NOT move. The VPN is NOT
	// set aside and the next tick retries it — an entry here is a warning that
	// the intended tidy-up did not happen, never a claim that it did.
	ActionVPNStaleFailed = "vpn.stale_failed"
	// ActionVPNAbsent records that the panel positively reported a VPN's client
	// gone, and the local status was reconciled to SERVER_UNAVAILABLE (D-024.5).
	ActionVPNAbsent = "vpn.absent"
	// ActionVPNRemoveAbsent records the removal of Veylix's record for a VPN
	// whose panel client is gone. The panel client is never deleted by this
	// (D-027.3), so the entry states that explicitly.
	ActionVPNRemoveAbsent   = "vpn.remove_absent"
	ActionQuotaAdjust       = "admin.quota_adjust"
	ActionAdminServerAssign = "admin.servers_assign"
	ActionOfferCreate       = "offer.create"
	ActionOfferUpdate       = "offer.update"
	// ActionOfferArchive records retiring an offer from the catalogue (P-4). The
	// row survives; this marks WHY it left the shop, which `active = false` alone
	// never recorded.
	ActionOfferArchive = "offer.archive"
	// ActionOfferDelete records removing an offer nothing depended on (P-4).
	ActionOfferDelete = "offer.delete"
	// ActionPanelClientTag records a FREE/PREMIUM (re)classification of a panel
	// client (D-024.3/.7). The context carries the server, the panel email, the
	// previous tag ("untagged" when there was none) and the new one, so a later
	// reader can reconstruct exactly what changed without reading the table.
	ActionPanelClientTag = "panel_client.tag"
	// ActionPanelClientHwidRevoked records that a Super Admin freed one device
	// slot on a panel client (D8). It is a real change on the panel — a device
	// that was blocked can connect again — so it is audited separately from
	// tagging, which never touches the panel. The context carries the device's
	// short fingerprint, never a full hardware hash (the panel does not expose
	// one, and Veylix must not invent what it cannot verify).
	ActionPanelClientHwidRevoked = "panel_client.hwid_revoked"
	// ActionPanelClientTagDenied records that tagging was REFUSED because the
	// client is already issued. This is a business-rule refusal, not a fault,
	// so it is audited with Result DENIED — an attempt to change what a
	// customer already paid for is exactly what an audit trail must show.
	ActionPanelClientTagDenied = "panel_client.tag_denied"
	// ActionVPNTransfer records that an assigned VPN changed hands (D-064).
	// Separate from ActionPanelClientTag (a classification) and from
	// ActionProvision (a new panel client): this write moves OWNERSHIP, which is
	// the single most consequential local change in the system, so it must be
	// filterable on its own. The context names both parties and the reason.
	ActionVPNTransfer      = "vpn.transfer"
	ActionPaymentConfirm   = "payment.confirm"
	ActionPaymentReject    = "payment.reject"
	ActionPaymentRefund    = "payment.refund"
	ActionPaymentCreate    = "payment.create"
	ActionPaymentCancel    = "payment.cancel"
	ActionPaymentReference = "payment.reference"
	// ActionPaymentUpdate records a change to an ORDER's own facts — the chosen
	// destination, the proof channel, or the block on the VPN it produced
	// (D-036.4/.5). Separate from `payment.confirm` so the audit trail can tell
	// "who moved this order" apart from "who settled it".
	ActionPaymentUpdate       = "payment.update"
	ActionPaymentMethodCreate = "payment_method.create"
	ActionPaymentMethodUpdate = "payment_method.update"
	// ActionChatLock records a staff member locking or unlocking a conversation.
	// It changes what the other participant may do, so it must be attributable.
	ActionChatLock = "chat.lock"
	// ActionChatPin records a staff member pinning or unpinning a message.
	ActionChatPin = "chat.pin"
	// ActionVPNRelease records a client releasing a depleted VPN. The VPN is NOT
	// deleted: a 3-day undo window opens (D-027.2).
	ActionVPNRelease = "vpn.release"
	// ActionVPNForceRelease records a SUPER ADMIN releasing somebody else's VPN
	// (P-2). A separate action, not a flag on vpn.release: "the owner set this
	// aside" and "an operator set this aside on the owner's behalf" are different
	// events, and the difference is exactly what gets asked about months later.
	// It carries a mandatory reason.
	ActionVPNForceRelease = "vpn.force_release"
	// ActionVPNForcePurge records a SUPER ADMIN deleting a local VPN RECORD while
	// the panel client deliberately stays alive (P-3, D-027.3). Distinct from
	// vpn.remove_absent, where the panel positively reported the client gone: here
	// the client exists and was not touched, so the trail must not imply otherwise.
	ActionVPNForcePurge = "vpn.force_purge"
	// ActionVPNRestore records the undo of a release inside that window.
	ActionVPNRestore = "vpn.restore"
	// ActionVPNPurge records the LOCAL removal of a released VPN after the grace
	// period. The 3X-UI client is deliberately left intact (D-027.3/.5), and the
	// entry says so.
	ActionVPNPurge   = "vpn.purge"
	ActionRoleChange = "user.role_change"
	// ActionSettingsUpdate records a change to an operator-editable setting
	// (D-080), e.g. the Google OAuth client. The secret itself is never in the
	// entry: only whether one was set.
	ActionSettingsUpdate = "settings.update"
)

// Entry is one audit record.
type Entry struct {
	ActorID    int64
	Action     string
	TargetType string
	TargetID   string
	Result     string // OK | DENIED | ERROR
	Context    map[string]any
}

// Notifier delivers one inbox entry. The audit package declares it so a logger
// can fan out without importing the store (which would make the dependency
// circular: every service imports audit).
type Notifier interface {
	Notify(ctx context.Context, userID int64, kind, title, body string) error
	// Admins returns the ids of active reseller accounts, so an operator event
	// can reach the people who run the platform.
	Admins(ctx context.Context) ([]int64, error)
}

// Logger writes audit entries.
type Logger struct {
	db *sql.DB
	// notifier is optional: a nil one simply means "no inbox yet", which is the
	// state of most unit tests.
	notifier Notifier
}

// New returns an audit logger bound to the database.
func New(db *sql.DB) *Logger { return &Logger{db: db} }

// WithNotifier attaches an inbox fan-out.
//
// Chosen deliberately over "every service calls CreateNotification": the audit
// logger already sees EVERY privileged action with its actor, so one wiring
// point turns the whole platform into notification producers instead of a dozen
// call sites that would silently drift apart.
func (l *Logger) WithNotifier(n Notifier) *Logger {
	l.notifier = n
	return l
}

// Record appends one entry.
//
// The context map is JSON-encoded after removing any key that looks secret, so
// a careless caller cannot leak a token into the audit trail.
func (l *Logger) Record(ctx context.Context, e Entry) error {
	if e.Action == "" {
		return fmt.Errorf("audit: action is required")
	}
	if e.Result == "" {
		e.Result = "OK"
	}
	ctxJSON := "{}"
	if len(e.Context) > 0 {
		raw, err := json.Marshal(sanitise(e.Context))
		if err != nil {
			return fmt.Errorf("audit: encode context: %w", err)
		}
		ctxJSON = string(raw)
	}

	var actor any
	if e.ActorID > 0 {
		actor = e.ActorID
	}

	_, err := l.db.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, context, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		actor, e.Action, e.TargetType, e.TargetID, e.Result, ctxJSON, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	l.notify(ctx, e)
	return nil
}

// notify turns an audit entry into inbox entries.
//
// Rules (REQUIREMENTS §2.8 — "the account is told about things that concern it"):
//   - every FAILED privileged action tells the ACTOR, because "your action did
//     not happen" is the single most useful message a user never receives;
//   - account-level events (role change, quota) tell the affected account;
//   - server-level events (added, revoked, unavailable) tell the actor AND, when
//     it was an operator decision, every reseller — a reseller whose server was
//     removed needs to know before a client asks them why.
//
// Delivery is best-effort: a notification must never roll back the operation it
// describes, so an error here is dropped rather than returned.
func (l *Logger) notify(ctx context.Context, e Entry) {
	if l.notifier == nil || l.db == nil {
		return
	}
	kind, title := describe(e)
	if title == "" {
		return
	}

	// A refused SIGN-IN is never pushed into the account's inbox (I-38).
	//
	// Two reasons, and both matter:
	//   1. Noise. Every typo, every password manager retry, every bot probing
	//      the login form would append an identical "Action not applied" row.
	//      A real inbox became 16 copies of one meaningless line.
	//   2. Disclosure. The row is written against the SUBJECT, so an attacker
	//      guessing passwords can watch the victim's inbox fill and learn that
	//      the account exists and that somebody is trying it — without ever
	//      signing in. A rejected sign-in therefore stays in the audit log
	//      (where an operator investigates it) and out of the inbox.
	if isSignInFailure(e.Action) {
		return
	}

	targets := map[int64]bool{}
	if e.ActorID > 0 {
		// The actor always hears about their own failures; successes only when
		// they are the subject of the event (see below).
		if e.Result != "" && e.Result != "OK" {
			targets[e.ActorID] = true
		}
	}

	switch {
	case strings.HasPrefix(e.Action, "user.") || e.Action == ActionQuotaAdjust:
		// The SUBJECT is who needs to know, not the operator.
		if id, ok := parseID(e.TargetID); ok {
			targets[id] = true
		}
	case e.Action == ActionCredentialChange || e.Action == ActionVPNAbsent,
		e.Action == ActionServerDisable, e.Action == ActionServerRemove:
		admins, err := l.notifier.Admins(ctx)
		if err == nil {
			for _, id := range admins {
				targets[id] = true
			}
		}
	}
	if len(targets) == 0 && e.ActorID > 0 && e.Result == "OK" {
		// A successful action still deserves a receipt when it changed something
		// the actor will want to point at later (provisioning, payments).
		switch e.Action {
		case ActionProvision, ActionRevoke, ActionSuspend, ActionResume,
			ActionPaymentConfirm, ActionPaymentReject, ActionPaymentRefund,
			ActionPaymentMethodCreate, ActionPaymentMethodUpdate,
			ActionOfferCreate, ActionOfferUpdate, ActionOfferArchive, ActionOfferDelete,
			ActionServerAdd, ActionServerDisable, ActionServerRemove:
			targets[e.ActorID] = true
		}
	}

	body := summarise(e)
	for id := range targets {
		_ = l.notifier.Notify(ctx, id, kind, title, body)
	}
}

// isSignInFailure reports the audit actions that describe a REFUSED SIGN-IN
// rather than a refused business action. They are logged, never notified (I-38).
func isSignInFailure(action string) bool {
	switch action {
	case ActionLoginFailed:
		return true
	}
	return false
}

// describe maps an action to an inbox kind and a human title.
// An empty title means "this action is not worth an inbox entry".
func describe(e Entry) (kind, title string) {
	if e.Result != "" && e.Result != "OK" {
		// A refusal still has to read like a sentence to the person who caused
		// it. "Action not applied" was the only failure title, so a refused
		// provisioning, a refused payment and a refused tag all arrived with
		// identical, meaningless wording (I-38).
		switch e.Action {
		case ActionProvision:
			return "error", "VPN could not be created"
		case ActionRevoke:
			return "error", "VPN could not be revoked"
		case ActionSuspend:
			return "error", "VPN could not be suspended"
		case ActionResume:
			return "error", "VPN could not be resumed"
		case ActionPaymentConfirm, ActionPaymentReject:
			return "error", "Payment decision was not applied"
		case ActionOfferCreate, ActionOfferUpdate:
			return "error", "Offer was not saved"
		case ActionOfferArchive:
			return "warning", "Offer archived — removed from the catalogue"
		case ActionOfferDelete:
			return "warning", "Offer deleted"
		case ActionQuotaAdjust:
			return "error", "Quota change was not applied"
		case ActionPasswordChangeFailed:
			return "error", "Password change was refused"
		default:
			return "error", "Your request was refused"
		}
	}
	switch e.Action {
	case ActionServerAdd:
		return "success", "Server added"
	case ActionServerTest:
		return "info", "Panel health check"
	case ActionServerUpdate:
		return "info", "Server updated"
	case ActionServerDisable:
		return "warning", "Server disabled"
	case ActionServerRemove:
		return "warning", "Server removed"
	case ActionCredentialChange:
		return "warning", "Panel credentials changed"
	case ActionVPNAbsent:
		return "error", "VPN missing on the panel"
	case ActionVPNRelease:
		return "warning", "VPN released — 3 days to undo"
	case ActionVPNForceRelease:
		return "warning", "VPN released by an operator — 3 days to undo"
	case ActionVPNForcePurge:
		return "warning", "VPN record purged — the panel client was kept"
	case ActionVPNRestore:
		return "success", "VPN restored"
	case ActionVPNPurge:
		return "info", "Released VPN record removed"
	case ActionVPNRemoveAbsent:
		return "info", "VPN record removed"
	case ActionVPNTransfer:
		return "warning", "VPN moved to another customer"
	case ActionProvision:
		return "success", "VPN created"
	case ActionRevoke:
		return "info", "VPN revoked"
	case ActionSuspend:
		return "warning", "VPN suspended"
	case ActionResume:
		return "success", "VPN resumed"
	case ActionQuotaAdjust:
		return "info", "Your quota changed"
	case ActionAdminServerAssign:
		return "info", "Server assignments updated"
	case ActionOfferCreate:
		return "success", "Offer created"
	case ActionOfferUpdate:
		return "info", "Offer updated"
	case ActionOfferArchive:
		return "info", "Offer archived"
	case ActionOfferDelete:
		return "info", "Offer deleted"
	case ActionPaymentConfirm:
		return "success", "Payment confirmed"
	case ActionPaymentReject:
		return "warning", "Payment rejected"
	case ActionPaymentRefund:
		return "info", "Payment refunded"
	case ActionPaymentMethodCreate:
		return "success", "Payment method added"
	case ActionPaymentMethodUpdate:
		return "info", "Payment method updated"
	case ActionChatLock:
		return "info", "Conversation locked state changed"
	case ActionChatPin:
		return "info", "Pinned message changed"
	case ActionRoleChange:
		return "warning", "Your account role changed"
	default:
		return "", ""
	}
}

// summarise builds a one-line body from the non-secret context.
//
// The body is read by a PERSON in an inbox, so it never prints raw identifiers
// or audit vocabulary. "user 15 · result=DENIED" was the visible text on every
// refused action (I-38); a customer cannot act on a row that names their own
// numeric id back at them.
func summarise(e Entry) string {
	parts := make([]string, 0, 3)
	// A readable subject: the panel client e-mail or a named server, never a
	// bare "user 15".
	if e.TargetType != "" && e.TargetID != "" {
		switch e.TargetType {
		case "user":
			// The reader IS this user; naming them adds nothing.
		case "panel_client":
			parts = append(parts, e.TargetID)
		default:
			parts = append(parts, e.TargetType+" "+e.TargetID)
		}
	}
	for _, k := range []string{"kind", "server_id", "name", "status", "amount_cents"} {
		if v, ok := e.Context[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	// `result=` is audit vocabulary; the title already says it was refused.
	return strings.Join(parts, " · ")
}

// parseID reads a target id without caring whether it is numeric.
func parseID(s string) (int64, bool) {
	var id int64
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// secretKeys are dropped from context before persisting.
var secretKeys = map[string]bool{
	"token": true, "api_token": true, "authorization": true, "password": true,
	"secret": true, "encryption_key": true, "session": true, "cookie": true,
	"token_ciphertext": true,
}

func sanitise(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if secretKeys[k] {
			out[k] = "[redacted]"
			continue
		}
		out[k] = v
	}
	return out
}
