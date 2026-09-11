// Package e2e runs the pacenotch binary: parity with the reference script, exit codes, the
// cache paths and watch mode. With PACENOTCH_E2E_BIN set (by scripts/cover.sh) it tests that
// coverage-instrumented binary; otherwise it builds one.
package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const now = 1789126200 // Fri 11 Sep 2026 11:30:00 UTC

// variants mirrors tui.GoldenVariants minus --ascii, which the reference does not have.
var variants = [][]string{
	{"--color", "never", "--width", "40"},
	{"--color", "never", "--width", "59"},
	{"--color", "never", "--width", "60"},
	{"--color", "never", "--width", "90"},
	{"--color", "never", "--width", "200"},
	{"--color", "never", "--width", "90", "-c"},
	{"--color", "never", "--width", "90", "-b", "10"},
	{"--color", "always", "--width", "40"},
	{"--color", "always", "--width", "90"},
}

var bin string

func TestMain(m *testing.M) {
	bin = os.Getenv("PACENOTCH_E2E_BIN")
	cleanup := func() {}
	if bin == "" {
		dir, err := os.MkdirTemp("", "pacenotch-e2e")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cleanup = func() { os.RemoveAll(dir) }
		bin = filepath.Join(dir, "pacenotch")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		build := exec.Command("go", "build", "-o", bin, "./cmd/pacenotch")
		build.Dir = ".."
		build.Stdout, build.Stderr = os.Stderr, os.Stderr
		if err := build.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "building pacenotch:", err)
			cleanup()
			os.Exit(1)
		}
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
}

// env is the parent environment without anything that changes the output, plus overrides.
func env(overrides map[string]string) []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(k, "PACENOTCH_"), strings.HasPrefix(k, "LC_"), k == "LANG", k == "TZ",
			k == "NO_COLOR", k == "COLUMNS", k == "TERM", k == "CLAUDE_PACE_TOKEN":
			continue
		}
		if _, over := overrides[k]; !over {
			out = append(out, kv)
		}
	}
	out = append(out, "LC_ALL=C", "TZ=UTC", "PACENOTCH_NOW="+strconv.Itoa(now))
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

func run(t *testing.T, name string, args []string, stdin []byte, overrides map[string]string) result {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = ".."
	cmd.Env = env(overrides)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return result{stdout.String(), stderr.String(), code}
}

func fixtures(t *testing.T) map[string][]byte {
	files, _ := filepath.Glob(filepath.Join("..", "testdata", "fixtures", "*.json"))
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	out := map[string][]byte{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = b
	}
	return out
}

// TestParity: same input, same now, same flags → the same stdout and exit code as the
// reference script (patched only to read PACENOTCH_NOW and print "pacenotch").
func TestParity(t *testing.T) {
	for _, tool := range []string{"bash", "jq", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("parity needs %s: %v", tool, err)
		}
	}
	reference := filepath.Join("testdata", "reference.sh")
	for name, data := range fixtures(t) {
		t.Run(name, func(t *testing.T) {
			for _, v := range variants {
				for _, from := range []string{"-", "testdata/fixtures/" + name + ".json"} {
					args := append([]string{"--from", from}, v...)
					want := run(t, "bash", append([]string{reference}, args...), data, nil)
					got := run(t, bin, args, data, nil)
					if got.stdout != want.stdout || got.code != want.code {
						t.Errorf("pacenotch %s\nexit %d, reference exit %d\n%s", strings.Join(args, " "), got.code, want.code,
							firstDiff(want.stdout, got.stdout))
					}
				}
			}
		})
	}
}

func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("line %d\nreference %q\npacenotch %q", i+1, wl, gl)
		}
	}
	return "(identical stdout)"
}

