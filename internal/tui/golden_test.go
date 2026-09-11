package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/golden from the current output")

// GoldenVariants are the argument sets rendered for every fixture. The e2e parity tests
// run the same sets against the reference script.
var GoldenVariants = [][]string{
	{"--color", "never", "--width", "40"},
	{"--color", "never", "--width", "59"},
	{"--color", "never", "--width", "60"},
	{"--color", "never", "--width", "90"},
	{"--color", "never", "--width", "200"},
	{"--color", "never", "--width", "90", "-c"},
	{"--color", "never", "--width", "90", "-b", "10"},
	{"--color", "always", "--width", "40"},
	{"--color", "always", "--width", "90"},
	{"--ascii", "--color", "never", "--width", "60"},
	{"--ascii", "--color", "always", "--width", "40"},
}

// TestGolden renders every fixture with every variant and compares with testdata/golden.
// Run `make golden` (go test ./internal/tui -run Golden -update) to regenerate, then review
// the diff.
func TestGolden(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			var got strings.Builder
			for _, v := range GoldenVariants {
				h := newHarness(t, append([]string{"--from", "-"}, v...)...)
				h.deps.Stdin = strings.NewReader(string(readFixture(t, name)))
				code := h.run()
				fmt.Fprintf(&got, "=== pacenotch --from - %s (exit %d)\n%s", strings.Join(v, " "), code, h.stdout.String())
				if h.stderr.String() != "" {
					t.Errorf("%v: stderr %q", v, h.stderr)
				}
			}
			path := filepath.Join("..", "..", "testdata", "golden", name+".txt")
			if *update {
				os.MkdirAll(filepath.Dir(path), 0o755)
				if err := os.WriteFile(path, []byte(got.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run make golden)", err)
			}
			if got.String() != string(want) {
				t.Errorf("output differs from %s (run make golden and review the diff)\n%s", path, firstDiff(string(want), got.String()))
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
			return fmt.Sprintf("line %d\nwant %q\n got %q", i+1, wl, gl)
		}
	}
	return ""
}
