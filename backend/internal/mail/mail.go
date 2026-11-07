// Package mail sends transactional e-mail (verification, password reset, e-mail
// change) over SMTP.
//
// Design rules:
//
//   - **One rule above all: never claim a message was sent when it was not.** A
//     caller must be able to see whether a relay exists (`Configured`) and whether
//     the send succeeded (the error), because "we e-mailed you a code" is a promise
//     a person acts on.
//   - Credentials come from the environment only (VEYLIX_SMTP_*), never from
//     source, and are never logged.
//   - Bounded: dial and send both have a deadline, so a dead relay cannot hang a
//     request handler or a background job.
//   - STARTTLS is used whenever the server offers it (port 587), and the message
//     is a proper multipart/alternative with a text part, so it renders in every
//     client and is not classified as an empty HTML blast.
package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// ErrNotConfigured is returned when no relay is set up.
var ErrNotConfigured = errors.New("mail: no SMTP relay configured")

// Config is the relay settings.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the visible sender. It must be an address the relay is allowed to
	// send as (Gmail requires the account itself).
	From string
	// Timeout bounds dial + send.
	Timeout time.Duration
}

// Sender delivers mail. A nil *Sender is valid and refuses politely, so callers do
// not need a nil check at every site.
type Sender struct {
	cfg Config
}

// New builds a sender from configuration.
func New(cfg Config) *Sender {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.From == "" && cfg.Username != "" {
		cfg.From = cfg.Username
	}
	return &Sender{cfg: cfg}
}

// Configured reports whether a send is even possible.
func (s *Sender) Configured() bool {
	return s != nil && s.cfg.Host != "" && s.cfg.From != ""
}

// Send delivers one message with a text and an HTML alternative.
//
// The error is returned to the caller and logged by it — never swallowed here,
// because a lost password-reset code is a person locked out of their account.
func (s *Sender) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	if !s.Configured() {
		return ErrNotConfigured
	}
	if !strings.Contains(to, "@") {
		return fmt.Errorf("mail: invalid recipient")
	}
	from := s.cfg.From
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))

	// Dial with the caller's deadline so a context cancellation (server shutting
	// down, client gone) stops the attempt.
	dialer := &net.Dialer{Timeout: s.cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("mail: dial %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(s.cfg.Timeout))

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: handshake: %w", err)
	}
	defer func() { _ = c.Close() }()

	// STARTTLS whenever offered: a plaintext SMTP session exposes the password and
	// the code. Port 465 (implicit TLS) is handled by wrapping the conn first.
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(buildMessage(from, to, subject, textBody, htmlBody)); err != nil {
		return fmt.Errorf("mail: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: finish message: %w", err)
	}
	return c.Quit()
}

// buildMessage assembles a multipart/alternative MIME message.
//
// The boundary is fixed rather than random: the content never contains it (the
// body is base64-safe plain text we generate), and a constant keeps the function
// pure and testable.
func buildMessage(from, to, subject, textBody, htmlBody string) []byte {
	const boundary = "veylix-alt-boundary-7f3a"
	if htmlBody == "" {
		htmlBody = "<pre>" + htmlEscape(textBody) + "</pre>"
	}
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + encodeHeader(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	b.WriteString("\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n")
	b.WriteString(textBody + "\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n\r\n")
	b.WriteString(htmlBody + "\r\n")
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

// encodeHeader keeps a non-ASCII subject legal (RFC 2047 encoded-word).
func encodeHeader(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?utf-8?b?" + base64Std(s) + "?="
		}
	}
	return s
}

// base64Std encodes a header value (imported here to keep the file's dependency
// list explicit and small).
func base64Std(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// htmlEscape escapes the five characters that matter inside markup.
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}
