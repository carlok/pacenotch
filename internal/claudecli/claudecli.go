// Package claudecli finds Claude Code's command-line tool and uses it to get an expired
// OAuth token refreshed. pacenotch never writes credentials itself: Claude Code refreshes
// its own token when it makes a request, so one tiny headless request is enough.
package claudecli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EnvPath overrides where the claude binary is.
const EnvPath = "PACENOTCH_CLAUDE"

// Args is the request that makes Claude Code refresh its token: one short prompt on the
// smallest model, without tools, user settings or MCP servers, not saved as a session.
var Args = []string{"-p", "Reply with OK.", "--model", "haiku", "--no-session-persistence", "--restricted", "--strict-mcp-config"}

// Errors are fixed strings: the command's output can contain account details.
var (
	ErrNotFound = errors.New("claude (Claude Code CLI) not found")
	ErrFailed   = errors.New("claude could not refresh the token")
)

// Env is where the binary is looked up. Every field can be replaced in tests.
type Env struct {
	GOOS     string
	Getenv   func(string) string
	HomeDir  func() (string, error)
	Glob     func(string) ([]string, error)
	Stat     func(string) (fs.FileInfo, error)
	LookPath func(string) (string, error)
}

// DefaultEnv uses the real environment and filesystem.
func DefaultEnv() Env {
	return Env{GOOS: runtime.GOOS, Getenv: os.Getenv, HomeDir: os.UserHomeDir, Glob: filepath.Glob, Stat: os.Stat, LookPath: exec.LookPath}
}

// Candidates lists possible locations, most preferred first. Apps started from Finder or at
// login get a minimal PATH, so the usual install locations are listed explicitly.
func Candidates(e Env) []string {
	var out []string
	add := func(p string) {
		if p != "" {
			out = append(out, p)
		}
	}
	add(e.Getenv(EnvPath))
	if p, err := e.LookPath("claude"); err == nil {
		add(p)
	}
	home, herr := e.HomeDir()
	if e.GOOS == "windows" {
		if appData := e.Getenv("APPDATA"); appData != "" {
			add(filepath.Join(appData, "npm", "claude.cmd"))
		}
		if herr == nil {
			add(filepath.Join(home, ".local", "bin", "claude.exe"))
		}
		return out
	}
	if herr == nil {
		add(filepath.Join(home, ".local", "bin", "claude"))
		add(filepath.Join(home, ".claude", "local", "claude"))
	}
	add("/opt/homebrew/bin/claude")
	add("/usr/local/bin/claude")
	if prefix := e.Getenv("npm_config_prefix"); prefix != "" {
		add(filepath.Join(prefix, "bin", "claude"))
	}
	if herr == nil {
		matches, _ := e.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "claude"))
		sort.SliceStable(matches, func(i, j int) bool { return newer(nodeVersion(matches[i]), nodeVersion(matches[j])) })
		out = append(out, matches...)
	}
	return out
}

// nodeVersion reads v22.22.0 from .../node/v22.22.0/bin/claude.
func nodeVersion(p string) []int {
	var v []int
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Base(filepath.Dir(filepath.Dir(p))), "v"), ".") {
		n, _ := strconv.Atoi(part)
		v = append(v, n)
	}
	return v
}

func newer(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return len(a) > len(b)
}

// Locate returns the first candidate that is an executable file.
func Locate(e Env) (string, error) {
	for _, p := range Candidates(e) {
		st, err := e.Stat(p)
		if err != nil || st.IsDir() || (e.GOOS != "windows" && st.Mode().Perm()&0o111 == 0) {
			continue
		}
		return p, nil
	}
	return "", ErrNotFound
}

// Exec runs path with args in dir with the given environment, discarding all output.
type Exec func(ctx context.Context, dir, path string, args, env []string) error

// Refresher asks Claude Code to refresh an expired token.
type Refresher struct {
	Env     Env
	Exec    Exec
	TempDir func() (dir string, cleanup func(), err error)
	Environ func() []string
	Timeout time.Duration
}

// NewRefresher uses the real environment, a temporary directory and a two-minute timeout.
func NewRefresher() *Refresher {
	return &Refresher{Env: DefaultEnv(), Exec: Run, TempDir: tempDir, Environ: os.Environ, Timeout: 2 * time.Minute}
}

func tempDir() (string, func(), error) {
	dir, err := os.MkdirTemp("", "pacenotch-claude-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

// Refresh runs the request in an empty directory, so no project instructions are loaded,
// with the binary's directory first in PATH: an npm install needs node next to it.
func (r *Refresher) Refresh(ctx context.Context) error {
	path, err := Locate(r.Env)
	if err != nil {
		return err
	}
	dir, cleanup, err := r.TempDir()
	if err != nil {
		return ErrFailed
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	if r.Exec(ctx, dir, path, Args, WithPath(r.Environ(), filepath.Dir(path), r.Env.GOOS)) != nil {
		return ErrFailed
	}
	return nil
}

// WithPath returns env with dir first in PATH (matched case-insensitively on Windows).
func WithPath(env []string, dir, goos string) []string {
	sep := ":"
	if goos == "windows" {
		sep = ";"
	}
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if k == "PATH" || (goos == "windows" && strings.EqualFold(k, "PATH")) {
			if !found {
				out = append(out, k+"="+dir+sep+v)
				found = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, "PATH="+dir)
	}
	return out
}
