package usage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type staticToken struct {
	tok  string
	warn string
	err  error
}

func (s staticToken) Token(context.Context) (Token, string, error) {
	return Token{s.tok}, s.warn, s.err
}

func TestFetchStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string // "" = success
		code   int
	}{
		{"200 object", 200, `{"five_hour":{"utilization":1}}`, "", 0},
		{"200 array", 200, `[1]`, "HTTP 200 from usage endpoint", 200},
		{"200 invalid body", 200, `{"five_hour":`, "HTTP 200 from usage endpoint", 200},
		{"401", 401, `{"error":"x"}`, "HTTP 401: token rejected (start Claude Code once to refresh it)", 401},
		{"429", 429, ``, "HTTP 429: usage endpoint is rate limiting, try again later", 429},
		{"500", 500, `oops`, "HTTP 500 from usage endpoint", 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+fakeToken ||
					r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
					w.WriteHeader(418)
					return
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			f := &Fetcher{URL: srv.URL, Client: srv.Client(), Creds: staticToken{tok: fakeToken, warn: "w"}, Timeout: time.Second}
			body, warn, err := f.Fetch(context.Background())
			if warn != "w" {
				t.Errorf("warn = %q", warn)
			}
			if tt.want == "" {
				if err != nil || string(body) != tt.body {
					t.Fatalf("got %q, %v", body, err)
				}
				return
			}
			var fe *FetchError
			if !errors.As(err, &fe) || fe.Error() != tt.want || fe.Status != tt.code || body != nil {
				t.Fatalf("got %q, %v; want %q", body, err, tt.want)
			}
		})
	}
}

func TestFetchTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	f := &Fetcher{URL: srv.URL, Client: srv.Client(), Creds: staticToken{tok: fakeToken}, Timeout: 20 * time.Millisecond}
	_, _, err := f.Fetch(context.Background())
	if err == nil || err.Error() != "network error" {
		t.Fatalf("err = %v, want network error", err)
	}
}

func TestFetchNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	f := &Fetcher{URL: url, Client: http.DefaultClient, Creds: staticToken{tok: fakeToken}, Timeout: time.Second}
	if _, _, err := f.Fetch(context.Background()); err == nil || err.Error() != "network error" {
		t.Fatalf("err = %v, want network error", err)
	}
}

func TestFetchTokenError(t *testing.T) {
	f := &Fetcher{URL: "http://127.0.0.1:1", Client: failDoer{}, Creds: staticToken{err: ErrNoCredentials, warn: "w"}, Timeout: time.Second}
	_, warn, err := f.Fetch(context.Background())
	if !errors.Is(err, ErrNoCredentials) || warn != "w" {
		t.Fatalf("got %q, %v", warn, err)
	}
}

func TestFetchBadURL(t *testing.T) {
	f := &Fetcher{URL: "://bad", Client: failDoer{}, Creds: staticToken{tok: fakeToken}, Timeout: time.Second}
	if _, _, err := f.Fetch(context.Background()); err == nil || err.Error() != "invalid usage endpoint URL" {
		t.Fatalf("err = %v", err)
	}
}

type failDoer struct{}

func (failDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("must not be called")
}

type bodyErrDoer struct{}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func (bodyErrDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(errReader{})}, nil
}

func TestFetchBodyReadError(t *testing.T) {
	f := &Fetcher{URL: "http://example.invalid", Client: bodyErrDoer{}, Creds: staticToken{tok: fakeToken}, Timeout: time.Second}
	if _, _, err := f.Fetch(context.Background()); err == nil || err.Error() != "network error" {
		t.Fatalf("err = %v", err)
	}
}

func TestNewFetcher(t *testing.T) {
	f := NewFetcher(staticToken{})
	if f.URL != Endpoint || f.Client != http.DefaultClient || f.Timeout != 10*time.Second || !strings.HasPrefix(Endpoint, "https://") {
		t.Fatalf("unexpected defaults: %+v", f)
	}
}
