package auth

import (
	"context"
	"time"

	"veylix/backend/internal/store"
)

// fakeIdentityStore is an in-memory IdentityStore for the linking rules.
type fakeIdentityStore struct {
	states    map[string]store.OAuthState
	bySubject map[string]int64
	users     map[int64]*store.UserRow
	byEmail   map[string]int64
	nextID    int64
}

func newFakeStore() *fakeIdentityStore {
	return &fakeIdentityStore{
		states:    map[string]store.OAuthState{},
		bySubject: map[string]int64{},
		users:     map[int64]*store.UserRow{},
		byEmail:   map[string]int64{},
		nextID:    10,
	}
}

func (f *fakeIdentityStore) seedUser(email, role string) int64 {
	f.nextID++
	row := &store.UserRow{ID: f.nextID, Email: email, DisplayName: email, Role: role, EmailVerified: true}
	f.users[row.ID] = row
	f.byEmail[email] = row.ID
	return row.ID
}

func (f *fakeIdentityStore) CreateOAuthState(_ context.Context, st store.OAuthState) error {
	f.states[st.State] = st
	return nil
}

func (f *fakeIdentityStore) ConsumeOAuthState(_ context.Context, state string) (store.OAuthState, error) {
	st, ok := f.states[state]
	if !ok {
		return store.OAuthState{}, store.ErrNoSuchIdentity
	}
	delete(f.states, state) // single use: a replay finds nothing
	return st, nil
}

func (f *fakeIdentityStore) UserIDForIdentity(_ context.Context, _, subject string) (int64, error) {
	if id, ok := f.bySubject[subject]; ok {
		return id, nil
	}
	return 0, store.ErrNoSuchIdentity
}

func (f *fakeIdentityStore) LinkIdentity(_ context.Context, userID int64, _, subject, _ string) error {
	if existing, ok := f.bySubject[subject]; ok && existing != userID {
		return store.ErrIdentityTaken
	}
	f.bySubject[subject] = userID
	return nil
}

func (f *fakeIdentityStore) UserByEmail(_ context.Context, email string) (int64, string, error) {
	if id, ok := f.byEmail[email]; ok {
		return id, f.users[id].Role, nil
	}
	return 0, "", store.ErrNoSuchIdentity
}

func (f *fakeIdentityStore) CreateOAuthUser(_ context.Context, email, name, subject string) (int64, error) {
	id := f.seedUser(email, string(RoleClient))
	f.users[id].DisplayName = name
	f.bySubject[subject] = id
	return id, nil
}

func (f *fakeIdentityStore) UserByID(_ context.Context, id int64) (*store.UserRow, error) {
	if row, ok := f.users[id]; ok {
		return row, nil
	}
	return nil, store.ErrNoSuchIdentity
}

func (f *fakeIdentityStore) CreateEmailVerification(context.Context, store.EmailVerification) error {
	return nil
}

func (f *fakeIdentityStore) ConsumeEmailVerification(context.Context, string) (store.EmailVerification, error) {
	return store.EmailVerification{}, store.ErrNoSuchIdentity
}

func (f *fakeIdentityStore) MarkEmailVerified(context.Context, int64) error { return nil }

var _ IdentityStore = (*fakeIdentityStore)(nil)

// nowFn keeps the fake deterministic.
func nowFn() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
