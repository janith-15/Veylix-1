package mail

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Without a relay every send must fail loudly, because the caller's UI says
// "we e-mailed you a code" — and that must never be a lie.
func TestUnconfiguredSenderRefuses(t *testing.T) {
	s := New(Config{})
	if s.Configured() {
		t.Fatal("an empty config reports configured")
	}
	if err := s.Send(context.Background(), "a@b.test", "s", "t", ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Send err = %v, want ErrNotConfigured", err)
	}

	// A nil sender is valid and behaves the same, so call sites need no nil check.
	var nilSender *Sender
	if nilSender.Configured() {
		t.Fatal("a nil sender reports configured")
	}
	if err := nilSender.Send(context.Background(), "a@b.test", "s", "t", ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil Send err = %v, want ErrNotConfigured", err)
	}
}

// A host plus a From address is what makes a send possible; the rest defaults.
func TestConfiguredNeedsHostAndFrom(t *testing.T) {
	if New(Config{Host: "smtp.example.com"}).Configured() {
		t.Fatal("host without From must not be configured")
	}
	s := New(Config{Host: "smtp.example.com", Username: "me@example.com"})
	if !s.Configured() {
		t.Fatal("From must default to the username")
	}
	if s.cfg.Port != 587 {
		t.Fatalf("default port = %d, want 587", s.cfg.Port)
	}
}

// The message must be a real multipart/alternative with both a text and an HTML
// part, and must escape markup in the HTML body.
func TestBuildMessageIsMultipartWithEscapedHtml(t *testing.T) {
	raw := string(buildMessage("Veylix <me@example.com>", "him@example.com", "Your code",
		"Your code is 123456", "<p>Your code is <strong>123456</strong> &amp; it expires</p>"))
	for _, want := range []string{
		"From: Veylix <me@example.com>",
		"To: him@example.com",
		"Subject: Your code",
		"MIME-Version: 1.0",
		"multipart/alternative",
		"Content-Type: text/plain; charset=\"utf-8\"",
		"Content-Type: text/html; charset=\"utf-8\"",
		"Your code is 123456",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("message is missing %q", want)
		}
	}

	// A text-only send still carries an HTML part, with the text escaped.
	only := string(buildMessage("me@example.com", "him@example.com", "Hi", "<b>not bold</b>", ""))
	if !strings.Contains(only, "&lt;b&gt;not bold&lt;/b&gt;") {
		t.Error("text-only body was not escaped into the HTML part")
	}

	// A non-ASCII subject becomes an RFC 2047 encoded word.
	enc := string(buildMessage("me@example.com", "him@example.com", "ඔබේ කේතය", "x", ""))
	if !strings.Contains(enc, "Subject: =?utf-8?b?") {
		t.Error("a non-ASCII subject was not encoded")
	}
}

// An address without "@" is refused before any network work.
func TestSendRefusesAnInvalidRecipient(t *testing.T) {
	s := New(Config{Host: "smtp.example.com", From: "me@example.com"})
	if err := s.Send(context.Background(), "not-an-address", "s", "t", ""); err == nil {
		t.Fatal("an invalid recipient was accepted")
	}
}
