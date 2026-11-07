package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// printResetCode issues a password-reset code for an account and prints it.
//
// This is the loopback-operator path for a deployment with no SMTP relay (D-041):
// the API still answers 202 and reports `delivery: "outbox"`, and the code is
// readable here instead. The same hashing, TTL and single-use rules apply — this
// function only *prints* what the mail would have carried, so it cannot weaken the
// flow; anyone with access to this tool already has the database.
func printResetCode(ctx context.Context, st *store.Store, identifier string, dryRun bool) error {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	var (
		id    int64
		email string
		role  string
	)
	err := st.DB().QueryRowContext(ctx,
		`SELECT id, email, role FROM users
		  WHERE email = ? OR (username IS NOT NULL AND username <> '' AND username = ?)
		  LIMIT 1`, identifier, identifier).Scan(&id, &email, &role)
	if err != nil {
		return fmt.Errorf("no account matches %q: %w", identifier, err)
	}
	if dryRun {
		slog.Info("would issue a reset code", "email", email, "role", role)
		return nil
	}

	code, err := numericCode(6)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(code + resetCodeSalt)
	if err != nil {
		return err
	}
	// Ten minutes, matching the API's resetCodeTTL: a shorter life here would make
	// the handover window differ depending on who issued the code.
	if err := st.CreatePasswordResetCode(ctx, id, hash, 10*time.Minute); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Printf("  reset code for %s (%s)\n", email, role)
	fmt.Printf("  code     : %s\n", code)
	fmt.Println("  expires  : 10 minutes, single use")
	fmt.Println("  use      : POST /api/auth/reset {identifier, code, new_password}")
	fmt.Println("============================================================")
	fmt.Println()
	return nil
}

// resetCodeSalt mirrors the API's suffix so a code hashed here verifies there.
//
// It must be the same literal: a 6-digit value alone is inside a trivially
// precomputable space, and the suffix removes that.
const resetCodeSalt = "|veylix-verification-code|"

// numericCode returns a cryptographically random decimal code of `digits` length.
// rand.Int avoids the modulo bias of `% 1000000`.
func numericCode(digits int) (string, error) {
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%0*s", digits, n.String()), nil
}
