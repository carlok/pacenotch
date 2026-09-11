package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carlok/pacenotch/internal/usage"
)

const testNow = 1789126200 // Fri 11 Sep 2026 11:30:00 UTC

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureNames(t testing.TB) []string {
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "fixtures", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	return names
}

// syncBuffer is a goroutine-safe writer that also reports each Write on a channel.
type syncBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	writes chan string
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.writes != nil {
		b.writes <- string(p)
	}
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type fakeSource struct {
	mu    sync.Mutex
	calls int
	body  string
	err   error
}

func (s *fakeSource) Fetch(context.Context) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, "", s.err
	}
	return []byte(s.body), "", nil
}

func (s *fakeSource) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// loaderWith builds real loaders whose API path uses src and a cache in dir.
func loaderWith(dir string, src usage.Source) LoaderFunc {
	return func(from string, stdin io.Reader, ttl time.Duration, now func() time.Time) (*usage.Loader, error) {
		l := &usage.Loader{From: from, Stdin: stdin, ReadFile: os.ReadFile, TTL: ttl, Now: now}
		if from == "" {
			l.Cache = &usage.Cache{Path: filepath.Join(dir, "usage.json"), Now: now}
			l.Source = src
		}
		return l, nil
	}
}

type harness struct {
	deps           Deps
	stdout, stderr *syncBuffer
	env            map[string]string
	src            *fakeSource
	dir            string
}

func newHarness(t *testing.T, args ...string) *harness {
	h := &harness{
		stdout: &syncBuffer{}, stderr: &syncBuffer{},
		env: map[string]string{NowEnv: strconv.Itoa(testNow)},
		src: &fakeSource{body: string(readFixture(t, "api-sample"))},
		dir: t.TempDir(),
	}
	h.deps = Deps{
		Ctx: context.Background(), Args: args, Stdin: strings.NewReader(""),
		Stdout: h.stdout, Stderr: h.stderr,
		Getenv:     func(k string) string { return h.env[k] },
		Loc:        time.UTC,
		IsTerminal: func() bool { return false },
		EnableVT:   func() bool { return true },
		Size:       func() (int, error) { return 0, errors.New("no terminal") },
		Resize:     func(context.Context, func() (int, error)) <-chan struct{} { return nil },
		After:      time.After,
		NewLoader:  loaderWith(h.dir, h.src),
		Version:    "test",
	}
	return h
}

func (h *harness) run() int { return Main(h.deps) }

func TestMainExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		stdin  string
		code   int
		stdout string // substring
		stderr string // exact
	}{
		{"help", []string{"-h"}, "", 0, "vertical pace notch", ""},
		{"version", []string{"--version"}, "", 0, "pacenotch test\n", ""},
		{"bad flag", []string{"--nope"}, "", 1, "", "pacenotch: unknown option: --nope (try --help)\n"},
		{"weekly ahead", []string{"--from", "-", "--width", "90"}, "api-sample", 2, "7d all models  72% used", ""},
		{"weekly on pace", []string{"--from", "-"}, "statusline-epoch", 0, "7d all models", ""},
		{"no windows", []string{"--from", "-"}, "empty", 0, NoWindows, ""},
		{"invalid JSON", []string{"--from", "-"}, `{"five_hour":`, 1, "", "pacenotch: response is not valid JSON\n"},
		{"missing file", []string{"--from", "/nonexistent/x.json"}, "", 1, "", "pacenotch: cannot read /nonexistent/x.json\n"},
		{"raw", []string{"--raw", "--from", "-"}, `{"a":[1,{"b":null}]}`, 0, "{\n  \"a\": [\n    1,\n    {\n      \"b\": null\n    }\n  ]\n}\n", ""},
		{"raw invalid", []string{"--raw", "--from", "-"}, `{"a":`, 1, "", "pacenotch: response is not valid JSON\n"},
		{"raw missing file", []string{"--raw", "--from", "/nonexistent/x.json"}, "", 1, "", "pacenotch: cannot read /nonexistent/x.json\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.args...)
			in := tt.stdin
			if !strings.HasPrefix(in, "{") && in != "" {
				in = string(readFixture(t, in))
			}
			h.deps.Stdin = strings.NewReader(in)
			if code := h.run(); code != tt.code {
				t.Errorf("exit %d, want %d (stderr %q)", code, tt.code, h.stderr.String())
			}
			if !strings.Contains(h.stdout.String(), tt.stdout) || h.stderr.String() != tt.stderr {
				t.Errorf("stdout %q\nstderr %q", h.stdout.String(), h.stderr.String())
			}
		})
	}
}

