// Command pwreset sets one DEV account's password to a value read from a file,
// so a local verification run can sign in as a persona whose password is not
// known to the agent running it.
//
// This is a DEVELOPMENT tool, not part of the server, and it is deliberately
// narrow:
//   - it requires VEYLIX_DBEXEC=1, the same guard cmd/dbexec uses, so a
//     production shell cannot run it by accident;
//   - it refuses a super_admin target, because that is the account an operator
//     must reset through cmd/accounttool with a printed password;
//   - the password is read from --password-file, never from the command line,
//     so it never lands in shell history or a process list;
//   - the new hash goes through auth.HashPassword, so the stored value obeys the
//     same argon2id policy the API enforces;
//   - it prints only the e-mail and the resulting row count — never the
//     password, never the hash.
//
// Usage:
//
//	set VEYLIX_DBEXEC=1
//	go run ./cmd/pwreset --email client@veylix.test --password-file ../.veylix-secrets/_pw.txt
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "pwreset: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	emailFlag := flag.String("email", "", "account e-mail to reset (required)")
	fileFlag := flag.String("password-file", "", "file holding the new password (required)")
	dryRun := flag.Bool("dry-run", false, "report what would change, write nothing")
	flag.Parse()

	if os.Getenv("VEYLIX_DBEXEC") != "1" {
		return fmt.Errorf("refusing to run: set VEYLIX_DBEXEC=1 (development tool, never part of the server)")
	}
	email := strings.ToLower(strings.TrimSpace(*emailFlag))
	if email == "" {
		return fmt.Errorf("--email is required")
	}
	if strings.TrimSpace(*fileFlag) == "" {
		return fmt.Errorf("--password-file is required (a password must never be passed on the command line)")
	}
	raw, err := os.ReadFile(*fileFlag)
	if err != nil {
		return fmt.Errorf("read password file: %w", err)
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if err := auth.ValidatePasswordPolicy(password); err != nil {
		return fmt.Errorf("password file does not satisfy the policy: %w", err)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := sql.Open("sqlite", "file:"+cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = db.Close() }()

	var userID int64
	var role string
	err = db.QueryRowContext(ctx, `SELECT id, role FROM users WHERE lower(email) = ?`, email).Scan(&userID, &role)
	if err == sql.ErrNoRows {
		return fmt.Errorf("no account with e-mail %s", email)
	}
	if err != nil {
		return fmt.Errorf("lookup: %w", err)
	}
	if role == string(auth.RoleSuperAdmin) {
		return fmt.Errorf("refusing to reset a super_admin here — use cmd/accounttool -superadmin")
	}

	if *dryRun {
		fmt.Printf("would reset: id=%d email=%s role=%s\n", userID, email, role)
		return nil
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	res, err := db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, time.Now().Unix(), userID)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	fmt.Printf("ok: %d row(s) updated — id=%d email=%s role=%s\n", n, userID, email, role)
	return nil
}
