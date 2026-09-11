package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/carlok/pacenotch/internal/pace"
	"github.com/carlok/pacenotch/internal/usage"
)

var plain = NewStyle(false, false)

func TestBarEighthRemainders(t *testing.T) {
	for rem := 0; rem < 8; rem++ {
		eighths := 2*8 + rem
		ubp := (eighths*10000 + 63) / 64 // w=8: one eighth of a cell is 10000/64 bp
		got := Bar(8, ubp, -1, pace.Under, plain)
		want := "██"
		if rem > 0 {
			want += plain.Part[rem] + strings.Repeat("░", 5)
		} else {
			want += strings.Repeat("░", 6)
		}
		if got != want {
			t.Errorf("rem %d (ubp %d): %q, want %q", rem, ubp, got, want)
		}
	}
}

func TestBarNotch(t *testing.T) {
	tests := []struct {
		name     string
		ubp, ebp int
		want     string
	}{
		{"first cell, inside fill", 5000, 0, "┃████░░░░░"},
		{"first cell, empty bar", 0, 0, "┃░░░░░░░░░"},
		{"last cell", 5000, 10000, "█████░░░░┃"},
		{"last cell, full bar", 10000, 9999, "█████████┃"},
		{"inside fill", 8000, 5000, "█████┃██░░"},
		{"outside fill", 2000, 5000, "██░░░┃░░░░"},
		{"replaces the partial cell", 2500, 2500, "██┃░░░░░░░"},
		{"idle: no notch", 1250, -1, "█▎░░░░░░░░"},
		{"full, no notch", 10000, -1, "██████████"},
	}
	for _, tt := range tests {
		if got := Bar(10, tt.ubp, tt.ebp, pace.On, plain); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestBarNotchAlwaysPresent: for every fill and pace, an active bar has exactly one notch,
// at the pace position, and the bar is exactly w cells wide.
func TestBarNotchAlwaysPresent(t *testing.T) {
	for _, st := range []Style{plain, NewStyle(false, true)} {
		for _, w := range []int{5, 10, 37} {
			for ubp := 0; ubp <= 10000; ubp += 125 {
				for ebp := 0; ebp <= 10000; ebp += 250 {
					cells := []rune(Bar(w, ubp, ebp, pace.Ahead, st))
					if len(cells) != w {
						t.Fatalf("w %d ubp %d ebp %d: %d cells", w, ubp, ebp, len(cells))
					}
					notch := []rune(st.Notch)[0]
					if n := strings.Count(string(cells), st.Notch); n != 1 || cells[min(ebp*w/10000, w-1)] != notch {
						t.Fatalf("w %d ubp %d ebp %d: notch count %d in %q", w, ubp, ebp, n, string(cells))
					}
				}
			}
		}
	}
}

func TestBarColors(t *testing.T) {
	st := NewStyle(true, false)
	const R, fg, bg, empty, mark = "\033[0m", "\033[31m", "\033[41m", "\033[90m", "\033[1;97m"
	if got, want := Bar(4, 5000, 2500, pace.Ahead, st), R+fg+"█"+R+bg+mark+"┃"+R+empty+"░░"+R; got != want {
		t.Errorf("notch inside: %q, want %q", got, want)
	}
	if got, want := Bar(4, 5000, 7500, pace.Ahead, st), R+fg+"██"+R+empty+"░"+R+mark+"┃"+R; got != want {
		t.Errorf("notch outside: %q, want %q", got, want)
	}
	if got, want := Bar(2, 0, -1, pace.Idle, st), R+empty+"░░"+R; got != want {
		t.Errorf("idle: %q, want %q", got, want)
	}
}

func TestBarASCII(t *testing.T) {
	st := NewStyle(false, true)
	tests := []struct {
		ubp, ebp int
		want     string
	}{
		{5500, 5000, "#####|----"},
		{2500, -1, "###-------"},
		{2300, -1, "##--------"},
		{8000, 5000, "#####|##--"},
		{0, 0, "|---------"},
	}
	for _, tt := range tests {
		if got := Bar(10, tt.ubp, tt.ebp, pace.Under, st); got != tt.want {
			t.Errorf("Bar(%d, %d) = %q, want %q", tt.ubp, tt.ebp, got, tt.want)
		}
	}
	for _, s := range []Style{plain, st} {
		if s.Notch == s.Full || s.Notch == s.Blank {
			t.Error("the notch must differ from the fill and the empty cells")
		}
	}
}

func frameFor(t *testing.T, fixture string, cols int, compact bool, res usage.Result) Frame {
	t.Helper()
	snap, err := usage.Parse(readFixture(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	return Frame{
		Program: Program, Cols: cols, Compact: compact, Now: time.Unix(testNow, 0).UTC(), Result: res,
		Rows: pace.Rows(snap.Windows, testNow, 5), Extra: pace.ExtraFooter(snap.Extra),
	}
}

func TestRenderHeaderStates(t *testing.T) {
	tests := []struct {
		res  usage.Result
		want string
	}{
		{usage.Result{Age: -1, From: "x.json"}, "from x.json \n"},
		{usage.Result{Age: 42}, "data 42s old \n"},
		{usage.Result{Age: 3700, Stale: true, Err: "network error"}, "STALE 1h 1m old: network error \n"},
	}
	for _, tt := range tests {
		out := Render(frameFor(t, "api-sample", 90, false, tt.res), plain)
		first := strings.SplitN(out, "\n", 2)[0] + "\n"
		if !strings.HasPrefix(first, " pacenotch   Fri 11 Sep 11:30 ") || !strings.HasSuffix(first, tt.want) {
			t.Errorf("header %q, want suffix %q", first, tt.want)
		}
	}
	colored := Render(frameFor(t, "api-sample", 90, false, usage.Result{Stale: true, Age: 5, Err: "x"}), NewStyle(true, false))
	if !strings.Contains(colored, "\033[33mSTALE 0m old: x\033[0m ") {
		t.Error("the stale header must be yellow")
	}
}

func TestRenderWrapsNarrowHeader(t *testing.T) {
	out := Render(frameFor(t, "api-sample", 60, false, usage.Result{Age: -1, From: "testdata/fixtures/a-very-long-file-name.json"}), plain)
	lines := strings.Split(out, "\n")
	if lines[0] != " pacenotch   Fri 11 Sep 11:30" || lines[1] != "  from testdata/fixtures/a-very-long-file-name.json " {
		t.Errorf("wrapped header:\n%q\n%q", lines[0], lines[1])
	}
}

func TestRenderFooters(t *testing.T) {
	res := usage.Result{Age: 1, Stale: true, Err: "HTTP 429", Warn: usage.WarnExpired}
	full := Render(frameFor(t, "extra-enabled", 90, false, res), plain)
	compact := Render(frameFor(t, "extra-enabled", 90, true, res), plain)
	for _, want := range []string{" extra usage: on (38% of monthly limit)\n", " " + usage.WarnExpired + "\n"} {
		if !strings.Contains(full, want) || !strings.Contains(compact, want) {
			t.Errorf("missing footer %q", want)
		}
	}
	if strings.Contains(full, "stale data:") || !strings.HasSuffix(compact, " stale data: HTTP 429\n") {
		t.Error("the stale footer belongs to the compact view only")
	}
	if got := Render(Frame{Cols: 80}, plain); got != NoWindows+"\n" {
		t.Errorf("no rows: %q", got)
	}
}

// TestRenderMargin counts runes: every line leaves the last column free. Two exceptions are
// inherited from the reference: "budget ... at this rate ..." stays on one line when it
// fits in exactly the full width, and the fixed "no usage windows" message is not wrapped.
func TestRenderMargin(t *testing.T) {
	for _, fx := range fixtureNames(t) {
		for cols := 40; cols <= 200; cols++ {
			for _, compact := range []bool{false, true} {
				if !compact && cols < 60 {
					continue
				}
				out := Render(frameFor(t, fx, cols, compact, usage.Result{Age: -1, From: "-"}), plain)
				for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
					n := utf8.RuneCountInString(line)
					budgetLine := strings.HasPrefix(line, "   budget ") && strings.Contains(line, ")   ")
					if line == NoWindows {
						continue
					}
					if n > cols || (n == cols && !budgetLine) {
						t.Fatalf("%s cols=%d compact=%v: %d runes in %q", fx, cols, compact, n, line)
					}
				}
			}
		}
	}
}
