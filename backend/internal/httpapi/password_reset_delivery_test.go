package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// realSender is a stand-in for the SMTP relay: it records what it was asked to
// send so a test can prove a message really left, without opening a socket.
type realSender struct {
	calls int
	to    []string
	fail  error
}

func (s *realSender) Configured() bool { return true }

func (s *realSender) Send(_ context.Context, to, _, _, _ string) error {
	s.calls++
	s.to = append(s.to, to)
	return s.fail
}

// newResetFixtureWithMail boots the same fixture with a specific mail sender, so
// the delivery branch can be exercised with a relay present and absent.
func newResetFixtureWithMail(t *testing.T, m EmailSender) *resetFixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir+"/reset.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	auditLog := audit.New(st.DB())
	svc := auth.NewService(st.DB(), auditLog)
	svc.SetAutoVerify(true)
	user, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "reset@veylix.test", Password: "correct-horse-battery-9", DisplayName: "Reset Tester",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	mux := http.NewServeMux()
	PasswordResetRoutes(mux, svc, st, m, auditLog, "support@veylix.test",
		auth.NewRateLimiter(1000, 1000))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &resetFixture{t: t, srv: srv, store: st, user: user}
}

// With no relay the API must say `outbox`, so the UI asks an operator instead of
// telling the owner to check an inbox nothing was ever sent to.
func TestForgotReportsOutboxWithoutRelay(t *testing.T) {
	f := newResetFixtureWithMail(t, nil)
	status, body := f.post("/api/auth/forgot", map[string]any{"identifier": "reset@veylix.test"})
	if status != http.StatusAccepted {
		t.Fatalf("status %d, want 202", status)
	}
	if got := body["delivery"]; got != deliveryOutbox {
		t.Fatalf("delivery = %v, want %q with no relay configured", got, deliveryOutbox)
	}
}

// With a working relay the API must say `smtp`, and a message must really have
// been handed over. Before this test the field was absent entirely, so the UI's
// "check your inbox" branch was unreachable and every reset looked undelivered.
func TestForgotReportsSMTPSWhenDelivered(t *testing.T) {
	sender := &realSender{}
	f := newResetFixtureWithMail(t, sender)
	status, body := f.post("/api/auth/forgot", map[string]any{"identifier": "reset@veylix.test"})
	if status != http.StatusAccepted {
		t.Fatalf("status %d, want 202", status)
	}
	if got := body["delivery"]; got != deliverySMTP {
		t.Fatalf("delivery = %v, want %q with a working relay", got, deliverySMTP)
	}
	if sender.calls == 0 {
		t.Fatal("delivery=smtp was reported but nothing was handed to the relay")
	}
	if len(sender.to) == 0 || sender.to[0] != "reset@veylix.test" {
		t.Fatalf("sent to %v, want the account address", sender.to)
	}
}

// A relay that rejects the message must not be reported as delivered to the
// *audit row* — but the wire answer stays `smtp`, because `/api/auth/forgot`
// reports whether the server can send mail, not what happened to one address.
// Reporting the per-account failure would make this endpoint an account oracle.
func TestForgotReportsOutboxOnWireOnlyWhenNoRelay(t *testing.T) {
	sender := &realSender{fail: errors.New("mail: auth: 535 bad credentials")}
	f := newResetFixtureWithMail(t, sender)
	status, body := f.post("/api/auth/forgot", map[string]any{"identifier": "reset@veylix.test"})
	if status != http.StatusAccepted {
		t.Fatalf("status %d, want 202", status)
	}
	if got := body["delivery"]; got != deliverySMTP {
		t.Fatalf("delivery = %v, want %q: a configured relay is still a configured relay", got, deliverySMTP)
	}
	if sender.calls == 0 {
		t.Fatal("the relay should have been attempted")
	}
}

// An unknown address must answer exactly like a known one — including the
// delivery field — or the response becomes an account-existence oracle.
func TestForgotDeliveryIsSameForUnknownAccount(t *testing.T) {
	sender := &realSender{}
	f := newResetFixtureWithMail(t, sender)
	_, known := f.post("/api/auth/forgot", map[string]any{"identifier": "reset@veylix.test"})
	_, unknown := f.post("/api/auth/forgot", map[string]any{"identifier": "nobody@veylix.test"})
	if known["delivery"] != unknown["delivery"] {
		t.Fatalf("delivery differs: known=%v unknown=%v — this leaks account existence",
			known["delivery"], unknown["delivery"])
	}
}
