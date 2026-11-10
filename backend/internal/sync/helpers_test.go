package sync_test

import (
	"context"

	"veylix/backend/internal/panel"
	veylixsync "veylix/backend/internal/sync"
)

// fakePanel serves a fixed client list, optionally failing.
type fakePanel struct {
	rows      []panel.ClientRow
	total     int
	listErr   error
	health    panel.HealthResult
	listCalls int
	pageSizes []int
	// hang blocks ListClients until the context is done (timeout test).
	hang bool

	// existsByEmail overrides the ClientExists answer for specific emails.
	// Without an entry, a client counts as existing iff it is in `rows`, which
	// keeps the older tests' intent ("a client in the list is a live client").
	existsByEmail map[string]bool
	// existsErr, when set, makes every verification fail — the "panel became
	// unreadable" case.
	existsErr   error
	existsCalls int
}

func (f *fakePanel) Health(context.Context) panel.HealthResult { return f.health }

// ClientExists answers the verified-absence read (D-024.5).
func (f *fakePanel) ClientExists(_ context.Context, email string) (bool, error) {
	f.existsCalls++
	if f.existsErr != nil {
		return false, f.existsErr
	}
	if f.existsByEmail != nil {
		if v, ok := f.existsByEmail[email]; ok {
			return v, nil
		}
	}
	for _, r := range f.rows {
		if r.Email == email {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakePanel) ListClients(ctx context.Context, page, pageSize int) (*panel.ClientPage, error) {
	f.listCalls++
	f.pageSizes = append(f.pageSizes, pageSize)
	if f.hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	start := (page - 1) * pageSize
	if start >= len(f.rows) {
		return &panel.ClientPage{Items: nil, Total: f.total, Page: page, PageSize: pageSize}, nil
	}
	end := start + pageSize
	if end > len(f.rows) {
		end = len(f.rows)
	}
	return &panel.ClientPage{
		Items: f.rows[start:end], Total: f.total, Page: page, PageSize: pageSize,
	}, nil
}

// clientRow builds a panel row with a traffic block.
func clientRow(email string, up, down int64, resetCount int, enable bool) panel.ClientRow {
	return panel.ClientRow{
		Email:  email,
		Enable: enable,
		Traffic: &panel.ClientTraffic{
			Email: email, Up: up, Down: down, ResetCount: resetCount, Enable: enable,
		},
	}
}

// factoryFor returns a factory that always yields pf for the given server.
func factoryFor(pf veylixsync.Panel) veylixsync.ClientFactory {
	return func(context.Context, int64) (veylixsync.Panel, error) { return pf, nil }
}

// failingFactory returns a factory that refuses every server.
func failingFactory(err error) veylixsync.ClientFactory {
	return func(context.Context, int64) (veylixsync.Panel, error) { return nil, err }
}
