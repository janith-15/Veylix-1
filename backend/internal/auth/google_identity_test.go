package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"veylix/backend/internal/auth/google"
	"veylix/backend/internal/store"
)

// These cases are the whole point of D-039's linking rules:
//
//	unknown e-mail                  → a new client account (Google verified it);
//	existing e-mail + Google ✓      → LINK to that account, no duplicate;
//	existing e-mail + Google ✗      → REFUSE (this is the account-takeover path);
//	already linked                  → sign in as that user.
func TestGoogleLinkingRules(t *testing.T) {
	cases := []struct {
		name         string
		existingMail string
		ident        google.Identity
		wantErr      error
		wantCreated  bool
	}{
		{
			name:        "new e-mail creates a client account",
			ident:       google.Identity{Subject: "sub-new", Email: "new@example.com", EmailVerified: true, Name: "New"},
			wantCreated: true,
		},
		{
			name:         "existing verified e-mail is linked, not duplicated",
			existingMail: "owner@example.com",
			ident:        google.Identity{Subject: "sub-own", Email: "owner@example.com", EmailVerified: true},
			wantCreated:  false,
		},
		{
			name:         "existing e-mail + UNVERIFIED google address is refused",
			existingMail: "victim@example.com",
			ident:        google.Identity{Subject: "sub-evil", Email: "victim@example.com", EmailVerified: false},
			wantErr:      ErrGoogleEmailUnverified,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeStore()
			var seededID int64
			if tc.existingMail != "" {
				seededID = st.seedUser(tc.existingMail, string(RoleClient))
			}
			g := &GoogleAuth{Store: st, Now: nowFn}

			usr, created, err := g.Resolve(context.Background(), tc.ident)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if _, linked := st.bySubject[tc.ident.Subject]; linked {
					t.Fatal("a refused identity was linked anyway")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if created != tc.wantCreated {
				t.Errorf("created = %v, want %v", created, tc.wantCreated)
			}
			if usr == nil || usr.Email != tc.ident.Email {
				t.Fatalf("user = %+v, want %s", usr, tc.ident.Email)
			}
			if created && usr.Role != string(RoleClient) {
				t.Errorf("a Google sign-up must be a client, got %s (a caller may never pick its role)", usr.Role)
			}
			if tc.existingMail != "" && usr.ID != seededID {
				t.Errorf("linked to user %d, want the existing account %d", usr.ID, seededID)
			}
			if id, ok := st.bySubject[tc.ident.Subject]; !ok || id != usr.ID {
				t.Error("the Google subject was not linked to the resolved user")
			}
		})
	}
}

// Signing in again through Google must find the linked account, not create one.
func TestGoogleSecondLoginDoesNotCreateAnAccount(t *testing.T) {
	st := newFakeStore()
	g := &GoogleAuth{Store: st, Now: nowFn}
	ident := google.Identity{Subject: "sub-x", Email: "x@example.com", EmailVerified: true}

	first, created, err := g.Resolve(context.Background(), ident)
	if err != nil || !created {
		t.Fatalf("first login: created=%v err=%v", created, err)
	}
	second, created2, err := g.Resolve(context.Background(), ident)
	if err != nil || created2 {
		t.Fatalf("second login: created=%v err=%v (must sign in, not sign up)", created2, err)
	}
	if second.ID != first.ID {
		t.Fatalf("second login resolved to %d, want %d", second.ID, first.ID)
	}
	if len(st.users) != 1 {
		t.Fatalf("%d accounts exist, want 1", len(st.users))
	}
}

// An OAuth state that this server did not issue (or one already used) must be
// refused before any token work happens.
func TestOAuthStateIsSingleUse(t *testing.T) {
	st := newFakeStore()
	if err := st.CreateOAuthState(context.Background(), store.OAuthState{
		State: "s1", Verifier: "v", Nonce: "n", Expires: time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatalf("create state: %v", err)
	}
	if _, err := st.ConsumeOAuthState(context.Background(), "s1"); err != nil {
		t.Fatalf("first use refused: %v", err)
	}
	if _, err := st.ConsumeOAuthState(context.Background(), "s1"); err == nil {
		t.Fatal("a replayed state was accepted (CSRF window)")
	}
	if _, err := st.ConsumeOAuthState(context.Background(), "never-issued"); err == nil {
		t.Fatal("a state this server never issued was accepted")
	}
}

// Without credentials Finish must fail closed, never fall through to a login.
func TestGoogleWithoutCredentialsFailsClosed(t *testing.T) {
	st := newFakeStore()
	g := &GoogleAuth{Store: st, Client: google.New(google.Config{})}
	if _, err := g.Start(context.Background()); !errors.Is(err, ErrGoogleNotConfigured) {
		t.Fatalf("Start err = %v, want ErrGoogleNotConfigured", err)
	}
	if _, _, err := g.Finish(context.Background(), "state", "code"); !errors.Is(err, ErrGoogleNotConfigured) {
		t.Fatalf("Finish err = %v, want ErrGoogleNotConfigured", err)
	}
	if len(st.users) != 0 {
		t.Fatal("a login was created without a verified identity")
	}
}
