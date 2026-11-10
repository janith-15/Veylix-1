package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/provisioning"
)

// Health is the readiness surface the router needs from the store layer.
// It is an interface so the transport layer never imports the database driver.
type Health interface {
	Ping(ctx context.Context) error
	SchemaVersion(ctx context.Context) (int, error)
}

// Deps are the collaborators the router wires together. Nil entries disable the
// corresponding route group, so the binary can boot in a reduced mode and the
// surface never advertises an endpoint it cannot serve.
type Deps struct {
	// PublicOrigins are extra origins accepted by the CSRF check, e.g.
	// "https://panel.example.com". The request's own origin is always accepted,
	// so this only matters behind a proxy that presents a different host.
	PublicOrigins []string
	Health        Health
	Version       string
	// System answers "which release is running, what else exists, and is there
	// something newer?". Nil still exposes GET /api/system/version from Version
	// alone, and answers 503 for the release feed rather than 404.
	System       SystemInfoService
	Auth         AuthService
	Sessions     SessionManager
	Authn        Authenticator
	Integrations IntegrationService
	// PanelInspector reads a panel's client list for the read-only inspector.
	// Nil disables GET /api/servers/{id}/panel-clients (503, never an empty list).
	PanelInspector PanelInspector
	// PanelTags classifies panel clients FREE/PREMIUM (D-024.3).
	// Nil disables PUT /api/panel-clients/{server_id}/{email}/tag (503).
	PanelTags TagStore
	// Assignments creates the Veylix record for a panel client nobody holds
	// (D-037.2). Nil disables POST /api/panel-clients/{server_id}/{email}/assign.
	Assignments AssignmentStore
	// PanelRenamer renames the panel client so it joins the D-034 naming scheme.
	// Nil keeps the assignment local-only (no rename on the panel).
	PanelRenamer PanelRenamer
	// PanelTagAudit records tag writes and refusals. Nil tolerates the write
	// but leaves no trail, so production always sets it.
	PanelTagAudit *audit.Logger
	// Transfers moves an itemised VPN from one client account to another
	// (D-064). Nil disables POST /api/vpns/{id}/transfer and answers 503 —
	// never a silent no-op.
	Transfers TransferStore
	Servers   ServerStore
	Audit     AuditStore
	Quota     QuotaStore
	VPNs      VPNService
	VPNLister VPNLister
	// PanelVPNLister serves GET /api/vpns for the Super Admin, where the panel's
	// clients ARE the catalogue (D-024.3): failing to set it means the Super
	// Admin sees local rows only, so a panel-only client would not appear.
	PanelVPNLister VPNLister
	// FreeClaims reads the FREE claim ledger (D-025.4/.5). Nil disables
	// GET /api/vpns/claims/me and leaves the monthly cap at the policy default.
	FreeClaims FreeClaimStore
	Usage      UsageStore
	// PingSamples stores reachability probes (D-092). Nil keeps the ping route
	// working and simply has no latency history to plot.
	PingSamples PingSampleStore
	// SyncRunner runs a sync pass on demand (POST /api/sync/run). Nil leaves the
	// route unregistered, so a build without the scheduler still serves.
	SyncRunner SyncRunner
	// Offers is the catalogue (P3). Nil disables /api/offers entirely.
	Offers OfferStore
	// OfferAudit records offer writes. Nil is tolerated (the writes still work)
	// but then no trail is written, so production always sets it.
	OfferAudit *audit.Logger
	// Clients lists the client accounts a caller may act on (P5).
	Clients ClientStore
	// AdminAudit records assignment writes.
	AdminAudit *audit.Logger
	// SyncStaleAfter is the freshness window reported by /api/sync/status.
	SyncStaleAfter time.Duration
	// DisplayTimezone is the zone every screen renders an absolute instant in
	// (D-099), published through /api/me. Empty means "do not advertise one",
	// and the UI then falls back to the zone the browser reports — which is the
	// pre-D-099 behaviour rather than a guessed offset.
	DisplayTimezone string
	UserAdmin       UserAdminService
	SetupState      SetupStateReader
	// Accounts reads the safe projection of one account so GET /api/me can
	// answer the display name and creation date (D1/D2). Nil degrades that
	// route to the session's own facts — never to a wrong account.
	Accounts AccountReader
	// Hwids lists and revokes a panel client's registered devices (D8). Nil
	// disables both device routes (503), never an empty list.
	Hwids HwidService
	// Notifications backs the per-account inbox (P8). Nil disables
	// /api/notifications* entirely.
	Notifications NotificationStore
	// Directory lists the accounts a caller may address (chat picker).
	// Nil disables GET /api/directory.
	Directory DirectoryStore
	// Payments backs billing (P6). Nil disables every /api/payment* route.
	Payments PaymentStore
	// Orders is the D-031 settlement path: one confirmation that also builds the
	// VPN. Nil keeps payments at settlement-only, so a deployment that has not
	// wired provisioning can still record money — but a plan order then answers
	// a plain confirmed payment with no VPN, which is the pre-D-031 behaviour.
	Orders OrderService
	// Plans backs the plan catalogue (D-030): prices, slots, archive, delete.
	// Nil disables /api/plans*.
	Plans PlanStore
	// Pricing backs the unit rate card and the CUSTOM quote (D-035.4). Nil
	// disables /api/pricing and /api/orders/quote.
	Pricing PricingStore
	// Chat backs the internal support chat (P7). Nil disables /api/chat*.
	Chat ChatStore
	// OrderActions backs the D-036 order actions: destination change, proof
	// channel, timeline and block/unblock. Nil disables those routes.
	OrderActions OrderStore
	// OrderBlocker stops and restarts the panel client behind an order.
	// Nil keeps block/unblock local-only, which the route reports honestly.
	OrderBlocker OrderVPNBlocker
	// ReleaseTargets records which panel a CUSTOM order will be built on
	// (D-036.6). Nil disables POST /api/payments/{id}/release-target.
	ReleaseTargets ReleaseTargetStore
	// VPNConnection backs the links/ping routes (D-026). Nil disables
	// GET /api/vpns/{id}/links and POST /api/vpns/{id}/ping.
	VPNConnection VPNConnectionService
	// PanelPing measures server reachability for POST /api/vpns/{id}/ping.
	// Nil leaves latency unmeasured rather than inventing one.
	PanelPing provisioning.PanelPinger
	// Releases backs the D-027 release/restore lifecycle. Nil disables
	// /api/vpns/{id}/release, /api/vpn-releases* entirely.
	Releases ReleaseService
	// Google is the "Continue with Google" flow (D-039). Nil (or unconfigured)
	// keeps /api/auth/google/start answering 503, never a fake success.
	Google GoogleAuthenticator
	// Settings is the operator-editable settings surface (D-080), e.g. the Google
	// OAuth client. Nil keeps /api/super/settings/* absent rather than fake.
	Settings SettingsService
	// EmailVerifier issues and redeems e-mail verification tokens. Nil disables
	// /api/auth/verify* (the routes then 404 rather than pretend to send mail).
	EmailVerifier EmailVerificationService
	// Profile backs the self-service account surface (D-041): name, username,
	// contacts, avatar, bio and the e-mail-change flow. Nil disables it.
	Profile ProfileStore
	// Mail delivers e-mail (verification, change codes). Nil means "no relay": the
	// routes then report `delivery: "outbox"` instead of claiming a send.
	Mail EmailSender
	// PasswordResets backs the forgotten-password code flow (D-041). Nil disables
	// /api/auth/forgot and /api/auth/reset (reset then answers 503).
	PasswordResets PasswordResetStore
	// SupportEmail receives a copy of a super admin's password reset, so the
	// highest-privilege account stays recoverable (owner 2026-10-29).
	SupportEmail string
}

