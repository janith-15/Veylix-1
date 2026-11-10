package main

import (
	"context"
	"runtime"

	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/versioning"
)

// systemService adapts internal/versioning to the httpapi.SystemServices
// interface. It lives in cmd/ on purpose: the transport layer must not import
// the release feed, and the feed must not know about HTTP.
type systemService struct {
	ver *versioning.Service
}

// Info reports the running build. The schema version is filled in by the HTTP
// layer, which is the only place that holds the Health reader.
func (s systemService) Info(context.Context) (httpapi.SystemInfo, error) {
	return httpapi.SystemInfo{
		Version:   version,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}, nil
}

// Releases decorates the feed with is_current / is_latest against this build.
func (s systemService) Releases(ctx context.Context) ([]httpapi.ReleaseInfo, error) {
	rows, err := s.ver.MarkedReleases(ctx, version)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.ReleaseInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.ReleaseInfo{
			Tag:         r.Tag,
			PublishedAt: r.PublishedAt,
			IsCurrent:   r.IsCurrent,
			IsLatest:    r.IsLatest,
			NotesURL:    r.NotesURL,
		})
	}
	return out, nil
}

// CheckUpdate compares the running build against the newest stable release.
func (s systemService) CheckUpdate(ctx context.Context, current string) (httpapi.UpdateCheck, error) {
	res, err := s.ver.CheckUpdate(ctx, current)
	return httpapi.UpdateCheck{
		Current:         res.Current,
		Latest:          res.Latest,
		UpdateAvailable: res.UpdateAvailable,
		CheckedAt:       res.CheckedAt,
		ReleaseURL:      res.ReleaseURL,
		Error:           res.Error,
	}, err
}

// newSystemService builds the adapter. An empty repo disables the release feed:
// the version still reports, and the feed endpoints answer an explicit error
// instead of pretending there are no releases.
func newSystemService(repo string) systemService {
	var fetcher versioning.Fetcher
	if repo != "" {
		fetcher = versioning.NewGitHubFetcher(repo)
	}
	return systemService{ver: versioning.New(fetcher, 0)} // 0 -> default 1h TTL
}