func TestMainAPIPath(t *testing.T) {
	h := newHarness(t, "--width", "90")
	if code := h.run(); code != 2 || !strings.Contains(h.stdout.String(), "data 0s old") || h.src.Calls() != 1 {
		t.Fatalf("fresh fetch: exit %d, calls %d\n%s", code, h.src.Calls(), h.stdout)
	}

	// cache older than the TTL and a failing endpoint: stale data, still weekly ahead
	h2 := newHarness(t, "--width", "90")
	h2.dir, h2.src = h.dir, &fakeSource{err: &usage.FetchError{Status: 500, Msg: "HTTP 500 from usage endpoint"}}
	h2.deps.NewLoader = loaderWith(h2.dir, h2.src)
	h2.env[NowEnv] = strconv.Itoa(testNow + 3600)
	if code := h2.run(); code != 2 || !strings.Contains(h2.stdout.String(), "STALE 1h 0m old: HTTP 500 from usage endpoint") {
		t.Fatalf("stale: exit %d\n%s", code, h2.stdout)
	}

	// no cache and a failing endpoint: error
	h3 := newHarness(t)
	h3.src.err = errors.New(usage.ErrNoCredentials.Error())
	if code := h3.run(); code != 1 || h3.stderr.String() != "pacenotch: no Claude Code credentials found (run: claude login)\n" {
		t.Fatalf("no data: exit %d, stderr %q", code, h3.stderr)
	}
}

func TestMainSetupErrors(t *testing.T) {
	h := newHarness(t)
	h.env[NowEnv] = "yesterday"
	if code := h.run(); code != 1 || !strings.Contains(h.stderr.String(), "PACENOTCH_NOW must be Unix seconds") {
		t.Errorf("bad now: %d %q", code, h.stderr)
	}
	h = newHarness(t)
	h.deps.NewLoader = func(string, io.Reader, time.Duration, func() time.Time) (*usage.Loader, error) {
		return nil, errors.New("no user cache directory")
	}
	if code := h.run(); code != 1 || h.stderr.String() != "pacenotch: no user cache directory\n" {
		t.Errorf("loader: %d %q", code, h.stderr)
	}
}

func TestColorDecision(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		tty   bool
		vt    bool
		env   map[string]string
		color bool
	}{
		{"auto, terminal", nil, true, true, nil, true},
		{"auto, pipe", nil, false, true, nil, false},
		{"auto, NO_COLOR", nil, true, true, map[string]string{"NO_COLOR": "1"}, false},
		{"auto, dumb terminal", nil, true, true, map[string]string{"TERM": "dumb"}, false},
		{"auto, console without VT", nil, true, false, nil, false},
		{"always, pipe", []string{"--color", "always"}, false, false, map[string]string{"NO_COLOR": "1"}, true},
		{"never, terminal", []string{"--color", "never"}, true, true, nil, false},
		{"no-color, terminal", []string{"--no-color"}, true, true, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, append([]string{"--from", "-"}, tt.args...)...)
			h.deps.Stdin = strings.NewReader(string(readFixture(t, "api-sample")))
			h.deps.IsTerminal = func() bool { return tt.tty }
			h.deps.EnableVT = func() bool { return tt.vt }
			for k, v := range tt.env {
				h.env[k] = v
			}
			h.run()
			if got := strings.Contains(h.stdout.String(), "\033["); got != tt.color {
				t.Errorf("color = %v, want %v", got, tt.color)
			}
		})
	}
}

func TestColumns(t *testing.T) {
	h := newHarness(t)
	opts := Options{Width: -1}
	if got := columns(h.deps, opts); got != 80 {
		t.Errorf("fallback: %d", got)
	}
	h.env["COLUMNS"] = " 70 "
	if got := columns(h.deps, opts); got != 70 {
		t.Errorf("COLUMNS: %d", got)
	}
	h.deps.Size = func() (int, error) { return 123, nil }
	if got := columns(h.deps, opts); got != 123 {
		t.Errorf("terminal: %d", got)
	}
	if got := columns(h.deps, Options{Width: 0}); got != 0 {
		t.Errorf("--width 0: %d", got)
	}
}

func TestCompactSwitch(t *testing.T) {
	for cols, compact := range map[int]bool{59: true, 60: false} {
		h := newHarness(t, "--from", "-", "--width", strconv.Itoa(cols))
		h.deps.Stdin = strings.NewReader(string(readFixture(t, "api-sample")))
		h.run()
		if got := !strings.HasPrefix(h.stdout.String(), " pacenotch "); got != compact {
			t.Errorf("width %d: compact = %v\n%s", cols, got, h.stdout)
		}
	}
}

