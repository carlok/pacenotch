package usage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var unauthorized = &FetchError{401, "HTTP 401: token rejected (start Claude Code once to refresh it)"}

type result struct {
	body, warn string
	err        error
}

// seqSource returns results[i] on call i; the last one repeats.
type seqSource struct {
	mu      sync.Mutex
	calls   int
	results []result
}

func (s *seqSource) Fetch(context.Context) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.results[min(s.calls, len(s.results)-1)]
	s.calls++
	if r.err != nil {
		return nil, r.warn, r.err
	}
	return []byte(r.body), r.warn, nil
}

func (s *seqSource) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestNeedsRefresh(t *testing.T) {
	tests := []struct {
		err  error
		warn string
		want bool
	}{
		{unauthorized, "", true},
		{nil, WarnExpired, true},
		{nil, "", false},
		{&FetchError{500, "HTTP 500 from usage endpoint"}, WarnExpired, false},
		{ErrNoCredentials, "", false},
	}
	for _, tt := range tests {
		if got := NeedsRefresh(tt.err, tt.warn); got != tt.want {
			t.Errorf("NeedsRefresh(%v, %q) = %v", tt.err, tt.warn, got)
		}
	}
}

func TestRecoverRefreshesAndRetries(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1789126200, 0)}
	for _, first := range []result{{err: unauthorized}, {body: `{"old":1}`, warn: WarnExpired}} {
		inner := &seqSource{results: []result{first, {body: `{"n":1}`}}}
		refreshes := 0
		s := NewRecoveringSource(inner, func(context.Context) error { refreshes++; return nil }, clk.Now)
		body, warn, err := s.Fetch(ctx)
		if err != nil || warn != "" || string(body) != `{"n":1}` || inner.Calls() != 2 || refreshes != 1 {
			t.Errorf("first %+v: got %q %q %v, calls %d, refreshes %d", first, body, warn, err, inner.Calls(), refreshes)
		}
	}
}

func TestRecoverGapShortCircuitAndRecheck(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1789126200, 0)}
	inner := &seqSource{results: []result{{err: unauthorized}}}
	refreshes := 0
	fp := "a"
	s := NewRecoveringSource(inner, func(context.Context) error { refreshes++; return errors.New("claude failed") }, clk.Now)
	s.Fingerprint = func(context.Context) (string, error) { return fp, nil }

	fetch := func(step string, wantCalls, wantRefreshes int) {
		t.Helper()
		if _, _, err := s.Fetch(ctx); !errors.Is(err, unauthorized) {
			t.Fatalf("%s: err %v", step, err)
		}
		if inner.Calls() != wantCalls || refreshes != wantRefreshes {
			t.Fatalf("%s: calls %d (want %d), refreshes %d (want %d)", step, inner.Calls(), wantCalls, refreshes, wantRefreshes)
		}
	}
	fetch("401, refresh fails, no retry", 1, 1)
	clk.Add(time.Minute)
	fetch("same credentials: answered from memory", 1, 1)
	fp = "b"
	fetch("credentials changed: real fetch, refresh not due yet", 2, 1)
	clk.Add(10 * time.Minute)
	fetch("gap elapsed: fetch and refresh again", 3, 2)
	s.SetRefresh(nil)
	clk.Add(time.Minute)
	fetch("refresh off, same credentials: from memory", 3, 2)
	clk.Add(15 * time.Minute)
	fetch("recheck elapsed: real fetch", 4, 2)
}

func TestRecoverSuccessClearsTheFailure(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1789126200, 0)}
	inner := &seqSource{results: []result{{err: unauthorized}, {body: `{"n":1}`}, {body: `{"n":2}`}}}
	fp := "a"
	s := NewRecoveringSource(inner, nil, clk.Now)
	s.Fingerprint = func(context.Context) (string, error) { return fp, nil }
	s.Fetch(ctx)
	fp = "b" // Claude Code refreshed on its own
	if body, _, err := s.Fetch(ctx); err != nil || string(body) != `{"n":1}` {
		t.Fatalf("recovery: %q %v", body, err)
	}
	if body, _, err := s.Fetch(ctx); err != nil || string(body) != `{"n":2}` || inner.Calls() != 3 {
		t.Fatalf("after recovery every fetch is real: %q %v calls %d", body, err, inner.Calls())
	}
}

func TestRecoverFingerprintSources(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	if s := NewRecoveringSource(&seqSource{results: []result{{body: "{}"}}}, nil, now); s.Fingerprint != nil || s.fingerprint(ctx) != "" {
		t.Error("a source without credentials has no fingerprint")
	}
	f := &Fetcher{Creds: staticToken{tok: fakeToken}}
	s := NewRecoveringSource(f, nil, now)
	if s.Fingerprint == nil || s.fingerprint(ctx) != "" {
		t.Error("credentials without a fingerprint give an empty one")
	}
	if _, err := f.Fingerprint(ctx); err == nil {
		t.Error("Fetcher.Fingerprint needs fingerprintable credentials")
	}
	creds := (&fakeOS{env: map[string]string{TokenEnv: fakeToken}}).creds("linux")
	f.Creds = creds
	a, err := f.Fingerprint(ctx)
	b, _ := creds.Fingerprint(ctx)
	if err != nil || a == "" || a != b || strings.Contains(a, "FAKE") {
		t.Errorf("fingerprint %q %q %v", a, b, err)
	}
}

func TestCredentialsFingerprint(t *testing.T) {
	file := filepath.Join("home", ".claude", ".credentials.json")
	fp := func(content string) (string, error) {
		return (&fakeOS{home: "home", files: map[string]string{file: content}}).creds("linux").Fingerprint(ctx)
	}
	a, _ := fp(credsJSON("token-one", 1789135200000))
	a2, _ := fp(credsJSON("token-two", 1789135200000))
	b, _ := fp(credsJSON("token-two", 1789164000000))
	if a == "" || a != a2 || a == b || strings.Contains(a+b, "token") {
		t.Errorf("fingerprints follow the expiry, never the token: %q %q %q", a, a2, b)
	}
	if got, err := fp(`{"claudeAiOauth":{"accessToken":"t"}}`); got != "" || err != nil {
		t.Errorf("no expiry: %q %v", got, err)
	}
	if got, _ := (&fakeOS{env: map[string]string{TokenEnv: fakeToken}}).creds("linux").Fingerprint(ctx); got != "env" {
		t.Errorf("PACENOTCH_TOKEN: %q", got)
	}
	if _, err := (&fakeOS{home: "/nowhere"}).creds("linux").Fingerprint(ctx); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("no credentials: %v", err)
	}
}
