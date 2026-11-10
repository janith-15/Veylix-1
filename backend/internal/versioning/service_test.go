package versioning

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeFetcher returns a fixed feed and records how often it was called, so the
// cache can be proven rather than assumed.
type fakeFetcher struct {
	rows  []Release
	err   error
	calls int
}

func (f *fakeFetcher) Releases(context.Context) ([]Release, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

// ---------------------------------------------------------------------------
// CompareVersions
// ---------------------------------------------------------------------------

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.4.0", "v1.3.0", 1},
		{"v1.3.0", "v1.4.0", -1},
		{"v1.4.0", "v1.4.0", 0},
		{"v1.4", "v1.4.0", 0},    // missing component reads as zero
		{"v1.10.0", "v1.9.0", 1}, // THE bug lexicographic order causes
		{"v1.9.0", "v1.10.0", -1},
		{"v2.0.0", "v1.99.99", 1},
		{"1.4.0", "v1.4.0", 0}, // the v prefix is optional
		{"dev", "v1.0.0", -1},  // an unversioned build is older than any release
		{"v1.0.0", "dev", 1},
		{"dev", "dev", 0},
	}
	for _, c := range cases {
		got := CompareVersions(c.a, c.b)
		switch {
		case got > 0:
			got = 1
		case got < 0:
			got = -1
		}
		if got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	got := ParseVersion("v1.4.2")
	if len(got) != 3 || got[0] != 1 || got[1] != 4 || got[2] != 2 {
		t.Fatalf("ParseVersion(v1.4.2) = %v, want [1 4 2]", got)
	}
	if p := ParseVersion("dev"); len(p) != 0 {
		t.Fatalf("ParseVersion(dev) = %v, want empty", p)
	}
}

func TestSameVersion(t *testing.T) {
	if !SameVersion("v1.4.0", "1.4.0") {
		t.Error("v1.4.0 and 1.4.0 must be the same release")
	}
	if SameVersion("v1.4.0", "v1.4.1") {
		t.Error("v1.4.0 and v1.4.1 must differ")
	}
}

// ---------------------------------------------------------------------------
// CheckUpdate
// ---------------------------------------------------------------------------

func TestCheckUpdateReportsAnAvailableUpdate(t *testing.T) {
	f := &fakeFetcher{rows: []Release{{Tag: "v1.5.0"}, {Tag: "v1.4.0"}}}
	s := New(f, time.Hour)

	res, err := s.CheckUpdate(context.Background(), "v1.4.0")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if !res.UpdateAvailable {
		t.Fatal("an installed v1.4.0 with a published v1.5.0 must report an update")
	}
	if res.Latest != "v1.5.0" {
		t.Fatalf("latest = %q, want v1.5.0", res.Latest)
	}
	if res.Current != "v1.4.0" {
		t.Fatalf("current = %q, want v1.4.0", res.Current)
	}
}

func TestCheckUpdateSaysUpToDateWhenCurrent(t *testing.T) {
	f := &fakeFetcher{rows: []Release{{Tag: "v1.5.0"}, {Tag: "v1.4.0"}}}
	s := New(f, time.Hour)

	res, err := s.CheckUpdate(context.Background(), "v1.5.0")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if res.UpdateAvailable {
		t.Fatal("the newest release must not report an update")
	}
}

func TestCheckUpdateIgnoresPrereleases(t *testing.T) {
	f := &fakeFetcher{rows: []Release{
		{Tag: "v2.0.0", Prerelease: true},
		{Tag: "v1.5.0"},
	}}
	s := New(f, time.Hour)

	res, err := s.CheckUpdate(context.Background(), "v1.5.0")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if res.UpdateAvailable {
		t.Fatal("a prerelease must not be offered as an update")
	}
	if res.Latest != "v1.5.0" {
		t.Fatalf("latest = %q, want the newest STABLE v1.5.0", res.Latest)
	}
}

func TestCheckUpdateSaysUnknownWhenTheFeedFails(t *testing.T) {
	f := &fakeFetcher{err: errors.New("network down")}
	s := New(f, time.Hour)

	res, err := s.CheckUpdate(context.Background(), "v1.4.0")
	if err == nil {
		t.Fatal("a failed feed must be an error, not a silent 'up to date'")
	}
	if res.UpdateAvailable {
		t.Fatal("a failed feed must never report an update")
	}
	if res.Error == "" {
		t.Fatal("a failed feed must set Error so the UI can say why")
	}
}

func TestCheckUpdateWithNoStableRelease(t *testing.T) {
	f := &fakeFetcher{rows: []Release{{Tag: "v2.0.0", Prerelease: true}}}
	s := New(f, time.Hour)

	res, err := s.CheckUpdate(context.Background(), "v1.4.0")
	if err == nil {
		t.Fatal("a feed with only prereleases must not be treated as current")
	}
	if res.Error == "" {
		t.Fatal("Error must explain that no stable release exists")
	}
}

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

