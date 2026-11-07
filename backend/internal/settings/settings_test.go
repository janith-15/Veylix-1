package settings

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/config"
	"veylix/backend/internal/store"
)

// fakeStore is an in-memory Store so the service can be exercised without a
// database. It records exactly what was written, which is how the "the secret is
// sealed, not plaintext" assertion is made.
type fakeStore struct {
	rows map[string]store.AppSetting
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]store.AppSetting{}} }

func (f *fakeStore) GetAppSetting(_ context.Context, key string) (store.AppSetting, bool, error) {
	r, ok := f.rows[key]
	return r, ok, nil
}

func (f *fakeStore) SetAppSetting(_ context.Context, key string, value []byte, isSecret bool, updatedBy int64) error {
	f.rows[key] = store.AppSetting{Key: key, ValueCiphertext: value, IsSecret: isSecret, UpdatedBy: updatedBy}
	return nil
}

func newService(t *testing.T, env config.GoogleOAuth) (*Service, *fakeStore) {
	t.Helper()
	sealer, err := auth.NewSealer(bytes.Repeat([]byte("k"), 32))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	f := newFakeStore()
	return New(f, sealer, nil, env), f
}

func TestSaveSealsTheSecretAndNeverReturnsIt(t *testing.T) {
	svc, f := newService(t, config.GoogleOAuth{})
	ctx := context.Background()

	const secret = "GOCSPX-abcdefghijklmnop"
	view, err := svc.Save(ctx, GoogleInput{
		ClientID:     "123-abc.apps.googleusercontent.com",
		ClientSecret: secret,
		RedirectURL:  "https://vpn.example.com/api/auth/google/callback",
	}, 9)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !view.HasSecret || !view.Configured || view.Source != "database" {
		t.Fatalf("view = %+v, want has_secret+configured+database", view)
	}

	// Stored sealed: the row is flagged secret and does NOT contain the plaintext.
	row := f.rows[keyGoogleClientSecret]
	if !row.IsSecret {
		t.Fatal("the secret row is not flagged secret")
	}
	if bytes.Contains(row.ValueCiphertext, []byte(secret)) {
		t.Fatal("the secret is stored in plaintext")
	}
	if row.UpdatedBy != 9 {
		t.Fatalf("updated_by = %d, want 9", row.UpdatedBy)
	}

	// And it still resolves back to the exact value.
	r, err := svc.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.ClientSecret != secret {
		t.Fatalf("resolved secret = %q, want the saved one", r.ClientSecret)
	}
}

func TestResolvePrefersDatabaseOverEnvironment(t *testing.T) {
	svc, _ := newService(t, config.GoogleOAuth{
		ClientID: "env.apps.googleusercontent.com", RedirectURL: "https://env/api/auth/google/callback",
	})
	ctx := context.Background()
	if _, err := svc.Save(ctx, GoogleInput{
		ClientID:     "db.apps.googleusercontent.com",
		ClientSecret: "GOCSPX-db",
		RedirectURL:  "https://db/api/auth/google/callback",
	}, 1); err != nil {
		t.Fatalf("Save: %v", err)
	}
	r, err := svc.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.ClientID != "db.apps.googleusercontent.com" || r.RedirectURL != "https://db/api/auth/google/callback" {
		t.Fatalf("resolved = %+v, want the database values", r)
	}
}

func TestSaveKeepsTheStoredSecretWhenBlank(t *testing.T) {
	svc, _ := newService(t, config.GoogleOAuth{})
	ctx := context.Background()
	const secret = "GOCSPX-keepme"
	if _, err := svc.Save(ctx, GoogleInput{
		ClientID: "a.apps.googleusercontent.com", ClientSecret: secret, RedirectURL: "https://x/api/auth/google/callback",
	}, 1); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	// A second save with no secret must not erase it.
	if _, err := svc.Save(ctx, GoogleInput{
		ClientID: "a.apps.googleusercontent.com", RedirectURL: "https://x/api/auth/google/callback",
	}, 1); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	r, _ := svc.Resolve(ctx)
	if r.ClientSecret != secret {
		t.Fatalf("secret = %q, want it kept", r.ClientSecret)
	}
}

