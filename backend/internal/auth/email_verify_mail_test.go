package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"veylix/backend/internal/store"
)

// fakeMailer records what a verification link was sent to.
type fakeMailer struct {
	calls   int
	to      []string
	body    []string
	fail    error
	enabled bool
}

func (m *fakeMailer) Configured() bool { return m.enabled }

func (m *fakeMailer) Send(_ context.Context, to, _, text, _ string) error {
	m.calls++
	m.to = append(m.to, to)
	m.body = append(m.body, text)
	return m.fail
}

// verifyStore satisfies IdentityStore for the verifier tests. It embeds the
// existing in-memory fake and overrides only the two verification methods, so
// the suite keeps one implementation of the other eight.
type verifyStore struct {
	*fakeIdentityStore
	created []store.EmailVerification
}

func newVerifyStore() *verifyStore {
	return &verifyStore{fakeIdentityStore: newFakeStore()}
}

func (s *verifyStore) CreateEmailVerification(_ context.Context, v store.EmailVerification) error {
	s.created = append(s.created, v)
	return nil
}

func (s *verifyStore) ConsumeEmailVerification(_ context.Context, token string) (store.EmailVerification, error) {
	for _, v := range s.created {
		if v.Token == token {
			return v, nil
		}
	}
	return store.EmailVerification{}, ErrVerificationToken
}

func newVerifier(m EmailSender) (*EmailVerifier, *verifyStore) {
	st := newVerifyStore()
	return &EmailVerifier{Store: st, Mail: m, BaseURL: "https://vpn.example"}, st
}

// A verification link must actually reach the mailbox: before this, Issue()
// created a token and mailed nothing, so "verify your e-mail" was impossible to
// complete and every self-registered client was locked out at the door.
func TestVerifyLinkIsEmailed(t *testing.T) {
	m := &fakeMailer{enabled: true}
	v, st := newVerifier(m)
	token, delivery, err := v.IssueAndSend(context.Background(), 7, "user@example.com")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if delivery != "smtp" {
		t.Fatalf("delivery = %q, want smtp", delivery)
	}
	if m.calls != 1 {
		t.Fatalf("sent %d messages, want 1", m.calls)
	}
	if m.to[0] != "user@example.com" {
		t.Fatalf("sent to %q, want the account address", m.to[0])
	}
	if len(st.created) != 1 || st.created[0].Token != token {
		t.Fatal("the emailed token must be the stored one")
	}
	// An absolute link: a relative path is useless inside a mailbox.
	if !strings.Contains(m.body[0], "https://vpn.example/api/auth/verify/"+token) {
		t.Fatalf("body lacks an absolute link, got:\n%s", m.body[0])
	}
}

// With no relay the token still exists (an operator can hand it over) but the
// response must say `outbox` rather than claiming a delivery.
func TestVerifyLinkWithoutRelayIsOutbox(t *testing.T) {
	v, st := newVerifier(nil)
	token, delivery, err := v.IssueAndSend(context.Background(), 7, "user@example.com")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if delivery != "outbox" {
		t.Fatalf("delivery = %q, want outbox", delivery)
	}
	if token == "" || len(st.created) != 1 {
		t.Fatal("the token must still be created without a relay")
	}
}

// A relay that rejects the message must be reported as outbox: saying "smtp"
// would leave the user waiting for a link that cannot arrive.
func TestVerifyLinkReportsOutboxWhenSendFails(t *testing.T) {
	m := &fakeMailer{enabled: true, fail: errors.New("mail: auth: 535 bad credentials")}
	v, _ := newVerifier(m)
	_, delivery, err := v.IssueAndSend(context.Background(), 7, "user@example.com")
	if err == nil {
		t.Fatal("a failed send must surface an error")
	}
	if delivery != "outbox" {
		t.Fatalf("delivery = %q, want outbox after a send failure", delivery)
	}
}

// Delivery is a property of the server, so it must be identical for every
// caller — otherwise the endpoint becomes an account-existence oracle.
func TestVerifyDeliveryIsServerLevel(t *testing.T) {
	on, _ := newVerifier(&fakeMailer{enabled: true})
	got, err := on.Delivery(context.Background())
	if err != nil || got != "smtp" {
		t.Fatalf("configured relay: %q %v, want smtp", got, err)
	}
	off, _ := newVerifier(nil)
	got, err = off.Delivery(context.Background())
	if err != nil || got != "outbox" {
		t.Fatalf("no relay: %q %v, want outbox", got, err)
	}
}

// The token keeps the documented 24h life; a shorter default would expire
// between the e-mail arriving and a person opening it.
func TestVerifyTokenDefaultsTo24Hours(t *testing.T) {
	m := &fakeMailer{enabled: true}
	v, st := newVerifier(m)
	before := time.Now().Add(24 * time.Hour)
	if _, _, err := v.IssueAndSend(context.Background(), 7, "user@example.com"); err != nil {
		t.Fatalf("issue: %v", err)
	}
	exp := st.created[0].ExpiresAt
	if exp.Before(before.Add(-time.Minute)) {
		t.Fatalf("expiry %v is short of 24h from %v", exp, before)
	}
}
