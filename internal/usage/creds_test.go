package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeToken = "fake-oat01-FAKE-token-for-tests"

var credNow = time.Unix(1789126200, 0)

func credsJSON(token string, expiresAtMs any) string {
	b, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{"accessToken": token, "expiresAt": expiresAtMs}})
	return string(b)
}

type fakeOS struct {
	env      map[string]string
	security string
	secErr   error
	files    map[string]string
	home     string
	homeErr  error
	ran      []string
}

func (f *fakeOS) creds(goos string) *Credentials {
	return &Credentials{
		GOOS:   goos,
		Getenv: func(k string) string { return f.env[k] },
		HomeDir: func() (string, error) {
			return f.home, f.homeErr
		},
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := f.files[p]; ok {
				return []byte(s), nil
			}
			return nil, fs.ErrNotExist
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			f.ran = append(f.ran, name+" "+strings.Join(args, " "))
			return []byte(f.security), f.secErr
		},
		Now: func() time.Time { return credNow },
	}
}

func TestCredentialsLookupOrder(t *testing.T) {
	home := filepath.Join("home", "u")
	file := filepath.Join(home, ".claude", ".credentials.json")
	future := credNow.Add(time.Hour).UnixMilli()
	past := credNow.Add(-time.Second).UnixMilli()

	tests := []struct {
		name     string
		goos     string
		os       fakeOS
		wantTok  string
		wantWarn string
		wantErr  error
		wantRuns int
	}{
		{"env wins", "darwin", fakeOS{env: map[string]string{TokenEnv: "from-env"}, security: credsJSON("kc", future)}, "from-env", "", nil, 0},
		{"macOS keychain", "darwin", fakeOS{security: credsJSON("kc", future) + "\n", home: home, files: map[string]string{file: credsJSON("file", future)}}, "kc", "", nil, 1},
		{"macOS keychain fails, file fallback", "darwin", fakeOS{secErr: errors.New("exit 44"), home: home, files: map[string]string{file: credsJSON("file", future)}}, "file", "", nil, 1},
		{"macOS keychain empty, file fallback", "darwin", fakeOS{security: " \n", home: home, files: map[string]string{file: credsJSON("file", future)}}, "file", "", nil, 1},
		{"linux file, no security call", "linux", fakeOS{security: credsJSON("kc", future), home: home, files: map[string]string{file: credsJSON("file", future)}}, "file", "", nil, 0},
		{"windows file under USERPROFILE", "windows", fakeOS{home: home, files: map[string]string{file: credsJSON("win", future)}}, "win", "", nil, 0},
		{"expired token warns", "linux", fakeOS{home: home, files: map[string]string{file: credsJSON("old", past)}}, "old", WarnExpired, nil, 0},
		{"expiresAt 0 does not warn", "linux", fakeOS{home: home, files: map[string]string{file: credsJSON("t", 0)}}, "t", "", nil, 0},
		{"expiresAt string is ignored", "linux", fakeOS{home: home, files: map[string]string{file: credsJSON("t", "soon")}}, "t", "", nil, 0},
		{"expiresAt missing", "linux", fakeOS{home: home, files: map[string]string{file: `{"claudeAiOauth":{"accessToken":"t"}}`}}, "t", "", nil, 0},
		{"no file", "linux", fakeOS{home: home}, "", "", ErrNoCredentials, 0},
		{"no home", "windows", fakeOS{homeErr: errors.New("no home")}, "", "", ErrNoCredentials, 0},
		{"keychain and file missing", "darwin", fakeOS{secErr: errors.New("exit 44"), home: home}, "", "", ErrNoCredentials, 1},
		{"invalid JSON", "linux", fakeOS{home: home, files: map[string]string{file: "{nope"}}, "", "", ErrNoAccessToken, 0},
		{"no access token", "linux", fakeOS{home: home, files: map[string]string{file: `{"claudeAiOauth":{}}`}}, "", "", ErrNoAccessToken, 0},
		{"empty access token", "linux", fakeOS{home: home, files: map[string]string{file: credsJSON("", future)}}, "", "", ErrNoAccessToken, 0},
		{"numeric access token", "linux", fakeOS{home: home, files: map[string]string{file: `{"claudeAiOauth":{"accessToken":42}}`}}, "", "", ErrNoAccessToken, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.os
			tok, warn, err := f.creds(tt.goos).Token(context.Background())
			if tok.Reveal() != tt.wantTok || warn != tt.wantWarn || !errors.Is(err, tt.wantErr) {
				t.Errorf("got %q, %q, %v; want %q, %q, %v", tok.Reveal(), warn, err, tt.wantTok, tt.wantWarn, tt.wantErr)
			}
			if tok.Empty() != (tt.wantTok == "") {
				t.Errorf("Empty() = %v", tok.Empty())
			}
			if len(f.ran) != tt.wantRuns {
				t.Errorf("ran %v, want %d runs", f.ran, tt.wantRuns)
			}
			for _, r := range f.ran {
				if r != `security find-generic-password -s Claude Code-credentials -w` {
					t.Errorf("unexpected command %q", r)
				}
			}
		})
	}
}

func TestTokenIsRedacted(t *testing.T) {
	tok := Token{fakeToken}
	holder := struct{ T Token }{tok}
	js, _ := json.Marshal(holder)
	txt, _ := tok.MarshalText()
	for _, s := range []string{
		fmt.Sprint(tok), fmt.Sprintf("%v %+v %s %q %#v", tok, tok, tok, tok, tok),
		fmt.Sprintf("%v %+v %#v", holder, holder, holder), string(js), string(txt),
	} {
		if strings.Contains(s, "FAKE") {
			t.Errorf("token leaked in %q", s)
		}
	}
	if tok.Reveal() != fakeToken {
		t.Error("Reveal must return the token")
	}
}

func TestDefaultCredentials(t *testing.T) {
	c := DefaultCredentials(time.Now)
	if c.GOOS == "" || c.Getenv == nil || c.HomeDir == nil || c.ReadFile == nil || c.Now == nil {
		t.Fatal("DefaultCredentials left a field empty")
	}
	if _, err := c.Run(context.Background(), "pacenotch-command-that-does-not-exist"); err == nil {
		t.Error("running a missing command must fail")
	}
}
