// Package tui is the terminal renderer and watch mode. With --color never and a fixed
// width its output is byte-for-byte the reference script's output.
package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/carlok/pacenotch/internal/pace"
	"github.com/carlok/pacenotch/internal/usage"
)

// Style holds the escape sequences and the bar characters. Without color every escape is "".
type Style struct {
	Reset, Bold, Dim string
	FG, BG           map[pace.Class]string
	Empty, Mark      string

	Full, Blank, Notch string
	Part               [8]string // Part[n] is n/8 of a cell
}

// NewStyle returns the reference colors and block characters, or ASCII characters.
func NewStyle(color, ascii bool) Style {
	s := Style{
		FG: map[pace.Class]string{}, BG: map[pace.Class]string{},
		Full: "█", Blank: "░", Notch: "┃",
		Part: [8]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"},
	}
	if ascii {
		// the notch "|" stays distinct from both "#" and "-"
		s.Full, s.Blank, s.Notch = "#", "-", "|"
		s.Part = [8]string{"", "-", "-", "-", "#", "#", "#", "#"}
	}
	if color {
		s.Reset, s.Bold, s.Dim = "\033[0m", "\033[1m", "\033[2m"
		s.FG = map[pace.Class]string{pace.Ahead: "\033[31m", pace.On: "\033[33m", pace.Under: "\033[32m", pace.Idle: "\033[36m"}
		s.BG = map[pace.Class]string{pace.Ahead: "\033[41m", pace.On: "\033[43m", pace.Under: "\033[42m", pace.Idle: "\033[46m"}
		s.Empty, s.Mark = "\033[90m", "\033[1;97m"
	}
	return s
}

// Bar draws a horizontal bar w cells wide, filled to usedBP (hundredths of a percent) with
// 1/8-cell precision, and a vertical pace notch at expBP (no notch when expBP < 0).
// A notch inside the fill keeps the status color as its background.
func Bar(w, usedBP, expBP int, cls pace.Class, st Style) string {
	eighths := usedBP * w * 8 / 10000
	full, rem := eighths/8, eighths%8
	if full >= w {
		full, rem = w, 0
	}
	mark := -1
	if expBP >= 0 {
		mark = min(expBP*w/10000, w-1)
	}
	var b strings.Builder
	prev := ""
	for i := 0; i < w; i++ {
		var kind, ch string
		switch {
		case i == mark:
			kind, ch = "notch-out", st.Notch
			if i < full {
				kind = "notch-in"
			}
		case i < full:
			kind, ch = "fill", st.Full
		case i == full && rem > 0:
			kind, ch = "fill", st.Part[rem]
		default:
			kind, ch = "empty", st.Blank
		}
		if kind != prev {
			switch kind {
			case "fill":
				b.WriteString(st.Reset + st.FG[cls])
			case "empty":
				b.WriteString(st.Reset + st.Empty)
			case "notch-in":
				b.WriteString(st.Reset + st.BG[cls] + st.Mark)
			case "notch-out":
				b.WriteString(st.Reset + st.Mark)
			}
			prev = kind
		}
		b.WriteString(ch)
	}
	b.WriteString(st.Reset)
	return b.String()
}

// Frame is everything one screen depends on.
type Frame struct {
	Program string
	Cols    int
	Compact bool
	Now     time.Time
	Result  usage.Result
	Rows    []pace.Row
	Extra   string
}

// NoWindows is printed when the response has no usage windows.
const NoWindows = "no usage windows in response (not a Pro/Max plan, or no activity yet)"

