package usage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fakeSource returns results[i] on call i (the last one repeats); nil error = success.
type fakeSource struct {
	mu      sync.Mutex
	calls   int
	body    string
	warn    string
	results []error
}

func (s *fakeSource) Fetch(context.Context) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if len(s.results) > 0 {
		err = s.results[min(s.calls, len(s.results)-1)]
	}
	s.calls++
	if err != nil {
		return nil, s.warn, err
	}
	return []byte(s.body), s.warn, nil
}

func (s *fakeSource) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func newTestLoader(t *testing.T, src Source) (*Loader, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1789126200, 0)}
	return &Loader{
		Cache:  &Cache{Path: filepath.Join(t.TempDir(), "pacenotch", "usage.json"), Now: clk.Now},
		Source: src,
		TTL:    60 * time.Second,
		Now:    clk.Now,
	}, clk
}

var ctx = context.Background()

func TestLoaderFrom(t *testing.T) {
	file := filepath.Join(t.TempDir(), "in.json")
	os.WriteFile(file, []byte(`{"x":1}`), 0o600)
	l := &Loader{From: file, ReadFile: os.ReadFile}
	res, err := l.Load(ctx, false)
	if err != nil || string(res.Data) != `{"x":1}` || res.Age != -1 || res.From != file {
		t.Fatalf("got %+v, %v", res, err)
	}
	l = &Loader{From: "-", Stdin: strings.NewReader(`{"y":2}`)}
	if res, err = l.Load(ctx, false); err != nil || string(res.Data) != `{"y":2}` || res.From != "-" {
		t.Fatalf("stdin: %+v, %v", res, err)
	}
	l = &Loader{From: filepath.Join(t.TempDir(), "missing.json"), ReadFile: os.ReadFile}
	if _, err = l.Load(ctx, false); err == nil || !strings.HasPrefix(err.Error(), "cannot read ") {
		t.Fatalf("missing file: %v", err)
	}
}

func TestLoaderCacheAndTTL(t *testing.T) {
	src := &fakeSource{body: `{"n":1}`, warn: "expired"}
	l, clk := newTestLoader(t, src)

	res, err := l.Load(ctx, false) // no cache: fetch
	if err != nil || res.Age != 0 || string(res.Data) != `{"n":1}` || res.Warn != "expired" || src.Calls() != 1 {
		t.Fatalf("first load: %+v, %v, calls %d", res, err, src.Calls())
	}
	clk.Add(59 * time.Second) // within TTL: cached, no warning
	src.body = `{"n":2}`
	if res, err = l.Load(ctx, false); err != nil || res.Age != 59 || string(res.Data) != `{"n":1}` || res.Warn != "" || src.Calls() != 1 {
		t.Fatalf("cached load: %+v, %v, calls %d", res, err, src.Calls())
	}
	if res, _ = l.Load(ctx, true); string(res.Data) != `{"n":2}` || src.Calls() != 2 { // force
		t.Fatalf("forced load: %+v, calls %d", res, src.Calls())
	}
	clk.Add(60 * time.Second) // age == TTL: refetch
	src.body = `{"n":3}`
	if res, _ = l.Load(ctx, false); string(res.Data) != `{"n":3}` || res.Age != 0 || src.Calls() != 3 {
		t.Fatalf("expired load: %+v, calls %d", res, src.Calls())
	}
	b, mtime, _ := l.Cache.Read()
	if string(b) != `{"n":3}` || !mtime.Equal(clk.Now()) {
		t.Fatalf("cache %q at %v", b, mtime)
	}
}

func TestLoaderStaleAndErrors(t *testing.T) {
	src := &fakeSource{body: `{"n":1}`, results: []error{nil, &FetchError{500, "HTTP 500 from usage endpoint"}}}
	l, clk := newTestLoader(t, src)
	l.Load(ctx, false)
	clk.Add(10 * time.Minute)
	res, err := l.Load(ctx, false)
	if err != nil || !res.Stale || res.Age != 600 || res.Err != "HTTP 500 from usage endpoint" || string(res.Data) != `{"n":1}` {
		t.Fatalf("stale: %+v, %v", res, err)
	}

	src = &fakeSource{warn: "expired", results: []error{ErrNoCredentials}}
	l, _ = newTestLoader(t, src)
	res, err = l.Load(ctx, false)
	if err == nil || err.Error() != ErrNoCredentials.Error() || res.Warn != "expired" {
		t.Fatalf("no cache: %+v, %v", res, err)
	}
}

