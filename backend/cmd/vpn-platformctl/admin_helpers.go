package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"veylix/backend/internal/store"
)

// superAdminTarget is the row resolved from the database.
type superAdminTarget struct {
	id          int64
	email       string
	displayName string
}

// resolveSuperAdmin finds the account to reset.
//
// With no --email it requires EXACTLY ONE super_admin. Silently picking the first
// of several would let a `reset-admin` typed in a hurry change the wrong account.
func resolveSuperAdmin(ctx context.Context, st *store.Store, email string) (superAdminTarget, error) {
	var t superAdminTarget

	if strings.TrimSpace(email) != "" {
		err := st.DB().QueryRowContext(ctx,
			`SELECT id, email, display_name FROM users WHERE role = 'super_admin' AND email = ? COLLATE NOCASE`,
			strings.TrimSpace(email)).Scan(&t.id, &t.email, &t.displayName)
		if err != nil {
			return t, fmt.Errorf("no Super Admin with the email %q: %w", email, err)
		}
		return t, nil
	}

	rows, err := st.DB().QueryContext(ctx,
		`SELECT id, email, display_name FROM users WHERE role = 'super_admin' ORDER BY id`)
	if err != nil {
		return t, fmt.Errorf("cannot list Super Admins: %w", err)
	}
	defer rows.Close()

	var found []superAdminTarget
	for rows.Next() {
		var c superAdminTarget
		if err := rows.Scan(&c.id, &c.email, &c.displayName); err != nil {
			return t, fmt.Errorf("cannot read a Super Admin row: %w", err)
		}
		found = append(found, c)
	}
	if err := rows.Err(); err != nil {
		return t, fmt.Errorf("cannot list Super Admins: %w", err)
	}

	switch len(found) {
	case 0:
		return t, fmt.Errorf("there is no Super Admin in this database - " +
			"a fresh install creates one at /setup")
	case 1:
		return found[0], nil
	default:
		names := make([]string, len(found))
		for i, f := range found {
			names[i] = f.email
		}
		return t, usagef("there are %d Super Admins (%s) - pass --email to choose one",
			len(found), strings.Join(names, ", "))
	}
}

// readSecret prompts for a value without echoing it, then reads one line.
//
// It delegates to readSecretRaw because turning echo off is a terminal API and
// Go's stdlib has no cross-platform one.
func readSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	defer fmt.Fprintln(os.Stderr)
	return readSecretRaw()
}
