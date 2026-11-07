package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// Registration used to create an unverified account and e-mail nothing, so the
// new client was refused at login with a message claiming a code had been sent.
// This is the regression test for that dead end (D-041).
func TestRegisterSendsVerificationLink(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir+"/reg.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditLog := audit.New(st.DB())
	svc := auth.NewService(st.DB(), auditLog) // autoVerify stays false, as in prod

	sender := &captureSender{configured: true}
	verifier := &auth.EmailVerifier{Store: st, Mail: sender, BaseURL: "https://vpn.example"}
	svc.SendVerification = func(ctx context.Context, userID int64, email string) (string, error) {
		_, delivery, err := verifier.IssueAndSend(ctx, userID, email)
		return delivery, err
	}

	if _, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "new@veylix.test", Password: "correct-horse-battery-9", DisplayName: "New Client",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if sender.calls != 1 {
		t.Fatalf("register sent %d messages, want exactly 1", sender.calls)
	}
	if sender.to[0] != "new@veylix.test" {
		t.Fatalf("sent to %q, want the new account", sender.to[0])
	}
	// The link must be absolute and carry a real token, or the mail is useless.
	if !strings.Contains(sender.body[0], "https://vpn.example/api/auth/verify/") {
		t.Fatalf("body lacks an absolute verification link:\n%s", sender.body[0])
	}

	// The account must still be unverified until the link is opened.
	var verified int
	if err := st.DB().QueryRow(`SELECT email_verified FROM users WHERE email = ?`,
		"new@veylix.test").Scan(&verified); err != nil {
		t.Fatalf("read flag: %v", err)
	}
	if verified != 0 {
		t.Fatal("register must not verify the address; only the link may")
	}
}

// A relay failure must not roll the account back: the person is registered and
// can still be helped, and the audit row carries the reason.
func TestRegisterSurvivesVerificationFailure(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir+"/regfail.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := auth.NewService(st.DB(), audit.New(st.DB()))
	sender := &captureSender{configured: true, fail: true}
	verifier := &auth.EmailVerifier{Store: st, Mail: sender, BaseURL: "https://vpn.example"}
	svc.SendVerification = func(ctx context.Context, userID int64, email string) (string, error) {
		_, delivery, err := verifier.IssueAndSend(ctx, userID, email)
		return delivery, err
	}

	if _, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "fail@veylix.test", Password: "correct-horse-battery-9", DisplayName: "F",
	}); err != nil {
		t.Fatalf("a mail failure must not fail registration: %v", err)
	}
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, "fail@veylix.test").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatal("the account must exist even when the verification e-mail fails")
	}
}

// captureSender is a stub relay that records what was handed to it.
type captureSender struct {
	calls      int
	to         []string
	body       []string
	configured bool
	fail       bool
}

func (s *captureSender) Configured() bool { return s.configured }

func (s *captureSender) Send(_ context.Context, to, _, text, _ string) error {
	s.calls++
	s.to = append(s.to, to)
	s.body = append(s.body, text)
	if s.fail {
		return http.ErrServerClosed
	}
	return nil
}

// The login refusal must name a LINK, because that is what the user receives.
func TestUnverifiedLoginMentionsTheLink(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAuthError(rec, auth.ErrEmailNotVerified)
	body := rec.Body.String()
	// Match the wording only: the JSON envelope always carries a "code" field
	// (email_unverified), which is a machine key, not a promise to the reader.
	if strings.Contains(body, "sent you a code") || strings.Contains(body, "6-digit") {
		t.Fatalf("message still promises a code the flow never sends: %s", body)
	}
	if !strings.Contains(body, "link") {
		t.Fatalf("message must name the link: %s", body)
	}
}
