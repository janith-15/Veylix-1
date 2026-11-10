package main

import (
	"context"
	"fmt"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// cmdResetAdmin sets a new password for the Super Admin from the command line.
//
// This is the recovery path for the one account that cannot be recovered any
// other way: if the Super Admin password is lost there is no in-app route back
// in, and re-installing would destroy the platform. Without this command a
// forgotten password is a permanent lockout.
//
// It reuses auth.HashPassword and auth.ValidatePasswordPolicy instead of
// reimplementing them, so a password set here is byte-for-byte the same shape as
// one set from the UI and obeys the same policy. It mirrors
// auth.Service.ResetStaffPassword: update the hash, bump updated_at, delete every
// session.
func cmdResetAdmin(p paths, args []string) error {
	fs := newFlagSet("reset-admin")
	o := addCommonFlags(fs)
	email := fs.String("email", "", "the Super Admin account (default: the only super_admin)")
	password := fs.String("password", "", "the new password (default: prompt, not echoed)")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	if !fileExists(p.dbFile()) {
		return fmt.Errorf("no database at %s - is Veylix installed on this host?", p.dbFile())
	}

	newPassword := *password
	if newPassword == "" {
		if o.dryRun {
			out("[dry-run] would prompt for a new password and update the Super Admin")
			return nil
		}
		// Read twice: a mistyped password here locks the operator out of the
		// very account they are trying to recover.
		first, err := readSecret("New password: ")
		if err != nil {
			return err
		}
		second, err := readSecret("Repeat it:    ")
		if err != nil {
			return err
		}
		if first != second {
			return fmt.Errorf("the two passwords did not match; nothing was changed")
		}
		newPassword = first
	}

	// The policy check runs BEFORE the database is opened, so a weak password
	// cannot leave a half-done state behind.
	if err := auth.ValidatePasswordPolicy(newPassword); err != nil {
		return fmt.Errorf("that password is not acceptable: %w", err)
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("cannot hash the password: %w", err)
	}

	if o.dryRun {
		out("[dry-run] would set a new password for the Super Admin in %s", p.dbFile())
		out("[dry-run] would revoke every existing session")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.Open(ctx, p.dbFile())
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", p.dbFile(), err)
	}
	defer st.Close()

	// store.Open does NOT create the schema - veylixd calls Migrate explicitly
	// after opening. Skipping it here made reset-admin fail with "no such table:
	// users" on any database that had not yet been opened by the running service,
	// which is exactly the state after restore. Migrate is idempotent, so running
	// it is safe on an already-migrated database.
	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("cannot read the schema of %s: %w", p.dbFile(), err)
	}

	target, err := resolveSuperAdmin(ctx, st, *email)
	if err != nil {
		return err
	}

	step("reset the password")
	now := time.Now().Unix()
	res, err := st.DB().ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ? AND role = 'super_admin'`,
		hash, now, target.id)
	if err != nil {
		return fmt.Errorf("cannot update the account: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("expected to update exactly one super_admin row, updated %d", n)
	}

	step("revoke existing sessions")
	sres, err := st.DB().ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, target.id)
	if err != nil {
		// The password IS already changed at this point, so say so plainly
		// rather than implying the whole operation failed.
		return fmt.Errorf("the password WAS changed, but existing sessions could not be "+
			"revoked: %w\n  re-run the command to retry the revocation", err)
	}
	revoked, _ := sres.RowsAffected()

	out("")
	out("Super Admin password changed.")
	out("  account:  %s (%s)", target.email, target.displayName)
	out("  sessions: %d revoked", revoked)
	out("")
	out("Sign in at https://%s/login", configuredDomain(p.envFile()))
	return nil
}