func TestSaveValidatesShapes(t *testing.T) {
	svc, _ := newService(t, config.GoogleOAuth{})
	ctx := context.Background()
	cases := []GoogleInput{
		{ClientID: "not-a-google-id", ClientSecret: "GOCSPX-x", RedirectURL: "https://x/api/auth/google/callback"},
		{ClientID: "a.apps.googleusercontent.com", ClientSecret: "wrong-prefix", RedirectURL: "https://x/api/auth/google/callback"},
		{ClientID: "a.apps.googleusercontent.com", ClientSecret: "GOCSPX-x", RedirectURL: "not a url"},
		{ClientID: "a.apps.googleusercontent.com", ClientSecret: "GOCSPX-x", RedirectURL: "ftp://x/api/auth/google/callback"},
		{ClientID: "a.apps.googleusercontent.com", ClientSecret: "GOCSPX-x", RedirectURL: "https://x/login"},
	}
	for i, in := range cases {
		if _, err := svc.Save(ctx, in, 1); err == nil {
			t.Fatalf("case %d: expected a validation error", i)
		} else {
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("case %d: error %v is not a ValidationError", i, err)
			}
		}
	}
}

func TestFirstTimeNeedsASecret(t *testing.T) {
	svc, _ := newService(t, config.GoogleOAuth{})
	_, err := svc.Save(context.Background(), GoogleInput{
		ClientID: "a.apps.googleusercontent.com", RedirectURL: "https://x/api/auth/google/callback",
	}, 1)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a ValidationError requiring a secret", err)
	}
}

// TestSaveRequiresTheCallbackPath is the regression guard for the mismatch the
// operator cannot see coming. A redirect URL that does not end at the one route
// Veylix answers yields `redirect_uri_mismatch` at sign-in — the whole Google
// feature fails, and the message blames Google. It is refused at save time.
func TestSaveRequiresTheCallbackPath(t *testing.T) {
	svc, _ := newService(t, config.GoogleOAuth{})
	ctx := context.Background()

	bad := []string{
		"https://vpn.example.com/",
		"https://vpn.example.com/login",
		"https://vpn.example.com/api/auth/google",           // missing the tail
		"https://vpn.example.com/api/auth/google/callback/", // trailing slash is not the route
		"https://vpn.example.com/oauth2/callback",           // some other provider's shape
	}
	for _, r := range bad {
		_, err := svc.Save(ctx, GoogleInput{
			ClientID: "a.apps.googleusercontent.com", ClientSecret: "GOCSPX-x", RedirectURL: r,
		}, 1)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("Save(%q) = %v, want a ValidationError (wrong callback path)", r, err)
		}
	}

	// The canonical path is accepted, and so is a reverse-proxy sub-path that
	// still ends at it — the tail is what must match, not the whole path.
	good := []string{
		"https://vpn.example.com/api/auth/google/callback",
		"https://vpn.example.com/veylix/api/auth/google/callback",
	}
	for _, r := range good {
		if _, err := svc.Save(ctx, GoogleInput{
			ClientID: "a.apps.googleusercontent.com", ClientSecret: "GOCSPX-x", RedirectURL: r,
		}, 1); err != nil {
			t.Fatalf("Save(%q) = %v, want success", r, err)
		}
	}
}

func TestOnGoogleChangeFiresWithTheResolvedValue(t *testing.T) {
	svc, _ := newService(t, config.GoogleOAuth{})
	var got ResolvedGoogle
	svc.OnGoogleChange = func(r ResolvedGoogle) { got = r }
	if _, err := svc.Save(context.Background(), GoogleInput{
		ClientID: "a.apps.googleusercontent.com", ClientSecret: "GOCSPX-x", RedirectURL: "https://x/api/auth/google/callback",
	}, 1); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got.ClientID != "a.apps.googleusercontent.com" || got.ClientSecret != "GOCSPX-x" {
		t.Fatalf("OnGoogleChange got %+v, want the saved client", got)
	}
}
