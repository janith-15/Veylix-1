package versioning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHubFetcher reads https://api.github.com/repos/{repo}/releases.
//
// It is deliberately dumb: no pagination walking, no auth token. One request,
// one bounded read. The TTL cache above is what keeps it inside GitHub's
// unauthenticated rate limit (60 requests/hour).
type GitHubFetcher struct {
	Repo   string
	Client *http.Client
	// BaseURL is overridable so a fork or a test can point elsewhere.
	BaseURL string
}

// NewGitHubFetcher builds a fetcher with a bounded client, because a hung
// release feed must not stall a settings page.
func NewGitHubFetcher(repo string) *GitHubFetcher {
	return &GitHubFetcher{
		Repo:    repo,
		Client:  &http.Client{Timeout: 10 * time.Second},
		BaseURL: "https://api.github.com",
	}
}

// Releases implements Fetcher.
func (g *GitHubFetcher) Releases(ctx context.Context) ([]Release, error) {
	if strings.TrimSpace(g.Repo) == "" {
		return nil, errors.New("no release repository is configured")
	}
	base := g.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=100", strings.TrimRight(base, "/"), g.Repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the release feed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to decode
	case http.StatusNotFound:
		return nil, fmt.Errorf("the release feed for %q was not found", g.Repo)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("the release feed refused the request (rate limit); try again later")
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("the release feed answered %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw []struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("the release feed returned unreadable JSON: %w", err)
	}

	out := make([]Release, 0, len(raw))
	for _, r := range raw {
		// A draft is not published, so it must never be offered as an update.
		if r.Draft || strings.TrimSpace(r.TagName) == "" {
			continue
		}
		out = append(out, Release{
			Tag:         r.TagName,
			PublishedAt: r.PublishedAt,
			NotesURL:    r.HTMLURL,
			Prerelease:  r.Prerelease,
		})
	}
	return out, nil
}