func TestNowFunc(t *testing.T) {
	now, err := NowFunc(func(string) string { return "" })
	if err != nil || time.Since(now()) > time.Minute {
		t.Errorf("real clock: %v", err)
	}
	now, err = NowFunc(func(string) string { return "1789126200" })
	if err != nil || now().Unix() != testNow {
		t.Errorf("fixed clock: %v %v", now(), err)
	}
	if _, err = NowFunc(func(string) string { return "1.5" }); err == nil {
		t.Error("fractional seconds must be rejected")
	}
}

func TestDefaultDeps(t *testing.T) {
	d := DefaultDeps(context.Background(), []string{"-h"}, "v1")
	if d.Stdout != os.Stdout || d.Getenv == nil || d.NewLoader == nil || d.After == nil || d.Version != "v1" || d.Loc != time.Local {
		t.Fatalf("unexpected defaults: %+v", d)
	}
	d.IsTerminal()
	d.EnableVT()
	if w, err := d.Size(); err == nil && w <= 0 {
		t.Errorf("terminal width %d without error", w)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.Resize(ctx, d.Size)
	cancel()
}

// TestWatch drives watch mode with a controllable ticker and resize channel.
func TestWatch(t *testing.T) {
	h := newHarness(t, "-w", "5")
	h.stdout.writes = make(chan string, 64)
	ticks := make(chan time.Time)
	resized := make(chan struct{}, 1)
	var width atomic.Int64
	width.Store(90)
	var intervals []time.Duration
	var mu sync.Mutex
	h.deps.Size = func() (int, error) { return int(width.Load()), nil }
	h.deps.Resize = func(context.Context, func() (int, error)) <-chan struct{} { return resized }
	h.deps.After = func(d time.Duration) <-chan time.Time {
		mu.Lock()
		intervals = append(intervals, d)
		mu.Unlock()
		return ticks
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.deps.Ctx = ctx
	done := make(chan int)
	go func() { done <- h.run() }()

	next := func() string {
		select {
		case w := <-h.stdout.writes:
			return w
		case <-time.After(5 * time.Second):
			t.Fatal("no frame")
			return ""
		}
	}
	if w := next(); w != hideCursor {
		t.Fatalf("first write %q, want hide cursor", w)
	}
	if w := next(); !strings.HasPrefix(w, clearScreen+" pacenotch   Fri 11 Sep 11:30") || strings.HasSuffix(w, "\n\n") {
		t.Fatalf("frame 1: %q", w)
	}
	width.Store(50)
	resized <- struct{}{}
	if w := next(); !strings.HasPrefix(w, clearScreen+"5h    ") {
		t.Fatalf("frame 2 after resize should be compact: %q", w)
	}
	ticks <- time.Now()
	if w := next(); !strings.HasPrefix(w, clearScreen+"5h    ") {
		t.Fatalf("frame 3 after a tick: %q", w)
	}
	cancel()
	if code := <-done; code != 0 {
		t.Errorf("exit %d", code)
	}
	if w := next(); w != showCursor+"\n" {
		t.Errorf("last write %q, want show cursor", w)
	}
	if h.src.Calls() != 1 {
		t.Errorf("fetched %d times, want once: redraws must not refetch within the TTL", h.src.Calls())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(intervals) == 0 || intervals[0] != 5*time.Second {
		t.Errorf("intervals %v", intervals)
	}
}

func TestWatchShowsErrors(t *testing.T) {
	h := newHarness(t, "-w")
	h.stdout.writes = make(chan string, 64)
	h.src.err = &usage.FetchError{Status: 401, Msg: "HTTP 401: token rejected (start Claude Code once to refresh it)"}
	ctx, cancel := context.WithCancel(context.Background())
	h.deps.Ctx = ctx
	h.deps.After = func(time.Duration) <-chan time.Time { return nil }
	done := make(chan int)
	go func() { done <- h.run() }()
	<-h.stdout.writes
	w := <-h.stdout.writes
	if w != clearScreen+" pacenotch: HTTP 401: token rejected (start Claude Code once to refresh it)\n retrying every 1m0s\n" {
		t.Errorf("error frame %q", w)
	}
	cancel()
	<-done
}

func TestPollResize(t *testing.T) {
	var width atomic.Int64
	width.Store(80)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := pollResize(ctx, func() (int, error) { return int(width.Load()), nil }, time.Millisecond)
	width.Store(100)
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no resize event")
	}
	notify(make(chan struct{})) // never blocks, even with no reader
}
