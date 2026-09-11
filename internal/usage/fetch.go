package usage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Endpoint is the undocumented OAuth usage endpoint. It rate-limits aggressively.
const Endpoint = "https://api.anthropic.com/api/oauth/usage"

const maxBody = 4 << 20

// Doer is the part of *http.Client the fetcher needs.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Source returns fresh usage data, or an error, plus an optional warning.
type Source interface {
	Fetch(ctx context.Context) ([]byte, string, error)
}

// FetchError is an unsuccessful fetch. Its message never contains the token.
type FetchError struct {
	Status int // HTTP status, 0 for network errors
	Msg    string
}

func (e *FetchError) Error() string { return e.Msg }

// Fetcher calls the usage endpoint with the OAuth token.
type Fetcher struct {
	URL     string
	Client  Doer
	Creds   TokenSource
	Timeout time.Duration
}

// NewFetcher returns a fetcher for the real endpoint with a 10 s timeout.
func NewFetcher(creds TokenSource) *Fetcher {
	return &Fetcher{URL: Endpoint, Client: http.DefaultClient, Creds: creds, Timeout: 10 * time.Second}
}

// Fetch returns the response body when the endpoint answers 200 with a JSON object.
func (f *Fetcher) Fetch(ctx context.Context) ([]byte, string, error) {
	tok, warn, err := f.Creds.Token(ctx)
	if err != nil {
		return nil, warn, err
	}
	ctx, cancel := context.WithTimeout(ctx, f.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, warn, &FetchError{Msg: "invalid usage endpoint URL"}
	}
	req.Header.Set("Authorization", "Bearer "+tok.Reveal())
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "pacenotch")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, warn, &FetchError{Msg: "network error"}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, warn, &FetchError{Msg: "network error"}
	}
	if resp.StatusCode == http.StatusOK && IsObject(body) {
		return body, warn, nil
	}
	return nil, warn, statusError(resp.StatusCode)
}

func statusError(code int) *FetchError {
	switch code {
	case http.StatusUnauthorized:
		return &FetchError{code, "HTTP 401: token rejected (start Claude Code once to refresh it)"}
	case http.StatusTooManyRequests:
		return &FetchError{code, "HTTP 429: usage endpoint is rate limiting, try again later"}
	}
	return &FetchError{code, fmt.Sprintf("HTTP %d from usage endpoint", code)}
}
