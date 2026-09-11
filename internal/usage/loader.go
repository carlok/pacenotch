package usage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Result is what a renderer needs besides the rows.
type Result struct {
	Data  []byte
	Age   int64  // seconds since the data was fetched; -1 when read with From
	From  string // the From argument when Age is -1
	Stale bool   // the fetch failed and Data is the cached copy
	Err   string // why the data is stale
	Warn  string // e.g. an expired token
}

// Backoff limits after HTTP 429 in watch and GUI modes.
const (
	BackoffMin = 5 * time.Minute
	BackoffMax = 30 * time.Minute
)

// Loader decides between --from, the cache and a fetch, like the reference's load_data.
// It is safe for concurrent use; concurrent loads are serialized.
type Loader struct {
	From     string
	Stdin    io.Reader
	ReadFile func(string) ([]byte, error)
	Cache    *Cache
	Source   Source
	TTL      time.Duration
	Now      func() time.Time
	Backoff  bool // honour 429 backoff (watch and GUI modes)

	mu           sync.Mutex
	backoffUntil time.Time
	backoffStep  time.Duration
	lastErr      string
}

// Load returns fresh or cached data. force fetches even when the cache is within the TTL
// (it still respects an active 429 backoff). An error means there is nothing to show.
func (l *Loader) Load(ctx context.Context, force bool) (Result, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.From != "" {
		var b []byte
		var err error
		if l.From == "-" {
			b, err = io.ReadAll(l.Stdin)
		} else {
			b, err = l.ReadFile(l.From)
		}
		if err != nil {
			return Result{}, fmt.Errorf("cannot read %s", l.From)
		}
		return Result{Data: b, Age: -1, From: l.From}, nil
	}

	now := l.Now()
	data, mtime, cerr := l.Cache.Read()
	have := cerr == nil
	age := now.Unix() - mtime.Unix()
	if !have {
		age = now.Unix()
	}
	if have && age < int64(l.TTL/time.Second) && !force {
		return Result{Data: data, Age: age}, nil
	}

	var body []byte
	var warn string
	var err error
	if l.Backoff && now.Before(l.backoffUntil) {
		err = errors.New(l.lastErr)
	} else {
		body, warn, err = l.Source.Fetch(ctx)
	}
	if err == nil {
		l.backoffStep, l.backoffUntil = 0, time.Time{}
		_ = l.Cache.Write(body) // an unwritable cache must not hide fresh data
		return Result{Data: body, Age: 0, Warn: warn}, nil
	}

	msg := err.Error()
	var fe *FetchError
	if l.Backoff && errors.As(err, &fe) && fe.Status == 429 {
		l.backoffStep = min(max(2*l.backoffStep, BackoffMin), BackoffMax)
		l.backoffUntil = now.Add(l.backoffStep)
		msg = fmt.Sprintf("%s (backing off %s)", msg, l.backoffStep)
	}
	l.lastErr = msg
	if !have {
		return Result{Warn: warn}, errors.New(msg)
	}
	return Result{Data: data, Age: age, Stale: true, Err: msg, Warn: warn}, nil
}

// BackoffUntil reports when the current 429 backoff ends (zero when none).
func (l *Loader) BackoffUntil() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.backoffUntil
}

// NewLoader wires the real cache, credentials and endpoint.
func NewLoader(from string, stdin io.Reader, ttl time.Duration, now func() time.Time) (*Loader, error) {
	l := &Loader{From: from, Stdin: stdin, ReadFile: os.ReadFile, TTL: ttl, Now: now}
	if from != "" {
		return l, nil
	}
	path, err := DefaultCachePath()
	if err != nil {
		return nil, fmt.Errorf("no user cache directory: %v", err)
	}
	l.Cache = &Cache{Path: path, Now: now}
	l.Source = NewFetcher(DefaultCredentials(now))
	return l, nil
}
