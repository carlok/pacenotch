package usage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// canary is a recognizable fake token. It must never show up anywhere.
const canary = "fake-oat01-CANARY-7c1e5b0d-must-never-appear"

// TestTokenNeverLeaks drives every fetch outcome through the loader, including the stale
// path and cache writes. A hostile server echoes the Authorization header back in every
// error response. The token must not appear in errors, results, logs or cache files.
func TestTokenNeverLeaks(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"200", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"five_hour":{"utilization":1}}`) }},
		{"200 not an object", echo(200)},
		{"401", echo(401)},
		{"429", echo(429)},
		{"500", echo(500)},
		{"timeout", func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			dir := t.TempDir()
			env := map[string]string{TokenEnv: canary}
			creds := &Credentials{GOOS: "darwin", Getenv: func(k string) string { return env[k] }, Now: time.Now}
			fetcher := &Fetcher{URL: srv.URL, Client: srv.Client(), Creds: creds, Timeout: 50 * time.Millisecond}
			clk := &fakeClock{t: time.Unix(1789126200, 0)}
			l := &Loader{Cache: &Cache{Path: filepath.Join(dir, "usage.json"), Now: clk.Now}, Source: fetcher, TTL: time.Minute, Now: clk.Now, Backoff: true}

			var seen []string
			record := func(res Result, err error) {
				seen = append(seen, fmt.Sprintf("%+v %#v %v %s", res, res, err, res.Data))
				if err != nil {
					seen = append(seen, err.Error(), fmt.Sprintf("%#v", err))
				}
			}
			// empty cache, then stale over a seeded cache, then a forced reload
			record(l.Load(context.Background(), false))
			l.Cache.Write([]byte(`{"seven_day":{"utilization":2}}`))
			clk.Add(time.Hour)
			record(l.Load(context.Background(), false))
			record(l.Load(context.Background(), true))
			_, _, err := fetcher.Fetch(context.Background())
			seen = append(seen, fmt.Sprint(err), fmt.Sprintf("%#v %+v", fetcher, creds))

			filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					b, _ := os.ReadFile(p)
					seen = append(seen, string(b))
				}
				return nil
			})
			seen = append(seen, logs.String())
			for _, s := range seen {
				if strings.Contains(s, "CANARY") {
					t.Fatalf("token leaked: %q", s)
				}
			}
		})
	}
}

// TestRecoveringSourceNeverLeaks: a refresh whose error text contains the token must not
// surface it, and neither may the credential fingerprint.
func TestRecoveringSourceNeverLeaks(t *testing.T) {
	srv := httptest.NewServer(echo(401))
	defer srv.Close()
	env := map[string]string{TokenEnv: canary}
	creds := &Credentials{GOOS: "linux", Getenv: func(k string) string { return env[k] }, Now: time.Now}
	f := &Fetcher{URL: srv.URL, Client: srv.Client(), Creds: creds, Timeout: time.Second}
	clk := &fakeClock{t: time.Unix(1789126200, 0)}
	s := NewRecoveringSource(f, func(context.Context) error { return errors.New("claude said " + canary) }, clk.Now)
	var seen []string
	for i := 0; i < 3; i++ {
		body, warn, err := s.Fetch(context.Background())
		seen = append(seen, string(body), warn, fmt.Sprint(err), fmt.Sprintf("%#v", err))
		clk.Add(11 * time.Minute)
	}
	fp, _ := f.Fingerprint(context.Background())
	seen = append(seen, fp)
	for _, x := range seen {
		if strings.Contains(x, "CANARY") {
			t.Fatalf("token leaked: %q", x)
		}
	}
}

func echo(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprintf(w, `[%q]`, r.Header.Get("Authorization"))
	}
}
