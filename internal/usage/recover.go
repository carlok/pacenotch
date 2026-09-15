package usage

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Defaults for RecoveringSource.
const (
	RefreshGap  = 10 * time.Minute // minimum time between two refresh attempts
	AuthRecheck = 15 * time.Minute // call the endpoint again even if the credentials did not change
)

// NeedsRefresh reports whether a fetch shows an expired token: HTTP 401, or a success that
// came with the expired-token warning.
func NeedsRefresh(err error, warn string) bool {
	var fe *FetchError
	return (errors.As(err, &fe) && fe.Status == 401) || (err == nil && warn == WarnExpired)
}

type fingerprinter interface {
	Fingerprint(ctx context.Context) (string, error)
}

// RecoveringSource wraps a Source for expired tokens. It never touches the credentials.
//
// When the token has expired it calls the refresh function (which asks Claude Code to
// refresh its own token) at most every MinGap, then fetches once more. While
// authentication keeps failing and the credentials have not changed, it answers with the
// last failure instead of calling the endpoint again, until Recheck elapses.
type RecoveringSource struct {
	Inner       Source
	Fingerprint func(ctx context.Context) (string, error) // nil: credentials never look changed
	Now         func() time.Time
	MinGap      time.Duration
	Recheck     time.Duration

	mu          sync.Mutex
	refresh     func(ctx context.Context) error
	lastRefresh time.Time
	authErr     error
	authFP      string
	authAt      time.Time
}

// NewRecoveringSource wraps inner. refresh may be nil (no automatic refresh). The credential
// fingerprint comes from inner when it can provide one, like *Fetcher.
func NewRecoveringSource(inner Source, refresh func(ctx context.Context) error, now func() time.Time) *RecoveringSource {
	s := &RecoveringSource{Inner: inner, Now: now, MinGap: RefreshGap, Recheck: AuthRecheck, refresh: refresh}
	if f, ok := inner.(fingerprinter); ok {
		s.Fingerprint = f.Fingerprint
	}
	return s
}

// SetRefresh turns automatic refresh on (non-nil) or off.
func (s *RecoveringSource) SetRefresh(refresh func(ctx context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh = refresh
}

// Fetch implements Source.
func (s *RecoveringSource) Fetch(ctx context.Context) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	if s.authErr != nil && !s.refreshDue(now) && now.Sub(s.authAt) < s.Recheck && s.fingerprint(ctx) == s.authFP {
		return nil, "", s.authErr
	}
	body, warn, err := s.Inner.Fetch(ctx)
	if NeedsRefresh(err, warn) && s.refreshDue(now) {
		s.lastRefresh = now
		if s.refresh(ctx) == nil {
			body, warn, err = s.Inner.Fetch(ctx)
		}
	}
	s.authErr = nil
	if isAuth(err) {
		s.authErr, s.authAt, s.authFP = err, now, s.fingerprint(ctx)
	}
	return body, warn, err
}

func (s *RecoveringSource) refreshDue(now time.Time) bool {
	return s.refresh != nil && (s.lastRefresh.IsZero() || now.Sub(s.lastRefresh) >= s.MinGap)
}

func (s *RecoveringSource) fingerprint(ctx context.Context) string {
	if s.Fingerprint == nil {
		return ""
	}
	fp, err := s.Fingerprint(ctx)
	if err != nil {
		return ""
	}
	return fp
}