// Render returns the full output for one frame, ending with a newline.
func Render(f Frame, st Style) string {
	if len(f.Rows) == 0 {
		return NoWindows + "\n"
	}
	var b strings.Builder
	res := f.Result
	if !f.Compact {
		now := f.Now.Format("Mon 02 Jan 15:04")
		lp := " " + f.Program + "   " + now
		lc := " " + st.Bold + f.Program + st.Reset + "   " + now
		var rp string
		switch {
		case res.Age < 0:
			rp = "from " + res.From
		case res.Stale:
			rp = "STALE " + pace.Duration(res.Age) + " old: " + res.Err
		default:
			rp = fmt.Sprintf("data %ds old", res.Age)
		}
		rc := st.Dim + rp + st.Reset
		if res.Stale {
			rc = st.FG[pace.On] + rp + st.Reset
		}
		lr(&b, f.Cols, lp, lc, rp+" ", rc+" ")
		b.WriteString("\n")
	}

	for _, r := range f.Rows {
		fg := st.FG[r.Class]
		glyph, word := pace.Status(r.Class)
		sd := strconv.Itoa(r.Delta)
		if r.Delta > 0 {
			sd = "+" + sd
		}
		u, e := strconv.Itoa(r.Used), strconv.Itoa(r.Exp)

		if f.Compact {
			tail := u + "%/" + e + "% " + sd
			if r.Class == pace.Idle {
				tail = u + "% idle"
			}
			w := max(f.Cols-6-2-utf8.RuneCountInString(tail)-3, 5)
			fmt.Fprintf(&b, "%-5s %s  %s%s%s %s%s%s\n", r.Short, Bar(w, r.UsedBP, r.ExpBP, r.Class, st),
				fg, tail, st.Reset, fg, glyph, st.Reset)
			continue
		}

		barW := max(f.Cols-2, 10)
		if r.Class == pace.Idle {
			lp := " " + r.Label + "  " + u + "% used  * " + word
			lc := " " + st.Bold + r.Label + st.Reset + "  " + fg + st.Bold + u + "%" + st.Reset + " used  " + fg + glyph + " " + word + st.Reset
			rp := "starts with your next message"
			lr(&b, f.Cols, lp, lc, rp+" ", st.Dim+rp+st.Reset+" ")
			b.WriteString(" " + Bar(barW, r.UsedBP, r.ExpBP, r.Class, st) + "\n")
			b.WriteString(st.Dim + "   full budget available, no active window" + st.Reset + "\n\n")
			continue
		}

		lp := " " + r.Label + "  " + u + "% used  pace " + e + "%  (" + sd + ")  * " + word
		lc := " " + st.Bold + r.Label + st.Reset + "  " + fg + st.Bold + u + "%" + st.Reset +
			" used  pace " + e + "%  (" + sd + ")  " + fg + glyph + " " + word + st.Reset
		rp := "resets in " + pace.Duration(r.Left)
		lr(&b, f.Cols, lp, lc, rp+" ", st.Dim+rp+st.Reset+" ")
		b.WriteString(" " + Bar(barW, r.UsedBP, r.ExpBP, r.Class, st) + "\n")

		pline := "   budget " + r.Budget + "%/" + r.Unit + " for " + pace.Duration(r.Left) +
			" (flat pace " + r.Flat + "%/" + r.Unit + ")"
		var pp, pc string
		switch {
		case r.Hit >= 0 && r.Hit < r.Left:
			pp = "   at this rate: limit in " + pace.Duration(r.Hit) + ", " + pace.Duration(r.Left-r.Hit) + " before reset"
			pc = st.FG[pace.On]
			if r.Class == pace.Ahead {
				pc = st.FG[pace.Ahead]
			}
		case r.Proj >= 0:
			pp, pc = "   at this rate: ~"+strconv.Itoa(r.Proj)+"% at reset", st.Dim
		default:
			pp, pc = "   too early in the window to project", st.Dim
		}
		if utf8.RuneCountInString(pline)+utf8.RuneCountInString(pp) <= f.Cols {
			b.WriteString(st.Dim + pline + st.Reset + pc + pp + st.Reset + "\n")
		} else {
			b.WriteString(st.Dim + pline + st.Reset + "\n" + pc + pp + st.Reset + "\n")
		}
		b.WriteString("\n")
	}

	if f.Extra != "" {
		b.WriteString(st.Dim + " " + f.Extra + st.Reset + "\n")
	}
	if res.Warn != "" {
		b.WriteString(st.FG[pace.On] + " " + res.Warn + st.Reset + "\n")
	}
	if f.Compact && res.Stale {
		b.WriteString(st.FG[pace.On] + " stale data: " + res.Err + st.Reset + "\n")
	}
	return b.String()
}

// lr writes a left/right aligned line that leaves one free column. lp and rp are the plain
// texts used for measuring; lc and rc are what gets printed. When both do not fit, the right
// part goes on its own indented line.
func lr(b *strings.Builder, cols int, lp, lc, rp, rc string) {
	pad := cols - 1 - utf8.RuneCountInString(lp) - utf8.RuneCountInString(rp)
	if pad >= 2 {
		b.WriteString(lc + strings.Repeat(" ", pad) + rc + "\n")
	} else {
		b.WriteString(lc + "\n  " + rc + "\n")
	}
}