func TestLoaderUnwritableCacheStillReturnsData(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, []byte("x"), 0o600)
	src := &fakeSource{body: `{"n":1}`}
	l, _ := newTestLoader(t, src)
	l.Cache.Path = filepath.Join(file, "usage.json")
	if res, err := l.Load(ctx, false); err != nil || string(res.Data) != `{"n":1}` {
		t.Fatalf("got %+v, %v", res, err)
	}
}

func TestLoaderBackoff(t *testing.T) {
	rate := &FetchError{429, "HTTP 429: usage endpoint is rate limiting, try again later"}
	src := &fakeSource{body: `{"n":1}`, results: []error{nil, rate, rate, rate, rate, rate, nil, rate}}
	l, clk := newTestLoader(t, src)
	l.Backoff = true
	l.Load(ctx, false) // cache the first answer
	if !l.BackoffUntil().IsZero() {
		t.Fatal("no backoff expected yet")
	}

	for i, step := range []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 30 * time.Minute} {
		clk.Add(time.Minute) // TTL expired, still inside the previous backoff (if any)
		before := src.Calls()
		res, err := l.Load(ctx, false)
		if i > 0 {
			// inside the previous backoff: no request, stale with the last error
			if src.Calls() != before || err != nil || !res.Stale || !strings.Contains(res.Err, "HTTP 429") {
				t.Fatalf("step %d: request during backoff (%d calls) or %+v, %v", i, src.Calls(), res, err)
			}
			clk.Add(l.BackoffUntil().Sub(clk.Now())) // wait until the backoff ends
			res, err = l.Load(ctx, false)
		}
		if src.Calls() != before+1 || err != nil || !res.Stale {
			t.Fatalf("step %d: %+v, %v, calls %d", i, res, err, src.Calls())
		}
		if want := "(backing off " + step.String() + ")"; !strings.HasSuffix(res.Err, want) {
			t.Fatalf("step %d: err %q, want suffix %q", i, res.Err, want)
		}
		if got := l.BackoffUntil().Sub(clk.Now()); got != step {
			t.Fatalf("step %d: backoff %v, want %v", i, got, step)
		}
	}
	clk.Add(30 * time.Minute)
	if res, err := l.Load(ctx, false); err != nil || res.Stale || !l.BackoffUntil().IsZero() {
		t.Fatalf("success must reset the backoff: %+v, %v", res, err)
	}
	clk.Add(time.Minute)
	if res, _ := l.Load(ctx, false); l.BackoffUntil().Sub(clk.Now()) != 5*time.Minute || !res.Stale {
		t.Fatalf("backoff must restart at 5m: %+v", res)
	}
}

func TestLoaderNoBackoffInSingleShot(t *testing.T) {
	rate := &FetchError{429, "HTTP 429: usage endpoint is rate limiting, try again later"}
	src := &fakeSource{results: []error{rate}}
	l, _ := newTestLoader(t, src)
	_, err := l.Load(ctx, false)
	if err == nil || err.Error() != rate.Msg || !l.BackoffUntil().IsZero() {
		t.Fatalf("got %v, backoff until %v", err, l.BackoffUntil())
	}
	var fe *FetchError
	if errors.As(err, &fe) {
		t.Error("the returned error is a plain message")
	}
}

func TestLoaderConcurrent(t *testing.T) {
	src := &fakeSource{body: `{"n":1}`}
	l, clk := newTestLoader(t, src)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				clk.Add(time.Second)
				if _, err := l.Load(ctx, j%5 == 0); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestNewLoader(t *testing.T) {
	l, err := NewLoader("-", strings.NewReader("{}"), time.Minute, time.Now)
	if err != nil || l.Cache != nil || l.Source != nil {
		t.Fatalf("from: %+v, %v", l, err)
	}
	if _, err := DefaultCachePath(); err != nil {
		t.Skip("no cache dir on this machine")
	}
	l, err = NewLoader("", nil, time.Minute, time.Now)
	if err != nil || l.Cache == nil || l.Source == nil || l.TTL != time.Minute {
		t.Fatalf("api: %+v, %v", l, err)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("LocalAppData", "")
	if _, err := NewLoader("", nil, time.Minute, time.Now); err == nil {
		t.Error("expected an error without a cache directory")
	}
}
