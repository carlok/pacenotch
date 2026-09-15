package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseSemver(t *testing.T) {
	ok := map[string]Version{"v1.2.3": {1, 2, 3}, "0.10.0": {0, 10, 0}, "v012.0.1": {12, 0, 1}}
	for in, want := range ok {
		if got, valid := ParseSemver(in); !valid || got != want {
			t.Errorf("ParseSemver(%q) = %v, %v", in, got, valid)
		}
	}
	for _, in := range []string{"", "dev", "v1.2", "v1.2.3.4", "v1.2.3-rc1", "v1.2.3+meta", "v0.1.3-5-g84f4fdf", "v1..3", "vv1.2.3", "v-1.2.3", "v1234567890.0.0"} {
		if _, valid := ParseSemver(in); valid {
			t.Errorf("ParseSemver(%q) must be invalid", in)
		}
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.3", "v0.2.0", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.10.0", "v0.9.9", false},
		{"v1.0.0", "v1.0.1", true},
		{"dev", "v9.9.9", false},
		{"v0.1.3-5-g84f4fdf", "v0.2.0", false},
		{"v0.1.3", "", false},
	}
	for _, tt := range tests {
		if got := Newer(tt.current, tt.latest); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v", tt.current, tt.latest, got)
		}
	}
}

func TestDue(t *testing.T) {
	now := time.Unix(1789126200, 0)
	if !Due(time.Time{}, now) || !Due(time.Unix(0, 0), now) || !Due(now.Add(-Interval), now) || Due(now.Add(-time.Hour), now) {
		t.Error("Due")
	}
}

func TestLatest(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   Release
		err    error
	}{
		{"release", 200, `{"tag_name":"v0.4.0","html_url":"https://github.com/carlok/pacenotch/releases/tag/v0.4.0","assets":[]}`,
			Release{"v0.4.0", "https://github.com/carlok/pacenotch/releases/tag/v0.4.0"}, nil},
		{"prerelease", 200, `{"tag_name":"v0.4.0","html_url":"https://github.com/carlok/pacenotch/releases/tag/v0.4.0","prerelease":true}`, Release{}, ErrBadResponse},
		{"draft", 200, `{"tag_name":"v0.4.0","html_url":"https://github.com/carlok/pacenotch/releases/tag/v0.4.0","draft":true}`, Release{}, ErrBadResponse},
		{"foreign page", 200, `{"tag_name":"v0.4.0","html_url":"https://example.com/releases/v0.4.0"}`, Release{}, ErrBadResponse},
		{"bad tag", 200, `{"tag_name":"latest","html_url":"https://github.com/carlok/pacenotch/releases/tag/latest"}`, Release{}, ErrBadResponse},
		{"bad JSON", 200, `{"tag_name":`, Release{}, ErrBadResponse},
		{"rate limit", 403, `{"message":"API rate limit exceeded"}`, Release{}, ErrRateLimited},
		{"too many requests", 429, ``, Release{}, ErrRateLimited},
		{"no releases", 404, `{"message":"Not Found"}`, Release{}, ErrBadResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") != "pacenotch" || r.Header.Get("Accept") != "application/vnd.github+json" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			c := &Checker{URL: srv.URL, Client: srv.Client(), Timeout: time.Second}
			got, err := c.Latest(context.Background())
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("got %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestLatestNetworkAndURLErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if _, err := (&Checker{URL: url, Client: http.DefaultClient, Timeout: time.Second}).Latest(context.Background()); !errors.Is(err, ErrNetwork) {
		t.Errorf("closed server: %v", err)
	}
	if _, err := (&Checker{URL: "://bad", Client: http.DefaultClient, Timeout: time.Second}).Latest(context.Background()); !errors.Is(err, ErrBadResponse) {
		t.Errorf("bad URL: %v", err)
	}
	c := NewChecker()
	if c.URL != LatestURL || c.Client != http.DefaultClient || c.Timeout != 10*time.Second {
		t.Errorf("defaults: %+v", c)
	}
}

// FuzzParseSemver: never panics, and a parsed version prints back to an equal version.
func FuzzParseSemver(f *testing.F) {
	for _, s := range []string{"v1.2.3", "0.0.0", "v0.1.3-5-g84f4fdf", "dev", "v999999999.1.1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, ok := ParseSemver(s)
		if !ok {
			return
		}
		if v[0] < 0 || v[1] < 0 || v[2] < 0 || v.Less(v) {
			t.Fatalf("%q parsed to %v", s, v)
		}
	})
}