// Router builds the HTTP handler tree.
//
// Routes exist only where behaviour is real: an endpoint is added in the same
// commit as its handler (`new_update.md` §13 — no fake surface).
func Router(deps Deps) http.Handler {
	mux := http.NewServeMux()

	// Liveness: the process is up. Never touches dependencies.
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "veylix",
			"version": deps.Version,
		})
	})

	// Readiness: dependencies are usable. Reports the schema version so an
	// operator can see which migrations are applied.
	mux.HandleFunc("GET /api/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if err := deps.Health.Ping(ctx); err != nil {
			WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "database is not reachable")
			return
		}
		schema, err := deps.Health.SchemaVersion(ctx)
		if err != nil {
			WriteError(w, http.StatusServiceUnavailable, "db_schema_unavailable", "database schema is not readable")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":         "ready",
			"schema_version": schema,
			// `version` is the build's own version, so an operator (or
			// `vpn-platformctl status`) can answer "which release is actually
			// running?" from the running process rather than by trusting a
			// filename. An unset build leaves "dev", never an empty string.
			"version": buildVersion(deps.Version),
			// `ui` tells an operator whether this binary carries the interface
			// (E7). A build without it serves the API only, and every UI path
			// answers 404 — worth knowing before reporting "the app is broken".
			"ui": spaStatusText(),
		})
	})

	// Auth + account routes are only mounted when the services are wired.
	if deps.Auth != nil && deps.Sessions != nil && deps.Authn != nil {
		authed := RequireAuth(deps.Authn)
		// Version, release list and update check. Mounted inside the auth block
		// because even GET /api/system/version is behind a session: it names the
		// exact build, which is the first thing an attacker wants.
		SystemRoutes(mux, deps.System, authed, deps)
		// Operator-editable settings (D-080): the Google OAuth client, edited in
		// the UI and applied live. Super Admin only, mounted here so it shares the
		// same session gate as the rest of the management surface.
		SettingsRoutes(mux, deps.Settings, authed)
		if deps.SetupState != nil {
			SetupStatusRoutes(mux, deps.SetupState)
		}
		AuthRoutes(mux, deps.Auth, deps.Sessions, authed,
			// Sign-ups send mail, so they get their own bucket: three burst, then
			// one a minute. A shared bucket with login would let a spray of
			// sign-ups lock out real sign-ins.
			auth.NewRateLimiter(3, 1.0/60.0),
			deps.Mail,
			deps.Accounts,
			deps.DisplayTimezone)
		// Google sign-in/sign-up and e-mail verification (D-039). Mounted in the
		// same block because both mint a session through the same manager.
		GoogleRoutes(mux, deps.Google, deps.Sessions, deps.EmailVerifier, authed)
		// D-078: the PUBLIC "send me a new confirmation link" route. The
		// authenticated one above needs a session, and an unverified account can
		// never get one — so without this the dead end was complete: sign up, be
		// refused at the login door, and have no way to ask for another link.
		if deps.EmailVerifier != nil {
			// `auth.Service` implements this; the assertion is explicit rather
			// than silent so a missing wiring shows up here, not as a 404 on a
			// route a locked-out customer is told to use.
			if resender, ok := deps.UserAdmin.(VerificationResender); ok && resender != nil {
				AuthVerifyResendRoutes(mux, resender, deps.EmailVerifier)
			}
		}
		// D-041: the account's own profile. Mounted here because it mints a session
		// (an e-mail change rotates it) and needs the password verifier.
		if deps.Profile != nil {
			ProfileRoutes(mux, deps.Profile, deps.Mail, deps.AdminAudit, deps.Sessions,
				deps.Auth.VerifyPassword, authed)
		}
		// D-041: forgotten password. Its own limiter, because the login limiter is
		// tuned for sign-in attempts and a reset is a different rate of traffic.
		if deps.PasswordResets != nil {
			PasswordResetRoutes(mux, deps.Auth, deps.PasswordResets, deps.Mail,
				deps.AdminAudit, deps.SupportEmail,
				auth.NewRateLimiter(resetAttemptBurst, resetAttemptRate))
		}

		if deps.Integrations != nil && deps.Servers != nil {
			// The inspector is optional: nil disables the route, which then
			// answers 503 rather than pretending the panel has no clients.
			var inspector PanelInspector
			if deps.PanelInspector != nil {
				inspector = deps.PanelInspector
			}
			IntegrationRoutes(mux, deps.Integrations, deps.Servers, inspector, deps.PanelTags,
				deps.PanelTagAudit, authed)
			// Tagging and assignment are writes; they live in their own
			// registration so the inspector above stays visibly read-only.
			TagRoutes(mux, deps.PanelTags, deps.PanelTagAudit, authed)
			// D8: the device (HWID) list of a panel client and the revocation of
			// one device. Gated on `deps.Hwids` alone, so an install that wired
			// the panel but not tagging can still free a device slot. (It sits
			// inside the panel block because a device lives on a panel.)
			if deps.Hwids != nil {
				HwidRoutes(mux, deps.Hwids, deps.PanelTagAudit, authed)
			}
		}
		// Assignment only needs the local store (plus the panel renamer), so it is
		// gated separately: a deployment without the inspector can still give an
		// already-known panel client to a customer.
		if deps.Assignments != nil && deps.PanelTags != nil {
			AssignRoutes(mux, deps.Assignments, deps.PanelRenamer, deps.PanelTagAudit, authed)
		}
		// Transferring an ALREADY-assigned VPN (D-064). Registered separately
		// from assignment because the two answer different questions: an
		// assignment claims a panel client nobody holds, a transfer moves one
		// that somebody already holds. Needs only the local store — it makes no
		// panel call at all, so it keeps working during a panel outage.
		if deps.Transfers != nil {
			TransferRoutes(mux, deps.Transfers, deps.PanelTagAudit, authed)
		}
		if deps.Audit != nil && deps.Quota != nil {
			AccountRoutes(mux, deps.Audit, deps.Quota, authed)
		}
		if deps.VPNs != nil && deps.VPNLister != nil {
			// The Super Admin's list is panel-backed; everyone else's is local.
			// Falling back to the local lister keeps the route alive when the
			// panel lister was not wired, and the rows then say Source=local.
			superLister := deps.PanelVPNLister
			if superLister == nil {
				superLister = deps.VPNLister
			}
			VPNRoutes(mux, deps.VPNs, deps.VPNLister, superLister, deps.FreeClaims, authed)
			if deps.VPNConnection != nil {
				VPNConnectionRoutes(mux, deps.VPNConnection, deps.PanelPing,
					deps.PingSamples, authed)
			}
			if deps.Releases != nil {
				ReleaseRoutes(mux, deps.Releases, authed)
				// D-112: replace is one action — build the new VPN, then
				// release the old one. Only registered when both halves exist,
				// because a replace that cannot release is not a replace.
				ReplaceRoutes(mux, provisioningReplacer(deps.VPNs), deps.Releases, deps.FreeClaims, authed)
			}
		}
		if deps.UserAdmin != nil {
			AdminRoutes(mux, deps.UserAdmin, deps.AdminAudit, authed)
		}
		if deps.Clients != nil {
			ClientRoutes(mux, deps.Clients, authed)
		}
		// D-042: the Super Admin's client console. It needs the real store (detail,
		// block, per-client FREE limit), and it is gated by PermClientManage so a
		// reseller can never reach it.
		if full, ok := deps.Clients.(ClientAdminStore); ok && full != nil {
			// deps.Auth also implements ResetStaffPassword, so the console can issue
			// a customer a new password — the only recovery path when their mailbox
			// is wrong or gone and both reset and verification are e-mails.
			ClientAdminRoutesWithReset(mux, full, deps.AdminAudit, authed, deps.Auth)
			// D-077: and it can correct the ADDRESS itself, through the same
			// code-to-the-new-mailbox flow the customer would use. Without this,
			// "their e-mail is changed by them" was the console's only answer to
			// the one customer who cannot do that.
			if changer, ok := deps.Clients.(ClientEmailChanger); ok && changer != nil {
				ClientEmailChangeRoutes(mux, changer, deps.Mail, deps.AdminAudit, authed)
			}
		}
		// D-043: the VPN management console. It reuses the Super Admin's merged
		// panel-backed list, and reads per-server panel facts once per request.
		//
		// It must be `PanelVPNLister`, not `VPNLister`: the local lister only knows
		// rows Veylix wrote, so a client created directly in 3X-UI — exactly what
		// the console exists to surface — never appeared. This used to pass
		// `deps.VPNLister`, which is why the owner saw a handful of VPNs while the
		// panel held fourteen.
		superLister := deps.PanelVPNLister
		if superLister == nil {
			superLister = deps.VPNLister
		}
		if admin, ok := superLister.(adminVPNLister); ok && admin != nil {
			AdminVPNRoutes(mux, admin, deps.PanelInspector, authed)
		}
		// D-044: the staff console (admins + super admins). Needs both the read
		// store and the privileged account service, so it is gated on the pair.
		if staffStore, ok := deps.Clients.(StaffAdminStore); ok && staffStore != nil {
			if staffSvc, ok := deps.UserAdmin.(staffService); ok && staffSvc != nil {
				StaffAdminRoutes(mux, staffStore, staffSvc, deps.AdminAudit, authed)
			}
		}

		/*
		 * D-107: the Super Admin overview. One route that answers "is the business
		 * alive, and what must I do today?" — money and the reseller book, which no
		 * route exposed before. It is gated on the store implementing BOTH reads, so
		 * a partial store leaves the route absent rather than answering half a page.
		 */
		if ov, ok := deps.Clients.(OverviewStore); ok && ov != nil {
			OverviewRoutes(mux, ov, authed)
		}
		if deps.Offers != nil {
			OfferRoutes(mux, deps.Offers, deps.OfferAudit, authed)
		}
		if deps.Usage != nil {
			staleAfter := deps.SyncStaleAfter
			if staleAfter <= 0 {
				staleAfter = 15 * time.Minute
			}
			UsageRoutes(mux, deps.Usage, staleAfter, authed)
			SyncRoutes(mux, deps.SyncRunner, authed)
		}
		if deps.Notifications != nil {
			NotificationRoutes(mux, deps.Notifications, authed)
		}
		if deps.Directory != nil {
			DirectoryRoutes(mux, deps.Directory, authed)
		}
		if deps.Payments != nil {
			PaymentRoutes(mux, deps.Payments, deps.Orders, deps.AdminAudit, authed)
		}
		if deps.Plans != nil {
			PlanRoutes(mux, deps.Plans, deps.AdminAudit, authed)
		}
		if deps.Pricing != nil {
			PricingRoutes(mux, deps.Pricing, deps.AdminAudit, authed)
		}
		if deps.OrderActions != nil {
			OrderRoutes(mux, deps.OrderActions, deps.OrderBlocker, deps.AdminAudit, authed)
			BlockRoutes(mux, deps.OrderActions, deps.OrderBlocker, deps.AdminAudit, authed)
		}
		if deps.ReleaseTargets != nil {
			ReleaseTargetRoutes(mux, deps.ReleaseTargets, deps.AdminAudit, authed)
		}
		if deps.Chat != nil {
			ChatRoutes(mux, deps.Chat, deps.AdminAudit, authed)
		}
	}

	// Anything else under /api is a real 404 in the standard envelope.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
	})

	// The built single-page app, when one is embedded (E7). This is mounted LAST
	// and matches "/" so it only sees paths the API did not claim; `net/http`
	// gives a more specific pattern priority, so /api/* can never be swallowed
	// by the SPA fallback.
	//
	// Without a build the route is absent, and an unknown path answers the JSON
	// 404 below rather than a blank page — a binary with no UI must say so.
	if assets, ok := spaFS(); ok {
		mux.Handle("/", spaHandler(assets))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			WriteError(w, http.StatusNotFound, "not_found",
				"not found — this binary was built without the user interface (run .\\dev.ps1 -Package)")
		})
	}

	// Order matters: the access log wraps EVERYTHING (including SameOrigin's
	// refusals and withRecover's 500s), so a rejected cross-origin request and a
	// panicked handler both appear in the log with their real status.
	return LogRequests(withRecover(SameOrigin(deps.PublicOrigins...)(mux)))
}

// withRecover converts a panic into a 500 without leaking stack details.
//
// The panic is LOGGED before it is swallowed. A recovery that only writes 500 is
// indistinguishable from a handler that decided to fail: the operator sees one
// failed request, no stack, no route, no trace — and the next occurrence is
// diagnosed by guessing. The log line carries the method, the path and the panic
// value, which is everything needed to reproduce it; the RESPONSE carries none of
// that, because a stack trace is an internal detail an anonymous caller must
// never receive.
func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				// `http.ErrAbortHandler` is the documented way for a handler to
				// abandon a response mid-stream (a client disconnect on a long
				// download). It is not a bug, it is not loggable noise, and the
				// server deliberately suppresses it — so suppressing it here too
				// keeps the log honest about real panics.
				if p == http.ErrAbortHandler {
					return
				}
				slog.Error("httpapi: handler panic",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprint(p),
					"stack", string(debug.Stack()),
				)
				WriteError(w, http.StatusInternalServerError, "internal_error", "unexpected server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
