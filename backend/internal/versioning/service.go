// Package versioning answers "which release of Veylix is running, what else
// exists, and is there something newer?".
//
// It exists because an operator could not previously tell, from inside the
// product, whether the running binary was current. 3X-UI exposes exactly this as
// `x-ui -v` plus `GET /panel/api/server/update-check`; this is the same idea with
// the same honesty rules:
//
//   - the RUNNING PROCESS is the authority on the current version, never a file
//     on disk (a file can be replaced without the process restarting);
//   - a failed network read returns an explicit error, never "up to date";
//   - the release list is cached, because GitHub rate-limits unauthenticated
//     callers and a panel that hammers it gets blocked.
package versioning

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Release is one published version.
type Release struct {
	Tag         string `json:"tag"`
	PublishedAt string `json:"published_at,omitempty"`
	IsCurrent   bool   `json:"is_current"`
	IsLatest    bool   `json:"is_latest"`
	NotesURL    string `json:"notes_url,omitempty"`
	Prerelease  bool   `json:"prerelease,omitempty"`
}

// UpdateCheck is the answer to "is there something newer?".
type UpdateCheck struct {
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	CheckedAt       string `json:"checked_at,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	Error           string `json:"error,omitempty"`
}

// Fetcher reads the release feed. It is an interface so tests can drive the
// service without a network, and so a fork can point at its own feed.
type Fetcher interface {
	// Releases returns every published release, newest first.
	Releases(ctx context.Context) ([]Release, error)
}

// Service caches the feed and compares versions.
type Service struct {
	fetcher Fetcher
	ttl     time.Duration
	now     func() time.Time

	mu       sync.Mutex
	cached   []Release
	cachedAt time.Time
}

// New builds the service. A nil fetcher makes every call fail with a clear
// message rather than returning an empty list that reads as "no releases".
func New(f Fetcher, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &Service{fetcher: f, ttl: ttl, now: time.Now}
}

// SetClock overrides the clock. Tests use it; production does not.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Releases returns the cached feed, refreshing it when the TTL has expired.
func (s *Service) Releases(ctx context.Context) ([]Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.cached) > 0 && s.now().Sub(s.cachedAt) < s.ttl {
		return s.cached, nil
	}
	if s.fetcher == nil {
		return nil, errors.New("no release feed is configured on this installation")
	}
	rows, err := s.fetcher.Releases(ctx)
	if err != nil {
		// Keep serving the last good answer if the remote is briefly down: a
		// stale list is useful, a wrong one is not.
		if len(s.cached) > 0 {
			return s.cached, nil
		}
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return CompareVersions(rows[i].Tag, rows[j].Tag) > 0
	})
	s.cached = rows
	s.cachedAt = s.now()
	return rows, nil
}

// CheckUpdate compares current against the newest non-prerelease release.
func (s *Service) CheckUpdate(ctx context.Context, current string) (UpdateCheck, error) {
	res := UpdateCheck{Current: current, CheckedAt: s.now().UTC().Format(time.RFC3339)}

	rows, err := s.Releases(ctx)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	latest := newestStable(rows)
	if latest.Tag == "" {
		res.Error = "the release feed published no stable release"
		return res, errors.New(res.Error)
	}
	res.Latest = latest.Tag
	res.ReleaseURL = latest.NotesURL
	res.UpdateAvailable = CompareVersions(latest.Tag, current) > 0
	return res, nil
}

// MarkedReleases decorates the feed with is_current / is_latest for the UI.
func (s *Service) MarkedReleases(ctx context.Context, current string) ([]Release, error) {
	rows, err := s.Releases(ctx)
	if err != nil {
		return nil, err
	}
	latest := newestStable(rows)
	out := make([]Release, len(rows))
	copy(out, rows)
	for i := range out {
		out[i].IsCurrent = SameVersion(out[i].Tag, current)
		out[i].IsLatest = latest.Tag != "" && SameVersion(out[i].Tag, latest.Tag)
	}
	return out, nil
}

// newestStable returns the highest non-prerelease tag, or the zero Release.
func newestStable(rows []Release) Release {
	var latest Release
	for _, r := range rows {
		if r.Prerelease {
			continue
		}
		if latest.Tag == "" || CompareVersions(r.Tag, latest.Tag) > 0 {
			latest = r
		}
	}
	return latest
}

// ---------------------------------------------------------------------------
// Version comparison
//
// Tags are compared by their leading numeric components, because lexicographic
// order puts v1.10.0 BEFORE v1.9.0, which would make an installed v1.10.0 look
// older than a published v1.9.0 and offer a "downgrade" as an update.
// ---------------------------------------------------------------------------

var versionRe = regexp.MustCompile(`(\d+)`)

// ParseVersion returns the numeric components of a tag: "v1.4.2" -> [1,4,2].
func ParseVersion(tag string) []int {
	trimmed := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	m := versionRe.FindAllString(trimmed, -1)
	out := make([]int, 0, len(m))
	for _, s := range m {
		n, err := strconv.Atoi(s)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// CompareVersions returns >0 when a is newer, <0 when older, 0 when equal.
//
// A tag with no digits ("dev") sorts BELOW every numbered release, which is what
// makes a development build report as out of date rather than current.
func CompareVersions(a, b string) int {
	pa, pb := ParseVersion(a), ParseVersion(b)
	if len(pa) == 0 && len(pb) == 0 {
		return 0
	}
	if len(pa) == 0 {
		return -1
	}
	if len(pb) == 0 {
		return 1
	}
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		switch {
		case x > y:
			return 1
		case x < y:
			return -1
		}
	}
	return 0
}

// SameVersion reports whether two tags name the same release numerically.
func SameVersion(a, b string) bool { return CompareVersions(a, b) == 0 }
