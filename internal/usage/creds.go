package usage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Token is an OAuth access token. It formats as a placeholder, so it cannot leak through
// fmt, logs or JSON by accident. Reveal is only for building the Authorization header.
type Token struct{ s string }

const redacted = "[redacted]"

func (t Token) Reveal() string             { return t.s }
func (t Token) Empty() bool                { return t.s == "" }
func (Token) String() string               { return redacted }
func (Token) GoString() string             { return redacted }
func (Token) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }
func (Token) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// Errors and warnings from the credential lookup. None of them contains credential data.
var (
	ErrNoCredentials = errors.New("no Claude Code credentials found (run: claude login)")
	ErrNoAccessToken = errors.New("credentials found but no access token")
)

// WarnExpired is returned as a warning when claudeAiOauth.expiresAt is in the past.
const WarnExpired = "OAuth token looks expired: start Claude Code once to refresh it"

// TokenEnv overrides every other token source.
const TokenEnv = "PACENOTCH_TOKEN"

// Runner runs an external command and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// TokenSource yields the OAuth token and an optional warning.
type TokenSource interface {
	Token(ctx context.Context) (Token, string, error)
}

// Credentials looks the token up read-only: it never refreshes or writes credentials.
// Every side effect is a field so tests can replace it.
type Credentials struct {
	GOOS     string
	Getenv   func(string) string
	HomeDir  func() (string, error)
	ReadFile func(string) ([]byte, error)
	Run      Runner
	Now      func() time.Time
}

// DefaultCredentials uses the real environment, filesystem and `security` command.
func DefaultCredentials(now func() time.Time) *Credentials {
	return &Credentials{
		GOOS:     runtime.GOOS,
		Getenv:   os.Getenv,
		HomeDir:  os.UserHomeDir,
		ReadFile: os.ReadFile,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		Now: now,
	}
}

// Token returns the token from PACENOTCH_TOKEN, then from the macOS keychain (darwin
// only), then from ~/.claude/.credentials.json (%USERPROFILE%\.claude on Windows).
func (c *Credentials) Token(ctx context.Context) (Token, string, error) {
	if t := c.Getenv(TokenEnv); t != "" {
		return Token{t}, "", nil
	}
	var creds []byte
	if c.GOOS == "darwin" {
		out, err := c.Run(ctx, "security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
		if err == nil {
			creds = bytes.TrimSpace(out)
		}
	}
	if len(creds) == 0 {
		if home, err := c.HomeDir(); err == nil {
			if b, err := c.ReadFile(filepath.Join(home, ".claude", ".credentials.json")); err == nil {
				creds = bytes.TrimSpace(b)
			}
		}
	}
	if len(creds) == 0 {
		return Token{}, "", ErrNoCredentials
	}
	var doc struct {
		OAuth struct {
			AccessToken json.RawMessage `json:"accessToken"`
			ExpiresAt   json.RawMessage `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(creds, &doc) != nil {
		return Token{}, "", ErrNoAccessToken
	}
	var tok string
	if json.Unmarshal(doc.OAuth.AccessToken, &tok) != nil || tok == "" {
		return Token{}, "", ErrNoAccessToken
	}
	warn := ""
	if ms, ok := number(doc.OAuth.ExpiresAt); ok {
		if exp := math.Floor(ms / 1000); exp > 0 && exp < float64(c.Now().Unix()) {
			warn = WarnExpired
		}
	}
	return Token{tok}, warn, nil
}

// Fingerprint changes whenever the stored token changes, without revealing it: the first
// 8 bytes of its SHA-256, hex encoded. It lets a watcher notice that Claude Code refreshed
// the token without calling the endpoint.
func (c *Credentials) Fingerprint(ctx context.Context) (string, error) {
	tok, _, err := c.Token(ctx)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(tok.s))
	return hex.EncodeToString(sum[:8]), nil
}