func TestExitCodes(t *testing.T) {
	fx := fixtures(t)
	tests := []struct {
		name   string
		args   []string
		stdin  []byte
		code   int
		stdout string
		stderr string
	}{
		{"weekly ahead", []string{"--from", "-"}, fx["api-sample"], 2, "7d all models", ""},
		{"fine", []string{"--from", "-"}, fx["zero"], 0, "5h session", ""},
		{"no windows", []string{"--from", "-"}, fx["empty"], 0, "no usage windows", ""},
		{"invalid JSON", []string{"--from", "testdata/robust/truncated.json"}, nil, 1, "", "pacenotch: response is not valid JSON\n"},
		{"missing file", []string{"--from", "testdata/missing.json"}, nil, 1, "", "pacenotch: cannot read testdata/missing.json\n"},
		{"unknown flag", []string{"--bogus"}, nil, 1, "", "pacenotch: unknown option: --bogus (try --help)\n"},
		{"help", []string{"-h"}, nil, 0, "The notch ┃ marks an even pace", ""},
		{"gui in the CLI-only build", []string{"gui"}, nil, 1, "", "pacenotch: this is the CLI-only build; `pacenotch gui` needs the GUI build\n"},
		{"raw", []string{"--raw", "--from", "-"}, []byte(`{"a":1}`), 0, "{\n  \"a\": 1\n}\n", ""},
		{"ascii", []string{"--ascii", "--from", "-", "--width", "60", "--color", "never"}, fx["api-sample"], 2, "#####|", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := run(t, bin, tt.args, tt.stdin, nil)
			if r.code != tt.code || !strings.Contains(r.stdout, tt.stdout) || r.stderr != tt.stderr {
				t.Errorf("exit %d (want %d)\nstdout %q\nstderr %q", r.code, tt.code, r.stdout, r.stderr)
			}
		})
	}
}

// isolated points HOME and the cache at a temp dir and hides the macOS `security` command,
// so the binary can never reach real credentials or the real endpoint.
func isolated(t *testing.T) (map[string]string, string) {
	home := t.TempDir()
	emptyPath := t.TempDir()
	o := map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CACHE_HOME": filepath.Join(home, "cache"),
		"LocalAppData": filepath.Join(home, "cache"), "LOCALAPPDATA": filepath.Join(home, "cache"), "PATH": emptyPath,
	}
	var cache string
	switch runtime.GOOS {
	case "darwin":
		cache = filepath.Join(home, "Library", "Caches", "pacenotch", "usage.json")
	default:
		cache = filepath.Join(home, "cache", "pacenotch", "usage.json")
	}
	return o, cache
}

func seedCache(t *testing.T, path string, data []byte, age time.Duration) {
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.Unix(now, 0).Add(-age)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestCachePaths(t *testing.T) {
	fx := fixtures(t)

	o, _ := isolated(t)
	r := run(t, bin, nil, nil, o)
	if r.code != 1 || r.stderr != "pacenotch: no Claude Code credentials found (run: claude login)\n" {
		t.Errorf("no credentials, no cache: %+v", r)
	}

	o, cache := isolated(t)
	seedCache(t, cache, fx["api-sample"], 10*time.Second)
	r = run(t, bin, []string{"--width", "90", "--color", "never"}, nil, o)
	if r.code != 2 || !strings.Contains(r.stdout, "data 10s old") {
		t.Errorf("fresh cache: %+v", r)
	}

	seedCache(t, cache, fx["api-sample"], time.Hour)
	r = run(t, bin, []string{"--width", "120", "--color", "never"}, nil, o)
	if r.code != 2 || !strings.Contains(r.stdout, "STALE 1h 0m old: no Claude Code credentials found (run: claude login)") {
		t.Errorf("stale cache: %+v", r)
	}
	r = run(t, bin, []string{"--raw"}, nil, o)
	if r.code != 0 || !strings.Contains(r.stdout, `"five_hour": {`) {
		t.Errorf("raw from stale cache: %+v", r)
	}
}

func TestWatchInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt cannot be sent to a process on Windows")
	}
	cmd := exec.Command(bin, "-w", "1", "--from", "testdata/fixtures/api-sample.json", "--width", "90", "--color", "never")
	cmd.Dir = ".."
	cmd.Env = env(nil)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	buf := make([]byte, 4096)
	deadline := time.Now().Add(10 * time.Second)
	for strings.Count(out.String(), "\033[H\033[2J") < 2 && time.Now().Before(deadline) {
		n, err := stdout.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			break
		}
	}
	cmd.Process.Signal(os.Interrupt)
	rest, _ := io.ReadAll(stdout)
	out.Write(rest)
	err = cmd.Wait()
	s := out.String()
	if err != nil || !strings.HasPrefix(s, "\033[?25l\033[H\033[2J pacenotch   Fri 11 Sep 11:30") || !strings.HasSuffix(s, "\033[?25h\n") {
		t.Errorf("watch: err %v, output %q", err, s)
	}
	if strings.Count(s, "\033[H\033[2J") < 2 {
		t.Error("watch must redraw on its interval without new data")
	}
}
