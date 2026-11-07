package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"veylix/backend/internal/mail"
)

// testMail proves the relay works before anybody relies on it.
//
// It exists because the one thing worse than no mail relay is a relay that looks
// configured and silently fails: a password-reset code that never arrives is a
// person locked out. This sends one real message and reports exactly which step
// failed (dial, handshake, auth, MAIL, RCPT, DATA).
//
// The password is never printed. Only the host, the port and the outcome are.
func testMail(ctx context.Context, cfg mail.Config, to string) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("-test-mail needs a recipient address")
	}
	sender := mail.New(cfg)
	if !sender.Configured() {
		fmt.Println()
		fmt.Println("  SMTP is NOT configured.")
		fmt.Println("  Set VEYLIX_SMTP_HOST, VEYLIX_SMTP_USER, VEYLIX_SMTP_PASS and")
		fmt.Println("  VEYLIX_MAIL_FROM in .veylix-secrets/dev-env.ps1, then try again.")
		fmt.Println()
		return fmt.Errorf("mail: no SMTP relay configured")
	}

	fmt.Println()
	fmt.Printf("  relay   : %s:%d\n", cfg.Host, cfg.Port)
	fmt.Printf("  from    : %s\n", cfg.From)
	fmt.Printf("  to      : %s\n", to)
	fmt.Println("  sending : one test message …")

	started := time.Now()
	err := sender.Send(ctx,
		to,
		"Veylix — SMTP test",
		"This is a Veylix SMTP test.\n\nIf you are reading this, the relay works and\n"+
			"password-reset codes will reach you.\n",
		"<p>This is a Veylix SMTP test.</p><p>If you are reading this, the relay works and "+
			"password-reset codes will reach you.</p>",
	)
	if err != nil {
		fmt.Println()
		fmt.Printf("  FAILED after %s\n", time.Since(started).Round(time.Millisecond))
		fmt.Printf("  reason: %v\n", err)
		fmt.Println()
		return err
	}
	fmt.Println()
	fmt.Printf("  SMTP login OK — message accepted after %s\n",
		time.Since(started).Round(time.Millisecond))
	slog.Info("smtp test delivered", "to", to, "host", cfg.Host)
	fmt.Println("  Check that inbox (and the spam folder).")
	fmt.Println()
	return nil
}
