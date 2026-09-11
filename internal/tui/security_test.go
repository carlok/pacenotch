package tui

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/carlok/pacenotch/internal/usage"
)

const canary = "fake-oat01-CANARY-3b9f0a2e-must-never-appear"

// TestCLINeverPrintsToken runs the CLI against a hostile server that echoes the
// Authorization header in every error body, through success, every error, --raw, the stale
// banner, watch mode and cache writes. The token must not appear in stdout, stderr or files.
func TestCLINeverPrintsToken(t *testing.T) {
	status := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status == 200 {
			io.WriteString(w, `{"seven_day":{"utilization":72,"resets_at":"2026-09-14T00:59:59Z"}}`)
			return
		}
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"echo":%q}`, r.Header.Get("Authorization"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	runCLI := func(now int64, args ...string) string {
		h := newHarness(t, args...)
		h.env = map[string]string{NowEnv: strconv.FormatInt(now, 10), usage.TokenEnv: canary}
		h.deps.NewLoader = func(from string, stdin io.Reader, ttl time.Duration, clock func() time.Time) (*usage.Loader, error) {
			creds := usage.DefaultCredentials(clock)
			creds.Getenv = h.deps.Getenv
			return &usage.Loader{
				From: from, Stdin: stdin, ReadFile: os.ReadFile, TTL: ttl, Now: clock,
				Cache:  &usage.Cache{Path: filepath.Join(dir, "usage.json"), Now: clock},
				Source: &usage.Fetcher{URL: srv.URL, Client: srv.Client(), Creds: creds, Timeout: 100 * time.Millisecond},
			}, nil
		}
		if len(args) > 0 && args[0] == "-w" {
			ctx, cancel := context.WithCancel(context.Background())
			h.deps.Ctx = ctx
			h.stdout.writes = make(chan string, 8)
			h.deps.After = func(time.Duration) <-chan time.Time { return nil }
			done := make(chan int)
			go func() { done <- h.run() }()
			<-h.stdout.writes
			<-h.stdout.writes
			cancel()
			<-done
		} else {
			h.run()
		}
		return h.stdout.String() + h.stderr.String()
	}

	var outputs []string
	for _, s := range []int{200, 401, 429, 500, 403} {
		status = s
		os.Remove(filepath.Join(dir, "usage.json"))
		for step, args := range [][]string{{"--width", "90", "--color", "always"}, {"--raw"}, {"-c"}, {"-w", "1"}} {
			outputs = append(outputs, runCLI(testNow+int64(step)*3600, args...))
		}
		status = 200
		runCLI(testNow) // seed the cache, then let it go stale
		status = s
		outputs = append(outputs, runCLI(testNow+7200, "--width", "120"), runCLI(testNow+7200, "-w", "--width", "50"))
	}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			outputs = append(outputs, string(b))
		}
		return nil
	})
	sawStale := false
	for _, out := range outputs {
		if strings.Contains(out, "CANARY") {
			t.Fatalf("token leaked:\n%s", out)
		}
		sawStale = sawStale || strings.Contains(out, "STALE")
	}
	if !sawStale {
		t.Error("the stale banner path was not exercised")
	}
}
