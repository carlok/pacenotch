package claudecli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type info struct {
	dir  bool
	mode fs.FileMode
}

func (i info) Name() string       { return "claude" }
func (i info) Size() int64        { return 1 }
func (i info) Mode() fs.FileMode  { return i.mode }
func (i info) ModTime() time.Time { return time.Time{} }
func (i info) IsDir() bool        { return i.dir }
func (i info) Sys() any           { return nil }

type fakeEnv struct {
	goos    string
	vars    map[string]string
	home    string
	look    string
	globs   []string
	files   map[string]info
	homeErr bool
}

func (f fakeEnv) env() Env {
	return Env{
		GOOS:   f.goos,
		Getenv: func(k string) string { return f.vars[k] },
		HomeDir: func() (string, error) {
			if f.homeErr {
				return "", errors.New("no home")
			}
			return f.home, nil
		},
		Glob: func(string) ([]string, error) { return append([]string(nil), f.globs...), nil },
		Stat: func(p string) (fs.FileInfo, error) {
			if i, ok := f.files[p]; ok {
				return i, nil
			}
			return nil, fs.ErrNotExist
		},
		LookPath: func(string) (string, error) {
			if f.look == "" {
				return "", errors.New("not in PATH")
			}
			return f.look, nil
		},
	}
}

func nvm(version string) string {
	return filepath.Join("/Users/u", ".nvm", "versions", "node", version, "bin", "claude")
}

func TestCandidates(t *testing.T) {
	darwin := fakeEnv{
		goos: "darwin", home: "/Users/u", look: "/in/path/claude",
		vars:  map[string]string{EnvPath: "/custom/claude", "npm_config_prefix": "/npm"},
		globs: []string{nvm("v18.1.0"), nvm("v22.22.0"), nvm("v22.3.1"), nvm("v22.22")},
	}
	want := []string{
		"/custom/claude", "/in/path/claude",
		filepath.Join("/Users/u", ".local", "bin", "claude"), filepath.Join("/Users/u", ".claude", "local", "claude"),
		"/opt/homebrew/bin/claude", "/usr/local/bin/claude", filepath.Join("/npm", "bin", "claude"),
		nvm("v22.22.0"), nvm("v22.22"), nvm("v22.3.1"), nvm("v18.1.0"),
	}
	if got := Candidates(darwin.env()); !reflect.DeepEqual(got, want) {
		t.Errorf("darwin\n got %q\nwant %q", got, want)
	}

	bare := fakeEnv{goos: "linux", homeErr: true}
	if got := Candidates(bare.env()); !reflect.DeepEqual(got, []string{"/opt/homebrew/bin/claude", "/usr/local/bin/claude"}) {
		t.Errorf("no home, nothing in PATH: %q", got)
	}

	windows := fakeEnv{goos: "windows", home: `C:\Users\u`, look: `C:\bin\claude.exe`, vars: map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}}
	want = []string{`C:\bin\claude.exe`, filepath.Join(`C:\Users\u\AppData\Roaming`, "npm", "claude.cmd"), filepath.Join(`C:\Users\u`, ".local", "bin", "claude.exe")}
	if got := Candidates(windows.env()); !reflect.DeepEqual(got, want) {
		t.Errorf("windows\n got %q\nwant %q", got, want)
	}
	if got := Candidates((fakeEnv{goos: "windows", homeErr: true}).env()); len(got) != 0 {
		t.Errorf("windows without APPDATA or home: %q", got)
	}
}

func TestLocate(t *testing.T) {
	f := fakeEnv{goos: "darwin", home: "/Users/u", files: map[string]info{
		filepath.Join("/Users/u", ".local", "bin", "claude"):    {dir: true},
		filepath.Join("/Users/u", ".claude", "local", "claude"): {mode: 0o644},
		"/usr/local/bin/claude":                                 {mode: 0o755},
	}}
	if got, err := Locate(f.env()); err != nil || got != "/usr/local/bin/claude" {
		t.Errorf("Locate = %q, %v", got, err)
	}
	win := fakeEnv{goos: "windows", vars: map[string]string{"APPDATA": `C:\A`}, homeErr: true,
		files: map[string]info{filepath.Join(`C:\A`, "npm", "claude.cmd"): {mode: 0o644}}}
	if got, err := Locate(win.env()); err != nil || got != filepath.Join(`C:\A`, "npm", "claude.cmd") {
		t.Errorf("windows needs no exec bit: %q, %v", got, err)
	}
	if _, err := Locate((fakeEnv{goos: "linux", homeErr: true}).env()); !errors.Is(err, ErrNotFound) {
		t.Errorf("not found: %v", err)
	}
}

