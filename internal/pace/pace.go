// Package pace turns usage windows into display rows: the linear pace, the status
// against it, the budget and the projection. Everything here is a pure function and
// must match the reference script's jq program number for number.
package pace

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/carlok/pacenotch/internal/usage"
)

// Class is the status of a window against its linear pace.
type Class string

const (
	Ahead Class = "ahead" // used more than pace + band
	On    Class = "on"    // within the band
	Under Class = "under" // used less than pace - band
	Idle  Class = "idle"  // no active window: no elapsed time, no notch
)

// Window lengths in seconds.
const (
	FiveHour = 18000
	SevenDay = 604800
)

// Row is one window ready for display. Percentages ending in BP are hundredths of a
// percent (basis points), which the bar renderers use for sub-cell precision.
type Row struct {
	Key    string
	Label  string // "7d all models"
	Short  string // "7d"
	Window int64  // window length in seconds
	UsedBP int    // used percent x100, clamped to 0..10000
	ExpBP  int    // pace (elapsed fraction) x100; -1 when idle: the notch position
	Used   int    // rounded used percent
	Exp    int    // rounded pace percent; -1 when idle
	Delta  int    // rounded used - pace
	Class  Class
	Left   int64  // seconds until reset, clamped to 0..Window; -1 when idle
	Budget string // percent per unit for the rest of the window, one decimal; "" when idle
	Unit   string // "day" or "h"
	Flat   string // percent per unit of an even pace, one decimal
	Proj   int    // projected percent at reset; -1 when not projected
	Hit    int64  // seconds until the limit is hit; -1 when not projected to hit it
}

// WindowLength returns the window length for a key, or 0 for keys that are not windows.
func WindowLength(key string) int64 {
	switch {
	case strings.HasPrefix(key, "five_hour"):
		return FiveHour
	case strings.HasPrefix(key, "seven_day"):
		return SevenDay
	}
	return 0
}

func rank(key string) int {
	switch key {
	case "five_hour":
		return 0
	case "seven_day":
		return 1
	}
	return 2
}

// Rows computes display rows at Unix time now with the given "on pace" band.
func Rows(windows []usage.Window, now int64, band float64) []Row {
	var sel []usage.Window
	for _, w := range windows {
		if WindowLength(w.Key) != 0 {
			sel = append(sel, w)
		}
	}
	sort.SliceStable(sel, func(i, j int) bool { return rank(sel[i].Key) < rank(sel[j].Key) })
	rows := make([]Row, 0, len(sel))
	for _, w := range sel {
		rows = append(rows, row(w, float64(now), band))
	}
	return rows
}

func row(w usage.Window, now, band float64) Row {
	win := float64(WindowLength(w.Key))
	unit, uname := 3600.0, "h"
	if win == SevenDay {
		unit, uname = 86400, "day"
	}
	r := Row{
		Key: w.Key, Label: Label(w.Key), Short: Short(w.Key), Window: int64(win),
		Unit: uname, Flat: F1(100 / (win / unit)),
	}
	u := w.Used
	left := w.Reset - now
	if !w.HasReset || left <= 0 {
		// never started, or already reset since the data was fetched
		u0 := 0.0
		if !w.HasReset {
			u0 = u
		}
		r.UsedBP = clampInt(roundInt(u0*100), 0, 10000)
		r.ExpBP, r.Used, r.Exp, r.Class = -1, roundInt(u0), -1, Idle
		r.Left, r.Proj, r.Hit = -1, -1, -1
		return r
	}
	// Explicit float64 conversions stop the compiler from fusing multiply-add into FMA,
	// which would change the last bits compared with jq.
	el := clamp(win-left, 0, win)
	exp := float64(el / win * 100)
	d := float64(u - exp)
	switch {
	case d > band:
		r.Class = Ahead
	case d >= -band:
		r.Class = On
	default:
		r.Class = Under
	}
	budget := float64((100 - u) / (left / unit))
	r.Hit, r.Proj = -1, -1
	proj, hasProj := 0.0, el >= float64(win*0.05) && u > 0
	if hasProj {
		proj = float64(u / (el / win))
		r.Proj = roundInt(proj)
	}
	if u >= 100 {
		r.Hit = 0
	} else if hasProj && proj > 100 {
		r.Hit = int64(clamp(math.Floor((100-u)/(u/el)), 0, win))
	}
	r.UsedBP = clampInt(roundInt(u*100), 0, 10000)
	r.ExpBP = roundInt(exp * 100)
	r.Used, r.Exp, r.Delta = roundInt(u), roundInt(exp), roundInt(d)
	r.Left = int64(clamp(math.Floor(left), 0, win))
	r.Budget = F1(budget)
	return r
}

// Label is the full name of a window.
func Label(key string) string {
	switch key {
	case "five_hour":
		return "5h session"
	case "seven_day":
		return "7d all models"
	case "seven_day_sonnet":
		return "7d Sonnet"
	case "seven_day_opus":
		return "7d Opus"
	}
	if s, ok := strings.CutPrefix(key, "seven_day_"); ok {
		return "7d " + s
	}
	if s, ok := strings.CutPrefix(key, "five_hour_"); ok {
		return "5h " + s
	}
	return key
}

// Short is the compact name of a window.
func Short(key string) string {
	switch key {
	case "five_hour":
		return "5h"
	case "seven_day":
		return "7d"
	case "seven_day_sonnet":
		return "7d S"
	case "seven_day_opus":
		return "7d O"
	}
	r := []rune(key)
	return string(r[:min(5, len(r))])
}

// Glyph and word describing a status.
func Status(c Class) (glyph, word string) {
	switch c {
	case Ahead:
		return "▲", "slow down"
	case On:
		return "●", "on pace"
	case Idle:
		return "○", "not started"
	}
	return "▼", "room to spare"
}

// WeeklyAhead reports whether "7d all models" is ahead of pace (exit code 2).
func WeeklyAhead(rows []Row) bool {
	for _, r := range rows {
		if r.Short == "7d" && r.Class == Ahead {
			return true
		}
	}
	return false
}

// ExtraFooter is the extra-usage footer line, or "" when extra usage is off.
func ExtraFooter(ex usage.Extra) string {
	if !ex.Enabled {
		return ""
	}
	if !ex.HasUtilization {
		return "extra usage: on"
	}
	return fmt.Sprintf("extra usage: on (%d%% of monthly limit)", roundInt(ex.Utilization))
}

// F1 formats with one decimal the way the reference does: clamp to 0..1e9, round half away
// from zero.
func F1(x float64) string {
	t := math.Round(clamp(x, 0, 1e9) * 10)
	return fmt.Sprintf("%d.%d", int64(t/10), int64(t)%10)
}

// Duration formats seconds as "2d 13h", "4h 5m" or "7m".
func Duration(s int64) string {
	s = max(s, 0)
	d, h, m := s/86400, s%86400/3600, s%3600/60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh", d, h)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func clamp(x, lo, hi float64) float64 { return math.Min(math.Max(x, lo), hi) }

func clampInt(x, lo, hi int) int { return min(max(x, lo), hi) }

// roundInt rounds half away from zero like jq, saturating instead of overflowing.
func roundInt(x float64) int {
	return int(clamp(math.Round(x), -1e15, 1e15))
}
