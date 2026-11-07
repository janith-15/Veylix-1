// Command accounttool performs operator tasks against the SAME database the
// server uses:
//
//	-superadmin        create or RESET the super admin, printing a generated
//	                   password once (never stored in source, docs or logs)
//	-admin             create or reset an admin (reseller) account
//	-verify <email>    mark an account's e-mail verified
//	-purge-chat        delete chat data (backup first)
//
// Why a separate binary: the running server must never mint credentials, and an
// operator needs a recovery path for an account with no working password. The
// password is generated (crypto/rand) and emitted ONCE as a `CREDENTIAL {...}`
// line, which dev.ps1 moves into .veylix-secrets and keeps out of the console.
//
// Safety rails, shared with cmd/seedacct:
//   - VEYLIX_SEED=1 is required;
//   - the bind address must be loopback, so a production .env refuses;
//   - every write is audited.
//
// Usage (or through .\dev.ps1 -SuperAdmin / -CreateAdmin):
//
//	go run ./cmd/accounttool -superadmin -pattern readable
//	go run ./cmd/accounttool -admin -email a@b.test -username nimal -quota 25
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/config"
	"veylix/backend/internal/mail"
	"veylix/backend/internal/store"
)

// options is the parsed command line.
type options struct {
	superAdmin bool
	admin      bool
	email      string
	username   string
	name       string
	quota      int
	pattern    string
	verify     string
	purgeChat  bool
	// resetLink issues a password-reset code for an account and prints it, so a
	// loopback operator can hand it over when no SMTP relay is configured (D-041).
	resetLink string
	// testMail sends one message to prove the relay works before anyone relies
	// on it: "configured" and "working" are different claims.
	testMail string
	// resetData wipes every row except the super_admin accounts, after a backup.
	resetData bool
	dryRun    bool
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	var o options
	flag.BoolVar(&o.superAdmin, "superadmin", false, "create or reset the super admin account")
	flag.BoolVar(&o.admin, "admin", false, "create or reset an admin (reseller) account")
	flag.StringVar(&o.email, "email", "", "account e-mail")
	flag.StringVar(&o.username, "username", "", "login username (default: the e-mail prefix)")
	flag.StringVar(&o.name, "name", "", "display name")
	flag.IntVar(&o.quota, "quota", 0, "reseller quota ceiling (admin only)")
	flag.StringVar(&o.pattern, "pattern", "readable", "password pattern: readable | passphrase | base32")
	flag.StringVar(&o.verify, "verify", "", "mark this e-mail's account verified")
	flag.BoolVar(&o.purgeChat, "purge-chat", false, "delete all chat conversations and messages (backup first)")
	flag.StringVar(&o.resetLink, "reset-link", "",
		"issue a password-reset code for this e-mail/username and print it (loopback operator only)")
	flag.StringVar(&o.testMail, "test-mail", "",
		"send one test message to this address to prove the SMTP relay works")
	flag.BoolVar(&o.resetData, "reset-data", false,
		"delete all data except super_admin accounts (writes a backup first; pair with -dry-run to preview)")
	flag.BoolVar(&o.dryRun, "dry-run", false, "report what would happen, change nothing")
	flag.Parse()

	if err := run(o); err != nil {
		slog.Error("accounttool failed", "err", err)
		os.Exit(1)
	}
}

func run(o options) error {
	if !o.superAdmin && !o.admin && o.verify == "" && !o.purgeChat && o.resetLink == "" && o.testMail == "" && !o.resetData {
		return errors.New("nothing to do: pass -superadmin, -admin, -verify <email>, -reset-link <id>, -test-mail <to>, -purge-chat or -reset-data")
	}
	if os.Getenv("VEYLIX_SEED") != "1" {
		return errors.New("refusing to run: set VEYLIX_SEED=1 to confirm this is a development environment")
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if !isLoopbackAddr(cfg.HTTPAddr) {
		return fmt.Errorf("refusing to run: VEYLIX_ADDR=%q is not a loopback address", cfg.HTTPAddr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	if o.superAdmin {
		if err := provision(ctx, st, provisionRequest{
			role: auth.RoleSuperAdmin, email: o.email, username: o.username,
			name: o.name, pattern: o.pattern, dryRun: o.dryRun,
		}); err != nil {
			return err
		}
	}
	if o.admin {
		if err := provision(ctx, st, provisionRequest{
			role: auth.RoleAdmin, email: o.email, username: o.username, name: o.name,
			quota: o.quota, pattern: o.pattern, dryRun: o.dryRun,
		}); err != nil {
			return err
		}
	}
	if o.verify != "" {
		if err := verifyEmail(ctx, st, o.verify, o.dryRun); err != nil {
			return err
		}
	}
	if o.resetData {
		if err := resetExceptSuperAdmin(ctx, st, cfg.DBPath, o.dryRun); err != nil {
			return err
		}
	}
	if o.purgeChat {
		if err := purgeChatData(ctx, st, cfg.DBPath, o.dryRun); err != nil {
			return err
		}
	}
	if o.resetLink != "" {
		if err := printResetCode(ctx, st, o.resetLink, o.dryRun); err != nil {
			return err
		}
	}
	if o.testMail != "" {
		if err := testMail(ctx, mail.Config{
			Host:     cfg.SMTP.Host,
			Port:     cfg.SMTP.Port,
			Username: cfg.SMTP.Username,
			Password: cfg.SMTP.Password,
			From:     cfg.SMTP.From,
		}, o.testMail); err != nil {
			return err
		}
	}
	return nil
}
