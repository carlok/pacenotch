package gui

import (
	"context"
	"strings"
	"time"

	"github.com/carlok/pacenotch/internal/update"
)

// Update check preference keys: the last check (Unix seconds) and the latest release seen.
const (
	PrefUpdateLastCheck = "update.lastCheck"
	PrefUpdateTag       = "update.tag"
	PrefUpdateURL       = "update.url"
)

// UpdateText is the notice for a newer release, or "" when there is none.
func UpdateText(rel update.Release) string {
	if rel.Tag == "" {
		return ""
	}
	return "Update available: " + rel.Tag
}

// Updater checks GitHub for a newer release at most once a day, and only while enabled.
type Updater struct {
	Check   func(ctx context.Context) (update.Release, error)
	Store   Store
	Now     func() time.Time
	Version string
	Enabled func() bool
	OnFound func(update.Release) // called with a newer release, also one found on an earlier day
}

// Tick checks when a check is due, remembers the answer, and reports a newer release.
func (u *Updater) Tick(ctx context.Context) {
	if !u.Enabled() {
		return
	}
	now := u.Now()
	if update.Due(time.Unix(int64(u.Store.IntWithFallback(PrefUpdateLastCheck, 0)), 0), now) {
		u.Store.SetInt(PrefUpdateLastCheck, int(now.Unix())) // failures wait a day too
		if rel, err := u.Check(ctx); err == nil {
			u.Store.SetString(PrefUpdateTag, rel.Tag)
			u.Store.SetString(PrefUpdateURL, rel.URL)
		}
	}
	rel := update.Release{Tag: u.Store.StringWithFallback(PrefUpdateTag, ""), URL: u.Store.StringWithFallback(PrefUpdateURL, "")}
	if update.Newer(u.Version, rel.Tag) && strings.HasPrefix(rel.URL, update.ReleasesPrefix) {
		u.OnFound(rel)
	}
}

// Run ticks now and then every interval until ctx ends.
func (u *Updater) Run(ctx context.Context, every time.Duration, after func(time.Duration) <-chan time.Time) {
	for {
		u.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-after(every):
		}
	}
}