func TestWithPath(t *testing.T) {
	tests := []struct {
		env  []string
		goos string
		want []string
	}{
		{[]string{"A=1", "PATH=/usr/bin"}, "darwin", []string{"A=1", "PATH=/d:/usr/bin"}},
		{[]string{"A=1"}, "linux", []string{"A=1", "PATH=/d"}},
		{[]string{"Path=C:\\w", "PATH=dup"}, "windows", []string{"Path=/d;C:\\w"}},
		{[]string{"Path=x"}, "linux", []string{"Path=x", "PATH=/d"}},
	}
	for _, tt := range tests {
		if got := WithPath(tt.env, "/d", tt.goos); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("WithPath(%q, %s) = %q, want %q", tt.env, tt.goos, got, tt.want)
		}
	}
}

func TestRefresher(t *testing.T) {
	located := fakeEnv{goos: "darwin", homeErr: true, files: map[string]info{"/usr/local/bin/claude": {mode: 0o755}}}
	var gotDir, gotPath string
	var gotArgs, gotEnv []string
	var hasDeadline, cleaned bool
	execErr := error(nil)
	r := &Refresher{
		Env: located.env(),
		Exec: func(ctx context.Context, dir, path string, args, env []string) error {
			_, hasDeadline = ctx.Deadline()
			gotDir, gotPath, gotArgs, gotEnv = dir, path, args, env
			return execErr
		},
		TempDir: func() (string, func(), error) { return "/tmp/empty", func() { cleaned = true }, nil },
		Environ: func() []string { return []string{"PATH=/usr/bin"} },
		Timeout: time.Minute,
	}
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotDir != "/tmp/empty" || gotPath != "/usr/local/bin/claude" || !reflect.DeepEqual(gotArgs, Args) ||
		!reflect.DeepEqual(gotEnv, []string{"PATH=/usr/local/bin:/usr/bin"}) || !hasDeadline || !cleaned {
		t.Errorf("exec got dir=%q path=%q args=%q env=%q deadline=%v cleaned=%v", gotDir, gotPath, gotArgs, gotEnv, hasDeadline, cleaned)
	}

	execErr = errors.New("exit 1: output with account@example.com")
	if err := r.Refresh(context.Background()); err != ErrFailed {
		t.Errorf("a failing command must give the fixed error, got %v", err)
	}

	r.TempDir = func() (string, func(), error) { return "", nil, errors.New("disk full") }
	if err := r.Refresh(context.Background()); err != ErrFailed {
		t.Errorf("no temp dir: %v", err)
	}

	r.Env = (fakeEnv{goos: "linux", homeErr: true}).env()
	if err := r.Refresh(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Errorf("no claude: %v", err)
	}
	if !strings.Contains(strings.Join(Args, " "), "--no-session-persistence") {
		t.Error("the refresh request must not be saved as a session")
	}
}

func TestNewRefresher(t *testing.T) {
	r := NewRefresher()
	if r.Exec == nil || r.Environ == nil || r.Timeout != 2*time.Minute || r.Env.Stat == nil {
		t.Fatalf("defaults: %+v", r)
	}
	dir, cleanup, err := r.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("temp dir: %v", err)
	}
	cleanup()
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("cleanup must remove %s", dir)
	}
	if e := DefaultEnv(); e.GOOS == "" || e.Getenv == nil || e.HomeDir == nil || e.Glob == nil || e.LookPath == nil {
		t.Errorf("DefaultEnv: %+v", e)
	}

	missing := filepath.Join(t.TempDir(), "does", "not", "exist")
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, missing)
	}
	if _, _, err := r.TempDir(); err == nil {
		t.Error("an unusable temp directory must be an error")
	}
}

// TestHelperProcess is the child process started by TestRun.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("PACENOTCH_HELPER")
	if mode == "" {
		return
	}
	os.WriteFile("ran", []byte("yes"), 0o600)
	fmt.Println("output that must be discarded")
	if mode == "fail" {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestRun(t *testing.T) {
	args := []string{"-test.run=^TestHelperProcess$"}
	dir := t.TempDir()
	if err := Run(context.Background(), dir, os.Args[0], args, append(os.Environ(), "PACENOTCH_HELPER=ok")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ran")); err != nil {
		t.Errorf("the command must run in dir: %v", err)
	}
	if err := Run(context.Background(), t.TempDir(), os.Args[0], args, append(os.Environ(), "PACENOTCH_HELPER=fail")); err == nil {
		t.Error("a failing command must return an error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, t.TempDir(), os.Args[0], args, append(os.Environ(), "PACENOTCH_HELPER=ok")); err == nil {
		t.Error("a cancelled context must stop the command")
	}
}