func TestReleasesAreCachedWithinTheTTL(t *testing.T) {
	f := &fakeFetcher{rows: []Release{{Tag: "v1.0.0"}}}
	s := New(f, time.Hour)

	for i := 0; i < 5; i++ {
		if _, err := s.Releases(context.Background()); err != nil {
			t.Fatalf("Releases: %v", err)
		}
	}
	if f.calls != 1 {
		t.Fatalf("fetcher called %d times, want 1 (the TTL cache must absorb the rest)", f.calls)
	}
}

func TestCacheRefreshesAfterTheTTL(t *testing.T) {
	f := &fakeFetcher{rows: []Release{{Tag: "v1.0.0"}}}
	s := New(f, time.Minute)

	now := time.Now()
	s.SetClock(func() time.Time { return now })

	if _, err := s.Releases(context.Background()); err != nil {
		t.Fatalf("Releases: %v", err)
	}
	now = now.Add(2 * time.Minute) // past the TTL
	if _, err := s.Releases(context.Background()); err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if f.calls != 2 {
		t.Fatalf("fetcher called %d times, want 2 after the TTL elapsed", f.calls)
	}
}

func TestStaleCacheSurvivesARemoteOutage(t *testing.T) {
	f := &fakeFetcher{rows: []Release{{Tag: "v1.0.0"}}}
	s := New(f, time.Minute)

	now := time.Now()
	s.SetClock(func() time.Time { return now })
	if _, err := s.Releases(context.Background()); err != nil {
		t.Fatalf("Releases: %v", err)
	}

	// The feed dies and the TTL expires: the last good answer is still useful,
	// and returning an error here would blank the settings page for no reason.
	f.err = errors.New("network down")
	now = now.Add(2 * time.Minute)

	rows, err := s.Releases(context.Background())
	if err != nil {
		t.Fatalf("a populated cache must survive an outage, got %v", err)
	}
	if len(rows) != 1 || rows[0].Tag != "v1.0.0" {
		t.Fatalf("rows = %+v, want the cached v1.0.0", rows)
	}
}

func TestNoFetcherIsAnExplicitError(t *testing.T) {
	s := New(nil, time.Hour)
	if _, err := s.Releases(context.Background()); err == nil {
		t.Fatal("a nil fetcher must be an error, never an empty release list")
	}
}

// ---------------------------------------------------------------------------
// MarkedReleases
// ---------------------------------------------------------------------------

func TestMarkedReleasesFlagsCurrentAndLatest(t *testing.T) {
	f := &fakeFetcher{rows: []Release{
		{Tag: "v1.3.0"},
		{Tag: "v1.5.0"},
		{Tag: "v1.4.0"},
	}}
	s := New(f, time.Hour)

	rows, err := s.MarkedReleases(context.Background(), "v1.4.0")
	if err != nil {
		t.Fatalf("MarkedReleases: %v", err)
	}
	if rows[0].Tag != "v1.5.0" || rows[1].Tag != "v1.4.0" || rows[2].Tag != "v1.3.0" {
		t.Fatalf("rows are not newest-first: %+v", rows)
	}
	if !rows[0].IsLatest {
		t.Error("v1.5.0 must be flagged latest")
	}
	if !rows[1].IsCurrent {
		t.Error("v1.4.0 must be flagged current")
	}
	if rows[2].IsCurrent || rows[2].IsLatest {
		t.Error("v1.3.0 is neither current nor latest")
	}
}

// ---------------------------------------------------------------------------
// GitHubFetcher against a local server
// ---------------------------------------------------------------------------

func TestGitHubFetcherReadsReleasesAndSkipsDrafts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/releases" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.5.0","published_at":"2026-11-10T00:00:00Z","html_url":"https://x/v1.5.0","draft":false,"prerelease":false},
			{"tag_name":"v9.9.9","draft":true},
			{"tag_name":"v1.4.0","draft":false,"prerelease":false}
		]`))
	}))
	defer srv.Close()

	g := NewGitHubFetcher("o/r")
	g.BaseURL = srv.URL

	rows, err := g.Releases(context.Background())
	if err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (the draft must be dropped): %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.Tag == "v9.9.9" {
			t.Fatal("a draft was published into the release list")
		}
	}
}

func TestGitHubFetcherReportsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	g := NewGitHubFetcher("o/missing")
	g.BaseURL = srv.URL

	if _, err := g.Releases(context.Background()); err == nil {
		t.Fatal("a 404 feed must be an error")
	}
}

func TestGitHubFetcherReportsRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	g := NewGitHubFetcher("o/r")
	g.BaseURL = srv.URL

	_, err := g.Releases(context.Background())
	if err == nil {
		t.Fatal("a rate-limited feed must be an error")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("error = %v, want it to mention the rate limit", err)
	}
}

func TestGitHubFetcherRequiresARepo(t *testing.T) {
	g := NewGitHubFetcher("")
	if _, err := g.Releases(context.Background()); err == nil {
		t.Fatal("an empty repo must be refused before any request")
	}
}
