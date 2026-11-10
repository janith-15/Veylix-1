// Command veylixd is the Veylix backend server.
//
// Startup order matters: configuration (fail closed on missing secrets) →
// data directory → database open → migrations → HTTP listen → graceful
// shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/auth/google"
	"veylix/backend/internal/config"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/integrations"
	"veylix/backend/internal/mail"
	"veylix/backend/internal/payments"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/releases"
	"veylix/backend/internal/settings"
	"veylix/backend/internal/store"
	"veylix/backend/internal/sync"
)

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	// Never log secret material: Config.String() reports lengths only.
	slog.Info("configuration loaded", "config", cfg.String())
	// Authentication is always a real session (D-039). This line exists because
	// its absence used to be the confusing case: a local API that answers 401 to
	// everything is a working login, not a broken build.
	//
	// It must NOT mention the seed binaries. `veylixd` ships with an EMPTY
	// database by design — `cmd/seeddemo` / `cmd/seedacct` are developer tools
	// and are not imported here — so pointing an operator at seeded accounts on
	// a production host is advice that cannot work. A fresh install creates its
	// first account through /api/setup/bootstrap instead.
	slog.Info("authentication enforced: every authenticated route needs a session cookie; " +
		"sign in at /login (first run: create the Super Admin via /api/setup/bootstrap)")

	if err := cfg.EnsureDataDir(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			slog.Error("closing database", "err", cerr)
		}
	}()

	if err := st.Migrate(ctx); err != nil {
		return err
	}
	schemaVersion, err := st.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	slog.Info("database ready", "path", st.Path(), "schema_version", schemaVersion)

	// Auth stack (Stage E2). Secrets never leave this process; the sealer keeps
	// panel tokens encrypted at rest (D-016).
	sealer, err := auth.NewSealer(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	// The audit logger also fans every privileged action into the account's
	// inbox (P8). One wiring point turns the whole platform into notification
	// producers, so a new audited action becomes a notification automatically
	// instead of needing a second call site that people forget.
	auditLog := audit.New(st.DB()).WithNotifier(st)
	authSvc := auth.NewService(st.DB(), auditLog)
	sessions := auth.NewSessionStore(st.DB(), cfg.SecureCookies)
	integrationsSvc := integrations.New(st.DB(), sealer, auditLog)

	// "Continue with Google" (D-039). The client is always built so the UI can ask
	// /api/auth/google/status whether the feature is available; without credentials
	// Configured() is false and every entry point stays closed.
	//
	// D-080: the client id/secret/redirect are resolved DB-first, falling back to
	// the environment. That lets a Super Admin edit them in the UI (applied live,
	// below) while an installation that only ever set VEYLIX_GOOGLE_* keeps
	// working unchanged.
	settingsSvc := settings.New(st, sealer, auditLog, cfg.GoogleOAuth)
	resolvedGoogle, resolveErr := settingsSvc.Resolve(ctx)
	if resolveErr != nil {
		// A blocked read is reported; the feature then simply reports unconfigured
		// rather than silently using half of a merged value.
		slog.Error("google settings could not be resolved; google sign-in will report unconfigured", "err", resolveErr)
	}

	googleSvc := &auth.GoogleAuth{
		Client: google.New(google.Config{
			ClientID:     resolvedGoogle.ClientID,
			ClientSecret: resolvedGoogle.ClientSecret,
			RedirectURL:  resolvedGoogle.RedirectURL,
		}),
		Store: st,
		Audit: auditLog,
	}
	// Live apply: saving the Google client in the UI rebuilds this client in the
	// running process, so a change takes effect without a restart (the honest
	// form of "settings saved and applied").
	settingsSvc.OnGoogleChange = func(r settings.ResolvedGoogle) {
		googleSvc.Client = google.New(google.Config{
			ClientID:     r.ClientID,
			ClientSecret: r.ClientSecret,
			RedirectURL:  r.RedirectURL,
		})
		slog.Info("google settings applied live", "configured", r.Configured())
	}
	if googleSvc.Configured() {
		slog.Info("google sign-in enabled", "redirect", resolvedGoogle.RedirectURL, "source", "database-or-env")
	} else {
		slog.Info("google sign-in disabled: set it in the Super Admin settings page or via VEYLIX_GOOGLE_*")
	}

	// Mail relay (D-041). Without SMTP_* settings the sender is nil and every route
	// that would send mail reports "outbox" instead of claiming a delivery — the
	// one rule that keeps "we e-mailed you" honest.
	mailer := mail.New(mail.Config{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
	})
	if mailer.Configured() {
		slog.Info("e-mail relay configured", "host", cfg.SMTP.Host, "from", cfg.SMTP.From)
	} else {
		slog.Info("e-mail relay NOT configured: verification and change codes are not delivered (set VEYLIX_SMTP_*)")
	}

	// The verifier owns both halves: it mints the token AND e-mails the link.
	// Without the mailer it created a token nobody could ever receive, which made
	// "verify your e-mail" impossible to complete (a dead-end sign-up).
	emailVerifier := &auth.EmailVerifier{
		Store:   st,
		Mail:    mailer,
		BaseURL: cfg.PublicBaseURL,
	}

	// Registration now owns the whole sign-up: create the account AND send the
	// confirmation link. Before this hook the account was created unverified and
	// nothing was sent, so a new client could never sign in.
	authSvc.SendVerification = func(ctx context.Context, userID int64, email string) (string, error) {
		_, delivery, err := emailVerifier.IssueAndSend(ctx, userID, email)
		return delivery, err
	}

	// Provisioning needs a per-server panel client; the factory decrypts the
	// sealed token on demand so it never lives in memory longer than one call.
	provFactory := func(ctx context.Context, serverID int64) (provisioning.PanelClient, error) {
		return integrationsSvc.PanelClient(ctx, serverID)
	}
	provSvc := provisioning.New(st.DB(), st, auditLog, provFactory)

	// ONE panel-read cache, shared by every consumer.
	//
	// It must be a single instance: GET /api/super/vpns reads the panel through
	// BOTH the admin lister and the row enricher, and two separate caches would
	// double the panel traffic while each believing it was warm. Constructed here
	// (not inline in the Deps literal) precisely so both call sites below receive
	// the same object — wiring the raw service to one of them silently bypassed
	// the cache entirely and left the endpoint at its 545 ms measured cost.
	panelReads := httpapi.NewCachedPanelInspector(integrationsSvc, httpapi.DefaultPanelCacheTTL)

	// Stage E5: read-only panel sync. It is the ONLY writer of usage history and
	// it never writes to a panel, so it needs no D-018 authorisation.
	syncSvc := sync.New(st.DB(), st, func(ctx context.Context, serverID int64) (sync.Panel, error) {
		return integrationsSvc.PanelClient(ctx, serverID)
	}, sync.Config{
		PageSize: cfg.SyncPageSize,
		MaxPages: cfg.SyncMaxPages,
		Timeout:  cfg.SyncTimeout,
	})

	// The lister only returns integrations that can actually be read.
	syncScheduler := sync.NewScheduler(syncSvc, func(ctx context.Context) ([]int64, error) {
		return activeServerIDs(ctx, st.DB())
	}, cfg.SyncInterval)
	syncScheduler.Start(ctx)
	defer syncScheduler.Stop()

	// Milestone 5-B step 9: the 3-day grace period needs something to expire it.
	// A goroutine that wakes hourly is enough — a release that becomes removable
	// an hour late is harmless, whereas a busy loop would waste the 1 vCPU target.
	releaseSvc := releases.New(st, auditLog)
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			// Run once at start so a restart after downtime catches up.
			if n, err := releaseSvc.PurgeDue(ctx); err != nil {
				slog.Error("release purge failed", "err", err)
			} else if n > 0 {
				slog.Info("released VPN records removed", "count", n,
					"note", "panel clients left intact (D-027.3)")
			}
			// The D-098 sweep: move long-inactive VPNs into the SAME release
			// path rather than inventing a second deletion.
			//
			// `off` and `report` both change nothing. `report` exists so an
			// operator can run the rule on a live box for a few days and read the
			// audit trail BEFORE anything is set aside — the difference between
			// "here is what this would do" and "here is what it did" is the whole
			// safety margin of a lifecycle rule.
			switch cfg.LifecycleMode {
			case config.LifecycleReport, config.LifecycleEnforce:
				dryRun := cfg.LifecycleMode == config.LifecycleReport
				moved, err := releaseSvc.ExpireStale(ctx, dryRun)
				if err != nil {
					slog.Error("lifecycle sweep failed", "err", err)
				} else if len(moved) > 0 {
					slog.Warn("lifecycle sweep: long-inactive VPNs found",
						"count", len(moved), "mode", cfg.LifecycleMode,
						"note", "REPORT MODE — nothing was changed", "dry_run", dryRun)
				}
			}
			// Expired session rows are dead weight that also grow the table an
			// attacker could try to fill: drop them on the same hourly wake-up.
			if n, err := sessions.PurgeExpired(ctx); err != nil {
				slog.Error("session purge failed", "err", err)
			} else if n > 0 {
				slog.Info("expired sessions removed", "count", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	// P4 recovery loop: the outbox reconciler.
	//
	// Before this existed, Reconcile had ZERO production callers — only tests —
	// so a remote operation that landed in RETRY (exactly the "outcome unknown"
	// case pushCreate deliberately produces when a panel times out mid-write)
	// was never retried by anything. The VPN stayed SERVER_UNAVAILABLE forever
	// and only a human could fix it.
	//
	// Two minutes is the right cadence for a 1 vCPU box: Retry is scheduled with
	// exponential backoff (20s → 640s), so most ticks find nothing due and cost
	// a single indexed SELECT on outbox(state, next_attempt_at).
	go func() {
		const reconcileEvery, reconcileBatch = 2 * time.Minute, 25
		ticker := time.NewTicker(reconcileEvery)
		defer ticker.Stop()
		for {
			// Run once at start so a restart after downtime resumes the queue
			// immediately instead of waiting a full interval.
			func() {
				// A panic in one row must not kill recovery for every other row.
				defer func() {
					if r := recover(); r != nil {
						slog.Error("outbox reconcile panicked", "panic", r)
					}
				}()
				res, err := provSvc.Reconcile(ctx, reconcileBatch)
				if err != nil {
					slog.Error("outbox reconcile failed", "err", err)
					return
				}
				if res[provisioning.ReconcileApplied] > 0 ||
					res[provisioning.ReconcileExhausted] > 0 ||
					res[provisioning.ReconcileResolved] > 0 {
					slog.Info("outbox reconciled",
						"applied", res[provisioning.ReconcileApplied],
						"retry", res[provisioning.ReconcileRetry],
						"exhausted", res[provisioning.ReconcileExhausted],
						"resolved", res[provisioning.ReconcileResolved])
				}
			}()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.Router(httpapi.Deps{
			Health:       st,
			Version:      version,
			System:       newSystemService(os.Getenv("VEYLIX_REPO")),
			Auth:         authSvc,
			Sessions:     sessions,
			Authn:        sessions,
			Integrations: integrationsSvc,
			// The read-only panel inspector (D-024 Stage C).
			//
			// Wrapped in a short-TTL read-through cache: GET /api/super/vpns reads
			// the whole panel on every request (the panel is the truth, D-024.3),
			// which measured a 1,044 ms median and a 44.8 s worst case in
			// production. The wrapper caches READS only — it never writes, and an
			// unreadable panel is still returned as an error, so a row's "unread"
			// flag stays honest rather than becoming a stale success.
			PanelInspector: panelReads,
			// Tagging (D-024.3, step 4): a local write, audited to the same log.
			PanelTags:     st,
			PanelTagAudit: auditLog,
			// D8: a VPN's registered devices (HWIDs) — the panel is the only
			// source, and freeing a slot is a real panel write.
			Hwids: integrationsSvc,
			// Assigning an unassigned panel client to a customer (D-037.2), and
			// renaming it into the D-034 scheme.
			Assignments:  st,
			PanelRenamer: integrationsSvc,
			// Moving an already-assigned VPN between customers (D-064).
			// Local only: the panel client keeps its identity, so no panel write
			// happens and a transfer works even while a panel is unreachable.
			Transfers: st,
			Servers:   st,
			Audit:     st,
			Quota:     st,
			VPNs:      provSvc,
			// Links + ping (D-026, Milestone 5-B step 11): the endpoints that let
			// a client actually connect. The pinger resolves the server's panel
			// URL and times a TCP connect — a server-reachability number, never
			// the user's own latency.
			VPNConnection: provSvc,
			PanelPing:     httpapi.DialPinger(st.ServerPanelURL, 3*time.Second),
			Releases:      releaseSvc,
			VPNLister:     httpapi.NewSQLVPNLister(st.DB()),
			// The Super Admin's catalogue: the panel's clients ARE the VPN list
			// (D-024.3), so it reads the panel through the same bounded, read-only
			// inspector — and the SAME cache instance — as every other panel read.
			// Passing integrationsSvc directly here would bypass the cache.
			PanelVPNLister: httpapi.NewPanelVPNLister(st.DB(), panelReads),
			// FREE claim caps (D-025.4/.5): read by the UI before a claim.
			FreeClaims: st,
			Usage:      httpapi.NewSQLUsageStore(st.DB()),
			// Latency history (D-092). The same store serves the read and the
			// probe write, so ownership is decided by one predicate.
			PingSamples: httpapi.NewSQLUsageStore(st.DB()),
			// The on-demand trigger runs the SAME pass the scheduler runs, so the
			// console's "Sync now" cannot diverge from the background behaviour.
			SyncRunner:     syncSvc,
			Offers:         st,
			OfferAudit:     auditLog,
			SyncStaleAfter: staleAfter(cfg.SyncInterval),
			// D-099: the zone every screen renders an absolute instant in.
			// Config has already resolved it to a real location at startup, so a
			// bad VEYLIX_DISPLAY_TIMEZONE was logged then rather than failing a
			// request later.
			DisplayTimezone: cfg.DisplayTimezone,
			UserAdmin:       authSvc,
			AdminAudit:      auditLog,
			Clients:         st,
			SetupState:      authSvc,
			// D1/D2: GET /api/me answers the display name and creation date
			// from the account row, not from the session's e-mail.
			Accounts: st,
			// Inbox, directory and billing (P6/P8). Each is a real store, so a
			// nil here would silently disable its routes.
			Notifications: st,
			Directory:     st,
			Payments:      st,
			// D-031: confirming an order builds the VPN in the same click.
			Orders: payments.New(st, provSvc),
			// D-036: an order's own actions — destination change, proof
			// channel, timeline, and stopping the VPN when the money does not
			// arrive.
			OrderActions: st,
			OrderBlocker: provSvc,
			// D-036.6: where a CUSTOM order will be built.
			ReleaseTargets: st,
			// D-030: the plan catalogue (prices, slots, archive, delete).
			Plans: st,
			// D-035.4: the unit rate card and the CUSTOM quote.
			Pricing: st,
			Chat:    st,
			// D-039: Google sign-in/sign-up and e-mail verification.
			Google:        googleSvc,
			EmailVerifier: emailVerifier,
			// D-080: operator-editable settings (the Google OAuth client).
			Settings: settingsSvc,
			// D-041: the account's own profile (name, username, contacts, avatar,
			// bio) and the e-mail-change flow. Mail is nil until SMTP is configured,
			// and the routes then report `delivery: "outbox"` rather than pretend.
			Profile:        st,
			Mail:           mailer,
			PasswordResets: st,
			SupportEmail:   cfg.SMTP.Support,
		}),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", cfg.HTTPAddr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	slog.Info("shutdown complete")
	return nil
}

// activeServerIDs lists integrations that sync may read: ACTIVE only, because
// an integration that is DISABLED, REAUTH_REQUIRED or REMOVED must not be polled
// (REQUIREMENTS §3 says a failed credential blocks new work and backs off).
func activeServerIDs(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id FROM server_integrations WHERE status = 'ACTIVE' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sync: list active servers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sync: scan server id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// staleAfter is the freshness window shown by /api/sync/status: three sync
// intervals, floored so a very short interval still has a usable window.
func staleAfter(interval time.Duration) time.Duration {
	d := 3 * interval
	if d < 15*time.Minute {
		d = 15 * time.Minute
	}
	return d
}
