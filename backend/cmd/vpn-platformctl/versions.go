package main

import (
	"context"
	"time"

	"veylix/backend/internal/versioning"
)

// cmdVersions lists every published release and marks the one running here.
//
// This is the answer to "which versions exist, and which am I on?" — the same
// question 3X-UI answers with `x-ui -v` plus its update menu. It reads the
// RUNNING service first, because the process is the only authority on what is
// actually executing.
func cmdVersions(p paths, args []string) error {
	fs := newFlagSet("versions")
	o := addCommonFlags(fs)
	limit := fs.Int("limit", 20, "how many releases to show (0 = all)")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	// What is running right now: the process, not the file on disk.
	current := ""
	if rep, probeErr := probeReady(o.base); probeErr == nil {
		current = rep.Version
	}
	if current == "" {
		current = version
	}

	repo := repoSlug()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// VEYLIX_RELEASE_API lets a fork (or a test) point at its own release feed
	// instead of GitHub. It defaults to the public API.
	fetcher := versioning.NewGitHubFetcher(repo)
	fetcher.BaseURL = envOr("VEYLIX_RELEASE_API", "https://api.github.com")
	svc := versioning.New(fetcher, time.Hour)
	rows, relErr := svc.MarkedReleases(ctx, current)

	if o.json {
		if relErr != nil {
			return printJSONErr(map[string]any{
				"repo":      repo,
				"installed": current,
				"error":     relErr.Error(),
			}, relErr.Error())
		}
		if limit != nil && *limit > 0 && len(rows) > *limit {
			rows = rows[:*limit]
		}
		return printJSON(map[string]any{
			"repo":      repo,
			"installed": current,
			"releases":  rows,
		})
	}

	out("repo:      %s", repo)
	out("installed: %s", current)
	out("")

	if relErr != nil {
		// The build version is still worth printing; only the list is missing.
		warnf("could not read the release feed: %v", relErr)
		out("")
		out("Install one anyway:  vpn-platformctl update --version <tag>")
		return relErr
	}
	if len(rows) == 0 {
		out("the release feed published no releases yet")
		return nil
	}

	shown := rows
	if limit != nil && *limit > 0 && len(shown) > *limit {
		shown = shown[:*limit]
	}

	out("%-14s %-12s %s", "VERSION", "PUBLISHED", "")
	out("%-14s %-12s %s", "-------", "---------", "----")
	foundCurrent := false
	for _, r := range shown {
		mark := ""
		switch {
		case r.IsCurrent:
			mark = "<- installed"
			foundCurrent = true
		case r.IsLatest:
			mark = "(latest)"
		}
		if r.Prerelease {
			mark = trimJoin(mark, "prerelease")
		}
		day := r.PublishedAt
		if len(day) >= 10 {
			day = day[:10]
		}
		out("%-14s %-12s %s", r.Tag, day, mark)
	}

	out("")
	if !foundCurrent {
		// Say so rather than leaving the operator to notice a missing marker.
		warnf("the installed version %q is not in the release feed (a development build?)", current)
	}
	out("Install one:  vpn-platformctl update --version <tag>")
	out("Downgrade:    vpn-platformctl update --version <tag> --allow-downgrade --restore-backup <dir>")
	return nil
}

// trimJoin appends a word to a marker, keeping a single space between them.
func trimJoin(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}
