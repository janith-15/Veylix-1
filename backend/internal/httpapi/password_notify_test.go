package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veylix/backend/internal/auth"
)

// A password change must notify the account holder: a stolen session can change
// the password, and without this mail the real owner never learns it happened.
func TestPasswordChangeSendsNotification(t *testing.T) {
	sender := &captureSender{configured: true}
	r := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", nil)
	p := auth.Principal{UserID: 42, Email: "owner@veylix.test", Role: "client"}

	delivery := notifyPasswordChanged(r, sender, p)
	if delivery != "smtp" {
		t.Fatalf("delivery = %q, want smtp", delivery)
	}
	if sender.calls != 1 {
		t.Fatalf("sent %d messages, want 1", sender.calls)
	}
	if sender.to[0] != "owner@veylix.test" {
		t.Fatalf("sent to %q, want the account address", sender.to[0])
	}
	body := sender.body[0]
	// It must say what happened, when, and from where — and offer a way out.
	for _, want := range []string{"password was changed", "When:", "From:", "/forgot"} {
		if !strings.Contains(body, want) {
			t.Errorf("notification is missing %q:\n%s", want, body)
		}
	}
}

// With no relay the change still succeeds; the response just reports `outbox`
// rather than implying a message was sent (D-041).
func TestPasswordChangeWithoutRelayReportsOutbox(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", nil)
	p := auth.Principal{UserID: 42, Email: "owner@veylix.test"}

	if got := notifyPasswordChanged(r, nil, p); got != "outbox" {
		t.Fatalf("delivery = %q, want outbox with no relay", got)
	}
	// A relay that rejects the message is the same answer: not delivered.
	broken := &captureSender{configured: true, fail: true}
	if got := notifyPasswordChanged(r, broken, p); got != "outbox" {
		t.Fatalf("delivery = %q, want outbox after a failed send", got)
	}
}
