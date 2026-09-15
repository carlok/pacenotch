// Package update asks GitHub whether a newer pacenotch release exists. It reads only the
// latest release's tag and page address; it never downloads or installs anything.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// LatestURL is GitHub's latest-release API for this repository.
	LatestURL = "https://api.github.com/repos/carlok/pacenotch/releases/latest"
	// ReleasesPrefix is the only kind of page a found release may point at.
	ReleasesPrefix = "https://github.com/carlok/pacenotch/releases/"
	// Interval is the minimum time between two checks.
	Interval = 24 * time.Hour
)

// Errors are fixed strings.
var (
	ErrRateLimited = errors.New("GitHub rate limit reached")
	ErrBadResponse = errors.New("unexpected answer from GitHub")
	ErrNetwork     = errors.New("network error")
)

// Release is a published release.
type Release struct {
	Tag string // "v0.4.0"
	URL string // its page on github.com
}

// Version is major.minor.patch.
type Version [3]int

// ParseSemver reads "v1.2.3" or "1.2.3". Pre-releases, build metadata and git describe
// versions ("v0.1.3-5-g84f4fdf") are not release versions and are rejected.
func ParseSemver(s string) (Version, bool) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var v Version
	for i, p := range parts {
		if p == "" || len(p) > 9 || strings.Trim(p, "0123456789") != "" {
			return Version{}, false
		}
		v[i], _ = strconv.Atoi(p)
	}
	return v, true
}

// Less compares two versions.
func (v Version) Less(o Version) bool {
	for i := range v {
		if v[i] != o[i] {
			return v[i] < o[i]
		}
	}
	return false
}

// Newer reports whether latest is a newer release than current. Builds that are not
// releases (dev, git describe) never see a newer one.
func Newer(current, latest string) bool {
	c, okC := ParseSemver(current)
	l, okL := ParseSemver(latest)
	return okC && okL && c.Less(l)
}

// Due reports whether a check is due, last being the previous check.
func Due(last, now time.Time) bool {
	return now.Sub(last) >= Interval
}

// Doer is the part of *http.Client a Checker needs.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Checker reads the latest release.
type Checker struct {
	URL     string
	Client  Doer
	Timeout time.Duration
}

// NewChecker uses GitHub and a 10 s timeout.
func NewChecker() *Checker {
	return &Checker{URL: LatestURL, Client: http.DefaultClient, Timeout: 10 * time.Second}
}

// Latest returns the latest published release.
func (c *Checker) Latest(ctx context.Context) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return Release{}, ErrBadResponse
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pacenotch")
	resp, err := c.Client.Do(req)
	if err != nil {
		return Release{}, ErrNetwork
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden, http.StatusTooManyRequests:
		return Release{}, ErrRateLimited
	default:
		return Release{}, ErrBadResponse
	}
	var body struct {
		Tag        string `json:"tag_name"`
		URL        string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) != nil {
		return Release{}, ErrBadResponse
	}
	if _, ok := ParseSemver(body.Tag); !ok || body.Draft || body.Prerelease || !strings.HasPrefix(body.URL, ReleasesPrefix) {
		return Release{}, ErrBadResponse
	}
	return Release{Tag: body.Tag, URL: body.URL}, nil
}
